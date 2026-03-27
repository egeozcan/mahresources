package application_context

import (
	"strings"
	"testing"

	"mahresources/models"
	"mahresources/models/query_models"
)

// =============================================
// Bug 1: Phantom entity creation via many-to-many associations
//
// When creating/updating entities with nonexistent association IDs,
// GORM silently creates phantom records in the join table or even
// inserts stub rows into the referenced table. This test ensures
// that all association IDs are validated before being used.
// =============================================

func TestCreateGroup_WithNonexistentTag_ReturnsError(t *testing.T) {
	ctx := createCoverageTestContext(t, "phantom_group_tag")

	_, err := ctx.CreateGroup(&query_models.GroupCreator{
		Name: "Test Group",
		Tags: []uint{999999},
	})
	if err == nil {
		t.Error("CreateGroup with nonexistent tag ID should return an error, but it succeeded (phantom tag created)")
	}
}

func TestCreateGroup_WithNonexistentRelatedGroup_ReturnsError(t *testing.T) {
	ctx := createCoverageTestContext(t, "phantom_group_group")

	_, err := ctx.CreateGroup(&query_models.GroupCreator{
		Name:   "Test Group",
		Groups: []uint{999999},
	})
	if err == nil {
		t.Error("CreateGroup with nonexistent related group ID should return an error, but it succeeded")
	}
}

func TestUpdateGroup_WithNonexistentTag_ReturnsError(t *testing.T) {
	ctx := createCoverageTestContext(t, "phantom_update_group_tag")

	group, err := ctx.CreateGroup(&query_models.GroupCreator{Name: "Test Group"})
	if err != nil {
		t.Fatalf("Setup: CreateGroup: %v", err)
	}

	_, err = ctx.UpdateGroup(&query_models.GroupEditor{
		GroupCreator: query_models.GroupCreator{
			Name: "Test Group Updated",
			Tags: []uint{999999},
		},
		ID: group.ID,
	})
	if err == nil {
		t.Error("UpdateGroup with nonexistent tag ID should return an error")
	}
}

func TestUpdateGroup_WithNonexistentRelatedGroup_ReturnsError(t *testing.T) {
	ctx := createCoverageTestContext(t, "phantom_update_group_rg")

	group, err := ctx.CreateGroup(&query_models.GroupCreator{Name: "Test Group"})
	if err != nil {
		t.Fatalf("Setup: CreateGroup: %v", err)
	}

	_, err = ctx.UpdateGroup(&query_models.GroupEditor{
		GroupCreator: query_models.GroupCreator{
			Name:   "Test Group Updated",
			Groups: []uint{999999},
		},
		ID: group.ID,
	})
	if err == nil {
		t.Error("UpdateGroup with nonexistent related group ID should return an error")
	}
}

func TestCreateNote_WithNonexistentTag_ReturnsError(t *testing.T) {
	ctx := createCoverageTestContext(t, "phantom_note_tag")

	_, err := ctx.CreateOrUpdateNote(&query_models.NoteEditor{
		NoteCreator: query_models.NoteCreator{
			Name: "Test Note",
			Tags: []uint{999999},
		},
	})
	if err == nil {
		t.Error("CreateOrUpdateNote with nonexistent tag ID should return an error")
	}
}

func TestCreateNote_WithNonexistentGroup_ReturnsError(t *testing.T) {
	ctx := createCoverageTestContext(t, "phantom_note_group")

	_, err := ctx.CreateOrUpdateNote(&query_models.NoteEditor{
		NoteCreator: query_models.NoteCreator{
			Name:   "Test Note",
			Groups: []uint{999999},
		},
	})
	if err == nil {
		t.Error("CreateOrUpdateNote with nonexistent group ID should return an error")
	}
}

func TestCreateNote_WithNonexistentResource_ReturnsError(t *testing.T) {
	ctx := createCoverageTestContext(t, "phantom_note_res")

	_, err := ctx.CreateOrUpdateNote(&query_models.NoteEditor{
		NoteCreator: query_models.NoteCreator{
			Name:      "Test Note",
			Resources: []uint{999999},
		},
	})
	if err == nil {
		t.Error("CreateOrUpdateNote with nonexistent resource ID should return an error")
	}
}

func TestEditResource_WithNonexistentTag_ReturnsError(t *testing.T) {
	ctx := createCoverageTestContext(t, "phantom_edit_res_tag")

	// Create a resource to edit
	res := &models.Resource{Name: "Test Resource", Meta: []byte("{}"), OwnMeta: []byte("{}")}
	if err := ctx.db.Create(res).Error; err != nil {
		t.Fatalf("Setup: Create resource: %v", err)
	}

	_, err := ctx.EditResource(&query_models.ResourceEditor{
		ResourceQueryBase: query_models.ResourceQueryBase{
			Name: "Test Resource Updated",
			Tags: []uint{999999},
			Meta: "{}",
		},
		ID: res.ID,
	})
	if err == nil {
		t.Error("EditResource with nonexistent tag ID should return an error")
	}
}

func TestEditResource_WithNonexistentGroup_ReturnsError(t *testing.T) {
	ctx := createCoverageTestContext(t, "phantom_edit_res_grp")

	res := &models.Resource{Name: "Test Resource", Meta: []byte("{}"), OwnMeta: []byte("{}")}
	if err := ctx.db.Create(res).Error; err != nil {
		t.Fatalf("Setup: Create resource: %v", err)
	}

	_, err := ctx.EditResource(&query_models.ResourceEditor{
		ResourceQueryBase: query_models.ResourceQueryBase{
			Name:   "Test Resource Updated",
			Groups: []uint{999999},
			Meta:   "{}",
		},
		ID: res.ID,
	})
	if err == nil {
		t.Error("EditResource with nonexistent group ID should return an error")
	}
}

func TestEditResource_WithNonexistentNote_ReturnsError(t *testing.T) {
	ctx := createCoverageTestContext(t, "phantom_edit_res_note")

	res := &models.Resource{Name: "Test Resource", Meta: []byte("{}"), OwnMeta: []byte("{}")}
	if err := ctx.db.Create(res).Error; err != nil {
		t.Fatalf("Setup: Create resource: %v", err)
	}

	_, err := ctx.EditResource(&query_models.ResourceEditor{
		ResourceQueryBase: query_models.ResourceQueryBase{
			Name:  "Test Resource Updated",
			Notes: []uint{999999},
			Meta:  "{}",
		},
		ID: res.ID,
	})
	if err == nil {
		t.Error("EditResource with nonexistent note ID should return an error")
	}
}

// Bulk operations should also validate

func TestAddTagsToNote_WithNonexistentTag_ReturnsError(t *testing.T) {
	ctx := createCoverageTestContext(t, "phantom_bulk_note_tag")

	note, err := ctx.CreateOrUpdateNote(&query_models.NoteEditor{
		NoteCreator: query_models.NoteCreator{Name: "Test Note"},
	})
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}

	err = ctx.AddTagsToNote(note.ID, []uint{999999})
	if err == nil {
		t.Error("AddTagsToNote with nonexistent tag ID should return an error")
	}
}

func TestAddGroupsToNote_WithNonexistentGroup_ReturnsError(t *testing.T) {
	ctx := createCoverageTestContext(t, "phantom_bulk_note_grp")

	note, err := ctx.CreateOrUpdateNote(&query_models.NoteEditor{
		NoteCreator: query_models.NoteCreator{Name: "Test Note"},
	})
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}

	err = ctx.AddGroupsToNote(note.ID, []uint{999999})
	if err == nil {
		t.Error("AddGroupsToNote with nonexistent group ID should return an error")
	}
}

func TestAddResourcesToNote_WithNonexistentResource_ReturnsError(t *testing.T) {
	ctx := createCoverageTestContext(t, "phantom_bulk_note_res")

	note, err := ctx.CreateOrUpdateNote(&query_models.NoteEditor{
		NoteCreator: query_models.NoteCreator{Name: "Test Note"},
	})
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}

	err = ctx.AddResourcesToNote(note.ID, []uint{999999})
	if err == nil {
		t.Error("AddResourcesToNote with nonexistent resource ID should return an error")
	}
}

// Ensure valid IDs still work after adding validation

func TestCreateGroup_WithExistingTag_Succeeds(t *testing.T) {
	ctx := createCoverageTestContext(t, "valid_group_tag")

	tag := &models.Tag{Name: "Valid Tag"}
	if err := ctx.db.Create(tag).Error; err != nil {
		t.Fatalf("Setup: Create tag: %v", err)
	}

	group, err := ctx.CreateGroup(&query_models.GroupCreator{
		Name: "Test Group",
		Tags: []uint{tag.ID},
	})
	if err != nil {
		t.Fatalf("CreateGroup with existing tag should succeed, got: %v", err)
	}
	if group == nil {
		t.Fatal("CreateGroup returned nil group")
	}
}

func TestCreateNote_WithExistingGroup_Succeeds(t *testing.T) {
	ctx := createCoverageTestContext(t, "valid_note_group")

	group, err := ctx.CreateGroup(&query_models.GroupCreator{Name: "Valid Group"})
	if err != nil {
		t.Fatalf("Setup: CreateGroup: %v", err)
	}

	note, err := ctx.CreateOrUpdateNote(&query_models.NoteEditor{
		NoteCreator: query_models.NoteCreator{
			Name:   "Test Note",
			Groups: []uint{group.ID},
		},
	})
	if err != nil {
		t.Fatalf("CreateOrUpdateNote with existing group should succeed, got: %v", err)
	}
	if note == nil {
		t.Fatal("CreateOrUpdateNote returned nil note")
	}
}

// =============================================
// Bug 13: Empty metadata key accepted
//
// ValidateMeta allows JSON objects with empty string keys like {"": "val"}.
// This causes issues with downstream operations and is meaningless.
// =============================================

func TestValidateMeta_EmptyKey_ReturnsError(t *testing.T) {
	err := ValidateMeta(`{"": "value"}`)
	if err == nil {
		t.Error("ValidateMeta should reject JSON objects with empty string keys")
	}
}

func TestValidateMeta_WhitespaceOnlyKey_ReturnsError(t *testing.T) {
	err := ValidateMeta(`{"  ": "value"}`)
	if err == nil {
		t.Error("ValidateMeta should reject JSON objects with whitespace-only keys")
	}
}

func TestValidateMeta_MixedKeysWithOneEmpty_ReturnsError(t *testing.T) {
	err := ValidateMeta(`{"valid": "ok", "": "bad"}`)
	if err == nil {
		t.Error("ValidateMeta should reject JSON objects when any key is empty")
	}
}

func TestValidateMeta_ValidKeys_Succeeds(t *testing.T) {
	err := ValidateMeta(`{"key1": "value1", "key2": "value2"}`)
	if err != nil {
		t.Errorf("ValidateMeta should accept valid keys, got: %v", err)
	}
}

// =============================================
// Bug 8: No max length validation on entity names
//
// Entity names have no maximum length validation. A name with millions
// of characters could be submitted, causing database bloat and
// rendering issues.
// =============================================

func TestCreateGroup_NameTooLong_ReturnsError(t *testing.T) {
	ctx := createCoverageTestContext(t, "name_len_group")

	longName := strings.Repeat("a", 1001)
	_, err := ctx.CreateGroup(&query_models.GroupCreator{
		Name: longName,
	})
	if err == nil {
		t.Error("CreateGroup should reject names longer than 1000 characters")
	}
}

func TestUpdateGroup_NameTooLong_ReturnsError(t *testing.T) {
	ctx := createCoverageTestContext(t, "name_len_upd_group")

	group, err := ctx.CreateGroup(&query_models.GroupCreator{Name: "Valid Name"})
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}

	longName := strings.Repeat("a", 1001)
	_, err = ctx.UpdateGroup(&query_models.GroupEditor{
		GroupCreator: query_models.GroupCreator{
			Name: longName,
		},
		ID: group.ID,
	})
	if err == nil {
		t.Error("UpdateGroup should reject names longer than 1000 characters")
	}
}

func TestCreateNote_NameTooLong_ReturnsError(t *testing.T) {
	ctx := createCoverageTestContext(t, "name_len_note")

	longName := strings.Repeat("a", 1001)
	_, err := ctx.CreateOrUpdateNote(&query_models.NoteEditor{
		NoteCreator: query_models.NoteCreator{
			Name: longName,
		},
	})
	if err == nil {
		t.Error("CreateOrUpdateNote should reject names longer than 1000 characters")
	}
}

func TestCreateTag_NameTooLong_ReturnsError(t *testing.T) {
	ctx := createCoverageTestContext(t, "name_len_tag")

	longName := strings.Repeat("a", 1001)
	_, err := ctx.CreateTag(&query_models.TagCreator{
		Name: longName,
	})
	if err == nil {
		t.Error("CreateTag should reject names longer than 1000 characters")
	}
}

func TestCreateCategory_NameTooLong_ReturnsError(t *testing.T) {
	ctx := createCoverageTestContext(t, "name_len_category")

	longName := strings.Repeat("a", 1001)
	_, err := ctx.CreateCategory(&query_models.CategoryCreator{
		Name: longName,
	})
	if err == nil {
		t.Error("CreateCategory should reject names longer than 1000 characters")
	}
}

func TestCreateQuery_NameTooLong_ReturnsError(t *testing.T) {
	ctx := createCoverageTestContext(t, "name_len_query")

	longName := strings.Repeat("a", 1001)
	_, err := ctx.CreateQuery(&query_models.QueryCreator{
		Name: longName,
		Text: "SELECT 1",
	})
	if err == nil {
		t.Error("CreateQuery should reject names longer than 1000 characters")
	}
}

func TestCreateNoteType_NameTooLong_ReturnsError(t *testing.T) {
	ctx := createCoverageTestContext(t, "name_len_notetype")

	longName := strings.Repeat("a", 1001)
	_, err := ctx.CreateOrUpdateNoteType(&query_models.NoteTypeEditor{
		Name: longName,
	})
	if err == nil {
		t.Error("CreateOrUpdateNoteType should reject names longer than 1000 characters")
	}
}

func TestCreateResourceCategory_NameTooLong_ReturnsError(t *testing.T) {
	ctx := createCoverageTestContext(t, "name_len_rescat")

	longName := strings.Repeat("a", 1001)
	_, err := ctx.CreateResourceCategory(&query_models.ResourceCategoryCreator{
		Name: longName,
	})
	if err == nil {
		t.Error("CreateResourceCategory should reject names longer than 1000 characters")
	}
}

// Valid names (at exactly 1000) should still work

func TestCreateGroup_NameAtMaxLength_Succeeds(t *testing.T) {
	ctx := createCoverageTestContext(t, "name_len_group_ok")

	name := strings.Repeat("a", 1000)
	group, err := ctx.CreateGroup(&query_models.GroupCreator{
		Name: name,
	})
	if err != nil {
		t.Fatalf("CreateGroup with 1000-char name should succeed, got: %v", err)
	}
	if group == nil {
		t.Fatal("CreateGroup returned nil")
	}
}
