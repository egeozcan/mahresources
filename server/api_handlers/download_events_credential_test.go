package api_handlers

import (
	"bytes"
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
	"mahresources/jobs"
	"mahresources/models"
	"mahresources/plugin_system"
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
	reads   int
}

func (s *changingJobEventsSource) CurrentJobEvents() (JobEventsContext, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reads++
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

func (s *changingJobEventsSource) readCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reads
}

// A burst of events is checked in batches, not once per event: every open
// stream reads the credential for every event on the queue, whoever it belongs
// to, and a per-event read multiplies by both.
func TestALegacyStreamChecksABurstOfEventsInBatches(t *testing.T) {
	manager := download_queue.NewDownloadManager(nil, download_queue.TimeoutConfig{})
	t.Cleanup(manager.Shutdown)
	stub := &legacyJobEventsContextStub{manager: manager}
	source := &changingJobEventsSource{}
	source.set(&principalJobEventsContext{stub, &auth.Principal{UserID: 7, Role: models.RoleAdmin}})
	response, _ := startLegacyEventsHandler(t, source)

	before := source.readCount()
	const burst = 40
	ids := make([]string, 0, burst)
	for i := 0; i < burst; i++ {
		ids = append(ids, submitOwnedLegacyJob(t, manager, 8))
	}
	for _, id := range ids {
		if !waitForBody(response, id, 3*time.Second) {
			t.Fatalf("job %q from the burst was never delivered: %s", id, response.String())
		}
	}
	if reads := source.readCount() - before; reads > burst/4 {
		t.Fatalf("delivering %d jobs' events read the credential %d times", burst, reads)
	}
}

// stallingSSEWriter blocks the first write that carries a live frame until the
// test releases it, the way a slow client holds a flush.
type stallingSSEWriter struct {
	*sseTestWriter
	stalled  chan struct{}
	release  chan struct{}
	stallOne sync.Once
	// frames are the event prefixes that stall; nil means download frames.
	frames []string
}

func (w *stallingSSEWriter) Write(data []byte) (int, error) {
	frames := w.frames
	if frames == nil {
		frames = []string{"event: added\n", "event: updated\n"}
	}
	live := false
	for _, frame := range frames {
		live = live || bytes.HasPrefix(data, []byte(frame))
	}
	if live {
		w.stallOne.Do(func() {
			close(w.stalled)
			<-w.release
		})
	}
	return w.sseTestWriter.Write(data)
}

// One credential check answers for the frames written soon after it, not for
// frames a stalled client holds back: once the check has aged past its interval,
// the next held event is checked again before it is written.
func TestAHeldBatchIsCheckedAgainAfterAStalledWrite(t *testing.T) {
	manager := download_queue.NewDownloadManager(nil, download_queue.TimeoutConfig{})
	t.Cleanup(manager.Shutdown)
	stub := &legacyJobEventsContextStub{manager: manager}
	source := &changingJobEventsSource{}
	source.set(&principalJobEventsContext{stub, &auth.Principal{UserID: 7, Role: models.RoleAdmin}})

	writer := &stallingSSEWriter{sseTestWriter: newSSETestWriter(), stalled: make(chan struct{}), release: make(chan struct{})}
	requestCtx, cancel := context.WithCancel(context.Background())
	request := httptest.NewRequest(http.MethodGet, "/v1/jobs/events", nil).WithContext(requestCtx)
	finished := make(chan struct{})
	go func() {
		GetDownloadEventsHandler(source)(writer, request)
		close(finished)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-writer.release:
		default:
			close(writer.release)
		}
		<-finished
	})
	select {
	case <-writer.initWritten:
	case <-time.After(2 * time.Second):
		t.Fatal("the legacy stream did not send its initial state")
	}

	// Two events in one burst are held for the same check; the first frame's
	// write stalls.
	first := submitOwnedLegacyJob(t, manager, 8)
	second := submitOwnedLegacyJob(t, manager, 8)
	select {
	case <-writer.stalled:
	case <-time.After(3 * time.Second):
		t.Fatal("no live frame was written")
	}
	source.set(nil)
	time.Sleep(jobEventsCheckInterval + 50*time.Millisecond)
	close(writer.release)

	select {
	case <-finished:
	case <-time.After(3 * time.Second):
		t.Fatal("the stream stayed open after its credential was revoked during a stalled write")
	}
	if strings.Contains(writer.String(), second) {
		t.Fatalf("a held frame for %q was written on a check made before the stall (first was %q): %s",
			second, first, writer.String())
	}
}

// durableActionEventsContext is the legacy stub with a durable action
// projection, so the handler runs its periodic poll.
type durableActionEventsContext struct {
	*principalJobEventsContext
	mu   sync.Mutex
	rows []*plugin_system.ActionJob
}

func (c *durableActionEventsContext) JobService() *jobs.Service { return jobs.NewService() }

func (c *durableActionEventsContext) ProjectActionJobs() ([]*plugin_system.ActionJob, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]*plugin_system.ActionJob(nil), c.rows...), nil
}

// The durable poll's rows were projected for the principal of the check before
// them. When a stalled write ages that check and the next one finds the account
// demoted, the rows left in the poll are filtered for the account as it is now.
func TestAStalledDurablePollFiltersItsRemainingRowsForADemotedViewer(t *testing.T) {
	manager := download_queue.NewDownloadManager(nil, download_queue.TimeoutConfig{})
	t.Cleanup(manager.Shutdown)
	stub := &legacyJobEventsContextStub{manager: manager}
	other := uint(8)
	row := func(handle string) *plugin_system.ActionJob {
		return plugin_system.ProjectedActionJob{
			Handle: handle, CanonicalJobID: "job-" + handle, Plugin: "p", ActionID: "a",
			Status: "running", Owner: &other,
		}.ActionJob()
	}
	admin := &durableActionEventsContext{principalJobEventsContext: &principalJobEventsContext{stub, &auth.Principal{UserID: 7, Role: models.RoleAdmin}}}
	demoted := &durableActionEventsContext{principalJobEventsContext: &principalJobEventsContext{stub, &auth.Principal{UserID: 7, Role: models.RoleUser}}}
	source := &changingJobEventsSource{}
	source.set(admin)

	writer := &stallingSSEWriter{sseTestWriter: newSSETestWriter(), stalled: make(chan struct{}), release: make(chan struct{}),
		frames: []string{"event: action_added\n"}}
	requestCtx, cancel := context.WithCancel(context.Background())
	request := httptest.NewRequest(http.MethodGet, "/v1/jobs/events", nil).WithContext(requestCtx)
	finished := make(chan struct{})
	go func() {
		GetDownloadEventsHandler(source)(writer, request)
		close(finished)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-writer.release:
		default:
			close(writer.release)
		}
		<-finished
	})
	select {
	case <-writer.initWritten:
	case <-time.After(2 * time.Second):
		t.Fatal("the legacy stream did not send its initial state")
	}

	// Both rows appear in one poll, projected for the administrator.
	admin.mu.Lock()
	admin.rows = []*plugin_system.ActionJob{row("first"), row("second")}
	admin.mu.Unlock()
	select {
	case <-writer.stalled:
	case <-time.After(5 * time.Second):
		t.Fatal("the durable poll wrote no row")
	}
	source.set(demoted)
	time.Sleep(jobEventsCheckInterval + 50*time.Millisecond)
	close(writer.release)

	// Long enough for the stalled poll to finish its loop.
	time.Sleep(500 * time.Millisecond)
	if strings.Contains(writer.String(), `"id":"second"`) {
		t.Fatalf("a row projected for the administrator was written after the account was demoted: %s", writer.String())
	}
}

// blockingProjectionContext holds the first download projection until the
// test releases it, the way a projection waits on a slow database.
type blockingProjectionContext struct {
	*principalJobEventsContext
	blocked  chan struct{}
	release  chan struct{}
	blockOne *sync.Once
}

func (c *blockingProjectionContext) ProjectDownloadJob(id string) (download_queue.DownloadProjection, error) {
	c.blockOne.Do(func() {
		close(c.blocked)
		<-c.release
	})
	return c.principalJobEventsContext.ProjectDownloadJob(id)
}

// A projection is a read made for one check. When it outlasts that check and
// the account has changed by the time it returns, its row is not written on the
// old answer.
func TestALiveFrameProjectedBeforeADemotionIsNotWrittenAfterIt(t *testing.T) {
	manager := download_queue.NewDownloadManager(nil, download_queue.TimeoutConfig{})
	t.Cleanup(manager.Shutdown)
	stub := &legacyJobEventsContextStub{manager: manager}
	once := &sync.Once{}
	blocked, release := make(chan struct{}), make(chan struct{})
	admin := &blockingProjectionContext{&principalJobEventsContext{stub, &auth.Principal{UserID: 7, Role: models.RoleAdmin}}, blocked, release, once}
	demoted := &blockingProjectionContext{&principalJobEventsContext{stub, &auth.Principal{UserID: 7, Role: models.RoleUser}}, blocked, release, once}
	source := &changingJobEventsSource{}
	source.set(admin)
	response, _ := startLegacyEventsHandler(t, source)
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})

	hidden := submitOwnedLegacyJob(t, manager, 8)
	select {
	case <-blocked:
	case <-time.After(3 * time.Second):
		t.Fatal("the live event was never projected")
	}
	source.set(demoted)
	time.Sleep(jobEventsCheckInterval + 50*time.Millisecond)
	close(release)

	own := submitOwnedLegacyJob(t, manager, 7)
	if !waitForBody(response, own, 3*time.Second) {
		t.Fatalf("the demoted viewer's stream stopped delivering its own job %q: %s", own, response.String())
	}
	if strings.Contains(response.String(), hidden) {
		t.Fatalf("a row projected for the administrator was written after the account was demoted: %s", response.String())
	}
}
