package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/spf13/afero"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"mahresources/application_context"
	"mahresources/constants"
	"mahresources/jobs"
	"mahresources/models"
)

// TestAnEnabledPluginsStartJobAtBootIsDurable pins the startup ordering that decides
// whether a plugin's own boot-time work is durable.
//
// An enabled plugin's init() runs during activation, `mah.start_job` reaches the durable
// control plane only once the host half of that seam is installed, and the plugin manager
// keeps its in-memory registry otherwise. Installing the plane *after* activation
// therefore made every job a plugin started at boot a memory-only record — on every boot,
// while the same call from a request was durable. The order is asserted by driving the two
// production steps in the order main drives them.
func TestAnEnabledPluginsStartJobAtBootIsDurable(t *testing.T) {
	pluginDir := t.TempDir()
	pluginName := "boot-worker"
	if err := os.MkdirAll(filepath.Join(pluginDir, pluginName), 0o755); err != nil {
		t.Fatalf("mkdir plugin: %v", err)
	}
	source := `
plugin = { name = "` + pluginName + `", version = "1.0", api_version = 1,
           capabilities = { "jobs" } }

function work(job_id)
    mah.job_complete(job_id, { message = "boot child done" })
end

function init()
    mah.start_job("boot work", work)
end
`
	if err := os.WriteFile(filepath.Join(pluginDir, pluginName, "plugin.lua"), []byte(source), 0o644); err != nil {
		t.Fatalf("write plugin: %v", err)
	}

	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "boot.db")), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("underlying database: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(
		&models.PluginState{}, &models.PluginKV{}, &models.Group{}, &models.User{},
		&models.Resource{}, &models.ResourceCategory{}, &models.Series{}, &models.Tag{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := migrateJobCore(db); err != nil {
		t.Fatalf("migrateJobCore: %v", err)
	}

	ctx := application_context.NewMahresourcesContext(afero.NewMemMapFs(), db, sqlx.NewDb(sqlDB, "sqlite3"),
		&application_context.MahresourcesConfig{
			DbType:            constants.DbTypeSqlite,
			PluginPath:        pluginDir,
			MaxJobConcurrency: 4,
		})
	// The production keyring, loaded before the context is built exactly as main loads
	// it. A Job accepted without one could not be accepted at all.
	keyring, err := jobs.LoadReplayKeyring(jobs.ReplayKeyConfig{Dialect: constants.DbTypeSqlite, Ephemeral: true})
	if err != nil {
		t.Fatalf("load the replay keyring: %v", err)
	}
	ctx.SetJobReplayKeyring(keyring)

	if ctx.PluginManager() == nil {
		t.Fatal("the test context has no plugin manager")
	}
	// The operator's earlier decision, reproduced: a discovered plugin starts disabled,
	// activation is only about the ones already turned on, and the row that records the
	// decision is written by an earlier boot.
	if _, err := ctx.EnsurePluginStates(); err != nil {
		t.Fatalf("initialize plugin states: %v", err)
	}
	if err := db.Model(&models.PluginState{}).Where("plugin_name = ?", pluginName).
		Update("enabled", true).Error; err != nil {
		t.Fatalf("enable %s: %v", pluginName, err)
	}

	// The production step, the one main calls.
	installJobControlPlaneBeforePluginActivation(ctx)

	// The plugin's init() started work. With the plane installed first that work is a
	// durable Job; with the order reversed nothing durable exists to find.
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		var durable int64
		if err := db.Model(&models.Job{}).
			Where("kind = ?", application_context.JobKindPluginAction).Count(&durable).Error; err != nil {
			t.Fatalf("count the jobs table: %v", err)
		}
		if durable > 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("a plugin's mah.start_job during init left no durable job: the control plane was installed after activation")
}
