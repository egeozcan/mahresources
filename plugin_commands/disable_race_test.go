//go:build linux || darwin

package plugin_commands

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func runCancellationBarrierCase(t *testing.T, afterStart, afterCancellationCheck, disable bool) {
	t.Helper()
	root, commandDir := t.TempDir(), t.TempDir()
	helperExecutable(t, commandDir, "mah-helper")
	store := newRunnerTestStore()
	settings := runnerTestSettings{root: root, commandDir: commandDir, pending: 10, perRun: 1 << 20, global: 1 << 21}
	executor := NewExecutor(RunnerDependencies{Store: store, Settings: settings}).(*commandExecutor)
	reached, release := make(chan struct{}), make(chan struct{})
	barrier := func() { close(reached); <-release }
	if afterStart {
		executor.afterStart = barrier
	} else if afterCancellationCheck {
		executor.afterCancellationCheck = barrier
	} else {
		executor.beforeStart = barrier
	}
	jobs := &dispatcherTestJobs{}
	d := NewDispatcher(Dependencies{Store: store, Jobs: jobs, Executor: executor, Settings: settings})
	if err := d.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	declaration := Declaration{Name: "sleep", Argv: []string{"mah-helper", helperProcessFlag, "sleep"}, Timeout: time.Minute}
	runID, err := d.Submit(CommandRequest{PluginName: "p", PluginGeneration: 1, Declaration: declaration})
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return jobs.commandCount() == 1 })
	registered := jobs.commandSnapshot()[0]
	outcome := make(chan Outcome, 1)
	go func() { outcome <- registered.run(context.Background(), nopProgress{}) }()
	select {
	case <-reached:
	case <-time.After(time.Second):
		t.Fatal("runner did not reach cancellation barrier")
	}

	cancelResult := make(chan error, 1)
	go func() {
		if disable {
			cancelResult <- d.DisablePlugin("p", "plugin disabled")
		} else {
			cancelResult <- d.Cancel(runID, "operator cancelled")
		}
	}()
	select {
	case err := <-cancelResult:
		t.Fatalf("cancellation returned before the fork barrier was released: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	close(release)
	if err := <-cancelResult; err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-outcome:
		if got.Status != RunStatusCancelled {
			t.Fatalf("outcome = %+v", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cancelled command did not finish")
	}
	record, _, err := store.Run(runID)
	if err != nil {
		t.Fatal(err)
	}
	wantReason := "operator cancelled"
	if disable {
		wantReason = "plugin disabled"
	}
	if record.Status != RunStatusCancelled || record.Error != wantReason {
		t.Fatalf("record = %+v", record)
	}
	spawnWon := afterStart || afterCancellationCheck
	if !spawnWon && record.ProcessGroupID != nil {
		t.Fatalf("pre-fork cancellation spawned process group %d", *record.ProcessGroupID)
	}
	if spawnWon && record.ProcessGroupID == nil {
		t.Fatal("post-fork cancellation finished before pgid persistence")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := d.Stop(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestCancelImmediatelyBeforeForkPreventsSpawn(t *testing.T) {
	runCancellationBarrierCase(t, false, false, false)
}

func TestDisableImmediatelyBeforeForkPreventsSpawn(t *testing.T) {
	runCancellationBarrierCase(t, false, false, true)
}

func TestCancelCannotCommitInsideTheForkCriticalSection(t *testing.T) {
	runCancellationBarrierCase(t, false, true, false)
}

func TestDisableCannotCommitInsideTheForkCriticalSection(t *testing.T) {
	runCancellationBarrierCase(t, false, true, true)
}

func TestCancelAfterForkDoesNotWaitForProcessGroupPersistence(t *testing.T) {
	runCancellationBarrierCase(t, true, false, false)
}

func TestDisableAfterForkDoesNotWaitForProcessGroupPersistence(t *testing.T) {
	runCancellationBarrierCase(t, true, false, true)
}

func TestDisableCancelsQueuedImportsButLetsRunningImportsFinish(t *testing.T) {
	store := newDispatcherTestStore()
	jobs := &dispatcherTestJobs{}
	d := NewDispatcher(Dependencies{Store: store, Jobs: jobs, Executor: dispatcherTestExecutor{}, Settings: dispatcherTestSettings{pending: 10}})
	if err := d.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"running-a", "running-b", "queued-c"} {
		if err := d.submitImport(ImportJobSpec{ImportID: id, RunID: "run-" + id, PluginName: "p"}, func(context.Context, Progress) Outcome {
			return Outcome{Status: ImportStatusSucceeded}
		}); err != nil {
			t.Fatal(err)
		}
	}
	waitFor(t, func() bool { return jobs.importCount() == 2 })
	if err := d.DisablePlugin("p", "plugin disabled"); err != nil {
		t.Fatal(err)
	}
	store.mu.Lock()
	queuedFinish, queuedDone := store.importFinishes["queued-c"]
	_, firstWasCancelled := store.importFinishes["running-a"]
	_, secondWasCancelled := store.importFinishes["running-b"]
	store.mu.Unlock()
	if !queuedDone || queuedFinish.Status != ImportStatusCancelled || queuedFinish.Error != "plugin disabled" {
		t.Fatalf("queued import finish = %+v, present=%v", queuedFinish, queuedDone)
	}
	if firstWasCancelled || secondWasCancelled {
		t.Fatal("disable cancelled an import which had already entered its commit lane")
	}
	for _, registered := range jobs.importSnapshot() {
		if outcome := registered.run(context.Background(), nopProgress{}); outcome.Status != ImportStatusSucceeded {
			t.Fatalf("running import outcome = %+v", outcome)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := d.Stop(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestDisableCancelsImportWaitingInDispatchFailureRetry(t *testing.T) {
	store := newDispatcherTestStore()
	injected := errors.New("finish unavailable")
	store.finishImportErr = injected
	jobs := &dispatcherTestJobs{importErr: errors.New("managed lane full")}
	d := NewDispatcher(Dependencies{Store: store, Jobs: jobs, Executor: dispatcherTestExecutor{}, Settings: dispatcherTestSettings{pending: 10}})
	if err := d.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := d.submitImport(ImportJobSpec{ImportID: "retry-import", RunID: "run-retry-import", PluginName: "p"}, func(context.Context, Progress) Outcome {
		return Outcome{Status: ImportStatusSucceeded}
	}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool {
		store.mu.Lock()
		defer store.mu.Unlock()
		return store.finishImportCalls > 0
	})
	store.mu.Lock()
	store.finishImportErr = nil
	store.mu.Unlock()
	if err := d.DisablePlugin("p", "plugin disabled"); err != nil {
		t.Fatal(err)
	}
	store.mu.Lock()
	finish := store.importFinishes["retry-import"]
	store.mu.Unlock()
	if finish.Status != ImportStatusCancelled || finish.Error != "plugin disabled" {
		t.Fatalf("retrying import finish = %+v", finish)
	}
	if err := d.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestStopHonorsDeadlineWhileAcceptedTerminalPersistenceIsBlocked(t *testing.T) {
	store := newDispatcherTestStore()
	release := make(chan struct{})
	store.finishRunStarted = make(chan struct{}, 1)
	store.finishRunRelease = release
	jobs := &dispatcherTestJobs{}
	d := NewDispatcher(Dependencies{Store: store, Jobs: jobs, Executor: dispatcherTestExecutor{}, Settings: dispatcherTestSettings{pending: 10}})
	if err := d.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Submit(commandRequest("p", nil)); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	stopped := make(chan error, 1)
	go func() { stopped <- d.Stop(ctx) }()
	select {
	case <-store.finishRunStarted:
	case <-time.After(time.Second):
		t.Fatal("shutdown did not begin terminal persistence")
	}
	<-ctx.Done()
	if err := <-stopped; !errors.Is(err, context.DeadlineExceeded) {
		close(release)
		t.Fatalf("Stop error = %v, want deadline exceeded", err)
	}
	close(release)
	select {
	case <-d.done:
	case <-time.After(time.Second):
		t.Fatal("dispatcher did not finish after terminal persistence released")
	}
}

func TestContextDrivenShutdownPreservesPersistenceErrorForStop(t *testing.T) {
	store := newDispatcherTestStore()
	injected := errors.New("terminal write unavailable")
	store.finishRunErr = injected
	jobs := &dispatcherTestJobs{}
	d := NewDispatcher(Dependencies{Store: store, Jobs: jobs, Executor: dispatcherTestExecutor{}, Settings: dispatcherTestSettings{pending: 10}})
	ctx, cancel := context.WithCancel(context.Background())
	if err := d.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Submit(commandRequest("p", nil)); err != nil {
		t.Fatal(err)
	}
	cancel()
	select {
	case <-d.done:
	case <-time.After(time.Second):
		t.Fatal("context-driven shutdown did not finish")
	}
	if err := d.Stop(context.Background()); !errors.Is(err, injected) {
		t.Fatalf("Stop error = %v, want %v", err, injected)
	}
}

func TestShutdownPersistenceFailureSettlesEveryAdmittedCommandWithoutCallback(t *testing.T) {
	store := newDispatcherTestStore()
	injected := errors.New("terminal write unavailable")
	store.finishRunErr = injected
	jobs := &dispatcherTestJobs{}
	d := NewDispatcher(Dependencies{
		Store: store, Jobs: jobs, Executor: dispatcherTestExecutor{},
		Settings: dispatcherTestSettings{pending: 10},
	})
	if err := d.Start(context.Background()); err != nil {
		t.Fatal(err)
	}

	callbacks := make(chan Result, 3)
	ids := make([]string, 0, 3)
	for i := 0; i < 3; i++ {
		id, err := d.Submit(commandRequest("p", func(result Result) { callbacks <- result }))
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	// Two commands occupy the per-plugin managed slots; the third remains in
	// the dispatcher's private queue. Shutdown must settle both ownership paths.
	waitFor(t, func() bool { return jobs.commandCount() == 2 })

	if err := d.Stop(context.Background()); !errors.Is(err, injected) {
		t.Fatalf("Stop error = %v, want %v", err, injected)
	}
	if active := completionDispatchActive(d, "p"); active != 0 {
		t.Fatalf("shutdown persistence failure leaked %d completion lifecycles", active)
	}
	waited := make(chan struct{})
	go func() {
		d.completionDispatch.wait("p")
		close(waited)
	}()
	select {
	case <-waited:
	case <-time.After(time.Second):
		t.Fatal("a future plugin lifecycle wait hung after failed shutdown persistence")
	}
	select {
	case result := <-callbacks:
		t.Fatalf("completion without a durable terminal row: %+v", result)
	default:
	}
	for _, id := range ids {
		record, _, err := store.Run(id)
		if err != nil {
			t.Fatal(err)
		}
		if RunStatusTerminal(record.Status) {
			t.Fatalf("run %s unexpectedly became durable terminal: %+v", id, record)
		}
	}

	// The managed registry can race a late invocation with shutdown. Its refusal
	// settles the same lifecycle again; the one-shot owner makes that harmless
	// and must never resurrect a callback.
	for _, command := range jobs.commandSnapshot() {
		outcome := command.run(context.Background(), nopProgress{})
		if outcome.Status != RunStatusInterrupted {
			t.Fatalf("late managed invocation outcome = %+v", outcome)
		}
	}
	select {
	case result := <-callbacks:
		t.Fatalf("late worker refusal delivered completion: %+v", result)
	default:
	}
}

func TestActiveWorkerPersistenceFailureSuppressesCallbackAndJoinsDisable(t *testing.T) {
	store := newDispatcherTestStore()
	injected := errors.New("terminal write unavailable")
	store.finishRunErr = injected
	jobs := &dispatcherTestJobs{}
	executorEntered := make(chan struct{})
	persistenceFailed := make(chan struct{})
	releaseWorker := make(chan struct{})
	d := NewDispatcher(Dependencies{
		Store: store, Jobs: jobs, Settings: dispatcherTestSettings{pending: 10},
		Executor: dispatcherExecutorFunc(func(ctx context.Context, run QueuedRun) Outcome {
			if won, err := store.MarkRunRunning(run.RunID, time.Now().UTC()); err != nil || !won {
				t.Fatalf("MarkRunRunning() = %v, %v", won, err)
			}
			close(executorEntered)
			<-ctx.Done()
			_, err := store.FinishRun(run.RunID, RunFinish{
				Status: RunStatusCancelled, Error: "plugin disabled", FinishedAt: time.Now().UTC(),
			})
			if !errors.Is(err, injected) {
				t.Errorf("FinishRun() error = %v, want %v", err, injected)
			}
			close(persistenceFailed)
			<-releaseWorker
			return Outcome{Status: RunStatusFailed, Error: "persist terminal command: " + err.Error()}
		}),
	})
	if err := d.Start(context.Background()); err != nil {
		t.Fatal(err)
	}

	callbacks := make(chan Result, 1)
	runID, err := d.Submit(commandRequest("p", func(result Result) { callbacks <- result }))
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return jobs.commandCount() == 1 })
	workerDone := make(chan Outcome, 1)
	go func() { workerDone <- jobs.commandSnapshot()[0].run(context.Background(), nopProgress{}) }()
	select {
	case <-executorEntered:
	case <-time.After(time.Second):
		t.Fatal("managed worker did not claim completion ownership")
	}

	disabled := make(chan error, 1)
	go func() { disabled <- d.DisablePlugin("p", "plugin disabled") }()
	select {
	case <-persistenceFailed:
	case <-time.After(time.Second):
		t.Fatal("managed worker did not reach terminal persistence failure")
	}
	if active := completionDispatchActive(d, "p"); active != 1 {
		t.Fatalf("completion lifecycles after persistence failure = %d, want 1", active)
	}

	close(releaseWorker)
	select {
	case outcome := <-workerDone:
		if outcome.Status != RunStatusFailed || !strings.Contains(outcome.Error, injected.Error()) {
			t.Fatalf("worker outcome = %+v", outcome)
		}
	case <-time.After(time.Second):
		t.Fatal("managed worker did not settle")
	}
	select {
	case err := <-disabled:
		if err == nil || !strings.Contains(err.Error(), injected.Error()) {
			t.Fatalf("DisablePlugin() error = %v, want persistence failure", err)
		}
	case <-time.After(time.Second):
		t.Fatal("disable did not return after the worker settled")
	}
	select {
	case result := <-callbacks:
		t.Fatalf("callback received synthetic completion: %+v", result)
	default:
	}
	if active := completionDispatchActive(d, "p"); active != 0 {
		t.Fatalf("completion lifecycles after worker settlement = %d", active)
	}
	record, _, err := store.Run(runID)
	if err != nil {
		t.Fatal(err)
	}
	if RunStatusTerminal(record.Status) {
		t.Fatalf("failed terminal persistence was presented as durable: %+v", record)
	}
	if err := d.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestShutdownTimeoutLeavesClaimedWorkerLifecycleOwnedUntilWorkerSettles(t *testing.T) {
	store := newDispatcherTestStore()
	injected := errors.New("terminal write unavailable")
	store.finishRunErr = injected
	jobs := &dispatcherTestJobs{}
	workerEntered := make(chan struct{})
	persistenceFailed := make(chan struct{})
	releaseWorker := make(chan struct{})
	shutdownStarted := make(chan struct{})
	d := NewDispatcher(Dependencies{
		Store: store, Jobs: jobs, Settings: dispatcherTestSettings{pending: 10},
		Executor: dispatcherExecutorFunc(func(ctx context.Context, run QueuedRun) Outcome {
			if won, err := store.MarkRunRunning(run.RunID, time.Now().UTC()); err != nil || !won {
				t.Fatalf("MarkRunRunning() = %v, %v", won, err)
			}
			close(workerEntered)
			<-ctx.Done()
			_, err := store.FinishRun(run.RunID, RunFinish{
				Status: RunStatusInterrupted, Error: "server interrupted", FinishedAt: time.Now().UTC(),
			})
			if !errors.Is(err, injected) {
				t.Errorf("FinishRun() error = %v, want %v", err, injected)
			}
			close(persistenceFailed)
			<-releaseWorker
			return Outcome{Status: RunStatusFailed, Error: err.Error()}
		}),
	})
	d.shutdownStarted = func() { close(shutdownStarted) }
	if err := d.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	callbacks := make(chan Result, 1)
	if _, err := d.Submit(commandRequest("p", func(result Result) { callbacks <- result })); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return jobs.commandCount() == 1 })
	workerDone := make(chan Outcome, 1)
	go func() { workerDone <- jobs.commandSnapshot()[0].run(context.Background(), nopProgress{}) }()
	select {
	case <-workerEntered:
	case <-time.After(time.Second):
		t.Fatal("managed worker did not claim completion ownership")
	}

	stopCtx, cancelStop := context.WithCancel(context.Background())
	stopped := make(chan error, 1)
	go func() { stopped <- d.Stop(stopCtx) }()
	select {
	case <-shutdownStarted:
	case <-time.After(time.Second):
		t.Fatal("dispatcher shutdown did not start")
	}
	select {
	case <-persistenceFailed:
	case <-time.After(time.Second):
		t.Fatal("worker did not report terminal persistence failure")
	}
	cancelStop()
	select {
	case err := <-stopped:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Stop() error = %v, want context cancellation", err)
		}
	case <-time.After(time.Second):
		t.Fatal("bounded shutdown did not return")
	}
	select {
	case <-d.done:
	case <-time.After(time.Second):
		t.Fatal("dispatcher owner did not finish bounded shutdown")
	}
	if active := completionDispatchActive(d, "p"); active != 1 {
		t.Fatalf("shutdown forged claimed lifecycle drain: active=%d", active)
	}
	select {
	case result := <-callbacks:
		t.Fatalf("callback received synthetic completion: %+v", result)
	default:
	}

	close(releaseWorker)
	select {
	case <-workerDone:
	case <-time.After(time.Second):
		t.Fatal("managed worker did not return after release")
	}
	d.completionDispatch.wait("p")
	if active := completionDispatchActive(d, "p"); active != 0 {
		t.Fatalf("worker did not settle claimed lifecycle: active=%d", active)
	}
}

func TestShutdownTimeoutLeavesRunningGroupForRecovery(t *testing.T) {
	root := t.TempDir()
	exchange := filepath.Join(root, "helper-exchange")
	if err := os.MkdirAll(exchange, 0o700); err != nil {
		t.Fatal(err)
	}
	store := newDispatcherTestStore()
	jobs := &dispatcherTestJobs{}
	workerEntered := make(chan struct{})
	releaseWorker := make(chan struct{})
	spawned := make(chan *exec.Cmd, 1)
	settings := lifecycleSettings{root: root, exchange: time.Hour, output: time.Hour}
	d := NewDispatcher(Dependencies{
		Store: store, Jobs: jobs, Settings: settings,
		Executor: dispatcherExecutorFunc(func(_ context.Context, run QueuedRun) Outcome {
			cmd := exec.Command(os.Args[0], helperProcessFlag, "spawn-descendant", exchange)
			cmd.Env = append(os.Environ(), "MAHR_COMMAND_RUN_ID="+run.RunID)
			cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
			if err := cmd.Start(); err != nil {
				t.Errorf("start owned helper: %v", err)
				return Outcome{Status: RunStatusFailed, Error: err.Error()}
			}
			spawned <- cmd
			if won, err := store.MarkRunRunning(run.RunID, time.Now().UTC()); err != nil || !won {
				t.Errorf("MarkRunRunning() = %v, %v", won, err)
				return Outcome{Status: RunStatusFailed, Error: fmt.Sprint(err)}
			}
			if err := store.SetRunProcessGroup(run.RunID, cmd.Process.Pid, "test-boot-session"); err != nil {
				t.Errorf("SetRunProcessGroup: %v", err)
			}
			close(workerEntered)
			<-releaseWorker
			return Outcome{Status: RunStatusRunning}
		}),
	})
	if err := d.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	runID, err := d.Submit(CommandRequest{
		PluginName: "p", Declaration: Declaration{Name: "blocked", Argv: []string{"blocked"}, Timeout: time.Minute},
	})
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return jobs.commandCount() == 1 })
	workerDone := make(chan Outcome, 1)
	go func() { workerDone <- jobs.commandSnapshot()[0].run(context.Background(), nopProgress{}) }()
	select {
	case <-workerEntered:
	case <-time.After(time.Second):
		t.Fatal("worker did not start owned process group")
	}
	cmd := <-spawned
	pgid := cmd.Process.Pid
	waited := false
	waitDone := make(chan error, 1)
	waitStarted := false
	t.Cleanup(func() {
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
		if !waited {
			if !waitStarted {
				waitStarted = true
				go func() { waitDone <- cmd.Wait() }()
			}
			<-waitDone
		}
	})
	descendantPID := waitForHelperPID(t, filepath.Join(exchange, "descendant.pid"))

	stopCtx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	err = d.Stop(stopCtx)
	cancel()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Stop error = %v, want deadline exceeded", err)
	}
	record, _, err := store.Run(runID)
	if err != nil {
		t.Fatal(err)
	}
	if record.Status != RunStatusRunning || record.FinishedAt != nil {
		t.Fatalf("timed-out shutdown destroyed recovery evidence: %+v", record)
	}
	if err := syscall.Kill(descendantPID, 0); err != nil {
		t.Fatalf("precondition: descendant did not survive blocked worker: %v", err)
	}

	close(releaseWorker)
	select {
	case <-workerDone:
	case <-time.After(time.Second):
		t.Fatal("blocked worker did not release")
	}
	record, _, err = store.Run(runID)
	if err != nil {
		t.Fatal(err)
	}
	if record.Status != RunStatusRunning || record.FinishedAt != nil {
		t.Fatalf("worker completion changed the recovery row before Recover: %+v", record)
	}
	recovery := NewDispatcher(Dependencies{Store: store, Settings: settings})
	if err := recovery.Recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	identity, err := (nativeProcessInspector{}).InspectGroup(pgid, runID)
	if err != nil || identity.State != GroupDead {
		t.Fatalf("recovery returned before terminating descendant %d: identity=%+v err=%v", descendantPID, identity, err)
	}
	record, _, err = store.Run(runID)
	if err != nil {
		t.Fatal(err)
	}
	if record.Status != RunStatusInterrupted || record.FinishedAt == nil {
		t.Fatalf("recovery did not classify run before process wait: %+v", record)
	}
	waitStarted = true
	go func() { waitDone <- cmd.Wait() }()
	select {
	case <-waitDone:
	case <-time.After(time.Second):
		t.Fatalf("recovery left process group %d alive after returning", pgid)
	}
	waited = true
}

func TestShutdownDoesNotSettleClaimedCallbackBeforeDeliveryReturns(t *testing.T) {
	store := newDispatcherTestStore()
	jobs := &dispatcherTestJobs{}
	terminalReadStarted := make(chan struct{})
	allowTerminalRead := make(chan struct{})
	store.terminalRunReadStarted = terminalReadStarted
	store.terminalRunReadRelease = allowTerminalRead
	callbackEntered := make(chan struct{})
	allowCallback := make(chan struct{})
	shutdownStarted := make(chan struct{})
	d := NewDispatcher(Dependencies{
		Store: store, Jobs: jobs, Executor: dispatcherTestExecutor{store: store},
		Settings: dispatcherTestSettings{pending: 10},
	})
	d.shutdownStarted = func() { close(shutdownStarted) }
	if err := d.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	runID, err := d.Submit(commandRequest("p", func(Result) {
		close(callbackEntered)
		<-allowCallback
	}))
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return jobs.commandCount() == 1 })
	liveCtx, cancelLive := context.WithCancel(context.Background())
	workerDone := make(chan Outcome, 1)
	go func() { workerDone <- jobs.commandSnapshot()[0].run(liveCtx, nopProgress{}) }()
	waitFor(t, func() bool {
		record, _, readErr := store.Run(runID)
		return readErr == nil && record.Status == RunStatusRunning
	})
	cancelLive()
	select {
	case <-terminalReadStarted:
	case <-time.After(time.Second):
		t.Fatal("worker did not reach terminal-read delivery barrier")
	}

	stopped := make(chan error, 1)
	go func() { stopped <- d.Stop(context.Background()) }()
	select {
	case <-shutdownStarted:
	case <-time.After(time.Second):
		t.Fatal("dispatcher shutdown did not start")
	}
	close(allowTerminalRead)
	select {
	case <-callbackEntered:
	case <-time.After(time.Second):
		t.Fatal("completion callback did not start")
	}
	select {
	case <-workerDone:
	case <-time.After(time.Second):
		t.Fatal("managed worker did not return")
	}
	select {
	case err := <-stopped:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("shutdown did not return")
	}
	if active := completionDispatchActive(d, "p"); active != 1 {
		t.Fatalf("shutdown settled claimed callback early: active=%d", active)
	}

	close(allowCallback)
	d.completionDispatch.wait("p")
	if active := completionDispatchActive(d, "p"); active != 0 {
		t.Fatalf("callback did not settle completion lifecycle: active=%d", active)
	}
}

func TestShutdownPersistenceFailureSettlesQueuedCancellationWithoutCallback(t *testing.T) {
	store := newDispatcherTestStore()
	injected := errors.New("terminal write unavailable")
	store.finishRunErr = injected
	jobs := &dispatcherTestJobs{}
	d := NewDispatcher(Dependencies{
		Store: store, Jobs: jobs, Executor: dispatcherTestExecutor{},
		Settings: dispatcherTestSettings{pending: 10},
	})
	if err := d.Start(context.Background()); err != nil {
		t.Fatal(err)
	}

	callbacks := make(chan Result, 1)
	for i := 0; i < 2; i++ {
		if _, err := d.Submit(commandRequest("p", nil)); err != nil {
			t.Fatal(err)
		}
	}
	queued, err := d.Submit(commandRequest("p", func(result Result) { callbacks <- result }))
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return jobs.commandCount() == 2 })
	if err := d.Cancel(queued, "operator cancelled"); !errors.Is(err, injected) {
		t.Fatalf("Cancel error = %v, want %v", err, injected)
	}
	if err := d.Stop(context.Background()); !errors.Is(err, injected) {
		t.Fatalf("Stop error = %v, want %v", err, injected)
	}
	if active := completionDispatchActive(d, "p"); active != 0 {
		t.Fatalf("failed queued cancellation leaked %d completion lifecycles", active)
	}
	select {
	case result := <-callbacks:
		t.Fatalf("queued cancellation callback without durable terminal row: %+v", result)
	default:
	}
	record, _, err := store.Run(queued)
	if err != nil {
		t.Fatal(err)
	}
	if RunStatusTerminal(record.Status) {
		t.Fatalf("queued cancellation unexpectedly became terminal: %+v", record)
	}
}

func TestShutdownDoesNotDeadlockWhenWorkerCompletionInboxIsFull(t *testing.T) {
	store := newDispatcherTestStore()
	jobs := &dispatcherTestJobs{}
	d := NewDispatcher(Dependencies{Store: store, Jobs: jobs, Executor: dispatcherTestExecutor{store: store}, Settings: dispatcherTestSettings{pending: 10}})
	if err := d.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	runID, err := d.Submit(commandRequest("p", nil))
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return jobs.commandCount() == 1 })
	workerOutcome := make(chan Outcome, 1)
	go func() { workerOutcome <- jobs.commandSnapshot()[0].run(context.Background(), nopProgress{}) }()
	waitFor(t, func() bool {
		record, _, err := store.Run(runID)
		return err == nil && record.Status == RunStatusRunning
	})
	reached, release := make(chan struct{}), make(chan struct{})
	d.shutdownStarted = func() { close(reached); <-release }
	stopped := make(chan error, 1)
	go func() { stopped <- d.Stop(context.Background()) }()
	select {
	case <-reached:
	case <-time.After(time.Second):
		t.Fatal("shutdown did not reach barrier")
	}
	for i := 0; len(d.inbox) < cap(d.inbox); i++ {
		d.inbox <- importCompleted{importID: fmt.Sprintf("filler-%d", i)}
	}
	close(release)
	select {
	case err := <-stopped:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("shutdown deadlocked behind a full completion inbox")
	}
	select {
	case <-workerOutcome:
	case <-time.After(time.Second):
		t.Fatal("worker did not return after dispatcher shutdown")
	}
}

func TestShutdownCancelsRunningImportSourceAndDrainsIt(t *testing.T) {
	store := newDispatcherTestStore()
	jobs := &dispatcherTestJobs{}
	d := NewDispatcher(Dependencies{Store: store, Jobs: jobs, Executor: dispatcherTestExecutor{}, Settings: dispatcherTestSettings{pending: 10}})
	if err := d.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	if err := d.submitImport(ImportJobSpec{ImportID: "running-import", RunID: "run-running-import", PluginName: "p"}, func(ctx context.Context, _ Progress) Outcome {
		close(started)
		<-ctx.Done()
		if !errors.Is(context.Cause(ctx), errDispatcherShutdown) {
			return Outcome{Status: ImportStatusFailed, Error: "wrong cancellation cause"}
		}
		return Outcome{Status: ImportStatusInterrupted, Error: "server interrupted"}
	}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return jobs.importCount() == 1 })
	outcome := make(chan Outcome, 1)
	go func() { outcome <- jobs.importSnapshot()[0].run(context.Background(), nopProgress{}) }()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("import did not enter source reader")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := d.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-outcome:
		if got.Status != ImportStatusInterrupted {
			t.Fatalf("import outcome = %+v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("shutdown returned before import source drained")
	}
}

func TestShutdownClassifiesQueuedAndDurablyCancelledRuns(t *testing.T) {
	store := newDispatcherTestStore()
	jobs := &dispatcherTestJobs{}
	d := NewDispatcher(Dependencies{Store: store, Jobs: jobs, Executor: dispatcherTestExecutor{}, Settings: dispatcherTestSettings{pending: 10}})
	if err := d.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 0, 3)
	for i := 0; i < 3; i++ {
		id, err := d.Submit(commandRequest("p", nil))
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	waitFor(t, func() bool { return jobs.commandCount() == 2 })
	if err := store.RequestRunCancel(ids[0], "operator cancelled"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := d.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	for index, id := range ids {
		record, _, err := store.Run(id)
		if err != nil {
			t.Fatal(err)
		}
		want := RunStatusInterrupted
		if index == 0 {
			want = RunStatusCancelled
		}
		if record.Status != want {
			t.Errorf("run %d status = %q, want %q", index, record.Status, want)
		}
	}
}

func TestShutdownInterruptsRunningCommandRatherThanCancellingIt(t *testing.T) {
	root, commandDir := t.TempDir(), t.TempDir()
	helperExecutable(t, commandDir, "mah-helper")
	store := newRunnerTestStore()
	settings := runnerTestSettings{root: root, commandDir: commandDir, pending: 10, perRun: 1 << 20, global: 1 << 21}
	executor := NewExecutor(RunnerDependencies{Store: store, Settings: settings})
	jobs := &dispatcherTestJobs{}
	d := NewDispatcher(Dependencies{Store: store, Jobs: jobs, Executor: executor, Settings: settings})
	if err := d.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	runID, err := d.Submit(CommandRequest{PluginName: "p", PluginGeneration: 1, Declaration: Declaration{
		Name: "sleep", Argv: []string{"mah-helper", helperProcessFlag, "sleep"}, Timeout: time.Minute,
	}})
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return jobs.commandCount() == 1 })
	registered := jobs.commandSnapshot()[0]
	outcome := make(chan Outcome, 1)
	go func() { outcome <- registered.run(context.Background(), nopProgress{}) }()
	waitFor(t, func() bool {
		record, _, err := store.Run(runID)
		return err == nil && record.ProcessGroupID != nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := d.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-outcome:
		if got.Status != RunStatusInterrupted {
			t.Fatalf("outcome = %+v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("shutdown did not drain command outcome")
	}
	record, _, _ := store.Run(runID)
	if record.Status != RunStatusInterrupted {
		t.Fatalf("record = %+v", record)
	}
}
