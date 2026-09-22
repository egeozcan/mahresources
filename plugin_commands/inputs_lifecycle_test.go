//go:build !windows

package plugin_commands

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestSuppliedInputsCountTowardTheGlobalStagingSample pins spec §9.5's second
// half: the bytes occupy the staging root exactly like command output does, so
// the sample the admission check reads has to grow by them. The sample is
// refreshed at startup and after each sweep, never per run.
func TestSuppliedInputsCountTowardTheGlobalStagingSample(t *testing.T) {
	secret := []byte("SID=counts-toward-staging")
	f := newInputRunnerFixture(t, InputFile{Name: "cookies.txt", Content: append([]byte(nil), secret...)})
	if outcome := f.executor.Execute(context.Background(), f.run); outcome.Status != RunStatusSucceeded {
		t.Fatalf("outcome = %+v", outcome)
	}
	usage := f.executor.stagingUsageCache()
	if err := usage.Refresh(f.settings.root); err != nil {
		t.Fatal(err)
	}
	sample, err := usage.Current()
	if err != nil {
		t.Fatal(err)
	}
	if sample < int64(len(secret)) {
		t.Fatalf("global staging sample = %d, want at least the %d supplied bytes", sample, len(secret))
	}
}

// TestSuppliedInputsAreDiscardableAndSweptWithTheFolder pins spec §9.8: the
// plugin can discard one supplied file through the ordinary exchange surface,
// and the retention sweep removes the whole folder — inputs included — on
// schedule.
func TestSuppliedInputsAreDiscardableAndSweptWithTheFolder(t *testing.T) {
	f := newInputRunnerFixture(t, InputFile{Name: "cookies.txt", Content: []byte("SID=discard-me")})
	if outcome := f.executor.Execute(context.Background(), f.run); outcome.Status != RunStatusSucceeded {
		t.Fatalf("outcome = %+v", outcome)
	}
	exchange := NewExchange(f.store, f.settings)
	access := Access{PluginName: "plug", Administrator: true}

	if err := exchange.Discard(access, f.run.RunID, "cookies.txt"); err != nil {
		t.Fatalf("Discard: %v", err)
	}
	if _, err := exchange.Read(access, f.run.RunID, "cookies.txt", 1024); !errors.Is(err, ErrExchangeFileNotFound) {
		t.Fatalf("Read after Discard = %v, want %v", err, ErrExchangeFileNotFound)
	}
	if _, err := os.Lstat(filepath.Join(f.exchange, "cookies.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("discarded input is still on disk: %v", err)
	}

	// The whole folder goes on schedule, with whatever is left in it.
	root := t.TempDir()
	now := time.Now().UTC()
	store := &lifecycleStore{
		dispatcherTestStore: newDispatcherTestStore(),
		nonterminal:         map[string]bool{},
		expired:             []RunRecord{{ID: "expired-inputs", PluginName: "plugin", Status: RunStatusSucceeded}},
	}
	folder := filepath.Join(root, "plugin_exchange", "plugin", "expired-inputs")
	if err := os.MkdirAll(filepath.Join(folder, ".tmp"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(folder, "cookies.txt"), []byte("SID=sweep-me"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(folder, ".tmp", "input-crash"), []byte("SID=half"), 0o600); err != nil {
		t.Fatal(err)
	}
	d := NewDispatcher(Dependencies{Store: store, Settings: lifecycleSettings{root: root, exchange: time.Hour, output: time.Hour}})
	if err := d.sweep(now); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(folder); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expired folder with supplied inputs remains: %v", err)
	}
	if got := strings.Join(store.exchangeMarked, ","); got != "expired-inputs" {
		t.Fatalf("marked runs = %q, want expired-inputs", got)
	}
}

// TestRecoverySettlesARunLeftMidWrite pins spec §9.9 at the state level: a
// process killed between the scratch create and the rename leaves a durable
// nonterminal row, a folder with a partial scratch file, and no file at the
// declared name. Recovery settles the row without a new stuck state, and the
// declared name is simply absent rather than half-written.
func TestRecoverySettlesARunLeftMidWrite(t *testing.T) {
	root := t.TempDir()
	const runID = "left-mid-write"
	store := &recoveryStore{dispatcherTestStore: newDispatcherTestStore()}
	// The exact identity a kill during input writing leaves: running, marked
	// before the write, with no process group recorded because the spawn had not
	// happened yet. That is what sends recovery down the unverified branch.
	store.runs[runID] = RunRecord{ID: runID, PluginName: "plug", Status: RunStatusRunning}
	folder := filepath.Join(root, "plugin_exchange", "plug", runID)
	if err := os.MkdirAll(filepath.Join(folder, ".tmp"), 0o700); err != nil {
		t.Fatal(err)
	}
	// The half-written scratch the kill would leave, and no declared name.
	if err := os.WriteFile(filepath.Join(folder, ".tmp", "input-1234"), []byte("SID=half-writ"), 0o600); err != nil {
		t.Fatal(err)
	}
	settings := lifecycleSettings{root: root, exchange: time.Hour, output: time.Hour}
	d := NewDispatcher(Dependencies{
		Store: store, Settings: settings,
		Inspector: &recoveryInspector{states: map[int][]GroupIdentity{}},
	})
	if err := d.Recover(context.Background()); err != nil {
		t.Fatalf("Recover: %v", err)
	}
	record, _, err := store.Run(runID)
	if err != nil {
		t.Fatal(err)
	}
	if record.Status != RunStatusInterrupted {
		t.Fatalf("recovered status = %q, want %q", record.Status, RunStatusInterrupted)
	}
	if !record.OutputUnverified {
		t.Fatal("a run killed before its process group was recorded must be marked output-unverified")
	}

	// The settled run is terminal, so the read path can answer: the declared
	// name does not exist, and because recovery marked the output unverified the
	// folder is refused outright rather than read.
	exchange := NewExchange(store, settings)
	access := Access{PluginName: "plug", Administrator: true}
	if _, err := exchange.Read(access, runID, "cookies.txt", 1024); !errors.Is(err, ErrExchangeOutputUnverified) {
		t.Fatalf("Read of a name that was never renamed = %v, want %v", err, ErrExchangeOutputUnverified)
	}
	if _, err := exchange.Read(access, runID, ".tmp", 1024); err == nil {
		t.Fatal("the scratch directory is readable as an input")
	}
}
