package shortcodes

import "testing"

func TestLintAdjacent(t *testing.T) {
	known := KnownFromBuiltins()
	cases := []struct {
		src, want, sev string
	}{
		{`[adjacent series="yes"]`, `[adjacent] series must be "true" or "false"`, SeverityError},
		{`[adjacent][/adjacent]`, "[adjacent] is an inline shortcode and cannot have a closing tag", ""},
		{`[adjacnt]`, "did you mean [adjacent]", ""},
		{`[adjacent order="name"]`, `unknown attribute "order"`, SeverityWarning},
		{`[reload][adjacent][/reload]`, "[adjacent] cannot be used inside a [reload] button face", SeverityError},
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

	for _, ok := range []string{`[adjacent]`, `[adjacent series="true" label="Pages"]`, `[adjacent series="false"]`} {
		if issues := Lint(ok, LintOptions{Known: known}); len(issues) != 0 {
			t.Errorf("%s: expected no issues, got %+v", ok, issues)
		}
	}
}
