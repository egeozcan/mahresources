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

func TestPluginDBAdapter_PatchGroup(t *testing.T) {
	ctx := createTestContext(t)
	adapter := &pluginDBAdapter{ctx: ctx}

	// Create group with full fields
	tag, _ := adapter.CreateTag(map[string]any{"name": "patch-tag"})
	tagId := uint(tag["id"].(float64))
	result, err := adapter.CreateGroup(map[string]any{
		"name":        "Original Name",
		"description": "Original Desc",
		"meta":        `{"key":"value"}`,
		"tags":        []any{float64(tagId)},
	})
	if err != nil {
		t.Fatalf("CreateGroup failed: %v", err)
	}
	id := uint(result["id"].(float64))

	// Patch only the name — description, meta, tags should be preserved
	patched, err := adapter.PatchGroup(id, map[string]any{"name": "Patched Name"})
	if err != nil {
		t.Fatalf("PatchGroup failed: %v", err)
	}
	if patched["name"] != "Patched Name" {
		t.Errorf("expected name 'Patched Name', got %v", patched["name"])
	}
	if patched["description"] != "Original Desc" {
		t.Errorf("expected description preserved, got %v", patched["description"])
	}
	if patched["meta"] != `{"key":"value"}` {
		t.Errorf("expected meta preserved, got %v", patched["meta"])
	}

	// Verify tags preserved by reading back
	groupData, err := adapter.GetGroupData(id)
	if err != nil {
		t.Fatalf("GetGroupData failed: %v", err)
	}
	tags, ok := groupData["tags"].([]any)
	if !ok || len(tags) != 1 {
		t.Errorf("expected 1 tag preserved after patch, got %v", groupData["tags"])
	}
}

func TestPluginDBAdapter_PatchNote(t *testing.T) {
	ctx := createTestContext(t)
	adapter := &pluginDBAdapter{ctx: ctx}

	tag, _ := adapter.CreateTag(map[string]any{"name": "note-patch-tag"})
	tagId := uint(tag["id"].(float64))
	group, _ := adapter.CreateGroup(map[string]any{"name": "note-patch-group"})
	groupId := uint(group["id"].(float64))

	result, err := adapter.CreateNote(map[string]any{
		"name":        "Original Note",
		"description": "Original Note Desc",
		"tags":        []any{float64(tagId)},
		"groups":      []any{float64(groupId)},
	})
	if err != nil {
		t.Fatalf("CreateNote failed: %v", err)
	}
	id := uint(result["id"].(float64))

	// Patch only description
	patched, err := adapter.PatchNote(id, map[string]any{"description": "Patched Desc"})
	if err != nil {
		t.Fatalf("PatchNote failed: %v", err)
	}
	if patched["name"] != "Original Note" {
		t.Errorf("expected name preserved, got %v", patched["name"])
	}
	if patched["description"] != "Patched Desc" {
		t.Errorf("expected description 'Patched Desc', got %v", patched["description"])
	}

	// Verify associations preserved
	noteData, err := adapter.GetNoteData(id)
	if err != nil {
		t.Fatalf("GetNoteData failed: %v", err)
	}
	tags, ok := noteData["tags"].([]any)
	if !ok || len(tags) != 1 {
		t.Errorf("expected 1 tag preserved after patch, got %v", noteData["tags"])
	}
}

func TestPluginDBAdapter_PatchCategory(t *testing.T) {
	ctx := createTestContext(t)
	adapter := &pluginDBAdapter{ctx: ctx}

	result, err := adapter.CreateCategory(map[string]any{
		"name":          "Orig Cat",
		"description":   "Orig Desc",
		"custom_header": "<h1>Header</h1>",
	})
	if err != nil {
		t.Fatalf("CreateCategory failed: %v", err)
	}
	id := uint(result["id"].(float64))

	// Patch only name — custom_header should be preserved
	patched, err := adapter.PatchCategory(id, map[string]any{"name": "Patched Cat"})
	if err != nil {
		t.Fatalf("PatchCategory failed: %v", err)
	}
	if patched["name"] != "Patched Cat" {
		t.Errorf("expected 'Patched Cat', got %v", patched["name"])
	}

	// Verify custom_header preserved by reading the model directly
	cat, err := ctx.GetCategory(id)
	if err != nil {
		t.Fatalf("GetCategory failed: %v", err)
	}
	if cat.CustomHeader != "<h1>Header</h1>" {
		t.Errorf("expected custom_header preserved, got %q", cat.CustomHeader)
	}
}

func TestPluginDBAdapter_PatchTag(t *testing.T) {
	ctx := createTestContext(t)
	adapter := &pluginDBAdapter{ctx: ctx}

	result, _ := adapter.CreateTag(map[string]any{"name": "orig-tag", "description": "orig desc"})
	id := uint(result["id"].(float64))

	patched, err := adapter.PatchTag(id, map[string]any{"name": "patched-tag"})
	if err != nil {
		t.Fatalf("PatchTag failed: %v", err)
	}
	if patched["name"] != "patched-tag" {
		t.Errorf("expected 'patched-tag', got %v", patched["name"])
	}
	if patched["description"] != "orig desc" {
		t.Errorf("expected description preserved, got %v", patched["description"])
	}
}

func TestPluginDBAdapter_ResourceRelationships(t *testing.T) {
	ctx := createTestContext(t)
	adapter := &pluginDBAdapter{ctx: ctx}

	// Create a resource using base64 data
	resource, err := adapter.CreateResourceFromData(
		"SGVsbG8gV29ybGQ=", // "Hello World"
		map[string]any{"name": "test-resource.txt"},
	)
	if err != nil {
		t.Fatalf("CreateResourceFromData failed: %v", err)
	}
	resourceId := uint(resource["id"].(float64))

	tag, _ := adapter.CreateTag(map[string]any{"name": "res-tag"})
	tagId := uint(tag["id"].(float64))
	group, _ := adapter.CreateGroup(map[string]any{"name": "res-group"})
	groupId := uint(group["id"].(float64))

	// Add/remove tags on resource
	if err := adapter.AddTagsToEntity("resource", resourceId, []uint{tagId}); err != nil {
		t.Fatalf("AddTagsToEntity(resource) failed: %v", err)
	}
	if err := adapter.RemoveTagsFromEntity("resource", resourceId, []uint{tagId}); err != nil {
		t.Fatalf("RemoveTagsFromEntity(resource) failed: %v", err)
	}

	// Add/remove groups on resource
	if err := adapter.AddGroupsToEntity("resource", resourceId, []uint{groupId}); err != nil {
		t.Fatalf("AddGroupsToEntity(resource) failed: %v", err)
	}
	if err := adapter.RemoveGroupsFromEntity("resource", resourceId, []uint{groupId}); err != nil {
		t.Fatalf("RemoveGroupsFromEntity(resource) failed: %v", err)
	}

	// Delete resource
	if err := adapter.DeleteResource(resourceId); err != nil {
		t.Fatalf("DeleteResource failed: %v", err)
	}
}

func TestPluginDBAdapter_NoteResourceRelationships(t *testing.T) {
	ctx := createTestContext(t)
	adapter := &pluginDBAdapter{ctx: ctx}

	note, _ := adapter.CreateNote(map[string]any{"name": "Res Note"})
	noteId := uint(note["id"].(float64))
	resource, err := adapter.CreateResourceFromData(
		"SGVsbG8=",
		map[string]any{"name": "note-res.txt"},
	)
	if err != nil {
		t.Fatalf("CreateResourceFromData failed: %v", err)
	}
	resourceId := uint(resource["id"].(float64))

	if err := adapter.AddResourcesToNote(noteId, []uint{resourceId}); err != nil {
		t.Fatalf("AddResourcesToNote failed: %v", err)
	}
	if err := adapter.RemoveResourcesFromNote(noteId, []uint{resourceId}); err != nil {
		t.Fatalf("RemoveResourcesFromNote failed: %v", err)
	}
}

func TestPluginDBAdapter_GroupRelationCRUD(t *testing.T) {
	ctx := createTestContext(t)
	adapter := &pluginDBAdapter{ctx: ctx}

	// Create prerequisite entities: category is required for relation type matching
	cat, _ := adapter.CreateCategory(map[string]any{"name": "Rel Category"})
	catId := cat["id"].(float64)

	group1, _ := adapter.CreateGroup(map[string]any{"name": "Rel Group 1", "category_id": catId})
	group1Id := uint(group1["id"].(float64))
	group2, _ := adapter.CreateGroup(map[string]any{"name": "Rel Group 2", "category_id": catId})
	group2Id := uint(group2["id"].(float64))

	rt, err := adapter.CreateRelationType(map[string]any{
		"name":          "parent-of",
		"description":   "Parent relationship",
		"from_category": catId,
		"to_category":   catId,
	})
	if err != nil {
		t.Fatalf("CreateRelationType failed: %v", err)
	}
	rtId := uint(rt["id"].(float64))

	// Create group relation
	rel, err := adapter.CreateGroupRelation(map[string]any{
		"from_group_id":    float64(group1Id),
		"to_group_id":      float64(group2Id),
		"relation_type_id": float64(rtId),
		"name":             "test relation",
	})
	if err != nil {
		t.Fatalf("CreateGroupRelation failed: %v", err)
	}
	relId := uint(rel["id"].(float64))
	if rel["name"] != "test relation" {
		t.Errorf("expected name 'test relation', got %v", rel["name"])
	}

	// Update group relation
	updated, err := adapter.UpdateGroupRelation(map[string]any{
		"id":          float64(relId),
		"name":        "updated relation",
		"description": "updated desc",
	})
	if err != nil {
		t.Fatalf("UpdateGroupRelation failed: %v", err)
	}
	if updated["name"] != "updated relation" {
		t.Errorf("expected 'updated relation', got %v", updated["name"])
	}

	// Patch group relation — only change description
	patched, err := adapter.PatchGroupRelation(map[string]any{
		"id":          float64(relId),
		"description": "patched desc",
	})
	if err != nil {
		t.Fatalf("PatchGroupRelation failed: %v", err)
	}
	if patched["name"] != "updated relation" {
		t.Errorf("expected name preserved after patch, got %v", patched["name"])
	}
	if patched["description"] != "patched desc" {
		t.Errorf("expected 'patched desc', got %v", patched["description"])
	}

	// Delete group relation
	if err := adapter.DeleteGroupRelation(relId); err != nil {
		t.Fatalf("DeleteGroupRelation failed: %v", err)
	}
}

func TestPluginDBAdapter_RelationTypeCRUD(t *testing.T) {
	ctx := createTestContext(t)
	adapter := &pluginDBAdapter{ctx: ctx}

	// Create categories required by relation types
	catFrom, err := adapter.CreateCategory(map[string]any{
		"name": "From Cat",
	})
	if err != nil {
		t.Fatalf("CreateCategory (from) failed: %v", err)
	}
	catTo, err := adapter.CreateCategory(map[string]any{
		"name": "To Cat",
	})
	if err != nil {
		t.Fatalf("CreateCategory (to) failed: %v", err)
	}

	result, err := adapter.CreateRelationType(map[string]any{
		"name":          "belongs-to",
		"description":   "Belongs to relationship",
		"from_category": catFrom["id"],
		"to_category":   catTo["id"],
	})
	if err != nil {
		t.Fatalf("CreateRelationType failed: %v", err)
	}
	id := uint(result["id"].(float64))

	// Update
	updated, err := adapter.UpdateRelationType(map[string]any{
		"id":          float64(id),
		"name":        "owned-by",
		"description": "Owned by relationship",
	})
	if err != nil {
		t.Fatalf("UpdateRelationType failed: %v", err)
	}
	if updated["name"] != "owned-by" {
		t.Errorf("expected 'owned-by', got %v", updated["name"])
	}

	// Patch — only change description
	patched, err := adapter.PatchRelationType(map[string]any{
		"id":          float64(id),
		"description": "patched desc",
	})
	if err != nil {
		t.Fatalf("PatchRelationType failed: %v", err)
	}
	if patched["name"] != "owned-by" {
		t.Errorf("expected name preserved, got %v", patched["name"])
	}
	if patched["description"] != "patched desc" {
		t.Errorf("expected 'patched desc', got %v", patched["description"])
	}

	// Delete
	if err := adapter.DeleteRelationType(id); err != nil {
		t.Fatalf("DeleteRelationType failed: %v", err)
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
