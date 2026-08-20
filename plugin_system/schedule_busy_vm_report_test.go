package plugin_system

import (
	"sync"
	"testing"
	"time"
)

// A schedule that could not get the plugin's VM within its dispatch budget never
// entered its handler, and must not be reported as one that ran and failed.
//
// The distinction is not cosmetic. The jobs panel announces a failed action to a
// screen reader (`Action failed: <label>` in downloadCockpit.js) and retains the
// row after the removal event, because the removal snapshot is authoritative and
// terminal — so a plugin that is merely busy accuses itself, once per tick, of a
// failure it did not have. The application log gains a matching `[plugin]
// schedule ... failed` line for a handler that was never called.
//
// What the subscriber may see is "added" and "running": the job did take a slot.
// What it may not see is a terminal status.
func TestABusyVMIsNotReportedAsAFailedRun(t *testing.T) {
	dir := t.TempDir()
	pm, err := enablingPlugin(t, dir, "quiet", `plugin = { name = "quiet", version = "1.0", api_version = 1, capabilities = {"schedule"} }
function init()
    mah.schedule({ id = "poll", every = "30s", handler = function() end })
end
`)
	if err != nil {
		t.Fatalf("enable: %v", err)
	}

	pm.mu.RLock()
	regs := pm.schedules["quiet"]
	pm.mu.RUnlock()
	if len(regs) != 1 {
		t.Fatalf("expected one registered schedule, got %d", len(regs))
	}
	reg := regs[0]

	events := pm.SubscribeActionJobs()
	defer pm.UnsubscribeActionJobs(events)

	held := pm.LockVM(reg.state)
	if held == nil {
		t.Fatal("could not take the plugin's VM lock")
	}
	release := sync.OnceFunc(held.Unlock)
	defer release()

	const budget = 200 * time.Millisecond
	if _, ran, runErr := pm.RunSchedule(reg, 0, budget, true); ran || runErr != nil {
		t.Fatalf("RunSchedule reported ran=%v err=%v; a VM that stayed busy is 'not this tick'", ran, runErr)
	}
	release()

	// Everything the run emitted is already queued: notifyActionJobSubscribers
	// sends before RunSchedule returns, and the channel is buffered.
	for {
		select {
		case ev := <-events:
			if ev.Job == nil {
				continue
			}
			status := ev.Job.Status
			if status == "failed" || status == "completed" {
				t.Fatalf("the panel was told %q=%q for a schedule whose handler was never "+
					"entered; it announces that to a screen reader and keeps the row",
					ev.Type, status)
			}
		default:
			return
		}
	}
}
