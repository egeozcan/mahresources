package application_context

import (
	"context"
	"fmt"
	"net/http"
)

type deepSeekTemplateDraftProvider struct {
	url    string
	apiKey string
	model  string
	client *http.Client
}

// NewDeepSeekTemplateDraftProvider builds a TemplateDraftProvider over the same
// DeepSeek chat-completions transport used for MRQL generation.
func NewDeepSeekTemplateDraftProvider(url, apiKey, model string, client *http.Client) TemplateDraftProvider {
	if url == "" {
		url = DefaultDeepSeekChatCompletionsURL
	}
	if client == nil {
		client = http.DefaultClient
	}
	if model == "" {
		model = DefaultDeepSeekMRQLGenerationModel
	}
	return &deepSeekTemplateDraftProvider{url: url, apiKey: apiKey, model: model, client: client}
}

// GenerateDraft returns the raw JSON content string; the generator unmarshals it
// per target. Unlike the MRQL provider it tolerates a truncated ("length")
// response, so a whole-template answer that overflows max_tokens degrades to a
// reviewable invalid result instead of a transport error.
func (p *deepSeekTemplateDraftProvider) GenerateDraft(ctx context.Context, systemPrompt, userPrompt string, maxTokens int) (string, error) {
	if maxTokens <= 0 {
		maxTokens = DefaultTemplateGenerationMaxTokens
	}
	content, finishReason, err := postDeepSeekChatJSON(ctx, p.client, p.url, p.apiKey, p.model, systemPrompt, userPrompt, maxTokens)
	if err != nil {
		return "", err
	}
	switch finishReason {
	case "", "stop", "length":
	default:
		return "", fmt.Errorf("provider did not finish cleanly (%s)", finishReason)
	}
	return content, nil
}
