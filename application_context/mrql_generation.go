package application_context

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"mahresources/mrql"
)

const (
	MaxMRQLGenerationPromptLength        = 2000
	MaxMRQLGeneratedQueryLength          = 2000
	MaxMRQLGeneratedExplanationLength    = 1000
	DefaultDeepSeekMRQLGenerationModel   = "deepseek-v4-pro"
	DefaultDeepSeekMRQLGenerationTimeout = 20 * time.Second
)

var (
	ErrMRQLGenerationNotConfigured = errors.New("mrql generation is not configured")
	ErrMRQLGenerationBadRequest    = errors.New("bad mrql generation request")
	ErrMRQLGenerationProvider      = errors.New("mrql generation provider error")
	ErrMRQLGenerationTimeout       = errors.New("mrql generation provider timeout")
)

type MRQLGenerationConfig struct {
	APIKey  string
	Model   string
	Timeout time.Duration
}

type MRQLGenerationResult struct {
	Query       string           `json:"query"`
	Explanation string           `json:"explanation"`
	Valid       bool             `json:"valid"`
	Errors      []map[string]any `json:"errors"`
}

type MRQLGenerator interface {
	GenerateMRQL(ctx context.Context, prompt string) (*MRQLGenerationResult, error)
}

type providerMRQLDraft struct {
	Query       string
	Explanation string
}

type MRQLDraftProvider interface {
	GenerateDraft(ctx context.Context, prompt string) (providerMRQLDraft, error)
}

type defaultMRQLGenerator struct {
	provider MRQLDraftProvider
	config   MRQLGenerationConfig
}

func NewMRQLGenerator(provider MRQLDraftProvider, cfg MRQLGenerationConfig) MRQLGenerator {
	if cfg.Model == "" {
		cfg.Model = DefaultDeepSeekMRQLGenerationModel
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = DefaultDeepSeekMRQLGenerationTimeout
	}
	return &defaultMRQLGenerator{provider: provider, config: cfg}
}

func (g *defaultMRQLGenerator) GenerateMRQL(ctx context.Context, prompt string) (*MRQLGenerationResult, error) {
	if strings.TrimSpace(g.config.APIKey) == "" || g.provider == nil {
		return nil, ErrMRQLGenerationNotConfigured
	}
	prompt = strings.TrimSpace(prompt)
	if prompt == "" || len(prompt) > MaxMRQLGenerationPromptLength {
		return nil, fmt.Errorf("%w: prompt must be between 1 and %d characters", ErrMRQLGenerationBadRequest, MaxMRQLGenerationPromptLength)
	}

	callCtx, cancel := context.WithTimeout(ctx, g.config.Timeout)
	defer cancel()

	draft, err := g.provider.GenerateDraft(callCtx, buildMRQLGenerationPrompt(prompt))
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(callCtx.Err(), context.DeadlineExceeded) {
			return nil, ErrMRQLGenerationTimeout
		}
		return nil, fmt.Errorf("%w: %v", ErrMRQLGenerationProvider, err)
	}

	query := strings.TrimSpace(draft.Query)
	explanation := strings.TrimSpace(draft.Explanation)
	if query == "" || len(query) > MaxMRQLGeneratedQueryLength ||
		explanation == "" || len(explanation) > MaxMRQLGeneratedExplanationLength {
		return nil, fmt.Errorf("%w: provider returned an invalid draft shape", ErrMRQLGenerationProvider)
	}

	result := &MRQLGenerationResult{
		Query:       query,
		Explanation: explanation,
		Valid:       true,
		Errors:      []map[string]any{},
	}

	parsed, parseErr := mrql.Parse(query)
	if parseErr != nil {
		result.Valid = false
		result.Errors = []map[string]any{{"message": parseErr.Error()}}
		return result, nil
	}
	if validateErr := mrql.Validate(parsed); validateErr != nil {
		result.Valid = false
		result.Errors = []map[string]any{{"message": validateErr.Error()}}
		return result, nil
	}
	if lintErrs := mrql.LintGeneratedQuery(parsed); len(lintErrs) > 0 {
		result.Valid = false
		result.Errors = lintErrs
	}
	return result, nil
}

func buildMRQLGenerationPrompt(userPrompt string) string {
	return strings.Join([]string{
		"Generate one Mahresources MRQL query from the user request.",
		"Return strict JSON only with keys query and explanation.",
		"Use only MRQL syntax rules and values explicitly present in the user request.",
		"Do not use local tags, categories, note types, group names, filenames, metadata keys, or other local vocabulary unless the user typed them.",
		"Clause order: expression, SCOPE, GROUP BY, ORDER BY, LIMIT, OFFSET.",
		"Strings must be double-quoted.",
		"Use TEXT only as TEXT ~ \"plain words\" with at least one alphanumeric word.",
		"Use contentType ~ \"image/*\" for MIME patterns.",
		"Use relative dates like -30d or supported date functions, never natural-language dates.",
		"category and noteType require numeric IDs; if no ID is present, prefer tags, name, or TEXT.",
		"For resources, the resource category field is category. Do not emit resourceCategory.",
		"GROUP BY requires explicit type. Use COUNT() for aggregate counts.",
		"Add LIMIT 50 for broad queries unless the user asks for another small limit.",
		"User request: " + userPrompt,
	}, "\n")
}
