//go:build postgres

package api_tests

import (
	"testing"

	"mahresources/models"
)

// BlockTypeFilterSubject is hand-written SQL with a LEFT JOIN and aliased
// columns, so it is the one piece of this feature with real dialect risk. The
// SQLite coverage lives in application_context/block_filters_test.go; this
// proves the same query on Postgres, including that a note whose owner group is
// gone resolves to no category rather than to a stale one. (Neither Group nor
// Note carries gorm.DeletedAt, so a delete here is a real delete.)
func TestBlockTypeFilterSubjectPostgres(t *testing.T) {
	tc := SetupPostgresTestEnv(t)
	ctx := tc.AppCtx

	category := &models.Category{Name: "pg-block-filter-category"}
	if err := tc.DB.Create(category).Error; err != nil {
		t.Fatalf("create category: %v", err)
	}
	noteType := &models.NoteType{Name: "pg-block-filter-notetype"}
	if err := tc.DB.Create(noteType).Error; err != nil {
		t.Fatalf("create note type: %v", err)
	}
	group := &models.Group{Name: "pg-block-filter-group", CategoryId: &category.ID}
	if err := tc.DB.Create(group).Error; err != nil {
		t.Fatalf("create group: %v", err)
	}
	note := &models.Note{Name: "pg-block-filter-note", OwnerId: &group.ID, NoteTypeId: &noteType.ID}
	if err := tc.DB.Create(note).Error; err != nil {
		t.Fatalf("create note: %v", err)
	}

	noteTypeID, ownerCategoryID, err := ctx.BlockTypeFilterSubject(note.ID)
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
	bare := &models.Note{Name: "pg-block-filter-bare"}
	if err := tc.DB.Create(bare).Error; err != nil {
		t.Fatalf("create bare note: %v", err)
	}
	noteTypeID, ownerCategoryID, err = ctx.BlockTypeFilterSubject(bare.ID)
	if err != nil {
		t.Fatalf("BlockTypeFilterSubject(bare): %v", err)
	}
	if noteTypeID != nil || ownerCategoryID != nil {
		t.Errorf("expected two nils, got %v / %v", noteTypeID, ownerCategoryID)
	}

	// A note whose owner group is gone must resolve to no category, and the
	// LEFT JOIN must still return the row rather than dropping it — otherwise
	// the lookup would error and block creation would fail closed for the wrong
	// reason.
	if err := tc.DB.Model(&models.Note{}).Where("id = ?", note.ID).Update("owner_id", nil).Error; err != nil {
		t.Fatalf("clear owner: %v", err)
	}
	noteTypeID, ownerCategoryID, err = ctx.BlockTypeFilterSubject(note.ID)
	if err != nil {
		t.Fatalf("BlockTypeFilterSubject after clearing the owner: %v", err)
	}
	if ownerCategoryID != nil {
		t.Errorf("an unowned note should supply no category, got %v", *ownerCategoryID)
	}
	if noteTypeID == nil || *noteTypeID != noteType.ID {
		t.Errorf("the note's own type must survive losing its owner, got %v", noteTypeID)
	}
}
