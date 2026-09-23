package application_context

import (
	"strings"
	"testing"
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
