//go:build !windows

package plugin_commands

import (
	"context"
	"errors"
	"testing"
	"time"
)

func runCancellationBarrierCase(t *testing.T, afterStart bool, disable bool) {
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

	if disable {
		err = d.DisablePlugin("p", "plugin disabled")
	} else {
		err = d.Cancel(runID, "operator cancelled")
	}
	if err != nil {
		t.Fatal(err)
	}
	close(release)
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
	if !afterStart && record.ProcessGroupID != nil {
		t.Fatalf("pre-fork cancellation spawned process group %d", *record.ProcessGroupID)
	}
	if afterStart && record.ProcessGroupID == nil {
		t.Fatal("post-fork cancellation finished before pgid persistence")
	}
	if afterStart && record.OutputUnverified {
		t.Fatalf("owned post-fork process group was not verified: %+v", record)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := d.Stop(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestCancelImmediatelyBeforeForkPreventsSpawn(t *testing.T) {
	runCancellationBarrierCase(t, false, false)
}

func TestDisableImmediatelyBeforeForkPreventsSpawn(t *testing.T) {
	runCancellationBarrierCase(t, false, true)
}

func TestCancelAfterForkWaitsForProcessGroupPersistence(t *testing.T) {
	runCancellationBarrierCase(t, true, false)
}

func TestDisableAfterForkWaitsForProcessGroupPersistence(t *testing.T) {
	runCancellationBarrierCase(t, true, true)
}

func TestDisableCancelsQueuedImportsButLetsRunningImportsFinish(t *testing.T) {
	store := newDispatcherTestStore()
	jobs := &dispatcherTestJobs{}
	d := NewDispatcher(Dependencies{Store: store, Jobs: jobs, Executor: dispatcherTestExecutor{}, Settings: dispatcherTestSettings{pending: 10}})
	if err := d.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"running-a", "running-b", "queued-c"} {
		if err := d.submitImport(ImportJobSpec{ImportID: id, PluginName: "p"}, func(context.Context, Progress) Outcome {
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

func TestShutdownCancelsRunningImportSourceAndDrainsIt(t *testing.T) {
	store := newDispatcherTestStore()
	jobs := &dispatcherTestJobs{}
	d := NewDispatcher(Dependencies{Store: store, Jobs: jobs, Executor: dispatcherTestExecutor{}, Settings: dispatcherTestSettings{pending: 10}})
	if err := d.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	if err := d.submitImport(ImportJobSpec{ImportID: "running-import", PluginName: "p"}, func(ctx context.Context, _ Progress) Outcome {
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
