package api_handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"mahresources/auth"
	"mahresources/download_queue"
	"mahresources/models"
)

// principalJobEventsContext is the legacy stub reading as a fixed principal.
type principalJobEventsContext struct {
	*legacyJobEventsContextStub
	principal *auth.Principal
}

func (ctx *principalJobEventsContext) Principal() *auth.Principal { return ctx.principal }

// changingJobEventsSource answers with whatever the test last set: a context
// for a principal, or a refusal once the credential is revoked.
type changingJobEventsSource struct {
	mu      sync.Mutex
	current JobEventsContext
}

func (s *changingJobEventsSource) CurrentJobEvents() (JobEventsContext, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.current == nil {
		return nil, errors.New("the credential no longer authenticates")
	}
	return s.current, nil
}

func (s *changingJobEventsSource) set(ctx JobEventsContext) {
	s.mu.Lock()
	s.current = ctx
	s.mu.Unlock()
}

func startLegacyEventsHandler(t *testing.T, source JobEventsSource) (*sseTestWriter, <-chan struct{}) {
	t.Helper()
	response := newSSETestWriter()
	requestCtx, cancel := context.WithCancel(context.Background())
	request := httptest.NewRequest(http.MethodGet, "/v1/jobs/events", nil).WithContext(requestCtx)
	finished := make(chan struct{})
	go func() {
		GetDownloadEventsHandler(source)(response, request)
		close(finished)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-finished:
		case <-time.After(2 * time.Second):
			t.Error("the legacy stream did not stop after the client disconnected")
		}
	})
	select {
	case <-response.initWritten:
	case <-time.After(2 * time.Second):
		t.Fatal("the legacy stream did not send its initial state")
	}
	return response, finished
}

func submitOwnedLegacyJob(t *testing.T, manager *download_queue.DownloadManager, owner uint) string {
	t.Helper()
	job, err := manager.SubmitJobWithOptions(download_queue.JobOptions{
		Source: "credential-test", InitialPhase: "waiting", OwnerUserID: &owner,
	}, func(ctx context.Context, _ *download_queue.DownloadJob, _ download_queue.ProgressSink) error {
		<-ctx.Done()
		return ctx.Err()
	})
	if err != nil {
		t.Fatalf("submit a job owned by %d: %v", owner, err)
	}
	return job.ID
}

func waitForBody(response *sseTestWriter, text string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if strings.Contains(response.String(), text) {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return strings.Contains(response.String(), text)
}

func TestALegacyStreamAnswersEachFrameForTheCurrentPrincipal(t *testing.T) {
	manager := download_queue.NewDownloadManager(nil, download_queue.TimeoutConfig{})
	t.Cleanup(manager.Shutdown)
	stub := &legacyJobEventsContextStub{manager: manager}
	const viewer, other = uint(7), uint(8)
	source := &changingJobEventsSource{}
	source.set(&principalJobEventsContext{stub, &auth.Principal{UserID: viewer, Role: models.RoleAdmin}})
	response, _ := startLegacyEventsHandler(t, source)

	seen := submitOwnedLegacyJob(t, manager, other)
	if !waitForBody(response, seen, 2*time.Second) {
		t.Fatalf("an administrator did not receive another user's job %q: %s", seen, response.String())
	}

	// No idle tick separates the demotion from the next event, so only a check
	// made for that frame can keep it back.
	source.set(&principalJobEventsContext{stub, &auth.Principal{UserID: viewer, Role: models.RoleUser}})
	hidden := submitOwnedLegacyJob(t, manager, other)
	own := submitOwnedLegacyJob(t, manager, viewer)
	if !waitForBody(response, own, 2*time.Second) {
		t.Fatalf("the demoted viewer's stream stopped delivering its own job %q: %s", own, response.String())
	}
	if strings.Contains(response.String(), hidden) {
		t.Fatalf("a frame for another user's job %q was written for a principal that may no longer see it: %s",
			hidden, response.String())
	}
}

func TestALegacyStreamEndsAtTheFirstFrameAfterRevocation(t *testing.T) {
	manager := download_queue.NewDownloadManager(nil, download_queue.TimeoutConfig{})
	t.Cleanup(manager.Shutdown)
	stub := &legacyJobEventsContextStub{manager: manager}
	source := &changingJobEventsSource{}
	source.set(&principalJobEventsContext{stub, &auth.Principal{UserID: 7, Role: models.RoleAdmin}})
	response, finished := startLegacyEventsHandler(t, source)

	source.set(nil)
	hidden := submitOwnedLegacyJob(t, manager, 8)
	select {
	case <-finished:
	case <-time.After(3 * time.Second):
		t.Fatal("the legacy stream stayed open after its credential was revoked")
	}
	if strings.Contains(response.String(), hidden) {
		t.Fatalf("the legacy stream wrote job %q after its credential was revoked: %s", hidden, response.String())
	}
}

// With nothing to send, the stream has no frame to check its credential for, so
// it checks on a timer instead of holding a revoked connection open.
func TestAnIdleLegacyStreamEndsOnceItsCredentialIsRevoked(t *testing.T) {
	manager := download_queue.NewDownloadManager(nil, download_queue.TimeoutConfig{})
	t.Cleanup(manager.Shutdown)
	stub := &legacyJobEventsContextStub{manager: manager}
	source := &changingJobEventsSource{}
	source.set(&principalJobEventsContext{stub, &auth.Principal{UserID: 7, Role: models.RoleAdmin}})
	_, finished := startLegacyEventsHandler(t, source)

	source.set(nil)
	select {
	case <-finished:
	case <-time.After(3 * jobEventsRevalidateInterval):
		t.Fatal("an idle legacy stream stayed open after its credential was revoked")
	}
}

func TestALegacyStreamRefusesACredentialThatNoLongerAuthenticatesAtConnect(t *testing.T) {
	source := &changingJobEventsSource{}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/jobs/events", nil)
	GetDownloadEventsHandler(source)(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401: %s", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "event: init") {
		t.Fatalf("a refused stream sent its initial state: %s", recorder.Body.String())
	}
}

// A promotion counts from the next frame too: an event the viewer could not see
// under the last check is not dropped on that stale answer.
func TestALegacyStreamDeliversToAPromotedViewerBeforeTheNextTick(t *testing.T) {
	manager := download_queue.NewDownloadManager(nil, download_queue.TimeoutConfig{})
	t.Cleanup(manager.Shutdown)
	stub := &legacyJobEventsContextStub{manager: manager}
	const viewer, other = uint(7), uint(8)
	source := &changingJobEventsSource{}
	source.set(&principalJobEventsContext{stub, &auth.Principal{UserID: viewer, Role: models.RoleUser}})
	response, _ := startLegacyEventsHandler(t, source)

	source.set(&principalJobEventsContext{stub, &auth.Principal{UserID: viewer, Role: models.RoleAdmin}})
	promoted := submitOwnedLegacyJob(t, manager, other)
	if !waitForBody(response, promoted, 2*time.Second) {
		t.Fatalf("a viewer promoted to administrator did not receive another user's job %q: %s",
			promoted, response.String())
	}
}
