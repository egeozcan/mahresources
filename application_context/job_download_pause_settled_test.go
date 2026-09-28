package application_context

import (
	"context"
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

// pauseFromFollowerAtPublication leaves the real queue worker between settling a
// durable pause intent and publishing its held answer. The queue is Paused while
// the Job is still Running, exactly the interval in which a foreground Pause can
// receive a StateConflictError naming Paused.
func pauseFromFollowerAtPublication(t *testing.T, ctx *MahresourcesContext, jobID string, entry *download_queue.DownloadJob) func() {
	t.Helper()
	releasePublication := make(chan struct{})
	holdEntered := make(chan struct{})
	var enteredOnce, releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releasePublication) }) }
	t.Cleanup(download_queue.SetHoldPublicationHookForTest(func(queueJobID string) {
		if queueJobID != entry.ID {
			return
		}
		enteredOnce.Do(func() { close(holdEntered) })
		<-releasePublication
	}))
	t.Cleanup(release)

	recordPauseIntentForTest(t, ctx, jobID)
	claim := storedClaim(t, ctx, jobID)
	nextIntentCheck := time.Time{}
	ctx.deliverControlIntent(jobs.Execution{JobID: jobID, ExecutionToken: claim.ExecutionToken}, entry, &nextIntentCheck)
	select {
	case <-holdEntered:
	case <-time.After(3 * time.Second):
		t.Fatal("executor did not settle the follower pause at the publication hook")
	}
	if status := entry.GetStatus(); status != download_queue.JobStatusPaused {
		t.Fatalf("follower left queue entry %s, want paused", status)
	}
	snap := jobSnapshot(t, ctx.JobService(), ctx, jobID)
	if snap.State != jobs.StateRunning || snap.ControlIntent != jobs.ControlIntentPause {
		t.Fatalf("while hold publication is blocked, Job is %s with intent %q, want running with pause intent", snap.State, snap.ControlIntent)
	}
	return release
}

// A queue entry that is already Paused is not yet a confirmed durable hold. The
// foreground command answers that it is still requested while publication is
// blocked, then answers paused once the same hold is published into the Job.
func TestAPauseCommandWaitsForFollowerHoldAnswer(t *testing.T) {
	ctx := newDownloadJobContext(t)
	server, requests, _ := heldTransferServer(t)
	jobID, entry := runningHeldDownload(t, ctx, server.URL+"/follower-pause-answer.bin", requests)
	releasePublication := pauseFromFollowerAtPublication(t, ctx, jobID, entry)

	command := jobs.CommandExecution{JobID: jobID, Key: jobs.CommandPause, IdempotencyKey: "pause-follower-answer"}
	outcome, err := ctx.downloadAdapterFor(JobKindRemoteDownload).ExecuteCommand(context.Background(), command)
	if err != nil {
		t.Fatalf("pause while the held answer is pending: %v", err)
	}
	if outcome.Status != jobs.CommandStatusSucceeded || outcome.Message != jobDownloadPauseRequestedMessage {
		t.Fatalf("pending follower hold answered %s %q, want succeeded %q", outcome.Status, outcome.Message, jobDownloadPauseRequestedMessage)
	}
	stillRunning := jobSnapshot(t, ctx.JobService(), ctx, jobID)
	if stillRunning.State != jobs.StateRunning || stillRunning.ControlIntent != jobs.ControlIntentPause || entry.GetStatus() != download_queue.JobStatusPaused {
		t.Fatalf("pending hold has Job %s / intent %q and queue %s; want running / pause / paused", stillRunning.State, stillRunning.ControlIntent, entry.GetStatus())
	}

	// Releasing the publication seam makes the Job's durable answer authoritative.
	releasePublication()
	paused := waitForSnapshot(t, ctx, jobID, "the follower's hold to be published", func(snap jobs.Snapshot) bool {
		return snap.State == jobs.StatePaused && snap.ControlIntent == ""
	})
	if paused.Progress.Message != jobDownloadPausedMessage {
		t.Fatalf("published pause says %q, want %q", paused.Progress.Message, jobDownloadPausedMessage)
	}
	outcome, err = ctx.downloadAdapterFor(JobKindRemoteDownload).ExecuteCommand(context.Background(), command)
	if err != nil {
		t.Fatalf("pause after the durable hold was published: %v", err)
	}
	if outcome.Status != jobs.CommandStatusSucceeded || outcome.Message != jobDownloadPausedMessage {
		t.Fatalf("confirmed follower hold answered %s %q, want succeeded %q", outcome.Status, outcome.Message, jobDownloadPausedMessage)
	}
}

// The canonical command surface reports Applied only after the follower's hold
// publication clears the intent and moves the durable Job to Paused.
func TestACanonicalPauseConfirmsFollowerHoldAfterPublication(t *testing.T) {
	ctx := newDownloadJobContext(t)
	server, requests, _ := heldTransferServer(t)
	jobID, entry := runningHeldDownload(t, ctx, server.URL+"/follower-pause-command.bin", requests)
	releasePublication := pauseFromFollowerAtPublication(t, ctx, jobID, entry)
	running := jobSnapshot(t, ctx.JobService(), ctx, jobID)

	type answer struct {
		result jobs.CommandResult
		err    error
	}
	answered := make(chan answer, 1)
	go func() {
		result, err := ctx.ExecuteJobCommand(context.Background(), jobs.CommandRequest{
			JobID: jobID, Key: jobs.CommandPause, IdempotencyKey: "pause-follower-applied", ExpectedVersion: running.Version,
		})
		answered <- answer{result: result, err: err}
	}()

	// A command request advances the version before calling its adapter. Wait for
	// its committed intent before releasing publication; the follower may publish
	// before the foreground command reaches its bounded hold-answer wait.
	waitForSnapshot(t, ctx, jobID, "the foreground pause intent to commit", func(snap jobs.Snapshot) bool {
		return snap.Version > running.Version && snap.State == jobs.StateRunning && snap.ControlIntent == jobs.ControlIntentPause
	})
	releasePublication()

	var result answer
	select {
	case result = <-answered:
	case <-time.After(3 * time.Second):
		t.Fatal("foreground pause did not answer after the follower published its hold")
	}
	if result.err != nil {
		t.Fatalf("pause command: %v", result.err)
	}
	if result.result.Status != jobs.CommandStatusSucceeded || result.result.Code != jobs.CommandCodeApplied || result.result.Message != jobDownloadPausedMessage {
		t.Fatalf("pause answered %s/%s %q, want applied and %q", result.result.Status, result.result.Code, result.result.Message, jobDownloadPausedMessage)
	}
	paused := waitForSnapshot(t, ctx, jobID, "the foreground command's confirmed hold", func(snap jobs.Snapshot) bool {
		return snap.State == jobs.StatePaused && snap.ControlIntent == ""
	})
	if paused.Progress.Message != jobDownloadPausedMessage || entry.GetStatus() != download_queue.JobStatusPaused {
		t.Fatalf("the confirmed pause is Job %s saying %q with queue %s", paused.State, paused.Progress.Message, entry.GetStatus())
	}
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
