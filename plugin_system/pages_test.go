package plugin_system

import (
	"testing"
)

func TestPageRegistration(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "dashboard", `
plugin = { name = "dashboard", version = "1.0", description = "dashboard plugin" }

function init()
    mah.page("home", function(ctx)
        return "<h1>Home</h1>"
    end)
    mah.page("stats", function(ctx)
        return "<h1>Stats</h1>"
    end)
end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pm.Close()

	pages := pm.GetPages()
	if len(pages) != 2 {
		t.Fatalf("expected 2 pages, got %d", len(pages))
	}

	// Verify lookup works
	if !pm.HasPage("dashboard", "home") {
		t.Error("expected HasPage('dashboard', 'home') to be true")
	}
	if !pm.HasPage("dashboard", "stats") {
		t.Error("expected HasPage('dashboard', 'stats') to be true")
	}
	if pm.HasPage("dashboard", "nonexistent") {
		t.Error("expected HasPage('dashboard', 'nonexistent') to be false")
	}
	if pm.HasPage("unknown", "home") {
		t.Error("expected HasPage('unknown', 'home') to be false")
	}
}

func TestMenuRegistration(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "analytics", `
plugin = { name = "analytics", version = "1.0", description = "analytics plugin" }

function init()
    mah.page("dashboard", function(ctx) return "<h1>Dashboard</h1>" end)
    mah.menu("Dashboard", "dashboard")
    mah.menu("Reports", "reports")
end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pm.Close()

	items := pm.GetMenuItems()
	if len(items) != 2 {
		t.Fatalf("expected 2 menu items, got %d", len(items))
	}

	if items[0].Label != "Dashboard" {
		t.Errorf("expected label 'Dashboard', got %q", items[0].Label)
	}
	if items[0].FullPath != "/plugins/analytics/dashboard" {
		t.Errorf("expected path '/plugins/analytics/dashboard', got %q", items[0].FullPath)
	}
	if items[0].PluginName != "analytics" {
		t.Errorf("expected plugin name 'analytics', got %q", items[0].PluginName)
	}
	if items[1].Label != "Reports" {
		t.Errorf("expected label 'Reports', got %q", items[1].Label)
	}
	if items[1].FullPath != "/plugins/analytics/reports" {
		t.Errorf("expected path '/plugins/analytics/reports', got %q", items[1].FullPath)
	}
}
