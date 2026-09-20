//go:build darwin

package plugin_commands

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

func TestDarwinInspectorTreatsZombieOnlyGroupAsDead(t *testing.T) {
	assertNativeZombieOnlyGroupIsDead(t)
}

type darwinNativeSignalShim struct {
	mu                   sync.Mutex
	killCalls            int
	pgid                 int
	sawUnverified        bool
	secondAttemptStarted chan struct{}
	releaseSecondAttempt chan struct{}
}

func (i *darwinNativeSignalShim) InspectGroup(pgid int, runID string) (GroupIdentity, error) {
	identity, err := (nativeProcessInspector{}).InspectGroup(pgid, runID)
	i.mu.Lock()
	if err == nil && identity.State == GroupAliveUnverified {
		i.sawUnverified = true
	}
	i.mu.Unlock()
	return identity, err
}

func (i *darwinNativeSignalShim) KillGroup(pgid int) error {
	i.mu.Lock()
	i.killCalls++
	i.pgid = pgid
	attempt := i.killCalls
	i.mu.Unlock()
	if attempt == 1 {
		// Leave the environment-scrubbed member alive so Darwin's native
		// inspector, rather than a fabricated state, authorizes the re-signal.
		return nil
	}
	if attempt == 2 {
		close(i.secondAttemptStarted)
		<-i.releaseSecondAttempt
		return (nativeProcessInspector{}).KillGroup(pgid)
	}
	return errors.New("unexpected third Darwin process-group signal")
}

func (i *darwinNativeSignalShim) snapshot() (int, int, bool) {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.killCalls, i.pgid, i.sawUnverified
}

func TestDarwinRunnerResignalApplePlatformUnverifiedGroupOnce(t *testing.T) {
	root := t.TempDir()
	store := newRunnerTestStore()
	settings := runnerTestSettings{root: root, commandDir: "/bin", perRun: 1 << 20, global: 1 << 21}
	inspector := &darwinNativeSignalShim{
		secondAttemptStarted: make(chan struct{}),
		releaseSecondAttempt: make(chan struct{}),
	}
	executor := NewExecutor(RunnerDependencies{Store: store, Settings: settings, Inspector: inspector})
	configured := executor.(*commandExecutor)
	configured.pollInterval = 5 * time.Millisecond
	configured.stuckPollInterval = 40 * time.Millisecond
	configured.cleanupTimeout = 20 * time.Millisecond
	run := seedRunnerRun(t, executor, store, settings, "darwin-unverified", []string{
		"sh", "-c", `/usr/bin/env -i PATH=/bin:/usr/bin /bin/sleep 600 & printf %s $! > "$1/descendant.pid"`, "sh", "{{exchange_dir}}",
	}, 5*time.Second)

	result := make(chan Outcome, 1)
	go func() { result <- executor.Execute(context.Background(), run) }()
	pid := waitForHelperPID(t, filepath.Join(run.ExchangeDir, "descendant.pid"))
	defer func() { _ = syscall.Kill(pid, syscall.SIGKILL) }()

	select {
	case <-inspector.secondAttemptStarted:
	case <-time.After(2 * time.Second):
		kills, pgid, sawUnverified := inspector.snapshot()
		close(inspector.releaseSecondAttempt)
		if pgid > 0 {
			_ = syscall.Kill(-pgid, syscall.SIGKILL)
		}
		t.Fatalf("Darwin native cleanup observations before re-signal: kills=%d pgid=%d unverified=%t", kills, pgid, sawUnverified)
	}
	kills, pgid, sawUnverified := inspector.snapshot()
	if kills != 2 || !sawUnverified || pgid <= 0 {
		close(inspector.releaseSecondAttempt)
		if pgid > 0 {
			_ = syscall.Kill(-pgid, syscall.SIGKILL)
		}
		t.Fatalf("Darwin native cleanup observations: kills=%d pgid=%d unverified=%t", kills, pgid, sawUnverified)
	}
	requireRunnerStillBlocked(t, result, "the native Darwin inspector still observes the scrubbed descendant before the re-signal")
	close(inspector.releaseSecondAttempt)
	select {
	case outcome := <-result:
		if outcome.Status != RunStatusFailed || outcome.AuthoritativeStatus != RunStatusFailed {
			t.Fatalf("outcome = %+v", outcome)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("runner did not publish after native Darwin inspection observed group death")
	}
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
