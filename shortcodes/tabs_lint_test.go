package shortcodes

import (
	"strings"
	"testing"
)

func TestLintTabs(t *testing.T) {
	known := KnownFromBuiltins()
	cases := []struct {
		src, want, sev, at string // at: the text the issue must start on
	}{
		{`[tabs][tab label="A"]a[/tab]`, "[tabs] must be a block", SeverityError, `[tabs]`},
		{`[tabs]nothing[/tabs]`, tabsNoTabsProblem, SeverityError, `[tabs]`},
		{`[tabs][tab label="A"]a[/tabs]`, tabsUnclosedProblem, SeverityError, `[tab label`},
		{`[tabs][tab label="A"]a[tab label="B"]b[/tab][/tabs]`, tabsNestedTabProblem, SeverityError, `[tab label="A"]`},
		{`[tabs][tab label="A"]a[/tab][tab]b[/tab][/tabs]`, `[tab] has no label; it is shown as "Tab 2"`, SeverityWarning, `[tab]b`},
		{`[tabs][tab label=" "]a[/tab][/tabs]`, `[tab] has no label; it is shown as "Tab 1"`, SeverityWarning, `[tab label`},
		{`[tabs][tab label="A" lazy="yes"]a[/tab][/tabs]`, `lazy on [tab] must be "true" or "false"`, SeverityWarning, `[tab label`},
		{`[tabs][tab label="A" active="true"]a[/tab][/tabs]`, `unknown attribute "active" on [tab]`, SeverityWarning, `[tab label`},
		{`[tabs]<h3>Intro</h3>[tab label="A"]a[/tab][/tabs]`, "content outside [tab]…[/tab] inside [tabs] is not rendered", SeverityWarning, `<h3>Intro</h3>`},
		{`[reload][tabs][tab label="A"]a[/tab][/tabs][/reload]`, tabsReloadFaceProblem, SeverityError, `[tabs]`},
		{`<p>[tab label="A"]a[/tab]</p>`, "[tab] is only read directly inside [tabs]", SeverityWarning, `[tab label`},
		{`[tabs][tab label="A"][conditional path="x" empty="true"][tab label="B"]z[/tab][/conditional][/tab][/tabs]`, "[tab] is only read directly inside [tabs]", SeverityWarning, `[tab label="B"]`},
		{`<p>a[/tab]</p>`, "stray [/tab] closes no tab", SeverityError, `[/tab]`},
		{`[tabs][tab label="A"]a[/tab][/tab][/tabs]`, "stray [/tab] closes no tab", SeverityError, `[/tab][/tabs]`},
		{`[tabz][/tabz]`, "did you mean [tabs]", SeverityInfo, `[tabz]`},
	}
	for _, tc := range cases {
		issues := Lint(tc.src, LintOptions{Known: known})
		got := findIssue(issues, tc.want)
		if got == nil {
			t.Errorf("%s: expected an issue containing %q, got %+v", tc.src, tc.want, issues)
			continue
		}
		if got.Severity != tc.sev {
			t.Errorf("%s: severity %q, want %q", tc.src, got.Severity, tc.sev)
		}
		if !strings.HasPrefix(tc.src[got.Start:], tc.at) {
			t.Errorf("%s: issue %q starts at %q, want %q", tc.src, tc.want, tc.src[got.Start:], tc.at)
		}
	}
}

// Valid templates stay quiet: no near-miss for [tab], nothing about a lazy
// first tab, nothing for a [tab] the runtime never reads as one (a stylesheet,
// an attribute value), and one problem per malformed block rather than a
// cascade over the tags after it.
func TestLintTabsQuietCases(t *testing.T) {
	known := KnownFromBuiltins()
	for _, src := range []string{
		`[tabs label="Views"][tab label="Overview"]a[/tab][tab label="Files" lazy="true"][mrql query="type = resource" format="list"][/tab][/tabs]`,
		`[tabs][tab label="First" lazy="true"]a[/tab][tab label="Second" lazy="false"]b[/tab][/tabs]`,
		"[tabs]\n  [tab\n    label=\"A\"]a[/tab]\n[/tabs]",
		`[tabs][tab label="Outer"][tabs label="Inner"][tab label="I"]i[/tab][/tabs][/tab][/tabs]`,
		`<style>.x[tab] { color: red } .y::after { content: "[/tab]" }</style>`,
		`[property path="[tab]x[/tab]" default="none"]`,
	} {
		if issues := Lint(src, LintOptions{Known: known}); len(issues) != 0 {
			t.Errorf("%s: expected no issues, got %+v", src, issues)
		}
	}
	if issues := Lint(`.tabs [tab] { color: red } a::after { content: "[/tab]" }`, LintOptions{Known: known, CSSMode: true}); len(issues) != 0 {
		t.Errorf("CSS mode: expected no issues, got %+v", issues)
	}

	issues := Lint(`[tabs][tab label="A"]a[tab label="B"]b[/tab][/tabs]`, LintOptions{Known: known})
	if len(issues) != 1 || issues[0].Message != tabsNestedTabProblem {
		t.Errorf("a malformed block reports its one problem, got %+v", issues)
	}
}
