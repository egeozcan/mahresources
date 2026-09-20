//go:build darwin

package plugin_commands

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestDarwinInspectorTreatsZombieOnlyGroupAsDead(t *testing.T) {
	assertNativeZombieOnlyGroupIsDead(t)
}

func TestDarwinRunnerKillsApplePlatformDescendantBeforeTerminalPublication(t *testing.T) {
	root := t.TempDir()
	store := newRunnerTestStore()
	settings := runnerTestSettings{root: root, commandDir: "/bin", perRun: 1 << 20, global: 1 << 21}
	executor := NewExecutor(RunnerDependencies{Store: store, Settings: settings})
	executor.(*commandExecutor).cleanupTimeout = 100 * time.Millisecond
	run := seedRunnerRun(t, executor, store, settings, "apple-platform", []string{
		"sh", "-c", `sleep 600 & printf %s $! > "$1/descendant.pid"; wait`, "sh", "{{exchange_dir}}",
	}, 5*time.Second)

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan Outcome, 1)
	go func() { result <- executor.Execute(ctx, run) }()
	pid := waitForHelperPID(t, filepath.Join(run.ExchangeDir, "descendant.pid"))
	defer func() { _ = syscall.Kill(pid, syscall.SIGKILL) }()
	cancel()

	select {
	case outcome := <-result:
		if outcome.Status != RunStatusCancelled || outcome.AuthoritativeStatus != RunStatusCancelled {
			t.Fatalf("outcome = %+v", outcome)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("runner did not publish cancellation after terminating Apple platform process group")
	}
	if err := syscall.Kill(pid, 0); err == nil || !errors.Is(err, syscall.ESRCH) {
		t.Fatalf("Apple platform descendant %d survived terminal publication: %v", pid, err)
	}
	record, _, err := store.Run(run.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if record.OutputUnverified || strings.Contains(record.Error, "ownership could not be verified") {
		t.Fatalf("locally created group was treated as recovery identity: %+v", record)
	}
	if _, err := os.Stat(filepath.Join(run.ExchangeDir, "descendant.pid")); err != nil {
		t.Fatal(err)
	}
}
