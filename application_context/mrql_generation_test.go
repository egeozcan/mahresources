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
