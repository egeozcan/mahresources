package application_context

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"mahresources/download_queue"
	"mahresources/jobs"
	"mahresources/models"
	"mahresources/models/query_models"
)

// recordPauseIntentForTest records a person's pause against a running Job the way
// the command does when another process runs the transfer: the intent, not yet
// delivered.
func recordPauseIntentForTest(t *testing.T, ctx *MahresourcesContext, jobID string) {
	t.Helper()
	if err := ctx.db.Model(&models.Job{}).Where("id = ?", jobID).
		Updates(map[string]any{"control_intent": jobs.ControlIntentPause, "phase": jobs.PhasePausing}).Error; err != nil {
		t.Fatalf("record the pause: %v", err)
	}
}

// TestAPendingPauseSurvivesTheLossOfTheProcessHoldingTheTransfer: the process
// running a download is proved gone after a person asked to pause it and before it
// held the transfer. Reconciliation would queue the Job and a runtime would start
// the download again over the pause; the person asked for it to wait, so it ends
// paused, with its row saying what Resume does.
func TestAPendingPauseSurvivesTheLossOfTheProcessHoldingTheTransfer(t *testing.T) {
	ctx := newJobHarnessContext(t, false)
	input, err := json.Marshal(downloadJobInput{
		Creator: &query_models.ResourceFromRemoteCreator{URL: "https://example.invalid/paused-then-lost.bin"},
	})
	if err != nil {
		t.Fatalf("encode the input: %v", err)
	}
	accepted := acceptJobFor(t, ctx, jobs.Acceptance{
		Kind: JobKindRemoteDownload, KindVersion: jobDownloadKindVersion,
		State: jobs.StateQueued, Origin: "api",
		Replay: jobs.ReplayInput{Input: input},
	})
	if _, claimed, err := ctx.JobService().Claim(context.Background(), ctx.jobDeps(), jobs.ClaimRequest{
		Kind: JobKindRemoteDownload, KindVersion: jobDownloadKindVersion,
		JobID: accepted.ID, Claimant: goneRuntimeIdentityForTest(), Lease: 20 * time.Millisecond,
	}); err != nil || !claimed {
		t.Fatalf("claim: claimed=%v err=%v", claimed, err)
	}
	recordPauseIntentForTest(t, ctx, accepted.ID)
	time.Sleep(40 * time.Millisecond)

	if decision := reconcileOnce(t, ctx, accepted.ID); decision != jobs.ReconcilePause {
		t.Fatalf("a lost transfer with a pending pause was decided %q, want paused", decision)
	}
	paused := jobSnapshot(t, ctx.JobService(), ctx, accepted.ID)
	if paused.State != jobs.StatePaused || paused.ControlIntent != "" || paused.Progress.Message != jobDownloadPausedMessage {
		t.Fatalf("the Job is %s (intent %q) saying %q, want paused saying what Resume does",
			paused.State, paused.ControlIntent, paused.Progress.Message)
	}
	if held := storedCapacity(t, ctx, jobs.CapacityGroupGlobal); held != 0 {
		t.Fatalf("the paused Job still holds %d capacity slots", held)
	}

	// Without a pending pause the same loss queues the work again, as before.
	other := acceptJobFor(t, ctx, jobs.Acceptance{
		Kind: JobKindRemoteDownload, KindVersion: jobDownloadKindVersion,
		State: jobs.StateQueued, Origin: "api",
		Replay: jobs.ReplayInput{Input: input},
	})
	if _, claimed, err := ctx.JobService().Claim(context.Background(), ctx.jobDeps(), jobs.ClaimRequest{
		Kind: JobKindRemoteDownload, KindVersion: jobDownloadKindVersion,
		JobID: other.ID, Claimant: goneRuntimeIdentityForTest(), Lease: 20 * time.Millisecond,
	}); err != nil || !claimed {
		t.Fatalf("claim the other: claimed=%v err=%v", claimed, err)
	}
	time.Sleep(40 * time.Millisecond)
	if decision := reconcileOnce(t, ctx, other.ID); decision != jobs.ReconcileQueue {
		t.Fatalf("a lost transfer with no pause was decided %q, want queued", decision)
	}
}

// TestAPendingPauseSurvivesAShutdownHandBack: the server stops gracefully while a
// person's pause is on its way to the transfer it is running. The shutdown hands
// running downloads back to the queue, and one the person asked to pause would then
// start again elsewhere; it ends paused instead.
//
// The hand-back is driven directly: the transfer's own wait loop delivers a pause
// within a second, so a real shutdown races that delivery, and either answer ends
// the Job paused (the second part checks that end to end).
func TestAPendingPauseSurvivesAShutdownHandBack(t *testing.T) {
	ctx := newJobHarnessContext(t, false)
	server := stallingDownloadServer(t)
	submissions := ctx.SubmitRemoteDownloads(&query_models.ResourceFromRemoteCreator{URL: server.URL + "/paused-at-shutdown.bin"}, nil, "", "api")
	if len(submissions) != 1 || submissions[0].Err != nil || submissions[0].Job == nil {
		t.Fatalf("submit: %+v", submissions)
	}
	jobID := submissions[0].CanonicalJobID
	waitForSnapshot(t, ctx, jobID, "the transfer to run", func(snap jobs.Snapshot) bool {
		return snap.State == jobs.StateRunning && submissions[0].Job.GetStatus() == download_queue.JobStatusDownloading
	})
	token := storedClaim(t, ctx, jobID).ExecutionToken
	recordPauseIntentForTest(t, ctx, jobID)

	if err := ctx.requeueDownloadExecution(jobs.Execution{JobID: jobID, ExecutionToken: token},
		JobDownloadServerShutdownReason, downloadPhaseQueued, "Stopped by a server shutdown; it starts again from the beginning"); err != nil {
		t.Fatalf("hand the Job back: %v", err)
	}
	snap := jobSnapshot(t, ctx.JobService(), ctx, jobID)
	if snap.State != jobs.StatePaused || snap.ControlIntent != "" || snap.Progress.Message != jobDownloadPausedMessage {
		t.Fatalf("the handed-back Job is %s (intent %q) saying %q, want paused", snap.State, snap.ControlIntent, snap.Progress.Message)
	}

	// End to end: a second download, paused and then caught by the shutdown.
	second := ctx.SubmitRemoteDownloads(&query_models.ResourceFromRemoteCreator{URL: server.URL + "/paused-at-shutdown-2.bin"}, nil, "", "api")
	if len(second) != 1 || second[0].Err != nil || second[0].Job == nil {
		t.Fatalf("submit the second: %+v", second)
	}
	secondID := second[0].CanonicalJobID
	waitForSnapshot(t, ctx, secondID, "the second transfer to run", func(snap jobs.Snapshot) bool {
		return snap.State == jobs.StateRunning && second[0].Job.GetStatus() == download_queue.JobStatusDownloading
	})
	recordPauseIntentForTest(t, ctx, secondID)
	ctx.downloadManager.Shutdown()
	ended := waitForSnapshot(t, ctx, secondID, "the second Job to leave running", func(snap jobs.Snapshot) bool {
		return snap.State != jobs.StateRunning
	})
	if ended.State != jobs.StatePaused {
		t.Fatalf("a download paused as the server stopped is %s, want paused", ended.State)
	}
}
