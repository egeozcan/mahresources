package shortcodes

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLintCalendar(t *testing.T) {
	known := KnownFromBuiltins()
	cases := []struct {
		src, want, sev string
	}{
		{`[calendar]`, `[calendar] is missing required attribute "mrql"`, SeverityError},
		{`[calendar mrql='type = "note"' month="2026-13"]`, calendarMonthProblem, SeverityError},
		{`[calendar mrql='type = "note"' month="next"]`, calendarMonthProblem, SeverityError},
		{`[calendar mrql='type = "note"' date="name"]`, calendarDateProblem, SeverityError},
		{`[calendar mrql='type = "note" AND name = $n']`, "[calendar] mrql cannot use $parameters", SeverityError},
		{`[calendar mrql='type = "note" ORDER BY name ASC']`, "[calendar] mrql must be a filter only", SeverityError},
		{`[calendar mrql='type = "note" SCOPE 4']`, "[calendar] mrql must be a filter only", SeverityError},
		{`[calendar mrql='name = "x"']`, "[calendar] mrql must name one entity type", SeverityError},
		{`[calendar mrql='type = "note"'][/calendar]`, "[calendar] is an inline shortcode and cannot have a closing tag", SeverityError},
		{`[reload][calendar mrql='type = "note"'][/reload]`, "[calendar] cannot be used inside a [reload] button face", SeverityError},
		{`[calendar mrql='type = "note"' week-start="sun"]`, `unknown attribute "week-start"`, SeverityWarning},
		{`[calender mrql='type = "note"']`, "did you mean [calendar]", ""},
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

	// A filter that does not parse is the MRQL pass's to report, not a shape
	// error.
	issues := Lint(`[calendar mrql='type = "note" AND (']`, LintOptions{Known: known})
	assert.Empty(t, issues)

	for _, ok := range []string{
		`[calendar mrql='type = "note"']`,
		`[calendar mrql='type = "note"' date="meta.due"]`,
		`[calendar mrql='type = "resource"' date="created" month="2026-03" scope="global"]`,
		`[calendar mrql='type = group AND (name = "a" OR name = "b")' date="updated" month="current" scope="entity"]`,
	} {
		if issues := Lint(ok, LintOptions{Known: known}); len(issues) != 0 {
			t.Errorf("%s: expected no issues, got %+v", ok, issues)
		}
	}
}

// Lint validates the two queries the renderer runs, with the renderer's
// limits, for the authored month or calendarNow's.
func TestLintCalendarValidatesTheComposedQueries(t *testing.T) {
	pinCalendarClock(t, time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC), time.UTC)
	type call struct {
		query string
		opts  MRQLCheckOptions
	}
	var calls []call
	validate := func(q string, o MRQLCheckOptions) error {
		calls = append(calls, call{q, o})
		return nil
	}
	Lint(`[calendar mrql='type = "note"' date="meta.due"]`, LintOptions{Known: KnownFromBuiltins(), ValidateMRQL: validate})
	want, err := composeCalendarQueries(`type = "note"`, "meta.due", time.Date(2026, 7, 1, 0, 0, 0, 0, time.Local))
	require.NoError(t, err)
	require.Len(t, calls, 2)
	assert.Equal(t, call{want.List, MRQLCheckOptions{Limit: 101}}, calls[0])
	assert.Equal(t, call{want.Count, MRQLCheckOptions{Limit: 1000}}, calls[1])

	calls = nil
	Lint(`[calendar mrql='type = "note"' month="2025-12"]`, LintOptions{Known: KnownFromBuiltins(), ValidateMRQL: validate})
	require.Len(t, calls, 2)
	assert.Contains(t, calls[0].query, `created >= "2025-12-01" AND created < "2026-01-01"`)

	// An unparsable filter: its parse error is reported once, at mrql=.
	calls = nil
	src := `[calendar mrql='type = "note" AND (']`
	issues := Lint(src, LintOptions{Known: KnownFromBuiltins(), ValidateMRQL: func(q string, o MRQLCheckOptions) error {
		calls = append(calls, call{q, o})
		return assert.AnError
	}})
	require.Len(t, issues, 1)
	assert.Contains(t, issues[0].Message, "MRQL error in mrql")
	assert.True(t, strings.HasPrefix(src[issues[0].Start:], "mrql="), src[issues[0].Start:])
	assert.Len(t, calls, 1)
}
