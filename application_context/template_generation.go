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
	// DefaultTemplateGenerationMaxTokens is the provider's fallback budget for
	// a request that names none. Generation always names one, from the kind's
	// profile (templateTokenBudget).
	DefaultTemplateGenerationMaxTokens = 4000
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
	APIKey  string
	Model   string
	Timeout time.Duration
	// Postgres gates dialect-specific MRQL in the prompt's reference, as
	// MRQLGenerationConfig.Postgres does for the MRQL generator.
	Postgres bool
	// Overrides are the per-kind deviations from the shipped profiles, by
	// TemplateKind*. Model and Timeout above are their fallbacks.
	Overrides map[string]TemplateCallOverride
	// RepairDisabled turns off the automatic repair round.
	RepairDisabled bool
	// ProviderSlots bounds the provider calls in flight across the process
	// (DefaultTemplateProviderSlots when 0).
	ProviderSlots int
	// Logf receives one line per provider call (log.Printf when nil).
	Logf func(format string, args ...any)
	// BundleDeadline bounds a whole-template fan-out, the plan call and every
	// pair call together (DefaultTemplateBundleDeadline when 0). Each call keeps
	// its own profile timeout inside it.
	BundleDeadline time.Duration
}

// TemplateGenerationInput is the pure, DB-free grounding assembled by the
// handler. The generator never touches the database or plugin manager.
type TemplateGenerationInput struct {
	Target         string // slot | metaschema | bundle
	Mode           string // html | css | json
	Slot           string // CustomHeader ... CustomMRQLResult / CustomCSS; "" for metaschema/bundle
	EntityType     string // group | resource | note
	CurrentContent string // current editor content of the slot being refined
	// CategoryName and CategoryDescription identify the category, unsaved
	// values first.
	CategoryName        string
	CategoryDescription string
	// SectionConfig is the category's (possibly unsaved) Section Config JSON.
	SectionConfig string
	// SiblingSlots holds the category's other slot values by field name, the
	// saved ones overlaid with the client's unsaved ones.
	SiblingSlots map[string]string
	MetaSchema   string // the (possibly unsaved) MetaSchema being authored
	// Samples are up to five projected members of the category, the
	// client's chosen entity first; none on the create form.
	Samples []TemplateSample
	// SamplesArePreview marks Samples as the one preview entity the user chose
	// on the create form, which has no category members to sample yet.
	SamplesArePreview bool
	DocsBlock         string // pre-serialized built-in + plugin shortcode docs
	// PluginContext describes the enabled plugins and their note block types.
	// It is separate from DocsBlock because blocks are selected in the note
	// editor, not invoked as shortcodes in template markup.
	PluginContext string
	// Partials summarises the partial catalogue, at most
	// maxTemplatePartialsForPrompt of it; PartialCount is how many partials
	// exist, so the prompt can say when the list is incomplete.
	Partials     []TemplatePartialSummary
	PartialCount int
	BundleSlots  []string // target=bundle: which slot fields to fill
	// SlotsSelected is true when the user ticked the sections in BundleSlots, so
	// a fan-out must fill every one of them; false lets the planner choose.
	SlotsSelected bool
	// DisplayTypes are the enabled plugins' x-display values, taught to the
	// MetaSchema call and known to its lint. DisplayTypesTruncated is true
	// when the list was cut to fit the prompt, so the lint does not report a
	// plugin value that may exist beyond the cut.
	DisplayTypes          []string
	DisplayTypesTruncated bool

	// Postgres chooses the dialect the MRQL reference teaches (regex is
	// PostgreSQL-only). The generator sets it from
	// TemplateGenerationConfig.Postgres, so a caller cannot change it.
	Postgres bool

	// Validation seams (keep the generator plugin-agnostic and DB-free).
	Known        shortcodes.KnownShortcodes
	ValidateMRQL func(query string, opts shortcodes.MRQLCheckOptions) error
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
	// Repair reports the automatic repair round, when the first draft had
	// errors a repair could address.
	Repair *TemplateRepair `json:"repair,omitempty"`
	// FailedSlots names the HTML sections of a fan-out whose pair could not be
	// generated; they are absent from Slots and each has a warning in Issues.
	FailedSlots []string `json:"failedSlots,omitempty"`
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
	// Endpoint is the URL GenerateDraft posts to. The disclosure hashes it,
	// taken from the provider itself so the two cannot diverge.
	Endpoint() string
}

type defaultTemplateGenerator struct {
	provider TemplateDraftProvider
	config   TemplateGenerationConfig
	// slots is the process-wide provider semaphore (decision 4 of the plan). It
	// is created with the generator, which the context copies share.
	slots chan struct{}
	now   func() time.Time
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
	if cfg.ProviderSlots <= 0 {
		cfg.ProviderSlots = DefaultTemplateProviderSlots
	}
	return &defaultTemplateGenerator{provider: provider, config: cfg, slots: make(chan struct{}, cfg.ProviderSlots), now: time.Now}
}

func (g *defaultTemplateGenerator) DiscloseTemplate(in TemplateGenerationInput, prompt string) (*TemplateDisclosure, error) {
	request, err := g.disclose(in, prompt)
	if err != nil {
		return nil, err
	}
	return request.disclosure, nil
}

// templateRequest is a validated request with everything it can send assembled.
type templateRequest struct {
	in         TemplateGenerationInput
	prompt     string
	disclosure *TemplateDisclosure
	// fan is set when the request is a planner fan-out.
	fan *templateFanOut
}

// disclose validates and normalizes the request, then assembles every call it
// can make. GenerateTemplate sends what this returns, so the hash the user
// approved covers the messages that go out.
func (g *defaultTemplateGenerator) disclose(in TemplateGenerationInput, prompt string) (templateRequest, error) {
	if strings.TrimSpace(g.config.APIKey) == "" || g.provider == nil {
		return templateRequest{}, ErrTemplateGenerationNotConfigured
	}
	prompt = strings.TrimSpace(prompt)
	if prompt == "" || len(prompt) > MaxTemplateGenerationPromptLength {
		return templateRequest{}, fmt.Errorf("%w: prompt must be between 1 and %d characters", ErrTemplateGenerationBadRequest, MaxTemplateGenerationPromptLength)
	}

	if in.Target == "" {
		in.Target = TemplateTargetSlot
	}
	in.Postgres = g.config.Postgres

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

	if fan, err := templateFanOutFor(in); err != nil {
		return templateRequest{}, err
	} else if fan != nil {
		return g.discloseFanOut(in, prompt, fan), nil
	}

	sections := buildTemplatePromptSections(in, prompt)
	kind := templateCallKind(in)
	profile := g.config.profileFor(kind)
	maxCalls := 1
	if !g.config.RepairDisabled {
		// The possible repair call is part of what the user approves.
		maxCalls = 2
		sections = append(sections, templateRepairSections()...)
	}
	disclosure := g.newDisclosure([]TemplateDisclosedProfile{{
		Kind:      kind,
		Model:     profile.Model,
		Thinking:  profile.Thinking,
		MaxTokens: profile.MaxTokens,
		MaxCalls:  maxCalls,
	}}, sections)
	return templateRequest{in: in, prompt: prompt, disclosure: disclosure}, nil
}

func (g *defaultTemplateGenerator) GenerateTemplate(ctx context.Context, in TemplateGenerationInput, prompt, groundingHash string) (*TemplateGenerationResult, error) {
	request, err := g.disclose(in, prompt)
	if err != nil {
		return nil, err
	}
	in, disclosure := request.in, request.disclosure
	if groundingHash == "" {
		return nil, ErrTemplateGenerationGroundingRequired
	}
	if groundingHash != disclosure.GroundingHash {
		return nil, ErrTemplateGenerationGroundingChanged
	}
	if request.fan != nil {
		return g.generateFanOut(ctx, request)
	}

	disclosed := disclosure.Destination.Profiles[0]
	profile := g.config.profileFor(disclosed.Kind)
	started := g.now()
	first := TemplateDraftRequest{
		System:    promptMessage(disclosure.Sections, 0, promptRoleSystem),
		User:      promptMessage(disclosure.Sections, 0, promptRoleUser),
		Model:     disclosed.Model,
		Thinking:  disclosed.Thinking,
		MaxTokens: disclosed.MaxTokens,
	}
	resp, err := g.callProvider(ctx, profile, first, "draft", in)
	if err != nil {
		return nil, err
	}
	draft, err := parseTemplateDraft(in, resp)
	if err != nil {
		return nil, err
	}
	draft, repair := g.repair(ctx, in, profile, first, resp, draft, promptMessage(disclosure.Sections, 1, promptRoleUser), started)
	result, err := finalizeDraft(draft)
	if err != nil {
		if repair != nil {
			err = &templateRepairError{err: err, repair: repair}
		}
		return nil, err
	}
	result.Repair = repair
	return result, nil
}

// callProvider makes one provider call under its profile's timeout and the
// process-wide slot semaphore. The deadline covers the wait for a slot, so a
// call that cannot get one in time is a timeout.
func (g *defaultTemplateGenerator) callProvider(ctx context.Context, profile TemplateCallProfile, req TemplateDraftRequest, phase string, in TemplateGenerationInput) (TemplateDraftResponse, error) {
	callCtx, cancel := context.WithTimeout(ctx, profile.Timeout)
	defer cancel()
	select {
	case g.slots <- struct{}{}:
		defer func() { <-g.slots }()
	case <-callCtx.Done():
		return TemplateDraftResponse{}, templateCallError(callCtx, callCtx.Err())
	}
	// A slot and an expired deadline can both be ready; the deadline wins, so a
	// call past it is never made.
	if err := callCtx.Err(); err != nil {
		return TemplateDraftResponse{}, templateCallError(callCtx, err)
	}
	start := time.Now()
	resp, err := g.provider.GenerateDraft(callCtx, req)
	g.logCall(phase, profile, in, resp, err, time.Since(start))
	if tally, ok := ctx.Value(templateUsageTallyKey{}).(*templateUsageTally); ok && err == nil {
		tally.add(resp.Usage)
	}
	if err != nil {
		return resp, templateCallError(callCtx, err)
	}
	return resp, nil
}

// templateCallError sorts a failed call into the generator's two errors.
func templateCallError(callCtx context.Context, err error) error {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(callCtx.Err(), context.DeadlineExceeded) {
		return ErrTemplateGenerationTimeout
	}
	return fmt.Errorf("%w: %v", ErrTemplateGenerationProvider, err)
}

// logCall writes the one line per provider call. Usage stays out of the HTTP
// response; this line is where cache hits are observed.
func (g *defaultTemplateGenerator) logCall(phase string, profile TemplateCallProfile, in TemplateGenerationInput, resp TemplateDraftResponse, err error, dur time.Duration) {
	kind := profile.Kind
	if kind == "" {
		kind = templateCallKind(in)
	}
	finish := resp.FinishReason
	if err != nil {
		finish = "error"
	} else if finish == "" {
		finish = "none"
	}
	u := resp.Usage
	g.config.Logf("template generation: phase=%s kind=%s carrier=%s prompt=%d cache_hit=%d cache_miss=%d completion=%d reasoning=%d finish=%s dur=%dms",
		phase, kind, in.EntityType, u.Prompt, u.CacheHit, u.CacheMiss, u.Completion, u.Reasoning, finish, dur.Milliseconds())
}

// parseTemplateDraft parses a model reply into a draft and lints it. It stops
// short of the decision to keep the draft: a repair may replace it first.
func parseTemplateDraft(in TemplateGenerationInput, resp TemplateDraftResponse) (templateDraft, error) {
	if in.Target == TemplateTargetBundle {
		return parseBundleDraft(in, resp)
	}
	return parseSingleDraft(in, in.Target, resp)
}

// parseSingleDraft parses a {content, explanation} draft for a slot or
// metaschema target and validates it. Malformed JSON is a provider error
// (mirrors MRQL); a syntactically-valid but semantically-invalid draft is
// returned with valid:false so the user can review and apply anyway. A reply
// cut off at max_tokens is never an error: whatever arrived is returned with
// valid:false and the truncation issue.
func parseSingleDraft(in TemplateGenerationInput, target string, resp TemplateDraftResponse) (templateDraft, error) {
	truncated := resp.FinishReason == "length"
	var draft struct {
		Content     string `json:"content"`
		Explanation string `json:"explanation"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(resp.Content)), &draft); err != nil {
		if truncated {
			return templateDraft{result: truncatedTemplateResult(target), truncated: true}, nil
		}
		return templateDraft{}, fmt.Errorf("%w: provider returned invalid JSON", ErrTemplateGenerationProvider)
	}
	content := strings.TrimSpace(draft.Content)
	// Nothing to review is the one shape that stays a provider error. Size and
	// truncation are judged before the explanation, which only describes the
	// draft: a draft without one is still reviewable.
	if !truncated && content == "" {
		return templateDraft{}, fmt.Errorf("%w: provider returned an invalid draft shape", ErrTemplateGenerationProvider)
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
	return templateDraft{parsed: true, truncated: truncated, result: &TemplateGenerationResult{
		Target:      target,
		Content:     content,
		Explanation: explanation,
		Valid:       !hasErrorIssue(issues),
		Issues:      issues,
	}}, nil
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

// parseBundleDraft parses a {slots, explanation} draft. A reply cut off at
// max_tokens degrades to a reviewable invalid result with the truncation
// issue; one that is malformed although the model stopped on its own degrades
// with a message telling the user to generate slots individually. Neither has
// a draft to repair.
func parseBundleDraft(in TemplateGenerationInput, resp TemplateDraftResponse) (templateDraft, error) {
	truncated := resp.FinishReason == "length"
	var draft struct {
		Slots       map[string]string `json:"slots"`
		Explanation string            `json:"explanation"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(resp.Content)), &draft); err != nil {
		if truncated {
			return templateDraft{result: truncatedTemplateResult(TemplateTargetBundle), truncated: true}, nil
		}
		return templateDraft{result: &TemplateGenerationResult{
			Target: TemplateTargetBundle,
			Valid:  false,
			Issues: []shortcodes.LintIssue{{
				Severity: shortcodes.SeverityError,
				Message:  "The whole-template response was too large or malformed. Try generating slots individually.",
			}},
		}}, nil
	}
	if len(draft.Slots) == 0 {
		if truncated {
			return templateDraft{result: truncatedTemplateResult(TemplateTargetBundle), truncated: true}, nil
		}
		return templateDraft{}, fmt.Errorf("%w: provider returned no slots", ErrTemplateGenerationProvider)
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
	// returned half a pair broke the contract: that is a pair error, which a
	// repair may fix and which is a provider error if it stays. A reply cut off
	// part way through a pair merely ran out, so the half that arrived is kept
	// for review with an issue naming the missing companion, and the client
	// refuses to apply it.
	var pairErrors []templatePairError
	halfPair := map[string]string{}
	for _, slotName := range allowed {
		if strings.HasSuffix(slotName, "CSS") || !requested[slotName+"CSS"] {
			continue
		}
		_, htmlPresent := draft.Slots[slotName]
		_, cssPresent := draft.Slots[slotName+"CSS"]
		if htmlPresent != cssPresent {
			if !truncated {
				if htmlPresent {
					pairErrors = append(pairErrors, templatePairError{Present: slotName, Missing: slotName + "CSS"})
				} else {
					pairErrors = append(pairErrors, templatePairError{Present: slotName + "CSS", Missing: slotName})
				}
				continue
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
		return templateDraft{}, fmt.Errorf("%w: provider returned no recognized slots", ErrTemplateGenerationProvider)
	}
	result.Valid = !hasErrorIssue(result.Issues)
	return templateDraft{result: result, pairErrors: pairErrors, truncated: truncated, parsed: true}, nil
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
		return validateMetaSchemaJSON(content, shortcodes.MetaSchemaLintOptions{
			DisplayTypes:          in.DisplayTypes,
			DisplayTypesTruncated: in.DisplayTypesTruncated,
		})
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

// validateMetaSchemaJSON reports a draft that is not JSON or does not compile
// as the only issue, and otherwise the app-specific lint of a schema that
// compiles.
func validateMetaSchemaJSON(content string, opts shortcodes.MetaSchemaLintOptions) []shortcodes.LintIssue {
	var probe any
	if err := json.Unmarshal([]byte(content), &probe); err != nil {
		return []shortcodes.LintIssue{{Severity: shortcodes.SeverityError, Message: "Generated MetaSchema is not valid JSON: " + err.Error()}}
	}
	if err := compileGeneratedSchema(content); err != nil {
		return []shortcodes.LintIssue{{Severity: shortcodes.SeverityError, Message: "Generated MetaSchema is not a valid JSON Schema: " + err.Error()}}
	}
	if issues := shortcodes.LintMetaSchema(content, opts); issues != nil {
		return issues
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
