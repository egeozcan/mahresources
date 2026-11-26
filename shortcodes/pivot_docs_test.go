package shortcodes_test

import (
	"testing"

	"mahresources/application_context"
	"mahresources/shortcodes"
)

// lintPivotStatic lints src with the verdict rendering would give every query
// (application_context.CheckTemplateMRQL) on one static database, and counts
// the queries checked.
func lintPivotStatic(src string, env application_context.MRQLStaticEnv) ([]shortcodes.LintIssue, int) {
	checked := 0
	issues := shortcodes.Lint(src, shortcodes.LintOptions{
		Known: shortcodes.KnownFromBuiltins(),
		ValidateMRQL: func(query string, opts shortcodes.MRQLCheckOptions) error {
			checked++
			return application_context.CheckTemplateMRQL(query, opts, env)
		},
	})
	return issues, checked
}

var pivotStaticEnvs = []application_context.MRQLStaticEnv{{FTS: true}, {Postgres: true, FTS: true}}

// Every documented [pivot] example lints with no issue of any severity on
// SQLite and PostgreSQL, and each runs exactly one query.
func TestPivotBuiltinExamplesLintClean(t *testing.T) {
	var examples []shortcodes.DocExample
	for _, d := range shortcodes.BuiltinDocs() {
		if d.Name == "pivot" {
			examples = d.Examples
		}
	}
	if len(examples) == 0 {
		t.Fatal("no [pivot] examples documented")
	}
	for _, env := range pivotStaticEnvs {
		for _, ex := range examples {
			issues, checked := lintPivotStatic(ex.Code, env)
			for _, issue := range issues {
				t.Errorf("example %q (postgres=%t): %s %s", ex.Title, env.Postgres, issue.Severity, issue.Message)
			}
			if checked != 1 {
				t.Errorf("example %q (postgres=%t): %d queries checked, want 1", ex.Title, env.Postgres, checked)
			}
		}
	}
}

// The composed ORDER BY names the dimensions as written. A relation alias
// (group for groups) and the reserved word it spells still build on both
// dialects, as do the relation dimensions with an ID behind a name.
func TestPivotRelationDimensionsBuildOnBothDialects(t *testing.T) {
	for _, env := range pivotStaticEnvs {
		for _, src := range []string{
			`[pivot query="type = note" rows="group" cols="owner"]`,
			`[pivot query="type = resource" rows="owner" cols="groups" value="sum" field="fileSize"]`,
			`[pivot query="type = group" rows="parent" cols="children"]`,
			`[pivot query="type = note" rows="tags" cols="shared"]`,
			`[pivot query="type = resource" rows="notes" cols="created.week"]`,
		} {
			issues, checked := lintPivotStatic(src, env)
			for _, issue := range issues {
				t.Errorf("%s (postgres=%t): %s %s", src, env.Postgres, issue.Severity, issue.Message)
			}
			if checked != 1 {
				t.Errorf("%s (postgres=%t): %d queries checked", src, env.Postgres, checked)
			}
		}
	}
}
