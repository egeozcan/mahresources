//go:build !windows

package application_context

import (
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

	"mahresources/models"
	"mahresources/plugin_commands"
)

// commandInputInspector is only needed because Dispatcher.Recover refuses to run
// without one. A run killed during input writing has no process group recorded,
// so recovery classifies it without inspecting anything.
type commandInputInspector struct{}

func (commandInputInspector) InspectGroup(int, string) (plugin_commands.GroupIdentity, error) {
	return plugin_commands.GroupIdentity{State: plugin_commands.GroupDead}, nil
}

func (commandInputInspector) KillGroup(int) error { return nil }

// commandInputHarness wires the real store, the real runner and a real
// dispatcher together: the acceptance tests in the spec are about what these
// three do to each other, so a fake store would not test the claim.
type commandInputHarness struct {
	ctx        *MahresourcesContext
	root       string
	settings   testPluginCommandSettings
	usage      *plugin_commands.StagingUsageCache
	dispatcher *plugin_commands.Dispatcher
	logs       func() []string
}

func newCommandInputHarness(t *testing.T) *commandInputHarness {
	t.Helper()
	ctx := newPluginCommandStoreTestContext(t)
	root := t.TempDir()
	settings := testPluginCommandSettings{root: root, commandPath: "/bin"}
	usage := plugin_commands.NewStagingUsageCache()
	if err := usage.Refresh(root); err != nil {
		t.Fatal(err)
	}
	var logMu sync.Mutex
	var logLines []string
	executor := plugin_commands.NewExecutor(plugin_commands.RunnerDependencies{
		Store: ctx, Settings: settings, Usage: usage,
		Inspector: commandInputInspector{},
		Logf: func(format string, args ...any) {
			logMu.Lock()
			defer logMu.Unlock()
			logLines = append(logLines, fmt.Sprintf(format, args...))
		},
	})
	harness := &commandInputHarness{
		ctx: ctx, root: root, settings: settings, usage: usage,
		logs: func() []string {
			logMu.Lock()
			defer logMu.Unlock()
			return append([]string(nil), logLines...)
		},
	}
	harness.dispatcher = harness.newDispatcher(t, executor, lifecycleAsyncJobs{})
	return harness
}

func (h *commandInputHarness) newDispatcher(t *testing.T, executor plugin_commands.Executor, jobs plugin_commands.LiveJobs) *plugin_commands.Dispatcher {
	t.Helper()
	dispatcher := plugin_commands.NewDispatcher(plugin_commands.Dependencies{
		Store: h.ctx, Jobs: jobs, Executor: executor, Settings: h.settings,
		Usage: h.usage, Inspector: commandInputInspector{},
	})
	if err := dispatcher.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = dispatcher.Stop(stopCtx)
	})
	return dispatcher
}

// sweepOnlyDispatcher is a second dispatcher over the same store, as an
// operator's restart would be. It has to share the harness's staging usage
// cache, because the dispatcher refuses an executor that brings its own.
func (h *commandInputHarness) sweepOnlyDispatcher(t *testing.T) {
	t.Helper()
	h.newDispatcher(t, plugin_commands.NewExecutor(plugin_commands.RunnerDependencies{
		Store: h.ctx, Settings: h.settings, Usage: h.usage, Inspector: commandInputInspector{},
	}), lifecycleAsyncJobs{})
}

func (h *commandInputHarness) exchangeDir(runID string) string {
	return filepath.Join(h.root, "plugin_exchange", "plug", runID)
}

func (h *commandInputHarness) waitTerminal(t *testing.T, runID, want string) plugin_commands.RunRecord {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		record, _, err := h.ctx.Run(runID)
		if err != nil {
			t.Fatal(err)
		}
		if plugin_commands.RunStatusTerminal(record.Status) {
			if record.Status != want {
				t.Fatalf("run %s = %q (%s), want %q", runID, record.Status, record.Error, want)
			}
			return record
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("run %s never reached a terminal state", runID)
	return plugin_commands.RunRecord{}
}

// readInputDeclaration hands the program a file in its working directory and
// copies it to seen.txt, so a successful run proves the bytes arrived without
// putting them anywhere a record could keep.
func readInputDeclaration() plugin_commands.Declaration {
	return plugin_commands.Declaration{
		Name: "read", Timeout: 30 * time.Second,
		Argv:   []string{"sh", "-c", "cat cookies.txt > seen.txt; printf read-ok"},
		Inputs: []string{"cookies.txt"},
	}
}

func TestPluginCommandInputsAgainstTheRealStore(t *testing.T) {
	harness := newCommandInputHarness(t)
	secret := "# Netscape HTTP Cookie File\nSID=real-store-secret\n"

	runID, err := harness.dispatcher.Submit(plugin_commands.CommandRequest{
		PluginName: "plug", Declaration: readInputDeclaration(),
		Inputs: map[string]string{"cookies.txt": secret},
	})
	if err != nil {
		t.Fatal(err)
	}
	record := harness.waitTerminal(t, runID, plugin_commands.RunStatusSucceeded)

	// The program read exactly the supplied bytes, and the file is a private
	// regular file of the recorded size.
	dir := harness.exchangeDir(runID)
	seen, err := os.ReadFile(filepath.Join(dir, "seen.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(seen) != secret {
		t.Fatalf("program read %q", seen)
	}
	info, err := os.Lstat(filepath.Join(dir, "cookies.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 || info.Size() != int64(len(secret)) {
		t.Fatalf("supplied input = %v, %d bytes", info.Mode(), info.Size())
	}

	// The durable row keeps the name and the byte count, and no surface built
	// from it — the stored row, the output row, the run list the JSON API and the
	// admin history read — contains the contents.
	_, storedOutput, err := harness.ctx.Run(runID)
	if err != nil {
		t.Fatal(err)
	}
	if len(record.Inputs) != 1 || record.Inputs[0].Name != "cookies.txt" || record.Inputs[0].Bytes != int64(len(secret)) {
		t.Fatalf("recorded inputs = %+v", record.Inputs)
	}
	var stored models.PluginCommandRun
	if err := harness.ctx.db.First(&stored, "id = ?", runID).Error; err != nil {
		t.Fatal(err)
	}
	wantInputsJSON := fmt.Sprintf(`[{"name":"cookies.txt","bytes":%d}]`, len(secret))
	if stored.InputsJSON != wantInputsJSON {
		t.Fatalf("stored inputs JSON = %q, want %q", stored.InputsJSON, wantInputsJSON)
	}
	views, err := harness.ctx.Runs(plugin_commands.Access{PluginName: "plug", Administrator: true})
	if err != nil {
		t.Fatal(err)
	}
	for name, value := range map[string]any{"run row": record, "output row": storedOutput, "run list": views} {
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(encoded), "real-store-secret") {
			t.Fatalf("%s leaked the supplied contents: %s", name, encoded)
		}
	}
	if strings.Contains(storedOutput.OutputTail, "real-store-secret") {
		t.Fatalf("output tail leaked the supplied contents: %q", storedOutput.OutputTail)
	}
	if !strings.Contains(storedOutput.OutputTail, "read-ok") {
		t.Fatalf("output tail = %q", storedOutput.OutputTail)
	}
	for _, line := range harness.logs() {
		if strings.Contains(line, "real-store-secret") {
			t.Fatalf("the application log leaked the supplied contents: %q", line)
		}
	}

	// Discarding through the ordinary exchange surface removes the file the
	// plugin was handed, and reading it afterwards reports it as gone.
	exchange := plugin_commands.NewExchange(harness.ctx, harness.settings)
	access := plugin_commands.Access{PluginName: "plug", Administrator: true}
	if err := exchange.Discard(access, runID, "cookies.txt"); err != nil {
		t.Fatalf("Discard: %v", err)
	}
	if _, err := exchange.Read(access, runID, "cookies.txt", 1024); !errors.Is(err, plugin_commands.ErrExchangeFileNotFound) {
		t.Fatalf("Read after Discard = %v", err)
	}
}

func TestPluginCommandInputsAreSweptWithTheExpiredFolderAgainstTheRealStore(t *testing.T) {
	harness := newCommandInputHarness(t)
	runID, err := harness.dispatcher.Submit(plugin_commands.CommandRequest{
		PluginName: "plug", Declaration: readInputDeclaration(),
		Inputs: map[string]string{"cookies.txt": "SID=sweep-me"},
	})
	if err != nil {
		t.Fatal(err)
	}
	harness.waitTerminal(t, runID, plugin_commands.RunStatusSucceeded)

	// Age the run past the exchange retention window; the next dispatcher's
	// startup sweep is what an operator's restart would do.
	if err := harness.ctx.db.Exec(
		"UPDATE plugin_command_runs SET finished_at = ?, exchange_removed_at = NULL WHERE id = ?",
		time.Now().UTC().Add(-DefaultPluginCommandExchangeRetention-time.Hour), runID,
	).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(harness.exchangeDir(runID), "cookies.txt")); err != nil {
		t.Fatalf("supplied input vanished before the sweep: %v", err)
	}
	harness.sweepOnlyDispatcher(t)

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Lstat(harness.exchangeDir(runID)); errors.Is(err, os.ErrNotExist) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if _, err := os.Lstat(harness.exchangeDir(runID)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expired folder with supplied inputs remains: %v", err)
	}
	record, _, err := harness.ctx.Run(runID)
	if err != nil {
		t.Fatal(err)
	}
	if record.ExchangeRemovedAt == nil {
		t.Fatal("the sweep did not stamp exchange_removed_at")
	}
}

func TestPluginCommandUnknownProcessGroupDuringInputWriteRemainsQuarantined(t *testing.T) {
	harness := newCommandInputHarness(t)
	const runID = "killed-mid-write"
	now := time.Now().UTC()
	if err := harness.ctx.CreateRun(plugin_commands.RunRecord{
		ID: runID, PluginName: "plug", CommandName: "read", ParamsJSON: "{}",
		Inputs: []plugin_commands.SuppliedInput{{Name: "cookies.txt", Bytes: 9}},
		Status: plugin_commands.RunStatusQueued, CreatedAt: now,
	}, plugin_commands.RunOutput{RunID: runID, ArgvJSON: "[]", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	// The exact row a kill during input writing leaves: running, marked before
	// the write, with no process group and no boot session recorded because the
	// spawn had not happened yet.
	if won, err := harness.ctx.MarkRunRunning(runID, now); err != nil || !won {
		t.Fatalf("mark running: won=%v err=%v", won, err)
	}
	dir := harness.exchangeDir(runID)
	if err := os.MkdirAll(filepath.Join(dir, ".tmp"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".tmp", "input-1234"), []byte("SID=half"), 0o600); err != nil {
		t.Fatal(err)
	}

	var blocked *plugin_commands.RecoveryBlockedError
	if err := harness.dispatcher.Recover(context.Background()); !errors.As(err, &blocked) {
		t.Fatalf("Recover error = %v, want unknown process-group blocker", err)
	}
	record, _, err := harness.ctx.Run(runID)
	if err != nil {
		t.Fatal(err)
	}
	if record.Status != plugin_commands.RunStatusRunning || record.OutputUnverified {
		t.Fatalf("recovered row = status %q unverified=%v err=%q", record.Status, record.OutputUnverified, record.Error)
	}
	// The durable metadata remains available for diagnosis while the uncertain
	// process may still own this run.
	if len(record.Inputs) != 1 || record.Inputs[0].Name != "cookies.txt" || record.Inputs[0].Bytes != 9 {
		t.Fatalf("recovered row inputs = %+v", record.Inputs)
	}
	if _, err := os.Lstat(filepath.Join(dir, ".tmp", "input-1234")); err != nil {
		t.Fatalf("unproven live work was swept: %v", err)
	}
}

func TestPluginCommandQueuedCancellationWritesNoInputAgainstTheRealStore(t *testing.T) {
	harness := newCommandInputHarness(t)
	declaration := plugin_commands.Declaration{
		Name: "sleep", Timeout: 30 * time.Second,
		Argv:   []string{"sh", "-c", "sleep 20"},
		Inputs: []string{"cookies.txt"},
	}
	submitted := make([]string, 0, 3)
	for i := 0; i < 3; i++ {
		runID, err := harness.dispatcher.Submit(plugin_commands.CommandRequest{
			PluginName: "plug", Declaration: declaration,
			Inputs: map[string]string{"cookies.txt": "SID=queued"},
		})
		if err != nil {
			t.Fatal(err)
		}
		submitted = append(submitted, runID)
	}
	// The plugin's lane holds two commands, so the third waits in the queue.
	queued := submitted[2]
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		record, _, err := harness.ctx.Run(queued)
		if err != nil {
			t.Fatal(err)
		}
		if record.Status == plugin_commands.RunStatusQueued || record.Status == plugin_commands.RunStatusRunning {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err := harness.dispatcher.Cancel(queued, "operator cancelled"); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	record := harness.waitTerminal(t, queued, plugin_commands.RunStatusCancelled)
	if record.StartedAt != nil {
		t.Fatalf("a cancelled queued run was started at %s", record.StartedAt)
	}
	if _, err := os.Lstat(filepath.Join(harness.exchangeDir(queued), "cookies.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a queued cancellation wrote its input: %v", err)
	}
	for _, runID := range submitted[:2] {
		_ = harness.dispatcher.Cancel(runID, "test cleanup")
	}
}
