//go:build json1 && fts5

package application_context

import (
	"testing"
)

func TestPluginDBAdapter_GroupCRUD(t *testing.T) {
	ctx := createTestContext(t)
	adapter := &pluginDBAdapter{ctx: ctx}

	// Create
	result, err := adapter.CreateGroup(map[string]any{
		"name":        "Test Group",
		"description": "A test group",
	})
	if err != nil {
		t.Fatalf("CreateGroup failed: %v", err)
	}
	if result["name"] != "Test Group" {
		t.Errorf("expected name 'Test Group', got %v", result["name"])
	}
	id := uint(result["id"].(float64))
	if id == 0 {
		t.Fatal("expected non-zero ID")
	}

	// Update
	result, err = adapter.UpdateGroup(id, map[string]any{
		"name":        "Updated Group",
		"description": "Updated desc",
	})
	if err != nil {
		t.Fatalf("UpdateGroup failed: %v", err)
	}
	if result["name"] != "Updated Group" {
		t.Errorf("expected name 'Updated Group', got %v", result["name"])
	}

	// Delete
	if err := adapter.DeleteGroup(id); err != nil {
		t.Fatalf("DeleteGroup failed: %v", err)
	}
}

func TestPluginDBAdapter_NoteCRUD(t *testing.T) {
	ctx := createTestContext(t)
	adapter := &pluginDBAdapter{ctx: ctx}

	result, err := adapter.CreateNote(map[string]any{
		"name":        "Test Note",
		"description": "A test note",
	})
	if err != nil {
		t.Fatalf("CreateNote failed: %v", err)
	}
	id := uint(result["id"].(float64))

	result, err = adapter.UpdateNote(id, map[string]any{
		"name":        "Updated Note",
		"description": "Updated desc",
	})
	if err != nil {
		t.Fatalf("UpdateNote failed: %v", err)
	}
	if result["name"] != "Updated Note" {
		t.Errorf("expected 'Updated Note', got %v", result["name"])
	}

	if err := adapter.DeleteNote(id); err != nil {
		t.Fatalf("DeleteNote failed: %v", err)
	}
}

func TestPluginDBAdapter_TagCRUD(t *testing.T) {
	ctx := createTestContext(t)
	adapter := &pluginDBAdapter{ctx: ctx}

	result, err := adapter.CreateTag(map[string]any{"name": "test-tag"})
	if err != nil {
		t.Fatalf("CreateTag failed: %v", err)
	}
	id := uint(result["id"].(float64))

	result, err = adapter.UpdateTag(id, map[string]any{"name": "renamed-tag"})
	if err != nil {
		t.Fatalf("UpdateTag failed: %v", err)
	}
	if result["name"] != "renamed-tag" {
		t.Errorf("expected 'renamed-tag', got %v", result["name"])
	}

	if err := adapter.DeleteTag(id); err != nil {
		t.Fatalf("DeleteTag failed: %v", err)
	}
}

func TestPluginDBAdapter_CategoryCRUD(t *testing.T) {
	ctx := createTestContext(t)
	adapter := &pluginDBAdapter{ctx: ctx}

	result, err := adapter.CreateCategory(map[string]any{"name": "Test Category"})
	if err != nil {
		t.Fatalf("CreateCategory failed: %v", err)
	}
	id := uint(result["id"].(float64))

	result, err = adapter.UpdateCategory(id, map[string]any{"name": "Updated Cat"})
	if err != nil {
		t.Fatalf("UpdateCategory failed: %v", err)
	}
	if result["name"] != "Updated Cat" {
		t.Errorf("expected 'Updated Cat', got %v", result["name"])
	}

	if err := adapter.DeleteCategory(id); err != nil {
		t.Fatalf("DeleteCategory failed: %v", err)
	}
}

func TestPluginDBAdapter_ResourceCategoryCRUD(t *testing.T) {
	ctx := createTestContext(t)
	adapter := &pluginDBAdapter{ctx: ctx}

	result, err := adapter.CreateResourceCategory(map[string]any{"name": "Test RC"})
	if err != nil {
		t.Fatalf("CreateResourceCategory failed: %v", err)
	}
	id := uint(result["id"].(float64))

	result, err = adapter.UpdateResourceCategory(id, map[string]any{"name": "Updated RC"})
	if err != nil {
		t.Fatalf("UpdateResourceCategory failed: %v", err)
	}
	if result["name"] != "Updated RC" {
		t.Errorf("expected 'Updated RC', got %v", result["name"])
	}

	if err := adapter.DeleteResourceCategory(id); err != nil {
		t.Fatalf("DeleteResourceCategory failed: %v", err)
	}
}

func TestPluginDBAdapter_NoteTypeCRUD(t *testing.T) {
	ctx := createTestContext(t)
	adapter := &pluginDBAdapter{ctx: ctx}

	result, err := adapter.CreateNoteType(map[string]any{"name": "Test NoteType"})
	if err != nil {
		t.Fatalf("CreateNoteType failed: %v", err)
	}
	id := uint(result["id"].(float64))

	result, err = adapter.UpdateNoteType(id, map[string]any{"name": "Updated NT"})
	if err != nil {
		t.Fatalf("UpdateNoteType failed: %v", err)
	}
	if result["name"] != "Updated NT" {
		t.Errorf("expected 'Updated NT', got %v", result["name"])
	}

	if err := adapter.DeleteNoteType(id); err != nil {
		t.Fatalf("DeleteNoteType failed: %v", err)
	}
}

func TestPluginDBAdapter_TagRelationships(t *testing.T) {
	ctx := createTestContext(t)
	adapter := &pluginDBAdapter{ctx: ctx}

	// Create entities to work with
	groupRes, _ := adapter.CreateGroup(map[string]any{"name": "Rel Group"})
	groupId := uint(groupRes["id"].(float64))
	tagRes, _ := adapter.CreateTag(map[string]any{"name": "rel-tag"})
	tagId := uint(tagRes["id"].(float64))
	noteRes, _ := adapter.CreateNote(map[string]any{"name": "Rel Note"})
	noteId := uint(noteRes["id"].(float64))

	// Add tags to group
	if err := adapter.AddTagsToEntity("group", groupId, []uint{tagId}); err != nil {
		t.Fatalf("AddTagsToEntity(group) failed: %v", err)
	}
	// Remove tags from group
	if err := adapter.RemoveTagsFromEntity("group", groupId, []uint{tagId}); err != nil {
		t.Fatalf("RemoveTagsFromEntity(group) failed: %v", err)
	}

	// Add tags to note
	if err := adapter.AddTagsToEntity("note", noteId, []uint{tagId}); err != nil {
		t.Fatalf("AddTagsToEntity(note) failed: %v", err)
	}
	if err := adapter.RemoveTagsFromEntity("note", noteId, []uint{tagId}); err != nil {
		t.Fatalf("RemoveTagsFromEntity(note) failed: %v", err)
	}

	// Add groups to note
	if err := adapter.AddGroupsToEntity("note", noteId, []uint{groupId}); err != nil {
		t.Fatalf("AddGroupsToEntity(note) failed: %v", err)
	}
	if err := adapter.RemoveGroupsFromEntity("note", noteId, []uint{groupId}); err != nil {
		t.Fatalf("RemoveGroupsFromEntity(note) failed: %v", err)
	}

	// Invalid entity type
	if err := adapter.AddTagsToEntity("invalid", 1, []uint{1}); err == nil {
		t.Error("expected error for invalid entity type")
	}

	// Cleanup
	adapter.DeleteNote(noteId)
	adapter.DeleteTag(tagId)
	adapter.DeleteGroup(groupId)
}
