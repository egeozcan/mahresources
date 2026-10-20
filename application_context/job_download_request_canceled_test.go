package application_context

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"mahresources/auth"
	"mahresources/download_queue"
	"mahresources/jobs"
	"mahresources/models"
	"mahresources/models/query_models"
)

// Queue-backed work submitted from a request (the create form's "Download in
// background") outlives it, and the request's context is canceled as soon as
// the handler returns. The goroutines that own the execution kept the
// request's database handle, so each write failed at once with "context
// canceled", was taken for a transient refusal and retried every 250ms for the
// life of the process. On the live host this filled a 456G root disk with GORM
// error lines.

// The owner of a background execution keeps the request's acting user and
// scope but not its cancellation, nor the request itself (whose context
// callerContext would hand to a hook wait).
func TestABackgroundOwnerOutlivesTheRequestThatStartedIt(t *testing.T) {
	ctx := newJobHarnessContext(t, false)
	requestCtx, endRequest := context.WithCancel(auth.WithPrincipal(context.Background(),
		&auth.Principal{UserID: 7, Role: models.RoleAdmin}))
	request := httptest.NewRequest(http.MethodPost, "/v1/resource/remote", nil).WithContext(requestCtx)
	requestScoped := ctx.WithRequest(request).(*MahresourcesContext)

	owner := requestScoped.detachedFromRequest()
	endRequest()

	if err := owner.db.Statement.Context.Err(); err != nil {
		t.Fatalf("the owner's handle ended with the request: %v", err)
	}
	if id, ok := actingUserFromContext(owner.db.Statement.Context); !ok || id != 7 {
		t.Fatalf("the owner's acting user is %d (%v), want the request's 7", id, ok)
	}
	if owner.currentRequest != nil {
		t.Fatal("the owner still carries the request")
	}
	if err := owner.callerContext().Err(); err != nil {
		t.Fatalf("the owner's caller context ended with the request: %v", err)
	}
	var count int64
	if err := owner.db.Model(&models.Job{}).Count(&count).Error; err != nil {
		t.Fatalf("a read through the owner's handle: %v", err)
	}
}

// Whatever handle it was given, a terminal publication whose database context
// is gone stops: it can never succeed, and the next process reconciles the Job.
func TestATerminalPublicationStopsWhenItsContextIsCanceled(t *testing.T) {
	ctx := newJobHarnessContext(t, false)
	attempts := 0
	publish := func(*download_queue.DownloadJob) error {
		attempts++
		return fmt.Errorf("jobs: load visible job: %w", context.Canceled)
	}
	done := make(chan terminalPublication, 1)
	go func() {
		done <- ctx.publishTerminalOutcome(jobs.Execution{JobID: "canceled-publication"}, &download_queue.DownloadJob{}, publish)
	}()
	select {
	case result := <-done:
		if result != publicationUnfinished {
			t.Fatalf("the publication ended %v, want unfinished", result)
		}
		if attempts != 1 {
			t.Fatalf("the publication was offered %d times, want once", attempts)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the publication was still retrying a canceled context after 3s")
	}
}

// ownQueueExecution itself detaches: the publish callback is handed an owner
// whose handle outlives a request that ended before the execution finished.
func TestAQueueExecutionOwnedFromAnEndedRequestPublishesOnALiveHandle(t *testing.T) {
	ctx := newJobHarnessContext(t, false)
	input, err := json.Marshal(downloadJobInput{
		Creator: &query_models.ResourceFromRemoteCreator{URL: "https://example.invalid/owned.bin"},
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
		JobID: accepted.ID, Claimant: "owned-from-request-test",
	})
	if err != nil || !claimed {
		t.Fatalf("claim: claimed=%v err=%v", claimed, err)
	}

	requestCtx, endRequest := context.WithCancel(context.Background())
	request := httptest.NewRequest(http.MethodPost, "/v1/resource/remote", nil).WithContext(requestCtx)
	requestScoped := ctx.WithRequest(request).(*MahresourcesContext)
	endRequest()

	published := make(chan error, 1)
	entry := &download_queue.DownloadJob{ID: "owned-entry", Status: download_queue.JobStatusFailed}
	requestScoped.ownQueueExecution(queueJobAdmission{Accepted: accepted, Execution: execution, Lease: time.Minute}, entry,
		func(owner *MahresourcesContext, _ *download_queue.DownloadJob) error {
			if err := owner.db.Statement.Context.Err(); err != nil {
				published <- err
				return nil
			}
			var job models.Job
			published <- owner.db.Where("id = ?", accepted.ID).Take(&job).Error
			return nil
		})
	select {
	case err := <-published:
		if err != nil {
			t.Fatalf("the publish callback's handle: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the execution never published")
	}
	if !waitForWaitGroup(&ctx.queueFollowers.running, 5*time.Second) {
		t.Fatal("the execution's followers were still running 5s after it published")
	}
}
