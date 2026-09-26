//go:build postgres && json1 && fts5

package application_context

import (
	"context"
	"database/sql"
	"sync"
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
