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

	plugins := pm.Plugins()
	if len(plugins) != 1 {
		t.Fatalf("expected 1 plugin (bad skipped), got %d", len(plugins))
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

	plugins := pm.Plugins()
	if len(plugins) != 3 {
		t.Fatalf("expected 3 plugins, got %d", len(plugins))
	}

	expected := []string{"alpha", "bravo", "charlie"}
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
