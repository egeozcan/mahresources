package api_handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"mahresources/application_context"
)

// The event stream of a harness run (plan section 5). One goroutine owns every
// write and flush. Progress events reach it through a bounded channel and are
// droppable; the run's result or error is delivered separately, after progress
// has been drained and access re-checked (template_run_handlers.go).

const (
	// templateRunQueueSize is the progress channel's bound. A full channel drops
	// the event: progress is advisory, the result is not.
	templateRunQueueSize = 64
	// templateRunHeartbeat keeps proxies and the client's idle timer from closing
	// a stream that is waiting on the model.
	templateRunHeartbeat = 15 * time.Second
	// templateRunWriteTimeout bounds each frame's write and flush. The server's
	// own WriteTimeout is 45 minutes, so a client that stops reading would
	// otherwise hold the run for that long.
	templateRunWriteTimeout = 10 * time.Second
)

// templateRunStreamConfig carries the knobs a test shortens.
type templateRunStreamConfig struct {
	Heartbeat    time.Duration
	WriteTimeout time.Duration
}

func (c templateRunStreamConfig) withDefaults() templateRunStreamConfig {
	if c.Heartbeat <= 0 {
		c.Heartbeat = templateRunHeartbeat
	}
	if c.WriteTimeout <= 0 {
		c.WriteTimeout = templateRunWriteTimeout
	}
	return c
}

// templateRunRunEvent is the first frame: the run ID the client finishes the
// run by, and the run's caps when the generator reports them.
type templateRunRunEvent struct {
	RunID       string `json:"runId"`
	MaxTurns    int    `json:"maxTurns,omitempty"`
	Deadline    string `json:"deadline,omitempty"` // Go duration text, as the Disclosure's caps
	TokenBudget int    `json:"tokenBudget,omitempty"`
}

type templateRunTerminal struct {
	frame []byte
	errc  chan error
}

// templateRunStream is the application_context.TemplateRunSink of a streamed
// run and the single writer behind it. It does nothing until the first
// progress event: a run that fails before its first Turn (a bad request, a
// changed grounding hash) is still answered with an ordinary status code.
type templateRunStream struct {
	w   http.ResponseWriter
	rc  *http.ResponseController
	cfg templateRunStreamConfig
	run templateRunRunEvent
	// ctx is the run's context; cancelRun cancels it when a write fails.
	ctx       context.Context
	cancelRun context.CancelFunc

	events     chan []byte
	drainCh    chan chan struct{}
	terminalCh chan templateRunTerminal
	quit       chan struct{}
	done       chan struct{}

	mu        sync.Mutex
	started   bool
	accepting bool
	closeOnce sync.Once
	failed    atomic.Bool
}

func newTemplateRunStream(ctx context.Context, cancelRun context.CancelFunc, w http.ResponseWriter, run templateRunRunEvent, cfg templateRunStreamConfig) *templateRunStream {
	return &templateRunStream{
		w: w, rc: http.NewResponseController(w), cfg: cfg.withDefaults(), run: run,
		ctx: ctx, cancelRun: cancelRun,
		events:     make(chan []byte, templateRunQueueSize),
		drainCh:    make(chan chan struct{}),
		terminalCh: make(chan templateRunTerminal),
		quit:       make(chan struct{}),
		done:       make(chan struct{}),
		accepting:  true,
	}
}

// canStreamTo reports whether w can flush, which an event stream needs.
func canStreamTo(w http.ResponseWriter) bool {
	_, ok := w.(http.Flusher)
	return ok
}

// frame renders one event. json.Marshal never emits a raw newline, so the data
// line cannot be split or forged by a value.
func templateRunFrame(event string, payload any) []byte {
	data, err := json.Marshal(payload)
	if err != nil {
		data = []byte(`{}`)
	}
	out := make([]byte, 0, len(event)+len(data)+16)
	out = append(out, "event: "...)
	out = append(out, event...)
	out = append(out, "\ndata: "...)
	out = append(out, data...)
	out = append(out, "\n\n"...)
	return out
}

// offer queues a progress frame. The first one starts the writer. It never
// blocks: a finished run, a stream that stopped accepting progress and a full
// channel all drop the frame.
func (s *templateRunStream) offer(frame []byte) {
	if s.ctx.Err() != nil {
		return
	}
	s.mu.Lock()
	if !s.accepting {
		s.mu.Unlock()
		return
	}
	if !s.started {
		s.started = true
		go s.loop()
	}
	s.mu.Unlock()
	select {
	case s.events <- frame:
	default:
	}
}

// Started reports whether any byte may have been sent, so the status line is
// committed.
func (s *templateRunStream) Started() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.started
}

// Drain stops accepting progress and waits until every queued frame has been
// written. It reports false when the stream never started or its writer has
// failed, in which case nothing more can be delivered.
func (s *templateRunStream) Drain() bool {
	s.mu.Lock()
	s.accepting = false
	started := s.started
	s.mu.Unlock()
	if !started {
		return false
	}
	ack := make(chan struct{})
	select {
	case s.drainCh <- ack:
		<-ack
	case <-s.done:
	}
	return !s.failed.Load()
}

// Deliver writes the run's one terminal frame and ends the writer. It is the
// only way a result or error reaches the client.
func (s *templateRunStream) Deliver(frame []byte) error {
	t := templateRunTerminal{frame: frame, errc: make(chan error, 1)}
	select {
	case s.terminalCh <- t:
		return <-t.errc
	case <-s.done:
		return errors.New("the event stream is closed")
	}
}

// Close ends the writer if no terminal frame did, and waits for it, so the
// handler never returns while the writer still holds the ResponseWriter.
func (s *templateRunStream) Close() {
	s.closeOnce.Do(func() {
		s.mu.Lock()
		s.accepting = false
		started := s.started
		s.mu.Unlock()
		close(s.quit)
		if started {
			<-s.done
		}
	})
}

func (s *templateRunStream) fail() {
	if s.failed.CompareAndSwap(false, true) {
		s.cancelRun()
	}
}

// write sends one frame under the per-frame deadline. A write or flush error,
// a timeout included, cancels the run and turns every later write into a no-op.
func (s *templateRunStream) write(frame []byte) error {
	if s.failed.Load() {
		return errors.New("the event stream failed")
	}
	// ErrNotSupported (a recorder, a wrapped writer) leaves the server's own
	// deadline in force; any other error would also fail the write below.
	_ = s.rc.SetWriteDeadline(time.Now().Add(s.cfg.WriteTimeout))
	if _, err := s.w.Write(frame); err != nil {
		s.fail()
		return err
	}
	if err := s.rc.Flush(); err != nil {
		s.fail()
		return err
	}
	// The frame is out: a deadline left standing would expire while the stream
	// idles between heartbeats, and HTTP/2 resets a stream whose write deadline
	// has passed. Clearing it leaves the next frame to set its own.
	if err := s.rc.SetWriteDeadline(time.Time{}); err != nil && !errors.Is(err, http.ErrNotSupported) {
		s.fail()
		return err
	}
	return nil
}

func (s *templateRunStream) loop() {
	defer close(s.done)
	h := s.w.Header()
	h.Set("Content-Type", "text/event-stream; charset=utf-8")
	h.Set("Cache-Control", "no-cache")
	h.Set("X-Accel-Buffering", "no")
	s.w.WriteHeader(http.StatusOK)
	_ = s.write(templateRunFrame("run", s.run))

	heartbeat := time.NewTicker(s.cfg.Heartbeat)
	defer heartbeat.Stop()
	for {
		select {
		case frame := <-s.events:
			_ = s.write(frame)
		case <-heartbeat.C:
			_ = s.write([]byte(": heartbeat\n\n"))
		case ack := <-s.drainCh:
			for drained := false; !drained; {
				select {
				case frame := <-s.events:
					_ = s.write(frame)
				default:
					drained = true
				}
			}
			close(ack)
		case t := <-s.terminalCh:
			t.errc <- s.write(t.frame)
			return
		case <-s.quit:
			return
		}
	}
}

// Progress events. The sink is the only producer, and every payload is built
// from validated Tool names, enumerated values, IDs and counts, never from
// content, queries, names, explanations or error text: a frame sent after
// access changed carries nothing the account could not see before the run.

func (s *templateRunStream) TurnStarted(e application_context.TemplateTurnEvent) {
	s.offer(templateRunFrame("turn", map[string]any{
		"turn": e.Turn, "maxTurns": e.MaxTurns, "finalReason": enumOrEmpty(e.FinalReason, stopReasonSet),
	}))
}

func (s *templateRunStream) ToolFinished(e application_context.TemplateToolEvent) {
	s.offer(templateRunFrame("tool", map[string]any{
		"turn": e.Turn, "name": toolNameOrUnknown(e.Name), "args": sanitizeProgressArgs(e.Args),
		"bytes": e.Bytes, "isError": e.IsError, "durationMs": e.DurationMs,
	}))
}

func (s *templateRunStream) Submitted(e application_context.TemplateSubmissionEvent) {
	s.offer(templateRunFrame("submission", map[string]any{
		"turn": e.Turn, "field": fieldOrEmpty(e.Field), "errors": e.Errors, "warnings": e.Warnings,
	}))
}

func (s *templateRunStream) Usage(e application_context.TemplateUsageEvent) {
	s.offer(templateRunFrame("usage", map[string]any{
		"turn": e.Turn, "budgetUsed": e.BudgetUsed, "budgetMax": e.BudgetMax,
	}))
}

var stopReasonSet = map[string]bool{
	application_context.TemplateStopDone: true, application_context.TemplateStopTurnCap: true,
	application_context.TemplateStopFinishNow: true, application_context.TemplateStopDeadline: true,
	application_context.TemplateStopBudget: true, application_context.TemplateStopLengthRepeated: true,
	application_context.TemplateStopNoToolCall: true, application_context.TemplateStopProviderError: true,
}

func enumOrEmpty(v string, set map[string]bool) string {
	if set[v] {
		return v
	}
	return ""
}

var toolNameSet = func() map[string]bool {
	m := map[string]bool{}
	for _, spec := range application_context.TemplateRunToolSpecs() {
		m[spec.Name] = true
	}
	return m
}()

func toolNameOrUnknown(name string) string {
	if toolNameSet[name] {
		return name
	}
	return "unknown"
}

// progressFieldSet is every template field a progress event may name.
var progressFieldSet = func() map[string]bool {
	m := map[string]bool{"MetaSchema": true, "SectionConfig": true}
	for _, entityType := range []string{"group", "resource", "note"} {
		for _, slot := range templateGenerateBundleSlots(entityType) {
			m[slot] = true
		}
	}
	return m
}()

func fieldOrEmpty(field string) string {
	if progressFieldSet[field] {
		return field
	}
	return ""
}

var progressEnums = map[string]map[string]bool{
	"type":        {"group": true, "resource": true, "note": true},
	"kind":        {"group": true, "resource": true, "note": true},
	"entityType":  {"group": true, "resource": true, "note": true},
	"direction":   {"up": true, "down": true, "across": true},
	"carrierType": {"category": true, "resourceCategory": true, "noteType": true},
}

var progressNumberKeys = map[string]bool{
	"id": true, "entityId": true, "number": true, "from": true, "to": true,
	"offset": true, "limit": true, "buckets": true,
}

var progressDigits = regexp.MustCompile(`^[0-9]{1,10}$`)

// sanitizeProgressArgs re-validates the loop's progress arguments against the
// enumerations. The loop already filters them; this is the stream's own
// guarantee, so a future caller of the sink cannot put free text on the wire.
func sanitizeProgressArgs(args map[string]string) map[string]string {
	out := map[string]string{}
	for key, value := range args {
		switch {
		case key == "field":
			if progressFieldSet[value] {
				out[key] = value
			}
		case progressEnums[key] != nil:
			if progressEnums[key][value] {
				out[key] = value
			}
		case progressNumberKeys[key]:
			if progressDigits.MatchString(value) {
				if _, err := strconv.ParseUint(value, 10, 32); err == nil {
					out[key] = value
				}
			}
		}
	}
	return out
}
