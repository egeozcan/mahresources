package application_context

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"mahresources/download_queue"
	"mahresources/jobs"
	"mahresources/models"
	"mahresources/models/query_models"
)

// stallingDownloadServer sends headers and a few bytes, then nothing until the
// test ends, so a download stays running for as long as a test needs it to.
func stallingDownloadServer(t *testing.T) *httptest.Server {
	t.Helper()
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100000")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("the first bytes"))
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

// TestAGracefulShutdownReturnsARunningDownloadToTheQueue: nobody cancelled a
// download the server's own shutdown stopped. Its Job goes back to the queue, says
// why, and is started again by the next process; it is not recorded as cancelled,
// and no history row says it ended.
func TestAGracefulShutdownReturnsARunningDownloadToTheQueue(t *testing.T) {
	ctx := newJobHarnessContext(t, false)
	server := stallingDownloadServer(t)

	submissions := ctx.SubmitRemoteDownloads(&query_models.ResourceFromRemoteCreator{URL: server.URL + "/long.bin"}, nil, "", "api")
	if len(submissions) != 1 || submissions[0].Err != nil || submissions[0].Job == nil {
		t.Fatalf("submit: %+v", submissions)
	}
	jobID := submissions[0].CanonicalJobID
	waitForSnapshot(t, ctx, jobID, "the transfer to run", func(snap jobs.Snapshot) bool {
		return snap.State == jobs.StateRunning && submissions[0].Job.GetStatus() == download_queue.JobStatusDownloading
	})

	ctx.downloadManager.Shutdown()

	snap, err := ctx.GetJob(jobID)
	if err != nil {
		t.Fatalf("read the job: %v", err)
	}
	if snap.State != jobs.StateQueued || snap.Failure != nil {
		t.Fatalf("after the shutdown the job is %s (%+v), want queued with no failure", snap.State, snap.Failure)
	}
	if !strings.Contains(snap.Progress.Message, "server shutdown") || snap.Progress.Completed != nil {
		t.Fatalf("the queued row says %+v, want the shutdown named and no stale progress", snap.Progress)
	}
	timeline, err := ctx.GetJobTimeline(jobID, 0, 0)
	if err != nil {
		t.Fatalf("timeline: %v", err)
	}
	last := timeline[len(timeline)-1]
	var detail map[string]string
	_ = json.Unmarshal(last.Detail, &detail)
	if last.Type != jobs.EventQueued || detail["reason"] != JobDownloadServerShutdownReason {
		t.Fatalf("the last event is %s %s, want queued with reason %q", last.Type, last.Detail, JobDownloadServerShutdownReason)
	}
	for _, event := range timeline {
		if event.Type == jobs.EventCancelled {
			t.Fatalf("the timeline records a cancellation: %+v", timeline)
		}
	}
	var rows int64
	if err := ctx.db.Model(&models.DownloadHistoryEntry{}).Count(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Fatalf("the shutdown wrote %d history rows for a download that goes on", rows)
	}
}

// A deferred download that ran leaves a legacy history row like any other
// download, and its queue entry's id resolves to its Job: the id is the one the
// download surfaces show, so it is recorded as a handle of the Job when the
// transfer starts.
func TestAFiredDeferredDownloadLeavesAHistoryRow(t *testing.T) {
	ctx := newDownloadJobContext(t)
	enableDownloadTestPlugin(t, ctx)
	actor, err := ctx.CreateUser(&UserInput{Username: "deferred-history", Password: "password1", Role: models.RoleUser})
	if err != nil {
		t.Fatal(err)
	}
	row, err := ctx.CreateScheduledDownload(downloadTestPlugin, actor.ID,
		&query_models.ResourceFromRemoteCreator{URL: "https://example.invalid/deferred-history.bin"}, time.Now().Add(-time.Second))
	if err != nil {
		t.Fatalf("create the deferred download: %v", err)
	}
	job := deferredDownloadJob(t, ctx, row.ID)
	waitForSnapshot(t, ctx, job.ID, "the deferred download to run and end",
		func(snap jobs.Snapshot) bool { return snap.StartedAt != nil && snap.State.Terminal() })

	entry, found := ctx.downloadManager.GetJobByCanonicalJobID(job.ID)
	if !found {
		t.Fatal("the deferred download never reached the queue")
	}
	if resolved, err := ctx.JobService().ResolveLegacyHandle(ctx.jobDeps(), DownloadHandleNamespace, entry.ID); err != nil || resolved != job.ID {
		t.Fatalf("the queue entry's id resolves to %q (%v), want the deferred Job %s", resolved, err, job.ID)
	}
	deadline := time.Now().Add(5 * time.Second)
	var history models.DownloadHistoryEntry
	for {
		err := ctx.db.Where("job_id = ?", entry.ID).First(&history).Error
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the deferred download left no history row: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	var mapping models.JobSourceMapping
	if err := ctx.db.Where("source_kind = ? AND source_id = ?", jobMigrationDownloadHistory,
		strconv.FormatUint(uint64(history.ID), 10)).First(&mapping).Error; err != nil || mapping.JobID != job.ID {
		t.Fatalf("the history row maps to %q (%v), want the deferred Job %s", mapping.JobID, err, job.ID)
	}
}
