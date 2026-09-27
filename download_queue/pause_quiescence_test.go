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
