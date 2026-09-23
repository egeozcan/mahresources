package main

import (
	"context"
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
	"mahresources/download_queue"
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
	if _, err := installJobControlPlaneBeforePluginActivation(ctx); err != nil {
		t.Fatalf("install the Job control plane: %v", err)
	}

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

func TestImportRetryFactsBackfillOnUpgradeAndReconcileOnRestart(t *testing.T) {
	filesystem := afero.NewMemMapFs()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "import-backfill.db")), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("underlying database: %v", err)
	}
	sqlDB.SetMaxOpenConns(4)
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(
		&models.PluginState{}, &models.PluginKV{}, &models.Group{}, &models.User{},
		&models.Resource{}, &models.ResourceCategory{}, &models.Series{}, &models.Tag{},
	); err != nil {
		t.Fatalf("migrate application tables: %v", err)
	}
	if err := migrateJobCore(db); err != nil {
		t.Fatalf("migrate Job core: %v", err)
	}

	config := &application_context.MahresourcesConfig{
		DbType: constants.DbTypeSqlite, PluginPath: t.TempDir(), MaxJobConcurrency: 2,
	}
	prior := application_context.NewMahresourcesContext(filesystem, db, sqlx.NewDb(sqlDB, "sqlite3"), config)
	keyring, err := jobs.LoadReplayKeyring(jobs.ReplayKeyConfig{Dialect: constants.DbTypeSqlite, Ephemeral: true})
	if err != nil {
		t.Fatalf("load replay keyring: %v", err)
	}
	prior.SetJobReplayKeyring(keyring)
	prior.SetJobService(jobs.NewService())
	// The previous application version had the Job tables but no import file-fact
	// table. Seed its Jobs with that table absent, then run the core migration as
	// the upgrade that introduced the selector index.
	if err := db.Migrator().DropTable(&models.JobImportCommandFact{}); err != nil {
		t.Fatalf("remove the not-yet-upgraded import fact table: %v", err)
	}
	if err := filesystem.MkdirAll("_imports", 0o755); err != nil {
		t.Fatalf("create imports directory: %v", err)
	}
	for _, path := range []string{
		"_imports/upgrade-parse-valid.tar",
		"_imports/upgrade-apply-valid.tar",
		"_imports/upgrade-apply-valid.plan.json",
		"_imports/upgrade-apply-plan-missing.tar",
	} {
		if err := afero.WriteFile(filesystem, path, []byte("staged"), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	seedFailedImport := func(kind, input string) string {
		t.Helper()
		snapshot, err := prior.JobService().Accept(jobs.Deps{
			DB: db, Replay: &jobs.ReplayConfig{Keys: keyring},
		}, jobs.Acceptance{
			Kind: kind, KindVersion: 1, State: jobs.StateQueued, Origin: "startup-test",
			Title: "Existing import", Replay: jobs.ReplayInput{Input: []byte(input)},
		})
		if err != nil {
			t.Fatalf("accept existing %s Job: %v", kind, err)
		}
		if err := db.Model(&models.Job{}).Where("id = ?", snapshot.ID).Update("state", jobs.StateFailed).Error; err != nil {
			t.Fatalf("make existing %s Job failed: %v", kind, err)
		}
		return snapshot.ID
	}
	jobIDs := map[string]string{
		"parse-valid": seedFailedImport(application_context.JobKindGroupImportParse,
			`{"handle":"upgrade-parse-valid","archive":"_imports/upgrade-parse-valid.tar"}`),
		"parse-missing": seedFailedImport(application_context.JobKindGroupImportParse,
			`{"handle":"upgrade-parse-missing","archive":"_imports/upgrade-parse-missing.tar"}`),
		"apply-valid": seedFailedImport(application_context.JobKindGroupImportApply,
			`{"parseHandle":"upgrade-apply-valid","plan":"_imports/upgrade-apply-valid.plan.json","decisions":{"mappingActions":{},"danglingActions":{}}}`),
		"apply-plan-missing": seedFailedImport(application_context.JobKindGroupImportApply,
			`{"parseHandle":"upgrade-apply-plan-missing","plan":"_imports/upgrade-apply-plan-missing.plan.json","decisions":{"mappingActions":{},"danglingActions":{}}}`),
	}
	if err := migrateJobCore(db); err != nil {
		t.Fatalf("migrate Job core during upgrade: %v", err)
	}
	var beforeUpgrade int64
	if err := db.Model(&models.JobImportCommandFact{}).Count(&beforeUpgrade).Error; err != nil {
		t.Fatalf("count import facts before upgrade: %v", err)
	}
	if beforeUpgrade != 0 {
		t.Fatalf("pre-upgrade import fact rows = %d, want none", beforeUpgrade)
	}

	// The next context is the upgraded process. It runs the export sweep before its
	// Job service is installed, then the production installer must backfill command
	// facts from the persisted Job summaries and current artifact files.
	restarted := application_context.NewMahresourcesContext(filesystem, db, sqlx.NewDb(sqlDB, "sqlite3"), config)
	if restarted.PluginManager() != nil {
		t.Cleanup(restarted.PluginManager().Close)
	}
	restarted.DownloadManager().SetSettings(download_queue.NewStaticDownloadSettings(
		download_queue.TimeoutConfig{}, time.Hour))
	restarted.RunStartupExportSweep()
	if _, err := installJobControlPlaneBeforePluginActivation(restarted); err != nil {
		t.Fatalf("install the upgraded Job control plane: %v", err)
	}

	assertRetry := func(name string, want bool) {
		t.Helper()
		commands, err := restarted.JobService().AdvertisedCommands(context.Background(), jobs.Deps{
			DB: db, Replay: &jobs.ReplayConfig{Keys: keyring},
		}, jobs.Access{Administrator: true}, jobIDs[name])
		if err != nil {
			t.Fatalf("advertise %s commands: %v", name, err)
		}
		got := false
		for _, command := range commands {
			got = got || command.Key == jobs.CommandRetry
		}
		if got != want {
			t.Errorf("%s Retry advertised = %v, want %v; commands=%+v", name, got, want, commands)
		}
	}
	assertRetry("parse-valid", true)
	assertRetry("parse-missing", false)
	assertRetry("apply-valid", true)
	assertRetry("apply-plan-missing", false)
	var afterUpgrade int64
	if err := db.Model(&models.JobImportCommandFact{}).Count(&afterUpgrade).Error; err != nil {
		t.Fatalf("count import facts after upgrade: %v", err)
	}
	if afterUpgrade != 4 {
		t.Fatalf("backfilled import fact rows = %d, want one per parse handle", afterUpgrade)
	}

	// A later process reconciles facts that became stale while it was down. Both
	// formerly valid artifacts are removed before this restart; retry must vanish.
	for _, path := range []string{
		"_imports/upgrade-parse-valid.tar",
		"_imports/upgrade-apply-valid.plan.json",
	} {
		if err := filesystem.Remove(path); err != nil {
			t.Fatalf("remove %s before restart: %v", path, err)
		}
	}
	secondRestart := application_context.NewMahresourcesContext(filesystem, db, sqlx.NewDb(sqlDB, "sqlite3"), config)
	if secondRestart.PluginManager() != nil {
		t.Cleanup(secondRestart.PluginManager().Close)
	}
	secondRestart.DownloadManager().SetSettings(download_queue.NewStaticDownloadSettings(
		download_queue.TimeoutConfig{}, time.Hour))
	secondRestart.RunStartupExportSweep()
	if _, err := installJobControlPlaneBeforePluginActivation(secondRestart); err != nil {
		t.Fatalf("install the restarted Job control plane: %v", err)
	}
	for _, name := range []string{"parse-valid", "apply-valid"} {
		commands, err := secondRestart.JobService().AdvertisedCommands(context.Background(), jobs.Deps{
			DB: db, Replay: &jobs.ReplayConfig{Keys: keyring},
		}, jobs.Access{Administrator: true}, jobIDs[name])
		if err != nil {
			t.Fatalf("advertise %s commands after restart: %v", name, err)
		}
		for _, command := range commands {
			if command.Key == jobs.CommandRetry {
				t.Errorf("%s still advertises Retry after its artifact was removed", name)
			}
		}
	}
}
