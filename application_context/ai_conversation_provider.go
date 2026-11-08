package application_context

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
)

const defaultOpenAIResponsesURL = "https://api.openai.com/v1/responses"

// generationConversationProvider is the tool-calling seam over the existing
// generationProviderClient, so redirect blocking, the response size cap and
// sanitized errors are shared with classic mode. Three wire adapters serve four
// providers: Chat Completions with tools (DeepSeek, OpenRouter), OpenAI
// Responses, and Anthropic Messages.
type generationConversationProvider struct{ transport *generationProviderClient }

// NewGenerationConversationProvider builds the harness-mode provider. The
// endpoint is fixed per provider, like classic mode's.
func NewGenerationConversationProvider(provider, apiKey, model string, client *http.Client) TemplateConversationProvider {
	return &generationConversationProvider{transport: newGenerationProviderClient(provider, apiKey, model, client)}
}

func (p *generationConversationProvider) ProviderName() string { return p.transport.provider }

// Endpoint is the URL the run's requests go to. OpenAI differs from classic
// mode: tool calling on current OpenAI reasoning models needs Responses.
func (p *generationConversationProvider) Endpoint() string {
	if p.transport.provider == "openai" {
		return defaultOpenAIResponsesURL
	}
	return p.transport.endpoint
}

// ReasoningMode is the reasoning behaviour the adapter sends when the profile
// does not turn thinking on. DeepSeek is the only provider whose adapter always
// sends an explicit toggle ("disabled" unless the profile enables it); every
// other adapter sends no reasoning field, or only an enabling one, so the
// provider's own default applies. The Disclosure upgrades either answer to
// "enabled" when the profile asks for thinking and the adapter honours it.
func (p *generationConversationProvider) ReasoningMode() string {
	if p.transport.provider == "deepseek" {
		return "disabled"
	}
	return "provider_default"
}

func (p *generationConversationProvider) Transport() TemplateTransport {
	t := TemplateTransport{Stream: false, ResponseFormat: "tools", ContentType: "application/json", Authorization: "Bearer (your API key, not shown)"}
	switch p.transport.provider {
	case "anthropic":
		t.APIVersion = AnthropicGenerationAPIVersion
		t.Options = "cache_control=ephemeral"
	case "openrouter":
		t.Options = "provider.require_parameters=true; cache_control=ephemeral (anthropic/ and ~anthropic/ models)"
	case "openai":
		t.Options = "store=false; include=reasoning.encrypted_content; strict=false"
	}
	return t
}

// SkeletonStrings are the fixed strings this adapter's requests carry beyond
// their message content, for Privacy mode's readable corpus.
func (p *generationConversationProvider) SkeletonStrings() []string {
	switch p.transport.provider {
	case "anthropic":
		return anthropicSkeleton
	case "openai":
		return responsesSkeleton
	}
	return chatSkeleton
}

// HistoryRewritable is true only where earlier messages may be edited between
// Turns. Anthropic binds signed thinking to every preceding message, OpenAI's
// encrypted reasoning items are not documented either way, and an OpenRouter
// model outside "deepseek/" may be a Claude model replaying signed reasoning.
func (p *generationConversationProvider) HistoryRewritable(model string) bool {
	switch p.transport.provider {
	case "deepseek":
		return true
	case "openrouter":
		return strings.HasPrefix(model, "deepseek/")
	}
	return false
}

func (p *generationConversationProvider) Converse(ctx context.Context, req ConversationRequest) (ConversationResponse, error) {
	if p.transport.endpoint == "" {
		return ConversationResponse{}, newGenerationFailure("unknown generation provider", GenerationErrorClassTransport, 0)
	}
	if req.Model == "" {
		req.Model = p.transport.model
	}
	if req.MaxTokens <= 0 {
		req.MaxTokens = 8192
	}
	switch p.transport.provider {
	case "anthropic":
		return p.converseAnthropic(ctx, req)
	case "openai":
		return p.converseResponses(ctx, req)
	default:
		return p.converseChat(ctx, req)
	}
}

// classifyArguments applies the shared rule for a call's argument text. Valid
// JSON that is not an object is Malformed. Text that is not JSON at all is
// Incomplete when the reply was cut off at the output limit (the cut can only
// be in the last call), and Malformed otherwise.
func classifyArguments(call *ToolCall, cutOff bool) {
	text := strings.TrimSpace(call.Arguments)
	if json.Valid([]byte(text)) {
		call.Malformed = !bytes.HasPrefix([]byte(text), []byte("{"))
		return
	}
	if cutOff {
		call.Incomplete = true
		return
	}
	call.Malformed = true
}

func decodeFailure() error {
	return newGenerationFailure("provider returned an invalid reply", GenerationErrorClassDecode, 0)
}

func refusalFailure() error {
	return newGenerationFailure("provider refused the request", GenerationErrorClassRefusal, 0)
}

func nonEmptyRaw(raw json.RawMessage) bool {
	s := strings.TrimSpace(string(raw))
	return s != "" && s != "null"
}

// rawSplicer lets a request body carry provider payloads byte for byte.
// json.Marshal compacts and HTML-escapes a json.RawMessage, which would
// rewrite a Replay payload (whitespace outside strings, "<" as \u003c). The
// body is built with a placeholder string where each payload goes, and the
// placeholders are replaced in the encoded bytes.
type rawSplicer struct {
	nonce string
	raws  [][]byte
}

func newRawSplicer() *rawSplicer {
	buf := make([]byte, 12)
	_, _ = rand.Read(buf)
	return &rawSplicer{nonce: hex.EncodeToString(buf)}
}

// ref returns the placeholder to put in the body in place of raw.
func (s *rawSplicer) ref(raw []byte) string {
	s.raws = append(s.raws, raw)
	return "@@raw-" + s.nonce + "-" + strconv.Itoa(len(s.raws)-1) + "@@"
}

func (s *rawSplicer) encode(body any) ([]byte, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	for i, raw := range s.raws {
		placeholder := []byte(`"@@raw-` + s.nonce + "-" + strconv.Itoa(i) + `@@"`)
		payload = bytes.Replace(payload, placeholder, raw, 1)
	}
	if !json.Valid(payload) {
		return nil, errors.New("request body is not valid JSON")
	}
	return payload, nil
}
