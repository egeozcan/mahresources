package template_filters

import (
	"testing"

	"mahresources/models"
)

// The detail page decides which plugin actions to offer from this map. It set
// content_type for a resource but never category_id, so an action filtered on
// category_ids for a resource entity was never offered at all.
func TestActionEntityData_ResourceCarriesCategoryID(t *testing.T) {
	res := &models.Resource{ContentType: "image/png", ResourceCategoryId: 7}

	data := ActionEntityData(res, "resource")

	if got, ok := data["content_type"].(string); !ok || got != "image/png" {
		t.Errorf("content_type = %#v, want \"image/png\"", data["content_type"])
	}
	// uint, not int/uint64: actionMatchesFilters type-asserts uint, and a wrong
	// numeric type reads as "does not match".
	if got, ok := data["category_id"].(uint); !ok || got != 7 {
		t.Errorf("category_id = %#v, want uint(7)", data["category_id"])
	}
}

// Zero is not a real resource-category id, so it is left out entirely rather
// than published as one — the same rule the execute-time reader follows for a
// NULL column.
func TestActionEntityData_ResourceOmitsZeroCategoryID(t *testing.T) {
	res := &models.Resource{ContentType: "image/png"}

	data := ActionEntityData(res, "resource")

	if _, present := data["category_id"]; present {
		t.Errorf("category_id should be absent for an unset category, got %#v", data["category_id"])
	}
}

func TestActionEntityData_GroupAndNoteKeys(t *testing.T) {
	catID := uint(3)
	group := &models.Group{CategoryId: &catID}
	if got, ok := ActionEntityData(group, "group")["category_id"].(uint); !ok || got != catID {
		t.Errorf("group category_id not carried as uint(%d)", catID)
	}

	ntID := uint(5)
	note := &models.Note{NoteTypeId: &ntID}
	if got, ok := ActionEntityData(note, "note")["note_type_id"].(uint); !ok || got != ntID {
		t.Errorf("note note_type_id not carried as uint(%d)", ntID)
	}

	if _, present := ActionEntityData(&models.Note{}, "note")["note_type_id"]; present {
		t.Errorf("note_type_id should be absent when the note has no type")
	}
}

// A template may be handed an entity by value ([mrql] and [using] reflect over
// either form), and an [action] filter must read it exactly as it reads the
// pointer the detail page holds.
func TestActionEntityData_ValueForms(t *testing.T) {
	catID, ntID := uint(3), uint(5)
	res := ActionEntityData(models.Resource{ContentType: "image/png", ResourceCategoryId: 7}, "resource")
	if res["content_type"] != "image/png" || res["category_id"] != uint(7) {
		t.Errorf("resource value: %#v", res)
	}
	if got := ActionEntityData(models.Group{CategoryId: &catID}, "group")["category_id"]; got != catID {
		t.Errorf("group value category_id = %#v", got)
	}
	if got := ActionEntityData(models.Note{NoteTypeId: &ntID}, "note")["note_type_id"]; got != ntID {
		t.Errorf("note value note_type_id = %#v", got)
	}
	// Anything else, including a mismatched type, carries no filter data.
	if got := ActionEntityData(&models.Note{NoteTypeId: &ntID}, "resource"); len(got) != 0 {
		t.Errorf("a note read as a resource: %#v", got)
	}
	if got := ActionEntityData(nil, "group"); len(got) != 0 {
		t.Errorf("nil entity: %#v", got)
	}
}
