package plugin_system

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	lua "github.com/yuin/gopher-lua"
)

func writePlugin(t *testing.T, dir, name, code string) {
	t.Helper()
	pluginDir := filepath.Join(dir, name)
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.lua"), []byte(code), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestEmptyDirectory(t *testing.T) {
	dir := t.TempDir()
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pm.Close()

	if got := len(pm.Plugins()); got != 0 {
		t.Errorf("expected 0 plugins, got %d", got)
	}
}

func TestSingleValidPlugin(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "hello", `
plugin = { name = "hello", version = "1.0", description = "A hello plugin" }

function init()
    mah.log("info", "hello initialized")
end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pm.Close()

	if err := pm.EnablePlugin("hello"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	plugins := pm.Plugins()
	if len(plugins) != 1 {
		t.Fatalf("expected 1 plugin, got %d", len(plugins))
	}

	p := plugins[0]
	if p.Name != "hello" {
		t.Errorf("expected name 'hello', got %q", p.Name)
	}
	if p.Version != "1.0" {
		t.Errorf("expected version '1.0', got %q", p.Version)
	}
	if p.Description != "A hello plugin" {
		t.Errorf("expected description 'A hello plugin', got %q", p.Description)
	}
	if p.Dir != filepath.Join(dir, "hello") {
		t.Errorf("expected dir %q, got %q", filepath.Join(dir, "hello"), p.Dir)
	}
}

func TestBadSyntaxPluginSkipped(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "bad", `this is not valid lua @@@@`)
	writePlugin(t, dir, "good", `
plugin = { name = "good", version = "2.0", description = "good plugin" }
function init() end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pm.Close()

	// Bad plugin should not appear in discovered list
	discovered := pm.DiscoveredPlugins()
	if len(discovered) != 1 {
		t.Fatalf("expected 1 discovered plugin (bad skipped), got %d", len(discovered))
	}
	if discovered[0].Name != "good" {
		t.Errorf("expected 'good' plugin, got %q", discovered[0].Name)
	}

	if err := pm.EnablePlugin("good"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	plugins := pm.Plugins()
	if len(plugins) != 1 {
		t.Fatalf("expected 1 plugin, got %d", len(plugins))
	}
	if plugins[0].Name != "good" {
		t.Errorf("expected 'good' plugin, got %q", plugins[0].Name)
	}
}

func TestAlphabeticalLoadOrder(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"charlie", "alpha", "bravo"} {
		writePlugin(t, dir, name, `plugin = { name = "`+name+`", version = "1.0", description = "`+name+` plugin" }
function init() end
`)
	}

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pm.Close()

	// Discovery should be in alphabetical order
	discovered := pm.DiscoveredPlugins()
	if len(discovered) != 3 {
		t.Fatalf("expected 3 discovered plugins, got %d", len(discovered))
	}

	expected := []string{"alpha", "bravo", "charlie"}
	for i, want := range expected {
		if discovered[i].Name != want {
			t.Errorf("discovered[%d]: expected %q, got %q", i, want, discovered[i].Name)
		}
	}

	// Enable in alphabetical order and verify
	for _, name := range expected {
		if err := pm.EnablePlugin(name); err != nil {
			t.Fatalf("EnablePlugin(%q): %v", name, err)
		}
	}

	plugins := pm.Plugins()
	if len(plugins) != 3 {
		t.Fatalf("expected 3 plugins, got %d", len(plugins))
	}

	for i, want := range expected {
		if plugins[i].Name != want {
			t.Errorf("plugin[%d]: expected %q, got %q", i, want, plugins[i].Name)
		}
	}
}

func TestHookRegistration(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "hooker", `
plugin = { name = "hooker", version = "1.0", description = "hook test" }

function my_handler(data)
    return data
end

function init()
    mah.on("before_note_create", my_handler)
end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pm.Close()

	if err := pm.EnablePlugin("hooker"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	hooks := pm.GetHooks("before_note_create")
	if len(hooks) != 1 {
		t.Fatalf("expected 1 hook for 'before_note_create', got %d", len(hooks))
	}

	// Verify no hooks for unregistered event
	hooks = pm.GetHooks("nonexistent_event")
	if len(hooks) != 0 {
		t.Errorf("expected 0 hooks for 'nonexistent_event', got %d", len(hooks))
	}
}

func TestInjectionRegistration(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "injector", `
plugin = { name = "injector", version = "1.0", description = "injection test" }

function head_content()
    return "<script>console.log('hi')</script>"
end

function init()
    mah.inject("head", head_content)
end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pm.Close()

	if err := pm.EnablePlugin("injector"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	injections := pm.GetInjections("head")
	if len(injections) != 1 {
		t.Fatalf("expected 1 injection for 'head', got %d", len(injections))
	}

	// Verify no injections for unregistered slot
	injections = pm.GetInjections("footer")
	if len(injections) != 0 {
		t.Errorf("expected 0 injections for 'footer', got %d", len(injections))
	}
}

func TestNonexistentDirectory(t *testing.T) {
	pm, err := NewPluginManager("/tmp/nonexistent_plugin_dir_" + t.Name())
	if err != nil {
		t.Fatalf("expected no error for nonexistent dir, got: %v", err)
	}
	defer pm.Close()

	if got := len(pm.Plugins()); got != 0 {
		t.Errorf("expected 0 plugins, got %d", got)
	}
}

func TestAbortFunction(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "aborter", `
plugin = { name = "aborter", version = "1.0", description = "abort test" }

function before_create(data)
    mah.abort("not allowed")
end

function init()
    mah.on("before_note_create", before_create)
end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pm.Close()

	if err := pm.EnablePlugin("aborter"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	hooks := pm.GetHooks("before_note_create")
	if len(hooks) != 1 {
		t.Fatalf("expected 1 hook, got %d", len(hooks))
	}

	// Call the hook function and verify it raises a PLUGIN_ABORT error
	entry := hooks[0]
	if err := entry.state.CallByParam(lua.P{
		Fn:      entry.fn,
		NRet:    0,
		Protect: true,
	}); err != nil {
		if !strings.Contains(err.Error(), "PLUGIN_ABORT:") {
			t.Errorf("expected PLUGIN_ABORT error, got: %v", err)
		}
	} else {
		t.Error("expected abort to raise an error, but call succeeded")
	}
}

func TestMultipleHooksSameEvent(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "alpha", `
plugin = { name = "alpha", version = "1.0", description = "first" }
function handler(data) return data end
function init()
    mah.on("before_note_create", handler)
end
`)
	writePlugin(t, dir, "bravo", `
plugin = { name = "bravo", version = "1.0", description = "second" }
function handler(data) return data end
function init()
    mah.on("before_note_create", handler)
end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pm.Close()

	if err := pm.EnablePlugin("alpha"); err != nil {
		t.Fatalf("EnablePlugin(alpha): %v", err)
	}
	if err := pm.EnablePlugin("bravo"); err != nil {
		t.Fatalf("EnablePlugin(bravo): %v", err)
	}

	hooks := pm.GetHooks("before_note_create")
	if len(hooks) != 2 {
		t.Fatalf("expected 2 hooks for 'before_note_create', got %d", len(hooks))
	}
}

func TestPluginWithoutInit(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "noinit", `
plugin = { name = "noinit", version = "0.1", description = "no init function" }
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pm.Close()

	if err := pm.EnablePlugin("noinit"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	plugins := pm.Plugins()
	if len(plugins) != 1 {
		t.Fatalf("expected 1 plugin, got %d", len(plugins))
	}
	if plugins[0].Name != "noinit" {
		t.Errorf("expected name 'noinit', got %q", plugins[0].Name)
	}
}

func TestDirectoryWithoutPluginLua(t *testing.T) {
	dir := t.TempDir()
	// Create a subdirectory but no plugin.lua inside it
	if err := os.MkdirAll(filepath.Join(dir, "empty-dir"), 0755); err != nil {
		t.Fatal(err)
	}

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pm.Close()

	if got := len(pm.Plugins()); got != 0 {
		t.Errorf("expected 0 plugins, got %d", got)
	}
}

func TestDiscoverPlugins(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "alpha", `
plugin = {
    name = "alpha",
    version = "1.0",
    description = "first plugin",
    settings = {
        { name = "api_key", type = "password", label = "API Key", required = true },
    },
}
function init()
    mah.on("test_event", function(data) return data end)
end
`)
	writePlugin(t, dir, "bravo", `
plugin = {
    name = "bravo",
    version = "2.0",
    description = "second plugin",
}
function init()
    mah.inject("test_slot", function(ctx) return "<p>hi</p>" end)
end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pm.Close()

	// Verify discovered but NOT active
	discovered := pm.DiscoveredPlugins()
	if len(discovered) != 2 {
		t.Fatalf("expected 2 discovered plugins, got %d", len(discovered))
	}
	if discovered[0].Name != "alpha" {
		t.Errorf("expected discovered[0].Name='alpha', got %q", discovered[0].Name)
	}
	if discovered[0].Version != "1.0" {
		t.Errorf("expected discovered[0].Version='1.0', got %q", discovered[0].Version)
	}
	if len(discovered[0].Settings) != 1 {
		t.Fatalf("expected 1 setting for alpha, got %d", len(discovered[0].Settings))
	}
	if discovered[0].Settings[0].Name != "api_key" {
		t.Errorf("expected setting name 'api_key', got %q", discovered[0].Settings[0].Name)
	}
	if discovered[1].Name != "bravo" {
		t.Errorf("expected discovered[1].Name='bravo', got %q", discovered[1].Name)
	}

	// Verify nothing is active
	if len(pm.Plugins()) != 0 {
		t.Errorf("expected 0 active plugins, got %d", len(pm.Plugins()))
	}
	if len(pm.GetHooks("test_event")) != 0 {
		t.Error("expected no hooks registered before enable")
	}
	if len(pm.GetInjections("test_slot")) != 0 {
		t.Error("expected no injections registered before enable")
	}
	if pm.IsEnabled("alpha") {
		t.Error("expected alpha not to be enabled")
	}
}

func TestEnableDisablePlugin(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "fulltest", `
plugin = { name = "fulltest", version = "1.0", description = "full test" }

function init()
    mah.on("test_event", function(data) return data end)
    mah.inject("test_slot", function(ctx) return "<p>injected</p>" end)
    mah.page("home", function(ctx) return "<h1>Home</h1>" end)
    mah.menu("Home", "home")
end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pm.Close()

	// Enable
	if err := pm.EnablePlugin("fulltest"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	if !pm.IsEnabled("fulltest") {
		t.Error("expected fulltest to be enabled")
	}
	if len(pm.GetHooks("test_event")) != 1 {
		t.Error("expected 1 hook after enable")
	}
	if len(pm.GetInjections("test_slot")) != 1 {
		t.Error("expected 1 injection after enable")
	}
	if !pm.HasPage("fulltest", "home") {
		t.Error("expected page 'home' to be registered")
	}
	if len(pm.GetMenuItems()) != 1 {
		t.Error("expected 1 menu item after enable")
	}

	// Disable
	if err := pm.DisablePlugin("fulltest"); err != nil {
		t.Fatalf("DisablePlugin: %v", err)
	}

	if pm.IsEnabled("fulltest") {
		t.Error("expected fulltest to be disabled")
	}
	if len(pm.Plugins()) != 0 {
		t.Error("expected 0 active plugins after disable")
	}
	if len(pm.GetHooks("test_event")) != 0 {
		t.Error("expected 0 hooks after disable")
	}
	if len(pm.GetInjections("test_slot")) != 0 {
		t.Error("expected 0 injections after disable")
	}
	if pm.HasPage("fulltest", "home") {
		t.Error("expected page 'home' to be removed")
	}
	if len(pm.GetMenuItems()) != 0 {
		t.Error("expected 0 menu items after disable")
	}

	// Re-enable should work
	if err := pm.EnablePlugin("fulltest"); err != nil {
		t.Fatalf("re-EnablePlugin: %v", err)
	}
	if !pm.IsEnabled("fulltest") {
		t.Error("expected fulltest to be enabled after re-enable")
	}
	if len(pm.GetHooks("test_event")) != 1 {
		t.Error("expected 1 hook after re-enable")
	}
}

func TestEnableUnknownPlugin(t *testing.T) {
	dir := t.TempDir()
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pm.Close()

	err = pm.EnablePlugin("nonexistent")
	if err == nil {
		t.Fatal("expected error when enabling nonexistent plugin")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got: %v", err)
	}
}

func TestEnableAlreadyEnabled(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "myplug", `
plugin = { name = "myplug", version = "1.0", description = "test" }
function init() end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pm.Close()

	if err := pm.EnablePlugin("myplug"); err != nil {
		t.Fatalf("first EnablePlugin: %v", err)
	}

	err = pm.EnablePlugin("myplug")
	if err == nil {
		t.Fatal("expected error when enabling already-enabled plugin")
	}
	if !strings.Contains(err.Error(), "already enabled") {
		t.Errorf("expected 'already enabled' error, got: %v", err)
	}
}

func TestDisableNotEnabled(t *testing.T) {
	dir := t.TempDir()
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pm.Close()

	err = pm.DisablePlugin("nonexistent")
	if err == nil {
		t.Fatal("expected error when disabling non-enabled plugin")
	}
	if !strings.Contains(err.Error(), "not enabled") {
		t.Errorf("expected 'not enabled' error, got: %v", err)
	}
}

func TestGetSettingAPI(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "settings-test", `
plugin = {
    name = "settings-test",
    version = "1.0",
    description = "settings access test",
    settings = {
        { name = "api_key", type = "password", label = "API Key", required = true },
    }
}
function init()
    mah.page("show-key", function(ctx)
        local key = mah.get_setting("api_key")
        if key then
            return "key:" .. key
        end
        return "key:nil"
    end)
end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer pm.Close()

	// Set settings before enabling
	pm.SetPluginSettings("settings-test", map[string]any{"api_key": "secret123"})

	if err := pm.EnablePlugin("settings-test"); err != nil {
		t.Fatal(err)
	}

	html, err := pm.HandlePage("settings-test", "show-key", PageContext{
		Path: "/plugins/settings-test/show-key", Method: "GET",
	})
	if err != nil {
		t.Fatal(err)
	}
	if html != "key:secret123" {
		t.Errorf("expected 'key:secret123', got %q", html)
	}
}

func TestGetSettingUnknownKey(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "unknown-key", `
plugin = { name = "unknown-key", version = "1.0", description = "test" }
function init()
    mah.page("test", function(ctx)
        local v = mah.get_setting("nonexistent")
        if v == nil then return "nil" end
        return tostring(v)
    end)
end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer pm.Close()

	if err := pm.EnablePlugin("unknown-key"); err != nil {
		t.Fatal(err)
	}

	html, err := pm.HandlePage("unknown-key", "test", PageContext{
		Path: "/plugins/unknown-key/test", Method: "GET",
	})
	if err != nil {
		t.Fatal(err)
	}
	if html != "nil" {
		t.Errorf("expected 'nil', got %q", html)
	}
}
