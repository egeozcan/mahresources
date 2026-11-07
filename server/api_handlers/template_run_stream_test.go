package api_handlers

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"mahresources/application_context"
)

// streamWriter is a ResponseWriter that honours write deadlines the way a
// network connection does: while stalled, a Write blocks until the deadline set
// through http.ResponseController passes, then fails.
type streamWriter struct {
	mu       sync.Mutex
	header   http.Header
	body     strings.Builder
	status   int
	deadline time.Time
	stalled  atomic.Bool
	release  chan struct{}
	writes   atomic.Int32
}

func newStreamWriter() *streamWriter {
	return &streamWriter{header: http.Header{}, release: make(chan struct{})}
}

func (w *streamWriter) Header() http.Header { return w.header }
func (w *streamWriter) WriteHeader(code int) {
	w.mu.Lock()
	w.status = code
	w.mu.Unlock()
}
func (w *streamWriter) Flush() {}
func (w *streamWriter) SetWriteDeadline(t time.Time) error {
	w.mu.Lock()
	w.deadline = t
	w.mu.Unlock()
	return nil
}

func (w *streamWriter) Write(p []byte) (int, error) {
	w.writes.Add(1)
	if w.stalled.Load() {
		w.mu.Lock()
		d := w.deadline
		w.mu.Unlock()
		wait := time.Until(d)
		if d.IsZero() {
			wait = time.Hour
		}
		select {
		case <-w.release:
		case <-time.After(wait):
			return 0, os.ErrDeadlineExceeded
		}
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.body.Write(p)
	return len(p), nil
}

func (w *streamWriter) text() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.body.String()
}

type streamFrameT struct{ event, data string }

// parseStream splits a stream into events and comments, failing on any frame
// that is not one event line, one data line holding JSON, and a blank line.
func parseStream(t *testing.T, body string) (events []streamFrameT, comments int) {
	t.Helper()
	sc := bufio.NewScanner(strings.NewReader(body))
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	var cur streamFrameT
	for sc.Scan() {
		line := sc.Text()
		switch {
		case line == "":
			if cur.event != "" {
				events = append(events, cur)
			}
			cur = streamFrameT{}
		case line == ": heartbeat":
			comments++
		case strings.HasPrefix(line, "event: "):
			if cur.event != "" {
				t.Fatalf("a second event line inside one frame: %q", line)
			}
			cur.event = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			if cur.data != "" {
				t.Fatalf("a frame carries two data lines")
			}
			cur.data = strings.TrimPrefix(line, "data: ")
			var v any
			if err := json.Unmarshal([]byte(cur.data), &v); err != nil {
				t.Fatalf("corrupt frame data %q: %v", cur.data, err)
			}
		default:
			t.Fatalf("stray line in the stream: %q", line)
		}
	}
	if cur.event != "" {
		t.Fatalf("an unterminated frame at the end: %+v", cur)
	}
	return events, comments
}

func newTestStream(w http.ResponseWriter, cfg templateRunStreamConfig) (*templateRunStream, context.Context, *atomic.Bool) {
	ctx, cancel := context.WithCancel(context.Background())
	var cancelled atomic.Bool
	s := newTemplateRunStream(ctx, func() { cancelled.Store(true); cancel() }, w, templateRunRunEvent{RunID: "r1", MaxTurns: 3}, cfg)
	return s, ctx, &cancelled
}

// Simultaneous Tool completions and a heartbeat interleave without corrupting a
// frame (run with -race).
func TestStreamInterleavesToolEventsAndHeartbeats(t *testing.T) {
	w := newStreamWriter()
	s, _, _ := newTestStream(w, templateRunStreamConfig{Heartbeat: 2 * time.Millisecond})
	defer s.Close()
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 20; i++ {
				s.ToolFinished(application_context.TemplateToolEvent{Turn: 1, Name: "get_entity", Args: map[string]string{"type": "group", "id": "12"}, Bytes: 100 + i})
				time.Sleep(time.Millisecond)
			}
		}(g)
	}
	s.TurnStarted(application_context.TemplateTurnEvent{Turn: 1, MaxTurns: 3})
	wg.Wait()
	if !s.Drain() {
		t.Fatal("drain failed")
	}
	if err := s.Deliver(templateRunFrame("result", map[string]any{"content": "x"})); err != nil {
		t.Fatal(err)
	}
	events, comments := parseStream(t, w.text())
	if events[0].event != "run" || events[len(events)-1].event != "result" {
		t.Fatalf("a stream starts with run and ends with the result: first %q last %q", events[0].event, events[len(events)-1].event)
	}
	tools := 0
	for _, e := range events {
		if e.event == "tool" {
			tools++
		}
	}
	if tools == 0 || comments == 0 {
		t.Fatalf("expected tool events and heartbeats in one stream, got %d and %d", tools, comments)
	}
	if w.header.Get("Content-Type") != "text/event-stream; charset=utf-8" || w.status != http.StatusOK {
		t.Fatalf("headers: %v status %d", w.header, w.status)
	}
}

// A client that stops reading makes the writer time out within the write
// deadline, which cancels the run; nothing blocks after that.
func TestStreamSlowClientTimesOutAndCancelsTheRun(t *testing.T) {
	w := newStreamWriter()
	s, ctx, cancelled := newTestStream(w, templateRunStreamConfig{WriteTimeout: 60 * time.Millisecond, Heartbeat: time.Hour})
	defer s.Close()
	s.TurnStarted(application_context.TemplateTurnEvent{Turn: 1, MaxTurns: 3}) // starts the writer; the run frame goes out
	waitUntil(t, "the run frame", func() bool { return w.writes.Load() >= 1 })
	w.stalled.Store(true)
	start := time.Now()
	s.TurnStarted(application_context.TemplateTurnEvent{Turn: 2, MaxTurns: 3})
	select {
	case <-ctx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("the run was not cancelled by a stalled client")
	}
	if took := time.Since(start); took > time.Second {
		t.Fatalf("the writer took %s to time out a 60ms deadline", took)
	}
	if !cancelled.Load() {
		t.Fatal("cancel was not called")
	}
	if s.Drain() {
		t.Fatal("a failed stream must report that nothing more can be delivered")
	}
	if err := s.Deliver(templateRunFrame("result", nil)); err == nil {
		t.Fatal("a failed stream delivered a terminal frame")
	}
}

// Progress is droppable: a full queue never blocks a producer, and the terminal
// frame still arrives, with the failure's transcript, after what was queued.
func TestStreamDropsProgressButDeliversTheTerminalFrame(t *testing.T) {
	w := newStreamWriter()
	s, _, _ := newTestStream(w, templateRunStreamConfig{Heartbeat: time.Hour})
	defer s.Close()
	s.TurnStarted(application_context.TemplateTurnEvent{Turn: 1, MaxTurns: 3})
	waitUntil(t, "the run frame", func() bool { return w.writes.Load() >= 1 })
	w.stalled.Store(true)

	produced := make(chan struct{})
	go func() {
		defer close(produced)
		for i := 0; i < 500; i++ {
			s.ToolFinished(application_context.TemplateToolEvent{Turn: 1, Name: "lint", Bytes: i})
		}
	}()
	select {
	case <-produced:
	case <-time.After(2 * time.Second):
		t.Fatal("a producer blocked on a full queue")
	}
	close(w.release) // the client reads again
	if !s.Drain() {
		t.Fatal("drain failed")
	}
	transcript := []application_context.TemplateTranscriptRecord{{Turn: 1, Kind: application_context.TemplateTranscriptSeed, Text: "seed", Sent: true}}
	err := &application_context.TemplateRunError{Err: application_context.ErrTemplateGenerationProvider, StopReason: application_context.TemplateStopProviderError, Transcript: transcript}
	if err := s.Deliver(terminalFrame(nil, err)); err != nil {
		t.Fatal(err)
	}
	events, _ := parseStream(t, w.text())
	tools := 0
	for _, e := range events {
		if e.event == "tool" {
			tools++
		}
	}
	if tools >= 500 || tools == 0 {
		t.Fatalf("want some but not all of 500 progress events kept, got %d", tools)
	}
	last := events[len(events)-1]
	if last.event != "error" {
		t.Fatalf("the terminal frame must come last, got %q", last.event)
	}
	var payload struct {
		Status     int                                            `json:"status"`
		StopReason string                                         `json:"stopReason"`
		Transcript []application_context.TemplateTranscriptRecord `json:"transcript"`
	}
	if err := json.Unmarshal([]byte(last.data), &payload); err != nil || payload.Status != http.StatusBadGateway || payload.StopReason != "provider_error" || len(payload.Transcript) != 1 {
		t.Fatalf("bad error payload %s (%v)", last.data, err)
	}
}

// An access change carries no transcript, whatever the error held.
func TestTerminalFrameDropsTheTranscriptOnAccessChange(t *testing.T) {
	err := &application_context.TemplateRunError{
		Err:        application_context.ErrTemplateRunAccessChanged,
		StopReason: application_context.TemplateStopDone,
		Transcript: []application_context.TemplateTranscriptRecord{{Turn: 1, Kind: "seed", Text: "private"}},
	}
	frame := string(terminalFrame(nil, err))
	if strings.Contains(frame, "private") || strings.Contains(frame, "transcript") || !strings.Contains(frame, `"status":403`) {
		t.Fatalf("an access change must carry nothing the run read: %s", frame)
	}
}

// A stream that never got a progress event commits nothing, so the handler can
// still answer with an ordinary status code.
func TestStreamThatNeverStartedWritesNothing(t *testing.T) {
	w := newStreamWriter()
	s, _, _ := newTestStream(w, templateRunStreamConfig{})
	if s.Started() || s.Drain() {
		t.Fatal("a stream with no event must not report started")
	}
	s.Close()
	if w.writes.Load() != 0 || w.status != 0 || len(w.header) != 0 {
		t.Fatalf("an unstarted stream touched the response: writes=%d status=%d header=%v", w.writes.Load(), w.status, w.header)
	}
}

// The stream re-validates progress payloads: free text from a sink caller never
// reaches a frame.
func TestProgressPayloadsAreEnumeratedValuesOnly(t *testing.T) {
	w := newStreamWriter()
	s, _, _ := newTestStream(w, templateRunStreamConfig{Heartbeat: time.Hour})
	defer s.Close()
	const secret = "free-text-secret"
	s.TurnStarted(application_context.TemplateTurnEvent{Turn: 1, MaxTurns: 3, FinalReason: secret})
	s.ToolFinished(application_context.TemplateToolEvent{Turn: 1, Name: secret, Args: map[string]string{
		"field": secret, "type": secret, "id": "12", "offset": "-3", "query": secret, "content": secret, "direction": "up", "carrierType": "noteType",
	}})
	s.Submitted(application_context.TemplateSubmissionEvent{Turn: 1, Field: secret, Errors: 1})
	s.Submitted(application_context.TemplateSubmissionEvent{Turn: 1, Field: "CustomHeaderCSS"})
	if !s.Drain() {
		t.Fatal("drain failed")
	}
	_ = s.Deliver(templateRunFrame("result", nil))
	body := w.text()
	if strings.Contains(body, secret) {
		t.Fatalf("free text reached a frame:\n%s", body)
	}
	for _, want := range []string{`"name":"unknown"`, `"id":"12"`, `"direction":"up"`, `"carrierType":"noteType"`, `"field":"CustomHeaderCSS"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %s in:\n%s", want, body)
		}
	}
	if strings.Contains(body, `"offset"`) {
		t.Fatalf("a negative number must be dropped:\n%s", body)
	}
}

func waitUntil(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

var _ = errors.New

// A healthy stream that is idle for longer than the write deadline survives:
// over HTTP/2 a deadline left in the past resets the stream, so each frame's
// deadline is cleared once the frame is flushed.
func TestStreamIdleLongerThanTheWriteDeadlineStaysOpenOverHTTP2(t *testing.T) {
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s, _, _ := newTestStream(w, templateRunStreamConfig{Heartbeat: 250 * time.Millisecond, WriteTimeout: 80 * time.Millisecond})
		defer s.Close()
		s.TurnStarted(application_context.TemplateTurnEvent{Turn: 1, MaxTurns: 3})
		time.Sleep(600 * time.Millisecond) // the model is thinking; only heartbeats flow
		s.Drain()
		_ = s.Deliver(templateRunFrame("result", map[string]any{"content": "done"}))
	}))
	srv.EnableHTTP2 = true
	srv.StartTLS()
	defer srv.Close()
	resp, err := srv.Client().Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.ProtoMajor != 2 {
		t.Fatalf("want HTTP/2, got %s", resp.Proto)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("the idle stream was reset: %v (read %q)", err, body)
	}
	events, comments := parseStream(t, string(body))
	if comments == 0 || events[len(events)-1].event != "result" {
		t.Fatalf("want heartbeats and the result, got %d comments, events %v", comments, events)
	}
}
