package download_queue

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"mahresources/models/query_models"
)

// This file drives the queue's canonical-publication seam: a submission that was
// already accepted as a durable Job carries that execution's identity, and every
// lifecycle fact the transfer produces is mirrored through it. The properties
// worth pinning are that the mirror is a no-op for a queue with no control plane,
// that progress and the terminal outcome both arrive, and that a job whose claim
// was replaced can adopt the execution that replaced it.

// recordedMirror is one mirrored fact, with the ref it was published under.
type recordedMirror struct {
	ref  CanonicalRef
	snap *DownloadJob
}

// recordingCanonicalSink records every mirror the queue publishes.
type recordingCanonicalSink struct {
	mu       sync.Mutex
	progress []recordedMirror
	held     []recordedMirror
	finished []recordedMirror
}

func (s *recordingCanonicalSink) DownloadProgress(ref CanonicalRef, snap *DownloadJob) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.progress = append(s.progress, recordedMirror{ref: ref, snap: snap})
	return nil
}

func (s *recordingCanonicalSink) DownloadHeld(ref CanonicalRef, snap *DownloadJob) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.held = append(s.held, recordedMirror{ref: ref, snap: snap})
	return nil
}

func (s *recordingCanonicalSink) DownloadFinished(ref CanonicalRef, snap *DownloadJob) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.finished = append(s.finished, recordedMirror{ref: ref, snap: snap})
	return nil
}

func (s *recordingCanonicalSink) finishedFor(jobID string) []recordedMirror {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]recordedMirror, 0, len(s.finished))
	for _, mirror := range s.finished {
		if mirror.ref.JobID == jobID {
			out = append(out, mirror)
		}
	}
	return out
}

// waitForCanonical polls until cond holds or the test times out. The queue's
// workers are goroutines, so every assertion about a mirrored fact is an assertion
// about something that happens after the submit returns.
func waitForCanonical(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// TestACanonicalSubmissionMirrorsItsProgressAndOutcome proves the two facts the
// control plane needs from the queue: the transfer reports through the execution
// it was accepted with, and it does so with that execution's own token.
func TestACanonicalSubmissionMirrorsItsProgressAndOutcome(t *testing.T) {
	dm := createTestManager()
	dm.resourceCtx = &capturingResourceCreator{}
	sink := &recordingCanonicalSink{}
	dm.SetCanonicalSink(sink)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("canonical content"))
	}))
	defer server.Close()

	ref := CanonicalRef{JobID: "0192f0aa-0000-7000-8000-000000000001", ExecutionToken: "0192f0aa-0000-7000-8000-0000000000t1"}
	job, err := dm.SubmitForPluginWithOptions(
		&query_models.ResourceFromRemoteCreator{URL: server.URL + "/file.txt"},
		nil, "", SubmissionOptions{JobID: "legacy-1", Canonical: &ref})
	if err != nil {
		t.Fatalf("submit a canonical download: %v", err)
	}
	if job.CanonicalJobID != ref.JobID {
		t.Fatalf("the queue entry names canonical job %q, want %q", job.CanonicalJobID, ref.JobID)
	}
	if job.Snapshot().CanonicalJobID != ref.JobID {
		t.Fatalf("the legacy snapshot does not carry the canonical job id")
	}

	waitForCanonical(t, "the transfer to finish", func() bool { return len(sink.finishedFor(ref.JobID)) > 0 })

	finished := sink.finishedFor(ref.JobID)
	if len(finished) != 1 {
		t.Fatalf("the terminal outcome was mirrored %d times, want once", len(finished))
	}
	if finished[0].ref.ExecutionToken != ref.ExecutionToken {
		t.Fatalf("the mirror published under token %q, want %q", finished[0].ref.ExecutionToken, ref.ExecutionToken)
	}
	if finished[0].snap.Status != JobStatusCompleted {
		t.Fatalf("the mirrored outcome is %s, want completed", finished[0].snap.Status)
	}
	if finished[0].snap.ResourceID == nil || *finished[0].snap.ResourceID == 0 {
		t.Fatalf("the mirrored outcome carries no created resource")
	}
	if len(sink.progress) == 0 {
		t.Fatalf("the transfer mirrored no progress at all")
	}

	// And it is reachable by the canonical identity, which is how a control or a
	// reconciliation finds the transfer publishing into one Job.
	found, ok := dm.GetJobByCanonicalJobID(ref.JobID)
	if !ok || found.ID != job.ID {
		t.Fatalf("the queue could not find the entry publishing into %s", ref.JobID)
	}
}

// TestAQueueWithNoControlPlaneMirrorsNothing is the compatibility half of the same
// property: the CLI's queue, the package's own tests and any embedder that never
// installed a control plane submit exactly as they did before, and a job with no
// execution mirrors nothing even when a sink is installed.
func TestAQueueWithNoControlPlaneMirrorsNothing(t *testing.T) {
	dm := createTestManager()
	dm.resourceCtx = &capturingResourceCreator{}
	sink := &recordingCanonicalSink{}
	dm.SetCanonicalSink(sink)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("plain content"))
	}))
	defer server.Close()

	job, err := dm.SubmitForPlugin(&query_models.ResourceFromRemoteCreator{URL: server.URL + "/file.txt"}, nil, "")
	if err != nil {
		t.Fatalf("submit a legacy download: %v", err)
	}
	if job.CanonicalJobID != "" {
		t.Fatalf("a legacy submission claimed canonical job %q", job.CanonicalJobID)
	}

	waitForCanonical(t, "the legacy transfer to finish", func() bool { return job.GetStatus() == JobStatusCompleted })
	if len(sink.progress) != 0 || len(sink.finished) != 0 || len(sink.held) != 0 {
		t.Fatalf("a job with no canonical execution was mirrored: %d progress, %d finished",
			len(sink.progress), len(sink.finished))
	}
}

// TestAPausedTransferIsMirroredAsHeld pins the one nonterminal fact that changes a
// Job's state. §1 defines `paused` as a checkpoint the executor confirmed, and this
// queue restarts a paused transfer from the beginning — so what it reports is that
// a person is holding the work, not a checkpoint it cannot produce.
func TestAPausedTransferIsMirroredAsHeld(t *testing.T) {
	dm := createTestManager()
	dm.resourceCtx = &capturingResourceCreator{}
	sink := &recordingCanonicalSink{}
	dm.SetCanonicalSink(sink)

	blocked := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-blocked
		_, _ = w.Write([]byte("slow content"))
	}))
	defer server.Close()
	defer close(blocked)

	ref := CanonicalRef{JobID: "0192f0aa-0000-7000-8000-000000000002", ExecutionToken: "0192f0aa-0000-7000-8000-0000000000t2"}
	job, err := dm.SubmitForPluginWithOptions(
		&query_models.ResourceFromRemoteCreator{URL: server.URL + "/slow.txt"},
		nil, "", SubmissionOptions{Canonical: &ref})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	waitForCanonical(t, "the transfer to start", func() bool { return job.GetStatus() == JobStatusDownloading })

	if err := dm.Pause(job.ID); err != nil {
		t.Fatalf("pause: %v", err)
	}
	waitForCanonical(t, "the pause to be mirrored", func() bool {
		sink.mu.Lock()
		defer sink.mu.Unlock()
		return len(sink.held) == 1
	})
	sink.mu.Lock()
	held := sink.held[0]
	sink.mu.Unlock()
	if held.ref.JobID != ref.JobID {
		t.Fatalf("the held fact named job %q, want %q", held.ref.JobID, ref.JobID)
	}
	if held.snap.Status != JobStatusPaused {
		t.Fatalf("the held snapshot is %s, want the queue's own paused state", held.snap.Status)
	}
}

// TestAnExecutionReplacedUnderATransferIsAdopted proves the fence can be handed
// over: a claim replaced by a reconciliation mints a new token, and the transfer
// that is still running adopts it rather than publishing under the one it lost. A
// ref naming a different Job is refused — that is the crossing the token prevents.
func TestAnExecutionReplacedUnderATransferIsAdopted(t *testing.T) {
	job := &DownloadJob{ID: "adopt-1", Source: JobSourceDownload}
	first := CanonicalRef{JobID: "job-a", ExecutionToken: "token-1"}
	if !job.AttachCanonical(first) {
		t.Fatalf("a transfer refused its own first execution")
	}
	second := CanonicalRef{JobID: "job-a", ExecutionToken: "token-2"}
	if !job.AttachCanonical(second) {
		t.Fatalf("a transfer refused the execution that replaced its own")
	}
	if ref, ok := job.CanonicalExecution(); !ok || ref.ExecutionToken != "token-2" {
		t.Fatalf("the transfer publishes under %+v, want the replacement token", ref)
	}
	if job.AttachCanonical(CanonicalRef{JobID: "job-b", ExecutionToken: "token-3"}) {
		t.Fatalf("a transfer adopted another job's execution")
	}
	if ref, _ := job.CanonicalExecution(); ref.JobID != "job-a" {
		t.Fatalf("the refused adoption changed the execution to %+v", ref)
	}
}
