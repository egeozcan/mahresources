package application_context

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"mahresources/shortcodes"

	"github.com/santhosh-tekuri/jsonschema/v6"
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
)

type TemplateGenerationConfig struct {
	APIKey  string
	Model   string
	Timeout time.Duration
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
	PartialNames   []string
	BundleSlots    []string // target=bundle: which slot fields to fill

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
}

type TemplateGenerator interface {
	GenerateTemplate(ctx context.Context, in TemplateGenerationInput, prompt string) (*TemplateGenerationResult, error)
}

// TemplateDraftProvider returns the raw JSON content string from the model; the
// generator unmarshals it per target (templates have three output shapes, so —
// unlike the MRQL provider — the provider does not decode a fixed struct).
type TemplateDraftProvider interface {
	GenerateDraft(ctx context.Context, systemPrompt, userPrompt string, maxTokens int) (string, error)
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
	return &defaultTemplateGenerator{provider: provider, config: cfg}
}

func (g *defaultTemplateGenerator) GenerateTemplate(ctx context.Context, in TemplateGenerationInput, prompt string) (*TemplateGenerationResult, error) {
	if strings.TrimSpace(g.config.APIKey) == "" || g.provider == nil {
		return nil, ErrTemplateGenerationNotConfigured
	}
	prompt = strings.TrimSpace(prompt)
	if prompt == "" || len(prompt) > MaxTemplateGenerationPromptLength {
		return nil, fmt.Errorf("%w: prompt must be between 1 and %d characters", ErrTemplateGenerationBadRequest, MaxTemplateGenerationPromptLength)
	}

	target := in.Target
	if target == "" {
		target = TemplateTargetSlot
		in.Target = target
	}

	callCtx, cancel := context.WithTimeout(ctx, g.config.Timeout)
	defer cancel()

	systemPrompt, userMessage, maxTokens := buildTemplateGenerationPrompt(in, prompt)
	raw, err := g.provider.GenerateDraft(callCtx, systemPrompt, userMessage, maxTokens)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(callCtx.Err(), context.DeadlineExceeded) {
			return nil, ErrTemplateGenerationTimeout
		}
		return nil, fmt.Errorf("%w: %v", ErrTemplateGenerationProvider, err)
	}

	if target == TemplateTargetBundle {
		return finishBundle(in, raw)
	}
	return finishSingle(in, target, raw)
}

// finishSingle parses a {content, explanation} draft for a slot or metaschema
// target and validates it. Malformed JSON is a provider error (mirrors MRQL);
// a syntactically-valid but semantically-invalid draft is returned with
// valid:false so the user can review and apply anyway.
func finishSingle(in TemplateGenerationInput, target, raw string) (*TemplateGenerationResult, error) {
	var draft struct {
		Content     string `json:"content"`
		Explanation string `json:"explanation"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &draft); err != nil {
		return nil, fmt.Errorf("%w: provider returned invalid JSON", ErrTemplateGenerationProvider)
	}
	content := strings.TrimSpace(draft.Content)
	explanation := strings.TrimSpace(draft.Explanation)
	if content == "" || len(content) > MaxTemplateGeneratedContentLength ||
		explanation == "" || len(explanation) > MaxTemplateGeneratedExplanationLength {
		return nil, fmt.Errorf("%w: provider returned an invalid draft shape", ErrTemplateGenerationProvider)
	}

	issues := validateTemplateContent(target, in, content)
	return &TemplateGenerationResult{
		Target:      target,
		Content:     content,
		Explanation: explanation,
		Valid:       !hasErrorIssue(issues),
		Issues:      issues,
	}, nil
}

// finishBundle parses a {slots, explanation} draft. A whole-template response
// can overflow max_tokens and arrive truncated; rather than 502, we degrade to
// a reviewable invalid result telling the user to generate slots individually.
func finishBundle(in TemplateGenerationInput, raw string) (*TemplateGenerationResult, error) {
	var draft struct {
		Slots       map[string]string `json:"slots"`
		Explanation string            `json:"explanation"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &draft); err != nil {
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
		return nil, fmt.Errorf("%w: provider returned no slots", ErrTemplateGenerationProvider)
	}

	explanation := strings.TrimSpace(draft.Explanation)
	if len(explanation) > MaxTemplateGeneratedExplanationLength {
		explanation = explanation[:MaxTemplateGeneratedExplanationLength]
	}

	result := &TemplateGenerationResult{
		Target:      TemplateTargetBundle,
		Slots:       map[string]string{},
		Explanation: explanation,
		Issues:      []shortcodes.LintIssue{},
	}
	// Only keep slots the handler asked for, in a stable order, and lint each.
	allowed := in.BundleSlots
	if len(allowed) == 0 {
		for k := range draft.Slots {
			allowed = append(allowed, k)
		}
	}
	for _, slotName := range allowed {
		content, ok := draft.Slots[slotName]
		if !ok {
			continue
		}
		content = strings.TrimSpace(content)
		if content == "" {
			continue
		}
		if len(content) > MaxTemplateGeneratedContentLength {
			content = content[:MaxTemplateGeneratedContentLength]
		}
		result.Slots[slotName] = content
		result.Issues = append(result.Issues, validateTemplateContent(TemplateTargetSlot, in, content)...)
	}
	if len(result.Slots) == 0 {
		return nil, fmt.Errorf("%w: provider returned no recognized slots", ErrTemplateGenerationProvider)
	}
	result.Valid = !hasErrorIssue(result.Issues)
	return result, nil
}

// validateTemplateContent lints slot/bundle content as shortcodes and validates
// metaschema content as JSON + JSON Schema. Returned issues are never fatal —
// they flag the draft for review.
func validateTemplateContent(target string, in TemplateGenerationInput, content string) []shortcodes.LintIssue {
	if target == TemplateTargetMetaSchema {
		return validateMetaSchemaJSON(content)
	}
	issues := shortcodes.Lint(content, shortcodes.LintOptions{
		Known:        in.Known,
		ValidateMRQL: in.ValidateMRQL,
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
// Schema, mirroring plugin_system.compileSchema.
func compileGeneratedSchema(schemaJSON string) error {
	doc, err := jsonschema.UnmarshalJSON(strings.NewReader(schemaJSON))
	if err != nil {
		return err
	}
	c := jsonschema.NewCompiler()
	const id = "mem://generated-metaschema.json"
	if err := c.AddResource(id, doc); err != nil {
		return err
	}
	_, err = c.Compile(id)
	return err
}

func hasErrorIssue(issues []shortcodes.LintIssue) bool {
	for _, i := range issues {
		if i.Severity == shortcodes.SeverityError {
			return true
		}
	}
	return false
}
