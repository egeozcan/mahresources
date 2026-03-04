package plugin_system

import (
	"testing"
)

func TestJsonEncode_String(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "json-test", `
plugin = { name = "json-test", version = "1.0", description = "json api test" }
function init()
    mah.inject("test", function(ctx)
        return mah.json.encode("hello")
    end)
end
`)
	mgr, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()

	if err := mgr.EnablePlugin("json-test"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	html := mgr.RenderSlot("test", map[string]any{})
	if html != `"hello"` {
		t.Errorf("expected '\"hello\"', got %q", html)
	}
}

func TestJsonEncode_Table(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "json-test", `
plugin = { name = "json-test", version = "1.0", description = "json api test" }
function init()
    mah.inject("test", function(ctx)
        local result = mah.json.encode({name = "test", value = 42})
        -- Parse it back to verify it's valid JSON
        local decoded = mah.json.decode(result)
        return decoded.name .. "|" .. tostring(decoded.value)
    end)
end
`)
	mgr, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()

	if err := mgr.EnablePlugin("json-test"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	html := mgr.RenderSlot("test", map[string]any{})
	if html != "test|42" {
		t.Errorf("expected 'test|42', got %q", html)
	}
}

func TestJsonEncode_Array(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "json-test", `
plugin = { name = "json-test", version = "1.0", description = "json api test" }
function init()
    mah.inject("test", function(ctx)
        local result = mah.json.encode({"a", "b", "c"})
        return result
    end)
end
`)
	mgr, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()

	if err := mgr.EnablePlugin("json-test"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	html := mgr.RenderSlot("test", map[string]any{})
	if html != `["a","b","c"]` {
		t.Errorf("expected '[\"a\",\"b\",\"c\"]', got %q", html)
	}
}

func TestJsonEncode_NestedArrayInObject(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "json-test", `
plugin = { name = "json-test", version = "1.0", description = "json api test" }
function init()
    mah.inject("test", function(ctx)
        local result = mah.json.encode({urls = {"http://a.com", "http://b.com"}})
        local decoded = mah.json.decode(result)
        return tostring(#decoded.urls) .. "|" .. decoded.urls[1]
    end)
end
`)
	mgr, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()

	if err := mgr.EnablePlugin("json-test"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	html := mgr.RenderSlot("test", map[string]any{})
	if html != "2|http://a.com" {
		t.Errorf("expected '2|http://a.com', got %q", html)
	}
}

func TestJsonDecode_Object(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "json-test", `
plugin = { name = "json-test", version = "1.0", description = "json api test" }
function init()
    mah.inject("test", function(ctx)
        local obj = mah.json.decode('{"image":{"url":"https://example.com/img.png"}}')
        return obj.image.url
    end)
end
`)
	mgr, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()

	if err := mgr.EnablePlugin("json-test"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	html := mgr.RenderSlot("test", map[string]any{})
	if html != "https://example.com/img.png" {
		t.Errorf("expected 'https://example.com/img.png', got %q", html)
	}
}

func TestJsonDecode_Array(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "json-test", `
plugin = { name = "json-test", version = "1.0", description = "json api test" }
function init()
    mah.inject("test", function(ctx)
        local arr = mah.json.decode('[1, 2, 3]')
        return tostring(#arr) .. "|" .. tostring(arr[2])
    end)
end
`)
	mgr, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()

	if err := mgr.EnablePlugin("json-test"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	html := mgr.RenderSlot("test", map[string]any{})
	if html != "3|2" {
		t.Errorf("expected '3|2', got %q", html)
	}
}

func TestJsonDecode_InvalidJson(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "json-test", `
plugin = { name = "json-test", version = "1.0", description = "json api test" }
function init()
    mah.inject("test", function(ctx)
        local val, err = mah.json.decode("not json at all")
        if val then
            return "unexpected"
        end
        return "error:" .. tostring(err ~= nil)
    end)
end
`)
	mgr, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()

	if err := mgr.EnablePlugin("json-test"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	html := mgr.RenderSlot("test", map[string]any{})
	if html != "error:true" {
		t.Errorf("expected 'error:true', got %q", html)
	}
}

func TestJsonEncode_Boolean(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "json-test", `
plugin = { name = "json-test", version = "1.0", description = "json api test" }
function init()
    mah.inject("test", function(ctx)
        local result = mah.json.encode({enabled = true, disabled = false})
        local decoded = mah.json.decode(result)
        return tostring(decoded.enabled) .. "|" .. tostring(decoded.disabled)
    end)
end
`)
	mgr, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()

	if err := mgr.EnablePlugin("json-test"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	html := mgr.RenderSlot("test", map[string]any{})
	if html != "true|false" {
		t.Errorf("expected 'true|false', got %q", html)
	}
}

func TestJsonEncode_Nil(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "json-test", `
plugin = { name = "json-test", version = "1.0", description = "json api test" }
function init()
    mah.inject("test", function(ctx)
        return mah.json.encode(nil)
    end)
end
`)
	mgr, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()

	if err := mgr.EnablePlugin("json-test"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	html := mgr.RenderSlot("test", map[string]any{})
	if html != "null" {
		t.Errorf("expected 'null', got %q", html)
	}
}
