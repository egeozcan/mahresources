package plugin_system

import (
	lua "github.com/yuin/gopher-lua"
)

// TransactionBinding is the host object every call inside a mah.db.transaction
// runs through: the same adapter the host hands out normally, but bound to the
// open transaction.
//
// One object rather than four interfaces threaded separately, because the
// reason they travel together is physical rather than tidy. They all write to
// the database, and a write that goes out on a *second* connection while the
// transaction holds the first does not join it — it blocks on the writer lock
// until the connection's busy_timeout gives up, or deadlocks outright at
// -max-db-connections=1. mah.kv and mah.log are in here for exactly that
// reason: neither goes through BindInvocation, so both would otherwise write on
// the singleton's handle. The same argument is already recorded at
// application_context/relation_context.go:511 and plugin_scoped_access.go:48.
//
// The consequence is deliberate and worth stating: a plugin's stored data and
// its log lines roll back with the transaction, because they were part of it.
type TransactionBinding interface {
	EntityQuerier
	EntityWriter
	PluginLogger
	KVStore
}

// TransactionRunner opens a database transaction for a plugin. The host
// implements it, and must call body exactly once with an Invocation bound to
// the open transaction, commit when body returns nil, and roll back otherwise.
//
// It is discovered by type-asserting the PrincipalBinder rather than wired
// through a setter of its own: the host object that can bind a principal is the
// same one that owns the database handle, so a deployment that has one has the
// other, and a test double that implements only the binder degrades to "not
// available" instead of to a nil dereference.
type TransactionRunner interface {
	RunInTransaction(inv *Invocation, body func(txInv *Invocation) error) error
}

// transactionRunnerFor returns the object that opens a transaction for this
// call.
//
// When one is already open it is that transaction's OWN binding, so a nested
// mah.db.transaction becomes a SAVEPOINT on it. Going back to the process-wide
// binder there would open a second transaction on a second connection, which is
// both non-atomic and, on SQLite, a block on the writer lock the first is
// holding.
//
// The savepoint matters most across plugins, which is also where it is least
// visible: a before-hook fires inside the *triggering* plugin's transaction, so
// a plugin that wraps its own work in mah.db.transaction may be nesting without
// knowing it. A savepoint gives it what it asked for — its own writes roll back
// on its own failure — without letting it discard the caller's.
func (pm *PluginManager) transactionRunnerFor(inv *Invocation) TransactionRunner {
	if tx := inv.transactionBinding(); tx != nil {
		runner, _ := tx.(TransactionRunner)
		return runner
	}
	runner, _ := pm.getPrincipalBinder().(TransactionRunner)
	return runner
}

// errNoTransactionRunner is what a plugin sees when the host cannot open a
// transaction. It is phrased as a deployment fact rather than a bug because
// that is what it is: every wired-up context installs an adapter that
// implements TransactionRunner (application_context/context.go), so this is
// reachable only from a host that deliberately does not.
const errNoTransactionRunner = "database transactions are not available"

// runDbTransaction implements mah.db.transaction(fn). It returns the number of
// values pushed, following this module's convention: true on commit, (nil,
// error) on rollback.
func (pm *PluginManager) runDbTransaction(L *lua.LState, fn *lua.LFunction) int {
	// Refused from a coroutine, for two reasons that point the same way.
	// gopher-lua cannot yield across a Go call boundary, and the callback is
	// invoked from Go — a yield inside it does not suspend the transaction, it
	// abandons the frame (observably: the whole render returns empty and the
	// error is swallowed). And a transaction that *could* suspend would hold the
	// database write lock until something resumed it, which is the same rule
	// that refuses mah.sleep. Better a refusal that says so than a silent one.
	if L != mainState(L) {
		L.Push(lua.LNil)
		L.Push(lua.LString("mah.db.transaction cannot be called from a coroutine: its callback " +
			"runs from Go, which a coroutine cannot yield across, and a transaction that could " +
			"suspend would hold the database write lock until something resumed it"))
		return 2
	}

	inv := pm.invocationFor(L)

	runner := pm.transactionRunnerFor(inv)
	if runner == nil {
		if !inv.InTransaction() {
			L.Push(lua.LNil)
			L.Push(lua.LString(errNoTransactionRunner))
			return 2
		}
		// A transaction is open but its binding cannot nest one. Running the
		// body inline is the safe answer rather than the complete one: the
		// writes still land inside the open transaction, so "all or nothing"
		// holds at that boundary — this inner one just cannot fail on its own.
		if err := pm.callTransactionBody(L, fn); err != nil {
			return pm.pushTransactionError(L, err)
		}
		L.Push(lua.LTrue)
		return 1
	}

	err := runner.RunInTransaction(inv, func(txInv *Invocation) error {
		// Save and restore rather than SetContext/RemoveContext, which is what
		// every VM entry point does and what would be wrong here. The context is
		// a single slot holding the deadline, the request to cancel against, the
		// per-request MRQL cache, the acting user AND the re-entry chain; every
		// entry point clears it unconditionally on the way out because it owns
		// it. This call does not own it — it is running inside one — so clearing
		// it would leave the outer frame unattributed and its re-entry guard
		// disarmed, which is the permanent VM wedge described in hooks.go. The
		// pattern is executeSyncHttpRequest's.
		//
		// Installed on mainState because that is where invocationFor reads it;
		// a coroutine's own context is a snapshot taken when it was created.
		root := mainState(L)
		saved := root.Context()
		root.SetContext(withInvocation(saved, txInv))
		defer func() {
			if saved != nil {
				root.SetContext(saved)
			} else {
				root.RemoveContext()
			}
		}()

		return pm.callTransactionBody(L, fn)
	})
	if err != nil {
		return pm.pushTransactionError(L, err)
	}

	// The per-request MRQL cache is invalidated again, after the commit. Every
	// write inside the transaction already invalidated it, but mah.db.mrql_query
	// reads on a separate connection: a query made inside the transaction cached
	// the pre-commit view, and without this that entry outlives the commit and
	// serves the old answer for the rest of the request.
	InvalidateMRQLCache(pm.luaContext(L))

	L.Push(lua.LTrue)
	return 1
}

// callTransactionBody invokes the plugin's callback.
//
// The VM lock is deliberately NOT taken: the frame that called
// mah.db.transaction already holds it, and the mutex is not reentrant — taking
// it again from this goroutine wedges the VM permanently, with no panic and
// nothing in the log. Protect:true for the same reason every other Go→Lua call
// in this package uses it: an error must come back as a value, or the
// transaction unwinds without ever being rolled back.
func (pm *PluginManager) callTransactionBody(L *lua.LState, fn *lua.LFunction) error {
	return L.CallByParam(lua.P{Fn: fn, NRet: 0, Protect: true})
}

// pushTransactionError reports a rolled-back transaction to Lua.
func (pm *PluginManager) pushTransactionError(L *lua.LState, err error) int {
	// mah.abort inside the callback must still abort. The transaction has
	// already rolled back by the time we get here, so re-raising costs nothing
	// and preserves the one thing the marker exists for: a before-hook's veto
	// reaching RunBeforeHooks as a veto rather than as a database failure.
	if isAbort, reason := parseAbortError(err); isAbort {
		L.RaiseError("PLUGIN_ABORT: %s", reason)
	}
	L.Push(lua.LNil)
	L.Push(lua.LString(err.Error()))
	return 2
}

// The rule behind every refusal below: a transaction may not do I/O. There are
// two distinct reasons for that and each call is refused for one of them.
const (
	// whyItWaits — the database write lock is held for as long as the
	// transaction runs. SQLite's busy_timeout is 10s, while mah.http allows 120,
	// mah.sleep 30, and a remote resource fetch 30 minutes; every other writer
	// in the process fails with "database is locked" long before those return.
	whyItWaits = "it waits on the network or the filesystem, and a transaction must not hold " +
		"the database write lock across I/O"

	// whyItCannotBeUndone — the filesystem has no rollback. DeleteResource
	// removes the file once *its own* writes commit, which inside a transaction
	// is only a savepoint: a later failure puts the row back and the bytes are
	// still gone. That is worse than refusing, because it is silent.
	whyItCannotBeUndone = "it deletes the resource's file as soon as its own writes commit, and " +
		"rolling the transaction back restores the row but not the bytes"

	// whyItLeavesTheProcess — the same rule as whyItCannotBeUndone, for the
	// asynchronous HTTP calls. They hold no lock, so they are not the write-lock
	// problem; they are the other one. The request is on the wire the moment it
	// is made, and rolling the transaction back does not recall a POST to
	// somebody's webhook or payment API.
	whyItLeavesTheProcess = "it sends the request immediately and a rollback cannot recall it; " +
		"make the request first, or after the transaction returns"
)

// refusedInTransaction is the message a call that must not run inside a
// transaction returns.
func refusedInTransaction(call, why string) string {
	return call + " cannot run inside mah.db.transaction: " + why
}

// raiseAsyncHttpRefusal refuses an asynchronous mah.http call made inside a
// transaction, and raises rather than answering through the callback the way
// every other refusal on that surface does.
//
// The asymmetry is forced by the shape of the API rather than chosen. A
// synchronous call has a return value, so its refusal lands in the frame that
// made it, still inside the transaction. An asynchronous call has only the
// callback — and the callback is queued for the drain goroutine, which runs it
// after the calling hook has returned and released its VM lock, while the
// transaction that started all this is still open. A mah.db write from there
// would run on a second connection outside the transaction: on Postgres it
// commits and survives the rollback, on SQLite it contends with the write lock
// the transaction is holding. Queueing the *refusal* through that same channel
// would build exactly the escape the refusal exists to prevent.
//
// Raising also matches what this is: not a network failure, which the callback
// convention is for, but a call that cannot be made here at all — the same
// answer mah.sleep gives.
func raiseAsyncHttpRefusal(L *lua.LState, call string) int {
	L.RaiseError("%s", refusedInTransaction(call, whyItLeavesTheProcess))
	return 0
}

// inTransaction reports whether a mah.db.transaction is open on the call chain
// L belongs to.
func (pm *PluginManager) inTransaction(L *lua.LState) bool {
	return pm.invocationFor(L).InTransaction()
}
