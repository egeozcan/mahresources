package application_context

import (
	"testing"

	"mahresources/auth"
	"mahresources/models"
	"mahresources/models/query_models"
)

// scopeColumn (scoping.go) maps groups, resources and notes. It does not map
// group_relations, so no ORM callback confines a relation edge to a subtree —
// and the plugin writer exposes UpdateGroupRelation and DeleteGroupRelation,
// which a group-confined user's ordinary CRUD can wake through a hook.
//
// A relation edge is not taxonomy. It is structural data about two groups, and
// a principal that cannot see either endpoint must not be able to rename the
// edge between them, still less delete it.

// relationScopeFixture builds a relation between two groups OUTSIDE the
// principal's subtree, plus one wholly INSIDE it, and returns a principal
// confined to "inside".
//
// The user's role is RoleUser rather than RoleGuest for the same reason the
// hook-scope fixture gives: a guest cannot write at all, so a guest never
// reaches a write path. A scope-limited user is the principal that is both
// confined and able to act.
func relationScopeFixture(t *testing.T, ctx *MahresourcesContext) (principal *auth.Principal, outside, inside *models.GroupRelation) {
	t.Helper()

	category := &models.Category{Name: "rel-category"}
	if err := ctx.db.Create(category).Error; err != nil {
		t.Fatalf("create category: %v", err)
	}

	relType := &models.GroupRelationType{
		Name:           "rel-type",
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

	scopeGroup := newGroup("inside", nil)
	// Both endpoints inside the subtree: descendants of the scope group.
	insideA := newGroup("inside-a", &scopeGroup.ID)
	insideB := newGroup("inside-b", &scopeGroup.ID)
	// Both endpoints outside it entirely.
	outsideA := newGroup("outside-a", nil)
	outsideB := newGroup("outside-b", nil)

	newRelation := func(name string, from, to *models.Group) *models.GroupRelation {
		r := &models.GroupRelation{
			Name:           name,
			FromGroupId:    &from.ID,
			ToGroupId:      &to.ID,
			RelationTypeId: &relType.ID,
		}
		if err := ctx.db.Create(r).Error; err != nil {
			t.Fatalf("create relation %s: %v", name, err)
		}
		return r
	}

	outside = newRelation("outside-edge", outsideA, outsideB)
	inside = newRelation("inside-edge", insideA, insideB)

	user, err := ctx.CreateUser(&UserInput{
		Username:     "confined-rel",
		Password:     "password1",
		Role:         models.RoleUser,
		ScopeGroupId: &scopeGroup.ID,
	})
	if err != nil {
		t.Fatalf("create confined user: %v", err)
	}
	return auth.FromUser(user), outside, inside
}

func relationName(t *testing.T, ctx *MahresourcesContext, id uint) (string, bool) {
	t.Helper()
	var rel models.GroupRelation
	// Read on the unscoped context, so absence means deleted rather than hidden.
	if err := ctx.db.First(&rel, id).Error; err != nil {
		return "", false
	}
	return rel.Name, true
}

func TestEditRelation_ConfinedPrincipalCannotRenameAnEdgeItCannotSee(t *testing.T) {
	ctx := createTestContextWithPlugins(t, t.TempDir())
	principal, outside, _ := relationScopeFixture(t, ctx)

	scoped := ctx.WithPrincipal(principal)
	if _, err := scoped.EditRelation(query_models.GroupRelationshipQuery{
		Id:   outside.ID,
		Name: "renamed-by-outsider",
	}); err == nil {
		t.Error("a principal confined to another subtree renamed a relation between two groups it cannot see")
	}

	name, alive := relationName(t, ctx, outside.ID)
	if !alive {
		t.Fatalf("relation %d vanished", outside.ID)
	}
	if name != "outside-edge" {
		t.Errorf("relation name is %q, want %q: the write landed despite the error", name, "outside-edge")
	}
}

func TestDeleteRelationship_ConfinedPrincipalCannotDeleteAnEdgeItCannotSee(t *testing.T) {
	ctx := createTestContextWithPlugins(t, t.TempDir())
	principal, outside, _ := relationScopeFixture(t, ctx)

	scoped := ctx.WithPrincipal(principal)
	if err := scoped.DeleteRelationship(outside.ID); err == nil {
		t.Error("a principal confined to another subtree deleted a relation between two groups it cannot see")
	}

	if _, alive := relationName(t, ctx, outside.ID); !alive {
		t.Errorf("relation %d was deleted by a principal that cannot see either endpoint", outside.ID)
	}
}

// An actor whose account could not be resolved is denied everything reachable
// through the subtree mechanism. Relation edges must be part of that.
func TestRelationWrites_DeniedPluginPrincipalReachesNothing(t *testing.T) {
	ctx := createTestContextWithPlugins(t, t.TempDir())
	_, outside, inside := relationScopeFixture(t, ctx)

	denied := ctx.WithPrincipal(deniedPluginPrincipal(4242))

	for _, rel := range []*models.GroupRelation{outside, inside} {
		if _, err := denied.EditRelation(query_models.GroupRelationshipQuery{
			Id: rel.ID, Name: "renamed-by-nobody",
		}); err == nil {
			t.Errorf("a denied principal renamed relation %d", rel.ID)
		}
		if err := denied.DeleteRelationship(rel.ID); err == nil {
			t.Errorf("a denied principal deleted relation %d", rel.ID)
		}
		if _, alive := relationName(t, ctx, rel.ID); !alive {
			t.Errorf("relation %d was deleted by a denied principal", rel.ID)
		}
	}
}

// The other half of the rule: confining a principal must not blind it to the
// edges wholly inside its own subtree. A guard that refused everything would
// pass the tests above and be useless.
func TestRelationWrites_ConfinedPrincipalStillReachesItsOwnSubtree(t *testing.T) {
	ctx := createTestContextWithPlugins(t, t.TempDir())
	principal, _, inside := relationScopeFixture(t, ctx)

	scoped := ctx.WithPrincipal(principal)
	if _, err := scoped.EditRelation(query_models.GroupRelationshipQuery{
		Id: inside.ID, Name: "renamed-from-within",
	}); err != nil {
		t.Fatalf("a confined principal could not rename an edge inside its own subtree: %v", err)
	}
	if name, _ := relationName(t, ctx, inside.ID); name != "renamed-from-within" {
		t.Errorf("relation name is %q, want the rename to have landed", name)
	}

	if err := scoped.DeleteRelationship(inside.ID); err != nil {
		t.Fatalf("a confined principal could not delete an edge inside its own subtree: %v", err)
	}
	if _, alive := relationName(t, ctx, inside.ID); alive {
		t.Error("the delete reported success but the relation is still there")
	}
}

// Regression guard on the fix: an unscoped principal keeps the reach it has
// always had.
func TestRelationWrites_UnscopedPrincipalIsUnaffected(t *testing.T) {
	ctx := createTestContextWithPlugins(t, t.TempDir())
	_, outside, _ := relationScopeFixture(t, ctx)

	editor, err := ctx.CreateUser(&UserInput{Username: "rel-ed", Password: "password1", Role: models.RoleEditor})
	if err != nil {
		t.Fatalf("create editor: %v", err)
	}

	unscoped := ctx.WithPrincipal(auth.FromUser(editor))
	if _, err := unscoped.EditRelation(query_models.GroupRelationshipQuery{
		Id: outside.ID, Name: "renamed-by-editor",
	}); err != nil {
		t.Fatalf("an editor could not rename a relation: %v", err)
	}
	if err := unscoped.DeleteRelationship(outside.ID); err != nil {
		t.Fatalf("an editor could not delete a relation: %v", err)
	}
}
