package application_context

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"mahresources/shortcodes"
)

type fakeTemplateDraftProvider struct {
	response   string
	err        error
	seenSystem string
	seenUser   string
	seenTokens int
}

func (f *fakeTemplateDraftProvider) GenerateDraft(ctx context.Context, systemPrompt, userPrompt string, maxTokens int) (string, error) {
	f.seenSystem = systemPrompt
	f.seenUser = userPrompt
	f.seenTokens = maxTokens
	if f.err != nil {
		return "", f.err
	}
	return f.response, nil
}

func templateGenConfig() TemplateGenerationConfig {
	return TemplateGenerationConfig{APIKey: "key", Model: "deepseek-v4-pro", Timeout: time.Second}
}

func slotInput() TemplateGenerationInput {
	return TemplateGenerationInput{
		Target:       TemplateTargetSlot,
		Mode:         "html",
		Slot:         "CustomHeader",
		EntityType:   "group",
		MetaSchema:   `{"type":"object","properties":{"rating":{"type":"number"}}}`,
		DocsBlock:    "meta · [meta path=\"...\"] · renders a Meta value",
		Known:        shortcodes.KnownFromBuiltins(),
		ValidateMRQL: func(string) error { return nil },
	}
}

func TestTemplateGeneratorSlotSuccess(t *testing.T) {
	provider := &fakeTemplateDraftProvider{
		response: `{"content":"<div>[property path=\"Name\"]</div>","explanation":"Shows the name."}`,
	}
	gen := NewTemplateGenerator(provider, templateGenConfig())

	got, err := gen.GenerateTemplate(context.Background(), slotInput(), "show the name in a div")
	if err != nil {
		t.Fatalf("GenerateTemplate: %v", err)
	}
	if !got.Valid {
		t.Fatalf("expected valid result, got issues %#v", got.Issues)
	}
	if got.Content != `<div>[property path="Name"]</div>` || got.Explanation != "Shows the name." {
		t.Fatalf("unexpected result: %#v", got)
	}
	// The prompt must carry the grounding.
	for _, want := range []string{
		"CustomHeader slot of a group template",
		"renders at the top of the entity's detail page",
		"Shortcode reference:",
		"renders a Meta value", // DocsBlock survives into the prompt
		`"rating"`,             // schema embedded
		"User request: show the name in a div",
	} {
		if !strings.Contains(provider.seenUser, want) {
			t.Fatalf("prompt missing %q in:\n%s", want, provider.seenUser)
		}
	}
	if provider.seenTokens != DefaultTemplateGenerationMaxTokens {
		t.Fatalf("slot maxTokens = %d, want %d", provider.seenTokens, DefaultTemplateGenerationMaxTokens)
	}
}

func TestTemplateGeneratorMissingKey(t *testing.T) {
	gen := NewTemplateGenerator(&fakeTemplateDraftProvider{}, TemplateGenerationConfig{Model: "m", Timeout: time.Second})
	_, err := gen.GenerateTemplate(context.Background(), slotInput(), "anything")
	if !errors.Is(err, ErrTemplateGenerationNotConfigured) {
		t.Fatalf("expected ErrTemplateGenerationNotConfigured, got %v", err)
	}
}

func TestTemplateGeneratorPromptLength(t *testing.T) {
	gen := NewTemplateGenerator(&fakeTemplateDraftProvider{}, templateGenConfig())
	_, err := gen.GenerateTemplate(context.Background(), slotInput(), strings.Repeat("x", MaxTemplateGenerationPromptLength+1))
	if !errors.Is(err, ErrTemplateGenerationBadRequest) {
		t.Fatalf("expected bad request for long prompt, got %v", err)
	}
}

func TestTemplateGeneratorSlotInvalidLint(t *testing.T) {
	// [meta] is missing its required "path" attribute -> lint SeverityError.
	provider := &fakeTemplateDraftProvider{
		response: `{"content":"<div>[meta]</div>","explanation":"Broken."}`,
	}
	gen := NewTemplateGenerator(provider, templateGenConfig())

	got, err := gen.GenerateTemplate(context.Background(), slotInput(), "show a meta value")
	if err != nil {
		t.Fatalf("GenerateTemplate should return an invalid result, not a transport error: %v", err)
	}
	if got.Valid {
		t.Fatalf("expected invalid result for broken shortcode")
	}
	if len(got.Issues) == 0 {
		t.Fatalf("expected lint issues, got none")
	}
}

func TestTemplateGeneratorMetaSchemaValid(t *testing.T) {
	in := slotInput()
	in.Target = TemplateTargetMetaSchema
	in.Mode = "json"
	in.Slot = ""
	provider := &fakeTemplateDraftProvider{
		response: `{"content":"{\"type\":\"object\",\"properties\":{\"rating\":{\"type\":\"number\"}}}","explanation":"A rating field."}`,
	}
	gen := NewTemplateGenerator(provider, templateGenConfig())

	got, err := gen.GenerateTemplate(context.Background(), in, "add a rating number field")
	if err != nil {
		t.Fatalf("GenerateTemplate: %v", err)
	}
	if !got.Valid {
		t.Fatalf("expected valid schema, got issues %#v", got.Issues)
	}
	if !strings.Contains(provider.seenSystem, "JSON Schema") {
		t.Fatalf("metaschema system prompt missing JSON Schema instruction: %s", provider.seenSystem)
	}
}

func TestTemplateGeneratorMetaSchemaInvalidJSON(t *testing.T) {
	in := slotInput()
	in.Target = TemplateTargetMetaSchema
	provider := &fakeTemplateDraftProvider{
		response: `{"content":"this is not json","explanation":"Oops."}`,
	}
	gen := NewTemplateGenerator(provider, templateGenConfig())

	got, err := gen.GenerateTemplate(context.Background(), in, "add fields")
	if err != nil {
		t.Fatalf("GenerateTemplate should return invalid result, not error: %v", err)
	}
	if got.Valid {
		t.Fatalf("expected invalid result for non-JSON metaschema")
	}
}

func TestTemplateGeneratorBundle(t *testing.T) {
	in := slotInput()
	in.Target = TemplateTargetBundle
	in.Slot = ""
	in.BundleSlots = []string{"CustomHeader", "CustomCSS", "CustomSidebar"}
	provider := &fakeTemplateDraftProvider{
		response: `{"slots":{"CustomHeader":"<h1>[property path=\"Name\"]</h1>","CustomCSS":".card{padding:1rem}"},"explanation":"A simple template."}`,
	}
	gen := NewTemplateGenerator(provider, templateGenConfig())

	got, err := gen.GenerateTemplate(context.Background(), in, "a clean card layout")
	if err != nil {
		t.Fatalf("GenerateTemplate: %v", err)
	}
	if !got.Valid {
		t.Fatalf("expected valid bundle, got issues %#v", got.Issues)
	}
	if len(got.Slots) != 2 || got.Slots["CustomHeader"] == "" || got.Slots["CustomCSS"] == "" {
		t.Fatalf("unexpected slots: %#v", got.Slots)
	}
	if got.Content != "" {
		t.Fatalf("bundle result should not set Content, got %q", got.Content)
	}
	if provider.seenTokens != DefaultTemplateBundleMaxTokens {
		t.Fatalf("bundle maxTokens = %d, want %d", provider.seenTokens, DefaultTemplateBundleMaxTokens)
	}
}

func TestTemplateGeneratorBundleMalformedDegrades(t *testing.T) {
	in := slotInput()
	in.Target = TemplateTargetBundle
	in.BundleSlots = []string{"CustomHeader"}
	provider := &fakeTemplateDraftProvider{response: `{"slots":{"CustomHeader":"<h1>trunc`} // truncated JSON
	gen := NewTemplateGenerator(provider, templateGenConfig())

	got, err := gen.GenerateTemplate(context.Background(), in, "a template")
	if err != nil {
		t.Fatalf("bundle overflow should degrade, not error: %v", err)
	}
	if got.Valid || len(got.Issues) == 0 {
		t.Fatalf("expected invalid degraded bundle result, got %#v", got)
	}
}

func TestTemplateGeneratorProviderError(t *testing.T) {
	gen := NewTemplateGenerator(&fakeTemplateDraftProvider{err: errors.New("boom")}, templateGenConfig())
	_, err := gen.GenerateTemplate(context.Background(), slotInput(), "anything")
	if !errors.Is(err, ErrTemplateGenerationProvider) {
		t.Fatalf("expected provider error, got %v", err)
	}
}

func TestTemplateGeneratorTimeout(t *testing.T) {
	gen := NewTemplateGenerator(&fakeTemplateDraftProvider{err: context.DeadlineExceeded}, templateGenConfig())
	_, err := gen.GenerateTemplate(context.Background(), slotInput(), "anything")
	if !errors.Is(err, ErrTemplateGenerationTimeout) {
		t.Fatalf("expected timeout error, got %v", err)
	}
}

func TestMahresourcesContextTemplateGeneratorSeam(t *testing.T) {
	gen := NewTemplateGenerator(&fakeTemplateDraftProvider{}, templateGenConfig())
	ctx := &MahresourcesContext{}

	ctx.SetTemplateGenerator(gen)

	if ctx.TemplateGenerator() != gen {
		t.Fatalf("TemplateGenerator seam returned %#v, want %#v", ctx.TemplateGenerator(), gen)
	}
	if ctx.TemplateGenerationRateLimiter() == nil {
		t.Fatal("TemplateGenerationRateLimiter should lazily create a limiter")
	}
}

func countIssuesContaining(issues []shortcodes.LintIssue, want string) int {
	n := 0
	for _, iss := range issues {
		if strings.Contains(iss.Message, want) {
			n++
		}
	}
	return n
}

// TestTemplateGeneratorCSSSlotIsLintedAsAStylesheet pins that a generated
// CustomCSS slot is linted as a stylesheet. The slot carries no <style> wrapper
// of its own, so nothing in its text says so and the caller has to say it;
// without that the draft is reported clean while the editor gutter warns on the
// identical buffer.
// It takes either signal on its own. The slot name is the half the handler
// validates, so a client that names CustomCSS and mislabels the mode still gets
// the warning; a client that names no slot is taken at its word about the mode.
func TestTemplateGeneratorCSSSlotIsLintedAsAStylesheet(t *testing.T) {
	cases := map[string]struct{ mode, slot string }{
		"both signals agree":     {mode: "css", slot: "CustomCSS"},
		"slot alone":             {mode: "html", slot: "CustomCSS"},
		"slot alone, no mode":    {mode: "", slot: "CustomCSS"},
		"mode alone, no slot":    {mode: "css", slot: ""},
		"mode alone, mixed case": {mode: "CSS", slot: ""},
	}
	// A named slot outranks the mode, so this table may not gain a case that
	// names a markup slot; see TestTemplateGeneratorNamedSlotOutranksTheMode.

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			in := slotInput()
			in.Mode = tc.mode
			in.Slot = tc.slot
			provider := &fakeTemplateDraftProvider{
				response: `{"content":".badge{color:[meta path=\"colour\" inline=\"true\"]}","explanation":"Colours the badge."}`,
			}
			gen := NewTemplateGenerator(provider, templateGenConfig())

			got, err := gen.GenerateTemplate(context.Background(), in, "colour the badge from meta")
			if err != nil {
				t.Fatalf("GenerateTemplate: %v", err)
			}
			if n := countIssuesContaining(got.Issues, "CSS slot"); n != 1 {
				t.Fatalf("expected exactly one CSS-placement issue, got %d: %#v", n, got.Issues)
			}
			// Placement is a warning, not an error — the draft stays applicable.
			if !got.Valid {
				t.Errorf("a placement warning must not invalidate the draft: %#v", got.Issues)
			}
		})
	}
}

// TestTemplateGeneratorHTMLSlotIsNotLintedAsCSS is the other half: the same
// interpolation in an HTML slot is ordinary text and must stay quiet.
func TestTemplateGeneratorHTMLSlotIsNotLintedAsCSS(t *testing.T) {
	provider := &fakeTemplateDraftProvider{
		response: `{"content":"<div>[meta path=\"colour\" inline=\"true\"]</div>","explanation":"Shows the colour."}`,
	}
	gen := NewTemplateGenerator(provider, templateGenConfig())

	got, err := gen.GenerateTemplate(context.Background(), slotInput(), "show the colour")
	if err != nil {
		t.Fatalf("GenerateTemplate: %v", err)
	}
	if n := countIssuesContaining(got.Issues, "CSS slot"); n != 0 {
		t.Fatalf("an HTML slot must not be judged as a stylesheet, got %d: %#v", n, got.Issues)
	}
}

// TestTemplateGeneratorNamedSlotOutranksTheMode pins the direction that costs
// something. The slot is the half the handler validates; the mode is not
// validated at all, and a markup draft judged as CSS does not merely gain a
// spurious warning — shortcodes' CSS branch stands in place of the markup
// checks, so the raw= "becomes real elements on the page" warning, the XSS one,
// disappears.
func TestTemplateGeneratorNamedSlotOutranksTheMode(t *testing.T) {
	in := slotInput()
	in.Slot = "CustomHeader"
	in.Mode = "css" // the request contradicts itself; the slot is the half to believe
	provider := &fakeTemplateDraftProvider{
		response: `{"content":"<div>[meta path=\"colour\" inline=\"true\" raw=\"true\"]</div>","explanation":"Shows the colour."}`,
	}
	gen := NewTemplateGenerator(provider, templateGenConfig())

	got, err := gen.GenerateTemplate(context.Background(), in, "show the colour")
	if err != nil {
		t.Fatalf("GenerateTemplate: %v", err)
	}
	if n := countIssuesContaining(got.Issues, "CSS slot"); n != 0 {
		t.Errorf("a markup slot must not be judged as a stylesheet, got %d: %#v", n, got.Issues)
	}
	if n := countIssuesContaining(got.Issues, "becomes real elements"); n != 1 {
		t.Fatalf("the markup XSS warning is what a CSS reading would swallow; got %d: %#v", n, got.Issues)
	}
}

// TestTemplateGeneratorSlotGovernsThePromptToo pins the two halves together.
// The handler validates Slot and does not validate Mode, so a request can
// contradict itself; the prompt's mode rule and the linter's reading must not
// then answer it differently — asking the model for HTML and grading the reply
// as CSS is worse than either reading alone.
func TestTemplateGeneratorSlotGovernsThePromptToo(t *testing.T) {
	cases := map[string]struct {
		slot, mode string
		wantRule   string
		rejectRule string
	}{
		"CSS slot labelled html": {
			slot: "CustomCSS", mode: "html",
			wantRule: "Output CSS only", rejectRule: "Output HTML",
		},
		"markup slot labelled css": {
			slot: "CustomHeader", mode: "css",
			wantRule: "Output HTML", rejectRule: "Output CSS only",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			in := slotInput()
			in.Slot = tc.slot
			in.Mode = tc.mode
			provider := &fakeTemplateDraftProvider{
				response: `{"content":"x","explanation":"y"}`,
			}
			gen := NewTemplateGenerator(provider, templateGenConfig())

			if _, err := gen.GenerateTemplate(context.Background(), in, "do the thing"); err != nil {
				t.Fatalf("GenerateTemplate: %v", err)
			}
			if !strings.Contains(provider.seenUser, tc.wantRule) {
				t.Errorf("prompt should follow the slot and say %q:\n%s", tc.wantRule, provider.seenUser)
			}
			if strings.Contains(provider.seenUser, tc.rejectRule) {
				t.Errorf("prompt must not follow the contradicted mode and say %q:\n%s", tc.rejectRule, provider.seenUser)
			}
		})
	}
}

// TestTemplateGeneratorBundleDecidesCSSModePerSlot pins why cssMode is a
// parameter rather than something read off the input inside the function:
// finishBundle shares one TemplateGenerationInput across every slot it
// validates, so only the CustomCSS slot of a bundle may be linted as a
// stylesheet. Either whole-bundle answer is wrong — deriving from Mode="html"
// flags neither slot, deriving from Mode="css" flags both.
func TestTemplateGeneratorBundleDecidesCSSModePerSlot(t *testing.T) {
	// Each slot's content is diagnostic of the mode it was linted in, so the
	// assertions below name a direction rather than a count. Counting "CSS
	// slot" alone would not: inverting the condition merely moves that one
	// warning from CustomCSS to CustomHeader, leaving the total at one.
	//
	// CustomHeader carries raw= in plain text, which warns "becomes real
	// elements" as markup and "raw= ... in a CSS slot" as CSS. CustomCSS
	// carries a bare interpolation, which warns only as CSS and is silent as
	// markup.
	const draft = `{"slots":{"CustomHeader":"<div>[meta path=\"colour\" inline=\"true\" raw=\"true\"]</div>","CustomCSS":".badge{color:[meta path=\"colour\" inline=\"true\"]}"},"explanation":"Styles the badge."}`

	for _, mode := range []string{"html", "css"} {
		t.Run("mode="+mode, func(t *testing.T) {
			in := slotInput()
			in.Target = TemplateTargetBundle
			in.Mode = mode
			in.Slot = ""
			in.BundleSlots = []string{"CustomHeader", "CustomCSS"}

			gen := NewTemplateGenerator(&fakeTemplateDraftProvider{response: draft}, templateGenConfig())
			got, err := gen.GenerateTemplate(context.Background(), in, "style the badge")
			if err != nil {
				t.Fatalf("GenerateTemplate: %v", err)
			}
			// CustomCSS was read as a stylesheet — and only it, or this count
			// would be two.
			if n := countIssuesContaining(got.Issues, "CSS slot"); n != 1 {
				t.Fatalf("expected the CSS-placement issue on CustomCSS alone, got %d: %#v", n, got.Issues)
			}
			// ...and CustomHeader was read as markup. This is the assertion an
			// inverted condition fails: it would lint the header as CSS, whose
			// branch stands in place of this warning rather than beside it.
			if n := countIssuesContaining(got.Issues, "becomes real elements"); n != 1 {
				t.Fatalf("expected CustomHeader's markup XSS warning, got %d: %#v", n, got.Issues)
			}
		})
	}
}
