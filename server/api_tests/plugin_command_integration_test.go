package api_tests

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/spf13/afero"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"mahresources/application_context"
	"mahresources/constants"
	"mahresources/models"
	"mahresources/models/seed"
	"mahresources/plugin_commands"
	"mahresources/server"
)

const commandIntegrationPluginName = "command-integration"

// TestPluginCommandFixtureProcess is copied to the test's trusted command path
// and executed as the real child process. The MAHR marker keeps the ordinary
// parent test invocation inert.
func TestPluginCommandFixtureProcess(t *testing.T) {
	if os.Getenv("MAHR_COMMAND_RUN_ID") == "" {
		return
	}
	args := os.Args
	separator := -1
	for i, arg := range args {
		if arg == "--" {
			separator = i
			break
		}
	}
	if separator < 0 || len(args) != separator+3 {
		fmt.Fprintf(os.Stderr, "fixture args: %q\n", args)
		os.Exit(2)
	}
	mode, exchangeDir := args[separator+1], args[separator+2]
	switch mode {
	case "produce":
		if err := os.WriteFile(filepath.Join(exchangeDir, "import.bin"), []byte("real plugin command output"), 0o600); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(3)
		}
		if err := os.WriteFile(filepath.Join(exchangeDir, "discard.txt"), []byte("discard me"), 0o600); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(4)
		}
	case "restart":
		if err := os.WriteFile(filepath.Join(exchangeDir, "restart.bin"), []byte("restart callback output"), 0o600); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(5)
		}
	case "wait":
		if err := os.WriteFile(filepath.Join(exchangeDir, "started.marker"), []byte("started"), 0o600); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(6)
		}
		select {}
	default:
		fmt.Fprintf(os.Stderr, "unknown fixture mode %q\n", mode)
		os.Exit(7)
	}
}

type commandIntegrationSettings struct {
	root        string
	commandPath string
}

func (s commandIntegrationSettings) StagingRoot() string              { return s.root }
func (s commandIntegrationSettings) PendingPerPluginLimit() int       { return 100 }
func (s commandIntegrationSettings) PerRunQuota() int64               { return 8 << 30 }
func (s commandIntegrationSettings) GlobalStagingQuota() int64        { return 50 << 30 }
func (s commandIntegrationSettings) ExchangeRetention() time.Duration { return 7 * 24 * time.Hour }
func (s commandIntegrationSettings) OutputRetention() time.Duration   { return 30 * 24 * time.Hour }
func (s commandIntegrationSettings) CommandPath() string              { return s.commandPath }

// gatedResourceFS blocks only the resource destination Create call. Command
// staging remains the real descriptor-relative OS path, and the import worker
// reaches this gate only after it has durably transitioned to running, copied
// its snapshot, and entered the production AddResource path.
type gatedResourceFS struct {
	afero.Fs
	mu      sync.Mutex
	gate    chan struct{}
	entered chan struct{}
}

func (f *gatedResourceFS) arm(capacity int) <-chan struct{} {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.gate != nil {
		panic("resource filesystem gate already armed")
	}
	f.gate = make(chan struct{})
	f.entered = make(chan struct{}, capacity)
	return f.entered
}

func (f *gatedResourceFS) release() {
	f.mu.Lock()
	gate := f.gate
	f.gate = nil
	f.entered = nil
	f.mu.Unlock()
	if gate != nil {
		close(gate)
	}
}

func (f *gatedResourceFS) Create(name string) (afero.File, error) {
	f.mu.Lock()
	gate, entered := f.gate, f.entered
	f.mu.Unlock()
	if gate != nil && strings.HasPrefix(filepath.ToSlash(name), "/resources/") {
		select {
		case entered <- struct{}{}:
		default:
		}
		<-gate
	}
	return f.Fs.Create(name)
}

func waitForResourceCreates(t *testing.T, entered <-chan struct{}, count int) {
	t.Helper()
	for i := 0; i < count; i++ {
		select {
		case <-entered:
		case <-time.After(10 * time.Second):
			t.Fatalf("only %d of %d real imports reached the resource filesystem", i, count)
		}
	}
}

// openPersistentCommandTestContext mirrors process construction while keeping
// the database and resource filesystem explicit so the test can close every
// process-owned object and rebuild them against the same durable state.
func openPersistentCommandTestContext(t *testing.T, dbPath, pluginDir string, filesystem afero.Fs) (*TestContext, func()) {
	t.Helper()
	dsn := dbPath + "?_busy_timeout=10000&_journal_mode=WAL&_foreign_keys=on"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(
		&models.Query{}, &models.Series{}, &models.Resource{}, &models.ResourceVersion{},
		&models.Note{}, &models.NoteBlock{}, &models.Tag{}, &models.Group{},
		&models.Category{}, &models.ResourceCategory{}, &models.NoteType{}, &models.Preview{},
		&models.GroupRelation{}, &models.GroupRelationType{}, &models.ImageHash{},
		&models.ResourceSimilarity{}, &models.LogEntry{}, &models.PluginState{}, &models.PluginKV{},
		&models.SavedMRQLQuery{}, &models.TemplatePartial{}, &models.RuntimeSetting{}, &models.User{},
		&models.SavedSearch{}, &models.UserSetting{}, &models.Session{}, &models.ApiToken{},
		&models.DownloadHistoryEntry{}, &models.ScheduledDownload{}, &models.ResourceReduction{},
		&models.PluginSchedule{}, &models.PluginCommandRun{}, &models.PluginCommandRunOutput{},
		&models.PluginCommandImport{}, &models.PluginCommandImportMap{},
	); err != nil {
		t.Fatal(err)
	}
	if err := models.EnsureSupplementalIndexes(db); err != nil {
		t.Fatal(err)
	}
	seed.AddInitialData(db)

	config := &application_context.MahresourcesConfig{
		DbType: constants.DbTypeSqlite, PluginPath: pluginDir, AuthEnabled: true,
		SessionTTL: time.Hour, MaxUploadSize: 2 << 30, MaxImportSize: 10 << 30,
		MRQLDefaultLimit: 500, MRQLQueryTimeoutBoot: 10 * time.Second,
		RemoteResourceConnectTimeout: 30 * time.Second, RemoteResourceIdleTimeout: time.Minute,
		RemoteResourceOverallTimeout: 30 * time.Minute, ExportRetention: 24 * time.Hour,
		AltFileSystems: map[string]string{},
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	readOnlyDB := sqlx.NewDb(sqlDB, "sqlite3")
	appCtx := application_context.NewMahresourcesContext(filesystem, db, readOnlyDB, config)
	settings := application_context.NewRuntimeSettings(
		db, application_context.NewStdlibSettingsLogger(), application_context.BuildSpecsExported(),
		application_context.BuildDefaultsFromConfig(config),
	)
	if err := settings.Load(); err != nil {
		t.Fatal(err)
	}
	appCtx.SetSettings(settings)
	defaultRC := &models.ResourceCategory{ID: 1, Name: "Default", Description: "Default resource category."}
	if err := db.FirstOrCreate(defaultRC, 1).Error; err != nil {
		t.Fatal(err)
	}
	appCtx.DefaultResourceCategoryID = defaultRC.ID
	serverInstance := server.CreateServer(appCtx, filesystem, map[string]string{})
	closed := false
	closeContext := func() {
		if closed {
			return
		}
		closed = true
		if appCtx.PluginManager() != nil {
			appCtx.PluginManager().Close()
		}
		_ = sqlDB.Close()
	}
	return &TestContext{AppCtx: appCtx, Router: serverInstance.Handler, DB: db, Fs: filesystem}, closeContext
}

func installCommandIntegrationExecutable(t *testing.T, commandDir string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("plugin commands are unsupported on Windows")
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	name := "command-fixture"
	target := filepath.Join(commandDir, name)
	if err := os.Link(executable, target); err == nil {
		return name
	}
	source, err := os.Open(executable)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	destination, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o700)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(destination, source); err != nil {
		_ = destination.Close()
		t.Fatal(err)
	}
	if err := destination.Close(); err != nil {
		t.Fatal(err)
	}
	return name
}

func writeCommandIntegrationPlugin(t *testing.T, root, executable string) {
	t.Helper()
	dir := filepath.Join(root, commandIntegrationPluginName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	source := fmt.Sprintf(`
plugin = {
  name = %q, version = "1", api_version = 1,
  capabilities = {"commands", "db:write", "pages"},
  commands = {
    {name = "produce", argv = {%q, "-test.run=TestPluginCommandFixtureProcess", "--", "produce", "{{exchange_dir}}"}, timeout = 60},
    {name = "restart", argv = {%q, "-test.run=TestPluginCommandFixtureProcess", "--", "restart", "{{exchange_dir}}"}, timeout = 60},
    {name = "wait", argv = {%q, "-test.run=TestPluginCommandFixtureProcess", "--", "wait", "{{exchange_dir}}"}, timeout = 60},
  },
}

local completion_result = nil
local listing_result = nil
local import_result = nil
local redrive_before = nil
local redrive_id = nil
local restart_queued = false

local function queue_output(result)
  completion_result = result
  local listing, list_err = mah.fs.list(result.run_id)
  assert(listing, list_err)
  listing_result = listing
  for _, entry in ipairs(listing.entries) do
    if entry.name == "import.bin" then
      local import_id, import_err = mah.fs.create_resource(result.run_id, entry.name, {
        name = "command integration resource",
        description = "imported by the real command fixture",
        meta = {source = "plugin-command"},
      }, function(imported)
        import_result = imported
      end)
      assert(import_id, import_err)
    elseif entry.name == "discard.txt" then
      local ok, discard_err = mah.fs.discard(result.run_id, entry.name)
      assert(ok, discard_err)
    end
  end
end

function init()
  mah.page("start", function()
    local run_id, err = mah.commands.run("produce", {}, queue_output)
    assert(run_id, err)
    return "started:" .. run_id
  end)

  mah.page("status", function()
    return mah.json.encode({
      completion = completion_result,
      listing = listing_result,
      imported = import_result,
      restart_queued = restart_queued,
      redrive_before = redrive_before,
      redrive_id = redrive_id,
    })
  end)

  mah.page("start-restart", function()
    local run_id, err = mah.commands.run("restart", {}, function(result)
      completion_result = result
      local listing, list_err = mah.fs.list(result.run_id)
      assert(listing, list_err)
      listing_result = listing
      local import_id, import_err = mah.fs.create_resource(result.run_id, "restart.bin", {
        name = "restart redrive resource",
      }, function(imported)
        import_result = imported
      end)
      assert(import_id, import_err)
      restart_queued = true
    end)
    assert(run_id, err)
    return "started:" .. run_id
  end)

  mah.page("redrive", function()
    local runs, runs_err = mah.fs.runs()
    assert(runs, runs_err)
    local restart_run_id = nil
    for _, run in ipairs(runs) do
      if run.command == "restart" and run.imports["restart.bin"] ~= nil then
        restart_run_id = run.id
        redrive_before = run.imports["restart.bin"].status
        redrive_id = run.imports["restart.bin"].import_id
        break
      end
    end
    assert(restart_run_id, "restart run is absent from durable runs")
    local import_id, import_err = mah.fs.create_resource(restart_run_id, "restart.bin", {
      name = "restart redrive resource",
    }, function(imported)
      import_result = imported
    end)
    assert(import_id, import_err)
    return redrive_before .. ":" .. redrive_id .. ":" .. import_id
  end)

  mah.page("start-loss", function()
    local run_id, err = mah.commands.run("wait", {}, function(result)
      -- mah.log is independent of command/exchange admission. A persisted row
      -- therefore proves the callback body ran; unlike mah.fs.discard, it
      -- cannot be rejected merely because the old generation was revoked.
      mah.log("warning", "forbidden-command-callback:" .. result.run_id)
    end)
    assert(run_id, err)
    return "started:" .. run_id
  end)

  mah.page("loss-status", function()
    local runs, runs_err = mah.fs.runs()
    assert(runs, runs_err)
    for _, run in ipairs(runs) do
      if run.command == "wait" then
        return run.id .. ":" .. run.status
      end
    end
    return "missing"
  end)
end
`, commandIntegrationPluginName, executable, executable, executable)
	if err := os.WriteFile(filepath.Join(dir, "plugin.lua"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
}

func commandIntegrationBearer(t *testing.T, tc *TestContext) (*models.User, string) {
	t.Helper()
	user, err := tc.AppCtx.EnsureAdminUser("command-admin", "password1")
	if err != nil {
		t.Fatal(err)
	}
	raw, _, err := tc.AppCtx.CreateApiToken(user.ID, "command-integration", nil)
	if err != nil {
		t.Fatal(err)
	}
	return user, "Bearer " + raw
}

func waitForCommandRun(t *testing.T, ctx *application_context.MahresourcesContext, command string, statuses ...string) plugin_commands.RunRecord {
	t.Helper()
	allowed := make(map[string]bool, len(statuses))
	for _, status := range statuses {
		allowed[status] = true
	}
	deadline := time.Now().Add(10 * time.Second)
	var last []plugin_commands.RunRecord
	for time.Now().Before(deadline) {
		runs, _, err := ctx.GetPluginCommandRuns(0, 100)
		if err != nil {
			t.Fatal(err)
		}
		last = runs
		for _, run := range runs {
			if run.PluginName == commandIntegrationPluginName && run.CommandName == command && allowed[run.Status] {
				return run
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("command %q never reached %v; rows=%+v", command, statuses, last)
	return plugin_commands.RunRecord{}
}

func waitForImport(t *testing.T, ctx *application_context.MahresourcesContext, runID, name, status string) plugin_commands.ImportMapEntry {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	var last plugin_commands.ImportMapEntry
	for time.Now().Before(deadline) {
		view, _, err := ctx.GetPluginCommandRun(runID)
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range view.Imports {
			if item.FileName == name {
				last = item
				if item.Status == status {
					return item
				}
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("import %s/%s never reached %q; last=%+v", runID, name, status, last)
	return plugin_commands.ImportMapEntry{}
}

func createImportBlocker(t *testing.T, ctx *application_context.MahresourcesContext, stagingRoot, id string, actorID uint, generation uint64) {
	t.Helper()
	now := time.Now().UTC()
	if err := ctx.CreateRun(plugin_commands.RunRecord{
		ID: id, PluginName: commandIntegrationPluginName, CommandName: "produce",
		ParamsJSON: `{}`, Status: plugin_commands.RunStatusQueued, CreatedByUserID: &actorID, CreatedAt: now,
	}, plugin_commands.RunOutput{RunID: id, ArgvJSON: `[]`, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if won, err := ctx.MarkRunRunning(id, now); err != nil || !won {
		t.Fatalf("mark blocker running: won=%v err=%v", won, err)
	}
	if won, err := ctx.FinishRun(id, plugin_commands.RunFinish{Status: plugin_commands.RunStatusSucceeded, FinishedAt: now}); err != nil || !won {
		t.Fatalf("finish blocker: won=%v err=%v", won, err)
	}
	dir := filepath.Join(stagingRoot, "plugin_exchange", commandIntegrationPluginName, id)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	name := id + ".bin"
	if err := os.WriteFile(filepath.Join(dir, name), []byte("unique "+id), 0o600); err != nil {
		t.Fatal(err)
	}
	actor := actorID
	if _, err := ctx.SubmitCommandImport(plugin_commands.ImportSubmission{
		Access: plugin_commands.Access{PluginName: commandIntegrationPluginName, ActorUserID: &actor},
		RunID:  id, Name: name, Fields: plugin_commands.ResourceFields{Name: id},
		PluginGeneration: generation, ActorUserID: &actor,
	}); err != nil {
		t.Fatal(err)
	}
}

func waitForPageContains(t *testing.T, tc *TestContext, bearer, path string, values ...string) string {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	var body string
	for time.Now().Before(deadline) {
		response := doReq(tc, http.MethodGet, path, map[string]string{"Accept": "text/html", "Authorization": bearer}, nil, nil)
		if response.Code != http.StatusOK {
			t.Fatalf("GET %s = %d %s", path, response.Code, response.Body.String())
		}
		body = response.Body.String()
		matched := true
		for _, value := range values {
			if !strings.Contains(body, value) {
				matched = false
				break
			}
		}
		if matched {
			return body
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("GET %s never contained %q; body=%s", path, values, body)
	return ""
}

func TestPluginCommandHostIntegrationRestartRedriveAndCallbackLoss(t *testing.T) {
	pluginDir := t.TempDir()
	commandDir := t.TempDir()
	stagingRoot := t.TempDir()
	databasePath := filepath.Join(t.TempDir(), "command-integration.db")
	resourceRoot := t.TempDir()
	resourceFS := &gatedResourceFS{Fs: afero.NewBasePathFs(afero.NewOsFs(), resourceRoot)}
	executable := installCommandIntegrationExecutable(t, commandDir)
	writeCommandIntegrationPlugin(t, pluginDir, executable)
	settings := commandIntegrationSettings{root: stagingRoot, commandPath: commandDir}

	tc, closeCurrent := openPersistentCommandTestContext(t, databasePath, pluginDir, resourceFS)
	commandsStarted := false
	t.Cleanup(func() {
		resourceFS.release()
		if commandsStarted {
			_ = tc.AppCtx.StopPluginCommands()
		}
		closeCurrent()
	})
	if tc.AppCtx.PluginManager() == nil {
		t.Fatal("plugin manager is unavailable")
	}
	if err := tc.AppCtx.StartPluginCommands(context.Background(), settings); err != nil {
		t.Fatal(err)
	}
	commandsStarted = true
	if _, err := tc.AppCtx.EnsurePluginStates(); err != nil {
		t.Fatal(err)
	}
	actor, bearer := commandIntegrationBearer(t, tc)
	if err := tc.AppCtx.WithPrincipal(nil).SetPluginEnabledWithOptions(
		commandIntegrationPluginName, true, application_context.PluginEnableOptions{ConfirmCommands: true},
	); err != nil {
		t.Fatal(err)
	}

	// Block the production AddResource destination. The completion callback has
	// already listed and discarded files and SubmitCommandImport has returned;
	// meanwhile the real import worker is durably running inside byte transfer.
	// A status page must still acquire the plugin VM immediately.
	entered := resourceFS.arm(1)
	start := doReq(tc, http.MethodGet, "/plugins/command-integration/start", map[string]string{"Accept": "text/html", "Authorization": bearer}, nil, nil)
	if start.Code != http.StatusOK || !strings.Contains(start.Body.String(), "started:") {
		t.Fatalf("start page = %d %s", start.Code, start.Body.String())
	}
	produce := waitForCommandRun(t, tc.AppCtx, "produce", plugin_commands.RunStatusSucceeded)
	waitForResourceCreates(t, entered, 1)
	mapped := waitForImport(t, tc.AppCtx, produce.ID, "import.bin", plugin_commands.ImportStatusRunning)
	pageStarted := time.Now()
	status := doReq(tc, http.MethodGet, "/plugins/command-integration/status", map[string]string{"Accept": "text/html", "Authorization": bearer}, nil, nil)
	if status.Code != http.StatusOK || time.Since(pageStarted) > time.Second {
		t.Fatalf("status page waited for real import work: code=%d elapsed=%s body=%s", status.Code, time.Since(pageStarted), status.Body.String())
	}
	for _, want := range []string{`"ok":true`, produce.ID, `"name":"import.bin"`, `"truncated":false`} {
		if !strings.Contains(status.Body.String(), want) {
			t.Fatalf("status page %q does not contain %q", status.Body.String(), want)
		}
	}
	resourceFS.release()
	mapped = waitForImport(t, tc.AppCtx, produce.ID, "import.bin", plugin_commands.ImportStatusSucceeded)
	if mapped.ResourceID == nil {
		t.Fatal("successful import has no resource id")
	}
	waitForPageContains(t, tc, bearer, "/plugins/command-integration/status", mapped.ImportID, `"resource_id":`)

	var resource models.Resource
	if err := tc.DB.First(&resource, *mapped.ResourceID).Error; err != nil {
		t.Fatal(err)
	}
	if resource.CreatedByUserId == nil || *resource.CreatedByUserId != actor.ID {
		t.Fatalf("resource creator = %v, want actor %d", resource.CreatedByUserId, actor.ID)
	}
	if resource.Name != "command integration resource" {
		t.Fatalf("resource name = %q", resource.Name)
	}
	runDir := filepath.Join(stagingRoot, "plugin_exchange", commandIntegrationPluginName, produce.ID)
	if _, err := os.Stat(filepath.Join(runDir, "discard.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("discarded source remains: %v", err)
	}
	if _, err := os.Stat(filepath.Join(runDir, "import.bin")); err == nil {
		if !strings.Contains(mapped.Error, "imported-pending-delete") {
			t.Fatalf("retained imported source was not marked pending delete: %+v", mapped)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("inspect imported source: %v", err)
	}

	// Occupy both real import workers, then use the real command completion Lua
	// callback to admit restart.bin to the dispatcher's private queue. Shutdown
	// interrupts that already-admitted pending claim before it can start.
	plugins := tc.AppCtx.PluginManager().Plugins()
	if len(plugins) != 1 {
		t.Fatalf("enabled plugins = %+v", plugins)
	}
	entered = resourceFS.arm(2)
	createImportBlocker(t, tc.AppCtx, stagingRoot, "blocker-one", actor.ID, plugins[0].Generation)
	createImportBlocker(t, tc.AppCtx, stagingRoot, "blocker-two", actor.ID, plugins[0].Generation)
	waitForResourceCreates(t, entered, 2)
	restartStart := doReq(tc, http.MethodGet, "/plugins/command-integration/start-restart", map[string]string{"Accept": "text/html", "Authorization": bearer}, nil, nil)
	if restartStart.Code != http.StatusOK {
		t.Fatalf("restart start = %d %s", restartStart.Code, restartStart.Body.String())
	}
	restartRun := waitForCommandRun(t, tc.AppCtx, "restart", plugin_commands.RunStatusSucceeded)
	interrupted := waitForImport(t, tc.AppCtx, restartRun.ID, "restart.bin", plugin_commands.ImportStatusPending)
	waitForPageContains(t, tc, bearer, "/plugins/command-integration/status", `"restart_queued":true`)

	stopped := make(chan error, 1)
	go func() { stopped <- tc.AppCtx.StopPluginCommands() }()
	interrupted = waitForImport(t, tc.AppCtx, restartRun.ID, "restart.bin", plugin_commands.ImportStatusInterrupted)
	resourceFS.release()
	if err := <-stopped; err != nil {
		t.Fatal(err)
	}
	commandsStarted = false
	oldImportID := interrupted.ImportID
	closeCurrent()

	// Reopen the SQLite file and resource filesystem with a newly constructed
	// context, download manager, dispatcher, plugin manager, and Lua VM. Startup
	// recovery is published before the persisted enabled plugin is activated.
	tc, closeCurrent = openPersistentCommandTestContext(t, databasePath, pluginDir, resourceFS)
	if err := tc.AppCtx.StartPluginCommands(context.Background(), settings); err != nil {
		t.Fatal(err)
	}
	commandsStarted = true
	if _, err := tc.AppCtx.EnsurePluginStates(); err != nil {
		t.Fatal(err)
	}
	tc.AppCtx.ActivateEnabledPlugins()
	if tc.AppCtx.PluginManager() == nil || !tc.AppCtx.PluginManager().IsEnabled(commandIntegrationPluginName) {
		t.Fatal("persisted plugin did not reload after process reconstruction")
	}
	redrive := doReq(tc, http.MethodGet, "/plugins/command-integration/redrive", map[string]string{"Accept": "text/html", "Authorization": bearer}, nil, nil)
	wantRedrive := "interrupted:" + oldImportID + ":" + oldImportID
	if redrive.Code != http.StatusOK || !strings.Contains(redrive.Body.String(), wantRedrive) {
		t.Fatalf("redrive page = %d %s, want %q", redrive.Code, redrive.Body.String(), wantRedrive)
	}
	redriven := waitForImport(t, tc.AppCtx, restartRun.ID, "restart.bin", plugin_commands.ImportStatusSucceeded)
	if redriven.ImportID != oldImportID || redriven.ResourceID == nil {
		t.Fatalf("redriven map = %+v", redriven)
	}

	// Disable revokes the VM before the verified process group is cancelled.
	// The callback's independent mah.log side effect is the oracle: unlike an
	// exchange call, it cannot fail merely because command admission is closed.
	lossStart := doReq(tc, http.MethodGet, "/plugins/command-integration/start-loss", map[string]string{"Accept": "text/html", "Authorization": bearer}, nil, nil)
	if lossStart.Code != http.StatusOK {
		t.Fatalf("callback-loss start = %d %s", lossStart.Code, lossStart.Body.String())
	}
	lossRun := waitForCommandRun(t, tc.AppCtx, "wait", plugin_commands.RunStatusRunning)
	marker := filepath.Join(stagingRoot, "plugin_exchange", commandIntegrationPluginName, lossRun.ID, "started.marker")
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(marker); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("helper did not publish start marker %s", marker)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := tc.AppCtx.SetPluginEnabled(commandIntegrationPluginName, false); err != nil {
		t.Fatal(err)
	}
	lossRun = waitForCommandRun(t, tc.AppCtx, "wait", plugin_commands.RunStatusCancelled)
	// Dispatcher cancellation returns only after terminal publication has queued
	// the completion. Give that goroutine a scheduling window; if the revoked
	// callback body is entered, mah.log is an immediate durable write.
	time.Sleep(250 * time.Millisecond)
	var callbackLogs int64
	if err := tc.DB.Model(&models.LogEntry{}).Where("message = ?", "forbidden-command-callback:"+lossRun.ID).Count(&callbackLogs).Error; err != nil {
		t.Fatal(err)
	}
	if callbackLogs != 0 {
		t.Fatalf("revoked command callback executed %d time(s)", callbackLogs)
	}

	history := doReq(tc, http.MethodGet, "/v1/plugin/command-runs", map[string]string{"Accept": "application/json", "Authorization": bearer}, nil, nil)
	if history.Code != http.StatusOK || !strings.Contains(history.Body.String(), lossRun.ID) || !strings.Contains(history.Body.String(), plugin_commands.RunStatusCancelled) {
		t.Fatalf("durable admin history = %d %s", history.Code, history.Body.String())
	}
	detail := doReq(tc, http.MethodGet, "/v1/plugin/command-run?id="+lossRun.ID, map[string]string{"Accept": "application/json", "Authorization": bearer}, nil, nil)
	if detail.Code != http.StatusOK || !strings.Contains(detail.Body.String(), lossRun.ID) || !strings.Contains(detail.Body.String(), plugin_commands.RunStatusCancelled) {
		t.Fatalf("durable admin detail = %d %s", detail.Code, detail.Body.String())
	}
	if err := tc.AppCtx.WithPrincipal(nil).SetPluginEnabledWithOptions(
		commandIntegrationPluginName, true, application_context.PluginEnableOptions{ConfirmCommands: true},
	); err != nil {
		t.Fatal(err)
	}
	lossStatus := doReq(tc, http.MethodGet, "/plugins/command-integration/loss-status", map[string]string{"Accept": "text/html", "Authorization": bearer}, nil, nil)
	wantLoss := lossRun.ID + ":" + plugin_commands.RunStatusCancelled
	if lossStatus.Code != http.StatusOK || !strings.Contains(lossStatus.Body.String(), wantLoss) {
		t.Fatalf("mah.fs.runs after re-enable = %d %s, want %q", lossStatus.Code, lossStatus.Body.String(), wantLoss)
	}
}
