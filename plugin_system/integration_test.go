package plugin_system

import (
	"testing"
)

func TestEndToEnd_HookAndInjection(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "e2e-plugin", `
plugin = { name = "e2e", version = "1.0", description = "end to end test" }

function init()
    mah.on("before_note_create", function(entity)
        entity.name = entity.name .. " [via plugin]"
        return entity
    end)

    mah.inject("note_detail_after", function(ctx)
        if ctx.entity then
            return "<div class='plugin-injected'>Plugin: " .. ctx.entity.name .. "</div>"
        end
        return ""
    end)
end
`)

	mgr, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()

	if err := mgr.EnablePlugin("e2e"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	// Test hook modifies data
	data := map[string]any{"name": "Test Note", "description": "desc"}
	result, err := mgr.RunBeforeHooks("before_note_create", data)
	if err != nil {
		t.Fatal(err)
	}
	if result["name"] != "Test Note [via plugin]" {
		t.Errorf("hook didn't modify name: got %q", result["name"])
	}

	// Test injection renders with entity context
	html := mgr.RenderSlot("note_detail_after", map[string]any{
		"entity": map[string]any{"name": "Test Note [via plugin]"},
	})
	expected := "<div class='plugin-injected'>Plugin: Test Note [via plugin]</div>"
	if html != expected {
		t.Errorf("unexpected injection output: got %q, want %q", html, expected)
	}
}
