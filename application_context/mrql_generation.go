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
		"Prefer the simplest valid query that answers the request.",
		"Clause order: expression, SCOPE, GROUP BY, ORDER BY, LIMIT, OFFSET.",
		"Start with type = resource, type = note, or type = group when using entity-specific fields.",
		"Common fields: id, name, description, created, updated, tags, guid, meta.<key>, TEXT.",
		"Resource fields: contentType, fileSize, width, height, originalName, hash, category, owner, groups/group, notes.",
		"Note fields: noteType, owner, groups/group, resources.",
		"Group fields: category, parent, children, resources, notes.",
		"Strings must be double-quoted.",
		"Valid operators are =, !=, ~, !~, >, >=, <, <=, IN, NOT IN, IS EMPTY, IS NOT EMPTY, IS NULL, and IS NOT NULL.",
		"Never use HAS, CONTAINS, LIKE, MATCH, includes, or other natural-language operators.",
		"Use TEXT only as TEXT ~ \"plain words\" with at least one alphanumeric word.",
		"Use contentType ~ \"image/*\" for MIME patterns.",
		"Relations use names with =, !=, ~, !~; use IS EMPTY/IS NOT EMPTY for missing/present relations.",
		"Use tags = \"tag-name\" for a single tag. Use tags IN (\"a\", \"b\") for multiple tags.",
		"Use tags/groups IN (\"a\", \"b\") only for tags, groups, or group; do not use IN for owner, parent, or children.",
		"Use meta.<key> for metadata only when the user names the key.",
		"Use relative dates like -30d or supported date functions, never natural-language dates.",
		"category and noteType require numeric IDs; if no ID is present, prefer tags, name, or TEXT.",
		"For resources, the resource category field is category. Do not emit resourceCategory.",
		"Use <relation>.count with =, !=, >, >=, <, <= to compare relation sizes (e.g. tags.count = 0, resources.count >= 100); also valid in ORDER BY. Never count owner or parent.",
		"GROUP BY requires explicit type. Use COUNT() for aggregate counts.",
		"After GROUP BY aggregates, use HAVING with aggregate functions to filter buckets (e.g. GROUP BY hash COUNT() HAVING COUNT() > 1). HAVING never uses plain fields.",
		"GROUP BY supports date buckets created.day, created.week, created.month, created.year (same for updated). Date buckets are valid only in GROUP BY and its ORDER BY, never in the filter expression.",
		"Use ancestors.<group field> or descendants.<group field> for recursive hierarchy checks at any depth (e.g. ancestors.name = \"Archive\", descendants.tags = \"wip\", ancestors.meta.region = \"eu\"). ancestors/descendants are strict (they exclude the item's own group); combine with owner./parent. to include it. They take exactly one group leaf field, never IN, IS EMPTY, ORDER BY, or GROUP BY.",
		"Add LIMIT 50 for broad queries unless the user asks for another small limit.",
		"Example mappings use <placeholders>; replace them with user-provided values and never emit the placeholder tokens.",
		"images with tag <tag> -> type = resource AND contentType ~ \"image/*\" AND tags = \"<tag>\" LIMIT 50",
		"resources whose owner has tag <tag> -> type = resource AND owner.tags = \"<tag>\" LIMIT 50",
		"notes about <text> -> type = note AND TEXT ~ \"<text>\" LIMIT 50",
		"groups named <name> -> type = group AND name ~ \"<name>*\" LIMIT 50",
		"duplicate resources by hash -> type = resource GROUP BY hash COUNT() HAVING COUNT() > 1 ORDER BY count DESC LIMIT 50",
		"notes per month -> type = note GROUP BY created.month COUNT() ORDER BY created.month ASC LIMIT 50",
		"resources without notes -> type = resource AND notes IS EMPTY LIMIT 50",
		"groups with at least <n> resources -> type = group AND resources.count >= <n> ORDER BY resources.count DESC LIMIT 50",
		"untagged resources -> type = resource AND tags.count = 0 LIMIT 50",
		"resources anywhere under a group named <name> -> type = resource AND ancestors.name = \"<name>\" LIMIT 50",
		"groups containing a descendant tagged <tag> -> type = group AND descendants.tags = \"<tag>\" LIMIT 50",
		"User request: " + userPrompt,
	}, "\n")
}
