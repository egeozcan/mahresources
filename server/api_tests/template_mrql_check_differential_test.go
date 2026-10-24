package api_tests

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"mahresources/application_context"
	"mahresources/auth"
	"mahresources/models"
	"mahresources/models/types"
	"mahresources/mrql"
	"mahresources/server/template_handlers/template_filters"
	"mahresources/shortcodes"
)

// mrqlDifferentialQueries is the query corpus of the differential test, by
// feature. TestMRQLDifferentialCorpusCoverage keeps it representative: every
// field of every entity and every language feature appears in it.
var mrqlDifferentialQueries = map[string][]string{
	"common fields": {
		`type = resource AND id > 0`, `type = note AND name = "plan"`, `type = group AND description ~ "*x*"`,
		`type = resource AND created > -30d`, `type = note AND updated < "2030-01-01"`, `type = group AND tags = "x"`,
		`type = resource AND guid = "00000000-0000-0000-0000-000000000000"`, `type = note AND meta.status = "done"`,
		`name = "x"`, `tags IS EMPTY`,
	},
	"resource fields": {
		`type = resource AND groups = "x"`, `type = resource AND group = "x"`, `type = resource AND owner = "x"`,
		`type = resource AND category = 1`, `type = resource AND contentType ~ "image/*"`, `type = resource AND fileSize > 1mb`,
		`type = resource AND width >= 100`, `type = resource AND height < 50`, `type = resource AND originalName ~ "*.jpg"`,
		`type = resource AND originalLocation ~ "http*"`, `type = resource AND hash = "abc"`, `type = resource AND notes = "x"`,
		`type = resource AND similarImages IS NOT EMPTY`,
	},
	"note fields": {
		`type = note AND groups = "x"`, `type = note AND group = "x"`, `type = note AND owner.name = "x"`,
		`type = note AND noteType = 1`, `type = note AND startDate > -7d`, `type = note AND endDate IS NULL`,
		`type = note AND shared IS NOT NULL`, `type = note AND resources IS EMPTY`,
	},
	"group fields": {
		`type = group AND category = 1`, `type = group AND url ~ "https*"`, `type = group AND parent = "x"`,
		`type = group AND children IS NOT EMPTY`, `type = group AND resources.count > 0`, `type = group AND notes IS EMPTY`,
	},
	"traversal and hierarchy": {
		`type = group AND parent.name = "x"`, `type = group AND parent.meta.secret = "x"`, `type = group AND parent.tags = "x"`,
		`type = resource AND owner.parent.name = "x"`, `type = group AND children.name = "x"`, `type = group AND parent.name IS NULL`,
		`type = group AND ancestors.name = "x"`, `type = group AND descendants.tags = "x"`, `type = resource AND ancestors.meta.region = "eu"`,
		`type = group AND parent.name != "x"`,
	},
	"operators": {
		`type = resource AND tags IN ("a", "b")`, `type = resource AND tags NOT IN ("a")`, `type = note AND name !~ "*draft*"`,
		`type = resource AND width BETWEEN 10 AND 20`, `type = note AND created NOT BETWEEN -30d AND -1d`,
		`type = note AND (name = "a" OR name = "b")`, `type = note AND NOT name = "a"`, `type = resource AND name ~* "^a"`,
		`type = note AND TEXT ~ "plan"`, `type = resource AND tags.count = 0`, `type = note AND groups.count >= 1`,
		`type = resource AND SIMILAR TO resource(1) ORDER BY distance ASC`, `type = resource AND SIMILAR TO resource(1) WITHIN 2`,
	},
	"ordering and paging": {
		`type = resource ORDER BY name ASC LIMIT 5`, `type = note ORDER BY created DESC, name ASC`, `type = resource ORDER BY RANDOM() LIMIT 5`,
		`type = note AND TEXT ~ "plan" ORDER BY RANK`, `type = group ORDER BY resources.count DESC`, `type = resource LIMIT 5 OFFSET 2`,
		`type = resource LIMIT 50000`, `type = resource OFFSET 20000`,
	},
	"group by": {
		`type = resource GROUP BY contentType COUNT()`, `type = resource GROUP BY contentType`, `type = note GROUP BY created.month COUNT() ORDER BY created.month ASC`,
		`type = resource GROUP BY hash COUNT() HAVING COUNT() > 1`, `type = resource GROUP BY contentType COUNT() SUM(fileSize) AVG(width)`,
		`type = group GROUP BY parent.meta.secret COUNT()`, `type = resource GROUP BY owner.parent.name COUNT()`, `name = "x" GROUP BY name`,
	},
	"errors": {
		`type = resource AND`, `type = "book"`, `type = group AND parent = $self`, `type = resource AND width = "wide"`,
		`contentType ~ "image/*"`, `type = note AND meta.due-date > "2026-01-01"`, `type = resource SCOPE 1.5`, `type = note AND HAS tags`,
	},
}

// mrqlDifferentialFeatures are language features the corpus must exercise.
var mrqlDifferentialFeatures = []string{
	"GROUP BY", "COUNT()", "HAVING", "SUM(", "AVG(", "created.month", "BETWEEN", "NOT BETWEEN", "RANDOM()", "RANK", "SIMILAR TO", "WITHIN",
	"ancestors.", "descendants.", "parent.", "owner.", "children.", ".count", "IS EMPTY", "IS NOT EMPTY", "IS NULL", " IN (", "NOT IN (",
	"~*", "!~", "TEXT ~", "meta.", " OR ", "NOT ", "ORDER BY", "LIMIT", "OFFSET", "SCOPE",
}

func TestMRQLDifferentialCorpusCoverage(t *testing.T) {
	var all []string
	for _, queries := range mrqlDifferentialQueries {
		all = append(all, queries...)
	}
	text := strings.Join(all, "\n")
	for _, feature := range mrqlDifferentialFeatures {
		if !strings.Contains(text, feature) {
			t.Errorf("no corpus query uses %q", feature)
		}
	}
	for label, et := range map[string]mrql.EntityType{"resource": mrql.EntityResource, "note": mrql.EntityNote, "group": mrql.EntityGroup} {
		names := append(mrql.FieldNames(mrql.EntityUnspecified), mrql.FieldNames(et)...)
		for _, name := range names {
			used := regexp.MustCompile(`(?m)^type = ` + label + `\b.*\b` + regexp.QuoteMeta(name) + `\b`)
			if name == "tags" || name == "name" || name == "id" || name == "created" || name == "updated" || name == "description" || name == "guid" {
				// Common fields are covered on one entity each.
				used = regexp.MustCompile(`\b` + regexp.QuoteMeta(name) + `\b`)
			}
			if !used.MatchString(text) {
				t.Errorf("no %s query uses the field %q", label, name)
			}
		}
	}
}

func TestTemplateMRQLCheckMatchesRendering(t *testing.T) {
	runTemplateMRQLCheckMatchesRendering(t, SetupTestEnv(t))
}

// runTemplateMRQLCheckMatchesRendering is the differential test for
// application_context.CheckTemplateMRQL: for every case, the linter with the
// static check reports an MRQL error exactly when rendering the same markup on
// this database shows one. Each case is markup, not a bare query, so the
// attribute parsing, the limit and buckets overrides, params, numbered
// conditional sources and [elseif] guards go through both paths.
//
// Every case renders twice: unconfined, and for a principal confined to the
// group the template renders for, whose parent and related rows sit outside
// its subtree. The confined output must hold nothing of the outside; the
// unconfined output must, for some cases, so that assertion can fail.
//
// Parity limits, left out of the corpus: whether a SCOPE group (by ID or
// name), a scope= group or a saved= query exists, and whether a scope name
// is ambiguous, is database state the static check cannot see; it is
// decided at render time. Checks that need rows (bucket materialisation per
// key) are out of reach too.
func runTemplateMRQLCheckMatchesRendering(t *testing.T, tc *TestContext) {
	create := func(v any) {
		t.Helper()
		if err := tc.DB.Create(v).Error; err != nil {
			t.Fatal(err)
		}
	}
	outsideParent := &models.Group{Name: "OUTSIDE-parent", Meta: types.JSON(`{"secret":"OUTSIDE-secret"}`)}
	create(outsideParent)
	outsideGroup := &models.Group{Name: "OUTSIDE-group"}
	create(outsideGroup)
	group := &models.Group{Name: "Differential", OwnerId: &outsideParent.ID}
	create(group)
	create(&models.Note{Name: "plan", OwnerId: &group.ID, Groups: []*models.Group{outsideGroup}})
	create(&models.Resource{Name: "photo", OwnerId: &group.ID, Hash: "h", Location: "l", ContentType: "image/png", Groups: []*models.Group{outsideGroup}})
	create(&models.Note{Name: "OUTSIDE-note", OwnerId: &outsideGroup.ID})

	var queries []string
	for _, feature := range sortedCorpusKeys() {
		queries = append(queries, mrqlDifferentialQueries[feature]...)
	}
	queries = append(queries, fmt.Sprintf(`type = note SCOPE %d`, group.ID))
	// GROUP BY over every relation and field an entity groups by, rendered
	// with the default scope="entity", which joins the scope subtree.
	for _, et := range []string{"resource", "note", "group"} {
		for _, field := range []string{"owner", "groups", "tags", "category", "noteType", "parent", "children", "name", "contentType", "created.month"} {
			queries = append(queries, fmt.Sprintf(`type = %s GROUP BY %s COUNT()`, et, field), fmt.Sprintf(`type = %s GROUP BY %s`, et, field))
		}
	}
	var corpus []string
	for _, q := range queries {
		corpus = append(corpus, `[mrql query='`+q+`' format="table"]`)
	}
	corpus = append(corpus,
		`[mrql query='type = resource' limit="10001"]`,
		`[mrql query='type = resource GROUP BY contentType' buckets="1001"]`,
		`[mrql query='type = note AND name = $n' param-n="x"]`,
		`[mrql query='type = note AND name = $n']`,
		`[mrql query='type = note' param-n="x"]`,
		`[mrql saved="none" query='type = resource AND']`,
		`[mrql query='type = group GROUP BY parent COUNT()' scope="global" format="table"]`,
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

	env := tc.AppCtx.MRQLStaticEnv()
	validate := func(q string, opts shortcodes.MRQLCheckOptions) error {
		return application_context.CheckTemplateMRQL(q, opts, env)
	}
	principal := &auth.Principal{UserID: 1, Username: "guest", Role: models.RoleGuest, ScopeGroupID: &group.ID}
	confined := tc.AppCtx.WithPrincipal(principal)
	render := func(appCtx *application_context.MahresourcesContext, markup string) string {
		meta := template_filters.BuildMetaContextForEntity(group, appCtx)
		return shortcodes.Process(context.Background(), markup, *meta, nil, template_filters.BuildQueryExecutor(appCtx))
	}

	staticErrors, runtimeErrors, outsideShown := 0, 0, 0
	for _, markup := range corpus {
		static := false
		for _, issue := range shortcodes.Lint(markup, shortcodes.LintOptions{Known: shortcodes.KnownFromBuiltins(), ValidateMRQL: validate}) {
			if strings.HasPrefix(issue.Message, "MRQL error") {
				static = true
			}
		}
		unconfined := render(tc.AppCtx, markup)
		runtime := strings.Contains(unconfined, "mrql-error")
		if static != runtime {
			t.Errorf("%s\nstatic error=%t, rendering error=%t: %s", markup, static, runtime, unconfined)
		}
		if strings.Contains(unconfined, "0x") && strings.Contains(markup, "COUNT()") {
			t.Errorf("%s renders pointers: %s", markup, unconfined)
		}
		scoped := render(confined, markup)
		if scopedErr := strings.Contains(scoped, "mrql-error"); scopedErr != static {
			t.Errorf("%s\nstatic error=%t, confined rendering error=%t: %s", markup, static, scopedErr, scoped)
		}
		if strings.Contains(scoped, "OUTSIDE") {
			t.Errorf("%s\na confined principal's render shows outside data: %s", markup, scoped)
		}
		if static {
			staticErrors++
		}
		if runtime {
			runtimeErrors++
		}
		if strings.Contains(unconfined, "OUTSIDE") {
			outsideShown++
		}
	}
	if staticErrors < 10 || len(corpus)-runtimeErrors < 10 {
		t.Fatalf("the corpus must hold both verdicts: %d errors out of %d", runtimeErrors, len(corpus))
	}
	if outsideShown < 3 {
		t.Fatalf("mutation step: only %d unconfined renders show outside data, so the confined check has nothing to catch", outsideShown)
	}
}

func sortedCorpusKeys() []string {
	keys := make([]string, 0, len(mrqlDifferentialQueries))
	for k := range mrqlDifferentialQueries {
		keys = append(keys, k)
	}
	for i := range keys {
		for j := i + 1; j < len(keys); j++ {
			if keys[j] < keys[i] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}
	return keys
}
