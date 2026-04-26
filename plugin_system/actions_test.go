package plugin_system

import (
	"strings"
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

func TestActionRegistration_ShowWhen(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "conditional", `
plugin = { name = "conditional", version = "1.0", description = "conditional params" }

function handler(ctx) return { success = true } end

function init()
    mah.action({
        id = "process",
        label = "Process",
        entity = "resource",
        params = {
            { name = "model", type = "select", label = "Model", default = "a", options = {"a", "b"} },
            { name = "extra_a", type = "text", label = "Extra A", show_when = { model = "a" } },
            { name = "extra_b", type = "number", label = "Extra B",
              show_when = { model = "b", advanced = true } },
        },
        handler = handler,
    })
end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("NewPluginManager: %v", err)
	}
	defer pm.Close()
	if err := pm.EnablePlugin("conditional"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	actions := pm.GetActions("resource", nil)
	if len(actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(actions))
	}
	a := actions[0]
	if len(a.Params) != 3 {
		t.Fatalf("expected 3 params, got %d", len(a.Params))
	}

	if a.Params[0].ShowWhen != nil {
		t.Errorf("param[0] should have no ShowWhen, got %v", a.Params[0].ShowWhen)
	}

	pA := a.Params[1]
	if pA.ShowWhen == nil {
		t.Fatalf("param[1] (extra_a) should have ShowWhen set")
	}
	if pA.ShowWhen["model"] != "a" {
		t.Errorf("param[1].ShowWhen[model] = %v, want \"a\"", pA.ShowWhen["model"])
	}

	pB := a.Params[2]
	if pB.ShowWhen == nil {
		t.Fatalf("param[2] (extra_b) should have ShowWhen set")
	}
	if pB.ShowWhen["model"] != "b" {
		t.Errorf("param[2].ShowWhen[model] = %v, want \"b\"", pB.ShowWhen["model"])
	}
	if pB.ShowWhen["advanced"] != true {
		t.Errorf("param[2].ShowWhen[advanced] = %v, want true", pB.ShowWhen["advanced"])
	}
}

func TestActionRegistration_InfoTypeAndDescription(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "info-plugin", `
plugin = { name = "info-plugin", version = "1.0", description = "info type" }

function handler(ctx) return { success = true } end

function init()
    mah.action({
        id = "process",
        label = "Process",
        entity = "resource",
        params = {
            { name = "model", type = "select", label = "Model", default = "a", options = {"a", "b"} },
            { name = "model_info_a", type = "info", label = "About A",
              description = "Mode A is the simple path.",
              show_when = { model = "a" } },
            { name = "amount", type = "number", label = "Amount", default = 1,
              description = "Pick a value between 1 and 10." },
        },
        handler = handler,
    })
end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("NewPluginManager: %v", err)
	}
	defer pm.Close()
	if err := pm.EnablePlugin("info-plugin"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	actions := pm.GetActions("resource", nil)
	if len(actions) != 1 || len(actions[0].Params) != 3 {
		t.Fatalf("unexpected action shape: %+v", actions)
	}
	params := actions[0].Params

	if params[1].Type != "info" {
		t.Errorf("params[1].Type = %q, want \"info\"", params[1].Type)
	}
	if params[1].Description != "Mode A is the simple path." {
		t.Errorf("params[1].Description = %q, want help body", params[1].Description)
	}
	if params[2].Description != "Pick a value between 1 and 10." {
		t.Errorf("params[2].Description = %q, want help body", params[2].Description)
	}
}

func TestGetActions_FiltersByContentType(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "ct-filter", `
plugin = { name = "ct-filter", version = "1.0", description = "content type filter test" }

function handler(ctx) return { success = true } end

function init()
    mah.action({
        id = "image-only",
        label = "Image Only Action",
        entity = "resource",
        filters = {
            content_types = { "image/png", "image/jpeg" },
        },
        handler = handler,
    })
    mah.action({
        id = "any-resource",
        label = "Any Resource Action",
        entity = "resource",
        handler = handler,
    })
end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pm.Close()

	if err := pm.EnablePlugin("ct-filter"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	// image/png matches both (filtered action matches, unfiltered always matches)
	actions := pm.GetActions("resource", map[string]any{"content_type": "image/png"})
	if len(actions) != 2 {
		t.Errorf("expected 2 actions for image/png, got %d", len(actions))
	}

	// application/pdf matches only the unfiltered action
	actions = pm.GetActions("resource", map[string]any{"content_type": "application/pdf"})
	if len(actions) != 1 {
		t.Errorf("expected 1 action for application/pdf, got %d", len(actions))
	}
	if len(actions) == 1 && actions[0].ID != "any-resource" {
		t.Errorf("expected 'any-resource' action, got %q", actions[0].ID)
	}

	// nil entityData skips filtering, both actions returned
	actions = pm.GetActions("resource", nil)
	if len(actions) != 2 {
		t.Errorf("expected 2 actions for nil entityData, got %d", len(actions))
	}

	// wrong entity type returns 0
	actions = pm.GetActions("note", nil)
	if len(actions) != 0 {
		t.Errorf("expected 0 actions for note entity, got %d", len(actions))
	}
}

func TestGetActions_FiltersByCategoryID(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "cat-filter", `
plugin = { name = "cat-filter", version = "1.0", description = "category filter test" }

function handler(ctx) return { success = true } end

function init()
    mah.action({
        id = "cat-action",
        label = "Category Action",
        entity = "group",
        filters = {
            category_ids = { 1, 2 },
        },
        handler = handler,
    })
end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pm.Close()

	if err := pm.EnablePlugin("cat-filter"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	// category_id 1 matches
	actions := pm.GetActions("group", map[string]any{"category_id": uint(1)})
	if len(actions) != 1 {
		t.Errorf("expected 1 action for category_id=1, got %d", len(actions))
	}

	// category_id 99 does not match
	actions = pm.GetActions("group", map[string]any{"category_id": uint(99)})
	if len(actions) != 0 {
		t.Errorf("expected 0 actions for category_id=99, got %d", len(actions))
	}

	// nil entityData skips filtering, action returned
	actions = pm.GetActions("group", nil)
	if len(actions) != 1 {
		t.Errorf("expected 1 action for nil entityData, got %d", len(actions))
	}
}

func TestGetActions_FiltersByNoteTypeID(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "nt-filter", `
plugin = { name = "nt-filter", version = "1.0", description = "note type filter test" }

function handler(ctx) return { success = true } end

function init()
    mah.action({
        id = "nt-action",
        label = "Note Type Action",
        entity = "note",
        filters = {
            note_type_ids = { 3 },
        },
        handler = handler,
    })
end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pm.Close()

	if err := pm.EnablePlugin("nt-filter"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	// note_type_id 3 matches
	actions := pm.GetActions("note", map[string]any{"note_type_id": uint(3)})
	if len(actions) != 1 {
		t.Errorf("expected 1 action for note_type_id=3, got %d", len(actions))
	}

	// note_type_id 99 does not match
	actions = pm.GetActions("note", map[string]any{"note_type_id": uint(99)})
	if len(actions) != 0 {
		t.Errorf("expected 0 actions for note_type_id=99, got %d", len(actions))
	}
}

func TestGetActionsForPlacement_MultipleActions(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "place-multi", `
plugin = { name = "place-multi", version = "1.0", description = "placement multi test" }

function handler(ctx) return { success = true } end

function init()
    mah.action({
        id = "detail-only",
        label = "Detail Only",
        entity = "resource",
        placement = { "detail" },
        handler = handler,
    })
    mah.action({
        id = "multi-place",
        label = "Multi Place",
        entity = "resource",
        placement = { "detail", "card", "bulk" },
        handler = handler,
    })
end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pm.Close()

	if err := pm.EnablePlugin("place-multi"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	// detail: both actions
	actions := pm.GetActionsForPlacement("resource", "detail", nil)
	if len(actions) != 2 {
		t.Errorf("expected 2 actions for detail placement, got %d", len(actions))
	}

	// card: only multi-place
	actions = pm.GetActionsForPlacement("resource", "card", nil)
	if len(actions) != 1 {
		t.Errorf("expected 1 action for card placement, got %d", len(actions))
	}
	if len(actions) == 1 && actions[0].ID != "multi-place" {
		t.Errorf("expected 'multi-place' for card, got %q", actions[0].ID)
	}

	// bulk: only multi-place
	actions = pm.GetActionsForPlacement("resource", "bulk", nil)
	if len(actions) != 1 {
		t.Errorf("expected 1 action for bulk placement, got %d", len(actions))
	}
	if len(actions) == 1 && actions[0].ID != "multi-place" {
		t.Errorf("expected 'multi-place' for bulk, got %q", actions[0].ID)
	}

	// list: no actions
	actions = pm.GetActionsForPlacement("resource", "list", nil)
	if len(actions) != 0 {
		t.Errorf("expected 0 actions for list placement, got %d", len(actions))
	}
}

func TestActionRegistration_CleanedUpOnDisable(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "cleanup-test", `
plugin = { name = "cleanup-test", version = "1.0", description = "cleanup test" }

function handler(ctx) return { success = true } end

function init()
    mah.action({
        id = "cleanup-action",
        label = "Cleanup Action",
        entity = "resource",
        handler = handler,
    })
end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pm.Close()

	// Enable and verify action exists
	if err := pm.EnablePlugin("cleanup-test"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	actions := pm.GetActions("resource", nil)
	if len(actions) != 1 {
		t.Fatalf("expected 1 action after enable, got %d", len(actions))
	}

	// Disable and verify action is removed
	if err := pm.DisablePlugin("cleanup-test"); err != nil {
		t.Fatalf("DisablePlugin: %v", err)
	}

	actions = pm.GetActions("resource", nil)
	if len(actions) != 0 {
		t.Errorf("expected 0 actions after disable, got %d", len(actions))
	}
}

func TestActionRegistration_EntityRefParam_BasicFields(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "ref-plugin", `
plugin = { name = "ref-plugin", version = "1.0", description = "ref test" }

function init()
    mah.action({
        id = "ref-action",
        label = "Ref Action",
        entity = "resource",
        params = {
            { name = "extras", type = "entity_ref", entity = "resource", multi = true,
              label = "Extras", min = 0, max = 5, default = "trigger",
              filters = { content_types = {"image/png"} } },
        },
        handler = function(ctx) return { success = true } end,
    })
end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("NewPluginManager: %v", err)
	}
	defer pm.Close()
	if err := pm.EnablePlugin("ref-plugin"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	actions := pm.GetActions("resource", nil)
	if len(actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(actions))
	}
	p := actions[0].Params[0]
	if p.Type != "entity_ref" {
		t.Errorf("Type=%q", p.Type)
	}
	if p.Entity != "resource" {
		t.Errorf("Entity=%q", p.Entity)
	}
	if !p.Multi {
		t.Errorf("Multi=false")
	}
	if p.Default != "trigger" {
		t.Errorf("Default=%v", p.Default)
	}
	if p.Filters == nil {
		t.Fatalf("Filters nil")
	}
	if len(p.Filters.ContentTypes) != 1 || p.Filters.ContentTypes[0] != "image/png" {
		t.Errorf("Filters.ContentTypes=%v", p.Filters.ContentTypes)
	}
}

func TestActionRegistration_EntityRefParam_RejectsMissingEntity(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "bad-plugin", `
plugin = { name = "bad-plugin", version = "1.0", description = "" }
function init()
    mah.action({
        id = "x", label = "X", entity = "resource",
        params = { { name = "extras", type = "entity_ref", label = "X" } },
        handler = function(ctx) return { success = true } end,
    })
end
`)
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("NewPluginManager: %v", err)
	}
	defer pm.Close()
	err = pm.EnablePlugin("bad-plugin")
	if err == nil || !strings.Contains(err.Error(), "requires 'entity' field") {
		t.Errorf("expected entity-required error, got: %v", err)
	}
}

func TestActionRegistration_EntityRefParam_RejectsBadEntity(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "bad-plugin", `
plugin = { name = "bad-plugin", version = "1.0", description = "" }
function init()
    mah.action({
        id = "x", label = "X", entity = "resource",
        params = { { name = "extras", type = "entity_ref", entity = "tag", label = "X" } },
        handler = function(ctx) return { success = true } end,
    })
end
`)
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("NewPluginManager: %v", err)
	}
	defer pm.Close()
	err = pm.EnablePlugin("bad-plugin")
	if err == nil || !strings.Contains(err.Error(), "must be 'resource', 'note', or 'group'") {
		t.Errorf("expected entity-value error, got: %v", err)
	}
}

func TestActionRegistration_EntityRefParam_RejectsBothWithSingle(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "bad-plugin", `
plugin = { name = "bad-plugin", version = "1.0", description = "" }
function init()
    mah.action({
        id = "x", label = "X", entity = "resource",
        params = { { name = "extras", type = "entity_ref", entity = "resource", default = "both", label = "X" } },
        handler = function(ctx) return { success = true } end,
    })
end
`)
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("NewPluginManager: %v", err)
	}
	defer pm.Close()
	err = pm.EnablePlugin("bad-plugin")
	if err == nil || !strings.Contains(err.Error(), "default 'both' requires multi=true") {
		t.Errorf("expected both-requires-multi error, got: %v", err)
	}
}

func TestActionRegistration_EntityRefParam_RejectsNonStringDefault(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "bad-plugin", `
plugin = { name = "bad-plugin", version = "1.0", description = "" }
function init()
    mah.action({
        id = "x", label = "X", entity = "resource",
        params = { { name = "extras", type = "entity_ref", entity = "resource", default = 42, label = "X" } },
        handler = function(ctx) return { success = true } end,
    })
end
`)
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("NewPluginManager: %v", err)
	}
	defer pm.Close()
	err = pm.EnablePlugin("bad-plugin")
	if err == nil || !strings.Contains(err.Error(), "default must be a string for entity_ref") {
		t.Errorf("expected non-string default error, got: %v", err)
	}
}

func TestActionRegistration_RejectsRequiredWithShowWhen(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "bad-plugin", `
plugin = { name = "bad-plugin", version = "1.0", description = "" }
function init()
    mah.action({
        id = "x", label = "X", entity = "resource",
        params = {
            { name = "model", type = "select", label = "Model", options = {"a","b"}, default = "a" },
            { name = "extra", type = "text", label = "Extra", required = true,
              show_when = { model = "b" } },
        },
        handler = function(ctx) return { success = true } end,
    })
end
`)
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("NewPluginManager: %v", err)
	}
	defer pm.Close()
	err = pm.EnablePlugin("bad-plugin")
	if err == nil || !strings.Contains(err.Error(), "required") || !strings.Contains(err.Error(), "show_when") {
		t.Errorf("expected required+show_when rejection, got: %v", err)
	}
}

func TestActionRegistration_Validation(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "bad-action", `
plugin = { name = "bad-action", version = "1.0", description = "missing handler test" }

function init()
    mah.action({
        id = "no-handler",
        label = "No Handler",
        entity = "resource",
    })
end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pm.Close()

	err = pm.EnablePlugin("bad-action")
	if err == nil {
		t.Fatal("expected error when enabling plugin with action missing handler")
	}
	if !strings.Contains(err.Error(), "handler") {
		t.Errorf("expected error about missing handler, got: %v", err)
	}
}
