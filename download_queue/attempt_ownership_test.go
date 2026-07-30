package download_queue

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mahresources/contracts"
	"mahresources/models"
	"mahresources/models/query_models"
	"strings"
	"sync"
	"testing"
	"time"
)

// UI bug hunt 2026-07-29, round-3 audit of download_queue.
//
// Three rounds of review found five, six and five real defects in this package, at a
// steady rate — which says the state space had been sampled rather than covered. The
// matrix in docs/todo.md is that coverage; these are the cells that turned out to be
// broken.
//
// The single sentence they all come from: a job in pending, downloading or
// processing belongs to the attempt running it, and a job that is paused or terminal
// belongs to whichever control put it there. Only the owner may write.

// -------------------------------------------------------------- finding 1

// The attempt's *other* writes were never guarded. runID reached claimStart and
// finish, and in between the transfer's two callbacks wrote whatever they liked:
// progress on every chunk, and `processing` at EOF.
//
// The damaging order is EOF -> callback notifies subscribers -> a Pause claims
// `paused` and cancels the context in that gap -> the callback writes `processing`
// over it. finish then accepts, because it *is* the same attempt, and retires the
// job. The caller had been told 200 {"status":"paused"}; nothing about the job says
// it was ever paused.
//
// Driven through the production callbacks rather than through a live transfer: the
// window between EOF and the status write is a few instructions inside one Read, so
// landing a control in it by timing is not something a test can promise. What the
// type makes decidable is the property that closes it.
func TestAttemptReporter_WritesAreRefusedOnceAControlOwnsTheJob(t *testing.T) {
	newReporter := func(job *DownloadJob) *attemptReporter {
		runID, _ := job.attempt()
		return &attemptReporter{dm: createTestManager(), job: job, runID: runID, total: 8192}
	}

	t.Run("the EOF callback cannot overwrite a pause it raced with", func(t *testing.T) {
		dm := createTestManager()
		job := addTestJob(dm, "j", JobStatusDownloading)
		reporter := newReporter(job)

		if err := dm.Pause("j"); err != nil {
			t.Fatalf("pausing a downloading job failed: %v", err)
		}

		reporter.onComplete()

		if got := job.GetStatus(); got != JobStatusPaused {
			t.Errorf("the transfer's EOF callback left the job %q; the pause was answered 200 and then undone", got)
		}
	})

	t.Run("progress cannot move on a job a control has taken", func(t *testing.T) {
		dm := createTestManager()
		job := addTestJob(dm, "j", JobStatusDownloading)
		job.UpdateProgress(4096, 8192)
		reporter := newReporter(job)

		if err := dm.Pause("j"); err != nil {
			t.Fatalf("pausing failed: %v", err)
		}

		reporter.onProgress(7168)

		// A pause freezes the readout the panel shows. A straggler write moves it, and
		// the reader watches a paused download apparently keep downloading.
		if got := job.Snapshot().Progress; got != 4096 {
			t.Errorf("a paused job's progress moved to %d; it was frozen at 4096", got)
		}
	})

	t.Run("a stale attempt cannot write over the attempt that replaced it", func(t *testing.T) {
		dm := createTestManager()
		job := addTestJob(dm, "j", JobStatusDownloading)
		stale := newReporter(job) // attempt A

		if err := dm.Pause("j"); err != nil {
			t.Fatalf("pausing failed: %v", err)
		}
		if err := dm.Resume("j"); err != nil {
			t.Fatalf("resuming failed: %v", err)
		}
		// Attempt B is the job's own now. Give it a state of its own to be overwritten.
		liveRun, _ := job.attempt()
		if !job.setStatusForRun(liveRun, JobStatusDownloading) {
			t.Fatalf("the resumed attempt could not claim its own status")
		}
		if !job.updateProgressForRun(liveRun, 512, 8192) {
			t.Fatalf("the resumed attempt could not report its own progress")
		}

		stale.onProgress(7168)
		stale.onComplete()

		snap := job.Snapshot()
		if snap.Progress != 512 {
			t.Errorf("a stale attempt moved the live attempt's progress to %d, want 512", snap.Progress)
		}
		if snap.Status != JobStatusDownloading {
			t.Errorf("a stale attempt left the live job %q, want %q", snap.Status, JobStatusDownloading)
		}
	})

	t.Run("the control: an attempt that still owns the job writes normally", func(t *testing.T) {
		dm := createTestManager()
		job := addTestJob(dm, "j", JobStatusDownloading)
		reporter := newReporter(job)

		reporter.onProgress(2048)
		reporter.onComplete()

		snap := job.Snapshot()
		if snap.Progress != 2048 {
			t.Errorf("progress = %d, want 2048 — the guard refused a write it owns", snap.Progress)
		}
		if snap.Status != JobStatusProcessing {
			t.Errorf("status = %q, want %q — the guard refused a transition it owns", snap.Status, JobStatusProcessing)
		}
	})
}

// -------------------------------------------------------------- finding 2

// parkingCreator hands its body to the test and waits, so the test decides exactly
// when AddResource returns and what it returns. That is what makes the interleaving
// below a sequence rather than a hope: while the call is parked the worker cannot
// reach its terminal write, so any number of controls can run first.
type parkingCreator struct {
	entered  chan struct{}
	release  chan struct{}
	resource *models.Resource
	err      error
}

func newParkingCreator(resource *models.Resource, err error) *parkingCreator {
	return &parkingCreator{entered: make(chan struct{}, 4), release: make(chan struct{}), resource: resource, err: err}
}

func (p *parkingCreator) AddResource(file contracts.File, _ string, _ *query_models.ResourceCreator) (*models.Resource, error) {
	_, _ = io.CopyN(io.Discard, file, 1)
	select {
	case p.entered <- struct{}{}:
	default:
	}
	<-p.release
	return p.resource, p.err
}

func (p *parkingCreator) waitParked(t *testing.T) {
	t.Helper()
	select {
	case <-p.entered:
	case <-time.After(15 * time.Second):
		t.Fatalf("the worker never reached AddResource, so nothing was mid-flight to test")
	}
}

func (p *parkingCreator) releaseOne(t *testing.T) {
	t.Helper()
	select {
	case p.release <- struct{}{}:
	case <-time.After(15 * time.Second):
		t.Fatalf("no worker was parked to release")
	}
}

// inFlightJobWith is inFlightJob with a caller-supplied creator.
func inFlightJobWith(t *testing.T, creator ResourceCreator) (*DownloadManager, *DownloadJob) {
	t.Helper()

	dm := createTestManager()
	dm.resourceCtx = creator
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
	return dm, job
}

// A terminal state a control wrote is the control's, and the worker does not get to
// take it back.
//
// finish refused a *paused* job and nothing else, so an attempt whose AddResource
// happened to succeed after the job had already been cancelled wrote `completed`
// straight over it — with a resource id. What the reader saw: pause a download,
// cancel it, watch the row settle on "Cancelled", and then watch it turn into
// "Completed" a moment later. Two answered controls and neither of them holds.
//
// Fully sequenced: the worker is parked inside AddResource for both controls, and
// what it returns when released is the test's choice.
func TestFinish_AWorkerCannotUnretireAJobAControlAlreadyRetired(t *testing.T) {
	creator := newParkingCreator(&models.Resource{ID: 42}, nil)
	dm, job := inFlightJobWith(t, creator)
	creator.waitParked(t)

	if err := dm.Pause(job.ID); err != nil {
		t.Fatalf("pausing a downloading job failed: %v", err)
	}
	if err := dm.Cancel(job.ID); err != nil {
		t.Fatalf("cancelling the paused job failed: %v", err)
	}
	if got := job.GetStatus(); got != JobStatusCancelled {
		t.Fatalf("precondition: the job is %q after an answered cancel, want %q", got, JobStatusCancelled)
	}

	// Now the attempt's own AddResource succeeds anyway — it was already past the
	// bytes when the cancel landed.
	creator.releaseOne(t)

	// Give the worker every chance to write. A poll that stops at the first good
	// sample would pass against the broken code simply by looking too early.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if got := job.GetStatus(); got != JobStatusCancelled {
			t.Fatalf("the worker un-retired a cancelled job as %q; the cancel had already been answered and shown", got)
		}
		time.Sleep(5 * time.Millisecond)
	}
	if snap := job.Snapshot(); snap.ResourceID != nil {
		t.Errorf("the cancelled job carries resource id %d, so the row offers a link it should never have had", *snap.ResourceID)
	}
}

// The positive control: with no control in the way, the same worker and the same
// successful AddResource do retire the job as completed. Without this, a finish that
// refused everything would pass the test above.
func TestFinish_AnUncontestedAttemptStillCompletesTheJob(t *testing.T) {
	creator := newParkingCreator(&models.Resource{ID: 42}, nil)
	dm, job := inFlightJobWith(t, creator)
	creator.waitParked(t)
	_ = dm

	creator.releaseOne(t)

	if status := settle(t, job); status != JobStatusCompleted {
		t.Fatalf("an uncontested download settled at %q, want %q", status, JobStatusCompleted)
	}
	snap := job.Snapshot()
	if snap.ResourceID == nil || *snap.ResourceID != 42 {
		t.Errorf("the completed job's resource id is %v, want 42", snap.ResourceID)
	}
	if snap.CompletedAt == nil {
		t.Error("the completed job has no CompletedAt, so job retention will never retire it")
	}
}

// -------------------------------------------------------------- finding 3

// Everything the manager hands out is a copy.
//
// JobEvent.Job was the live job on every download path — generic_job.go already
// snapshotted, which is how the inconsistency was visible — and the SSE handler
// marshals it on its own goroutine holding no job lock, while the worker writes
// progress into it. This is that race, driven under a real transfer; run the package
// with -race and it is the detector that fails, not an assertion.
func TestJobPayloads_AreSafeToMarshalWhileAnAttemptWrites(t *testing.T) {
	creator := &drainingCreator{}
	dm, job := inFlightJobWith(t, creator)

	events, unsub := dm.Subscribe()
	defer unsub()

	stop := make(chan struct{})
	var wg sync.WaitGroup

	// The SSE stream: marshal every event as it arrives.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case ev, ok := <-events:
				if !ok {
					return
				}
				_, _ = json.Marshal(ev)
			case <-stop:
				return
			}
		}
	}()

	// The queue listing: marshal the whole queue, over and over.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				_, _ = json.Marshal(map[string]any{"jobs": dm.GetJobs()})
			}
		}
	}()

	// Long enough for the transfer to produce a stream of progress writes.
	time.Sleep(300 * time.Millisecond)
	_ = dm.Cancel(job.ID)
	close(stop)
	wg.Wait()

	if status := settle(t, job); status != JobStatusCancelled {
		t.Fatalf("the cancelled download settled at %q, want %q", status, JobStatusCancelled)
	}
}

// drainingCreator reads until the transfer stops, which keeps the progress callbacks
// firing for as long as the test needs them.
type drainingCreator struct{}

func (drainingCreator) AddResource(file contracts.File, _ string, _ *query_models.ResourceCreator) (*models.Resource, error) {
	if _, err := io.Copy(io.Discard, file); err != nil {
		return nil, err
	}
	return &models.Resource{ID: 1}, nil
}

// The listing is a copy, so a caller holding one is not reading a job the worker is
// still writing to.
func TestGetJobs_HandsOutCopiesRatherThanTheLiveJobs(t *testing.T) {
	dm := createTestManager()
	live := addTestJob(dm, "j", JobStatusDownloading)
	live.UpdateProgress(1024, 8192)

	listing := dm.GetJobs()
	if len(listing) != 1 {
		t.Fatalf("got %d jobs, want 1", len(listing))
	}
	// The control: the copy is of the job, at the moment it was asked for.
	if listing[0].ID != "j" || listing[0].Progress != 1024 {
		t.Fatalf("the listing reports id %q progress %d, want \"j\" and 1024", listing[0].ID, listing[0].Progress)
	}

	live.UpdateProgress(2048, 8192)

	if listing[0].Progress != 1024 {
		t.Errorf("the listing's progress followed the job to %d — the caller was handed the live job, and encoding it races the worker", listing[0].Progress)
	}
	if listing[0] == live {
		t.Error("the listing contains the live job pointer")
	}
}

// The same for the event stream.
func TestJobEvent_CarriesACopyOfTheJob(t *testing.T) {
	dm := createTestManager()
	job := addTestJob(dm, "j", JobStatusDownloading)
	job.UpdateProgress(1024, 8192)

	events, unsub := dm.Subscribe()
	defer unsub()

	dm.notifyJob("updated", job)

	var ev JobEvent
	select {
	case ev = <-events:
	case <-time.After(time.Second):
		t.Fatalf("no event arrived")
	}
	if ev.Job.Progress != 1024 {
		t.Fatalf("the event reports progress %d, want 1024", ev.Job.Progress)
	}

	job.UpdateProgress(2048, 8192)

	if ev.Job.Progress != 1024 {
		t.Errorf("the event's progress followed the job to %d — subscribers marshal this on their own goroutine", ev.Job.Progress)
	}
}

// -------------------------------------------------------------- finding 4

// A retry starts a run, not a continuation of the last one.
//
// Error, progress, timings and the resource id were all cleared; the three things a
// generic job *reports* were not. A retried import re-reports every warning it hits,
// so the failed attempt's list stayed underneath and the count climbed with each
// retry; ResultPath still named a tar this attempt had not written; and the phase
// counters read as a run part-way through a phase that had not begun.
func TestClaimRetry_ClearsThePreviousAttemptsReport(t *testing.T) {
	dm := createTestManager()
	job := addTestJob(dm, "j", JobStatusFailed)
	job.runFn = func(context.Context, *DownloadJob, ProgressSink) error { return nil }
	job.AppendWarning("skipped 3 resources")
	job.SetResultPath("_exports/j.tar")
	job.SetPhaseProgress(7, 12)
	job.SetError("boom")

	if err := dm.Retry("j"); err != nil {
		t.Fatalf("retrying a failed job failed: %v", err)
	}

	snap := job.Snapshot()
	if len(snap.Warnings) != 0 {
		t.Errorf("the retried job still carries %d warning(s) from the attempt that failed: %v", len(snap.Warnings), snap.Warnings)
	}
	if snap.ResultPath != "" {
		t.Errorf("the retried job still names %q, a result the new attempt has not produced", snap.ResultPath)
	}
	if snap.PhaseCount != 0 || snap.PhaseTotal != 0 {
		t.Errorf("the retried job reports phase progress %d/%d from the previous attempt", snap.PhaseCount, snap.PhaseTotal)
	}
	// The control, which was already right: the fields the retry always reset.
	if snap.Error != "" || snap.Status != JobStatusPending {
		t.Errorf("the retried job is %q with error %q, want pending and no error", snap.Status, snap.Error)
	}
}

// -------------------------------------------------------------- finding 5

// A generic job's owner is part of what it is, not something set on it afterwards.
//
// The export and import handlers called SetOwnerUserID on the job SubmitJob returned
// — by which point "added" had been broadcast and the worker had been started. Under
// -auth the SSE stream drops any event whose job the principal may not see, and an
// ownerless job is exactly that for a non-admin: their own export's "added" was
// filtered out, the early "updated" events were dropped by the panel as an unknown
// id, and the row only appeared on the next reconnect.
func TestSubmitJobWithOptions_TheAddedEventAlreadyNamesTheOwner(t *testing.T) {
	dm := createTestManager()
	events, unsub := dm.Subscribe()
	defer unsub()

	owner := uint(7)
	release := make(chan struct{})
	defer close(release)
	if _, err := dm.SubmitJobWithOptions(JobOptions{
		Source:       JobSourceGroupExport,
		InitialPhase: "queued",
		URL:          "_imports/staged.tar",
		OwnerUserID:  &owner,
	}, func(context.Context, *DownloadJob, ProgressSink) error {
		<-release
		return nil
	}); err != nil {
		t.Fatalf("submitting a generic job failed: %v", err)
	}

	var added JobEvent
	select {
	case added = <-events:
	case <-time.After(2 * time.Second):
		t.Fatalf("no event arrived")
	}
	if added.Type != "added" {
		t.Fatalf("the first event is %q, want \"added\" — nothing may describe a job before the event that says it exists", added.Type)
	}
	got := added.Job.GetOwnerUserID()
	if got == nil || *got != owner {
		t.Errorf("the added event's owner is %v, want %d — under -auth this event is filtered on it and the submitter never sees their own job", got, owner)
	}
	if added.Job.URL != "_imports/staged.tar" {
		t.Errorf("the added event's source path is %q, want the staged tar; a cancel landing here finds nothing to clean up", added.Job.URL)
	}
}

// The control: an unowned job is still submitted and still announced, so the fix did
// not turn the owner into a requirement.
func TestSubmitJob_StillWorksWithoutAnOwner(t *testing.T) {
	dm := createTestManager()
	events, unsub := dm.Subscribe()
	defer unsub()

	job, err := dm.SubmitJob(JobSourceGroupExport, "queued", func(context.Context, *DownloadJob, ProgressSink) error {
		return nil
	})
	if err != nil {
		t.Fatalf("submitting a generic job failed: %v", err)
	}

	select {
	case ev := <-events:
		if ev.Type != "added" || ev.Job.ID != job.ID {
			t.Fatalf("first event is %q for %q, want \"added\" for %q", ev.Type, ev.Job.ID, job.ID)
		}
		if ev.Job.GetOwnerUserID() != nil {
			t.Errorf("an unowned job reports owner %v", ev.Job.GetOwnerUserID())
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("no event arrived")
	}
	if status := settle(t, job); status != JobStatusCompleted {
		t.Errorf("the unowned job settled at %q, want %q", status, JobStatusCompleted)
	}
}

// -------------------------------------------------------------- under load

// The matrix's concurrent cells, repeated, so -race has something to look at and so
// an interleaving that only shows up occasionally has many chances to. Every
// iteration asserts the same invariant: whatever a control was told, the job's
// settled state agrees with it.
func TestControlsAndWorkerAgree_UnderLoad(t *testing.T) {
	if testing.Short() {
		t.Skip("load loop skipped in -short")
	}

	for i := 0; i < 40; i++ {
		creator := newParkingCreator(nil, errors.New("released by the test"))
		dm, job := inFlightJobWith(t, creator)
		creator.waitParked(t)

		// Four controls at once, at a job that is genuinely mid-transfer.
		var wg sync.WaitGroup
		var cancelErr, pauseErr error
		start := make(chan struct{})
		wg.Add(3)
		go func() { defer wg.Done(); <-start; cancelErr = dm.Cancel(job.ID) }()
		go func() { defer wg.Done(); <-start; pauseErr = dm.Pause(job.ID) }()
		go func() { defer wg.Done(); <-start; dm.ClearFinished(nil) }()
		close(start)
		wg.Wait()

		creator.releaseOne(t)
		status := settle(t, job)

		switch {
		case cancelErr == nil:
			// A cancel that was accepted must not leave a job offering Resume, and
			// must leave one retention can retire.
			if status != JobStatusCancelled {
				t.Fatalf("iteration %d: Cancel returned nil, Pause returned %v, and the job settled at %q", i, pauseErr, status)
			}
			if job.GetCompletedAt() == nil {
				t.Fatalf("iteration %d: the cancelled job has no CompletedAt", i)
			}
		case pauseErr == nil:
			// The mirror: a pause that was accepted must survive its worker unwinding.
			if status != JobStatusPaused {
				t.Fatalf("iteration %d: Pause returned nil, Cancel returned %v, and the job settled at %q", i, cancelErr, status)
			}
		default:
			t.Fatalf("iteration %d: both controls refused a mid-flight job (cancel=%v pause=%v)", i, cancelErr, pauseErr)
		}
	}
}

// -------------------------------------------------- the reader's own buffer

// abandonedReader parks inside Read, then writes into whatever buffer it was given
// once the test says so — which is the situation a cancelled or timed-out transfer
// leaves behind: TimeoutReaderWithContext returns to its caller while the goroutine
// it spawned is still waiting on the remote.
type abandonedReader struct {
	entered chan struct{}
	release chan struct{}
	wrote   chan struct{}
}

func (r *abandonedReader) Read(p []byte) (int, error) {
	close(r.entered)
	<-r.release
	n := copy(p, []byte("XXXXXXXX"))
	close(r.wrote)
	return n, nil
}

// A read this reader has walked away from must not write into the caller's buffer.
//
// It used to hand the caller's p straight to the goroutine, so after a cancellation
// the abandoned read wrote into memory the caller owns again — and `io.Copy` takes
// its buffer from a pool for some destinations, so that memory can already belong to
// a different transfer. Under -race across two concurrent downloads it shows up as
// exactly that: one address, two writers, from two unrelated AddResource calls.
func TestTimeoutReader_AnAbandonedReadDoesNotTouchTheCallersBuffer(t *testing.T) {
	src := &abandonedReader{
		entered: make(chan struct{}),
		release: make(chan struct{}),
		wrote:   make(chan struct{}),
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tr := NewTimeoutReaderWithContext(src, time.Minute, ctx)
	defer tr.Close()

	p := make([]byte, 8)
	returned := make(chan error, 1)
	go func() {
		_, err := tr.Read(p)
		returned <- err
	}()

	<-src.entered
	cancel()

	select {
	case err := <-returned:
		if err == nil {
			t.Fatalf("a cancelled read returned no error")
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("Read never returned after the context was cancelled")
	}

	// The caller has its buffer back. Now let the abandoned read do its write.
	close(src.release)
	<-src.wrote

	for i, b := range p {
		if b != 0 {
			t.Fatalf("the abandoned read wrote %q at byte %d of a buffer its caller had taken back", b, i)
		}
	}
}

// The control: an ordinary read still delivers its bytes to the caller's buffer.
func TestTimeoutReader_DeliversWhatItReads(t *testing.T) {
	tr := NewTimeoutReaderWithContext(strings.NewReader("hello world"), time.Minute, context.Background())
	defer tr.Close()

	got, err := io.ReadAll(tr)
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	if string(got) != "hello world" {
		t.Errorf("read %q, want %q", got, "hello world")
	}
}
