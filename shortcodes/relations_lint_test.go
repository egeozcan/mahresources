package shortcodes

import "testing"

func TestLintRelations(t *testing.T) {
	known := KnownFromBuiltins()
	cases := []struct {
		src, want, sev string
	}{
		{`[relations direction="both"]`, `[relations] direction must be "out" or "in"`, SeverityError},
		{`[relations direction="sideways"]`, `[relations] direction must be "out" or "in"`, SeverityError},
		{`[relations limit="0"]`, `[relations] limit must be a whole number from 1 to 100; it falls back to 20`, SeverityWarning},
		{`[relations limit="abc"]`, `[relations] limit must be a whole number from 1 to 100; it falls back to 20`, SeverityWarning},
		{`[relations limit="500"]`, `[relations] limit is capped at 100`, SeverityWarning},
		{`[relations order="name"]`, `unknown attribute "order"`, SeverityWarning},
		{`[relation]`, "did you mean [relations]", ""},
		{`[reload][relations][/reload]`, "[relations] cannot be used inside a [reload] button face", SeverityError},
		{`[item path="Name"]`, "[item] is only meaningful inside an [each] or [relations] block", SeverityWarning},
		{`[calc expr="item.GroupID * 2"]`, "[calc] uses item outside an [each] or [relations] block; it renders the default", SeverityWarning},
		{`[relations][using type="group" id="3"][item path="Name"][/using][/relations]`,
			"[item] inside a [using] block renders nothing: [using] switches to another entity, so the [each] or [relations] element is not available there", SeverityWarning},
		{`[relations][using type="group" id="3"][calc expr="item.GroupID"][/using][/relations]`,
			"[calc] inside a [using] block cannot read item", SeverityWarning},
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

	for _, ok := range []string{
		`[relations]`,
		`[relations type="Depends on" direction="out" limit="10"]`,
		`[relations direction="in" limit="100"]`,
		`[relations]<a href="/group?id=[item path='GroupID']">[item path="GroupName"]</a> [calc expr="item.GroupID * 2"][else]none[/relations]`,
		// [each] inside [relations] and the other way round: each [item] binds
		// to its innermost loop.
		`[relations][each path="tags"][item][/each][/relations]`,
		`[each path="tags"][relations][item path="Name"][/relations][/each]`,
	} {
		if issues := Lint(ok, LintOptions{Known: known}); len(issues) != 0 {
			t.Errorf("%s: expected no issues, got %+v", ok, issues)
		}
	}
}
