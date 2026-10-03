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
// the final flush. The outcome then has to carry the final amount itself, or the
// finished Job reads "4.9 MB of 5.0 MB" for good. So does a failure or a
// cancellation that lands after the body.
func TestAFinishedDownloadKeepsItsFinalAmountOverAStaleProgressWrite(t *testing.T) {
	resourceID := uint(4242)
	for _, outcome := range []struct {
		name  string
		state jobs.State
		// The queue's snapshot at the end, with the whole body received.
		snap func(size int64) *download_queue.DownloadJob
	}{
		{"succeeded", jobs.StateSucceeded, func(size int64) *download_queue.DownloadJob {
			return &download_queue.DownloadJob{Status: download_queue.JobStatusCompleted, Progress: size, TotalSize: size, ResourceID: &resourceID}
		}},
		// Saving the Resource failed after the whole body arrived.
		{"failed", jobs.StateFailed, func(size int64) *download_queue.DownloadJob {
			return &download_queue.DownloadJob{Status: download_queue.JobStatusFailed, Progress: size, TotalSize: size, FailureCode: download_queue.FailureDownloadFailed}
		}},
		{"cancelled", jobs.StateCancelled, func(size int64) *download_queue.DownloadJob {
			return &download_queue.DownloadJob{Status: download_queue.JobStatusCancelled, Progress: size, TotalSize: size}
		}},
	} {
		t.Run(outcome.name, func(t *testing.T) {
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
			if err := sink.DownloadFinished(ref, outcome.snap(size)); err != nil {
				t.Fatal(err)
			}

			finished, err := ctx.GetJob(accepted.ID)
			if err != nil {
				t.Fatal(err)
			}
			if finished.State != outcome.state {
				t.Fatalf("the Job is %s, want %s", finished.State, outcome.state)
			}
			progress := finished.Progress
			if progress.Completed == nil || progress.Total == nil || *progress.Completed != size || *progress.Total != size {
				t.Fatalf("the %s Job holds %v of %v bytes, want %d of %d", outcome.state, deref(progress.Completed), deref(progress.Total), size, size)
			}
		})
	}
}

func deref(value *int64) any {
	if value == nil {
		return nil
	}
	return *value
}
