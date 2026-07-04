package shortcodes

import (
	"errors"
	"strings"
	"testing"
)

// findIssue returns the first issue whose message contains substr, or nil.
func findIssue(issues []LintIssue, substr string) *LintIssue {
	for i := range issues {
		if strings.Contains(issues[i].Message, substr) {
			return &issues[i]
		}
	}
	return nil
}

func TestLint(t *testing.T) {
	known := KnownFromBuiltins()

	tests := []struct {
		name       string
		input      string
		wantSubstr string   // a message substring that must appear ("" = expect no issues)
		wantSev    string   // expected severity of that issue
		wantNone   []string // substrings that must NOT appear
	}{
		{
			name:  "valid meta",
			input: `[meta path="cooking.time"]`,
		},
		{
			name:  "valid conditional block",
			input: "[conditional path=\"x\" eq=\"y\"]yes[/conditional]",
		},
		{
			name:  "valid mrql inline",
			input: `[mrql query="resources" format="list"]`,
		},
		{
			name:       "meta missing path",
			input:      `[meta]`,
			wantSubstr: `missing required attribute "path"`,
			wantSev:    SeverityError,
		},
		{
			name:       "mrql missing query and saved",
			input:      `[mrql format="list"]`,
			wantSubstr: `requires a "query" or "saved"`,
			wantSev:    SeverityError,
		},
		{
			name:       "unclosed conditional block",
			input:      `[conditional path="x" eq="y"]hello`,
			wantSubstr: `must be a block`,
			wantSev:    SeverityError,
		},
		{
			name:       "orphan closing mrql",
			input:      `[/mrql]`,
			wantSubstr: `orphan closing tag`,
			wantSev:    SeverityError,
		},
		{
			name:       "inline shortcode with closing tag",
			input:      `[meta path="x"]text[/meta]`,
			wantSubstr: `inline shortcode and cannot have a closing tag`,
			wantSev:    SeverityError,
		},
		{
			name:       "conditional without operator",
			input:      "[conditional path=\"x\"]a[/conditional]",
			wantSubstr: `needs a comparison operator`,
			wantSev:    SeverityError,
		},
		{
			name:       "conditional without target",
			input:      "[conditional eq=\"y\"]a[/conditional]",
			wantSubstr: `needs a "path", "field", or "mrql"`,
			wantSev:    SeverityError,
		},
		{
			name:       "conditional with two else",
			input:      "[conditional path=\"x\" eq=\"y\"]a[else]b[else]c[/conditional]",
			wantSubstr: `more than one [else]`,
			wantSev:    SeverityError,
		},
		{
			name:  "conditional with one else is valid",
			input: "[conditional path=\"x\" eq=\"y\"]a[else]b[/conditional]",
		},
		{
			name:       "unknown attribute on documented shortcode",
			input:      `[meta path="x" bogus="1"]`,
			wantSubstr: `unknown attribute "bogus"`,
			wantSev:    SeverityWarning,
		},
		{
			name:     "param wildcard is known",
			input:    `[mrql saved="r" param-tag="x"]`,
			wantNone: []string{"unknown attribute"},
		},
		{
			name:       "misspelled builtin (single-char typo)",
			input:      `[met path="x"]`,
			wantSubstr: `did you mean [meta]`,
			wantSev:    SeverityInfo,
		},
		{
			name:       "misspelled conditional stays literal",
			input:      "[condtional path=\"x\" eq=\"y\"]a[/condtional]",
			wantSubstr: `did you mean [conditional]`,
			wantSev:    SeverityInfo,
		},
		{
			name:       "malformed plugin shortcode",
			input:      `[plugin:foo]`,
			wantSubstr: `malformed plugin shortcode`,
			wantSev:    SeverityInfo,
		},
		{
			name:     "plain html brackets not flagged",
			input:    `<div>styles[class] and array[0]</div>`,
			wantNone: []string{"did you mean", "unknown shortcode", "malformed"},
		},
		{
			name:       "meta hide-empty and default conflict",
			input:      `[meta path="x" hide-empty="true" default="n/a"]`,
			wantSubstr: `hide-empty wins`,
			wantSev:    SeverityWarning,
		},
		{
			name:     "meta default alone is fine",
			input:    `[meta path="x" default="n/a"]`,
			wantNone: []string{"hide-empty wins"},
		},
		{
			name:     "conditional numbered-suffix attrs not flagged",
			input:    `[conditional path="a" eq="1" path2="b" gte2="5" combine="any"]x[/conditional]`,
			wantNone: []string{"unknown attribute"},
		},
		{
			name:     "conditional new operators not flagged",
			input:    `[conditional path="s" in="a,b" matches="^x" gte="1" lte="9"]x[/conditional]`,
			wantNone: []string{"unknown attribute", "needs a comparison operator"},
		},
		{
			name:       "conditional invalid matches regex",
			input:      `[conditional path="s" matches="([bad"]x[/conditional]`,
			wantSubstr: `invalid regular expression in matches`,
			wantSev:    SeverityError,
		},
		{
			name:       "conditional invalid matches2 regex",
			input:      `[conditional path="s" eq="1" path2="t" matches2="([bad"]x[/conditional]`,
			wantSubstr: `invalid regular expression in matches2`,
			wantSev:    SeverityError,
		},
		{
			name:     "elseif divider not flagged as unknown shortcode",
			input:    `[conditional path="s" eq="1"]a[elseif path="s" eq="2"]b[else]c[/conditional]`,
			wantNone: []string{"did you mean", "unknown shortcode", "malformed"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issues := Lint(tt.input, LintOptions{Known: known})

			for _, none := range tt.wantNone {
				if got := findIssue(issues, none); got != nil {
					t.Errorf("expected no issue containing %q, got %+v", none, *got)
				}
			}

			if tt.wantSubstr == "" {
				if len(issues) != 0 {
					t.Errorf("expected no issues, got %+v", issues)
				}
				return
			}

			got := findIssue(issues, tt.wantSubstr)
			if got == nil {
				t.Fatalf("expected an issue containing %q, got %+v", tt.wantSubstr, issues)
			}
			if tt.wantSev != "" && got.Severity != tt.wantSev {
				t.Errorf("issue %q: expected severity %q, got %q", tt.wantSubstr, tt.wantSev, got.Severity)
			}
			if got.Start < 0 || got.End > len(tt.input) || got.Start > got.End {
				t.Errorf("issue %q: invalid offsets [%d,%d] for input len %d", tt.wantSubstr, got.Start, got.End, len(tt.input))
			}
		})
	}
}

func TestLintMRQLSyntax(t *testing.T) {
	known := KnownFromBuiltins()
	validate := func(q string) error {
		if strings.Contains(q, "BAD") {
			return errors.New("syntax error near BAD")
		}
		return nil
	}

	issues := Lint(`[mrql query="BAD SYNTAX"]`, LintOptions{Known: known, ValidateMRQL: validate})
	got := findIssue(issues, "MRQL error in query")
	if got == nil {
		t.Fatalf("expected MRQL syntax issue, got %+v", issues)
	}
	if got.Severity != SeverityError {
		t.Errorf("expected error severity, got %q", got.Severity)
	}

	// A conditional's mrql attribute is also validated.
	issues = Lint("[conditional mrql=\"BAD\" gt=\"0\"]x[/conditional]", LintOptions{Known: known, ValidateMRQL: validate})
	if findIssue(issues, "MRQL error in mrql") == nil {
		t.Fatalf("expected MRQL error on conditional mrql attr, got %+v", issues)
	}

	// Valid query produces no MRQL issue.
	issues = Lint(`[mrql query="resources"]`, LintOptions{Known: known, ValidateMRQL: validate})
	if findIssue(issues, "MRQL error") != nil {
		t.Errorf("expected no MRQL issue for valid query, got %+v", issues)
	}
}

func TestLintMRQLErrorAnchorsToAttr(t *testing.T) {
	known := KnownFromBuiltins()
	validate := func(q string) error {
		if strings.Contains(q, "BAD") {
			return errors.New("syntax error near BAD")
		}
		return nil
	}

	// "query=" is a suffix of "param-query="; the error range must anchor to
	// the real query attribute, not the earlier param-query occurrence.
	input := `[mrql param-query="resources" query="BAD"]`
	issues := Lint(input, LintOptions{Known: known, ValidateMRQL: validate})
	got := findIssue(issues, "MRQL error in query")
	if got == nil {
		t.Fatalf("expected MRQL syntax issue, got %+v", issues)
	}
	wantStart := strings.Index(input, ` query="BAD"`) + 1
	if got.Start != wantStart {
		t.Errorf("expected issue to start at %d (the query attr), got %d", wantStart, got.Start)
	}
}

func TestLintUndocumentedPluginSkipsAttrChecks(t *testing.T) {
	// A plugin shortcode present in the catalogue but undocumented: attribute
	// checks are skipped, but structural rules still apply.
	known := KnownFromBuiltins()
	known["plugin:foo:badge"] = KnownShortcode{
		Name:       "plugin:foo:badge",
		Block:      BlockOptional,
		Attrs:      map[string]DocAttr{},
		Documented: false,
	}

	issues := Lint(`[plugin:foo:badge anything="1"]`, LintOptions{Known: known})
	if got := findIssue(issues, "unknown attribute"); got != nil {
		t.Errorf("undocumented plugin should not flag unknown attrs, got %+v", *got)
	}
}
