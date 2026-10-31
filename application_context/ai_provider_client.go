package application_context

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
)

const (
	defaultOpenRouterChatCompletionsURL = "https://openrouter.ai/api/v1/chat/completions"
	defaultOpenAIChatCompletionsURL     = "https://api.openai.com/v1/chat/completions"
	defaultAnthropicMessagesURL         = "https://api.anthropic.com/v1/messages"
	// https://platform.claude.com/docs/en/api/versioning
	AnthropicGenerationAPIVersion      = "2026-10-21"
	maxGenerationProviderResponseBytes = 4 << 20
)

type generationProviderClient struct {
	provider, endpoint, apiKey, model string
	client                            *http.Client
}

func newGenerationProviderClient(provider, apiKey, model string, client *http.Client) *generationProviderClient {
	endpoint := ""
	switch provider {
	case "deepseek":
		endpoint = DefaultDeepSeekChatCompletionsURL
	case "openrouter":
		endpoint = defaultOpenRouterChatCompletionsURL
	case "openai":
		endpoint = defaultOpenAIChatCompletionsURL
	case "anthropic":
		endpoint = defaultAnthropicMessagesURL
	}
	if client == nil {
		client = http.DefaultClient
	}
	// Even an injected client must not replay credentials onto a redirected
	// request. Copy it so configuring the provider cannot change another caller.
	protectedClient := *client
	protectedClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &generationProviderClient{provider: provider, endpoint: endpoint, apiKey: apiKey, model: model, client: &protectedClient}
}

type generationMRQLDraftProvider struct {
	transport *generationProviderClient
	maxTokens int
}

// NewGenerationMRQLDraftProvider uses a fixed official endpoint for the chosen
// provider. The caller resolves the provider-specific model and output budget.
func NewGenerationMRQLDraftProvider(provider, apiKey, model string, maxTokens int, client *http.Client) MRQLDraftProvider {
	if maxTokens <= 0 {
		maxTokens = 8192
		if provider == "deepseek" {
			maxTokens = 800
		}
	}
	return &generationMRQLDraftProvider{transport: newGenerationProviderClient(provider, apiKey, model, client), maxTokens: maxTokens}
}

func (p *generationMRQLDraftProvider) GenerateDraft(ctx context.Context, prompt string) (providerMRQLDraft, error) {
	resp, err := p.transport.generate(ctx, TemplateDraftRequest{System: deepSeekMRQLSystemPrompt, User: prompt, MaxTokens: p.maxTokens})
	if err != nil {
		return providerMRQLDraft{}, err
	}
	if resp.FinishReason != "stop" {
		return providerMRQLDraft{}, errors.New("provider did not finish cleanly")
	}
	// Drafts are one object with the documented keys, not markdown, extra
	// objects or a provider's free-form explanation of an error.
	var draft *providerMRQLDraft
	decoder := json.NewDecoder(strings.NewReader(resp.Content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&draft); err != nil || draft == nil {
		return providerMRQLDraft{}, errors.New("provider returned invalid draft JSON")
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return providerMRQLDraft{}, errors.New("provider returned invalid draft JSON")
	}
	return *draft, nil
}

type generationTemplateDraftProvider struct{ transport *generationProviderClient }

// NewGenerationTemplateDraftProvider keeps the existing raw-JSON template seam
// while adapting the provider's request, finish reasons and token accounting.
func NewGenerationTemplateDraftProvider(provider, apiKey, model string, client *http.Client) TemplateDraftProvider {
	return &generationTemplateDraftProvider{transport: newGenerationProviderClient(provider, apiKey, model, client)}
}

func (p *generationTemplateDraftProvider) Endpoint() string { return p.transport.endpoint }

func (p *generationTemplateDraftProvider) ProviderName() string { return p.transport.provider }

func (p *generationTemplateDraftProvider) Transport() TemplateTransport {
	transport := TemplateTransport{Stream: false, ResponseFormat: "json_object", ContentType: "application/json", Authorization: "Bearer (your API key, not shown)"}
	if p.transport.provider == "anthropic" {
		transport.ResponseFormat = "prompt_json"
		transport.APIVersion = AnthropicGenerationAPIVersion
	}
	return transport
}

func (p *generationTemplateDraftProvider) ReasoningMode() string {
	if p.transport.provider == "deepseek" {
		return "disabled"
	}
	return "provider_default"
}

func (p *generationTemplateDraftProvider) GenerateDraft(ctx context.Context, req TemplateDraftRequest) (TemplateDraftResponse, error) {
	return p.transport.generate(ctx, req)
}

func (p *generationProviderClient) generate(ctx context.Context, req TemplateDraftRequest) (TemplateDraftResponse, error) {
	if p.endpoint == "" {
		return TemplateDraftResponse{}, errors.New("unknown generation provider")
	}
	if req.Model == "" {
		req.Model = p.model
	}
	if req.MaxTokens <= 0 {
		req.MaxTokens = 8192
		if p.provider == "deepseek" {
			req.MaxTokens = DefaultTemplateGenerationMaxTokens
		}
	}
	if p.provider == "anthropic" {
		return p.generateAnthropic(ctx, req)
	}
	body := map[string]any{
		"model": req.Model, "stream": false,
		"response_format": map[string]string{"type": "json_object"},
		"messages": []map[string]string{
			{"role": "system", "content": req.System},
			{"role": "user", "content": req.User},
		},
	}
	if p.provider == "openai" {
		body["max_completion_tokens"] = req.MaxTokens
	} else {
		body["max_tokens"] = req.MaxTokens
	}
	if p.provider == "deepseek" {
		thinking := "disabled"
		if req.Thinking {
			thinking = "enabled"
		}
		body["thinking"] = map[string]string{"type": thinking}
	}
	var decoded struct {
		Error   json.RawMessage `json:"error"`
		Choices []struct {
			Message struct {
				Content json.RawMessage `json:"content"`
				Refusal string          `json:"refusal"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			PromptTokens          int  `json:"prompt_tokens"`
			CompletionTokens      int  `json:"completion_tokens"`
			PromptCacheHitTokens  *int `json:"prompt_cache_hit_tokens"`
			PromptCacheMissTokens *int `json:"prompt_cache_miss_tokens"`
			PromptTokensDetails   struct {
				CachedTokens int `json:"cached_tokens"`
			} `json:"prompt_tokens_details"`
			CompletionTokensDetails struct {
				ReasoningTokens int `json:"reasoning_tokens"`
			} `json:"completion_tokens_details"`
		} `json:"usage"`
	}
	if err := p.post(ctx, body, &decoded); err != nil {
		return TemplateDraftResponse{}, err
	}
	if len(decoded.Error) > 0 && string(decoded.Error) != "null" {
		return TemplateDraftResponse{}, errors.New("provider returned an error")
	}
	if len(decoded.Choices) == 0 {
		return TemplateDraftResponse{}, errors.New("provider returned no choices")
	}
	choice := decoded.Choices[0]
	if choice.Message.Refusal != "" {
		return TemplateDraftResponse{}, errors.New("provider refused the request")
	}
	content, err := generationChatContent(choice.Message.Content)
	if err != nil {
		return TemplateDraftResponse{}, err
	}
	u := decoded.Usage
	hit, miss := u.PromptTokensDetails.CachedTokens, 0
	if u.PromptCacheHitTokens != nil {
		hit = *u.PromptCacheHitTokens
	}
	if u.PromptCacheMissTokens != nil {
		miss = *u.PromptCacheMissTokens
	} else if hit > 0 {
		miss = max(0, u.PromptTokens-hit)
	}
	return generationDraftResponse(content, choice.FinishReason, TemplateTokenUsage{Prompt: u.PromptTokens, Completion: u.CompletionTokens, Reasoning: u.CompletionTokensDetails.ReasoningTokens, CacheHit: hit, CacheMiss: miss})
}

func generationChatContent(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", nil
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text, nil
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &parts); err != nil {
		return "", errors.New("provider returned invalid message content")
	}
	var out strings.Builder
	for _, part := range parts {
		if part.Type != "text" {
			return "", errors.New("provider returned non-text content")
		}
		out.WriteString(part.Text)
	}
	return out.String(), nil
}

func (p *generationProviderClient) generateAnthropic(ctx context.Context, req TemplateDraftRequest) (TemplateDraftResponse, error) {
	body := map[string]any{
		"model": req.Model, "max_tokens": req.MaxTokens, "stream": false, "system": req.System,
		"messages": []map[string]string{{"role": "user", "content": req.User}},
	}
	var decoded struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		StopReason  string `json:"stop_reason"`
		StopDetails struct {
			Type string `json:"type"`
		} `json:"stop_details"`
		Usage struct {
			InputTokens              int `json:"input_tokens"`
			OutputTokens             int `json:"output_tokens"`
			CacheReadInputTokens     int `json:"cache_read_input_tokens"`
			CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
			OutputTokensDetails      struct {
				ThinkingTokens int `json:"thinking_tokens"`
			} `json:"output_tokens_details"`
		} `json:"usage"`
	}
	if err := p.post(ctx, body, &decoded); err != nil {
		return TemplateDraftResponse{}, err
	}
	if decoded.StopReason == "refusal" || decoded.StopDetails.Type == "refusal" {
		return TemplateDraftResponse{}, errors.New("provider refused the request")
	}
	finish := ""
	switch decoded.StopReason {
	case "end_turn":
		finish = "stop"
	case "max_tokens", "model_context_window_exceeded":
		finish = "length"
	default:
		return TemplateDraftResponse{}, errors.New("provider did not finish cleanly")
	}
	var content strings.Builder
	for _, block := range decoded.Content {
		switch block.Type {
		case "text":
			content.WriteString(block.Text)
		case "thinking", "redacted_thinking":
			// Internal reasoning is not part of the JSON draft.
		default:
			return TemplateDraftResponse{}, errors.New("provider returned non-text content")
		}
	}
	u := decoded.Usage
	return generationDraftResponse(content.String(), finish, TemplateTokenUsage{
		Prompt: u.InputTokens + u.CacheCreationInputTokens + u.CacheReadInputTokens, Completion: u.OutputTokens,
		Reasoning: u.OutputTokensDetails.ThinkingTokens, CacheHit: u.CacheReadInputTokens, CacheMiss: u.InputTokens + u.CacheCreationInputTokens,
	})
}

func generationDraftResponse(content, finish string, usage TemplateTokenUsage) (TemplateDraftResponse, error) {
	switch finish {
	case "", "stop":
		finish = "stop"
	case "length":
	default:
		return TemplateDraftResponse{}, errors.New("provider did not finish cleanly")
	}
	content = strings.TrimSpace(content)
	if content == "" && finish != "length" {
		return TemplateDraftResponse{}, errors.New("provider returned empty content")
	}
	return TemplateDraftResponse{Content: content, FinishReason: finish, Usage: usage}, nil
}

func (p *generationProviderClient) post(ctx context.Context, body any, out any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return errors.New("provider request encoding failed")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(payload))
	if err != nil {
		return errors.New("provider request is invalid")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	if p.provider == "anthropic" {
		req.Header.Set("Anthropic-Version", AnthropicGenerationAPIVersion)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return generationTransportError(ctx, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("provider returned HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxGenerationProviderResponseBytes+1))
	if err != nil {
		return generationTransportError(ctx, err)
	}
	if len(data) > maxGenerationProviderResponseBytes {
		return errors.New("provider response exceeded the size limit")
	}
	if err := json.Unmarshal(data, out); err != nil {
		return errors.New("provider returned invalid response JSON")
	}
	return nil
}

// Neither arbitrary transport error strings nor JSON parse errors are safe to
// expose: they may contain request headers, provider content or redirect URLs.
func generationTransportError(ctx context.Context, err error) error {
	if cause := ctx.Err(); cause != nil {
		return cause
	}
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	var timeout net.Error
	if errors.As(err, &timeout) && timeout.Timeout() {
		return context.DeadlineExceeded
	}
	return errors.New("provider request failed")
}
