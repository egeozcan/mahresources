package application_context

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"
)

// Test doubles for the harness run loop: a scripted conversation provider that
// records a deep copy of every request, and a fake Tool executor whose access
// check can be made to fail at a chosen call.

type convReply struct {
	resp ConversationResponse
	err  error
	// hook runs inside Converse before the reply is returned; it may block until
	// ctx is done to model a slow provider.
	hook func(ctx context.Context, req ConversationRequest)
}

type scriptedConversation struct {
	mu          sync.Mutex
	replies     []convReply
	requests    []ConversationRequest
	rewritable  bool
	unsupported bool
	supportsErr error
	endpoint    string
}

func (s *scriptedConversation) Endpoint() string {
	if s.endpoint != "" {
		return s.endpoint
	}
	return "https://api.example.test/v1/chat/completions"
}
func (s *scriptedConversation) ProviderName() string { return "fake" }
func (s *scriptedConversation) Transport() TemplateTransport {
	return TemplateTransport{ResponseFormat: "tools", ContentType: "application/json", Authorization: "Bearer (your API key, not shown)"}
}
func (s *scriptedConversation) SupportsTools(context.Context, string) (bool, error) {
	return !s.unsupported, s.supportsErr
}
func (s *scriptedConversation) HistoryRewritable(string) bool { return s.rewritable }

func (s *scriptedConversation) Converse(ctx context.Context, req ConversationRequest) (ConversationResponse, error) {
	s.mu.Lock()
	idx := len(s.requests)
	s.requests = append(s.requests, cloneConversationRequest(req))
	if idx >= len(s.replies) {
		s.mu.Unlock()
		return ConversationResponse{}, errors.New("script exhausted")
	}
	r := s.replies[idx]
	s.mu.Unlock()
	if r.hook != nil {
		r.hook(ctx, req)
	}
	// A real provider fails when its context ends mid-call.
	if err := ctx.Err(); err != nil {
		return ConversationResponse{}, err
	}
	return r.resp, r.err
}

func (s *scriptedConversation) seen() []ConversationRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]ConversationRequest(nil), s.requests...)
}

func cloneConversationRequest(req ConversationRequest) ConversationRequest {
	out := req
	out.Tools = append([]ToolSpec(nil), req.Tools...)
	out.Messages = make([]ConversationMessage, len(req.Messages))
	for i, m := range req.Messages {
		c := m
		c.Calls = append([]ToolCall(nil), m.Calls...)
		c.Results = append([]ToolResult(nil), m.Results...)
		c.Replay = append(json.RawMessage(nil), m.Replay...)
		out.Messages[i] = c
	}
	return out
}

func replyCalls(calls ...ToolCall) convReply {
	return convReply{resp: ConversationResponse{Calls: calls, Finish: "tool_calls", Replay: json.RawMessage(`{"role":"assistant"}`)}}
}

func replyText(text string) convReply {
	return convReply{resp: ConversationResponse{Text: text, Finish: "stop"}}
}

func replyLength(calls ...ToolCall) convReply {
	return convReply{resp: ConversationResponse{Calls: calls, Finish: "length"}}
}

func (r convReply) withUsage(prompt, completion int) convReply {
	r.resp.Usage = ConversationUsage{Prompt: &prompt, Completion: &completion}
	return r
}

func (r convReply) withHook(h func(ctx context.Context, req ConversationRequest)) convReply {
	r.hook = h
	return r
}

func submitCall(id string, fields map[string]string, done bool) ToolCall {
	args := map[string]any{"fields": fields}
	if done {
		args["done"] = true
	}
	raw, _ := json.Marshal(args)
	return ToolCall{ID: id, Name: ToolSubmit, Arguments: string(raw)}
}

func getEntityCall(id string) ToolCall {
	return ToolCall{ID: id, Name: ToolGetEntity, Arguments: `{"type":"group","id":1}`}
}

// accessCall is the state of a CheckAccess context at the moment of the call.
type accessCall struct {
	err         error
	deadline    time.Time
	hasDeadline bool
}

type fakeRunExecutor struct {
	mu          sync.Mutex
	accessCalls int
	accessCtx   []accessCall
	// access decides the error of the n-th (1-based) CheckAccess call.
	access   func(n int) error
	run      func(ctx context.Context, call TemplateToolInvocation) (TemplateToolOutput, error)
	executed []TemplateToolInvocation
}

func (f *fakeRunExecutor) CheckAccess(ctx context.Context) error {
	f.mu.Lock()
	f.accessCalls++
	n := f.accessCalls
	dl, hasDL := ctx.Deadline()
	f.accessCtx = append(f.accessCtx, accessCall{err: ctx.Err(), deadline: dl, hasDeadline: hasDL})
	f.mu.Unlock()
	if f.access != nil {
		return f.access(n)
	}
	return nil
}

func (f *fakeRunExecutor) Execute(ctx context.Context, call TemplateToolInvocation) (TemplateToolOutput, error) {
	f.mu.Lock()
	f.executed = append(f.executed, call)
	f.mu.Unlock()
	if f.run != nil {
		return f.run(ctx, call)
	}
	text := "<<<BEGIN " + call.Name + ">>>\nok " + call.CallID + "\n<<<END " + call.Name + ">>>"
	return TemplateToolOutput{Text: text, Bytes: len(text), FullBytes: len(text)}, nil
}

func (f *fakeRunExecutor) executedNames() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var names []string
	for _, c := range f.executed {
		names = append(names, c.Name+":"+c.CallID)
	}
	return names
}

type recordingSink struct {
	mu    sync.Mutex
	turns []TemplateTurnEvent
	tools []TemplateToolEvent
	subs  []TemplateSubmissionEvent
	usage []TemplateUsageEvent
}

func (r *recordingSink) TurnStarted(e TemplateTurnEvent) {
	r.mu.Lock()
	r.turns = append(r.turns, e)
	r.mu.Unlock()
}
func (r *recordingSink) ToolFinished(e TemplateToolEvent) {
	r.mu.Lock()
	r.tools = append(r.tools, e)
	r.mu.Unlock()
}
func (r *recordingSink) Submitted(e TemplateSubmissionEvent) {
	r.mu.Lock()
	r.subs = append(r.subs, e)
	r.mu.Unlock()
}
func (r *recordingSink) Usage(e TemplateUsageEvent) {
	r.mu.Lock()
	r.usage = append(r.usage, e)
	r.mu.Unlock()
}

// harnessSettingsForTest keeps the timeouts short enough for the deadline tests.
func harnessGen(conv TemplateConversationProvider, mutate func(*TemplateGenerationConfig)) *defaultTemplateGenerator {
	cfg := templateGenConfig()
	cfg.Timeout = 5 * time.Second
	cfg.Harness = &TemplateHarnessSettings{MaxTurns: 10, Deadline: time.Minute, TokenBudget: 400000}
	if mutate != nil {
		mutate(&cfg)
	}
	return NewTemplateHarnessGenerator(conv, cfg).(*defaultTemplateGenerator)
}

// harnessCSSInput is a standalone CustomCSS run.
func harnessCSSInput(exec TemplateToolExecutor) TemplateGenerationInput {
	in := slotInput()
	in.Slot, in.Mode = "CustomCSS", "css"
	in.Run = &TemplateRunEnv{Executor: exec, Account: TemplateRunAccount{Label: "everything", UserID: 1, Role: "admin"}}
	in.PartialExists = func(_ context.Context, name string) (bool, error) { return name == "header-bar", nil }
	return in
}

// harnessPairInput is the cluster request the handler builds: a two-slot bundle.
func harnessPairInput(exec TemplateToolExecutor) TemplateGenerationInput {
	in := harnessCSSInput(exec)
	in.Target, in.Slot, in.Mode = TemplateTargetBundle, "", "html"
	in.BundleSlots = []string{"CustomHeader", "CustomHeaderCSS"}
	return in
}

// harnessWholeInput is a whole-template request (fan-out shape) for two
// sections and CustomCSS, with a stylesheet already in the editor.
func harnessWholeInput(exec TemplateToolExecutor, selected bool, seedCSS string) TemplateGenerationInput {
	in := harnessPairInput(exec)
	in.BundleSlots = []string{"CustomHeader", "CustomHeaderCSS", "CustomSidebar", "CustomSidebarCSS", "CustomCSS"}
	in.SlotsSelected = selected
	if seedCSS != "" {
		raw, _ := json.Marshal(map[string]string{"CustomCSS": seedCSS})
		in.CurrentContent = string(raw)
	}
	return in
}

func mustRunError(t *testing.T, err error) *TemplateRunError {
	t.Helper()
	var re *TemplateRunError
	if !errors.As(err, &re) {
		t.Fatalf("want a *TemplateRunError, got %T: %v", err, err)
	}
	return re
}

func recordsOf(res *TemplateGenerationResult, kind string) []TemplateTranscriptRecord {
	var out []TemplateTranscriptRecord
	for _, r := range res.Transcript {
		if r.Kind == kind {
			out = append(out, r)
		}
	}
	return out
}
