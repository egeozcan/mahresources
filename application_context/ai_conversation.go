package application_context

import (
	"context"
	"encoding/json"
	"fmt"
)

// The conversation seam beside TemplateDraftProvider (which stays for MRQL and
// classic mode): one multi-Turn exchange with native tool calling. See
// docs/plans/2026-11-07-template-generation-harness.md section 1.

// ConversationRequest is one provider call of a run. System, Tools and the
// Seed message are byte-stable across the Turns of a run so they are cached.
type ConversationRequest struct {
	System    string
	Messages  []ConversationMessage
	Tools     []ToolSpec
	Model     string
	Thinking  bool
	MaxTokens int
}

// ConversationMessage is one message. Role is user, assistant or tool.
type ConversationMessage struct {
	Role    string
	Text    string
	Calls   []ToolCall   // assistant only
	Results []ToolResult // tool only: every result of a Turn, in call order
	// Replay is the provider's own assistant payload, resent verbatim.
	Replay json.RawMessage
}

// ToolSpec declares one Tool. Parameters is a JSON Schema object.
type ToolSpec struct {
	Name, Description string
	Parameters        json.RawMessage
}

// ToolCall is one call the model made.
type ToolCall struct {
	ID, Name string
	// Arguments is the exact argument text the model sent, never a
	// json.RawMessage: a call cut off at the output limit carries text that is
	// not JSON, and a RawMessage holding it makes every later encoding of the
	// result, transcript or eval report fail. The loop decodes it only after
	// validation.
	Arguments string
	// Incomplete marks a call cut off by the output limit (Finish "length"):
	// its arguments did not arrive whole. Malformed marks complete arguments
	// that are not a JSON object. Neither runs; each gets an error result, so
	// every call ID is answered and the conversation stays valid.
	Incomplete, Malformed bool
}

// ToolResult answers one ToolCall.
type ToolResult struct {
	CallID, Text string
	IsError      bool
}

// ConversationResponse is one provider reply.
type ConversationResponse struct {
	Text   string
	Calls  []ToolCall
	Finish string // stop | length | tool_calls
	Replay json.RawMessage
	Usage  ConversationUsage
}

// ConversationUsage is the provider's accounting for one Turn. A field is nil
// when the provider did not report it, so a missing value is never read as
// zero. Classic mode keeps TemplateTokenUsage.
type ConversationUsage struct {
	Prompt, Completion, Reasoning, CacheHit, CacheMiss *int
}

// TemplateConversationProvider is the tool-calling provider seam.
type TemplateConversationProvider interface {
	Converse(ctx context.Context, req ConversationRequest) (ConversationResponse, error)
	Endpoint() string
	ProviderName() string
	Transport() TemplateTransport
	// SupportsTools answers without a paid call where the provider can tell
	// (OpenRouter model metadata); otherwise true, and the first Turn's error
	// says so. An error means "unknown" and does not refuse the run.
	SupportsTools(ctx context.Context, model string) (bool, error)
	// HistoryRewritable reports whether earlier messages may be edited
	// (stubbed) between Turns for this model: true only for DeepSeek and
	// OpenRouter "deepseek/" models, because other providers bind signed
	// reasoning to every preceding message.
	HistoryRewritable(model string) bool
}

// Classes of GenerationProviderError.
const (
	GenerationErrorClassHTTP      = "http"
	GenerationErrorClassTransport = "transport"
	GenerationErrorClassDecode    = "decode"
	GenerationErrorClassSize      = "size"
	GenerationErrorClassRefusal   = "refusal"
)

// GenerationProviderError is a sanitized, typed provider failure: it carries
// the HTTP status and a class, never a body or header. The eval recorder reads
// it with errors.As.
type GenerationProviderError struct {
	Status int
	Class  string
}

func (e *GenerationProviderError) Error() string {
	if e.Class == GenerationErrorClassHTTP && e.Status != 0 {
		return fmt.Sprintf("provider returned HTTP %d", e.Status)
	}
	return fmt.Sprintf("provider call failed (%s)", e.Class)
}
