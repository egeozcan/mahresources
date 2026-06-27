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
	body := map[string]any{
		"model":      p.model,
		"stream":     false,
		"max_tokens": 800,
		"response_format": map[string]string{
			"type": "json_object",
		},
		"messages": []map[string]string{
			{"role": "system", "content": "You generate MRQL. Return JSON only."},
			{"role": "user", "content": prompt},
		},
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return providerMRQLDraft{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.url, bytes.NewReader(payload))
	if err != nil {
		return providerMRQLDraft{}, err
	}
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return providerMRQLDraft{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return providerMRQLDraft{}, fmt.Errorf("provider returned HTTP %d", resp.StatusCode)
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
		return providerMRQLDraft{}, err
	}
	if len(decoded.Choices) == 0 {
		return providerMRQLDraft{}, fmt.Errorf("provider returned no choices")
	}
	choice := decoded.Choices[0]
	switch choice.FinishReason {
	case "", "stop":
	default:
		return providerMRQLDraft{}, fmt.Errorf("provider did not finish cleanly")
	}

	content := strings.TrimSpace(choice.Message.Content)
	if content == "" {
		return providerMRQLDraft{}, fmt.Errorf("provider returned empty content")
	}

	var draft providerMRQLDraft
	if err := json.Unmarshal([]byte(content), &draft); err != nil {
		return providerMRQLDraft{}, err
	}
	return draft, nil
}
