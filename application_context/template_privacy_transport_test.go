package application_context

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
)

type capturingTransport struct {
	mu     sync.Mutex
	bodies [][]byte
}

func (c *capturingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	body, _ := io.ReadAll(r.Body)
	c.mu.Lock()
	c.bodies = append(c.bodies, body)
	c.mu.Unlock()
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{}`)), Request: r}, nil
}

func (c *capturingTransport) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.bodies)
}

func stringLeaves(v any, out *[]string) {
	switch x := v.(type) {
	case string:
		*out = append(*out, x)
	case map[string]any:
		for _, e := range x {
			stringLeaves(e, out)
		}
	case []any:
		for _, e := range x {
			stringLeaves(e, out)
		}
	}
}

func transportTestRequest(text string) ConversationRequest {
	return ConversationRequest{
		System: "system prompt", Model: "test-model", MaxTokens: 100,
		Tools:    []ToolSpec{{Name: "get_entity", Description: "reads an entity", Parameters: json.RawMessage(`{"type":"object","properties":{"id":{"type":"integer","description":"the id"}}}`)}},
		Messages: []ConversationMessage{{Role: "user", Text: text}},
	}
}

func TestGuardRunsInPostPayloadForEveryWireAdapter(t *testing.T) {
	for _, provider := range []string{"deepseek", "openrouter", "openai", "anthropic"} {
		t.Run(provider, func(t *testing.T) {
			rt := &capturingTransport{}
			conv := NewGenerationConversationProvider(provider, "key", "test-model", &http.Client{Transport: rt})
			p := NewTemplatePrivacy("tr")
			secret := "Quarterly \"Moonbase\"\nplans — naïve"
			p.Text(secret)
			ctx := WithTemplatePrivacy(context.Background(), p)

			_, err := conv.Converse(ctx, transportTestRequest("here it is: "+secret))
			if !errors.Is(err, ErrTemplatePrivacyLeak) {
				t.Fatalf("a request carrying a value must be refused: %v", err)
			}
			if rt.count() != 0 {
				t.Fatal("the refused request must not be sent")
			}
			// The same leak inside a JSON Tool result string.
			escaped, _ := json.Marshal(map[string]string{"name": secret})
			if _, err := conv.Converse(ctx, transportTestRequest(string(escaped))); !errors.Is(err, ErrTemplatePrivacyLeak) || rt.count() != 0 {
				t.Fatalf("JSON-escaped leak: %v (calls %d)", err, rt.count())
			}
			// A clean request reaches the transport.
			_, _ = conv.Converse(ctx, transportTestRequest("nothing here"))
			if rt.count() != 1 {
				t.Fatalf("a clean request is sent, calls = %d", rt.count())
			}
			// No privacy on the context: no guard.
			_, _ = conv.Converse(context.Background(), transportTestRequest(secret))
			if rt.count() != 2 {
				t.Fatalf("privacy off must not scan: calls = %d", rt.count())
			}
		})
	}
}

// Every string leaf an adapter sends beyond message content must already be in
// the corpus, or a one-letter value could never be told from the skeleton.
func TestAdapterSkeletonStringsAreAllRegistered(t *testing.T) {
	for _, provider := range []string{"deepseek", "openrouter", "openai", "anthropic"} {
		t.Run(provider, func(t *testing.T) {
			rt := &capturingTransport{}
			conv := NewGenerationConversationProvider(provider, "key", "test-model", &http.Client{Transport: rt})
			p := NewTemplatePrivacy("sk")
			req := transportTestRequest("")
			req.System = ""
			req.Thinking = true
			registerRunSkeleton(p, conv, req.Model, req.Tools)
			_, _ = conv.Converse(WithTemplatePrivacy(context.Background(), p), req)
			if rt.count() != 1 {
				t.Fatalf("the skeleton request was refused or not sent")
			}
			var doc any
			if err := json.Unmarshal(rt.bodies[0], &doc); err != nil {
				t.Fatal(err)
			}
			var leaves []string
			stringLeaves(doc, &leaves)
			for _, leaf := range leaves {
				if leaf == "" {
					continue
				}
				if _, ok := p.guard.corpusSet[strings.ToLower(leaf)]; !ok {
					t.Errorf("string leaf %q of the %s request is not in the corpus", leaf, provider)
				}
			}
		})
	}
}

func TestGuardCoversClassicRequests(t *testing.T) {
	for _, provider := range []string{"deepseek", "anthropic"} {
		rt := &capturingTransport{}
		draft := NewGenerationTemplateDraftProvider(provider, "key", "m", &http.Client{Transport: rt})
		p := NewTemplatePrivacy("cl")
		p.Text("Classic Secret Value")
		ctx := WithTemplatePrivacy(context.Background(), p)
		_, err := draft.GenerateDraft(ctx, TemplateDraftRequest{System: "s", User: "u Classic Secret Value", MaxTokens: 10})
		if !errors.Is(err, ErrTemplatePrivacyLeak) || rt.count() != 0 {
			t.Fatalf("%s: %v (calls %d)", provider, err, rt.count())
		}
	}
	_ = bytes.MinRead
}
