package plugin_system

import (
	"context"

	"mahresources/auth"

	lua "github.com/yuin/gopher-lua"
)

// This file is the *only* place in plugin_system that may import mahresources/auth,
// and it deliberately narrows what crosses that edge to a single uint.
//
// The plugin host must know *who* triggered a call so that entities a plugin
// creates are attributed to that user rather than to nobody (see
// docs/plans/2026-08-15-plugin-invocation-and-hook-integrity.md §1). It must not
// know anything else about the principal: a group-confined principal is still
// denied every plugin code path outright by auth.PluginCodeAllowed, and lifting
// that deny is a separate change that has to land behind per-plugin capability
// grants. Letting a *auth.Principal travel further into this package is how that
// deny gets lifted by accident.
//
// internal/arch/plugin_auth_import_test.go fails the build if a second file here
// imports auth.

// invocationCtxKey carries an *Invocation on the context a VM entry point
// installs on its LState.
type invocationCtxKey struct{}

// Invocation identifies who triggered a plugin call and which plugin VMs are
// already executing on the current call chain.
//
// The state set — not a single state — is what makes the re-entry guard correct.
// Hooks are dispatched synchronously on the caller's goroutine, so plugin P can
// write an entity that plugin Q hooks, and Q's hook can write an entity that P
// hooks, arriving back at a VM mutex P's outer frame still holds. Comparing
// against only the immediately-executing VM closes the self-hook case and leaves
// that one deadlocking exactly as permanently. See RunAfterHooks.
//
// An Invocation is immutable once built: with() returns a new value rather than
// appending in place, so a parent's set cannot be mutated by a nested call that
// shares its backing array.
type Invocation struct {
	// ActorUserID is the user whose request or job triggered this call, or 0
	// when there is none (auth off, or a context-less worker path).
	ActorUserID uint

	// JobID is the durable Job this call is executing inside, when it is
	// executing inside one. It is what makes mah.start_job's work a child of the
	// execution that asked for it: a Job Center reader can see that one job
	// started another, and the child inherits nothing else — no authority, no
	// input, no outcome. Empty for a call that no Job is running (an ordinary
	// request, a hook, a shortcode), which is why a start_job from there has no
	// parent rather than a made-up one.
	JobID string

	// JobEventDispatch reports that this call is the *delivery of a terminal job
	// event*: an after_job_completed, after_job_failed or after_job_cancelled
	// hook. A Job started from here must not announce its own terminal event,
	// because the feed would hand that announcement back to the very hook that
	// caused it, which starts another Job, forever. The flag rides the call chain
	// (it is copied by with() like tx is) so a hook that reaches another plugin's
	// VM still counts as being inside the dispatch.
	JobEventDispatch bool

	// Egress is the network policy of the plugin that made this call, when the
	// call can reach the network. Nil for every other call, and for host-side
	// fetches that no plugin triggered — which is what keeps operator-initiated
	// downloads on their existing, unrestricted path.
	Egress *NetworkPolicy

	// tx is the host object every mah.db, mah.kv and mah.log call on this chain
	// must run through while a mah.db.transaction is open, and nil otherwise.
	//
	// It rides here rather than on the LState because the Invocation is the one
	// channel that already spans *several plugins* on a single call chain: a
	// write inside the transaction fires another plugin's hook, and the host
	// hands this Invocation to RunBeforeHooks/RunAfterHooks, which installs it
	// on that plugin's own state. So the hook's writes join the transaction
	// instead of opening a second connection and blocking on the writer lock the
	// transaction is holding. Anything keyed on the LState would have covered
	// only the plugin that opened it.
	tx TransactionBinding

	states []*lua.LState
}

// InTransaction reports whether a mah.db.transaction is open on this call chain.
func (inv *Invocation) InTransaction() bool {
	return inv != nil && inv.tx != nil
}

// BoundToTransaction returns a copy of inv whose database, key-value and log
// calls run through binding. The host calls it once, inside the transaction it
// has just opened, and hands the result back through TransactionRunner.
func (inv *Invocation) BoundToTransaction(binding TransactionBinding) *Invocation {
	if binding == nil {
		return inv
	}
	var copied Invocation
	if inv != nil {
		copied = *inv
	}
	copied.tx = binding
	return &copied
}

// DetachedFromTransaction returns a copy of inv that keeps its actor and its
// call chain but no longer routes through the transaction.
//
// It is what an after-hook deferred to the commit must be dispatched with. The
// chain has to survive, or the plugin whose own write raised the hook stops
// being recognised as the one that made it and gets notified of it. The binding
// must not: by the time the hook runs the transaction has committed, and a
// write through a finished transaction's handle is a write through a dead one.
func (inv *Invocation) DetachedFromTransaction() *Invocation {
	if inv == nil || inv.tx == nil {
		return inv
	}
	copied := *inv
	copied.tx = nil
	return &copied
}

// transactionBinding is the open transaction's host object, or nil.
func (inv *Invocation) transactionBinding() TransactionBinding {
	if inv == nil {
		return nil
	}
	return inv.tx
}

// withEgress returns a copy carrying a plugin's network policy.
func (inv *Invocation) withEgress(policy NetworkPolicy) *Invocation {
	if inv == nil {
		return nil
	}
	copied := *inv
	copied.Egress = &policy
	return &copied
}

// NewInvocation returns an Invocation for a call originating outside any plugin
// VM — an ordinary HTTP request whose entity write happens to fire a hook.
func NewInvocation(actorUserID uint) *Invocation {
	return &Invocation{ActorUserID: actorUserID}
}

// NewJobInvocation returns an Invocation for a call that is executing one
// durable Job, so nested work can name it as its parent.
func NewJobInvocation(actorUserID uint, jobID string) *Invocation {
	return &Invocation{ActorUserID: actorUserID, JobID: jobID}
}

// NewJobEventInvocation returns an Invocation for the delivery of a terminal job
// event — the after_job_* hooks.
//
// It is separate from NewInvocation because a hook that starts work is the one
// place the job-event feed can feed itself: the Job it starts would announce its
// own completion, which is the next delivery of the same hook. The flag says so
// for the whole call chain rather than for one plugin, so a Job accepted from a
// hook is marked as not announcing, whatever plugin ends up asking.
func NewJobEventInvocation(actorUserID uint) *Invocation {
	return &Invocation{ActorUserID: actorUserID, JobEventDispatch: true}
}

// invocationIsJobEventDispatch reports whether a call chain is the delivery of a
// terminal job event.
func invocationIsJobEventDispatch(inv *Invocation) bool {
	return inv != nil && inv.JobEventDispatch
}

// invocationJobID answers the durable Job a call chain is executing, or "".
func invocationJobID(inv *Invocation) string {
	if inv == nil {
		return ""
	}
	return inv.JobID
}

// holds reports whether L is already executing somewhere on this call chain, and
// therefore whether taking its VM lock again would deadlock.
func (inv *Invocation) holds(L *lua.LState) bool {
	if inv == nil {
		return false
	}
	for _, s := range inv.states {
		if s == L {
			return true
		}
	}
	return false
}

// with returns an Invocation whose chain also contains L. The receiver is left
// unchanged, and the result never aliases the receiver's backing array.
func (inv *Invocation) with(L *lua.LState) *Invocation {
	if inv == nil {
		return &Invocation{states: []*lua.LState{L}}
	}
	if inv.holds(L) {
		return inv
	}
	states := make([]*lua.LState, len(inv.states), len(inv.states)+1)
	copy(states, inv.states)
	// tx is carried: a write inside a transaction fires a hook in another
	// plugin, and this is the copy that plugin's mah.db calls read. Dropping it
	// here would silently put the hook's writes on a second connection, which is
	// the whole failure this field exists to prevent. Egress is deliberately not
	// carried — querierForFetch attaches it per call, after this.
	//
	// JobID is carried for the same reason tx is: a hook that fires while one
	// Job's execution is writing is still that execution, and a mah.start_job
	// inside it is that Job's child. Dropping it here would make the relationship
	// depend on which entry point happened to run the Lua.
	//
	// JobEventDispatch is carried for the same reason and to the same end: a
	// plugin reached from the delivery of a terminal job event is still inside
	// that delivery, so work it starts must still not announce.
	return &Invocation{
		ActorUserID: inv.ActorUserID, JobID: inv.JobID, JobEventDispatch: inv.JobEventDispatch,
		tx: inv.tx, states: append(states, L),
	}
}

// withInvocation returns a child context carrying inv, for a VM entry point that
// has no request context to read one from (hooks, async jobs, HTTP callbacks).
func withInvocation(ctx context.Context, inv *Invocation) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if inv == nil {
		return ctx
	}
	return context.WithValue(ctx, invocationCtxKey{}, inv)
}

// invocationFromContext returns an Invocation explicitly installed by an entry
// point, or nil.
func invocationFromContext(ctx context.Context) *Invocation {
	if ctx == nil {
		return nil
	}
	inv, _ := ctx.Value(invocationCtxKey{}).(*Invocation)
	return inv
}

// actorFromContext reads *only* the acting user id from a request context.
// Nothing else about the principal may leave this function.
//
// The auth-disabled system principal yields 0, not root's id, matching
// principalOwnerID in the HTTP layer: with auth off every request runs as the
// same implicit administrator, so recording it as the actor would claim a
// specific human made the change when none did. Attribution is unaffected — the
// stamp callback's own no-auth default already resolves root — and it keeps a
// plugin job ownerless under auth-off, exactly like an async action submitted
// through the API.
func actorFromContext(ctx context.Context) uint {
	p := auth.PrincipalFromContext(ctx)
	if p == nil || p.SuperUser {
		return 0
	}
	return p.UserID
}

// actorFor is the actor for work started from L: the invocation's, which covers
// the entry points that have no request principal to read (hooks, async jobs,
// drained HTTP callbacks) as well as those that do.
//
// Reading the principal off L.Context() directly would be wrong for exactly
// those paths — a hook's context carries an Invocation, not a Principal, so a
// callback registered from inside a hook would run as nobody.
func (pm *PluginManager) actorFor(L *lua.LState) uint {
	return pm.invocationFor(L).ActorUserID
}

// ownerFromInvocation returns the actor as a job owner, or nil when there is
// none. A fresh pointer, never one shared with anything else: job ownership is
// read under the job's own lock and must not alias a caller's variable.
func ownerFromInvocation(inv *Invocation) *uint {
	if inv == nil || inv.ActorUserID == 0 {
		return nil
	}
	owner := inv.ActorUserID
	return &owner
}

// invocationContextForJob returns the Background-derived context an async job's
// Lua call runs under, carrying the job's submitter as the actor and the durable
// Job it is executing as its parent.
//
// Background, not a request: the job outlives whatever submitted it, so tying it
// to request cancellation would kill work the user explicitly backgrounded. The
// chain starts empty because a job is a fresh entry into the VM, not a nested one.
func invocationContextForJob(job *ActionJob) context.Context {
	var actor uint
	if owner := job.Owner(); owner != nil {
		actor = *owner
	}
	var jobID string
	if host := job.hostJobRef(); host != nil {
		jobID = host.JobID
	}
	return withInvocation(context.Background(), NewJobInvocation(actor, jobID))
}

// mainState returns the LState that owns L's VM: L itself for a plugin's main
// state, and the state the coroutine was spawned from for a coroutine.
//
// The coroutine library is open to plugins (see registerLibs), and gopher-lua
// hands a Go function the *coroutine's* LState, which is a different pointer
// with its own context. Two things break if that pointer is used as-is:
//
//   - Attribution. LState.NewThread copies the parent's context *at creation*
//     and never refreshes it. A coroutine created during init() has no context
//     at all, so its writes would be unattributed; worse, one created during
//     user A's request and resumed during user B's would still carry A — a
//     stale actor is a wrong answer, not a missing one.
//   - Re-entry detection. Hooks are registered against the main state and
//     vmLocks is keyed on it, so a chain recording the coroutine pointer never
//     matches, and a coroutine writing an entity its own plugin hooks would go
//     back around for a lock the outer frame already holds.
//
// Normalising here fixes both at once, because the entry point installs its
// context on the main state and that is where the live answer lives.
//
// G.MainThread is nil until the first call on a state; falling back to L is
// correct there, since nothing but the main state has run yet.
func mainState(L *lua.LState) *lua.LState {
	if L == nil {
		return nil
	}
	if L.G != nil && L.G.MainThread != nil {
		return L.G.MainThread
	}
	return L
}

// invocationFor builds the Invocation for a mah.db call made from L, adding the
// VM that owns L to the chain.
//
// Two sources, in order: an Invocation an entry point installed explicitly (a
// hook, an async job, an HTTP callback), or — for the request-serving entry
// points, which item 07 already gave a request context — the principal on that
// request. A VM with no context at all (a mah.db call from init()) yields an
// actor of 0, which binds nothing and preserves the historical behaviour.
func (pm *PluginManager) invocationFor(L *lua.LState) *Invocation {
	root := mainState(L)
	if root == nil {
		return NewInvocation(0)
	}
	ctx := root.Context()
	inv := invocationFromContext(ctx)
	if inv == nil {
		inv = NewInvocation(actorFromContext(ctx))
	}
	return inv.with(root)
}

// luaContext returns the context of the VM that owns L. A coroutine has its own
// context, snapshotted when it was created and never refreshed, so reading it
// directly would miss the current request — including the per-request MRQL cache
// a write must invalidate and a query should be served from.
func (pm *PluginManager) luaContext(L *lua.LState) context.Context {
	root := mainState(L)
	if root == nil {
		return context.Background()
	}
	if ctx := root.Context(); ctx != nil {
		return ctx
	}
	return context.Background()
}

// shortcodeCanWrite exposes capability, never the principal, to renderers.
func shortcodeCanWrite(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	return auth.PrincipalFromContext(ctx).CanWrite()
}
