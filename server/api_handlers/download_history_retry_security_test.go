package api_handlers

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"mahresources/download_queue"
	"mahresources/jobs"
	"mahresources/models"
	"mahresources/models/query_models"
)

type hiddenHandleRetryContext struct {
	DownloadHistoryContext
	manager *download_queue.DownloadManager
}

func (ctx hiddenHandleRetryContext) DownloadManager() *download_queue.DownloadManager {
	return ctx.manager
}

func (hiddenHandleRetryContext) ProjectDownloadJobForRetry(string) (download_queue.DownloadProjection, bool, error) {
	return download_queue.DownloadProjection{}, true, jobs.ErrNotFound
}

func (hiddenHandleRetryContext) DownloadRestartPayload(string) (*query_models.ResourceFromRemoteCreator, error) {
	return nil, errors.New("unexpected restart payload lookup")
}

func (hiddenHandleRetryContext) ExecuteJobCommand(context.Context, jobs.CommandRequest) (jobs.CommandResult, error) {
	return jobs.CommandResult{}, errors.New("unexpected command execution")
}

func (hiddenHandleRetryContext) ReplayJobCommand(context.Context, jobs.CommandRequest) (jobs.CommandResult, bool, error) {
	return jobs.CommandResult{}, false, errors.New("unexpected command replay")
}

func TestRetryDoesNotFallbackWhenAHiddenCanonicalHandleExists(t *testing.T) {
	manager := download_queue.NewDownloadManager(nil, download_queue.TimeoutConfig{})
	t.Cleanup(manager.Shutdown)
	var attempts atomic.Int32
	secondAttempt := make(chan struct{})
	job, err := manager.SubmitJob(download_queue.JobSourceDownload, "", func(context.Context, *download_queue.DownloadJob, download_queue.ProgressSink) error {
		if attempts.Add(1) == 2 {
			close(secondAttempt)
		}
		return errors.New("failed legacy attempt")
	})
	if err != nil {
		t.Fatalf("submit fixture job: %v", err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for job.GetStatus() != download_queue.JobStatusFailed && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if job.GetStatus() != download_queue.JobStatusFailed {
		t.Fatalf("fixture job reached %s, want failed", job.GetStatus())
	}

	ctx := hiddenHandleRetryContext{manager: manager}
	entry := &models.DownloadHistoryEntry{JobID: job.ID, Status: string(download_queue.JobStatusFailed)}
	_, _, err = retryOrResubmit(ctx, entry, nil, nil, nil, "")
	if !errors.Is(err, jobs.ErrNotFound) {
		t.Fatalf("retry through a hidden current handle = %v, want not found", err)
	}
	select {
	case <-secondAttempt:
		t.Fatal("retry fell back to the old queue entry after the canonical handle was hidden")
	case <-time.After(50 * time.Millisecond):
	}
}

type movedHandleRetryContext struct {
	hiddenHandleRetryContext
}

func (movedHandleRetryContext) ProjectDownloadJobForRetry(string) (download_queue.DownloadProjection, bool, error) {
	return download_queue.DownloadProjection{}, true,
		fmt.Errorf("%w: %s", jobs.ErrNotFound, "01a0ddc1-851e-7dfc-b004-7ab8461fbead")
}

// An administrator's Retry moves the row's handle onto a Job the row's owner
// cannot see. The owner's retry is refused, and the refusal must neither name
// that Job nor read as an internal error. Retention deleting the Job the handle
// names looks the same from here, so the refusal claims neither cause.
func TestRetryThroughAHandleAnotherAccountMovedNamesNoJob(t *testing.T) {
	manager := download_queue.NewDownloadManager(nil, download_queue.TimeoutConfig{})
	t.Cleanup(manager.Shutdown)

	entry := &models.DownloadHistoryEntry{JobID: "legacy-handle", Status: string(download_queue.JobStatusFailed)}
	_, _, err := retryOrResubmit(movedHandleRetryContext{hiddenHandleRetryContext{manager: manager}}, entry, nil, nil, nil, "")
	if !errors.Is(err, jobs.ErrNotFound) {
		t.Fatalf("retry through a moved handle = %v, want not found", err)
	}
	if strings.Contains(err.Error(), "01a0ddc1") || strings.Contains(err.Error(), "jobs:") {
		t.Fatalf("the refusal %q names the hidden job or reads as an internal error", err.Error())
	}
	if strings.Contains(err.Error(), "another account") || !strings.Contains(err.Error(), "no longer available to you") {
		t.Fatalf("the refusal %q claims a cause, or does not say the job is no longer available", err.Error())
	}
}
