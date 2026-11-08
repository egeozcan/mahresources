package application_context

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func oneTurn() ConversationRequest {
	return ConversationRequest{System: "S", Messages: []ConversationMessage{{Role: "user", Text: "x"}}, Tools: sampleTools(), MaxTokens: 10}
}

func TestConversationErrorsAreTypedAndSanitized(t *testing.T) {
	t.Run("http status", func(t *testing.T) {
		rec := newConversationRecorder(func(int, string, []byte) (int, string) { return 429, `{"error":"secret-key"}` })
		_, err := converse(t, "deepseek", "m", rec.client(), oneTurn())
		wantProviderError(t, err, GenerationErrorClassHTTP, 429)
		if err.Error() != "provider returned HTTP 429" {
			t.Fatalf("message = %q", err.Error())
		}
	})
	t.Run("transport", func(t *testing.T) {
		client := &http.Client{Transport: generationRoundTripFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("secret-key") })}
		_, err := converse(t, "deepseek", "m", client, oneTurn())
		wantProviderError(t, err, GenerationErrorClassTransport, 0)
		if strings.Contains(err.Error(), "secret-key") || err.Error() != "provider request failed" {
			t.Fatalf("message = %q", err.Error())
		}
	})
	t.Run("cancellation keeps its cause", func(t *testing.T) {
		client := &http.Client{Transport: generationRoundTripFunc(func(r *http.Request) (*http.Response, error) {
			<-r.Context().Done()
			return nil, r.Context().Err()
		})}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := NewGenerationConversationProvider("deepseek", "k", "m", client).Converse(ctx, oneTurn())
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v", err)
		}
		wantProviderError(t, err, GenerationErrorClassTransport, 0)
	})
	t.Run("decode", func(t *testing.T) {
		rec := newConversationRecorder(fixedReply(`<html>secret-key`))
		_, err := converse(t, "deepseek", "m", rec.client(), oneTurn())
		wantProviderError(t, err, GenerationErrorClassDecode, 0)
		if strings.Contains(err.Error(), "secret-key") || err.Error() != "provider returned invalid response JSON" {
			t.Fatalf("message = %q", err.Error())
		}
	})
	t.Run("size", func(t *testing.T) {
		body := &generationCountingReader{Reader: strings.NewReader(strings.Repeat("x", 2*maxGenerationProviderResponseBytes))}
		client := &http.Client{Transport: generationRoundTripFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(body)}, nil
		})}
		_, err := converse(t, "anthropic", "m", client, oneTurn())
		wantProviderError(t, err, GenerationErrorClassSize, 0)
		if body.bytesRead > maxGenerationProviderResponseBytes+1 {
			t.Fatalf("read %d bytes past the cap", body.bytesRead)
		}
	})
	t.Run("redirect is not followed", func(t *testing.T) {
		followed := false
		client := &http.Client{Transport: generationRoundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.Host == "evil.example" {
				followed = true
			}
			return &http.Response{StatusCode: 307, Header: http.Header{"Location": {"https://evil.example/steal"}}, Body: io.NopCloser(strings.NewReader(""))}, nil
		})}
		_, err := converse(t, "openai", "m", client, oneTurn())
		if followed {
			t.Fatal("the redirect was followed with credentials")
		}
		wantProviderError(t, err, GenerationErrorClassHTTP, 307)
	})
	t.Run("unknown provider", func(t *testing.T) {
		_, err := NewGenerationConversationProvider("nope", "k", "m", nil).Converse(context.Background(), oneTurn())
		if err == nil {
			t.Fatal("expected error")
		}
	})
}
