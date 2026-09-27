package application_context

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"

	"mahresources/download_queue"
	"mahresources/jobs"
	"mahresources/models"
	"mahresources/models/query_models"
)

// runningHeldDownload submits a download whose server holds the response, and
// answers its Job and its queue entry once the transfer is under way.
func runningHeldDownload(t *testing.T, ctx *MahresourcesContext, url string, requests *atomic.Int64) (string, *download_queue.DownloadJob) {
	t.Helper()
	submissions := ctx.SubmitRemoteDownloads(&query_models.ResourceFromRemoteCreator{URL: url}, nil, "", "api")
	if len(submissions) != 1 || submissions[0].Err != nil || submissions[0].Job == nil {
		t.Fatalf("submit: %+v", submissions)
	}
	jobID := submissions[0].CanonicalJobID
	waitForSnapshot(t, ctx, jobID, "the transfer to start",
		func(snap jobs.Snapshot) bool { return snap.State == jobs.StateRunning })
	waitFor(t, "the transfer to reach the server", func() bool { return requests.Load() >= 1 })
	entry, ok := ctx.DownloadManager().GetJobByCanonicalJobID(jobID)
	if !ok {
		t.Fatalf("no queue entry runs Job %s", jobID)
	}
	return jobID, entry
}

// A pause is confirmed only when the Job is paused by it. A claim that moved on
// between the attempt's exit and the hold's publication refuses the hold, and the
// pause is answered as not confirmed rather than as "paused" over a Job that is
// still running under its new execution.
func TestAPauseIsNotConfirmedWhenTheClaimMovedBeforeItsHoldWasRecorded(t *testing.T) {
	ctx := newDownloadJobContext(t)
	server, requests, _ := heldTransferServer(t)
	jobID, entry := runningHeldDownload(t, ctx, server.URL+"/claim-moved.bin", requests)

	t.Cleanup(download_queue.SetHoldPublicationHookForTest(func(string) {
		if err := ctx.db.Model(&models.Job{}).Where("id = ?", jobID).
			Update("execution_token", "0192f0aa-0000-7000-8000-00000000c1a1").Error; err != nil {
			t.Errorf("move the claim: %v", err)
		}
	}))

	var pending *download_queue.HoldPendingError
	if err := ctx.DownloadManager().PauseSettled(entry.ID, 500*time.Millisecond); !errors.As(err, &pending) {
		t.Fatalf("a pause whose hold the moved claim refused answered %v, want a pending hold", err)
	}
	if snap := jobSnapshot(t, ctx.JobService(), ctx, jobID); snap.State != jobs.StateRunning {
		t.Fatalf("the Job whose claim moved is %s, want running under its new execution", snap.State)
	}
}

// A cancellation recorded against the Job while its hold was being published owns
// the outcome. The Job ends cancelled, the queue entry follows it, and the pause is
// answered with the cancellation rather than with "paused".
func TestAPauseOvertakenByACancellationAnswersTheCancellation(t *testing.T) {
	ctx := newDownloadJobContext(t)
	server, requests, _ := heldTransferServer(t)
	jobID, entry := runningHeldDownload(t, ctx, server.URL+"/cancel-won.bin", requests)

	t.Cleanup(download_queue.SetHoldPublicationHookForTest(func(string) {
		if err := ctx.db.Model(&models.Job{}).Where("id = ?", jobID).
			Update("control_intent", jobs.ControlIntentCancel).Error; err != nil {
			t.Errorf("record the cancellation: %v", err)
		}
	}))

	var conflict *download_queue.StateConflictError
	if err := ctx.DownloadManager().PauseSettled(entry.ID, 5*time.Second); !errors.As(err, &conflict) ||
		conflict.Status != download_queue.JobStatusCancelled {
		t.Fatalf("a pause a cancellation overtook answered %v, want the cancellation", err)
	}
	if snap := jobSnapshot(t, ctx.JobService(), ctx, jobID); snap.State != jobs.StateCancelled {
		t.Fatalf("the Job is %s, want cancelled", snap.State)
	}
	if status := entry.GetStatus(); status != download_queue.JobStatusCancelled {
		t.Fatalf("the queue entry of the cancelled Job is %s, want cancelled", status)
	}
}

// Asking again after a pause timed out is safe: once the hold is recorded the
// answer is "paused", as it would have been had the first ask waited long enough.
func TestAPauseAskedAgainAfterItTimedOutIsConfirmed(t *testing.T) {
	ctx := newDownloadJobContext(t)
	server, requests, _ := heldTransferServer(t)
	jobID, entry := runningHeldDownload(t, ctx, server.URL+"/asked-again.bin", requests)

	release := make(chan struct{})
	var once sync.Once
	t.Cleanup(download_queue.SetHoldPublicationHookForTest(func(string) {
		once.Do(func() { <-release })
	}))
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})

	var pending *download_queue.HoldPendingError
	if err := ctx.DownloadManager().PauseSettled(entry.ID, 200*time.Millisecond); !errors.As(err, &pending) {
		t.Fatalf("the first ask answered %v, want a pending hold", err)
	}
	close(release)
	if err := ctx.DownloadManager().PauseSettled(entry.ID, 5*time.Second); err != nil {
		t.Fatalf("asking again answered %v, want the hold confirmed", err)
	}
	if snap := jobSnapshot(t, ctx.JobService(), ctx, jobID); snap.State != jobs.StatePaused {
		t.Fatalf("the Job is %s after the pause was confirmed, want paused", snap.State)
	}
}

// A hold whose write failed does not leave the Job running over a paused entry.
// The execution that owns the Job renews its claim for as long as it waits, so no
// lease expiry would ever settle it elsewhere: the owner writes the hold again,
// under its own token, until the Job answers.
func TestAHoldWhoseWriteFailedIsWrittenAgainByItsOwner(t *testing.T) {
	ctx := newDownloadJobContext(t)
	server, requests, _ := heldTransferServer(t)
	jobID, entry := runningHeldDownload(t, ctx, server.URL+"/hold-write-failed.bin", requests)

	var armed atomic.Bool
	armed.Store(true)
	const name = "test:fail-the-first-hold-write"
	if err := ctx.db.Callback().Update().Before("gorm:update").Register(name, func(db *gorm.DB) {
		updates, ok := db.Statement.Dest.(map[string]any)
		if ok && db.Statement.Table == "jobs" && updates["state"] == string(jobs.StatePaused) && armed.CompareAndSwap(true, false) {
			_ = db.AddError(errors.New("injected write failure"))
		}
	}); err != nil {
		t.Fatalf("register the failing write: %v", err)
	}
	t.Cleanup(func() { _ = ctx.db.Callback().Update().Remove(name) })

	if err := ctx.DownloadManager().Pause(entry.ID); err != nil {
		t.Fatalf("pause: %v", err)
	}
	waitForSnapshot(t, ctx, jobID, "the hold to be recorded after its first write failed",
		func(snap jobs.Snapshot) bool { return snap.State == jobs.StatePaused })
	if armed.Load() {
		t.Fatalf("the hold's first write was never attempted, so nothing failed")
	}
	// The entry heard the Job's answer: a pause asked of it now is confirmed.
	if err := ctx.DownloadManager().PauseSettled(entry.ID, 5*time.Second); err != nil {
		t.Fatalf("a pause of the recorded hold answered %v, want it confirmed", err)
	}
}
