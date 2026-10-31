package application_context

import "net/http"

// NewDeepSeekTemplateDraftProvider retains the explicit-endpoint constructor for
// existing callers and evaluations. model is the fallback when a call names none.
func NewDeepSeekTemplateDraftProvider(url, apiKey, model string, client *http.Client) TemplateDraftProvider {
	if model == "" {
		model = DefaultDeepSeekMRQLGenerationModel
	}
	provider := NewGenerationTemplateDraftProvider("deepseek", apiKey, model, client).(*generationTemplateDraftProvider)
	if url != "" {
		provider.transport.endpoint = url
	}
	return provider
}
