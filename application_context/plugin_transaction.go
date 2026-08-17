package application_context

import (
	"fmt"

	"mahresources/plugin_system"
)

// deferredPluginHooks holds after-hooks raised while a plugin transaction was
// open, so they can be dispatched once it has committed.
//
// The queue exists because of what an after-hook means. Every entity path in
// this package fires its before-hook before opening a transaction and its
// after-hook after committing, so no plugin Lua has ever run with a host
// transaction open. mah.db.transaction is the first thing that can put one
// there, and an after-hook dispatched inside it would announce a write that can
// still roll back — to a plugin that is free to act on it.
//
// Before-hooks are deliberately NOT deferred: their job is to veto, so they
// have to run before the write they are vetoing.
//
// No mutex. Everything that appends to a queue runs on the goroutine that
// opened the transaction, under the VM lock that goroutine holds — including
// another plugin's hook, which is dispatched synchronously on it.
type deferredPluginHooks struct {
	calls []deferredPluginHook
}

// deferredPluginHook carries the invocation the hook was *raised* with, not just
// the event.
//
// Deferring must delay a notification, never re-address it. The re-entry rule —
// a plugin is not told about a write it made while it was already running — is
// decided from the call chain on the invocation, so draining every queued hook
// through the transaction opener's chain tells a nested plugin about its own
// write. That is a duplicate external effect, and it happens only inside a
// transaction, which is the worst place to discover it.
type deferredPluginHook struct {
	inv   *plugin_system.Invocation
	event string
	data  map[string]any
}

func (q *deferredPluginHooks) add(inv *plugin_system.Invocation, event string, data map[string]any) {
	// Detached here rather than at drain: the chain is what must survive, and
	// the transaction binding is what must not — these hooks run after the
	// commit, so a write through the transaction's handle would be a write
	// through a finished one.
	q.calls = append(q.calls, deferredPluginHook{
		inv:   inv.DetachedFromTransaction(),
		event: event,
		data:  data,
	})
}

// drain dispatches the queued hooks through ctx.
//
// When ctx has a queue of its own the calls are handed to it rather than
// dispatched. That is the nested case, and it is the whole reason a released
// savepoint must not dispatch anything: the outer transaction can still roll
// back, taking the inner block's committed writes with it, and the hooks would
// already have announced them. Forwarding defers the question to whoever
// actually commits.
func (q *deferredPluginHooks) drain(ctx *MahresourcesContext) {
	if q == nil {
		return
	}
	calls := q.calls
	q.calls = nil
	if outer := ctx.deferredHooks; outer != nil {
		outer.calls = append(outer.calls, calls...)
		return
	}
	if ctx.pluginManager == nil {
		return
	}
	for _, call := range calls {
		// Dispatched with the stored invocation rather than ctx.hookInvocation(),
		// which is the opener's. See deferredPluginHook.
		ctx.pluginManager.RunAfterHooks(call.inv, call.event, call.data)
	}
}

// RunInTransaction opens one database transaction for a plugin's
// mah.db.transaction and runs body inside it.
//
// The bind order is principal first, then transaction, and it matters in both
// directions. WithPrincipal rewrites the db handle to attach the subtree filter
// and the acting user, so binding it after the transaction would swap the
// transactional handle back out; and GORM's Begin re-plants Statement.Context
// on the transaction session, so the scope and actor a principal bind installed
// survive it (TestScopedHandle_BeginInheritsScopeAndActor pins that direction).
//
// The invocation handed back carries the transaction-bound adapter, which is
// also stored on the transaction's own context as pluginInvocation — so a hook
// fired by a write inside the transaction is dispatched with it, and that
// plugin's writes join the transaction rather than opening a second connection.
func (a *pluginDBAdapter) RunInTransaction(inv *plugin_system.Invocation, body func(*plugin_system.Invocation) error) error {
	bound := a.boundContext(inv)

	var queue *deferredPluginHooks
	if err := bound.WithTransaction(func(txCtx *MahresourcesContext) error {
		queue = &deferredPluginHooks{}
		txCtx.deferredHooks = queue

		txAdapter := &pluginDBAdapter{ctx: txCtx}
		txInv := inv.BoundToTransaction(txAdapter)
		txCtx.pluginInvocation = txInv

		if err := body(txInv); err != nil {
			return err
		}
		return txCtx.transactionCanStillCommit()
	}); err != nil {
		// Rolled back: the queued hooks describe writes that did not happen.
		return err
	}

	// The search cache is cleared AFTER the commit, and that is a window this
	// feature opened rather than a pre-existing one. Each entity path
	// invalidates its own cache entries at the end of its write — which outside
	// a transaction is after that write committed, and inside one is before
	// anything has. A concurrent search landing in the gap repopulates the cache
	// from the pre-commit state and nothing invalidates it again, so the stale
	// answer survives for the cache's whole TTL.
	//
	// Cleared wholesale rather than by type because the transaction may have
	// touched any of them, and a plugin transaction is a deliberate, occasional
	// thing; the cost of one cold cache is not worth tracking which types were
	// written.
	bound.ClearSearchCache()

	// Committed, so the writes are real and the notifications are due. Drained
	// through bound rather than the transaction's context: bound has no queue,
	// and its handle is no longer inside a transaction, so a hook that writes
	// gets its own.
	queue.drain(bound)
	return nil
}

// transactionCanStillCommit refuses to report success for a transaction that
// cannot commit, and exists for the *nested* case.
//
// Postgres marks a transaction aborted after any failed statement, and mah.db
// hands write failures to Lua as return values rather than raising — so a plugin
// can ignore one and keep writing into a transaction that can no longer commit.
// At the outermost boundary pgx already catches this ("commit unexpectedly
// resulted in rollback"), which is why the probe was written, removed as
// redundant, and then put back: a *nested* transaction is a savepoint, and GORM
// releases a savepoint by doing nothing at all when the callback returns nil.
// There is no commit there for a driver to check. Without this, an inner block
// that ignored a failed write is told `true` while the outer transaction is
// already doomed — and the plugin may act on that answer before it fails,
// through mah.start_job, which does not join the transaction.
//
// One trivial statement, which SQLite always answers (it does not poison a
// transaction) and an aborted Postgres transaction refuses with 25P02. Applied
// at both levels rather than only the nested one, so the two databases and the
// two nesting depths all answer the same question the same way.
func (ctx *MahresourcesContext) transactionCanStillCommit() error {
	var probe int
	if err := ctx.db.Raw("SELECT 1").Scan(&probe).Error; err != nil {
		return fmt.Errorf("transaction can no longer be committed, rolling back: %w", err)
	}
	return nil
}

// Compile-time check: the adapter the host wires in as PrincipalBinder is also
// what mah.db.transaction type-asserts for a runner.
var _ plugin_system.TransactionRunner = (*pluginDBAdapter)(nil)

// A transaction-bound adapter has to be able to stand in for every host surface
// a plugin can reach that writes to the database, or the ones it cannot stand
// in for would write on a second connection and block on the lock this
// transaction holds.
var _ plugin_system.TransactionBinding = (*pluginDBAdapter)(nil)
