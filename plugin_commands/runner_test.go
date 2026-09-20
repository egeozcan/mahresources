//go:build !windows

package plugin_commands

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

type runnerTestSettings struct {
	root       string
	commandDir string
	pending    int
	perRun     int64
	global     int64
}

func (s runnerTestSettings) StagingRoot() string { return s.root }
func (s runnerTestSettings) PendingPerPluginLimit() int {
	if s.pending <= 0 {
		return 100
	}
	return s.pending
}
func (s runnerTestSettings) PerRunQuota() int64               { return s.perRun }
func (s runnerTestSettings) GlobalStagingQuota() int64        { return s.global }
func (s runnerTestSettings) ExchangeRetention() time.Duration { return time.Hour }
func (s runnerTestSettings) OutputRetention() time.Duration   { return time.Hour }
func (s runnerTestSettings) CommandPath() string              { return s.commandDir }

type runnerTestStore struct {
	mu                     sync.Mutex
	runs                   map[string]RunRecord
	outputs                map[string]RunOutput
	bootSessionIDs         map[string]string
	imports                []ImportRecord
	setProcessGroupStarted chan struct{}
	setProcessGroupRelease <-chan struct{}
	beforeFinish           func(string, RunFinish)
	markRunErr             error
	finishErr              error
	runErr                 error
}

func newRunnerTestStore() *runnerTestStore {
	return &runnerTestStore{runs: make(map[string]RunRecord), outputs: make(map[string]RunOutput), bootSessionIDs: make(map[string]string)}
}

func (s *runnerTestStore) CreateRun(run RunRecord, output RunOutput) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.runs[run.ID]; exists {
		return errors.New("duplicate run")
	}
	s.runs[run.ID] = run
	s.outputs[run.ID] = output
	return nil
}
func (s *runnerTestStore) MarkRunRunning(id string, started time.Time) (bool, error) {
	if s.markRunErr != nil {
		return false, s.markRunErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	run, ok := s.runs[id]
	if !ok || run.Status != RunStatusQueued {
		return false, nil
	}
	run.Status, run.StartedAt = RunStatusRunning, &started
	s.runs[id] = run
	return true, nil
}
func (s *runnerTestStore) SetRunProcessGroup(id string, pgid int, bootSessionID string) error {
	if s.setProcessGroupStarted != nil {
		select {
		case s.setProcessGroupStarted <- struct{}{}:
		default:
		}
	}
	if s.setProcessGroupRelease != nil {
		<-s.setProcessGroupRelease
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	run, ok := s.runs[id]
	if !ok || run.Status != RunStatusRunning || run.ProcessGroupID != nil {
		return errors.New("invalid process group transition")
	}
	run.ProcessGroupID = &pgid
	s.runs[id] = run
	if s.bootSessionIDs == nil {
		s.bootSessionIDs = make(map[string]string)
	}
	s.bootSessionIDs[id] = bootSessionID
	return nil
}
func (s *runnerTestStore) RequestRunCancel(id, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	run, ok := s.runs[id]
	if !ok {
		return ErrRunNotFound
	}
	if RunStatusTerminal(run.Status) || run.CancelRequested {
		return ErrRunNotCancellable
	}
	run.CancelRequested, run.Error = true, reason
	s.runs[id] = run
	return nil
}
func (s *runnerTestStore) FinishRun(id string, finish RunFinish) (bool, error) {
	if s.beforeFinish != nil {
		s.beforeFinish(id, finish)
	}
	if s.finishErr != nil {
		return false, s.finishErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	run, ok := s.runs[id]
	if !ok || RunStatusTerminal(run.Status) {
		return false, nil
	}
	if run.Status != RunStatusRunning && finish.Status != RunStatusCancelled && finish.Status != RunStatusInterrupted {
		return false, nil
	}
	if (finish.Status == RunStatusSucceeded || finish.Status == RunStatusFailed) && run.CancelRequested {
		return false, nil
	}
	run.Status, run.Error, run.ExitCode = finish.Status, finish.Error, finish.ExitCode
	run.OutputUnverified, run.FinishedAt = finish.OutputUnverified, &finish.FinishedAt
	s.runs[id] = run
	output := s.outputs[id]
	output.OutputTail = finish.OutputTail
	s.outputs[id] = output
	return true, nil
}
func (s *runnerTestStore) Run(id string) (RunRecord, RunOutput, error) {
	if s.runErr != nil {
		return RunRecord{}, RunOutput{}, s.runErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	run, ok := s.runs[id]
	if !ok {
		return RunRecord{}, RunOutput{}, ErrRunNotFound
	}
	return run, s.outputs[id], nil
}
func (s *runnerTestStore) Runs(Access) ([]RunView, error) { return nil, nil }
func (s *runnerTestStore) NonterminalRuns() ([]RecoveryRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var runs []RecoveryRun
	for _, run := range s.runs {
		if !RunStatusTerminal(run.Status) {
			runs = append(runs, RecoveryRun{RunRecord: run, BootSessionID: s.bootSessionIDs[run.ID]})
		}
	}
	return runs, nil
}
func (s *runnerTestStore) ExpiredTerminalRunBoundary(time.Time) (*RetentionCursor, error) {
	return nil, nil
}
func (s *runnerTestStore) ExpiredTerminalRuns(time.Time, *RetentionCursor, *RetentionCursor, int) ([]RunRecord, error) {
	return nil, nil
}
func (s *runnerTestStore) MarkRunExchangeRemoved(string, time.Time) error { return nil }
func (s *runnerTestStore) PruneRunOutputs(time.Time) (int64, error)       { return 0, nil }
func (s *runnerTestStore) ImportMap(string, string) (ImportMapEntry, bool, error) {
	return ImportMapEntry{}, false, nil
}
func (s *runnerTestStore) ClaimImport(ImportClaimRequest) (ImportClaimResult, error) {
	return ImportClaimResult{}, nil
}
func (s *runnerTestStore) MarkImportRunning(string, time.Time) (bool, error) { return true, nil }
func (s *runnerTestStore) FinishImport(string, ImportFinish) (bool, error)   { return true, nil }
func (s *runnerTestStore) SetImportSourceDeletePending(string, bool) error   { return nil }
func (s *runnerTestStore) InterruptNonterminalImports(time.Time) error       { return nil }
func (s *runnerTestStore) NonterminalImports() ([]ImportRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]ImportRecord(nil), s.imports...), nil
}
func (s *runnerTestStore) HasNonterminalImports(string) (bool, error) { return false, nil }

type recordingCommandProgress struct {
	statuses []string
}

func (*recordingCommandProgress) SetPhase(string)               {}
func (*recordingCommandProgress) SetPhaseProgress(int64, int64) {}
func (p *recordingCommandProgress) SetAuthoritativeStatus(status string) {
	p.statuses = append(p.statuses, status)
}

func TestRunnerPublishesRunningOnlyAfterTheDurableTransition(t *testing.T) {
	settings := runnerTestSettings{root: t.TempDir(), commandDir: t.TempDir()}
	newRun := func(id string, store *runnerTestStore, progress Progress) QueuedRun {
		store.runs[id] = RunRecord{ID: id, Status: RunStatusQueued}
		store.outputs[id] = RunOutput{RunID: id}
		return QueuedRun{
			RunID:       id,
			Request:     CommandRequest{PluginName: "plug", Declaration: Declaration{Timeout: time.Second}},
			ExchangeDir: filepath.Join(settings.root, "plugin_exchange", "plug", id),
			Invocation:  Invocation{Argv: []string{"missing-command"}},
			progress:    progress,
		}
	}

	t.Run("failed durable start stays unpublished", func(t *testing.T) {
		store := newRunnerTestStore()
		store.markRunErr = errors.New("database unavailable")
		progress := &recordingCommandProgress{}
		executor := NewExecutor(RunnerDependencies{Store: store, Settings: settings})
		outcome := executor.Execute(context.Background(), newRun("mark-failed", store, progress))
		if !strings.Contains(outcome.Error, "mark command running") {
			t.Fatalf("outcome = %+v", outcome)
		}
		if outcome.AuthoritativeStatus != "" {
			t.Fatalf("authoritative status = %q, want none", outcome.AuthoritativeStatus)
		}
		if len(progress.statuses) != 0 {
			t.Fatalf("published statuses = %v, want none", progress.statuses)
		}
	})

	t.Run("successful durable start publishes running", func(t *testing.T) {
		store := newRunnerTestStore()
		progress := &recordingCommandProgress{}
		executor := NewExecutor(RunnerDependencies{Store: store, Settings: settings})
		outcome := executor.Execute(context.Background(), newRun("mark-succeeded", store, progress))
		if outcome.Status != RunStatusFailed || outcome.AuthoritativeStatus != RunStatusFailed {
			t.Fatalf("outcome = %+v, want confirmed durable failure", outcome)
		}
		if len(progress.statuses) != 1 || progress.statuses[0] != RunStatusRunning {
			t.Fatalf("published statuses = %v, want [running]", progress.statuses)
		}
		record, _, err := store.Run("mark-succeeded")
		if err != nil || record.Status != RunStatusFailed {
			t.Fatalf("durable run = %+v, %v", record, err)
		}
	})
}

func TestRunnerPersistenceFailurePublishesOnlyConfirmedDurableStatus(t *testing.T) {
	t.Run("successful reread publishes persisted running", func(t *testing.T) {
		store := newRunnerTestStore()
		store.runs["run-1"] = RunRecord{ID: "run-1", Status: RunStatusRunning}
		store.outputs["run-1"] = RunOutput{RunID: "run-1"}
		store.finishErr = errors.New("database unavailable")
		executor := &commandExecutor{deps: RunnerDependencies{Store: store}}

		outcome := executor.finish(QueuedRun{RunID: "run-1"}, RunFinish{
			Status: RunStatusSucceeded, FinishedAt: time.Now().UTC(),
		})
		if outcome.Status != RunStatusRunning || outcome.AuthoritativeStatus != RunStatusRunning {
			t.Fatalf("outcome = %+v, want confirmed durable running", outcome)
		}
		if !strings.Contains(outcome.Error, "persist terminal command: database unavailable") {
			t.Fatalf("error = %q", outcome.Error)
		}
	})

	t.Run("failed reread leaves durable status unknown", func(t *testing.T) {
		store := newRunnerTestStore()
		store.runs["run-2"] = RunRecord{ID: "run-2", Status: RunStatusRunning}
		store.outputs["run-2"] = RunOutput{RunID: "run-2"}
		store.finishErr = errors.New("database unavailable")
		store.runErr = errors.New("read unavailable")
		executor := &commandExecutor{deps: RunnerDependencies{Store: store}}

		outcome := executor.finish(QueuedRun{RunID: "run-2"}, RunFinish{
			Status: RunStatusSucceeded, FinishedAt: time.Now().UTC(),
		})
		if outcome.Status != RunStatusFailed || outcome.AuthoritativeStatus != "" {
			t.Fatalf("outcome = %+v, want generic failure without durable authority", outcome)
		}
		if !strings.Contains(outcome.Error, "read durable command status: read unavailable") {
			t.Fatalf("error = %q", outcome.Error)
		}
	})

	t.Run("lost terminal transition and failed reread is a generic failure", func(t *testing.T) {
		store := newRunnerTestStore()
		store.runs["run-3"] = RunRecord{ID: "run-3", Status: RunStatusInterrupted}
		store.outputs["run-3"] = RunOutput{RunID: "run-3"}
		store.runErr = errors.New("read unavailable")
		executor := &commandExecutor{deps: RunnerDependencies{Store: store}}

		outcome := executor.finish(QueuedRun{RunID: "run-3"}, RunFinish{
			Status: RunStatusSucceeded, FinishedAt: time.Now().UTC(),
		})
		if outcome.Status != RunStatusFailed || outcome.AuthoritativeStatus != "" {
			t.Fatalf("outcome = %+v, want generic failure without durable authority", outcome)
		}
		if !strings.Contains(outcome.Error, "read terminal command: read unavailable") {
			t.Fatalf("error = %q", outcome.Error)
		}
	})
}

func TestRunnerAdmissionPersistsRedactedInvocationAndCleansRejectedFolder(t *testing.T) {
	root, commandDir := t.TempDir(), t.TempDir()
	helperExecutable(t, commandDir, "mah-helper")
	store := newRunnerTestStore()
	settings := runnerTestSettings{root: root, commandDir: commandDir, pending: 1, perRun: 1 << 20, global: 1 << 21}
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
	declaration := Declaration{
		Name: "record", Timeout: time.Minute, SensitiveParams: []string{"token"},
		Argv: []string{"mah-helper", helperProcessFlag, "record", "{{exchange_dir}}", "{{token}}"},
	}
	firstID, err := dispatcher.Submit(CommandRequest{PluginName: "plug", Declaration: declaration, Params: map[string]string{"token": "top-secret"}})
	if err != nil {
		t.Fatal(err)
	}
	record, output, err := store.Run(firstID)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(record.ParamsJSON, "top-secret") || strings.Contains(output.ArgvJSON, "top-secret") {
		t.Fatalf("sensitive parameter persisted: run=%+v output=%+v", record, output)
	}
	if !strings.Contains(record.ParamsJSON, redactedValue) || !strings.Contains(output.ArgvJSON, redactedValue) {
		t.Fatalf("redacted value missing: run=%+v output=%+v", record, output)
	}
	for i := 0; i < 2; i++ {
		if _, err := dispatcher.Submit(CommandRequest{PluginName: "plug", Declaration: declaration, Params: map[string]string{"token": "another"}}); err != nil {
			t.Fatal(err)
		}
	}
	_, err = dispatcher.Submit(CommandRequest{PluginName: "plug", Declaration: declaration, Params: map[string]string{"token": "rejected"}})
	if err == nil || !strings.Contains(err.Error(), "queue is full") {
		t.Fatalf("fourth Submit error = %v", err)
	}
	entries, err := os.ReadDir(filepath.Join(root, "plugin_exchange", "plug"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("run directories = %d; want 3 (rejected admission cleaned)", len(entries))
	}
}

func TestRunnerProcessGroupPersistsBootSessionWithEnvironmentArgvPathAndRedaction(t *testing.T) {
	root := t.TempDir()
	trusted := t.TempDir()
	rogue := t.TempDir()
	helperExecutable(t, trusted, "mah-helper")
	rogueMarker := filepath.Join(root, "rogue-ran")
	roguePath := filepath.Join(rogue, "mah-helper")
	if err := os.WriteFile(roguePath, []byte("#!/bin/sh\ntouch "+rogueMarker+"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", rogue+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("HOME", "/operator/home")
	t.Setenv("LANG", "en_US.UTF-8")
	t.Setenv("TZ", "UTC")
	t.Setenv("SHOULD_NOT_LEAK", "secret")

	settings := runnerTestSettings{root: root, commandDir: trusted, perRun: 1 << 20, global: 1 << 21}
	store := newRunnerTestStore()
	const bootSessionID = "boot-session-runner"
	executor := NewExecutor(RunnerDependencies{Store: store, Settings: settings, BootSessionID: bootSessionID})
	runID := "0123456789abcdef0123456789abcdef"
	exchange := filepath.Join(root, "plugin_exchange", "plug", runID)
	declaration := Declaration{
		Name: "record", Timeout: 5 * time.Second, SensitiveParams: []string{"token"},
		Argv: []string{"mah-helper", helperProcessFlag, "record", "{{exchange_dir}}", "{{token}}", "literal with spaces", "$(not-shell)"},
	}
	invocation, err := BuildInvocation(declaration, map[string]string{"token": "top-secret"}, exchange)
	if err != nil {
		t.Fatal(err)
	}
	run := QueuedRun{RunID: runID, ExchangeDir: exchange, Invocation: invocation, Request: CommandRequest{PluginName: "plug", Declaration: declaration}}
	if err := executor.(stagingUsageCacheProvider).stagingUsageCache().Refresh(root); err != nil {
		t.Fatal(err)
	}
	if err := executor.(interface{ Prepare(QueuedRun) error }).Prepare(run); err != nil {
		t.Fatal(err)
	}
	params, _ := json.Marshal(invocation.ParamView)
	argv, _ := json.Marshal(invocation.RedactedArgv)
	if err := store.CreateRun(RunRecord{ID: runID, PluginName: "plug", Status: RunStatusQueued, ParamsJSON: string(params)}, RunOutput{RunID: runID, ArgvJSON: string(argv)}); err != nil {
		t.Fatal(err)
	}

	outcome := executor.Execute(context.Background(), run)
	if outcome.Status != RunStatusSucceeded {
		t.Fatalf("outcome = %+v", outcome)
	}
	store.mu.Lock()
	persistedBootSessionID := store.bootSessionIDs[runID]
	store.mu.Unlock()
	if persistedBootSessionID != bootSessionID {
		t.Fatalf("persisted boot session = %q, want %q", persistedBootSessionID, bootSessionID)
	}
	encoded, err := os.ReadFile(filepath.Join(exchange, "record.json"))
	if err != nil {
		t.Fatal(err)
	}
	var record helperRecord
	if err := json.Unmarshal(encoded, &record); err != nil {
		t.Fatal(err)
	}
	if record.Arg0 != "mah-helper" {
		t.Fatalf("argv[0] = %q; want manifest basename", record.Arg0)
	}
	wantArgs := []string{"top-secret", "literal with spaces", "$(not-shell)"}
	if strings.Join(record.Args, "\x00") != strings.Join(wantArgs, "\x00") {
		t.Fatalf("argv = %#v; want %#v", record.Args, wantArgs)
	}
	gotCWD, err := os.Stat(record.Cwd)
	if err != nil {
		t.Fatal(err)
	}
	wantCWD, err := os.Stat(exchange)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(gotCWD, wantCWD) {
		t.Fatalf("cwd = %q; want same directory as %q", record.Cwd, exchange)
	}
	if record.Stdin != "" {
		t.Fatalf("stdin = %q; want EOF", record.Stdin)
	}
	wantEnv := map[string]string{
		"PATH": trusted, "HOME": "/operator/home", "TMPDIR": filepath.Join(exchange, ".tmp"),
		"LANG": "en_US.UTF-8", "TZ": "UTC", "MAHR_PLUGIN_NAME": "plug",
		"MAHR_COMMAND_RUN_ID": runID, "MAHR_EXCHANGE_DIR": exchange,
	}
	gotEnv := make(map[string]string)
	for _, item := range record.Env {
		key, value, ok := strings.Cut(item, "=")
		if ok {
			gotEnv[key] = value
		}
	}
	if len(gotEnv) != len(wantEnv) {
		t.Fatalf("environment = %#v; want only %#v", gotEnv, wantEnv)
	}
	for key, want := range wantEnv {
		if gotEnv[key] != want {
			t.Fatalf("environment %s = %q; want %q", key, gotEnv[key], want)
		}
	}
	if _, err := os.Stat(rogueMarker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("ambient PATH executable ran: %v", err)
	}
	persisted, output, err := store.Run(runID)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(persisted.ParamsJSON, "top-secret") || strings.Contains(output.ArgvJSON, "top-secret") {
		t.Fatalf("sensitive parameter persisted: run=%+v output=%+v", persisted, output)
	}
	if !strings.Contains(output.OutputTail, "stdout-record") || !strings.Contains(output.OutputTail, "stderr-record") {
		t.Fatalf("combined output = %q", output.OutputTail)
	}
	for _, path := range []string{exchange, filepath.Join(exchange, ".tmp")} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o700 {
			t.Fatalf("%s mode = %o; want 700", path, info.Mode().Perm())
		}
	}
}

func TestRunnerRejectsInvalidCommandPath(t *testing.T) {
	for _, commandPath := range []string{"", "relative", string(os.PathListSeparator) + "/bin"} {
		if _, err := resolveExecutable("tool", commandPath); err == nil {
			t.Fatalf("resolveExecutable accepted command path %q", commandPath)
		}
	}
}

func TestRunnerNormalizesTrustedCommandPath(t *testing.T) {
	dir := t.TempDir()
	helperExecutable(t, dir, "mah-helper")
	raw := filepath.Join(dir, ".") + string(os.PathSeparator) + string(os.PathSeparator)
	got, err := resolveExecutable("mah-helper", raw)
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Join(dir, "mah-helper") {
		t.Fatalf("resolved executable = %q", got)
	}
}

func TestRunnerExitStatusMappingAndCannotStart(t *testing.T) {
	root, commandDir := t.TempDir(), t.TempDir()
	helperExecutable(t, commandDir, "mah-helper")
	store := newRunnerTestStore()
	settings := runnerTestSettings{root: root, commandDir: commandDir, perRun: 1 << 20, global: 1 << 21}
	executor := NewExecutor(RunnerDependencies{Store: store, Settings: settings})
	run := seedRunnerRun(t, executor, store, settings, "nonzero", []string{"mah-helper", helperProcessFlag, "exit", "7"}, 5*time.Second)
	outcome := executor.Execute(context.Background(), run)
	if outcome.Status != RunStatusFailed || !strings.Contains(outcome.Error, "status 7") {
		t.Fatalf("nonzero outcome = %+v", outcome)
	}
	record, _, _ := store.Run(run.RunID)
	if record.ExitCode == nil || *record.ExitCode != 7 {
		t.Fatalf("exit code = %v; want 7", record.ExitCode)
	}

	badPath := filepath.Join(commandDir, "bad-executable")
	if err := os.WriteFile(badPath, []byte("not an executable format"), 0o700); err != nil {
		t.Fatal(err)
	}
	bad := seedRunnerRun(t, executor, store, settings, "bad", []string{"bad-executable"}, 5*time.Second)
	outcome = executor.Execute(context.Background(), bad)
	if outcome.Status != RunStatusFailed || !strings.Contains(outcome.Error, "start command") {
		t.Fatalf("cannot-start outcome = %+v", outcome)
	}
}

func TestOutputTailStripsSplitTerminalSequence(t *testing.T) {
	tail := newOutputTail()
	_, _ = tail.Write([]byte("before\x1b[3"))
	_, _ = tail.Write([]byte("1mafter\x00\n"))
	if got, want := tail.String(), "beforeafter\n"; got != want {
		t.Fatalf("tail = %q; want %q", got, want)
	}
}

func TestOutputTailIsBoundedAndStripsTerminalControls(t *testing.T) {
	root, commandDir := t.TempDir(), t.TempDir()
	helperExecutable(t, commandDir, "mah-helper")
	store := newRunnerTestStore()
	settings := runnerTestSettings{root: root, commandDir: commandDir, perRun: 1 << 20, global: 1 << 21}
	executor := NewExecutor(RunnerDependencies{Store: store, Settings: settings})
	run := seedRunnerRun(t, executor, store, settings, "tail", []string{"mah-helper", helperProcessFlag, "tail", strconv.Itoa(outputTailBytes + 4096)}, 5*time.Second)
	outcome := executor.Execute(context.Background(), run)
	if outcome.Status != RunStatusSucceeded {
		t.Fatalf("outcome = %+v", outcome)
	}
	_, output, _ := store.Run(run.RunID)
	if len(output.OutputTail) != outputTailBytes {
		t.Fatalf("tail length = %d; want %d", len(output.OutputTail), outputTailBytes)
	}
	if strings.ContainsAny(output.OutputTail, "\x00\x1b") {
		t.Fatalf("terminal controls survived: %q", output.OutputTail[:32])
	}
	if !strings.HasSuffix(output.OutputTail, "\n") {
		t.Fatal("ordinary newline was not preserved")
	}
}

func requireNativeProcessOwnership(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("native process-group ownership inspection is unavailable on this Unix target")
	}
}

func TestRunnerTimeoutKillsProcessGroupWithScrubbedDescendantBeforePublishingFinalOutput(t *testing.T) {
	requireNativeProcessOwnership(t)
	root, commandDir := t.TempDir(), t.TempDir()
	helperExecutable(t, commandDir, "mah-helper")
	store := newRunnerTestStore()
	settings := runnerTestSettings{root: root, commandDir: commandDir, perRun: 1 << 20, global: 1 << 21}
	executor := NewExecutor(RunnerDependencies{Store: store, Settings: settings})
	run := seedRunnerRun(t, executor, store, settings, "descendant", []string{"mah-helper", helperProcessFlag, "spawn-scrubbed-descendant", "{{exchange_dir}}"}, 250*time.Millisecond)
	outcome := executor.Execute(context.Background(), run)
	if outcome.Status != RunStatusFailed || !strings.Contains(outcome.Error, "timeout") {
		t.Fatalf("outcome = %+v", outcome)
	}
	pidBytes, err := os.ReadFile(filepath.Join(run.ExchangeDir, "descendant.pid"))
	if err != nil {
		t.Fatal(err)
	}
	pid, _ := strconv.Atoi(string(pidBytes))
	if processAlive(pid) {
		t.Fatalf("descendant %d remains alive after terminal publication", pid)
	}
	time.Sleep(800 * time.Millisecond)
	if _, err := os.Stat(filepath.Join(run.ExchangeDir, "late-write")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("descendant wrote after terminal publication: %v", err)
	}
	_, output, _ := store.Run(run.RunID)
	if strings.Contains(output.OutputTail, "late-descendant-output") {
		t.Fatal("output arrived after terminal publication")
	}
}

type unverifiedUntilKilledInspector struct {
	mu                    sync.Mutex
	killCalls             int
	inspectCalls          int
	postKillInspectErrors int
	killed                bool
}

func (i *unverifiedUntilKilledInspector) InspectGroup(int, string) (GroupIdentity, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.inspectCalls++
	if i.killed && i.postKillInspectErrors > 0 {
		i.postKillInspectErrors--
		return GroupIdentity{}, errors.New("transient inspection failure")
	}
	if i.killed {
		return GroupIdentity{State: GroupDead}, nil
	}
	return GroupIdentity{State: GroupAliveUnverified}, nil
}

func (i *unverifiedUntilKilledInspector) KillGroup(pgid int) error {
	i.mu.Lock()
	i.killCalls++
	i.mu.Unlock()
	err := syscall.Kill(-pgid, syscall.SIGKILL)
	i.mu.Lock()
	i.killed = err == nil || errors.Is(err, syscall.ESRCH)
	i.mu.Unlock()
	return err
}

func TestRunnerKillsLocallyCreatedGroupWhenMemberEnvironmentIsUnverified(t *testing.T) {
	root, commandDir := t.TempDir(), t.TempDir()
	helperExecutable(t, commandDir, "mah-helper")
	store := newRunnerTestStore()
	settings := runnerTestSettings{root: root, commandDir: commandDir, perRun: 1 << 20, global: 1 << 21}
	inspector := &unverifiedUntilKilledInspector{postKillInspectErrors: 2}
	executor := NewExecutor(RunnerDependencies{Store: store, Settings: settings, Inspector: inspector})
	executor.(*commandExecutor).cleanupTimeout = 100 * time.Millisecond
	run := seedRunnerRun(t, executor, store, settings, "local-owned", []string{"mah-helper", helperProcessFlag, "spawn-scrubbed-descendant", "{{exchange_dir}}"}, 100*time.Millisecond)
	outcome := executor.Execute(context.Background(), run)
	if outcome.Status != RunStatusFailed || outcome.AuthoritativeStatus != RunStatusFailed || !strings.Contains(outcome.Error, "timeout") {
		t.Fatalf("outcome = %+v", outcome)
	}
	inspector.mu.Lock()
	killCalls := inspector.killCalls
	inspector.mu.Unlock()
	if killCalls != 1 {
		t.Fatalf("group kills = %d, want 1", killCalls)
	}
	pid := waitForHelperPID(t, filepath.Join(run.ExchangeDir, "descendant.pid"))
	if processAlive(pid) {
		t.Fatalf("descendant %d remains alive after terminal publication", pid)
	}
}

type boundedResignalInspector struct {
	mu                       sync.Mutex
	state                    GroupState
	killCalls                int
	deadAfterKills           int
	inspectionErrorAfterKill bool
	proveDead                bool
	inspectionTimes          []time.Time
	killTimes                []time.Time
}

func (i *boundedResignalInspector) InspectGroup(int, string) (GroupIdentity, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.inspectionTimes = append(i.inspectionTimes, time.Now())
	if i.proveDead || i.deadAfterKills > 0 && i.killCalls >= i.deadAfterKills {
		return GroupIdentity{State: GroupDead}, nil
	}
	if i.inspectionErrorAfterKill && i.killCalls > 0 {
		return GroupIdentity{}, errors.New("inspection unavailable after first signal")
	}
	return GroupIdentity{State: i.state}, nil
}

func (i *boundedResignalInspector) KillGroup(int) error {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.killCalls++
	i.killTimes = append(i.killTimes, time.Now())
	return nil
}

func (i *boundedResignalInspector) setDead() {
	i.mu.Lock()
	i.proveDead = true
	i.mu.Unlock()
}

func (i *boundedResignalInspector) snapshot() (int, []time.Time, []time.Time) {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.killCalls, append([]time.Time(nil), i.inspectionTimes...), append([]time.Time(nil), i.killTimes...)
}

func startBoundedResignalRun(t *testing.T, inspector *boundedResignalInspector, warn func(RuntimeWarning)) (<-chan Outcome, QueuedRun) {
	t.Helper()
	root, commandDir := t.TempDir(), "/bin"
	store := newRunnerTestStore()
	settings := runnerTestSettings{root: root, commandDir: commandDir, perRun: 1 << 20, global: 1 << 21}
	executor := NewExecutor(RunnerDependencies{Store: store, Settings: settings, Inspector: inspector, Warn: warn})
	configured := executor.(*commandExecutor)
	configured.pollInterval = 5 * time.Millisecond
	configured.stuckPollInterval = 40 * time.Millisecond
	configured.cleanupTimeout = 20 * time.Millisecond
	configured.quotaInterval = 10 * time.Millisecond
	configured.stuckWarningAfter = 80 * time.Millisecond
	run := seedRunnerRun(t, executor, store, settings, "bounded-resignal", []string{"sh", "-c", "exit 0"}, 5*time.Second)
	result := make(chan Outcome, 1)
	go func() { result <- executor.Execute(context.Background(), run) }()
	return result, run
}

func requireBoundedResignal(t *testing.T, state GroupState) {
	t.Helper()
	inspector := &boundedResignalInspector{state: state, deadAfterKills: 2}
	result, _ := startBoundedResignalRun(t, inspector, nil)
	select {
	case outcome := <-result:
		if outcome.Status != RunStatusFailed || outcome.AuthoritativeStatus != RunStatusFailed {
			t.Fatalf("outcome = %+v", outcome)
		}
	case <-time.After(750 * time.Millisecond):
		inspector.setDead()
		<-result
		t.Fatal("runner did not publish after its bounded second group signal")
	}
	kills, _, _ := inspector.snapshot()
	if kills != 2 {
		t.Fatalf("group kills = %d, want 2", kills)
	}
}

func TestRunnerResignalAliveUnverifiedGroupOnce(t *testing.T) {
	requireBoundedResignal(t, GroupAliveUnverified)
}

func TestRunnerResignalAliveOwnedGroupOnce(t *testing.T) {
	requireBoundedResignal(t, GroupAliveOwned)
}

func requireRunnerStillBlocked(t *testing.T, result <-chan Outcome, reason string) {
	t.Helper()
	select {
	case outcome := <-result:
		t.Fatalf("runner published while %s: %+v", reason, outcome)
	default:
	}
}

func TestRunnerInspectionErrorDoesNotPermitResignal(t *testing.T) {
	inspector := &boundedResignalInspector{state: GroupAliveOwned, inspectionErrorAfterKill: true}
	result, _ := startBoundedResignalRun(t, inspector, nil)
	time.Sleep(180 * time.Millisecond)
	kills, _, _ := inspector.snapshot()
	if kills != 1 {
		inspector.setDead()
		<-result
		t.Fatalf("group kills after an inspection error = %d, want 1", kills)
	}
	requireRunnerStillBlocked(t, result, "process-group inspection remains unavailable")
	inspector.setDead()
	select {
	case <-result:
	case <-time.After(time.Second):
		t.Fatal("runner did not publish after group death became observable")
	}
}

func TestRunnerResignalNeverSignalsAStuckGroupMoreThanTwice(t *testing.T) {
	inspector := &boundedResignalInspector{state: GroupAliveOwned}
	result, _ := startBoundedResignalRun(t, inspector, nil)
	time.Sleep(260 * time.Millisecond)
	kills, _, _ := inspector.snapshot()
	if kills != 2 {
		inspector.setDead()
		<-result
		t.Fatalf("group kills across later cleanup deadlines = %d, want 2", kills)
	}
	requireRunnerStillBlocked(t, result, "the group remains alive after the bounded second signal")
	inspector.setDead()
	select {
	case <-result:
	case <-time.After(time.Second):
		t.Fatal("runner did not publish after group death became observable")
	}
}

func TestRunnerPollBackoffAndPinnedSlotWarning(t *testing.T) {
	inspector := &boundedResignalInspector{state: GroupAliveUnverified}
	var warningMu sync.Mutex
	var warnings []RuntimeWarning
	warningReady := make(chan struct{}, 1)
	result, run := startBoundedResignalRun(t, inspector, func(warning RuntimeWarning) {
		warningMu.Lock()
		warnings = append(warnings, warning)
		warningMu.Unlock()
		select {
		case warningReady <- struct{}{}:
		default:
		}
	})
	select {
	case <-warningReady:
	case <-time.After(time.Second):
		inspector.setDead()
		<-result
		t.Fatal("stuck command warning was not emitted")
	}
	// Leave the group alive across more slow polls to prove both the backoff and
	// the one-shot warning behavior.
	time.Sleep(110 * time.Millisecond)
	kills, inspections, killTimes := inspector.snapshot()
	if kills != 2 || len(killTimes) != 2 {
		inspector.setDead()
		<-result
		t.Fatalf("group kills = %d at %v, want exactly two", kills, killTimes)
	}
	var beforeForced, afterForced []time.Time
	for _, inspectedAt := range inspections {
		if inspectedAt.Before(killTimes[1]) {
			beforeForced = append(beforeForced, inspectedAt)
		} else {
			afterForced = append(afterForced, inspectedAt)
		}
	}
	if len(beforeForced) < 2 {
		t.Errorf("pre-cleanup inspections = %d, want high-frequency polling", len(beforeForced))
	}
	if len(afterForced) < 3 {
		t.Errorf("post-cleanup inspections = %d, want at least 3", len(afterForced))
	}
	for index := 1; index < len(afterForced); index++ {
		if interval := afterForced[index].Sub(afterForced[index-1]); interval < 32*time.Millisecond {
			t.Errorf("post-cleanup inspection interval = %s, want at least 32ms", interval)
		}
	}
	warningMu.Lock()
	gotWarnings := append([]RuntimeWarning(nil), warnings...)
	warningMu.Unlock()
	if len(gotWarnings) != 1 {
		t.Errorf("warnings = %+v, want exactly one", gotWarnings)
	} else {
		warning := gotWarnings[0]
		if warning.Event != RuntimeWarningEventPinnedSlot || warning.RunID != run.RunID || warning.ProcessGroupID <= 0 || warning.ActiveLimit != maxActiveCommands {
			t.Errorf("warning = %+v", warning)
		}
	}
	requireRunnerStillBlocked(t, result, "the pinned-slot warning has been emitted for a live group")
	inspector.setDead()
	select {
	case <-result:
	case <-time.After(time.Second):
		t.Fatal("runner did not publish after group death became observable")
	}
}

type countingNativeInspector struct {
	mu    sync.Mutex
	calls int
}

func (i *countingNativeInspector) InspectGroup(pgid int, runID string) (GroupIdentity, error) {
	i.mu.Lock()
	i.calls++
	i.mu.Unlock()
	return (nativeProcessInspector{}).InspectGroup(pgid, runID)
}

func (i *countingNativeInspector) KillGroup(pgid int) error {
	return (nativeProcessInspector{}).KillGroup(pgid)
}

func TestRunnerDoesNotPollProcessGroupWhileParentIsRunning(t *testing.T) {
	requireNativeProcessOwnership(t)
	root, commandDir := t.TempDir(), t.TempDir()
	helperExecutable(t, commandDir, "mah-helper")
	store := newRunnerTestStore()
	settings := runnerTestSettings{root: root, commandDir: commandDir, perRun: 1 << 20, global: 1 << 21}
	inspector := &countingNativeInspector{}
	executor := NewExecutor(RunnerDependencies{Store: store, Settings: settings, Inspector: inspector})
	run := seedRunnerRun(t, executor, store, settings, "no-poll", []string{"mah-helper", helperProcessFlag, "sleep-ms", "300"}, 2*time.Second)
	outcome := executor.Execute(context.Background(), run)
	if outcome.Status != RunStatusSucceeded {
		t.Fatalf("outcome = %+v", outcome)
	}
	inspector.mu.Lock()
	calls := inspector.calls
	inspector.mu.Unlock()
	if calls > 5 {
		t.Fatalf("process-group inspections = %d while parent ran; want at most 5", calls)
	}
}

func TestRunnerCancellationKillsProcessGroup(t *testing.T) {
	requireNativeProcessOwnership(t)
	root, commandDir := t.TempDir(), t.TempDir()
	helperExecutable(t, commandDir, "mah-helper")
	store := newRunnerTestStore()
	settings := runnerTestSettings{root: root, commandDir: commandDir, perRun: 1 << 20, global: 1 << 21}
	executor := NewExecutor(RunnerDependencies{Store: store, Settings: settings})
	run := seedRunnerRun(t, executor, store, settings, "cancel", []string{"mah-helper", helperProcessFlag, "spawn-descendant", "{{exchange_dir}}"}, 5*time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		for i := 0; i < 200; i++ {
			if _, err := os.Stat(filepath.Join(run.ExchangeDir, "descendant.pid")); err == nil {
				cancel()
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
		cancel()
	}()
	outcome := executor.Execute(ctx, run)
	if outcome.Status != RunStatusCancelled {
		t.Fatalf("outcome = %+v", outcome)
	}
}

func TestRunnerTimeoutStartsWhenTheProcessSpawns(t *testing.T) {
	requireNativeProcessOwnership(t)
	root, commandDir := t.TempDir(), t.TempDir()
	helperExecutable(t, commandDir, "mah-helper")
	release := make(chan struct{})
	defer close(release)
	store := newRunnerTestStore()
	store.setProcessGroupStarted = make(chan struct{}, 1)
	store.setProcessGroupRelease = release
	settings := runnerTestSettings{root: root, commandDir: commandDir, perRun: 1 << 20, global: 1 << 21}
	executor := NewExecutor(RunnerDependencies{Store: store, Settings: settings})
	executor.(*commandExecutor).cleanupTimeout = 100 * time.Millisecond
	run := seedRunnerRun(t, executor, store, settings, "persist-delay", []string{"mah-helper", helperProcessFlag, "spawn-descendant", "{{exchange_dir}}"}, 75*time.Millisecond)

	result := make(chan Outcome, 1)
	go func() { result <- executor.Execute(context.Background(), run) }()
	select {
	case <-store.setProcessGroupStarted:
	case <-time.After(time.Second):
		t.Fatal("process group persistence did not start")
	}
	descendantPID := waitForHelperPID(t, filepath.Join(run.ExchangeDir, "descendant.pid"))
	select {
	case outcome := <-result:
		if outcome.Status != RunStatusFailed || !strings.Contains(outcome.Error, "timeout") {
			t.Fatalf("outcome = %+v", outcome)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waited for process-group persistence")
	}
	if err := syscall.Kill(descendantPID, 0); err == nil || !errors.Is(err, syscall.ESRCH) {
		t.Fatalf("descendant %d survived terminal publication: %v", descendantPID, err)
	}
	record, output, err := store.Run(run.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if !record.OutputUnverified {
		t.Fatalf("persist-stalled terminal output = record %+v output %+v", record, output)
	}
}

func TestRunnerDurableCancellationWinsALateTerminalRace(t *testing.T) {
	root, commandDir := t.TempDir(), t.TempDir()
	helperExecutable(t, commandDir, "mah-helper")
	store := newRunnerTestStore()
	var once sync.Once
	store.beforeFinish = func(id string, finish RunFinish) {
		if finish.Status == RunStatusSucceeded || finish.Status == RunStatusFailed {
			once.Do(func() {
				if err := store.RequestRunCancel(id, "operator cancelled"); err != nil {
					t.Errorf("RequestRunCancel: %v", err)
				}
			})
		}
	}
	settings := runnerTestSettings{root: root, commandDir: commandDir, perRun: 1 << 20, global: 1 << 21}
	executor := NewExecutor(RunnerDependencies{Store: store, Settings: settings})
	run := seedRunnerRun(t, executor, store, settings, "late-cancel", []string{"mah-helper", helperProcessFlag, "exit", "0"}, 5*time.Second)

	outcome := executor.Execute(context.Background(), run)
	if outcome.Status != RunStatusCancelled || outcome.Error != "operator cancelled" {
		t.Fatalf("outcome = %+v", outcome)
	}
	record, _, err := store.Run(run.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if record.Status != RunStatusCancelled || record.FinishedAt == nil {
		t.Fatalf("record = %+v", record)
	}
}

func TestRunnerBoundsDetachedPipeDrain(t *testing.T) {
	root, commandDir := t.TempDir(), t.TempDir()
	helperExecutable(t, commandDir, "mah-helper")
	store := newRunnerTestStore()
	settings := runnerTestSettings{root: root, commandDir: commandDir, perRun: 1 << 20, global: 1 << 21}
	executor := NewExecutor(RunnerDependencies{Store: store, Settings: settings})
	executor.(*commandExecutor).cleanupTimeout = 100 * time.Millisecond
	run := seedRunnerRun(t, executor, store, settings, "detached-pipe", []string{"mah-helper", helperProcessFlag, "spawn-detached-descendant", "{{exchange_dir}}"}, 5*time.Second)

	result := make(chan Outcome, 1)
	go func() { result <- executor.Execute(context.Background(), run) }()
	pid := waitForHelperPID(t, filepath.Join(run.ExchangeDir, "descendant.pid"))
	defer syscall.Kill(pid, syscall.SIGKILL)
	select {
	case outcome := <-result:
		if outcome.Status != RunStatusFailed || !strings.Contains(outcome.Error, "output pipes") {
			t.Fatalf("outcome = %+v", outcome)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("runner waited indefinitely for detached pipe writer")
	}
	record, _, err := store.Run(run.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if !record.OutputUnverified {
		t.Fatalf("record = %+v; want output_unverified", record)
	}
}

type controlledFailingProcessInspector struct {
	mu        sync.Mutex
	proveDead bool
}

func (i *controlledFailingProcessInspector) InspectGroup(int, string) (GroupIdentity, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.proveDead {
		return GroupIdentity{State: GroupDead}, nil
	}
	return GroupIdentity{}, errors.New("inspection unavailable")
}
func (*controlledFailingProcessInspector) KillGroup(int) error { return errors.New("kill unavailable") }

func TestRunnerDoesNotPublishWhileGroupInspectionCannotProveDeath(t *testing.T) {
	root, commandDir := t.TempDir(), t.TempDir()
	helperExecutable(t, commandDir, "mah-helper")
	store := newRunnerTestStore()
	settings := runnerTestSettings{root: root, commandDir: commandDir, perRun: 1 << 20, global: 1 << 21}
	inspector := &controlledFailingProcessInspector{}
	executor := NewExecutor(RunnerDependencies{Store: store, Settings: settings, Inspector: inspector})
	executor.(*commandExecutor).cleanupTimeout = 50 * time.Millisecond
	run := seedRunnerRun(t, executor, store, settings, "inspect-fail", []string{"mah-helper", helperProcessFlag, "sleep"}, 50*time.Millisecond)

	result := make(chan Outcome, 1)
	go func() { result <- executor.Execute(context.Background(), run) }()
	select {
	case outcome := <-result:
		t.Fatalf("published while process-group death was unverified: %+v", outcome)
	case <-time.After(250 * time.Millisecond):
	}
	inspector.mu.Lock()
	inspector.proveDead = true
	inspector.mu.Unlock()
	select {
	case outcome := <-result:
		if outcome.Status != RunStatusFailed || !strings.Contains(outcome.Error, "timeout") {
			t.Fatalf("outcome = %+v", outcome)
		}
	case <-time.After(time.Second):
		t.Fatal("runner did not publish after process-group death became observable")
	}
	record, _, err := store.Run(run.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if !record.OutputUnverified {
		t.Fatalf("record = %+v; want output_unverified", record)
	}
}

func waitForHelperPID(t *testing.T, path string) int {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err == nil {
			pid, parseErr := strconv.Atoi(strings.TrimSpace(string(data)))
			if parseErr == nil && pid > 0 {
				return pid
			}
			time.Sleep(5 * time.Millisecond)
			continue
		}
		if !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("helper pid was not written to %s", path)
	return 0
}

func processAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

func seedRunnerRun(t *testing.T, executor Executor, store *runnerTestStore, settings runnerTestSettings, suffix string, argv []string, timeout time.Duration) QueuedRun {
	t.Helper()
	runID := strings.Repeat(string("abcdef0123456789"[len(suffix)%16]), 32)
	exchange := filepath.Join(settings.root, "plugin_exchange", "plug", runID)
	declaration := Declaration{Name: "run-" + suffix, Argv: argv, Timeout: timeout}
	invocation, err := BuildInvocation(declaration, nil, exchange)
	if err != nil {
		t.Fatal(err)
	}
	run := QueuedRun{RunID: runID, ExchangeDir: exchange, Invocation: invocation, Request: CommandRequest{PluginName: "plug", Declaration: declaration}}
	if provider, ok := executor.(stagingUsageCacheProvider); ok {
		if err := provider.stagingUsageCache().Refresh(settings.root); err != nil {
			t.Fatal(err)
		}
	}
	if err := executor.(interface{ Prepare(QueuedRun) error }).Prepare(run); err != nil {
		t.Fatal(err)
	}
	argvJSON, _ := json.Marshal(invocation.RedactedArgv)
	if err := store.CreateRun(RunRecord{ID: runID, PluginName: "plug", Status: RunStatusQueued}, RunOutput{RunID: runID, ArgvJSON: string(argvJSON)}); err != nil {
		t.Fatal(err)
	}
	return run
}
