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

	t.Run("a script or style in SVG has its escaping undone", func(t *testing.T) {
		for _, tc := range []struct{ src, lang string }{
			{`<svg><script>var s = "[property path='Name']"</script></svg>`, "JavaScript"},
			{`<svg><style>.c{color:"[property path='Name']"}</style></svg>`, "CSS"},
			{`<svg><g><script>var s = "[property path='Name']"</script></g></svg>`, "JavaScript"},
			// <svg> inside <annotation-xml> is taken by HTML rules whatever the
			// encoding says, and the HTML rule for <svg> enters SVG.
			{`<math><annotation-xml><svg><script>var s = "[property path='Name']"</script></svg></annotation-xml></math>`, "JavaScript"},
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

	t.Run("MathML has no script and no style, so neither is a program", func(t *testing.T) {
		// MathML Core defines neither, and the parser spec special-cases the end
		// tag of an SVG <script> and has nothing of the sort for MathML. So a
		// browser runs nothing and applies nothing here, and the body is read as
		// the ordinary markup it is.
		for _, src := range []string{
			`<math><script>var s = "[property path='Name']"</script></math>`,
			`<math><style>.c{color:[property path='Name']}</style></math>`,
			`<math><annotation-xml><script>var s = "[property path='Name']"</script></annotation-xml></math>`,
			// <mglyph> and <malignmark> are the two start tags a MathML text
			// integration point does NOT hand to HTML, so this script is MathML.
			`<math><mtext><mglyph><script>var s = "[property path='Name']"</script></mglyph></mtext></math>`,
		} {
			if got := lintWarnings(src); len(got) != 0 {
				t.Errorf("%s: no browser runs or applies this, got %q", src, got)
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
		// annotation-xml without an HTML encoding is not one, so its children
		// are still MathML — covered by the MathML subtest above.
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
				// The raw= reasons are the exception, and must be: an unescaped
				// value can write "</noscript>" and go on in markup whichever
				// way the body was read, so naming one mode would understate it.
				if strings.Contains(msg, "with raw=") {
					if strings.Contains(msg, scriptingCaveat) {
						t.Errorf("%s: raw= holds under both readings, so it takes no caveat: %q", tc.src, msg)
					}
					continue
				}
				if !strings.Contains(msg, scriptingCaveat) {
					t.Errorf("%s: a placement reason from a <noscript> body says which mode it applies in, got %q", tc.src, msg)
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
	"svg/", "iframe/", "script/", "path/", "mglyph",
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

// The linter reads namespaces off a token stream, and what it is approximating
// is a namespace-aware parser. So ask one. Each probe below places a value in
// one position, golang.org/x/net/html's PARSER says whether the element that
// makes that position dangerous actually exists, and the linter has to warn
// exactly when it does.
//
// A warning with no live element behind it is the failure this file is graded
// on, and it is what two earlier attempts at this shipped. The silent direction
// is a missed warning, which is the state everything here was in before; the
// four that are missed are named below.
func TestLintPlacementsAgainstTheParser(t *testing.T) {
	type probe struct {
		name string
		// lintSrc and oracleSrc are the same markup, one carrying a shortcode
		// and one a plain value.
		lintSrc, oracleSrc string
		// element is what has to exist for the placement to be dangerous, in one
		// of namespaces.
		element, attr, attrPrefix string
		namespaces                []string
		// childText requires VALUE to reach the element's CHILD TEXT CONTENT —
		// its direct Text children, which is what a program's source text is
		// built from. Existence alone would let a warning about JavaScript pass
		// on markup where no JavaScript ever sees the value.
		childText bool
		// scriptingOff asks the parser again with scripting disabled, for the
		// placements that are live in that mode. The rules that describe
		// execution do not, since neither mode runs a script in a <noscript>.
		scriptingOff bool
		msg          string
	}
	probes := []probe{
		{
			name: "javascript URL", msg: `continues a "javascript:" URL`,
			lintSrc:   `<a href="javascript:[property path='Name']">go</a>`,
			oracleSrc: `<a href="javascript:VALUE">go</a>`,
			element:   "a", attr: "href", attrPrefix: "javascript:",
			// Every namespace: the URL rules have always matched on attribute
			// name rather than on element, which is the same looseness that
			// warns about <div href="javascript:…"> in HTML.
			namespaces: []string{"", "svg", "math"},
		},
		{
			name: "srcdoc", msg: `sits in a "srcdoc" attribute`,
			lintSrc:   `<iframe srcdoc="[property path='Name']"></iframe>`,
			oracleSrc: `<iframe srcdoc="VALUE"></iframe>`,
			element:   "iframe", attr: "srcdoc",
			// HTML only: a foreign <iframe> has no browsing context.
			namespaces: []string{""}, scriptingOff: true,
		},
		{
			// No scriptingOff, and that is the whole distinction the <noscript>
			// rules rest on: with scripting disabled the body is markup and the
			// <script> in it is a real element, but it still does not run.
			name: "script program", msg: "JavaScript",
			lintSrc:   `<script>var s = "[property path='Name']"</script>`,
			oracleSrc: `<script>var s = "VALUE"</script>`,
			element:   "script", namespaces: []string{"", "svg"}, childText: true,
		},
		{
			// The same probe with the value wrapped in an element, which is the
			// child-text-content rule from the other side: inside an HTML
			// <script> the wrapper is raw text and the value IS the program,
			// while inside an SVG one it is a real element and the value is not.
			name: "script program (wrapped)", msg: "JavaScript",
			lintSrc:   `<script><span>[property path='Name']</span></script>`,
			oracleSrc: `<script><span>VALUE</span></script>`,
			element:   "script", namespaces: []string{"", "svg"}, childText: true,
		},
		{
			// A <style> in the same place IS live in that mode: it is a real
			// stylesheet, and a value in it can close the declaration and open
			// another. Nothing about it needs a script.
			name: "style program", msg: "CSS",
			lintSrc:   `<style>.c{color:[property path='Name']}</style>`,
			oracleSrc: `<style>.c{color:VALUE}</style>`,
			element:   "style", namespaces: []string{"", "svg"}, scriptingOff: true, childText: true,
		},
	}

	// A value inside a <script> or <style> body is judged as landing in a
	// program rather than in markup, so anything written around it there draws
	// the language message instead. These are the only nests where a probe and
	// the parser disagree, and the message that does come back is asserted
	// below rather than merely excused — an exception that suppresses a miss
	// without saying what replaces it would hide the next one.
	inAProgramBody := map[string]bool{
		`<svg><script><a href="javascript:[property path='Name']">go</a></script></svg>`: true,
		`<svg><style><a href="javascript:[property path='Name']">go</a></style></svg>`:   true,
		`<svg><style><script>var s = "[property path='Name']"</script></style></svg>`:    true,
		`<svg><script><style>.c{color:[property path='Name']}</style></script></svg>`:    true,
	}

	exists := func(src string, p probe, scripting bool) bool {
		opts := []html.ParseOption{}
		if !scripting {
			opts = append(opts, html.ParseOptionEnableScripting(false))
		}
		doc, err := html.ParseWithOptions(strings.NewReader(src), opts...)
		if err != nil {
			return false
		}
		var walk func(*html.Node) bool
		walk = func(n *html.Node) bool {
			if n.Type == html.ElementNode && n.Data == p.element {
				for _, ns := range p.namespaces {
					if n.Namespace != ns {
						continue
					}
					switch {
					case p.childText:
						// Direct Text children only, in tree order — the
						// definition the source text of a script or a
						// stylesheet is built from.
						for c := n.FirstChild; c != nil; c = c.NextSibling {
							if c.Type == html.TextNode && strings.Contains(c.Data, "VALUE") {
								return true
							}
						}
					case p.attr == "":
						return true
					default:
						for _, a := range n.Attr {
							if a.Key == p.attr && strings.HasPrefix(strings.ToLower(a.Val), p.attrPrefix) {
								return true
							}
						}
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

	check := func(parts []string) {
		for _, p := range probes {
			lintSrc := nestMarkup(parts, p.lintSrc)
			oracleSrc := nestMarkup(parts, p.oracleSrc)
			live := exists(oracleSrc, p, true)
			if !live && p.scriptingOff {
				live = exists(oracleSrc, p, false)
			}
			warned := lintSaysAny(lintSrc, p.msg)
			switch {
			case warned && !live:
				t.Errorf("FALSE POSITIVE (%s): no browser makes this live, yet %s warns:\n  %q",
					p.name, lintSrc, lintWarnings(lintSrc))
			case live && !warned && !inAProgramBody[lintSrc]:
				t.Errorf("MISSED (%s): this is live in a browser, and %s is silent", p.name, lintSrc)
			case live && !warned:
				if !lintSaysAny(lintSrc, "which is foreign content, where the parser decodes entities") {
					t.Errorf("EXCUSED BUT SILENT (%s): %s draws neither message, got %q",
						p.name, lintSrc, lintWarnings(lintSrc))
				}
			}
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
	deep := []string{"svg", "math", "div", "iframe", "textarea", "title", "foreignObject",
		"g", `font color="red"`, "noscript", "p", "mtext", "mglyph", "annotation-xml"}
	for _, a := range deep {
		for _, b := range deep {
			for _, c := range deep {
				check([]string{a, b, c})
			}
		}
	}

	// Balanced markup is the easy half. Every defect this scan has had came
	// from markup that is not: a stray end tag closing something that was never
	// opened, a void element that a browser pops and a frame stack does not,
	// and a "/>" that HTML ignores. checkRaw takes the source as written.
	// Only the false-positive direction: malformed markup has many legitimate
	// misses (a region past the depth bound, a value inside a program body),
	// and the balanced sweep above is where the missed half is asserted.
	checkRaw := func(before, after string) {
		for _, p := range probes {
			lintSrc := before + p.lintSrc + after
			oracleSrc := before + p.oracleSrc + after
			live := exists(oracleSrc, p, true)
			if !live && p.scriptingOff {
				live = exists(oracleSrc, p, false)
			}
			if lintSaysAny(lintSrc, p.msg) && !live {
				t.Errorf("FALSE POSITIVE (%s): no browser makes this live, yet %s warns:\n  %q",
					p.name, lintSrc, lintWarnings(lintSrc))
			}
		}
	}
	openers := []string{
		"<svg>", "<math>", "<svg><iframe>", "<math><iframe>", "<svg><textarea>",
		"<math><mtext>", "<svg><g>", "<math><annotation-xml>", "<div>",
		// Two roots deep, so the closer that arrives belongs to the outer one.
		"<svg><iframe><math>", "<math><iframe><svg>", "<svg><math><iframe>",
		"<math><mtext><div><svg><iframe>",
		// A program region, and a scope boundary above the closer.
		"<svg><script>", "<svg><style>", "<svg><script><div>",
		"<div><math><annotation-xml>", "<div><math><mtext>",
		"<svg><script><foreignObject><div>", "<svg><iframe><foreignObject>",
	}
	strays := []string{
		"", "</svg>", "</math>", "</textarea>", "</iframe>", "</div>", "</p>", "</br>",
		"<br>", "<img>", "<hr>", "<div/>", "<span/>", "<path/>", "<mglyph>",
		"<br><mglyph>", "<div/><mglyph>", "</svg><script>", "</math><script>",
		// Close something, then probe: the shape where a stale program or a
		// stale namespace shows up.
		"<div></div>", "<g></g>", "<span></span>", "<p></p>", "<foreignObject></foreignObject>",
	}
	for _, o := range openers {
		for _, stray := range strays {
			checkRaw(o+stray, "")
			checkRaw(o+stray, "</iframe></svg>")
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

// Reading a foreign region as markup exposes attributes that were never read
// before, and an attribute means what the ELEMENT it sits on makes it mean.
// srcdoc is the one that matters: it is a property of HTMLIFrameElement, and a
// foreign <iframe> is an inert element with no browsing context at all.
func TestLintForeignElementsAreNotHTMLElements(t *testing.T) {
	t.Run("srcdoc on a foreign element is inert", func(t *testing.T) {
		for _, src := range []string{
			`<svg><iframe srcdoc="[property path='Name']"></iframe></svg>`,
			`<svg><textarea><iframe srcdoc="[property path='Name']"></iframe></textarea></svg>`,
			`<math><iframe srcdoc="[property path='Name']"></iframe></math>`,
		} {
			if lintSaysAny(src, `sits in a "srcdoc" attribute`) {
				t.Errorf("%s: a foreign <iframe> has no browsing context, got %q", src, lintWarnings(src))
			}
		}
	})

	t.Run("the same attribute on a real iframe still warns", func(t *testing.T) {
		for _, src := range []string{
			`<iframe srcdoc="[property path='Name']"></iframe>`,
			`<svg><foreignObject><iframe srcdoc="[property path='Name']"></iframe></foreignObject></svg>`,
			`<math><mtext><iframe srcdoc="[property path='Name']"></iframe></mtext></math>`,
		} {
			if !lintSaysAny(src, `sits in a "srcdoc" attribute`) {
				t.Errorf("%s: this one is a real iframe, got %q", src, lintWarnings(src))
			}
		}
	})

	t.Run("what every namespace honours is unaffected", func(t *testing.T) {
		// style= and an on* handler work on an SVG element as much as on an
		// HTML one, so narrowing by namespace must not reach them.
		if !lintSaysAny(`<svg><text style="fill:[property path='Name']">x</text></svg>`, `sits in a "style" attribute`) {
			t.Error("SVG elements are styled with the same attribute")
		}
		if !lintSaysAny(`<svg><text onclick="f('[property path=`+"`"+`Name`+"`"+`]')">x</text></svg>`, "event handler") {
			t.Error("SVG elements take the same event handlers")
		}
	})
}

// A region is read again from its source, so the scan that reads it does not
// have the open elements above it. Two shapes make that visible and both were
// false positives first.
func TestLintForeignRegionEndsWithItsEnclosingElement(t *testing.T) {
	for _, src := range []string{
		// A browser's "</svg>" pops the foreign <iframe> and the SVG root, so
		// the <textarea> is HTML RCDATA and the anchor in it is only text.
		`<svg><iframe></svg><textarea><a href="javascript:[property path='Name']">go</a></textarea></iframe>`,
		// "</p>" and "</br>" are the end-tag half of the breakout rule.
		`<svg></p><textarea><a href="javascript:[property path='Name']">go</a></textarea>`,
		`<svg></br><textarea><a href="javascript:[property path='Name']">go</a></textarea>`,
	} {
		if got := lintWarnings(src); len(got) != 0 {
			t.Errorf("%s: a browser has left foreign content here, got %q", src, got)
		}
	}
}

// The region-ending rule above is deliberately narrow, because leaving foreign
// content is not the quiet direction in both senses: it makes the raw-text
// names raw again (fewer warnings) but it also makes srcdoc live again (more).
// So only an end tag that really did end the region counts.
func TestLintOnlyTheRegionsOwnEndTagEndsIt(t *testing.T) {
	// A browser ignores a stray "</textarea>" here and stays in SVG, where the
	// <iframe> is inert and its srcdoc is never parsed as a document.
	src := `<svg><iframe></textarea><iframe srcdoc="[property path='Name']"></iframe>`
	if lintSaysAny(src, `sits in a "srcdoc" attribute`) {
		t.Errorf("a stray end tag does not leave foreign content, got %q", lintWarnings(src))
	}
	// An end tag that really did end the region does, and then the srcdoc is on
	// a real HTML iframe again.
	src = `<svg><iframe></svg><iframe srcdoc="[property path='Name']"></iframe>`
	if !lintSaysAny(src, `sits in a "srcdoc" attribute`) {
		t.Errorf("past the </svg> this is HTML again, got %q", lintWarnings(src))
	}
	// Closing the region's own element without leaving the <svg> does not: the
	// next <iframe> is a second inert SVG element, not an HTML one.
	src = `<svg><iframe></iframe><iframe srcdoc="[property path='Name']"></iframe></svg>`
	if lintSaysAny(src, `sits in a "srcdoc" attribute`) {
		t.Errorf("still inside the <svg>, so still inert, got %q", lintWarnings(src))
	}
}

// The namespace of everything after an element is read off the frame stack, so
// a frame a browser does not have puts every later element in the wrong
// namespace. Three ways to acquire one, and each was a false positive or a
// missed warning first.
func TestLintFrameStackMatchesTheParsers(t *testing.T) {
	t.Run("a void element leaves no frame", func(t *testing.T) {
		// A browser pops <br> the instant it inserts it, so <mglyph> is still
		// opened inside <mtext> — where it is one of the two names that stay
		// MathML — and the <iframe> under it is inert.
		src := `<math><mtext><br><mglyph><iframe srcdoc="[property path='Name']"></iframe></mglyph></mtext></math>`
		if got := lintWarnings(src); len(got) != 0 {
			t.Errorf("the <br> is gone before the <mglyph> opens, got %q", got)
		}
	})

	t.Run("HTML ignores a solidus, so it leaves one", func(t *testing.T) {
		// The mirror: "<div/>" is not self-closing in HTML, so the div IS open
		// and everything under it is HTML, including a real iframe.
		src := `<math><mtext><div/><mglyph><iframe srcdoc="[property path='Name']"></iframe></mglyph></mtext></math>`
		if !lintSaysAny(src, `sits in a "srcdoc" attribute`) {
			t.Errorf(`"<div/>" opens a div in HTML, got %q`, lintWarnings(src))
		}
		// In foreign content the same solidus really does close the tag.
		src = `<svg><path/><a href="javascript:[property path='Name']">go</a></svg>`
		if !lintSaysAny(src, `continues a "javascript:" URL`) {
			t.Errorf("a self-closed <path> leaves no frame in SVG, got %q", lintWarnings(src))
		}
	})

	t.Run("only the region's own root ends it", func(t *testing.T) {
		// A "</svg>" inside a MathML region closes nothing, and a browser stays
		// in MathML, where a <script> runs nothing.
		src := `<math><iframe></svg><script>var s="[property path='Name']"</script></iframe></math>`
		if got := lintWarnings(src); len(got) != 0 {
			t.Errorf("still MathML, so still inert, got %q", got)
		}
	})
}

// <annotation-xml> is the one element whose namespace is decided by an
// attribute value, which makes it the one place a lower-privileged value can
// change how the document is parsed.
func TestLintAnnotationXMLEncoding(t *testing.T) {
	live := `sits in a "srcdoc" attribute`

	t.Run("the match is case-insensitive and nothing more", func(t *testing.T) {
		if !lintSaysAny(`<math><annotation-xml encoding="TEXT/HTML"><iframe srcdoc="[property path='Name']"></iframe></annotation-xml></math>`, live) {
			t.Error("an ASCII case-insensitive match is a match")
		}
		// Not trimmed: a browser compares the attribute as written.
		for _, src := range []string{
			`<math><annotation-xml encoding=" text/html "><iframe srcdoc="[property path='Name']"></iframe></annotation-xml></math>`,
			`<math><annotation-xml encoding="image/svg+xml"><iframe srcdoc="[property path='Name']"></iframe></annotation-xml></math>`,
			`<math><annotation-xml><iframe srcdoc="[property path='Name']"></iframe></annotation-xml></math>`,
		} {
			if lintSaysAny(src, live) {
				t.Errorf("%s: this is still MathML, got %q", src, lintWarnings(src))
			}
		}
	})

	t.Run("an interpolated encoding fails closed", func(t *testing.T) {
		// The value decides whether everything inside is HTML, and it is written
		// by whoever can edit the entity. Reading it as the integration point it
		// may turn out to be is the same choice the unterminated-tag rule makes.
		src := `<math><annotation-xml encoding="[property path='Encoding']"><script>var s="[property path='Name']"</script></annotation-xml></math>`
		if !lintSaysAny(src, "reaches JavaScript") {
			t.Errorf("an encoding nobody can predict must not read as inert, got %q", lintWarnings(src))
		}
	})
}

// srcdoc is defined on <iframe> and on nothing else, so the rule that describes
// it is the one place an element name is worth checking.
func TestLintSrcdocIsAnIframeAttribute(t *testing.T) {
	if lintSaysAny(`<div srcdoc="[property path='Name']"></div>`, `sits in a "srcdoc" attribute`) {
		t.Error("a srcdoc on a div is an inert unknown attribute")
	}
	if !lintSaysAny(`<iframe srcdoc="[property path='Name']"></iframe>`, `sits in a "srcdoc" attribute`) {
		t.Error("on an iframe it is the real thing")
	}
}

// An SVG <script> or <style> body is a program AND markup at the same time,
// which is the pair of facts an HTML one does not have. Only what lands in the
// TEXT is program source.
func TestLintSVGProgramBodiesAreAlsoMarkup(t *testing.T) {
	t.Run("a direct child text node is the program", func(t *testing.T) {
		for _, tc := range []struct{ src, lang string }{
			{`<svg><script>var s = "[property path='Name']"</script></svg>`, "JavaScript"},
			{`<svg><style>.c{color:[property path='Name']}</style></svg>`, "CSS"},
			// Still a direct child, after an element in the middle closed.
			{`<svg><script><g></g>var s = "[property path='Name']"</script></svg>`, "JavaScript"},
		} {
			if !lintSaysAny(tc.src, tc.lang) {
				t.Errorf("%s: this text is %s, got %q", tc.src, tc.lang, lintWarnings(tc.src))
			}
		}
	})

	t.Run("a descendant text node is not", func(t *testing.T) {
		// The source text is built from "child text content", which is the Text
		// CHILDREN and not textContent, so a value written inside an element in
		// there is markup no browser executes or applies.
		for _, src := range []string{
			`<svg><script><g>[property path="Name"]</g></script></svg>`,
			`<svg><style><g>[property path="Name"]</g></style></svg>`,
		} {
			if got := lintWarnings(src); len(got) != 0 {
				t.Errorf("%s: not child text content, got %q", src, got)
			}
		}
		// And the region ends with the element it belongs to, so text past a
		// "</svg>" that popped the script is ordinary text again.
		src := `<svg><script></svg>[property path="Name"]`
		if got := lintWarnings(src); len(got) != 0 {
			t.Errorf("%s: the script is closed, got %q", src, got)
		}
	})

	t.Run("an attribute in one is an attribute", func(t *testing.T) {
		// The body is parsed, so <g> here is a real SVG element and its
		// data-x is an inert attribute of it — not a line of JavaScript, which
		// is what treating the whole raw token as program source claimed.
		for _, src := range []string{
			`<svg><script><g data-x="[property path='Name']"></g></script></svg>`,
			`<svg><style><g data-x="[property path='Name']"></g></style></svg>`,
		} {
			if got := lintWarnings(src); len(got) != 0 {
				t.Errorf("%s: an attribute of a real element, got %q", src, got)
			}
		}
		// And when the attribute is one that does something, the rule that
		// describes it is the one that fires.
		src := `<svg><script><a href="javascript:[property path='Name']">go</a></script></svg>`
		if !lintSaysAny(src, `continues a "javascript:" URL`) {
			t.Errorf("a real SVG link inside the body, got %q", lintWarnings(src))
		}
	})

	t.Run("an HTML program body is still raw text", func(t *testing.T) {
		// No tag inside one is a tag, so the whole body is program source and
		// the "attribute" below is only characters in it.
		src := `<script><g data-x="[property path='Name']"></g></script>`
		if !lintSaysAny(src, "reaches JavaScript") {
			t.Errorf("an HTML <script> body is raw text throughout, got %q", lintWarnings(src))
		}
	})
}

// <annotation-xml> is the one element whose namespace an attribute VALUE
// decides, so an interpolation in it is asked what it could complete rather
// than whether it is there — the shape couldStillBecomeExecutable already uses
// for a URL scheme.
func TestLintInterpolatedEncodingAsksWhatItCouldBecome(t *testing.T) {
	inert := func(src string) {
		t.Helper()
		if got := lintWarnings(src); len(got) != 0 {
			t.Errorf("%s: this stays MathML, got %q", src, got)
		}
	}
	live := func(src string) {
		t.Helper()
		if !lintSaysAny(src, "reaches JavaScript") {
			t.Errorf("%s: this could become HTML, got %q", src, lintWarnings(src))
		}
	}
	// Nothing appended to "image/" is "text/html" or "application/xhtml+xml".
	inert(`<math><annotation-xml encoding="image/[property path='E']"><script>let x="[property path='N']"</script></annotation-xml></math>`)
	inert(`<math><annotation-xml encoding="[property path='E']/svg+xml"><script>let x="[property path='N']"</script></annotation-xml></math>`)
	// These could.
	live(`<math><annotation-xml encoding="[property path='E']"><script>let x="[property path='N']"</script></annotation-xml></math>`)
	live(`<math><annotation-xml encoding="text/[property path='E']"><script>let x="[property path='N']"</script></annotation-xml></math>`)
	live(`<math><annotation-xml encoding="[property path='E']/html"><script>let x="[property path='N']"</script></annotation-xml></math>`)
	// The static path compares what the browser sees, because z.TagAttr decodes
	// entities. So must this one, or "text&#x2f;" reads as inert.
	live(`<math><annotation-xml encoding="text&#x2f;[property path='E']"><script>let x="[property path='N']"</script></annotation-xml></math>`)
	// A fixed run BETWEEN two interpolations is immutable too. No assignment
	// makes "[E]zz[F]" either encoding, because neither contains a "zz"...
	inert(`<math><annotation-xml encoding="[property path='E']zz[property path='F']"><script>let x="[property path='N']"</script></annotation-xml></math>`)
	// ... while "[E]x[F]" is text/html with E="te" and F="t/html".
	live(`<math><annotation-xml encoding="[property path='E']x[property path='F']"><script>let x="[property path='N']"</script></annotation-xml></math>`)
}

// A <style> and a <script> inside a <noscript> are the pair that shows where
// the dividing line is: both are real elements with scripting disabled, and
// only one of them does anything.
func TestLintNoscriptStyleIsAStylesheetAndScriptIsNot(t *testing.T) {
	src := `<noscript><style>.x{color:[property path='Name']}</style></noscript>`
	if !lintSaysAny(src, "reaches CSS") {
		t.Errorf("with scripting off this is a live stylesheet, got %q", lintWarnings(src))
	}
	if !lintSaysAny(src, "whose body is markup only when scripting is disabled") {
		t.Errorf("and it says which mode, got %q", lintWarnings(src))
	}
	src = `<noscript><script>var s="[property path='Name']"</script></noscript>`
	if got := lintWarnings(src); len(got) != 0 {
		t.Errorf("the script is real in that mode and still does not run, got %q", got)
	}
}

// In foreign content the search for what an end tag closes stops at the first
// element sitting directly inside an HTML one: past that the tag is HTML's
// business, and HTML ignores one that names nothing open.
func TestLintForeignEndTagSearchStopsAtTheHTMLBoundary(t *testing.T) {
	src := `<x-foo><div><math></x-foo><iframe srcdoc="[property path='Name']"></iframe>`
	if got := lintWarnings(src); len(got) != 0 {
		t.Errorf("a browser ignores that end tag and stays in MathML, got %q", got)
	}
	// The walk past that boundary is HTML's own, and it stops at a SPECIAL
	// element rather than at nothing: here it finds the <div>, pops the math
	// with it, and what follows is a real HTML iframe. The pair is the point —
	// one boundary has to answer no above and yes here.
	src = `<div><math></div><iframe srcdoc="[property path='Name']"></iframe>`
	if !lintSaysAny(src, `sits in a "srcdoc" attribute`) {
		t.Errorf("the </div> pops the math with it, got %q", lintWarnings(src))
	}
}

// A <style> is a stylesheet only when its type is one a browser supports, and
// otherwise it applies nothing at all — so the body is markup and only markup.
func TestLintStyleTypeDecidesWhetherItIsAStylesheet(t *testing.T) {
	t.Run("an unsupported type applies nothing", func(t *testing.T) {
		for _, src := range []string{
			`<style type="text/plain">[property path="Name"]</style>`,
			`<noscript><style type="text/plain">[property path="Name"]</style></noscript>`,
			`<svg><style type="text/plain">[property path="Name"]</style></svg>`,
		} {
			if got := lintWarnings(src); len(got) != 0 {
				t.Errorf("%s: a browser applies nothing here, got %q", src, got)
			}
		}
		// It is markup, though, so a real link written in one is a real link.
		src := `<svg><style type="text/plain"><a href="javascript:[property path='Name']">x</a></style></svg>`
		if !lintSaysAny(src, `continues a "javascript:" URL`) {
			t.Errorf("%s: the body is markup, got %q", src, lintWarnings(src))
		}
	})

	t.Run("absent, empty or text/css is a stylesheet", func(t *testing.T) {
		for _, src := range []string{
			`<style>.c{color:[property path='Name']}</style>`,
			`<style type="">.c{color:[property path='Name']}</style>`,
			`<style type="TEXT/CSS">.c{color:[property path='Name']}</style>`,
			`<svg><style type="text/css">.c{color:[property path='Name']}</style></svg>`,
		} {
			if !lintSaysAny(src, "CSS") {
				t.Errorf("%s: this one applies, got %q", src, lintWarnings(src))
			}
		}
		// An interpolated type is a value nobody here can read, so it is taken
		// as the stylesheet it may turn out to be.
		if !lintSaysAny(`<style type="[property path='T']">.c{color:[property path='Name']}</style>`, "CSS") {
			t.Error("an interpolated type= fails closed")
		}
	})

	// <script> deliberately has no equivalent: a style has one valid type, and
	// deciding whether a script runs means classifying the JavaScript MIME
	// types. That is named as residue rather than half-implemented.
	t.Run("a script with a data type is residue, not a rule", func(t *testing.T) {
		if !lintSaysAny(`<script type="application/json">{"n":"[property path='Name']"}</script>`, "reaches JavaScript") {
			t.Error("unchanged: the script rule has never read type=")
		}
	})
}

// Two answers are reached outside scanMarkup — the "<" proof for an interpolated
// element name, and the unterminated-tag pass — and neither knows what kind of
// body it is looking at. In a raw-text or RCDATA body a "<" starts nothing, so
// the scan now says so rather than leaving the question open.
func TestLintInertBodiesAnswerTheFallbacks(t *testing.T) {
	for _, src := range []string{
		`<textarea><[property path="Name"]</textarea>`,
		`<title><[property path="Name"]</title>`,
		`<iframe><[property path="Name"]</iframe>`,
		`<xmp><[property path="Name"]</xmp>`,
		`<style type="text/plain"><[property path="Name"]</style>`,
	} {
		if got := lintWarnings(src); len(got) != 0 {
			t.Errorf("%s: a browser shows that \"<\" as text, got %q", src, got)
		}
	}
	// The real case is untouched: outside such a body the "<" is the whole
	// proof that a name is being interpolated.
	src := `<div><[property path="Name"] class="c">x</div>`
	if !lintSaysAny(src, "interpolated into a tag or attribute NAME") {
		t.Errorf("%s: this one really is a name, got %q", src, lintWarnings(src))
	}
	// And raw= is unescaped in one of those bodies too, since the value can
	// write "</xmp>" and go on in markup.
	if !lintSaysAny(`<xmp>[property path="Name" raw="true"]</xmp>`, "becomes real elements") {
		t.Error("raw= is not what an inert body settles")
	}
}

// Leaving foreign content leaves the program with it, and the two walks that
// decide where an end tag lands both stop at a SPECIAL element.
func TestLintProgramAndWalkEndWhereABrowserEndsThem(t *testing.T) {
	t.Run("a breakout ends the program", func(t *testing.T) {
		// The <div> takes a browser out of SVG, popping the script with it, so
		// what follows is ordinary HTML text.
		for _, src := range []string{
			`<svg><script><div></div>[property path="Name"]</script></svg>`,
			`<svg><script><p>[property path="Name"]</script></svg>`,
		} {
			if got := lintWarnings(src); len(got) != 0 {
				t.Errorf("%s: past the breakout this is not the program, got %q", src, got)
			}
		}
	})

	t.Run("the walk stops at a special element", func(t *testing.T) {
		// annotation-xml ends the scope the </div> would need, so a browser
		// ignores it and the iframe stays MathML.
		src := `<div><math><annotation-xml></div><iframe srcdoc="[property path='Name']"></iframe>`
		if got := lintWarnings(src); len(got) != 0 {
			t.Errorf("%s: the </div> is ignored, got %q", src, got)
		}
		// The region-end fallback obeys the same stop: the current node is an
		// HTML <div>, and HTML's rule refuses to walk past one.
		src = `<svg><script><foreignObject><div></svg></div></foreignObject><iframe srcdoc="[property path='Name']"></iframe></script></svg>`
		if got := lintWarnings(src); len(got) != 0 {
			t.Errorf("%s: the </svg> is ignored, got %q", src, got)
		}
	})
}

// Three notions in this scan are HTML's and stop at the namespace boundary, and
// each of them crossed it once.
func TestLintHTMLNotionsStopAtTheNamespaceBoundary(t *testing.T) {
	t.Run("void is an HTML notion", func(t *testing.T) {
		// An <svg><source> is an ordinary foreign element that stays open, so
		// the value under it is a descendant text node rather than the script's
		// own child text.
		src := `<svg><script><source>[property path="Name"]</source></script></svg>`
		if got := lintWarnings(src); len(got) != 0 {
			t.Errorf("%s: the <source> is still open, got %q", src, got)
		}
		// In HTML the same body is raw text throughout, so the value is source.
		if !lintSaysAny(`<script><source>[property path="Name"]</source></script>`, "reaches JavaScript") {
			t.Error("an HTML <script> body is raw text throughout")
		}
	})

	t.Run("the special-element stop is an HTML rule", func(t *testing.T) {
		// The foreign end-tag walk has no such stop: it walks past the special
		// <foreignObject>, finds the svg and leaves, so the second iframe is a
		// real HTML one.
		src := `<svg><iframe><foreignObject><math></svg><iframe srcdoc="[property path='Name']"></iframe></iframe>`
		if !lintSaysAny(src, `sits in a "srcdoc" attribute`) {
			t.Errorf("%s: the </svg> is honoured here, got %q", src, lintWarnings(src))
		}
		// With an HTML element as the current node, HTML's rule governs and
		// refuses to walk past one. The pair is the point.
		src = `<svg><script><foreignObject><div></svg></div></foreignObject><iframe srcdoc="[property path='Name']"></iframe></script></svg>`
		if got := lintWarnings(src); len(got) != 0 {
			t.Errorf("%s: the </svg> is ignored here, got %q", src, got)
		}
	})

	t.Run("case-insensitive means ASCII", func(t *testing.T) {
		// strings.EqualFold folds U+017F with "s", so "text/cſſ" read as
		// text/css and drew a warning about a stylesheet no browser builds.
		for _, src := range []string{
			`<style type="text/c` + "ſſ" + `">[property path="Name"]</style>`,
			`<math><annotation-xml encoding="text/htm` + "ſ" + `"><iframe srcdoc="[property path='Name']"></iframe></annotation-xml></math>`,
		} {
			if got := lintWarnings(src); len(got) != 0 {
				t.Errorf("%s: a browser matches ASCII only, got %q", src, got)
			}
		}
		// ASCII case still folds, which is the half that has to keep working.
		if !lintSaysAny(`<style type="TEXT/CSS">[property path="Name"]</style>`, "reaches CSS") {
			t.Error("TEXT/CSS is text/css")
		}
		if !lintSaysAny(`<math><annotation-xml encoding="TEXT/HTML"><iframe srcdoc="[property path='Name']"></iframe></annotation-xml></math>`, `sits in a "srcdoc" attribute`) {
			t.Error("TEXT/HTML is text/html")
		}
	})
}

// A region is read from its own source, so an end tag inside it that closes
// something OUTSIDE it has to be reported back — the caller opened that element
// and is the one holding the state for it.
func TestLintRegionExitReachesTheCaller(t *testing.T) {
	// A browser leaves MathML at the "</math>", so the second <textarea> is
	// HTML RCDATA and the anchor written in it is literal text.
	src := `<math><textarea></math></textarea><textarea><a href="javascript:[property path='Name']">go</a></textarea>`
	if got := lintWarnings(src); len(got) != 0 {
		t.Errorf("%s: past the </math> this is HTML, got %q", src, got)
	}
	// Without the exit it is still MathML, where the same anchor is a real
	// element.
	src = `<math><textarea><a href="javascript:[property path='Name']">go</a></textarea></math>`
	if !lintSaysAny(src, `continues a "javascript:" URL`) {
		t.Errorf("%s: still MathML here, got %q", src, lintWarnings(src))
	}
}

// Leaving foreign content from an HTML current node is HTML's "any other end
// tag" — a foreign root is no HTML element and has no rule of its own — so it
// stops at the first SPECIAL element like every other instance of that walk.
func TestLintLeavingForeignContentObeysTheSpecialStop(t *testing.T) {
	// A browser meets the <div> and ignores the "</svg>", so the iframe after
	// it is still an inert SVG element.
	src := `<svg><foreignObject><div></svg></div></foreignObject><iframe srcdoc="[property path='Name']"></iframe></svg>`
	if got := lintWarnings(src); len(got) != 0 {
		t.Errorf("%s: the </svg> is ignored, got %q", src, got)
	}
	// With nothing special above it the same tag is honoured.
	src = `<svg><g></svg><iframe srcdoc="[property path='Name']"></iframe>`
	if !lintSaysAny(src, `sits in a "srcdoc" attribute`) {
		t.Errorf("%s: past the </svg> this is HTML, got %q", src, lintWarnings(src))
	}
	// And ordinary HTML nesting is untouched: the walk only applies when the
	// tag would take the scan out of a foreign namespace.
	src = `<div><p><iframe srcdoc="[property path='Name']"></iframe></div>`
	if !lintSaysAny(src, `sits in a "srcdoc" attribute`) {
		t.Errorf("%s: plain HTML, got %q", src, lintWarnings(src))
	}
}

// Attribute names are ASCII-lowercased by a browser and by nothing more, so
// this normalizes them the same way. U+212A KELVIN SIGN lowercases to "k" under
// Unicode folding, and a rule that matched a name exactly would have read
// "on\u212Aeydown" as the onkeydown a browser never creates.
//
// No rule here matches a name exactly today — the one that would, the on*
// family, is a PREFIX test and fires on any attribute starting with "on",
// including that one. That looseness is pre-existing and is the same trade the
// URL rules make, so this is normalization hygiene rather than a change in what
// is reported; what it buys is that the next exact-match rule cannot inherit
// the defect.
func TestLintAttributeNamesFoldOnlyASCII(t *testing.T) {
	// The ASCII half folds, which is what the rules are written against.
	if !lintSaysAny(`<button onKEYDOWN="[property path='Name']">x</button>`, "event handler") {
		t.Error("an onKEYDOWN is an onkeydown")
	}
	if !lintSaysAny(`<div STYLE="color:[property path='Name']">x</div>`, `sits in a "style" attribute`) {
		t.Error("a STYLE is a style")
	}
	// A non-ASCII fold does not create a name a browser never made: the reported
	// attribute is the one that was written.
	src := `<button on` + "K" + `eydown="[property path='Name']">x</button>`
	for _, msg := range lintWarnings(src) {
		if strings.Contains(msg, `"onkeydown"`) {
			t.Errorf("%s: a browser makes no onkeydown here, got %q", src, msg)
		}
	}
}

// A "</form>" removes the form element from the stack rather than popping down
// to it, which is the one end tag in HTML that works that way.
func TestLintFormEndTagRemovesRatherThanPops(t *testing.T) {
	// The <svg> the author forgot to close survives the </form>, so the iframe
	// after it is an inert SVG element.
	src := `<form><svg></form><iframe srcdoc="[property path='Name']"></iframe>`
	if got := lintWarnings(src); len(got) != 0 {
		t.Errorf("%s: the <svg> outlives the </form>, got %q", src, got)
	}
	// With the svg closed, the same iframe is a real one.
	src = `<form><svg></svg></form><iframe srcdoc="[property path='Name']"></iframe>`
	if !lintSaysAny(src, `sits in a "srcdoc" attribute`) {
		t.Errorf("%s: plain HTML here, got %q", src, lintWarnings(src))
	}
}
