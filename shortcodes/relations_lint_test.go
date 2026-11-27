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
		{`[item path="Name"]`, "[item] is only bound at the top level of the item branch", SeverityWarning},
		{`[calc expr="item.GroupID * 2"]`, "[calc] item operands have no value here", SeverityWarning},
		{`[relations][using type="group" id="3"][item path="Name"][/using][/relations]`,
			"[item] is only bound at the top level of the item branch", SeverityWarning},
		{`[relations][using type="group" id="3"][calc expr="item.GroupID"][/using][/relations]`,
			"[calc] item operands have no value here", SeverityWarning},
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

func TestLintItemBindingMatchesTheTopLevelElementBranch(t *testing.T) {
	for _, tc := range []struct {
		src string
		tag string
	}{
		{`[relations][conditional path="show" not-empty="true"][item path="GroupName"][/conditional][/relations]`, "item"},
		{`[relations][else][calc expr="coalesce(item.GroupID, 0)"][/relations]`, "calc"},
		{`[relations][tabs][tab label="Related group"][item path="GroupName"][/tab][/tabs][/relations]`, "item"},
		{`[each path="tags"][tabs][tab label="Tags" lazy="true"][item][/tab][/tabs][/each]`, "item"},
		{`[each path="tags"][relations]yes[else][item][/relations][/each]`, "item"},
	} {
		issues := Lint(tc.src, LintOptions{Known: KnownFromBuiltins()})
		issue := findIssue(issues, "top level of the item branch")
		if issue == nil || issue.Severity != SeverityWarning || tc.src[issue.Start:issue.Start+1+len(tc.tag)] != "["+tc.tag {
			t.Errorf("%s: expected an unbound [%s] warning on the token, got %+v", tc.src, tc.tag, issues)
		}
	}
	for _, src := range []string{
		`[relations][conditional path="show" not-empty="true"]y[else]n[/conditional][item path="GroupName"][else]None[/relations]`,
		`[relations][else][each path="tags"][item][/each][/relations]`,
		`[each path="tags"][tabs][tab label="Groups"][relations][item path="GroupName"][/relations][/tab][/tabs][/each]`,
	} {
		if issues := Lint(src, LintOptions{Known: KnownFromBuiltins()}); len(issues) > 0 {
			t.Errorf("%s: expected item binding to the inner loop, got %+v", src, issues)
		}
	}
}
