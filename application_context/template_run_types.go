package application_context

import (
	"context"
	"encoding/json"
	"errors"
)

// Shared types of a harness run (docs/plans/2026-11-07-template-generation-harness.md
// sections 2 and 3). They live apart from the loop so the executor, the loop
// and the HTTP layer compile against one definition.

// Generation modes. TemplateGenerationInput.Mode is the HTML/CSS/JSON mode of
// the content; GenerationMode is this.
const (
	TemplateGenerationModeHarness = "harness"
	TemplateGenerationModeClassic = "classic"
)

// Stop reasons of a run.
const (
	TemplateStopDone           = "done"
	TemplateStopTurnCap        = "turn_cap"
	TemplateStopFinishNow      = "finish_now"
	TemplateStopDeadline       = "deadline"
	TemplateStopBudget         = "budget"
	TemplateStopLengthRepeated = "length_repeated"
	TemplateStopNoToolCall     = "no_tool_call"
	TemplateStopProviderError  = "provider_error"
	// TemplateStopPrivacyGuard ends a run whose next request carried an Entity
	// value; the request was not sent.
	TemplateStopPrivacyGuard = "privacy_guard"
)

// ErrTemplateRunAccessChanged ends a run whose account was disabled, demoted or
// newly scoped since it started. No Draft or transcript is released.
var ErrTemplateRunAccessChanged = errors.New("your access changed during the run, so it was stopped")

// TemplateRunEnv is what a harness run needs from the request. The handler
// builds it; TemplateGenerationInput carries it as Run (nil for classic).
type TemplateRunEnv struct {
	Executor TemplateToolExecutor
	Sink     TemplateRunSink    // nil: no progress events
	Finish   func() bool        // Finish now requested
	Account  TemplateRunAccount // label shown, identity hashed
	OptIns   TemplateRunOptIns
}

// TemplateRunAccount identifies who the run reads for. Label is shown in the
// Disclosure; the remaining fields are hashed and never shown.
type TemplateRunAccount struct {
	Label        string
	UserID       uint
	Role         string
	ScopeGroupID *uint
	SuperUser    bool
}

// TemplateRunOptIns are the two per-run opt-ins, both off by default.
type TemplateRunOptIns struct {
	PreviewMRQL    bool
	PreviewPlugins bool
}

// TemplateToolExecutor runs Tools against the live database as the account.
type TemplateToolExecutor interface {
	// CheckAccess re-resolves the original credential under ctx and returns
	// ErrTemplateRunAccessChanged when it is no longer valid or the account's
	// role or scope changed since the run started.
	CheckAccess(ctx context.Context) error
	Execute(ctx context.Context, call TemplateToolInvocation) (TemplateToolOutput, error)
}

// TemplateToolInvocation is one validated-by-name Tool call.
type TemplateToolInvocation struct {
	CallID    string
	Name      string
	Arguments string // exact argument text; the executor decodes and validates it
	Turn      int
	// Snapshot holds the resolved value of every field the model has committed
	// a submission for, as of the start of this Turn (plan section 3, "Turn-start
	// snapshot"). lint and preview with no content read their field here; a
	// field absent from it has no submission, so the executor uses the editor
	// value from the Seed. For a whole-template CustomCSS the resolved value is
	// the Seed's stylesheet plus the latest additions, the value Apply receives.
	// Read-only: the loop shares one map across the Turn's parallel calls.
	Snapshot map[string]string
}

// TemplateToolOutput is a Tool result as sent to the model: already capped and
// fenced. Bytes is the sent size, FullBytes the size before the cap.
type TemplateToolOutput struct {
	Text      string
	IsError   bool
	Bytes     int
	FullBytes int
	Truncated bool
}

// TemplateRunSink receives progress events. Calls from parallel Tools are
// concurrent, so an implementation must be safe for concurrent use and must
// not block. Events carry only enumerated values, IDs and counts, never free
// text (plan section 5).
type TemplateRunSink interface {
	TurnStarted(TemplateTurnEvent)
	ToolFinished(TemplateToolEvent)
	Submitted(TemplateSubmissionEvent)
	Usage(TemplateUsageEvent)
}

// TemplateTurnEvent marks the start of a Turn. FinalReason is empty for an
// ordinary Turn, else one of the stop-reason constants that made it final.
type TemplateTurnEvent struct {
	Turn        int
	MaxTurns    int
	FinalReason string
}

// TemplateToolEvent reports a finished Tool call. Name is a catalogue name (an
// unknown name is reported as "unknown"). Args holds only enumerated
// arguments (field, type, direction, kind, carrierType) and numbers as text.
type TemplateToolEvent struct {
	Turn       int
	Name       string
	Args       map[string]string
	Bytes      int
	IsError    bool
	DurationMs int64
}

// TemplateSubmissionEvent reports one committed submission of a field.
type TemplateSubmissionEvent struct {
	Turn     int
	Field    string
	Errors   int
	Warnings int
}

// TemplateUsageEvent reports the budget used so far.
type TemplateUsageEvent struct {
	Turn       int
	BudgetUsed int
	BudgetMax  int
}

// Transcript record kinds.
const (
	TemplateTranscriptSeed        = "seed"
	TemplateTranscriptAssistant   = "assistant"
	TemplateTranscriptToolResult  = "tool_result"
	TemplateTranscriptInstruction = "instruction"
)

// TemplateTranscriptRecord is one message sent or received, in the exact
// transmitted form (after cap and fence). Replay is the provider payload resent
// verbatim, shown collapsed.
type TemplateTranscriptRecord struct {
	Turn    int                      `json:"turn"`
	Kind    string                   `json:"kind"`
	Text    string                   `json:"text,omitempty"`
	Calls   []TemplateTranscriptCall `json:"calls,omitempty"`
	CallID  string                   `json:"callId,omitempty"`
	IsError bool                     `json:"isError,omitempty"`
	Bytes   int                      `json:"bytes,omitempty"`
	// Sent is true when the record's text reached the provider. Every kind is
	// sent by construction except a Tool result, which goes out with the next
	// request: results of the last Turn, or of a Turn the run then refused to
	// continue, stay false so the UI can label them "not sent to the provider".
	Sent        bool                     `json:"sent"`
	StubbedFrom int                      `json:"stubbedFrom,omitempty"`
	DurationMs  int64                    `json:"durationMs,omitempty"`
	Usage       *TemplateTranscriptUsage `json:"usage,omitempty"`
	Replay      json.RawMessage          `json:"replay,omitempty"`
	// ReplayText is Replay as a string, because encoding a RawMessage compacts
	// it and escapes HTML: the panel shows these bytes, exactly as received.
	ReplayText string `json:"replayText,omitempty"`
}

// TemplateTranscriptCall is a Tool call as the model sent it.
type TemplateTranscriptCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// TemplateTranscriptUsage is a Turn's normalized accounting; a nil field was
// not reported by the provider.
type TemplateTranscriptUsage struct {
	Prompt     *int `json:"prompt,omitempty"`
	Completion *int `json:"completion,omitempty"`
	Reasoning  *int `json:"reasoning,omitempty"`
	CacheHit   *int `json:"cacheHit,omitempty"`
	CacheMiss  *int `json:"cacheMiss,omitempty"`
}

// TemplateRunError is a run that failed after starting. It wraps the usual
// sentinel error so status mapping is unchanged and carries the transcript so
// far (empty for an access change).
type TemplateRunError struct {
	Err        error
	StopReason string
	Transcript []TemplateTranscriptRecord
	// Legend maps the run's Placeholders to their values. It travels with the
	// transcript and is dropped exactly when the transcript is.
	Legend []TemplatePrivacyLegendEntry
}

func (e *TemplateRunError) Error() string { return e.Err.Error() }
func (e *TemplateRunError) Unwrap() error { return e.Err }
