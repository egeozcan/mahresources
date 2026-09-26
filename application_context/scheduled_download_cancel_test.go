package application_context

import (
	"context"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"

	"mahresources/auth"
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

// A deferred download cancelled while it waited, and retried while its time is
// still ahead, is scheduled again for that time; the control says so, and the
// plugin's row is pending again until the time comes.
func TestRetryingACancelledDeferredDownloadSchedulesItAgain(t *testing.T) {
	ctx, key, actor, _ := newRetiredDeferredDownloadContext(t)
	due := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	row, err := ctx.CreateScheduledDownload(downloadTestPlugin, actor.ID,
		&query_models.ResourceFromRemoteCreator{URL: "https://example.invalid/again.bin"}, due)
	if err != nil {
		t.Fatalf("create the deferred download: %v", err)
	}
	cancelJobAsItsOwner(t, ctx, deferredDownloadJob(t, ctx, row.ID))
	cancelled := deferredDownloadJob(t, ctx, row.ID)

	command, offered := advertisedCommand(t, ctx, cancelled.ID, jobs.CommandRetry)
	if !offered || command.Label != "Schedule again" {
		t.Fatalf("the cancelled deferred download offers retry=%v labelled %q, want \"Schedule again\"", offered, command.Label)
	}
	// Retried by an administrator, who owns the successor: the row fires as the
	// actor the successor runs as, so it is the row's owner from now on.
	admin, err := ctx.CreateUser(&UserInput{Username: "deferred-admin", Password: "password1", Role: models.RoleAdmin})
	if err != nil {
		t.Fatalf("create the administrator: %v", err)
	}
	asAdmin := ctx.WithPrincipal(&auth.Principal{UserID: admin.ID, Username: admin.Username, Role: models.RoleAdmin})
	successor := retryJob(t, asAdmin, cancelled)
	if successor.State != jobs.StateScheduled || successor.ScheduledFor == nil || !successor.ScheduledFor.Equal(due) {
		t.Fatalf("the successor is %s for %v, want scheduled for %v", successor.State, successor.ScheduledFor, due)
	}
	reopened := scheduledDownloadRow(t, ctx, row.ID)
	if reopened.Status != models.ScheduledDownloadStatusPending || reopened.Attempts != 0 || reopened.JobID != "" {
		t.Fatalf("the row is %s after %d attempts naming %q, want pending again", reopened.Status, reopened.Attempts, reopened.JobID)
	}
	if reopened.CreatedByUserId == nil || *reopened.CreatedByUserId != admin.ID || successor.ActorUserID == nil || *successor.ActorUserID != admin.ID {
		t.Fatalf("the reopened row is owned by %v and the successor acts as %v, want both the administrator %d",
			reopened.CreatedByUserId, successor.ActorUserID, admin.ID)
	}

	fireDueDeferredDownloads(t, ctx, time.Now())
	if got := scheduledDownloadRow(t, ctx, row.ID); got.Status != models.ScheduledDownloadStatusPending {
		t.Fatalf("the rescheduled row is %s before its time, want pending", got.Status)
	}
	fireDueDeferredDownloads(t, ctx, due.Add(time.Minute))
	if got := scheduledDownloadRow(t, ctx, row.ID); got.Status != models.ScheduledDownloadStatusSubmitted || got.JobID != successor.ID {
		t.Fatalf("the rescheduled row is %s naming %q at its time, want submitted naming the successor %s", got.Status, got.JobID, successor.ID)
	}

	ctx = restartJobProcess(t, ctx, key)
	requireCleanBoot(t, ctx)
}

// Once the time has passed, the Retry of a download that never ran starts it
// now, and the control says that instead; the cancelled deferral stays cancelled.
func TestRetryingADeferredDownloadWhoseTimePassedStartsItNow(t *testing.T) {
	ctx, _, actor, _ := newRetiredDeferredDownloadContext(t)
	row, err := ctx.CreateScheduledDownload(downloadTestPlugin, actor.ID,
		&query_models.ResourceFromRemoteCreator{URL: "https://example.invalid/now.bin"}, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("create the deferred download: %v", err)
	}
	cancelJobAsItsOwner(t, ctx, deferredDownloadJob(t, ctx, row.ID))
	cancelled := deferredDownloadJob(t, ctx, row.ID)
	if err := ctx.db.Model(&models.Job{}).Where("id = ?", cancelled.ID).
		Update("scheduled_for", time.Now().Add(-time.Minute).UTC()).Error; err != nil {
		t.Fatalf("let the due time pass: %v", err)
	}

	command, offered := advertisedCommand(t, ctx, cancelled.ID, jobs.CommandRetry)
	if !offered || command.Label != "Download now" {
		t.Fatalf("the overdue cancelled download offers retry=%v labelled %q, want \"Download now\"", offered, command.Label)
	}
	successor := retryJob(t, ctx, deferredDownloadJob(t, ctx, row.ID))
	if successor.State != jobs.StateQueued || successor.ScheduledFor != nil {
		t.Fatalf("the successor is %s for %v, want queued now", successor.State, successor.ScheduledFor)
	}
	if got := scheduledDownloadRow(t, ctx, row.ID); got.Status != models.ScheduledDownloadStatusCancelled {
		t.Fatalf("the cancelled deferral's row is %s after an immediate retry, want still cancelled", got.Status)
	}
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
