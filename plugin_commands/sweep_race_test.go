package plugin_commands

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestLeaseBlocksSweepAndSweepClosesCheckToAcquireGap(t *testing.T) {
	leases := NewLeaseManager()
	release, err := leases.Acquire("run")
	if err != nil {
		t.Fatal(err)
	}
	if end, ok := leases.BeginSweep("run"); ok {
		end()
		t.Fatal("sweep began while a lease was active")
	}
	release()

	endSweep, ok := leases.BeginSweep("run")
	if !ok {
		t.Fatal("sweep did not begin after lease release")
	}
	if _, err := leases.Acquire("run"); err == nil || !strings.Contains(err.Error(), "run swept") {
		t.Fatalf("acquire during sweep error = %v", err)
	}
	endSweep()
	secondRelease, err := leases.Acquire("run")
	if err != nil {
		t.Fatalf("acquire after sweep decision completed: %v", err)
	}
	secondRelease()
}

func TestImportAdmissionPinsRunBeforeWaitingForWorkerSlot(t *testing.T) {
	leases := NewLeaseManager()
	store := newDispatcherTestStore()
	jobs := &dispatcherTestJobs{}
	d := NewDispatcher(Dependencies{
		Store: store, Jobs: jobs, Executor: dispatcherTestExecutor{},
		Settings: dispatcherTestSettings{pending: 10}, Leases: leases,
	})
	if err := d.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"a", "b", "pending"} {
		if err := d.submitImport(ImportJobSpec{ImportID: id, RunID: "run-" + id, PluginName: "p"}, func(context.Context, Progress) Outcome {
			return Outcome{Status: ImportStatusSucceeded}
		}); err != nil {
			t.Fatalf("submit import %s: %v", id, err)
		}
	}
	waitFor(t, func() bool { return jobs.importCount() == maxActiveImports })
	if end, ok := leases.BeginSweep("run-pending"); ok {
		end()
		t.Fatal("sweep began while an admitted import waited for a worker slot")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := d.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	for _, runID := range []string{"run-a", "run-b", "run-pending"} {
		end, ok := leases.BeginSweep(runID)
		if !ok {
			t.Fatalf("import lease for %s survived dispatcher stop", runID)
		}
		end()
	}
}

func TestLeaseSweepInterleavingHasNoCheckToAcquireWindow(t *testing.T) {
	leases := NewLeaseManager()
	checked := make(chan struct{})
	allowDelete := make(chan struct{})
	deleted := make(chan struct{})

	go func() {
		end, ok := leases.BeginSweep("expired")
		if !ok {
			close(deleted)
			return
		}
		defer end()
		close(checked)
		<-allowDelete
		close(deleted)
	}()
	<-checked

	result := make(chan error, 1)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, err := leases.Acquire("expired")
		result <- err
	}()
	acquireErr := <-result
	if acquireErr == nil || !strings.Contains(acquireErr.Error(), "run swept") {
		t.Fatalf("operation slipped into sweep decision: %v", acquireErr)
	}
	close(allowDelete)
	<-deleted
	wg.Wait()
}
