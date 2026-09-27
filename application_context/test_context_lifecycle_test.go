package application_context

import (
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"

	"mahresources/jobs"
	"mahresources/models"
)

// cleanupMahresourcesTestContext stops the process-lifetime workers started by
// NewMahresourcesContext and closes its database after each fixture test. Test
// contexts otherwise leave queue/plugin loops and SQLite connection openers
// alive until the package process exits, which compounds under -count.
func cleanupMahresourcesTestContext(t *testing.T, ctx *MahresourcesContext) {
	t.Helper()
	t.Cleanup(func() {
		if ctx == nil {
			return
		}
		if ctx.pluginScheduler != nil {
			ctx.pluginScheduler.Stop()
		}
		if err := ctx.StopPluginCommands(); err != nil {
			t.Errorf("stop plugin commands: %v", err)
		}
		if ctx.downloadManager != nil && !ctx.downloadManager.ShuttingDown() {
			ctx.downloadManager.Shutdown()
		}
		// A queue-backed execution's follower goes on writing after its Job reads
		// terminal, and a write during the directory's removal recreates the journal.
		if !ctx.waitQueueFollowers(10 * time.Second) {
			t.Errorf("a queue follower was still running when the fixture closed its database")
		}
		if ctx.pluginManager != nil {
			ctx.pluginManager.Close()
		}
		if ctx.db != nil {
			if sqlDB, err := ctx.db.DB(); err == nil {
				_ = sqlDB.Close()
			}
		}
	})
}

func TestMahresourcesTestContextCleanupStopsQueueAndClosesDatabase(t *testing.T) {
	var ctx *MahresourcesContext
	t.Run("fixture", func(t *testing.T) {
		ctx = createTestContext(t)
	})
	if ctx == nil {
		t.Fatal("fixture did not create a context")
	}
	if !ctx.downloadManager.ShuttingDown() {
		t.Fatal("fixture cleanup left the download cleanup loop running")
	}
	sqlDB, err := ctx.db.DB()
	if err != nil {
		t.Fatalf("get fixture database: %v", err)
	}
	if err := sqlDB.Ping(); err == nil || !strings.Contains(err.Error(), "database is closed") {
		t.Fatalf("database ping after fixture cleanup = %v, want closed database", err)
	}
}

// A queue-backed execution's outcome is published by a goroutine that goes on
// writing after the Job reads terminal: a Resource Reduction's source mapping is
// refreshed once the outcome has committed. The fixture's cleanup closes the
// database and removes its directory, so it has to wait that goroutine out. A
// write that lands during the removal recreates the SQLite journal and fails the
// test that had already passed with "directory not empty".
func TestTestContextCleanupWaitsForTheQueueFollower(t *testing.T) {
	var mappingWritten atomic.Bool
	t.Run("fixture", func(t *testing.T) {
		ctx := newWorkflowJobContext(t)
		isMapping := func(db *gorm.DB) bool { return db.Statement.Table == "job_source_mappings" }
		// Slow enough that the test body has returned before the write lands.
		slow := func(db *gorm.DB) {
			if isMapping(db) {
				time.Sleep(300 * time.Millisecond)
			}
		}
		landed := func(db *gorm.DB) {
			if isMapping(db) && db.Error == nil {
				mappingWritten.Store(true)
			}
		}
		callbacks := ctx.db.Callback()
		if err := errors.Join(
			callbacks.Create().Before("gorm:create").Register("test:slow-mapping-create", slow),
			callbacks.Update().Before("gorm:update").Register("test:slow-mapping-update", slow),
			callbacks.Create().After("gorm:create").Register("test:mapping-created", landed),
			callbacks.Update().After("gorm:update").Register("test:mapping-updated", landed),
		); err != nil {
			t.Fatalf("register callbacks: %v", err)
		}

		reduction := createReductionRowForTest(t, ctx, `{"clusters":[]}`, models.ReductionStatusFailed)
		if _, err := ctx.RequestReductionCompute(reduction.ID, reduction.Version, nil, false, nil); err != nil {
			t.Fatalf("request the compute: %v", err)
		}
		snap := jobOfKindForTest(t, ctx, JobKindReductionCompute)
		waitForSnapshot(t, ctx, snap.ID, "the clustering run to finish", func(s jobs.Snapshot) bool {
			return s.State.Terminal()
		})
	})
	if !mappingWritten.Load() {
		t.Fatal("the fixture closed its database while the queue follower was still publishing")
	}
}
