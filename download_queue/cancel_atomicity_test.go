package download_queue

import (
	"context"
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
// Each parked call gets its own gate, and releaseOne opens the oldest one still
// closed. One shared release channel was not enough once a test parked two attempts
// at a time: `b.release <- struct{}{}` is taken by whichever goroutine reaches the
// receive first, so a test that meant "let attempt A go" could let B go instead and
// then fail on B's terminal write — a scheduler-dependent failure in a test whose
// whole claim is that it is sequenced. Round-4 review.
type blockingResourceCreator struct {
	mu     sync.Mutex
	gates  []chan struct{} // parked and not yet released, oldest first
	parked chan chan struct{}
}

func newBlockingResourceCreator() *blockingResourceCreator {
	return &blockingResourceCreator{parked: make(chan chan struct{}, 4)}
}

func (b *blockingResourceCreator) AddResource(file contracts.File, _ string, _ *query_models.ResourceCreator) (*models.Resource, error) {
	_, _ = io.CopyN(io.Discard, file, 1)
	gate := make(chan struct{})
	b.parked <- gate
	<-gate
	return nil, errors.New("aborted while the test held the worker")
}

// takeGate returns the next gate, waiting for a worker to park if none has yet.
func (b *blockingResourceCreator) takeGate(t *testing.T) chan struct{} {
	t.Helper()
	b.mu.Lock()
	if len(b.gates) > 0 {
		gate := b.gates[0]
		b.gates = b.gates[1:]
		b.mu.Unlock()
		return gate
	}
	b.mu.Unlock()

	select {
	case gate := <-b.parked:
		return gate
	case <-time.After(15 * time.Second):
		t.Fatalf("no worker was parked in AddResource")
		return nil
	}
}

// releaseOne lets the earliest-parked worker that is still waiting continue.
func (b *blockingResourceCreator) releaseOne(t *testing.T) {
	t.Helper()
	close(b.takeGate(t))
}

// waitParked blocks until a worker is parked inside AddResource, and remembers it so
// a later releaseOne frees that one rather than whichever goroutine wins a race.
func (b *blockingResourceCreator) waitParked(t *testing.T) {
	t.Helper()
	select {
	case gate := <-b.parked:
		b.mu.Lock()
		b.gates = append(b.gates, gate)
		b.mu.Unlock()
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

// The other half of the same discipline, and a hole the first pass left: the
// worker's *forward* status writes were unconditional.
//
// A job's goroutine starts while the job is `pending`, and `pending` is pausable.
// So a Pause landing between the worker acquiring its semaphore slot and its
// `SetStatus(JobStatusDownloading)` was overwritten — the caller was told 200
// `paused`, the worker then downloaded under an already-cancelled context, and the
// job ended `cancelled` with its progress discarded. That is finding 1 in mirror
// image: the answer and the outcome disagree, in the other direction.
//
// processJob is called directly here because that is exactly the state the race
// produces — a worker reaching its first write on a job a control has already taken
// away — and it needs no injected hook to reach it.
func TestProcessJob_DoesNotTakeOverAPausedJob(t *testing.T) {
	dm := createTestManager()
	job := addTestJob(dm, "held", JobStatusPaused)
	job.UpdateProgress(196608, 52428800)

	dm.processJob(job)

	if got := job.GetStatus(); got != JobStatusPaused {
		t.Errorf("the worker took over a paused job and left it %q; the pause was reported as done", got)
	}
	if got := job.Progress; got != 196608 {
		t.Errorf("the paused job's progress is %d, want it untouched at 196608", got)
	}
	if job.GetCompletedAt() != nil {
		t.Error("the paused job was retired, so Resume can never pick it up again")
	}
}

// The positive control: a worker whose job is still pending does start, and an
// accepted cancel still ends the job cancelled rather than failed — so refusing to
// start a paused job did not turn into refusing to start anything.
func TestProcessJob_StartsAPendingJobAndHonoursAnAcceptedCancel(t *testing.T) {
	dm := createTestManager()
	dm.settings = NewStaticDownloadSettings(TimeoutConfig{
		ConnectTimeout: time.Second, IdleTimeout: time.Second, OverallTimeout: time.Second,
	}, 0)
	job := addTestJob(dm, "doomed", JobStatusPending)
	job.creator = &query_models.ResourceFromRemoteCreator{}
	job.URL = "http://127.0.0.1:1/nothing-here"

	if err := dm.Cancel("doomed"); err != nil {
		t.Fatalf("cancelling a pending job failed: %v", err)
	}
	dm.processJob(job)

	if got := job.GetStatus(); got != JobStatusCancelled {
		t.Errorf("a pending job with an accepted cancel settled at %q, want %q", got, JobStatusCancelled)
	}
	if job.GetCompletedAt() == nil {
		t.Error("the cancelled job has no CompletedAt, so job retention will never retire it")
	}
}

// ---------------------------------------------- review round 2 (pi, sol:high)

// A paused-then-resumed job has two attempts alive for a moment: the first is
// unwinding while the second starts. The first must not be able to stamp a terminal
// state over the second — it used to, because `finish` accepted any non-paused
// status, and the first attempt's `cancelled` then landed on a job that was already
// downloading again. The panel showed "Cancelled" for a live download, and the
// second attempt's own terminal write later overwrote that.
//
// Fully sequenced: attempt A is parked inside AddResource for the whole test, so
// "while A unwinds" is not a timing hope.
func TestFinish_AStaleAttemptCannotStampARestartedJob(t *testing.T) {
	dm, job, blocker := inFlightJob(t)

	if err := dm.Pause(job.ID); err != nil {
		t.Fatalf("pausing failed: %v", err)
	}
	if err := dm.Resume(job.ID); err != nil {
		t.Fatalf("resuming failed: %v", err)
	}
	// Attempt B reaches AddResource and parks there too.
	blocker.waitParked(t)
	if got := job.GetStatus(); got != JobStatusDownloading {
		t.Fatalf("the resumed attempt is %q, want %q", got, JobStatusDownloading)
	}

	// Now let attempt A go. Its context was cancelled by the pause, so it classifies
	// itself as cancelled and tries to write that.
	blocker.releaseOne(t)

	// B is still parked, so any terminal status now can only have come from A.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if status := job.GetStatus(); status != JobStatusDownloading {
			t.Fatalf("a stale attempt stamped %q on a job that is downloading again", status)
		}
		time.Sleep(5 * time.Millisecond)
	}

	blocker.releaseOne(t)
}

// A generic job's runFn may honour cancellation by returning nil — "I stopped, and
// that is not an error". Reporting that as `completed` makes Cancel's 200 a lie and
// tells the user an export finished when it was abandoned. What the context says
// outranks what the runFn returned.
func TestProcessGenericJob_AnAbandonedRunIsCancelledNotCompleted(t *testing.T) {
	dm := createTestManager()

	started := make(chan struct{})
	release := make(chan struct{})
	job, err := dm.SubmitJob("test", "running", func(ctx context.Context, j *DownloadJob, p ProgressSink) error {
		close(started)
		<-release
		// Honours the cancellation by stopping, with nothing to report as an error.
		return nil
	})
	if err != nil {
		t.Fatalf("submitting a generic job failed: %v", err)
	}

	select {
	case <-started:
	case <-time.After(15 * time.Second):
		t.Fatalf("the runFn never started")
	}

	if err := dm.Cancel(job.ID); err != nil {
		t.Fatalf("cancelling a running generic job failed: %v", err)
	}
	close(release)

	if status := settle(t, job); status != JobStatusCancelled {
		t.Errorf("an abandoned run settled at %q, want %q — Cancel had already answered 200", status, JobStatusCancelled)
	}
}

// The control for the test above: a run that finishes on its own, with no cancel in
// sight, still reports completed.
func TestProcessGenericJob_AnUninterruptedRunStillCompletes(t *testing.T) {
	dm := createTestManager()

	job, err := dm.SubmitJob("test", "running", func(ctx context.Context, j *DownloadJob, p ProgressSink) error {
		return nil
	})
	if err != nil {
		t.Fatalf("submitting a generic job failed: %v", err)
	}

	if status := settle(t, job); status != JobStatusCompleted {
		t.Errorf("an uninterrupted run settled at %q, want %q", status, JobStatusCompleted)
	}
}

// The claims own the context, not their callers. Pause used to write `paused`,
// release the job's lock, and only then cancel — and a Resume landing in that gap
// swapped the context out, so the pause cancelled the *new* attempt's context and
// left the old attempt running with a live one. Both controls returned success.
//
// The interleaving itself is a few instructions wide and needs a hook to drive; what
// is asserted here is the invariant that closes it — that the status transition and
// the cancellation are one step.
func TestClaims_CancelTheContextTheyObserved(t *testing.T) {
	t.Run("pause", func(t *testing.T) {
		job := addTestJob(createTestManager(), "j", JobStatusDownloading)
		observed := job.GetContext()

		if _, ok := job.claimPause(); !ok {
			t.Fatalf("claiming a pause on a downloading job failed")
		}

		select {
		case <-observed.Done():
		default:
			t.Error("claimPause left the context it observed live, so a Resume between the claim and the caller's cancel would swap it")
		}
	})

	t.Run("cancel", func(t *testing.T) {
		job := addTestJob(createTestManager(), "j", JobStatusDownloading)
		observed := job.GetContext()

		if _, ok := job.claimCancel(time.Now()); !ok {
			t.Fatalf("claiming a cancel on a downloading job failed")
		}

		select {
		case <-observed.Done():
		default:
			t.Error("claimCancel left the context it observed live")
		}
	})
}

// A job Resume or Retry has just started must be in the registry. Retry used to
// resolve the job, drop the manager's lock, and only then claim and start it — so a
// ClearFinished in that gap deleted a job whose worker was about to run: not
// listable, not cancellable, and never retired.
//
// Concurrent rather than sequenced, because the gap is a few instructions wide; the
// invariant holds for every interleaving.
//
// The retried attempt is held open for the whole window, and that is what makes the
// assertion mean the gap. Round 3 of the audit found this test failing 2 of 10 under
// `-race -count=10`, for a reason that is not the defect: it retried a real download
// against a refused port with 1 ms timeouts, so the new attempt reached `failed`
// within microseconds, and a `ClearFinished` landing *after that* removed a job that
// was terminal again — correctly. The assertion window was wider than its subject.
// With a run that cannot finish on its own the job is `cancelled`, `pending` or
// `processing` throughout, and only `cancelled` is clearable — so the sole way to
// reach the failure is a delete between the lookup and the start.
func TestRetry_AStartedJobIsAlwaysStillInTheRegistry(t *testing.T) {
	for i := 0; i < 200; i++ {
		dm := createTestManager()
		held := make(chan struct{})
		job := addTestJob(dm, "gone", JobStatusCancelled)
		job.runFn = func(ctx context.Context, _ *DownloadJob, _ ProgressSink) error {
			<-held
			return nil
		}
		completedAt := time.Now()
		job.CompletedAt = &completedAt

		var wg sync.WaitGroup
		var retryErr error
		start := make(chan struct{})
		wg.Add(2)
		go func() { defer wg.Done(); <-start; retryErr = dm.Retry("gone") }()
		go func() { defer wg.Done(); <-start; dm.ClearFinished(nil) }()
		close(start)
		wg.Wait()

		missing := false
		if retryErr == nil {
			_, ok := dm.GetJob("gone")
			missing = !ok
		}
		close(held)
		if missing {
			t.Fatalf("iteration %d: Retry reported success and the job is not in the queue — nothing can list, cancel or retire it now", i)
		}
	}
}
