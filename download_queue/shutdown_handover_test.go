package download_queue

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"mahresources/models/query_models"
)

// This file pins what a graceful shutdown does to a download it stops. Nobody
// cancelled that download, so nothing may record it as cancelled: a download a
// durable Job owns goes back to that Job, which starts it again in the next
// process, and one no Job owns is recorded as stopped by the shutdown.

// shutdownTestManager is a manager Shutdown can stop, with every observer the
// terminal edge reports to installed.
func shutdownTestManager() (*DownloadManager, *recordingCanonicalSink, *recordingHistory, *recordingJobEvents) {
	dm := createTestManager()
	dm.done = make(chan struct{})
	dm.cleanupTicker = time.NewTicker(time.Hour)
	dm.resourceCtx = &capturingResourceCreator{}
	sink := &recordingCanonicalSink{}
	history := &recordingHistory{}
	events := &recordingJobEvents{}
	dm.SetCanonicalSink(sink)
	dm.SetHistoryRecorder(history)
	dm.SetJobEventSink(events)
	return dm, sink, history, events
}

// stallingServer answers with headers and then sends nothing until the test ends.
func stallingServer(t *testing.T) *httptest.Server {
	t.Helper()
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1000")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("first bytes"))
		w.(http.Flusher).Flush()
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(func() {
		close(release)
		server.Close()
	})
	return server
}

func TestAShutdownHandsARunningCanonicalDownloadBackToItsJob(t *testing.T) {
	dm, sink, history, events := shutdownTestManager()
	server := stallingServer(t)

	ref := CanonicalRef{JobID: "0192f0aa-0000-7000-8000-00000000sd01", ExecutionToken: "token-sd01"}
	job, err := dm.SubmitForPluginWithOptions(&query_models.ResourceFromRemoteCreator{URL: server.URL + "/video.bin"},
		nil, "", SubmissionOptions{JobID: "legacy-sd01", Canonical: &ref})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	waitForCanonical(t, "the transfer to start", func() bool { return job.GetStatus() == JobStatusDownloading })

	dm.Shutdown()

	sink.mu.Lock()
	interrupted := append([]recordedMirror(nil), sink.interrupted...)
	sink.mu.Unlock()
	if len(interrupted) != 1 || interrupted[0].ref != ref {
		t.Fatalf("the Job was handed back %d times (%+v), want once under its own execution", len(interrupted), interrupted)
	}
	if finished := sink.finishedFor(ref.JobID); len(finished) != 0 {
		t.Fatalf("the shutdown published an outcome for the Job: %s", finished[0].snap.Status)
	}
	if records := history.all(); len(records) != 0 {
		t.Fatalf("the shutdown wrote a history row for work its Job goes on with: %+v", records)
	}
	if got := events.all(); len(got) != 0 {
		t.Fatalf("the shutdown announced a terminal event for work that did not end: %+v", got)
	}
	if status := job.GetStatus(); status == JobStatusCancelled {
		t.Fatalf("the entry reads %s, as if a person had cancelled it", status)
	}
}

func TestAShutdownLeavesAHeldCanonicalDownloadHeld(t *testing.T) {
	dm, sink, history, events := shutdownTestManager()
	server := stallingServer(t)

	ref := CanonicalRef{JobID: "0192f0aa-0000-7000-8000-00000000sd02", ExecutionToken: "token-sd02"}
	job, err := dm.SubmitForPluginWithOptions(&query_models.ResourceFromRemoteCreator{URL: server.URL + "/held.bin"},
		nil, "", SubmissionOptions{JobID: "legacy-sd02", Canonical: &ref})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	waitForCanonical(t, "the transfer to start", func() bool { return job.GetStatus() == JobStatusDownloading })
	if err := dm.Pause(job.ID); err != nil {
		t.Fatalf("pause: %v", err)
	}
	waitForCanonical(t, "the pause", func() bool { return job.GetStatus() == JobStatusPaused })

	dm.Shutdown()

	sink.mu.Lock()
	finished, interrupted := len(sink.finished), len(sink.interrupted)
	sink.mu.Unlock()
	if finished != 0 || interrupted != 0 {
		t.Fatalf("the held Job was published by the shutdown: %d outcomes, %d hand-backs", finished, interrupted)
	}
	if records := history.all(); len(records) != 0 {
		t.Fatalf("the shutdown recorded a held download as ended: %+v", records)
	}
	if got := events.all(); len(got) != 0 {
		t.Fatalf("the shutdown announced a held download as ended: %+v", got)
	}
}

// A person's cancel that landed first is still a cancel: the shutdown that
// followed it did not stop the download, the person did.
func TestACancelBeforeTheShutdownStaysACancel(t *testing.T) {
	dm, _, history, _ := shutdownTestManager()
	server := stallingServer(t)

	job, err := dm.Submit(&query_models.ResourceFromRemoteCreator{URL: server.URL + "/cancelled.bin"}, nil)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	waitForCanonical(t, "the transfer to start", func() bool { return job.GetStatus() == JobStatusDownloading })
	if _, _, ok := job.claimCancel(time.Now()); !ok {
		t.Fatal("the cancel was refused")
	}

	dm.Shutdown()

	records := history.all()
	if len(records) != 1 || records[0].Status != string(JobStatusCancelled) {
		t.Fatalf("history after a cancel then a shutdown = %+v, want one cancelled row", records)
	}
}

// assertStoppedByShutdown checks what a download no Job owns records when a
// shutdown stops it: the reason, and not the word a person's cancel uses.
func assertStoppedByShutdown(t *testing.T, rec HistoryRecord) {
	t.Helper()
	if rec.Status != string(JobStatusFailed) {
		t.Errorf("status = %q, want failed", rec.Status)
	}
	if !strings.Contains(rec.Error, "shut down") {
		t.Errorf("error = %q, want it to say the server shut down", rec.Error)
	}
}
