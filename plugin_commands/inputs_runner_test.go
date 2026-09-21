//go:build !windows

package plugin_commands

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// inputRunnerFixture is one run wired to the helper process in read-input mode:
// the command copies the named file it was handed into a second file and prints
// a fixed token, so a test can prove the bytes arrived without any content
// reaching a record.
type inputRunnerFixture struct {
	executor    *commandExecutor
	store       *runnerTestStore
	settings    runnerTestSettings
	declaration Declaration
	run         QueuedRun
	exchange    string
	secret      []byte
}

func newInputRunnerFixture(t *testing.T, inputs ...InputFile) *inputRunnerFixture {
	t.Helper()
	root, commandDir := t.TempDir(), t.TempDir()
	helperExecutable(t, commandDir, "mah-helper")
	store := newRunnerTestStore()
	settings := runnerTestSettings{root: root, commandDir: commandDir, perRun: 1 << 20, global: 1 << 21}
	executor := NewExecutor(RunnerDependencies{Store: store, Settings: settings}).(*commandExecutor)

	declaration := Declaration{
		Name: "read", Timeout: time.Minute,
		Argv:   []string{"mah-helper", helperProcessFlag, "read-input", "cookies.txt", "seen.txt"},
		Inputs: []string{"cookies.txt"},
	}
	const runID = "0123456789abcdef0123456789abcdef"
	exchange := filepath.Join(root, "plugin_exchange", "plug", runID)
	invocation, err := BuildInvocation(declaration, nil, exchange)
	if err != nil {
		t.Fatal(err)
	}
	run := QueuedRun{
		RunID: runID, ExchangeDir: exchange, Invocation: invocation,
		Request: CommandRequest{PluginName: "plug", Declaration: declaration},
		Inputs:  inputs,
	}
	if err := executor.stagingUsageCache().Refresh(root); err != nil {
		t.Fatal(err)
	}
	if err := executor.Prepare(run); err != nil {
		t.Fatal(err)
	}
	params, _ := json.Marshal(invocation.ParamView)
	argv, _ := json.Marshal(invocation.RedactedArgv)
	if err := store.CreateRun(RunRecord{ID: runID, PluginName: "plug", Status: RunStatusQueued, ParamsJSON: string(params)}, RunOutput{RunID: runID, ArgvJSON: string(argv)}); err != nil {
		t.Fatal(err)
	}
	return &inputRunnerFixture{
		executor: executor, store: store, settings: settings, declaration: declaration,
		run: run, exchange: exchange, secret: inputs[0].Content,
	}
}

func TestRunnerWritesSuppliedInputsBeforeTheSpawn(t *testing.T) {
	secret := []byte("# Netscape HTTP Cookie File\n.youtube.com\tTRUE\t/\tTRUE\t0\tSID\tsecret\n")
	f := newInputRunnerFixture(t, InputFile{Name: "cookies.txt", Content: append([]byte(nil), secret...)})
	// The submission map is what the dispatcher's copies share, so clearing it
	// has to be observable from the caller's side.
	f.run.Request.Inputs = map[string]string{"cookies.txt": string(secret)}
	callerMap := f.run.Request.Inputs
	callerSlice := f.run.Inputs

	outcome := f.executor.Execute(context.Background(), f.run)
	if outcome.Status != RunStatusSucceeded {
		t.Fatalf("outcome = %+v", outcome)
	}

	seen, err := os.ReadFile(filepath.Join(f.exchange, "seen.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(seen, secret) {
		t.Fatalf("the program read %q, want the supplied bytes", seen)
	}
	info, err := os.Lstat(filepath.Join(f.exchange, "cookies.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		t.Fatalf("input mode = %v, want a regular file at 0600", info.Mode())
	}
	if info.Size() != int64(len(secret)) {
		t.Fatalf("input size = %d, want %d", info.Size(), len(secret))
	}

	record, output, err := f.store.Run(f.run.RunID)
	if err != nil {
		t.Fatal(err)
	}
	for name, text := range map[string]string{
		"params": record.ParamsJSON, "argv": output.ArgvJSON, "output": output.OutputTail, "error": record.Error,
	} {
		if strings.Contains(text, "SID") {
			t.Fatalf("%s leaked the supplied contents: %q", name, text)
		}
	}
	if !strings.Contains(output.OutputTail, "read-ok") {
		t.Fatalf("output = %q, want the fixture token", output.OutputTail)
	}

	// Contents are dropped once they are on disk: the map the dispatcher's
	// copies share and the slice's backing array both lose them.
	for name, content := range callerMap {
		if content != "" {
			t.Fatalf("request inputs still hold contents for %q", name)
		}
	}
	for i, input := range callerSlice {
		if !bytes.Equal(input.Content, make([]byte, len(input.Content))) {
			t.Fatalf("validated input %d still holds contents", i)
		}
	}
}

func TestRunnerRefusesTheSpawnWhenAnInputCannotBeWritten(t *testing.T) {
	f := newInputRunnerFixture(t, InputFile{Name: "cookies.txt", Content: []byte("SID=secret")})
	f.executor.writeInputFileFn = func(dir, name string, content []byte) error {
		// A crash between the scratch create and the rename: the declared name
		// must not exist, and the debris must sit in the directory mah.fs
		// cannot list or read.
		if err := os.WriteFile(filepath.Join(dir, ".tmp", "input-crash"), content[:len(content)/2], 0o600); err != nil {
			t.Fatal(err)
		}
		return errors.New("host killed between create and rename")
	}
	outcome := f.executor.Execute(context.Background(), f.run)
	if outcome.Status != RunStatusFailed || !strings.Contains(outcome.Error, "cookies.txt") {
		t.Fatalf("outcome = %+v, want a failure naming the file", outcome)
	}
	if _, err := os.Lstat(filepath.Join(f.exchange, "cookies.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a partial file was visible at the declared name: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(f.exchange, "seen.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the program ran despite the failed write: %v", err)
	}
	entries, err := os.ReadDir(f.exchange)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name() != ".tmp" {
			t.Fatalf("crash debris outside .tmp: %q", entry.Name())
		}
	}
	record, _, err := f.store.Run(f.run.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if record.Status != RunStatusFailed || !strings.Contains(record.Error, "write input file") {
		t.Fatalf("durable outcome = %+v", record)
	}

	// The scratch is inside the directory the read path cannot address: it is
	// not listable and not readable, so a crash window is invisible to the
	// plugin rather than a half-written file it might mistake for an input.
	exchangeService := NewExchange(f.store, f.settings)
	access := Access{PluginName: "plug", Administrator: true}
	listing, err := exchangeService.List(access, f.run.RunID)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range listing.Entries {
		if entry.Name == ".tmp" || strings.HasPrefix(entry.Name, "input-") {
			t.Fatalf("scratch is listable: %+v", listing)
		}
	}
	if _, err := exchangeService.Read(access, f.run.RunID, ".tmp", 1024); err == nil {
		t.Fatal("the scratch directory is readable")
	}
}

func TestRunnerRefusesTheSpawnWhenTheWrittenInputIsShort(t *testing.T) {
	f := newInputRunnerFixture(t, InputFile{Name: "cookies.txt", Content: []byte("SID=secret")})
	f.executor.writeInputFileFn = func(dir, name string, content []byte) error {
		return writeInputFile(dir, name, content[:1])
	}
	outcome := f.executor.Execute(context.Background(), f.run)
	if outcome.Status != RunStatusFailed || !strings.Contains(outcome.Error, "verify input file") {
		t.Fatalf("outcome = %+v, want verification to refuse the spawn", outcome)
	}
	if _, err := os.Lstat(filepath.Join(f.exchange, "seen.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the program ran after a short write: %v", err)
	}
}

func TestRunnerWritesNothingForADoomedSpawn(t *testing.T) {
	f := newInputRunnerFixture(t, InputFile{Name: "cookies.txt", Content: []byte("SID=secret")})
	f.settings.commandDir = t.TempDir() // no helper resolved from here
	f.executor.deps.Settings = f.settings
	outcome := f.executor.Execute(context.Background(), f.run)
	if outcome.Status != RunStatusFailed {
		t.Fatalf("outcome = %+v", outcome)
	}
	if _, err := os.Lstat(filepath.Join(f.exchange, "cookies.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a secret reached disk for a run that could never start: %v", err)
	}
}

func TestRunnerWritesNothingWhenTheRunIsCancelledBeforeItStarts(t *testing.T) {
	f := newInputRunnerFixture(t, InputFile{Name: "cookies.txt", Content: []byte("SID=secret")})
	if err := f.store.RequestRunCancel(f.run.RunID, "operator cancelled"); err != nil {
		t.Fatal(err)
	}
	outcome := f.executor.Execute(context.Background(), f.run)
	if outcome.Status != RunStatusCancelled {
		t.Fatalf("outcome = %+v, want cancelled", outcome)
	}
	if _, err := os.Lstat(filepath.Join(f.exchange, "cookies.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a cancelled-before-start run wrote its input: %v", err)
	}
}

func TestRunnerCancellationDuringTheWriteStopsTheSpawn(t *testing.T) {
	f := newInputRunnerFixture(t, InputFile{Name: "cookies.txt", Content: []byte("SID=secret")})
	f.executor.writeInputFileFn = func(dir, name string, content []byte) error {
		if err := f.store.RequestRunCancel(f.run.RunID, "operator cancelled"); err != nil {
			t.Fatal(err)
		}
		return writeInputFile(dir, name, content)
	}
	outcome := f.executor.Execute(context.Background(), f.run)
	if outcome.Status != RunStatusCancelled {
		t.Fatalf("outcome = %+v, want cancelled", outcome)
	}
	if _, err := os.Lstat(filepath.Join(f.exchange, "seen.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the program ran after a cancellation during the write: %v", err)
	}
	// The file is on disk because the write completed; that is the retained
	// case the documentation names, not a leak.
	if _, err := os.Lstat(filepath.Join(f.exchange, "cookies.txt")); err != nil {
		t.Fatalf("expected the completed write to remain: %v", err)
	}
}

func TestRunnerWritesAZeroByteInput(t *testing.T) {
	f := newInputRunnerFixture(t, InputFile{Name: "cookies.txt", Content: []byte{}})
	if outcome := f.executor.Execute(context.Background(), f.run); outcome.Status != RunStatusSucceeded {
		t.Fatalf("outcome = %+v", outcome)
	}
	info, err := os.Lstat(filepath.Join(f.exchange, "cookies.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !info.Mode().IsRegular() || info.Size() != 0 {
		t.Fatalf("zero-byte input = %v, %d bytes", info.Mode(), info.Size())
	}
	seen, err := os.ReadFile(filepath.Join(f.exchange, "seen.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if len(seen) != 0 {
		t.Fatalf("the program read %q, want an empty file", seen)
	}
}

func TestRunnerLeavesNoScratchBehindOnASuccessfulWrite(t *testing.T) {
	f := newInputRunnerFixture(t, InputFile{Name: "cookies.txt", Content: []byte("SID=secret")})
	if outcome := f.executor.Execute(context.Background(), f.run); outcome.Status != RunStatusSucceeded {
		t.Fatalf("outcome = %+v", outcome)
	}
	scratch, err := os.ReadDir(filepath.Join(f.exchange, ".tmp"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range scratch {
		if strings.HasPrefix(entry.Name(), "input-") {
			t.Fatalf("scratch file left behind: %q", entry.Name())
		}
	}
}
