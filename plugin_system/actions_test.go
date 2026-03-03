package plugin_system

import (
	"testing"
)

func TestActionRegistration_BasicFields(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "resizer", `
plugin = { name = "resizer", version = "1.0", description = "image resizer" }

function do_resize(ctx)
    return { success = true }
end

function init()
    mah.action({
        id = "resize",
        label = "Resize Image",
        description = "Resize an image to specified dimensions",
        icon = "crop",
        entity = "resource",
        placement = { "detail", "bulk" },
        filters = {
            content_types = { "image/jpeg", "image/png" },
        },
        params = {
            { name = "width", type = "number", label = "Width", required = true, min = 1, max = 10000 },
            { name = "height", type = "number", label = "Height", required = false, default = 0, min = 1, max = 10000 },
        },
        async = true,
        confirm = "Are you sure you want to resize?",
        bulk_max = 50,
        handler = do_resize,
    })
end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pm.Close()

	if err := pm.EnablePlugin("resizer"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	// Get all actions for resource entity
	actions := pm.GetActions("resource", nil)
	if len(actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(actions))
	}

	a := actions[0]

	// Basic fields
	if a.PluginName != "resizer" {
		t.Errorf("expected PluginName 'resizer', got %q", a.PluginName)
	}
	if a.ID != "resize" {
		t.Errorf("expected ID 'resize', got %q", a.ID)
	}
	if a.Label != "Resize Image" {
		t.Errorf("expected Label 'Resize Image', got %q", a.Label)
	}
	if a.Description != "Resize an image to specified dimensions" {
		t.Errorf("expected Description, got %q", a.Description)
	}
	if a.Icon != "crop" {
		t.Errorf("expected Icon 'crop', got %q", a.Icon)
	}
	if a.Entity != "resource" {
		t.Errorf("expected Entity 'resource', got %q", a.Entity)
	}
	if a.Async != true {
		t.Errorf("expected Async true, got false")
	}
	if a.Confirm != "Are you sure you want to resize?" {
		t.Errorf("expected Confirm message, got %q", a.Confirm)
	}
	if a.BulkMax != 50 {
		t.Errorf("expected BulkMax 50, got %d", a.BulkMax)
	}
	if a.Handler == nil {
		t.Error("expected Handler to be set")
	}

	// Placement
	if len(a.Placement) != 2 {
		t.Fatalf("expected 2 placements, got %d", len(a.Placement))
	}
	if a.Placement[0] != "detail" || a.Placement[1] != "bulk" {
		t.Errorf("expected placements [detail, bulk], got %v", a.Placement)
	}

	// Filters
	if len(a.Filters.ContentTypes) != 2 {
		t.Fatalf("expected 2 content types, got %d", len(a.Filters.ContentTypes))
	}
	if a.Filters.ContentTypes[0] != "image/jpeg" || a.Filters.ContentTypes[1] != "image/png" {
		t.Errorf("expected content types [image/jpeg, image/png], got %v", a.Filters.ContentTypes)
	}

	// Params
	if len(a.Params) != 2 {
		t.Fatalf("expected 2 params, got %d", len(a.Params))
	}

	p0 := a.Params[0]
	if p0.Name != "width" {
		t.Errorf("expected param[0].Name 'width', got %q", p0.Name)
	}
	if p0.Type != "number" {
		t.Errorf("expected param[0].Type 'number', got %q", p0.Type)
	}
	if p0.Label != "Width" {
		t.Errorf("expected param[0].Label 'Width', got %q", p0.Label)
	}
	if p0.Required != true {
		t.Errorf("expected param[0].Required true")
	}
	if p0.Min == nil || *p0.Min != 1 {
		t.Errorf("expected param[0].Min 1, got %v", p0.Min)
	}
	if p0.Max == nil || *p0.Max != 10000 {
		t.Errorf("expected param[0].Max 10000, got %v", p0.Max)
	}

	p1 := a.Params[1]
	if p1.Name != "height" {
		t.Errorf("expected param[1].Name 'height', got %q", p1.Name)
	}
	if p1.Required != false {
		t.Errorf("expected param[1].Required false")
	}

	// Filter matching: should match image/jpeg resource
	actionsFiltered := pm.GetActions("resource", map[string]any{
		"content_type": "image/jpeg",
	})
	if len(actionsFiltered) != 1 {
		t.Errorf("expected 1 action for image/jpeg, got %d", len(actionsFiltered))
	}

	// Filter matching: should NOT match text/plain resource
	actionsNoMatch := pm.GetActions("resource", map[string]any{
		"content_type": "text/plain",
	})
	if len(actionsNoMatch) != 0 {
		t.Errorf("expected 0 actions for text/plain, got %d", len(actionsNoMatch))
	}

	// Wrong entity type: should return 0
	actionsNote := pm.GetActions("note", nil)
	if len(actionsNote) != 0 {
		t.Errorf("expected 0 actions for note entity, got %d", len(actionsNote))
	}

	// Placement filtering
	detailActions := pm.GetActionsForPlacement("resource", "detail", nil)
	if len(detailActions) != 1 {
		t.Errorf("expected 1 action for detail placement, got %d", len(detailActions))
	}

	listActions := pm.GetActionsForPlacement("resource", "list", nil)
	if len(listActions) != 0 {
		t.Errorf("expected 0 actions for list placement, got %d", len(listActions))
	}
}
