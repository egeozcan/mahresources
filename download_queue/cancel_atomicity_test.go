package download_queue

import (
	"errors"
	"io"
	"mahresources/contracts"
	"mahresources/models"
	"mahresources/models/query_models"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// UI bug hunt 2026-07-29, review remediation finding 1 (high): Cancel was not
// atomic. It read the job's status, decided from that read whether to take the
// paused branch, and only then acted — so a Pause landing in between made the
// answer and the outcome disagree:
//
//  1. Cancel observes `downloading`, so it skips the paused branch
//  2. Pause writes `paused` and cancels the context
//  3. Cancel's job.Cancel() is a no-op — the context is already cancelled
//  4. processJob sees `paused` and returns without stamping a terminal state
//
// The job then sits at `paused`, offering Resume, while Cancel returned nil and the
// handler answered 200 {"status":"cancelled"}.
//
// download_queue/cancel_paused_test.go could not catch this: every job in it is in
// a settled state and only one control ever touches it. These tests drive two
// controls at a job that is genuinely mid-flight.

// blockingResourceCreator parks the download worker mid-flight. AddResource reads
// one byte off the body — so the job is genuinely `downloading`, with progress on
// it — and then waits for the test to hand it a release token.
//
// That is what makes the interleavings below deterministic rather than a timing
// gamble: while the worker is parked here it cannot reach its terminal write, so a
// test can drive Cancel and Pause into exactly the window the old code left open.
type blockingResourceCreator struct {
	entered chan struct{} // buffered: one token per AddResource entry
	release chan struct{} // one token releases one parked call
}

func newBlockingResourceCreator() *blockingResourceCreator {
	return &blockingResourceCreator{entered: make(chan struct{}, 4), release: make(chan struct{})}
}

func (b *blockingResourceCreator) AddResource(file contracts.File, _ string, _ *query_models.ResourceCreator) (*models.Resource, error) {
	_, _ = io.CopyN(io.Discard, file, 1)
	select {
	case b.entered <- struct{}{}:
	default:
	}
	<-b.release
	return nil, errors.New("aborted while the test held the worker")
}

// releaseOne lets a single parked worker continue.
func (b *blockingResourceCreator) releaseOne(t *testing.T) {
	t.Helper()
	select {
	case b.release <- struct{}{}:
	case <-time.After(15 * time.Second):
		t.Fatalf("no worker was parked to release")
	}
}

// waitParked blocks until a worker is parked inside AddResource.
func (b *blockingResourceCreator) waitParked(t *testing.T) {
	t.Helper()
	select {
	case <-b.entered:
	case <-time.After(15 * time.Second):
		t.Fatalf("the worker never reached AddResource, so nothing was mid-flight to test")
	}
}

// trickleServer answers 200 and then dribbles bytes until the client goes away, so
// a download against it stays in flight for as long as the test needs.
func trickleServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		for {
			select {
			case <-r.Context().Done():
				return
			default:
			}
			if _, err := w.Write([]byte("0123456789")); err != nil {
				return
			}
			if flusher != nil {
				flusher.Flush()
			}
			time.Sleep(5 * time.Millisecond)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// inFlightJob submits a real download and returns once the worker is parked inside
// AddResource with the job in `downloading`.
func inFlightJob(t *testing.T) (*DownloadManager, *DownloadJob, *blockingResourceCreator) {
	t.Helper()

	blocker := newBlockingResourceCreator()
	dm := createTestManager()
	dm.resourceCtx = blocker
	// A zero IdleTimeout makes the timeout watcher fail the read after ~100ms, which
	// would end the download before the test could drive anything.
	dm.settings = NewStaticDownloadSettings(TimeoutConfig{
		ConnectTimeout: 5 * time.Second,
		IdleTimeout:    30 * time.Second,
		OverallTimeout: time.Minute,
	}, 0)

	srv := trickleServer(t)
	job, err := dm.Submit(&query_models.ResourceFromRemoteCreator{URL: srv.URL + "/slow.dat"}, nil)
	if err != nil {
		t.Fatalf("submitting a download failed: %v", err)
	}

	blocker.waitParked(t)
	if got := job.GetStatus(); got != JobStatusDownloading {
		t.Fatalf("precondition: job is %q, want %q", got, JobStatusDownloading)
	}
	return dm, job, blocker
}

// settle waits for the worker to leave the states it can still move out of on its
// own. It deliberately treats `paused` as settled, so the pre-fix stranding is
// reported at once instead of after the timeout.
func settle(t *testing.T, job *DownloadJob) JobStatus {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		switch status := job.GetStatus(); status {
		case JobStatusPending, JobStatusDownloading, JobStatusProcessing:
			time.Sleep(2 * time.Millisecond)
		default:
			return status
		}
	}
	return job.GetStatus()
}

// TestCancelThenPause_CannotStrandAJobPaused is the sequential form of the defect,
// and it needs no injected hook: cancelling a download does not stop the worker
// instantly — it has to observe the context — so a Pause arriving right after a
// Cancel is an ordinary pair of API calls that lands in the same window.
//
// What is asserted is the agreement between the answer and the outcome: if Cancel
// reports success, the job must not end up in a state that offers Resume.
func TestCancelThenPause_CannotStrandAJobPaused(t *testing.T) {
	dm, job, blocker := inFlightJob(t)

	if err := dm.Cancel(job.ID); err != nil {
		t.Fatalf("cancelling a downloading job failed: %v", err)
	}

	// The worker is still parked, so it has not observed the cancellation yet. This
	// is the window: before the fix CanPause() saw `downloading` and let the pause
	// through, overwriting a cancellation that had already been reported as done.
	pauseErr := dm.Pause(job.ID)

	blocker.releaseOne(t)
	status := settle(t, job)

	if status != JobStatusCancelled {
		t.Errorf("finding 1: Cancel returned nil but the job settled at %q — a job that offers Resume is not a cancelled job", status)
	}
	// The losing control has to say so. Answering 200 to both is how the two came to
	// disagree in the first place.
	var conflict *StateConflictError
	if !errors.As(pauseErr, &conflict) {
		t.Errorf("finding 1: pausing a job whose cancel was already accepted returned %v, want a StateConflictError", pauseErr)
	}
	if job.GetCompletedAt() == nil {
		t.Errorf("finding 1: the cancelled job has no CompletedAt, so job retention will never retire it")
	}
}

// The positive control for the test above, in the other order: when the pause wins,
// it is the *cancel* that has to cope, and the job still ends up cancelled rather
// than stranded. Without this, a fix that simply refused every Pause would pass.
func TestPauseThenCancel_StillEndsCancelled(t *testing.T) {
	dm, job, blocker := inFlightJob(t)

	if err := dm.Pause(job.ID); err != nil {
		t.Fatalf("pausing a downloading job failed: %v", err)
	}
	if got := job.GetStatus(); got != JobStatusPaused {
		t.Fatalf("the pause did not take: job is %q", got)
	}
	if err := dm.Cancel(job.ID); err != nil {
		t.Fatalf("cancelling the paused job failed: %v", err)
	}

	blocker.releaseOne(t)

	if status := settle(t, job); status != JobStatusCancelled {
		t.Errorf("a paused job that was then cancelled settled at %q, want %q", status, JobStatusCancelled)
	}
}

// TestCancelAndPauseConcurrently_Agree drives the two controls at once, which is
// the form the report describes and the form `-race` exercises. The invariant is
// the same on every iteration: whichever control wins the claim, a Cancel that
// reported success must not leave the job paused.
func TestCancelAndPauseConcurrently_Agree(t *testing.T) {
	for i := 0; i < 25; i++ {
		dm, job, blocker := inFlightJob(t)

		start := make(chan struct{})
		var cancelErr, pauseErr error
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			cancelErr = dm.Cancel(job.ID)
		}()
		go func() {
			defer wg.Done()
			<-start
			pauseErr = dm.Pause(job.ID)
		}()
		close(start)
		wg.Wait()

		blocker.releaseOne(t)
		status := settle(t, job)

		if cancelErr != nil {
			t.Fatalf("iteration %d: cancelling a downloading job failed: %v", i, cancelErr)
		}
		if status != JobStatusCancelled {
			t.Fatalf("iteration %d: Cancel returned nil, Pause returned %v, and the job settled at %q; want %q",
				i, pauseErr, status, JobStatusCancelled)
		}
	}
}

// A resumed job must not inherit a cancel that was accepted while it was paused:
// Resume installs a fresh context, so the stale cancellation would not stop the new
// download and the row would say "Downloading" for a job the user abandoned.
func TestResume_RefusesAJobWhoseCancelWasAlreadyAccepted(t *testing.T) {
	dm := createTestManager()
	job := addTestJob(dm, "paused", JobStatusPaused)

	if err := dm.Cancel("paused"); err != nil {
		t.Fatalf("cancelling a paused job failed: %v", err)
	}

	var conflict *StateConflictError
	if err := dm.Resume("paused"); !errors.As(err, &conflict) {
		t.Errorf("resuming a cancelled job returned %v, want a StateConflictError", err)
	}
	if got := job.GetStatus(); got != JobStatusCancelled {
		t.Errorf("the job is %q after a refused resume, want %q", got, JobStatusCancelled)
	}
}

// Retry is the deliberate exception: the user has asked for this job to run again,
// so the cancel that ended it no longer stands. Without this a retried job would be
// born with its cancel already accepted and could never be paused again.
func TestRetry_ClearsAnAcceptedCancel(t *testing.T) {
	dm, job, blocker := inFlightJob(t)

	if err := dm.Cancel(job.ID); err != nil {
		t.Fatalf("cancelling failed: %v", err)
	}
	blocker.releaseOne(t)
	if status := settle(t, job); status != JobStatusCancelled {
		t.Fatalf("the job settled at %q, want %q", status, JobStatusCancelled)
	}

	if err := dm.Retry(job.ID); err != nil {
		t.Fatalf("retrying a cancelled job failed: %v", err)
	}
	// The retried download parks in the same place, so the assertion below is made
	// against a job that is really mid-flight again rather than one that has already
	// failed for its own reasons.
	blocker.waitParked(t)
	if got := job.GetStatus(); got != JobStatusDownloading {
		t.Fatalf("a retried job is %q, want %q", got, JobStatusDownloading)
	}

	if err := dm.Pause(job.ID); err != nil {
		t.Errorf("pausing a retried job failed with %v — the retry did not clear the earlier cancel", err)
	}
	blocker.releaseOne(t)
}
