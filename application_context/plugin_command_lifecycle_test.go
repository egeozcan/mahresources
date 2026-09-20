package application_context

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"
	"mahresources/models"
	"mahresources/plugin_commands"
)

func TestPluginCommandConfigDefaultsAndValidation(t *testing.T) {
	commandDir := t.TempDir()
	files := t.TempDir()

	got, err := ResolvePluginCommandConfig(PluginCommandConfigInput{
		InheritedPath: commandDir,
		FileSavePath:  files,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.CommandPath != commandDir {
		t.Fatalf("command path = %q, want inherited %q", got.CommandPath, commandDir)
	}
	wantStaging := filepath.Join(files, "_plugin_commands")
	if got.StagingPath != wantStaging {
		t.Fatalf("staging path = %q, want %q", got.StagingPath, wantStaging)
	}
	if got.RunQuota != DefaultPluginCommandRunQuota || got.StagingQuota != DefaultPluginCommandStagingQuota {
		t.Fatalf("quota defaults = (%d,%d)", got.RunQuota, got.StagingQuota)
	}
	if got.ExchangeRetention != DefaultPluginCommandExchangeRetention || got.OutputRetention != DefaultPluginCommandOutputRetention {
		t.Fatalf("retention defaults = (%s,%s)", got.ExchangeRetention, got.OutputRetention)
	}
	info, err := os.Stat(got.StagingPath)
	if err != nil || !info.IsDir() {
		t.Fatalf("staging root not created as directory: info=%v err=%v", info, err)
	}
}

func TestPluginCommandConfigResolvesRelativeStagingRoots(t *testing.T) {
	workDir := t.TempDir()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(workDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })
	commandDir := t.TempDir()
	absoluteWorkDir, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}

	fromFiles, err := ResolvePluginCommandConfig(PluginCommandConfigInput{
		InheritedPath: commandDir,
		FileSavePath:  "relative-files",
	})
	if err != nil {
		t.Fatal(err)
	}
	wantFiles := filepath.Join(absoluteWorkDir, "relative-files", "_plugin_commands")
	if fromFiles.StagingPath != wantFiles || !filepath.IsAbs(fromFiles.StagingPath) {
		t.Fatalf("relative file-save staging = %q, want absolute %q", fromFiles.StagingPath, wantFiles)
	}

	explicit, err := ResolvePluginCommandConfig(PluginCommandConfigInput{
		InheritedPath:       commandDir,
		StagingPath:         "relative-staging",
		StagingPathExplicit: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	wantExplicit := filepath.Join(absoluteWorkDir, "relative-staging")
	if explicit.StagingPath != wantExplicit || !filepath.IsAbs(explicit.StagingPath) {
		t.Fatalf("explicit relative staging = %q, want absolute %q", explicit.StagingPath, wantExplicit)
	}
}

func TestPluginCommandConfigNormalizesTrustedPathEntries(t *testing.T) {
	first := t.TempDir()
	second := t.TempDir()
	raw := first + string(os.PathSeparator) + string(os.PathSeparator) + string(os.PathListSeparator) + filepath.Join(second, ".", "") + string(os.PathSeparator)
	got, err := ResolvePluginCommandConfig(PluginCommandConfigInput{
		CommandPath: raw, CommandPathExplicit: true,
		StagingPath: t.TempDir(), StagingPathExplicit: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Clean(first) + string(os.PathListSeparator) + filepath.Clean(second)
	if got.CommandPath != want {
		t.Fatalf("normalized command path = %q, want %q", got.CommandPath, want)
	}
}

func TestPluginCommandConfigExplicitPathAndEphemeralRoot(t *testing.T) {
	trusted := t.TempDir()
	rogue := t.TempDir()
	got, err := ResolvePluginCommandConfig(PluginCommandConfigInput{
		CommandPath: trusted, CommandPathExplicit: true,
		InheritedPath: rogue,
		MemoryFS:      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.CommandPath != trusted {
		t.Fatalf("command path = %q, want explicit trusted path %q", got.CommandPath, trusted)
	}
	if !got.TemporaryStaging || got.StagingPath == "" || !filepath.IsAbs(got.StagingPath) {
		t.Fatalf("ephemeral staging = %+v", got)
	}
	if filepath.Clean(got.StagingPath) == filepath.Clean(rogue) {
		t.Fatal("ephemeral staging reused inherited command path")
	}
	t.Cleanup(func() { _ = os.RemoveAll(got.StagingPath) })
}

func TestPluginCommandConfigRejectsUnsafeValues(t *testing.T) {
	commandDir := t.TempDir()
	file := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		in   PluginCommandConfigInput
	}{
		{name: "explicit empty command path", in: PluginCommandConfigInput{CommandPathExplicit: true, CommandPath: "", StagingPath: t.TempDir(), StagingPathExplicit: true}},
		{name: "empty path element", in: PluginCommandConfigInput{CommandPathExplicit: true, CommandPath: commandDir + string(os.PathListSeparator), StagingPath: t.TempDir(), StagingPathExplicit: true}},
		{name: "relative command directory", in: PluginCommandConfigInput{CommandPathExplicit: true, CommandPath: "relative", StagingPath: t.TempDir(), StagingPathExplicit: true}},
		{name: "command path file", in: PluginCommandConfigInput{CommandPathExplicit: true, CommandPath: file, StagingPath: t.TempDir(), StagingPathExplicit: true}},
		{name: "staging path file", in: PluginCommandConfigInput{CommandPath: commandDir, CommandPathExplicit: true, StagingPath: file, StagingPathExplicit: true}},
		{name: "zero run quota", in: PluginCommandConfigInput{CommandPath: commandDir, CommandPathExplicit: true, StagingPath: t.TempDir(), StagingPathExplicit: true, RunQuota: 0, RunQuotaSet: true}},
		{name: "zero staging quota", in: PluginCommandConfigInput{CommandPath: commandDir, CommandPathExplicit: true, StagingPath: t.TempDir(), StagingPathExplicit: true, StagingQuota: 0, StagingQuotaSet: true}},
		{name: "zero exchange retention", in: PluginCommandConfigInput{CommandPath: commandDir, CommandPathExplicit: true, StagingPath: t.TempDir(), StagingPathExplicit: true, ExchangeRetention: 0, ExchangeRetentionSet: true}},
		{name: "zero output retention", in: PluginCommandConfigInput{CommandPath: commandDir, CommandPathExplicit: true, StagingPath: t.TempDir(), StagingPathExplicit: true, OutputRetention: 0, OutputRetentionSet: true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ResolvePluginCommandConfig(tc.in); err == nil {
				t.Fatal("expected refusal")
			}
		})
	}
}

func TestPluginCommandLifecycleLeaseRefusesBeforeRecoveryMutation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("plugin commands are unsupported on Windows")
	}
	ctx := newPluginCommandStoreTestContext(t)
	root := t.TempDir()
	settings := testPluginCommandSettings{root: root, commandPath: t.TempDir()}
	owner := uint(7)
	now := time.Now().UTC()
	if err := ctx.CreateRun(plugin_commands.RunRecord{
		ID: "lease-guarded", PluginName: "lifecycle", CommandName: "command", ParamsJSON: `{}`,
		Status: plugin_commands.RunStatusQueued, CreatedByUserID: &owner, CreatedAt: now,
	}, plugin_commands.RunOutput{RunID: "lease-guarded", ArgvJSON: `[]`, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	orphan := filepath.Join(root, "import_tmp", "orphan", "source")
	if err := os.MkdirAll(filepath.Dir(orphan), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(orphan, []byte("still-owned"), 0o600); err != nil {
		t.Fatal(err)
	}
	lease, err := plugin_commands.AcquireRuntimeLease(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := ctx.StartPluginCommands(context.Background(), settings); err == nil || !strings.Contains(err.Error(), "active runtime") {
		_ = lease.Close()
		t.Fatalf("second runtime startup = %v", err)
	}
	run, _, err := ctx.Run("lease-guarded")
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != plugin_commands.RunStatusQueued {
		t.Fatalf("refused startup mutated run = %+v", run)
	}
	if body, err := os.ReadFile(orphan); err != nil || string(body) != "still-owned" {
		t.Fatalf("refused startup mutated recovery files: body=%q err=%v", body, err)
	}
	if err := lease.Close(); err != nil {
		t.Fatal(err)
	}
	if err := ctx.StartPluginCommands(context.Background(), settings); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ctx.StopPluginCommands() })
}

func TestPluginCommandLifecycleRecoveryFailureReleasesLease(t *testing.T) {
	ctx := newPluginCommandStoreTestContext(t)
	root := t.TempDir()
	if err := ctx.db.Migrator().DropTable(&models.PluginCommandImport{}); err != nil {
		t.Fatal(err)
	}
	if err := ctx.StartPluginCommands(context.Background(), testPluginCommandSettings{
		root: root, commandPath: t.TempDir(),
	}); err == nil {
		t.Fatal("startup unexpectedly survived recovery-store failure")
	}
	lease, err := plugin_commands.AcquireRuntimeLease(root)
	if err != nil {
		t.Fatalf("recovery failure retained runtime lease: %v", err)
	}
	_ = lease.Close()
}

func TestPluginCommandLifecycleRecoversBeforePublishingHost(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("plugin commands are unsupported on Windows")
	}
	ctx := newPluginCommandStoreTestContext(t)
	pluginDir := t.TempDir()
	ctx.Config.PluginPath = pluginDir
	ctx.Config.PluginsDisabled = false
	// newPluginCommandStoreTestContext builds its manager before PluginPath is
	// replaced, so only the recovery/publication ordering is exercised here.
	if ctx.pluginManager == nil {
		t.Fatal("plugin manager unavailable")
	}

	now := time.Now().UTC().Add(-time.Hour)
	owner := uint(7)
	if err := ctx.CreateRun(plugin_commands.RunRecord{
		ID: "lifecycle-run", PluginName: "lifecycle", CommandName: "command",
		ParamsJSON: `{}`, Status: plugin_commands.RunStatusQueued,
		CreatedByUserID: &owner, CreatedAt: now,
	}, plugin_commands.RunOutput{RunID: "lifecycle-run", ArgvJSON: `[]`, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	partial := filepath.Join(root, "import_tmp", "orphan", "upload-partial")
	if err := os.MkdirAll(filepath.Dir(partial), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(partial, []byte("partial"), 0o600); err != nil {
		t.Fatal(err)
	}
	settings := testPluginCommandSettings{root: root, commandPath: t.TempDir()}
	recoveryBlocked := make(chan struct{})
	releaseRecovery := make(chan struct{})
	var blockOnce sync.Once
	const callbackName = "test:plugin-command-recovery-publication"
	if err := ctx.db.Callback().Update().Before("gorm:update").Register(callbackName, func(db *gorm.DB) {
		if db.Statement.Table == "plugin_command_runs" {
			blockOnce.Do(func() {
				close(recoveryBlocked)
				<-releaseRecovery
			})
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ctx.db.Callback().Update().Remove(callbackName) })
	started := make(chan error, 1)
	go func() { started <- ctx.StartPluginCommands(context.Background(), settings) }()
	select {
	case <-recoveryBlocked:
	case <-time.After(time.Second):
		close(releaseRecovery)
		t.Fatal("recovery did not reach durable publication barrier")
	}
	if _, err := ctx.SubmitPluginCommand(plugin_commands.CommandRequest{}); err == nil || !strings.Contains(err.Error(), "startup recovery completes") {
		close(releaseRecovery)
		t.Fatalf("command host was published during recovery: %v", err)
	}
	close(releaseRecovery)
	if err := <-started; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ctx.StopPluginCommands() })

	run, _, err := ctx.Run("lifecycle-run")
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != plugin_commands.RunStatusInterrupted {
		t.Fatalf("recovered status = %q", run.Status)
	}
	if _, err := os.Stat(filepath.Join(root, "import_tmp", "orphan")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("orphan import temp remains: %v", err)
	}
	if ctx.pluginCommandDispatcher == nil || ctx.pluginCommandExchange == nil {
		t.Fatal("host published incompletely")
	}
}

type lifecycleBlockingImporter struct{ started chan string }

func (i lifecycleBlockingImporter) ValidateImport(plugin_commands.ImportValidation) error { return nil }
func (i lifecycleBlockingImporter) ImportResource(ctx context.Context, source plugin_commands.ImportSource, _ plugin_commands.ResourceFields, _ string) (uint, error) {
	i.started <- source.ImportID
	<-ctx.Done()
	return 0, ctx.Err()
}

func TestPluginCommandLifecycleShutdownPersistsOutcome(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("plugin commands are unsupported on Windows")
	}
	ctx := newPluginCommandStoreTestContext(t)
	if ctx.pluginManager == nil {
		t.Fatal("plugin manager unavailable")
	}
	commandDir := t.TempDir()
	commandPath := filepath.Join(commandDir, "slow-command")
	if err := os.WriteFile(commandPath, []byte("#!/bin/sh\nsleep 30\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	stagingRoot := t.TempDir()
	settings := testPluginCommandSettings{root: stagingRoot, commandPath: commandDir}
	if err := ctx.StartPluginCommands(context.Background(), settings); err != nil {
		t.Fatal(err)
	}
	owner := uint(11)
	runID, err := ctx.pluginCommandDispatcher.Submit(plugin_commands.CommandRequest{
		PluginName: "lifecycle", ActorUserID: &owner,
		Declaration: plugin_commands.Declaration{Name: "slow", Argv: []string{"slow-command"}, Timeout: time.Minute},
	})
	if err != nil {
		_ = ctx.StopPluginCommands()
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		run, _, readErr := ctx.Run(runID)
		if readErr != nil {
			_ = ctx.StopPluginCommands()
			t.Fatal(readErr)
		}
		if run.Status == plugin_commands.RunStatusRunning {
			break
		}
		if time.Now().After(deadline) {
			_ = ctx.StopPluginCommands()
			t.Fatalf("run never started: %+v", run)
		}
		time.Sleep(10 * time.Millisecond)
	}

	// A running import has its own pre-commit cancellation path and must also be
	// durably classified before Stop returns.
	importRunID := "lifecycle-import-run"
	now := time.Now().UTC()
	if err := ctx.CreateRun(plugin_commands.RunRecord{
		ID: importRunID, PluginName: "lifecycle", CommandName: "produce", ParamsJSON: `{}`,
		Status: plugin_commands.RunStatusQueued, CreatedByUserID: &owner, CreatedAt: now,
	}, plugin_commands.RunOutput{RunID: importRunID, ArgvJSON: `[]`, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if won, err := ctx.MarkRunRunning(importRunID, now); err != nil || !won {
		t.Fatalf("start import source run: won=%v err=%v", won, err)
	}
	if won, err := ctx.FinishRun(importRunID, plugin_commands.RunFinish{Status: plugin_commands.RunStatusSucceeded, FinishedAt: now}); err != nil || !won {
		t.Fatalf("finish import source run: won=%v err=%v", won, err)
	}
	runDir := filepath.Join(stagingRoot, "plugin_exchange", "lifecycle", importRunID)
	if err := os.MkdirAll(runDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "result.bin"), []byte("result"), 0o600); err != nil {
		t.Fatal(err)
	}
	importer := lifecycleBlockingImporter{started: make(chan string, 1)}
	ctx.pluginCommandDispatcher.SetImporter(importer)
	claim, err := ctx.pluginCommandDispatcher.SubmitImport(plugin_commands.ImportSubmission{
		Access: plugin_commands.Access{PluginName: "lifecycle", ActorUserID: &owner},
		RunID:  importRunID, Name: "result.bin", PluginGeneration: 1, ActorUserID: &owner,
		Fields: plugin_commands.ResourceFields{Name: "result"},
	})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-importer.started:
	case <-time.After(5 * time.Second):
		t.Fatal("import never started")
	}

	if err := ctx.StopPluginCommands(); err != nil {
		t.Fatal(err)
	}
	run, _, err := ctx.Run(runID)
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != plugin_commands.RunStatusInterrupted || run.FinishedAt == nil {
		t.Fatalf("shutdown outcome = %+v", run)
	}
	mapped, found, err := ctx.ImportMap(importRunID, "result.bin")
	if err != nil || !found {
		t.Fatalf("shutdown import map: found=%v err=%v", found, err)
	}
	if mapped.ImportID != claim.ImportID || mapped.Status != plugin_commands.ImportStatusInterrupted {
		t.Fatalf("shutdown import outcome = %+v, claim=%+v", mapped, claim)
	}
}

func TestPluginCommandLifecycleShutdownReturnsPersistenceError(t *testing.T) {
	ctx := newPluginCommandStoreTestContext(t)
	if ctx.pluginManager == nil {
		t.Fatal("plugin manager unavailable")
	}
	root := t.TempDir()
	if err := ctx.StartPluginCommands(context.Background(), testPluginCommandSettings{
		root: root, commandPath: t.TempDir(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := ctx.db.Migrator().DropTable(&models.PluginCommandImport{}); err != nil {
		t.Fatal(err)
	}
	if err := ctx.StopPluginCommands(); err == nil {
		t.Fatal("shutdown persistence failure was swallowed")
	}
	lease, err := plugin_commands.AcquireRuntimeLease(root)
	if err != nil {
		t.Fatalf("shutdown error retained runtime lease: %v", err)
	}
	_ = lease.Close()
}

func TestPluginCommandLifecycleDisabledLeavesHostUnavailable(t *testing.T) {
	ctx := newPluginCommandStoreTestContext(t)
	ctx.Config.PluginsDisabled = true
	if ctx.pluginManager == nil {
		t.Fatal("plugin manager unavailable")
	}
	if err := ctx.StartPluginCommandsIfEnabled(context.Background(), testPluginCommandSettings{
		root: t.TempDir(), commandPath: t.TempDir(),
	}); err != nil {
		t.Fatal(err)
	}
	if ctx.pluginCommandDispatcher != nil || ctx.pluginCommandExchange != nil {
		t.Fatal("disabled context unexpectedly has plugin command host")
	}
	_, err := ctx.SubmitPluginCommand(plugin_commands.CommandRequest{})
	if err == nil {
		t.Fatal("disabled command API unexpectedly available")
	}
}

type testPluginCommandSettings struct {
	root        string
	commandPath string
}

func (s testPluginCommandSettings) StagingRoot() string        { return s.root }
func (s testPluginCommandSettings) PendingPerPluginLimit() int { return 100 }
func (s testPluginCommandSettings) PerRunQuota() int64         { return DefaultPluginCommandRunQuota }
func (s testPluginCommandSettings) GlobalStagingQuota() int64 {
	return DefaultPluginCommandStagingQuota
}
func (s testPluginCommandSettings) ExchangeRetention() time.Duration {
	return DefaultPluginCommandExchangeRetention
}
func (s testPluginCommandSettings) OutputRetention() time.Duration {
	return DefaultPluginCommandOutputRetention
}
func (s testPluginCommandSettings) CommandPath() string { return s.commandPath }

func TestPluginCommandLifecycleOutputPruneKeepsDurableRows(t *testing.T) {
	ctx := newPluginCommandStoreTestContext(t)
	now := time.Now().UTC()
	finished := now.Add(-2 * time.Hour)
	owner := uint(9)
	if err := ctx.CreateRun(plugin_commands.RunRecord{
		ID: "prune-run", PluginName: "lifecycle", CommandName: "command", ParamsJSON: `{}`,
		Status: plugin_commands.RunStatusQueued, CreatedByUserID: &owner, CreatedAt: finished,
	}, plugin_commands.RunOutput{RunID: "prune-run", ArgvJSON: `[]`, OutputTail: "tail", CreatedAt: finished}); err != nil {
		t.Fatal(err)
	}
	if won, err := ctx.MarkRunRunning("prune-run", finished); err != nil || !won {
		t.Fatalf("start run: won=%v err=%v", won, err)
	}
	if won, err := ctx.FinishRun("prune-run", plugin_commands.RunFinish{Status: plugin_commands.RunStatusSucceeded, FinishedAt: finished}); err != nil || !won {
		t.Fatalf("finish run: won=%v err=%v", won, err)
	}
	if err := ctx.db.Create(&models.PluginCommandImportMap{RunID: "prune-run", FileName: "result.bin", ImportID: "prune-import", Status: plugin_commands.ImportStatusSucceeded}).Error; err != nil {
		t.Fatal(err)
	}
	if n, err := ctx.PruneRunOutputs(now.Add(-time.Hour)); err != nil || n != 1 {
		t.Fatalf("prune outputs: n=%d err=%v", n, err)
	}
	var runCount, mapCount int64
	if err := ctx.db.Model(&models.PluginCommandRun{}).Where("id = ?", "prune-run").Count(&runCount).Error; err != nil {
		t.Fatal(err)
	}
	if err := ctx.db.Model(&models.PluginCommandImportMap{}).Where("run_id = ?", "prune-run").Count(&mapCount).Error; err != nil {
		t.Fatal(err)
	}
	if runCount != 1 || mapCount != 1 {
		t.Fatalf("durable rows after prune: runs=%d maps=%d", runCount, mapCount)
	}
}
