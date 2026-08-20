package plugin_system

import (
	"sync"
	"testing"
	"time"
)

// `overlap = "allow"` buys queueing, not parallelism: two runs of one plugin
// still serialize on its VM lock, and the point of advancing the row at dispatch
// rather than at completion is that an overrunning run does not cause the next
// to be skipped. That sentence is in CLAUDE.md, and it is only true if the next
// run *waits* for the VM.
//
// The bound that TestRunScheduleGivesUpWhenTheVMStaysBusy demands exists to
// protect a database claim held across the wait. Under "allow" there is no such
// claim: the dispatcher advances next_due_at and releases before RunSchedule is
// called, and its ran=false branch releases nothing back. So a bound there
// protects nothing and costs the policy its meaning — worse than under "skip",
// because the row has already moved on and no later tick will retry the
// interval. It is not deferred; it is gone.
//
// The two tests are deliberately a pair: same busy VM, opposite answers,
// selected by the one thing that differs.
func TestOverlapAllowWaitsOutABusyVM(t *testing.T) {
	dir := t.TempDir()
	pm, err := enablingPlugin(t, dir, "queued", `plugin = { name = "queued", version = "1.0", api_version = 1, capabilities = {"schedule"} }
function init()
    mah.schedule({ id = "poll", every = "30s", overlap = "allow", handler = function() end })
end
`)
	if err != nil {
		t.Fatalf("enable: %v", err)
	}

	pm.mu.RLock()
	regs := pm.schedules["queued"]
	pm.mu.RUnlock()
	if len(regs) != 1 {
		t.Fatalf("expected one registered schedule, got %d", len(regs))
	}
	reg := regs[0]

	// The previous run of this same schedule, still going.
	held := pm.LockVM(reg.state)
	if held == nil {
		t.Fatal("could not take the plugin's VM lock")
	}
	// Once, and whatever happens: the manager's own Cleanup closes every state,
	// which takes this same lock, so a test that fails while still holding it
	// hangs the package rather than reporting.
	release := sync.OnceFunc(held.Unlock)
	defer release()

	const budget = 300 * time.Millisecond
	done := make(chan bool, 1)
	go func() {
		_, ran, _ := pm.RunSchedule(reg, 0, budget, false /* the claim is already released */)
		done <- ran
	}()

	// Well past the budget a "skip" dispatch would have spent.
	select {
	case ran := <-done:
		t.Fatalf("RunSchedule returned ran=%v while the previous run still held the VM; "+
			"under overlap=\"allow\" the row was advanced before this call, so giving up "+
			"here drops the interval permanently rather than deferring it", ran)
	case <-time.After(5 * budget):
	}

	release()

	select {
	case ran := <-done:
		if !ran {
			t.Fatal("the VM was released and the schedule still did not run")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the VM was released 10s ago and the schedule has still not run")
	}
}
