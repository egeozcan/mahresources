package templateeval

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"mahresources/application_context"
	"mahresources/shortcodes"
)

const echoedSecret = "sk-echoed-secret-0123456789"

// decodedStringsContaining encodes v, decodes it generically, and returns every
// decoded string value or key that contains needle, so an escaped form in the
// encoding cannot hide it.
func decodedStringsContaining(t *testing.T, v any, needle string) []string {
	t.Helper()
	encoded, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var generic any
	if err := json.Unmarshal(encoded, &generic); err != nil {
		t.Fatal(err)
	}
	var found []string
	var walk func(any)
	walk = func(n any) {
		switch x := n.(type) {
		case string:
			if strings.Contains(x, needle) {
				found = append(found, x)
			}
		case []any:
			for _, e := range x {
				walk(e)
			}
		case map[string]any:
			for k, e := range x {
				walk(k)
				walk(e)
			}
		}
	}
	walk(generic)
	return found
}

func intp(v int) *int { return &v }

// fakeDraft is a classic provider returning a scripted reply or error.
type fakeDraft struct {
	resp application_context.TemplateDraftResponse
	err  error
	got  []application_context.TemplateDraftRequest
}

func (f *fakeDraft) GenerateDraft(_ context.Context, req application_context.TemplateDraftRequest) (application_context.TemplateDraftResponse, error) {
	f.got = append(f.got, req)
	return f.resp, f.err
}
func (f *fakeDraft) Endpoint() string { return "https://example.test/classic" }

// fakeConversation is a harness provider returning scripted replies.
type fakeConversation struct {
	replies []application_context.ConversationResponse
	errs    []error
	got     []application_context.ConversationRequest
}

func (f *fakeConversation) Converse(_ context.Context, req application_context.ConversationRequest) (application_context.ConversationResponse, error) {
	n := len(f.got)
	f.got = append(f.got, req)
	if n < len(f.errs) && f.errs[n] != nil {
		return application_context.ConversationResponse{}, f.errs[n]
	}
	return f.replies[n], nil
}
func (f *fakeConversation) Endpoint() string     { return "https://example.test/conv" }
func (f *fakeConversation) ProviderName() string { return "fake" }
func (f *fakeConversation) Transport() application_context.TemplateTransport {
	return application_context.TemplateTransport{}
}
func (f *fakeConversation) SupportsTools(context.Context, string) (bool, error) { return true, nil }
func (f *fakeConversation) HistoryRewritable(string) bool                       { return false }

func TestRecorderClassicRecordsTheSeamAndPassesThrough(t *testing.T) {
	inner := &fakeDraft{resp: application_context.TemplateDraftResponse{
		Content: `{"content":"x ` + echoedSecret + `"}`, FinishReason: "length",
		Usage: application_context.TemplateTokenUsage{Prompt: 120, Completion: 30, Reasoning: 7, CacheHit: 100, CacheMiss: 20},
	}}
	recorder := NewRecorder(echoedSecret)
	provider := recorder.WrapDraftProvider(inner)
	req := application_context.TemplateDraftRequest{System: "s", User: "u", Model: "deepseek-flash", Thinking: true, MaxTokens: 4000}
	resp, err := provider.GenerateDraft(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if resp != inner.resp || len(inner.got) != 1 || inner.got[0] != req {
		t.Fatalf("the exchange was altered: resp=%#v got=%#v", resp, inner.got)
	}
	if provider.Endpoint() != "https://example.test/classic" {
		t.Fatal("Endpoint is not forwarded")
	}
	calls := recorder.Take()
	if len(calls) != 1 {
		t.Fatalf("calls = %d", len(calls))
	}
	c := calls[0]
	want := Usage{Prompt: 120, Completion: 30, Reasoning: 7, CacheHit: 100, CacheMiss: 20}
	if c.Mode != ModeClassic || c.Model != "deepseek-flash" || !c.Thinking || c.MaxTokens != 4000 || c.FinishReason != "length" || c.Usage != want || c.Status != 200 {
		t.Fatalf("recorded %#v", c)
	}
	if strings.Contains(c.Content, "echoed-secret") || !strings.Contains(c.Content, "[redacted]") {
		t.Fatalf("content = %q", c.Content)
	}
	if len(recorder.Take()) != 0 {
		t.Fatal("Take did not clear the recording")
	}
}

func TestRecorderClassifiesProviderErrorsWithoutKeepingTheirText(t *testing.T) {
	typed := func(class string, status int) error {
		return &application_context.GenerationProviderError{Class: class, Status: status}
	}
	// A wrapped error whose message carries a credential: only class and
	// status may reach the recording.
	leaky := func(cause error) error {
		return errors.Join(errors.New("upstream said "+echoedSecret), cause)
	}
	var netTimeout net.Error = &net.DNSError{IsTimeout: true}
	tests := []struct {
		name       string
		err        error
		class      string
		status     int
		classicCtx func() (context.Context, context.CancelFunc)
	}{
		{"http", leaky(typed(application_context.GenerationErrorClassHTTP, 429)), ErrorClassHTTPStatus, 429, nil},
		{"decode", leaky(typed(application_context.GenerationErrorClassDecode, 0)), ErrorClassDecode, 0, nil},
		{"size", leaky(typed(application_context.GenerationErrorClassSize, 0)), ErrorClassSize, 0, nil},
		{"refusal", leaky(typed(application_context.GenerationErrorClassRefusal, 0)), ErrorClassRefusal, 0, nil},
		{"transport", leaky(typed(application_context.GenerationErrorClassTransport, 0)), ErrorClassTransport, 0, nil},
		{"transport with deadline cause", leaky(errors.Join(context.DeadlineExceeded, typed(application_context.GenerationErrorClassTransport, 0))), ErrorClassTimeout, 0, nil},
		{"untyped timeout", netTimeout, ErrorClassTimeout, 0, nil},
		{"untyped", errors.New(echoedSecret), ErrorClassTransport, 0, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := NewRecorder(echoedSecret)
			if _, err := recorder.WrapDraftProvider(&fakeDraft{err: tt.err}).GenerateDraft(context.Background(), application_context.TemplateDraftRequest{Model: "m"}); !errors.Is(err, tt.err) {
				t.Fatalf("the error was not returned unchanged: %v", err)
			}
			conv := &fakeConversation{errs: []error{tt.err}}
			if _, err := recorder.WrapConversationProvider(conv).Converse(context.Background(), application_context.ConversationRequest{Model: "m"}); !errors.Is(err, tt.err) {
				t.Fatalf("the conversation error was not returned unchanged: %v", err)
			}
			calls := recorder.Take()
			if len(calls) != 2 {
				t.Fatalf("calls = %d", len(calls))
			}
			for _, c := range calls {
				if c.ErrorClass != tt.class || c.Status != tt.status {
					t.Fatalf("%s call: class=%q status=%d, want %q %d", c.Mode, c.ErrorClass, c.Status, tt.class, tt.status)
				}
			}
			if found := decodedStringsContaining(t, calls, "echoed-secret"); len(found) > 0 {
				t.Fatalf("the recording carries upstream text: %q", found)
			}
		})
	}
}

func TestRecorderDeadlineOnTheContextIsATimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	<-ctx.Done()
	recorder := NewRecorder()
	_, _ = recorder.WrapDraftProvider(&fakeDraft{err: errors.New("boom")}).GenerateDraft(ctx, application_context.TemplateDraftRequest{})
	if calls := recorder.Take(); len(calls) != 1 || calls[0].ErrorClass != ErrorClassTimeout {
		t.Fatalf("recorded %#v", calls)
	}
}

func submitCall(args string) application_context.ToolCall {
	return application_context.ToolCall{ID: "c1", Name: "submit", Arguments: args}
}

func TestRecorderHarnessRecordsTurnsToolCallsAndUsage(t *testing.T) {
	replay := json.RawMessage(`{"role":"assistant","content":"hi","tool_calls":[{"id":"c1","function":{"name":"submit","arguments":"{\"fields\":{\"CustomHeader\":\"` + echoedSecret + `\"}}"}}]}`)
	calls := []application_context.ToolCall{
		submitCall(`{"fields":{"CustomHeader":"<p>` + echoedSecret + `</p>"},"done":true}`),
		{ID: "c2", Name: "made_up_tool_" + echoedSecret, Arguments: `{"a":1}`},
		{ID: "c3", Name: "submit", Arguments: `{"fields":{"A":"\u0073k-echoed-secret-0123456789`, Incomplete: true},
		{ID: "c4", Name: "get_entity", Arguments: `[1]`, Malformed: true},
	}
	inner := &fakeConversation{replies: []application_context.ConversationResponse{
		{Text: "thinking " + echoedSecret, Calls: []application_context.ToolCall{{ID: "g", Name: "get_entity", Arguments: `{"id":1}`}}, Finish: "tool_calls",
			Usage: application_context.ConversationUsage{Prompt: intp(1000), Completion: intp(50), Reasoning: intp(10), CacheHit: intp(800), CacheMiss: intp(200)}},
		{Text: "done", Calls: calls, Finish: "length", Replay: replay,
			Usage: application_context.ConversationUsage{Prompt: intp(1100), Completion: intp(60)}},
	}}
	recorder := NewRecorder(echoedSecret)
	provider := recorder.WrapConversationProvider(inner)
	tools := application_context.TemplateRunToolSpecs()
	req1 := application_context.ConversationRequest{System: "S", Messages: []application_context.ConversationMessage{{Role: "user", Text: "seed"}}, Tools: tools, Model: "m", Thinking: true, MaxTokens: 900}
	r1, err := provider.Converse(context.Background(), req1)
	if err != nil {
		t.Fatal(err)
	}
	req2 := req1
	req2.Messages = append(append([]application_context.ConversationMessage(nil), req1.Messages...),
		application_context.ConversationMessage{Role: "assistant", Calls: r1.Calls},
		application_context.ConversationMessage{Role: "tool", Results: []application_context.ToolResult{{CallID: "g", Text: "ok"}}})
	// Keep a private copy of what the provider must see, to prove the live
	// exchange is never altered.
	liveReplay := append(json.RawMessage(nil), replay...)
	liveCalls := append([]application_context.ToolCall(nil), calls...)
	r2, err := provider.Converse(context.Background(), req2)
	if err != nil {
		t.Fatal(err)
	}
	if string(r2.Replay) != string(liveReplay) || len(r2.Calls) != len(liveCalls) {
		t.Fatalf("the live replay was altered: %s", r2.Replay)
	}
	for i := range liveCalls {
		if r2.Calls[i] != liveCalls[i] {
			t.Fatalf("live call %d altered: %#v", i, r2.Calls[i])
		}
	}
	if string(inner.replies[1].Replay) != string(liveReplay) {
		t.Fatal("the scripted reply's replay bytes changed in place")
	}

	rec := recorder.Take()
	if len(rec) != 2 {
		t.Fatalf("calls = %d", len(rec))
	}
	first, second := rec[0], rec[1]
	if first.Mode != ModeHarness || first.Turn != 1 || second.Turn != 2 {
		t.Fatalf("turns = %d, %d (mode %q)", first.Turn, second.Turn, first.Mode)
	}
	if first.Model != "m" || !first.Thinking || first.MaxTokens != 900 || first.ToolCount != len(tools) || first.FinishReason != "tool_calls" {
		t.Fatalf("first = %#v", first)
	}
	if first.Usage != (Usage{Prompt: 1000, Completion: 50, Reasoning: 10, CacheHit: 800, CacheMiss: 200}) || first.UsageUnreported {
		t.Fatalf("first usage = %#v unreported=%v", first.Usage, first.UsageUnreported)
	}
	if !second.UsageUnreported || second.Usage != (Usage{Prompt: 1100, Completion: 60}) {
		t.Fatalf("a missing cache count must not read as reported: %#v", second)
	}
	if second.FinishReason != "length" || second.Content != "done" {
		t.Fatalf("second = %#v", second)
	}
	if len(second.ToolCalls) != 4 || second.ToolCalls[0].Name != "submit" || second.ToolCalls[1].Name != UnknownToolName || second.ToolCalls[3].Name != "get_entity" {
		t.Fatalf("tool calls = %#v", second.ToolCalls)
	}
	if !second.ToolCalls[2].Incomplete || !second.ToolCalls[3].Malformed || second.ToolCalls[0].Incomplete {
		t.Fatalf("flags lost: %#v", second.ToolCalls)
	}
	if got := second.ToolCalls[2].Arguments; !strings.HasPrefix(got, "[unparsed arguments: ") {
		t.Fatalf("cut-off arguments = %q", got)
	}
	var args struct {
		Fields map[string]string
		Done   bool
	}
	if err := json.Unmarshal([]byte(second.ToolCalls[0].Arguments), &args); err != nil || args.Fields["CustomHeader"] != "<p>[redacted]</p>" || !args.Done {
		t.Fatalf("sanitized arguments = %q (%v)", second.ToolCalls[0].Arguments, err)
	}
	if found := decodedStringsContaining(t, rec, "echoed-secret"); len(found) > 0 {
		t.Fatalf("the recording carries the credential: %q", found)
	}
	var replayDoc map[string]any
	if err := json.Unmarshal(second.Replay, &replayDoc); err != nil || replayDoc["role"] != "assistant" {
		t.Fatalf("recorded replay = %s (%v)", second.Replay, err)
	}
	if first.Replay != nil {
		t.Fatalf("a reply without a replay recorded one: %s", first.Replay)
	}
}

func TestRecorderHarnessWithoutAnyUsageIsUnreported(t *testing.T) {
	inner := &fakeConversation{replies: []application_context.ConversationResponse{{Text: "x", Finish: "stop"}}}
	recorder := NewRecorder()
	if _, err := recorder.WrapConversationProvider(inner).Converse(context.Background(), application_context.ConversationRequest{Model: "m"}); err != nil {
		t.Fatal(err)
	}
	if c := recorder.Take()[0]; !c.UsageUnreported || c.Usage != (Usage{}) {
		t.Fatalf("recorded %#v", c)
	}
}

func TestRecorderMapsUnknownFinishReasonsToOther(t *testing.T) {
	recorder := NewRecorder()
	_, _ = recorder.WrapDraftProvider(&fakeDraft{resp: application_context.TemplateDraftResponse{FinishReason: "echo " + echoedSecret}}).GenerateDraft(context.Background(), application_context.TemplateDraftRequest{})
	_, _ = recorder.WrapConversationProvider(&fakeConversation{replies: []application_context.ConversationResponse{{Finish: "echo " + echoedSecret}}}).Converse(context.Background(), application_context.ConversationRequest{})
	for _, c := range recorder.Take() {
		if c.FinishReason != "other" {
			t.Fatalf("finish reason = %q", c.FinishReason)
		}
	}
}

func TestWrappedProvidersKeepTheOptionalInterfacesTheGeneratorReads(t *testing.T) {
	recorder := NewRecorder()
	conv := recorder.WrapConversationProvider(&fakeConversation{})
	if conv.ProviderName() != "fake" || conv.Endpoint() != "https://example.test/conv" {
		t.Fatal("identity not forwarded")
	}
	if conv.HistoryRewritable("x") {
		t.Fatal("HistoryRewritable not forwarded")
	}
	if ok, err := conv.SupportsTools(context.Background(), "m"); !ok || err != nil {
		t.Fatal("SupportsTools not forwarded")
	}
}

func TestRedactReportCoversEveryStringField(t *testing.T) {
	report := RunReport{Results: []CaseResult{{
		Details:  &Details{ProviderError: "Bearer\n" + "xyz123"},
		Draft:    &Draft{Error: echoedSecret, Slots: map[string]string{"CustomHeader": "x " + echoedSecret, echoedSecret: "key"}},
		Previews: map[string]Preview{"CustomHeader": {HTML: "bearer abc.def-ghi", Error: "<" + echoedSecret + ">"}},
		Calls:    []Call{{Content: "Authorization: Bearer\t" + echoedSecret}},
	}}}
	redactedReport := RedactReport(report, echoedSecret)
	for _, leaked := range []string{echoedSecret, "abc.def-ghi", "xyz123"} {
		if found := decodedStringsContaining(t, redactedReport, leaked); len(found) > 0 {
			t.Fatalf("redacted report still contains %q: %q", leaked, found)
		}
	}
	if report.Results[0].Draft.Error != echoedSecret {
		t.Fatal("RedactReport mutated its input")
	}
}

// roundTrip redirects every request to a test server, so an adapter built for a
// provider's fixed endpoint talks to a fake.
type redirectTransport struct{ target string }

func (r redirectTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.URL.Scheme = "http"
	clone.URL.Host = strings.TrimPrefix(r.target, "http://")
	clone.Host = clone.URL.Host
	return http.DefaultTransport.RoundTrip(clone)
}

func TestRecorderRecognizesTheProductionPlanningCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var sent struct {
			Messages []struct{ Role, Content string }
		}
		if err := json.NewDecoder(r.Body).Decode(&sent); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		req := application_context.TemplateDraftRequest{}
		for _, message := range sent.Messages {
			if message.Role == "user" {
				req.User = message.Content
			}
		}
		reply, err := (planAndPairProvider{}).GenerateDraft(r.Context(), req)
		if err != nil {
			t.Error(err)
			w.WriteHeader(500)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": reply.Content}, "finish_reason": "stop"}}})
	}))
	defer server.Close()
	recorder := NewRecorder()
	provider := recorder.WrapDraftProvider(application_context.NewGenerationTemplateDraftProvider("deepseek", "k", "m", &http.Client{Transport: redirectTransport{server.URL}}))
	generator := application_context.NewTemplateGenerator(provider, application_context.TemplateGenerationConfig{APIKey: "k", Timeout: time.Second})
	_, err := disclosedGenerate(generator, application_context.TemplateGenerationInput{Target: application_context.TemplateTargetBundle, EntityType: "group", SlotsSelected: true, BundleSlots: []string{"CustomHeader", "CustomHeaderCSS", "CustomSidebar", "CustomSidebarCSS"}}, "a template")
	if err != nil {
		t.Fatal(err)
	}
	calls := recorder.Take()
	if len(calls) != 3 || !calls[0].Plan || calls[1].Plan || calls[2].Plan {
		t.Fatalf("planning call not distinguished from pairs: %#v", calls)
	}
}

func TestRecorderRecognizesTheProductionRepairCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var sent struct {
			Messages []struct{ Role, Content string }
		}
		if err := json.NewDecoder(r.Body).Decode(&sent); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		content := `.x{color:red}[meta]`
		for _, message := range sent.Messages {
			if strings.Contains(message.Content, "\nRepair request:") {
				content = `.x{color:green}`
			}
		}
		draft, _ := json.Marshal(map[string]string{"content": content, "explanation": "e"})
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": string(draft)}, "finish_reason": "stop"}}})
	}))
	defer server.Close()
	recorder := NewRecorder()
	provider := recorder.WrapDraftProvider(application_context.NewGenerationTemplateDraftProvider("deepseek", "k", "m", &http.Client{Transport: redirectTransport{server.URL}}))
	generator := application_context.NewTemplateGenerator(provider, application_context.TemplateGenerationConfig{APIKey: "k", Timeout: 10 * time.Second})
	result, err := disclosedGenerate(generator, application_context.TemplateGenerationInput{Target: application_context.TemplateTargetSlot, EntityType: "group", Slot: "CustomCSS", Mode: "css", Known: shortcodes.KnownFromBuiltins()}, "some CSS")
	if err != nil {
		t.Fatal(err)
	}
	if result.Repair == nil || !result.Repair.Applied {
		t.Fatalf("control did not apply the repair: %#v", result.Repair)
	}
	calls := recorder.Take()
	if len(calls) != 2 || calls[0].Repair || !calls[1].Repair {
		t.Fatalf("repair not distinguished from original: %#v", calls)
	}
}
