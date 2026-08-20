package plugin_system

import (
	"testing"
	"time"
)

// A schedule's claim is sized from what a run can legitimately take:
// ScheduleClaimTTL is ScheduleDispatchWait + MaxAsyncJobDuration + a margin, and
// TestScheduleClaimTTLExceedsTheLongestPossibleRun pins that inequality. The
// formula names two waits — the job slot, then the run — and RunSchedule has a
// third between them that neither term covers: the plugin's VM lock, acquired
// through LockVM, whose own doc says "Background never ends, so this is the same
// unbounded wait it always was".
//
// Anything else holding that VM counts: a hook inside mah.http (120s), another
// async job (5m), a remote fetch (30m). Past the TTL the row reads as unclaimed
// again — `claimed_at < now - ScheduleClaimTTL` in both DuePluginSchedules and
// ClaimPluginSchedule — so the very next tick of the *same* process claims it
// and dispatches a second run. Under overlap="skip" that is exactly the
// double-fire the claim exists to prevent.
//
// So the wait has to be bounded by the budget the caller passed, and running out
// of it has to report the same "not this tick" the full-job-budget path already
// reports: ran=false, which makes the dispatcher hand the claim back and leave
// the row due for the next tick.
func TestRunScheduleGivesUpWhenTheVMStaysBusy(t *testing.T) {
	dir := t.TempDir()
	pm, err := enablingPlugin(t, dir, "busy", `plugin = { name = "busy", version = "1.0", api_version = 1, capabilities = {"schedule"} }
function init()
    mah.schedule({ id = "poll", every = "30s", handler = function() end })
end
`)
	if err != nil {
		t.Fatalf("enable: %v", err)
	}

	pm.mu.RLock()
	regs := pm.schedules["busy"]
	pm.mu.RUnlock()
	if len(regs) != 1 {
		t.Fatalf("expected one registered schedule, got %d", len(regs))
	}
	reg := regs[0]

	// Exactly what a hook mid-fetch leaves behind, and held for the whole test.
	held := pm.LockVM(reg.state)
	if held == nil {
		t.Fatal("could not take the plugin's VM lock")
	}
	defer held.Unlock()

	const budget = 300 * time.Millisecond
	type outcome struct {
		ran     bool
		elapsed time.Duration
	}
	done := make(chan outcome, 1)
	start := time.Now()
	go func() {
		_, ran, _ := pm.RunSchedule(reg, 0, budget, true)
		done <- outcome{ran: ran, elapsed: time.Since(start)}
	}()

	select {
	case got := <-done:
		if got.ran {
			t.Fatal("RunSchedule reported that it ran while another caller held the VM")
		}
		if got.elapsed > 20*budget {
			t.Fatalf("RunSchedule gave up, but only after %s on a %s budget; the wait is not "+
				"bounded by what the caller allowed", got.elapsed, budget)
		}
	case <-time.After(10 * time.Second):
		t.Fatalf("RunSchedule is still waiting for the VM lock after 10s on a %s budget. "+
			"ScheduleClaimTTL is computed as the job-slot wait plus the run, so an unbounded "+
			"wait here outlives the claim and the next tick fires the same schedule again.", budget)
	}
}
