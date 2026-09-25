package api_handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
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
	pages     [][]jobs.Event
	// progress is the live-progress read's answer per call, and progressSince
	// the watermark each call was made with.
	progress         [][]jobs.Snapshot
	progressErr      error
	progressSince    []time.Time
	progressSinceIDs []string
	progressMu       sync.Mutex
}

func (s *jobEventContextStub) GetLiveJobProgress(since time.Time, sinceID string, _ int) ([]jobs.Snapshot, error) {
	s.progressMu.Lock()
	defer s.progressMu.Unlock()
	call := len(s.progressSince)
	s.progressSince = append(s.progressSince, since)
	s.progressSinceIDs = append(s.progressSinceIDs, sinceID)
	if call < len(s.progress) {
		return s.progress[call], s.progressErr
	}
	return nil, s.progressErr
}

func (s *jobEventContextStub) progressCalls() []time.Time {
	s.progressMu.Lock()
	defer s.progressMu.Unlock()
	return append([]time.Time(nil), s.progressSince...)
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
	if len(s.pages) > 0 {
		pageIndex := s.called - 1
		if pageIndex < len(s.pages) {
			return s.pages[pageIndex], s.err
		}
		return nil, s.err
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

func TestCanonicalJobSSEReconnectPrefersNewerLastEventID(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/v1/jobs/events?version=2&cursor=v2:5", nil)
	request.Header.Set("Last-Event-ID", "v2:9")

	cursor, err := canonicalJobEventCursor(request)
	if err != nil || cursor != 9 {
		t.Fatalf("reconnect cursor = %d, err=%v; want Last-Event-ID v2:9 to supersede stale URL cursor v2:5", cursor, err)
	}
}

func TestCanonicalJobSSEReconnectStillValidatesBothCursorFormats(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/v1/jobs/events?version=2&cursor=legacy:5", nil)
	request.Header.Set("Last-Event-ID", "v2:9")

	if _, err := canonicalJobEventCursor(request); err == nil {
		t.Fatal("reconnect accepted a malformed URL cursor because Last-Event-ID was valid")
	}
}

func TestCanonicalJobSSECatchesUpWithVersionedDeliveryCursor(t *testing.T) {
	delivery := uint64(6)
	ctx := &jobEventContextStub{events: []jobs.Event{{
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
	case <-response.caughtUpWritten:
	case <-time.After(2 * time.Second):
		t.Fatal("SSE did not announce that durable catch-up finished")
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
	if !strings.HasSuffix(body, "event: job-caught-up\ndata: {\"cursor\":\"v2:6\"}\n\n") || strings.Count(body, "id: ") != 1 {
		t.Fatalf("SSE body = %q, want a non-durable caught-up marker after events without its own id", body)
	}
	if len(ctx.after) < 1 || ctx.after[0] != 5 {
		t.Fatalf("initial catch-up cursor = %v, want 5", ctx.after)
	}
}

func TestCanonicalJobSSEWaitsForAllCatchUpPagesBeforeControlMarker(t *testing.T) {
	pageSize := jobs.DefaultEventPageSize
	firstPage := make([]jobs.Event, 0, pageSize)
	for i := 0; i < pageSize; i++ {
		sequence := uint64(i + 1)
		firstPage = append(firstPage, jobs.Event{
			ID: "event-row-" + strconv.FormatUint(sequence, 10), JobID: "job-123",
			Sequence: sequence, Type: jobs.EventQueued, DeliverySequence: &sequence,
		})
	}
	last := uint64(pageSize + 1)
	secondPage := []jobs.Event{{
		ID: "event-row-last", JobID: "job-123", Sequence: last,
		Type: jobs.EventSucceeded, DeliverySequence: &last,
	}}
	ctx := &jobEventContextStub{pages: [][]jobs.Event{firstPage, secondPage}}
	response := newSSETestWriter()
	requestCtx, cancel := context.WithCancel(context.Background())
	request := httptest.NewRequest(http.MethodGet, "/v1/jobs/events?version=2&cursor=v2:0", nil).WithContext(requestCtx)
	finished := make(chan struct{})
	go func() {
		GetCanonicalJobEventsHandler(ctx)(response, request)
		close(finished)
	}()
	select {
	case <-response.caughtUpWritten:
	case <-time.After(2 * time.Second):
		cancel()
		t.Fatal("SSE did not announce completion after draining all catch-up pages")
	}
	cancel()
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("SSE handler did not stop after the client disconnected")
	}

	body := response.String()
	if len(ctx.after) != 2 || ctx.after[0] != 0 || ctx.after[1] != uint64(pageSize) {
		t.Fatalf("catch-up cursors = %v, want 0 then %d", ctx.after, pageSize)
	}
	if strings.Count(body, "event: job\ndata:") != pageSize+1 {
		t.Fatalf("SSE emitted %d durable events, want %d", strings.Count(body, "event: job\ndata:"), pageSize+1)
	}
	if !strings.HasSuffix(body, "event: job-caught-up\ndata: {\"cursor\":\"v2:"+strconv.FormatUint(last, 10)+"\"}\n\n") || strings.Count(body, "id: ") != pageSize+1 {
		tail := body
		if len(tail) > 120 {
			tail = tail[len(tail)-120:]
		}
		t.Fatalf("SSE did not mark the final replay cursor without a control-event id: suffix %q", tail)
	}
}

type legacyJobEventsContextStub struct {
	manager *download_queue.DownloadManager
	rows    []*download_queue.DownloadJob
}

func (s *legacyJobEventsContextStub) DownloadManager() *download_queue.DownloadManager {
	return s.manager
}
func (s *legacyJobEventsContextStub) ProjectDownloadQueue() ([]*download_queue.DownloadJob, error) {
	return s.rows, nil
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

func TestLegacyJobEventsKeepHandleAndAddCanonicalIdentity(t *testing.T) {
	manager := download_queue.NewDownloadManager(nil, download_queue.TimeoutConfig{})
	t.Cleanup(manager.Shutdown)
	legacy := &legacyJobEventsContextStub{manager: manager, rows: []*download_queue.DownloadJob{{
		ID: "legacy-handle-1", CanonicalJobID: "job-uuid-1", Status: download_queue.JobStatusPending,
	}}}
	canonical := &jobEventContextStub{}
	response := newSSETestWriter()
	requestCtx, cancel := context.WithCancel(context.Background())
	request := httptest.NewRequest(http.MethodGet, "/v1/jobs/events", nil).WithContext(requestCtx)
	finished := make(chan struct{})
	go func() {
		GetJobsEventsHandler(legacy, canonical, false)(response, request)
		close(finished)
	}()
	select {
	case <-response.initWritten:
		cancel()
	case <-time.After(2 * time.Second):
		cancel()
		t.Fatal("legacy stream did not send its initial state")
	}
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("legacy stream did not stop after the client disconnected")
	}

	body := response.String()
	if !strings.Contains(body, `"id":"legacy-handle-1"`) || !strings.Contains(body, `"canonicalJobId":"job-uuid-1"`) {
		t.Fatalf("legacy stream did not preserve the handle and add its canonical identity: %q", body)
	}
	if strings.Contains(body, "id: ") || canonical.called != 0 {
		t.Fatalf("unversioned stream acquired canonical cursor behavior: %q (canonical reads %d)", body, canonical.called)
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
	caughtUpWritten  chan struct{}
	initWritten      chan struct{}
	once             sync.Once
	caughtUpOnce     sync.Once
	initOnce         sync.Once
	legacyOnce       sync.Once
	legacyJobWritten chan struct{}
}

func newSSETestWriter() *sseTestWriter {
	return &sseTestWriter{header: make(http.Header), eventWritten: make(chan struct{}), caughtUpWritten: make(chan struct{}), initWritten: make(chan struct{}), legacyJobWritten: make(chan struct{})}
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
	if bytes.Contains(data, []byte("event: job\ndata:")) {
		w.once.Do(func() { close(w.eventWritten) })
	}
	if bytes.Contains(data, []byte("event: job-caught-up\n")) {
		w.caughtUpOnce.Do(func() { close(w.caughtUpWritten) })
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

func TestCanonicalJobSSESendsLiveProgressWithoutACursor(t *testing.T) {
	delivery := uint64(6)
	updated := time.Now().Add(time.Hour).UTC()
	completed, total := int64(400), int64(1000)
	rate := 100.0
	ctx := &jobEventContextStub{
		events: []jobs.Event{{
			ID: "event-row-6", JobID: "job-123", Sequence: 4, JobVersion: 5,
			Type: jobs.EventStarted, DeliverySequence: &delivery, CreatedAt: time.Now().UTC(),
		}},
		progress: [][]jobs.Snapshot{{{
			ID: "job-123", Version: 5, State: jobs.StateRunning,
			Progress: jobs.Progress{
				Completed: &completed, Total: &total, Unit: "bytes",
				Metrics: []jobs.Metric{{Key: "segments", Label: "Segments", Value: 4, Graph: true}},
			},
			ProgressSeries: jobs.ProgressSeries{
				IntervalMs: 1000, Unit: "bytes", Rate: &rate,
				Anchor: &jobs.RateAnchor{At: time.Now().UnixMilli(), Completed: 400},
				Points: []jobs.SeriesPoint{{At: 1, Completed: new(float64)}, {At: 1001, Rate: &rate, Values: map[string]float64{"segments": 4}}},
			},
			ProgressUpdatedAt: &updated,
		}}},
	}
	response := newSSETestWriter()
	requestCtx, cancel := context.WithCancel(context.Background())
	request := httptest.NewRequest(http.MethodGet, "/v1/jobs/events?version=2&cursor=v2:5", nil).WithContext(requestCtx)
	finished := make(chan struct{})
	go func() {
		GetCanonicalJobEventsHandler(ctx)(response, request)
		close(finished)
	}()
	deadline := time.Now().Add(3 * time.Second)
	for len(ctx.progressCalls()) < 2 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	<-finished

	body := response.String()
	caughtUp := strings.Index(body, "event: job-caught-up\n")
	frameAt := strings.Index(body, "event: job-progress\ndata: ")
	if caughtUp < 0 || frameAt < 0 || frameAt < caughtUp {
		t.Fatalf("SSE body = %q; want a job-progress frame after the caught-up marker", body)
	}
	if strings.Count(body, "id: ") != 1 {
		t.Fatalf("SSE body = %q; a live progress frame must carry no delivery id", body)
	}
	line := body[frameAt+len("event: job-progress\ndata: "):]
	line = line[:strings.Index(line, "\n")]
	var frame JobProgressFrame
	if err := json.Unmarshal([]byte(line), &frame); err != nil {
		t.Fatalf("frame %q: %v", line, err)
	}
	if frame.JobID != "job-123" || frame.Progress.Completed == nil || *frame.Progress.Completed != 400 ||
		frame.Progress.Rate == nil || *frame.Progress.Rate != 100 || !frame.Progress.ETAEstimated || frame.Progress.ETA == nil {
		t.Fatalf("frame = %+v; want the Job's progress with its live rate and an estimated ETA", frame)
	}
	if len(frame.Progress.Metrics) != 1 || frame.Progress.Metrics[0].Key != "segments" {
		t.Fatalf("frame metrics = %+v", frame.Progress.Metrics)
	}
	if frame.Point == nil || frame.Point.T != 1001 || frame.Point.V["segments"] != 4 || frame.Progress.Series != nil {
		t.Fatalf("frame point = %+v, series = %+v; want only the latest point", frame.Point, frame.Progress.Series)
	}
	calls := ctx.progressCalls()
	if len(calls) < 2 || !calls[1].Equal(updated) {
		t.Fatalf("live progress watermarks = %v; want the second read to start from the delivered row's %v", calls, updated)
	}
	ctx.progressMu.Lock()
	ids := append([]string(nil), ctx.progressSinceIDs...)
	ctx.progressMu.Unlock()
	if ids[0] != "" || ids[1] != "job-123" {
		t.Fatalf("live progress watermark ids = %v; want the delivered row's id to resume after", ids)
	}
}
