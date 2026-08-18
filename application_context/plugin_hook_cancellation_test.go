package application_context

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"mahresources/models/query_models"
	"mahresources/plugin_system"
)

// Hook dispatch and the caller's context, which is one rule for before-hooks and
// the opposite rule for after-hooks.
//
// A before-hook runs before the write it can veto, so abandoning its wait fails
// the write — the same answer ErrHookVMBusy already gives when a nested
// dispatch's bound expires, and a safe one: nobody is left believing a write
// happened. An after-hook runs after the write committed, so abandoning it drops
// a notification for a change that really happened and leaves the plugin's view
// of the database diverged from the database. A client that has gone away is not
// a reason to do that, and deferredPluginHooks makes the gap wider still: those
// are dispatched at commit, by which point the request may be gone by design.
//
// The context reaches the dispatcher inside the db handle (ctx.db.Statement.
// Context, read that way in visibleGroupIDs), so none of the ~35 call sites that
// raise a hook has to carry one.

// hookCancellationPlugin counts its before_tag_create and after_tag_create
// fires, and offers a page that holds its VM inside a Go call.
//
// mah.sleep is what makes the hold realistic: it blocks in Go, so the Lua
// deadline cannot preempt it and the VM lock is held for the whole call — the
// shape a 120-second mah.http.*_sync request has from the outside.
//
// The page writes a marker tag before it sleeps, which is how a test knows the
// lock is taken rather than guessing with a sleep of its own. That write raises
// the tag hooks too, but on the plugin that is already running, so
// skipReentrantHook drops them and the counters stay clean.
func hookCancellationPlugin() string {
	return `plugin = { name = "hooktest", version = "1.0", description = "counts hooks and can hold its VM" }
local fires = {}

local function bump(name)
    fires[name] = (fires[name] or 0) + 1
end

function init()
    mah.on("before_tag_create", function(data)
        bump("before")
        return data
    end)
    mah.on("after_tag_create", function(data)
        bump("after")
        return data
    end)
    mah.page("hold", function(ctx)
        mah.db.create_tag({ name = "` + vmHeldMarkerTag + `" })
        mah.sleep(` + vmHoldSeconds + `)
        return "held"
    end)
    mah.inject("page_bottom", function(ctx)
        local out = {}
        for k, v in pairs(fires) do
            out[#out + 1] = k .. "=" .. tostring(v)
        end
        table.sort(out)
        return table.concat(out, ",")
    end)
end
`
}

const (
	// vmHeldMarkerTag is written from inside the holding page, so a test can wait
	// for the lock to actually be taken.
	vmHeldMarkerTag = "vm-held"

	// vmHoldSeconds is how long the holding page keeps the VM. Long enough that
	// a dispatcher which parked on the lock is unmistakable: an abandoned wait
	// returns in milliseconds, a parked one returns after this.
	vmHoldSeconds = "2"

	// abandonedWithin is the bound a cancelled caller must return inside. Half
	// the hold, so it cannot be met by the holder finishing.
	abandonedWithin = time.Second

	// waitedAtLeast is the floor a caller that was NOT cancelled must exceed, to
	// prove it waited the holder out rather than failing fast.
	waitedAtLeast = 300 * time.Millisecond
)

// holdPluginVM starts the holding page on its own goroutine and returns once the
// VM lock is provably held. The returned function waits for the page to finish;
// call it before the test returns, or pm.Close in the fixture's cleanup blocks
// on a VM nobody gives back.
//
// One hold per test: the marker is a tag name, and a second create would collide
// with the first.
func holdPluginVM(t *testing.T, ctx *MahresourcesContext) func() {
	t.Helper()

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = ctx.PluginManager().HandlePage(context.Background(), "hooktest", "hold",
			plugin_system.PageContext{Path: "/hold"})
	}()

	deadline := time.Now().Add(10 * time.Second)
	for {
		for _, name := range tagNames(t, ctx) {
			if name == vmHeldMarkerTag {
				return func() { <-done }
			}
		}
		if time.Now().After(deadline) {
			<-done
			t.Fatal("the holding page never took the VM lock")
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// withCallerContext returns a copy of ctx whose db handle carries c, which is
// how a caller's cancellation reaches hook dispatch. It mirrors what
// applyPrincipalScope does to bind a principal: shallow copy, new handle.
func withCallerContext(ctx *MahresourcesContext, c context.Context) *MahresourcesContext {
	cp := *ctx
	cp.db = ctx.db.WithContext(c)
	return &cp
}

// cancelledContext is an already-dead caller: the shape a deferred after-hook
// sees at commit time, and the shape a before-hook must not be tripped by when
// there is nothing to wait for.
func cancelledContext() context.Context {
	c, cancel := context.WithCancel(context.Background())
	cancel()
	return c
}

// A before-hook queued behind a busy VM must stop waiting when its caller goes
// away, and the caller must be told: an error, not a silent skip.
//
// The wake-up path deliberately, matching the VM-lock tests: the context is live
// when the dispatch starts and is cancelled while it is already parked. A
// pre-cancelled context would pass against a bare ctx.Err() check on the way in,
// which is not the defect — the defect is that a dispatcher already blocked on
// the lock has no way to be told the caller is gone.
func TestRunBeforePluginHooks_AbandonedWhileWaitingForABusyVM(t *testing.T) {
	ctx := newPluginHookTestContext(t, hookCancellationPlugin())
	release := holdPluginVM(t, ctx)
	defer release()

	caller, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	_, err := withCallerContext(ctx, caller).RunBeforePluginHooks("before_tag_create", map[string]any{
		"id":          float64(0),
		"name":        "abandoned",
		"description": "",
	})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatalf("RunBeforePluginHooks succeeded after %s for a caller that was cancelled while its hook's "+
			"VM was busy: a caller that has gone must be failed, not made to wait the holder out", elapsed)
	}
	if elapsed > abandonedWithin {
		t.Fatalf("returned after %s; the caller was cancelled at 100ms and the holder runs for %ss, "+
			"so this waited the holder out instead of honouring the cancellation", elapsed, vmHoldSeconds)
	}
	if fires := hookFires(t, ctx, "before"); fires != 0 {
		t.Errorf("the hook fired %d times for an abandoned wait; abandoning must not run it", fires)
	}

	// The error has to say which of the two things happened. A plugin abort is a
	// veto and maps to a different status; "no longer available" is what a
	// disabled plugin returns and sends an operator looking for a plugin that is
	// fine.
	var abort *plugin_system.PluginAbortError
	if errors.As(err, &abort) {
		t.Errorf("an abandoned wait was reported as a plugin veto: %v", err)
	}
	if strings.Contains(err.Error(), "no longer available") {
		t.Errorf("an abandoned wait was reported as a missing plugin: %v", err)
	}
}

// The same thing through a real write, which is the property that matters: the
// caller gets an error and nothing is written.
//
// A before-hook silently skipped would be worse than a failed write — it is the
// security shape of a policy hook that did not run — so the assertions are the
// ones that tell the two apart. The error alone proves nothing here (the write
// runs on a cancelled handle, so the INSERT would fail anyway): the discriminators
// are that it returned before the holder released, that the hook never ran, and
// that no row exists.
func TestCreateTag_FailsWhenItsBeforeHookWaitIsAbandoned(t *testing.T) {
	ctx := newPluginHookTestContext(t, hookCancellationPlugin())
	release := holdPluginVM(t, ctx)
	defer release()

	caller, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	tag, err := withCallerContext(ctx, caller).CreateTag(&query_models.TagCreator{Name: "abandoned"})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatalf("CreateTag succeeded (%v) although its before-hook never ran", tag)
	}
	if elapsed > abandonedWithin {
		t.Fatalf("CreateTag returned after %s; a write whose caller has gone must not wait out "+
			"the %ss another call is holding the plugin's VM for", elapsed, vmHoldSeconds)
	}
	if fires := hookFires(t, ctx, "before"); fires != 0 {
		t.Errorf("the before-hook fired %d times for a write that was abandoned before it could run", fires)
	}
	for _, name := range tagNames(t, ctx) {
		if name == "abandoned" {
			t.Error("the tag was written although its before-hook never ran: a skipped veto is worse than a failed write")
		}
	}
}

// An after-hook may not honour the caller's context. It describes a write that
// already committed, so skipping it leaves the plugin's view of the database
// diverged from the database, and no client disconnect justifies that.
//
// Pre-cancelled *and* contended, which is the combination that catches both ways
// a fix could overreach: a ctx.Err() check on the way in, and a cancellable wait
// shared with the before path.
func TestRunAfterPluginHooks_RunsOnAnAlreadyCancelledCaller(t *testing.T) {
	ctx := newPluginHookTestContext(t, hookCancellationPlugin())
	release := holdPluginVM(t, ctx)
	defer release()

	start := time.Now()
	withCallerContext(ctx, cancelledContext()).RunAfterPluginHooks("after_tag_create", map[string]any{
		"id":   float64(1),
		"name": "committed",
	})
	elapsed := time.Since(start)

	if fires := hookFires(t, ctx, "after"); fires != 1 {
		t.Fatalf("the after-hook fired %d times, want 1: a committed write must be announced even when "+
			"the caller that made it has gone", fires)
	}
	if elapsed < waitedAtLeast {
		t.Errorf("returned after %s without the holder having released the VM; the hook cannot have run "+
			"under contention, so this test proves nothing", elapsed)
	}
}

// The deferred queue is the case where a cancelled caller is not an anomaly but
// the design: these hooks are dispatched after the plugin transaction commits,
// by which point the request that opened it may legitimately be gone. An
// after-hook that honoured the caller's context would be skipped routinely here,
// not rarely.
func TestDeferredPluginHooks_DrainRunsForACallerThatIsAlreadyGone(t *testing.T) {
	ctx := newPluginHookTestContext(t, hookCancellationPlugin())
	gone := withCallerContext(ctx, cancelledContext())

	// Raised inside the transaction: queued, not dispatched.
	queue := &deferredPluginHooks{}
	inTx := *gone
	inTx.deferredHooks = queue
	inTx.RunAfterPluginHooks("after_tag_create", map[string]any{"id": float64(1), "name": "committed"})

	if len(queue.calls) != 1 {
		t.Fatalf("queued %d hooks, want 1: an after-hook raised inside a plugin transaction must be deferred", len(queue.calls))
	}
	if fires := hookFires(t, ctx, "after"); fires != 0 {
		t.Fatalf("the after-hook fired %d times before the transaction committed", fires)
	}

	// Contended as well as abandoned: the drain has to wait for the VM, which is
	// where honouring a context would show up. Held after the probe above, which
	// needs the same lock.
	release := holdPluginVM(t, ctx)
	defer release()

	// Drained through a context with no queue of its own, the way
	// RunInTransaction drains through `bound` once the commit has happened.
	queue.drain(gone)

	if fires := hookFires(t, ctx, "after"); fires != 1 {
		t.Errorf("the after-hook fired %d times after the commit, want 1: a deferred hook runs when the "+
			"transaction commits, which is exactly when the request may already be gone", fires)
	}
}

// Cancellation abandons a *wait*; it does not refuse a hook that has nothing to
// wait for. Refusing on ctx.Err() first would fail writes that would have gone
// through, which is the trade LockWithin's fast path already declined.
func TestRunBeforePluginHooks_CancelledCallerStillRunsAHookWhoseVMIsFree(t *testing.T) {
	ctx := newPluginHookTestContext(t, hookCancellationPlugin())

	data, err := withCallerContext(ctx, cancelledContext()).RunBeforePluginHooks("before_tag_create", map[string]any{
		"id":          float64(0),
		"name":        "uncontended",
		"description": "",
	})
	if err != nil {
		t.Fatalf("RunBeforePluginHooks failed for a cancelled caller whose hook VM was free: %v", err)
	}
	if data["name"] != "uncontended" {
		t.Errorf("hook data = %v, want the input back", data)
	}
	if fires := hookFires(t, ctx, "before"); fires != 1 {
		t.Errorf("the hook fired %d times, want 1: cancellation must abandon a wait, not skip a hook that "+
			"could run immediately", fires)
	}
}

// A caller that is still there keeps waiting for as long as it takes. This is
// what makes the change a defect fix rather than a policy change: no write gains
// a deadline it did not have.
func TestRunBeforePluginHooks_LiveCallerStillWaitsOutABusyVM(t *testing.T) {
	ctx := newPluginHookTestContext(t, hookCancellationPlugin())
	release := holdPluginVM(t, ctx)
	defer release()

	start := time.Now()
	_, err := ctx.RunBeforePluginHooks("before_tag_create", map[string]any{
		"id":          float64(0),
		"name":        "patient",
		"description": "",
	})
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("RunBeforePluginHooks failed for a caller that was never cancelled: %v", err)
	}
	if elapsed < waitedAtLeast {
		t.Fatalf("returned after %s, sooner than the holder could have released the VM; an uncancelled "+
			"caller must wait rather than fail fast", elapsed)
	}
	if fires := hookFires(t, ctx, "before"); fires != 1 {
		t.Errorf("the hook fired %d times, want 1", fires)
	}
}

// Abandoning one waiter must not strand the others queued behind the same VM.
//
// The lock is a buffered channel, so a wait that gives up has to give up without
// leaving anything in it; an implementation that abandoned the send after it had
// already succeeded would wedge the plugin for every later caller, and the only
// place that shows up is with a second waiter present.
func TestRunBeforePluginHooks_AbandoningOneWaiterDoesNotStrandAnother(t *testing.T) {
	ctx := newPluginHookTestContext(t, hookCancellationPlugin())
	release := holdPluginVM(t, ctx)
	defer release()

	abandoned, cancel := context.WithCancel(context.Background())
	defer cancel()

	type outcome struct {
		err     error
		elapsed time.Duration
	}
	quits := make(chan outcome, 1)
	stays := make(chan outcome, 1)

	// Both dispatchers are joined before the test returns even when an assertion
	// above has already given up on one of them: the fixture closes the plugin
	// manager on cleanup, and closing it under a dispatch still inside a VM is
	// its own failure, unrelated to this one.
	var dispatchers sync.WaitGroup
	dispatchers.Add(2)
	defer dispatchers.Wait()

	go func() {
		defer dispatchers.Done()
		start := time.Now()
		_, err := withCallerContext(ctx, abandoned).RunBeforePluginHooks("before_tag_create", map[string]any{
			"id": float64(0), "name": "quits", "description": "",
		})
		quits <- outcome{err, time.Since(start)}
	}()
	go func() {
		defer dispatchers.Done()
		start := time.Now()
		_, err := ctx.RunBeforePluginHooks("before_tag_create", map[string]any{
			"id": float64(0), "name": "stays", "description": "",
		})
		stays <- outcome{err, time.Since(start)}
	}()

	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case got := <-quits:
		if got.err == nil {
			t.Error("the abandoned caller's hook dispatch succeeded; it should have failed when its caller went away")
		}
		if got.elapsed > abandonedWithin {
			t.Errorf("the abandoned caller returned after %s, so it waited the holder out", got.elapsed)
		}
	case <-time.After(abandonedWithin):
		t.Error("the abandoned caller never returned within the bound")
	}

	select {
	case got := <-stays:
		if got.err != nil {
			t.Errorf("the caller that stayed got %v; abandoning another waiter must not fail it", got.err)
		}
		if got.elapsed < waitedAtLeast {
			t.Errorf("the caller that stayed returned after %s, sooner than the holder could have released", got.elapsed)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("the caller that stayed never returned: abandoning one waiter stranded the queue behind it")
	}

	if fires := hookFires(t, ctx, "before"); fires != 1 {
		t.Errorf("the hook fired %d times, want 1: only the caller that stayed should have run it", fires)
	}
}
