package plugin_system

import (
	"context"
	"sort"
	"strings"
	"testing"
)

// Display types could only be looked up by exact name, so nothing could
// enumerate them: a schema author had to hand-type the "x-display" string from
// a README, and a typo degraded silently.
func TestGetDisplayTypes(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "renderers", `
plugin = { name = "renderers", version = "1.0", description = "display types" }
function init()
    mah.display_type({
        type = "stars",
        label = "Star Rating",
        render = function(ctx) return "<span>stars</span>" end,
    })
    mah.display_type({
        type = "geo",
        label = "Map Pin",
        render = function(ctx) return "<span>geo</span>" end,
    })
end
`)
	mgr, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()
	if err := mgr.EnablePlugin("renderers"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	types := mgr.GetDisplayTypes()
	if len(types) != 2 {
		t.Fatalf("expected 2 display types, got %d", len(types))
	}

	var names, labels []string
	for _, dt := range types {
		names = append(names, dt.TypeName)
		labels = append(labels, dt.Label)
		if dt.PluginName != "renderers" {
			t.Errorf("expected plugin name 'renderers', got %q", dt.PluginName)
		}
	}
	sort.Strings(names)
	sort.Strings(labels)

	wantNames := "plugin:renderers:geo,plugin:renderers:stars"
	if got := strings.Join(names, ","); got != wantNames {
		t.Errorf("type names = %q, want %q", got, wantNames)
	}
	// The label every plugin is required to declare was read by no code outside
	// this package; the catalogue is what makes it reachable.
	wantLabels := "Map Pin,Star Rating"
	if got := strings.Join(labels, ","); got != wantLabels {
		t.Errorf("labels = %q, want %q", got, wantLabels)
	}
}

func TestGetDisplayTypesEmpty(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "quiet", `
plugin = { name = "quiet", version = "1.0", description = "no display types" }
function init() end
`)
	mgr, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()
	if err := mgr.EnablePlugin("quiet"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}
	if types := mgr.GetDisplayTypes(); len(types) != 0 {
		t.Errorf("expected no display types, got %d", len(types))
	}
}

// A renderer receives what it is rendering, not just the field's value.
func TestRenderDisplay_ReceivesEntityIdentity(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "renderers", `
plugin = { name = "renderers", version = "1.0", description = "display types" }
function init()
    mah.display_type({
        type = "identity",
        label = "Identity",
        render = function(ctx)
            return ctx.entity_type .. "#" .. string.format("%d", ctx.entity_id) .. ":" .. ctx.field_path
        end,
    })
end
`)
	mgr, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()
	if err := mgr.EnablePlugin("renderers"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	html, err := mgr.RenderDisplay(context.Background(), "renderers", "plugin:renderers:identity", DisplayRenderContext{
		Value:      "x",
		FieldPath:  "location.lat",
		EntityType: "resource",
		EntityID:   42,
	})
	if err != nil {
		t.Fatalf("RenderDisplay: %v", err)
	}
	if html != "resource#42:location.lat" {
		t.Errorf("got %q, want %q", html, "resource#42:location.lat")
	}
}

// The schema-editor preview has no stored entity, and must say so rather than
// hand the renderer an id pointing at something else.
func TestRenderDisplay_UnboundPreviewHasNoEntity(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "renderers", `
plugin = { name = "renderers", version = "1.0", description = "display types" }
function init()
    mah.display_type({
        type = "identity",
        label = "Identity",
        render = function(ctx)
            if ctx.entity_type == "" and ctx.entity_id == 0 then return "unbound" end
            return "bound"
        end,
    })
end
`)
	mgr, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()
	if err := mgr.EnablePlugin("renderers"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	html, err := mgr.RenderDisplay(context.Background(), "renderers", "plugin:renderers:identity", DisplayRenderContext{Value: "x"})
	if err != nil {
		t.Fatalf("RenderDisplay: %v", err)
	}
	if html != "unbound" {
		t.Errorf("got %q, want %q", html, "unbound")
	}
}
