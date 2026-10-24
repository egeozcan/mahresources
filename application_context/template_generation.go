package application_context

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"mahresources/shortcodes"
)

const (
	// MaxTemplateGenerationPromptLength caps the user's natural-language request
	// (not the embedded documentation, which is guidance).
	MaxTemplateGenerationPromptLength = 2000
	// MaxTemplateGeneratedContentLength caps a single generated slot / metaschema.
	MaxTemplateGeneratedContentLength = 20000
	// MaxTemplateGeneratedExplanationLength caps the human explanation.
	MaxTemplateGeneratedExplanationLength = 1000
	// DefaultTemplateGenerationMaxTokens bounds a single-slot / metaschema call.
	DefaultTemplateGenerationMaxTokens = 1600
	// DefaultTemplateBundleMaxTokens bounds a whole-template call (several slots
	// in one response).
	DefaultTemplateBundleMaxTokens = 4000
)

// Generation targets.
const (
	TemplateTargetSlot       = "slot"
	TemplateTargetMetaSchema = "metaschema"
	TemplateTargetBundle     = "bundle"
)

var (
	ErrTemplateGenerationNotConfigured = errors.New("template generation is not configured")
	ErrTemplateGenerationBadRequest    = errors.New("bad template generation request")
	ErrTemplateGenerationProvider      = errors.New("template generation provider error")
	ErrTemplateGenerationTimeout       = errors.New("template generation provider timeout")
	// ErrTemplateGenerationGroundingRequired is a real request without the
	// groundingHash of the disclosure the user approved.
	ErrTemplateGenerationGroundingRequired = errors.New("groundingHash is required: request a dry run and review what will be sent first")
	// ErrTemplateGenerationGroundingChanged is a groundingHash that no longer
	// matches what the request would send.
	ErrTemplateGenerationGroundingChanged = errors.New("the data to be sent changed; review it again")
)

type TemplateGenerationConfig struct {
	APIKey string
	Model  string
	// ProviderURL is where the provider sends requests; its host is disclosed
	// and hashed (DefaultDeepSeekChatCompletionsURL when empty).
	ProviderURL string
	Timeout     time.Duration
	// Logf receives one line per provider call (log.Printf when nil).
	Logf func(format string, args ...any)
}

// TemplateGenerationInput is the pure, DB-free grounding assembled by the
// handler. The generator never touches the database or plugin manager.
type TemplateGenerationInput struct {
	Target         string // slot | metaschema | bundle
	Mode           string // html | css | json
	Slot           string // CustomHeader ... CustomMRQLResult / CustomCSS; "" for metaschema/bundle
	EntityType     string // group | resource | note
	CurrentContent string // current editor content of the slot being refined
	MetaSchema     string // the (possibly unsaved) MetaSchema being authored
	SampleMeta     string // an example entity's Meta JSON, or "" (schema-only)
	DocsBlock      string // pre-serialized built-in + plugin shortcode docs
	// PluginContext describes the enabled plugins and their note block types.
	// It is separate from DocsBlock because blocks are selected in the note
	// editor, not invoked as shortcodes in template markup.
	PluginContext string
	PartialNames  []string
	BundleSlots   []string // target=bundle: which slot fields to fill

	// Validation seams (keep the generator plugin-agnostic and DB-free).
	Known        shortcodes.KnownShortcodes
	ValidateMRQL func(query string) error
}

type TemplateGenerationResult struct {
	Target      string                 `json:"target"`
	Content     string                 `json:"content,omitempty"` // slot | metaschema
	Slots       map[string]string      `json:"slots,omitempty"`   // bundle
	Explanation string                 `json:"explanation"`
	Valid       bool                   `json:"valid"`
	Issues      []shortcodes.LintIssue `json:"issues,omitempty"`
	// SlotIssues groups a bundle's slot issues by slot, for the review panel.
	// Issues still lists every issue, plus those about the reply as a whole
	// (truncation), which belong to no slot.
	SlotIssues map[string][]shortcodes.LintIssue `json:"slotIssues,omitempty"`
}

// templateTruncationMessage is the issue for a reply cut off at max_tokens.
// internal/templateeval classifies it by this text.
const templateTruncationMessage = "The model's reply was cut off at its output limit, so this draft is incomplete. Try a shorter request, or generate fewer slots at once."

func templateTruncationIssue() shortcodes.LintIssue {
	return shortcodes.LintIssue{Severity: shortcodes.SeverityError, Message: templateTruncationMessage}
}

// templateOversizeIssue reports content over MaxTemplateGeneratedContentLength.
// It is returned whole and marked invalid, never sliced: a cut draft looks
// complete. internal/templateeval classifies the issue by its "Output exceeded"
// prefix.
func templateOversizeIssue(field string, size int) shortcodes.LintIssue {
	return shortcodes.LintIssue{
		Severity: shortcodes.SeverityError,
		Message:  fmt.Sprintf("Output exceeded %d bytes in %s (%d bytes). Nothing was cut; shorten it before saving.", MaxTemplateGeneratedContentLength, field, size),
	}
}

// TemplateGenerator drafts templates in two requests (decision 11 of
// docs/plans/2026-10-24-template-generation-quality.md): DiscloseTemplate
// assembles everything a request would send and calls no provider;
// GenerateTemplate rebuilds it, refuses unless its groundingHash matches the
// one the user approved, and sends exactly those messages.
type TemplateGenerator interface {
	DiscloseTemplate(in TemplateGenerationInput, prompt string) (*TemplateDisclosure, error)
	GenerateTemplate(ctx context.Context, in TemplateGenerationInput, prompt, groundingHash string) (*TemplateGenerationResult, error)
}

// TemplateDraftRequest is one provider call.
type TemplateDraftRequest struct {
	System, User string
	Model        string
	Thinking     bool
	MaxTokens    int
}

// TemplateTokenUsage is the provider's token accounting for one call.
type TemplateTokenUsage struct {
	Prompt, Completion, Reasoning, CacheHit, CacheMiss int
}

// TemplateDraftResponse is one provider reply. Content is the raw JSON string
// and may be empty when FinishReason is "length".
type TemplateDraftResponse struct {
	Content, FinishReason string
	Usage                 TemplateTokenUsage
}

// TemplateDraftProvider returns the raw JSON content string from the model; the
// generator unmarshals it per target (templates have three output shapes, so —
// unlike the MRQL provider — the provider does not decode a fixed struct).
type TemplateDraftProvider interface {
	GenerateDraft(ctx context.Context, req TemplateDraftRequest) (TemplateDraftResponse, error)
}

type defaultTemplateGenerator struct {
	provider TemplateDraftProvider
	config   TemplateGenerationConfig
}

func NewTemplateGenerator(provider TemplateDraftProvider, cfg TemplateGenerationConfig) TemplateGenerator {
	if cfg.Model == "" {
		cfg.Model = DefaultDeepSeekMRQLGenerationModel
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = DefaultDeepSeekMRQLGenerationTimeout
	}
	if cfg.Logf == nil {
		cfg.Logf = log.Printf
	}
	if cfg.ProviderURL == "" {
		cfg.ProviderURL = DefaultDeepSeekChatCompletionsURL
	}
	return &defaultTemplateGenerator{provider: provider, config: cfg}
}

func (g *defaultTemplateGenerator) DiscloseTemplate(in TemplateGenerationInput, prompt string) (*TemplateDisclosure, error) {
	_, _, disclosure, err := g.disclose(in, prompt)
	return disclosure, err
}

// disclose validates and normalizes the request, then assembles every call it
// can make. GenerateTemplate sends what this returns, so the hash the user
// approved covers the messages that go out.
func (g *defaultTemplateGenerator) disclose(in TemplateGenerationInput, prompt string) (TemplateGenerationInput, string, *TemplateDisclosure, error) {
	if strings.TrimSpace(g.config.APIKey) == "" || g.provider == nil {
		return in, "", nil, ErrTemplateGenerationNotConfigured
	}
	prompt = strings.TrimSpace(prompt)
	if prompt == "" || len(prompt) > MaxTemplateGenerationPromptLength {
		return in, "", nil, fmt.Errorf("%w: prompt must be between 1 and %d characters", ErrTemplateGenerationBadRequest, MaxTemplateGenerationPromptLength)
	}

	if in.Target == "" {
		in.Target = TemplateTargetSlot
	}

	// Settle "is this slot CSS?" once, here, so the prompt and the linter cannot
	// answer it differently. The handler validates Slot and does not validate
	// Mode, so a request can arrive contradicting itself; left alone,
	// slot=CustomCSS with mode=html asked the model for HTML and then judged the
	// answer as CSS. in is a value copy, so this rewrite is local to the call.
	//
	// A slot draft is one of exactly two things, so the mode is rewritten to
	// whichever singleSlotIsCSS says rather than passed through. That also
	// canonicalises a mixed-case "CSS", which that predicate accepts and
	// modeRuleLine's exact-match switch does not, and refuses "json", which
	// belongs to the metaschema target and would otherwise have asked for JSON
	// and linted the reply as markup. Other targets keep their own mode.
	if in.Target == TemplateTargetSlot {
		if singleSlotIsCSS(in) {
			in.Mode = "css"
		} else {
			in.Mode = "html"
		}
	}

	sections, maxTokens := buildTemplatePromptSections(in, prompt)
	disclosure := newTemplateDisclosure(g.config.ProviderURL, []TemplateDisclosedProfile{{
		Kind:      templateCallKind(in),
		Model:     g.config.Model,
		MaxTokens: maxTokens,
		MaxCalls:  1,
	}}, sections)
	return in, prompt, disclosure, nil
}

func (g *defaultTemplateGenerator) GenerateTemplate(ctx context.Context, in TemplateGenerationInput, prompt, groundingHash string) (*TemplateGenerationResult, error) {
	in, prompt, disclosure, err := g.disclose(in, prompt)
	if err != nil {
		return nil, err
	}
	if groundingHash == "" {
		return nil, ErrTemplateGenerationGroundingRequired
	}
	if groundingHash != disclosure.GroundingHash {
		return nil, ErrTemplateGenerationGroundingChanged
	}

	callCtx, cancel := context.WithTimeout(ctx, g.config.Timeout)
	defer cancel()

	profile := disclosure.Destination.Profiles[0]
	start := time.Now()
	resp, err := g.provider.GenerateDraft(callCtx, TemplateDraftRequest{
		System:    promptMessage(disclosure.Sections, 0, promptRoleSystem),
		User:      promptMessage(disclosure.Sections, 0, promptRoleUser),
		Model:     profile.Model,
		Thinking:  profile.Thinking,
		MaxTokens: profile.MaxTokens,
	})
	g.logCall("draft", in, resp, err, time.Since(start))
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(callCtx.Err(), context.DeadlineExceeded) {
			return nil, ErrTemplateGenerationTimeout
		}
		return nil, fmt.Errorf("%w: %v", ErrTemplateGenerationProvider, err)
	}

	if in.Target == TemplateTargetBundle {
		return finishBundle(in, resp)
	}
	return finishSingle(in, in.Target, resp)
}

// logCall writes the one line per provider call. Usage stays out of the HTTP
// response; this line is where cache hits are observed.
func (g *defaultTemplateGenerator) logCall(phase string, in TemplateGenerationInput, resp TemplateDraftResponse, err error, dur time.Duration) {
	finish := resp.FinishReason
	if err != nil {
		finish = "error"
	} else if finish == "" {
		finish = "none"
	}
	u := resp.Usage
	g.config.Logf("template generation: phase=%s kind=%s carrier=%s prompt=%d cache_hit=%d cache_miss=%d completion=%d reasoning=%d finish=%s dur=%dms",
		phase, templateCallKind(in), in.EntityType, u.Prompt, u.CacheHit, u.CacheMiss, u.Completion, u.Reasoning, finish, dur.Milliseconds())
}

// templateCallKind names the kind of call a request makes: CustomCSS is a slot,
// an HTML slot with its companion a pair, and any other bundle a bundle.
func templateCallKind(in TemplateGenerationInput) string {
	switch in.Target {
	case TemplateTargetMetaSchema:
		return "metaschema"
	case TemplateTargetBundle:
		if len(in.BundleSlots) == 2 && in.BundleSlots[0]+"CSS" == in.BundleSlots[1] {
			return "pair"
		}
		return "bundle"
	default:
		return "slot"
	}
}

// finishSingle parses a {content, explanation} draft for a slot or metaschema
// target and validates it. Malformed JSON is a provider error (mirrors MRQL);
// a syntactically-valid but semantically-invalid draft is returned with
// valid:false so the user can review and apply anyway. A reply cut off at
// max_tokens is never an error: whatever arrived is returned with valid:false
// and the truncation issue.
func finishSingle(in TemplateGenerationInput, target string, resp TemplateDraftResponse) (*TemplateGenerationResult, error) {
	truncated := resp.FinishReason == "length"
	var draft struct {
		Content     string `json:"content"`
		Explanation string `json:"explanation"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(resp.Content)), &draft); err != nil {
		if truncated {
			return truncatedTemplateResult(target), nil
		}
		return nil, fmt.Errorf("%w: provider returned invalid JSON", ErrTemplateGenerationProvider)
	}
	content := strings.TrimSpace(draft.Content)
	// Nothing to review is the one shape that stays a provider error. Size and
	// truncation are judged before the explanation, which only describes the
	// draft: a draft without one is still reviewable.
	if !truncated && content == "" {
		return nil, fmt.Errorf("%w: provider returned an invalid draft shape", ErrTemplateGenerationProvider)
	}

	issues := []shortcodes.LintIssue{}
	if truncated {
		issues = append(issues, templateTruncationIssue())
	}
	if len(content) > MaxTemplateGeneratedContentLength {
		field := in.Slot
		if target == TemplateTargetMetaSchema {
			field = "MetaSchema"
		}
		issues = append(issues, templateOversizeIssue(field, len(content)))
	}
	explanation, explanationIssues := templateExplanation(draft.Explanation, truncated)
	issues = append(issues, explanationIssues...)
	if content != "" {
		issues = append(issues, validateTemplateContent(target, in, content, singleSlotIsCSS(in))...)
	}
	return &TemplateGenerationResult{
		Target:      target,
		Content:     content,
		Explanation: explanation,
		Valid:       !hasErrorIssue(issues),
		Issues:      issues,
	}, nil
}

// templateExplanation trims and clips the model's explanation. A missing or
// overlong one is a warning, unless the reply was cut off, which already says
// why the explanation may be missing.
func templateExplanation(raw string, truncated bool) (string, []shortcodes.LintIssue) {
	explanation := strings.TrimSpace(raw)
	var issues []shortcodes.LintIssue
	switch {
	case len(explanation) > MaxTemplateGeneratedExplanationLength:
		explanation = explanation[:MaxTemplateGeneratedExplanationLength]
		if !truncated {
			issues = append(issues, shortcodes.LintIssue{Severity: shortcodes.SeverityWarning, Message: fmt.Sprintf("The model's explanation was shortened to %d bytes.", MaxTemplateGeneratedExplanationLength)})
		}
	case explanation == "" && !truncated:
		issues = append(issues, shortcodes.LintIssue{Severity: shortcodes.SeverityWarning, Message: "The model gave no explanation for this draft."})
	}
	return explanation, issues
}

// truncatedTemplateResult is the result for a reply cut off before anything
// usable arrived: empty, or JSON that stops mid-value.
func truncatedTemplateResult(target string) *TemplateGenerationResult {
	return &TemplateGenerationResult{
		Target: target,
		Valid:  false,
		Issues: []shortcodes.LintIssue{templateTruncationIssue()},
	}
}

// finishBundle parses a {slots, explanation} draft. A reply cut off at
// max_tokens degrades to a reviewable invalid result with the truncation
// issue; one that is malformed although the model stopped on its own degrades
// with a message telling the user to generate slots individually.
func finishBundle(in TemplateGenerationInput, resp TemplateDraftResponse) (*TemplateGenerationResult, error) {
	truncated := resp.FinishReason == "length"
	var draft struct {
		Slots       map[string]string `json:"slots"`
		Explanation string            `json:"explanation"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(resp.Content)), &draft); err != nil {
		if truncated {
			return truncatedTemplateResult(TemplateTargetBundle), nil
		}
		return &TemplateGenerationResult{
			Target: TemplateTargetBundle,
			Valid:  false,
			Issues: []shortcodes.LintIssue{{
				Severity: shortcodes.SeverityError,
				Message:  "The whole-template response was too large or malformed. Try generating slots individually.",
			}},
		}, nil
	}
	if len(draft.Slots) == 0 {
		if truncated {
			return truncatedTemplateResult(TemplateTargetBundle), nil
		}
		return nil, fmt.Errorf("%w: provider returned no slots", ErrTemplateGenerationProvider)
	}

	explanation, _ := templateExplanation(draft.Explanation, true)

	result := &TemplateGenerationResult{
		Target:      TemplateTargetBundle,
		Slots:       map[string]string{},
		Explanation: explanation,
		Issues:      []shortcodes.LintIssue{},
		SlotIssues:  map[string][]shortcodes.LintIssue{},
	}
	if truncated {
		result.Issues = append(result.Issues, templateTruncationIssue())
	}
	// Only keep slots the handler asked for, in a stable order, and lint each.
	allowed := in.BundleSlots
	if len(allowed) == 0 {
		for k := range draft.Slots {
			allowed = append(allowed, k)
		}
	}
	requested := make(map[string]bool, len(allowed))
	for _, slotName := range allowed {
		requested[slotName] = true
	}
	// A pair is applied whole or not at all. A model that stopped on its own and
	// returned half a pair broke the contract; a reply cut off part way through
	// a pair merely ran out, so the half that arrived is kept for review with an
	// issue naming the missing companion, and the client refuses to apply it.
	halfPair := map[string]string{}
	for _, slotName := range allowed {
		if strings.HasSuffix(slotName, "CSS") || !requested[slotName+"CSS"] {
			continue
		}
		_, htmlPresent := draft.Slots[slotName]
		_, cssPresent := draft.Slots[slotName+"CSS"]
		if htmlPresent != cssPresent {
			if !truncated {
				return nil, fmt.Errorf("%w: provider must return %s and %s together", ErrTemplateGenerationProvider, slotName, slotName+"CSS")
			}
			if htmlPresent {
				halfPair[slotName] = slotName + "CSS"
			} else {
				halfPair[slotName+"CSS"] = slotName
			}
		}
	}
	for _, slotName := range allowed {
		content, ok := draft.Slots[slotName]
		if !ok {
			continue
		}
		content = strings.TrimSpace(content)
		// An explicitly empty slot clears its editor; an absent key leaves it unspecified.
		result.Slots[slotName] = content
		var issues []shortcodes.LintIssue
		if missing, half := halfPair[slotName]; half {
			issues = append(issues, shortcodes.LintIssue{
				Severity: shortcodes.SeverityError,
				Message:  fmt.Sprintf("%s came back without %s before the reply was cut off, so this pair cannot be applied.", slotName, missing),
			})
		}
		if len(content) > MaxTemplateGeneratedContentLength {
			issues = append(issues, templateOversizeIssue(slotName, len(content)))
		}
		// One input covers every slot of a bundle, so its Mode says nothing
		// about the slot in hand: CustomCSS and its companions are stylesheets.
		issues = append(issues, validateTemplateContent(TemplateTargetSlot, in, content, strings.HasSuffix(slotName, "CSS"))...)
		if len(issues) > 0 {
			result.SlotIssues[slotName] = issues
		}
		result.Issues = append(result.Issues, issues...)
	}
	if len(result.Slots) == 0 && !truncated {
		return nil, fmt.Errorf("%w: provider returned no recognized slots", ErrTemplateGenerationProvider)
	}
	result.Valid = !hasErrorIssue(result.Issues)
	return result, nil
}

// singleSlotIsCSS reports whether a slot-target draft is a stylesheet.
//
// The slot decides whenever there is one. It is the half the handler validates
// (templateGenerateSlotAllowed) and the half that actually names the document,
// so a client that names CustomCSS and mislabels or omits the mode still gets
// the placement warnings — and, more importantly, one that names a markup slot
// is not read as CSS because it mislabelled the mode. That direction is not a
// harmless false positive. At a non-attribute position — where a stylesheet
// interpolation sits — shortcodes' CSS branch *returns* in place of the markup
// checks rather than adding to them, so a markup draft judged as CSS loses the
// raw= "becomes real elements on the page" warning, which is the XSS one.
// (Inside an attribute value the CSS note is merely appended and nothing is
// lost; the non-attribute case is the one that costs something.) Mode answers
// only when no slot is named, and an unlabelled request then falls through to
// markup, the safe reading.
func singleSlotIsCSS(in TemplateGenerationInput) bool {
	if in.Slot != "" {
		return strings.HasSuffix(in.Slot, "CSS")
	}
	return strings.EqualFold(in.Mode, "css")
}

// validateTemplateContent lints slot/bundle content as shortcodes and validates
// metaschema content as JSON + JSON Schema. Returned issues are never fatal —
// they flag the draft for review.
//
// cssMode is a parameter rather than something read off in, because the caller
// is the only one who knows: a CustomCSS slot is a stylesheet with no <style>
// wrapper of its own, so nothing in the text says so, and finishBundle shares
// one input across every slot it validates.
func validateTemplateContent(target string, in TemplateGenerationInput, content string, cssMode bool) []shortcodes.LintIssue {
	if target == TemplateTargetMetaSchema {
		return validateMetaSchemaJSON(content)
	}
	issues := shortcodes.Lint(content, shortcodes.LintOptions{
		Known:        in.Known,
		ValidateMRQL: in.ValidateMRQL,
		CSSMode:      cssMode,
	})
	if issues == nil {
		return []shortcodes.LintIssue{}
	}
	return issues
}

func validateMetaSchemaJSON(content string) []shortcodes.LintIssue {
	var probe any
	if err := json.Unmarshal([]byte(content), &probe); err != nil {
		return []shortcodes.LintIssue{{Severity: shortcodes.SeverityError, Message: "Generated MetaSchema is not valid JSON: " + err.Error()}}
	}
	if err := compileGeneratedSchema(content); err != nil {
		return []shortcodes.LintIssue{{Severity: shortcodes.SeverityError, Message: "Generated MetaSchema is not a valid JSON Schema: " + err.Error()}}
	}
	return []shortcodes.LintIssue{}
}

// compileGeneratedSchema verifies the generated MetaSchema compiles as a JSON
// Schema. It shares ValidateMetaSchema with the write paths (findings 17/93) so
// a generated schema and an authored one are held to one rule.
func compileGeneratedSchema(schemaJSON string) error {
	return ValidateMetaSchema(schemaJSON)
}

func hasErrorIssue(issues []shortcodes.LintIssue) bool {
	for _, i := range issues {
		if i.Severity == shortcodes.SeverityError {
			return true
		}
	}
	return false
}
