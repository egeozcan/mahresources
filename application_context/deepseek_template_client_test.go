package application_context

import (
	"context"
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

	provider := NewDeepSeekTemplateDraftProvider(server.URL, "secret-key", "deepseek-v4-pro", server.Client())
	raw, err := provider.GenerateDraft(context.Background(), "sys", "user", 4000)
	if err != nil {
		t.Fatalf("GenerateDraft: %v", err)
	}
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

	provider := NewDeepSeekTemplateDraftProvider(server.URL, "secret-key", "deepseek-v4-pro", server.Client())
	raw, err := provider.GenerateDraft(context.Background(), "sys", "user", 4000)
	if err != nil {
		t.Fatalf("template provider should tolerate a truncated (length) response, got: %v", err)
	}
	if raw == "" {
		t.Fatal("expected partial content to be returned")
	}
}
