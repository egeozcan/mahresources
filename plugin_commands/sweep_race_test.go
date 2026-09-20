package plugin_commands

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type blockingRuntimeSweepStore struct {
	*dispatcherTestStore
	calls   atomic.Int32
	entered chan struct{}
	release chan struct{}
}

func (s *blockingRuntimeSweepStore) ExpiredTerminalRunBoundary(time.Time) (*RetentionCursor, error) {
	return &RetentionCursor{FinishedAt: time.Unix(1, 0).UTC(), RunID: "boundary"}, nil
}

func (s *blockingRuntimeSweepStore) ExpiredTerminalRuns(before time.Time, after, through *RetentionCursor, limit int) ([]RunRecord, error) {
	if s.calls.Add(1) > 1 {
		select {
		case <-s.entered:
		default:
			close(s.entered)
		}
		<-s.release
	}
	return s.dispatcherTestStore.ExpiredTerminalRuns(before, after, through, limit)
}

func TestRuntimeLeaseRemainsHeldUntilTimedOutSweepQuiesces(t *testing.T) {
	store := &blockingRuntimeSweepStore{
		dispatcherTestStore: newDispatcherTestStore(),
		entered:             make(chan struct{}),
		release:             make(chan struct{}),
	}
	d := NewDispatcher(Dependencies{
		Store: store, Jobs: &dispatcherTestJobs{}, Executor: dispatcherTestExecutor{},
		Settings: dispatcherTestSettings{pending: 10},
	})
	d.sweepInterval = time.Millisecond
	if err := d.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-store.entered:
	case <-time.After(time.Second):
		close(store.release)
		t.Fatal("periodic sweep did not start")
	}

	stopCtx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	err := d.Stop(stopCtx)
	cancel()
	if err == nil || !strings.Contains(err.Error(), context.DeadlineExceeded.Error()) {
		close(store.release)
		t.Fatalf("Stop() = %v, want sweep deadline", err)
	}
	if d.RuntimeLeaseReleasable() {
		close(store.release)
		t.Fatal("runtime lease became releasable while a timed-out sweep remained active")
	}

	close(store.release)
	waitFor(t, d.RuntimeLeaseReleasable)
}

func TestSuccessfulStopPublishesOnlyAfterSweepOwnershipIsReleased(t *testing.T) {
	store := &blockingRuntimeSweepStore{
		dispatcherTestStore: newDispatcherTestStore(),
		entered:             make(chan struct{}),
		release:             make(chan struct{}),
	}
	d := NewDispatcher(Dependencies{
		Store: store, Jobs: &dispatcherTestJobs{}, Executor: dispatcherTestExecutor{},
		Settings: dispatcherTestSettings{pending: 10},
	})
	d.sweepInterval = time.Millisecond
	if err := d.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-store.entered:
	case <-time.After(time.Second):
		close(store.release)
		t.Fatal("periodic sweep did not start")
	}

	stopResult := make(chan error, 1)
	go func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		stopResult <- d.Stop(stopCtx)
	}()
	select {
	case err := <-stopResult:
		close(store.release)
		t.Fatalf("Stop returned before its active sweep finished: %v", err)
	case <-time.After(10 * time.Millisecond):
	}
	close(store.release)
	if err := <-stopResult; err != nil {
		t.Fatal(err)
	}
	if !d.RuntimeLeaseReleasable() {
		t.Fatal("successful Stop published before dispatcher runtime ownership quiesced")
	}
}

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
