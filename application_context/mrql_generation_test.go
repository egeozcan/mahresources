package application_context

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type fakeMRQLDraftProvider struct {
	query       string
	explanation string
	err         error
	seenPrompt  string
}

func (f *fakeMRQLDraftProvider) GenerateDraft(ctx context.Context, prompt string) (providerMRQLDraft, error) {
	f.seenPrompt = prompt
	if f.err != nil {
		return providerMRQLDraft{}, f.err
	}
	return providerMRQLDraft{Query: f.query, Explanation: f.explanation}, nil
}

func TestMRQLGeneratorSuccessValidatesAndExplains(t *testing.T) {
	provider := &fakeMRQLDraftProvider{
		query:       `type = resource AND contentType ~ "image/*" LIMIT 50`,
		explanation: "Finds up to 50 image resources.",
	}
	gen := NewMRQLGenerator(provider, MRQLGenerationConfig{APIKey: "key", Model: "deepseek-v4-pro", Timeout: time.Second})

	got, err := gen.GenerateMRQL(context.Background(), "show image resources")
	if err != nil {
		t.Fatalf("GenerateMRQL: %v", err)
	}
	if !got.Valid {
		t.Fatalf("expected valid result, got %#v", got.Errors)
	}
	if got.Query != provider.query || got.Explanation != provider.explanation {
		t.Fatalf("unexpected result: %#v", got)
	}
}

func TestMRQLGeneratorMissingKey(t *testing.T) {
	gen := NewMRQLGenerator(&fakeMRQLDraftProvider{}, MRQLGenerationConfig{Model: "deepseek-v4-pro", Timeout: time.Second})
	_, err := gen.GenerateMRQL(context.Background(), "anything")
	if !errors.Is(err, ErrMRQLGenerationNotConfigured) {
		t.Fatalf("expected ErrMRQLGenerationNotConfigured, got %v", err)
	}
}

func TestMRQLGeneratorPromptLength(t *testing.T) {
	gen := NewMRQLGenerator(&fakeMRQLDraftProvider{}, MRQLGenerationConfig{APIKey: "key", Model: "deepseek-v4-pro", Timeout: time.Second})
	_, err := gen.GenerateMRQL(context.Background(), strings.Repeat("x", MaxMRQLGenerationPromptLength+1))
	if !errors.Is(err, ErrMRQLGenerationBadRequest) {
		t.Fatalf("expected bad request for long prompt, got %v", err)
	}
}

func TestMRQLGeneratorInvalidGeneratedQuery(t *testing.T) {
	provider := &fakeMRQLDraftProvider{query: `type = resource LIMIT 1000000`, explanation: "Too many."}
	gen := NewMRQLGenerator(provider, MRQLGenerationConfig{APIKey: "key", Model: "deepseek-v4-pro", Timeout: time.Second})

	got, err := gen.GenerateMRQL(context.Background(), "all resources")
	if err != nil {
		t.Fatalf("GenerateMRQL should return invalid result, not transport error: %v", err)
	}
	if got.Valid {
		t.Fatalf("expected invalid result")
	}
	if len(got.Errors) == 0 || !strings.Contains(got.Errors[0]["message"].(string), "LIMIT") {
		t.Fatalf("expected LIMIT lint error, got %#v", got.Errors)
	}
}

func TestMRQLGeneratorDoesNotLeakLocalVocabularyIntoPrompt(t *testing.T) {
	provider := &fakeMRQLDraftProvider{query: `TEXT ~ "invoice" LIMIT 50`, explanation: "Finds invoice text."}
	gen := NewMRQLGenerator(provider, MRQLGenerationConfig{APIKey: "key", Model: "deepseek-v4-pro", Timeout: time.Second})

	_, err := gen.GenerateMRQL(context.Background(), "find invoices")
	if err != nil {
		t.Fatalf("GenerateMRQL: %v", err)
	}
	for _, forbidden := range []string{"SecretLocalTag", "PrivateCategory", "ConfidentialNoteType", "HiddenResourceCategory"} {
		if strings.Contains(provider.seenPrompt, forbidden) {
			t.Fatalf("prompt leaked local vocabulary %q in %q", forbidden, provider.seenPrompt)
		}
	}
}

func TestMRQLGeneratorPromptExplainsTagSyntaxAndBansHas(t *testing.T) {
	provider := &fakeMRQLDraftProvider{
		query:       `type = resource AND contentType ~ "image/*" AND tags = "keo" LIMIT 50`,
		explanation: "Finds image resources tagged keo.",
	}
	gen := NewMRQLGenerator(provider, MRQLGenerationConfig{APIKey: "key", Model: "deepseek-v4-pro", Timeout: time.Second})

	_, err := gen.GenerateMRQL(context.Background(), `images with the tag "keo"`)
	if err != nil {
		t.Fatalf("GenerateMRQL: %v", err)
	}

	for _, want := range []string{
		`Use tags = "tag-name"`,
		`Use tags IN ("a", "b")`,
		`Never use HAS`,
	} {
		if !strings.Contains(provider.seenPrompt, want) {
			t.Fatalf("prompt missing %q in:\n%s", want, provider.seenPrompt)
		}
	}
	if strings.Contains(provider.seenPrompt, `tags = "keo" LIMIT 50`) {
		t.Fatalf("prompt should not hard-code request-specific tag values in examples:\n%s", provider.seenPrompt)
	}
}

func TestMRQLGeneratorPromptIncludesCompactSyntaxGuide(t *testing.T) {
	provider := &fakeMRQLDraftProvider{
		query:       `type = resource AND name ~ "report*" LIMIT 50`,
		explanation: "Finds resource names matching report.",
	}
	gen := NewMRQLGenerator(provider, MRQLGenerationConfig{APIKey: "key", Model: "deepseek-v4-pro", Timeout: time.Second})

	_, err := gen.GenerateMRQL(context.Background(), "resource names like report")
	if err != nil {
		t.Fatalf("GenerateMRQL: %v", err)
	}

	for _, want := range []string{
		"Prefer the simplest valid query that answers the request.",
		"Start with type = resource, type = note, or type = group when using entity-specific fields.",
		"Common fields: id, name, description, created, updated, tags, guid, meta.<key>, TEXT.",
		"Resource fields: contentType, fileSize, width, height, originalName, hash, category, owner, groups/group, notes.",
		"Note fields: noteType, owner, groups/group, resources.",
		"Group fields: category, parent, children, resources, notes.",
		"Relations use names with =, !=, ~, !~; use IS EMPTY/IS NOT EMPTY for missing/present relations.",
		`Use tags/groups IN ("a", "b") only for tags, groups, or group; do not use IN for owner, parent, or children.`,
		"Use meta.<key> for metadata only when the user names the key.",
		"Example mappings use <placeholders>; replace them with user-provided values and never emit the placeholder tokens.",
		`images with tag <tag> -> type = resource AND contentType ~ "image/*" AND tags = "<tag>" LIMIT 50`,
		`resources whose owner has tag <tag> -> type = resource AND owner.tags = "<tag>" LIMIT 50`,
		`notes about <text> -> type = note AND TEXT ~ "<text>" LIMIT 50`,
		`groups named <name> -> type = group AND name ~ "<name>*" LIMIT 50`,
		"Use <relation>.count with =, !=, >, >=, <, <= to compare relation sizes (e.g. tags.count = 0, resources.count >= 100); also valid in ORDER BY. Never count owner or parent.",
		"After GROUP BY aggregates, use HAVING with aggregate functions to filter buckets (e.g. GROUP BY hash COUNT() HAVING COUNT() > 1). HAVING never uses plain fields.",
		"GROUP BY supports date buckets created.day, created.week, created.month, created.year (same for updated). Date buckets are valid only in GROUP BY and its ORDER BY, never in the filter expression.",
		"duplicate resources by hash -> type = resource GROUP BY hash COUNT() HAVING COUNT() > 1 ORDER BY count DESC LIMIT 50",
		"notes per month -> type = note GROUP BY created.month COUNT() ORDER BY created.month ASC LIMIT 50",
	} {
		if !strings.Contains(provider.seenPrompt, want) {
			t.Fatalf("prompt missing %q in:\n%s", want, provider.seenPrompt)
		}
	}
}

func TestMRQLGeneratorProviderErrors(t *testing.T) {
	gen := NewMRQLGenerator(
		&fakeMRQLDraftProvider{err: errors.New("provider exploded")},
		MRQLGenerationConfig{APIKey: "key", Model: "deepseek-v4-pro", Timeout: time.Second},
	)

	_, err := gen.GenerateMRQL(context.Background(), "anything")
	if !errors.Is(err, ErrMRQLGenerationProvider) {
		t.Fatalf("expected provider error, got %v", err)
	}
}

func TestMahresourcesContextMRQLGeneratorSeam(t *testing.T) {
	gen := NewMRQLGenerator(
		&fakeMRQLDraftProvider{query: `type = resource LIMIT 50`, explanation: "Finds resources."},
		MRQLGenerationConfig{APIKey: "key", Model: "deepseek-v4-pro", Timeout: time.Second},
	)
	ctx := &MahresourcesContext{}

	ctx.SetMRQLGenerator(gen)

	if ctx.MRQLGenerator() != gen {
		t.Fatalf("MRQLGenerator seam returned %#v, want %#v", ctx.MRQLGenerator(), gen)
	}
}
