package application_context

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"mahresources/download_queue"
	"mahresources/jobs"
	"mahresources/models"
	"mahresources/models/query_models"
)

// gatedDownloadServer holds every request until release is closed (or the
// request is abandoned), then answers each with bytes of its own.
func gatedDownloadServer(t *testing.T) (*httptest.Server, chan struct{}) {
	t.Helper()
	release := make(chan struct{})
	var served atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
			_, _ = fmt.Fprintf(w, "answer %d", served.Add(1))
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
		server.Close()
	})
	return server, release
}

func hasEvent(t *testing.T, ctx *MahresourcesContext, jobID, eventType string) bool {
	t.Helper()
	timeline, err := ctx.GetJobTimeline(jobID, 0, 0)
	if err != nil {
		t.Fatalf("timeline of %s: %v", jobID, err)
	}
	for _, event := range timeline {
		if event.Type == eventType {
			return true
		}
	}
	return false
}

// submitRunning submits one URL and waits until its transfer is running.
func submitRunning(t *testing.T, ctx *MahresourcesContext, url string) (string, *download_queue.DownloadJob) {
	t.Helper()
	submissions := ctx.SubmitRemoteDownloads(&query_models.ResourceFromRemoteCreator{URL: url}, nil, "", "api")
	if len(submissions) != 1 || submissions[0].Err != nil || submissions[0].Job == nil {
		t.Fatalf("submit %s: %+v", url, submissions)
	}
	entry := submissions[0].Job
	waitForSnapshot(t, ctx, submissions[0].CanonicalJobID, "the transfer to run", func(jobs.Snapshot) bool {
		return entry.GetStatus() == download_queue.JobStatusDownloading
	})
	return submissions[0].CanonicalJobID, entry
}

func cancelJob(t *testing.T, ctx *MahresourcesContext, jobID string) {
	t.Helper()
	snap, err := ctx.GetJob(jobID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ctx.ExecuteJobCommand(context.Background(), jobs.CommandRequest{
		JobID: jobID, Key: jobs.CommandCancel, IdempotencyKey: "cancel-" + jobID + "-" + strconv.FormatUint(snap.Version, 10),
		ExpectedVersion: snap.Version,
	}); err != nil {
		t.Fatalf("cancel %s: %v", jobID, err)
	}
}

// retryWhileTheURLDownloads cancels one transfer of url, starts another, and
// retries the cancelled one while the second is still running.
func retryWhileTheURLDownloads(t *testing.T, ctx *MahresourcesContext, url string) (running, successor string) {
	t.Helper()
	first, _ := submitRunning(t, ctx, url)
	cancelJob(t, ctx, first)
	cancelled := waitForSnapshot(t, ctx, first, "the first transfer to be cancelled",
		func(snap jobs.Snapshot) bool { return snap.State == jobs.StateCancelled })

	running, _ = submitRunning(t, ctx, url)
	retried := retryJob(t, ctx, cancelled)
	waitForSnapshot(t, ctx, retried.ID, "the Retry to wait for the running transfer", func(snap jobs.Snapshot) bool {
		return snap.State == jobs.StateRunning && snap.Progress.Phase == "waiting"
	})
	return running, retried.ID
}

// A Retry of a download whose URL another transfer is fetching right now waits for
// that transfer, says so, and then starts on its own. It is never left blocked
// with nothing to release it.
func TestARetryOfAURLThatIsDownloadingWaitsForItAndThenRuns(t *testing.T) {
	ctx := newDownloadJobContext(t)
	server, release := gatedDownloadServer(t)
	running, successor := retryWhileTheURLDownloads(t, ctx, server.URL+"/wanted.bin")

	if snap, _ := ctx.GetJob(successor); snap.Progress.Message == "" {
		t.Fatalf("the waiting Retry does not say what it waits for: %+v", snap.Progress)
	}
	close(release)

	waitForSnapshot(t, ctx, running, "the running transfer to finish",
		func(snap jobs.Snapshot) bool { return snap.State == jobs.StateSucceeded })
	done := waitForSnapshot(t, ctx, successor, "the Retry to run once the URL is free",
		func(snap jobs.Snapshot) bool { return snap.State.Terminal() })
	if done.State != jobs.StateSucceeded {
		t.Fatalf("the Retry ended %s (%+v)", done.State, done.Failure)
	}
	if hasEvent(t, ctx, successor, jobs.EventBlocked) {
		t.Fatalf("the Retry was blocked on its way")
	}
}

// A waiting Retry answers a cancel without waiting for the transfer it is
// waiting on, and leaves that transfer alone.
func TestAWaitingRetryCanBeCancelled(t *testing.T) {
	ctx := newDownloadJobContext(t)
	server, release := gatedDownloadServer(t)
	running, successor := retryWhileTheURLDownloads(t, ctx, server.URL+"/cancel-waiting.bin")

	cancelJob(t, ctx, successor)
	waitForSnapshot(t, ctx, successor, "the waiting Retry to be cancelled",
		func(snap jobs.Snapshot) bool { return snap.State == jobs.StateCancelled })
	if snap, _ := ctx.GetJob(running); snap.State != jobs.StateRunning {
		t.Fatalf("cancelling the waiting Retry moved the running transfer to %s", snap.State)
	}
	close(release)
	waitForSnapshot(t, ctx, running, "the running transfer to finish",
		func(snap jobs.Snapshot) bool { return snap.State == jobs.StateSucceeded })
}

// A due deferred row that has a durable Job is handed to that Job whatever the
// row sweep could check: whether its plugin is enabled, whether its URL is
// downloading. The Job's own dispatch decides both, once, so the row cannot be
// failed or moved while its Job goes on to run.
func TestADueDeferredRowIsHandedToItsJobWhateverTheSweepSees(t *testing.T) {
	cases := map[string]ScheduledDownloadFireConfig{
		"its plugin is disabled": {PluginAvailable: func(string) bool { return false }},
		"its URL is downloading": {
			PluginAvailable: func(string) bool { return true },
			ActiveDownload:  func(string) (string, bool) { return "another-transfer", true },
		},
	}
	for name, cfg := range cases {
		t.Run(name, func(t *testing.T) {
			ctx := newJobHarnessContext(t, false)
			actor, err := ctx.CreateUser(&UserInput{Username: "deferrer", Password: "password1", Role: models.RoleUser})
			if err != nil {
				t.Fatal(err)
			}
			due := time.Now().Add(-time.Second)
			row, err := ctx.CreateScheduledDownload(downloadTestPlugin, actor.ID,
				&query_models.ResourceFromRemoteCreator{URL: "https://example.test/deferred.bin"}, due)
			if err != nil {
				t.Fatalf("create the deferred download: %v", err)
			}
			job := deferredDownloadJob(t, ctx, row.ID)

			cfg.Now = time.Now()
			cfg.Submit = func(*query_models.ResourceFromRemoteCreator, *uint, string) (string, error) {
				t.Fatalf("a deferred row with a durable Job was submitted to the queue")
				return "", nil
			}
			if _, err := ctx.FireDueScheduledDownloads(cfg); err != nil {
				t.Fatalf("fire: %v", err)
			}
			got := scheduledDownloadRow(t, ctx, row.ID)
			if got.Status != models.ScheduledDownloadStatusSubmitted || got.JobID != job.ID {
				t.Fatalf("the row is %s naming %q (%s), want submitted naming its Job %s", got.Status, got.JobID, got.LastError, job.ID)
			}
			if !got.DueAt.Equal(row.DueAt) {
				t.Fatalf("the row's due time moved from %v to %v while its Job's did not", row.DueAt, got.DueAt)
			}
			if snap, _ := ctx.GetJob(job.ID); snap.State != jobs.StateQueued {
				t.Fatalf("the Job is %s, want queued for its own dispatch to decide", snap.State)
			}
		})
	}
}
