package application_context

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

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
	if err := ctx.StartPluginCommands(context.Background(), settings); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ctx.StopPluginCommands)

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
	settings := testPluginCommandSettings{root: t.TempDir(), commandPath: commandDir}
	if err := ctx.StartPluginCommands(context.Background(), settings); err != nil {
		t.Fatal(err)
	}
	owner := uint(11)
	runID, err := ctx.pluginCommandDispatcher.Submit(plugin_commands.CommandRequest{
		PluginName: "lifecycle", ActorUserID: &owner,
		Declaration: plugin_commands.Declaration{Name: "slow", Argv: []string{"slow-command"}, Timeout: time.Minute},
	})
	if err != nil {
		ctx.StopPluginCommands()
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		run, _, readErr := ctx.Run(runID)
		if readErr != nil {
			ctx.StopPluginCommands()
			t.Fatal(readErr)
		}
		if run.Status == plugin_commands.RunStatusRunning {
			break
		}
		if time.Now().After(deadline) {
			ctx.StopPluginCommands()
			t.Fatalf("run never started: %+v", run)
		}
		time.Sleep(10 * time.Millisecond)
	}
	ctx.StopPluginCommands()
	run, _, err := ctx.Run(runID)
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != plugin_commands.RunStatusInterrupted || run.FinishedAt == nil {
		t.Fatalf("shutdown outcome = %+v", run)
	}
}

func TestPluginCommandLifecycleDisabledLeavesHostUnavailable(t *testing.T) {
	ctx := newPluginCommandStoreTestContext(t)
	ctx.Config.PluginsDisabled = true
	ctx.pluginManager = nil
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
