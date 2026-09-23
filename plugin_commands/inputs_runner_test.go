//go:build !windows

package plugin_commands

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
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
	// callerMap and callerSlice are what the dispatcher's own QueuedRun values
	// share: the submission map and the validated slice's backing array.
	callerMap   map[string]string
	callerSlice []InputFile
	// logLines returns everything the executor logged during the run, so a test
	// can prove supplied contents never reach the application log.
	logLines func() []string
}

func newInputRunnerFixture(t *testing.T, inputs ...InputFile) *inputRunnerFixture {
	t.Helper()
	root, commandDir := t.TempDir(), t.TempDir()
	helperExecutable(t, commandDir, "mah-helper")
	store := newRunnerTestStore()
	settings := runnerTestSettings{root: root, commandDir: commandDir, perRun: 1 << 20, global: 1 << 21}
	var logMu sync.Mutex
	var logs []string
	executor := NewExecutor(RunnerDependencies{Store: store, Settings: settings, Logf: func(format string, args ...any) {
		logMu.Lock()
		defer logMu.Unlock()
		logs = append(logs, fmt.Sprintf(format, args...))
	}}).(*commandExecutor)

	declaration := Declaration{
		Name: "read", Timeout: time.Minute,
		Argv:   []string{"mah-helper", helperProcessFlag, "read-input", "cookies.txt", "seen.txt"},
		Inputs: []string{"cookies.txt"},
	}
	const runID = "0123456789abcdef0123456789abcdef"
	exchange := filepath.Join(root, "plugin_exchange", "plug", runID)
	submissionInputs := make(map[string]string, len(inputs))
	for _, input := range inputs {
		submissionInputs[input.Name] = string(input.Content)
	}
	invocation, err := BuildInvocation(declaration, nil, exchange)
	if err != nil {
		t.Fatal(err)
	}
	run := QueuedRun{
		RunID: runID, ExchangeDir: exchange, Invocation: invocation,
		Request: CommandRequest{PluginName: "plug", Declaration: declaration, Inputs: submissionInputs},
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
	if err := store.CreateRun(RunRecord{ID: runID, PluginName: "plug", Status: RunStatusQueued, ParamsJSON: string(params), Inputs: suppliedInputs(inputs)}, RunOutput{RunID: runID, ArgvJSON: string(argv)}); err != nil {
		t.Fatal(err)
	}
	return &inputRunnerFixture{
		executor: executor, store: store, settings: settings, declaration: declaration,
		run: run, exchange: exchange, secret: inputs[0].Content,
		callerMap: submissionInputs, callerSlice: inputs,
		logLines: func() []string {
			logMu.Lock()
			defer logMu.Unlock()
			return append([]string(nil), logs...)
		},
	}
}

// requireContentsDropped asserts the memory-hygiene contract from the caller's
// side: the map the dispatcher's copies share is empty and the validated slice's
// backing array is zeroed, whether the write succeeded or failed.
func (f *inputRunnerFixture) requireContentsDropped(t *testing.T) {
	t.Helper()
	for name, content := range f.callerMap {
		if content != "" {
			t.Errorf("request inputs still hold contents for %q", name)
		}
	}
	for i, input := range f.callerSlice {
		if !bytes.Equal(input.Content, make([]byte, len(input.Content))) {
			t.Errorf("validated input %d still holds contents", i)
		}
	}
}

func TestRunnerWritesSuppliedInputsBeforeTheSpawn(t *testing.T) {
	secret := []byte("# Netscape HTTP Cookie File\n.youtube.com\tTRUE\t/\tTRUE\t0\tSID\tsecret\n")
	f := newInputRunnerFixture(t, InputFile{Name: "cookies.txt", Content: append([]byte(nil), secret...)})
	callerMap := f.callerMap
	callerSlice := f.callerSlice

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
	// The record keeps the name and the size, and the surfaces built from it
	// (the admin history, the JSON API, the application log) carry nothing else.
	if len(record.Inputs) != 1 || record.Inputs[0].Name != "cookies.txt" || record.Inputs[0].Bytes != int64(len(secret)) {
		t.Fatalf("recorded inputs = %+v", record.Inputs)
	}
	encodedRecord, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	encodedOutput, err := json.Marshal(RunView{RunRecord: record, Output: output})
	if err != nil {
		t.Fatal(err)
	}
	for name, encoded := range map[string][]byte{"run record JSON": encodedRecord, "run view JSON": encodedOutput} {
		if strings.Contains(string(encoded), "SID") {
			t.Fatalf("%s leaked the supplied contents: %s", name, encoded)
		}
	}
	for _, line := range f.logLines() {
		if strings.Contains(line, "SID") {
			t.Fatalf("the application log leaked the supplied contents: %q", line)
		}
	}
	if !strings.Contains(output.OutputTail, "read-ok") {
		t.Fatalf("output = %q, want the fixture token", output.OutputTail)
	}

	// Contents are dropped once they are on disk: the map the dispatcher's
	// copies share and the slice's backing array both lose them.
	if callerMap == nil || callerSlice == nil {
		t.Fatal("the fixture did not expose the shared copies")
	}
	f.requireContentsDropped(t)
}

func TestRunnerRedactsSensitiveParameterAndSuppliedInputFromStdoutAndStderr(t *testing.T) {
	const inputSecret = "COOKIE_CORPUS=runner-input-secret-9a31"
	const parameterSecret = "https://user:password@media.example.invalid/file?token=runner-token-48fd"
	f := newInputRunnerFixture(t, InputFile{Name: "cookies.txt", Content: []byte(inputSecret)})
	f.settings.commandDir = "/bin"
	f.executor.deps.Settings = f.settings
	declaration := Declaration{
		Name: "echo-secrets", Timeout: 10 * time.Second,
		Argv:            []string{"sh", "-c", "cat cookies.txt; cat cookies.txt >&2; printf '%s\\n' \"$1\"; printf '%s\\n' \"$1\" >&2; printf 'safe-diagnostic\\n'", "command", "{{token}}"},
		SensitiveParams: []string{"token"}, Inputs: []string{"cookies.txt"},
	}
	params := map[string]string{"token": parameterSecret}
	invocation, err := BuildInvocation(declaration, params, f.exchange)
	if err != nil {
		t.Fatal(err)
	}
	f.run.Request.Declaration = declaration
	f.run.Request.Params = params
	f.run.Invocation = invocation
	outcome := f.executor.Execute(context.Background(), f.run)
	if outcome.Status != RunStatusSucceeded {
		t.Fatalf("outcome = %+v", outcome)
	}

	_, output, err := f.store.Run(f.run.RunID)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{inputSecret, parameterSecret} {
		if strings.Contains(output.OutputTail, secret) {
			t.Fatalf("captured output persisted %q: %q", secret, output.OutputTail)
		}
	}
	if strings.Count(output.OutputTail, "[redacted]") < 4 || !strings.Contains(output.OutputTail, "safe-diagnostic") {
		t.Fatalf("output tail did not redact the echoes and keep ordinary diagnostics: %q", output.OutputTail)
	}
}

func TestRunnerRedactsSuppliedInputAfterTerminalControlsAreStripped(t *testing.T) {
	const printableSecret = "CONTROLLEDinput-corpus-34b1"
	f := newInputRunnerFixture(t, InputFile{Name: "cookies.txt", Content: []byte("CONTROLLED\x00input-corpus-34b1")})
	f.settings.commandDir = "/bin"
	f.executor.deps.Settings = f.settings
	declaration := Declaration{
		Name: "echo-controlled-input", Timeout: 10 * time.Second,
		Argv:   []string{"sh", "-c", "cat cookies.txt; printf 'safe-diagnostic\\n'", "command"},
		Inputs: []string{"cookies.txt"},
	}
	invocation, err := BuildInvocation(declaration, nil, f.exchange)
	if err != nil {
		t.Fatal(err)
	}
	f.run.Request.Declaration = declaration
	f.run.Invocation = invocation
	outcome := f.executor.Execute(context.Background(), f.run)
	if outcome.Status != RunStatusSucceeded {
		t.Fatalf("outcome = %+v", outcome)
	}

	_, output, err := f.store.Run(f.run.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.OutputTail, printableSecret) {
		t.Fatalf("terminal filtering exposed normalized secret bytes: %q", output.OutputTail)
	}
	if !strings.Contains(output.OutputTail, "safe-diagnostic") {
		t.Fatalf("output tail lost the safe diagnostic: %q", output.OutputTail)
	}
}

func TestRunnerRedactsSecretBeforeApplyingThePersistedTailBoundary(t *testing.T) {
	const inputSecret = "boundary-secret-74fd"
	f := newInputRunnerFixture(t, InputFile{Name: "cookies.txt", Content: []byte(inputSecret)})
	f.settings.commandDir = "/bin"
	f.executor.deps.Settings = f.settings
	declaration := Declaration{
		Name: "echo-boundary", Timeout: 10 * time.Second,
		Argv:   []string{"sh", "-c", "i=0; while [ $i -lt 100 ]; do printf x; i=$((i + 1)); done; cat cookies.txt; i=0; while [ $i -lt 65503 ]; do printf x; i=$((i + 1)); done; printf 'tail-diagnostic\\n'", "command"},
		Inputs: []string{"cookies.txt"},
	}
	invocation, err := BuildInvocation(declaration, nil, f.exchange)
	if err != nil {
		t.Fatal(err)
	}
	f.run.Request.Declaration = declaration
	f.run.Invocation = invocation
	outcome := f.executor.Execute(context.Background(), f.run)
	if outcome.Status != RunStatusSucceeded {
		t.Fatalf("outcome = %+v", outcome)
	}

	_, output, err := f.store.Run(f.run.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.OutputTail, inputSecret) || strings.Contains(output.OutputTail, "ndary-secret-74fd") {
		t.Fatalf("a secret crossing the old tail boundary remained visible: %q", output.OutputTail[:min(len(output.OutputTail), 100)])
	}
	if !strings.Contains(output.OutputTail, "tail-diagnostic") {
		t.Fatalf("output tail lost the safe diagnostic: %q", output.OutputTail)
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

	// A failed write must not leave the bytes in memory either: the failure
	// return happens before the all-success path used to reach the drop.
	f.requireContentsDropped(t)
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
	f.requireContentsDropped(t)
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
