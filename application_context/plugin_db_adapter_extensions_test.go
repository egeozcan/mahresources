//go:build json1 && fts5

package application_context

import (
	"testing"
)

// A getter that finds nothing must say so with (nil, nil), not with an error.
// Lua reads that split as "no such entity" versus "the read failed", and a
// plugin that cannot tell them apart takes its empty-data branch during an
// outage.
func TestPluginDBAdapter_GettersSeparateMissingFromFailed(t *testing.T) {
	ctx := createTestContext(t)
	adapter := &pluginDBAdapter{ctx: ctx}

	cases := []struct {
		name string
		get  func(uint) (map[string]any, error)
	}{
		{"note", adapter.GetNoteData},
		{"resource", adapter.GetResourceData},
		{"group", adapter.GetGroupData},
		{"tag", adapter.GetTagData},
		{"category", adapter.GetCategoryData},
		{"note_type", adapter.GetNoteTypeData},
		{"resource_category", adapter.GetResourceCategoryData},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			data, err := c.get(99999)
			if err != nil {
				t.Errorf("missing %s should not be an error, got %v", c.name, err)
			}
			if data != nil {
				t.Errorf("missing %s should be nil, got %v", c.name, data)
			}
		})
	}
}

// The find-or-create pattern: resolve a tag by name before creating one.
func TestPluginDBAdapter_ListTags(t *testing.T) {
	ctx := createTestContext(t)
	adapter := &pluginDBAdapter{ctx: ctx}

	if _, err := adapter.CreateTag(map[string]any{"name": "alpha"}); err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	if _, err := adapter.CreateTag(map[string]any{"name": "beta"}); err != nil {
		t.Fatalf("CreateTag: %v", err)
	}

	all, err := adapter.ListTags(map[string]any{})
	if err != nil {
		t.Fatalf("ListTags: %v", err)
	}
	if len(all) < 2 {
		t.Fatalf("expected at least 2 tags, got %d", len(all))
	}

	matched, err := adapter.ListTags(map[string]any{"name": "alpha"})
	if err != nil {
		t.Fatalf("ListTags(name): %v", err)
	}
	if len(matched) != 1 || matched[0]["name"] != "alpha" {
		t.Fatalf("expected exactly the 'alpha' tag, got %v", matched)
	}

	none, err := adapter.ListTags(map[string]any{"name": "nonexistent-tag"})
	if err != nil {
		t.Fatalf("ListTags(missing): %v", err)
	}
	if len(none) != 0 {
		t.Fatalf("expected no matches, got %v", none)
	}
}

func TestPluginDBAdapter_ListTaxonomies(t *testing.T) {
	ctx := createTestContext(t)
	adapter := &pluginDBAdapter{ctx: ctx}

	if _, err := adapter.CreateCategory(map[string]any{"name": "Projects"}); err != nil {
		t.Fatalf("CreateCategory: %v", err)
	}
	if _, err := adapter.CreateNoteType(map[string]any{"name": "Meeting"}); err != nil {
		t.Fatalf("CreateNoteType: %v", err)
	}
	if _, err := adapter.CreateResourceCategory(map[string]any{"name": "Photos"}); err != nil {
		t.Fatalf("CreateResourceCategory: %v", err)
	}

	categories, err := adapter.ListCategories(map[string]any{})
	if err != nil {
		t.Fatalf("ListCategories: %v", err)
	}
	if !containsName(categories, "Projects") {
		t.Errorf("ListCategories missing 'Projects': %v", categories)
	}

	noteTypes, err := adapter.ListNoteTypes(map[string]any{})
	if err != nil {
		t.Fatalf("ListNoteTypes: %v", err)
	}
	if !containsName(noteTypes, "Meeting") {
		t.Errorf("ListNoteTypes missing 'Meeting': %v", noteTypes)
	}

	resourceCategories, err := adapter.ListResourceCategories(map[string]any{})
	if err != nil {
		t.Fatalf("ListResourceCategories: %v", err)
	}
	if !containsName(resourceCategories, "Photos") {
		t.Errorf("ListResourceCategories missing 'Photos': %v", resourceCategories)
	}
}

func TestPluginDBAdapter_GetNoteTypeAndResourceCategory(t *testing.T) {
	ctx := createTestContext(t)
	adapter := &pluginDBAdapter{ctx: ctx}

	nt, err := adapter.CreateNoteType(map[string]any{"name": "Recipe"})
	if err != nil {
		t.Fatalf("CreateNoteType: %v", err)
	}
	fetched, err := adapter.GetNoteTypeData(uint(nt["id"].(float64)))
	if err != nil {
		t.Fatalf("GetNoteTypeData: %v", err)
	}
	if fetched["name"] != "Recipe" {
		t.Errorf("expected 'Recipe', got %v", fetched["name"])
	}

	rc, err := adapter.CreateResourceCategory(map[string]any{"name": "Scans"})
	if err != nil {
		t.Fatalf("CreateResourceCategory: %v", err)
	}
	fetchedRC, err := adapter.GetResourceCategoryData(uint(rc["id"].(float64)))
	if err != nil {
		t.Fatalf("GetResourceCategoryData: %v", err)
	}
	if fetchedRC["name"] != "Scans" {
		t.Errorf("expected 'Scans', got %v", fetchedRC["name"])
	}
}

func TestPluginDBAdapter_UpdateResource(t *testing.T) {
	ctx := createTestContext(t)
	adapter := &pluginDBAdapter{ctx: ctx}

	// Distinct bytes per test: AddResource dedupes on content hash, so reusing
	// another test's payload returns that resource instead of creating one.
	created, err := adapter.CreateResourceFromData(
		"VXBkYXRlIG1lIGVudGlyZWx5",
		map[string]any{"name": "before.txt", "description": "before desc"},
	)
	if err != nil {
		t.Fatalf("CreateResourceFromData: %v", err)
	}
	id := uint(created["id"].(float64))

	updated, err := adapter.UpdateResource(id, map[string]any{
		"name":        "after.txt",
		"description": "after desc",
	})
	if err != nil {
		t.Fatalf("UpdateResource: %v", err)
	}
	if updated["name"] != "after.txt" {
		t.Errorf("expected name 'after.txt', got %v", updated["name"])
	}
	if updated["description"] != "after desc" {
		t.Errorf("expected description 'after desc', got %v", updated["description"])
	}
}

// The documented exception to update_resource's replace-all contract:
// EditResource ignores an empty meta/width/height rather than writing them, so
// those three survive an update that omits them and cannot be cleared. It is
// the HTTP edit path's behaviour too, so it is pinned rather than diverged from.
func TestPluginDBAdapter_UpdateResourceCannotClearMeta(t *testing.T) {
	ctx := createTestContext(t)
	adapter := &pluginDBAdapter{ctx: ctx}

	created, err := adapter.CreateResourceFromData(
		"TWV0YSBzdXJ2aXZlcyBhbiB1cGRhdGU=",
		map[string]any{"name": "meta.txt", "meta": `{"kept":true}`},
	)
	if err != nil {
		t.Fatalf("CreateResourceFromData: %v", err)
	}
	id := uint(created["id"].(float64))

	// Omitted entirely.
	if _, err := adapter.UpdateResource(id, map[string]any{"name": "renamed.txt"}); err != nil {
		t.Fatalf("UpdateResource: %v", err)
	}
	after, err := adapter.GetResourceData(id)
	if err != nil {
		t.Fatalf("GetResourceData: %v", err)
	}
	if after["meta"] != `{"kept":true}` {
		t.Errorf("meta should survive an update that omits it, got %v", after["meta"])
	}

	// Explicitly emptied — also ignored.
	if _, err := adapter.PatchResource(id, map[string]any{"meta": ""}); err != nil {
		t.Fatalf("PatchResource: %v", err)
	}
	after, err = adapter.GetResourceData(id)
	if err != nil {
		t.Fatalf("GetResourceData: %v", err)
	}
	if after["meta"] != `{"kept":true}` {
		t.Errorf("an empty meta is ignored, not written; got %v", after["meta"])
	}

	// "{}" is how a plugin actually empties it.
	if _, err := adapter.PatchResource(id, map[string]any{"meta": "{}"}); err != nil {
		t.Fatalf("PatchResource: %v", err)
	}
	after, err = adapter.GetResourceData(id)
	if err != nil {
		t.Fatalf("GetResourceData: %v", err)
	}
	if after["meta"] != "{}" {
		t.Errorf(`meta should be emptied by "{}", got %v`, after["meta"])
	}
}

// Patch must leave alone what it was not asked to change — including the tags,
// which UpdateResource would clear.
func TestPluginDBAdapter_PatchResourcePreservesUnmentionedFields(t *testing.T) {
	ctx := createTestContext(t)
	adapter := &pluginDBAdapter{ctx: ctx}

	// Distinct bytes per test: AddResource dedupes on content hash, so reusing
	// another test's payload returns that resource instead of creating one.
	created, err := adapter.CreateResourceFromData(
		"UGF0Y2ggbWUsIG5vdCB0aGUgcmVzdA==",
		map[string]any{"name": "keep-me.txt", "description": "original desc"},
	)
	if err != nil {
		t.Fatalf("CreateResourceFromData: %v", err)
	}
	id := uint(created["id"].(float64))

	tag, err := adapter.CreateTag(map[string]any{"name": "patch-tag"})
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	tagID := uint(tag["id"].(float64))
	if err := adapter.AddTagsToEntity("resource", id, []uint{tagID}); err != nil {
		t.Fatalf("AddTagsToEntity: %v", err)
	}

	patched, err := adapter.PatchResource(id, map[string]any{"description": "patched desc"})
	if err != nil {
		t.Fatalf("PatchResource: %v", err)
	}
	if patched["description"] != "patched desc" {
		t.Errorf("expected description 'patched desc', got %v", patched["description"])
	}
	if patched["name"] != "keep-me.txt" {
		t.Errorf("patch clobbered the name: got %v", patched["name"])
	}

	after, err := adapter.GetResourceData(id)
	if err != nil {
		t.Fatalf("GetResourceData: %v", err)
	}
	tags, _ := after["tags"].([]any)
	if len(tags) != 1 {
		t.Fatalf("patch dropped the resource's tags: %v", after["tags"])
	}
}

// A value of the wrong shape where an ID list belongs must not be read as "no
// ids". getUintSliceOpt cannot tell "you asked for none" from "I could not read
// what you sent", and on a patch those have opposite consequences: this used to
// succeed and strip every tag off the resource.
func TestPluginDBAdapter_PatchIgnoresMistypedAssociations(t *testing.T) {
	ctx := createTestContext(t)
	adapter := &pluginDBAdapter{ctx: ctx}

	created, err := adapter.CreateResourceFromData(
		"TWlzdHlwZWQgYXNzb2NpYXRpb25z",
		map[string]any{"name": "tagged.txt"},
	)
	if err != nil {
		t.Fatalf("CreateResourceFromData: %v", err)
	}
	id := uint(created["id"].(float64))

	tag, err := adapter.CreateTag(map[string]any{"name": "mistyped-tag"})
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	if err := adapter.AddTagsToEntity("resource", id, []uint{uint(tag["id"].(float64))}); err != nil {
		t.Fatalf("AddTagsToEntity: %v", err)
	}

	if _, err := adapter.PatchResource(id, map[string]any{"tags": "not-an-id-array"}); err != nil {
		t.Fatalf("PatchResource: %v", err)
	}
	after, err := adapter.GetResourceData(id)
	if err != nil {
		t.Fatalf("GetResourceData: %v", err)
	}
	if tags, _ := after["tags"].([]any); len(tags) != 1 {
		t.Errorf("a mistyped tags value must not clear the tags, got %v", after["tags"])
	}

	// An explicit empty list is unambiguous and still clears.
	if _, err := adapter.PatchResource(id, map[string]any{"tags": []any{}}); err != nil {
		t.Fatalf("PatchResource(empty): %v", err)
	}
	after, err = adapter.GetResourceData(id)
	if err != nil {
		t.Fatalf("GetResourceData: %v", err)
	}
	if tags, _ := after["tags"].([]any); len(tags) != 0 {
		t.Errorf("an explicit empty list should clear the tags, got %v", after["tags"])
	}
}

// get_category returned only id/name/description, so a plugin could enumerate
// categories and read their templates but not fetch one by id and do the same.
func TestPluginDBAdapter_GetCategoryDataReturnsFullShape(t *testing.T) {
	ctx := createTestContext(t)
	adapter := &pluginDBAdapter{ctx: ctx}

	created, err := adapter.CreateCategory(map[string]any{
		"name":          "Full shape",
		"description":   "has templates",
		"custom_header": "<b>Configured</b>",
	})
	if err != nil {
		t.Fatalf("CreateCategory: %v", err)
	}

	fetched, err := adapter.GetCategoryData(uint(created["id"].(float64)))
	if err != nil {
		t.Fatalf("GetCategoryData: %v", err)
	}
	if fetched["custom_header"] != "<b>Configured</b>" {
		t.Errorf("custom_header = %v, want the configured template", fetched["custom_header"])
	}
	for _, key := range []string{"custom_sidebar", "custom_summary", "custom_avatar", "meta_schema"} {
		if _, present := fetched[key]; !present {
			t.Errorf("get_category should expose %q, matching list_categories", key)
		}
	}
}

func containsName(items []map[string]any, name string) bool {
	for _, item := range items {
		if item["name"] == name {
			return true
		}
	}
	return false
}

// A mistyped scalar field must not blank the stored value. getStringOpt answers
// "" for a value it cannot read, and on a patch that is "blank it", not "leave
// it alone".
func TestPluginDBAdapter_PatchIgnoresMistypedStrings(t *testing.T) {
	ctx := createTestContext(t)
	adapter := &pluginDBAdapter{ctx: ctx}

	created, err := adapter.CreateResourceFromData(
		"TWlzdHlwZWQgc3RyaW5ncw==",
		map[string]any{"name": "keep-my-name.txt", "description": "keep me"},
	)
	if err != nil {
		t.Fatalf("CreateResourceFromData: %v", err)
	}
	id := uint(created["id"].(float64))

	if _, err := adapter.PatchResource(id, map[string]any{"name": false}); err != nil {
		t.Fatalf("PatchResource: %v", err)
	}
	after, err := adapter.GetResourceData(id)
	if err != nil {
		t.Fatalf("GetResourceData: %v", err)
	}
	if after["name"] != "keep-my-name.txt" {
		t.Errorf("a mistyped name must not blank the stored one, got %q", after["name"])
	}

	// An explicit empty string is unambiguous and still clears.
	if _, err := adapter.PatchResource(id, map[string]any{"description": ""}); err != nil {
		t.Fatalf("PatchResource(empty): %v", err)
	}
	after, err = adapter.GetResourceData(id)
	if err != nil {
		t.Fatalf("GetResourceData: %v", err)
	}
	if after["description"] != "" {
		t.Errorf("an explicit empty string should clear, got %q", after["description"])
	}
}

func TestPluginDBAdapter_NoteTypeShareTemplateFlagRoundTrips(t *testing.T) {
	ctx := createTestContext(t)
	adapter := &pluginDBAdapter{ctx: ctx}
	created, err := adapter.CreateNoteType(map[string]any{
		"name": "Shared PM Task", "apply_templates_to_shares": true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if created["apply_templates_to_shares"] != true {
		t.Fatalf("created flag = %#v", created["apply_templates_to_shares"])
	}
	id := uint(created["id"].(float64))
	patched, err := adapter.PatchNoteType(id, map[string]any{"apply_templates_to_shares": false})
	if err != nil {
		t.Fatal(err)
	}
	if patched["apply_templates_to_shares"] != false {
		t.Fatalf("patched flag = %#v", patched["apply_templates_to_shares"])
	}
}
