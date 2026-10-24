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
// DeepSeek chat-completions transport used for MRQL generation. model is the
// fallback for a request that names none.
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
// response, including one with no content at all, so a draft that overflows
// max_tokens degrades to a reviewable invalid result instead of a transport
// error.
func (p *deepSeekTemplateDraftProvider) GenerateDraft(ctx context.Context, req TemplateDraftRequest) (TemplateDraftResponse, error) {
	opts := deepSeekChatOptions{Model: req.Model, MaxTokens: req.MaxTokens, Thinking: req.Thinking}
	if opts.Model == "" {
		opts.Model = p.model
	}
	if opts.MaxTokens <= 0 {
		opts.MaxTokens = DefaultTemplateGenerationMaxTokens
	}
	content, finishReason, usage, err := postDeepSeekChatJSON(ctx, p.client, p.url, p.apiKey, req.System, req.User, opts)
	if err != nil {
		return TemplateDraftResponse{}, err
	}
	switch finishReason {
	case "", "stop", "length":
	default:
		return TemplateDraftResponse{}, fmt.Errorf("provider did not finish cleanly (%s)", finishReason)
	}
	return TemplateDraftResponse{Content: content, FinishReason: finishReason, Usage: usage}, nil
}
