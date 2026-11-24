package shortcodes

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLintSimilar(t *testing.T) {
	known := KnownFromBuiltins()
	cases := []struct {
		src, want, sev string
	}{
		{`[similar size="xl"]`, `[similar] size must be "sm", "md" or "lg"`, SeverityError},
		{`[similar max-distance="12"]`, "[similar] max-distance must be a whole number from 0 to 11", SeverityError},
		{`[similar max-distance="2.5"]`, "[similar] max-distance must be a whole number from 0 to 11", SeverityError},
		{`[similar limit="0"]`, "[similar] limit must be a whole number from 1 to 24", SeverityError},
		{`[similar limit="25"]`, "[similar] limit must be a whole number from 1 to 24", SeverityError},
		{`[similar][/similar]`, "[similar] is an inline shortcode and cannot have a closing tag", SeverityError},
		{`[simlar]`, "did you mean [similar]", ""},
		{`[similar id="3"]`, `unknown attribute "id"`, SeverityWarning},
		{`[reload][similar][/reload]`, "[similar] cannot be used inside a [reload] button face", SeverityError},
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
		`[similar]`,
		`[similar max-distance="2" limit="12" size="sm" label="Possible duplicates"]`,
		`[similar max-distance="0" limit="1" size="lg" label=""]`,
		`[using type="resource" id-path="cover"][similar][/using]`,
	} {
		if issues := Lint(ok, LintOptions{Known: known}); len(issues) != 0 {
			t.Errorf("%s: expected no issues, got %+v", ok, issues)
		}
	}
}

// Every attribute error the linter reports is the one the page renders.
func TestLintSimilarMatchesRenderer(t *testing.T) {
	known := KnownFromBuiltins()
	for _, attrs := range []map[string]string{
		{"size": "xl"}, {"max-distance": "-1"}, {"max-distance": "abc"}, {"limit": "x"},
	} {
		_, problem := parseSimilarAttrs(attrs)
		src := "[similar"
		for k, v := range attrs {
			src += " " + k + `="` + v + `"`
		}
		src += "]"
		issue := findIssue(Lint(src, LintOptions{Known: known}), problem)
		if assert.NotNil(t, issue, src) {
			assert.Equal(t, problem, issue.Message)
			assert.Equal(t, SeverityError, issue.Severity)
		}
		f := &fakeSimilarExecutor{result: flat()}
		out := renderSimilar(t, attrs, similarCtx(), f)
		assert.Equal(t, shortcodeErrorMarker("similar", problem), out)
	}
}

func TestSimilarRefusedInsideAReloadFace(t *testing.T) {
	f := &fakeSimilarExecutor{result: flat(simItem(17, "A", "h", intp(1)))}
	out := Process(context.Background(), `[reload][similar][/reload]`, similarCtx(), nil, f.exec)
	assert.Contains(t, out, "[similar] cannot be used inside a [reload] button face")
	assert.NotContains(t, out, "similar-strip")
}
