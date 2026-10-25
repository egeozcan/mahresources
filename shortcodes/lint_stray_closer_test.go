package shortcodes

import (
	"context"
	"strings"
	"testing"
)

func TestLintStrayDividerClosers(t *testing.T) {
	known := KnownFromBuiltins()
	bad := []struct {
		name, input, substr, stray string
	}{
		{"else in mrql", `[mrql query="type = note"]<li>x</li>[else]none[/else][/mrql]`, "[else] has no closing tag", "[/else]"},
		{"else in conditional", `[conditional path="a" eq="b"]A[else]B[/else][/conditional]`, "[else] has no closing tag", "[/else]"},
		{"else in each", `[each path="a"]<i>[item]</i>[else]none[/else][/each]`, "[else] has no closing tag", "[/else]"},
		{"stray elseif", `[conditional path="a" eq="b"]A[elseif path="a" eq="c"]C[/elseif][/conditional]`, "[elseif] has no closing tag", "[/elseif]"},
		{"unmatched header", `[mrql query="type = note"]x[/header][/mrql]`, "[/header]", "[/header]"},
		{"header outside mrql", `[header]h[/header]`, "[/header]", "[/header]"},
		{"unmatched footer", `<p>x</p>[/footer]`, "[/footer]", "[/footer]"},
		{"header with attributes stays literal", `[mrql query="type = note"][header class="x"]h[/header]x[/mrql]`, "[/header]", "[/header]"},
		{"header in another block stays literal", `[mrql query="type = note"][conditional path="a" eq="b"][header]h[/header][/conditional]x[/mrql]`, "[/header]", "[/header]"},
		{"second header pair stays literal", `[mrql query="type = note"][header]a[/header]x[header]b[/header][/mrql]`, "[/header]", "[/header]"},
	}
	for _, tc := range bad {
		t.Run(tc.name, func(t *testing.T) {
			issues := Lint(tc.input, LintOptions{Known: known})
			is := findIssue(issues, tc.substr)
			if is == nil {
				t.Fatalf("no issue containing %q in %+v", tc.substr, issues)
			}
			if is.Severity != SeverityError {
				t.Errorf("severity = %s, want error", is.Severity)
			}
			if got := tc.input[is.Start:is.End]; got != tc.stray {
				t.Errorf("issue anchored on %q, want %q", got, tc.stray)
			}
		})
	}

	good := []string{
		`[mrql query="type = note"][header]h[/header]<li>x</li>[footer]f[/footer][else]none[/mrql]`,
		`[conditional path="a" eq="b"]A[elseif path="a" eq="c"]C[else]B[/conditional]`,
		`[each path="a"]<i>[item]</i>[else]none[/each]`,
		`[mrql query="type = note"]<style>[header]h[/header]</style>x[/mrql]`,
		`[mrql query="type = note"][header]h[/header][mrql query="type = note"][header]n[/header]y[/mrql][/mrql]`,
	}
	for _, in := range good {
		for _, is := range Lint(in, LintOptions{Known: known}) {
			if strings.Contains(is.Message, "closing tag") || strings.Contains(is.Message, "literal text") {
				t.Errorf("%q: unexpected issue %+v", in, is)
			}
		}
	}
}

// A stray closer can survive when its branch renders; static syntax checks
// apply whether or not that branch is selected.
func TestStrayDividerClosersRenderLiterally(t *testing.T) {
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 1, Meta: makeMetaJSON(t, map[string]any{"a": "b"})}
	for _, in := range []string{
		`[conditional path="a" eq="zz"]A[else]B[/else][/conditional]`,
		`[conditional path="a" eq="zz"]A[elseif path="a" eq="b"]C[/elseif][/conditional]`,
		`<p>[/header]</p>`,
	} {
		if findIssue(Lint(in, LintOptions{Known: KnownFromBuiltins()}), "may be ignored or left as literal text") == nil {
			t.Errorf("%q: lint reported nothing", in)
		}
		out := Process(context.Background(), in, ctx, nil, nil)
		if !strings.Contains(out, "[/else]") && !strings.Contains(out, "[/elseif]") && !strings.Contains(out, "[/header]") {
			t.Errorf("%q rendered %q without a literal closer", in, out)
		}
	}
}

func TestValidSlotDividersAreConsumedWhenRendered(t *testing.T) {
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 1, Meta: makeMetaJSON(t, map[string]any{"a": "b", "arr": []string{"item"}})}
	executor := mockExecutor(&QueryResult{Mode: "flat", Items: []QueryResultItem{{EntityType: "note", EntityID: 2}}}, nil)
	for _, tc := range []struct{ input, want string }{
		{`[conditional path="a" eq="z"]A[elseif path="a" eq="b"]chosen[else]B[/conditional]`, "chosen"},
		{`[each path="arr"][item][else]none[/each]`, "item"},
		{`[mrql query="type = note"][header]head[/header]row[footer]foot[/footer][else]none[/mrql]`, "row"},
	} {
		out := Process(context.Background(), tc.input, ctx, nil, executor)
		if !strings.Contains(out, tc.want) {
			t.Fatalf("%q rendered %q without its positive control %q", tc.input, out, tc.want)
		}
		for _, tag := range []string{"[else]", "[elseif", "[header]", "[/header]", "[footer]", "[/footer]"} {
			if strings.Contains(out, tag) {
				t.Errorf("%q still contains %s", out, tag)
			}
		}
	}
	literal := "<code>&#91;/else]</code>"
	if issues := Lint(literal, LintOptions{Known: KnownFromBuiltins()}); len(issues) > 0 {
		t.Fatalf("escaped literal reported as syntax: %+v", issues)
	}
}

func TestMisplacedNestedHeaderStaysInvalidWhetherItsBodyRenders(t *testing.T) {
	input := `[mrql query="type = note"][each path="arr"][header]h[/header][/each]x[/mrql]`
	issue := findIssue(Lint(input, LintOptions{Known: KnownFromBuiltins()}), "[/header]")
	if issue == nil || issue.Severity != SeverityError {
		t.Fatalf("misplaced header not diagnosed: %#v", issue)
	}
	for _, populated := range []bool{false, true} {
		var meta []byte
		if populated {
			meta = []byte(`{"arr":["v"]}`)
		}
		executor := mockExecutor(&QueryResult{Mode: "flat", Items: []QueryResultItem{{EntityType: "note", EntityID: 2, Meta: meta}}}, nil)
		out := Process(context.Background(), input, MetaShortcodeContext{}, nil, executor)
		if !strings.Contains(out, "x") {
			t.Fatalf("missing rendered positive control: %q", out)
		}
		if strings.Contains(out, "[/header]") != populated {
			t.Fatalf("populated=%v rendered %q", populated, out)
		}
	}
}
