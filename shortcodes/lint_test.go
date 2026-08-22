package shortcodes

import (
	"errors"
	"strings"
	"testing"
)

// findIssue returns the first issue whose message contains substr, or nil.
func findIssue(issues []LintIssue, substr string) *LintIssue {
	for i := range issues {
		if strings.Contains(issues[i].Message, substr) {
			return &issues[i]
		}
	}
	return nil
}

func TestLint(t *testing.T) {
	known := KnownFromBuiltins()

	tests := []struct {
		name       string
		input      string
		wantSubstr string   // a message substring that must appear ("" = expect no issues)
		wantSev    string   // expected severity of that issue
		wantNone   []string // substrings that must NOT appear
	}{
		{
			name:  "valid meta",
			input: `[meta path="cooking.time"]`,
		},
		{
			name:  "valid conditional block",
			input: "[conditional path=\"x\" eq=\"y\"]yes[/conditional]",
		},
		{
			name:  "valid mrql inline",
			input: `[mrql query="resources" format="list"]`,
		},
		{
			name:       "meta missing path",
			input:      `[meta]`,
			wantSubstr: `missing required attribute "path"`,
			wantSev:    SeverityError,
		},
		{
			name:       "mrql missing query and saved",
			input:      `[mrql format="list"]`,
			wantSubstr: `requires a "query" or "saved"`,
			wantSev:    SeverityError,
		},
		{
			name:       "unclosed conditional block",
			input:      `[conditional path="x" eq="y"]hello`,
			wantSubstr: `must be a block`,
			wantSev:    SeverityError,
		},
		{
			name:       "orphan closing mrql",
			input:      `[/mrql]`,
			wantSubstr: `orphan closing tag`,
			wantSev:    SeverityError,
		},
		{
			name:       "inline shortcode with closing tag",
			input:      `[meta path="x"]text[/meta]`,
			wantSubstr: `inline shortcode and cannot have a closing tag`,
			wantSev:    SeverityError,
		},
		{
			name:       "conditional without operator",
			input:      "[conditional path=\"x\"]a[/conditional]",
			wantSubstr: `needs a comparison operator`,
			wantSev:    SeverityError,
		},
		{
			name:       "conditional without target",
			input:      "[conditional eq=\"y\"]a[/conditional]",
			wantSubstr: `needs a "path", "field", or "mrql"`,
			wantSev:    SeverityError,
		},
		{
			name:       "conditional with two else",
			input:      "[conditional path=\"x\" eq=\"y\"]a[else]b[else]c[/conditional]",
			wantSubstr: `more than one [else]`,
			wantSev:    SeverityError,
		},
		{
			name:  "conditional with one else is valid",
			input: "[conditional path=\"x\" eq=\"y\"]a[else]b[/conditional]",
		},
		{
			name:  "each with item inside is valid",
			input: "[each path=\"tags\"]<li>[item]</li>[/each]",
		},
		{
			name:       "each missing path",
			input:      "[each]<li>[item]</li>[/each]",
			wantSubstr: `missing required attribute "path"`,
			wantSev:    SeverityError,
		},
		{
			name:       "unclosed each block",
			input:      `[each path="x"]<li>[item]</li>`,
			wantSubstr: `must be a block`,
			wantSev:    SeverityError,
		},
		{
			name:       "item outside each",
			input:      `<li>[item path="name"]</li>`,
			wantSubstr: `only meaningful inside an [each] block`,
			wantSev:    SeverityWarning,
		},
		{
			name:     "item inside each not flagged",
			input:    "[each path=\"x\"][item path=\"n\"][/each]",
			wantNone: []string{"only meaningful inside"},
		},
		{
			name:       "unknown attribute on documented shortcode",
			input:      `[meta path="x" bogus="1"]`,
			wantSubstr: `unknown attribute "bogus"`,
			wantSev:    SeverityWarning,
		},
		{
			name:     "param wildcard is known",
			input:    `[mrql saved="r" param-tag="x"]`,
			wantNone: []string{"unknown attribute"},
		},
		{
			name:       "misspelled builtin (single-char typo)",
			input:      `[met path="x"]`,
			wantSubstr: `did you mean [meta]`,
			wantSev:    SeverityInfo,
		},
		{
			name:       "misspelled conditional stays literal",
			input:      "[condtional path=\"x\" eq=\"y\"]a[/condtional]",
			wantSubstr: `did you mean [conditional]`,
			wantSev:    SeverityInfo,
		},
		{
			name:       "malformed plugin shortcode",
			input:      `[plugin:foo]`,
			wantSubstr: `malformed plugin shortcode`,
			wantSev:    SeverityInfo,
		},
		{
			name:     "plain html brackets not flagged",
			input:    `<div>styles[class] and array[0]</div>`,
			wantNone: []string{"did you mean", "unknown shortcode", "malformed"},
		},
		{
			name:       "meta hide-empty and default conflict",
			input:      `[meta path="x" hide-empty="true" default="n/a"]`,
			wantSubstr: `hide-empty wins`,
			wantSev:    SeverityWarning,
		},
		{
			name:     "meta default alone is fine",
			input:    `[meta path="x" default="n/a"]`,
			wantNone: []string{"hide-empty wins"},
		},
		{
			name:     "conditional numbered-suffix attrs not flagged",
			input:    `[conditional path="a" eq="1" path2="b" gte2="5" combine="any"]x[/conditional]`,
			wantNone: []string{"unknown attribute"},
		},
		{
			name:     "conditional new operators not flagged",
			input:    `[conditional path="s" in="a,b" matches="^x" gte="1" lte="9"]x[/conditional]`,
			wantNone: []string{"unknown attribute", "needs a comparison operator"},
		},
		{
			name:       "conditional invalid matches regex",
			input:      `[conditional path="s" matches="([bad"]x[/conditional]`,
			wantSubstr: `invalid regular expression in matches`,
			wantSev:    SeverityError,
		},
		{
			name:       "conditional invalid matches2 regex",
			input:      `[conditional path="s" eq="1" path2="t" matches2="([bad"]x[/conditional]`,
			wantSubstr: `invalid regular expression in matches2`,
			wantSev:    SeverityError,
		},
		{
			name:     "elseif divider not flagged as unknown shortcode",
			input:    `[conditional path="s" eq="1"]a[elseif path="s" eq="2"]b[else]c[/conditional]`,
			wantNone: []string{"did you mean", "unknown shortcode", "malformed"},
		},
		{
			name:  "valid mrql inline value",
			input: `[mrql query="resources" value="count"]`,
		},
		{
			name:  "valid mrql with slots and link-all",
			input: "[mrql query=\"resources\" link-all=\"true\"][header]<h4>{count}</h4>[/header]<li>x</li>[else]none[/mrql]",
		},
		{
			name:       "mrql value with block body conflict",
			input:      `[mrql query="resources" value="count"]<b>x</b>[/mrql]`,
			wantSubstr: `cannot have a block body`,
			wantSev:    SeverityError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issues := Lint(tt.input, LintOptions{Known: known})

			for _, none := range tt.wantNone {
				if got := findIssue(issues, none); got != nil {
					t.Errorf("expected no issue containing %q, got %+v", none, *got)
				}
			}

			if tt.wantSubstr == "" {
				if len(issues) != 0 {
					t.Errorf("expected no issues, got %+v", issues)
				}
				return
			}

			got := findIssue(issues, tt.wantSubstr)
			if got == nil {
				t.Fatalf("expected an issue containing %q, got %+v", tt.wantSubstr, issues)
			}
			if tt.wantSev != "" && got.Severity != tt.wantSev {
				t.Errorf("issue %q: expected severity %q, got %q", tt.wantSubstr, tt.wantSev, got.Severity)
			}
			if got.Start < 0 || got.End > len(tt.input) || got.Start > got.End {
				t.Errorf("issue %q: invalid offsets [%d,%d] for input len %d", tt.wantSubstr, got.Start, got.End, len(tt.input))
			}
		})
	}
}

func TestLintMRQLSyntax(t *testing.T) {
	known := KnownFromBuiltins()
	validate := func(q string) error {
		if strings.Contains(q, "BAD") {
			return errors.New("syntax error near BAD")
		}
		return nil
	}

	issues := Lint(`[mrql query="BAD SYNTAX"]`, LintOptions{Known: known, ValidateMRQL: validate})
	got := findIssue(issues, "MRQL error in query")
	if got == nil {
		t.Fatalf("expected MRQL syntax issue, got %+v", issues)
	}
	if got.Severity != SeverityError {
		t.Errorf("expected error severity, got %q", got.Severity)
	}

	// A conditional's mrql attribute is also validated.
	issues = Lint("[conditional mrql=\"BAD\" gt=\"0\"]x[/conditional]", LintOptions{Known: known, ValidateMRQL: validate})
	if findIssue(issues, "MRQL error in mrql") == nil {
		t.Fatalf("expected MRQL error on conditional mrql attr, got %+v", issues)
	}

	// Valid query produces no MRQL issue.
	issues = Lint(`[mrql query="resources"]`, LintOptions{Known: known, ValidateMRQL: validate})
	if findIssue(issues, "MRQL error") != nil {
		t.Errorf("expected no MRQL issue for valid query, got %+v", issues)
	}
}

func TestLintMRQLErrorAnchorsToAttr(t *testing.T) {
	known := KnownFromBuiltins()
	validate := func(q string) error {
		if strings.Contains(q, "BAD") {
			return errors.New("syntax error near BAD")
		}
		return nil
	}

	// "query=" is a suffix of "param-query="; the error range must anchor to
	// the real query attribute, not the earlier param-query occurrence.
	input := `[mrql param-query="resources" query="BAD"]`
	issues := Lint(input, LintOptions{Known: known, ValidateMRQL: validate})
	got := findIssue(issues, "MRQL error in query")
	if got == nil {
		t.Fatalf("expected MRQL syntax issue, got %+v", issues)
	}
	wantStart := strings.Index(input, ` query="BAD"`) + 1
	if got.Start != wantStart {
		t.Errorf("expected issue to start at %d (the query attr), got %d", wantStart, got.Start)
	}
}

func TestLintUndocumentedPluginSkipsAttrChecks(t *testing.T) {
	// A plugin shortcode present in the catalogue but undocumented: attribute
	// checks are skipped, but structural rules still apply.
	known := KnownFromBuiltins()
	known["plugin:foo:badge"] = KnownShortcode{
		Name:       "plugin:foo:badge",
		Block:      BlockOptional,
		Attrs:      map[string]DocAttr{},
		Documented: false,
	}

	issues := Lint(`[plugin:foo:badge anything="1"]`, LintOptions{Known: known})
	if got := findIssue(issues, "unknown attribute"); got != nil {
		t.Errorf("undocumented plugin should not flag unknown attrs, got %+v", *got)
	}
}

func TestLintNestedReload(t *testing.T) {
	const src = `[reload]Outer [reload]inner[/reload][/reload]`
	issue := findIssue(Lint(src, LintOptions{Known: KnownFromBuiltins()}), "cannot contain another [reload]")
	if issue == nil {
		t.Fatal("expected a nested-[reload] diagnostic")
	}
	// The renderer refuses this outright, so it is an error, not a suggestion.
	if issue.Severity != SeverityError {
		t.Errorf("severity = %v, want %v", issue.Severity, SeverityError)
	}
	// It must anchor to the inner opener — the one the author has to delete.
	if want := strings.Index(src, "[reload]inner"); issue.Start != want {
		t.Errorf("anchored at %d, want the inner opener at %d", issue.Start, want)
	}
}

func TestLintSiblingReloadsAreFine(t *testing.T) {
	issues := Lint(`[reload]a[/reload] and [reload]b[/reload]`, LintOptions{Known: KnownFromBuiltins()})
	if issue := findIssue(issues, "cannot contain another [reload]"); issue != nil {
		t.Errorf("sibling [reload] blocks flagged as nested: %+v", issue)
	}
}

func TestLintDeferredBlockInsideReloadFace(t *testing.T) {
	for _, name := range []string{"lazy", "details"} {
		src := "[reload][" + name + "]x[/" + name + "][/reload]"
		want := "[" + name + "] cannot be used inside a [reload] button face"
		issue := findIssue(Lint(src, LintOptions{Known: KnownFromBuiltins()}), want)
		if issue == nil {
			t.Errorf("no diagnostic for %s inside a reload face", name)
			continue
		}
		if issue.Severity != SeverityError {
			t.Errorf("%s: severity = %v, want %v", name, issue.Severity, SeverityError)
		}
	}

	// Outside a reload face they are ordinary blocks.
	issues := Lint(`[lazy][reload][/reload][/lazy]`, LintOptions{Known: KnownFromBuiltins()})
	if issue := findIssue(issues, "cannot be used inside a [reload] button face"); issue != nil {
		t.Errorf("[lazy] wrapping a [reload] flagged: %+v", issue)
	}
}

// The linter's known-attribute table is built from BuiltinDocs, so an attribute
// implemented in the handler but never documented is reported as unknown and the
// editor neither completes nor explains it. These pin the [meta] inline family.
func TestLintAcceptsInlineMetaAttributes(t *testing.T) {
	known := KnownFromBuiltins()
	src := `<a href="/x/[meta path='slug' inline='true' format='date' layout='Jan 2, 2006' raw='true' default='n/a' hide-empty='true']">go</a>`
	for _, issue := range Lint(src, LintOptions{Known: known}) {
		if strings.Contains(issue.Message, "unknown attribute") {
			t.Errorf("lint rejected a documented [meta] attribute: %s", issue.Message)
		}
	}
}

func TestLintStillRejectsAnUndocumentedMetaAttribute(t *testing.T) {
	known := KnownFromBuiltins()
	var found bool
	for _, issue := range Lint(`[meta path="x" inlien="true"]`, LintOptions{Known: known}) {
		if strings.Contains(issue.Message, `unknown attribute "inlien"`) {
			found = true
		}
	}
	if !found {
		t.Error("a typo'd attribute should still be reported as unknown")
	}
}

// Escaping keeps a value inside a quoted attribute and does nothing beyond that.
// These are the three contexts where the browser re-parses the value afterwards,
// plus the unquoted case where it never had a boundary to begin with. The
// template author is an admin or editor; the Meta value is written by anyone who
// can edit the entity, so the warning sits across a real privilege boundary.
func TestLintWarnsOnUnsafeInlineMetaContexts(t *testing.T) {
	cases := []struct {
		name, src, want string
	}{
		{"unquoted attribute", `<a href=/x/[meta path='s' inline='true']>x</a>`, "unquoted attribute value"},
		{"event handler", `<a onclick="f('[meta path='s' inline='true']')">x</a>`, "event handler"},
		{"style attribute", `<div style="color:[meta path='s' inline='true']">x</div>`, "style"},
		{"whole href", `<a href="[meta path='s' inline='true']">x</a>`, `whole "href" URL`},
		{"whole src", `<img src="[meta path='s' inline='true']">`, `whole "src" URL`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var found bool
			for _, issue := range Lint(tc.src, LintOptions{Known: KnownFromBuiltins()}) {
				if strings.Contains(issue.Message, tc.want) {
					found = true
				}
			}
			if !found {
				t.Errorf("no warning containing %q for %s", tc.want, tc.src)
			}
		})
	}
}

// The safe shapes must stay quiet, or the warning is noise and gets ignored.
func TestLintIsQuietOnSafeInlineMetaContexts(t *testing.T) {
	for _, src := range []string{
		`<a href="/x/[meta path='s' inline='true']">x</a>`,
		`<a href='/x/[meta path="s" inline="true"]'>x</a>`,
		`<span title="[meta path='s' inline='true']">x</span>`,
		`<div data-status="[meta path='s' inline='true']"></div>`,
		`<p>[meta path="s" inline="true"]</p>`,
		// Not inline: the widget is an element, never an attribute value.
		`<p>[meta path="s"]</p>`,
	} {
		for _, issue := range Lint(src, LintOptions{Known: KnownFromBuiltins()}) {
			if strings.Contains(issue.Message, "[meta inline]") {
				t.Errorf("unexpected warning for %s: %s", src, issue.Message)
			}
		}
	}
}

// The three placements a backwards, quote-unaware scan missed, plus raw=.
// Each is a real way to get script or markup out of a Meta value that an
// ordinary user wrote into an entity an admin's template renders.
func TestLintCatchesTheHardUnsafeContexts(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{
			"a > inside a handler is not the end of the tag",
			`<button onclick="if (x > 0) [meta path='x' inline='true']">x</button>`,
			"event handler",
		},
		{
			"a < inside a handler is not the start of one",
			`<button onclick="if (x < 1) [meta path='x' inline='true']">x</button>`,
			"event handler",
		},
		{
			"srcdoc is parsed as a document, prefix or not",
			`<iframe srcdoc="prefix [meta path='x' inline='true']"></iframe>`,
			"parses as HTML",
		},
		{
			"raw= is unescaped, so any attribute is unsafe",
			`<span title="[meta path='x' inline='true' raw='true']">x</span>`,
			"not escaped at all",
		},
		{
			"a valueless attribute before the target does not hide it",
			`<input disabled href=[meta path='x' inline='true']>`,
			"unquoted attribute value",
		},
		{
			"tabs and newlines between attributes",
			"<a\n\tclass=\"c\"\n\thref=\"[meta path='x' inline='true']\">x</a>",
			`whole "href" URL`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var found bool
			for _, issue := range Lint(tc.src, LintOptions{Known: KnownFromBuiltins()}) {
				if strings.Contains(issue.Message, tc.want) {
					found = true
				}
			}
			if !found {
				t.Errorf("no warning containing %q for %s", tc.want, tc.src)
			}
		})
	}
}

// Text outside any tag, and a value that merely follows a tag, must stay quiet.
func TestLintIsQuietOutsideAttributeValues(t *testing.T) {
	for _, src := range []string{
		`<p>[meta path="x" inline="true"]</p>`,
		`<div class="c">before [meta path="x" inline="true"] after</div>`,
		`plain text [meta path="x" inline="true"] more text`,
		`<img src="/a.png"> [meta path="x" inline="true"]`,
		`<!-- <a href=x> --> [meta path="x" inline="true"]`,
		`</div>[meta path="x" inline="true"]`,
	} {
		for _, issue := range Lint(src, LintOptions{Known: KnownFromBuiltins()}) {
			if strings.Contains(issue.Message, "[meta inline") {
				t.Errorf("unexpected warning for %s: %s", src, issue.Message)
			}
		}
	}
}
