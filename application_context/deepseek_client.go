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
	content, finishReason, err := postDeepSeekChatJSON(ctx, p.client, p.url, p.apiKey, p.model, deepSeekMRQLSystemPrompt, prompt, 800)
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

// postDeepSeekChatJSON sends a chat-completions request with json_object response
// format and returns the first choice's raw content string and finish reason.
// The caller applies its own finish-reason policy (MRQL rejects a truncated
// "length" response; template generation tolerates it and degrades gracefully).
func postDeepSeekChatJSON(ctx context.Context, client *http.Client, url, apiKey, model, systemPrompt, userPrompt string, maxTokens int) (content, finishReason string, err error) {
	body := map[string]any{
		"model":      model,
		"stream":     false,
		"max_tokens": maxTokens,
		"thinking": map[string]string{
			"type": "disabled",
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
		return "", "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", fmt.Errorf("provider returned HTTP %d", resp.StatusCode)
	}

	var decoded struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return "", "", err
	}
	if len(decoded.Choices) == 0 {
		return "", "", fmt.Errorf("provider returned no choices")
	}
	choice := decoded.Choices[0]
	content = strings.TrimSpace(choice.Message.Content)
	if content == "" {
		return "", choice.FinishReason, fmt.Errorf("provider returned empty content")
	}
	return content, choice.FinishReason, nil
}
