package application_context

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"

	"mahresources/download_queue"
	"mahresources/jobs"
	"mahresources/models"
	"mahresources/models/query_models"
)

// A deferred download is two records: the plugin's scheduled_downloads row, which
// the management surfaces list and the scheduler sweeps, and the canonical
// scheduled Job, which the Job Center shows and the dispatch loop runs at its due
// time. Cancelling either one cancels the deferral, so it must end both.

func deferredDownloadJob(t *testing.T, ctx *MahresourcesContext, rowID uint) jobs.Snapshot {
	t.Helper()
	job, err := ctx.ResolveJobHandle(ScheduledDownloadHandleNamespace, strconv.FormatUint(uint64(rowID), 10))
	if err != nil {
		t.Fatalf("resolve the Job of scheduled download %d: %v", rowID, err)
	}
	return job
}

func cancelJobAsItsOwner(t *testing.T, ctx *MahresourcesContext, job jobs.Snapshot) {
	t.Helper()
	result, err := ctx.ExecuteJobCommand(context.Background(), jobs.CommandRequest{
		JobID: job.ID, Key: jobs.CommandCancel, IdempotencyKey: "cancel-" + job.ID, ExpectedVersion: job.Version,
	})
	if err != nil || result.Status != jobs.CommandStatusSucceeded {
		t.Fatalf("cancel Job %s = %+v, %v", job.ID, result, err)
	}
}

// runOneDispatchPass runs the dispatch loop once, which claims a scheduled Job
// whose time has come whether or not the row sweep has seen it.
func runOneDispatchPass(ctx *MahresourcesContext) {
	runtime := NewJobRuntime(ctx, ctx.JobService(), JobRuntimeConfig{Claimant: "deferred-cancel-test", Interval: time.Hour})
	runtime.tick(context.Background())
	runtime.Stop()
}

func TestCancellingADeferredDownloadsJobCancelsItsRow(t *testing.T) {
	ctx, key, actor, _ := newRetiredDeferredDownloadContext(t)
	row, err := ctx.CreateScheduledDownload(downloadTestPlugin, actor.ID,
		&query_models.ResourceFromRemoteCreator{URL: "https://example.invalid/cancelled.bin"}, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("create the deferred download: %v", err)
	}
	cancelJobAsItsOwner(t, ctx, deferredDownloadJob(t, ctx, row.ID))

	if got := scheduledDownloadRow(t, ctx, row.ID); got.Status != models.ScheduledDownloadStatusCancelled {
		t.Fatalf("the row is %s after its Job was cancelled, want cancelled", got.Status)
	}
	fireDueDeferredDownloads(t, ctx, time.Now().Add(2*time.Hour))
	if got := scheduledDownloadRow(t, ctx, row.ID); got.Status != models.ScheduledDownloadStatusCancelled || got.JobID != "" {
		t.Fatalf("the cancelled row is %s naming job %q after its due time, want cancelled and naming none", got.Status, got.JobID)
	}

	ctx = restartJobProcess(t, ctx, key)
	requireCleanBoot(t, ctx)
}

func TestCancellingADeferredRowCancelsItsJob(t *testing.T) {
	ctx, _, actor, _ := newRetiredDeferredDownloadContext(t)
	row, err := ctx.CreateScheduledDownload(downloadTestPlugin, actor.ID,
		&query_models.ResourceFromRemoteCreator{URL: "https://example.invalid/row-cancelled.bin"}, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("create the deferred download: %v", err)
	}
	cancelled, err := ctx.CancelScheduledDownload(row.ID)
	if err != nil || !cancelled {
		t.Fatalf("cancel the row = %v, %v", cancelled, err)
	}
	job := deferredDownloadJob(t, ctx, row.ID)
	if job.State != jobs.StateCancelled {
		t.Fatalf("the Job is %s after its row was cancelled, want cancelled", job.State)
	}

	// Past the due time, the dispatch loop has nothing to run.
	if err := ctx.db.Model(&models.Job{}).Where("id = ?", job.ID).
		Update("scheduled_for", time.Now().Add(-time.Minute).UTC()).Error; err != nil {
		t.Fatalf("bring the due time forward: %v", err)
	}
	runOneDispatchPass(ctx)
	if _, dispatched := ctx.DownloadManager().GetJobByCanonicalJobID(job.ID); dispatched {
		t.Fatalf("a cancelled deferred download was dispatched")
	}
	if after := deferredDownloadJob(t, ctx, row.ID); after.State != jobs.StateCancelled || after.StartedAt != nil {
		t.Fatalf("the cancelled Job is %s (started %v) after a dispatch pass", after.State, after.StartedAt)
	}
}

// Once the dispatch loop has run the Job, the deferral is no longer something the
// row can cancel: the download started.
func TestADeferredRowWhoseJobAlreadyRanCannotBeCancelled(t *testing.T) {
	ctx, _, actor, _ := newRetiredDeferredDownloadContext(t)
	row, err := ctx.CreateScheduledDownload(downloadTestPlugin, actor.ID,
		&query_models.ResourceFromRemoteCreator{URL: "https://example.invalid/already-ran.bin"}, time.Now().Add(-time.Second))
	if err != nil {
		t.Fatalf("create the deferred download: %v", err)
	}
	runOneDispatchPass(ctx)
	job := waitForSnapshot(t, ctx, deferredDownloadJob(t, ctx, row.ID).ID, "the deferred Job to run",
		func(snap jobs.Snapshot) bool { return snap.StartedAt != nil })

	cancelled, err := ctx.CancelScheduledDownload(row.ID)
	if err != nil || cancelled {
		t.Fatalf("cancel the row of a Job that already ran = %v, %v; want refused", cancelled, err)
	}
	if got := scheduledDownloadRow(t, ctx, row.ID); got.Status == models.ScheduledDownloadStatusCancelled {
		t.Fatalf("the row of a download that started reads cancelled")
	}
	if after := deferredDownloadJob(t, ctx, row.ID); after.State == jobs.StateCancelled && job.State != jobs.StateCancelled {
		t.Fatalf("refusing the row cancel still cancelled the Job")
	}
}

// An earlier release cancelled the Job and left the row pending. When that row
// comes due, it records the cancellation rather than a submission naming a Job
// that never ran.
func TestADueRowWhoseJobWasCancelledBeforeItRanIsNotSubmitted(t *testing.T) {
	ctx, key, actor, _ := newRetiredDeferredDownloadContext(t)
	row, err := ctx.CreateScheduledDownload(downloadTestPlugin, actor.ID,
		&query_models.ResourceFromRemoteCreator{URL: "https://example.invalid/cancelled-earlier.bin"}, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("create the deferred download: %v", err)
	}
	job := deferredDownloadJob(t, ctx, row.ID)
	if _, err := ctx.JobService().Transition(ctx.jobDeps(), jobs.Transition{
		JobID: job.ID, ExpectedVersion: job.Version, To: jobs.StateCancelled,
	}); err != nil {
		t.Fatalf("cancel the Job alone: %v", err)
	}
	if got := scheduledDownloadRow(t, ctx, row.ID); got.Status != models.ScheduledDownloadStatusPending {
		t.Fatalf("setup: the row is %s, want the pending row an earlier release left", got.Status)
	}

	fireDueDeferredDownloads(t, ctx, time.Now().Add(2*time.Hour))
	got := scheduledDownloadRow(t, ctx, row.ID)
	if got.Status != models.ScheduledDownloadStatusCancelled || got.JobID != "" || got.Attempts != 0 {
		t.Fatalf("the due row is %s naming job %q after %d attempts, want cancelled, naming none, never submitted",
			got.Status, got.JobID, got.Attempts)
	}

	ctx = restartJobProcess(t, ctx, key)
	requireCleanBoot(t, ctx)
}

func advertisedCommand(t *testing.T, ctx *MahresourcesContext, jobID, key string) (jobs.Command, bool) {
	t.Helper()
	commands, err := ctx.JobService().AdvertisedCommands(context.Background(), ctx.jobDeps(), ctx.jobAccess(), jobID)
	if err != nil {
		t.Fatalf("advertised commands of %s: %v", jobID, err)
	}
	for _, command := range commands {
		if command.Key == key {
			return command, true
		}
	}
	return jobs.Command{}, false
}

func retryJob(t *testing.T, ctx *MahresourcesContext, job jobs.Snapshot) jobs.Snapshot {
	t.Helper()
	result, err := ctx.ExecuteJobCommand(context.Background(), jobs.CommandRequest{
		JobID: job.ID, Key: jobs.CommandRetry, IdempotencyKey: "retry-" + job.ID, ExpectedVersion: job.Version,
	})
	if err != nil || result.Status != jobs.CommandStatusSucceeded || result.SuccessorID == "" {
		t.Fatalf("retry Job %s = %+v, %v", job.ID, result, err)
	}
	successor, err := ctx.JobService().Get(ctx.jobDeps(), jobs.Access{Administrator: true}, result.SuccessorID)
	if err != nil {
		t.Fatalf("read the successor: %v", err)
	}
	return successor
}

// A Retry of a deferred download cancelled before it started downloads now. The
// control says so, and asks, before it is used: the time the download was
// scheduled for is not kept. The plugin's row stays cancelled; the successor is an
// ordinary download the row does not track. A deferred download that ran offers
// an ordinary Retry.
func TestRetryingACancelledDeferredDownloadDownloadsNow(t *testing.T) {
	ctx, key, actor, _ := newRetiredDeferredDownloadContext(t)
	due := time.Now().Add(time.Hour)
	row, err := ctx.CreateScheduledDownload(downloadTestPlugin, actor.ID,
		&query_models.ResourceFromRemoteCreator{URL: "https://example.invalid/now.bin"}, due)
	if err != nil {
		t.Fatalf("create the deferred download: %v", err)
	}
	cancelJobAsItsOwner(t, ctx, deferredDownloadJob(t, ctx, row.ID))
	cancelled := deferredDownloadJob(t, ctx, row.ID)

	command, offered := advertisedCommand(t, ctx, cancelled.ID, jobs.CommandRetry)
	if !offered || command.Label != "Download now" || !strings.Contains(command.Confirmation, "does not wait for the scheduled time") {
		t.Fatalf("the cancelled deferred download offers retry=%v labelled %q confirming %q, want \"Download now\" saying the time is not kept",
			offered, command.Label, command.Confirmation)
	}
	successor := retryJob(t, ctx, cancelled)
	if successor.State != jobs.StateQueued || successor.ScheduledFor != nil {
		t.Fatalf("the successor is %s for %v, want queued now", successor.State, successor.ScheduledFor)
	}
	fireDueDeferredDownloads(t, ctx, due.Add(time.Minute))
	if got := scheduledDownloadRow(t, ctx, row.ID); got.Status != models.ScheduledDownloadStatusCancelled || got.JobID != "" {
		t.Fatalf("the cancelled deferral's row is %s naming %q after a Retry and its due time, want still cancelled", got.Status, got.JobID)
	}

	ran, err := ctx.CreateScheduledDownload(downloadTestPlugin, actor.ID,
		&query_models.ResourceFromRemoteCreator{URL: "https://example.invalid/ran.bin"}, time.Now().Add(-time.Second))
	if err != nil {
		t.Fatalf("create a deferred download that runs: %v", err)
	}
	runOneDispatchPass(ctx)
	finished := waitForSnapshot(t, ctx, deferredDownloadJob(t, ctx, ran.ID).ID, "the deferred download to run and end",
		func(snap jobs.Snapshot) bool { return snap.StartedAt != nil && snap.State.Terminal() })
	if finished.State != jobs.StateSucceeded {
		// Every address this context can reach is refused by the fetch policy, which
		// is a failure no Retry offers to repeat. What is at issue here is the
		// label of a Retry that is offered, so the failure is recorded as one a
		// Retry could answer differently.
		if err := ctx.db.Model(&models.Job{}).Where("id = ?", finished.ID).Updates(map[string]any{
			"failure_code": download_queue.FailureRemoteConnection, "failure_class": jobs.FailureClassDependency,
		}).Error; err != nil {
			t.Fatalf("record a retryable failure: %v", err)
		}
		if command, offered := advertisedCommand(t, ctx, finished.ID, jobs.CommandRetry); !offered || command.Label != "Retry" || command.Confirmation != "" {
			t.Fatalf("a deferred download that ran offers retry=%v labelled %q confirming %q, want a plain Retry", offered, command.Label, command.Confirmation)
		}
	}

	ctx = restartJobProcess(t, ctx, key)
	requireCleanBoot(t, ctx)
}

// The sweep can reach a due row before the dispatch loop claims its Job: the row
// is then submitted and the Job queued, and nothing has run. Cancelling the Job
// in that interval still ends the row, rather than leaving it submitted naming a
// cancelled Job.
func TestCancellingADeferredJobTheSweepQueuedCancelsItsRow(t *testing.T) {
	ctx, _, actor, _ := newRetiredDeferredDownloadContext(t)
	row, err := ctx.CreateScheduledDownload(downloadTestPlugin, actor.ID,
		&query_models.ResourceFromRemoteCreator{URL: "https://example.invalid/queued.bin"}, time.Now().Add(-time.Second))
	if err != nil {
		t.Fatalf("create the deferred download: %v", err)
	}
	fireDueDeferredDownloads(t, ctx, time.Now())
	job := deferredDownloadJob(t, ctx, row.ID)
	if got := scheduledDownloadRow(t, ctx, row.ID); got.Status != models.ScheduledDownloadStatusSubmitted || got.JobID != job.ID ||
		job.State != jobs.StateQueued || job.StartedAt != nil {
		t.Fatalf("setup: row %s naming %q, Job %s started %v; want submitted naming a queued Job nothing ran",
			got.Status, got.JobID, job.State, job.StartedAt)
	}
	cancelJobAsItsOwner(t, ctx, job)
	if got := scheduledDownloadRow(t, ctx, row.ID); got.Status != models.ScheduledDownloadStatusCancelled {
		t.Fatalf("the row is %s after its unstarted Job was cancelled, want cancelled", got.Status)
	}
}

// The sweep reserves a due row, queues its Job, and only then records the Job's
// id on the row. A cancel of the Job landing between the last two leaves nothing
// to record: the row ends cancelled and the sweep reports no submission.
func TestCancellingADeferredJobWhileTheSweepRecordsItCancelsItsRow(t *testing.T) {
	ctx, _, actor, earlier := newRetiredDeferredDownloadContext(t)
	// Only the row under test is due, so the interleave lands on its sweep.
	if ok, err := ctx.CancelScheduledDownload(earlier.ID); err != nil || !ok {
		t.Fatalf("setup: cancel the fixture's other row = %v, %v", ok, err)
	}
	row, err := ctx.CreateScheduledDownload(downloadTestPlugin, actor.ID,
		&query_models.ResourceFromRemoteCreator{URL: "https://example.invalid/recording.bin"}, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("create the deferred download: %v", err)
	}
	var cancelledJob atomic.Bool
	const name = "test:cancel-before-the-sweep-records-the-job"
	if err := ctx.db.Callback().Update().Before("gorm:update").Register(name, func(tx *gorm.DB) {
		if tx.Statement.Table != "scheduled_downloads" {
			return
		}
		updates, ok := tx.Statement.Dest.(map[string]any)
		if _, recordsJob := updates["job_id"]; !ok || !recordsJob || !cancelledJob.CompareAndSwap(false, true) {
			return
		}
		cancelJobAsItsOwner(t, ctx, deferredDownloadJob(t, ctx, row.ID))
	}); err != nil {
		t.Fatalf("register the interleave: %v", err)
	}
	t.Cleanup(func() { _ = ctx.db.Callback().Update().Remove(name) })

	fired, err := ctx.FireDueScheduledDownloads(ScheduledDownloadFireConfig{
		Now:             time.Now().Add(2 * time.Hour),
		PluginAvailable: func(string) bool { return true },
		Submit: func(*query_models.ResourceFromRemoteCreator, *uint, string) (string, error) {
			t.Fatalf("a deferred row with a durable Job was submitted to the queue")
			return "", nil
		},
	})
	if !cancelledJob.Load() {
		t.Fatalf("setup: the Job was never cancelled inside the sweep")
	}
	if err != nil {
		t.Fatalf("the sweep failed when the Job it queued was cancelled: %v", err)
	}
	if fired != 0 {
		t.Fatalf("the sweep reported %d submissions of a download whose Job was cancelled, want none", fired)
	}
	got := scheduledDownloadRow(t, ctx, row.ID)
	if got.Status != models.ScheduledDownloadStatusCancelled || got.JobID != "" {
		t.Fatalf("the row is %s naming %q after the sweep, want cancelled naming none", got.Status, got.JobID)
	}
}

// A Job cancelled after the sweep reserved its row, and before the sweep asked the
// Job service, ends the row itself; the sweep then finds the Job ended and its
// reservation gone. That is not a failure, and the rows behind it still fire.
func TestCancellingADeferredJobTheSweepJustReservedDoesNotStopTheSweep(t *testing.T) {
	ctx, _, actor, earlier := newRetiredDeferredDownloadContext(t)
	if ok, err := ctx.CancelScheduledDownload(earlier.ID); err != nil || !ok {
		t.Fatalf("setup: cancel the fixture's other row = %v, %v", ok, err)
	}
	first, err := ctx.CreateScheduledDownload(downloadTestPlugin, actor.ID,
		&query_models.ResourceFromRemoteCreator{URL: "https://example.invalid/reserved.bin"}, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("create the first deferred download: %v", err)
	}
	behind, err := ctx.CreateScheduledDownload(downloadTestPlugin, actor.ID,
		&query_models.ResourceFromRemoteCreator{URL: "https://example.invalid/behind.bin"}, time.Now().Add(time.Hour+time.Minute))
	if err != nil {
		t.Fatalf("create the second deferred download: %v", err)
	}
	var cancelledJob atomic.Bool
	const name = "test:cancel-after-the-sweep-reserves"
	if err := ctx.db.Callback().Update().After("gorm:update").Register(name, func(tx *gorm.DB) {
		if tx.Statement.Table != "scheduled_downloads" {
			return
		}
		updates, ok := tx.Statement.Dest.(map[string]any)
		if !ok || updates["status"] != models.ScheduledDownloadStatusSubmitted || !cancelledJob.CompareAndSwap(false, true) {
			return
		}
		cancelJobAsItsOwner(t, ctx, deferredDownloadJob(t, ctx, first.ID))
	}); err != nil {
		t.Fatalf("register the interleave: %v", err)
	}
	t.Cleanup(func() { _ = ctx.db.Callback().Update().Remove(name) })

	fired, err := ctx.FireDueScheduledDownloads(ScheduledDownloadFireConfig{
		Now:             time.Now().Add(2 * time.Hour),
		PluginAvailable: func(string) bool { return true },
		Submit: func(*query_models.ResourceFromRemoteCreator, *uint, string) (string, error) {
			t.Fatalf("a deferred row with a durable Job was submitted to the queue")
			return "", nil
		},
	})
	if !cancelledJob.Load() {
		t.Fatalf("setup: the Job was never cancelled inside the sweep")
	}
	if err != nil {
		t.Fatalf("the sweep failed when the Job it had reserved was cancelled: %v", err)
	}
	if fired != 1 {
		t.Fatalf("the sweep fired %d rows, want only the row behind the cancelled one", fired)
	}
	if got := scheduledDownloadRow(t, ctx, first.ID); got.Status != models.ScheduledDownloadStatusCancelled || got.Attempts != 0 {
		t.Fatalf("the cancelled row is %s after %d attempts, want cancelled and never submitted", got.Status, got.Attempts)
	}
	if got := scheduledDownloadRow(t, ctx, behind.ID); got.Status != models.ScheduledDownloadStatusSubmitted {
		t.Fatalf("the row behind the cancelled one is %s, want submitted", got.Status)
	}
}

// A pending row an earlier release left behind a cancelled Job is recorded
// cancelled when it comes due, before anything about its plugin or owner is
// checked: the deferral ended when its Job did, and a refusal found afterwards
// would record a failure of something that was never going to run.
func TestADueRowWhoseJobWasCancelledIsCancelledEvenWhenItsPluginIsGone(t *testing.T) {
	ctx, _, actor, earlier := newRetiredDeferredDownloadContext(t)
	if ok, err := ctx.CancelScheduledDownload(earlier.ID); err != nil || !ok {
		t.Fatalf("setup: cancel the fixture's other row = %v, %v", ok, err)
	}
	row, err := ctx.CreateScheduledDownload(downloadTestPlugin, actor.ID,
		&query_models.ResourceFromRemoteCreator{URL: "https://example.invalid/plugin-gone.bin"}, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("create the deferred download: %v", err)
	}
	job := deferredDownloadJob(t, ctx, row.ID)
	if _, err := ctx.JobService().Transition(ctx.jobDeps(), jobs.Transition{
		JobID: job.ID, ExpectedVersion: job.Version, To: jobs.StateCancelled,
	}); err != nil {
		t.Fatalf("cancel the Job alone: %v", err)
	}
	if _, err := ctx.FireDueScheduledDownloads(ScheduledDownloadFireConfig{
		Now:             time.Now().Add(2 * time.Hour),
		PluginAvailable: func(string) bool { return false },
		Submit: func(*query_models.ResourceFromRemoteCreator, *uint, string) (string, error) {
			t.Fatalf("a row whose Job was cancelled was submitted")
			return "", nil
		},
	}); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if got := scheduledDownloadRow(t, ctx, row.ID); got.Status != models.ScheduledDownloadStatusCancelled || got.Attempts != 0 {
		t.Fatalf("the row is %s after %d attempts, want cancelled and never submitted", got.Status, got.Attempts)
	}
}

// A Job cancelled while the sweep holds its row (here, just before the sweep
// reserves it) ends the row first. The sweep then has no row left to hand on,
// which is no reason to stop before the rows behind it.
func TestCancellingADeferredJobWhileTheSweepHoldsItsRowDoesNotStopTheSweep(t *testing.T) {
	ctx, _, actor, earlier := newRetiredDeferredDownloadContext(t)
	if ok, err := ctx.CancelScheduledDownload(earlier.ID); err != nil || !ok {
		t.Fatalf("setup: cancel the fixture's other row = %v, %v", ok, err)
	}
	first, err := ctx.CreateScheduledDownload(downloadTestPlugin, actor.ID,
		&query_models.ResourceFromRemoteCreator{URL: "https://example.invalid/held.bin"}, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("create the first deferred download: %v", err)
	}
	behind, err := ctx.CreateScheduledDownload(downloadTestPlugin, actor.ID,
		&query_models.ResourceFromRemoteCreator{URL: "https://example.invalid/behind-held.bin"}, time.Now().Add(time.Hour+time.Minute))
	if err != nil {
		t.Fatalf("create the second deferred download: %v", err)
	}
	firstJob := deferredDownloadJob(t, ctx, first.ID)

	// The cancel lands before the first reserve of the sweep runs: the row is
	// claimed, and nothing has been handed on yet. It is injected before the
	// reserve's transaction begins, so the reserve does not hold the writer lock
	// the cancel needs.
	var injected atomic.Bool
	const name = "test:cancel-before-reserve"
	if err := ctx.db.Callback().Update().Before("gorm:begin_transaction").Register(name, func(tx *gorm.DB) {
		if tx.Statement.Table != "scheduled_downloads" {
			return
		}
		updates, ok := tx.Statement.Dest.(map[string]any)
		if !ok || updates["status"] != models.ScheduledDownloadStatusSubmitted || !injected.CompareAndSwap(false, true) {
			return
		}
		cancelJobAsItsOwner(t, ctx, firstJob)
	}); err != nil {
		t.Fatalf("register the interleave: %v", err)
	}
	t.Cleanup(func() { _ = ctx.db.Callback().Update().Remove(name) })

	fired, err := ctx.FireDueScheduledDownloads(ScheduledDownloadFireConfig{
		Now:             time.Now().Add(2 * time.Hour),
		PluginAvailable: func(string) bool { return true },
		Submit: func(*query_models.ResourceFromRemoteCreator, *uint, string) (string, error) {
			t.Fatalf("a deferred row with a durable Job was submitted to the queue")
			return "", nil
		},
	})
	if err != nil {
		t.Fatalf("the sweep failed when a Job was cancelled while it held the row: %v", err)
	}
	if !injected.Load() {
		t.Fatalf("setup: the cancel was never interleaved")
	}
	if fired != 1 {
		t.Fatalf("the sweep fired %d rows, want only the row behind", fired)
	}
	if got := scheduledDownloadRow(t, ctx, first.ID); got.Status != models.ScheduledDownloadStatusCancelled {
		t.Fatalf("the first row is %s, want cancelled", got.Status)
	}
	if got := scheduledDownloadRow(t, ctx, behind.ID); got.Status != models.ScheduledDownloadStatusSubmitted {
		t.Fatalf("the row behind is %s, want submitted", got.Status)
	}
}

// Earlier releases left two kinds of row disagreeing with a Job cancelled before
// anything ran it: one still pending, and one the sweep had marked submitted
// naming that Job. Startup reconciles both, and leaves alone a row whose Job is
// still waiting to run.
func TestStartupReconcilesDeferredRowsWhoseJobWasCancelledBeforeItRan(t *testing.T) {
	ctx, _, actor, earlier := newRetiredDeferredDownloadContext(t)
	if ok, err := ctx.CancelScheduledDownload(earlier.ID); err != nil || !ok {
		t.Fatalf("setup: cancel the fixture's other row = %v, %v", ok, err)
	}
	create := func(name string) (models.ScheduledDownload, jobs.Snapshot) {
		t.Helper()
		row, err := ctx.CreateScheduledDownload(downloadTestPlugin, actor.ID,
			&query_models.ResourceFromRemoteCreator{URL: "https://example.invalid/" + name + ".bin"}, time.Now().Add(time.Hour))
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		return *row, deferredDownloadJob(t, ctx, row.ID)
	}
	cancelAlone := func(job jobs.Snapshot) {
		t.Helper()
		if _, err := ctx.JobService().Transition(ctx.jobDeps(), jobs.Transition{
			JobID: job.ID, ExpectedVersion: job.Version, To: jobs.StateCancelled,
		}); err != nil {
			t.Fatalf("cancel the Job alone: %v", err)
		}
	}
	pendingRow, pendingJob := create("left-pending")
	cancelAlone(pendingJob)
	submittedRow, submittedJob := create("left-submitted")
	cancelAlone(submittedJob)
	if err := ctx.db.Model(&models.ScheduledDownload{}).Where("id = ?", submittedRow.ID).
		Updates(map[string]any{"status": models.ScheduledDownloadStatusSubmitted, "job_id": submittedJob.ID, "attempts": 1}).Error; err != nil {
		t.Fatalf("mark the row submitted as an earlier release did: %v", err)
	}
	waitingRow, waitingJob := create("still-waiting")
	if err := ctx.db.Model(&models.ScheduledDownload{}).Where("id = ?", waitingRow.ID).
		Updates(map[string]any{"status": models.ScheduledDownloadStatusSubmitted, "job_id": waitingJob.ID, "attempts": 1}).Error; err != nil {
		t.Fatalf("mark the waiting row submitted: %v", err)
	}

	reconciled, err := ctx.ReconcileDeferredDownloadRows()
	if err != nil || reconciled != 2 {
		t.Fatalf("reconcile = %d, %v; want the 2 rows whose Job was cancelled", reconciled, err)
	}
	for _, id := range []uint{pendingRow.ID, submittedRow.ID} {
		if got := scheduledDownloadRow(t, ctx, id); got.Status != models.ScheduledDownloadStatusCancelled || got.Attempts != 0 {
			t.Fatalf("row %d is %s after %d attempts, want cancelled and never submitted", id, got.Status, got.Attempts)
		}
	}
	if got := scheduledDownloadRow(t, ctx, waitingRow.ID); got.Status != models.ScheduledDownloadStatusSubmitted {
		t.Fatalf("the row whose Job is still waiting is %s, want submitted", got.Status)
	}
}

// A pending row can outlive its Job. An earlier release could cancel the Job and
// leave the row pending; and the dispatch loop runs a Job at its time whether or
// not the row sweep has recorded it, so the Job can also start and end first.
// Either Job can then be deleted by retention, which keeps nothing of how it
// ended. The row is closed without submitting its stored payload again, and says
// that the outcome is not known rather than that the Job was cancelled before it
// ran. Startup, the due-time sweep and a cancel of the row all reach it, with the
// sources retired or not; the cancel answers that the row had already ended.
func TestAPendingRowWhoseJobRetentionRemovedIsNeverSubmitted(t *testing.T) {
	endings := map[string]func(*testing.T, *MahresourcesContext, jobs.Snapshot){
		"cancelled while it waited": func(t *testing.T, ctx *MahresourcesContext, job jobs.Snapshot) {
			if _, err := ctx.JobService().Transition(ctx.jobDeps(), jobs.Transition{
				JobID: job.ID, ExpectedVersion: job.Version, To: jobs.StateCancelled,
			}); err != nil {
				t.Fatalf("cancel the Job alone: %v", err)
			}
		},
		"started and failed": func(t *testing.T, ctx *MahresourcesContext, job jobs.Snapshot) {
			execution, claimed, err := ctx.JobService().Claim(context.Background(), ctx.jobDeps(), jobs.ClaimRequest{
				Kind: JobKindDeferredDownload, KindVersion: 1, Claimant: "deferred-outlived-test",
			})
			if err != nil || !claimed || execution.JobID != job.ID {
				t.Fatalf("start the Job = %s, %v, %v; want %s", execution.JobID, claimed, err, job.ID)
			}
			if _, err := ctx.JobService().Finish(ctx.jobDeps(), jobs.FinishRequest{
				ExecutionRef:    jobs.ExecutionRef{JobID: job.ID, ExecutionToken: execution.ExecutionToken},
				ExpectedVersion: execution.Version, Outcome: jobs.StateFailed,
				Failure: &jobs.Failure{Code: "unreachable", Class: jobs.FailureClassDependency, Message: "the origin did not answer"},
			}); err != nil {
				t.Fatalf("fail the started Job: %v", err)
			}
		},
	}
	settle := map[string]func(*testing.T, *MahresourcesContext, uint){
		"at startup": func(t *testing.T, ctx *MahresourcesContext, _ uint) {
			if reconciled, err := ctx.ReconcileDeferredDownloadRows(); err != nil || reconciled != 1 {
				t.Fatalf("reconcile = %d, %v; want the row whose Job is gone", reconciled, err)
			}
		},
		"at its due time": func(t *testing.T, ctx *MahresourcesContext, _ uint) {
			fireDueDeferredDownloads(t, ctx, time.Now().Add(2*time.Hour))
		},
		"by a row cancel": func(t *testing.T, ctx *MahresourcesContext, rowID uint) {
			if cancelled, err := ctx.CancelScheduledDownload(rowID); cancelled || !errors.Is(err, ErrScheduledDownloadEnded) {
				t.Fatalf("cancel = %v, %v; want the answer that the row already ended", cancelled, err)
			}
		},
	}
	for ending, end := range endings {
		for _, retired := range []bool{false, true} {
			for when, settle := range settle {
				t.Run(fmt.Sprintf("%s, %s, sources retired %v", ending, when, retired), func(t *testing.T) {
					ctx := newJobHarnessContext(t, false)
					key := sharedReplayKey(t)
					holdJobReplayKey(t, ctx, key)
					enableDownloadTestPlugin(t, ctx)
					actor, err := ctx.CreateUser(&UserInput{Username: "deferred-owner", Password: "password1", Role: models.RoleUser})
					if err != nil {
						t.Fatalf("create the acting user: %v", err)
					}
					// Due already, so the dispatch loop could claim its Job.
					row, err := ctx.CreateScheduledDownload(downloadTestPlugin, actor.ID,
						&query_models.ResourceFromRemoteCreator{URL: "https://example.invalid/outlived.bin?token=private"}, time.Now().Add(-time.Second))
					if err != nil {
						t.Fatalf("create the deferred download: %v", err)
					}
					if retired {
						if result, err := ctx.RunJobMigrationToGate(JobMigrationOptions{BatchSize: 50, MaxBatches: 20, WritersDrained: true}); err != nil || !result.Complete {
							t.Fatalf("retire the job sources = %+v, %v", result, err)
						}
					}
					job := deferredDownloadJob(t, ctx, row.ID)
					end(t, ctx, job)
					if got := scheduledDownloadRow(t, ctx, row.ID); got.Status != models.ScheduledDownloadStatusPending {
						t.Fatalf("setup: the row is %s, want still pending behind its ended Job", got.Status)
					}
					if err := ctx.db.Model(&models.Job{}).Where("id = ?", job.ID).
						Update("expires_at", time.Now().UTC().Add(-time.Hour)).Error; err != nil {
						t.Fatalf("expire the ended Job: %v", err)
					}
					if sweep, err := ctx.JobService().Sweep(ctx.jobDeps(), jobs.RetentionPolicy{}, jobs.SweepCursor{}, 100); err != nil || sweep.Pruned != 1 {
						t.Fatalf("retention = %+v, %v; want the ended Job pruned", sweep, err)
					}
					if retired {
						ctx = restartJobProcess(t, ctx, key)
						requireCleanBoot(t, ctx)
					}

					settle(t, ctx, row.ID)
					got := scheduledDownloadRow(t, ctx, row.ID)
					if got.Status != models.ScheduledDownloadStatusFailed || got.JobID != "" || got.Attempts != 0 ||
						!strings.Contains(got.LastError, "not known") || strings.Contains(got.LastError, "before it started") {
						t.Fatalf("the row is %s naming job %q after %d attempts (%q), want closed with an unknown outcome and never submitted",
							got.Status, got.JobID, got.Attempts, got.LastError)
					}
					if retired {
						ctx = restartJobProcess(t, ctx, key)
						requireCleanBoot(t, ctx)
					}
				})
			}
		}
	}
}

// legacyPendingRowRetried builds a state earlier releases left: the deferred Job
// cancelled while its row stayed pending, then retried. The Retry's successor is
// an ordinary queued download, and it has taken over the row's legacy handle.
func legacyPendingRowRetried(t *testing.T, ctx *MahresourcesContext, actorID uint, name string) (models.ScheduledDownload, jobs.Snapshot, jobs.Snapshot) {
	t.Helper()
	row, err := ctx.CreateScheduledDownload(downloadTestPlugin, actorID,
		&query_models.ResourceFromRemoteCreator{URL: "https://example.invalid/" + name + ".bin"}, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("create the deferred download: %v", err)
	}
	job := deferredDownloadJob(t, ctx, row.ID)
	cancelled, err := ctx.JobService().Transition(ctx.jobDeps(), jobs.Transition{JobID: job.ID, ExpectedVersion: job.Version, To: jobs.StateCancelled})
	if err != nil {
		t.Fatalf("cancel the Job alone: %v", err)
	}
	successor := retryJob(t, ctx, cancelled)
	if got := scheduledDownloadRow(t, ctx, row.ID); got.Status != models.ScheduledDownloadStatusPending {
		t.Fatalf("setup: the row is %s, want the pending row an earlier release left", got.Status)
	}
	return *row, cancelled, successor
}

// The row's own Job is the one it was accepted with, not whatever its handle was
// moved to: cancelling such a row cancels the row, and leaves the Retry's
// download alone.
func TestCancellingAnEarlierReleasesPendingRowLeavesItsRetryAlone(t *testing.T) {
	ctx, _, actor, _ := newRetiredDeferredDownloadContext(t)
	row, _, successor := legacyPendingRowRetried(t, ctx, actor.ID, "legacy-retried-cancel")
	cancelled, err := ctx.CancelScheduledDownload(row.ID)
	if err != nil || !cancelled {
		t.Fatalf("cancel the row = %v, %v; want cancelled", cancelled, err)
	}
	after, err := ctx.JobService().Get(ctx.jobDeps(), jobs.Access{Administrator: true}, successor.ID)
	if err != nil {
		t.Fatalf("read the successor: %v", err)
	}
	if after.State == jobs.StateCancelled {
		t.Fatalf("cancelling the row cancelled the Retry's download")
	}
}

// Startup reconciles such a row against the Job it was accepted with, and a
// later sweep at its due time leaves it cancelled rather than recording the
// Retry's download as its submission.
func TestStartupReconcilesAnEarlierReleasesPendingRowAcrossARetry(t *testing.T) {
	ctx, key, actor, _ := newRetiredDeferredDownloadContext(t)
	row, _, _ := legacyPendingRowRetried(t, ctx, actor.ID, "legacy-retried-reconcile")
	if reconciled, err := ctx.ReconcileDeferredDownloadRows(); err != nil || reconciled < 1 {
		t.Fatalf("reconcile = %d, %v; want the row recorded", reconciled, err)
	}
	fireDueDeferredDownloads(t, ctx, time.Now().Add(2*time.Hour))
	if got := scheduledDownloadRow(t, ctx, row.ID); got.Status != models.ScheduledDownloadStatusCancelled || got.JobID != "" {
		t.Fatalf("the row is %s naming %q, want cancelled naming none", got.Status, got.JobID)
	}
	ctx = restartJobProcess(t, ctx, key)
	requireCleanBoot(t, ctx)
}

// An earlier release's sweep resolved a row's Job through its legacy handle, so a
// row left pending behind a cancelled Job and then retried was marked submitted
// naming the Retry's successor, an ordinary download the row does not track.
// Startup records the end of the row's own Job, whatever the successor does.
func TestStartupReconcilesAnEarlierReleasesSubmittedRowNamingARetry(t *testing.T) {
	for _, retired := range []bool{false, true} {
		t.Run(fmt.Sprintf("sources retired %v", retired), func(t *testing.T) {
			ctx := newJobHarnessContext(t, false)
			key := sharedReplayKey(t)
			holdJobReplayKey(t, ctx, key)
			enableDownloadTestPlugin(t, ctx)
			actor, err := ctx.CreateUser(&UserInput{Username: "deferred-owner", Password: "password1", Role: models.RoleUser})
			if err != nil {
				t.Fatalf("create the acting user: %v", err)
			}
			if retired {
				if result, err := ctx.RunJobMigrationToGate(JobMigrationOptions{BatchSize: 50, MaxBatches: 20, WritersDrained: true}); err != nil || !result.Complete {
					t.Fatalf("retire the job sources = %+v, %v", result, err)
				}
			}
			row, _, successor := legacyPendingRowRetried(t, ctx, actor.ID, "legacy-retried-submitted")
			if err := ctx.db.Model(&models.ScheduledDownload{}).Where("id = ?", row.ID).
				Updates(map[string]any{"status": models.ScheduledDownloadStatusSubmitted, "job_id": successor.ID, "attempts": 1}).Error; err != nil {
				t.Fatalf("fire the row as that release's sweep did: %v", err)
			}

			if reconciled, err := ctx.ReconcileDeferredDownloadRows(); err != nil || reconciled != 1 {
				t.Fatalf("reconcile = %d, %v; want the row recorded", reconciled, err)
			}
			if got := scheduledDownloadRow(t, ctx, row.ID); got.Status != models.ScheduledDownloadStatusCancelled || got.Attempts != 0 {
				t.Fatalf("the row is %s after %d attempts, want cancelled as its own Job was, never submitted", got.Status, got.Attempts)
			}
			if job, err := ctx.JobService().Get(ctx.jobDeps(), jobs.Access{Administrator: true}, successor.ID); err != nil || job.State != successor.State {
				t.Fatalf("the Retry's successor is %s (%v), want it left %s", job.State, err, successor.State)
			}
			if retired {
				ctx = restartJobProcess(t, ctx, key)
				requireCleanBoot(t, ctx)
			}
		})
	}
}

// The same earlier-release row after retention deleted both its own cancelled Job
// and the Retry's: nothing says any more how its own deferral ended, and the row
// names a download it does not track, so it is closed with its outcome unknown. A
// submitted row naming its own deleted Job already records that Job and is kept.
func TestStartupClosesASubmittedRowNamingARetryOnceItsOwnJobIsGone(t *testing.T) {
	ctx, _, actor, _ := newRetiredDeferredDownloadContext(t)
	row, own, successor := legacyPendingRowRetried(t, ctx, actor.ID, "legacy-retried-gone")
	if err := ctx.db.Model(&models.ScheduledDownload{}).Where("id = ?", row.ID).
		Updates(map[string]any{"status": models.ScheduledDownloadStatusSubmitted, "job_id": successor.ID, "attempts": 1}).Error; err != nil {
		t.Fatalf("fire the row as that release's sweep did: %v", err)
	}
	kept, err := ctx.CreateScheduledDownload(downloadTestPlugin, actor.ID,
		&query_models.ResourceFromRemoteCreator{URL: "https://example.invalid/kept.bin"}, time.Now().Add(-time.Second))
	if err != nil {
		t.Fatalf("create the row that fires normally: %v", err)
	}
	fireDueDeferredDownloads(t, ctx, time.Now())
	keptJob := deferredDownloadJob(t, ctx, kept.ID)
	for _, job := range []jobs.Snapshot{successor, keptJob} {
		if _, err := ctx.JobService().Transition(ctx.jobDeps(), jobs.Transition{
			JobID: job.ID, ExpectedVersion: job.Version, To: jobs.StateCancelled,
		}); err != nil {
			t.Fatalf("end Job %s: %v", job.ID, err)
		}
	}
	gone := []string{own.ID, successor.ID, keptJob.ID}
	if err := ctx.db.Model(&models.Job{}).Where("id IN ?", gone).
		Update("expires_at", time.Now().UTC().Add(-time.Hour)).Error; err != nil {
		t.Fatal(err)
	}
	if sweep, err := ctx.JobService().Sweep(ctx.jobDeps(), jobs.RetentionPolicy{}, jobs.SweepCursor{}, 100); err != nil || sweep.Pruned != len(gone) {
		t.Fatalf("retention = %+v, %v; want the %d ended Jobs deleted", sweep, err, len(gone))
	}
	if got := scheduledDownloadRow(t, ctx, kept.ID); got.Status != models.ScheduledDownloadStatusSubmitted {
		t.Fatalf("setup: the normally fired row is %s, want submitted", got.Status)
	}

	if reconciled, err := ctx.ReconcileDeferredDownloadRows(); err != nil || reconciled != 1 {
		t.Fatalf("reconcile = %d, %v; want only the row naming the Retry", reconciled, err)
	}
	if got := scheduledDownloadRow(t, ctx, row.ID); got.Status != models.ScheduledDownloadStatusFailed || !strings.Contains(got.LastError, "not known") {
		t.Fatalf("the row naming the Retry is %s (%q), want failed with its outcome unknown", got.Status, got.LastError)
	}
	if got := scheduledDownloadRow(t, ctx, kept.ID); got.Status != models.ScheduledDownloadStatusSubmitted {
		t.Fatalf("the row naming its own deleted Job is %s, want it kept submitted", got.Status)
	}
}

// A row an earlier release left pending behind a cancelled Job, then retried,
// changes before the sources are retired, so the migration re-proves it. The
// mapping keeps naming the Job the row was accepted with, not the Retry the row's
// legacy handle has moved to, and a cancel of the row leaves that Retry alone.
func TestAMigrationRefreshKeepsTheRowsOwnJobAcrossARetry(t *testing.T) {
	ctx := newJobHarnessContext(t, false)
	holdJobReplayKey(t, ctx, sharedReplayKey(t))
	enableDownloadTestPlugin(t, ctx)
	actor, err := ctx.CreateUser(&UserInput{Username: "deferred-owner", Password: "password1", Role: models.RoleUser})
	if err != nil {
		t.Fatalf("create the acting user: %v", err)
	}
	row, own, successor := legacyPendingRowRetried(t, ctx, actor.ID, "legacy-retried-refreshed")
	if err := ctx.db.Model(&models.ScheduledDownload{}).Where("id = ?", row.ID).
		Update("due_at", row.DueAt.Add(time.Hour)).Error; err != nil {
		t.Fatalf("change the pending row: %v", err)
	}
	if _, err := ctx.RunJobMigrationToGate(JobMigrationOptions{BatchSize: 10, MaxBatches: 20}); err != nil {
		t.Fatalf("copy and verify the sources: %v", err)
	}
	if mapping := scheduledDownloadMapping(t, ctx, row.ID); mapping.JobID != own.ID {
		t.Fatalf("the refreshed mapping names %s, want the row's own Job %s, not the Retry %s", mapping.JobID, own.ID, successor.ID)
	}

	if cancelled, err := ctx.CancelScheduledDownload(row.ID); err != nil || !cancelled {
		t.Fatalf("cancel the row = %v, %v", cancelled, err)
	}
	if job, err := ctx.JobService().Get(ctx.jobDeps(), jobs.Access{Administrator: true}, successor.ID); err != nil || job.State != successor.State {
		t.Fatalf("the Retry is %s (%v), want it left %s", job.State, err, successor.State)
	}
}

// A row an earlier release left pending behind a cancelled Job, then retried,
// comes due without startup having reconciled it: the sweep records the end of
// its own Job, not the Retry's download.
func TestADueEarlierReleasePendingRowIgnoresItsRetry(t *testing.T) {
	ctx, _, actor, _ := newRetiredDeferredDownloadContext(t)
	row, _, _ := legacyPendingRowRetried(t, ctx, actor.ID, "legacy-retried-due")
	fireDueDeferredDownloads(t, ctx, time.Now().Add(2*time.Hour))
	if got := scheduledDownloadRow(t, ctx, row.ID); got.Status != models.ScheduledDownloadStatusCancelled || got.JobID != "" {
		t.Fatalf("the row is %s naming %q, want cancelled naming none", got.Status, got.JobID)
	}
}
