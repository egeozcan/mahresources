package shortcodes

import "testing"

func TestLintAction(t *testing.T) {
	known := KnownFromBuiltins()
	cases := []struct {
		src, want, sev string
	}{
		{`[action name="a"]`, `[action] is missing required attribute "plugin"`, SeverityError},
		{`[action plugin="p" name=" "]`, `[action] is missing required attribute "name"`, SeverityError},
		{`[action plugin="p" name="a"]x[/action]`, "[action] is an inline shortcode and cannot have a closing tag", SeverityError},
		{`[reload][action plugin="p" name="a"][/reload]`, "[action] cannot be used inside a [reload] button face", SeverityError},
		{`[action plugin="p" name="a" refresh="page"]`, `unknown attribute "refresh" on [action]`, SeverityWarning},
		{`[acton plugin="p" name="a"]`, "did you mean [action]", SeverityInfo},
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
	}

	for _, ok := range []string{
		`[action plugin="p" name="a"]`,
		`[action plugin="p" name="a" label="Go" confirm="Sure?" param-scale="2" param-mode="fast"]`,
	} {
		if issues := Lint(ok, LintOptions{Known: known}); len(issues) != 0 {
			t.Errorf("%s: expected no issues, got %+v", ok, issues)
		}
	}
}

func TestLintAction_ActionExists(t *testing.T) {
	known := KnownFromBuiltins()
	var asked []string
	exists := func(plugin, name string) bool {
		asked = append(asked, plugin+"/"+name)
		return plugin == "p" && name == "real"
	}
	src := `[action plugin="p" name="real"] [action plugin="p" name="gone"] [action plugin=" p " name="gone"] [action plugin="q" name="real"]`
	issues := Lint(src, LintOptions{Known: known, ActionExists: exists})

	want := map[string]bool{
		`no action "gone" of plugin "p" is available; this renders nothing`: true,
		`no action "real" of plugin "q" is available; this renders nothing`: true,
	}
	found := 0
	for _, issue := range issues {
		if want[issue.Message] {
			found++
			if issue.Severity != SeverityWarning {
				t.Errorf("%q: severity %q, want warning", issue.Message, issue.Severity)
			}
			continue
		}
		t.Errorf("unexpected issue %+v", issue)
	}
	if found != 3 {
		t.Errorf("expected 3 missing-action warnings (gone twice, q/real once), got %d: %+v", found, issues)
	}
	if len(asked) != 3 {
		t.Errorf("ActionExists is memoized per (plugin, name) after trimming; asked %v", asked)
	}

	// No resolver, no check: a caller that cannot look actions up reports nothing.
	if issues := Lint(`[action plugin="p" name="gone"]`, LintOptions{Known: known}); len(issues) != 0 {
		t.Errorf("nil ActionExists must not warn, got %+v", issues)
	}
	// An incomplete opener is the missing-attribute error alone.
	asked = nil
	Lint(`[action plugin="p"]`, LintOptions{Known: known, ActionExists: exists})
	if len(asked) != 0 {
		t.Errorf("ActionExists asked about an opener with no name: %v", asked)
	}
}
