package shortcodes

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLintCancelledReportsOneIssue(t *testing.T) {
	cctx, cancel := context.WithCancel(context.Background())
	cancel()
	in := strings.Repeat(`<a href="[meta path="x" inline="true"]">[meta path="y"]</a>`, 300)
	issues := Lint(in, LintOptions{Known: KnownFromBuiltins(), Context: cctx})
	require.Len(t, issues, 1)
	assert.Equal(t, SeverityError, issues[0].Severity)
	assert.Contains(t, issues[0].Message, "too complex")
}

func TestLintWithoutContextUnchanged(t *testing.T) {
	in := `[meta path="x" inline="true" raw="true"] <a href="[meta path="y" inline="true" raw="true"]">z</a>`
	a := Lint(in, LintOptions{Known: KnownFromBuiltins()})
	b := Lint(in, LintOptions{Known: KnownFromBuiltins(), Context: context.Background()})
	assert.Equal(t, a, b)
	assert.NotEmpty(t, a)
}

// A pattern that fits the source limit but compiles past the allowance.
func TestLintRegexAllowance(t *testing.T) {
	in := `[conditional path="a" matches="` + strings.Repeat("a{1000}", 100) + `"]x[/conditional]`
	issues := Lint(in, LintOptions{Known: KnownFromBuiltins()})
	var found bool
	for _, i := range issues {
		if i.Severity == SeverityError && strings.Contains(i.Message, "too complex") {
			found = true
		}
	}
	assert.True(t, found, "issues: %+v", issues)
	// Ordinary valid and invalid patterns keep their messages.
	issues = Lint(`[conditional path="a" matches="("]x[/conditional]`, LintOptions{Known: KnownFromBuiltins()})
	require.NotEmpty(t, issues)
	assert.Contains(t, issues[0].Message, "invalid regular expression")
}

// The substitution phase is admitted before classification: an exhausted
// allowance returns before any parse.
func TestAttributeContextsForStopsWhenWorkIsSpent(t *testing.T) {
	w := newLintWork(nil)
	w.maxBytes = 10
	input := `<a href="[meta path="x" inline="true"]">`
	out, _ := attributeContextsForW(w, input, []bareValueSpan{{start: 9, end: 38}})
	assert.Error(t, w.Err())
	assert.Empty(t, out)
}

// Nested conditionals with [else] dividers: the divider count is unchanged by
// the span search rewrite.
func TestCountTopLevelElseNested(t *testing.T) {
	inner := `a[else]b[conditional path="x" eq="1"]c[else]d[/conditional][else]e`
	assert.Equal(t, 2, countTopLevelElse(inner))
	assert.Equal(t, 0, countTopLevelElse(`[each path="v"]x[else]y[/each]`))
	big := strings.Repeat(`[conditional path="x" eq="1"]a[else]b[/conditional]`, 500) + "[else]"
	assert.Equal(t, 1, countTopLevelElse(big))
	_ = fmt.Sprint
}
