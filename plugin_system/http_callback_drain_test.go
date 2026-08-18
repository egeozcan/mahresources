package plugin_system

import (
	"testing"
	"time"

	lua "github.com/yuin/gopher-lua"
)

// An async mah.http callback is delivered on a background drain, and until now
// there was exactly one of it for the whole process, walking a single pending
// list and taking each callback's VM lock as it came to it.
//
// So the queue was only as fast as its slowest plugin. One plugin sitting in a
// 120-second mah.http.*_sync call stalls the drain at its own callback, and
// every other plugin's callbacks -- for entirely unrelated plugins, serving
// entirely unrelated users -- wait behind it. Nothing about them needed that
// VM; they were merely later in one list.
//
// This is the sharper half of the same shape C1 fixed: there the waiter was a
// request that had gone away, here it is every other plugin in the process.

// drainPlugin is a plugin with a callback that records, in a Lua global, that
// it ran.
func drainPlugin(name string) string {
	return `
plugin = { name = "` + name + `", version = "1.0", description = "callback target" }

ran = false

function cb(resp)
    ran = true
end

function init() end
`
}

// callbackRan reports whether the plugin's callback has recorded itself. It
// takes the VM lock: the drain may be running on this very state.
func callbackRan(pm *PluginManager, L *lua.LState) bool {
	mu := pm.LockVM(L)
	if mu == nil {
		return false
	}
	defer mu.Unlock()
	return lua.LVAsBool(L.GetGlobal("ran"))
}

// callbackFn pulls the Lua callback out of a loaded plugin.
func callbackFn(t *testing.T, pm *PluginManager, L *lua.LState) *lua.LFunction {
	t.Helper()
	mu := pm.LockVM(L)
	if mu == nil {
		t.Fatal("plugin VM is gone")
	}
	defer mu.Unlock()
	fn, ok := L.GetGlobal("cb").(*lua.LFunction)
	if !ok {
		t.Fatal("plugin has no cb function")
	}
	return fn
}

// orderPlugin records the order its callbacks arrive in.
func orderPlugin(name string) string {
	return `
plugin = { name = "` + name + `", version = "1.0", description = "records arrival order" }

seen = ""

function cb(resp)
    seen = seen .. resp.tag
end

function init() end
`
}

func seenSoFar(pm *PluginManager, L *lua.LState) string {
	mu := pm.LockVM(L)
	if mu == nil {
		return ""
	}
	defer mu.Unlock()
	return lua.LVAsString(L.GetGlobal("seen"))
}

// One VM's callbacks must arrive in the order they were queued.
//
// The obvious way to stop plugins blocking each other is a goroutine per
// callback, and it silently breaks this: a plugin that fires three requests and
// mutates the same state in each callback would see them applied in whatever
// order the scheduler picked. One worker per VM is what buys both properties at
// once, and this is the half of it that no cross-plugin test would catch.
func TestHttpCallbacks_ArriveInQueueOrderWithinOnePlugin(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "ordered", orderPlugin("ordered"))

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("NewPluginManager: %v", err)
	}
	t.Cleanup(pm.Close)
	if err := pm.EnablePlugin("ordered"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	L := stateForPlugin(t, pm, "ordered")
	fn := callbackFn(t, pm, L)

	// Queued while the VM is held, so all of them are waiting before any runs —
	// otherwise the first could be delivered before the last is even queued and
	// the test would prove nothing about ordering.
	held := pm.LockVM(L)
	if held == nil {
		t.Fatal("could not take the VM lock")
	}
	// Long enough that a concurrent implementation cannot pass by luck: with a
	// goroutine per callback the observed order is whatever the scheduler picks,
	// and six of them land in the right order roughly once in seven hundred
	// tries. Twenty does not happen.
	const want = "abcdefghijklmnopqrst"
	for _, tag := range want {
		pm.queueHttpCallback(httpCallback{vm: L, fn: fn, response: map[string]any{"tag": string(tag)}})
	}
	held.Unlock()

	deadline := time.Now().Add(3 * time.Second)
	for {
		got := seenSoFar(pm, L)
		if got == want {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("callbacks arrived as %q, want %q", got, want)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// A callback queued while its VM's worker is mid-flight must still be delivered.
//
// This is the lost-wakeup window, and it is the one bug this design could
// plausibly have. The worker takes a batch and leaves the queue empty; a new
// callback lands and signals; the dispatcher sees the VM already has a worker
// and correctly declines to start a second — so that signal is now spent. If
// the worker then finished without looking at the queue again, the callback
// would sit there with nothing left to deliver it and no signal outstanding,
// and a plugin would simply never see its response.
func TestHttpCallbacks_QueuedWhileAWorkerIsRunningAreStillDelivered(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "ordered", orderPlugin("ordered"))

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("NewPluginManager: %v", err)
	}
	t.Cleanup(pm.Close)
	if err := pm.EnablePlugin("ordered"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	L := stateForPlugin(t, pm, "ordered")
	fn := callbackFn(t, pm, L)

	// Hold the VM so the worker starts, claims the first callback, and parks
	// on the lock. That is precisely the in-flight state the window needs.
	held := pm.LockVM(L)
	if held == nil {
		t.Fatal("could not take the VM lock")
	}
	pm.queueHttpCallback(httpCallback{vm: L, fn: fn, response: map[string]any{"tag": "a"}})

	// Give the worker time to take the batch and block, so the second callback
	// genuinely lands after the queue was emptied rather than joining the batch.
	waitFor(t, 2*time.Second, func() bool {
		pm.httpMu.Lock()
		defer pm.httpMu.Unlock()
		return pm.httpDraining[L] && len(pm.httpPending[L]) == 0
	}, "the worker never claimed the first callback")

	// Queued deliberately WITHOUT signalling, which is the whole point.
	//
	// Going through queueHttpCallback would send a signal and leave the outcome
	// to a scheduling race: if the dispatcher happened to consume it after the
	// worker cleared its mark, it would start a fresh worker and deliver "b"
	// even with the re-check removed — so the test would pass against the very
	// defect it names. Appending with no signal reproduces exactly the state a
	// spent signal leaves behind: a callback queued, a worker running, and
	// nothing outstanding to start another. The worker's own re-check is then
	// the only thing that can deliver it.
	pm.httpMu.Lock()
	pm.httpPending[L] = append(pm.httpPending[L], httpCallback{vm: L, fn: fn, response: map[string]any{"tag": "b"}})
	pm.httpMu.Unlock()

	held.Unlock()

	deadline := time.Now().Add(3 * time.Second)
	for {
		got := seenSoFar(pm, L)
		if got == "ab" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("delivered %q, want \"ab\"; a callback queued while the worker was "+
				"running was stranded with no worker and no signal left to start one", got)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// Shutting down must not leave undelivered callbacks holding a Lua state alive.
//
// The drain goroutine selects between done and the notify channel, so when both
// are ready it can take done and exit with callbacks still queued. Nothing
// collects them afterwards: their VM has no worker and the goroutine that would
// have started one is gone. Each stranded entry pins an *lua.LState and an
// *lua.LFunction, so a manager still reachable after Close keeps a whole Lua
// state alive — the kind of retention that shows up as a slow leak across
// reloads rather than as a failure anyone can point at.
func TestHttpCallbacks_ShutdownDoesNotStrandQueuedCallbacks(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "first", orderPlugin("first"))
	writePlugin(t, dir, "second", orderPlugin("second"))

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("NewPluginManager: %v", err)
	}
	// Enable order fixes teardown order: Close copies pm.states and walks it in
	// order, so "first" is torn down before "second".
	for _, name := range []string{"first", "second"} {
		if err := pm.EnablePlugin(name); err != nil {
			t.Fatalf("EnablePlugin(%s): %v", name, err)
		}
	}

	first := stateForPlugin(t, pm, "first")
	L := stateForPlugin(t, pm, "second")
	fn := callbackFn(t, pm, L)

	// Hold the second plugin's VM so Close stops partway through its teardown
	// loop, with one state already torn down and this one still to go.
	held := pm.LockVM(L)
	if held == nil {
		t.Fatal("could not take the VM lock")
	}
	heldReleased := false
	release := func() {
		if !heldReleased {
			heldReleased = true
			held.Unlock()
		}
	}
	// Guarded, because every failure below happens while Close is parked on this
	// lock: bailing out without releasing would strand that goroutine for the
	// rest of the package run.
	defer release()

	// The wait below is only meaningful if this starts out non-nil.
	if pm.VMLock(first) == nil {
		t.Fatal("the first plugin has no VM lock entry to begin with; the wait below would be vacuous")
	}

	closed := make(chan struct{})
	go func() {
		pm.Close()
		close(closed)
	}()

	// Two plugins rather than one, because waiting on pm.done would prove only
	// that Close got past httpWg.Wait -- not that it is inside the teardown
	// loop, which is what the placement turns on. Close revokes a state's lock
	// entry while tearing it down, so once the first plugin's entry is gone
	// Close is demonstrably in the loop with only the held state left.
	//
	// This pins the clear as being after the loop rather than before it. It does
	// not distinguish every conceivable position inside the loop -- a clear
	// wedged into the second state's own teardown would also land after the
	// enqueue -- and no polling test could, absent instrumentation for "Close is
	// now blocked on this exact lock".
	waitFor(t, 5*time.Second, func() bool {
		return pm.VMLock(first) == nil
	}, "Close never entered its teardown loop")

	// Nothing may be pending or draining yet, or the emptiness asserted at the
	// end could be somebody else's doing.
	pm.httpMu.Lock()
	dirty := len(pm.httpPending) != 0 || len(pm.httpDraining) != 0
	pm.httpMu.Unlock()
	if dirty {
		t.Fatal("callbacks were already queued before the window under test; this test needs to be " +
			"the only thing putting anything in those maps")
	}

	// Enqueued without signalling, and the omission is what keeps the result
	// meaningful. queueErrorCallback signals, and closing done does not prove
	// the drain goroutine has run: it may still be sitting in its select when
	// the signal arrives, take httpNotify over done, and dispatch a worker. That
	// worker removes the batch from the map itself before it ever blocks on the
	// VM lock -- so the map would come out empty whether or not Close cleared
	// it, and the test would pass against the defect it is here to catch.
	//
	// This is a state production reaches transiently rather than rests in:
	// queueHttpCallback publishes under httpMu and signals only after releasing
	// it, so a producer descheduled between those two steps leaves exactly this.
	// The producer is real -- the bindings call queueErrorCallback on the
	// calling render's own goroutine, which httpWg does not count, exactly when
	// they find the manager closing.
	pm.httpMu.Lock()
	pm.httpPending[L] = append(pm.httpPending[L], httpCallback{
		vm: L, fn: fn, response: map[string]any{"error": errShuttingDown},
	})
	pm.httpMu.Unlock()

	release()
	select {
	case <-closed:
	case <-time.After(10 * time.Second):
		t.Fatal("Close never returned")
	}

	pm.httpMu.Lock()
	pending := len(pm.httpPending)
	pm.httpMu.Unlock()

	if pending != 0 {
		t.Fatalf("%d VM(s) still hold undelivered callbacks after Close; each one pins the "+
			"Lua state and function it was going to call", pending)
	}
	// Deliberately no assertion about httpDraining. No worker runs in this test,
	// so there is no mark here to survive, and checking for one would read as
	// coverage while proving nothing. Close clears that map too, but a mark is
	// self-clearing in practice -- the worker that sets it deletes it on its way
	// out -- so the clear is a backstop for a worker that dies unexpectedly, and
	// a test that manufactured a mark with no worker behind it would be pinning
	// a state production never produces.
}

// waitFor polls until cond holds, failing with msg if it never does.
func waitFor(t *testing.T, within time.Duration, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(within)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal(msg)
		}
		time.Sleep(time.Millisecond)
	}
}

// A plugin holding its own VM must not hold up another plugin's callbacks.
func TestHttpCallbacks_ABusyPluginDoesNotStallAnotherPluginsCallback(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "busy", drainPlugin("busy"))
	writePlugin(t, dir, "idle", drainPlugin("idle"))

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("NewPluginManager: %v", err)
	}
	t.Cleanup(pm.Close)
	for _, name := range []string{"busy", "idle"} {
		if err := pm.EnablePlugin(name); err != nil {
			t.Fatalf("EnablePlugin(%s): %v", name, err)
		}
	}

	busy := stateForPlugin(t, pm, "busy")
	idle := stateForPlugin(t, pm, "idle")
	busyFn := callbackFn(t, pm, busy)
	idleFn := callbackFn(t, pm, idle)

	// Stand in for a plugin inside a long synchronous HTTP call: the VM lock is
	// held and the Lua deadline cannot preempt it.
	held := pm.LockVM(busy)
	if held == nil {
		t.Fatal("could not take the busy plugin's VM lock")
	}
	heldReleased := false
	releaseBusy := func() {
		if !heldReleased {
			heldReleased = true
			held.Unlock()
		}
	}
	defer releaseBusy()

	// Order matters: the blocked plugin's callback is queued first, so a drain
	// that walks one list in order reaches it before the other one.
	pm.queueHttpCallback(httpCallback{vm: busy, fn: busyFn, response: map[string]any{"status": 200}})
	pm.queueHttpCallback(httpCallback{vm: idle, fn: idleFn, response: map[string]any{"status": 200}})

	deadline := time.Now().Add(3 * time.Second)
	for !callbackRan(pm, idle) {
		if time.Now().After(deadline) {
			// Release before failing: the drain is parked on the busy VM, and
			// pm.Close would queue behind it forever, hanging the package run
			// instead of reporting this.
			releaseBusy()
			t.Fatal("the idle plugin's callback never ran while another plugin held its own VM; " +
				"one busy plugin is stalling the whole callback queue")
		}
		time.Sleep(5 * time.Millisecond)
	}

	// The blocked one still gets delivered once its VM frees up: this is about
	// ordering between plugins, not about dropping work.
	releaseBusy()
	deadline = time.Now().Add(3 * time.Second)
	for !callbackRan(pm, busy) {
		if time.Now().After(deadline) {
			t.Fatal("the busy plugin's callback was never delivered after its VM was released")
		}
		time.Sleep(5 * time.Millisecond)
	}
}
