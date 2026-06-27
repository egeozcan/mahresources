package application_context

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDeepSeekClientSendsJSONChatRequest(t *testing.T) {
	var auth string
	var body string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		raw, _ := io.ReadAll(r.Body)
		body = string(raw)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"query\":\"type = resource LIMIT 50\",\"explanation\":\"Finds resources.\"}"},"finish_reason":"stop"}]}`))
	}))
	defer server.Close()

	client := NewDeepSeekMRQLDraftProvider(server.URL, "secret-key", "deepseek-v4-pro", server.Client())
	got, err := client.GenerateDraft(context.Background(), "prompt body")
	if err != nil {
		t.Fatalf("GenerateDraft: %v", err)
	}
	if auth != "Bearer secret-key" {
		t.Fatalf("Authorization header = %q", auth)
	}
	for _, want := range []string{`"model":"deepseek-v4-pro"`, `"stream":false`, `"response_format"`, `"json_object"`, `"max_tokens":800`} {
		if !strings.Contains(body, want) {
			t.Fatalf("request body missing %s: %s", want, body)
		}
	}
	var requestBody map[string]any
	if err := json.Unmarshal([]byte(body), &requestBody); err != nil {
		t.Fatalf("decode request body: %v", err)
	}
	thinking, ok := requestBody["thinking"].(map[string]any)
	if !ok || thinking["type"] != "disabled" {
		t.Fatalf("request body should disable thinking mode, got: %s", body)
	}
	messages, ok := requestBody["messages"].([]any)
	if !ok || len(messages) == 0 {
		t.Fatalf("request body missing messages: %s", body)
	}
	system, ok := messages[0].(map[string]any)
	if !ok {
		t.Fatalf("first message has unexpected shape: %#v", messages[0])
	}
	systemContent, ok := system["content"].(string)
	if !ok || !strings.Contains(systemContent, `{"query":"type = resource LIMIT 50","explanation":"Finds resources."}`) {
		t.Fatalf("system prompt should include exact JSON example, got: %q", systemContent)
	}
	if got.Query != `type = resource LIMIT 50` || got.Explanation != "Finds resources." {
		t.Fatalf("unexpected draft: %#v", got)
	}
}

func TestDeepSeekClientRejectsMalformedProviderContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"not-json"},"finish_reason":"stop"}]}`))
	}))
	defer server.Close()

	client := NewDeepSeekMRQLDraftProvider(server.URL, "secret-key", "deepseek-v4-pro", server.Client())
	if _, err := client.GenerateDraft(context.Background(), "prompt body"); err == nil {
		t.Fatal("expected malformed content error")
	}
}

func TestDeepSeekClientRejectsLengthFinishReason(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"query\":\"type = resource LIMIT 50\",\"explanation\":\"x\"}"},"finish_reason":"length"}]}`))
	}))
	defer server.Close()

	client := NewDeepSeekMRQLDraftProvider(server.URL, "secret-key", "deepseek-v4-pro", server.Client())
	if _, err := client.GenerateDraft(context.Background(), "prompt body"); err == nil {
		t.Fatal("expected finish_reason error")
	}
}

func TestDeepSeekClientTimeoutUsesContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()

	client := NewDeepSeekMRQLDraftProvider(server.URL, "secret-key", "deepseek-v4-pro", server.Client())
	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	if _, err := client.GenerateDraft(ctx, "prompt body"); err == nil {
		t.Fatal("expected timeout/context error")
	}
}
