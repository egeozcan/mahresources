package download_queue

import (
	"errors"
	"testing"
	"time"

	"mahresources/contracts"
	"mahresources/models"
	"mahresources/models/query_models"
)

// parkingJobCreator is parkingCreator for a canonical download, which saves its
// file through the receipt-capable seam.
type parkingJobCreator struct{ *parkingCreator }

func (p parkingJobCreator) AddResourceForJob(_ string, _ *uint, file contracts.File, fileName string, query *query_models.ResourceCreator) (*models.Resource, error) {
	return p.AddResource(file, fileName, query)
}

// parkedCanonicalTransfer submits one canonical download whose AddResource parks,
// so a test decides when the attempt's last write happens.
func parkedCanonicalTransfer(t *testing.T, creator *parkingCreator) (*DownloadManager, *DownloadJob, *recordingCanonicalSink) {
	t.Helper()
	dm := createTestManager()
	dm.resourceCtx = parkingJobCreator{creator}
	dm.settings = NewStaticDownloadSettings(TimeoutConfig{
		ConnectTimeout: 5 * time.Second,
		IdleTimeout:    30 * time.Second,
		OverallTimeout: time.Minute,
	}, 0)
	sink := &recordingCanonicalSink{}
	dm.SetCanonicalSink(sink)
	srv := trickleServer(t)
	ref := CanonicalRef{JobID: "0192f0aa-0000-7000-8000-00000000pa01", ExecutionToken: "0192f0aa-0000-7000-8000-00000000pt01"}
	job, err := dm.SubmitForPluginWithOptions(&query_models.ResourceFromRemoteCreator{URL: srv.URL + "/slow.dat"},
		nil, "", SubmissionOptions{Canonical: &ref})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	return dm, job, sink
}

func heldMirrors(sink *recordingCanonicalSink) int {
	sink.mu.Lock()
	defer sink.mu.Unlock()
	return len(sink.held)
}

// A pause is confirmed to the durable Job only once the attempt it stopped has
// unwound. Confirming it at Pause released the Job's claim and capacity while the
// attempt could still be inside AddResource, so a Resume could start a second
// transfer beside the first one's last writes.
func TestAPauseIsConfirmedOnlyOnceTheAttemptItStoppedHasUnwound(t *testing.T) {
	creator := newParkingCreator(nil, errors.New("the transfer was stopped"))
	dm, job, sink := parkedCanonicalTransfer(t, creator)
	creator.waitParked(t)

	if err := dm.Pause(job.ID); err != nil {
		t.Fatalf("pause: %v", err)
	}
	if job.HoldSettled() {
		t.Fatalf("the hold reads settled while its attempt is still inside AddResource")
	}
	time.Sleep(50 * time.Millisecond)
	if n := heldMirrors(sink); n != 0 {
		t.Fatalf("the hold was confirmed %d times while its attempt could still write", n)
	}

	creator.releaseOne(t)
	waitForWorkerToExit(t, dm)
	waitForCanonical(t, "the hold to be confirmed", func() bool { return heldMirrors(sink) == 1 })
	if !job.HoldSettled() || job.GetStatus() != JobStatusPaused {
		t.Fatalf("after its attempt unwound the job is %s, settled=%v", job.GetStatus(), job.HoldSettled())
	}
}

// A pause that lands after the attempt's AddResource has saved the file has landed
// too late: the download is done, and it completes with its resource rather than
// waiting paused beside a file it already created.
func TestAPauseThatLandedAfterTheFileWasSavedCompletesTheDownload(t *testing.T) {
	creator := newParkingCreator(&models.Resource{ID: 77}, nil)
	dm, job, sink := parkedCanonicalTransfer(t, creator)
	creator.waitParked(t)

	if err := dm.Pause(job.ID); err != nil {
		t.Fatalf("pause: %v", err)
	}
	creator.releaseOne(t)
	waitForWorkerToExit(t, dm)

	snap := job.Snapshot()
	if snap.Status != JobStatusCompleted || snap.ResourceID == nil || *snap.ResourceID != 77 {
		t.Fatalf("a download whose file was saved before the pause took effect is %s (resource %v), want completed with it",
			snap.Status, snap.ResourceID)
	}
	if n := heldMirrors(sink); n != 0 {
		t.Fatalf("the completed download was also confirmed held %d times", n)
	}
	ref, _ := job.CanonicalExecution()
	waitForCanonical(t, "the completion to be mirrored", func() bool { return len(sink.finishedFor(ref.JobID)) == 1 })
}

// A hold is published under the execution its attempt ran for. A Resume
// dispatched between the settlement and the publication attaches a new execution;
// publishing under whatever is attached then would pause the new execution's
// running Job while its transfer runs.
func TestAHoldIsPublishedUnderTheExecutionItsAttemptRanFor(t *testing.T) {
	creator := newParkingCreator(nil, errors.New("the transfer was stopped"))
	dm, job, sink := parkedCanonicalTransfer(t, creator)
	creator.waitParked(t)
	old, _ := job.CanonicalExecution()
	replacement := CanonicalRef{JobID: old.JobID, ExecutionToken: "0192f0aa-0000-7000-8000-00000000pt02"}

	holdPublicationHookForTest = func(held *DownloadJob) {
		if !held.AttachCanonical(replacement) {
			t.Errorf("the replacement execution was not attached")
		}
	}
	t.Cleanup(func() { holdPublicationHookForTest = nil })

	if err := dm.Pause(job.ID); err != nil {
		t.Fatalf("pause: %v", err)
	}
	creator.releaseOne(t)
	waitForWorkerToExit(t, dm)
	waitForCanonical(t, "the hold to be published", func() bool { return heldMirrors(sink) == 1 })
	sink.mu.Lock()
	published := sink.held[0].ref
	sink.mu.Unlock()
	if published.ExecutionToken != old.ExecutionToken {
		t.Fatalf("the hold was published under %q, want the attempt's own %q", published.ExecutionToken, old.ExecutionToken)
	}
}

// PauseSettled answers once the hold is recorded, and a hold still settling
// when the caller stops waiting is answered as pending rather than as paused.
func TestPauseSettledAnswersOnceTheHoldIsRecorded(t *testing.T) {
	creator := newParkingCreator(nil, errors.New("the transfer was stopped"))
	dm, job, sink := parkedCanonicalTransfer(t, creator)
	creator.waitParked(t)

	var pending *HoldPendingError
	if err := dm.PauseSettled(job.ID, 50*time.Millisecond); !errors.As(err, &pending) {
		t.Fatalf("a pause whose attempt is still saving answered %v, want a pending hold", err)
	}
	creator.releaseOne(t)
	waitForWorkerToExit(t, dm)
	if status, recorded := job.holdAnswer(); status != JobStatusPaused || !recorded || heldMirrors(sink) != 1 {
		t.Fatalf("after the attempt exited the job is %s, recorded=%v, held mirrors %d", status, recorded, heldMirrors(sink))
	}

	// With nothing being saved the hold settles at once.
	quick := newParkingCreator(nil, errors.New("the transfer was stopped"))
	dm2, job2, sink2 := parkedCanonicalTransfer(t, quick)
	quick.waitParked(t)
	go func() {
		time.Sleep(20 * time.Millisecond)
		quick.releaseOne(t)
	}()
	if err := dm2.PauseSettled(job2.ID, 5*time.Second); err != nil {
		t.Fatalf("a pause answered %v, want the hold recorded", err)
	}
	if heldMirrors(sink2) != 1 {
		t.Fatalf("PauseSettled answered before the hold was published")
	}
}

// pauseSettledOverParkedSave asks PauseSettled to pause a transfer parked inside
// AddResource and lets the save return once the pause has landed, so the attempt
// the pause stopped is the one that exits.
func pauseSettledOverParkedSave(t *testing.T, dm *DownloadManager, job *DownloadJob, creator *parkingCreator, wait time.Duration) error {
	t.Helper()
	answer := make(chan error, 1)
	go func() { answer <- dm.PauseSettled(job.ID, wait) }()
	waitForCanonical(t, "the pause to land", func() bool { return job.GetStatus() == JobStatusPaused })
	creator.releaseOne(t)
	return <-answer
}

// A hold is confirmed only by the durable Job's answer. A publication the Job
// refused (its execution moved on) or could not write leaves the Job unpaused, so
// the person who asked is told the pause is not confirmed rather than "paused".
func TestAHoldTheJobDidNotRecordIsNotConfirmed(t *testing.T) {
	creator := newParkingCreator(nil, errors.New("the transfer was stopped"))
	dm, job, sink := parkedCanonicalTransfer(t, creator)
	sink.heldAnswer = func(CanonicalRef) HoldRecord { return HoldNotRecorded }
	creator.waitParked(t)

	var pending *HoldPendingError
	if err := pauseSettledOverParkedSave(t, dm, job, creator, 300*time.Millisecond); !errors.As(err, &pending) {
		t.Fatalf("a pause whose hold the Job did not record answered %v, want a pending hold", err)
	}
	if status, recorded := job.holdAnswer(); status != JobStatusPaused || recorded {
		t.Fatalf("the unrecorded hold reads %s, recorded=%v", status, recorded)
	}
}

// A cancellation that ended the Job while the hold was being told to it is the
// outcome: the entry is cancelled with it, its history row says so, and a pause
// waiting on the entry answers what the download became rather than "paused".
func TestAHoldACancellationEndedCancelsTheEntry(t *testing.T) {
	creator := newParkingCreator(nil, errors.New("the transfer was stopped"))
	dm, job, sink := parkedCanonicalTransfer(t, creator)
	history := &recordingHistory{}
	dm.SetHistoryRecorder(history)
	sink.heldAnswer = func(CanonicalRef) HoldRecord { return HoldCancelled }
	creator.waitParked(t)

	var conflict *StateConflictError
	if err := pauseSettledOverParkedSave(t, dm, job, creator, 5*time.Second); !errors.As(err, &conflict) || conflict.Status != JobStatusCancelled {
		t.Fatalf("a pause a cancellation overtook answered %v, want the cancellation", err)
	}
	if status := job.GetStatus(); status != JobStatusCancelled {
		t.Fatalf("the entry of a cancelled Job is %s, want cancelled", status)
	}
	if records := waitForRecords(t, history, 1); records[0].Status != string(JobStatusCancelled) {
		t.Fatalf("the history row says %q, want cancelled", records[0].Status)
	}
}

// Asking again after a pause timed out is safe: the hold is published again under
// the execution it settled with, and once the Job records it the answer is the
// same as if the first ask had waited long enough.
func TestAPauseAskedAgainAfterATimeoutIsConfirmed(t *testing.T) {
	creator := newParkingCreator(nil, errors.New("the transfer was stopped"))
	dm, job, sink := parkedCanonicalTransfer(t, creator)
	original, _ := job.CanonicalExecution()
	answers := make(chan HoldRecord, 2)
	answers <- HoldNotRecorded
	answers <- HoldRecorded
	sink.heldAnswer = func(CanonicalRef) HoldRecord { return <-answers }
	creator.waitParked(t)

	var pending *HoldPendingError
	if err := pauseSettledOverParkedSave(t, dm, job, creator, 300*time.Millisecond); !errors.As(err, &pending) {
		t.Fatalf("the first ask answered %v, want a pending hold", err)
	}
	waitForCanonical(t, "the first publication", func() bool { return heldMirrors(sink) == 1 })
	if err := dm.PauseSettled(job.ID, 5*time.Second); err != nil {
		t.Fatalf("asking again answered %v, want the hold confirmed", err)
	}
	sink.mu.Lock()
	defer sink.mu.Unlock()
	if len(sink.held) != 2 || sink.held[1].ref != original {
		t.Fatalf("the hold was published %d times, the last under %+v; want twice, under %+v", len(sink.held), sink.held[len(sink.held)-1].ref, original)
	}
}

// A retry that arrives while the first publication is still being written waits
// for it rather than failing because the download is already paused.
func TestAPauseAskedAgainWhileItsHoldIsWrittenWaitsForIt(t *testing.T) {
	creator := newParkingCreator(nil, errors.New("the transfer was stopped"))
	dm, job, _ := parkedCanonicalTransfer(t, creator)
	release := make(chan struct{})
	holdPublicationHookForTest = func(*DownloadJob) { <-release }
	t.Cleanup(func() { holdPublicationHookForTest = nil })
	creator.waitParked(t)

	var pending *HoldPendingError
	if err := pauseSettledOverParkedSave(t, dm, job, creator, 300*time.Millisecond); !errors.As(err, &pending) {
		t.Fatalf("the first ask answered %v, want a pending hold", err)
	}
	close(release)
	if err := dm.PauseSettled(job.ID, 5*time.Second); err != nil {
		t.Fatalf("asking again answered %v, want the hold confirmed", err)
	}
}
