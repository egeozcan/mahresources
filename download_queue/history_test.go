package download_queue

import (
	"context"
	"encoding/json"
	"mahresources/models/query_models"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// recordingHistory is a HistoryRecorder that keeps what it was handed.
type recordingHistory struct {
	mu      sync.Mutex
	records []HistoryRecord
	err     error
}

func (r *recordingHistory) RecordTerminalDownload(rec HistoryRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.records = append(r.records, rec)
	return r.err
}

func (r *recordingHistory) all() []HistoryRecord {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]HistoryRecord, len(r.records))
	copy(out, r.records)
	return out
}

func waitForRecords(t *testing.T, h *recordingHistory, want int) []HistoryRecord {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if got := h.all(); len(got) >= want {
			return got
		}
		time.Sleep(5 * time.Millisecond)
	}
	got := h.all()
	t.Fatalf("expected %d history record(s), got %d: %+v", want, len(got), got)
	return nil
}

// A download that fails is recorded once, with the reason and the submitted
// payload — the payload is what makes a retry possible after the job itself has
// been evicted from the in-memory queue.
func TestFailedDownloadIsRecorded(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusNotFound)
	}))
	defer server.Close()

	history := &recordingHistory{}
	dm := createTestManager()
	dm.resourceCtx = &capturingResourceCreator{}
	dm.SetHistoryRecorder(history)

	owner := uint(7)
	job, err := dm.Submit(&query_models.ResourceFromRemoteCreator{
		URL:               server.URL,
		ResourceQueryBase: query_models.ResourceQueryBase{Name: "wanted"},
	}, &owner)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	records := waitForRecords(t, history, 1)
	if len(records) != 1 {
		t.Fatalf("expected exactly one record, got %d", len(records))
	}
	rec := records[0]
	if rec.JobID != job.ID {
		t.Errorf("job id = %q, want %q", rec.JobID, job.ID)
	}
	if rec.Status != string(JobStatusFailed) {
		t.Errorf("status = %q, want failed", rec.Status)
	}
	if rec.Error == "" {
		t.Error("expected a recorded error message")
	}
	if rec.CreatedByUserId == nil || *rec.CreatedByUserId != owner {
		t.Errorf("owner = %v, want %d", rec.CreatedByUserId, owner)
	}
	if rec.CompletedAt == nil {
		t.Error("expected a completion time")
	}
	var payload query_models.ResourceFromRemoteCreator
	if err := json.Unmarshal(rec.Payload, &payload); err != nil {
		t.Fatalf("payload is not decodable: %v", err)
	}
	if payload.Name != "wanted" {
		t.Errorf("payload name = %q, want %q", payload.Name, "wanted")
	}
}

// A paused download that is then cancelled is terminal too, and its terminal
// state is stamped by Cancel rather than by a worker — so that branch has to
// record as well, or a cancelled-while-paused download would never be listed.
func TestCancelOfPausedDownloadIsRecorded(t *testing.T) {
	history := &recordingHistory{}
	dm := createTestManager()
	dm.SetHistoryRecorder(history)

	job := addTestJob(dm, "paused-one", JobStatusPaused)
	job.Source = JobSourceDownload
	job.creator = &query_models.ResourceFromRemoteCreator{URL: "http://example.invalid/x"}
	job.URL = "http://example.invalid/x"

	if err := dm.Cancel(job.ID); err != nil {
		t.Fatalf("cancel: %v", err)
	}

	records := waitForRecords(t, history, 1)
	if records[0].Status != string(JobStatusCancelled) {
		t.Errorf("status = %q, want cancelled", records[0].Status)
	}
}

// Generic jobs (exports, imports, plugin actions) are not downloads: they have no
// URL to retry and their artifacts have their own retention, so they must not
// reach the history table at all.
func TestGenericJobIsNotRecorded(t *testing.T) {
	history := &recordingHistory{}
	dm := createTestManager()
	dm.SetHistoryRecorder(history)

	done := make(chan struct{})
	if _, err := dm.SubmitJobWithOptions(JobOptions{Source: JobSourceGroupExport}, func(ctx context.Context, j *DownloadJob, p ProgressSink) error {
		close(done)
		return nil
	}); err != nil {
		t.Fatalf("submit job: %v", err)
	}

	<-done
	// Give any (incorrect) recording a chance to land before asserting absence.
	time.Sleep(50 * time.Millisecond)
	if got := history.all(); len(got) != 0 {
		t.Fatalf("expected no history records for a generic job, got %+v", got)
	}
}

// A recorder that errors must not change the job's outcome: history is a
// side-record, not part of the download's contract.
func TestHistoryRecorderErrorDoesNotAffectJob(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("hello"))
	}))
	defer server.Close()

	history := &recordingHistory{err: context.DeadlineExceeded}
	dm := createTestManager()
	dm.resourceCtx = &capturingResourceCreator{}
	dm.SetHistoryRecorder(history)

	job, err := dm.Submit(&query_models.ResourceFromRemoteCreator{URL: server.URL}, nil)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && job.GetStatus() != JobStatusCompleted {
		time.Sleep(5 * time.Millisecond)
	}
	if status := job.GetStatus(); status != JobStatusCompleted {
		t.Fatalf("status = %q, want completed", status)
	}
	waitForRecords(t, history, 1)
}

// A manager with no recorder wired (the default, and every existing test) keeps
// working — the sink is optional.
func TestNoRecorderIsSafe(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("hello"))
	}))
	defer server.Close()

	dm := createTestManager()
	dm.resourceCtx = &capturingResourceCreator{}

	job, err := dm.Submit(&query_models.ResourceFromRemoteCreator{URL: server.URL}, nil)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && job.GetStatus() != JobStatusCompleted {
		time.Sleep(5 * time.Millisecond)
	}
	if status := job.GetStatus(); status != JobStatusCompleted {
		t.Fatalf("status = %q, want completed", status)
	}
}

// A download cancelled while it was still waiting for a worker slot never runs,
// so it never reaches processJob's ordinary terminal write — its status is
// stamped by the semaphore branch instead. It is a failed download as far as the
// user is concerned, and the one they are most likely to look for later: it is
// recorded, with the payload that makes it retryable.
func TestCancelledBeforeStartingIsRecorded(t *testing.T) {
	history := &recordingHistory{}
	dm := createTestManager()
	dm.resourceCtx = &capturingResourceCreator{}
	dm.SetHistoryRecorder(history)

	// One slot, already taken: the submitted job's worker blocks on the semaphore
	// and can only leave through the cancelled-before-starting branch.
	dm.semaphore = make(chan struct{}, 1)
	dm.semaphore <- struct{}{}

	owner := uint(11)
	job, err := dm.Submit(&query_models.ResourceFromRemoteCreator{
		URL:               "http://127.0.0.1:9/never",
		ResourceQueryBase: query_models.ResourceQueryBase{Name: "queued and abandoned"},
	}, &owner)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	if err := dm.Cancel(job.ID); err != nil {
		t.Fatalf("cancel: %v", err)
	}

	records := waitForRecords(t, history, 1)
	if len(records) != 1 {
		t.Fatalf("expected exactly one record, got %d: %+v", len(records), records)
	}
	rec := records[0]
	if rec.Status != string(JobStatusCancelled) {
		t.Errorf("status = %q, want cancelled", rec.Status)
	}
	if rec.JobID != job.ID {
		t.Errorf("job id = %q, want %q", rec.JobID, job.ID)
	}
	if rec.CreatedByUserId == nil || *rec.CreatedByUserId != owner {
		t.Errorf("owner = %v, want %d", rec.CreatedByUserId, owner)
	}
	if len(rec.Payload) == 0 {
		t.Error("no payload stored: the row could not be retried")
	}
}

// The record describes the terminal state that was stamped, not whatever the job
// looked like by the time the write happened. Subscribers are notified before the
// row is written, so a client can press Retry in between — and a record read from
// the live job then would say `pending`, which is not a terminal status, is not
// retryable, and falls outside every retention window. Driven directly rather
// than through a real download because the window is a scheduling accident: it
// has to be opened deliberately to be tested at all.
func TestRecordDescribesTheStampedState(t *testing.T) {
	history := &recordingHistory{}
	dm := createTestManager()
	dm.SetHistoryRecorder(history)

	job := addTestJob(dm, "stamped", JobStatusDownloading)
	job.Source = JobSourceDownload
	job.creator = &query_models.ResourceFromRemoteCreator{URL: "http://example.com/x"}
	job.URL = job.creator.URL
	runID, _ := job.attempt()

	snap, stamped := job.finishSnapshot(runID, JobStatusFailed, "boom", 0, time.Now())
	if !stamped {
		t.Fatalf("the terminal write was refused")
	}

	// The retry a client presses on the failure it was just told about. Everything
	// the record needs is now gone from the live job: it is pending again, with no
	// completion time.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if _, ok := job.claimRetry(ctx, cancel); !ok {
		t.Fatalf("the retry was refused")
	}

	dm.recordTerminal(job, snap)

	records := waitForRecords(t, history, 1)
	if records[0].Status != string(JobStatusFailed) {
		t.Errorf("status = %q, want failed: the record was read from the job after a retry had reset it", records[0].Status)
	}
	if records[0].Error != "boom" {
		t.Errorf("error = %q, want %q", records[0].Error, "boom")
	}
	if records[0].CompletedAt == nil {
		t.Error("no completion time: the row would never expire")
	}
}

// A job whose history row the user has just deleted must not have that row put
// back by a terminal write that was still on its way to the store.
func TestDiscardedJobIsNotRecorded(t *testing.T) {
	history := &recordingHistory{}
	dm := createTestManager()
	dm.SetHistoryRecorder(history)

	job := addTestJob(dm, "discarded", JobStatusDownloading)
	job.Source = JobSourceDownload
	job.creator = &query_models.ResourceFromRemoteCreator{URL: "http://example.com/x"}
	runID, _ := job.attempt()
	snap, stamped := job.finishSnapshot(runID, JobStatusFailed, "boom", 0, time.Now())
	if !stamped {
		t.Fatalf("the terminal write was refused")
	}

	// The delete: it takes the queue entry with the row.
	if removed := dm.RemoveFinished([]string{job.ID}, nil); len(removed) != 1 {
		t.Fatalf("RemoveFinished released %d jobs, want 1", len(removed))
	}

	dm.recordTerminal(job, snap)

	time.Sleep(20 * time.Millisecond)
	if got := history.all(); len(got) != 0 {
		t.Fatalf("the deleted download was written back to the store: %+v", got)
	}
}

// A deployment restart cancels whatever is downloading. The record of that
// cancellation is the point of the history table, so Shutdown waits for the
// workers to write it instead of exiting under them.
func TestShutdownWaitsForTheTerminalWrite(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
	}))
	defer server.Close()
	defer close(release)

	history := &recordingHistory{}
	dm := createTestManager()
	dm.done = make(chan struct{})
	dm.cleanupTicker = time.NewTicker(time.Hour)
	dm.resourceCtx = &capturingResourceCreator{}
	dm.SetHistoryRecorder(history)

	job, err := dm.Submit(&query_models.ResourceFromRemoteCreator{URL: server.URL}, nil)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && job.GetStatus() != JobStatusDownloading {
		time.Sleep(5 * time.Millisecond)
	}

	dm.Shutdown()

	// No polling: by the time Shutdown returns the write must already have
	// happened, which is the whole difference the drain makes.
	records := history.all()
	if len(records) != 1 {
		t.Fatalf("history records after shutdown = %d, want 1: the process would have exited before the download's outcome was stored", len(records))
	}
	if records[0].Status != string(JobStatusCancelled) {
		t.Errorf("status = %q, want cancelled", records[0].Status)
	}
}

// A paused download has no worker left to stamp anything — Pause ended it — so a
// restart would take it away with the process and leave no record at all. It is
// abandoned like any cancellation on the way out, which is also what a restart
// does to it in fact.
func TestShutdownRecordsPausedDownloads(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
	}))
	defer server.Close()
	defer close(release)

	history := &recordingHistory{}
	dm := createTestManager()
	dm.done = make(chan struct{})
	dm.cleanupTicker = time.NewTicker(time.Hour)
	dm.resourceCtx = &capturingResourceCreator{}
	dm.SetHistoryRecorder(history)

	job, err := dm.Submit(&query_models.ResourceFromRemoteCreator{URL: server.URL}, nil)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && job.GetStatus() != JobStatusDownloading {
		time.Sleep(5 * time.Millisecond)
	}
	if err := dm.Pause(job.ID); err != nil {
		t.Fatalf("pause: %v", err)
	}
	for time.Now().Before(deadline) && job.GetStatus() != JobStatusPaused {
		time.Sleep(5 * time.Millisecond)
	}
	if status := job.GetStatus(); status != JobStatusPaused {
		t.Fatalf("status = %q, want paused", status)
	}

	dm.Shutdown()

	records := history.all()
	if len(records) != 1 {
		t.Fatalf("history records after shutdown = %d, want 1: the paused download left no record", len(records))
	}
	if records[0].Status != string(JobStatusCancelled) {
		t.Errorf("status = %q, want cancelled", records[0].Status)
	}
	if len(records[0].Payload) == 0 {
		t.Error("no payload stored: the row could not be retried after the restart")
	}
}
