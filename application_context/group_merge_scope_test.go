package application_context

import (
	"testing"

	"mahresources/auth"
	"mahresources/models"
)

// The merge closure ends with a sweep that removes self-referential relation
// edges. Every other raw-SQL statement in that closure carries the subtree
// filter the GORM scope callbacks cannot supply — raw SQL bypasses them — and
// this one did not, so a group-limited principal merging two groups inside its
// own subtree issued a database-wide DELETE.
//
// AddRelation refuses to create a self-edge, so the only rows the sweep can
// reach are legacy or imported ones. That is why the fixture writes one
// directly through the db handle rather than through AddRelation: routed
// through the guard, the row this test is about cannot exist.
func mergeScopeFixture(t *testing.T, ctx *MahresourcesContext) (scopeGroup, winner, loser *models.Group, outsideEdge, insideEdge *models.GroupRelation) {
	t.Helper()

	category := &models.Category{Name: "merge-category"}
	if err := ctx.db.Create(category).Error; err != nil {
		t.Fatalf("create category: %v", err)
	}
	relType := &models.GroupRelationType{
		Name:           "merge-rel-type",
		FromCategoryId: &category.ID,
		ToCategoryId:   &category.ID,
	}
	if err := ctx.db.Create(relType).Error; err != nil {
		t.Fatalf("create relation type: %v", err)
	}

	newGroup := func(name string, owner *uint) *models.Group {
		g := &models.Group{Name: name, CategoryId: &category.ID, OwnerId: owner}
		if err := ctx.db.Create(g).Error; err != nil {
			t.Fatalf("create group %s: %v", name, err)
		}
		return g
	}

	scopeGroup = newGroup("inside", nil)
	winner = newGroup("inside-winner", &scopeGroup.ID)
	loser = newGroup("inside-loser", &scopeGroup.ID)
	outside := newGroup("outside", nil)

	newSelfEdge := func(name string, on *models.Group) *models.GroupRelation {
		r := &models.GroupRelation{
			Name:           name,
			FromGroupId:    &on.ID,
			ToGroupId:      &on.ID,
			RelationTypeId: &relType.ID,
		}
		if err := ctx.db.Create(r).Error; err != nil {
			t.Fatalf("create self edge %s: %v", name, err)
		}
		return r
	}

	// Both directions are needed. Asserting only that the outside edge survives
	// would also pass for a filter that matches nothing at all, which is the
	// mutation that "add a predicate" most easily produces.
	outsideEdge = newSelfEdge("legacy-self-edge-outside", outside)
	insideEdge = newSelfEdge("legacy-self-edge-inside", winner)
	return scopeGroup, winner, loser, outsideEdge, insideEdge
}

func relationExists(t *testing.T, ctx *MahresourcesContext, id uint) bool {
	t.Helper()
	var count int64
	// Counted on the unscoped handle, so absence means deleted rather than hidden.
	if err := ctx.db.Model(&models.GroupRelation{}).Where("id = ?", id).Count(&count).Error; err != nil {
		t.Fatalf("count relation %d: %v", id, err)
	}
	return count > 0
}

func TestMergeGroups_ScopedMergeLeavesAnOutOfSubtreeSelfEdgeAlone(t *testing.T) {
	ctx := createTestContextWithPlugins(t, t.TempDir())
	scopeGroup, winner, loser, outsideEdge, insideEdge := mergeScopeFixture(t, ctx)

	scoped := ctx.WithPrincipal(&auth.Principal{
		UserID:       1,
		Username:     "confined-merger",
		Role:         models.RoleUser,
		ScopeGroupID: &scopeGroup.ID,
	})
	if err := scoped.MergeGroups(winner.ID, []uint{loser.ID}); err != nil {
		t.Fatalf("merge inside the principal's own subtree failed: %v", err)
	}

	if !relationExists(t, ctx, outsideEdge.ID) {
		t.Error("merging two in-subtree groups deleted a self-referential relation outside the subtree")
	}
	if relationExists(t, ctx, insideEdge.ID) {
		t.Error("the sweep no longer removes an in-subtree self-edge: the filter matches nothing at all")
	}
}

// The other direction, so the filter cannot be written as a blanket refusal:
// an unscoped principal still sweeps, which is the behaviour that existed
// before the filter and the reason the statement is there at all.
func TestMergeGroups_UnscopedMergeStillSweepsSelfEdges(t *testing.T) {
	ctx := createTestContextWithPlugins(t, t.TempDir())
	_, winner, loser, outsideEdge, insideEdge := mergeScopeFixture(t, ctx)

	if err := ctx.MergeGroups(winner.ID, []uint{loser.ID}); err != nil {
		t.Fatalf("unscoped merge failed: %v", err)
	}

	if relationExists(t, ctx, outsideEdge.ID) || relationExists(t, ctx, insideEdge.ID) {
		t.Error("an unscoped merge left a self-referential relation behind; the sweep no longer runs")
	}
}
