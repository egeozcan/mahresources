package templateeval

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"mahresources/application_context"
)

// These fixtures run the real provider adapters against an httptest fake in the
// normal suite (no eval build tag), through the recorder, and pin what the
// recording keeps of each: content, finish reason, tool calls, and the token
// and cache totals. Wire shapes follow application_context/ai_conversation_*_test.go.

const (
	// Chat Completions, DeepSeek flavour: reasoning_content must be replayed.
	fixtureDeepSeekTools = `{"id":"x","choices":[{"index":0,"finish_reason":"tool_calls","message":{"role":"assistant","content":null,"reasoning_content":"I should look  at entity 1 and 2.","tool_calls":[{"id":"call_a","type":"function","function":{"name":"get_entity","arguments":"{\"id\": 1}"}},{"id":"call_b","type":"function","function":{"name":"submit","arguments":"{\"fields\":{\"CustomHeader\":\"<p>hi</p>\"},\"done\":true}"}}]}}],"usage":{"prompt_tokens":1200,"completion_tokens":90,"prompt_cache_hit_tokens":1000,"prompt_cache_miss_tokens":200,"completion_tokens_details":{"reasoning_tokens":40}}}`
	// OpenRouter reports cache hits as prompt_tokens_details.cached_tokens only,
	// and this reply was cut inside its second call's arguments.
	fixtureOpenRouterCut  = `{"id":"y","choices":[{"index":0,"finish_reason":"length","message":{"role":"assistant","content":"Working on it.","tool_calls":[{"id":"call_a","type":"function","function":{"name":"get_entity","arguments":"{\"id\":1}"}},{"id":"call_b","type":"function","function":{"name":"submit","arguments":"{\"fields\":{\"CustomHea"}}]}}],"usage":{"prompt_tokens":500,"completion_tokens":80,"prompt_tokens_details":{"cached_tokens":400}}}`
	fixtureResponsesTools = `{"id":"resp_1","status":"completed","output":[{"type":"reasoning","id":"rs_1","summary":[],"encrypted_content":"gAAAA  opaque=="},{"type":"function_call","id":"fc_1","call_id":"call_a","name":"get_entity","arguments":"{\"id\": 1}","status":"completed"},{"type":"function_call","id":"fc_2","call_id":"call_b","name":"submit","arguments":"{\"fields\":{\"CustomCSS\":\"a{b:c}\"}}","status":"completed"}],"usage":{"input_tokens":900,"output_tokens":60,"input_tokens_details":{"cached_tokens":512},"output_tokens_details":{"reasoning_tokens":30}}}`
	fixtureAnthropicTools = `{"id":"msg_1","role":"assistant","content":[{"type":"thinking","thinking":"Look  at both.","signature":"sig=="},{"type":"text","text":"Checking."},{"type":"tool_use","id":"toolu_a","name":"get_entity","input":{"id": 1}},{"type":"tool_use","id":"toolu_b","name":"submit","input":{"fields":{"CustomHeader":"<p>x</p>"}}}],"stop_reason":"tool_use","usage":{"input_tokens":120,"output_tokens":55,"cache_read_input_tokens":2000,"cache_creation_input_tokens":300,"output_tokens_details":{"thinking_tokens":25}}}`

	fixtureChatClassic         = `{"choices":[{"message":{"content":"{\"content\":\"a{b:c}\",\"explanation\":\"e\"}"},"finish_reason":"stop"}],"usage":{"prompt_tokens":100,"completion_tokens":30,"prompt_tokens_details":{"cached_tokens":64},"completion_tokens_details":{"reasoning_tokens":5}}}`
	fixtureDeepSeekCut         = `{"choices":[{"message":{"content":"{\"content\":\"a{b"},"finish_reason":"length"}],"usage":{"prompt_tokens":120,"completion_tokens":30,"prompt_cache_hit_tokens":100,"prompt_cache_miss_tokens":20,"completion_tokens_details":{"reasoning_tokens":7}}}`
	fixtureAnthropicClassicCut = `{"id":"msg_2","content":[{"type":"text","text":"{\"content\":\"a{b"}],"stop_reason":"max_tokens","usage":{"input_tokens":120,"output_tokens":55,"cache_read_input_tokens":2000,"cache_creation_input_tokens":300,"output_tokens_details":{"thinking_tokens":25}}}`
)

func fakeUpstream(t *testing.T, status int, reply string) *http.Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, reply)
	}))
	t.Cleanup(server.Close)
	return &http.Client{Transport: redirectTransport{server.URL}}
}

func conversationRequest() application_context.ConversationRequest {
	return application_context.ConversationRequest{
		System: "SYS", Messages: []application_context.ConversationMessage{{Role: "user", Text: "seed"}},
		Tools: application_context.TemplateRunToolSpecs(), Model: "model-x", Thinking: true, MaxTokens: 1234,
	}
}

type conversationFixture struct {
	name, provider, reply string
	finish                string
	text                  string
	tools                 []string
	incomplete            []bool
	usage                 Usage
	replayHas             string
}

func TestAdapterFixturesHarnessMode(t *testing.T) {
	fixtures := []conversationFixture{
		{"deepseek", "deepseek", fixtureDeepSeekTools, "tool_calls", "", []string{"get_entity", "submit"}, []bool{false, false},
			Usage{Prompt: 1200, Completion: 90, Reasoning: 40, CacheHit: 1000, CacheMiss: 200}, "reasoning_content"},
		{"openrouter", "openrouter", fixtureOpenRouterCut, "length", "Working on it.", []string{"get_entity", "submit"}, []bool{false, true},
			Usage{Prompt: 500, Completion: 80, CacheHit: 400, CacheMiss: 100}, "tool_calls"},
		{"openai", "openai", fixtureResponsesTools, "tool_calls", "", []string{"get_entity", "submit"}, []bool{false, false},
			Usage{Prompt: 900, Completion: 60, Reasoning: 30, CacheHit: 512, CacheMiss: 388}, "encrypted_content"},
		{"anthropic", "anthropic", fixtureAnthropicTools, "tool_calls", "Checking.", []string{"get_entity", "submit"}, []bool{false, false},
			Usage{Prompt: 2420, Completion: 55, Reasoning: 25, CacheHit: 2000, CacheMiss: 420}, "signature"},
	}
	for _, f := range fixtures {
		t.Run(f.name, func(t *testing.T) {
			client := fakeUpstream(t, 200, f.reply)
			direct, err := application_context.NewGenerationConversationProvider(f.provider, "secret-key", "model-x", client).Converse(context.Background(), conversationRequest())
			if err != nil {
				t.Fatal(err)
			}
			recorder := NewRecorder("secret-key")
			wrapped := recorder.WrapConversationProvider(application_context.NewGenerationConversationProvider(f.provider, "secret-key", "model-x", client))
			resp, err := wrapped.Converse(context.Background(), conversationRequest())
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(resp, direct) {
				t.Fatalf("the recorder altered the reply:\n got %#v\nwant %#v", resp, direct)
			}
			calls := recorder.Take()
			if len(calls) != 1 {
				t.Fatalf("calls = %d", len(calls))
			}
			c := calls[0]
			if c.Mode != ModeHarness || c.Turn != 1 || c.Status != 200 || c.ErrorClass != "" || c.Model != "model-x" || c.MaxTokens != 1234 || !c.Thinking {
				t.Fatalf("recorded %#v", c)
			}
			if c.FinishReason != f.finish || c.Content != f.text || c.Usage != f.usage {
				t.Fatalf("finish=%q content=%q usage=%+v, want %q %q %+v", c.FinishReason, c.Content, c.Usage, f.finish, f.text, f.usage)
			}
			if len(c.ToolCalls) != len(f.tools) {
				t.Fatalf("tool calls = %#v", c.ToolCalls)
			}
			for i, tc := range c.ToolCalls {
				if tc.Name != f.tools[i] || tc.Incomplete != f.incomplete[i] {
					t.Fatalf("tool call %d = %#v", i, tc)
				}
			}
			if !strings.Contains(string(c.Replay), f.replayHas) || !json.Valid(c.Replay) {
				t.Fatalf("recorded replay lacks %q: %s", f.replayHas, c.Replay)
			}
			// A complete submit's arguments are kept, decoded and re-encoded.
			if !f.incomplete[1] && !strings.Contains(c.ToolCalls[1].Arguments, `"fields"`) {
				t.Fatalf("submit arguments = %q", c.ToolCalls[1].Arguments)
			}
			if f.incomplete[1] && !strings.HasPrefix(c.ToolCalls[1].Arguments, "[unparsed arguments: ") {
				t.Fatalf("cut-off arguments = %q", c.ToolCalls[1].Arguments)
			}
			if c.UsageUnreported {
				t.Fatalf("usage reported but flagged unreported: %#v", c)
			}
		})
	}
}

type draftFixture struct {
	name, provider, reply string
	finish, content       string
	usage                 Usage
}

func TestAdapterFixturesClassicMode(t *testing.T) {
	fixtures := []draftFixture{
		{"deepseek", "deepseek", fixtureDeepSeekCut, "length", `{"content":"a{b`, Usage{Prompt: 120, Completion: 30, Reasoning: 7, CacheHit: 100, CacheMiss: 20}},
		{"openrouter", "openrouter", fixtureChatClassic, "stop", `{"content":"a{b:c}","explanation":"e"}`, Usage{Prompt: 100, Completion: 30, Reasoning: 5, CacheHit: 64, CacheMiss: 36}},
		{"openai", "openai", fixtureChatClassic, "stop", `{"content":"a{b:c}","explanation":"e"}`, Usage{Prompt: 100, Completion: 30, Reasoning: 5, CacheHit: 64, CacheMiss: 36}},
		{"anthropic", "anthropic", fixtureAnthropicClassicCut, "length", `{"content":"a{b`, Usage{Prompt: 2420, Completion: 55, Reasoning: 25, CacheHit: 2000, CacheMiss: 420}},
	}
	for _, f := range fixtures {
		t.Run(f.name, func(t *testing.T) {
			client := fakeUpstream(t, 200, f.reply)
			req := application_context.TemplateDraftRequest{System: "s", User: "u", Model: "model-x", Thinking: true, MaxTokens: 777}
			direct, err := application_context.NewGenerationTemplateDraftProvider(f.provider, "secret-key", "model-x", client).GenerateDraft(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}
			recorder := NewRecorder("secret-key")
			resp, err := recorder.WrapDraftProvider(application_context.NewGenerationTemplateDraftProvider(f.provider, "secret-key", "model-x", client)).GenerateDraft(context.Background(), req)
			if err != nil || resp != direct {
				t.Fatalf("the recorder altered the reply: %#v %v", resp, err)
			}
			calls := recorder.Take()
			if len(calls) != 1 {
				t.Fatalf("calls = %d", len(calls))
			}
			c := calls[0]
			if c.Mode != ModeClassic || c.Status != 200 || c.Model != "model-x" || c.MaxTokens != 777 || c.FinishReason != f.finish || c.Content != f.content || c.Usage != f.usage {
				t.Fatalf("recorded %#v", c)
			}
			if len(c.ToolCalls) != 0 || len(c.Replay) != 0 {
				t.Fatalf("a classic call recorded harness fields: %#v", c)
			}
		})
	}
}

func TestAdapterFixturesRecordHTTPStatusAndErrorClassInBothModes(t *testing.T) {
	upstreamBody := `{"error":{"message":"bad key secret-key"}}`
	for _, provider := range []string{"deepseek", "openrouter", "openai", "anthropic"} {
		t.Run(provider, func(t *testing.T) {
			client := fakeUpstream(t, http.StatusTooManyRequests, upstreamBody)
			recorder := NewRecorder("secret-key")
			_, err1 := recorder.WrapDraftProvider(application_context.NewGenerationTemplateDraftProvider(provider, "secret-key", "m", client)).GenerateDraft(context.Background(), application_context.TemplateDraftRequest{Model: "m"})
			_, err2 := recorder.WrapConversationProvider(application_context.NewGenerationConversationProvider(provider, "secret-key", "m", client)).Converse(context.Background(), conversationRequest())
			if err1 == nil || err2 == nil {
				t.Fatal("expected provider errors")
			}
			calls := recorder.Take()
			if len(calls) != 2 {
				t.Fatalf("calls = %d", len(calls))
			}
			for _, c := range calls {
				if c.Status != 429 || c.ErrorClass != ErrorClassHTTPStatus {
					t.Fatalf("%s: %#v", c.Mode, c)
				}
			}
			if found := decodedStringsContaining(t, calls, "bad key"); len(found) > 0 {
				t.Fatalf("the upstream body was recorded: %q", found)
			}
		})
	}
}

func TestAdapterFixturesRecordDecodeAndRefusalClasses(t *testing.T) {
	tests := []struct {
		name, provider, reply, class string
	}{
		{"chat refusal", "deepseek", `{"choices":[{"finish_reason":"stop","message":{"role":"assistant","content":null,"refusal":"no"}}]}`, ErrorClassRefusal},
		{"chat garbage", "openrouter", `not json at all`, ErrorClassDecode},
		{"anthropic garbage", "anthropic", `{"content":`, ErrorClassDecode},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := NewRecorder()
			client := fakeUpstream(t, 200, tt.reply)
			if _, err := recorder.WrapConversationProvider(application_context.NewGenerationConversationProvider(tt.provider, "k", "m", client)).Converse(context.Background(), conversationRequest()); err == nil {
				t.Fatal("expected an error")
			}
			if c := recorder.Take()[0]; c.ErrorClass != tt.class {
				t.Fatalf("class = %q, want %q", c.ErrorClass, tt.class)
			}
		})
	}
}
