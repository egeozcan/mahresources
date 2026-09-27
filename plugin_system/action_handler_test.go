package plugin_system

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	lua "github.com/yuin/gopher-lua"
)

// This file pins how the host ends a running handler — its timeout, a disable,
// a shutdown and a person's cancel all go through one stop — and that one
// execution reports exactly one outcome whichever of them races its own.

const stoppablePlugin = `
plugin = { name = "stoppable", version = "1.0", api_version = 1, capabilities = { "actions", "jobs" } }

-- sleeps in mah.sleep, which is where a long handler spends its time
function sleeper(ctx)
    entered(ctx.entity_id)
    for i = 1, 100 do mah.sleep(1) end
    mah.job_complete(ctx.job_id, { message = "slept" })
end

-- spins without calling into the host
function spinner(ctx)
    entered(ctx.entity_id)
    while true do end
end

-- waits in a Go call that does not watch its context
function blocker(ctx)
    entered(ctx.entity_id)
    block_until_released()
    mah.job_complete(ctx.job_id, { message = "released" })
end

-- finishes by itself
function quick(ctx)
    entered(ctx.entity_id)
    mah.sleep(0.2)
    mah.job_complete(ctx.job_id, { message = "quick done" })
end

function init()
    mah.action({ id = "sleeper", label = "Sleeper", entity = "resource", async = true, handler = sleeper })
    mah.action({ id = "spinner", label = "Spinner", entity = "resource", async = true, handler = spinner })
    mah.action({ id = "blocker", label = "Blocker", entity = "resource", async = true, handler = blocker })
    mah.action({ id = "quick", label = "Quick", entity = "resource", async = true, handler = quick })
end
`

// stoppableHooks are the Go functions the stoppable plugin calls: which entities
// entered their handler, and a release for the blocker.
type stoppableHooks struct {
	mu       sync.Mutex
	entered  []int
	released atomic.Bool
}

func (h *stoppableHooks) enteredCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.entered)
}

// installStoppableHooks installs the Go functions on the plugin's current VM.
func installStoppableHooks(t *testing.T, pm *PluginManager) *stoppableHooks {
	t.Helper()
	_, L, err := pm.FindAction("stoppable", "sleeper")
	if err != nil {
		t.Fatalf("find the plugin's VM: %v", err)
	}
	hooks := &stoppableHooks{}
	mu := pm.LockVM(L)
	if mu == nil {
		t.Fatal("the plugin has no VM")
	}
	L.SetGlobal("entered", L.NewFunction(func(L *lua.LState) int {
		hooks.mu.Lock()
		hooks.entered = append(hooks.entered, int(L.CheckNumber(1)))
		hooks.mu.Unlock()
		return 0
	}))
	L.SetGlobal("block_until_released", L.NewFunction(func(L *lua.LState) int {
		for !hooks.released.Load() {
			time.Sleep(5 * time.Millisecond)
		}
		return 0
	}))
	mu.Unlock()
	return hooks
}

func newStoppablePlugin(t *testing.T) (*PluginManager, *stoppableHooks) {
	t.Helper()
	dir := t.TempDir()
	writePlugin(t, dir, "stoppable", stoppablePlugin)
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("plugin manager: %v", err)
	}
	t.Cleanup(pm.Close)
	if err := pm.EnablePlugin("stoppable"); err != nil {
		t.Fatalf("enable: %v", err)
	}
	return pm, installStoppableHooks(t, pm)
}

// runStoppable submits one action with a recording sink and waits for its
// handler to be entered.
func runStoppable(t *testing.T, pm *PluginManager, hooks *stoppableHooks, action string, entity uint, sink HostJobSink) {
	t.Helper()
	before := hooks.enteredCount()
	id := action + "-" + time.Now().Format("150405.000000000")
	if _, err := pm.RunActionAsyncForHost(&HostJobRef{JobID: id, Handle: id, Sink: sink},
		nil, "stoppable", action, entity, nil, ""); err != nil {
		t.Fatalf("run %s: %v", action, err)
	}
	waitUntil(t, action+" to enter its handler", 5*time.Second, func() bool { return hooks.enteredCount() > before })
}

// shortenShutdown makes Close's waits short enough to observe.
func shortenShutdown(t *testing.T, grace, stopWait, settleWait time.Duration) {
	t.Helper()
	oldGrace, oldStop, oldSettle := shutdownHandlerGrace, shutdownHandlerStopWait, shutdownSettleWait
	shutdownHandlerGrace, shutdownHandlerStopWait, shutdownSettleWait = grace, stopWait, settleWait
	t.Cleanup(func() {
		shutdownHandlerGrace, shutdownHandlerStopWait, shutdownSettleWait = oldGrace, oldStop, oldSettle
	})
}

// TestAHandlerThatRunsOutOfTimeFailsAsATimeout pins the classification a Job
// reader sees: a handler stopped by its own time limit failed because it ran too
// long, which is neither the plugin reporting a failure nor a Lua error — whether
// it spun without calling the host or waited in mah.sleep.
func TestAHandlerThatRunsOutOfTimeFailsAsATimeout(t *testing.T) {
	old := asyncHandlerTimeout
	asyncHandlerTimeout = 300 * time.Millisecond
	defer func() { asyncHandlerTimeout = old }()
	pm, hooks := newStoppablePlugin(t)

	for i, action := range []string{"spinner", "sleeper"} {
		sink := &recordingSink{}
		runStoppable(t, pm, hooks, action, uint(i+1), sink)
		waitUntil(t, action+" to fail", 5*time.Second, func() bool {
			_, _, failed, _ := sink.counts()
			return failed == 1
		})
		sink.mu.Lock()
		failure := sink.failures[0]
		sink.mu.Unlock()
		if failure.Cause != FailureTimeout {
			t.Fatalf("%s ended with a %q failure (%q), want a timeout", action, failure.Cause, failure.Message)
		}
	}
}

// TestDisablingAPluginStopsItsRunningHandler pins that a disable stops the
// plugin's running work rather than leaving it running on a revoked VM for the
// rest of its allowance, and that the Job is told why: the handler was stopped
// because its plugin was disabled, which is not a failure of the work.
func TestDisablingAPluginStopsItsRunningHandler(t *testing.T) {
	pm, hooks := newStoppablePlugin(t)
	sink := &recordingSink{}
	runStoppable(t, pm, hooks, "sleeper", 1, sink)

	began := time.Now()
	if err := pm.DisablePlugin("stoppable"); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if took := time.Since(began); took > 3*time.Second {
		t.Fatalf("the disable took %s: it waited for the handler instead of stopping it", took)
	}
	waitUntil(t, "the stopped handler to report", 5*time.Second, func() bool {
		sink.mu.Lock()
		defer sink.mu.Unlock()
		return len(sink.stopped) == 1
	})
	sink.mu.Lock()
	defer sink.mu.Unlock()
	if sink.stopped[0] != StopPluginDisabled || sink.completed != 0 || sink.failed != 0 || len(sink.lost) != 0 {
		t.Fatalf("the host was told stopped=%v completed=%d failed=%d lost=%v, want one stop for the disable",
			sink.stopped, sink.completed, sink.failed, sink.lost)
	}
}

// TestAReenabledPluginDoesNotRunBesideItsStoppedHandler pins "one handler of a
// plugin at a time" across a disable and an enable. A handler inside a Go call
// cannot be stopped until the call returns, so it is still running on the old VM
// when the plugin is enabled again; work submitted to the new VM waits for it,
// because both wait in the plugin's one lane.
func TestAReenabledPluginDoesNotRunBesideItsStoppedHandler(t *testing.T) {
	pm, hooks := newStoppablePlugin(t)
	oldSink := &recordingSink{}
	runStoppable(t, pm, hooks, "blocker", 1, oldSink)

	if err := pm.DisablePlugin("stoppable"); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if err := pm.EnablePlugin("stoppable"); err != nil {
		t.Fatalf("enable again: %v", err)
	}
	newHooks := installStoppableHooks(t, pm)
	newSink := &recordingSink{}
	if _, err := pm.RunActionAsyncForHost(&HostJobRef{JobID: "after-enable", Handle: "after-enable", Sink: newSink},
		nil, "stoppable", "quick", 2, nil, ""); err != nil {
		t.Fatalf("run on the new VM: %v", err)
	}
	time.Sleep(500 * time.Millisecond)
	if newHooks.enteredCount() != 0 {
		t.Fatal("the new VM ran the plugin's work while the old VM's handler was still running")
	}

	hooks.released.Store(true)
	waitUntil(t, "the new VM's work to run once the old handler returned", 5*time.Second, func() bool {
		_, completed, _, _ := newSink.counts()
		return completed == 1
	})
	oldSink.mu.Lock()
	defer oldSink.mu.Unlock()
	if len(oldSink.stopped) != 1 || oldSink.stopped[0] != StopPluginDisabled || oldSink.completed != 0 {
		t.Fatalf("the old handler reported stopped=%v completed=%d, want the stop the disable asked for",
			oldSink.stopped, oldSink.completed)
	}
}

// TestAShutdownStopsAHandlerThatOutlivesItsGrace pins the bound on Close: a
// handler that would run for minutes is given the grace period and then stopped,
// and its Job is told the server is shutting down — once, as a stop, and not
// also as a lost callback.
func TestAShutdownStopsAHandlerThatOutlivesItsGrace(t *testing.T) {
	shortenShutdown(t, 300*time.Millisecond, 2*time.Second, 2*time.Second)
	pm, hooks := newStoppablePlugin(t)
	sink := &recordingSink{}
	runStoppable(t, pm, hooks, "sleeper", 1, sink)

	began := time.Now()
	pm.Close()
	if took := time.Since(began); took > 3*time.Second {
		t.Fatalf("Close took %s with a handler that had minutes left: the drain is not bounded", took)
	}
	sink.mu.Lock()
	defer sink.mu.Unlock()
	if len(sink.stopped) != 1 || sink.stopped[0] != StopRuntimeStopping {
		t.Fatalf("the host was told stopped=%v, want one stop for the shutdown", sink.stopped)
	}
	if sink.completed != 0 || sink.failed != 0 || len(sink.lost) != 0 {
		t.Fatalf("the host was also told completed=%d failed=%d lost=%v: one execution, one outcome",
			sink.completed, sink.failed, sink.lost)
	}
}

// TestAShutdownLetsAHandlerFinishWithinItsGrace is the other side of the grace:
// work that finishes during it is recorded as it finished.
func TestAShutdownLetsAHandlerFinishWithinItsGrace(t *testing.T) {
	shortenShutdown(t, 3*time.Second, 2*time.Second, 2*time.Second)
	pm, hooks := newStoppablePlugin(t)
	sink := &recordingSink{}
	runStoppable(t, pm, hooks, "quick", 1, sink)

	pm.Close()
	sink.mu.Lock()
	defer sink.mu.Unlock()
	if sink.completed != 1 || len(sink.stopped) != 0 || len(sink.lost) != 0 {
		t.Fatalf("the host was told completed=%d stopped=%v lost=%v, want the completion the handler reached",
			sink.completed, sink.stopped, sink.lost)
	}
}

// slowCompletionSink takes a while to record a completion, which is what
// publishing a result output and then finishing the Job looks like from here.
type slowCompletionSink struct {
	recordingSink
	delay time.Duration
}

func (s *slowCompletionSink) Completed(message string, result map[string]any) error {
	time.Sleep(s.delay)
	return s.recordingSink.Completed(message, result)
}

// TestAnOutcomeBeingRecordedAtShutdownIsNotReportedLost pins the race a shutdown
// used to lose. A handler returns and gives its VM back; the shutdown, which was
// waiting for that VM, closes it at once and reports every unfinished callback
// lost — while the handler's own goroutine is still recording its completion. The
// execution records that it settles itself before it gives the VM back, so the
// shutdown waits for that outcome, and when the outcome takes longer than the
// wait it is still the only one reported.
func TestAnOutcomeBeingRecordedAtShutdownIsNotReportedLost(t *testing.T) {
	for _, tc := range []struct {
		name       string
		settleWait time.Duration
	}{
		{"recorded within the wait", 3 * time.Second},
		{"recorded after the wait", 50 * time.Millisecond},
	} {
		t.Run(tc.name, func(t *testing.T) {
			shortenShutdown(t, 3*time.Second, 2*time.Second, tc.settleWait)
			pm, hooks := newStoppablePlugin(t)
			sink := &slowCompletionSink{delay: 500 * time.Millisecond}
			runStoppable(t, pm, hooks, "quick", 1, sink)

			pm.Close()
			waitUntil(t, "the completion to be recorded", 5*time.Second, func() bool {
				_, completed, _, _ := sink.counts()
				return completed == 1
			})
			sink.mu.Lock()
			defer sink.mu.Unlock()
			if len(sink.lost) != 0 {
				t.Fatalf("a handler that returned was also reported lost (%v)", sink.lost)
			}
		})
	}
}

// TestAHandlerThatWillNotStopIsReportedLostOnce pins the last resort: a handler
// inside a Go call that ignores its context cannot be stopped, so the shutdown
// stops waiting for it, reports it lost, and exits without it. If it returns
// after that, it reports nothing more — the Job already has its outcome.
func TestAHandlerThatWillNotStopIsReportedLostOnce(t *testing.T) {
	shortenShutdown(t, 200*time.Millisecond, 300*time.Millisecond, 300*time.Millisecond)
	pm, hooks := newStoppablePlugin(t)
	sink := &recordingSink{}
	runStoppable(t, pm, hooks, "blocker", 1, sink)

	began := time.Now()
	pm.Close()
	if took := time.Since(began); took > 3*time.Second {
		t.Fatalf("Close took %s waiting for a handler that cannot be stopped", took)
	}
	sink.mu.Lock()
	lost := append([]string(nil), sink.lost...)
	sink.mu.Unlock()
	if len(lost) != 1 || lost[0] != StopRuntimeStopping {
		t.Fatalf("the host was told lost=%v, want the handler that would not stop reported once", lost)
	}

	hooks.released.Store(true)
	time.Sleep(300 * time.Millisecond)
	sink.mu.Lock()
	defer sink.mu.Unlock()
	if sink.completed != 0 || sink.failed != 0 || len(sink.stopped) != 0 {
		t.Fatalf("a handler reported lost then reported completed=%d failed=%d stopped=%v: two outcomes",
			sink.completed, sink.failed, sink.stopped)
	}
}

// TestCancellingARunningHandlerStopsIt pins the host's half of a cancel: the Job
// named is the one stopped, and it reports that a person cancelled it.
func TestCancellingARunningHandlerStopsIt(t *testing.T) {
	pm, hooks := newStoppablePlugin(t)
	sink := &recordingSink{}
	if _, err := pm.RunActionAsyncForHost(&HostJobRef{JobID: "to-cancel", Handle: "to-cancel", Sink: sink},
		nil, "stoppable", "sleeper", 1, nil, ""); err != nil {
		t.Fatalf("run: %v", err)
	}
	waitUntil(t, "the handler to be entered", 5*time.Second, func() bool { return hooks.enteredCount() == 1 })

	if pm.StopHostJob("another-job") {
		t.Fatal("a cancel for another Job stopped something")
	}
	if !pm.StopHostJob("to-cancel") {
		t.Fatal("the cancel found no running handler for the Job")
	}
	waitUntil(t, "the cancelled handler to report", 5*time.Second, func() bool {
		sink.mu.Lock()
		defer sink.mu.Unlock()
		return len(sink.stopped) == 1
	})
	sink.mu.Lock()
	defer sink.mu.Unlock()
	if sink.stopped[0] != StopCancelled || sink.completed != 0 || sink.failed != 0 {
		t.Fatalf("the host was told stopped=%v completed=%d failed=%d, want one cancellation",
			sink.stopped, sink.completed, sink.failed)
	}
}
