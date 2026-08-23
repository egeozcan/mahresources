package shortcodes

import (
	"errors"
	"math/rand"
	"strings"
	"testing"

	"golang.org/x/net/html"
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
		{"whole href", `<a href="[meta path='s' inline='true']">x</a>`, `choose the scheme of the "href" URL`},
		{"whole src", `<img src="[meta path='s' inline='true']">`, `choose the scheme of the "src" URL`},
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
			`choose the scheme of the "href" URL`,
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

// Round-3 findings: a raw-text element body is not markup, a non-empty URL
// prefix does not necessarily fix the scheme, and "<" followed by a space is
// text rather than a tag.
func TestLintScannerHandlesRawTextAndPartialSchemes(t *testing.T) {
	warns := func(src, want string) bool {
		for _, issue := range Lint(src, LintOptions{Known: KnownFromBuiltins()}) {
			if strings.Contains(issue.Message, want) {
				return true
			}
		}
		return false
	}

	// A "<" inside a <script> body must not open an attribute value that
	// swallows the next real tag.
	src := `<script>const x = '<x a="';</script><button onclick="[meta path='x' inline='true']">x</button>`
	if !warns(src, "event handler") {
		t.Error("a fake tag inside <script> desynchronized the scan and hid a real handler")
	}

	// A prefix that has not yet decided the scheme still lets the value pick it.
	if !warns(`<a href="java[meta path='x' inline='true']">x</a>`, "choose the scheme") {
		t.Error(`href="java[meta ...]" completes to javascript: and must warn`)
	}
	// A prefix that has decided it must stay quiet.
	for _, quiet := range []string{
		`<a href="/x/[meta path='s' inline='true']">x</a>`,
		`<a href="https://example.com/[meta path='s' inline='true']">x</a>`,
		`<a href="?q=[meta path='s' inline='true']">x</a>`,
		`<a href="#[meta path='s' inline='true']">x</a>`,
	} {
		if warns(quiet, "choose the scheme") {
			t.Errorf("scheme already fixed, should be quiet: %s", quiet)
		}
	}

	// "< " is text in HTML, not a tag.
	if warns(`plain < onclick="[meta path='x' inline='true']" text`, "event handler") {
		t.Error(`"< " is text; the scanner treated it as a tag`)
	}
}

// One scan serves every bare value in a template. The in-attribute placement
// was linear from the start, because the tokenizer hands each tag's occurrences
// back together; the ones that resolve through the *fallback* scan — a value in
// ordinary text, and the same value in a CSS slot — were the quadratic pair,
// each costing a search of the whole document plus a rescan to EOF. This is the
// placement an author writes most often, and lint runs on every debounced
// keystroke in the template editor.
//
// Correctness at that scale is all this asserts, and the name says so: it
// passes against the quadratic shape too, which merely takes 25x longer.
// A wall-clock assertion is the one thing that would make it flaky. What pins
// the rewritten scan is TestUnterminatedTagScanMatchesThePerOffsetScan below;
// the linearity itself is structural, one pass with the offsets already known.
func TestLintManyBareValuesAtScale(t *testing.T) {
	const n = 2000

	t.Run("in ordinary text", func(t *testing.T) {
		src := strings.Repeat(`[property path="Name"] and a little filler. `, n)
		for _, issue := range Lint(src, LintOptions{Known: KnownFromBuiltins()}) {
			t.Fatalf("escaped text is a safe context, got: %s", issue.Message)
		}
	})

	t.Run("in a CSS slot", func(t *testing.T) {
		src := strings.Repeat(`.c{color:[property path="Name"]}`, n)
		got := 0
		for _, issue := range Lint(src, LintOptions{Known: KnownFromBuiltins(), CSSMode: true}) {
			if !strings.Contains(issue.Message, "CSS slot") {
				t.Fatalf("unexpected issue: %s", issue.Message)
			}
			got++
		}
		if got != n {
			t.Fatalf("got %d CSS-slot warnings, want %d", got, n)
		}
	})

	t.Run("in a safe attribute", func(t *testing.T) {
		body := strings.Repeat(`[meta path='x' inline='true']`, n)
		src := `<a title="` + body + `">x</a>`
		for _, issue := range Lint(src, LintOptions{Known: KnownFromBuiltins()}) {
			if strings.Contains(issue.Message, "[meta inline") {
				t.Fatalf("title= is a safe context, got: %s", issue.Message)
			}
		}
	})
}

// The unterminated-tag test used to run once per occurrence, each time locating
// the sentinel in the whole document, walking back to the nearest "<" and then
// forward to EOF. It is now one left-to-right pass that answers for every
// occurrence at once. That old shape is short enough to hold in the head, so it
// serves here as the specification the new one is held to, over hand-picked
// shapes and randomized ones, at every byte offset.
//
// Specification of what the code did, not of what is ideal. It restarts its
// quote state at the nearest "<" even when that "<" is inside a quoted value,
// so `<div title="a <b">[property path="Name"]</div>` is reported as an
// unterminated tag when it is nothing of the sort. That predates this rewrite,
// which had to preserve it exactly; the differential is what says so.
func TestUnterminatedTagScanMatchesThePerOffsetScan(t *testing.T) {
	// The pre-rewrite algorithm, verbatim apart from taking an offset instead of
	// searching for a sentinel.
	reference := func(s string, at int) bool {
		open := strings.LastIndexByte(s[:at], '<')
		if open < 0 {
			return false
		}
		if open+1 >= len(s) || !isTagNameStart(s[open+1]) {
			return false
		}
		var q byte
		for i := open; i < len(s); i++ {
			c := s[i]
			switch {
			case q != 0:
				if c == q {
					q = 0
				}
			case c == '"' || c == '\'':
				q = c
			case c == '>':
				return false
			}
		}
		return true
	}

	// Every offset, len(s) included: an offset at the very end is a legitimate
	// question the old shape answered, even though a recorded sentinel start is
	// always followed by the sentinel's own bytes. Sparse subsets go with it,
	// because a real call asks about a handful of offsets, and they are what
	// exercises the early exit once nothing is left pending.
	check := func(t *testing.T, s string) {
		t.Helper()
		verify := func(at []int) {
			got := insideUnterminatedTags(s, at)
			for k, off := range at {
				if want := reference(s, off); got[k] != want {
					t.Fatalf("offset %d of %q (asked %v): got %v, want %v", off, s, at, got[k], want)
				}
			}
		}
		all := make([]int, len(s)+1)
		for i := range all {
			all[i] = i
		}
		verify(all)
		for _, step := range []int{2, 3, 7} {
			var sparse []int
			for i := 0; i < len(all); i += step {
				sparse = append(sparse, all[i])
			}
			verify(sparse)
		}
		for _, one := range all {
			verify([]int{one})
		}
	}

	for _, s := range []string{
		``,
		`<`,
		`<a`,
		`plain text`,
		`Score < 5 and more`,
		`<div title="x">after</div>`,
		`<div title="before > still open`,
		`<div title='mixed " quoting' >done`,
		`<a href="/x/">one</a><b class="y`,
		// A "<" inside a quoted value is still the nearest one, and the forward
		// scan restarts its quote state there.
		`<div title="a <b" > tail`,
		`<div x=y> a=" </div>`,
		`<!-- > <x a=" --> <button onclick="f()">`,
		`</`,
		`<//a b="`,
		`<a b='c"d' e="f'g" >`,
	} {
		check(t, s)
	}

	rng := rand.New(rand.NewSource(20260823))
	alphabet := []byte(`<>"'/= abx!-`)
	for n := 0; n < 2000; n++ {
		buf := make([]byte, rng.Intn(28))
		for i := range buf {
			buf[i] = alphabet[rng.Intn(len(alphabet))]
		}
		check(t, string(buf))
	}
}

// Round-4 findings, all of which came from the scanner trying to find tag
// boundaries itself. Tag finding is now golang.org/x/net/html's job.
func TestLintScannerHandlesTheCasesOnlyAParserGets(t *testing.T) {
	warns := func(src, want string) bool {
		for _, issue := range Lint(src, LintOptions{Known: KnownFromBuiltins()}) {
			if strings.Contains(issue.Message, want) {
				return true
			}
		}
		return false
	}
	must := func(t *testing.T, src, want string) {
		t.Helper()
		if !warns(src, want) {
			t.Errorf("no warning containing %q for %s", want, src)
		}
	}
	quiet := func(t *testing.T, src, notWant string) {
		t.Helper()
		if warns(src, notWant) {
			t.Errorf("unexpected %q warning for %s", notWant, src)
		}
	}

	t.Run("a comment ends at --> not at the first >", func(t *testing.T) {
		must(t, `<!-- > <x a=" --> <button onclick="[meta path='x' inline='true']">go</button>`, "event handler")
	})
	t.Run("</scripture> does not close a script", func(t *testing.T) {
		must(t, `<script>const x='</scripture><x a="';</script><button onclick="[meta path='x' inline='true']">go</button>`, "event handler")
	})
	t.Run("an unquoted value ends at >", func(t *testing.T) {
		must(t, `<div data-x=y> a=" </div><button onclick="[meta path='x' inline='true']">go</button>`, "event handler")
	})
	t.Run("an executable scheme is not a safe prefix", func(t *testing.T) {
		must(t, `<a href="javascript:[meta path='x' inline='true']">go</a>`, "executes rather than fetches")
		must(t, `<a href="data:text/html,[meta path='x' inline='true']">go</a>`, "executes rather than fetches")
	})
	t.Run("a character reference is decoded before the scheme is judged", func(t *testing.T) {
		must(t, `<a href="java&#x73;cript[meta path='x' inline='true']">go</a>`, "choose the scheme")
	})
	t.Run(`"on" alone is not an event handler`, func(t *testing.T) {
		quiet(t, `<div on="[meta path='x' inline='true']"></div>`, "event handler")
	})
	t.Run("a shortcode inside a comment is not in an attribute", func(t *testing.T) {
		quiet(t, `<!-- <a href="[meta path='x' inline='true']"> -->`, "[meta inline")
	})
	t.Run("uppercase tags and attributes", func(t *testing.T) {
		must(t, `<A HREF="[meta path='x' inline='true']">go</A>`, "choose the scheme")
	})
	t.Run("inside a textarea body", func(t *testing.T) {
		quiet(t, `<textarea>[meta path='x' inline='true']</textarea>`, "[meta inline")
	})
	t.Run("two shortcodes in one tag", func(t *testing.T) {
		src := `<a href="/x/[meta path='a' inline='true']" onclick="f('[meta path='b' inline='true']')">go</a>`
		must(t, src, "event handler")
		quiet(t, src, "choose the scheme")
	})
}

// Round-5 findings. Four were ways the warning went quiet; three were noise.
func TestLintAttributeContextRoundFive(t *testing.T) {
	warns := func(src, want string) bool {
		for _, issue := range Lint(src, LintOptions{Known: KnownFromBuiltins()}) {
			if strings.Contains(issue.Message, want) {
				return true
			}
		}
		return false
	}

	t.Run("an unterminated tag fails closed", func(t *testing.T) {
		// The tokenizer never sees a tag here, so the occurrence is unresolved.
		// Reporting "not in an attribute" would be silence on the most broken
		// template there is.
		if !warns(`<div title="[meta path='x' inline='true' raw='true']`, "never closed") {
			t.Error("an occurrence in an unclosed tag must warn, not fail open")
		}
	})

	t.Run("literal sentinel bytes cannot remap an occurrence", func(t *testing.T) {
		src := "<button onclick=\"[meta path='x' inline='true']\"></button>" +
			"<i title=\"[meta path='\x00mahlint0\x00' inline='true']\"></i>"
		if !warns(src, "event handler") {
			t.Error("a template carrying the sentinel bytes stole the first occurrence's context")
		}
	})

	t.Run("a slash separates attribute names", func(t *testing.T) {
		if !warns(`<div x/onclick="[meta path='x' inline='true']">`, "event handler") {
			t.Error(`x/onclick is two attributes; the handler was missed`)
		}
	})

	t.Run("control characters do not hide an executable scheme", func(t *testing.T) {
		if !warns(`<a href="java&#9;script:[meta path='x' inline='true']">go</a>`, "executes rather than fetches") {
			t.Error("browsers strip tab from a URL; the scheme is javascript:")
		}
	})

	t.Run("a prefix no executable scheme starts with is quiet", func(t *testing.T) {
		if warns(`<a href="https[meta path='x' inline='true']">go</a>`, "choose the scheme") {
			t.Error(`nothing appended to "https" makes an executable scheme`)
		}
		if !warns(`<a href="java[meta path='x' inline='true']">go</a>`, "choose the scheme") {
			t.Error(`"java" still completes to javascript:`)
		}
	})

	t.Run("a duplicate attribute never reaches the page", func(t *testing.T) {
		if warns(`<a href="/safe" href="[meta path='x' inline='true']">go</a>`, "[meta inline") {
			t.Error("the parser keeps the first href and drops the second")
		}
	})
}

// Round-6 findings. Both are realistic in this app specifically: it wraps every
// entity-bound slot in an Alpine x-data scope and its own docs recommend
// directives for reading Meta client-side.
func TestLintAttributeContextRoundSix(t *testing.T) {
	warns := func(src, want string) bool {
		for _, issue := range Lint(src, LintOptions{Known: KnownFromBuiltins()}) {
			if strings.Contains(issue.Message, want) {
				return true
			}
		}
		return false
	}

	t.Run("Alpine directives evaluate their value as script", func(t *testing.T) {
		for _, src := range []string{
			`<button x-on:click="f('[meta path='x' inline='true']')">go</button>`,
			`<button @click="f('[meta path='x' inline='true']')">go</button>`,
			`<div x-init="s = '[meta path='x' inline='true']'"></div>`,
			`<a :href="'/x/' + '[meta path='x' inline='true']'">go</a>`,
			`<div x-text="'[meta path='x' inline='true']'"></div>`,
		} {
			if !warns(src, "Alpine directive") {
				t.Errorf("no Alpine warning for %s", src)
			}
		}
	})

	t.Run("a colon inside a name is not an Alpine shorthand", func(t *testing.T) {
		// xlink:href is a URL attribute, not a directive.
		if warns(`<a xlink:href="/x/[meta path='x' inline='true']">go</a>`, "Alpine directive") {
			t.Error("xlink:href was read as an Alpine binding")
		}
	})

	t.Run("interpolating a name is undelimited", func(t *testing.T) {
		if !warns(`<div data-[meta path='k' inline='true']="safe"></div>`, "attribute NAME") {
			t.Error("an interpolated attribute name must warn")
		}
		if !warns(`<[meta path='k' inline='true'] class="c">x</div>`, "attribute NAME") {
			t.Error("an interpolated element name must warn")
		}
	})

	t.Run("a > inside a quoted value does not close the tag", func(t *testing.T) {
		if !warns(`<div title="before > [meta path='x' inline='true' raw='true']`, "never closed") {
			t.Error("the tag is still unterminated; the > is inside the value")
		}
	})
}

// Round-7 findings. The script-body case inverts the intuition the earlier
// rounds built: a raw-text body is where escaping helps *least*, because the
// parser decodes no entities there at all.
func TestLintAttributeContextRoundSeven(t *testing.T) {
	warns := func(src, want string) bool {
		for _, issue := range Lint(src, LintOptions{Known: KnownFromBuiltins()}) {
			if strings.Contains(issue.Message, want) {
				return true
			}
		}
		return false
	}

	t.Run("a script body reaches JavaScript with its escaping in it", func(t *testing.T) {
		// A template literal makes this concrete: "${...}" contains not one
		// character html.EscapeString touches. Not verbatim — a quote does
		// arrive as &#34; and stays that way, since nothing decodes it — but
		// escaped is not the same as safe in a language.
		src := "<script>const label = `[meta path=\"label\" inline=\"true\"]`;</script>"
		if !warns(src, "<script> body") {
			t.Error("an inline value in a script body must warn")
		}
	})

	t.Run("a style body reaches CSS the same way", func(t *testing.T) {
		src := `<style>.card{color:[meta path="colour" inline="true"]}</style>`
		if !warns(src, "<style> body") {
			t.Error("an inline value in a style body must warn")
		}
	})

	t.Run("textarea and title decode entities, so escaping works there", func(t *testing.T) {
		for _, src := range []string{
			`<textarea>[meta path="x" inline="true"]</textarea>`,
			`<title>[meta path="x" inline="true"]</title>`,
		} {
			if warns(src, "body") {
				t.Errorf("RCDATA body is safe, should be quiet: %s", src)
			}
		}
	})

	t.Run("Alpine directives that take a literal are not expressions", func(t *testing.T) {
		for _, attr := range []string{"x-ref", "x-cloak", "x-ignore", "x-teleport"} {
			src := `<div ` + attr + `="[meta path='k' inline='true']"></div>`
			if warns(src, "Alpine directive") {
				t.Errorf("%s takes a literal, not an expression: %s", attr, src)
			}
		}
		// The evaluating ones still warn.
		if !warns(`<div x-text="[meta path='k' inline='true']"></div>`, "Alpine directive") {
			t.Error("x-text evaluates its value")
		}
	})
}

// Round-8 findings. Both majors are about context the earlier rounds never
// looked at: raw= outside an attribute, and a slot that is CSS with nothing in
// its own text to say so.
func TestLintAttributeContextRoundEight(t *testing.T) {
	warnsWith := func(src string, opts LintOptions, want string) bool {
		opts.Known = KnownFromBuiltins()
		for _, issue := range Lint(src, opts) {
			if strings.Contains(issue.Message, want) {
				return true
			}
		}
		return false
	}
	warns := func(src, want string) bool { return warnsWith(src, LintOptions{}, want) }

	t.Run("raw in plain text injects markup", func(t *testing.T) {
		src := `<div class="bio">[meta path="bio" inline="true" raw="true"]</div>`
		if !warns(src, "becomes real elements") {
			t.Error("raw= is unescaped wherever it lands, text included")
		}
		// Without raw, plain text is genuinely safe.
		if warns(`<div class="bio">[meta path="bio" inline="true"]</div>`, "[meta inline") {
			t.Error("escaped text is safe and must stay quiet")
		}
	})

	t.Run("a CSS slot lands in a stylesheet", func(t *testing.T) {
		src := `.badge{color:[meta path="colour" inline="true"]}`
		if !warnsWith(src, LintOptions{CSSMode: true}, "CSS slot") {
			t.Error("a CustomCSS slot has no <style> wrapper; the mode has to say so")
		}
		// The same text in an HTML slot is ordinary text.
		if warns(src, "[meta inline") {
			t.Error("without CSS mode this is plain text")
		}
	})

	t.Run("a less-than in prose is not an unclosed tag", func(t *testing.T) {
		if warns(`Score < [meta path="limit" inline="true"]`, "never closed") {
			t.Error(`"< " is text`)
		}
	})

	t.Run("Alpine transition modifiers take literals", func(t *testing.T) {
		for _, attr := range []string{"x-transition:enter", "x-transition.opacity", "x-ref"} {
			src := `<div ` + attr + `="[meta path='c' inline='true']"></div>`
			if warns(src, "Alpine directive") {
				t.Errorf("%s takes a literal: %s", attr, src)
			}
		}
	})
}

// Round-9 findings. The rule was never really about [meta inline]: it is about
// a shortcode writing a value straight into the surrounding markup, and
// [property], [item] and [mrql value=] do exactly that.
func TestLintCoversEveryBareValueShortcode(t *testing.T) {
	warns := func(src, want string) bool {
		for _, issue := range Lint(src, LintOptions{Known: KnownFromBuiltins()}) {
			if strings.Contains(issue.Message, want) {
				return true
			}
		}
		return false
	}

	t.Run("property with raw injects markup", func(t *testing.T) {
		// This shape is in the project's own reference panel.
		if !warns(`<div>[property path="Description" raw="true"]</div>`, "becomes real elements") {
			t.Error(`[property raw="true"] in text must warn`)
		}
	})

	t.Run("item with raw injects markup", func(t *testing.T) {
		src := `[each path="credits"]<span>[item path="name" raw="true"]</span>[/each]`
		if !warns(src, "becomes real elements") {
			t.Error(`[item raw="true"] must warn`)
		}
	})

	t.Run("the placement rules apply to every bare-value shortcode", func(t *testing.T) {
		for _, src := range []string{
			`<button onclick="f('[property path='Name']')">go</button>`,
			`<a href="[property path='URL']">go</a>`,
			`<script>var s = "[property path='Name']";</script>`,
		} {
			if !warns(src, "[property]") {
				t.Errorf("no placement warning for %s", src)
			}
		}
		if !warns(`<button onclick="f('[mrql query="type = group" value="count"]')">go</button>`, "[mrql]") {
			t.Error("[mrql value=] emits a bare value too")
		}
	})

	t.Run("escaped output in plain text stays quiet", func(t *testing.T) {
		for _, src := range []string{
			`<div>[property path="Name"]</div>`,
			`[each path="c"]<span>[item path="n"]</span>[/each]`,
			`<div>[meta path="x" inline="true"]</div>`,
		} {
			if warns(src, "real elements") {
				t.Errorf("escaped text is safe: %s", src)
			}
		}
	})

	t.Run("x-id evaluates its value", func(t *testing.T) {
		if !warns(`<div x-id="['panel-[meta path='slug' inline='true']']"></div>`, "Alpine directive") {
			t.Error("x-id takes an array expression, not a literal")
		}
	})
}

// [meta]'s documented attribute set gained raw, format and layout, which took
// away the `unknown attribute "raw" on [meta]` diagnostic they used to draw.
// Only RenderMetaShortcode's inline branch reads them, so without inline="true"
// they now do nothing and nothing says so.
func TestLintMetaAttributesThatOnlyInlineReads(t *testing.T) {
	issue := func(src, want string) *LintIssue {
		return findIssue(Lint(src, LintOptions{Known: KnownFromBuiltins()}), want)
	}

	t.Run("raw, format and layout are inert without inline", func(t *testing.T) {
		for _, tc := range []struct{ src, want string }{
			{`[meta path="x" raw="true"]`, `raw="true" without inline="true"`},
			{`[meta path="x" format="date"]`, `format= without inline="true"`},
			{`[meta path="x" layout="Jan 2, 2006"]`, `layout= without inline="true"`},
			{`[meta path="x" inline="false" raw="true"]`, `raw="true" without inline="true"`},
		} {
			got := issue(tc.src, tc.want)
			if got == nil {
				t.Errorf("no warning containing %q for %s", tc.want, tc.src)
				continue
			}
			if got.Severity != SeverityWarning {
				t.Errorf("%s: severity %q, want %q", tc.src, got.Severity, SeverityWarning)
			}
		}
	})

	t.Run("with inline they are read, so they stay quiet", func(t *testing.T) {
		for _, src := range []string{
			`[meta path="x" inline="true" raw="true"]`,
			`[meta path="x" inline="true" format="date"]`,
			`[meta path="x" inline="true" layout="Jan 2, 2006"]`,
		} {
			if got := issue(src, `without inline="true"`); got != nil {
				t.Errorf("%s: unexpected %q", src, got.Message)
			}
		}
	})

	t.Run("editable is the other way round", func(t *testing.T) {
		got := issue(`[meta path="x" inline="true" editable="true"]`, "editable is ignored")
		if got == nil {
			t.Error(`inline="true" renders the bare value, so there is no editor to turn on`)
		} else if got.Severity != SeverityWarning {
			t.Errorf("severity %q, want %q", got.Severity, SeverityWarning)
		}
		if got := issue(`[meta path="x" editable="true"]`, "editable is ignored"); got != nil {
			t.Errorf("editable alone is the widget's whole point: %s", got.Message)
		}
	})

	t.Run("a no-op the author asked for stays quiet", func(t *testing.T) {
		// raw="false" is what the default already is, so saying it changes
		// nothing whether inline is set or not. Warning there would be noise.
		if got := issue(`[meta path="x" raw="false"]`, `without inline="true"`); got != nil {
			t.Errorf("unexpected %q", got.Message)
		}
	})
}

// The tokenizer raw-texts ten elements and scriptLikeElements lists two, so a
// tag written inside one of the other eight is never emitted as a tag. For six
// of them, in the HTML namespace, the value in the body is analysed as ordinary
// prose and that is the right answer rather than a hole — twice it was treated
// as one, first by labelling all eight bodies unreadable and then by re-reading
// a <noscript> body as markup and reporting placements in it that cannot
// execute. The argument is in scriptLikeElements' comment; these pin it, so the
// next reader finds tests rather than only prose.
//
// The two that are re-read — <noscript>, and any of the ten inside <svg> or
// <math> — have their own tests in TestLintNoscriptBodyPlacement and
// TestLintForeignContentPlacement.
func TestLintRawTextElementBodies(t *testing.T) {
	warns := func(src, want string) bool {
		for _, issue := range Lint(src, LintOptions{Known: KnownFromBuiltins()}) {
			if strings.Contains(issue.Message, want) {
				return true
			}
		}
		return false
	}

	t.Run("no script in a noscript body can execute, under either parse", func(t *testing.T) {
		// With scripting on the body is inert raw text; with scripting off the
		// tags are real and no script runs. So every *executable* placement is
		// a false positive there, which is what re-reading the body as markup
		// produced. The href below is not one of those: it is a live link with
		// scripting off, and its silence is the URL-bearing residue named in
		// scriptLikeElements' comment, accepted on the same terms.
		for _, src := range []string{
			`<noscript>[property path="Name"]</noscript>`,
			`<noscript><div class="c">[meta path='x' inline='true']</div></noscript>`,
			`<noscript><a href="[meta path='x' inline='true']">go</a></noscript>`,
			`<noscript><button onclick="f('[meta path='x' inline='true']')">go</button></noscript>`,
			`<noscript><script>var s = "[property path='Name']";</script></noscript>`,
		} {
			for _, issue := range Lint(src, LintOptions{Known: KnownFromBuiltins()}) {
				t.Errorf("no script runs in a <noscript> body, got %q for %s", issue.Message, src)
			}
		}
		// raw= is the one that still matters: an unescaped value can close the
		// element and continue in markup whichever way the body was read.
		if !warns(`<noscript>[property path="Name" raw="true"]</noscript>`, "becomes real elements") {
			t.Error("a raw value in a noscript body must still warn")
		}
	})

	t.Run("the unconditional raw-text bodies contain an escaped value", func(t *testing.T) {
		// These five are raw text in an HTML parser whatever the scripting flag
		// says, so a tag inside one is text to the browser as well and "not in
		// an attribute" is the truth rather than a gap. An escaped value cannot
		// close the element either: html.EscapeString leaves no "<", and no
		// entity is decoded there to give one back.
		for _, el := range []string{"iframe", "noembed", "noframes", "plaintext", "xmp"} {
			src := "<" + el + `><a href="[meta path='x' inline='true']">go</a></` + el + ">"
			if warns(src, "[meta inline") {
				t.Errorf("an escaped value in <%s> is contained: %s", el, src)
			}
		}
	})

	t.Run("a raw value in a closable one is the raw rule's business", func(t *testing.T) {
		// "</xmp>" is exactly how a raw value gets back out into markup, and
		// the answer — drop raw= — is the one that rule already gives.
		// plaintext is left out: it runs to EOF, so a raw value there cannot
		// escape either, and the warning it draws is the raw rule being
		// conservative rather than anything this classification decided.
		for _, el := range []string{"iframe", "noembed", "noframes", "xmp"} {
			src := "<" + el + `>[meta path='x' inline='true' raw='true']</` + el + ">"
			if !warns(src, "becomes real elements") {
				t.Errorf("a raw value in <%s> must warn: %s", el, src)
			}
		}
	})

	t.Run("script and style keep their own language", func(t *testing.T) {
		if !warns(`<script>var s = "[meta path='x' inline='true']";</script>`, "reaches JavaScript") {
			t.Error("a script body still names JavaScript")
		}
		if !warns(`<style>.c{color:[meta path='x' inline='true']}</style>`, "reaches CSS") {
			t.Error("a style body still names CSS")
		}
	})

	t.Run("an unclosed one swallows the document, and so does a browser's", func(t *testing.T) {
		// The rest of the slot stops being analysed, which looks like the worst
		// kind of silence — a typo turning off the check for everything after
		// it. It is not, because a browser's tokenizer runs to EOF in raw text
		// (or RCDATA, or PLAINTEXT) too: the href below is not a link there
		// either. <script> is the one that still speaks, because it is listed
		// and its message keeps applying to every value after it.
		//
		// <noscript> is deliberately not in this list, and no longer for the
		// same reason: with scripting disabled an unclosed one is an ordinary
		// open element and everything after it IS live markup, so it is read
		// rather than passed over. TestLintNoscriptBodyPlacement covers it.
		tail := `<a href="javascript:[meta path='x' inline='true']">go</a>` +
			`<div style="color:[meta path='y' inline='true']">x</div>`
		for _, opener := range []string{"<iframe>", "<noembed>", "<noframes>", "<xmp>", "<plaintext>", "<textarea>", "<title>"} {
			for _, issue := range Lint(opener+tail, LintOptions{Known: KnownFromBuiltins()}) {
				t.Errorf("after an unclosed %s nothing is markup, got %q", opener, issue.Message)
			}
		}
		if !warns("<script>"+tail, "reaches JavaScript") {
			t.Error("an unclosed <script> still takes every value after it into JavaScript")
		}
		// The value that is still dangerous after a closable one is a raw one,
		// which can write "</xmp>" and continue in markup. Not after
		// <plaintext>, which nothing closes — the warning there is the raw=
		// rule being conservative, as it is for a closed <plaintext> body.
		if !warns(`<xmp>[property path="Name" raw="true"]`, "becomes real elements") {
			t.Error("raw= is unescaped wherever the element ends")
		}
	})

	t.Run("RCDATA bodies stay quiet, tags inside them included", func(t *testing.T) {
		// textarea and title are raw-tagged by the tokenizer too, but their
		// bodies decode entities, so an escaped value is inert there — and the
		// <a href> inside is literal text to the browser as well, exactly as the
		// zero-value context reports.
		for _, el := range []string{"textarea", "title"} {
			src := "<" + el + `><a href="[meta path='x' inline='true']">go</a></` + el + ">"
			if warns(src, "[meta inline") {
				t.Errorf("<%s> is RCDATA and safe: %s", el, src)
			}
		}
	})
}

// lintWarnings is the message list for one source, for the placement tests
// below, which care about what was said rather than where.
func lintWarnings(src string) []string {
	var out []string
	for _, issue := range Lint(src, LintOptions{Known: KnownFromBuiltins()}) {
		out = append(out, issue.Message)
	}
	return out
}

func lintSaysAny(src, want string) bool {
	for _, msg := range lintWarnings(src) {
		if strings.Contains(msg, want) {
			return true
		}
	}
	return false
}

// Inside <svg> and <math> the tokenizer's ten raw-text names carry no such
// meaning, which the tokenizer cannot know because it is namespace-unaware.
// Two things follow, and both were wrong before: a <script> there does decode
// entities, so the escaping is undone rather than preserved; and an <iframe>
// there is an inert SVG element whose children are real, so the link inside it
// is a live link. The negative half of this test is the important half — the
// value in an ordinary <svg><text> or <svg><title> must stay silent, and so
// must anything after an HTML element has broken the parser out of foreign
// content.
func TestLintForeignContentPlacement(t *testing.T) {
	const decodesEntities = "which is foreign content, where the parser decodes entities"
	const htmlRawText = "does not decode entities"

	t.Run("a script or style in foreign content has its escaping undone", func(t *testing.T) {
		for _, tc := range []struct{ src, lang string }{
			{`<svg><script>var s = "[property path='Name']"</script></svg>`, "JavaScript"},
			{`<svg><style>.c{color:"[property path='Name']"}</style></svg>`, "CSS"},
			{`<math><script>var s = "[property path='Name']"</script></math>`, "JavaScript"},
		} {
			if !lintSaysAny(tc.src, decodesEntities) {
				t.Errorf("%s: want the foreign-content rationale, got %q", tc.src, lintWarnings(tc.src))
			}
			if lintSaysAny(tc.src, htmlRawText) {
				t.Errorf("%s: entities ARE decoded there, so the HTML rationale is wrong", tc.src)
			}
			if !lintSaysAny(tc.src, tc.lang) {
				t.Errorf("%s: the message still names %s", tc.src, tc.lang)
			}
		}
	})

	t.Run("an integration point puts HTML rules back", func(t *testing.T) {
		// Inside these the children are parsed with HTML rules again, so a
		// <script> there is an HTML script and its body decodes nothing.
		for _, src := range []string{
			`<svg><foreignObject><script>var s = "[property path='Name']"</script></foreignObject></svg>`,
			`<svg><desc><script>var s = "[property path='Name']"</script></desc></svg>`,
			`<svg><title><script>var s = "[property path='Name']"</script></title></svg>`,
			`<math><mtext><script>var s = "[property path='Name']"</script></mtext></math>`,
			`<math><annotation-xml encoding="text/html"><script>var s = "[property path='Name']"</script></annotation-xml></math>`,
			`<svg></svg><script>var s = "[property path='Name']"</script>`,
		} {
			if !lintSaysAny(src, htmlRawText) {
				t.Errorf("%s: want the HTML <script> rationale, got %q", src, lintWarnings(src))
			}
		}
		// annotation-xml without an HTML encoding is not one, so its script is
		// still MathML and still decodes.
		src := `<math><annotation-xml><script>var s = "[property path='Name']"</script></annotation-xml></math>`
		if !lintSaysAny(src, decodesEntities) {
			t.Errorf("%s: want the foreign-content rationale, got %q", src, lintWarnings(src))
		}
	})

	t.Run("the other raw-text names are ordinary elements in foreign content", func(t *testing.T) {
		// The tokenizer swallows each of these bodies as raw text and the value
		// in them was therefore analysed as prose. A browser reads real
		// elements there, and an SVG <a href="javascript:…"> runs.
		for _, src := range []string{
			`<svg><iframe><a href="javascript:[property path='Name']">go</a></iframe></svg>`,
			`<svg><textarea><a href="javascript:[property path='Name']">go</a></textarea></svg>`,
			`<svg><xmp><a href="javascript:[property path='Name']">go</a></xmp></svg>`,
			`<svg><noscript><a href="javascript:[property path='Name']">go</a></noscript></svg>`,
			`<svg><g><textarea><a href="javascript:[property path='Name']">go</a></textarea></g></svg>`,
			`<math><noembed><a href="javascript:[property path='Name']">go</a></noembed></math>`,
			// "/>" really self-closes in foreign content, so what follows a
			// <script/> there is a sibling rather than a script body.
			`<svg><script/><a href="javascript:[property path='Name']">go</a></svg>`,
			// <svg><title> is an integration point, so the anchor in it is a
			// real HTML anchor.
			`<svg><title><a href="javascript:[property path='Name']">go</a></title></svg>`,
		} {
			if !lintSaysAny(src, `continues a "javascript:" URL`) {
				t.Errorf("%s: a live link in foreign content, got %q", src, lintWarnings(src))
			}
		}
	})

	t.Run("ordinary foreign markup stays silent", func(t *testing.T) {
		// This is the half that matters. An icon with a label in it is the
		// realistic template, and there is nothing wrong with any of these.
		for _, src := range []string{
			`<svg viewBox="0 0 24 24"><text>[property path="Name"]</text></svg>`,
			`<svg><title>[property path="Name"]</title></svg>`,
			`<svg><desc>[property path="Name"]</desc></svg>`,
			`<svg><iframe><text>[property path="Name"]</text></iframe></svg>`,
			`<svg><text class="c">[property path="Name"]</text></svg>`,
			`<svg><a href="/x/[property path='Name']">go</a></svg>`,
			`<svg><foreignObject><div class="c">[property path="Name"]</div></foreignObject></svg>`,
		} {
			if got := lintWarnings(src); len(got) != 0 {
				t.Errorf("%s: nothing is wrong here, got %q", src, got)
			}
		}
	})

	t.Run("an HTML element breaks the parser out of foreign content", func(t *testing.T) {
		// A browser leaves foreign content at any of ~40 HTML tag names, so
		// the <textarea> below is HTML RCDATA and the anchor written in it is
		// literal text. Reading it as foreign content would warn on a template
		// whose only fault is an <svg> the author forgot to close.
		for _, src := range []string{
			`<svg><div><textarea><a href="javascript:[property path='Name']">go</a></textarea></div></svg>`,
			`<svg><div>hi</div><textarea><a href="javascript:[property path='Name']">go</a></textarea>`,
			`<svg><font color="red"><textarea><a href="javascript:[property path='Name']">go</a></textarea></font></svg>`,
			`<svg><p><iframe><a href="javascript:[property path='Name']">go</a></iframe></p></svg>`,
			// Past the </svg> the rules are HTML's again.
			`<svg><path d="M0 0"/></svg><textarea><a href="javascript:[property path='Name']">go</a></textarea>`,
		} {
			if got := lintWarnings(src); len(got) != 0 {
				t.Errorf("%s: a browser is out of foreign content here, got %q", src, got)
			}
		}
		// <font> only breaks out when it carries one of three attributes.
		src := `<svg><font><textarea><a href="javascript:[property path='Name']">go</a></textarea></font></svg>`
		if !lintSaysAny(src, `continues a "javascript:" URL`) {
			t.Errorf("%s: a bare <font> stays in foreign content, got %q", src, lintWarnings(src))
		}
	})
}

// With scripting disabled a <noscript> body is real markup — which is the mode
// the element exists for — so a placement in it that needs no script to hurt is
// reachable. The tokenizer raw-texts the body whatever the scripting flag says,
// so none of it was analysed at all. What must NOT come back is the execution
// set: an on* handler, an Alpine directive, a javascript: URL and a <script>
// body are inapplicable under BOTH readings, and warning on them is the false
// positive that made an earlier attempt at this withdrawn.
func TestLintNoscriptBodyPlacement(t *testing.T) {
	const scriptingCaveat = "whose body is markup only when scripting is disabled"

	t.Run("the placements that need no script are reported", func(t *testing.T) {
		for _, tc := range []struct{ src, want string }{
			{
				`<noscript><iframe srcdoc="[property path='Name']"></iframe></noscript>`,
				`sits in a "srcdoc" attribute`,
			},
			{
				`<noscript><div title=[property path='Name']>x</div></noscript>`,
				"sits in an unquoted attribute value",
			},
			{
				`<noscript><div style="color:[property path='Name']">x</div></noscript>`,
				`sits in a "style" attribute`,
			},
			{
				`<noscript><div data-[property path='Name']="v">x</div></noscript>`,
				"interpolated into a tag or attribute NAME",
			},
			{
				`<noscript><div title="[property path='Name' raw='true']">x</div></noscript>`,
				"with raw= is not escaped at all",
			},
			// An unclosed one is the same case at its widest: with scripting
			// disabled everything after it is live markup.
			{
				`<noscript><div style="color:[property path='Name']">x</div>`,
				`sits in a "style" attribute`,
			},
		} {
			if !lintSaysAny(tc.src, tc.want) {
				t.Errorf("%s: want %q, got %q", tc.src, tc.want, lintWarnings(tc.src))
			}
			for _, msg := range lintWarnings(tc.src) {
				if !strings.Contains(msg, scriptingCaveat) {
					t.Errorf("%s: every message from a <noscript> body says which mode it applies in, got %q", tc.src, msg)
				}
			}
		}
	})

	t.Run("nothing that needs a script to hurt is reported", func(t *testing.T) {
		// None of these can execute in either reading: with scripting on the
		// body is inert raw text, with it off the tags are real and no script
		// in them runs. This is the list that must stay empty.
		for _, src := range []string{
			`<noscript>[property path="Name"]</noscript>`,
			`<noscript><div class="c">[property path="Name"]</div></noscript>`,
			`<noscript><a href="[property path='Name']">go</a></noscript>`,
			`<noscript><a href="javascript:[property path='Name']">go</a></noscript>`,
			`<noscript><button onclick="f('[property path=` + "`" + `Name` + "`" + `]')">go</button></noscript>`,
			`<noscript><div x-text="[property path='Name']">x</div></noscript>`,
			`<noscript><div @click="f('[property path=` + "`" + `Name` + "`" + `]')">x</div></noscript>`,
			`<noscript><script>var s = "[property path='Name']";</script></noscript>`,
			`<noscript><plaintext>[property path="Name"]</noscript>`,
			`<noscript><form action="javascript:[property path='Name']"></form></noscript>`,
		} {
			if got := lintWarnings(src); len(got) != 0 {
				t.Errorf("%s: no script runs in a <noscript> body, got %q", src, got)
			}
		}
	})

	t.Run("the same placements outside a noscript keep their plain message", func(t *testing.T) {
		// The caveat is the whole difference, so it must not leak out.
		for _, src := range []string{
			`<iframe srcdoc="[property path='Name']"></iframe>`,
			`<div style="color:[property path='Name']">x</div>`,
			`<div title=[property path='Name']>x</div>`,
		} {
			for _, msg := range lintWarnings(src) {
				if strings.Contains(msg, scriptingCaveat) {
					t.Errorf("%s: this is live in every mode, got %q", src, msg)
				}
			}
		}
	})
}

// The two regions scanMarkup reads again are read by recursion, and the parts
// of the answer that are reached outside it are not. Both facts are deliberate
// and both are easy to change by accident, so they are pinned here.
func TestLintRereadRegionEdges(t *testing.T) {
	t.Run("the two regions compose", func(t *testing.T) {
		// An <svg> inside a <noscript> is both at once, and the noscript half
		// wins on the rules it withholds: with scripting disabled the SVG link
		// is real and still runs no script, and with it enabled none of this is
		// markup at all.
		src := `<noscript><svg><iframe><a href="javascript:[property path='Name']">x</a></iframe></svg></noscript>`
		if got := lintWarnings(src); len(got) != 0 {
			t.Errorf("no script runs in a <noscript> body, whatever namespace it is in, got %q", got)
		}
		// A style= in the same place is reachable, and says so.
		src = `<noscript><svg><iframe><text style="fill:[property path='Name']">x</text></iframe></svg></noscript>`
		if !lintSaysAny(src, "whose body is markup only when scripting is disabled") {
			t.Errorf("a style= in that placement is live with scripting off, got %q", lintWarnings(src))
		}
	})

	t.Run("the answers reached outside the region scan still apply", func(t *testing.T) {
		// An interpolated ELEMENT name is proved by the "<" in front of it, and
		// an unterminated tag is answered by one pass over the whole document.
		// Both run after scanMarkup and neither is told which region it is
		// looking at, so they warn without the scripting-mode caveat. Leaving
		// them there is fail-closed, and both are in the set a <noscript> body
		// reaches with scripting disabled anyway.
		for _, tc := range []struct{ src, want string }{
			{`<noscript><[property path="Name"] class="c">x</noscript>`, "interpolated into a tag or attribute NAME"},
			{`<noscript><div title="[property path='Name']`, "inside a tag that is never closed"},
			{`<svg><iframe><[property path="Name"] class="c">x</iframe></svg>`, "interpolated into a tag or attribute NAME"},
		} {
			if !lintSaysAny(tc.src, tc.want) {
				t.Errorf("%s: want %q, got %q", tc.src, tc.want, lintWarnings(tc.src))
			}
		}
	})

	t.Run("nesting past the scan depth falls back to silence", func(t *testing.T) {
		// A realistic nest is read all the way down.
		nested := `<svg><iframe><svg><iframe><a href="javascript:[property path='Name']">x</a></iframe></svg></iframe></svg>`
		if !lintSaysAny(nested, `continues a "javascript:" URL`) {
			t.Errorf("a nested foreign region is still read, got %q", lintWarnings(nested))
		}
		// Past the bound the region is left alone, which is what happened to
		// every one of them before scanMarkup existed. The bound is there
		// because each level tokenizes a copy of what it reads, so an unbounded
		// nest would cost O(len × depth).
		deep := strings.Repeat("<svg><iframe>", maxMarkupScanDepth+4) +
			`<a href="javascript:[property path='Name']">x</a>`
		if got := lintWarnings(deep); len(got) != 0 {
			t.Errorf("past the depth bound the region is unanalysed, got %q", got)
		}
	})
}

// foreignNestWrappers are the element names that decide how the markup inside
// them is read: the two foreign roots, the ten the tokenizer raw-texts, the
// integration points, a few breakout tags and a few inert ones. Nesting them
// against each other is what produces the cases nobody writes down.
var foreignNestWrappers = []string{
	"svg", "math", "div", "p", "span", "g", "text", "title", "desc",
	"foreignObject", "iframe", "textarea", "xmp", "noembed", "noframes",
	"noscript", "plaintext", "style", "script", "mtext", "mi",
	"annotation-xml", "font", "b", "table", "li", "a", "section",
	`font color="red"`, `annotation-xml encoding="text/html"`,
	"svg/", "iframe/", "script/", "path/",
}

// nestMarkup wraps probe in each of parts, in order, closing anything that was
// not written self-closing.
func nestMarkup(parts []string, probe string) string {
	var open string
	var closers []string
	for _, p := range parts {
		open += "<" + p + ">"
		if strings.HasSuffix(p, "/") {
			continue
		}
		name := p
		if i := strings.IndexByte(name, ' '); i >= 0 {
			name = name[:i]
		}
		closers = append([]string{"</" + name + ">"}, closers...)
	}
	return open + probe + strings.Join(closers, "")
}

// The linter reads foreign content from a token stream, and the thing it is
// approximating is a namespace-aware parser. So ask one. For every nest of the
// names that decide how markup is read, golang.org/x/net/html's PARSER says
// whether the probe below is a real element with a live "javascript:" href, and
// the linter has to warn exactly when it is.
//
// A warning with no real element behind it is the failure this file is graded
// on, and it is what two earlier attempts at this shipped. The silent direction
// is a missed warning, which is the state everything here was in before; the
// four that are missed are named and explained.
func TestLintForeignContentAgainstTheParser(t *testing.T) {
	liveJSLink := func(src string) bool {
		doc, err := html.Parse(strings.NewReader(src))
		if err != nil {
			return false
		}
		var walk func(*html.Node) bool
		walk = func(n *html.Node) bool {
			if n.Type == html.ElementNode && n.Data == "a" {
				for _, a := range n.Attr {
					if a.Key == "href" && strings.HasPrefix(strings.ToLower(a.Val), "javascript:") {
						return true
					}
				}
			}
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if walk(c) {
					return true
				}
			}
			return false
		}
		return walk(doc)
	}

	// A value in a <script> or <style> body is judged as landing in a program
	// rather than in markup, so the anchor written around it draws the language
	// message instead of the URL one. That is the better message for the
	// placement, and these are the only nests where the two disagree.
	programBody := map[string]bool{
		`<svg><script><a href="javascript:[property path='Name']">go</a></script></svg>`:   true,
		`<svg><style><a href="javascript:[property path='Name']">go</a></style></svg>`:     true,
		`<math><script><a href="javascript:[property path='Name']">go</a></script></math>`: true,
		`<math><style><a href="javascript:[property path='Name']">go</a></style></math>`:   true,
	}

	check := func(parts []string) {
		src := nestMarkup(parts, `<a href="javascript:[property path='Name']">go</a>`)
		oracle := nestMarkup(parts, `<a href="javascript:VALUE">go</a>`)
		live := liveJSLink(oracle)
		warned := lintSaysAny(src, `continues a "javascript:" URL`)
		switch {
		case warned && !live:
			t.Errorf("FALSE POSITIVE: no browser makes a link here, yet %s warns:\n  %q", src, lintWarnings(src))
		case live && !warned && !programBody[src]:
			t.Errorf("MISSED: a real javascript: link here, and %s is silent", src)
		}
	}

	for _, a := range foreignNestWrappers {
		check([]string{a})
		for _, b := range foreignNestWrappers {
			check([]string{a, b})
		}
	}
	// Three deep over the names that actually change the reading, which is
	// where a stack that pops the wrong frame shows up.
	deep := []string{"svg", "math", "div", "iframe", "textarea", "title", "foreignObject", "g", `font color="red"`, "noscript", "p"}
	for _, a := range deep {
		for _, b := range deep {
			for _, c := range deep {
				check([]string{a, b, c})
			}
		}
	}
}

// The other half of the same sweep: a value written somewhere ordinary must
// draw nothing at all, wherever it is nested. The only messages allowed are the
// two that are about landing in a program rather than about the placement.
func TestLintSafePlacementsStaySilentEverywhere(t *testing.T) {
	allowed := []string{"sits inside a <script>", "sits inside a <style>"}
	safeProbes := []string{
		`<span class="c">[property path="Name"]</span>`,
		`<a href="/x/[property path='Name']">go</a>`,
		`<img src="/v1/r/[property path='ID']" alt="[property path='Name']">`,
		`[property path="Name"]`,
	}
	for _, a := range foreignNestWrappers {
		for _, b := range foreignNestWrappers {
			for _, probe := range safeProbes {
				src := nestMarkup([]string{a, b}, probe)
				for _, msg := range lintWarnings(src) {
					ok := false
					for _, prefix := range allowed {
						if strings.Contains(msg, prefix) {
							ok = true
						}
					}
					if !ok {
						t.Errorf("nothing is wrong with %s, yet it draws %q", src, msg)
					}
				}
			}
		}
	}
}
