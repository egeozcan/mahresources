package application_context

import (
	"context"
	"testing"
	"time"

	"mahresources/download_queue"
	"mahresources/models/query_models"
)

// A plugin command's live entry names the Job it runs for, as every other row of the
// legacy queue listing does, so a legacy client can follow it to the canonical Job.
func TestTheLegacyQueueListingNamesAPluginCommandEntrysJob(t *testing.T) {
	ctx := newJobHarnessContext(t, false)
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	entry, err := ctx.downloadManager.SubmitManagedJob(download_queue.ManagedJobOptions{
		JobOptions:     download_queue.JobOptions{Source: pluginCommandJobSource, InitialPhase: "starting command"},
		CanonicalJobID: "01aaaaaa-0000-7000-8000-000000000001",
	}, func(context.Context, *download_queue.DownloadJob, download_queue.ManagedProgressSink) download_queue.ManagedJobOutcome {
		<-release
		return download_queue.ManagedJobOutcome{Status: download_queue.JobStatusCompleted}
	})
	if err != nil {
		t.Fatalf("submit the managed entry: %v", err)
	}

	rows, err := ctx.ProjectDownloadQueue()
	if err != nil {
		t.Fatalf("project the queue: %v", err)
	}
	row, found := rowForHandle(rows, entry.ID)
	if !found {
		t.Fatalf("the listing dropped the command's entry: %+v", rows)
	}
	if row.CanonicalJobID != "01aaaaaa-0000-7000-8000-000000000001" {
		t.Fatalf("the command's row names Job %q, want the Job it runs for", row.CanonicalJobID)
	}

	projection, err := ctx.ProjectDownloadJob(entry.ID)
	if err != nil {
		t.Fatalf("project the entry by its id: %v", err)
	}
	if projection.Row == nil || projection.Row.CanonicalJobID != "01aaaaaa-0000-7000-8000-000000000001" {
		t.Fatalf("the entry read by its id names Job %+v, want the Job it runs for", projection.Row)
	}
}

// Every row of the legacy queue listing states its times in UTC, whether the row
// comes from a live entry or from a durable Job, so one listing does not mix offsets.
func TestTheLegacyQueueListingStatesItsTimesInUTC(t *testing.T) {
	previous := time.Local
	time.Local = time.FixedZone("UTC+2", 2*60*60)
	t.Cleanup(func() { time.Local = previous })

	ctx := newJobHarnessContext(t, false)
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	if _, err := ctx.downloadManager.SubmitManagedJob(download_queue.ManagedJobOptions{
		JobOptions: download_queue.JobOptions{Source: pluginImportJobSource, InitialPhase: "running"},
	}, func(context.Context, *download_queue.DownloadJob, download_queue.ManagedProgressSink) download_queue.ManagedJobOutcome {
		<-release
		return download_queue.ManagedJobOutcome{Status: download_queue.JobStatusCompleted}
	}); err != nil {
		t.Fatalf("submit the managed entry: %v", err)
	}
	ctx.Config.MaxJobConcurrency = 1
	occupied := ctx.SubmitRemoteDownloads(&query_models.ResourceFromRemoteCreator{URL: "http://127.0.0.1:9/waits.bin"}, nil, "", "api")
	if len(occupied) != 1 || occupied[0].Err != nil {
		t.Fatalf("submit a download: %+v", occupied)
	}

	rows, err := ctx.ProjectDownloadQueue()
	if err != nil {
		t.Fatalf("project the queue: %v", err)
	}
	if len(rows) < 2 {
		t.Fatalf("want the managed entry and the download listed, got %+v", rows)
	}
	for _, row := range rows {
		if row.CreatedAt.Location() != time.UTC {
			t.Errorf("row %s states createdAt in %s, want UTC", row.ID, row.CreatedAt.Location())
		}
		for name, at := range map[string]*time.Time{"startedAt": row.StartedAt, "completedAt": row.CompletedAt} {
			if at != nil && at.Location() != time.UTC {
				t.Errorf("row %s states %s in %s, want UTC", row.ID, name, at.Location())
			}
		}
	}
}
