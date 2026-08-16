//go:build json1 && fts5

package application_context

import (
	"testing"

	"mahresources/models"
)

func TestActionEntityDataReader_ResourceKeysAndTypes(t *testing.T) {
	ctx := createIsolatedTestContext(t)
	cat := createResourceCategory(t, ctx, "Photos")
	res := createResourceInCategory(t, ctx, "a.png", "image/png", cat.ID)

	data, err := NewActionEntityDataReader(ctx).EntityFilterData("resource", []uint{res.ID})
	if err != nil {
		t.Fatalf("EntityFilterData: %v", err)
	}
	entry, ok := data[res.ID]
	if !ok {
		t.Fatalf("expected an entry for resource %d, got %#v", res.ID, data)
	}
	// The types are load-bearing: actionMatchesFilters asserts string and uint,
	// and any other numeric type reads as "does not match".
	if got, ok := entry["content_type"].(string); !ok || got != "image/png" {
		t.Errorf("content_type = %#v, want \"image/png\"", entry["content_type"])
	}
	if got, ok := entry["category_id"].(uint); !ok || got != cat.ID {
		t.Errorf("category_id = %#v, want uint(%d)", entry["category_id"], cat.ID)
	}
}

func TestActionEntityDataReader_NoteAndGroupKeys(t *testing.T) {
	ctx := createIsolatedTestContext(t)
	nt := createNoteType(t, ctx, "Journal")
	note := createNoteWithType(t, ctx, "entry", nt.ID)
	cat := createCategory(t, ctx, "People")
	group := createGroupWithCategory(t, ctx, "band", cat.ID)

	reader := NewActionEntityDataReader(ctx)

	noteData, err := reader.EntityFilterData("note", []uint{note.ID})
	if err != nil {
		t.Fatalf("EntityFilterData(note): %v", err)
	}
	if got, ok := noteData[note.ID]["note_type_id"].(uint); !ok || got != nt.ID {
		t.Errorf("note_type_id = %#v, want uint(%d)", noteData[note.ID]["note_type_id"], nt.ID)
	}

	groupData, err := reader.EntityFilterData("group", []uint{group.ID})
	if err != nil {
		t.Fatalf("EntityFilterData(group): %v", err)
	}
	if got, ok := groupData[group.ID]["category_id"].(uint); !ok || got != cat.ID {
		t.Errorf("category_id = %#v, want uint(%d)", groupData[group.ID]["category_id"], cat.ID)
	}
}

// A NULL column is an ABSENT key, never a zero: actionMatchesFilters reads a
// missing key as "does not match" (the fail-closed answer), while 0 is a real
// id that could match the wrong filter.
func TestActionEntityDataReader_OmitsNullColumns(t *testing.T) {
	ctx := createIsolatedTestContext(t)
	note := &models.Note{Name: "untyped"}
	if err := ctx.db.Create(note).Error; err != nil {
		t.Fatalf("create note: %v", err)
	}
	group := createGroupWithCategory(t, ctx, "uncategorised", 0)

	reader := NewActionEntityDataReader(ctx)

	noteData, err := reader.EntityFilterData("note", []uint{note.ID})
	if err != nil {
		t.Fatalf("EntityFilterData(note): %v", err)
	}
	entry, ok := noteData[note.ID]
	if !ok {
		t.Fatalf("expected an entry for the untyped note")
	}
	if _, present := entry["note_type_id"]; present {
		t.Errorf("note_type_id should be absent for a NULL note type, got %#v", entry["note_type_id"])
	}

	groupData, err := reader.EntityFilterData("group", []uint{group.ID})
	if err != nil {
		t.Fatalf("EntityFilterData(group): %v", err)
	}
	if _, present := groupData[group.ID]["category_id"]; present {
		t.Errorf("category_id should be absent for a NULL category, got %#v", groupData[group.ID]["category_id"])
	}
}

// An id that does not exist gets NO entry — not an empty one. An empty entry
// would still fail a filter, but only by accident; absence is the contract the
// validator relies on.
func TestActionEntityDataReader_MissingIDsAreAbsent(t *testing.T) {
	ctx := createIsolatedTestContext(t)
	res := createResourceWithType(t, ctx, "a.png", "image/png")

	data, err := NewActionEntityDataReader(ctx).EntityFilterData("resource", []uint{res.ID, 999999})
	if err != nil {
		t.Fatalf("EntityFilterData: %v", err)
	}
	if _, present := data[999999]; present {
		t.Errorf("expected no entry for a non-existent id, got %#v", data[999999])
	}
	if len(data) != 1 {
		t.Errorf("expected exactly one entry, got %#v", data)
	}
}

// The id set is chunked to stay under SQLite's variable-binding limit, like the
// entity_ref reader beside it.
func TestActionEntityDataReader_Chunking(t *testing.T) {
	ctx := createIsolatedTestContext(t)
	ids := make([]uint, 0, entityRefChunkSize+50)
	for i := 0; i < entityRefChunkSize+50; i++ {
		ids = append(ids, createResourceWithType(t, ctx, "r.png", "image/png").ID)
	}

	data, err := NewActionEntityDataReader(ctx).EntityFilterData("resource", ids)
	if err != nil {
		t.Fatalf("EntityFilterData: %v", err)
	}
	if len(data) != len(ids) {
		t.Errorf("expected %d entries across chunks, got %d", len(ids), len(data))
	}
}

func TestActionEntityDataReader_UnknownEntityErrors(t *testing.T) {
	ctx := createIsolatedTestContext(t)

	if _, err := NewActionEntityDataReader(ctx).EntityFilterData("tag", []uint{1}); err == nil {
		t.Error("expected an error for an unknown entity type")
	}
}
