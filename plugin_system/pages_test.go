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

	if err := pm.EnablePlugin("dashboard"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

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

func TestHandlePage_Success(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "myapp", `
plugin = { name = "myapp", version = "1.0", description = "test app" }

function init()
    mah.page("hello", function(ctx)
        return "<h1>Hello from " .. ctx.method .. " " .. ctx.path .. "</h1>"
    end)
end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pm.Close()

	if err := pm.EnablePlugin("myapp"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	html, err := pm.HandlePage("myapp", "hello", PageContext{
		Path:   "/plugins/myapp/hello",
		Method: "GET",
		Query:  map[string]any{},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "<h1>Hello from GET /plugins/myapp/hello</h1>"
	if html != expected {
		t.Errorf("expected %q, got %q", expected, html)
	}
}

func TestHandlePage_WithQueryParams(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "search", `
plugin = { name = "search", version = "1.0", description = "search" }

function init()
    mah.page("results", function(ctx)
        return "<p>Query: " .. (ctx.query.q or "none") .. "</p>"
    end)
end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pm.Close()

	if err := pm.EnablePlugin("search"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	html, err := pm.HandlePage("search", "results", PageContext{
		Path:   "/plugins/search/results",
		Method: "GET",
		Query:  map[string]any{"q": "test"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if html != "<p>Query: test</p>" {
		t.Errorf("expected '<p>Query: test</p>', got %q", html)
	}
}

func TestHandlePage_NotFound(t *testing.T) {
	dir := t.TempDir()
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pm.Close()

	_, err = pm.HandlePage("nonexistent", "page", PageContext{})
	if err == nil {
		t.Fatal("expected error for nonexistent plugin page")
	}
}

func TestHandlePage_LuaError(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "broken", `
plugin = { name = "broken", version = "1.0", description = "broken" }

function init()
    mah.page("crash", function(ctx)
        error("intentional crash")
    end)
end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pm.Close()

	if err := pm.EnablePlugin("broken"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	_, err = pm.HandlePage("broken", "crash", PageContext{})
	if err == nil {
		t.Fatal("expected error from crashing handler")
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

	if err := pm.EnablePlugin("analytics"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

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
