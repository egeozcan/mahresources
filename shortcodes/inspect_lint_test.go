package shortcodes

import "testing"

func TestLintInspect(t *testing.T) {
	known := KnownFromBuiltins()
	cases := []struct {
		src, want, sev string
	}{
		{`[inspect][/inspect]`, "[inspect] is an inline shortcode and cannot have a closing tag", ""},
		{`[inspect foo="1"]`, `unknown attribute "foo"`, SeverityWarning},
		{`[inpect]`, "did you mean [inspect]", ""}, // one-edit typo; the near-miss check does not count transpositions
		{`[reload][inspect][/reload]`, "[inspect] cannot be used inside a [reload] button face", SeverityError},
	}
	for _, tc := range cases {
		issues := Lint(tc.src, LintOptions{Known: known})
		got := findIssue(issues, tc.want)
		if got == nil {
			t.Errorf("%s: expected an issue containing %q, got %+v", tc.src, tc.want, issues)
			continue
		}
		if tc.sev != "" && got.Severity != tc.sev {
			t.Errorf("%s: severity %q, want %q", tc.src, got.Severity, tc.sev)
		}
	}

	for _, ok := range []string{`[inspect]`, `[inspect path="project"]`, "[each path=\"steps\"]\n  [inspect]\n  [item path=\"title\"]\n[/each]"} {
		if issues := Lint(ok, LintOptions{Known: known}); len(issues) != 0 {
			t.Errorf("%s: expected no issues, got %+v", ok, issues)
		}
	}
}
