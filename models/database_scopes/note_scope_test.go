package database_scopes

import (
	"testing"

	"mahresources/models"
	"mahresources/models/query_models"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// newNoteTestDB opens an isolated in-memory SQLite DB and migrates the
// minimal note_types and notes table schema needed for note scope tests.
func newNoteTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open in-memory DB: %v", err)
	}
	if err := db.AutoMigrate(&models.NoteType{}, &models.Note{}); err != nil {
		t.Fatalf("failed to migrate NoteType/Note tables: %v", err)
	}
	return db
}

// seedNoteType inserts a minimal NoteType row with the given name and returns it.
func seedNoteType(t *testing.T, db *gorm.DB, name string) models.NoteType {
	t.Helper()
	nt := models.NoteType{Name: name}
	if err := db.Create(&nt).Error; err != nil {
		t.Fatalf("failed to seed note type %q: %v", name, err)
	}
	return nt
}

// seedNote inserts a minimal Note row with the given name and optional NoteTypeId.
func seedNote(t *testing.T, db *gorm.DB, name string, noteTypeId *uint) {
	t.Helper()
	n := models.Note{Name: name, NoteTypeId: noteTypeId}
	if err := db.Create(&n).Error; err != nil {
		t.Fatalf("failed to seed note %q: %v", name, err)
	}
}

func TestNoteScope_NoteTypeIds_AllowsListedTypes(t *testing.T) {
	db := newNoteTestDB(t)
	nt1 := seedNoteType(t, db, "Type 1")
	nt2 := seedNoteType(t, db, "Type 2")
	nt3 := seedNoteType(t, db, "Type 3")
	seedNote(t, db, "n1", &nt1.ID)
	seedNote(t, db, "n2", &nt2.ID)
	seedNote(t, db, "n3", &nt3.ID)

	var got []models.Note
	err := db.Model(&models.Note{}).
		Scopes(NoteQuery(&query_models.NoteQuery{
			NoteTypeIds: []uint{nt1.ID, nt2.ID},
		}, true, db)).
		Find(&got).Error
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 results, got %d", len(got))
	}
	for _, n := range got {
		if n.NoteTypeId != nil && *n.NoteTypeId == nt3.ID {
			t.Errorf("note with type 3 should not be in results")
		}
	}
}
