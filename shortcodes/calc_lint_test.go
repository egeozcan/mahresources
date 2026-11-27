package shortcodes

import (
	"strings"
	"testing"
)

func TestLintCalc(t *testing.T) {
	known := KnownFromBuiltins()
	cases := []struct {
		name, input, want, sev string
	}{
		{"syntax error", `[calc expr="qty * 2"]`, `[calc] expr: `, SeverityError},
		{"limit", `[calc expr="` + strings.Repeat("(", 33) + "1" + strings.Repeat(")", 33) + `"]`, `[calc] expr: `, SeverityError},
		{"bad decimals", `[calc expr="1" decimals="x"]`, `[calc] decimals must be a whole number from 0 to 10`, SeverityError},
		{"decimals too large", `[calc expr="1" decimals="11"]`, `[calc] decimals must be a whole number from 0 to 10`, SeverityError},
		{"bad format", `[calc expr="1" format="currency"]`, `[calc] format must be`, SeverityError},
		{"missing expr", `[calc decimals="2"]`, `[calc] is missing required attribute "expr"`, SeverityError},
		{"item outside each", `[calc expr="item.qty * 2"]`, `[calc] item operands have no value here`, SeverityWarning},
		{"item in using in each", `[each path="xs"][using rel="owner"][calc expr="item"][/using][/each]`, `[calc] item operands have no value here`, SeverityWarning},
		{"closer", `[calc expr="1"][/calc]`, `[calc] is an inline shortcode and cannot have a closing tag`, SeverityError},
		{"unknown attr", `[calc expr="1" raw="true"]`, `unknown attribute "raw" on [calc]`, SeverityWarning},
	}
	for _, c := range cases {
		is := findIssue(Lint(c.input, LintOptions{Known: known}), c.want)
		if is == nil {
			t.Errorf("%s: no issue containing %q in %v", c.name, c.want, Lint(c.input, LintOptions{Known: known}))
			continue
		}
		if is.Severity != c.sev {
			t.Errorf("%s: severity %s, want %s", c.name, is.Severity, c.sev)
		}
	}

	for _, ok := range []string{
		`[calc expr="meta.qty * meta.unit_price" decimals="2" format="grouped" default="-"]`,
		`[each path="lines"][calc expr="item.qty * item.price"][/each]`,
		`<meter value="[calc expr='meta.a / meta.b' default='0']" aria-label="x">[calc expr='meta.a' format='percent']</meter>`,
	} {
		if issues := Lint(ok, LintOptions{Known: known}); len(issues) != 0 {
			t.Errorf("%s: unexpected issues %v", ok, issues)
		}
	}
}

func TestLintCalcChecksMetaReferencesAgainstTheSchema(t *testing.T) {
	schema := `{"type":"object","properties":{"qty":{"type":"number"},"price":{"type":"number"}}}`
	got := lintMessages(`[calc expr="meta.qty * meta.prcie + meta.price"]`, LintOptions{Known: KnownFromBuiltins(), MetaSchema: schema})
	if !strings.Contains(got, `warning: [calc] path "prcie": "prcie" is not declared in the metadata schema`) {
		t.Fatalf("got:\n%s", got)
	}
	if !strings.Contains(got, "falls back to coalesce or the default attribute") {
		t.Fatalf("the [calc] warning should say what a typo does to [calc], got %q", got)
	}
	if strings.Count(got, "is not declared") != 1 || strings.Contains(got, "[meta]") {
		t.Fatalf("got:\n%s", got)
	}
	// Inside [each] (and [using], [mrql]) Meta is checked no more than [meta] is.
	if got := lintMessages(`[each path="qty"][calc expr="meta.nope"][/each]`, LintOptions{Known: KnownFromBuiltins(), MetaSchema: schema}); strings.Contains(got, "not declared") {
		t.Fatalf("got:\n%s", got)
	}
	// [meta]'s own message is unchanged.
	if got := lintMessages(`[meta path="nope"]`, LintOptions{Known: KnownFromBuiltins(), MetaSchema: schema}); !strings.Contains(got, `[meta] path "nope"`) {
		t.Fatalf("got:\n%s", got)
	}
}

func TestCalcBuiltinExamplesLintClean(t *testing.T) {
	for _, d := range BuiltinDocs() {
		if d.Name != "calc" {
			continue
		}
		if len(d.Examples) < 4 {
			t.Fatalf("[calc] has %d examples", len(d.Examples))
		}
		for _, ex := range d.Examples {
			if issues := Lint(ex.Code, LintOptions{Known: KnownFromBuiltins()}); len(issues) != 0 {
				t.Errorf("example %q: %v", ex.Title, issues)
			}
		}
		return
	}
	t.Fatal("no [calc] in BuiltinDocs")
}
