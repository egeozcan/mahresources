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
	"testing"
	"time"

	"mahresources/application_context"
	"mahresources/models"
	"mahresources/plugin_commands"
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
	case "wait":
		if err := os.WriteFile(filepath.Join(exchangeDir, "started.marker"), []byte("started"), 0o600); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(5)
		}
		select {}
	default:
		fmt.Fprintf(os.Stderr, "unknown fixture mode %q\n", mode)
		os.Exit(6)
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
    {name = "wait", argv = {%q, "-test.run=TestPluginCommandFixtureProcess", "--", "wait", "{{exchange_dir}}"}, timeout = 60},
  },
}

local completion_result = nil
local listing_result = nil
local import_result = nil
local redrive_before = nil
local redrive_id = nil

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
      redrive_before = redrive_before,
      redrive_id = redrive_id,
    })
  end)

  mah.page("redrive", function()
    local runs, runs_err = mah.fs.runs()
    assert(runs, runs_err)
    for _, run in ipairs(runs) do
      if run.id == "restart-run" then
        redrive_before = run.imports["restart.bin"].status
      end
    end
    local import_id, import_err = mah.fs.create_resource("restart-run", "restart.bin", {
      name = "restart redrive resource",
    }, function(imported)
      import_result = imported
    end)
    assert(import_id, import_err)
    redrive_id = import_id
    return redrive_before .. ":" .. import_id
  end)

  mah.page("start-loss", function()
    local run_id, err = mah.commands.run("wait", {}, function(result)
      -- If this callback runs after disable it removes the evidence. The test
      -- verifies the marker remains while the durable cancelled row survives.
      mah.fs.discard(result.run_id, "started.marker")
    end)
    assert(run_id, err)
    return "started:" .. run_id
  end)
end
`, commandIntegrationPluginName, executable, executable)
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
	executable := installCommandIntegrationExecutable(t, commandDir)
	writeCommandIntegrationPlugin(t, pluginDir, executable)

	tc := setupTestEnvWithConfig(t, func(config *application_context.MahresourcesConfig) {
		config.AuthEnabled = true
		config.SessionTTL = time.Hour
		config.PluginPath = pluginDir
	})
	if sqlDB, err := tc.DB.DB(); err == nil {
		sqlDB.SetMaxOpenConns(1)
	}
	if tc.AppCtx.PluginManager() == nil {
		t.Fatal("plugin manager is unavailable")
	}
	t.Cleanup(tc.AppCtx.PluginManager().Close)
	settings := commandIntegrationSettings{root: stagingRoot, commandPath: commandDir}
	if err := tc.AppCtx.StartPluginCommands(context.Background(), settings); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := tc.AppCtx.StopPluginCommands(); err != nil {
			t.Errorf("stop plugin commands: %v", err)
		}
	})
	if _, err := tc.AppCtx.EnsurePluginStates(); err != nil {
		t.Fatal(err)
	}
	actor, bearer := commandIntegrationBearer(t, tc)
	bound := tc.AppCtx.WithPrincipal(nil)
	if err := bound.SetPluginEnabledWithOptions(commandIntegrationPluginName, true, application_context.PluginEnableOptions{ConfirmCommands: true}); err != nil {
		t.Fatal(err)
	}

	start := doReq(tc, http.MethodGet, "/plugins/command-integration/start", map[string]string{"Accept": "text/html", "Authorization": bearer}, nil, nil)
	if start.Code != http.StatusOK || !strings.Contains(start.Body.String(), "started:") {
		t.Fatalf("start page = %d %s", start.Code, start.Body.String())
	}
	produce := waitForCommandRun(t, tc.AppCtx, "produce", plugin_commands.RunStatusSucceeded)
	mapped := waitForImport(t, tc.AppCtx, produce.ID, "import.bin", plugin_commands.ImportStatusSucceeded)
	if mapped.ResourceID == nil {
		t.Fatal("successful import has no resource id")
	}
	waitForPageContains(t, tc, bearer, "/plugins/command-integration/status",
		`"ok":true`, produce.ID, mapped.ImportID, `"resource_id":`)

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
		// Darwin has no unlink-by-open-descriptor primitive. The import remains
		// safely mapped and is marked for the retention sweep rather than deleting
		// a path that may have been replaced after validation.
		mapped = waitForImport(t, tc.AppCtx, produce.ID, "import.bin", plugin_commands.ImportStatusSucceeded)
		if !strings.Contains(mapped.Error, "imported-pending-delete") {
			t.Fatalf("retained imported source was not marked pending delete: %+v", mapped)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("inspect imported source: %v", err)
	}

	// Create the exact crash boundary: a terminal producer and durable pending
	// import whose queue admission has not happened. Stop interrupts the claim;
	// the rebuilt dispatcher uses the same database and staging root.
	now := time.Now().UTC()
	if err := tc.AppCtx.CreateRun(plugin_commands.RunRecord{
		ID: "restart-run", PluginName: commandIntegrationPluginName, CommandName: "produce",
		ParamsJSON: `{}`, Status: plugin_commands.RunStatusQueued, CreatedByUserID: &actor.ID, CreatedAt: now,
	}, plugin_commands.RunOutput{RunID: "restart-run", ArgvJSON: `[]`, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if won, err := tc.AppCtx.MarkRunRunning("restart-run", now); err != nil || !won {
		t.Fatalf("mark restart run running: won=%v err=%v", won, err)
	}
	if won, err := tc.AppCtx.FinishRun("restart-run", plugin_commands.RunFinish{Status: plugin_commands.RunStatusSucceeded, FinishedAt: now}); err != nil || !won {
		t.Fatalf("finish restart run: won=%v err=%v", won, err)
	}
	restartDir := filepath.Join(stagingRoot, "plugin_exchange", commandIntegrationPluginName, "restart-run")
	if err := os.MkdirAll(restartDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(restartDir, "restart.bin"), []byte("restart output"), 0o600); err != nil {
		t.Fatal(err)
	}
	plugins := tc.AppCtx.PluginManager().Plugins()
	if len(plugins) != 1 {
		t.Fatalf("enabled plugins = %+v", plugins)
	}
	claim, err := tc.AppCtx.ClaimImport(plugin_commands.ImportClaimRequest{
		ImportID: "restart-import", RunID: "restart-run", FileName: "restart.bin",
		PluginGeneration: plugins[0].Generation, CreatedByUserID: &actor.ID, CreatedAt: now,
	})
	if err != nil || !claim.Enqueue {
		t.Fatalf("claim restart import = %+v err=%v", claim, err)
	}
	if err := tc.AppCtx.StopPluginCommands(); err != nil {
		t.Fatal(err)
	}
	interrupted := waitForImport(t, tc.AppCtx, "restart-run", "restart.bin", plugin_commands.ImportStatusInterrupted)
	if interrupted.ImportID != "restart-import" {
		t.Fatalf("interrupted claim id = %q", interrupted.ImportID)
	}
	if err := tc.AppCtx.StartPluginCommands(context.Background(), settings); err != nil {
		t.Fatal(err)
	}
	redrive := doReq(tc, http.MethodGet, "/plugins/command-integration/redrive", map[string]string{"Accept": "text/html", "Authorization": bearer}, nil, nil)
	if redrive.Code != http.StatusOK || !strings.Contains(redrive.Body.String(), "interrupted:restart-import") {
		t.Fatalf("redrive page = %d %s", redrive.Code, redrive.Body.String())
	}
	redriven := waitForImport(t, tc.AppCtx, "restart-run", "restart.bin", plugin_commands.ImportStatusSucceeded)
	if redriven.ImportID != "restart-import" || redriven.ResourceID == nil {
		t.Fatalf("redriven map = %+v", redriven)
	}
	if _, err := os.Stat(filepath.Join(restartDir, "restart.bin")); err == nil {
		redriven = waitForImport(t, tc.AppCtx, "restart-run", "restart.bin", plugin_commands.ImportStatusSucceeded)
		if !strings.Contains(redriven.Error, "imported-pending-delete") {
			t.Fatalf("retained redriven source was not marked pending delete: %+v", redriven)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("inspect redriven source: %v", err)
	}

	// A helper blocks behind a marker so disable lands before terminal delivery.
	// Disable revokes the VM first, then cancels the verified process group. If
	// the at-most-once callback were replayed, it would delete this marker.
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
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("disabled VM callback ran or output disappeared: %v", err)
	}

	history := doReq(tc, http.MethodGet, "/v1/plugin/command-runs", map[string]string{"Accept": "application/json", "Authorization": bearer}, nil, nil)
	if history.Code != http.StatusOK || !strings.Contains(history.Body.String(), lossRun.ID) || !strings.Contains(history.Body.String(), plugin_commands.RunStatusCancelled) {
		t.Fatalf("durable admin history = %d %s", history.Code, history.Body.String())
	}
	detail := doReq(tc, http.MethodGet, "/v1/plugin/command-run?id="+lossRun.ID, map[string]string{"Accept": "application/json", "Authorization": bearer}, nil, nil)
	if detail.Code != http.StatusOK || !strings.Contains(detail.Body.String(), lossRun.ID) || !strings.Contains(detail.Body.String(), plugin_commands.RunStatusCancelled) {
		t.Fatalf("durable admin detail = %d %s", detail.Code, detail.Body.String())
	}
}
