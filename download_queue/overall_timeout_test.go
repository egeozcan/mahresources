package download_queue

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"mahresources/models/query_models"
)

// A download that runs past the overall time limit was stopped by the limit, and
// says so. Nobody cancelled it, so "download cancelled" is the wrong answer, and
// net/http's own "Client.Timeout or context cancellation" names neither cause.

func overallTimeoutManager(overall time.Duration) *DownloadManager {
	dm := createTestManager()
	dm.resourceCtx = &capturingResourceCreator{}
	dm.settings = NewStaticDownloadSettings(TimeoutConfig{
		ConnectTimeout: 5 * time.Second,
		IdleTimeout:    5 * time.Second,
		OverallTimeout: overall,
	}, 0)
	return dm
}

func waitForTerminalStatus(t *testing.T, job *DownloadJob) *DownloadJob {
	t.Helper()
	waitForCanonical(t, "the download to end", func() bool { return downloadQueueStatusTerminal(job.GetStatus()) })
	return job.Snapshot()
}

func assertStoppedByTheOverallLimit(t *testing.T, snap *DownloadJob) {
	t.Helper()
	if snap.Status != JobStatusFailed {
		t.Fatalf("status = %s, want failed: nobody cancelled it", snap.Status)
	}
	for _, text := range []string{snap.Error, snap.FailureReason} {
		if !strings.Contains(text, "overall time limit") || strings.Contains(strings.ToLower(text), "cancel") {
			t.Fatalf("the reason %q does not name the overall time limit", text)
		}
	}
	if snap.FailureCode != FailureOverallTimeout {
		t.Fatalf("failure code = %q, want %q", snap.FailureCode, FailureOverallTimeout)
	}
}

func TestAStreamThatOutrunsTheOverallLimitSaysSo(t *testing.T) {
	for _, chunked := range []bool{false, true} {
		name := "with a length"
		if chunked {
			name = "chunked"
		}
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !chunked {
					w.Header().Set("Content-Length", "100000")
				}
				w.WriteHeader(http.StatusOK)
				for i := 0; i < 200; i++ {
					if _, err := w.Write([]byte("x")); err != nil {
						return
					}
					w.(http.Flusher).Flush()
					select {
					case <-r.Context().Done():
						return
					case <-time.After(20 * time.Millisecond):
					}
				}
			}))
			defer server.Close()

			dm := overallTimeoutManager(300 * time.Millisecond)
			job, err := dm.Submit(&query_models.ResourceFromRemoteCreator{URL: server.URL + "/slow.bin"}, nil)
			if err != nil {
				t.Fatalf("submit: %v", err)
			}
			assertStoppedByTheOverallLimit(t, waitForTerminalStatus(t, job))
		})
	}
}

func TestAServerSlowerToAnswerThanTheOverallLimitSaysSo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(3 * time.Second):
		}
	}))
	defer server.Close()

	dm := overallTimeoutManager(200 * time.Millisecond)
	job, err := dm.Submit(&query_models.ResourceFromRemoteCreator{URL: server.URL + "/late.bin"}, nil)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	assertStoppedByTheOverallLimit(t, waitForTerminalStatus(t, job))
}

// A person's cancel is still a cancel when an overall limit is configured.
func TestACancelUnderAnOverallLimitIsStillACancel(t *testing.T) {
	server := stallingServer(t)
	dm := overallTimeoutManager(time.Minute)
	job, err := dm.Submit(&query_models.ResourceFromRemoteCreator{URL: server.URL + "/cancel.bin"}, nil)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	waitForCanonical(t, "the transfer to start", func() bool { return job.GetStatus() == JobStatusDownloading })
	if err := dm.Cancel(job.ID); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if snap := waitForTerminalStatus(t, job); snap.Status != JobStatusCancelled {
		t.Fatalf("status = %s (%s), want cancelled", snap.Status, snap.Error)
	}
}
