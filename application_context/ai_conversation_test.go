package application_context

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
)

// conversationRecorder is an http.Client transport that answers from a handler
// and keeps every request, so a test can assert the exact URL, headers and wire
// body of each call without a real network.
type conversationRecorder struct {
	mu       sync.Mutex
	urls     []string
	headers  []http.Header
	bodies   [][]byte
	respond  func(call int, url string, body []byte) (status int, reply string)
	callsFor func(url string) int
}

func newConversationRecorder(respond func(call int, url string, body []byte) (int, string)) *conversationRecorder {
	return &conversationRecorder{respond: respond}
}

func (c *conversationRecorder) client() *http.Client {
	return &http.Client{Transport: generationRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		var body []byte
		if r.Body != nil {
			body, _ = io.ReadAll(r.Body)
		}
		c.mu.Lock()
		call := len(c.urls)
		c.urls = append(c.urls, r.URL.String())
		c.headers = append(c.headers, r.Header.Clone())
		c.bodies = append(c.bodies, body)
		c.mu.Unlock()
		status, reply := c.respond(call, r.URL.String(), body)
		return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(reply))}, nil
	})}
}

func (c *conversationRecorder) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.urls)
}

// body decodes the n-th request body into a generic map.
func (c *conversationRecorder) body(t *testing.T, n int) map[string]any {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()
	if n >= len(c.bodies) {
		t.Fatalf("only %d requests were made, wanted request %d", len(c.bodies), n)
	}
	var out map[string]any
	if err := json.Unmarshal(c.bodies[n], &out); err != nil {
		t.Fatalf("request %d body is not JSON: %v", n, err)
	}
	return out
}

func (c *conversationRecorder) raw(n int) []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.bodies[n]
}

func fixedReply(reply string) func(int, string, []byte) (int, string) {
	return func(int, string, []byte) (int, string) { return 200, reply }
}

func intp(v int) *int { return &v }

func sampleTools() []ToolSpec {
	return []ToolSpec{
		{Name: "get_entity", Description: "read one entity", Parameters: json.RawMessage(`{"type":"object","properties":{"id":{"type":"integer"}},"required":["id"]}`)},
		{Name: "submit", Description: "submit fields", Parameters: json.RawMessage(`{"type":"object","properties":{"fields":{"type":"object","additionalProperties":{"type":"string"}}}}`)},
	}
}

func converse(t *testing.T, provider, model string, client *http.Client, req ConversationRequest) (ConversationResponse, error) {
	t.Helper()
	if req.Model == "" {
		req.Model = model
	}
	return NewGenerationConversationProvider(provider, "secret-key", model, client).Converse(context.Background(), req)
}

func mustJSONEqual(t *testing.T, got json.RawMessage, want string) {
	t.Helper()
	var a, b any
	if err := json.Unmarshal(got, &a); err != nil {
		t.Fatalf("got is not JSON: %v: %s", err, got)
	}
	if err := json.Unmarshal([]byte(want), &b); err != nil {
		t.Fatal(err)
	}
	ga, _ := json.Marshal(a)
	gb, _ := json.Marshal(b)
	if !bytes.Equal(ga, gb) {
		t.Fatalf("JSON mismatch\n got: %s\nwant: %s", ga, gb)
	}
}

func wantProviderError(t *testing.T, err error, class string, status int) {
	t.Helper()
	var pe *GenerationProviderError
	if !errors.As(err, &pe) {
		t.Fatalf("error %v is not a *GenerationProviderError", err)
	}
	if pe.Class != class || pe.Status != status {
		t.Fatalf("provider error = {%d %s}, want {%d %s}", pe.Status, pe.Class, status, class)
	}
}

func getPath(t *testing.T, v any, path ...any) any {
	t.Helper()
	for _, p := range path {
		switch k := p.(type) {
		case string:
			m, ok := v.(map[string]any)
			if !ok {
				t.Fatalf("path %v: %T is not an object at %q", path, v, k)
			}
			v = m[k]
		case int:
			a, ok := v.([]any)
			if !ok || k >= len(a) {
				t.Fatalf("path %v: not an array of enough length at %d (%T)", path, k, v)
			}
			v = a[k]
		}
	}
	return v
}

// A Replay payload is resent byte for byte: json.Marshal alone would compact the
// whitespace between tokens and rewrite "<" and "&" as < and &.
func TestConversationReplayIsByteIdenticalOnEveryAdapter(t *testing.T) {
	cases := map[string]struct {
		replay string
	}{
		"deepseek":   {`{ "role":"assistant",  "content":"a<b&c", "reasoning_content":"r  <x>" }`},
		"openrouter": {`{ "role":"assistant",  "content":"a<b&c", "reasoning_details":[ {"type":"reasoning.text", "text":"<t>"} ] }`},
		"openai":     {`[ {"type":"reasoning", "id":"rs", "encrypted_content":"a<b&c=="} , {"type":"message","role":"assistant","content":[]} ]`},
		"anthropic":  {`[ {"type":"thinking", "thinking":"a<b&c", "signature":"s"} , {"type":"text","text":"t"} ]`},
	}
	for provider, tc := range cases {
		t.Run(provider, func(t *testing.T) {
			rec := newConversationRecorder(fixedReply(`{}`))
			req := ConversationRequest{System: "S", Tools: sampleTools(), MaxTokens: 10, Messages: []ConversationMessage{
				{Role: "user", Text: "seed"},
				{Role: "assistant", Replay: json.RawMessage(tc.replay)},
				{Role: "user", Text: "next"},
			}}
			// The reply {} is rejected as a decode error; only the request matters here.
			_, _ = converse(t, provider, "m", rec.client(), req)
			want := tc.replay
			if provider == "openai" {
				// Items are spliced one by one; each must appear verbatim.
				for _, item := range []string{`{"type":"reasoning", "id":"rs", "encrypted_content":"a<b&c=="}`, `{"type":"message","role":"assistant","content":[]}`} {
					if !bytes.Contains(rec.raw(0), []byte(item)) {
						t.Fatalf("item not verbatim: %s\nin %s", item, rec.raw(0))
					}
				}
				return
			}
			if !bytes.Contains(rec.raw(0), []byte(want)) {
				t.Fatalf("replay not verbatim:\nwant %s\nin   %s", want, rec.raw(0))
			}
		})
	}
}

func TestConversationInvalidReplayIsAnErrorNotABrokenRequest(t *testing.T) {
	rec := newConversationRecorder(fixedReply(`{}`))
	_, err := converse(t, "deepseek", "m", rec.client(), ConversationRequest{System: "S", Tools: sampleTools(), MaxTokens: 10, Messages: []ConversationMessage{
		{Role: "user", Text: "seed"}, {Role: "assistant", Replay: json.RawMessage(`{"broken`)},
	}})
	if err == nil || rec.count() != 0 {
		t.Fatalf("err = %v, requests = %d", err, rec.count())
	}
}

func TestRuntimeBuildsTheConversationProviderFromTemplateSettings(t *testing.T) {
	s := newAITestSettings(t, TemplateGenerationConfig{})
	g := newRuntimeAIGeneration(s, TemplateGenerationConfig{}, nil)
	if _, err := g.templateConversationProvider(); !errors.Is(err, ErrTemplateGenerationNotConfigured) {
		t.Fatalf("keyless: %v", err)
	}
	if err := s.Apply(map[string]string{"ai_openai_api_key": "k", "ai_template_provider": "openai", "ai_template_openai_model": "my-model"}, nil, "", ""); err != nil {
		t.Fatal(err)
	}
	p, err := g.templateConversationProvider()
	if err != nil {
		t.Fatal(err)
	}
	if p.ProviderName() != "openai" || p.Endpoint() != "https://api.openai.com/v1/responses" {
		t.Fatalf("provider = %s %s", p.ProviderName(), p.Endpoint())
	}
}

// The harness Disclosure reports what each adapter really sends for reasoning:
// only DeepSeek always sends a toggle, so only it may read as "off".
func TestHarnessDisclosureReportsEffectiveReasoning(t *testing.T) {
	cases := []struct {
		provider string
		thinking bool
		want     string
	}{
		{"deepseek", false, "disabled"},
		{"deepseek", true, "enabled"},
		{"anthropic", false, "provider_default"},
		{"openai", false, "provider_default"},
		{"openrouter", false, "provider_default"},
		{"openrouter", true, "enabled"},
	}
	for _, c := range cases {
		t.Run(fmt.Sprintf("%s/thinking=%t", c.provider, c.thinking), func(t *testing.T) {
			conv := NewGenerationConversationProvider(c.provider, "k", "m", nil)
			gen := harnessGen(conv, func(cfg *TemplateGenerationConfig) {
				cfg.Overrides = map[string]TemplateCallOverride{}
				for _, kind := range templateKinds {
					cfg.Overrides[kind] = TemplateCallOverride{Thinking: c.thinking}
				}
			})
			d, err := gen.DiscloseTemplate(context.Background(), harnessCSSInput(&fakeRunExecutor{}), "go")
			if err != nil {
				t.Fatal(err)
			}
			if got := d.Destination.Profiles[0].ReasoningMode; got != c.want {
				t.Fatalf("reasoningMode = %q, want %q", got, c.want)
			}
		})
	}
}
