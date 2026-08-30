package application_context

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/spf13/afero"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"mahresources/constants"
	"mahresources/models"
)

// schedulerTestContext is createTestContextWithPlugins plus the schedule table
// and a temp-file database.
//
// Temp-file rather than mode=memory&cache=private: the scheduler dispatches on
// its own goroutines, and a per-connection database would give each of them an
// empty schema — the failure SetupTestEnv's own lesson is about, and one that
// looks like "the schedule never ran".
func schedulerTestContext(t *testing.T, pluginDir string) *MahresourcesContext {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sched.db")
	dsn := fmt.Sprintf("file:%s?_journal_mode=WAL&_busy_timeout=10000&_synchronous=NORMAL", path)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(
		&models.PluginSchedule{}, &models.ScheduledDownload{}, &models.PluginKV{}, &models.PluginState{},
		&models.LogEntry{}, &models.User{}, &models.Group{}, &models.Note{},
		&models.Resource{}, &models.Tag{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	sqlDB, _ := db.DB()
	return NewMahresourcesContext(afero.NewMemMapFs(), db, sqlx.NewDb(sqlDB, "sqlite3"),
		&MahresourcesConfig{DbType: constants.DbTypeSqlite, PluginPath: pluginDir})
}

// writeSchedulePlugin writes a plugin whose schedule records each run in mah.kv,
// which is the only durable thing a plugin can write without further grants.
func writeSchedulePlugin(t *testing.T, dir, name, every, overlap string) {
	t.Helper()
	pluginDir := filepath.Join(dir, name)
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	src := fmt.Sprintf(`
plugin = { api_version = 1, name = %q, version = "1.0",
           description = "runs on a schedule", capabilities = { "schedule", "kv" } }
function init()
    mah.schedule({ id = "tick", every = %q, overlap = %q, handler = function(job_id)
        local n = tonumber(mah.kv.get("runs") or "0") or 0
        mah.kv.set("runs", tostring(n + 1))
    end })
end
`, name, every, overlap)
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.lua"), []byte(src), 0o644); err != nil {
		t.Fatalf("write plugin: %v", err)
	}
}

// kvRuns reads the counter back. mah.kv stores JSON, so a Lua string lands as
// `"1"` and not as `1` — reading it as a bare number silently yields zero, which
// looks exactly like "the schedule never ran".
func kvRuns(t *testing.T, ctx *MahresourcesContext, plugin string) int {
	t.Helper()
	var kv models.PluginKV
	if err := ctx.db.Where("plugin_name = ? AND key = ?", plugin, "runs").First(&kv).Error; err != nil {
		return 0
	}
	var raw any
	if err := json.Unmarshal([]byte(kv.Value), &raw); err != nil {
		t.Fatalf("plugin kv %q is not JSON: %q (%v)", "runs", kv.Value, err)
	}
	switch v := raw.(type) {
	case string:
		n, err := strconv.Atoi(v)
		if err != nil {
			t.Fatalf("plugin kv \"runs\" is not a number: %q", v)
		}
		return n
	case float64:
		return int(v)
	default:
		t.Fatalf("plugin kv \"runs\" has unexpected type %T", raw)
		return 0
	}
}

// The whole path, end to end: a plugin declares a schedule, the bridge records
// it against the operator who enabled it, a tick claims it, and the handler
// actually runs.
func TestSchedulerRunsADueScheduleAsItsOwner(t *testing.T) {
	dir := t.TempDir()
	writeSchedulePlugin(t, dir, "poller", "1m", "skip")
	ctx := schedulerTestContext(t, dir)

	operator := models.User{Username: "op", Role: models.RoleAdmin, PasswordHash: "x"}
	if err := ctx.db.Create(&operator).Error; err != nil {
		t.Fatalf("seed operator: %v", err)
	}
	ctx.refreshRootAdmin()

	pm := ctx.PluginManager()
	if pm == nil {
		t.Fatal("no plugin manager")
	}
	if err := pm.EnablePlugin("poller"); err != nil {
		t.Fatalf("enable: %v", err)
	}
	if err := ctx.SyncPluginSchedules("poller", pm.DeclaredSchedules("poller")); err != nil {
		t.Fatalf("sync: %v", err)
	}

	// A new schedule is due one interval out, not immediately.
	sched := scheduleRow(t, ctx, 1)
	if !sched.NextDueAt.After(time.Now()) {
		t.Fatal("a freshly registered schedule was already due")
	}
	scheduler := NewPluginScheduler(ctx, time.Minute)
	scheduler.Tick(time.Now())
	scheduler.Stop()
	if got := kvRuns(t, ctx, "poller"); got != 0 {
		t.Fatalf("a schedule that is not due ran %d times", got)
	}

	// Make it due, and it runs exactly once.
	if err := ctx.db.Model(&models.PluginSchedule{}).Where("id = ?", sched.ID).
		Update("next_due_at", time.Now().Add(-time.Minute)).Error; err != nil {
		t.Fatalf("make due: %v", err)
	}
	scheduler = NewPluginScheduler(ctx, time.Minute)
	scheduler.Tick(time.Now())
	scheduler.Stop()

	if got := kvRuns(t, ctx, "poller"); got != 1 {
		t.Fatalf("the handler ran %d times, want 1", got)
	}
	after := scheduleRow(t, ctx, sched.ID)
	if after.Runs != 1 {
		t.Errorf("Runs = %d, want 1", after.Runs)
	}
	if after.LastStatus != models.PluginScheduleStatusCompleted {
		t.Errorf("LastStatus = %q, want %q", after.LastStatus, models.PluginScheduleStatusCompleted)
	}
	if after.ClaimToken != "" {
		t.Error("the run finished with its claim still held; the schedule would never run again")
	}
	if !after.NextDueAt.After(time.Now()) {
		t.Error("the schedule was not advanced, so the next tick runs it again immediately")
	}
}

// A disabled plugin's row survives and is inert. That is the join the whole
// design rests on: nothing deletes the row, and nothing runs it either.
func TestSchedulerSkipsARowWhosePluginIsDisabled(t *testing.T) {
	dir := t.TempDir()
	writeSchedulePlugin(t, dir, "poller", "1m", "skip")
	ctx := schedulerTestContext(t, dir)

	pm := ctx.PluginManager()
	if err := pm.EnablePlugin("poller"); err != nil {
		t.Fatalf("enable: %v", err)
	}
	if err := ctx.SyncPluginSchedules("poller", pm.DeclaredSchedules("poller")); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if err := ctx.db.Model(&models.PluginSchedule{}).Where("plugin_name = ?", "poller").
		Update("next_due_at", time.Now().Add(-time.Minute)).Error; err != nil {
		t.Fatalf("make due: %v", err)
	}

	if err := pm.DisablePlugin("poller"); err != nil {
		t.Fatalf("disable: %v", err)
	}

	scheduler := NewPluginScheduler(ctx, time.Minute)
	scheduler.Tick(time.Now())
	scheduler.Stop()

	if got := kvRuns(t, ctx, "poller"); got != 0 {
		t.Fatalf("a disabled plugin's schedule ran %d times", got)
	}
	var count int64
	if err := ctx.db.Model(&models.PluginSchedule{}).Where("plugin_name = ?", "poller").
		Count(&count).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("the row was deleted on disable (count=%d); re-enabling would lose the "+
			"operator binding it already had", count)
	}
	// And the claim was never taken, so nothing has to expire before it can run
	// again once the plugin comes back.
	var row models.PluginSchedule
	if err := ctx.db.Where("plugin_name = ?", "poller").First(&row).Error; err != nil {
		t.Fatalf("read: %v", err)
	}
	if row.ClaimToken != "" {
		t.Fatalf("a skipped row was left claimed (%q); it would be unavailable until the "+
			"claim expired", row.ClaimToken)
	}
}

// A handler that raises records a failure and still advances, rather than
// wedging on a row it can never get past.
func TestSchedulerRecordsAFailingHandlerAndMovesOn(t *testing.T) {
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "breaker")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	src := `
plugin = { api_version = 1, name = "breaker", version = "1.0",
           description = "fails on purpose", capabilities = { "schedule" } }
function init()
    mah.schedule({ id = "tick", every = "1m", handler = function(job_id)
        error("the remote said no")
    end })
end
`
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.lua"), []byte(src), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	ctx := schedulerTestContext(t, dir)
	// Without an admin the acting user resolves to 0, the row is created unowned,
	// and an unowned row is never claimed — so this test would assert nothing
	// about failure handling and pass anyway.
	if err := ctx.db.Create(&models.User{Username: "op", Role: models.RoleAdmin, PasswordHash: "x"}).Error; err != nil {
		t.Fatalf("seed operator: %v", err)
	}
	ctx.refreshRootAdmin()

	pm := ctx.PluginManager()
	if err := pm.EnablePlugin("breaker"); err != nil {
		t.Fatalf("enable: %v", err)
	}
	if err := ctx.SyncPluginSchedules("breaker", pm.DeclaredSchedules("breaker")); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if err := ctx.db.Model(&models.PluginSchedule{}).Where("plugin_name = ?", "breaker").
		Update("next_due_at", time.Now().Add(-time.Minute)).Error; err != nil {
		t.Fatalf("make due: %v", err)
	}

	scheduler := NewPluginScheduler(ctx, time.Minute)
	scheduler.Tick(time.Now())
	scheduler.Stop()

	var row models.PluginSchedule
	if err := ctx.db.Where("plugin_name = ?", "breaker").First(&row).Error; err != nil {
		t.Fatalf("read: %v", err)
	}
	if row.LastStatus != models.PluginScheduleStatusFailed {
		t.Fatalf("LastStatus = %q, want %q", row.LastStatus, models.PluginScheduleStatusFailed)
	}
	if row.LastError == "" {
		t.Error("a failed run recorded no error, so an operator has nothing to look at")
	}
	if row.ClaimToken != "" {
		t.Fatal("a failed run kept its claim; the schedule would be stuck until it expired")
	}
	if !row.NextDueAt.After(time.Now()) {
		t.Fatal("a failed run did not advance the schedule, so it retries in a tight loop")
	}
}

// The other half of the bounded acquire, and the half that matters to the
// database: a tick that could not get a job slot must hand its claim back and
// leave the row due. Without this the schedule is unavailable until the claim
// expires — seven to eight minutes of silence for a row that is due now.
func TestDispatchReleasesTheClaimWhenTheJobBudgetIsFull(t *testing.T) {
	dir := t.TempDir()
	writeSchedulePlugin(t, dir, "poller", "1m", "skip")
	ctx := schedulerTestContext(t, dir)
	if err := ctx.db.Create(&models.User{Username: "op", Role: models.RoleAdmin, PasswordHash: "x"}).Error; err != nil {
		t.Fatalf("seed operator: %v", err)
	}
	ctx.refreshRootAdmin()

	pm := ctx.PluginManager()
	if err := pm.EnablePlugin("poller"); err != nil {
		t.Fatalf("enable: %v", err)
	}
	if err := ctx.SyncPluginSchedules("poller", pm.DeclaredSchedules("poller")); err != nil {
		t.Fatalf("sync: %v", err)
	}
	due := time.Now().Add(-time.Minute)
	if err := ctx.db.Model(&models.PluginSchedule{}).Where("plugin_name = ?", "poller").
		Update("next_due_at", due).Error; err != nil {
		t.Fatalf("make due: %v", err)
	}

	release := pm.FillJobBudgetForTest()
	defer release()

	scheduler := NewPluginScheduler(ctx, time.Minute)
	// The give-up is what is under test, not how long it takes to give up.
	scheduler.dispatchWait = 50 * time.Millisecond
	scheduler.Tick(time.Now())
	scheduler.Stop()

	var row models.PluginSchedule
	if err := ctx.db.Where("plugin_name = ?", "poller").First(&row).Error; err != nil {
		t.Fatalf("read: %v", err)
	}
	if row.ClaimToken != "" {
		t.Fatalf("a tick that could not get a job slot kept its claim (%q); the schedule is "+
			"unavailable until that claim expires", row.ClaimToken)
	}
	if row.Runs != 0 {
		t.Errorf("Runs = %d for a run that never started", row.Runs)
	}
	if !row.NextDueAt.Equal(due) && row.NextDueAt.After(due.Add(time.Second)) {
		t.Errorf("NextDueAt moved to %s; a tick that ran nothing must leave the row due", row.NextDueAt)
	}

	// And the next tick, with the budget free again, runs it.
	release()
	scheduler = NewPluginScheduler(ctx, time.Minute)
	scheduler.Tick(time.Now())
	scheduler.Stop()
	if got := kvRuns(t, ctx, "poller"); got != 1 {
		t.Fatalf("the handler ran %d times once the budget freed up, want 1", got)
	}
}

// overlap = "allow" takes the other branch entirely: it advances and releases at
// dispatch rather than at completion, so the outcome is recorded by a write that
// no longer holds the claim. Nothing else in this file executes that path.
func TestSchedulerRunsAnOverlapAllowSchedule(t *testing.T) {
	dir := t.TempDir()
	writeSchedulePlugin(t, dir, "poller", "1m", "allow")
	ctx := schedulerTestContext(t, dir)
	if err := ctx.db.Create(&models.User{Username: "op", Role: models.RoleAdmin, PasswordHash: "x"}).Error; err != nil {
		t.Fatalf("seed operator: %v", err)
	}
	ctx.refreshRootAdmin()

	pm := ctx.PluginManager()
	if err := pm.EnablePlugin("poller"); err != nil {
		t.Fatalf("enable: %v", err)
	}
	if err := ctx.SyncPluginSchedules("poller", pm.DeclaredSchedules("poller")); err != nil {
		t.Fatalf("sync: %v", err)
	}

	var row models.PluginSchedule
	if err := ctx.db.Where("plugin_name = ?", "poller").First(&row).Error; err != nil {
		t.Fatalf("read: %v", err)
	}
	if row.Overlap != models.PluginScheduleOverlapAllow {
		t.Fatalf("Overlap = %q, want %q — the declaration did not reach the row",
			row.Overlap, models.PluginScheduleOverlapAllow)
	}
	if err := ctx.db.Model(&models.PluginSchedule{}).Where("id = ?", row.ID).
		Update("next_due_at", time.Now().Add(-time.Minute)).Error; err != nil {
		t.Fatalf("make due: %v", err)
	}

	scheduler := NewPluginScheduler(ctx, time.Minute)
	scheduler.Tick(time.Now())
	scheduler.Stop()

	if got := kvRuns(t, ctx, "poller"); got != 1 {
		t.Fatalf("the handler ran %d times, want 1", got)
	}
	after := scheduleRow(t, ctx, row.ID)
	if after.ClaimToken != "" {
		t.Error("the claim was not released")
	}
	if after.Runs != 1 {
		t.Errorf("Runs = %d, want 1", after.Runs)
	}
	if after.LastStatus != models.PluginScheduleStatusCompleted {
		t.Errorf("LastStatus = %q, want %q", after.LastStatus, models.PluginScheduleStatusCompleted)
	}
	if !after.NextDueAt.After(time.Now()) {
		t.Error("the schedule was not advanced")
	}
}
