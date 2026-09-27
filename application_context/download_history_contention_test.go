package application_context

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/spf13/afero"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"mahresources/constants"
	"mahresources/download_queue"
	"mahresources/models"
)

// TestRecordingADownloadSurvivesACommitFromAnotherConnection lands a commit from
// another connection inside the history write's transaction, right after its read
// of the writer epoch, which is where other downloads' commits land under load. On
// SQLite in WAL mode a transaction that read first and writes second cannot
// promote its stale snapshot once anyone else has committed: it fails at once with
// "database is locked", busy_timeout never applies, and the history row was lost.
func TestRecordingADownloadSurvivesACommitFromAnotherConnection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "download-history.db")
	dsn := fmt.Sprintf("file:%s?_journal_mode=WAL&_busy_timeout=10000&_synchronous=NORMAL", path)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&models.DownloadHistoryEntry{}, &models.User{}, &models.LogEntry{},
		&models.RuntimeSetting{}, &models.JobWriterEpoch{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := models.EnsureJobWriterEpoch(db); err != nil {
		t.Fatalf("seed writer epoch: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	ctx := NewMahresourcesContext(afero.NewMemMapFs(), db, sqlx.NewDb(sqlDB, "sqlite3"), &MahresourcesConfig{DbType: constants.DbTypeSqlite})
	cleanupMahresourcesTestContext(t, ctx)
	if _, err := sqlDB.Exec(`CREATE TABLE interleaved_commits (n INTEGER)`); err != nil {
		t.Fatalf("create the competing table: %v", err)
	}

	var landed atomic.Bool
	var injected atomic.Int32
	var pending sync.WaitGroup
	const name = "test:commit-after-history-epoch-read"
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

	completed := time.Now()
	err = ctx.RecordTerminalDownload(download_queue.HistoryRecord{
		JobID: "under-load", URL: "https://example.invalid/under-load.bin", Status: "failed",
		Error: "HTTP 503", CreatedAt: completed, CompletedAt: &completed,
	})
	pending.Wait()
	if !landed.Load() {
		t.Fatalf("setup: the competing commit never landed, so nothing was interleaved")
	}
	if err != nil {
		t.Fatalf("a history write failed because another connection committed during its transaction: %v", err)
	}
	var row models.DownloadHistoryEntry
	if err := ctx.db.Where("job_id = ?", "under-load").First(&row).Error; err != nil {
		t.Fatalf("the history row was lost: %v", err)
	}
}
