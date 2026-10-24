package api_tests

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"mahresources/application_context"
	"mahresources/auth"
	"mahresources/models"
	"mahresources/mrql"
)

func runConfinedMRQL(t *testing.T, appCtx *application_context.MahresourcesContext, query string) string {
	t.Helper()
	parsed, err := mrql.Parse(query)
	if err == nil {
		err = mrql.Validate(parsed)
	}
	if err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	var result any
	if parsed.GroupBy != nil {
		parsed.EntityType = mrql.ExtractEntityType(parsed)
		result, err = appCtx.ExecuteMRQLGroupedWithScope(context.Background(), parsed, 0)
	} else {
		result, err = appCtx.ExecuteMRQLScoped(context.Background(), parsed, 0)
	}
	if err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return summarizeMRQLResult(t, result)
}

func TestMRQLConfinedSeesTheWholeSubtree(t *testing.T) {
	runMRQLConfinedSeesTheWholeSubtree(t, SetupTestEnv(t))
}

// runMRQLConfinedSeesTheWholeSubtree: a confined principal's MRQL subtree is
// the same closure its other reads use, at any depth. A 60-deep chain under
// the scope root is visible at every level: as a result row, through a
// traversal, children, a count and GROUP BY.
func runMRQLConfinedSeesTheWholeSubtree(t *testing.T, tc *TestContext) {
	root := &models.Group{Name: "chain-root"}
	if err := tc.DB.Create(root).Error; err != nil {
		t.Fatal(err)
	}
	prev := root
	for i := 1; i <= 60; i++ {
		g := &models.Group{Name: fmt.Sprintf("g%02d", i), OwnerId: &prev.ID}
		if err := tc.DB.Create(g).Error; err != nil {
			t.Fatal(err)
		}
		prev = g
	}
	if err := tc.DB.Create(&models.Resource{Name: "deep-res", OwnerId: &prev.ID, Hash: "deep", Location: "deep"}).Error; err != nil {
		t.Fatal(err)
	}
	principal := &auth.Principal{UserID: 1, Username: "guest", Role: models.RoleGuest, ScopeGroupID: &root.ID}
	confined := tc.AppCtx.WithPrincipal(principal)
	for query, want := range map[string]string{
		`type = group AND name = "g60"`:                         "g60",
		`type = group AND children.name = "g60"`:                "g59",
		`type = group AND name = "g59" AND children.count = 1`:  "g59",
		`type = group AND parent.name = "g59"`:                  "g60",
		`type = group AND name = "g60" GROUP BY parent COUNT()`: "count=1 parent=g59",
		`type = resource AND owner.name = "g60"`:                "deep-res",
		`type = resource GROUP BY owner COUNT()`:                "count=1 owner=g60",
	} {
		for name, appCtx := range map[string]*application_context.MahresourcesContext{"confined": confined, "unconfined": tc.AppCtx} {
			if got := runConfinedMRQL(t, appCtx, query); got != want {
				t.Errorf("%s %s:\n got %q\nwant %q", name, query, got, want)
			}
		}
	}
	if n := strings.Count(runConfinedMRQL(t, confined, `type = group`), ","); n != 60 {
		t.Errorf("a confined principal sees %d of 61 groups", n+1)
	}
}

func TestMRQLConfinedRankIgnoresHiddenDocuments(t *testing.T) {
	runMRQLConfinedRankIgnoresHiddenDocuments(t, SetupTestEnv(t))
}

// runMRQLConfinedRankIgnoresHiddenDocuments: SQLite's bm25 weighs a match by
// statistics over every document, hidden ones included, so a confined
// principal's ORDER BY RANK there is matches first, highest ID first, an
// order no hidden document can change. Postgres's ts_rank scores each row
// alone, so there a confined principal's order is the unconfined one, less
// the hidden rows.
func runMRQLConfinedRankIgnoresHiddenDocuments(t *testing.T, tc *TestContext) {
	if err := tc.AppCtx.InitFTS(); err != nil {
		t.Fatalf("full-text search must initialise: %v", err)
	}
	inside := &models.Group{Name: "rank-inside"}
	outside := &models.Group{Name: "rank-outside"}
	for _, g := range []*models.Group{inside, outside} {
		if err := tc.DB.Create(g).Error; err != nil {
			t.Fatal(err)
		}
	}
	note := func(name, description string, owner *models.Group) {
		t.Helper()
		if err := tc.DB.Create(&models.Note{Name: name, Description: description, OwnerId: &owner.ID}).Error; err != nil {
			t.Fatal(err)
		}
	}
	// bm25 normalises a match by the corpus's average document length: while
	// documents are short, "short" (one match, few words) outranks "long"
	// (three matches, many words); once hidden documents raise the average,
	// "long" outranks it.
	note("rank-short", "kestrel", inside)
	note("rank-long", "kestrel kestrel kestrel "+strings.Repeat("pad ", 27), inside)
	principal := &auth.Principal{UserID: 1, Username: "guest", Role: models.RoleGuest, ScopeGroupID: &inside.ID}
	confined := tc.AppCtx.WithPrincipal(principal)
	const query = `type = note AND TEXT ~ "kestrel" ORDER BY RANK`

	before := runConfinedMRQL(t, confined, query)
	unconfinedBefore := runConfinedMRQL(t, tc.AppCtx, query)
	// Hidden documents that shift the corpus statistics bm25 uses.
	for i := 0; i < 40; i++ {
		note(fmt.Sprintf("hidden-%02d", i), strings.Repeat("pad ", 2000), outside)
	}
	after := runConfinedMRQL(t, confined, query)
	unconfinedAfter := runConfinedMRQL(t, tc.AppCtx, query)
	if before != after {
		t.Errorf("hidden documents changed a confined order: %q, then %q", before, after)
	}
	if tc.AppCtx.MRQLStaticEnv().Postgres {
		if before != unconfinedBefore || after != unconfinedAfter {
			t.Errorf("Postgres: confined %q then %q, unconfined %q then %q", before, after, unconfinedBefore, unconfinedAfter)
		}
		return
	}
	if before != "rank-long,rank-short" {
		t.Errorf("SQLite confined RANK = %q, want matches by highest ID first", before)
	}
	// The hazard is real: the same hidden documents reverse bm25's order.
	if unconfinedBefore != "rank-short,rank-long" || unconfinedAfter != "rank-long,rank-short" {
		t.Errorf("SQLite unconfined RANK = %q, then %q; want bm25 reversed by hidden documents", unconfinedBefore, unconfinedAfter)
	}
}
