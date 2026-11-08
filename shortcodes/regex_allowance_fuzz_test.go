package shortcodes

import (
	"context"
	"errors"
	"regexp"
	"testing"
)

// The allowance's pre-scan reads regex syntax by hand, so it must agree with
// Go's parser on what is valid: it may refuse a pattern as too complex, but it
// must never panic, never call a valid pattern invalid, and never accept an
// invalid one. The seeds are the shapes the scanner treats specially.
func FuzzRegexAllowanceAgreesWithRegexp(f *testing.F) {
	for _, seed := range []string{
		`[[:]]`, `[[:`, `[:`, `[[:alpha:]]`, `[[:^alpha:]]`, `[[:alpha:]`, `[[:nope:]]`,
		`[a[:b]`, `[]]`, `[^]]`, `[]a]`, `[^]a]`, `[[]`, `[\]]`, `[\Q]\E]`,
		`\Q[[:]]\E`, `\Q`, `\`, `\p`, `\pL`, `\p{L}`, `\P{Greek}`, `[\pL]`, `[^\PL]`,
		`a{0,1000}`, `(a)`, `(`, `)`, `[`, `]`, `[[:alpha:]-z]`, `x[[:digit:]]y`,
		`[a-]`, `[-a]`, `[:alpha:]`, `[[:word:][:space:]]`, `[[::]]`, `[[:a:]]`,
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, pattern string) {
		if len(pattern) > 256 {
			return
		}
		_, want := regexp.Compile(pattern)
		_, got := NewRegexAllowance(context.Background()).Compile(pattern)
		var tooComplex *RegexTooComplexError
		if errors.As(got, &tooComplex) {
			return
		}
		if (want == nil) != (got == nil) {
			t.Fatalf("%q: regexp.Compile err = %v, allowance err = %v", pattern, want, got)
		}
	})
}

// `[[:]]` is a valid class of "[" and ":" followed by a literal "]". A
// conditional using it renders and lints instead of panicking.
func TestConditionalMatchesPOSIXLookalike(t *testing.T) {
	sc := Shortcode{Name: "conditional", Attrs: map[string]string{"path": "s", "matches": "[[:]]"}, InnerContent: "yes", IsBlock: true}
	hit := MetaShortcodeContext{Meta: makeMetaJSON(t, map[string]any{"s": ":]"})}
	if got := RenderConditionalShortcode(context.Background(), sc, hit, nil, nil, 0); got != "yes" {
		t.Fatalf("render = %q", got)
	}
	issues := Lint(`[conditional path="s" matches="[[:]]"]yes[/conditional]`, LintOptions{Known: KnownFromBuiltins()})
	for _, iss := range issues {
		if iss.Severity == SeverityError {
			t.Fatalf("lint error on a valid pattern: %+v", iss)
		}
	}
}
