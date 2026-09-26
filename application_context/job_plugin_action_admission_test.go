package application_context

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"

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

// TestAnOccurrenceSomebodyElseWithdrewIsNotReportedAsRun pins what a waiter reads
// from a Job it did not run. An occurrence another runtime claimed and then gave
// back unstarted ends cancelled with a not-started event; reading "it ended" as
// "it ran" would record a completed run for a tick that never ran, and advance
// the schedule's row past it.
func TestAnOccurrenceSomebodyElseWithdrewIsNotReportedAsRun(t *testing.T) {
	ctx := newJobHarnessContext(t, false)
	for _, claimFirst := range []bool{false, true} {
		accepted := acceptJobFor(t, ctx, jobs.Acceptance{
			Kind: JobKindPluginAction, KindVersion: jobPluginActionKindVersion, State: jobs.StateQueued,
			Origin: "schedule", Title: "occurrence", Replay: jobs.ReplayInput{NonReplayable: true},
		})
		execution := jobs.Execution{JobID: accepted.ID}
		if claimFirst {
			claimed, err := ctx.JobService().ClaimJob(context.Background(), ctx.jobDeps(), jobs.ClaimRequest{
				Kind: JobKindPluginAction, KindVersion: jobPluginActionKindVersion, JobID: accepted.ID,
				Claimant: "another-runtime",
			})
			if err != nil {
				t.Fatalf("claim: %v", err)
			}
			execution = claimed
		}
		if err := ctx.withdrawPluginActionJob(execution, "not-started", "busy"); err != nil {
			t.Fatalf("withdraw: %v", err)
		}
		run, err := ctx.awaitPluginActionRun(context.Background(), jobs.Execution{JobID: accepted.ID})
		if err != nil {
			t.Fatalf("await: %v", err)
		}
		if run.Started || run.Failed {
			t.Fatalf("claimed first=%v: a withdrawn occurrence was reported as started=%v failed=%v",
				claimFirst, run.Started, run.Failed)
		}
	}
}

// TestAdoptionFillsALaneOnlyAsDeepAsItCanDrain pins the bound on what this process
// holds in memory for work it cannot run yet. With the job slots full, a pass over a
// long waiting queue hands a plugin's lane a bounded number of Jobs, and the rest
// stay durable and waiting.
func TestAdoptionFillsALaneOnlyAsDeepAsItCanDrain(t *testing.T) {
	ctx := newJobHarnessContext(t, false)
	pm := ctx.PluginManager()
	if err := pm.EnablePlugin(pluginActionTestPlugin); err != nil {
		t.Fatalf("enable %s: %v", pluginActionTestPlugin, err)
	}
	release := pm.FillJobBudgetForTest()
	defer release()

	input, err := json.Marshal(pluginActionJobInput{
		Subtype: pluginActionSubtypeRegistered, Plugin: pluginActionTestPlugin, Action: "async-work",
		EntityType: "resource", Runtime: plugin_system.CurrentRuntimeIdentity().String(),
	})
	if err != nil {
		t.Fatalf("encode the input: %v", err)
	}
	const waiting = 3 * pluginActionAdoptDepth
	ids := make([]string, 0, waiting)
	for i := 0; i < waiting; i++ {
		ids = append(ids, acceptJobFor(t, ctx, jobs.Acceptance{
			Kind: JobKindPluginAction, KindVersion: jobPluginActionKindVersion, State: jobs.StateQueued,
			Origin: "api", Title: "Async Work", Replay: jobs.ReplayInput{Input: input},
		}).ID)
	}

	adapter := &pluginActionAdapter{ctx: ctx}
	adapter.AdoptWaiting(context.Background())
	adapter.AdoptWaiting(context.Background())

	held := 0
	for _, id := range ids {
		if pm.HostJobHeld(id) {
			held++
		}
		if state := jobStateForTest(t, ctx, id); state != jobs.StateQueued {
			t.Fatalf("job %s is %s while no job slot is free, want queued", id, state)
		}
	}
	if held != pluginActionAdoptDepth || pm.LaneDepth(pluginActionTestPlugin) != pluginActionAdoptDepth {
		t.Fatalf("adoption holds %d of %d waiting jobs in a lane %d deep, want %d",
			held, waiting, pm.LaneDepth(pluginActionTestPlugin), pluginActionAdoptDepth)
	}

	// Once the lane can drain, later passes hand over the rest.
	release()
	adapter.AdoptWaiting(context.Background())
	waitFor(t, "every waiting job to run", func() bool {
		adapter.AdoptWaiting(context.Background())
		for _, id := range ids {
			if !jobStateForTest(t, ctx, id).Terminal() {
				return false
			}
		}
		return true
	})
}

// TestAnOccurrenceAdmittedUnderADeadlinePublishesAfterIt pins where the admission's
// deadline lives: on the claim's own queries, never on the handle the execution
// publishes through for the rest of its run. A handler that finishes after the
// dispatch budget is spent still records its result.
func TestAnOccurrenceAdmittedUnderADeadlinePublishesAfterIt(t *testing.T) {
	ctx := newJobHarnessContext(t, false)
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
	if err := ctx.db.Model(&models.PluginSchedule{}).
		Where("plugin_name = ? AND schedule_id = ?", pluginActionTestPlugin, "slow-tick").
		Update("next_due_at", time.Now().Add(-time.Minute)).Error; err != nil {
		t.Fatalf("make the schedule due: %v", err)
	}

	scheduler := NewPluginScheduler(ctx, time.Minute)
	scheduler.dispatchWait = 150 * time.Millisecond
	scheduler.Tick(time.Now())
	scheduler.Stop()

	occurrence := pluginActionJobBySubtype(t, ctx, pluginActionSubtypeScheduled, 1)
	if occurrence.State != jobs.StateSucceeded {
		t.Fatalf("the slow occurrence ended %s (%+v), want succeeded", occurrence.State, occurrence.Failure)
	}
	outputs, err := ctx.JobService().Outputs(ctx.jobDeps(), jobs.Access{Administrator: true}, occurrence.ID)
	if err != nil {
		t.Fatalf("read the outputs: %v", err)
	}
	if len(outputs) != 1 || outputs[0].Key != "result" {
		t.Fatalf("the occurrence published %+v, want its result", outputs)
	}
}

// TestARefusalThatCannotBeRecordedAtOnceIsRecordedLater pins the one write that
// comes after this process has given up an execution it claimed. A refusal made
// under the claim — here an acting account that was disabled while the action
// waited — ends the execution before anything runs; if recording the block were
// dropped on a transient failure, the Job would stay running, heartbeated, with
// nothing to run it and a slot of the budget held for good.
func TestARefusalThatCannotBeRecordedAtOnceIsRecordedLater(t *testing.T) {
	ctx := newJobHarnessContext(t, false)
	pm := ctx.PluginManager()
	if err := pm.EnablePlugin(pluginActionTestPlugin); err != nil {
		t.Fatalf("enable %s: %v", pluginActionTestPlugin, err)
	}
	actor := models.User{Username: "disabled-actor", Role: models.RoleUser, PasswordHash: "x", Disabled: true}
	if err := ctx.db.Create(&actor).Error; err != nil {
		t.Fatalf("seed the actor: %v", err)
	}

	var failures atomic.Int64
	failures.Store(2)
	if err := ctx.db.Callback().Update().Before("gorm:update").Register("fail-first-blocks", func(db *gorm.DB) {
		updates, ok := db.Statement.Dest.(map[string]any)
		if !ok || db.Statement.Table != "jobs" || updates["state"] != string(jobs.StateBlocked) {
			return
		}
		if failures.Add(-1) >= 0 {
			_ = db.AddError(errors.New("database is locked"))
		}
	}); err != nil {
		t.Fatalf("register the failing callback: %v", err)
	}

	input := &pluginActionJobInput{
		Subtype: pluginActionSubtypeRegistered, Plugin: pluginActionTestPlugin, Action: "async-work",
		EntityType: "resource", Runtime: plugin_system.CurrentRuntimeIdentity().String(),
	}
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("encode the input: %v", err)
	}
	owner := actor.ID
	accepted := acceptJobFor(t, ctx, jobs.Acceptance{
		Kind: JobKindPluginAction, KindVersion: jobPluginActionKindVersion, State: jobs.StateQueued,
		Origin: "api", Title: "Async Work", OwnerUserID: &owner, ActorUserID: &owner,
		Replay: jobs.ReplayInput{Input: raw},
	})
	if err := ctx.queueRegisteredPluginAction(pm, accepted.ID, "refused-handle", &owner, input); err != nil {
		t.Fatalf("queue the action: %v", err)
	}

	waitFor(t, "the refusal to be recorded", func() bool {
		return jobStateForTest(t, ctx, accepted.ID) == jobs.StateBlocked
	})
	if failures.Load() >= 0 {
		t.Fatalf("the injected failures were not all consumed (%d left): the test did not reach the retry", failures.Load())
	}
	if got := pluginKVForTest(t, ctx, "ran"); got != "" {
		t.Fatalf("a refused action ran its handler (%q)", got)
	}
	if held := storedCapacity(t, ctx, jobs.CapacityGroupGlobal); held != 0 {
		t.Fatalf("a blocked action still holds %d slots of the budget", held)
	}
}

// TestAWithdrawalThatCannotBeRecordedAtOnceIsRecordedLater pins the lost callback
// before the claim. A closure whose VM went away can never run, and no other
// process will withdraw it while this one is alive; a withdrawal dropped on a
// transient failure would leave it queued for good.
func TestAWithdrawalThatCannotBeRecordedAtOnceIsRecordedLater(t *testing.T) {
	ctx := newJobHarnessContext(t, false)
	closure := acceptClosureJobForTest(t, ctx, plugin_system.CurrentRuntimeIdentity().String())

	var failures atomic.Int64
	failures.Store(2)
	if err := ctx.db.Callback().Update().Before("gorm:update").Register("fail-first-withdrawals", func(db *gorm.DB) {
		updates, ok := db.Statement.Dest.(map[string]any)
		if !ok || db.Statement.Table != "jobs" || updates["state"] != string(jobs.StateCancelled) {
			return
		}
		if failures.Add(-1) >= 0 {
			_ = db.AddError(errors.New("database is locked"))
		}
	}); err != nil {
		t.Fatalf("register the failing callback: %v", err)
	}

	admission := ctx.newPluginActionAdmission(closure.ID,
		&pluginActionJobInput{Subtype: pluginActionSubtypeClosure, Plugin: pluginActionTestPlugin}, nil)
	admission.CallbackLost("plugin-unavailable")

	waitFor(t, "the withdrawal to be recorded", func() bool {
		return jobStateForTest(t, ctx, closure.ID) == jobs.StateCancelled
	})
	if failures.Load() >= 0 {
		t.Fatalf("the injected failures were not all consumed (%d left): the test did not reach the retry", failures.Load())
	}
}

// acceptOccurrenceForTest accepts one queued occurrence of the fixture's retryable
// schedule, recording this process as the one that accepted it.
func acceptOccurrenceForTest(t *testing.T, ctx *MahresourcesContext) jobs.Snapshot {
	t.Helper()
	raw, err := json.Marshal(pluginActionJobInput{
		Subtype: pluginActionSubtypeScheduled, Plugin: pluginActionTestPlugin, ScheduleID: "retryable-tick",
		Overlap: plugin_system.ScheduleOverlapSkip, Runtime: plugin_system.CurrentRuntimeIdentity().String(),
	})
	if err != nil {
		t.Fatalf("encode the occurrence: %v", err)
	}
	return acceptJobFor(t, ctx, jobs.Acceptance{
		Kind: JobKindPluginAction, KindVersion: jobPluginActionKindVersion, State: jobs.StateQueued,
		Origin: "schedule", Title: "retryable-tick", Replay: jobs.ReplayInput{Input: raw},
	})
}

// TestAnOccurrenceThatRanElsewhereIsReportedAsRun pins the scheduler's answer
// when its occurrence was run by another runtime while it waited: that run is
// this row's run, and reporting it as not started would leave the row due and
// fire the same interval again.
func TestAnOccurrenceThatRanElsewhereIsReportedAsRun(t *testing.T) {
	ctx := newJobHarnessContext(t, false)
	occurrence := acceptOccurrenceForTest(t, ctx)
	execution, err := ctx.JobService().ClaimJob(context.Background(), ctx.jobDeps(), jobs.ClaimRequest{
		Kind: JobKindPluginAction, KindVersion: jobPluginActionKindVersion, JobID: occurrence.ID,
		Claimant: "another-runtime",
	})
	if err != nil {
		t.Fatalf("claim elsewhere: %v", err)
	}
	if _, err := ctx.JobService().Finish(ctx.jobDeps(), jobs.FinishRequest{
		ExecutionRef:    jobs.ExecutionRef{JobID: execution.JobID, ExecutionToken: execution.ExecutionToken},
		ExpectedVersion: execution.Version, Outcome: jobs.StateSucceeded,
	}); err != nil {
		t.Fatalf("finish elsewhere: %v", err)
	}

	admission := ctx.newPluginActionAdmission(occurrence.ID,
		&pluginActionJobInput{Subtype: pluginActionSubtypeScheduled, Plugin: pluginActionTestPlugin}, nil)
	run, err := ctx.settleUnstartedOccurrence(admission)
	if err != nil {
		t.Fatalf("settle: %v", err)
	}
	if !run.Started || run.Failed {
		t.Fatalf("an occurrence another runtime ran to success was reported started=%v failed=%v", run.Started, run.Failed)
	}
}

// TestAFreshOccurrenceIsLeftToItsSchedulerAndARetryIsAdopted pins which scheduled
// occurrences adoption may take. A fresh one belongs to the scheduler holding its
// row, which records its outcome; a Retry's successor has no row waiting on it and
// nothing else would run it.
func TestAFreshOccurrenceIsLeftToItsSchedulerAndARetryIsAdopted(t *testing.T) {
	ctx := newJobHarnessContext(t, false)
	pm := ctx.PluginManager()
	if err := pm.EnablePlugin(pluginActionTestPlugin); err != nil {
		t.Fatalf("enable %s: %v", pluginActionTestPlugin, err)
	}
	adapter := &pluginActionAdapter{ctx: ctx}

	fresh := acceptOccurrenceForTest(t, ctx)
	adapter.AdoptWaiting(context.Background())
	time.Sleep(200 * time.Millisecond)
	if pm.HostJobHeld(fresh.ID) || jobStateForTest(t, ctx, fresh.ID) != jobs.StateQueued {
		t.Fatalf("a fresh occurrence was adopted (held=%v, %s)", pm.HostJobHeld(fresh.ID), jobStateForTest(t, ctx, fresh.ID))
	}

	// Fail the occurrence under a claim and retry it: the successor is adoptable.
	execution, err := ctx.JobService().ClaimJob(context.Background(), ctx.jobDeps(), jobs.ClaimRequest{
		Kind: JobKindPluginAction, KindVersion: jobPluginActionKindVersion, JobID: fresh.ID, Claimant: "test",
	})
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	failed, err := ctx.JobService().Finish(ctx.jobDeps(), jobs.FinishRequest{
		ExecutionRef:    jobs.ExecutionRef{JobID: execution.JobID, ExecutionToken: execution.ExecutionToken},
		ExpectedVersion: execution.Version, Outcome: jobs.StateFailed,
		Failure: &jobs.Failure{Code: "test", Class: jobs.FailureClassInternal, Message: "failed for the test"},
	})
	if err != nil {
		t.Fatalf("fail: %v", err)
	}
	result, err := ctx.JobService().ExecuteCommand(context.Background(), ctx.jobDeps(), jobs.CommandRequest{
		JobID: fresh.ID, Key: jobs.CommandRetry, IdempotencyKey: "retry-occurrence",
		ExpectedVersion: failed.Version, Actor: jobs.Access{Administrator: true},
	})
	if err != nil || result.SuccessorID == "" {
		t.Fatalf("retry: %+v %v", result, err)
	}
	waitFor(t, "the retried occurrence to run", func() bool {
		adapter.AdoptWaiting(context.Background())
		return jobStateForTest(t, ctx, result.SuccessorID) == jobs.StateSucceeded
	})
	if got := pluginKVForTest(t, ctx, "retryable-scheduled"); got != "1" {
		t.Fatalf("the retried occurrence's handler ran %q times, want once", got)
	}
}

// TestAnActionThisProcessCannotRunIsLeftForOneThatCan pins what adoption may
// conclude from a plugin that is not loaded here: nothing, unless the deployment
// has it disabled. Another process may have it loaded and run the action.
func TestAnActionThisProcessCannotRunIsLeftForOneThatCan(t *testing.T) {
	ctx := newJobHarnessContext(t, false)
	adapter := &pluginActionAdapter{ctx: ctx}
	state := models.PluginState{PluginName: "elsewhere-plugin", Enabled: true}
	if err := ctx.db.Create(&state).Error; err != nil {
		t.Fatalf("seed the plugin state: %v", err)
	}
	raw, err := json.Marshal(pluginActionJobInput{
		Subtype: pluginActionSubtypeRegistered, Plugin: "elsewhere-plugin", Action: "work",
		EntityType: "resource", Runtime: "another-host/boot/1",
	})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	job := acceptJobFor(t, ctx, jobs.Acceptance{
		Kind: JobKindPluginAction, KindVersion: jobPluginActionKindVersion, State: jobs.StateQueued,
		Origin: "api", Title: "Work", Replay: jobs.ReplayInput{Input: raw},
	})

	adapter.AdoptWaiting(context.Background())
	if got := jobStateForTest(t, ctx, job.ID); got != jobs.StateQueued {
		t.Fatalf("an action whose plugin is enabled in the deployment but not loaded here is %s, want left queued", got)
	}

	if err := ctx.db.Model(&models.PluginState{}).Where("plugin_name = ?", "elsewhere-plugin").
		Update("enabled", false).Error; err != nil {
		t.Fatalf("disable the plugin: %v", err)
	}
	adapter.AdoptWaiting(context.Background())
	if got := jobStateForTest(t, ctx, job.ID); got != jobs.StateBlocked {
		t.Fatalf("an action whose plugin the deployment has disabled is %s, want blocked", got)
	}
}
