package api_tests

import (
	"context"
	"encoding/json"
	"fmt"
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

// runMRQLJoinsStayInsideTheSubtree: for a principal confined to a subtree, a
// row outside it behaves as if it does not exist in every part of an MRQL
// query, not only the rows it returns: a joined parent, owner, group, note or
// resource, a relation filter, a count, an existence test, a hierarchy walk
// and a similarity target. Each query runs on the scoped context, which must
// show nothing of the outside, and on the unscoped one, which must (the
// mutation step), so the fixture really holds the outside rows.
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
	root := &models.Group{Name: "inside-root", OwnerId: &parent.ID, Meta: types.JSON(`{"secret":"inside"}`)}
	create(root)
	child := &models.Group{Name: "inside-child", OwnerId: &root.ID}
	create(child)
	outsideNote := &models.Note{Name: "OUTSIDE-note", OwnerId: &other.ID}
	create(outsideNote)
	outsideRes := &models.Resource{Name: "OUTSIDE-res", OwnerId: &other.ID, Hash: "outside-hash", Location: "outside"}
	create(outsideRes)
	insideRes := &models.Resource{Name: "inside-res", OwnerId: &root.ID, Hash: "inside-hash", Location: "inside", Groups: []*models.Group{other}, Notes: []*models.Note{outsideNote}}
	create(insideRes)
	insideNote := &models.Note{Name: "inside-note", OwnerId: &root.ID, Groups: []*models.Group{other}, Resources: []*models.Resource{outsideRes}}
	create(insideNote)
	create(&models.ResourceSimilarity{ResourceID1: insideRes.ID, ResourceID2: outsideRes.ID, HammingDistance: 0})

	principal := &auth.Principal{UserID: 1, Username: "guest", Role: models.RoleGuest, ScopeGroupID: &root.ID}
	scoped := tc.AppCtx.WithPrincipal(principal)
	// The result structures, as JSON: what the /mrql API and the shortcode
	// executor build their output from.
	render := func(appCtx *application_context.MahresourcesContext, query string) string {
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
			return "error: " + err.Error()
		}
		out, err := json.Marshal(result)
		if err != nil {
			t.Fatal(err)
		}
		return string(out)
	}

	cases := []struct {
		query string
		// leak is what the scoped output must not contain: an outside name or
		// secret, or an inside row that only outside data can select.
		leak string
	}{
		// Joined rows named by GROUP BY.
		{`type = group GROUP BY parent COUNT()`, "OUTSIDE"},
		{`type = group GROUP BY parent`, "OUTSIDE"},
		{`type = group GROUP BY parent.name COUNT()`, "OUTSIDE"},
		{`type = group GROUP BY parent.meta.secret COUNT()`, "OUTSIDE"},
		{`type = resource GROUP BY owner.parent.name COUNT()`, "OUTSIDE"},
		{`type = resource GROUP BY groups COUNT()`, "OUTSIDE"},
		{`type = note GROUP BY groups COUNT()`, "OUTSIDE"},
		{`type = resource GROUP BY notes COUNT()`, "OUTSIDE"},
		{`type = note GROUP BY resources COUNT()`, "OUTSIDE"},
		// Filters as oracles: an inside row selected by outside data.
		{`type = group AND parent.meta.secret = "OUTSIDE-SECRET"`, "inside-root"},
		{`type = group AND parent.name = "OUTSIDE-P"`, "inside-root"},
		{`type = group AND parent.tags = "outside-tag"`, "inside-root"},
		{`type = group AND ancestors.name = "OUTSIDE-P"`, "inside-root"},
		{`type = resource AND owner.parent.name = "OUTSIDE-P"`, "inside-res"},
		{`type = resource AND groups = "OUTSIDE-O"`, "inside-res"},
		{`type = note AND groups = "OUTSIDE-O"`, "inside-note"},
		{`type = resource AND notes = "OUTSIDE-note"`, "inside-res"},
		{`type = note AND resources = "OUTSIDE-res"`, "inside-note"},
		{`type = resource AND groups IS NOT EMPTY`, "inside-res"},
		{`type = note AND resources IS NOT EMPTY`, "inside-note"},
		{`type = resource AND groups.count > 0`, "inside-res"},
		{`type = note AND groups.count > 0`, "inside-note"},
		{fmt.Sprintf(`type = resource AND SIMILAR TO resource(%d)`, outsideRes.ID), "inside-res"},
	}
	for _, c := range cases {
		t.Run(c.query, func(t *testing.T) {
			if got := render(scoped, c.query); strings.Contains(got, c.leak) || strings.Contains(got, "OUTSIDE") {
				t.Errorf("a confined principal sees outside data (%q):\n%s", c.leak, got)
			}
			if got := render(tc.AppCtx, c.query); !strings.Contains(got, c.leak) {
				t.Errorf("mutation step: unscoped, the output should show %q:\n%s", c.leak, got)
			}
		})
	}
}
