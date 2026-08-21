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

// relationScopeFixture builds three edges — one wholly OUTSIDE the principal's
// subtree, one wholly INSIDE it, and one MIXED — and returns a principal
// confined to "inside".
//
// The mixed edge is the one that separates "both endpoints visible" from
// "either endpoint visible", and it is the case that matters: an edge with one
// foot in the subtree still names a group outside it, so renaming or deleting it
// is a write about a group the principal may not see. A guard written with ||
// instead of && passes every test that only uses wholly-outside edges, which is
// exactly how mutation testing caught an earlier draft of this file.
//
// The principal is a scoped EDITOR, and it is synthetic on purpose: no stored
// account can be both, because normalizeScopeGroup nils an editor's scope group
// on create and on update. It is built by hand here so these tests keep
// measuring the scope rule alone. Edge writes also require the editor role
// (requireEditorRole, role_capability.go), so a scope-limited *user* is now
// refused before scope is ever consulted — and a fixture like that would make
// every assertion below pass for the wrong reason, including the one that
// exists to prove the guard does not over-refuse.
func relationScopeFixture(t *testing.T, ctx *MahresourcesContext) (principal *auth.Principal, outside, inside, mixed *models.GroupRelation) {
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
	mixed = newRelation("mixed-edge", insideA, outsideB)

	scoped := &auth.Principal{
		UserID:       1,
		Username:     "confined-rel",
		Role:         models.RoleEditor,
		ScopeGroupID: &scopeGroup.ID,
	}
	return scoped, outside, inside, mixed
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
	principal, outside, _, _ := relationScopeFixture(t, ctx)

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
	principal, outside, _, _ := relationScopeFixture(t, ctx)

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
	_, outside, inside, _ := relationScopeFixture(t, ctx)

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
	principal, _, inside, _ := relationScopeFixture(t, ctx)

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
	_, outside, _, _ := relationScopeFixture(t, ctx)

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

// An edge with exactly one endpoint inside the subtree. This is the case that
// pins the guard to "both endpoints", not "either": a mutation relaxing && to
// || is invisible to every test above, because their out-of-scope edge has both
// feet outside and stays denied either way.
func TestRelationWrites_ConfinedPrincipalCannotReachAnEdgeThatLeavesItsSubtree(t *testing.T) {
	ctx := createTestContextWithPlugins(t, t.TempDir())
	principal, _, _, mixed := relationScopeFixture(t, ctx)

	scoped := ctx.WithPrincipal(principal)

	if _, err := scoped.EditRelation(query_models.GroupRelationshipQuery{
		Id: mixed.ID, Name: "renamed-across-the-boundary",
	}); err == nil {
		t.Error("a confined principal renamed an edge whose far endpoint is outside its subtree")
	}
	if name, _ := relationName(t, ctx, mixed.ID); name != "mixed-edge" {
		t.Errorf("relation name is %q; the write landed despite the error", name)
	}

	if err := scoped.DeleteRelationship(mixed.ID); err == nil {
		t.Error("a confined principal deleted an edge whose far endpoint is outside its subtree")
	}
	if _, alive := relationName(t, ctx, mixed.ID); !alive {
		t.Error("the edge was deleted by a principal that cannot see its far endpoint")
	}
}

// The edge guard is only worth what its narrowest door is worth. EditRelation
// and DeleteRelationship are not the only ways an edge dies: relation types and
// categories cascade to `DELETE FROM group_relations`, database-wide, with no
// scope predicate at all — and mah.db exposes delete_relation_type,
// update_relation_type and delete_category to a hook.
//
// So a confined principal that cannot touch an edge directly could still delete
// every edge of its type, including edges between groups it cannot see. Any
// claim that edges are confined has to survive this test, not just the direct
// writes.
func TestRelationTypeAndCategoryWrites_ConfinedPrincipalCannotCascadeAcrossSubtrees(t *testing.T) {
	for _, tc := range []struct {
		name string
		run  func(ctx *MahresourcesContext, relTypeID, categoryID uint) error
	}{
		{
			name: "delete_relation_type",
			run: func(ctx *MahresourcesContext, relTypeID, _ uint) error {
				return ctx.DeleteRelationshipType(relTypeID)
			},
		},
		{
			name: "update_relation_type re-pointing its categories",
			run: func(ctx *MahresourcesContext, relTypeID, categoryID uint) error {
				_, err := ctx.EditRelationType(&query_models.RelationshipTypeEditorQuery{
					Id: relTypeID, Name: "repointed", FromCategory: categoryID, ToCategory: categoryID,
				})
				return err
			},
		},
		{
			name: "delete_category",
			run: func(ctx *MahresourcesContext, _, categoryID uint) error {
				return ctx.DeleteCategory(categoryID)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := createTestContextWithPlugins(t, t.TempDir())
			principal, outside, _, _ := relationScopeFixture(t, ctx)

			var rel models.GroupRelation
			if err := ctx.db.First(&rel, outside.ID).Error; err != nil {
				t.Fatalf("load the out-of-scope edge: %v", err)
			}
			var relType models.GroupRelationType
			if err := ctx.db.First(&relType, *rel.RelationTypeId).Error; err != nil {
				t.Fatalf("load the relation type: %v", err)
			}
			// A second category, so update_relation_type has somewhere to point
			// that no group matches, which is what triggers the cascade delete.
			other := &models.Category{Name: "other-category"}
			if err := ctx.db.Create(other).Error; err != nil {
				t.Fatalf("create second category: %v", err)
			}

			scoped := ctx.WithPrincipal(principal)
			if err := tc.run(scoped, relType.ID, other.ID); err == nil {
				t.Errorf("a confined principal performed a global taxonomy write that cascades to relation edges")
			}

			if _, alive := relationName(t, ctx, outside.ID); !alive {
				t.Errorf("the edge between two groups the principal cannot see was deleted by the cascade")
			}
		})
	}
}

// The same operations must keep working for a principal that is not confined.
func TestRelationTypeWrites_UnscopedPrincipalIsUnaffected(t *testing.T) {
	ctx := createTestContextWithPlugins(t, t.TempDir())
	_, outside, _, _ := relationScopeFixture(t, ctx)

	var rel models.GroupRelation
	if err := ctx.db.First(&rel, outside.ID).Error; err != nil {
		t.Fatalf("load edge: %v", err)
	}

	editor, err := ctx.CreateUser(&UserInput{Username: "rt-ed", Password: "password1", Role: models.RoleEditor})
	if err != nil {
		t.Fatalf("create editor: %v", err)
	}
	if err := ctx.WithPrincipal(auth.FromUser(editor)).DeleteRelationshipType(*rel.RelationTypeId); err != nil {
		t.Fatalf("an editor could not delete a relation type: %v", err)
	}
}

// globalCascadeDeleteCallback keys on the db HANDLE carrying a scope filter,
// not on the principal, which is this tree's doctrine: scope rides inside the
// *gorm.DB. So the representative test is a delete issued through a
// request-scoped handle, which is what any writer built per request would use.
//
// It is deliberately NOT written against CategoryCRUD(). That writer captures
// the singleton handle at startup (server/routes.go builds it once), and the
// singleton carries no scope filter, so the callback could not fire for it
// however the routes were wired. A test that built one from a scoped context
// would be testing a construction production never performs.
func TestGlobalCascade_ScopedHandleDeleteIsRefused(t *testing.T) {
	ctx := createTestContextWithPlugins(t, t.TempDir())
	principal, outside, _, _ := relationScopeFixture(t, ctx)

	var rel models.GroupRelation
	if err := ctx.db.First(&rel, outside.ID).Error; err != nil {
		t.Fatalf("load edge: %v", err)
	}

	scoped := ctx.WithPrincipal(principal)
	if err := scoped.db.Delete(&models.GroupRelationType{}, *rel.RelationTypeId).Error; err == nil {
		t.Error("a raw ORM delete removed a relation type through a scoped handle")
	}
	if _, alive := relationName(t, ctx, outside.ID); !alive {
		t.Error("the relation-type delete cascaded to an edge outside the subtree")
	}

	if err := scoped.db.Delete(&models.Category{}, *ctx.mustCategoryID(t, rel)).Error; err == nil {
		t.Error("a raw ORM delete removed a category through a scoped handle")
	}
}

// The unscoped side of the same callback: an admin's handle carries no scope
// filter, so nothing is refused. Asserted through DeleteRelationshipType rather
// than the generic CRUDWriter — see docs/todo.md, that writer would
// cascade-delete every group in the category, which is a defect to fix rather
// than a behaviour to pin.
func TestGlobalCascade_UnscopedHandleIsUnaffected(t *testing.T) {
	ctx := createTestContextWithPlugins(t, t.TempDir())
	_, outside, _, _ := relationScopeFixture(t, ctx)

	var rel models.GroupRelation
	if err := ctx.db.First(&rel, outside.ID).Error; err != nil {
		t.Fatalf("load edge: %v", err)
	}
	admin, err := ctx.CreateUser(&UserInput{Username: "cascade-admin", Password: "password1", Role: models.RoleAdmin})
	if err != nil {
		t.Fatalf("create admin: %v", err)
	}
	if err := ctx.WithPrincipal(auth.FromUser(admin)).DeleteRelationshipType(*rel.RelationTypeId); err != nil {
		t.Fatalf("an admin could not delete a relation type: %v", err)
	}
}

// An edge with a NULL endpoint belongs to no subtree, so relationInScope denies
// it. Untested until a mutation run pointed out that flipping that branch to
// "return true" changed nothing.
func TestRelationInScope_DanglingEndpointIsDenied(t *testing.T) {
	ctx := createTestContextWithPlugins(t, t.TempDir())
	principal, outside, _, _ := relationScopeFixture(t, ctx)

	var rel models.GroupRelation
	if err := ctx.db.First(&rel, outside.ID).Error; err != nil {
		t.Fatalf("load edge: %v", err)
	}
	rel.ToGroupId = nil
	scoped := ctx.WithPrincipal(principal)

	if scoped.relationInScope(&rel) {
		t.Error("an edge with a NULL endpoint was treated as in scope")
	}
	if scoped.relationInScope(nil) {
		t.Error("a nil relation was treated as in scope")
	}
}

// The cascade guard must cover a deny-all principal, not only a confined one.
// A mutation narrowing isScopedPrincipal() to IsScoped() drops exactly this
// case, and nothing else in the suite ran a deny-all principal against the
// three cascade operations.
func TestGlobalCascade_DeniedPluginPrincipalIsRefused(t *testing.T) {
	ctx := createTestContextWithPlugins(t, t.TempDir())
	_, outside, _, _ := relationScopeFixture(t, ctx)

	var rel models.GroupRelation
	if err := ctx.db.First(&rel, outside.ID).Error; err != nil {
		t.Fatalf("load edge: %v", err)
	}
	denied := ctx.WithPrincipal(deniedPluginPrincipal(4242))

	if _, err := denied.EditRelationType(&query_models.RelationshipTypeEditorQuery{
		Id: *rel.RelationTypeId, Name: "repointed",
	}); err == nil {
		t.Error("a denied principal edited a relation type")
	}
	if err := denied.DeleteRelationshipType(*rel.RelationTypeId); err == nil {
		t.Error("a denied principal deleted a relation type")
	}
	if _, alive := relationName(t, ctx, outside.ID); !alive {
		t.Error("a denied principal's cascade deleted an out-of-scope edge")
	}
}

// mustCategoryID resolves the category behind an edge's relation type, so the
// callback test can name one without threading it through the fixture.
func (ctx *MahresourcesContext) mustCategoryID(t *testing.T, rel models.GroupRelation) *uint {
	t.Helper()
	var rt models.GroupRelationType
	if err := ctx.db.First(&rt, *rel.RelationTypeId).Error; err != nil {
		t.Fatalf("load relation type: %v", err)
	}
	return rt.FromCategoryId
}

// UpdateGroup's scoped UPDATE matches no rows for a group outside the subtree,
// but RowsAffected is never consulted and the relation cleanup that follows is
// keyed on the caller-supplied id rather than on what was actually updated. So
// a scoped hook calling mah.db.update_group with an arbitrary id deletes the
// constrained edges incident to a group it cannot see.
//
// This is not the known-open "a group's own category change" case: here the
// principal controls NEITHER endpoint, and no category of its own changes.
func TestUpdateGroup_ConfinedPrincipalCannotCascadeIntoAGroupItCannotSee(t *testing.T) {
	ctx := createTestContextWithPlugins(t, t.TempDir())
	principal, outside, _, _ := relationScopeFixture(t, ctx)

	var rel models.GroupRelation
	if err := ctx.db.First(&rel, outside.ID).Error; err != nil {
		t.Fatalf("load edge: %v", err)
	}

	scoped := ctx.WithPrincipal(principal)
	// Clearing the category is what triggers the cascade delete of every
	// constrained edge incident to *from_group_id*.
	_, err := scoped.UpdateGroup(&query_models.GroupEditor{
		GroupCreator: query_models.GroupCreator{Name: "renamed-from-outside"},
		ID:           *rel.FromGroupId,
	})
	if err == nil {
		t.Error("a confined principal updated a group outside its subtree")
	}

	if _, alive := relationName(t, ctx, outside.ID); !alive {
		t.Error("the update cascaded into an edge between two groups the principal cannot see")
	}
}

// Item 4.1. Re-categorising a group inside your own subtree used to delete every
// edge the new category invalidates, including one whose far endpoint is outside
// that subtree — a group the caller cannot see, on an edge only an admin or
// editor could have created, since a scope-limited user cannot write edges at
// all. The near edge still goes; the far one is spared.
//
// The residue is a row whose category no longer matches its relation type. That
// is a create-time check (AddRelation), never enforced at read time, and
// MergeGroups already produces such rows routinely by copying edges with no
// revalidation. An inconsistent row an editor can repair beats a deleted edge
// nobody can restore.
func TestUpdateGroup_CategoryChangeSparesAnEdgeReachingOutsideTheSubtree(t *testing.T) {
	ctx := createTestContextWithPlugins(t, t.TempDir())
	principal, _, inside, mixed := relationScopeFixture(t, ctx)

	// insideA is the from-endpoint of both edges, and is inside the subtree.
	insideA := *inside.FromGroupId

	scoped := ctx.WithPrincipal(principal)
	// Clearing the category invalidates every constrained edge incident to it.
	if _, err := scoped.UpdateGroup(&query_models.GroupEditor{
		GroupCreator: query_models.GroupCreator{Name: "inside-a", Meta: "{}"},
		ID:           insideA,
	}); err != nil {
		t.Fatalf("UpdateGroup by a principal that owns the group: %v", err)
	}

	if _, alive := relationName(t, ctx, inside.ID); alive {
		t.Error("the edge with both endpoints inside the subtree should have been cleaned up")
	}

	if _, alive := relationName(t, ctx, mixed.ID); !alive {
		t.Error("a confined caller's own category change destroyed an edge reaching a group it cannot see")
	}
}

// The control, and the half that must not regress: an unscoped caller still
// cleans up both. visibleGroupIDs reports every id visible when no scope filter
// is installed, so the sparing branch is inert for admins and editors.
func TestUpdateGroup_CategoryChangeStillCleansUpEverythingWhenUnscoped(t *testing.T) {
	ctx := createTestContextWithPlugins(t, t.TempDir())
	_, _, inside, mixed := relationScopeFixture(t, ctx)

	insideA := *inside.FromGroupId

	if _, err := ctx.UpdateGroup(&query_models.GroupEditor{
		GroupCreator: query_models.GroupCreator{Name: "inside-a", Meta: "{}"},
		ID:           insideA,
	}); err != nil {
		t.Fatalf("UpdateGroup unscoped: %v", err)
	}

	if _, alive := relationName(t, ctx, inside.ID); alive {
		t.Error("unscoped cleanup left the inside edge behind")
	}
	if _, alive := relationName(t, ctx, mixed.ID); alive {
		t.Error("unscoped cleanup left the cross-subtree edge behind: the sparing branch is not inert for an unscoped caller")
	}
}
