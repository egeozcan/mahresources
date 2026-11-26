package shortcodes

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLintPivot(t *testing.T) {
	known := KnownFromBuiltins()
	cases := []struct {
		src, want, sev string
	}{
		{`[pivot query="type = note" rows="meta.status" cols="owner"][/pivot]`, "[pivot] is an inline shortcode and cannot have a closing tag", SeverityError},
		{`[reload][pivot query="type = note" rows="meta.status" cols="owner"][/reload]`, "[pivot] cannot be used inside a [reload] button face", SeverityError},
		{`[pivot query="type = note" rows="meta.status" cols="owner" field="meta.hours"]`, `[pivot] field is read only with value="sum"; it is ignored`, SeverityWarning},
		{`[pivot query="type = note" rows="meta.status" cols="owner" scope="here"]`, `[pivot] scope must be entity, parent, root, global or a group ID; "here" counts in the entity's own group`, SeverityWarning},
		{`[pivot query="type = note" rows="meta.status" cols="owner" colour="red"]`, `unknown attribute "colour"`, SeverityWarning},
		{`[pivit query="type = note"]`, "did you mean [pivot]", ""},
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
		`[pivot query="type = note" rows="meta.status" cols="owner"]`,
		`[pivot query="type = resource" rows="contentType" cols="created.year" value="sum" field="fileSize" scope="global" max-rows="5" max-cols="3"]`,
		`[pivot query='type = group AND name ~ "a*"' rows="parent" cols="tags" scope="12" rows-label="Parent" cols-label="Tag" caption="Groups"]`,
		`[pivot query="type = note" rows="shared" cols="group" value="count" scope="entity"]`,
	} {
		if issues := Lint(ok, LintOptions{Known: known}); len(issues) != 0 {
			t.Errorf("%s: expected no issues, got %+v", ok, issues)
		}
	}
}

// Every refusal the renderer would show is reported once, word for word, at
// query=; a missing attribute only by the required-attribute check.
func TestLintPivotReportsTheRenderersErrorOnce(t *testing.T) {
	known := KnownFromBuiltins()
	for _, attrs := range []map[string]string{
		{"query": "type = note GROUP BY name", "rows": "meta.status", "cols": "owner"},
		{"query": `name = "x"`, "rows": "meta.status", "cols": "owner"},
		{"query": "type = note AND (", "rows": "meta.status", "cols": "owner"},
		{"query": "type = note", "rows": "owner.name", "cols": "meta.status"},
		{"query": "type = note", "rows": "created", "cols": "meta.status"},
		{"query": "type = note", "rows": "noteType", "cols": "meta.status"},
		{"query": "type = note", "rows": "group", "cols": "groups"},
		{"query": "type = note", "rows": "meta.status", "cols": "owner", "value": "avg"},
		{"query": "type = note", "rows": "meta.status", "cols": "owner", "value": "sum"},
		{"query": "type = note", "rows": "meta.status", "cols": "owner", "value": "sum", "field": "fileSize"},
	} {
		_, _, problem := pivotQuery(attrs)
		require.Error(t, problem, "%v", attrs)
		src := `[pivot query='` + attrs["query"] + `' rows="` + attrs["rows"] + `" cols="` + attrs["cols"] + `"`
		for _, k := range []string{"value", "field"} {
			if v, ok := attrs[k]; ok {
				src += " " + k + `="` + v + `"`
			}
		}
		src += "]"
		calls := 0
		issues := Lint(src, LintOptions{Known: known, ValidateMRQL: func(string, MRQLCheckOptions) error {
			calls++
			return nil
		}})
		var errs []LintIssue
		for _, is := range issues {
			if is.Severity == SeverityError {
				errs = append(errs, is)
			}
		}
		if assert.Len(t, errs, 1, src) {
			assert.Equal(t, problem.Error(), errs[0].Message)
			assert.True(t, strings.HasPrefix(errs[0].Message, "[pivot] "))
			start := strings.Index(src, "query=")
			assert.Equal(t, start, errs[0].Start, "anchored at query=: %s", src)
		}
		assert.Zero(t, calls, "a pivot that does not compose runs no query: %s", src)
	}

	issues := Lint(`[pivot query="type = note" rows="meta.status"]`, LintOptions{Known: known})
	require.Len(t, issues, 1)
	assert.Equal(t, `[pivot] is missing required attribute "cols"`, issues[0].Message)
}

func TestLintPivotValidatesTheComposedQuery(t *testing.T) {
	known := KnownFromBuiltins()
	var got []string
	var opts []MRQLCheckOptions
	issues := Lint(`[pivot query="type = note" rows="meta.status" cols="owner" value="sum" field="meta.hours"]`, LintOptions{Known: known, ValidateMRQL: func(q string, o MRQLCheckOptions) error {
		got = append(got, q)
		opts = append(opts, o)
		return nil
	}})
	assert.Empty(t, issues)
	require.Len(t, got, 1)
	assert.Equal(t, "(type = note) GROUP BY meta.status, owner COUNT() SUM(meta.hours) ORDER BY count DESC, meta.status ASC, owner ASC", got[0])
	assert.Equal(t, pivotFetchLimit+1, opts[0].Limit)

	issues = Lint(`[pivot query="type = note" rows="meta.status" cols="owner"]`, LintOptions{Known: known, ValidateMRQL: func(string, MRQLCheckOptions) error {
		return assert.AnError
	}})
	require.Len(t, issues, 1)
	assert.True(t, strings.HasPrefix(issues[0].Message, "MRQL error in query: "), issues[0].Message)
}
