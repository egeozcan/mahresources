package application_context

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

const DefaultDeepSeekChatCompletionsURL = "https://api.deepseek.com/chat/completions"

const deepSeekMRQLSystemPrompt = `You generate MRQL. Return JSON only. The response content must be one JSON object with exactly the keys query and explanation, like {"query":"type = resource LIMIT 50","explanation":"Finds resources."}. Do not use markdown or extra keys.`

type deepSeekMRQLDraftProvider struct {
	url    string
	apiKey string
	model  string
	client *http.Client
}

func NewDeepSeekMRQLDraftProvider(url, apiKey, model string, client *http.Client) MRQLDraftProvider {
	if url == "" {
		url = DefaultDeepSeekChatCompletionsURL
	}
	if client == nil {
		client = http.DefaultClient
	}
	if model == "" {
		model = DefaultDeepSeekMRQLGenerationModel
	}
	return &deepSeekMRQLDraftProvider{url: url, apiKey: apiKey, model: model, client: client}
}

func (p *deepSeekMRQLDraftProvider) GenerateDraft(ctx context.Context, prompt string) (providerMRQLDraft, error) {
	content, finishReason, _, err := postDeepSeekChatJSON(ctx, p.client, p.url, p.apiKey, deepSeekMRQLSystemPrompt, prompt, deepSeekChatOptions{Model: p.model, MaxTokens: 800})
	if err != nil {
		return providerMRQLDraft{}, err
	}
	switch finishReason {
	case "", "stop":
	default:
		return providerMRQLDraft{}, fmt.Errorf("provider did not finish cleanly")
	}

	var draft providerMRQLDraft
	if err := json.Unmarshal([]byte(content), &draft); err != nil {
		return providerMRQLDraft{}, err
	}
	return draft, nil
}

// deepSeekChatOptions are the per-call settings of one chat-completions
// request.
type deepSeekChatOptions struct {
	Model     string
	MaxTokens int
	Thinking  bool
}

// postDeepSeekChatJSON sends a chat-completions request with json_object response
// format and returns the first choice's raw content string, finish reason and
// token usage. The caller applies its own finish-reason policy (MRQL rejects a
// truncated "length" response; template generation tolerates it and degrades
// gracefully). Empty content is an error except when the reply was cut off by
// max_tokens: then it is returned as ("", "length", usage, nil), so the caller
// can report truncation instead of a transport failure.
func postDeepSeekChatJSON(ctx context.Context, client *http.Client, url, apiKey, systemPrompt, userPrompt string, opts deepSeekChatOptions) (content, finishReason string, usage TemplateTokenUsage, err error) {
	thinking := "disabled"
	if opts.Thinking {
		thinking = "enabled"
	}
	body := map[string]any{
		"model":      opts.Model,
		"stream":     false,
		"max_tokens": opts.MaxTokens,
		"thinking": map[string]string{
			"type": thinking,
		},
		"response_format": map[string]string{
			"type": "json_object",
		},
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userPrompt},
		},
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return "", "", usage, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return "", "", usage, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return "", "", usage, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", usage, fmt.Errorf("provider returned HTTP %d", resp.StatusCode)
	}

	var decoded struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			PromptTokens            int `json:"prompt_tokens"`
			CompletionTokens        int `json:"completion_tokens"`
			PromptCacheHitTokens    int `json:"prompt_cache_hit_tokens"`
			PromptCacheMissTokens   int `json:"prompt_cache_miss_tokens"`
			CompletionTokensDetails struct {
				ReasoningTokens int `json:"reasoning_tokens"`
			} `json:"completion_tokens_details"`
		} `json:"usage"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return "", "", usage, err
	}
	usage = TemplateTokenUsage{
		Prompt:     decoded.Usage.PromptTokens,
		Completion: decoded.Usage.CompletionTokens,
		Reasoning:  decoded.Usage.CompletionTokensDetails.ReasoningTokens,
		CacheHit:   decoded.Usage.PromptCacheHitTokens,
		CacheMiss:  decoded.Usage.PromptCacheMissTokens,
	}
	if len(decoded.Choices) == 0 {
		return "", "", usage, fmt.Errorf("provider returned no choices")
	}
	choice := decoded.Choices[0]
	content = strings.TrimSpace(choice.Message.Content)
	if content == "" && choice.FinishReason != "length" {
		return "", choice.FinishReason, usage, fmt.Errorf("provider returned empty content")
	}
	return content, choice.FinishReason, usage, nil
}
