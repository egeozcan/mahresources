package download_queue

import (
	"errors"
	"testing"
)

// UI bug hunt 2026-07-29, finding 2 (high): a paused download job could never be
// cancelled.
//
// `IsActive()` is pending|downloading|processing, `Cancel` rejected anything that
// was not active with "job %s already finished", and the handler mapped every
// manager error to 404 — so a paused 1 GB download was stuck in the queue with
// `{"error":"job <id> already finished"}` HTTP 404 as the only answer.
//
// Two things are asserted here at the manager layer: that a paused job can be
// cancelled, and that the refusals are *typed* so the handler can tell a missing
// job (404) from a state conflict (409) without matching on message text.

func TestCancel_PausedJobIsCancellable(t *testing.T) {
	dm := createTestManager()
	events := make(chan JobEvent, 10)
	dm.subscribers[events] = struct{}{}

	job := addTestJob(dm, "paused", JobStatusPaused)
	job.UpdateProgress(196608, 52428800)

	if err := dm.Cancel("paused"); err != nil {
		t.Fatalf("finding 2: cancelling a paused job failed: %v", err)
	}

	if got := job.GetStatus(); got != JobStatusCancelled {
		t.Errorf("finding 2: a cancelled paused job should end up %q, got %q", JobStatusCancelled, got)
	}

	// A paused job has no goroutine left to observe the context cancellation, so
	// the status transition and the notification have to happen in Cancel itself.
	// Without the notification the panel would keep showing "Paused" until the
	// next reload.
	select {
	case ev := <-events:
		if ev.Type != "updated" || ev.Job == nil || ev.Job.Status != JobStatusCancelled {
			t.Errorf("finding 2: expected an updated event carrying the cancelled status, got %+v", ev)
		}
	default:
		t.Errorf("finding 2: cancelling a paused job notified no subscriber, so the panel would keep showing Paused")
	}

	if job.GetCompletedAt() == nil {
		t.Errorf("finding 2: a cancelled paused job has no CompletedAt, so job retention will never clean it up")
	}
}

// The positive control for the test above: an *active* job is still cancelled the
// way it always was, through its context, and its status is left to the download
// goroutine.
func TestCancel_ActiveJobStillCancelsThroughItsContext(t *testing.T) {
	dm := createTestManager()
	job := addTestJob(dm, "active", JobStatusDownloading)

	if err := dm.Cancel("active"); err != nil {
		t.Fatalf("cancelling an active job failed: %v", err)
	}

	select {
	case <-job.GetContext().Done():
	default:
		t.Error("cancelling an active job did not cancel its context")
	}
}

func TestCancel_FinishedJobIsATypedStateConflict(t *testing.T) {
	for _, status := range []JobStatus{JobStatusCompleted, JobStatusFailed, JobStatusCancelled} {
		t.Run(string(status), func(t *testing.T) {
			dm := createTestManager()
			addTestJob(dm, "done", status)

			err := dm.Cancel("done")
			if err == nil {
				t.Fatalf("expected an error cancelling a %s job", status)
			}

			var conflict *StateConflictError
			if !errors.As(err, &conflict) {
				t.Fatalf("finding 2: cancelling a %s job returned %T (%v); the handler needs a typed conflict to answer 409 instead of 404", status, err, err)
			}
			if conflict.Status != status {
				t.Errorf("the conflict should name the status it refused (%s), got %s", status, conflict.Status)
			}

			var missing *NotFoundError
			if errors.As(err, &missing) {
				t.Errorf("a state conflict must not also read as not-found, or 404 comes back")
			}
		})
	}
}

func TestJobControls_UnknownJobIsATypedNotFound(t *testing.T) {
	actions := map[string]func(*DownloadManager) error{
		"cancel": func(dm *DownloadManager) error { return dm.Cancel("nope") },
		"pause":  func(dm *DownloadManager) error { return dm.Pause("nope") },
		"resume": func(dm *DownloadManager) error { return dm.Resume("nope") },
		"retry":  func(dm *DownloadManager) error { return dm.Retry("nope") },
	}

	for name, action := range actions {
		t.Run(name, func(t *testing.T) {
			dm := createTestManager()
			err := action(dm)
			if err == nil {
				t.Fatalf("expected an error for an unknown job")
			}
			var missing *NotFoundError
			if !errors.As(err, &missing) {
				t.Fatalf("%s of an unknown job returned %T (%v), want *NotFoundError", name, err, err)
			}
		})
	}
}

func TestJobControls_WrongStateIsATypedStateConflict(t *testing.T) {
	cases := []struct {
		name   string
		status JobStatus
		action func(*DownloadManager) error
	}{
		{"pause a paused job", JobStatusPaused, func(dm *DownloadManager) error { return dm.Pause("j") }},
		{"resume a downloading job", JobStatusDownloading, func(dm *DownloadManager) error { return dm.Resume("j") }},
		{"retry a paused job", JobStatusPaused, func(dm *DownloadManager) error { return dm.Retry("j") }},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dm := createTestManager()
			addTestJob(dm, "j", c.status)
			err := c.action(dm)
			if err == nil {
				t.Fatalf("expected an error: %s", c.name)
			}
			var conflict *StateConflictError
			if !errors.As(err, &conflict) {
				t.Fatalf("%s returned %T (%v), want *StateConflictError", c.name, err, err)
			}
		})
	}
}

// Finding 40: finished jobs could not be dismissed — the panel accumulated 20
// permanent entries across sessions and there was no control anywhere to clear
// them. ClearFinished is what the new "Clear completed" button calls.
func TestClearFinished_RemovesTerminalJobsAndKeepsTheRest(t *testing.T) {
	dm := createTestManager()
	events := make(chan JobEvent, 20)
	dm.subscribers[events] = struct{}{}

	addTestJob(dm, "completed", JobStatusCompleted)
	addTestJob(dm, "failed", JobStatusFailed)
	addTestJob(dm, "cancelled", JobStatusCancelled)
	addTestJob(dm, "downloading", JobStatusDownloading)
	addTestJob(dm, "pending", JobStatusPending)
	addTestJob(dm, "paused", JobStatusPaused)

	cleared := dm.ClearFinished(func(*uint) bool { return true })
	if cleared != 3 {
		t.Errorf("expected 3 terminal jobs cleared, got %d", cleared)
	}

	remaining := map[string]bool{}
	for _, job := range dm.GetJobs() {
		remaining[job.ID] = true
	}
	for _, gone := range []string{"completed", "failed", "cancelled"} {
		if remaining[gone] {
			t.Errorf("finding 40: %s job survived Clear completed", gone)
		}
	}
	// A paused job is *not* finished. Clearing it would be exactly the data loss
	// finding 2 is about — a half-downloaded job silently discarded.
	for _, kept := range []string{"downloading", "pending", "paused"} {
		if !remaining[kept] {
			t.Errorf("finding 40: Clear completed removed the %s job, which is not finished", kept)
		}
	}

	// jobOrder has to be pruned too, or makeRoomForNewJob dereferences a nil job.
	for _, id := range dm.jobOrder {
		if dm.jobs[id] == nil {
			t.Fatalf("jobOrder still lists %q after it was cleared", id)
		}
	}
}

// Only the caller's own jobs may be cleared: with -auth on, a non-admin sees only
// the jobs it submitted, so "Clear completed" must not reach anyone else's.
func TestClearFinished_HonoursVisibility(t *testing.T) {
	dm := createTestManager()
	mine := uint(7)
	theirs := uint(9)

	a := addTestJob(dm, "mine", JobStatusCompleted)
	a.SetOwnerUserID(mine)
	b := addTestJob(dm, "theirs", JobStatusCompleted)
	b.SetOwnerUserID(theirs)

	cleared := dm.ClearFinished(func(owner *uint) bool { return owner != nil && *owner == mine })
	if cleared != 1 {
		t.Fatalf("expected only the caller's own job cleared, got %d", cleared)
	}
	if _, ok := dm.GetJob("theirs"); !ok {
		t.Error("another user's completed job was cleared")
	}
}
