package templateeval

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"time"

	"mahresources/application_context"
)

// Usage is the token accounting of one provider call.
type Usage struct {
	Prompt     int `json:"prompt"`
	Completion int `json:"completion"`
	Reasoning  int `json:"reasoning"`
	CacheHit   int `json:"cacheHit"`
	CacheMiss  int `json:"cacheMiss"`
}

func (u Usage) add(o Usage) Usage {
	return Usage{u.Prompt + o.Prompt, u.Completion + o.Completion, u.Reasoning + o.Reasoning, u.CacheHit + o.CacheHit, u.CacheMiss + o.CacheMiss}
}

// Generation modes a call can be recorded in. A call without a mode (a report
// older than scorer version 4) is classic.
const (
	ModeClassic = "classic"
	ModeHarness = "harness"
)

// Error classes of a recorded call. http_status, decode, size and refusal come
// from application_context.GenerationProviderError; timeout and transport are
// the two readings of its transport class.
const (
	ErrorClassTimeout    = "timeout"
	ErrorClassHTTPStatus = "http_status"
	ErrorClassTransport  = "transport"
	ErrorClassDecode     = "decode"
	ErrorClassSize       = "size"
	ErrorClassRefusal    = "refusal"
)

// UnknownToolName replaces the name of a tool call that names no Tool of its
// request: the name is model-chosen text, and the aggregate report may only
// hold fixed vocabulary.
const UnknownToolName = "unknown"

// knownFinishReasons are the finish reasons the adapters normalize to. Any
// other value is recorded as "other", so the field never carries upstream text.
var knownFinishReasons = map[string]bool{"": true, "stop": true, "length": true, "tool_calls": true, "content_filter": true, "insufficient_system_resource": true, "other": true}

// ToolCall is one tool call of a harness reply as recorded. Arguments is the
// sanitized text (SanitizeArguments); AggregateOnly drops it.
type ToolCall struct {
	Name       string `json:"name"`
	Arguments  string `json:"arguments,omitempty"`
	Incomplete bool   `json:"incomplete,omitempty"`
	Malformed  bool   `json:"malformed,omitempty"`
	// SourceCheck is the harness's source admission (byte and shortcode-token
	// limits) of a submit call's raw field values, decided before sanitizing
	// re-encoded the arguments: SourceAdmitted or SourceRejected. Empty on a
	// non-submit call, on arguments that did not decode, and on older
	// recordings, which the scorer checks from the sanitized text.
	SourceCheck string `json:"sourceCheck,omitempty"`
}

// SourceCheck values.
const (
	SourceAdmitted = "admitted"
	SourceRejected = "rejected"
)

// submitSourceCheck applies CheckTemplateToolSource to every raw string field
// value of a submit call's arguments. It returns "" when the arguments are not
// a submit object with string fields; the shape checks judge those.
func submitSourceCheck(name, arguments string) string {
	if name != application_context.ToolSubmit {
		return ""
	}
	var decoded struct {
		Fields map[string]json.RawMessage `json:"fields"`
	}
	if json.Unmarshal([]byte(arguments), &decoded) != nil || len(decoded.Fields) == 0 {
		return ""
	}
	for _, raw := range decoded.Fields {
		var text *string
		if json.Unmarshal(raw, &text) != nil || text == nil {
			continue
		}
		if application_context.CheckTemplateToolSource(*text) != nil {
			return SourceRejected
		}
	}
	return SourceAdmitted
}

// Call is what the recorder saw of one provider call at the provider-neutral
// seam. It keeps no headers, no body and no upstream error text: a failure is
// its class and, for an HTTP error, the status code. An upstream can echo the
// Authorization header back in an error body, in any encoding, so the only safe
// amount of that text to keep is none.
type Call struct {
	// Mode is classic (a TemplateDraftRequest/Response pair) or harness (a
	// ConversationRequest/Response pair).
	Mode      string `json:"mode,omitempty"`
	Model     string `json:"model"`
	Thinking  bool   `json:"thinking"`
	Plan      bool   `json:"plan,omitempty"`
	Repair    bool   `json:"repair,omitempty"`
	MaxTokens int    `json:"maxTokens"`
	// Turn is the 1-based Turn of a harness call: the number of assistant
	// messages in its request, plus one.
	Turn      int `json:"turn,omitempty"`
	ToolCount int `json:"toolCount,omitempty"`
	// Status is 200 for a call that returned a reply, the HTTP status of an HTTP
	// failure, and 0 for a failure with no status; ErrorClass is the class of a
	// failure.
	Status       int    `json:"status"`
	ErrorClass   string `json:"errorClass,omitempty"`
	FinishReason string `json:"finishReason,omitempty"`
	Usage        Usage  `json:"usage"`
	// UsageUnreported is set when the provider left out a prompt or completion
	// count, or both cache counts: Usage then holds zeros that mean "unknown".
	UsageUnreported bool  `json:"usageUnreported,omitempty"`
	LatencyMS       int64 `json:"latencyMs"`
	// Content is the model's raw reply (harness: its text beside any tool
	// calls). AggregateOnly drops it.
	Content string `json:"content,omitempty"`
	// ToolCalls and Replay are harness only. Replay is the sanitized
	// diagnostic copy of the provider's assistant payload.
	ToolCalls []ToolCall      `json:"toolCalls,omitempty"`
	Replay    json.RawMessage `json:"replay,omitempty"`
}

// Recorder records every provider call that passes through a provider it
// wrapped, at the provider-neutral seams, and hands requests and responses on
// unchanged. One Recorder can wrap a classic and a harness provider built from
// one provider configuration; their calls interleave in Take order.
type Recorder struct {
	secrets []string
	mu      sync.Mutex
	calls   []Call
}

// NewRecorder returns a recorder that redacts secrets, and any "Bearer ..."
// token, from everything it records.
func NewRecorder(secrets ...string) *Recorder {
	return &Recorder{secrets: secrets}
}

// WrapDraftProvider records classic-mode calls. The wrapper also exposes the
// optional Transport, ProviderName and ReasoningMode methods the generator's
// disclosure reads, forwarding to inner where it has them.
func (r *Recorder) WrapDraftProvider(inner application_context.TemplateDraftProvider) application_context.TemplateDraftProvider {
	return &recordingDraftProvider{inner: inner, rec: r}
}

// WrapConversationProvider records harness-mode calls, forwarding the rest of
// the provider interface unchanged.
func (r *Recorder) WrapConversationProvider(inner application_context.TemplateConversationProvider) application_context.TemplateConversationProvider {
	return &recordingConversationProvider{TemplateConversationProvider: inner, rec: r}
}

type recordingDraftProvider struct {
	inner application_context.TemplateDraftProvider
	rec   *Recorder
}

func (p *recordingDraftProvider) Endpoint() string { return p.inner.Endpoint() }

func (p *recordingDraftProvider) ProviderName() string {
	if named, ok := p.inner.(interface{ ProviderName() string }); ok {
		return named.ProviderName()
	}
	return ""
}

func (p *recordingDraftProvider) Transport() application_context.TemplateTransport {
	if t, ok := p.inner.(interface {
		Transport() application_context.TemplateTransport
	}); ok {
		return t.Transport()
	}
	return application_context.TemplateTransport{}
}

func (p *recordingDraftProvider) ReasoningMode() string {
	if m, ok := p.inner.(interface{ ReasoningMode() string }); ok {
		return m.ReasoningMode()
	}
	return ""
}

func (p *recordingDraftProvider) GenerateDraft(ctx context.Context, req application_context.TemplateDraftRequest) (application_context.TemplateDraftResponse, error) {
	call := Call{
		Mode: ModeClassic, Model: req.Model, Thinking: req.Thinking, MaxTokens: req.MaxTokens,
		Repair: strings.Contains(req.User, "\nRepair request: your previous reply could not be used as it stands."),
		Plan:   strings.Contains(req.User, "\nTask: plan the design of a whole template."),
	}
	start := time.Now()
	resp, err := p.inner.GenerateDraft(ctx, req)
	call.LatencyMS = time.Since(start).Milliseconds()
	if err != nil {
		call.Status, call.ErrorClass = classifyError(ctx, err)
		p.rec.record(call)
		return resp, err
	}
	call.Status = 200
	call.Content = resp.Content
	call.FinishReason = normalizeFinish(resp.FinishReason)
	u := resp.Usage
	call.Usage = Usage{Prompt: u.Prompt, Completion: u.Completion, Reasoning: u.Reasoning, CacheHit: u.CacheHit, CacheMiss: u.CacheMiss}
	p.rec.record(call)
	return resp, nil
}

type recordingConversationProvider struct {
	application_context.TemplateConversationProvider
	rec *Recorder
}

// ReasoningMode forwards the inner adapter's answer, so the Disclosure of a
// recorded run reads the same as an unrecorded one.
func (p *recordingConversationProvider) ReasoningMode() string {
	if m, ok := p.TemplateConversationProvider.(interface{ ReasoningMode() string }); ok {
		return m.ReasoningMode()
	}
	return ""
}

func (p *recordingConversationProvider) Converse(ctx context.Context, req application_context.ConversationRequest) (application_context.ConversationResponse, error) {
	turn := 1
	for _, m := range req.Messages {
		if m.Role == "assistant" {
			turn++
		}
	}
	call := Call{Mode: ModeHarness, Model: req.Model, Thinking: req.Thinking, MaxTokens: req.MaxTokens, Turn: turn, ToolCount: len(req.Tools)}
	start := time.Now()
	resp, err := p.TemplateConversationProvider.Converse(ctx, req)
	call.LatencyMS = time.Since(start).Milliseconds()
	if err != nil {
		call.Status, call.ErrorClass = classifyError(ctx, err)
		p.rec.record(call)
		return resp, err
	}
	call.Status = 200
	call.Content = resp.Text
	call.FinishReason = normalizeFinish(resp.Finish)
	call.Usage, call.UsageUnreported = conversationUsage(resp.Usage)
	known := make(map[string]bool, len(req.Tools))
	for _, tool := range req.Tools {
		known[tool.Name] = true
	}
	for _, c := range resp.Calls {
		name := c.Name
		if !known[name] {
			name = UnknownToolName
		}
		call.ToolCalls = append(call.ToolCalls, ToolCall{Name: name, Arguments: SanitizeArguments(c.Arguments, p.rec.secrets...), Incomplete: c.Incomplete, Malformed: c.Malformed, SourceCheck: submitSourceCheck(c.Name, c.Arguments)})
	}
	if len(resp.Replay) > 0 {
		call.Replay = SanitizeReplay(resp.Replay, p.rec.secrets...)
	}
	p.rec.record(call)
	return resp, nil
}

// conversationUsage turns the pointer-valued usage into counts. A missing cache
// side is derived from the other and the prompt total, as classic mode does;
// anything the provider left out reads as zero and sets unreported.
func conversationUsage(u application_context.ConversationUsage) (Usage, bool) {
	val := func(p *int) int {
		if p == nil {
			return 0
		}
		return *p
	}
	out := Usage{Prompt: val(u.Prompt), Completion: val(u.Completion), Reasoning: val(u.Reasoning), CacheHit: val(u.CacheHit), CacheMiss: val(u.CacheMiss)}
	switch {
	case u.CacheHit != nil && u.CacheMiss == nil:
		out.CacheMiss = max(0, out.Prompt-out.CacheHit)
	case u.CacheHit == nil && u.CacheMiss != nil:
		out.CacheHit = max(0, out.Prompt-out.CacheMiss)
	}
	return out, u.Prompt == nil || u.Completion == nil || (u.CacheHit == nil && u.CacheMiss == nil)
}

func normalizeFinish(finish string) string {
	if knownFinishReasons[finish] {
		return finish
	}
	return "other"
}

// classifyError reads the sanitized typed error the adapters return, never its
// text. An error with no type (a fake, or a failure before the adapter) is a
// timeout when a deadline caused it and a transport failure otherwise.
func classifyError(ctx context.Context, err error) (status int, class string) {
	var typed *application_context.GenerationProviderError
	if errors.As(err, &typed) {
		switch typed.Class {
		case application_context.GenerationErrorClassHTTP:
			return typed.Status, ErrorClassHTTPStatus
		case application_context.GenerationErrorClassDecode:
			return typed.Status, ErrorClassDecode
		case application_context.GenerationErrorClassSize:
			return typed.Status, ErrorClassSize
		case application_context.GenerationErrorClassRefusal:
			return typed.Status, ErrorClassRefusal
		}
	}
	var netErr net.Error
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) || (errors.As(err, &netErr) && netErr.Timeout()) {
		return 0, ErrorClassTimeout
	}
	return 0, ErrorClassTransport
}

func (r *Recorder) record(c Call) {
	// The reply stays in the full report for diagnosis; AggregateOnly drops it.
	c.Content = Redact(c.Content, r.secrets...)
	r.mu.Lock()
	r.calls = append(r.calls, c)
	r.mu.Unlock()
}

// Take returns the calls recorded since the last Take and clears them.
func (r *Recorder) Take() []Call {
	r.mu.Lock()
	defer r.mu.Unlock()
	calls := r.calls
	r.calls = nil
	return calls
}

var bearerToken = regexp.MustCompile(`(?i)\bbearer\s+[^\s"'<>]+`)

const redacted = "[redacted]"

// Redact removes every secret value and every "Bearer <token>" from s. It is
// applied to decoded strings, never to an encoding of them.
func Redact(s string, secrets ...string) string {
	for _, secret := range secrets {
		if secret != "" {
			s = strings.ReplaceAll(s, secret, redacted)
		}
	}
	return bearerToken.ReplaceAllString(s, "Bearer "+redacted)
}

// RedactReport returns a deep copy of report with Redact applied to every
// string value and map key in it. It is for the full report, which keeps
// drafts and replies and stays out of git; the committed aggregate holds no
// free text at all (AggregateOnly).
func RedactReport(report RunReport, secrets ...string) RunReport {
	return redactValue(reflect.ValueOf(report), secrets).Interface().(RunReport)
}

func redactValue(v reflect.Value, secrets []string) reflect.Value {
	switch v.Kind() {
	case reflect.String:
		return reflect.ValueOf(Redact(v.String(), secrets...)).Convert(v.Type())
	case reflect.Struct:
		out := reflect.New(v.Type()).Elem()
		for i := 0; i < v.NumField(); i++ {
			out.Field(i).Set(redactValue(v.Field(i), secrets))
		}
		return out
	case reflect.Pointer:
		if v.IsNil() {
			return v
		}
		out := reflect.New(v.Type().Elem())
		out.Elem().Set(redactValue(v.Elem(), secrets))
		return out
	case reflect.Slice:
		if v.IsNil() {
			return v
		}
		out := reflect.MakeSlice(v.Type(), v.Len(), v.Len())
		for i := 0; i < v.Len(); i++ {
			out.Index(i).Set(redactValue(v.Index(i), secrets))
		}
		return out
	case reflect.Map:
		if v.IsNil() {
			return v
		}
		out := reflect.MakeMapWithSize(v.Type(), v.Len())
		iter := v.MapRange()
		for iter.Next() {
			out.SetMapIndex(redactValue(iter.Key(), secrets), redactValue(iter.Value(), secrets))
		}
		return out
	}
	return v
}
