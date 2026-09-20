//go:build !windows

package plugin_commands

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestPrepareUsesCachedGlobalUsageWithoutRescanning(t *testing.T) {
	root, commandDir := t.TempDir(), t.TempDir()
	var calls atomic.Int32
	usage := &StagingUsageCache{measure: func(path string) (int64, error) {
		calls.Add(1)
		return pathUsageNoSymlinks(path)
	}}
	if err := usage.Refresh(root); err != nil {
		t.Fatal(err)
	}
	settings := runnerTestSettings{root: root, commandDir: commandDir, perRun: 1024, global: 1024}
	executor := NewExecutor(RunnerDependencies{Store: newRunnerTestStore(), Settings: settings, Usage: usage})
	runID := strings.Repeat("c", 32)
	run := QueuedRun{RunID: runID, ExchangeDir: filepath.Join(root, "plugin_exchange", "plug", runID), Request: CommandRequest{PluginName: "plug"}}

	if err := executor.(interface{ Prepare(QueuedRun) error }).Prepare(run); err != nil {
		t.Fatal(err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("usage measurements = %d, want startup sample only", got)
	}
}

func TestQuotaGlobalAdmissionLeavesNoRunOrFolder(t *testing.T) {
	root, commandDir := t.TempDir(), t.TempDir()
	helperExecutable(t, commandDir, "mah-helper")
	if err := os.WriteFile(filepath.Join(root, "already-full"), []byte(strings.Repeat("x", 33)), 0o600); err != nil {
		t.Fatal(err)
	}
	store := newRunnerTestStore()
	settings := runnerTestSettings{root: root, commandDir: commandDir, perRun: 1024, global: 32}
	executor := NewExecutor(RunnerDependencies{Store: store, Settings: settings})
	jobs := &dispatcherTestJobs{}
	dispatcher := NewDispatcher(Dependencies{Store: store, Jobs: jobs, Executor: executor, Settings: settings})
	if err := dispatcher.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = dispatcher.Stop(ctx)
	})
	_, err := dispatcher.Submit(CommandRequest{PluginName: "plug", Declaration: Declaration{Name: "run", Argv: []string{"mah-helper"}, Timeout: time.Second}})
	if err == nil || !strings.Contains(err.Error(), "global staging quota") {
		t.Fatalf("Submit error = %v", err)
	}
	if len(store.runs) != 0 {
		t.Fatalf("quota-refused run was persisted: %#v", store.runs)
	}
	entries, err := os.ReadDir(filepath.Join(root, "plugin_exchange", "plug"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("quota-refused folder remains: %#v", entries)
	}
}

func TestQuotaGlobalLimitDoesNotBlockImportDrain(t *testing.T) {
	root, commandDir := t.TempDir(), t.TempDir()
	helperExecutable(t, commandDir, "mah-helper")
	if err := os.WriteFile(filepath.Join(root, "already-full"), []byte(strings.Repeat("x", 33)), 0o600); err != nil {
		t.Fatal(err)
	}
	store := newRunnerTestStore()
	settings := runnerTestSettings{root: root, commandDir: commandDir, perRun: 1024, global: 32}
	executor := NewExecutor(RunnerDependencies{Store: store, Settings: settings})
	jobs := &dispatcherTestJobs{}
	dispatcher := NewDispatcher(Dependencies{Store: store, Jobs: jobs, Executor: executor, Settings: settings})
	if err := dispatcher.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = dispatcher.Stop(ctx)
	})
	if err := dispatcher.submitImport(ImportJobSpec{ImportID: "import-1", RunID: "run-import-1", PluginName: "plug"}, func(context.Context, Progress) Outcome {
		return Outcome{Status: ImportStatusSucceeded}
	}); err != nil {
		t.Fatalf("import drain was blocked by global command quota: %v", err)
	}
}

func TestQuotaPerRunIncludesExchangeAndImportTemps(t *testing.T) {
	root, commandDir := t.TempDir(), t.TempDir()
	helperExecutable(t, commandDir, "mah-helper")
	store := newRunnerTestStore()
	settings := runnerTestSettings{root: root, commandDir: commandDir, perRun: 64, global: 1 << 20}
	executor := NewExecutor(RunnerDependencies{Store: store, Settings: settings})
	run := seedRunnerRun(t, executor, store, settings, "quota", []string{"mah-helper", helperProcessFlag, "write", "{{exchange_dir}}", "40"}, 5*time.Second)
	importID := "import-for-run"
	store.imports = []ImportRecord{{ID: importID, RunID: run.RunID, Status: ImportStatusRunning}}
	importDir := filepath.Join(root, "import_tmp", importID)
	if err := os.MkdirAll(importDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(importDir, "scratch"), []byte(strings.Repeat("i", 40)), 0o600); err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	outcome := executor.Execute(context.Background(), run)
	if outcome.Status != RunStatusFailed || !strings.Contains(outcome.Error, "per-run quota") {
		t.Fatalf("outcome = %+v", outcome)
	}
	if elapsed := time.Since(started); elapsed < 800*time.Millisecond {
		t.Fatalf("sampled quota killed before the one-second sample: %s", elapsed)
	}
	payload, err := os.Stat(filepath.Join(run.ExchangeDir, "payload.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if payload.Size() != 40 {
		t.Fatalf("fast writer payload = %d; want 40-byte overshoot before sampling", payload.Size())
	}
}

func TestQuotaFinalSampleRejectsAFastSuccessfulWriter(t *testing.T) {
	root, commandDir := t.TempDir(), t.TempDir()
	helperExecutable(t, commandDir, "mah-helper")
	store := newRunnerTestStore()
	settings := runnerTestSettings{root: root, commandDir: commandDir, perRun: 32, global: 1 << 20}
	executor := NewExecutor(RunnerDependencies{Store: store, Settings: settings})
	executor.(*commandExecutor).quotaInterval = time.Hour
	run := seedRunnerRun(t, executor, store, settings, "fast-quota", []string{"mah-helper", helperProcessFlag, "write-exit", "{{exchange_dir}}", "64"}, 5*time.Second)

	outcome := executor.Execute(context.Background(), run)
	if outcome.Status != RunStatusFailed || !strings.Contains(outcome.Error, "per-run quota") {
		t.Fatalf("outcome = %+v", outcome)
	}
}

func TestUsageRejectsASymlinkedRoot(t *testing.T) {
	realRoot := t.TempDir()
	link := filepath.Join(t.TempDir(), "staging")
	if err := os.Symlink(realRoot, link); err != nil {
		t.Fatal(err)
	}
	if _, err := pathUsageNoSymlinks(link); err == nil || !strings.Contains(err.Error(), "root is a symlink") {
		t.Fatalf("pathUsageNoSymlinks error = %v", err)
	}

	settings := runnerTestSettings{root: link, commandDir: t.TempDir(), perRun: 1024, global: 1024}
	executor := NewExecutor(RunnerDependencies{Store: newRunnerTestStore(), Settings: settings})
	runID := strings.Repeat("a", 32)
	run := QueuedRun{
		RunID:       runID,
		ExchangeDir: filepath.Join(link, "plugin_exchange", "plug", runID),
		Request:     CommandRequest{PluginName: "plug"},
	}
	if err := executor.(interface{ Prepare(QueuedRun) error }).Prepare(run); err == nil || !strings.Contains(err.Error(), "root is a symlink") {
		t.Fatalf("Prepare error = %v", err)
	}
}

func TestUsageDoesNotFollowSymlinks(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(outside, []byte(strings.Repeat("x", 4096)), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "inside"), []byte("12345"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := pathUsageNoSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if got != 5 {
		t.Fatalf("usage = %d; want 5", got)
	}
}
