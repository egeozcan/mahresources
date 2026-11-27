package shortcodes

import (
	"strings"
	"testing"
)

func TestLintExcerpt(t *testing.T) {
	known := KnownFromBuiltins()
	cases := []struct {
		name, input, want, sev string
	}{
		{"path and field", `[excerpt path="a" field="Name"]`, `[excerpt] takes path or field, not both`, SeverityError},
		{"dotted field", `[excerpt field="Owner.Name"]`, `[excerpt] field= names one field of this entity; use [using] to preview a related entity`, SeverityWarning},
		{"words zero", `[excerpt words="0"]`, `[excerpt] words= must be a whole number from 1 to 200`, SeverityWarning},
		{"words text", `[excerpt words="x"]`, `[excerpt] words= must be a whole number from 1 to 200`, SeverityWarning},
		{"words too many", `[excerpt words="201"]`, `[excerpt] words= must be a whole number from 1 to 200`, SeverityWarning},
		{"in href", `<a href="[excerpt]">x</a>`, `[excerpt]`, SeverityWarning},
		{"closer", `[excerpt][/excerpt]`, `[excerpt] is an inline shortcode and cannot have a closing tag`, SeverityError},
		{"near miss", `[exerpt]`, `did you mean [excerpt]?`, SeverityInfo},
		{"more in reload face", `[reload][excerpt more="More"][/reload]`, `[excerpt] more= inside a [reload] button face renders no link`, SeverityWarning},
		{"unknown attr", `[excerpt raw="true"]`, `unknown attribute "raw" on [excerpt]`, SeverityWarning},
	}
	for _, c := range cases {
		issues := Lint(c.input, LintOptions{Known: known})
		is := findIssue(issues, c.want)
		if is == nil {
			t.Errorf("%s: no issue containing %q in %v", c.name, c.want, issues)
			continue
		}
		if is.Severity != c.sev {
			t.Errorf("%s: severity %s, want %s", c.name, is.Severity, c.sev)
		}
	}

	for _, ok := range []string{
		`[excerpt words="20" more="x"]`,
		`[excerpt words="1"] [excerpt words="200"] [excerpt words=" 7 "]`,
		`[excerpt field="Name"] [excerpt path="intro.text" default="none"]`,
		`<span title="[excerpt words='25']">x</span>`,
		`<a href="[link]">[excerpt more="x"]</a>`,
		`[reload][excerpt][/reload]`,
		`[mrql query="type = note"]<p>[excerpt words="12"]</p>[/mrql]`,
	} {
		if issues := Lint(ok, LintOptions{Known: known}); len(issues) != 0 {
			t.Errorf("%s: unexpected issues %v", ok, issues)
		}
	}
}

func TestLintExcerptAttributePlacementWithAndWithoutMore(t *testing.T) {
	for _, c := range []struct{ name, input, warning string }{
		{"event handler", `<button onclick="[excerpt%s]">x</button>`, "event handler"},
		{"Alpine directive", `<button @click="[excerpt%s]">x</button>`, "Alpine directive"},
		{"URL scheme", `<a href="[excerpt%s]">x</a>`, `can still choose the scheme of the "href" URL`},
		{"srcdoc", `<iframe srcdoc="[excerpt%s]"></iframe>`, `browser decodes and parses as HTML`},
		{"unquoted", `<span title=[excerpt%s]>x</span>`, "unquoted attribute value"},
		{"inert", `<span aria-label="[excerpt%s]">x</span>`, ""},
		{"duplicate", `<span title="first" title="[excerpt%s]">x</span>`, ""},
		{"text", `<span>[excerpt%s]</span>`, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			for _, more := range []string{"", " more='Read more'", " more=' '"} {
				input := strings.Replace(c.input, "%s", more, 1)
				issues := Lint(input, LintOptions{Known: KnownFromBuiltins()})
				if c.warning != "" {
					if issue := findIssue(issues, c.warning); issue == nil || issue.Severity != SeverityWarning {
						t.Errorf("%s: want warning %q, got %v", input, c.warning, issues)
					}
				}
				issue := findIssue(issues, "[excerpt] more= cannot be used in an attribute")
				if more == " more='Read more'" && c.name != "text" {
					if issue == nil || issue.Severity != SeverityError {
						t.Errorf("%s: want link-markup error, got %v", input, issues)
					}
				} else if issue != nil || c.warning == "" && len(issues) != 0 {
					t.Errorf("%s: unexpected issues %v", input, issues)
				}
			}
		})
	}
}

func TestLintExcerptChecksPathAgainstTheSchema(t *testing.T) {
	schema := `{"type":"object","properties":{"intro":{"type":"string"}}}`
	got := lintMessages(`[excerpt path="intor"]`, LintOptions{Known: KnownFromBuiltins(), MetaSchema: schema})
	if !strings.Contains(got, `warning: [excerpt] path "intor": "intor" is not declared in the metadata schema`) {
		t.Fatalf("got:\n%s", got)
	}
	for _, quiet := range []LintOptions{
		{Known: KnownFromBuiltins(), MetaSchema: schema},
		{Known: KnownFromBuiltins()},
	} {
		input := `[excerpt path="intro"]`
		if quiet.MetaSchema == "" {
			input = `[excerpt path="intor"]`
		}
		if got := lintMessages(input, quiet); got != "" {
			t.Errorf("%s: unexpected issues:\n%s", input, got)
		}
	}
	// Inside [mrql] the excerpt reads another entity's Meta.
	if got := lintMessages(`[mrql query="type = note"][excerpt path="intor"][/mrql]`, LintOptions{Known: KnownFromBuiltins(), MetaSchema: schema}); got != "" {
		t.Errorf("unexpected issues inside [mrql]:\n%s", got)
	}
}
