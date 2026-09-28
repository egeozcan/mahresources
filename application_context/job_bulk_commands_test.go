package application_context

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"mahresources/jobs"
	"mahresources/models/query_models"
)

// TestSeveralDownloadsCanBeRetriedAndCancelledAtOnce pins that Cancel and Retry
// are offered in bulk, and that a bulk Retry is each Job's own Retry: a Job
// that no longer offers it (here one already retried) is refused for itself,
// with a reason, while the others get their successors.
func TestSeveralDownloadsCanBeRetriedAndCancelledAtOnce(t *testing.T) {
	ctx := newDownloadJobContext(t)
	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "no", http.StatusInternalServerError)
	}))
	t.Cleanup(failing.Close)
	release := make(chan struct{})
	hanging := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1048576")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(func() { close(release); hanging.Close() })

	submit := func(server *httptest.Server, name string) string {
		t.Helper()
		submissions := ctx.SubmitRemoteDownloads(&query_models.ResourceFromRemoteCreator{URL: server.URL + "/" + name}, nil, "", "api")
		if len(submissions) != 1 || submissions[0].Err != nil {
			t.Fatalf("submit %s: %+v", name, submissions)
		}
		return submissions[0].CanonicalJobID
	}

	var failed []string
	for i := range 3 {
		id := submit(failing, fmt.Sprintf("bulk-retry-%d.bin", i))
		waitForSnapshot(t, ctx, id, "the download to fail", func(snap jobs.Snapshot) bool { return snap.State == jobs.StateFailed })
		failed = append(failed, id)
	}
	first, err := ctx.GetJob(failed[0])
	if err != nil {
		t.Fatalf("read the first failure: %v", err)
	}
	retried, err := ctx.ExecuteJobCommand(context.Background(), jobs.CommandRequest{
		JobID: failed[0], Key: jobs.CommandRetry, IdempotencyKey: "single-retry", ExpectedVersion: first.Version, Origin: "api",
	})
	if err != nil || retried.SuccessorID == "" {
		t.Fatalf("retry the first failure alone: %+v, %v", retried, err)
	}

	results := ctx.ExecuteBulkJobCommand(context.Background(), jobs.BulkCommandRequest{
		JobIDs: failed, Key: jobs.CommandRetry, IdempotencyKey: "bulk-retry", Origin: "api",
	})
	if len(results) != 3 {
		t.Fatalf("a bulk Retry answered %d results, want one per Job", len(results))
	}
	if results[0].Status != jobs.CommandStatusFailed || results[0].Message == "" || results[0].SuccessorID != "" {
		t.Fatalf("the Job retried already = %+v, want refused for itself with a reason", results[0])
	}
	for _, result := range results[1:] {
		if result.Status != jobs.CommandStatusSucceeded || result.Code != jobs.CommandCodeApplied || result.SuccessorID == "" {
			t.Fatalf("a retryable failure = %+v, want its own successor", result)
		}
	}

	var running []string
	for i := range 2 {
		id := submit(hanging, fmt.Sprintf("bulk-cancel-%d.bin", i))
		waitForSnapshot(t, ctx, id, "the download to run", func(snap jobs.Snapshot) bool { return snap.State == jobs.StateRunning })
		running = append(running, id)
	}
	for _, id := range running {
		commands, err := ctx.AdvertisedJobCommands(context.Background(), id)
		if err != nil {
			t.Fatalf("read the commands of %s: %v", id, err)
		}
		offered := false
		for _, command := range commands {
			offered = offered || (command.Key == jobs.CommandCancel && command.Bulk && command.Destructive)
		}
		if !offered {
			t.Fatalf("a running download offers %+v, want Cancel in bulk, still marked destructive so it asks first", commands)
		}
	}
	cancelled := ctx.ExecuteBulkJobCommand(context.Background(), jobs.BulkCommandRequest{
		JobIDs: running, Key: jobs.CommandCancel, IdempotencyKey: "bulk-cancel", Origin: "api",
	})
	for _, result := range cancelled {
		if result.Status != jobs.CommandStatusSucceeded {
			t.Fatalf("a bulk Cancel of a running download = %+v", result)
		}
	}
	for _, id := range running {
		waitForSnapshot(t, ctx, id, "the download to be cancelled", func(snap jobs.Snapshot) bool { return snap.State == jobs.StateCancelled })
	}
}
