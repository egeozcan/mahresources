package application_context

import (
	"database/sql"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"

	"mahresources/models/query_models"
)

// TestCreatingADeferredDownloadSurvivesACommitFromAnotherConnection lands a
// commit from another connection inside the create transaction, right after its
// read of the writer epoch, which is where download workers' commits land under
// load. On SQLite in WAL mode a transaction that read first and writes second
// cannot promote its stale snapshot once anyone else has committed: it fails at
// once with "database is locked", and busy_timeout never applies.
//
// The interleave is injected rather than raced, and on every attempt, so a
// transaction that reads before it writes fails however often it is retried. The
// competing write waits a bounded time, because once the transaction writes
// first it holds the writer lock and the competing commit can only land after it.
func TestCreatingADeferredDownloadSurvivesACommitFromAnotherConnection(t *testing.T) {
	ctx := newScheduledDownloadTestContext(t)
	owner := createDownloadOwner(t, ctx)
	sqlDB, err := ctx.db.DB()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sqlDB.Exec(`CREATE TABLE interleaved_commits (n INTEGER)`); err != nil {
		t.Fatalf("create the competing table: %v", err)
	}

	var landed atomic.Bool
	var injected atomic.Int32
	var pending sync.WaitGroup
	const name = "test:commit-after-epoch-read"
	if err := ctx.db.Callback().Query().After("gorm:query").Register(name, func(tx *gorm.DB) {
		if tx.Statement.Table != "job_writer_epochs" {
			return
		}
		if _, inTransaction := tx.Statement.ConnPool.(*sql.Tx); !inTransaction {
			return
		}
		if injected.Add(1) > 10 {
			return
		}
		done := make(chan struct{})
		pending.Add(1)
		go func() {
			defer pending.Done()
			defer close(done)
			if _, err := sqlDB.Exec(`INSERT INTO interleaved_commits (n) VALUES (1)`); err == nil {
				landed.Store(true)
			}
		}()
		select {
		case <-done:
		case <-time.After(500 * time.Millisecond):
		}
	}); err != nil {
		t.Fatalf("register the interleave: %v", err)
	}
	t.Cleanup(func() {
		pending.Wait()
		_ = ctx.db.Callback().Query().Remove(name)
	})

	row, err := ctx.CreateScheduledDownload("feeds", owner.ID,
		&query_models.ResourceFromRemoteCreator{URL: "https://example.invalid/under-load.bin"}, time.Now().Add(time.Hour))
	pending.Wait()
	if !landed.Load() {
		t.Fatalf("setup: the competing commit never landed, so nothing was interleaved")
	}
	if err != nil {
		t.Fatalf("a deferred download failed because another connection committed during its transaction: %v", err)
	}
	if got := scheduledDownloadRow(t, ctx, row.ID); got.URL != "https://example.invalid/under-load.bin" {
		t.Fatalf("the stored row = %+v", got)
	}
}
