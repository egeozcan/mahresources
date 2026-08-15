//go:build json1 && fts5

package application_context

import (
	"testing"

	"mahresources/models/block_types"
	"mahresources/models/query_models"
	"mahresources/plugin_system"
)

// registerFilteredBlockType registers a plugin block type with the given
// filters for the duration of the test. The block registry is process-global,
// so it has to be removed again.
func registerFilteredBlockType(t *testing.T, typeName string, filters plugin_system.BlockTypeFilter) {
	t.Helper()
	bt, err := plugin_system.NewPluginBlockType(plugin_system.PluginBlockTypeConfig{
		PluginName: "test",
		TypeName:   typeName,
		Label:      "Filtered",
		DefContent: []byte(`{}`),
		DefState:   []byte(`{}`),
		Filters:    filters,
	})
	if err != nil {
		t.Fatalf("NewPluginBlockType: %v", err)
	}
	block_types.RegisterBlockType(bt)
	t.Cleanup(func() { block_types.UnregisterBlockType(typeName) })
}

// BlockTypeFilterSubject is the single resolver both the create path and the
// block-type listing use, so they cannot disagree about what a note is.
func TestBlockTypeFilterSubject(t *testing.T) {
	ctx := createTestContext(t)

	category, err := ctx.CreateCategory(&query_models.CategoryCreator{Name: "Recipes-subject"})
	if err != nil {
		t.Fatalf("CreateCategory: %v", err)
	}
	group, err := ctx.CreateGroup(&query_models.GroupCreator{Name: "Cookbook-subject", CategoryId: category.ID})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	noteType, err := ctx.CreateOrUpdateNoteType(&query_models.NoteTypeEditor{Name: "Recipe-subject"})
	if err != nil {
		t.Fatalf("CreateNoteType: %v", err)
	}

	owned, err := ctx.CreateOrUpdateNote(&query_models.NoteEditor{
		NoteCreator: query_models.NoteCreator{Name: "Soup", OwnerId: group.ID, NoteTypeId: noteType.ID},
	})
	if err != nil {
		t.Fatalf("CreateOrUpdateNote: %v", err)
	}

	noteTypeID, ownerCategoryID, err := ctx.BlockTypeFilterSubject(owned.ID)
	if err != nil {
		t.Fatalf("BlockTypeFilterSubject: %v", err)
	}
	if noteTypeID == nil || *noteTypeID != noteType.ID {
		t.Errorf("note type = %v, want %d", noteTypeID, noteType.ID)
	}
	if ownerCategoryID == nil || *ownerCategoryID != category.ID {
		t.Errorf("owner category = %v, want %d", ownerCategoryID, category.ID)
	}

	// An unowned, untyped note resolves to two nils rather than an error.
	bare, err := ctx.CreateOrUpdateNote(&query_models.NoteEditor{
		NoteCreator: query_models.NoteCreator{Name: "Loose note"},
	})
	if err != nil {
		t.Fatalf("CreateOrUpdateNote: %v", err)
	}
	noteTypeID, ownerCategoryID, err = ctx.BlockTypeFilterSubject(bare.ID)
	if err != nil {
		t.Fatalf("BlockTypeFilterSubject: %v", err)
	}
	if noteTypeID != nil || ownerCategoryID != nil {
		t.Errorf("expected two nils for an unowned untyped note, got %v / %v", noteTypeID, ownerCategoryID)
	}
}

// filters.category_ids was parsed, stored and shipped to the browser while
// nothing read it: only note-type ids were enforced, so a block type scoped to
// one category could be created on any note in the app.
func TestCreateBlock_EnforcesCategoryFilter(t *testing.T) {
	ctx := createTestContext(t)

	// Names are unique per test: createTestContext hands out contexts over one
	// database, so a name reused across tests collides.
	recipes, err := ctx.CreateCategory(&query_models.CategoryCreator{Name: "Recipes-enforce"})
	if err != nil {
		t.Fatalf("CreateCategory: %v", err)
	}
	other, err := ctx.CreateCategory(&query_models.CategoryCreator{Name: "Invoices-enforce"})
	if err != nil {
		t.Fatalf("CreateCategory: %v", err)
	}

	inCategory, err := ctx.CreateGroup(&query_models.GroupCreator{Name: "Cookbook-enforce", CategoryId: recipes.ID})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	outOfCategory, err := ctx.CreateGroup(&query_models.GroupCreator{Name: "Ledger-enforce", CategoryId: other.ID})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}

	registerFilteredBlockType(t, "plugin:test:ingredients", plugin_system.BlockTypeFilter{
		CategoryIDs: []uint{recipes.ID},
	})

	allowed, err := ctx.CreateOrUpdateNote(&query_models.NoteEditor{
		NoteCreator: query_models.NoteCreator{Name: "Soup", OwnerId: inCategory.ID},
	})
	if err != nil {
		t.Fatalf("CreateOrUpdateNote: %v", err)
	}
	refused, err := ctx.CreateOrUpdateNote(&query_models.NoteEditor{
		NoteCreator: query_models.NoteCreator{Name: "March", OwnerId: outOfCategory.ID},
	})
	if err != nil {
		t.Fatalf("CreateOrUpdateNote: %v", err)
	}

	if _, err := ctx.CreateBlock(&query_models.NoteBlockEditor{
		NoteID: allowed.ID,
		Type:   "plugin:test:ingredients",
	}); err != nil {
		t.Errorf("a note in the allowed category should accept the block: %v", err)
	}

	if _, err := ctx.CreateBlock(&query_models.NoteBlockEditor{
		NoteID: refused.ID,
		Type:   "plugin:test:ingredients",
	}); err == nil {
		t.Error("a note outside the allowed category must be refused")
	}
}

// A note with no owner cannot satisfy a category filter — fail closed rather
// than treating "no category" as "any category".
func TestCreateBlock_UnownedNoteFailsCategoryFilter(t *testing.T) {
	ctx := createTestContext(t)

	recipes, err := ctx.CreateCategory(&query_models.CategoryCreator{Name: "Recipes-unowned"})
	if err != nil {
		t.Fatalf("CreateCategory: %v", err)
	}
	registerFilteredBlockType(t, "plugin:test:ingredients-2", plugin_system.BlockTypeFilter{
		CategoryIDs: []uint{recipes.ID},
	})

	note, err := ctx.CreateOrUpdateNote(&query_models.NoteEditor{
		NoteCreator: query_models.NoteCreator{Name: "Loose"},
	})
	if err != nil {
		t.Fatalf("CreateOrUpdateNote: %v", err)
	}

	if _, err := ctx.CreateBlock(&query_models.NoteBlockEditor{
		NoteID: note.ID,
		Type:   "plugin:test:ingredients-2",
	}); err == nil {
		t.Error("an unowned note must not satisfy a category filter")
	}
}

// The note-type filter that already worked must keep working, now that both
// run through one predicate.
func TestCreateBlock_StillEnforcesNoteTypeFilter(t *testing.T) {
	ctx := createTestContext(t)

	allowedType, err := ctx.CreateOrUpdateNoteType(&query_models.NoteTypeEditor{Name: "Recipe-notetype"})
	if err != nil {
		t.Fatalf("CreateNoteType: %v", err)
	}
	otherType, err := ctx.CreateOrUpdateNoteType(&query_models.NoteTypeEditor{Name: "Invoice-notetype"})
	if err != nil {
		t.Fatalf("CreateNoteType: %v", err)
	}

	registerFilteredBlockType(t, "plugin:test:steps", plugin_system.BlockTypeFilter{
		NoteTypeIDs: []uint{allowedType.ID},
	})

	ok, err := ctx.CreateOrUpdateNote(&query_models.NoteEditor{
		NoteCreator: query_models.NoteCreator{Name: "Stew", NoteTypeId: allowedType.ID},
	})
	if err != nil {
		t.Fatalf("CreateOrUpdateNote: %v", err)
	}
	nope, err := ctx.CreateOrUpdateNote(&query_models.NoteEditor{
		NoteCreator: query_models.NoteCreator{Name: "Bill", NoteTypeId: otherType.ID},
	})
	if err != nil {
		t.Fatalf("CreateOrUpdateNote: %v", err)
	}

	if _, err := ctx.CreateBlock(&query_models.NoteBlockEditor{NoteID: ok.ID, Type: "plugin:test:steps"}); err != nil {
		t.Errorf("the allowed note type should accept the block: %v", err)
	}
	if _, err := ctx.CreateBlock(&query_models.NoteBlockEditor{NoteID: nope.ID, Type: "plugin:test:steps"}); err == nil {
		t.Error("a different note type must be refused")
	}
}
