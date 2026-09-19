//go:build linux || darwin

package plugin_commands

import (
	"context"
	"errors"
	"fmt"
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
	if spawnWon && record.OutputUnverified {
		t.Fatalf("owned post-fork process group was not verified: %+v", record)
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

func TestCancelAfterForkWaitsForProcessGroupPersistence(t *testing.T) {
	runCancellationBarrierCase(t, true, false, false)
}

func TestDisableAfterForkWaitsForProcessGroupPersistence(t *testing.T) {
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

func TestStopWaitsForAcceptedTerminalPersistenceAfterDeadline(t *testing.T) {
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
	select {
	case err := <-stopped:
		t.Fatalf("Stop returned before accepted terminal persistence drained: %v", err)
	default:
	}
	close(release)
	if err := <-stopped; err != nil {
		t.Fatal(err)
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
