package application_context

import (
	"context"
	"testing"
	"time"

	"mahresources/jobs"
	"mahresources/models"
	"mahresources/plugin_system"
)

// TestAContinuationNobodyHoldsIsAdoptedAndRun pins what the dispatch loop's cadence
// is still for once it no longer claims plugin work. The command plane accepts a
// Continue's successor as waiting work that no execution in any process holds; the
// adoption pass hands it to its plugin's lane, and it runs.
func TestAContinuationNobodyHoldsIsAdoptedAndRun(t *testing.T) {
	ctx := newPluginActionJobContext(t)

	_, canonical, err := ctx.RunPluginActionAsync(nil, pluginActionTestPlugin, "partial-work", 4, nil, "")
	if err != nil {
		t.Fatalf("run the action: %v", err)
	}
	first := waitForJobState(t, ctx, canonical, "the first run to finish", func(s jobs.Snapshot) bool {
		return s.State.Terminal()
	})
	result, err := ctx.JobService().ExecuteCommand(context.Background(), ctx.jobDeps(), jobs.CommandRequest{
		JobID: first.ID, Key: jobs.CommandContinue, IdempotencyKey: "continue-adopted",
		ExpectedVersion: first.Version, Actor: jobs.Access{Administrator: true},
	})
	if err != nil || result.SuccessorID == "" {
		t.Fatalf("continue the action: %+v, %v", result, err)
	}
	successor := waitForJobState(t, ctx, result.SuccessorID, "the continuation to run", func(s jobs.Snapshot) bool {
		return s.State.Terminal()
	})
	if successor.State != jobs.StateSucceeded {
		t.Fatalf("the continuation ended %s (%+v), want succeeded", successor.State, successor.Failure)
	}
	if got := pluginKVForTest(t, ctx, "partial"); got != "2" {
		t.Fatalf("the handler ran %q times, want twice: once for the action and once for its continuation", got)
	}
}

// TestAWaitingClosureIsOnlyEverWithdrawnWhenItsProcessIsGone pins the one subtype
// adoption must never run: a closure's callback lives in the process that started
// it. One whose process is provably gone can never start and is withdrawn as never
// started; one whose process cannot be inspected is left alone.
func TestAWaitingClosureIsOnlyEverWithdrawnWhenItsProcessIsGone(t *testing.T) {
	ctx := newPluginActionJobContext(t)

	foreign := acceptClosureJobForTest(t, ctx, "another-host/boot-1/4242")
	gone := acceptClosureJobForTest(t, ctx, plugin_system.CurrentRuntimeIdentity().Host+"/boot-that-ended/4243")

	withdrawn := waitForJobState(t, ctx, gone.ID, "the orphaned closure to be withdrawn", func(s jobs.Snapshot) bool {
		return s.State.Terminal()
	})
	if withdrawn.State != jobs.StateCancelled || lastEventType(t, ctx, gone.ID) != "not-started" {
		t.Fatalf("an orphaned closure ended %s (last event %q), want cancelled as never started",
			withdrawn.State, lastEventType(t, ctx, gone.ID))
	}
	if withdrawn.StartedAt != nil {
		t.Fatalf("an orphaned closure records a start at %s", withdrawn.StartedAt)
	}

	// Several passes later, the one whose process may still be alive is untouched.
	time.Sleep(300 * time.Millisecond)
	if got := jobStateForTest(t, ctx, foreign.ID); got != jobs.StateQueued {
		t.Fatalf("a closure whose process cannot be inspected is %s, want left queued", got)
	}
}

// TestAQueuedExecutionThatLosesItsProcessEndsByWhetherItCanRunElsewhere pins what
// a lost callback means before the claim. Nothing has run. A registered action's
// input is replayable, so the Job stays queued for the next process to pick up; a
// closure's function and an occurrence's tick die with this process, so they are
// withdrawn as never started.
func TestAQueuedExecutionThatLosesItsProcessEndsByWhetherItCanRunElsewhere(t *testing.T) {
	ctx := newJobHarnessContext(t, false)
	pm := ctx.PluginManager()
	if err := pm.EnablePlugin(pluginActionTestPlugin); err != nil {
		t.Fatalf("enable %s: %v", pluginActionTestPlugin, err)
	}
	// Every job slot is taken, so the action below waits at the head of its lane
	// without ever being claimed.
	release := pm.FillJobBudgetForTest()
	defer release()

	_, registered, err := ctx.RunPluginActionAsync(nil, pluginActionTestPlugin, "async-work", 1, nil, "")
	if err != nil {
		t.Fatalf("run the action: %v", err)
	}
	closure := acceptClosureJobForTest(t, ctx, plugin_system.CurrentRuntimeIdentity().String())
	closureAdmission := ctx.newPluginActionAdmission(closure.ID,
		&pluginActionJobInput{Subtype: pluginActionSubtypeClosure, Plugin: pluginActionTestPlugin}, nil)

	pm.Close()
	closureAdmission.CallbackLost("plugin-runtime-stopping")

	if got := jobStateForTest(t, ctx, registered); got != jobs.StateQueued {
		t.Fatalf("a registered action whose process stopped before it started is %s, want queued", got)
	}
	ended, err := ctx.JobService().Get(ctx.jobDeps(), jobs.Access{Administrator: true}, closure.ID)
	if err != nil {
		t.Fatalf("read the closure job: %v", err)
	}
	if ended.State != jobs.StateCancelled || lastEventType(t, ctx, closure.ID) != "not-started" {
		t.Fatalf("a closure whose process stopped before it started is %s (last event %q), want cancelled as never started",
			ended.State, lastEventType(t, ctx, closure.ID))
	}
}

// lastEventType is the type of the newest event on one Job's timeline.
func lastEventType(t *testing.T, ctx *MahresourcesContext, jobID string) string {
	t.Helper()
	events, err := ctx.JobService().Timeline(ctx.jobDeps(), jobs.Access{Administrator: true}, jobID, 0, 100)
	if err != nil {
		t.Fatalf("read the timeline of %s: %v", jobID, err)
	}
	if len(events) == 0 {
		return ""
	}
	return events[len(events)-1].Type
}

// TestAnOccurrenceThatCannotBeAdmittedGivesItsRowBack pins the admission inside the
// dispatch budget. A due tick of a plugin whose occurrence finds the deployment's
// budget full for the whole dispatch wait is not run, is not claimed, releases the
// row's claim, and runs on a later tick once the budget has room.
func TestAnOccurrenceThatCannotBeAdmittedGivesItsRowBack(t *testing.T) {
	ctx := newJobHarnessContext(t, false)
	ctx.Config.MaxJobConcurrency = 1
	pm := ctx.PluginManager()
	if err := pm.EnablePlugin(pluginActionTestPlugin); err != nil {
		t.Fatalf("enable %s: %v", pluginActionTestPlugin, err)
	}
	operator := models.User{Username: "schedule-operator", Role: models.RoleAdmin, PasswordHash: "x"}
	if err := ctx.db.Create(&operator).Error; err != nil {
		t.Fatalf("seed operator: %v", err)
	}
	ctx.refreshRootAdmin()
	if err := ctx.SyncPluginSchedules(pluginActionTestPlugin, pm.DeclaredSchedules(pluginActionTestPlugin)); err != nil {
		t.Fatalf("sync schedules: %v", err)
	}
	makeDue := func() {
		if err := ctx.db.Model(&models.PluginSchedule{}).
			Where("plugin_name = ? AND schedule_id = ?", pluginActionTestPlugin, "tick").
			Update("next_due_at", time.Now().Add(-time.Minute)).Error; err != nil {
			t.Fatalf("make the schedule due: %v", err)
		}
	}
	makeDue()

	// The deployment's only slot is held by a Job nobody runs.
	holder := acceptClosureJobForTest(t, ctx, "another-host/boot-1/4242")
	held, err := ctx.JobService().ClaimJob(context.Background(), ctx.jobDeps(), jobs.ClaimRequest{
		Kind: JobKindPluginAction, KindVersion: jobPluginActionKindVersion, JobID: holder.ID,
		Claimant: "holder", Capacity: ctx.hostClaimCapacityBudget(),
	})
	if err != nil {
		t.Fatalf("hold the budget: %v", err)
	}

	scheduler := NewPluginScheduler(ctx, time.Minute)
	scheduler.dispatchWait = 300 * time.Millisecond
	scheduler.Tick(time.Now())
	scheduler.Stop()

	if got := pluginKVForTest(t, ctx, "scheduled"); got != "" {
		t.Fatalf("the handler ran %q times with the budget full", got)
	}
	var row models.PluginSchedule
	if err := ctx.db.Where("plugin_name = ? AND schedule_id = ?", pluginActionTestPlugin, "tick").First(&row).Error; err != nil {
		t.Fatalf("read the row: %v", err)
	}
	if row.ClaimToken != "" || row.Runs != 0 {
		t.Fatalf("a tick that could not be admitted left claim %q and %d runs, want the row given back", row.ClaimToken, row.Runs)
	}
	occurrence := pluginActionJobBySubtype(t, ctx, pluginActionSubtypeScheduled, 1)
	if occurrence.State != jobs.StateCancelled || occurrence.StartedAt != nil {
		t.Fatalf("the unadmitted occurrence is %s (started %v), want withdrawn without a start",
			occurrence.State, occurrence.StartedAt)
	}
	if held := storedCapacity(t, ctx, jobs.CapacityGroupGlobal); held != 1 {
		t.Fatalf("the budget holds %d slots, want only the holder's", held)
	}

	if _, err := ctx.JobService().Finish(ctx.jobDeps(), jobs.FinishRequest{
		ExecutionRef:    jobs.ExecutionRef{JobID: held.JobID, ExecutionToken: held.ExecutionToken},
		ExpectedVersion: held.Version, Outcome: jobs.StateSucceeded,
	}); err != nil {
		t.Fatalf("free the budget: %v", err)
	}
	makeDue()
	scheduler = NewPluginScheduler(ctx, time.Minute)
	scheduler.Tick(time.Now())
	scheduler.Stop()
	if got := pluginKVForTest(t, ctx, "scheduled"); got != "1" {
		t.Fatalf("the handler ran %q times once the budget freed, want once", got)
	}
}
