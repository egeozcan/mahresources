package application_context

import (
	"context"
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
}
