package application_context

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDeepSeekTemplateClientSendsConfigurableMaxTokens(t *testing.T) {
	var body string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		body = string(raw)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"content\":\"<div></div>\",\"explanation\":\"x\"}"},"finish_reason":"stop"}]}`))
	}))
	defer server.Close()

	provider := NewDeepSeekTemplateDraftProvider(server.URL, "secret-key", "deepseek-flash", server.Client())
	resp, err := provider.GenerateDraft(context.Background(), TemplateDraftRequest{System: "sys", User: "user", MaxTokens: 4000})
	if err != nil {
		t.Fatalf("GenerateDraft: %v", err)
	}
	raw := resp.Content
	for _, want := range []string{`"response_format"`, `"json_object"`, `"max_tokens":4000`, `"content":"sys"`, `"content":"user"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("request body missing %s: %s", want, body)
		}
	}
	if !strings.Contains(raw, `"content":"<div></div>"`) {
		t.Fatalf("raw content not returned verbatim: %q", raw)
	}
}

func TestDeepSeekTemplateClientToleratesLengthFinishReason(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"slots\":{\"CustomHeader\":\"<h1>trunc"},"finish_reason":"length"}]}`))
	}))
	defer server.Close()

	provider := NewDeepSeekTemplateDraftProvider(server.URL, "secret-key", "deepseek-flash", server.Client())
	resp, err := provider.GenerateDraft(context.Background(), TemplateDraftRequest{System: "sys", User: "user", MaxTokens: 4000})
	if err != nil {
		t.Fatalf("template provider should tolerate a truncated (length) response, got: %v", err)
	}
	if resp.Content == "" || resp.FinishReason != "length" {
		t.Fatal("expected partial content to be returned")
	}
}

func TestDeepSeekTemplateClientSendsRequestModelThinkingAndMaxTokens(t *testing.T) {
	var bodies []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		bodies = append(bodies, body)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{}"},"finish_reason":"stop"}]}`))
	}))
	defer server.Close()

	provider := NewDeepSeekTemplateDraftProvider(server.URL, "secret-key", "fallback-model", server.Client())
	for _, req := range []TemplateDraftRequest{
		{System: "s", User: "u", Model: "deepseek-pro", Thinking: true, MaxTokens: 8192},
		{System: "s", User: "u", MaxTokens: 1600},
	} {
		if _, err := provider.GenerateDraft(context.Background(), req); err != nil {
			t.Fatalf("GenerateDraft: %v", err)
		}
	}
	want := []struct {
		model, thinking string
		maxTokens       float64
	}{
		{"deepseek-pro", "enabled", 8192},
		{"fallback-model", "disabled", 1600},
	}
	for i, w := range want {
		body := bodies[i]
		thinking, _ := body["thinking"].(map[string]any)
		if body["model"] != w.model || thinking["type"] != w.thinking || body["max_tokens"] != w.maxTokens {
			t.Errorf("request %d: model=%v thinking=%v max_tokens=%v, want %s %s %v", i, body["model"], thinking["type"], body["max_tokens"], w.model, w.thinking, w.maxTokens)
		}
	}
}

func TestDeepSeekTemplateClientDecodesUsage(t *testing.T) {
	for name, tc := range map[string]struct {
		body string
		want TemplateTokenUsage
	}{
		"present": {
			body: `{"choices":[{"message":{"content":"{}"},"finish_reason":"stop"}],"usage":{"prompt_tokens":900,"completion_tokens":120,"prompt_cache_hit_tokens":768,"prompt_cache_miss_tokens":132,"completion_tokens_details":{"reasoning_tokens":40}}}`,
			want: TemplateTokenUsage{Prompt: 900, Completion: 120, Reasoning: 40, CacheHit: 768, CacheMiss: 132},
		},
		"missing": {
			body: `{"choices":[{"message":{"content":"{}"},"finish_reason":"stop"}]}`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			resp, err := NewDeepSeekTemplateDraftProvider(server.URL, "k", "m", server.Client()).GenerateDraft(context.Background(), TemplateDraftRequest{MaxTokens: 10})
			if err != nil {
				t.Fatalf("GenerateDraft: %v", err)
			}
			if resp.Usage != tc.want {
				t.Fatalf("usage = %+v, want %+v", resp.Usage, tc.want)
			}
		})
	}
}

// TestDeepSeekTemplateClientEmptyLengthIsTruncationNotError pins that a reply
// cut off before any content arrived reaches the generator, which reports it as
// truncation. An empty reply that stopped on its own is still an error.
func TestDeepSeekTemplateClientEmptyLengthIsTruncationNotError(t *testing.T) {
	for finish, wantErr := range map[string]bool{"length": false, "stop": true} {
		t.Run(finish, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"  "},"finish_reason":"` + finish + `"}],"usage":{"completion_tokens":1600}}`))
			}))
			defer server.Close()
			resp, err := NewDeepSeekTemplateDraftProvider(server.URL, "k", "m", server.Client()).GenerateDraft(context.Background(), TemplateDraftRequest{MaxTokens: 1600})
			if (err != nil) != wantErr {
				t.Fatalf("err = %v, want error: %t", err, wantErr)
			}
			if !wantErr && (resp.Content != "" || resp.FinishReason != "length" || resp.Usage.Completion != 1600) {
				t.Fatalf("resp = %+v", resp)
			}
		})
	}
}

// TestDeepSeekTemplateClientReportsTheURLItPostsTo: the disclosure hashes the
// provider's own endpoint, so it must be the URL requests actually go to.
func TestDeepSeekTemplateClientReportsTheURLItPostsTo(t *testing.T) {
	var posted string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		posted = "http://" + r.Host + r.RequestURI
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{}"},"finish_reason":"stop"}]}`))
	}))
	defer server.Close()
	provider := NewDeepSeekTemplateDraftProvider(server.URL+"/v1/a%2Fb/chat?tenant=A&x=1", "k", "m", server.Client())
	if _, err := provider.GenerateDraft(context.Background(), TemplateDraftRequest{MaxTokens: 1}); err != nil {
		t.Fatal(err)
	}
	if provider.Endpoint() != posted {
		t.Fatalf("Endpoint() = %q, posted to %q", provider.Endpoint(), posted)
	}
	if NewDeepSeekTemplateDraftProvider("", "k", "m", nil).Endpoint() != DefaultDeepSeekChatCompletionsURL {
		t.Fatal("the default endpoint is not reported")
	}
}
