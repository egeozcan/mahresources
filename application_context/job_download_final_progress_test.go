package application_context

import (
	"context"
	"encoding/json"
	"testing"

	"mahresources/download_queue"
	"mahresources/jobs"
	"mahresources/models/query_models"
)

// A download's Job has two progress writers: the transfer's own mirror, which
// flushes the final amount at the end of the body, and the queue follower, which
// copies the entry's snapshot on its own tick. Neither write is ordered against
// the other, so a follower snapshot taken a chunk before the end can commit after
// the final flush. The success then has to carry the final amount itself, or the
// finished Job reads "4.9 MB of 5.0 MB" for good.
func TestASucceededDownloadKeepsItsFinalAmountOverAStaleProgressWrite(t *testing.T) {
	ctx := newJobHarnessContext(t, false)
	input, err := json.Marshal(downloadJobInput{
		Creator: &query_models.ResourceFromRemoteCreator{URL: "https://example.invalid/final-amount.bin"},
	})
	if err != nil {
		t.Fatal(err)
	}
	accepted := acceptJobFor(t, ctx, jobs.Acceptance{
		Kind: JobKindRemoteDownload, KindVersion: jobDownloadKindVersion,
		State: jobs.StateQueued, Origin: "api",
		Replay: jobs.ReplayInput{Input: input},
	})
	execution, claimed, err := ctx.JobService().Claim(context.Background(), ctx.jobDeps(), jobs.ClaimRequest{
		Kind: JobKindRemoteDownload, KindVersion: jobDownloadKindVersion,
		JobID: accepted.ID, Claimant: "final-amount-test",
	})
	if err != nil || !claimed {
		t.Fatalf("claim: claimed=%v err=%v", claimed, err)
	}
	ref := download_queue.CanonicalRef{JobID: execution.JobID, ExecutionToken: execution.ExecutionToken}
	sink := &jobDownloadSink{ctx: ctx}
	const size = int64(5 << 20)

	// The transfer's flush at the end of the body.
	if err := sink.DownloadProgress(ref, &download_queue.DownloadJob{
		Status: download_queue.JobStatusDownloading, Progress: size, TotalSize: size,
	}); err != nil {
		t.Fatal(err)
	}
	// The follower's snapshot from a chunk earlier, committing after it.
	stale, total := size-64<<10, size
	if _, err := ctx.JobService().UpdateProgress(ctx.jobDeps(), executionRefOf(ref), jobs.Progress{
		Completed: &stale, Total: &total, Unit: "bytes",
	}); err != nil {
		t.Fatal(err)
	}
	resourceID := uint(4242)
	if err := sink.DownloadFinished(ref, &download_queue.DownloadJob{
		Status: download_queue.JobStatusCompleted, Progress: size, TotalSize: size, ResourceID: &resourceID,
	}); err != nil {
		t.Fatal(err)
	}

	finished, err := ctx.GetJob(accepted.ID)
	if err != nil {
		t.Fatal(err)
	}
	if finished.State != jobs.StateSucceeded {
		t.Fatalf("the Job is %s, want succeeded", finished.State)
	}
	progress := finished.Progress
	if progress.Completed == nil || progress.Total == nil || *progress.Completed != size || *progress.Total != size {
		t.Fatalf("the succeeded Job holds %v of %v bytes, want %d of %d", deref(progress.Completed), deref(progress.Total), size, size)
	}
}

func deref(value *int64) any {
	if value == nil {
		return nil
	}
	return *value
}
