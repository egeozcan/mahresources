package plugin_system

import (
	"encoding/base64"
	"fmt"
	"testing"
)

type mockQuerier struct{}

func (m *mockQuerier) GetNoteData(id uint) (map[string]any, error) {
	if id == 1 {
		return map[string]any{"id": float64(1), "name": "Test Note", "description": "A note"}, nil
	}
	return nil, fmt.Errorf("not found")
}

func (m *mockQuerier) GetResourceData(id uint) (map[string]any, error) {
	if id == 1 {
		return map[string]any{"id": float64(1), "name": "test.jpg", "content_type": "image/jpeg"}, nil
	}
	return nil, fmt.Errorf("not found")
}

func (m *mockQuerier) GetGroupData(id uint) (map[string]any, error) {
	if id == 1 {
		return map[string]any{"id": float64(1), "name": "Test Group"}, nil
	}
	return nil, fmt.Errorf("not found")
}

func (m *mockQuerier) GetTagData(id uint) (map[string]any, error) {
	if id == 1 {
		return map[string]any{"id": float64(1), "name": "test-tag"}, nil
	}
	return nil, fmt.Errorf("not found")
}

func (m *mockQuerier) GetCategoryData(id uint) (map[string]any, error) {
	if id == 1 {
		return map[string]any{"id": float64(1), "name": "Test Category"}, nil
	}
	return nil, fmt.Errorf("not found")
}

func (m *mockQuerier) QueryNotes(filter map[string]any) ([]map[string]any, error) {
	return []map[string]any{
		{"id": float64(1), "name": "Note 1"},
		{"id": float64(2), "name": "Note 2"},
	}, nil
}

func (m *mockQuerier) QueryResources(filter map[string]any) ([]map[string]any, error) {
	return []map[string]any{
		{"id": float64(1), "name": "file.jpg"},
	}, nil
}

func (m *mockQuerier) QueryGroups(filter map[string]any) ([]map[string]any, error) {
	return []map[string]any{
		{"id": float64(1), "name": "Group A"},
	}, nil
}

func (m *mockQuerier) GetResourceFileData(id uint) (string, string, error) {
	if id == 1 {
		return base64.StdEncoding.EncodeToString([]byte("fake-image-data")), "image/jpeg", nil
	}
	return "", "", fmt.Errorf("not found")
}

func (m *mockQuerier) CreateResourceFromURL(url string, options map[string]any) (map[string]any, error) {
	name := "downloaded.jpg"
	if n, ok := options["name"].(string); ok {
		name = n
	}
	return map[string]any{
		"id":           float64(42),
		"name":         name,
		"content_type": "image/jpeg",
	}, nil
}

func (m *mockQuerier) CreateResourceFromData(base64Data string, options map[string]any) (map[string]any, error) {
	if base64Data == "" {
		return nil, fmt.Errorf("empty data")
	}
	if _, err := base64.StdEncoding.DecodeString(base64Data); err != nil {
		return nil, fmt.Errorf("invalid base64 data: %w", err)
	}
	name := "uploaded.bin"
	if n, ok := options["name"].(string); ok {
		name = n
	}
	return map[string]any{
		"id":           float64(43),
		"name":         name,
		"content_type": "application/octet-stream",
	}, nil
}

func TestDbApi_GetNote(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "db-test", `
plugin = { name = "db-test", version = "1.0", description = "db api test" }
function init()
    mah.inject("test", function(ctx)
        local note = mah.db.get_note(1)
        if note then
            return note.name
        end
        return "not found"
    end)
end
`)
	mgr, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()
	mgr.SetEntityQuerier(&mockQuerier{})

	if err := mgr.EnablePlugin("db-test"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	html := mgr.RenderSlot("test", map[string]any{})
	if html != "Test Note" {
		t.Errorf("expected 'Test Note', got %q", html)
	}
}

func TestDbApi_GetNoteNotFound(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "db-test", `
plugin = { name = "db-test", version = "1.0", description = "db api test" }
function init()
    mah.inject("test", function(ctx)
        local note = mah.db.get_note(999)
        if note then
            return "found"
        end
        return "nil"
    end)
end
`)
	mgr, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()
	mgr.SetEntityQuerier(&mockQuerier{})

	if err := mgr.EnablePlugin("db-test"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	html := mgr.RenderSlot("test", map[string]any{})
	if html != "nil" {
		t.Errorf("expected 'nil', got %q", html)
	}
}

func TestDbApi_GetResource(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "db-test", `
plugin = { name = "db-test", version = "1.0", description = "db api test" }
function init()
    mah.inject("test", function(ctx)
        local r = mah.db.get_resource(1)
        if r then
            return r.name .. "|" .. r.content_type
        end
        return "not found"
    end)
end
`)
	mgr, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()
	mgr.SetEntityQuerier(&mockQuerier{})

	if err := mgr.EnablePlugin("db-test"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	html := mgr.RenderSlot("test", map[string]any{})
	if html != "test.jpg|image/jpeg" {
		t.Errorf("expected 'test.jpg|image/jpeg', got %q", html)
	}
}

func TestDbApi_GetGroup(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "db-test", `
plugin = { name = "db-test", version = "1.0", description = "db api test" }
function init()
    mah.inject("test", function(ctx)
        local g = mah.db.get_group(1)
        if g then
            return g.name
        end
        return "not found"
    end)
end
`)
	mgr, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()
	mgr.SetEntityQuerier(&mockQuerier{})

	if err := mgr.EnablePlugin("db-test"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	html := mgr.RenderSlot("test", map[string]any{})
	if html != "Test Group" {
		t.Errorf("expected 'Test Group', got %q", html)
	}
}

func TestDbApi_GetTag(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "db-test", `
plugin = { name = "db-test", version = "1.0", description = "db api test" }
function init()
    mah.inject("test", function(ctx)
        local tag = mah.db.get_tag(1)
        if tag then
            return tag.name
        end
        return "not found"
    end)
end
`)
	mgr, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()
	mgr.SetEntityQuerier(&mockQuerier{})

	if err := mgr.EnablePlugin("db-test"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	html := mgr.RenderSlot("test", map[string]any{})
	if html != "test-tag" {
		t.Errorf("expected 'test-tag', got %q", html)
	}
}

func TestDbApi_GetCategory(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "db-test", `
plugin = { name = "db-test", version = "1.0", description = "db api test" }
function init()
    mah.inject("test", function(ctx)
        local cat = mah.db.get_category(1)
        if cat then
            return cat.name
        end
        return "not found"
    end)
end
`)
	mgr, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()
	mgr.SetEntityQuerier(&mockQuerier{})

	if err := mgr.EnablePlugin("db-test"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	html := mgr.RenderSlot("test", map[string]any{})
	if html != "Test Category" {
		t.Errorf("expected 'Test Category', got %q", html)
	}
}

func TestDbApi_QueryNotes(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "db-test", `
plugin = { name = "db-test", version = "1.0", description = "db api test" }
function init()
    mah.inject("test", function(ctx)
        local notes = mah.db.query_notes({limit = 10})
        if notes then
            return tostring(#notes)
        end
        return "0"
    end)
end
`)
	mgr, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()
	mgr.SetEntityQuerier(&mockQuerier{})

	if err := mgr.EnablePlugin("db-test"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	html := mgr.RenderSlot("test", map[string]any{})
	if html != "2" {
		t.Errorf("expected '2', got %q", html)
	}
}

func TestDbApi_QueryResources(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "db-test", `
plugin = { name = "db-test", version = "1.0", description = "db api test" }
function init()
    mah.inject("test", function(ctx)
        local resources = mah.db.query_resources({limit = 5})
        if resources then
            return resources[1].name
        end
        return "none"
    end)
end
`)
	mgr, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()
	mgr.SetEntityQuerier(&mockQuerier{})

	if err := mgr.EnablePlugin("db-test"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	html := mgr.RenderSlot("test", map[string]any{})
	if html != "file.jpg" {
		t.Errorf("expected 'file.jpg', got %q", html)
	}
}

func TestDbApi_QueryGroups(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "db-test", `
plugin = { name = "db-test", version = "1.0", description = "db api test" }
function init()
    mah.inject("test", function(ctx)
        local groups = mah.db.query_groups()
        if groups then
            return groups[1].name
        end
        return "none"
    end)
end
`)
	mgr, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()
	mgr.SetEntityQuerier(&mockQuerier{})

	if err := mgr.EnablePlugin("db-test"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	html := mgr.RenderSlot("test", map[string]any{})
	if html != "Group A" {
		t.Errorf("expected 'Group A', got %q", html)
	}
}

func TestDbApi_NoProvider(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "db-test", `
plugin = { name = "db-test", version = "1.0", description = "db api test" }
function init()
    mah.inject("test", function(ctx)
        local note = mah.db.get_note(1)
        if note then
            return "found"
        end
        return "nil"
    end)
end
`)
	mgr, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()
	// Don't set entity querier — should return nil gracefully

	if err := mgr.EnablePlugin("db-test"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	html := mgr.RenderSlot("test", map[string]any{})
	if html != "nil" {
		t.Errorf("expected 'nil' when no provider set, got %q", html)
	}
}

func TestDbApi_NoProviderQueryNotes(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "db-test", `
plugin = { name = "db-test", version = "1.0", description = "db api test" }
function init()
    mah.inject("test", function(ctx)
        local notes = mah.db.query_notes()
        if notes then
            return "found"
        end
        return "nil"
    end)
end
`)
	mgr, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()

	if err := mgr.EnablePlugin("db-test"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	html := mgr.RenderSlot("test", map[string]any{})
	if html != "nil" {
		t.Errorf("expected 'nil' when no provider set, got %q", html)
	}
}

func TestDbApi_GetResourceData(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "db-test", `
plugin = { name = "db-test", version = "1.0", description = "db api test" }
function init()
    mah.inject("test", function(ctx)
        local data, mime = mah.db.get_resource_data(1)
        if data then
            return mime .. "|" .. tostring(#data > 0)
        end
        return "nil"
    end)
end
`)
	mgr, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()
	mgr.SetEntityQuerier(&mockQuerier{})

	if err := mgr.EnablePlugin("db-test"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	html := mgr.RenderSlot("test", map[string]any{})
	if html != "image/jpeg|true" {
		t.Errorf("expected 'image/jpeg|true', got %q", html)
	}
}

func TestDbApi_GetResourceDataNotFound(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "db-test", `
plugin = { name = "db-test", version = "1.0", description = "db api test" }
function init()
    mah.inject("test", function(ctx)
        local data = mah.db.get_resource_data(999)
        if data then
            return "found"
        end
        return "nil"
    end)
end
`)
	mgr, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()
	mgr.SetEntityQuerier(&mockQuerier{})

	if err := mgr.EnablePlugin("db-test"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	html := mgr.RenderSlot("test", map[string]any{})
	if html != "nil" {
		t.Errorf("expected 'nil', got %q", html)
	}
}

func TestDbApi_GetResourceDataNoProvider(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "db-test", `
plugin = { name = "db-test", version = "1.0", description = "db api test" }
function init()
    mah.inject("test", function(ctx)
        local data = mah.db.get_resource_data(1)
        if data then
            return "found"
        end
        return "nil"
    end)
end
`)
	mgr, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()

	if err := mgr.EnablePlugin("db-test"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	html := mgr.RenderSlot("test", map[string]any{})
	if html != "nil" {
		t.Errorf("expected 'nil' when no provider set, got %q", html)
	}
}

func TestDbApi_CreateResourceFromURL(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "db-test", `
plugin = { name = "db-test", version = "1.0", description = "db api test" }
function init()
    mah.inject("test", function(ctx)
        local res, err = mah.db.create_resource_from_url("https://example.com/image.jpg", {name = "my-image.jpg"})
        if res then
            return tostring(res.id) .. "|" .. res.name
        end
        return "error:" .. (err or "unknown")
    end)
end
`)
	mgr, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()
	mgr.SetEntityQuerier(&mockQuerier{})

	if err := mgr.EnablePlugin("db-test"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	html := mgr.RenderSlot("test", map[string]any{})
	if html != "42|my-image.jpg" {
		t.Errorf("expected '42|my-image.jpg', got %q", html)
	}
}

func TestDbApi_CreateResourceFromData(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "db-test", `
plugin = { name = "db-test", version = "1.0", description = "db api test" }
function init()
    mah.inject("test", function(ctx)
        local res, err = mah.db.create_resource_from_data("aGVsbG8=", {name = "test-upload.bin"})
        if res then
            return tostring(res.id) .. "|" .. res.name
        end
        return "error:" .. (err or "unknown")
    end)
end
`)
	mgr, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()
	mgr.SetEntityQuerier(&mockQuerier{})

	if err := mgr.EnablePlugin("db-test"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	html := mgr.RenderSlot("test", map[string]any{})
	if html != "43|test-upload.bin" {
		t.Errorf("expected '43|test-upload.bin', got %q", html)
	}
}

func TestDbApi_CreateResourceFromURLNoProvider(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "db-test", `
plugin = { name = "db-test", version = "1.0", description = "db api test" }
function init()
    mah.inject("test", function(ctx)
        local res, err = mah.db.create_resource_from_url("https://example.com/img.jpg")
        if res then
            return "ok"
        end
        return "error:" .. (err or "unknown")
    end)
end
`)
	mgr, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()

	if err := mgr.EnablePlugin("db-test"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	html := mgr.RenderSlot("test", map[string]any{})
	if html != "error:database not available" {
		t.Errorf("expected 'error:database not available', got %q", html)
	}
}

func TestDbApi_CreateResourceFromDataInvalidBase64(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "db-test", `
plugin = { name = "db-test", version = "1.0", description = "db api test" }
function init()
    mah.inject("test", function(ctx)
        local res, err = mah.db.create_resource_from_data("!!!not-base64!!!", {name = "bad.bin"})
        if res then
            return "ok"
        end
        if err and err:find("base64") then
            return "base64_error"
        end
        return "error:" .. (err or "unknown")
    end)
end
`)
	mgr, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()
	mgr.SetEntityQuerier(&mockQuerier{})

	if err := mgr.EnablePlugin("db-test"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	html := mgr.RenderSlot("test", map[string]any{})
	if html != "base64_error" {
		t.Errorf("expected 'base64_error', got %q", html)
	}
}
