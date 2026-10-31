package application_context

import "net/http"

const DefaultDeepSeekChatCompletionsURL = "https://api.deepseek.com/chat/completions"

const deepSeekMRQLSystemPrompt = `You generate MRQL. Return JSON only. The response content must be one JSON object with exactly the keys query and explanation, like {"query":"type = resource LIMIT 50","explanation":"Finds resources."}. Do not use markdown or extra keys.`

// NewDeepSeekMRQLDraftProvider retains the explicit-endpoint constructor used by
// existing callers and evaluations. Runtime configuration uses the fixed-endpoint
// NewGenerationMRQLDraftProvider constructor.
func NewDeepSeekMRQLDraftProvider(url, apiKey, model string, client *http.Client) MRQLDraftProvider {
	if model == "" {
		model = DefaultDeepSeekMRQLGenerationModel
	}
	provider := NewGenerationMRQLDraftProvider("deepseek", apiKey, model, 800, client).(*generationMRQLDraftProvider)
	if url != "" {
		provider.transport.endpoint = url
	}
	return provider
}
