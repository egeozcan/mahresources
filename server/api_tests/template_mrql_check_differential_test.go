package api_tests

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"mahresources/application_context"
	"mahresources/models"
	"mahresources/server/template_handlers/template_filters"
	"mahresources/shortcodes"
)

// TestTemplateMRQLCheckMatchesRendering is the differential test for
// application_context.CheckTemplateMRQL: for every case, the linter with the
// static check reports an MRQL error exactly when rendering the same markup on
// this database shows one. Each case is markup, not a bare query, so the
// attribute parsing, the limit and buckets overrides, params, numbered
// conditional sources and [elseif] guards go through both paths.
//
// Excluded, as a known gap: whether a SCOPE group, a scope= group or a saved
// query exists is database state the static check cannot see, so those cases
// are not in the corpus.
func TestTemplateMRQLCheckMatchesRendering(t *testing.T) {
	tc := SetupTestEnv(t)
	group := &models.Group{Name: "Differential"}
	if err := tc.DB.Create(group).Error; err != nil {
		t.Fatal(err)
	}
	if err := tc.DB.Create(&models.Note{Name: "plan", OwnerId: &group.ID}).Error; err != nil {
		t.Fatal(err)
	}
	env := tc.AppCtx.MRQLStaticEnv()
	metaCtx := template_filters.BuildMetaContextForEntity(group, tc.AppCtx)
	executor := template_filters.BuildQueryExecutor(tc.AppCtx)

	queries := []string{
		`type = resource`,
		`type = resource AND tags = "x"`,
		`type = note AND name ~ "a*"`,
		`name = "x"`,
		`contentType ~ "image/*"`,
		`type = resource GROUP BY contentType COUNT()`,
		`type = resource GROUP BY contentType`,
		`name = "x" GROUP BY name`,
		`type = note AND TEXT ~ "plan" ORDER BY RANK`,
		`type = resource AND name ~* "^a"`,
		`type = resource AND width = "wide"`,
		`type = "book"`,
		`type = group AND parent = $self`,
		`type = resource ORDER BY RANDOM() LIMIT 5`,
		`type = resource AND created BETWEEN -30d AND -1d`,
		`type = group AND ancestors.name = "Archive"`,
		`type = resource AND SIMILAR TO resource(1) ORDER BY distance ASC`,
		`type = resource AND tags.count = 0`,
		`type = note GROUP BY created.month COUNT() ORDER BY created.month ASC`,
		`type = resource GROUP BY hash COUNT() HAVING COUNT() > 1`,
		`type = resource AND`,
		`type = resource LIMIT 50000`,
		`type = resource OFFSET 20000`,
		`type = resource SCOPE 1.5`,
		fmt.Sprintf(`type = note SCOPE %d`, group.ID),
		`type = note AND meta.due-date > "2026-01-01"`,
	}
	var corpus []string
	for _, q := range queries {
		corpus = append(corpus, `[mrql query='`+q+`']`)
	}
	corpus = append(corpus,
		`[mrql query='type = resource' limit="10001"]`,
		`[mrql query='type = resource GROUP BY contentType' buckets="1001"]`,
		`[mrql query='type = note AND name = $n' param-n="x"]`,
		`[mrql query='type = note AND name = $n']`,
		`[mrql query='type = note' param-n="x"]`,
		`[mrql saved="none" query='type = resource AND']`,
		`[conditional mrql="type = note" gt="-1" mrql2="type = resource AND" gt2="0"]x[/conditional]`,
		`[conditional mrql="type = note" gt="-1" mrql2="type = resource" limit2="10001" gt2="-1"]x[/conditional]`,
		`[conditional mrql="type = note" gt="-1" mrql2="type = resource" buckets2="1001" gt2="-1"]x[/conditional]`,
		`[conditional path="x" eq="a"]A[elseif mrql="type = resource AND" gt="0"]B[/conditional]`,
		`[conditional path="x" eq="a"]A[elseif mrql="type = note" gt="-1"]B[/conditional]`,
		`[conditional path="x" eq="a"]A[elseif path="y" eq="b" mrql2="type = resource" limit2="10001" gt2="0"]B[/conditional]`,
	)
	// The built-in examples that run a query of their own (a saved= example
	// depends on a stored query, and is out of reach).
	for _, d := range shortcodes.BuiltinDocs() {
		for _, ex := range d.Examples {
			if strings.Contains(ex.Code, "mrql") && !strings.Contains(ex.Code, "saved=") {
				corpus = append(corpus, ex.Code)
			}
		}
	}

	validate := func(q string, opts shortcodes.MRQLCheckOptions) error {
		return application_context.CheckTemplateMRQL(q, opts, env)
	}
	staticErrors, runtimeErrors := 0, 0
	for _, markup := range corpus {
		static := false
		for _, issue := range shortcodes.Lint(markup, shortcodes.LintOptions{Known: shortcodes.KnownFromBuiltins(), ValidateMRQL: validate}) {
			if strings.HasPrefix(issue.Message, "MRQL error") {
				static = true
			}
		}
		rendered := shortcodes.Process(context.Background(), markup, *metaCtx, nil, executor)
		runtime := strings.Contains(rendered, "mrql-error")
		if static != runtime {
			t.Errorf("%s\nstatic error=%t, rendering error=%t: %s", markup, static, runtime, rendered)
		}
		if static {
			staticErrors++
		}
		if runtime {
			runtimeErrors++
		}
	}
	if staticErrors < 10 || len(corpus)-runtimeErrors < 10 {
		t.Fatalf("the corpus must hold both verdicts: %d errors out of %d", runtimeErrors, len(corpus))
	}
}
