package download_queue

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestManagedJobLaneIsIndependentFromOrdinaryAdmission(t *testing.T) {
	dm := createTestManager()
	for i := 0; i < MaxQueueSize; i++ {
		addTestJob(dm, fmt.Sprintf("ordinary-%03d", i), JobStatusPending)
	}

	release := make(chan struct{})
	started := make(chan struct{}, MaxManagedLiveJobs)
	for i := 0; i < MaxManagedLiveJobs; i++ {
		_, err := dm.SubmitManagedJob(ManagedJobOptions{JobOptions: JobOptions{Source: "managed"}}, func(ctx context.Context, _ *DownloadJob, _ ProgressSink) ManagedJobOutcome {
			started <- struct{}{}
			select {
			case <-release:
				return ManagedJobOutcome{Status: JobStatusCompleted, AuthoritativeStatus: "succeeded"}
			case <-ctx.Done():
				return ManagedJobOutcome{Status: JobStatusCancelled, AuthoritativeStatus: "cancelled"}
			}
		})
		if err != nil {
			t.Fatalf("submit managed %d: %v", i, err)
		}
	}
	for i := 0; i < MaxManagedLiveJobs; i++ {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("managed job waited for the ordinary semaphore/registry")
		}
	}
	if got := len(dm.GetJobs()); got != MaxQueueSize+MaxManagedLiveJobs {
		t.Fatalf("registry size = %d, want %d", got, MaxQueueSize+MaxManagedLiveJobs)
	}
	if _, err := dm.SubmitManagedJob(ManagedJobOptions{}, func(context.Context, *DownloadJob, ProgressSink) ManagedJobOutcome { return ManagedJobOutcome{} }); err == nil {
		t.Fatal("seventh managed job was admitted")
	}
	close(release)
}

func TestManagedJobsDoNotConsumeOrdinaryBudget(t *testing.T) {
	dm := createTestManager()
	for i := 0; i < MaxManagedLiveJobs; i++ {
		job := addTestJob(dm, fmt.Sprintf("managed-%d", i), JobStatusPending)
		job.managed = true
	}
	for i := 0; i < MaxQueueSize; i++ {
		if !dm.makeRoomForNewJob() {
			t.Fatalf("ordinary admission %d refused", i)
		}
		addTestJob(dm, fmt.Sprintf("ordinary-%d", i), JobStatusPending)
	}
	if dm.makeRoomForNewJob() {
		t.Fatal("101st ordinary job was admitted")
	}
}

func TestManagedAdmissionEvictsTerminalManagedEntry(t *testing.T) {
	dm := createTestManager()
	for i := 0; i < MaxManagedLiveJobs; i++ {
		status := JobStatusPending
		if i == 0 {
			status = JobStatusCompleted
		}
		job := addTestJob(dm, fmt.Sprintf("managed-%d", i), status)
		job.managed = true
	}

	release := make(chan struct{})
	_, err := dm.SubmitManagedJob(ManagedJobOptions{}, func(context.Context, *DownloadJob, ProgressSink) ManagedJobOutcome {
		<-release
		return ManagedJobOutcome{Status: JobStatusCompleted}
	})
	if err != nil {
		t.Fatalf("replacement submit: %v", err)
	}
	if _, ok := dm.GetJob("managed-0"); ok {
		t.Fatal("admission did not evict oldest terminal managed job")
	}
	close(release)
}

func TestCleanupDirectDeletionReleasesManagedCapacity(t *testing.T) {
	dm := createTestManager()
	dm.jobRetention = time.Millisecond
	old := time.Now().Add(-time.Hour)
	for i := 0; i < MaxManagedLiveJobs; i++ {
		status := JobStatusPending
		job := addTestJob(dm, fmt.Sprintf("managed-%d", i), status)
		job.managed = true
		if i == 0 {
			job.Status = JobStatusCompleted
			job.CompletedAt = &old
		}
	}

	dm.cleanupOldJobs()
	if _, ok := dm.GetJob("managed-0"); ok {
		t.Fatal("cleanup did not directly remove expired managed job")
	}
	release := make(chan struct{})
	_, err := dm.SubmitManagedJob(ManagedJobOptions{}, func(context.Context, *DownloadJob, ProgressSink) ManagedJobOutcome {
		<-release
		return ManagedJobOutcome{Status: JobStatusCompleted}
	})
	if err != nil {
		t.Fatalf("submit after cleanup: %v", err)
	}
	close(release)
}

func TestManagedJobControlsAndAuthoritativeStatus(t *testing.T) {
	dm := createTestManager()
	events := make(chan JobEvent, 16)
	dm.subscribers[events] = struct{}{}

	var job *DownloadJob
	var once sync.Once
	cancelled := make(chan struct{})
	cancelFn := func(string) error {
		_ = job.Snapshot() // deadlocks if manager or job lock is held by Cancel.
		once.Do(func() { close(cancelled) })
		return nil
	}
	job, _ = dm.SubmitManagedJob(ManagedJobOptions{
		Controls: JobControls{Cancel: true},
		Cancel:   cancelFn,
	}, func(ctx context.Context, _ *DownloadJob, _ ProgressSink) ManagedJobOutcome {
		<-cancelled
		return ManagedJobOutcome{Status: JobStatusCancelled, AuthoritativeStatus: "cancelled", Error: "operator stopped it"}
	})

	for _, call := range []struct {
		name string
		fn   func(string) error
	}{{"pause", dm.Pause}, {"resume", dm.Resume}, {"retry", dm.Retry}} {
		err := call.fn(job.ID)
		var conflict *StateConflictError
		if !errors.As(err, &conflict) {
			t.Fatalf("%s error = %v, want StateConflictError", call.name, err)
		}
	}

	done := make(chan error, 1)
	go func() { done <- dm.Cancel(job.ID) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("cancel: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("managed cancellation deadlocked")
	}

	deadline := time.After(time.Second)
	for {
		select {
		case event := <-events:
			if event.Job.ID == job.ID && event.Job.Status == JobStatusCancelled {
				if event.Job.AuthoritativeStatus != "cancelled" {
					t.Fatalf("authoritative status = %q", event.Job.AuthoritativeStatus)
				}
				return
			}
		case <-deadline:
			t.Fatal("terminal managed event not received")
		}
	}
}
