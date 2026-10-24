package api_tests

import (
	"context"
	"fmt"
	"sort"
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
	note("rank-other", "osprey", inside)
	principal := &auth.Principal{UserID: 1, Username: "guest", Role: models.RoleGuest, ScopeGroupID: &inside.ID}
	confined := tc.AppCtx.WithPrincipal(principal)
	queries := []string{
		`type = note AND TEXT ~ "kestrel" ORDER BY RANK`,
		// A row that does not match sorts after every match.
		`type = note AND (TEXT ~ "kestrel" OR name = "rank-other") ORDER BY RANK`,
		`type = note AND NOT TEXT ~ "kestrel" ORDER BY RANK, name ASC`,
	}
	// Only the visible notes, in the order they came.
	visible := func(summary string) string {
		var kept []string
		for _, name := range strings.Split(summary, ",") {
			if strings.HasPrefix(name, "rank-") {
				kept = append(kept, name)
			}
		}
		return strings.Join(kept, ",")
	}
	order := func(appCtx *application_context.MahresourcesContext) []string {
		var out []string
		for _, q := range queries {
			out = append(out, visible(runConfinedMRQL(t, appCtx, q)))
		}
		return out
	}
	before, unconfinedBefore := order(confined), order(tc.AppCtx)
	// Hidden documents that hold the searched term and shift the corpus
	// statistics bm25 uses.
	for i := 0; i < 40; i++ {
		note(fmt.Sprintf("hidden-%02d", i), "kestrel "+strings.Repeat("pad ", 2000), outside)
	}
	after, unconfinedAfter := order(confined), order(tc.AppCtx)
	for i, q := range queries {
		if before[i] != after[i] {
			t.Errorf("%s: hidden documents changed a confined order: %q, then %q", q, before[i], after[i])
		}
	}
	want := []string{"rank-long,rank-short", "rank-long,rank-short,rank-other", "rank-other"}
	if tc.AppCtx.MRQLStaticEnv().Postgres {
		// ts_rank scores each row alone: three matches outrank one, for
		// everyone, and hidden rows change nothing.
		for i, q := range queries {
			if before[i] != want[i] || unconfinedBefore[i] != want[i] || unconfinedAfter[i] != want[i] {
				t.Errorf("Postgres %s: confined %q, unconfined %q then %q, want %q", q, before[i], unconfinedBefore[i], unconfinedAfter[i], want[i])
			}
		}
		return
	}
	for i, q := range queries {
		if before[i] != want[i] {
			t.Errorf("SQLite confined %s = %q, want matches by highest ID first, then non-matches: %q", q, before[i], want[i])
		}
	}
	// The hazard is real: the same hidden documents reverse bm25's order.
	if unconfinedBefore[0] != "rank-short,rank-long" || unconfinedAfter[0] != "rank-long,rank-short" {
		t.Errorf("SQLite unconfined RANK = %q, then %q; want bm25 reversed by hidden documents", unconfinedBefore[0], unconfinedAfter[0])
	}
}

func TestMRQLScopeReachesTheWholeSubtree(t *testing.T) {
	runMRQLScopeReachesTheWholeSubtree(t, SetupTestEnv(t))
}

// runMRQLScopeReachesTheWholeSubtree: an unconfined user's explicit SCOPE is
// the whole subtree at any depth, excludes a sibling subtree, and ends at a
// group-tree cycle without repeating a row.
func runMRQLScopeReachesTheWholeSubtree(t *testing.T, tc *TestContext) {
	create := func(g *models.Group) *models.Group {
		t.Helper()
		if err := tc.DB.Create(g).Error; err != nil {
			t.Fatal(err)
		}
		return g
	}
	root := create(&models.Group{Name: "scope-root"})
	prev := root
	var names []string
	for i := 1; i <= 60; i++ {
		prev = create(&models.Group{Name: fmt.Sprintf("s%02d", i), OwnerId: &prev.ID})
		names = append(names, prev.Name)
	}
	if err := tc.DB.Create(&models.Resource{Name: "scope-deep-res", OwnerId: &prev.ID, Hash: "sd", Location: "sd"}).Error; err != nil {
		t.Fatal(err)
	}
	sibling := create(&models.Group{Name: "sibling-root"})
	create(&models.Group{Name: "sibling-child", OwnerId: &sibling.ID})
	// A cycle, which the write path refuses but a direct edit can make.
	c1 := create(&models.Group{Name: "cyc-1"})
	c2 := create(&models.Group{Name: "cyc-2", OwnerId: &c1.ID})
	if err := tc.DB.Model(&models.Group{}).Where("id = ?", c1.ID).Update("owner_id", c2.ID).Error; err != nil {
		t.Fatal(err)
	}

	want := strings.Join(append(names, "scope-root"), ",")
	for query, expected := range map[string]string{
		fmt.Sprintf(`type = group SCOPE %d LIMIT 100`, root.ID):                         want,
		fmt.Sprintf(`type = resource SCOPE %d`, root.ID):                                "scope-deep-res",
		fmt.Sprintf(`type = group SCOPE %d`, sibling.ID):                                "sibling-child,sibling-root",
		fmt.Sprintf(`type = group SCOPE %d`, c1.ID):                                     "cyc-1,cyc-2",
		fmt.Sprintf(`count:type = group SCOPE %d`, c2.ID):                               "total=2",
		fmt.Sprintf(`type = group SCOPE %d GROUP BY parent COUNT()`, c1.ID):             "count=1 parent=cyc-1; count=1 parent=cyc-2",
		fmt.Sprintf(`type = group SCOPE %d GROUP BY parent COUNT() LIMIT 100`, root.ID): "",
	} {
		count := strings.HasPrefix(query, "count:")
		q := strings.TrimPrefix(query, "count:")
		parsed, err := mrql.Parse(q)
		if err == nil {
			err = mrql.Validate(parsed)
		}
		if err != nil {
			t.Fatalf("%s: %v", q, err)
		}
		// The shortcode executor and the API resolve an explicit SCOPE first.
		scopeID, err := tc.AppCtx.ResolveMRQLScope(parsed)
		if err != nil {
			t.Fatalf("%s: %v", q, err)
		}
		var got string
		switch {
		case count:
			total, err := tc.AppCtx.CountMRQLScoped(context.Background(), parsed, scopeID)
			if err != nil {
				t.Fatalf("%s: %v", q, err)
			}
			got = fmt.Sprintf("total=%d", total)
		case parsed.GroupBy != nil:
			parsed.EntityType = mrql.ExtractEntityType(parsed)
			result, err := tc.AppCtx.ExecuteMRQLGroupedWithScope(context.Background(), parsed, scopeID)
			if err != nil {
				t.Fatalf("%s: %v", q, err)
			}
			got = summarizeMRQLResult(t, result)
		default:
			result, err := tc.AppCtx.ExecuteMRQLScoped(context.Background(), parsed, scopeID)
			if err != nil {
				t.Fatalf("%s: %v", q, err)
			}
			rows := strings.Split(summarizeMRQLResult(t, result), ",")
			sort.Strings(rows)
			got = strings.Join(rows, ",")
		}
		if expected == "" {
			// Every level groups under its own parent: 60 rows of one, and the
			// root under the parent it has none of.
			if n := strings.Count(got, "count=1 "); n != 61 {
				t.Errorf("%s: %d groups counted at their parent, want 61: %.200q", query, n, got)
			}
			continue
		}
		if got != expected {
			t.Errorf("%s:\n got %.300q\nwant %.300q", query, got, expected)
		}
	}
}
