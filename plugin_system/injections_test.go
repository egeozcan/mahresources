package plugin_system

import (
	"strings"
	"testing"
)

func TestRenderSlot_SinglePlugin(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "banner", `
plugin = { name = "banner", version = "1.0", description = "banner plugin" }

function render_banner(ctx)
    return "<div>Hello " .. ctx.path .. "</div>"
end

function init()
    mah.inject("resource_header", render_banner)
end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pm.Close()

	result := pm.RenderSlot("resource_header", map[string]any{
		"path": "/resource",
	})

	expected := "<div>Hello /resource</div>"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestRenderSlot_MultiplePlugins(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "alpha", `
plugin = { name = "alpha", version = "1.0", description = "first plugin" }

function render(ctx)
    return "<header>Alpha</header>"
end

function init()
    mah.inject("page_top", render)
end
`)
	writePlugin(t, dir, "bravo", `
plugin = { name = "bravo", version = "1.0", description = "second plugin" }

function render(ctx)
    return "<header>Bravo</header>"
end

function init()
    mah.inject("page_top", render)
end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pm.Close()

	result := pm.RenderSlot("page_top", map[string]any{})

	if result != "<header>Alpha</header><header>Bravo</header>" {
		t.Errorf("expected concatenated output in alphabetical order, got %q", result)
	}
}

func TestRenderSlot_ErrorSkipped(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "alpha_bad", `
plugin = { name = "alpha_bad", version = "1.0", description = "broken plugin" }

function render(ctx)
    error("something went wrong")
end

function init()
    mah.inject("sidebar", render)
end
`)
	writePlugin(t, dir, "bravo_good", `
plugin = { name = "bravo_good", version = "1.0", description = "working plugin" }

function render(ctx)
    return "<aside>OK</aside>"
end

function init()
    mah.inject("sidebar", render)
end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pm.Close()

	result := pm.RenderSlot("sidebar", map[string]any{})

	if result != "<aside>OK</aside>" {
		t.Errorf("expected only second plugin output, got %q", result)
	}
}

func TestRenderSlot_EmptySlot(t *testing.T) {
	dir := t.TempDir()
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pm.Close()

	result := pm.RenderSlot("nonexistent_slot", map[string]any{})

	if result != "" {
		t.Errorf("expected empty string for empty slot, got %q", result)
	}
}

func TestRenderSlot_WithEntityContext(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "entity_display", `
plugin = { name = "entity_display", version = "1.0", description = "entity context test" }

function render(ctx)
    return "<span>" .. ctx.entity.name .. "</span>"
end

function init()
    mah.inject("entity_info", render)
end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pm.Close()

	result := pm.RenderSlot("entity_info", map[string]any{
		"entity": map[string]any{
			"name": "My Resource",
		},
	})

	if !strings.Contains(result, "My Resource") {
		t.Errorf("expected output to contain 'My Resource', got %q", result)
	}

	expected := "<span>My Resource</span>"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}
