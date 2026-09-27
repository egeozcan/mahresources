package application_context

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"mahresources/jobs"
	"mahresources/models"
	"mahresources/plugin_system"
)

// This file pins what a plugin-action Job records when the host, not the
// handler, ends its execution: a disable, a shutdown, and an admission that does
// not reach its handler after all. Each is one outcome that says why, and none is
// a failure of the plugin's work.

// waitForLongWork waits until the long-work handler of this context's plugin is
// running.
func waitForLongWork(t *testing.T, ctx *MahresourcesContext, jobID string) {
	t.Helper()
	waitForJobState(t, ctx, jobID, "the long action to run", func(s jobs.Snapshot) bool {
		return s.State == jobs.StateRunning
	})
	deadline := time.Now().Add(10 * time.Second)
	for pluginKVForTest(t, ctx, "long") != "running" {
		if time.Now().After(deadline) {
			t.Fatal("the long action never entered its handler")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// TestDisablingAPluginInterruptsItsRunningActionWithAReason pins C5's outcome: the
// handler of a disabled plugin is stopped rather than left running on a revoked
// VM, and its Job says why instead of reading as a failure of the plugin's work.
func TestDisablingAPluginInterruptsItsRunningActionWithAReason(t *testing.T) {
	ctx := newPluginActionJobContext(t)
	_, jobID, err := ctx.RunPluginActionAsync(nil, pluginActionTestPlugin, "long-work", 1, nil, "")
	if err != nil {
		t.Fatalf("run the long action: %v", err)
	}
	waitForLongWork(t, ctx, jobID)

	if err := ctx.PluginManager().DisablePlugin(pluginActionTestPlugin); err != nil {
		t.Fatalf("disable: %v", err)
	}
	job := waitForJobState(t, ctx, jobID, "the stopped action to end", func(s jobs.Snapshot) bool {
		return s.State.Terminal()
	})
	if job.State != jobs.StateInterrupted || job.Failure == nil || job.Failure.Code != plugin_system.StopPluginDisabled {
		t.Fatalf("the action of a disabled plugin ended %s (%+v), want interrupted because the plugin was disabled",
			job.State, job.Failure)
	}
	if job.Failure.Message != "The plugin was disabled while this was running." {
		t.Fatalf("the interruption says %q", job.Failure.Message)
	}
	if got := pluginKVForTest(t, ctx, "long"); got == "finished" {
		t.Fatal("the handler ran to its end after its plugin was disabled")
	}
}

// TestAShutdownInterruptsTheRunningActionAndLeavesQueuedWorkForTheNextProcess
// pins R2's one-classification rule for plugin work. The handler that is running
// when the grace period ends is stopped and its Job is interrupted with the
// shutdown as its reason; the action waiting behind it never started, so it stays
// queued for the next process rather than being failed or interrupted.
func TestAShutdownInterruptsTheRunningActionAndLeavesQueuedWorkForTheNextProcess(t *testing.T) {
	defer plugin_system.SetShutdownBoundsForTest(200*time.Millisecond, 2*time.Second, 2*time.Second)()
	ctx := newPluginActionJobContext(t)
	_, running, err := ctx.RunPluginActionAsync(nil, pluginActionTestPlugin, "long-work", 1, nil, "")
	if err != nil {
		t.Fatalf("run the long action: %v", err)
	}
	waitForLongWork(t, ctx, running)
	_, waiting, err := ctx.RunPluginActionAsync(nil, pluginActionTestPlugin, "async-work", 2, nil, "")
	if err != nil {
		t.Fatalf("queue an action behind it: %v", err)
	}

	began := time.Now()
	ctx.PluginManager().Close()
	if took := time.Since(began); took > 5*time.Second {
		t.Fatalf("the plugin manager took %s to close with a long handler running", took)
	}

	stopped, err := ctx.JobService().Get(ctx.jobDeps(), jobs.Access{Administrator: true}, running)
	if err != nil {
		t.Fatalf("read the running action: %v", err)
	}
	if stopped.State != jobs.StateInterrupted || stopped.Failure == nil ||
		stopped.Failure.Code != plugin_system.StopRuntimeStopping {
		t.Fatalf("the running action ended %s (%+v), want interrupted by the shutdown", stopped.State, stopped.Failure)
	}
	if stopped.Failure.Message != "The server shut down while this was running." {
		t.Fatalf("the interruption says %q", stopped.Failure.Message)
	}
	if got := jobStateForTest(t, ctx, waiting); got != jobs.StateQueued {
		t.Fatalf("the action that never started is %s, want queued for the next process", got)
	}
}

// TestAnAdmittedExecutionThatIsNotEnteredEndsByWhetherItCanRunElsewhere pins the
// host's answer when an execution was claimed and then did not enter its handler:
// a registered action goes back to the queue under its own claim, and a closure,
// whose function dies with this process, is withdrawn as never started. Neither
// is a failure.
func TestAnAdmittedExecutionThatIsNotEnteredEndsByWhetherItCanRunElsewhere(t *testing.T) {
	ctx := newPluginActionJobContext(t)

	actor := models.User{Username: "not-entered-actor", Role: models.RoleUser, PasswordHash: "x"}
	if err := ctx.db.Create(&actor).Error; err != nil {
		t.Fatalf("seed the actor: %v", err)
	}
	registered, _ := acceptRegisteredActionForTest(t, ctx, actor.ID, 1)
	execution, err := ctx.JobService().ClaimJob(context.Background(), ctx.jobDeps(), jobs.ClaimRequest{
		Kind: JobKindPluginAction, KindVersion: jobPluginActionKindVersion, JobID: registered.ID,
		Claimant: plugin_system.CurrentRuntimeIdentity().String(),
	})
	if err != nil {
		t.Fatalf("claim the registered action: %v", err)
	}
	input, err := pluginActionInputOf(execution.Input)
	if err != nil {
		t.Fatalf("read the claimed input: %v", err)
	}
	newPluginActionSink(ctx, execution, input).NotStarted(plugin_system.StopRuntimeStopping)
	if got := jobStateForTest(t, ctx, registered.ID); got != jobs.StateQueued {
		t.Fatalf("a registered action admitted and not entered is %s, want queued", got)
	}

	closure := acceptClosureJobForTest(t, ctx, plugin_system.CurrentRuntimeIdentity().String())
	claimed, err := ctx.JobService().ClaimJob(context.Background(), ctx.jobDeps(), jobs.ClaimRequest{
		Kind: JobKindPluginAction, KindVersion: jobPluginActionKindVersion, JobID: closure.ID,
		Claimant: plugin_system.CurrentRuntimeIdentity().String(),
	})
	if err != nil {
		t.Fatalf("claim the closure: %v", err)
	}
	newPluginActionSink(ctx, claimed, &pluginActionJobInput{Subtype: pluginActionSubtypeClosure, Plugin: pluginActionTestPlugin}).
		NotStarted(plugin_system.StopPluginDisabled)
	if got := jobStateForTest(t, ctx, closure.ID); got != jobs.StateCancelled || lastEventType(t, ctx, closure.ID) != pluginActionNotStartedEvent {
		t.Fatalf("a closure admitted and not entered is %s (last event %q), want withdrawn as never started",
			got, lastEventType(t, ctx, closure.ID))
	}
	// The phase says it did not start, on the ended Job, where the list's phase
	// label reads it; the terminal transition must not reset it.
	if withdrawn := jobSnapshot(t, ctx.JobService(), ctx, closure.ID); withdrawn.Phase != pluginActionNotStartedEvent ||
		withdrawn.Progress.Message != "the plugin was disabled before this ran" {
		t.Fatalf("the withdrawn closure reads phase %q, message %q", withdrawn.Phase, withdrawn.Progress.Message)
	}
}

// cancelJobForTest runs the Cancel command on one Job as an administrator.
func cancelJobForTest(t *testing.T, ctx *MahresourcesContext, jobID, idempotencyKey string) error {
	t.Helper()
	snap := jobSnapshot(t, ctx.JobService(), ctx, jobID)
	_, err := ctx.ExecuteJobCommand(context.Background(), jobs.CommandRequest{
		JobID: jobID, Key: jobs.CommandCancel, IdempotencyKey: idempotencyKey,
		ExpectedVersion: snap.Version,
	})
	return err
}

// TestAQueuedPluginActionCanBeCancelledBeforeItStarts pins C4 for work that has
// not started: a Job waiting for its plugin is offered Cancel whatever its
// registration says, because none of it has run, and cancelling it means its
// handler never runs.
func TestAQueuedPluginActionCanBeCancelledBeforeItStarts(t *testing.T) {
	ctx := newPluginActionJobContext(t)
	release := ctx.PluginManager().FillJobBudgetForTest()
	defer release()
	before := pluginKVForTest(t, ctx, "ran")

	_, jobID, err := ctx.RunPluginActionAsync(nil, pluginActionTestPlugin, "async-work", 1, nil, "")
	if err != nil {
		t.Fatalf("run the action: %v", err)
	}
	if !offersCommand(advertisedForTest(t, ctx, jobID), jobs.CommandCancel) {
		t.Fatalf("a queued plugin action offers %+v, want Cancel", advertisedForTest(t, ctx, jobID))
	}
	if err := cancelJobForTest(t, ctx, jobID, "cancel-queued"); err != nil {
		t.Fatalf("cancel the queued action: %v", err)
	}
	if got := jobStateForTest(t, ctx, jobID); got != jobs.StateCancelled {
		t.Fatalf("the cancelled action is %s, want cancelled", got)
	}

	release()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if ctx.PluginManager().LaneDepth(pluginActionTestPlugin) == 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if got := pluginKVForTest(t, ctx, "ran"); got != before {
		t.Fatalf("the handler of a cancelled action ran (ran %q -> %q)", before, got)
	}
}

// TestARunningPluginActionIsCancelledOnlyWhereItsRegistrationAllows pins C4 for
// a running handler. Stopping it partway is only offered where the registration
// declares that is safe (cancel = true); there it ends the handler at its next
// step and the Job cancelled, not failed or interrupted. Without the declaration
// Cancel is neither offered nor accepted.
func TestARunningPluginActionIsCancelledOnlyWhereItsRegistrationAllows(t *testing.T) {
	ctx := newPluginActionJobContext(t)

	_, cancellable, err := ctx.RunPluginActionAsync(nil, pluginActionTestPlugin, "cancellable-work", 1, nil, "")
	if err != nil {
		t.Fatalf("run the cancellable action: %v", err)
	}
	waitForLongWork(t, ctx, cancellable)
	if !offersCommand(advertisedForTest(t, ctx, cancellable), jobs.CommandCancel) {
		t.Fatalf("a running action that declares cancel = true offers %+v, want Cancel",
			advertisedForTest(t, ctx, cancellable))
	}
	if err := cancelJobForTest(t, ctx, cancellable, "cancel-running"); err != nil {
		t.Fatalf("cancel the running action: %v", err)
	}
	job := waitForJobState(t, ctx, cancellable, "the cancelled action to end", func(s jobs.Snapshot) bool {
		return s.State.Terminal()
	})
	if job.State != jobs.StateCancelled {
		t.Fatalf("the cancelled action ended %s (%+v), want cancelled", job.State, job.Failure)
	}
	if got := pluginKVForTest(t, ctx, "long"); got == "finished" {
		t.Fatal("the handler ran to its end after it was cancelled")
	}

	if err := ctx.PluginKVSet(pluginActionTestPlugin, "long", `""`); err != nil {
		t.Fatalf("reset the plugin's marker: %v", err)
	}
	_, plain, err := ctx.RunPluginActionAsync(nil, pluginActionTestPlugin, "long-work", 2, nil, "")
	if err != nil {
		t.Fatalf("run the plain action: %v", err)
	}
	waitForLongWork(t, ctx, plain)
	if offersCommand(advertisedForTest(t, ctx, plain), jobs.CommandCancel) {
		t.Fatal("a running action that does not declare cancel = true offers Cancel")
	}
	if err := cancelJobForTest(t, ctx, plain, "cancel-plain"); err == nil {
		t.Fatal("a Cancel of a running action that does not declare it was accepted")
	}
	if got := jobStateForTest(t, ctx, plain); got != jobs.StateRunning {
		t.Fatalf("the refused cancel left the action %s, want running", got)
	}
}

// TestACancelRecordedByAnotherProcessStopsTheHandler pins the multi-process half:
// the process running a handler is not necessarily the one that ran the Cancel
// command, so it reads the recorded intent from the Job and stops its handler.
func TestACancelRecordedByAnotherProcessStopsTheHandler(t *testing.T) {
	ctx := newPluginActionJobContext(t)
	_, jobID, err := ctx.RunPluginActionAsync(nil, pluginActionTestPlugin, "cancellable-work", 1, nil, "")
	if err != nil {
		t.Fatalf("run the cancellable action: %v", err)
	}
	waitForLongWork(t, ctx, jobID)

	// What the Cancel command in another process leaves behind: the intent on the
	// Job, and no call into this process.
	if err := ctx.db.Model(&models.Job{}).Where("id = ?", jobID).
		Updates(map[string]any{"control_intent": jobs.ControlIntentCancel, "control_requested_at": time.Now()}).Error; err != nil {
		t.Fatalf("record the cancellation: %v", err)
	}
	job := waitForJobState(t, ctx, jobID, "the action to stop", func(s jobs.Snapshot) bool {
		return s.State.Terminal()
	})
	if job.State != jobs.StateCancelled {
		t.Fatalf("the action cancelled from elsewhere ended %s (%+v), want cancelled", job.State, job.Failure)
	}
}

// TestAJobHeldByAProcessThatIsGoneIsReconciledWithoutWaitingForItsLease pins R4's
// "running with frozen progress for two minutes after a crash". The process that
// claimed the Job is proved gone — this host, this boot, no such process — so the
// runtime reconciles it on its next tick: a plugin action ends interrupted, and it
// says why.
func TestAJobHeldByAProcessThatIsGoneIsReconciledWithoutWaitingForItsLease(t *testing.T) {
	ctx := newJobHarnessContext(t, true)
	current := plugin_system.CurrentRuntimeIdentity()
	if current.BootSession == "" {
		t.Skip("this platform records no boot session, so no process can be proved gone")
	}
	// This process's own identity with another process's id: whatever else an
	// identity records, only the process differs.
	deadIdentity := current
	deadIdentity.PID = unusedPIDForTest(t)
	dead := deadIdentity.String()
	job := acceptClosureJobForTest(t, ctx, dead)
	claimJobForTestAs(t, ctx, job.ID, dead)

	ended := waitForJobState(t, ctx, job.ID, "the abandoned Job to be reconciled", func(s jobs.Snapshot) bool {
		return s.State.Terminal()
	})
	if ended.State != jobs.StateInterrupted || ended.Failure == nil || ended.Failure.Code != jobs.ReconcileRuntimeLostCode {
		t.Fatalf("a Job whose process is gone ended %s (%+v), want interrupted because its runtime was lost",
			ended.State, ended.Failure)
	}
}

// unusedPIDForTest answers a process id on this host that the runtime identity's
// own liveness proof reads as gone.
func unusedPIDForTest(t *testing.T) int {
	t.Helper()
	current := plugin_system.CurrentRuntimeIdentity()
	for pid := 4_000_000; pid > 3_000_000; pid -= 7919 {
		candidate := current
		candidate.PID = pid
		if candidate.Liveness() == plugin_system.RuntimeGone {
			return pid
		}
	}
	t.Skip("no process id that is provably unused was found")
	return 0
}

// TestAHandlerLostAtShutdownSaysWhy pins the last-resort path of a shutdown: a
// handler that would not stop is reported lost, and its Job says the server shut
// down rather than ending interrupted with no reason.
func TestAHandlerLostAtShutdownSaysWhy(t *testing.T) {
	ctx := newPluginActionJobContext(t)
	actor := models.User{Username: "lost-actor", Role: models.RoleUser, PasswordHash: "x"}
	if err := ctx.db.Create(&actor).Error; err != nil {
		t.Fatalf("seed the actor: %v", err)
	}
	accepted, input := acceptRegisteredActionForTest(t, ctx, actor.ID, 1)
	execution, err := ctx.JobService().ClaimJob(context.Background(), ctx.jobDeps(), jobs.ClaimRequest{
		Kind: JobKindPluginAction, KindVersion: jobPluginActionKindVersion, JobID: accepted.ID,
		Claimant: plugin_system.CurrentRuntimeIdentity().String(),
	})
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	newPluginActionSink(ctx, execution, input).CallbackLost(plugin_system.StopRuntimeStopping)
	job := jobSnapshot(t, ctx.JobService(), ctx, accepted.ID)
	if job.State != jobs.StateInterrupted || job.Failure == nil ||
		job.Failure.Message != "The server shut down while this was running." {
		t.Fatalf("a lost handler's Job is %s (%+v), want interrupted with the shutdown as its reason", job.State, job.Failure)
	}
}

// TestCancelFollowsWhatTheJobRecordsNotTheCurrentRegistration pins where a
// running handler's permission to be stopped comes from. The Job records it at
// acceptance, and every process reads that one fact, so a process whose
// plugin.lua declares cancel = true cannot authorize stopping a Job accepted
// against one that did not; and a Job accepted as cancellable is not run by a
// registration that no longer allows it.
func TestCancelFollowsWhatTheJobRecordsNotTheCurrentRegistration(t *testing.T) {
	ctx := newPluginActionJobContext(t)
	actor := models.User{Username: "cancel-authority-actor", Role: models.RoleUser, PasswordHash: "x"}
	if err := ctx.db.Create(&actor).Error; err != nil {
		t.Fatalf("seed the actor: %v", err)
	}
	owner := actor.ID

	// Accepted against a registration that did not declare cancel, though the
	// one registered now does.
	raw, err := json.Marshal(pluginActionJobInput{
		Subtype: pluginActionSubtypeRegistered, Plugin: pluginActionTestPlugin, Action: "cancellable-work",
		EntityType: "resource", EntityID: 1, Runtime: plugin_system.CurrentRuntimeIdentity().String(),
	})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	unmarked := acceptJobFor(t, ctx, jobs.Acceptance{
		Kind: JobKindPluginAction, KindVersion: jobPluginActionKindVersion, State: jobs.StateQueued,
		Origin: "api", Title: "Cancellable Work", OwnerUserID: &owner, ActorUserID: &owner,
		Replay: jobs.ReplayInput{Input: raw},
	})
	if _, err := ctx.JobService().ClaimJob(context.Background(), ctx.jobDeps(), jobs.ClaimRequest{
		Kind: JobKindPluginAction, KindVersion: jobPluginActionKindVersion, JobID: unmarked.ID,
		Claimant: plugin_system.CurrentRuntimeIdentity().String(),
	}); err != nil {
		t.Fatalf("claim: %v", err)
	}
	if offersCommand(advertisedForTest(t, ctx, unmarked.ID), jobs.CommandCancel) {
		t.Fatal("a running Job accepted without cancel = true offers Cancel because today's registration declares it")
	}

	// Accepted as cancellable, against a registration that no longer is.
	marked := &pluginActionJobInput{
		Subtype: pluginActionSubtypeRegistered, Plugin: pluginActionTestPlugin, Action: "long-work",
		EntityType: "resource", EntityID: 1, Cancellable: true,
	}
	if refusal := ctx.pluginActionRegistrationRefusal(ctx.PluginManager(), marked); refusal != "registration-changed" {
		t.Fatalf("a Job accepted as cancellable against a registration that no longer is was refused %q, want registration-changed", refusal)
	}
}

// TestACancelledScheduledRunIsRecordedAsCancelled pins the schedule row's history
// for a run a person cancelled: it did not complete, so it is not recorded as a
// completed run.
func TestACancelledScheduledRunIsRecordedAsCancelled(t *testing.T) {
	ctx := newPluginActionJobContext(t)
	pm := ctx.PluginManager()
	operator := models.User{Username: "cancelled-run-operator", Role: models.RoleAdmin, PasswordHash: "x"}
	if err := ctx.db.Create(&operator).Error; err != nil {
		t.Fatalf("seed operator: %v", err)
	}
	ctx.refreshRootAdmin()
	if err := ctx.SyncPluginSchedules(pluginActionTestPlugin, pm.DeclaredSchedules(pluginActionTestPlugin)); err != nil {
		t.Fatalf("sync schedules: %v", err)
	}
	scheduler := NewPluginScheduler(ctx, time.Minute)
	defer scheduler.Stop()
	if err := scheduler.RunNow(pluginActionTestPlugin, "cancellable-tick"); err != nil {
		t.Fatalf("run now: %v", err)
	}
	running := pluginActionJobBySubtype(t, ctx, pluginActionSubtypeScheduled, 1)
	waitForLongWork(t, ctx, running.ID)
	if err := cancelJobForTest(t, ctx, running.ID, "cancel-scheduled-run"); err != nil {
		t.Fatalf("cancel the run: %v", err)
	}
	waitForJobState(t, ctx, running.ID, "the run to end", func(s jobs.Snapshot) bool { return s.State.Terminal() })

	deadline := time.Now().Add(10 * time.Second)
	for {
		var row models.PluginSchedule
		if err := ctx.db.Where("plugin_name = ? AND schedule_id = ?", pluginActionTestPlugin, "cancellable-tick").First(&row).Error; err != nil {
			t.Fatalf("read the row: %v", err)
		}
		if row.LastStatus != "" {
			if row.LastStatus != models.PluginScheduleStatusCancelled {
				t.Fatalf("a cancelled run is recorded %q on its schedule, want %q", row.LastStatus, models.PluginScheduleStatusCancelled)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("the cancelled run never recorded its outcome on the schedule")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// TestOnlyAClaimantOfThisBootSessionIsExpiredEarly pins what counts as proof that
// the process holding a claim is gone. A hostname is not unique across machines:
// a claim recorded under this hostname with another boot session may belong to a
// live machine configured with the same name, so it waits for its lease. Only
// this boot session's missing process is proof.
func TestOnlyAClaimantOfThisBootSessionIsExpiredEarly(t *testing.T) {
	current := plugin_system.CurrentRuntimeIdentity()
	if current.BootSession == "" {
		t.Skip("this platform records no boot session, so no process can be proved gone")
	}
	otherBoot := current
	otherBoot.BootSession = "another-machine-with-this-name"
	if runtimeClaimantGone(otherBoot.String()) {
		t.Fatal("a claimant with this hostname and another boot session was treated as gone")
	}
	gone := current
	gone.PID = unusedPIDForTest(t)
	if !runtimeClaimantGone(gone.String()) {
		t.Fatal("a claimant of this boot session with no such process was not treated as gone")
	}
	if runtimeClaimantGone(current.String()) {
		t.Fatal("this very process was treated as gone")
	}
	if runtimeClaimantGone("plugin-command:0123") {
		t.Fatal("a claimant that is not a runtime identity was treated as gone")
	}
}
