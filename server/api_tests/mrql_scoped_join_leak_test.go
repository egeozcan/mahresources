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
		if len(r.Order) > 0 {
			// Cross-entity: the merged order is the result's own.
			byID := map[string]string{}
			for _, x := range r.Resources {
				byID[fmt.Sprint("resource", x.ID)] = x.Name
			}
			for _, x := range r.Notes {
				byID[fmt.Sprint("note", x.ID)] = x.Name
			}
			for _, x := range r.Groups {
				byID[fmt.Sprint("group", x.ID)] = x.Name
			}
			var ordered []string
			for _, id := range r.Order {
				ordered = append(ordered, byID[fmt.Sprint(id.EntityType, id.ID)])
			}
			return "order:" + strings.Join(ordered, ",")
		}
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
		if r.Mode != "aggregated" {
			lines = append(lines, fmt.Sprintf("total=%d", r.TotalGroups))
		}
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
		// "count:" asks for the pagination total; "set:" for the rows as a
		// set, for an order that is random.
		kind := ""
		if before, after, ok := strings.Cut(query, ":"); ok && (before == "count" || before == "set") {
			kind, query = before, after
		}
		parsed, err := mrql.Parse(query)
		if err == nil {
			err = mrql.Validate(parsed)
		}
		if err != nil {
			t.Fatalf("%s: %v", query, err)
		}
		if kind == "count" {
			total, err := appCtx.CountMRQLScoped(context.Background(), parsed, 0)
			if err != nil {
				t.Fatalf("%s: %v", query, err)
			}
			return fmt.Sprintf("total=%d", total)
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
		summary := summarizeMRQLResult(t, result)
		if kind == "set" || (parsed.GroupBy == nil && len(parsed.OrderBy) == 0) {
			rows := strings.Split(summary, ",")
			sort.Strings(rows)
			summary = strings.Join(rows, ",")
		}
		return summary
	}

	const allRes = "inside-res,inside-res2,inside-res3,inside-res4"
	cases := []struct{ query, want, unscoped string }{
		// Joined rows named by GROUP BY; hidden ones fall in the NULL bucket,
		// and a hidden junction row adds no row of its own.
		{`type = group GROUP BY parent COUNT()`,
			"count=1 parent=<nil>; count=1 parent=inside-root",
			"count=1 parent=OUTSIDE-P; count=1 parent=inside-root; count=3 parent=<nil>"},
		{`type = group GROUP BY parent`,
			fmt.Sprintf("parent=<nil> parent_id=<nil>: inside-root; parent=inside-root parent_id=%d: inside-child; total=2", root.ID),
			fmt.Sprintf("parent=<nil> parent_id=<nil>: OUTSIDE-O,OUTSIDE-O2,OUTSIDE-P; parent=OUTSIDE-P parent_id=%d: inside-root; parent=inside-root parent_id=%d: inside-child; total=3", parent.ID, root.ID)},
		{`type = group GROUP BY parent.name COUNT()`,
			"count=1 parent.name=<nil>; count=1 parent.name=inside-root",
			"count=1 parent.name=OUTSIDE-P; count=1 parent.name=inside-root; count=3 parent.name=<nil>"},
		{`type = group GROUP BY parent.meta.secret COUNT()`,
			"count=1 parent.meta.secret=<nil>; count=1 parent.meta.secret=inside",
			"count=1 parent.meta.secret=OUTSIDE-SECRET; count=1 parent.meta.secret=inside; count=3 parent.meta.secret=<nil>"},
		{`type = resource GROUP BY owner.parent.name COUNT()`,
			"count=4 owner.parent.name=<nil>",
			"count=1 owner.parent.name=<nil>; count=4 owner.parent.name=OUTSIDE-P"},
		{`type = resource GROUP BY groups COUNT()`,
			"count=4 groups=<nil>",
			"count=1 groups=OUTSIDE-O; count=1 groups=OUTSIDE-O2; count=4 groups=<nil>"},
		{`type = resource GROUP BY groups`,
			"groups=<nil> groups_id=<nil>: " + allRes + "; total=1",
			fmt.Sprintf("groups=<nil> groups_id=<nil>: OUTSIDE-res,inside-res2,inside-res3,inside-res4; groups=OUTSIDE-O groups_id=%d: inside-res; groups=OUTSIDE-O2 groups_id=%d: inside-res; total=3", other.ID, other2.ID)},
		{`type = resource GROUP BY groups COUNT() HAVING COUNT() = 1`,
			"",
			"count=1 groups=OUTSIDE-O; count=1 groups=OUTSIDE-O2"},
		{`type = note GROUP BY groups COUNT()`,
			"count=1 groups=<nil>",
			"count=1 groups=<nil>; count=1 groups=OUTSIDE-O"},
		{`type = resource GROUP BY notes COUNT()`,
			"count=4 notes=<nil>",
			// resource_notes is one junction for both sides: inside-res links
			// the outside note, and inside-note links the outside resource.
			"count=1 notes=OUTSIDE-note; count=1 notes=inside-note; count=3 notes=<nil>"},
		{`type = resource GROUP BY notes`,
			"notes=<nil> notes_id=<nil>: " + allRes + "; total=1",
			fmt.Sprintf("notes=<nil> notes_id=<nil>: inside-res2,inside-res3,inside-res4; notes=OUTSIDE-note notes_id=%d: inside-res; notes=inside-note notes_id=%d: OUTSIDE-res; total=3", outsideNote.ID, insideNote.ID)},
		{`type = note GROUP BY resources COUNT()`,
			"count=1 resources=<nil>",
			"count=1 resources=OUTSIDE-res; count=1 resources=inside-res"},
		// Filters as oracles: no inside row is selected by outside data.
		{`type = group AND parent.meta.secret = "OUTSIDE-SECRET"`, "", "inside-root"},
		{`type = group AND parent.name = "OUTSIDE-P"`, "", "inside-root"},
		{`type = group AND parent.tags = "outside-tag"`, "", "inside-root"},
		{`type = group AND ancestors.name = "OUTSIDE-P"`, "", "inside-child,inside-root"},
		{`type = resource AND owner.parent.name = "OUTSIDE-P"`, "", allRes},
		{`type = resource AND groups = "OUTSIDE-O"`, "", "inside-res"},
		{`type = note AND groups = "OUTSIDE-O"`, "", "inside-note"},
		{`type = resource AND notes = "OUTSIDE-note"`, "", "inside-res"},
		{`type = note AND resources = "OUTSIDE-res"`, "", "inside-note"},
		{`type = resource AND groups IS NOT EMPTY`, "", "inside-res"},
		{`type = note AND resources IS NOT EMPTY`, "", "OUTSIDE-note,inside-note"},
		{`type = resource AND groups.count > 0`, "", "inside-res"},
		{`type = note AND groups.count > 0`, "", "inside-note"},
		{`type = resource AND similarImages IS NOT EMPTY`, "", "OUTSIDE-res,inside-res"},
		{fmt.Sprintf(`type = resource AND SIMILAR TO resource(%d)`, outsideRes.ID), "", "inside-res,inside-res3,inside-res4"},
		// A hidden target matches nothing, so NOT SIMILAR TO keeps every row,
		// and distance to it orders like distance to no target: by name.
		{fmt.Sprintf(`type = resource AND NOT SIMILAR TO resource(%d) WITHIN 2 ORDER BY distance ASC, name ASC`, outsideRes.ID),
			allRes, "inside-res3,inside-res4,OUTSIDE-res,inside-res2"},
		{fmt.Sprintf(`type = resource AND NOT SIMILAR TO resource(%d) WITHIN 2 ORDER BY distance DESC, name ASC`, outsideRes.ID),
			allRes, "OUTSIDE-res,inside-res2,inside-res4,inside-res3"},
		// A random order, compared as a set.
		{`set:type = resource ORDER BY RANDOM()`, allRes, "OUTSIDE-res," + allRes},
		{`set:type = resource AND groups = "OUTSIDE-O" ORDER BY RANDOM()`, "", "inside-res"},
		// Pagination totals.
		{`count:type = resource`, "total=4", "total=5"},
		{`count:type = resource AND groups IS NOT EMPTY`, "total=0", "total=1"},
		{`count:type = note AND groups = "OUTSIDE-O"`, "total=0", "total=1"},
		// A foreign key to a hidden group counts as missing in every form:
		// IS NULL, IS EMPTY, = and != agree with parent.name IS NULL.
		{`type = group AND parent IS NULL`, "inside-root", "OUTSIDE-O,OUTSIDE-O2,OUTSIDE-P"},
		{`type = group AND parent IS EMPTY`, "inside-root", "OUTSIDE-O,OUTSIDE-O2,OUTSIDE-P"},
		{`type = group AND parent.name IS NULL`, "inside-root", "OUTSIDE-O,OUTSIDE-O2,OUTSIDE-P"},
		{`type = group AND parent IS NOT NULL`, "inside-child", "inside-child,inside-root"},
		{`type = group AND parent IS NOT EMPTY`, "inside-child", "inside-child,inside-root"},
		{fmt.Sprintf(`type = group AND parent = %d`, parent.ID), "", "inside-root"},
		{fmt.Sprintf(`type = group AND parent != %d`, parent.ID), "inside-child", "inside-child"},
		{fmt.Sprintf(`type = group AND parent = %d`, root.ID), "inside-child", "inside-child"},
		{`type = resource AND owner IS NULL`, "", ""},
		{fmt.Sprintf(`type = resource AND owner = %d`, root.ID), allRes, allRes},
		{fmt.Sprintf(`type = note AND owner = %d`, other.ID), "", "OUTSIDE-note"},
		{`type = note AND owner IS NOT EMPTY`, "inside-note", "OUTSIDE-note,inside-note"},
		// Cross-entity: the merged order.
		{`name ~ "inside-r*" ORDER BY name ASC`, "order:" + allRes + ",inside-root", "order:" + allRes + ",inside-root"},
		{`name ~ "OUTSIDE*" ORDER BY name ASC`, "", "order:OUTSIDE-note,OUTSIDE-O,OUTSIDE-O2,OUTSIDE-P,OUTSIDE-res"},
	}
	for _, c := range cases {
		t.Run(c.query, func(t *testing.T) {
			if got := run(scoped, c.query); got != c.want {
				t.Errorf("confined result:\n got %q\nwant %q", got, c.want)
			}
			if got := run(tc.AppCtx, c.query); got != c.unscoped {
				t.Errorf("unconfined baseline:\n got %q\nwant %q", got, c.unscoped)
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
