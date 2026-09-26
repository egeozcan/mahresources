package plugin_system

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	lua "github.com/yuin/gopher-lua"
)

// laneGate is a gate a plugin's Lua waits on and the test opens, installed into
// the plugin's VM as two globals: started(n) records that the handler for entity
// n was entered, and gate_open() answers whether it may return.
type laneGate struct {
	open    atomic.Bool
	mu      sync.Mutex
	entered []int
}

func (g *laneGate) order() []int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]int(nil), g.entered...)
}

// installLaneGate puts the gate's globals into one enabled plugin's VM.
func installLaneGate(t *testing.T, pm *PluginManager, plugin, action string) *laneGate {
	t.Helper()
	_, L, err := pm.FindAction(plugin, action)
	if err != nil {
		t.Fatalf("find %s/%s: %v", plugin, action, err)
	}
	gate := &laneGate{}
	mu := pm.LockVM(L)
	if mu == nil {
		t.Fatalf("%s has no VM", plugin)
	}
	L.SetGlobal("started", L.NewFunction(func(L *lua.LState) int {
		gate.mu.Lock()
		gate.entered = append(gate.entered, int(L.CheckNumber(1)))
		gate.mu.Unlock()
		return 0
	}))
	L.SetGlobal("gate_open", L.NewFunction(func(L *lua.LState) int {
		L.Push(lua.LBool(gate.open.Load()))
		return 1
	}))
	mu.Unlock()
	return gate
}

const busyLanePlugin = `
plugin = { name = "busy", version = "1.0", api_version = 1, capabilities = { "actions", "jobs", "schedule" } }

function work(ctx)
    started(ctx.entity_id)
    while not gate_open() do mah.sleep(0.01) end
end

function init()
    mah.action({ id = "work", label = "Work", entity = "resource", async = true, handler = work })
    mah.action({ id = "needs-mode", label = "Needs Mode", entity = "resource", async = true,
                 params = { {name = "mode", type = "text", label = "Mode", required = true} },
                 handler = work })
    mah.schedule({ id = "tick", every = "1m", overlap = "skip", handler = function(job_id) started(0) end })
end
`

const idleLanePlugin = `
plugin = { name = "idle", version = "1.0", api_version = 1, capabilities = { "actions", "jobs", "schedule" } }

function work(ctx)
    mah.job_complete(ctx.job_id, { message = "idle done" })
end

function init()
    mah.action({ id = "work", label = "Work", entity = "resource", async = true, handler = work })
    mah.schedule({ id = "tick", every = "1m", overlap = "skip", handler = function(job_id) end })
end
`

func newLanePluginManager(t *testing.T) *PluginManager {
	t.Helper()
	dir := t.TempDir()
	writePlugin(t, dir, "busy", busyLanePlugin)
	writePlugin(t, dir, "idle", idleLanePlugin)
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("plugin manager: %v", err)
	}
	t.Cleanup(pm.Close)
	for _, name := range []string{"busy", "idle"} {
		if err := pm.EnablePlugin(name); err != nil {
			t.Fatalf("enable %s: %v", name, err)
		}
	}
	return pm
}

func waitUntil(t *testing.T, what string, within time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out after %s waiting for %s", within, what)
}

func jobStatuses(pm *PluginManager, plugin string) map[string]int {
	counts := map[string]int{}
	for _, job := range pm.GetAllActionJobs() {
		if job.PluginName == plugin {
			counts[job.Status]++
		}
	}
	return counts
}

// TestAPluginsBacklogWaitsInItsLaneHoldingNothingShared is the lane's reason to
// exist. A plugin runs one Lua call at a time, so everything else it was asked to
// do is waiting for its VM — and waiting is only harmless while it holds nothing.
// Five actions of one plugin must occupy one of the process's job slots, read as
// waiting rather than running, start in the order they were submitted, and leave
// another plugin's work free to run while they wait.
func TestAPluginsBacklogWaitsInItsLaneHoldingNothingShared(t *testing.T) {
	pm := newLanePluginManager(t)
	gate := installLaneGate(t, pm, "busy", "work")
	defer gate.open.Store(true)

	const backlog = 5
	for entity := 1; entity <= backlog; entity++ {
		if _, err := pm.RunActionAsyncForOwner(nil, "busy", "work", uint(entity), nil, ""); err != nil {
			t.Fatalf("submit busy work %d: %v", entity, err)
		}
	}
	waitUntil(t, "the first busy action to start", 5*time.Second, func() bool { return len(gate.order()) == 1 })

	// Long enough for a runner that takes a slot before its VM to have taken them.
	time.Sleep(100 * time.Millisecond)
	if got := len(pm.actionSemaphore); got != 1 {
		t.Fatalf("one plugin's backlog holds %d of the process's %d job slots, want 1", got, maxConcurrentActions)
	}
	if got := jobStatuses(pm, "busy"); got["running"] != 1 || got["pending"] != backlog-1 {
		t.Fatalf("busy jobs by status = %v, want 1 running and %d pending", got, backlog-1)
	}

	if _, err := pm.RunActionAsyncForOwner(nil, "idle", "work", 1, nil, ""); err != nil {
		t.Fatalf("submit idle work: %v", err)
	}
	waitUntil(t, "another plugin's action to finish while the backlog waits", 3*time.Second, func() bool {
		return jobStatuses(pm, "idle")["completed"] == 1
	})
	if got := len(gate.order()); got != 1 {
		t.Fatalf("%d busy actions were entered while the gate was closed, want 1", got)
	}

	gate.open.Store(true)
	waitUntil(t, "the backlog to drain", 10*time.Second, func() bool {
		return jobStatuses(pm, "busy")["completed"] == backlog
	})
	want := []int{1, 2, 3, 4, 5}
	if got := gate.order(); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("the backlog started in the order %v, want the submission order %v", got, want)
	}
}

// TestAScheduleOfAnotherPluginIsNotStarvedByABacklog pins the scheduler's half:
// a due tick of an idle plugin gets its job slot while a different plugin has a
// backlog, rather than finding every slot held by work that is only waiting.
func TestAScheduleOfAnotherPluginIsNotStarvedByABacklog(t *testing.T) {
	pm := newLanePluginManager(t)
	gate := installLaneGate(t, pm, "busy", "work")
	defer gate.open.Store(true)

	for entity := 1; entity <= maxConcurrentActions+2; entity++ {
		if _, err := pm.RunActionAsyncForOwner(nil, "busy", "work", uint(entity), nil, ""); err != nil {
			t.Fatalf("submit busy work %d: %v", entity, err)
		}
	}
	waitUntil(t, "the first busy action to start", 5*time.Second, func() bool { return len(gate.order()) == 1 })

	regs := pm.DeclaredSchedules("idle")
	if len(regs) != 1 {
		t.Fatalf("idle declares %d schedules, want 1", len(regs))
	}
	_, ran, err := pm.RunSchedule(regs[0], 1, 500*time.Millisecond, true)
	if err != nil || !ran {
		t.Fatalf("the idle plugin's tick ran=%v err=%v while another plugin's backlog waited, want it run", ran, err)
	}
}

// TestAScheduleThatCannotGetItsLaneGivesUpHoldingNothing pins the bounded wait:
// a tick whose own plugin is busy with other background work gives up within its
// budget, leaves no entry behind, and leaves the lane working for the work behind
// it.
func TestAScheduleThatCannotGetItsLaneGivesUpHoldingNothing(t *testing.T) {
	pm := newLanePluginManager(t)
	gate := installLaneGate(t, pm, "busy", "work")
	defer gate.open.Store(true)

	if _, err := pm.RunActionAsyncForOwner(nil, "busy", "work", 1, nil, ""); err != nil {
		t.Fatalf("submit busy work: %v", err)
	}
	waitUntil(t, "the busy action to start", 5*time.Second, func() bool { return len(gate.order()) == 1 })

	regs := pm.DeclaredSchedules("busy")
	began := time.Now()
	_, ran, err := pm.RunSchedule(regs[0], 1, 100*time.Millisecond, true)
	if err != nil || ran {
		t.Fatalf("a tick whose plugin is busy answered ran=%v err=%v, want not run and no error", ran, err)
	}
	if waited := time.Since(began); waited > 2*time.Second {
		t.Fatalf("the tick waited %s against a 100ms budget", waited)
	}
	for _, job := range pm.GetAllActionJobs() {
		if job.ActionID == "schedule:tick" {
			t.Fatalf("a tick that never ran left the entry %+v behind", job)
		}
	}

	if _, err := pm.RunActionAsyncForOwner(nil, "busy", "work", 2, nil, ""); err != nil {
		t.Fatalf("submit the next busy work: %v", err)
	}
	gate.open.Store(true)
	waitUntil(t, "the work behind the tick to run", 5*time.Second, func() bool {
		return jobStatuses(pm, "busy")["completed"] == 2
	})
}

// scriptedAdmission answers the host's side of admission from a script, and
// records how often it was asked.
type scriptedAdmission struct {
	mu      sync.Mutex
	answers []AdmitResult
	asked   int
}

func (a *scriptedAdmission) Admit(time.Time) AdmitResult {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.asked++
	if len(a.answers) == 0 {
		return Admitted
	}
	next := a.answers[0]
	a.answers = a.answers[1:]
	return next
}

func (a *scriptedAdmission) askedCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.asked
}

// TestTheHeadOfALaneAsksTheHostAgainUntilAdmitted pins the durable half: a full
// deployment budget is "later", asked again from the head of the lane, and nothing
// is reported to the Job until it is admitted.
func TestTheHeadOfALaneAsksTheHostAgainUntilAdmitted(t *testing.T) {
	saved := hostAdmissionPollInterval
	hostAdmissionPollInterval = 10 * time.Millisecond
	defer func() { hostAdmissionPollInterval = saved }()

	pm := newLanePluginManager(t)
	admission := &scriptedAdmission{answers: []AdmitResult{AdmitLater, AdmitLater}}
	sink := &recordingSink{}
	if _, err := pm.RunActionAsyncForHost(
		&HostJobRef{JobID: "job-later", Handle: "handle-later", Sink: sink, Admission: admission},
		nil, "idle", "work", 1, nil, ""); err != nil {
		t.Fatalf("run: %v", err)
	}
	waitUntil(t, "the admitted action to complete", 5*time.Second, func() bool {
		_, completed, _, _ := sink.counts()
		return completed == 1
	})
	if got := admission.askedCount(); got != 3 {
		t.Fatalf("the host was asked %d times, want 3 (two refusals, then the admission)", got)
	}
	if started, _, failed, _ := sink.counts(); started != 1 || failed != 0 {
		t.Fatalf("the host was told started=%d failed=%d, want one start and no failure", started, failed)
	}

	// The same Job handed over twice results in one execution.
	again := &scriptedAdmission{answers: []AdmitResult{AdmitLater, AdmitLater, AdmitLater}}
	ref := &HostJobRef{JobID: "job-once", Handle: "handle-once", Sink: &recordingSink{}, Admission: again}
	if _, err := pm.RunActionAsyncForHost(ref, nil, "idle", "work", 2, nil, ""); err != nil {
		t.Fatalf("first hand-over: %v", err)
	}
	if !pm.HostJobHeld("job-once") {
		t.Fatal("a Job with an execution waiting in this process is not reported as held")
	}
	if _, err := pm.RunActionAsyncForHost(ref, nil, "idle", "work", 2, nil, ""); err != nil {
		t.Fatalf("second hand-over: %v", err)
	}
	waitUntil(t, "the held Job to run", 5*time.Second, func() bool { return !pm.HostJobHeld("job-once") })
	if got := again.askedCount(); got != 4 {
		t.Fatalf("admission was asked %d times across two hand-overs, want 4 from one execution", got)
	}
}

// TestAWithdrawnExecutionLeavesWithoutAWordAndFreesItsLane pins the other
// answer: a Job that is no longer waiting — cancelled, blocked, claimed elsewhere
// — leaves this process's lane without starting, without reporting, and without
// leaving a row behind, and the work behind it runs.
func TestAWithdrawnExecutionLeavesWithoutAWordAndFreesItsLane(t *testing.T) {
	pm := newLanePluginManager(t)
	withdrawn := &recordingSink{}
	if _, err := pm.RunActionAsyncForHost(
		&HostJobRef{JobID: "job-gone", Handle: "handle-gone", Sink: withdrawn,
			Admission: &scriptedAdmission{answers: []AdmitResult{AdmitWithdrawn}}},
		nil, "idle", "work", 1, nil, ""); err != nil {
		t.Fatalf("run: %v", err)
	}
	next := &recordingSink{}
	if _, err := pm.RunActionAsyncForHost(
		&HostJobRef{JobID: "job-next", Handle: "handle-next", Sink: next, Admission: &scriptedAdmission{}},
		nil, "idle", "work", 2, nil, ""); err != nil {
		t.Fatalf("run the next: %v", err)
	}
	waitUntil(t, "the work behind the withdrawn one to complete", 5*time.Second, func() bool {
		_, completed, _, _ := next.counts()
		return completed == 1
	})
	if started, completed, failed, _ := withdrawn.counts(); started+completed+failed != 0 {
		t.Fatalf("a withdrawn execution reported started=%d completed=%d failed=%d", started, completed, failed)
	}
	if job := pm.GetActionJob("handle-gone"); job != nil {
		t.Fatalf("a withdrawn execution left the entry %+v behind", job)
	}
}

// TestALaneHandsItselfOnInArrivalOrderAndSurvivesAGiveUp covers the lane on its
// own: a waiter that gives up is removed rather than skipped over later, and one
// that is handed the lane in the instant it gives up passes it on.
func TestALaneHandsItselfOnInArrivalOrderAndSurvivesAGiveUp(t *testing.T) {
	lane := &pluginLane{}
	never := make(chan struct{})

	first := lane.join()
	if !first.wait(never, time.Time{}) {
		t.Fatal("the first ticket of a free lane did not get its turn")
	}
	quitter := lane.join()
	second := lane.join()
	if quitter.wait(never, time.Now().Add(10*time.Millisecond)) {
		t.Fatal("a ticket behind a held lane got its turn")
	}
	lane.release()
	if !second.wait(never, time.Now().Add(time.Second)) {
		t.Fatal("the lane was not handed to the ticket behind one that gave up")
	}

	// Handed over while giving up: the lane must not be dropped.
	late := lane.join()
	lane.release()
	late.leave()
	after := lane.join()
	if !after.wait(never, time.Now().Add(time.Second)) {
		t.Fatal("a lane handed to a ticket that then left was lost")
	}
	lane.release()
}

// TestAScheduleTickGoesAheadOfItsPluginsQueuedActions pins the one exception to
// arrival order. A tick's wait is bounded and it recurs; behind the plugin's whole
// backlog of actions it would give up at every tick until the backlog drained.
// It waits for the handler that is running and then runs, ahead of the actions
// that were queued before it.
func TestAScheduleTickGoesAheadOfItsPluginsQueuedActions(t *testing.T) {
	pm := newLanePluginManager(t)
	gate := installLaneGate(t, pm, "busy", "work")
	defer gate.open.Store(true)

	for entity := 1; entity <= 3; entity++ {
		if _, err := pm.RunActionAsyncForOwner(nil, "busy", "work", uint(entity), nil, ""); err != nil {
			t.Fatalf("submit busy work %d: %v", entity, err)
		}
	}
	waitUntil(t, "the first busy action to start", 5*time.Second, func() bool { return len(gate.order()) == 1 })

	regs := pm.DeclaredSchedules("busy")
	result := make(chan bool, 1)
	go func() {
		_, ran, _ := pm.RunSchedule(regs[0], 1, 5*time.Second, true)
		result <- ran
	}()
	// The tick is waiting in the lane; let the running action finish.
	time.Sleep(100 * time.Millisecond)
	gate.open.Store(true)
	select {
	case ran := <-result:
		if !ran {
			t.Fatal("the tick gave up although only one handler was ahead of it")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the tick never ran")
	}
	waitUntil(t, "the backlog to drain", 10*time.Second, func() bool { return len(gate.order()) == 4 })
	if got := fmt.Sprint(gate.order()); got != fmt.Sprint([]int{1, 0, 2, 3}) {
		t.Fatalf("handlers were entered in the order %s, want the running action, then the tick (0), then the queued actions", got)
	}
}

// lateAdmission admits only after its caller's deadline has passed.
type lateAdmission struct{ asked atomic.Int64 }

func (a *lateAdmission) Admit(deadline time.Time) AdmitResult {
	a.asked.Add(1)
	if !deadline.IsZero() {
		time.Sleep(time.Until(deadline) + 20*time.Millisecond)
	}
	return Admitted
}

// TestAnAdmissionThatArrivesAfterTheDeadlineIsNotRun pins the bound through the
// admission: a claim the host grants after a bounded caller stopped waiting is
// given back rather than run, so the caller's own claim cannot be outlived.
func TestAnAdmissionThatArrivesAfterTheDeadlineIsNotRun(t *testing.T) {
	pm := newLanePluginManager(t)
	gate := installLaneGate(t, pm, "busy", "work")
	gate.open.Store(true)

	regs := pm.DeclaredSchedules("busy")
	admission := &lateAdmission{}
	sink := &recordingSink{}
	_, ran, err := pm.RunScheduleForHost(regs[0], 1, 100*time.Millisecond, true,
		&HostJobRef{JobID: "late-job", Handle: "late-handle", Sink: sink, Admission: admission})
	if err != nil || ran {
		t.Fatalf("a late admission answered ran=%v err=%v, want not run", ran, err)
	}
	if admission.asked.Load() != 1 {
		t.Fatalf("the host was asked %d times, want once", admission.asked.Load())
	}
	if started, completed, failed, _ := sink.counts(); started+completed+failed != 0 || len(gate.order()) != 0 {
		t.Fatalf("a run admitted too late reported started=%d completed=%d failed=%d and entered %v",
			started, completed, failed, gate.order())
	}
}

// TestAQueuedActionIsCheckedAgainstTheRegistrationItRuns pins the check made when
// a queued action finally starts: it runs the handler registered then, so that
// registration must still accept the payload validated at submission — the same
// kind of entity and params that still validate.
func TestAQueuedActionIsCheckedAgainstTheRegistrationItRuns(t *testing.T) {
	pm := newLanePluginManager(t)

	valid := &ActionJob{PluginName: "busy", ActionID: "needs-mode", EntityType: "resource"}
	if _, _, err := pm.resolveQueuedAction(valid, map[string]any{"mode": "fast"}, ""); err != nil {
		t.Fatalf("a payload the registration accepts was refused: %v", err)
	}
	if _, _, err := pm.resolveQueuedAction(valid, map[string]any{}, ""); err == nil {
		t.Fatal("a payload missing a param the registration now requires was accepted")
	}
	wrongEntity := &ActionJob{PluginName: "busy", ActionID: "needs-mode", EntityType: "note"}
	if _, _, err := pm.resolveQueuedAction(wrongEntity, map[string]any{"mode": "fast"}, ""); err == nil {
		t.Fatal("a queued action for a note ran a registration that now acts on resources")
	}
	gone := &ActionJob{PluginName: "busy", ActionID: "removed", EntityType: "resource"}
	if _, _, err := pm.resolveQueuedAction(gone, nil, ""); err == nil {
		t.Fatal("a queued action whose registration is gone was resolved")
	}
}
