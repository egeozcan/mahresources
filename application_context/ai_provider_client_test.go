package application_context

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

var generationProviderEndpoints = map[string]string{
	"deepseek":   "https://api.deepseek.com/chat/completions",
	"openrouter": "https://openrouter.ai/api/v1/chat/completions",
	"openai":     "https://api.openai.com/v1/chat/completions",
	"anthropic":  "https://api.anthropic.com/v1/messages",
}

type generationRoundTripFunc func(*http.Request) (*http.Response, error)

func (f generationRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// The provider still chooses its production URL; only the connection is routed
// to the test server. This pins the fixed destination as well as the wire body.
func generationTestClient(t *testing.T, provider string, handler http.HandlerFunc) *http.Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	target, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Client{Transport: generationRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != generationProviderEndpoints[provider] {
			t.Errorf("destination = %s, want %s", r.URL, generationProviderEndpoints[provider])
		}
		forward := r.Clone(r.Context())
		forward.URL.Scheme, forward.URL.Host = target.Scheme, target.Host
		return server.Client().Transport.RoundTrip(forward)
	})}
}

func generationReply(t *testing.T, w http.ResponseWriter, provider, content, finish string) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	var reply any
	if provider == "anthropic" {
		reply = map[string]any{
			"content": []map[string]string{{"type": "text", "text": content}}, "stop_reason": finish,
			"usage": map[string]any{"input_tokens": 20, "output_tokens": 12, "cache_read_input_tokens": 5, "cache_creation_input_tokens": 3,
				"output_tokens_details": map[string]int{"thinking_tokens": 4}},
		}
	} else {
		reply = map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"content": content}, "finish_reason": finish}},
			"usage": map[string]any{"prompt_tokens": 28, "completion_tokens": 12, "prompt_tokens_details": map[string]int{"cached_tokens": 5},
				"completion_tokens_details": map[string]int{"reasoning_tokens": 4}},
		}
	}
	if err := json.NewEncoder(w).Encode(reply); err != nil {
		t.Error(err)
	}
}

func generationStop(provider string) string {
	if provider == "anthropic" {
		return "end_turn"
	}
	return "stop"
}

func TestGenerationProvidersSendTheirProtocolAndDecodeDrafts(t *testing.T) {
	for provider := range generationProviderEndpoints {
		t.Run(provider, func(t *testing.T) {
			var bodies []map[string]any
			client := generationTestClient(t, provider, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer secret-key" || r.Header.Get("Content-Type") != "application/json" {
					t.Errorf("unexpected method/auth/content type")
				}
				if provider == "anthropic" && r.Header.Get("Anthropic-Version") != "2026-10-21" {
					t.Errorf("Anthropic version = %q", r.Header.Get("Anthropic-Version"))
				}
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				bodies = append(bodies, body)
				if len(bodies) == 1 {
					generationReply(t, w, provider, `{"query":"type = resource LIMIT 50","explanation":"Finds resources."}`, generationStop(provider))
				} else {
					generationReply(t, w, provider, `{"content":"<p>Hello</p>","explanation":"Greeting."}`, generationStop(provider))
				}
			})
			mrql := NewGenerationMRQLDraftProvider(provider, "secret-key", "default-model", 4321, client)
			draft, err := mrql.GenerateDraft(context.Background(), "user prompt")
			if err != nil || draft.Query != "type = resource LIMIT 50" || draft.Explanation != "Finds resources." {
				t.Fatalf("MRQL draft = %+v, err = %v", draft, err)
			}
			template := NewGenerationTemplateDraftProvider(provider, "secret-key", "default-model", client)
			resp, err := template.GenerateDraft(context.Background(), TemplateDraftRequest{System: "system JSON instructions", User: "user template request", Model: "kind-model", MaxTokens: 1234, Thinking: true})
			if err != nil || resp.Content != `{"content":"<p>Hello</p>","explanation":"Greeting."}` || resp.FinishReason != "stop" {
				t.Fatalf("template response = %+v, err = %v", resp, err)
			}
			if want := (TemplateTokenUsage{Prompt: 28, Completion: 12, CacheHit: 5, CacheMiss: 23, Reasoning: 4}); resp.Usage != want {
				t.Fatalf("usage = %+v, want %+v", resp.Usage, want)
			}
			for i, body := range bodies {
				wantModel, wantTokens := "default-model", float64(4321)
				if i == 1 {
					wantModel, wantTokens = "kind-model", 1234
				}
				if body["model"] != wantModel || body["stream"] != false {
					t.Errorf("request %d model/stream = %v/%v", i, body["model"], body["stream"])
				}
				limitField := "max_tokens"
				if provider == "openai" {
					limitField = "max_completion_tokens"
					if _, exists := body["max_tokens"]; exists {
						t.Error("OpenAI request contains legacy max_tokens")
					}
				}
				if body[limitField] != wantTokens {
					t.Errorf("request %d token limit = %v, want %v", i, body[limitField], wantTokens)
				}
				thinking, thinkingPresent := body["thinking"].(map[string]any)
				if provider == "deepseek" {
					wantThinking := "disabled"
					if i == 1 {
						wantThinking = "enabled"
					}
					if thinking["type"] != wantThinking {
						t.Errorf("thinking = %v, want %s", thinking, wantThinking)
					}
				} else if thinkingPresent {
					t.Error("non-DeepSeek request contains DeepSeek thinking")
				}
				messages, _ := body["messages"].([]any)
				if provider == "anthropic" {
					if _, present := body["response_format"]; present || len(messages) != 1 {
						t.Error("Anthropic request contains chat-completions format")
					}
					if i == 1 && body["system"] != "system JSON instructions" {
						t.Errorf("Anthropic system = %v", body["system"])
					}
				} else {
					format, _ := body["response_format"].(map[string]any)
					if format["type"] != "json_object" || len(messages) != 2 {
						t.Error("chat request missing JSON mode/system/user")
					}
				}
			}
			if template.Endpoint() != generationProviderEndpoints[provider] {
				t.Errorf("Endpoint = %s", template.Endpoint())
			}
			metadata := template.(interface {
				Transport() TemplateTransport
				ReasoningMode() string
			})
			wantFormat, wantReasoning := "json_object", "provider_default"
			if provider == "anthropic" {
				wantFormat = "prompt_json"
			}
			if provider == "deepseek" {
				wantReasoning = "disabled"
			}
			if metadata.Transport().ResponseFormat != wantFormat || metadata.ReasoningMode() != wantReasoning {
				t.Errorf("metadata = %+v / %s", metadata.Transport(), metadata.ReasoningMode())
			}
		})
	}
}

func TestGenerationProvidersHandleTruncationAndRefusals(t *testing.T) {
	for provider := range generationProviderEndpoints {
		for _, outcome := range []string{"partial", "reasoning only", "empty stop", "refusal", "unexpected stop"} {
			t.Run(provider+"/"+outcome, func(t *testing.T) {
				client := generationTestClient(t, provider, func(w http.ResponseWriter, r *http.Request) {
					content, finish := "", generationStop(provider)
					switch outcome {
					case "partial", "reasoning only":
						if outcome == "reasoning only" {
							if provider == "anthropic" {
								_, _ = io.WriteString(w, `{"content":[{"type":"thinking","thinking":"private reasoning"}],"stop_reason":"max_tokens","usage":{"output_tokens":8192,"output_tokens_details":{"thinking_tokens":4}}}`)
							} else {
								_, _ = io.WriteString(w, `{"choices":[{"message":{"content":null,"reasoning":"private reasoning"},"finish_reason":"length"}],"usage":{"completion_tokens":8192,"completion_tokens_details":{"reasoning_tokens":4}}}`)
							}
							return
						}
						finish = "length"
						if provider == "anthropic" {
							finish = "max_tokens"
						}
						if outcome == "partial" {
							content = `{"content":"unfinished`
						}
					case "refusal":
						if provider == "anthropic" {
							_, _ = io.WriteString(w, `{"content":[{"type":"text","text":"{}"}],"stop_reason":"refusal"}`)
						} else {
							_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"{}","refusal":"secret-key"},"finish_reason":"stop"}]}`)
						}
						return
					case "unexpected stop":
						content, finish = "{}", "secret-key"
					}
					generationReply(t, w, provider, content, finish)
				})
				resp, err := NewGenerationTemplateDraftProvider(provider, "secret-key", "m", client).GenerateDraft(context.Background(), TemplateDraftRequest{MaxTokens: 8192})
				truncated := outcome == "partial" || outcome == "reasoning only"
				if truncated {
					if err != nil || resp.FinishReason != "length" || resp.Usage.Reasoning != 4 {
						t.Fatalf("truncation lost: %+v, %v", resp, err)
					}
				} else if err == nil || strings.Contains(err.Error(), "secret-key") {
					t.Fatalf("expected sanitized error, got %v", err)
				}
				_, err = NewGenerationMRQLDraftProvider(provider, "secret-key", "m", 8192, client).GenerateDraft(context.Background(), "prompt")
				if err == nil || strings.Contains(err.Error(), "secret-key") {
					t.Fatalf("MRQL accepted incomplete/refused draft or leaked content: %v", err)
				}
			})
		}
	}
}

func TestGenerationProvidersRejectBadResponsesWithoutExposingBodies(t *testing.T) {
	for provider := range generationProviderEndpoints {
		for _, body := range []string{"secret-key", `{}`, `{"content":[],"stop_reason":"end_turn","choices":[]}`, `{"error":{"message":"secret-key"}}`} {
			t.Run(provider+"/"+body, func(t *testing.T) {
				client := generationTestClient(t, provider, func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, body) })
				_, err := NewGenerationTemplateDraftProvider(provider, "secret-key", "m", client).GenerateDraft(context.Background(), TemplateDraftRequest{})
				if err == nil || strings.Contains(err.Error(), "secret-key") {
					t.Fatalf("expected sanitized error, got %v", err)
				}
			})
		}
		t.Run(provider+"/HTTP error", func(t *testing.T) {
			client := generationTestClient(t, provider, func(w http.ResponseWriter, r *http.Request) { http.Error(w, "secret-key", http.StatusUnauthorized) })
			_, err := NewGenerationTemplateDraftProvider(provider, "secret-key", "m", client).GenerateDraft(context.Background(), TemplateDraftRequest{})
			if err == nil || !strings.Contains(err.Error(), "401") || strings.Contains(err.Error(), "secret-key") {
				t.Fatalf("expected sanitized HTTP status, got %v", err)
			}
		})
		t.Run(provider+"/invalid MRQL JSON", func(t *testing.T) {
			client := generationTestClient(t, provider, func(w http.ResponseWriter, r *http.Request) {
				generationReply(t, w, provider, `{"query":"q","explanation":"e","secret-key":true}`, generationStop(provider))
			})
			_, err := NewGenerationMRQLDraftProvider(provider, "secret-key", "m", 8192, client).GenerateDraft(context.Background(), "prompt")
			if err == nil || strings.Contains(err.Error(), "secret-key") {
				t.Fatalf("expected strict sanitized draft error, got %v", err)
			}
		})
	}
}

func TestGenerationProvidersRefuseRedirectsWithoutMutatingInjectedClient(t *testing.T) {
	for provider := range generationProviderEndpoints {
		t.Run(provider, func(t *testing.T) {
			calls, originalPolicyCalls := 0, 0
			client := generationTestClient(t, provider, func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.Header().Set("Location", "https://sink.invalid/secret-key")
				w.WriteHeader(http.StatusTemporaryRedirect)
			})
			client.CheckRedirect = func(*http.Request, []*http.Request) error { originalPolicyCalls++; return nil }
			_, err := NewGenerationTemplateDraftProvider(provider, "secret-key", "m", client).GenerateDraft(context.Background(), TemplateDraftRequest{})
			if err == nil || strings.Contains(err.Error(), "secret-key") || calls != 1 || originalPolicyCalls != 0 {
				t.Fatalf("redirect result: err=%v calls=%d original policy=%d", err, calls, originalPolicyCalls)
			}
			_ = client.CheckRedirect(nil, nil)
			if originalPolicyCalls != 1 {
				t.Fatal("provider mutated the caller's client")
			}
		})
	}
}

func TestGenerationProviderSanitizesTransportErrorsAndPreservesCancellation(t *testing.T) {
	for _, outcome := range []string{"transport", "cancelled", "deadline"} {
		t.Run(outcome, func(t *testing.T) {
			client := &http.Client{Transport: generationRoundTripFunc(func(r *http.Request) (*http.Response, error) {
				if outcome == "transport" {
					return nil, errors.New("secret-key")
				}
				<-r.Context().Done()
				return nil, fmt.Errorf("secret-key: %w", r.Context().Err())
			})}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if outcome == "cancelled" {
				cancel()
			}
			if outcome == "deadline" {
				var deadlineCancel context.CancelFunc
				ctx, deadlineCancel = context.WithTimeout(ctx, 10*time.Millisecond)
				defer deadlineCancel()
			}
			_, err := NewGenerationTemplateDraftProvider("openai", "secret-key", "m", client).GenerateDraft(ctx, TemplateDraftRequest{})
			if err == nil || strings.Contains(err.Error(), "secret-key") {
				t.Fatalf("unsafe transport error: %v", err)
			}
			if outcome == "cancelled" && !errors.Is(err, context.Canceled) || outcome == "deadline" && !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("context cause lost: %v", err)
			}
		})
	}
}

func TestGenerationProviderBoundsResponseBody(t *testing.T) {
	body := &generationCountingReader{Reader: strings.NewReader(strings.Repeat("x", 2*maxGenerationProviderResponseBytes))}
	client := &http.Client{Transport: generationRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(body)}, nil
	})}
	_, err := NewGenerationTemplateDraftProvider("openai", "secret-key", "m", client).GenerateDraft(context.Background(), TemplateDraftRequest{})
	if err == nil || !strings.Contains(err.Error(), "limit") {
		t.Fatalf("expected response limit error, got %v", err)
	}
	if body.bytesRead > maxGenerationProviderResponseBytes+1 {
		t.Fatalf("read %d response bytes past the limit", body.bytesRead)
	}
}

type generationCountingReader struct {
	io.Reader
	bytesRead int
}

func (r *generationCountingReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	r.bytesRead += n
	return n, err
}

func TestGenerationProvidersGenerateLocallyValidatedDrafts(t *testing.T) {
	for provider := range generationProviderEndpoints {
		t.Run(provider, func(t *testing.T) {
			calls := 0
			client := generationTestClient(t, provider, func(w http.ResponseWriter, r *http.Request) {
				calls++
				content := `{"query":"type = resource LIMIT 50","explanation":"Finds resources."}`
				if calls > 1 {
					content = `{"content":"body { color: #123456; }","explanation":"Uses a blue text color."}`
				}
				generationReply(t, w, provider, content, generationStop(provider))
			})
			mrql := NewMRQLGenerator(NewGenerationMRQLDraftProvider(provider, "key", "model", 8192, client), MRQLGenerationConfig{APIKey: "key", Model: "model", Timeout: time.Second})
			query, err := mrql.GenerateMRQL(context.Background(), "find resources")
			if err != nil || !query.Valid {
				t.Fatalf("MRQL result = %+v, err = %v", query, err)
			}
			generator := NewTemplateGenerator(NewGenerationTemplateDraftProvider(provider, "key", "model", client), TemplateGenerationConfig{
				APIKey: "key", Model: "model", Timeout: time.Second, RepairDisabled: true, Logf: func(string, ...any) {},
			})
			input := TemplateGenerationInput{Target: TemplateTargetSlot, EntityType: "group", Slot: "CustomCSS", Mode: "css"}
			disclosure, err := generator.DiscloseTemplate(input, "use blue text")
			if err != nil || calls != 1 {
				t.Fatalf("disclosure called provider or failed: calls=%d, err=%v", calls, err)
			}
			draft, err := generator.GenerateTemplate(context.Background(), input, "use blue text", disclosure.GroundingHash)
			if err != nil || !draft.Valid || calls != 2 {
				t.Fatalf("template result = %+v, calls=%d, err=%v", draft, calls, err)
			}
		})
	}
}

func TestGenerationProviderContentBlocks(t *testing.T) {
	for _, provider := range []string{"openai", "openrouter", "anthropic"} {
		t.Run(provider, func(t *testing.T) {
			client := generationTestClient(t, provider, func(w http.ResponseWriter, r *http.Request) {
				body := `{"choices":[{"message":{"content":[{"type":"text","text":"{\"query\":\"type = resource LIMIT 50\","},{"type":"text","text":"\"explanation\":\"Finds resources.\"}"}]},"finish_reason":"stop"}]}`
				if provider == "anthropic" {
					body = `{"content":[{"type":"thinking","thinking":"private"},{"type":"redacted_thinking","data":"private"},{"type":"text","text":"{\"query\":\"type = resource LIMIT 50\","},{"type":"text","text":"\"explanation\":\"Finds resources.\"}"}],"stop_reason":"end_turn"}`
				}
				_, _ = io.WriteString(w, body)
			})
			draft, err := NewGenerationMRQLDraftProvider(provider, "key", "model", 8192, client).GenerateDraft(context.Background(), "find resources")
			if err != nil || draft.Query != "type = resource LIMIT 50" || draft.Explanation != "Finds resources." {
				t.Fatalf("draft = %+v, err = %v", draft, err)
			}
		})
	}
}
