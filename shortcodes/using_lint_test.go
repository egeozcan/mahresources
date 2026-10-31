package shortcodes

import (
	"strings"
	"testing"
)

func TestLintUsing(t *testing.T) {
	known := KnownFromBuiltins()
	tests := []struct {
		name     string
		src      string
		want     string // substring of the expected message; "" means no [using] issue
		severity string
	}{
		{"valid id", `[using type="group" id="7"]x[/using]`, "", ""},
		{"valid rel", `[using rel="owner"]x[else]y[/using]`, "", ""},
		{"valid id-path", `[using type="note" id-path="links.note"]x[/using]`, "", ""},
		{"valid id-prop", `[using type="group" id-prop="OwnerId"]x[/using]`, "", ""},
		{"valid query", `[using query="type = note ORDER BY created DESC" param-x="1"]x[/using]`, "", ""},

		{"inline form", `[using rel="owner"]`, "[using] must be a block", SeverityError},
		{"no source", `[using type="group"]x[/using]`, "needs one of id, id-path, id-prop, rel or query", SeverityError},
		{"two sources", `[using type="group" id="1" id-path="a"]x[/using]`, "found id, id-path", SeverityError},
		{"missing type", `[using id="1"]x[/using]`, `with id= needs type=`, SeverityError},
		{"bad type", `[using type="tag" id="1"]x[/using]`, `not "tag"`, SeverityError},
		{"bad id", `[using type="group" id="x1"]x[/using]`, "positive whole number", SeverityError},
		{"zero id", `[using type="group" id="0"]x[/using]`, "positive whole number", SeverityError},
		{"bad rel", `[using rel="parent"]x[/using]`, `rel must be "owner" or "root"`, SeverityError},
		{"type with rel", `[using type="group" rel="owner"]x[/using]`, "ignores type= with rel=", SeverityWarning},
		{"type with query", `[using type="note" query="type = note ORDER BY name"]x[/using]`, "ignores type= with query=", SeverityWarning},
		{"scope with id", `[using type="group" id="1" scope="root"]x[/using]`, "only with query=", SeverityWarning},
		{"param with id", `[using type="group" id="1" param-x="1"]x[/using]`, "only with query=", SeverityWarning},
		{"category id-prop", `[using type="group" id-prop="CategoryId"]x[/using]`, "holds a category ID", SeverityWarning},
		{"group by", `[using query="type = note GROUP BY name"]x[/using]`, "cannot use GROUP BY", SeverityError},
		{"no type in query", `[using query='name = "x" ORDER BY name']x[/using]`, "does not name an entity type", SeverityWarning},
		{"no order", `[using query="type = note"]x[/using]`, "no ORDER BY", SeverityWarning},
		{"unknown attr", `[using type="group" id="1" limit="3"]x[/using]`, `unknown attribute "limit"`, SeverityWarning},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			issues := Lint(tc.src, LintOptions{Known: known})
			if tc.want == "" {
				for _, is := range issues {
					if strings.Contains(is.Message, "[using]") || strings.Contains(is.Message, "using") {
						t.Fatalf("unexpected issue: %+v", is)
					}
				}
				return
			}
			got := findIssue(issues, tc.want)
			if got == nil {
				t.Fatalf("expected %q, got %+v", tc.want, issues)
			}
			if got.Severity != tc.severity {
				t.Errorf("severity = %q, want %q", got.Severity, tc.severity)
			}
		})
	}
}

func TestLintUsingQueryIsValidatedWithLimitOne(t *testing.T) {
	var gotQuery string
	var gotOpts MRQLCheckOptions
	validate := func(q string, opts MRQLCheckOptions) error {
		gotQuery, gotOpts = q, opts
		return nil
	}
	Lint(`[using query="type = note AND name = $n ORDER BY name" param-n="Index"]x[/using]`, LintOptions{Known: KnownFromBuiltins(), ValidateMRQL: validate})
	if gotQuery != "type = note AND name = $n ORDER BY name" {
		t.Fatalf("query not validated, got %q", gotQuery)
	}
	if gotOpts.Limit != 1 || gotOpts.Params["n"] != "Index" {
		t.Errorf("opts = %+v, want limit 1 and the bound param", gotOpts)
	}
}

func TestLintItemInsideUsingInsideEach(t *testing.T) {
	known := KnownFromBuiltins()
	const msg = "[item] inside a [using] block renders nothing"

	src := `[each path="rows"][using rel="owner"][item][/using][/each]`
	issue := findIssue(Lint(src, LintOptions{Known: known}), msg)
	if issue == nil {
		t.Fatal("expected a diagnostic for [item] inside [using]")
	}
	if want := strings.Index(src, "[item]"); issue.Start != want {
		t.Errorf("anchored at %d, want %d", issue.Start, want)
	}

	// An [each] inside the [using] gives the [item] its own loop back.
	src = `[using rel="owner"][each path="rows"][item][/each][/using]`
	if issue := findIssue(Lint(src, LintOptions{Known: known}), msg); issue != nil {
		t.Errorf("[item] in an [each] nested in [using] flagged: %+v", issue)
	}
	// Beside the [using], inside the same [each], it is fine.
	src = `[each path="rows"][item][using rel="owner"]x[/using][/each]`
	if issue := findIssue(Lint(src, LintOptions{Known: known}), msg); issue != nil {
		t.Errorf("[item] beside [using] flagged: %+v", issue)
	}
}

func TestLintUsingElseIsASlot(t *testing.T) {
	issues := Lint(`[using rel="owner"]x[else]y[/else][/using]`, LintOptions{Known: KnownFromBuiltins()})
	if findIssue(issues, "[else] has no closing tag") == nil {
		t.Errorf("expected the stray [/else] diagnostic, got %+v", issues)
	}
	if spans := SlotTagSpans("using", `x[else]y`); len(spans) != 1 || spans[0] != [2]int{1, 7} {
		t.Errorf("SlotTagSpans(using) = %v", spans)
	}
}
