package api_tests

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"mahresources/application_context"
	"mahresources/server/api_handlers"
)

// Harness-mode test doubles for the generate handler (lane D1): a scripted
// conversation provider, a generator wrapper that observes the handler's calls,
// and a small reader for the event stream.

type convStep func(ctx context.Context, n int, req application_context.ConversationRequest) (application_context.ConversationResponse, error)

// fakeConversation is a TemplateConversationProvider driven by one step
// function. n is the 1-based call number.
type fakeConversation struct {
	mu    sync.Mutex
	step  convStep
	reqs  []application_context.ConversationRequest
	calls atomic.Int32
}

func (f *fakeConversation) Endpoint() string     { return "https://api.example.test/v1/chat/completions" }
func (f *fakeConversation) ProviderName() string { return "fake" }
func (f *fakeConversation) Transport() application_context.TemplateTransport {
	return application_context.TemplateTransport{ResponseFormat: "tools", ContentType: "application/json", Authorization: "Bearer (your API key, not shown)"}
}
func (f *fakeConversation) SupportsTools(context.Context, string) (bool, error) { return true, nil }
func (f *fakeConversation) HistoryRewritable(string) bool                       { return false }

func (f *fakeConversation) Converse(ctx context.Context, req application_context.ConversationRequest) (application_context.ConversationResponse, error) {
	n := int(f.calls.Add(1))
	f.mu.Lock()
	f.reqs = append(f.reqs, req)
	f.mu.Unlock()
	return f.step(ctx, n, req)
}

func (f *fakeConversation) requests() []application_context.ConversationRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]application_context.ConversationRequest(nil), f.reqs...)
}

func toolReply(calls ...application_context.ToolCall) application_context.ConversationResponse {
	return application_context.ConversationResponse{Calls: calls, Finish: "tool_calls", Replay: json.RawMessage(`{"role":"assistant"}`)}
}

func apiToolCall(id, name string, args any) application_context.ToolCall {
	raw, _ := json.Marshal(args)
	return application_context.ToolCall{ID: id, Name: name, Arguments: string(raw)}
}

func apiSubmit(id string, fields map[string]string, done bool) application_context.ToolCall {
	args := map[string]any{"fields": fields}
	if done {
		args["done"] = true
	}
	return apiToolCall(id, application_context.ToolSubmit, args)
}

// submitOnce is a provider that submits one CustomCSS rule and is done.
func submitOnce(css string) convStep {
	return func(_ context.Context, _ int, _ application_context.ConversationRequest) (application_context.ConversationResponse, error) {
		return toolReply(apiSubmit("s1", map[string]string{"CustomCSS": css}, true)), nil
	}
}

func harnessConfig(maxTurns int) application_context.TemplateGenerationConfig {
	return application_context.TemplateGenerationConfig{
		APIKey: "key", Model: "deepseek-flash", Timeout: 5 * time.Second,
		Harness: &application_context.TemplateHarnessSettings{MaxTurns: maxTurns, Deadline: time.Minute, TokenBudget: 400000},
	}
}

// installHarness installs a harness-mode generator over conv.
func installHarness(tc *TestContext, conv application_context.TemplateConversationProvider, maxTurns int) application_context.TemplateGenerator {
	gen := application_context.NewTemplateHarnessGenerator(conv, harnessConfig(maxTurns))
	tc.AppCtx.SetTemplateGenerator(gen)
	return gen
}

// observedGenerator wraps a generator so a test sees (and can act on) the
// handler's calls: how often it prepared, and when GenerateTemplate returned.
type observedGenerator struct {
	inner      application_context.TemplateGenerator
	mu         sync.Mutex
	prepares   int
	afterRun   func()
	lastRes    *application_context.TemplateGenerationResult
	lastErr    error
	prepareHk  func()
	modeToggle func() string // when set, a Prepare after the first reports this mode
}

func (o *observedGenerator) PrepareTemplateGeneration() (application_context.TemplateGenerationPrep, error) {
	o.mu.Lock()
	o.prepares++
	n := o.prepares
	o.mu.Unlock()
	if o.prepareHk != nil {
		o.prepareHk()
	}
	prep := application_context.TemplateGenerationPrep{GenerationMode: application_context.TemplateGenerationModeHarness, Generator: o}
	if n > 1 && o.modeToggle != nil {
		prep.GenerationMode = o.modeToggle()
	}
	return prep, nil
}

func (o *observedGenerator) prepareCount() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.prepares
}

func (o *observedGenerator) DiscloseTemplate(ctx context.Context, in application_context.TemplateGenerationInput, prompt string) (*application_context.TemplateDisclosure, error) {
	return o.inner.DiscloseTemplate(ctx, in, prompt)
}

func (o *observedGenerator) GenerateTemplate(ctx context.Context, in application_context.TemplateGenerationInput, prompt, hash string) (*application_context.TemplateGenerationResult, error) {
	res, err := o.inner.GenerateTemplate(ctx, in, prompt, hash)
	o.mu.Lock()
	o.lastRes, o.lastErr = res, err
	o.mu.Unlock()
	if o.afterRun != nil {
		o.afterRun()
	}
	return res, err
}

func installObserved(tc *TestContext, conv application_context.TemplateConversationProvider, maxTurns int) *observedGenerator {
	obs := &observedGenerator{inner: application_context.NewTemplateHarnessGenerator(conv, harnessConfig(maxTurns))}
	tc.AppCtx.SetTemplateGenerator(obs)
	return obs
}

// harnessSlotBody is a standalone CustomCSS request on the category route.
func harnessSlotBody() map[string]any {
	return map[string]any{"target": "slot", "slot": "CustomCSS", "mode": "css", "prompt": "make it blue", "privacyOff": true}
}

// disclose runs the dry run and returns the groundingHash.
func disclose(t *testing.T, tc *TestContext, path string, body map[string]any) string {
	t.Helper()
	dry := map[string]any{"dryRun": true}
	for k, v := range body {
		dry[k] = v
	}
	resp := tc.MakeRequest(http.MethodPost, path, dry)
	if resp.Code != http.StatusOK {
		t.Fatalf("dry run: %d %s", resp.Code, resp.Body.String())
	}
	var d struct {
		GroundingHash string `json:"groundingHash"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &d); err != nil || d.GroundingHash == "" {
		t.Fatalf("dry run body has no hash: %v %s", err, resp.Body.String())
	}
	return d.GroundingHash
}

func withHash(body map[string]any, hash string) map[string]any {
	out := map[string]any{"groundingHash": hash}
	for k, v := range body {
		out[k] = v
	}
	return out
}

// sseFrame is one parsed event of the stream.
type sseFrame struct {
	Event string
	Data  string
	Raw   string
}

// sseReader parses an event stream, skipping comment frames (heartbeats).
type sseReader struct {
	br       *bufio.Reader
	Comments int
}

func newSSEReader(r io.Reader) *sseReader { return &sseReader{br: bufio.NewReader(r)} }

func (s *sseReader) next() (sseFrame, error) {
	var f sseFrame
	var raw bytes.Buffer
	for {
		line, err := s.br.ReadString('\n')
		raw.WriteString(line)
		if err != nil {
			return f, err
		}
		line = strings.TrimRight(line, "\r\n")
		switch {
		case line == "":
			if f.Event != "" || f.Data != "" {
				f.Raw = raw.String()
				return f, nil
			}
			raw.Reset()
		case strings.HasPrefix(line, ":"):
			s.Comments++
		case strings.HasPrefix(line, "event: "):
			f.Event = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			if f.Data != "" {
				f.Data += "\n"
			}
			f.Data += strings.TrimPrefix(line, "data: ")
		}
	}
}

// nextWithin reads the next frame or fails the test after d.
func (s *sseReader) nextWithin(t *testing.T, d time.Duration) sseFrame {
	t.Helper()
	type res struct {
		f   sseFrame
		err error
	}
	ch := make(chan res, 1)
	go func() {
		f, err := s.next()
		ch <- res{f, err}
	}()
	select {
	case r := <-ch:
		if r.err != nil {
			t.Fatalf("reading the event stream: %v", r.err)
		}
		return r.f
	case <-time.After(d):
		t.Fatalf("no event within %s", d)
		return sseFrame{}
	}
}

// readUntil collects frames until one named event arrives.
func (s *sseReader) readUntil(t *testing.T, event string, d time.Duration) []sseFrame {
	t.Helper()
	deadline := time.Now().Add(d)
	var frames []sseFrame
	for {
		f := s.nextWithin(t, time.Until(deadline))
		frames = append(frames, f)
		if f.Event == event {
			return frames
		}
		if f.Event == "result" || f.Event == "error" {
			t.Fatalf("stream ended with %q before %q: %s", f.Event, event, f.Data)
		}
	}
}

// readUntilEnd collects frames until the terminal result or error event.
func (s *sseReader) readUntilEnd(t *testing.T, d time.Duration) []sseFrame {
	t.Helper()
	deadline := time.Now().Add(d)
	var frames []sseFrame
	for {
		f := s.nextWithin(t, time.Until(deadline))
		frames = append(frames, f)
		if f.Event == "result" || f.Event == "error" {
			return frames
		}
	}
}

// disclose2 reads the groundingHash out of a dry-run body.
func disclose2(t *testing.T, body string) string {
	t.Helper()
	var d struct {
		GroundingHash string `json:"groundingHash"`
	}
	if err := json.Unmarshal([]byte(body), &d); err != nil || d.GroundingHash == "" {
		t.Fatalf("no groundingHash in %s", body)
	}
	return d.GroundingHash
}

func eventNames(frames []sseFrame) []string {
	var out []string
	for _, f := range frames {
		out = append(out, f.Event)
	}
	return out
}

// postStream opens the generate request as the editor does, on a real server.
func postStream(t *testing.T, baseURL, path string, body map[string]any, header http.Header) (*http.Response, context.CancelFunc) {
	t.Helper()
	raw, _ := json.Marshal(body)
	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+path, bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	for k, v := range header {
		req.Header[k] = v
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		cancel()
		t.Fatalf("POST %s: %v", path, err)
	}
	t.Cleanup(func() { cancel(); resp.Body.Close() })
	return resp, cancel
}

func newHarnessServer(t *testing.T, tc *TestContext) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(tc.Router)
	t.Cleanup(srv.Close)
	return srv
}

// runIDOf reads the run event's ID.
func runIDOf(t *testing.T, f sseFrame) string {
	t.Helper()
	if f.Event != "run" {
		t.Fatalf("want the run event first, got %q (%s)", f.Event, f.Data)
	}
	var run struct {
		RunID string `json:"runId"`
	}
	if err := json.Unmarshal([]byte(f.Data), &run); err != nil || run.RunID == "" {
		t.Fatalf("run event has no runId: %v %s", err, f.Data)
	}
	return run.RunID
}

// sameClient makes every request of a test one account when auth is off: the
// run limit and the finish route key on the client address.
var sameClient = http.Header{"X-Forwarded-For": {"198.51.100.1"}}

func postRunJSON(tc *TestContext, path string, body map[string]any, header http.Header) *httptest.ResponseRecorder {
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range header {
		req.Header[k] = v
	}
	rr := httptest.NewRecorder()
	tc.Router.ServeHTTP(rr, req)
	return rr
}

func postFinish(tc *TestContext, path, runID string, header http.Header) *httptest.ResponseRecorder {
	raw, _ := json.Marshal(map[string]any{"runId": runID})
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range header {
		req.Header[k] = v
	}
	rr := httptest.NewRecorder()
	tc.Router.ServeHTTP(rr, req)
	return rr
}

// waitFor polls cond for up to d.
func waitFor(t *testing.T, d time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

var _ = fmt.Sprintf
var _ = api_handlers.GetGenerateTemplateHandler

// returned is what the wrapped generator last returned.
func (o *observedGenerator) returned() (*application_context.TemplateGenerationResult, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.lastRes, o.lastErr
}
