package api_handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"mahresources/download_queue"
	"mahresources/jobs"
	"mahresources/plugin_system"
)

type jobEventContextStub struct {
	events    []jobs.Event
	err       error
	called    int
	lastAfter uint64
	after     []uint64
	afterSix  chan struct{}
}

func (s *jobEventContextStub) GetJobTimeline(_ string, after uint64, _ int) ([]jobs.Event, error) {
	s.called++
	s.lastAfter = after
	return s.events, s.err
}

func (s *jobEventContextStub) GetPublishedJobEvents(after uint64, _ int) ([]jobs.Event, error) {
	s.called++
	s.lastAfter = after
	s.after = append(s.after, after)
	if after == 6 && s.afterSix != nil {
		select {
		case s.afterSix <- struct{}{}:
		default:
		}
	}
	if s.called > 1 {
		return nil, nil
	}
	return s.events, s.err
}

func TestJobTimelineRejectsOversizedPageBeforeReading(t *testing.T) {
	ctx := &jobEventContextStub{}
	request := mux.SetURLVars(httptest.NewRequest(http.MethodGet, "/v1/jobs/job-123/events?limit=1001", nil), map[string]string{"id": "job-123"})
	recorder := httptest.NewRecorder()

	GetJobTimelineHandler(ctx)(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
	if ctx.called != 0 {
		t.Fatal("timeline was read for an oversized page")
	}
}

func TestCanonicalJobSSERejectsLegacyCursor(t *testing.T) {
	ctx := &jobEventContextStub{}
	request := httptest.NewRequest(http.MethodGet, "/v1/jobs/events?version=2", nil)
	request.Header.Set("Last-Event-ID", "legacy-download-event-4")
	recorder := httptest.NewRecorder()

	GetCanonicalJobEventsHandler(ctx)(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
	if ctx.called != 0 {
		t.Fatal("published events were read for a legacy cursor")
	}
}

func TestCanonicalJobSSECatchesUpWithVersionedDeliveryCursor(t *testing.T) {
	delivery := uint64(6)
	ctx := &jobEventContextStub{afterSix: make(chan struct{}, 1), events: []jobs.Event{{
		ID: "event-row-6", JobID: "job-123", Sequence: 4, JobVersion: 5,
		Type: jobs.EventAccepted, Detail: json.RawMessage(`{"origin":"api"}`),
		DeliverySequence: &delivery, CreatedAt: time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC),
	}}}
	response := newSSETestWriter()
	requestCtx, cancel := context.WithCancel(context.Background())
	request := httptest.NewRequest(http.MethodGet, "/v1/jobs/events?version=2&cursor=v2:5", nil).WithContext(requestCtx)
	finished := make(chan struct{})
	go func() {
		GetCanonicalJobEventsHandler(ctx)(response, request)
		close(finished)
	}()
	select {
	case <-response.eventWritten:
	case <-time.After(2 * time.Second):
		t.Fatal("SSE did not catch up the published event")
	}
	select {
	case <-ctx.afterSix:
	case <-time.After(2 * time.Second):
		t.Fatal("SSE did not advance its durable cursor after sending the event")
	}
	cancel()
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("SSE handler did not stop after the client disconnected")
	}
	body := response.String()
	if !strings.Contains(body, "id: v2:6\nevent: job\n") || !strings.Contains(body, `"jobId":"job-123"`) {
		t.Fatalf("SSE body = %q, want the canonical event and delivery cursor", body)
	}
	if len(ctx.after) < 2 || ctx.after[0] != 5 || ctx.after[1] != 6 {
		t.Fatalf("catch-up cursors = %v, want 5 then 6", ctx.after)
	}
}

type legacyJobEventsContextStub struct {
	manager *download_queue.DownloadManager
}

func (s *legacyJobEventsContextStub) DownloadManager() *download_queue.DownloadManager {
	return s.manager
}
func (s *legacyJobEventsContextStub) ProjectDownloadQueue() ([]*download_queue.DownloadJob, error) {
	return nil, nil
}
func (s *legacyJobEventsContextStub) ProjectDownloadJob(id string) (download_queue.DownloadProjection, error) {
	if s.manager == nil {
		return download_queue.DownloadProjection{}, nil
	}
	job, found := s.manager.GetJob(id)
	if !found {
		return download_queue.DownloadProjection{}, nil
	}
	return download_queue.DownloadProjection{ID: id, Row: job.Snapshot()}, nil
}
func (*legacyJobEventsContextStub) PluginManager() *plugin_system.PluginManager { return nil }

func TestJobsEventsWithoutVersionKeepsLegacyWireFormat(t *testing.T) {
	manager := download_queue.NewDownloadManager(nil, download_queue.TimeoutConfig{})
	t.Cleanup(manager.Shutdown)
	legacy := &legacyJobEventsContextStub{manager: manager}
	canonical := &jobEventContextStub{}
	response := newSSETestWriter()
	requestCtx, cancel := context.WithCancel(context.Background())
	request := httptest.NewRequest(http.MethodGet, "/v1/jobs/events?cursor=legacy-query-cursor", nil).WithContext(requestCtx)
	request.Header.Set("Last-Event-ID", "legacy-event-7")
	finished := make(chan struct{})
	go func() {
		GetJobsEventsHandler(legacy, canonical, false)(response, request)
		close(finished)
	}()
	select {
	case <-response.initWritten:
	case <-time.After(2 * time.Second):
		cancel()
		t.Fatal("legacy stream did not send its initial state")
	}
	job, err := manager.SubmitJob(download_queue.JobSourceGroupExport, "legacy-phase", func(ctx context.Context, _ *download_queue.DownloadJob, _ download_queue.ProgressSink) error {
		<-ctx.Done()
		return ctx.Err()
	})
	if err != nil {
		cancel()
		t.Fatalf("submit legacy SSE fixture: %v", err)
	}
	select {
	case <-response.legacyJobWritten:
		cancel()
	case <-time.After(2 * time.Second):
		cancel()
		t.Fatal("legacy stream did not send its live Job event")
	}
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("legacy stream did not stop after the client disconnected")
	}

	body := response.String()
	legacyInit := "event: init\ndata: {\"actionJobs\":[],\"jobs\":[]}\n\n"
	if !strings.HasPrefix(body, legacyInit) {
		t.Fatalf("legacy init event = %q, want exact legacy bytes %q", body, legacyInit)
	}
	if strings.Contains(body, "id: ") || !strings.Contains(body, "\nevent: added\ndata: {\"type\":\"added\",\"job\":{\"id\":\""+job.ID+"\"") {
		t.Fatalf("unversioned event frame = %q, want legacy added event with job id %q and no SSE cursor", body, job.ID)
	}
	if canonical.called != 0 {
		t.Fatal("unversioned request was sent to the canonical event reader")
	}
}

func TestJobsEventsVersionTwoIsGatedUntilCutover(t *testing.T) {
	legacy := &legacyJobEventsContextStub{}
	canonical := &jobEventContextStub{}
	request := httptest.NewRequest(http.MethodGet, "/v1/jobs/events?version=2", nil)
	recorder := httptest.NewRecorder()

	GetJobsEventsHandler(legacy, canonical, false)(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want gated 404: %s", recorder.Code, recorder.Body.String())
	}
	if canonical.called != 0 {
		t.Fatal("canonical events were read before cutover")
	}
}

type sseTestWriter struct {
	mu               sync.Mutex
	header           http.Header
	status           int
	body             bytes.Buffer
	eventWritten     chan struct{}
	initWritten      chan struct{}
	once             sync.Once
	initOnce         sync.Once
	legacyOnce       sync.Once
	legacyJobWritten chan struct{}
}

func newSSETestWriter() *sseTestWriter {
	return &sseTestWriter{header: make(http.Header), eventWritten: make(chan struct{}), initWritten: make(chan struct{}), legacyJobWritten: make(chan struct{})}
}

func (w *sseTestWriter) Header() http.Header { return w.header }
func (w *sseTestWriter) WriteHeader(status int) {
	w.mu.Lock()
	w.status = status
	w.mu.Unlock()
}
func (w *sseTestWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	n, err := w.body.Write(data)
	if bytes.Contains(data, []byte("event: job")) {
		w.once.Do(func() { close(w.eventWritten) })
	}
	if bytes.Contains(data, []byte("event: init\n")) {
		w.initOnce.Do(func() { close(w.initWritten) })
	}
	if bytes.Contains(data, []byte("event: added\n")) {
		w.legacyOnce.Do(func() { close(w.legacyJobWritten) })
	}
	return n, err
}
func (w *sseTestWriter) Flush() {}
func (w *sseTestWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.body.String()
}
