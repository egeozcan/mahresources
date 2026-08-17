package application_context

import (
	"testing"

	"mahresources/auth"
	"mahresources/models"
)

// These seven writers build a bare stub — models.Note{ID: n} — and hand it to
// GORM's Association API, which emits ONLY join-table SQL. No statement ever
// touches `notes` or `resources`, so scopeReadCallback never fires, and none of
// note_tags / groups_related_notes / resource_notes / groups_related_resources
// is in scopeColumn either. Nothing narrows and nothing errors: the call
// succeeds silently against an entity the principal cannot see.
//
// They are reachable from a hook through mah.db.add_tags / remove_tags /
// add_groups / remove_groups / add_resources_to_note / remove_resources_from_note.
//
// The idiom that fixes it is already used by CreateOrUpdateNote, EditResource
// and deleteNoteInTransaction: load the row through the scoped handle first and
// return the error.
func TestAssociationWriters_ConfinedPrincipalCannotTouchEntitiesItCannotSee(t *testing.T) {
	ctx := createTestContextWithPlugins(t, t.TempDir())
	principal, _, _, _ := relationScopeFixture(t, ctx)

	outsideNote, outsideRes, insideGroup, tag := associationFixture(t, ctx, "deny")

	scoped := ctx.WithPrincipal(principal)

	for _, tc := range []struct {
		name string
		run  func() error
	}{
		{"AddTagsToNote", func() error { return scoped.AddTagsToNote(outsideNote.ID, []uint{tag.ID}) }},
		{"RemoveTagsFromNote", func() error { return scoped.RemoveTagsFromNote(outsideNote.ID, []uint{tag.ID}) }},
		{"AddGroupsToNote", func() error { return scoped.AddGroupsToNote(outsideNote.ID, []uint{insideGroup.ID}) }},
		{"RemoveGroupsFromNote", func() error { return scoped.RemoveGroupsFromNote(outsideNote.ID, []uint{insideGroup.ID}) }},
		{"AddResourcesToNote", func() error { return scoped.AddResourcesToNote(outsideNote.ID, []uint{outsideRes.ID}) }},
		{"RemoveResourcesFromNote", func() error { return scoped.RemoveResourcesFromNote(outsideNote.ID, []uint{outsideRes.ID}) }},
		{"RemoveGroupsFromResource", func() error { return scoped.RemoveGroupsFromResource(outsideRes.ID, []uint{insideGroup.ID}) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.run(); err == nil {
				t.Errorf("%s succeeded against an entity outside the principal's subtree", tc.name)
			}
		})
	}
}

// The same writers must keep working for an unscoped principal, and for a
// confined one acting inside its own subtree.
func TestAssociationWriters_LegitimateUseIsUnaffected(t *testing.T) {
	ctx := createTestContextWithPlugins(t, t.TempDir())
	principal, _, _, _ := relationScopeFixture(t, ctx)

	outsideNote, _, insideGroup, tag := associationFixture(t, ctx, "allow")

	// A note owned by the scope group itself: inside the subtree.
	insideNote := &models.Note{Name: "assoc-inside-note", OwnerId: &insideGroup.ID}
	if err := ctx.db.Create(insideNote).Error; err != nil {
		t.Fatalf("create inside note: %v", err)
	}

	scoped := ctx.WithPrincipal(principal)
	if err := scoped.AddTagsToNote(insideNote.ID, []uint{tag.ID}); err != nil {
		t.Errorf("a confined principal could not tag a note inside its own subtree: %v", err)
	}
	if err := scoped.RemoveTagsFromNote(insideNote.ID, []uint{tag.ID}); err != nil {
		t.Errorf("a confined principal could not untag a note inside its own subtree: %v", err)
	}

	editor, err := ctx.CreateUser(&UserInput{Username: "assoc-ed", Password: "password1", Role: models.RoleEditor})
	if err != nil {
		t.Fatalf("create editor: %v", err)
	}
	unscoped := ctx.WithPrincipal(auth.FromUser(editor))
	if err := unscoped.AddTagsToNote(outsideNote.ID, []uint{tag.ID}); err != nil {
		t.Errorf("an editor could not tag a note: %v", err)
	}
}

// associationFixture builds a note and a resource OUTSIDE the principal's
// subtree (no owner at all), plus the in-scope group and a tag. The suffix keeps
// names unique across subtests sharing a database.
func associationFixture(t *testing.T, ctx *MahresourcesContext, suffix string) (*models.Note, *models.Resource, *models.Group, *models.Tag) {
	t.Helper()

	var insideGroup models.Group
	if err := ctx.db.Where("name = ?", "inside").First(&insideGroup).Error; err != nil {
		t.Fatalf("load scope group: %v", err)
	}

	note := &models.Note{Name: "assoc-outside-note-" + suffix}
	if err := ctx.db.Create(note).Error; err != nil {
		t.Fatalf("create outside note: %v", err)
	}
	res := &models.Resource{
		Name:     "assoc-outside-res-" + suffix,
		Hash:     "assoc-hash-" + suffix,
		Location: "assoc-loc-" + suffix,
	}
	if err := ctx.db.Create(res).Error; err != nil {
		t.Fatalf("create outside resource: %v", err)
	}
	tag := &models.Tag{Name: "assoc-tag-" + suffix}
	if err := ctx.db.Create(tag).Error; err != nil {
		t.Fatalf("create tag: %v", err)
	}
	return note, res, &insideGroup, tag
}
