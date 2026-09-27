//go:build postgres && json1 && fts5

package application_context

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"

	"mahresources/jobs"
	"mahresources/models"
	"mahresources/models/query_models"
)

// TestCancellingADeferredRowAndItsJobTogetherDoesNotDeadlock runs the two cancels
// of one deferral at once: the plugin row's, and the Job's own. The Job's cancel
// locks the Job and then, through the adapter's host-transition hook, the row, so
// the row's cancel has to take them in the same order. The Job's cancel is landed
// inside the row cancel's transaction, right after the row is updated, which is
// where the opposite order would hold the row and wait for the Job.
func TestCancellingADeferredRowAndItsJobTogetherDoesNotDeadlock(t *testing.T) {
	ctx, _, _ := newPostgresOwnershipFixture(t, 2)
	if err := models.EnsureJobWriterEpoch(ctx.db); err != nil {
		t.Fatalf("seed writer epoch: %v", err)
	}
	actor, err := ctx.CreateUser(&UserInput{Username: "pg-deferred-cancel", Password: "password1", Role: models.RoleUser})
	if err != nil {
		t.Fatalf("create the acting user: %v", err)
	}
	row, err := ctx.CreateScheduledDownload("planner-plugin", actor.ID,
		&query_models.ResourceFromRemoteCreator{URL: "https://example.invalid/both.bin"}, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("create the deferred download: %v", err)
	}
	job := deferredDownloadJob(t, ctx, row.ID)

	var once sync.Once
	var jobCancel sync.WaitGroup
	var jobCancelErr error
	const name = "test:cancel-the-job-inside-the-row-cancel"
	if err := ctx.db.Callback().Update().After("gorm:update").Register(name, func(tx *gorm.DB) {
		if tx.Statement.Table != "scheduled_downloads" {
			return
		}
		if _, inTransaction := tx.Statement.ConnPool.(*sql.Tx); !inTransaction {
			return
		}
		once.Do(func() {
			done := make(chan struct{})
			jobCancel.Add(1)
			go func() {
				defer jobCancel.Done()
				defer close(done)
				_, jobCancelErr = ctx.ExecuteJobCommand(context.Background(), jobs.CommandRequest{
					JobID: job.ID, Key: jobs.CommandCancel, IdempotencyKey: "pg-cancel-" + job.ID, ExpectedVersion: job.Version,
				})
			}()
			select {
			case <-done:
			case <-time.After(1500 * time.Millisecond):
			}
		})
	}); err != nil {
		t.Fatalf("register the interleave: %v", err)
	}
	t.Cleanup(func() { _ = ctx.db.Callback().Update().Remove(name) })

	cancelled, rowErr := ctx.CancelScheduledDownload(row.ID)
	jobCancel.Wait()
	if isDeadlockError(rowErr) || isDeadlockError(jobCancelErr) {
		t.Fatalf("the two cancels deadlocked: row cancel %v, Job cancel %v", rowErr, jobCancelErr)
	}
	if rowErr != nil || !cancelled {
		t.Fatalf("the row cancel = %v, %v; want it to win the race it started", cancelled, rowErr)
	}
	if got := deferredDownloadJob(t, ctx, row.ID); got.State != jobs.StateCancelled {
		t.Fatalf("the Job is %s, want cancelled", got.State)
	}
	if got := scheduledDownloadRow(t, ctx, row.ID); got.Status != models.ScheduledDownloadStatusCancelled {
		t.Fatalf("the row is %s, want cancelled", got.Status)
	}
}

// TestCancellingADeferredRowLeavesARetryAlone lands a Retry inside the row's
// cancel, after it read which Job the row was accepted with and before it locks
// that Job: the Retry moves the row's handle to an ordinary download that starts
// now. The row's cancel acts on the Job it read and locked, whose end is already
// final, so it cancels the row and leaves the Retry's download alone. The
// starting state is one earlier releases left: the Job cancelled, the row still
// pending.
func TestCancellingADeferredRowLeavesARetryAlone(t *testing.T) {
	ctx, _, _ := newPostgresOwnershipFixture(t, 2)
	if err := models.EnsureJobWriterEpoch(ctx.db); err != nil {
		t.Fatalf("seed writer epoch: %v", err)
	}
	actor, err := ctx.CreateUser(&UserInput{Username: "pg-deferred-retry-race", Password: "password1", Role: models.RoleUser})
	if err != nil {
		t.Fatalf("create the acting user: %v", err)
	}
	row, err := ctx.CreateScheduledDownload("planner-plugin", actor.ID,
		&query_models.ResourceFromRemoteCreator{URL: "https://example.invalid/retried.bin"}, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("create the deferred download: %v", err)
	}
	ancestor := deferredDownloadJob(t, ctx, row.ID)
	if _, err := ctx.JobService().Transition(ctx.jobDeps(), jobs.Transition{
		JobID: ancestor.ID, ExpectedVersion: ancestor.Version, To: jobs.StateCancelled,
	}); err != nil {
		t.Fatalf("cancel the Job alone: %v", err)
	}
	ancestor = deferredDownloadJob(t, ctx, row.ID)

	var retried atomic.Bool
	var successorID string
	var retryErr error
	const name = "test:retry-after-the-job-read"
	if err := ctx.db.Callback().Query().After("gorm:query").Register(name, func(tx *gorm.DB) {
		if tx.Statement.Table != "job_source_mappings" {
			return
		}
		if _, inTransaction := tx.Statement.ConnPool.(*sql.Tx); !inTransaction || !retried.CompareAndSwap(false, true) {
			return
		}
		result, err := ctx.ExecuteJobCommand(context.Background(), jobs.CommandRequest{
			JobID: ancestor.ID, Key: jobs.CommandRetry, IdempotencyKey: "pg-retry-" + ancestor.ID, ExpectedVersion: ancestor.Version,
		})
		successorID, retryErr = result.SuccessorID, err
	}); err != nil {
		t.Fatalf("register the retry: %v", err)
	}
	t.Cleanup(func() { _ = ctx.db.Callback().Query().Remove(name) })

	ok, err := ctx.CancelScheduledDownload(row.ID)
	if retryErr != nil || successorID == "" {
		t.Fatalf("setup: the concurrent Retry = %q, %v", successorID, retryErr)
	}
	if err != nil || !ok {
		t.Fatalf("the row cancel = %v, %v; want the row cancelled", ok, err)
	}
	successor, err := ctx.JobService().Get(ctx.jobDeps(), jobs.Access{Administrator: true}, successorID)
	if err != nil {
		t.Fatalf("read the successor: %v", err)
	}
	if successor.State == jobs.StateCancelled {
		t.Fatalf("the row's cancel cancelled the Retry's download")
	}
	if got := scheduledDownloadRow(t, ctx, row.ID); got.Status != models.ScheduledDownloadStatusCancelled {
		t.Fatalf("the row is %s, want cancelled", got.Status)
	}
}

// The startup reconciliation's joins and casts, on PostgreSQL.
func TestStartupReconcilesDeferredRowsWhoseJobWasCancelledBeforeItRanPG(t *testing.T) {
	ctx, _, _ := newPostgresOwnershipFixture(t, 2)
	if err := models.EnsureJobWriterEpoch(ctx.db); err != nil {
		t.Fatalf("seed writer epoch: %v", err)
	}
	actor, err := ctx.CreateUser(&UserInput{Username: "pg-deferred-reconcile", Password: "password1", Role: models.RoleUser})
	if err != nil {
		t.Fatalf("create the acting user: %v", err)
	}
	ids := make([]uint, 0, 2)
	for i, status := range []string{models.ScheduledDownloadStatusPending, models.ScheduledDownloadStatusSubmitted} {
		row, err := ctx.CreateScheduledDownload("planner-plugin", actor.ID,
			&query_models.ResourceFromRemoteCreator{URL: fmt.Sprintf("https://example.invalid/reconcile-%d.bin", i)}, time.Now().Add(time.Hour))
		if err != nil {
			t.Fatalf("create the deferred download: %v", err)
		}
		job := deferredDownloadJob(t, ctx, row.ID)
		if _, err := ctx.JobService().Transition(ctx.jobDeps(), jobs.Transition{
			JobID: job.ID, ExpectedVersion: job.Version, To: jobs.StateCancelled,
		}); err != nil {
			t.Fatalf("cancel the Job alone: %v", err)
		}
		if status == models.ScheduledDownloadStatusSubmitted {
			if err := ctx.db.Model(&models.ScheduledDownload{}).Where("id = ?", row.ID).
				Updates(map[string]any{"status": status, "job_id": job.ID, "attempts": 1}).Error; err != nil {
				t.Fatalf("mark the row submitted: %v", err)
			}
		}
		ids = append(ids, row.ID)
	}
	reconciled, err := ctx.ReconcileDeferredDownloadRows()
	if err != nil || reconciled != 2 {
		t.Fatalf("reconcile = %d, %v; want 2", reconciled, err)
	}
	for _, id := range ids {
		if got := scheduledDownloadRow(t, ctx, id); got.Status != models.ScheduledDownloadStatusCancelled || got.Attempts != 0 {
			t.Fatalf("row %d is %s after %d attempts, want cancelled", id, got.Status, got.Attempts)
		}
	}
}
