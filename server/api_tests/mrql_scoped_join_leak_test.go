package api_tests

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"

	"mahresources/application_context"
	"mahresources/auth"
	"mahresources/models"
	"mahresources/models/types"
	"mahresources/mrql"
)

func TestMRQLJoinsStayInsideTheSubtree(t *testing.T) {
	runMRQLJoinsStayInsideTheSubtree(t, SetupTestEnv(t))
}

// summarizeMRQLResult is a result as the exact rows a reader sees: the names
// of a flat result in order, each aggregated row as sorted key=value pairs,
// and each bucket as its key and its items' names.
func summarizeMRQLResult(t *testing.T, result any) string {
	t.Helper()
	names := func(items any) []string {
		var out []string
		switch list := items.(type) {
		case []models.Resource:
			for _, r := range list {
				out = append(out, r.Name)
			}
		case []models.Note:
			for _, n := range list {
				out = append(out, n.Name)
			}
		case []models.Group:
			for _, g := range list {
				out = append(out, g.Name)
			}
		}
		return out
	}
	pairs := func(m map[string]any) string {
		raw, err := json.Marshal(m)
		if err != nil {
			t.Fatal(err)
		}
		var plain map[string]any
		_ = json.Unmarshal(raw, &plain)
		var kv []string
		for k, v := range plain {
			kv = append(kv, fmt.Sprintf("%s=%v", k, v))
		}
		sort.Strings(kv)
		return strings.Join(kv, " ")
	}
	switch r := result.(type) {
	case *application_context.MRQLResult:
		all := append(append(names(r.Resources), names(r.Notes)...), names(r.Groups)...)
		return strings.Join(all, ",")
	case *application_context.MRQLGroupedResult:
		var lines []string
		if r.Mode == "aggregated" {
			for _, row := range r.Rows {
				lines = append(lines, pairs(row))
			}
		} else {
			for _, bucket := range r.Groups {
				items := names(bucket.Items)
				sort.Strings(items)
				lines = append(lines, pairs(bucket.Key)+": "+strings.Join(items, ","))
			}
		}
		sort.Strings(lines)
		return strings.Join(lines, "; ")
	}
	t.Fatalf("unexpected result %T", result)
	return ""
}

// runMRQLJoinsStayInsideTheSubtree: for a principal confined to a subtree, a
// row outside it behaves as if it does not exist in every part of an MRQL
// query, not only the rows it returns: a joined parent, owner, group, note or
// resource, a junction row to one, a relation filter, a count, an existence
// test, a hierarchy walk, a similarity target or distance, and a shared image
// hash. Each query must give exactly the scoped result below, and the
// unscoped context a different one (the mutation step), so the fixture
// really holds outside rows that would change it.
func runMRQLJoinsStayInsideTheSubtree(t *testing.T, tc *TestContext) {
	create := func(v any) {
		t.Helper()
		if err := tc.DB.Create(v).Error; err != nil {
			t.Fatal(err)
		}
	}
	outsideTag := &models.Tag{Name: "outside-tag"}
	create(outsideTag)
	parent := &models.Group{Name: "OUTSIDE-P", Meta: types.JSON(`{"secret":"OUTSIDE-SECRET"}`), Tags: []*models.Tag{outsideTag}}
	create(parent)
	other := &models.Group{Name: "OUTSIDE-O", Meta: types.JSON(`{"secret":"OUTSIDE-SECRET"}`)}
	create(other)
	other2 := &models.Group{Name: "OUTSIDE-O2"}
	create(other2)
	root := &models.Group{Name: "inside-root", OwnerId: &parent.ID, Meta: types.JSON(`{"secret":"inside"}`)}
	create(root)
	child := &models.Group{Name: "inside-child", OwnerId: &root.ID}
	create(child)
	outsideNote := &models.Note{Name: "OUTSIDE-note", OwnerId: &other.ID}
	create(outsideNote)
	outsideRes := &models.Resource{Name: "OUTSIDE-res", OwnerId: &other.ID, Hash: "outside-hash", Location: "outside"}
	create(outsideRes)
	// inside-res has only hidden associations: two outside groups and an
	// outside note. It must group like inside-res2, which has none.
	insideRes := &models.Resource{Name: "inside-res", OwnerId: &root.ID, Hash: "inside-hash", Location: "inside", Groups: []*models.Group{other, other2}, Notes: []*models.Note{outsideNote}}
	create(insideRes)
	insideRes2 := &models.Resource{Name: "inside-res2", OwnerId: &root.ID, Hash: "h2", Location: "l2"}
	create(insideRes2)
	insideRes3 := &models.Resource{Name: "inside-res3", OwnerId: &root.ID, Hash: "h3", Location: "l3"}
	create(insideRes3)
	insideRes4 := &models.Resource{Name: "inside-res4", OwnerId: &root.ID, Hash: "h4", Location: "l4"}
	create(insideRes4)
	insideNote := &models.Note{Name: "inside-note", OwnerId: &root.ID, Groups: []*models.Group{other}, Resources: []*models.Resource{outsideRes}}
	create(insideNote)
	// Similarity to the outside resource: inside-res near, res3 and res4 at
	// distances only a distance ordering would reveal.
	create(&models.ResourceSimilarity{ResourceID1: insideRes.ID, ResourceID2: outsideRes.ID, HammingDistance: 0})
	create(&models.ResourceSimilarity{ResourceID1: outsideRes.ID, ResourceID2: insideRes3.ID, HammingDistance: 5})
	create(&models.ResourceSimilarity{ResourceID1: insideRes4.ID, ResourceID2: outsideRes.ID, HammingDistance: 9})
	// inside-res shares its exact image hash only with the outside resource.
	create(&models.ImageHash{ResourceId: &insideRes.ID, DHash: "dup"})
	create(&models.ImageHash{ResourceId: &outsideRes.ID, DHash: "dup"})

	principal := &auth.Principal{UserID: 1, Username: "guest", Role: models.RoleGuest, ScopeGroupID: &root.ID}
	scoped := tc.AppCtx.WithPrincipal(principal)
	run := func(appCtx *application_context.MahresourcesContext, query string) string {
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

	const allRes = "inside-res,inside-res2,inside-res3,inside-res4"
	cases := []struct{ query, want string }{
		// Joined rows named by GROUP BY; hidden ones fall in the NULL bucket,
		// and a hidden junction row adds no row of its own.
		{`type = group GROUP BY parent COUNT()`, "count=1 parent=<nil>; count=1 parent=inside-root"},
		{`type = group GROUP BY parent`, fmt.Sprintf("parent=<nil> parent_id=<nil>: inside-root; parent=inside-root parent_id=%d: inside-child", root.ID)},
		{`type = group GROUP BY parent.name COUNT()`, "count=1 parent.name=<nil>; count=1 parent.name=inside-root"},
		{`type = group GROUP BY parent.meta.secret COUNT()`, "count=1 parent.meta.secret=<nil>; count=1 parent.meta.secret=inside"},
		{`type = resource GROUP BY owner.parent.name COUNT()`, "count=4 owner.parent.name=<nil>"},
		{`type = resource GROUP BY groups COUNT()`, "count=4 groups=<nil>"},
		{`type = resource GROUP BY groups`, "groups=<nil> groups_id=<nil>: " + allRes},
		{`type = note GROUP BY groups COUNT()`, "count=1 groups=<nil>"},
		{`type = resource GROUP BY notes COUNT()`, "count=4 notes=<nil>"},
		{`type = resource GROUP BY notes`, "notes=<nil> notes_id=<nil>: " + allRes},
		{`type = note GROUP BY resources COUNT()`, "count=1 resources=<nil>"},
		// Filters as oracles: no inside row is selected by outside data.
		{`type = group AND parent.meta.secret = "OUTSIDE-SECRET"`, ""},
		{`type = group AND parent.name = "OUTSIDE-P"`, ""},
		{`type = group AND parent.tags = "outside-tag"`, ""},
		{`type = group AND ancestors.name = "OUTSIDE-P"`, ""},
		{`type = resource AND owner.parent.name = "OUTSIDE-P"`, ""},
		{`type = resource AND groups = "OUTSIDE-O"`, ""},
		{`type = note AND groups = "OUTSIDE-O"`, ""},
		{`type = resource AND notes = "OUTSIDE-note"`, ""},
		{`type = note AND resources = "OUTSIDE-res"`, ""},
		{`type = resource AND groups IS NOT EMPTY`, ""},
		{`type = note AND resources IS NOT EMPTY`, ""},
		{`type = resource AND groups.count > 0`, ""},
		{`type = note AND groups.count > 0`, ""},
		{`type = resource AND similarImages IS NOT EMPTY`, ""},
		{fmt.Sprintf(`type = resource AND SIMILAR TO resource(%d)`, outsideRes.ID), ""},
		// A hidden target matches nothing, so NOT SIMILAR TO keeps every row,
		// and distance to it orders like distance to no target: by name.
		{fmt.Sprintf(`type = resource AND NOT SIMILAR TO resource(%d) WITHIN 2 ORDER BY distance ASC, name ASC`, outsideRes.ID), allRes},
		{fmt.Sprintf(`type = resource AND NOT SIMILAR TO resource(%d) WITHIN 2 ORDER BY distance DESC, name ASC`, outsideRes.ID), allRes},
	}
	for _, c := range cases {
		t.Run(c.query, func(t *testing.T) {
			if got := run(scoped, c.query); got != c.want {
				t.Errorf("confined result:\n got %q\nwant %q", got, c.want)
			}
			if got := run(tc.AppCtx, c.query); got == c.want {
				t.Errorf("mutation step: the unscoped result equals the confined one, %q", got)
			}
		})
	}
	// A hidden distance target sorts exactly as a target that does not exist.
	for _, dir := range []string{"ASC", "DESC"} {
		missing := run(scoped, fmt.Sprintf(`type = resource AND NOT SIMILAR TO resource(999999) WITHIN 2 ORDER BY distance %s, name ASC`, dir))
		hidden := run(scoped, fmt.Sprintf(`type = resource AND NOT SIMILAR TO resource(%d) WITHIN 2 ORDER BY distance %s, name ASC`, outsideRes.ID, dir))
		if hidden != missing {
			t.Errorf("%s: a hidden target orders %q, a missing one %q", dir, hidden, missing)
		}
	}
}
