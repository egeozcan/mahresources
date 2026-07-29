# Plugin Activation & Settings Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a plugin management system where plugins are disabled by default, can be enabled/disabled at runtime, and can declare settings (API keys, toggles, etc.) configurable from a dedicated management page.

**Architecture:** New `PluginState` GORM model persists enabled/disabled status and JSON settings per plugin. `PluginManager` gains discovery-without-init, runtime enable/disable, and settings APIs. A new `/plugins/manage` page and JSON API endpoints let users toggle plugins and configure settings. Lua plugins declare settings in `plugin.settings` table and read them at runtime via `mah.get_setting()`.

**Tech Stack:** Go, GORM, gopher-lua, Pongo2 templates, Playwright (E2E tests)

---

### Task 1: PluginState Model

**Files:**
- Create: `models/plugin_state_model.go`
- Modify: `main.go:233-253` (AutoMigrate list)

**Step 1: Write the model**

Create `models/plugin_state_model.go`:

```go
package models

import "time"

// PluginState persists a plugin's enabled/disabled status and settings.
type PluginState struct {
	ID           uint      `gorm:"primarykey"`
	CreatedAt    time.Time `gorm:"index"`
	UpdatedAt    time.Time `gorm:"index"`
	PluginName   string    `gorm:"uniqueIndex:idx_plugin_name"`
	Enabled      bool      `gorm:"default:false"`
	SettingsJSON string    `gorm:"type:text"`
}
```

**Step 2: Add to AutoMigrate**

In `main.go`, find the `db.AutoMigrate(` call (line 233). Add `&models.PluginState{}` after `&models.LogEntry{}`:

```go
	if err := db.AutoMigrate(
		// ... existing models ...
		&models.LogEntry{},
		&models.PluginState{},
	); err != nil {
```

**Step 3: Run tests to verify compilation**

Run: `go build --tags 'json1 fts5'`
Expected: Compiles successfully

**Step 4: Run existing Go tests**

Run: `go test ./... --tags 'json1 fts5'`
Expected: All existing tests pass

**Step 5: Commit**

```bash
git add models/plugin_state_model.go main.go
git commit -m "feat(plugins): add PluginState model for plugin activation/settings"
```

---

### Task 2: Settings Types & Validation

**Files:**
- Create: `plugin_system/settings.go`
- Test: `plugin_system/settings_test.go`

**Step 1: Write the failing tests**

Create `plugin_system/settings_test.go`:

```go
package plugin_system

import "testing"

func TestParseSettingsDeclaration(t *testing.T) {
	lua := `plugin = {
		name = "test",
		version = "1.0",
		description = "test plugin",
		settings = {
			{ name = "api_key", type = "password", label = "API Key", required = true },
			{ name = "city", type = "string", label = "City", default = "Berlin" },
			{ name = "units", type = "select", label = "Units", options = {"metric", "imperial"}, default = "metric" },
			{ name = "enabled", type = "boolean", label = "Enabled", default = true },
			{ name = "count", type = "number", label = "Count", default = 42 },
		}
	}`
	defs, err := parseSettingsFromLua(lua)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(defs) != 5 {
		t.Fatalf("expected 5 settings, got %d", len(defs))
	}

	// Verify password type
	if defs[0].Name != "api_key" || defs[0].Type != "password" || !defs[0].Required {
		t.Errorf("unexpected api_key: %+v", defs[0])
	}
	// Verify select options
	if defs[2].Type != "select" || len(defs[2].Options) != 2 {
		t.Errorf("unexpected units: %+v", defs[2])
	}
	// Verify boolean default
	if defs[3].DefaultValue != true {
		t.Errorf("expected boolean default true, got %v", defs[3].DefaultValue)
	}
	// Verify number default
	if defs[4].DefaultValue != float64(42) {
		t.Errorf("expected number default 42, got %v", defs[4].DefaultValue)
	}
}

func TestParseSettingsNoSettings(t *testing.T) {
	lua := `plugin = { name = "simple", version = "1.0", description = "no settings" }`
	defs, err := parseSettingsFromLua(lua)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(defs) != 0 {
		t.Errorf("expected 0 settings, got %d", len(defs))
	}
}

func TestValidateSettings_AllTypes(t *testing.T) {
	defs := []SettingDefinition{
		{Name: "key", Type: "password", Label: "Key", Required: true},
		{Name: "name", Type: "string", Label: "Name"},
		{Name: "on", Type: "boolean", Label: "On"},
		{Name: "num", Type: "number", Label: "Num"},
		{Name: "mode", Type: "select", Label: "Mode", Options: []string{"a", "b"}},
	}

	// Valid settings
	valid := map[string]any{"key": "secret", "name": "test", "on": true, "num": float64(3.14), "mode": "a"}
	if errs := ValidateSettings(defs, valid); len(errs) != 0 {
		t.Errorf("expected no errors, got %v", errs)
	}

	// Missing required
	missing := map[string]any{"name": "test"}
	if errs := ValidateSettings(defs, missing); len(errs) == 0 {
		t.Error("expected error for missing required field")
	}

	// Invalid select
	badSelect := map[string]any{"key": "s", "mode": "c"}
	errs := ValidateSettings(defs, badSelect)
	found := false
	for _, e := range errs {
		if e.Field == "mode" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected validation error for mode, got %v", errs)
	}

	// Invalid boolean
	badBool := map[string]any{"key": "s", "on": "notbool"}
	errs = ValidateSettings(defs, badBool)
	found = false
	for _, e := range errs {
		if e.Field == "on" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected validation error for on, got %v", errs)
	}

	// Invalid number
	badNum := map[string]any{"key": "s", "num": "notnum"}
	errs = ValidateSettings(defs, badNum)
	found = false
	for _, e := range errs {
		if e.Field == "num" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected validation error for num, got %v", errs)
	}
}

func TestCheckRequiredSettings(t *testing.T) {
	defs := []SettingDefinition{
		{Name: "key", Type: "password", Label: "Key", Required: true},
		{Name: "opt", Type: "string", Label: "Opt"},
	}

	// Missing required
	missing := CheckRequiredSettings(defs, map[string]any{})
	if len(missing) != 1 || missing[0] != "Key" {
		t.Errorf("expected [Key], got %v", missing)
	}

	// All present
	present := CheckRequiredSettings(defs, map[string]any{"key": "value"})
	if len(present) != 0 {
		t.Errorf("expected no missing, got %v", present)
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./plugin_system/ --tags 'json1 fts5' -run TestParseSettings -v`
Expected: FAIL (functions don't exist)

**Step 3: Write the implementation**

Create `plugin_system/settings.go`:

```go
package plugin_system

import (
	"fmt"

	lua "github.com/yuin/gopher-lua"
)

// SettingDefinition describes a single plugin setting.
type SettingDefinition struct {
	Name         string   `json:"name"`
	Type         string   `json:"type"` // string, password, boolean, number, select
	Label        string   `json:"label"`
	Required     bool     `json:"required,omitempty"`
	DefaultValue any      `json:"default,omitempty"`
	Options      []string `json:"options,omitempty"` // for type=select
}

// ValidationError describes a single setting validation failure.
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// parseSettingsFromLua executes a Lua script string and extracts plugin.settings.
// This is used during discovery to parse settings without calling init().
func parseSettingsFromLua(script string) ([]SettingDefinition, error) {
	L := lua.NewState(lua.Options{SkipOpenLibs: true})
	defer L.Close()

	// Open only safe libs
	for _, pair := range []struct {
		name string
		fn   lua.LGFunction
	}{
		{lua.BaseLibName, lua.OpenBase},
		{lua.TabLibName, lua.OpenTable},
		{lua.StringLibName, lua.OpenString},
		{lua.MathLibName, lua.OpenMath},
	} {
		L.Push(L.NewFunction(pair.fn))
		L.Push(lua.LString(pair.name))
		L.Call(1, 0)
	}

	if err := L.DoString(script); err != nil {
		return nil, fmt.Errorf("executing lua: %w", err)
	}

	return extractSettingsFromState(L), nil
}

// extractSettingsFromState reads plugin.settings from an already-executed Lua state.
func extractSettingsFromState(L *lua.LState) []SettingDefinition {
	pluginTable := L.GetGlobal("plugin")
	tbl, ok := pluginTable.(*lua.LTable)
	if !ok {
		return nil
	}

	settingsVal := tbl.RawGetString("settings")
	settingsTbl, ok := settingsVal.(*lua.LTable)
	if !ok {
		return nil
	}

	var defs []SettingDefinition
	settingsTbl.ForEach(func(_, value lua.LValue) {
		entry, ok := value.(*lua.LTable)
		if !ok {
			return
		}

		def := SettingDefinition{}
		if v := entry.RawGetString("name"); v != lua.LNil {
			def.Name = v.String()
		}
		if v := entry.RawGetString("type"); v != lua.LNil {
			def.Type = v.String()
		}
		if v := entry.RawGetString("label"); v != lua.LNil {
			def.Label = v.String()
		}
		if v := entry.RawGetString("required"); v == lua.LTrue {
			def.Required = true
		}

		if v := entry.RawGetString("default"); v != lua.LNil {
			switch def.Type {
			case "boolean":
				def.DefaultValue = (v == lua.LTrue)
			case "number":
				if num, ok := v.(lua.LNumber); ok {
					def.DefaultValue = float64(num)
				}
			default:
				def.DefaultValue = v.String()
			}
		}

		if v := entry.RawGetString("options"); v != lua.LNil {
			if optTbl, ok := v.(*lua.LTable); ok {
				optTbl.ForEach(func(_, optVal lua.LValue) {
					def.Options = append(def.Options, optVal.String())
				})
			}
		}

		defs = append(defs, def)
	})

	return defs
}

// ValidateSettings checks setting values against their definitions.
func ValidateSettings(defs []SettingDefinition, values map[string]any) []ValidationError {
	var errs []ValidationError

	for _, def := range defs {
		val, exists := values[def.Name]

		if def.Required && (!exists || val == nil || val == "") {
			errs = append(errs, ValidationError{
				Field:   def.Name,
				Message: fmt.Sprintf("%s is required", def.Label),
			})
			continue
		}

		if !exists || val == nil {
			continue
		}

		switch def.Type {
		case "boolean":
			if _, ok := val.(bool); !ok {
				errs = append(errs, ValidationError{
					Field:   def.Name,
					Message: fmt.Sprintf("%s must be a boolean", def.Label),
				})
			}
		case "number":
			switch val.(type) {
			case float64, int, int64:
				// ok
			default:
				errs = append(errs, ValidationError{
					Field:   def.Name,
					Message: fmt.Sprintf("%s must be a number", def.Label),
				})
			}
		case "select":
			strVal, ok := val.(string)
			if !ok {
				errs = append(errs, ValidationError{
					Field:   def.Name,
					Message: fmt.Sprintf("%s must be a string", def.Label),
				})
			} else {
				found := false
				for _, opt := range def.Options {
					if opt == strVal {
						found = true
						break
					}
				}
				if !found {
					errs = append(errs, ValidationError{
						Field:   def.Name,
						Message: fmt.Sprintf("%s must be one of: %v", def.Label, def.Options),
					})
				}
			}
		}
	}

	return errs
}

// CheckRequiredSettings returns labels of required settings that are missing or empty.
func CheckRequiredSettings(defs []SettingDefinition, values map[string]any) []string {
	var missing []string
	for _, def := range defs {
		if !def.Required {
			continue
		}
		val, exists := values[def.Name]
		if !exists || val == nil || val == "" {
			missing = append(missing, def.Label)
		}
	}
	return missing
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./plugin_system/ --tags 'json1 fts5' -run "TestParseSettings|TestValidateSettings|TestCheckRequired" -v`
Expected: PASS

**Step 5: Commit**

```bash
git add plugin_system/settings.go plugin_system/settings_test.go
git commit -m "feat(plugins): add settings types, parsing, and validation"
```

---

### Task 3: Discovery-Only Plugin Loading

**Files:**
- Modify: `plugin_system/manager.go`
- Test: `plugin_system/manager_test.go` (add tests)

This task adds the ability to scan plugins and parse metadata (including settings) **without** calling `init()`. The `PluginManager` now has a two-phase lifecycle: discovery, then selective activation.

**Step 1: Write failing tests**

Add to `plugin_system/manager_test.go`:

```go
func TestDiscoverPlugins(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "with-settings", `
plugin = {
    name = "with-settings",
    version = "1.0",
    description = "plugin with settings",
    settings = {
        { name = "api_key", type = "password", label = "API Key", required = true },
        { name = "city", type = "string", label = "City", default = "Berlin" },
    }
}
function init()
    mah.inject("page_top", function(ctx) return "banner" end)
end
`)
	writePlugin(t, dir, "no-settings", `
plugin = { name = "no-settings", version = "2.0", description = "simple plugin" }
function init()
    mah.on("after_note_create", function(data) end)
end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer pm.Close()

	discovered := pm.DiscoveredPlugins()
	if len(discovered) != 2 {
		t.Fatalf("expected 2 discovered plugins, got %d", len(discovered))
	}

	// Should be alphabetical
	if discovered[0].Name != "no-settings" {
		t.Errorf("expected first plugin 'no-settings', got %q", discovered[0].Name)
	}
	if discovered[1].Name != "with-settings" {
		t.Errorf("expected second plugin 'with-settings', got %q", discovered[1].Name)
	}

	// Check settings parsed
	if len(discovered[1].Settings) != 2 {
		t.Fatalf("expected 2 settings for 'with-settings', got %d", len(discovered[1].Settings))
	}
	if discovered[1].Settings[0].Name != "api_key" {
		t.Errorf("expected first setting 'api_key', got %q", discovered[1].Settings[0].Name)
	}

	// No plugins should be active yet (all disabled by default)
	if len(pm.Plugins()) != 0 {
		t.Errorf("expected 0 active plugins before enable, got %d", len(pm.Plugins()))
	}

	// No hooks should be registered
	if len(pm.GetHooks("after_note_create")) != 0 {
		t.Error("expected no hooks before enable")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./plugin_system/ --tags 'json1 fts5' -run TestDiscoverPlugins -v`
Expected: FAIL (DiscoveredPlugins doesn't exist)

**Step 3: Refactor PluginManager for discovery + activation**

This is the key architectural change. Modify `plugin_system/manager.go`:

1. Add new types and fields to `PluginManager`:

```go
// DiscoveredPlugin holds metadata about a discovered (but not necessarily loaded) plugin.
type DiscoveredPlugin struct {
	Name        string
	Version     string
	Description string
	Dir         string
	Settings    []SettingDefinition
}
```

Add to `PluginManager` struct:

```go
	discovered []DiscoveredPlugin
	// pluginSettings stores in-memory settings for enabled plugins (pluginName -> key -> value)
	pluginSettings map[string]map[string]any
```

2. Modify `NewPluginManager` to only discover (not load) plugins:

```go
func NewPluginManager(dir string) (*PluginManager, error) {
	pm := &PluginManager{
		hooks:          make(map[string][]hookEntry),
		injections:     make(map[string][]injectionEntry),
		pages:          make(map[string]map[string]pageEntry),
		vmLocks:        make(map[*lua.LState]*sync.Mutex),
		pluginSettings: make(map[string]map[string]any),
		httpClient:     newHttpClient(),
		httpNotify:     make(chan struct{}, 1),
		httpStop:       make(chan struct{}),
		httpSem:        make(chan struct{}, maxConcurrentHttpReqs),
	}

	go pm.drainHttpCallbacks()

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return pm, nil
		}
		return nil, fmt.Errorf("reading plugin directory: %w", err)
	}

	var pluginDirs []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		entryPath := filepath.Join(dir, entry.Name(), "plugin.lua")
		if _, err := os.Stat(entryPath); err == nil {
			pluginDirs = append(pluginDirs, entry.Name())
		}
	}
	sort.Strings(pluginDirs)

	for _, name := range pluginDirs {
		pluginDir := filepath.Join(dir, name)
		scriptPath := filepath.Join(pluginDir, "plugin.lua")
		dp, err := pm.discoverPlugin(pluginDir, scriptPath)
		if err != nil {
			log.Printf("[plugin] warning: skipping %q: %v", name, err)
			continue
		}
		pm.discovered = append(pm.discovered, dp)
	}

	return pm, nil
}
```

3. Add `discoverPlugin` — reads metadata + settings without `init()`:

```go
// discoverPlugin reads plugin.lua to extract metadata and settings without
// calling init() or registering any hooks/pages/menus.
func (pm *PluginManager) discoverPlugin(pluginDir, scriptPath string) (DiscoveredPlugin, error) {
	code, err := os.ReadFile(scriptPath)
	if err != nil {
		return DiscoveredPlugin{}, fmt.Errorf("reading plugin.lua: %w", err)
	}

	L := lua.NewState(lua.Options{SkipOpenLibs: true})
	defer L.Close()

	for _, pair := range []struct {
		name string
		fn   lua.LGFunction
	}{
		{lua.BaseLibName, lua.OpenBase},
		{lua.TabLibName, lua.OpenTable},
		{lua.StringLibName, lua.OpenString},
		{lua.MathLibName, lua.OpenMath},
	} {
		L.Push(L.NewFunction(pair.fn))
		L.Push(lua.LString(pair.name))
		L.Call(1, 0)
	}

	if err := L.DoString(string(code)); err != nil {
		return DiscoveredPlugin{}, fmt.Errorf("parsing plugin.lua: %w", err)
	}

	dp := DiscoveredPlugin{Dir: pluginDir}
	pluginTable := L.GetGlobal("plugin")
	if tbl, ok := pluginTable.(*lua.LTable); ok {
		if v := tbl.RawGetString("name"); v != lua.LNil {
			dp.Name = v.String()
		}
		if v := tbl.RawGetString("version"); v != lua.LNil {
			dp.Version = v.String()
		}
		if v := tbl.RawGetString("description"); v != lua.LNil {
			dp.Description = v.String()
		}
	}

	dp.Settings = extractSettingsFromState(L)

	return dp, nil
}
```

4. Add `DiscoveredPlugins()`:

```go
// DiscoveredPlugins returns a copy of all discovered plugin metadata.
func (pm *PluginManager) DiscoveredPlugins() []DiscoveredPlugin {
	result := make([]DiscoveredPlugin, len(pm.discovered))
	copy(result, pm.discovered)
	return result
}
```

5. The existing `loadPlugin` method stays as-is — it's now called by `EnablePlugin()` (Task 4).

**Step 4: Run tests**

Run: `go test ./plugin_system/ --tags 'json1 fts5' -run TestDiscoverPlugins -v`
Expected: PASS

Also run all existing tests to confirm nothing broke:

Run: `go test ./plugin_system/ --tags 'json1 fts5' -v`
Expected: Some existing tests may fail because `NewPluginManager` no longer auto-loads plugins. The existing tests that expect plugins to be loaded immediately will need updating — they should call `EnablePlugin` after construction (added in Task 4). **Fix these in Task 4.**

**Step 5: Commit**

```bash
git add plugin_system/manager.go plugin_system/manager_test.go
git commit -m "feat(plugins): add discovery-only plugin loading with settings parsing"
```

---

### Task 4: Runtime Enable/Disable

**Files:**
- Modify: `plugin_system/manager.go`
- Modify: `plugin_system/manager_test.go`

**Step 1: Write failing tests**

Add to `plugin_system/manager_test.go`:

```go
func TestEnableDisablePlugin(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "toggle", `
plugin = { name = "toggle", version = "1.0", description = "toggle test" }
function init()
    mah.on("after_note_create", function(data) end)
    mah.inject("page_top", function(ctx) return "banner" end)
    mah.page("info", function(ctx) return "<h1>Info</h1>" end)
    mah.menu("Info Page", "info")
end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer pm.Close()

	// Initially nothing active
	if len(pm.Plugins()) != 0 {
		t.Fatal("expected 0 active plugins")
	}

	// Enable
	if err := pm.EnablePlugin("toggle"); err != nil {
		t.Fatalf("EnablePlugin failed: %v", err)
	}

	if len(pm.Plugins()) != 1 {
		t.Fatal("expected 1 active plugin after enable")
	}
	if len(pm.GetHooks("after_note_create")) != 1 {
		t.Error("expected hook after enable")
	}
	if len(pm.GetInjections("page_top")) != 1 {
		t.Error("expected injection after enable")
	}
	if !pm.HasPage("toggle", "info") {
		t.Error("expected page after enable")
	}
	if len(pm.GetMenuItems()) != 1 {
		t.Error("expected menu item after enable")
	}

	// Disable
	if err := pm.DisablePlugin("toggle"); err != nil {
		t.Fatalf("DisablePlugin failed: %v", err)
	}

	if len(pm.Plugins()) != 0 {
		t.Error("expected 0 active plugins after disable")
	}
	if len(pm.GetHooks("after_note_create")) != 0 {
		t.Error("expected no hooks after disable")
	}
	if len(pm.GetInjections("page_top")) != 0 {
		t.Error("expected no injections after disable")
	}
	if pm.HasPage("toggle", "info") {
		t.Error("expected no page after disable")
	}
	if len(pm.GetMenuItems()) != 0 {
		t.Error("expected no menu items after disable")
	}
}

func TestEnableUnknownPlugin(t *testing.T) {
	dir := t.TempDir()
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer pm.Close()

	if err := pm.EnablePlugin("nonexistent"); err == nil {
		t.Error("expected error for unknown plugin")
	}
}

func TestEnableAlreadyEnabled(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "double", `
plugin = { name = "double", version = "1.0", description = "test" }
function init() end
`)
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer pm.Close()

	if err := pm.EnablePlugin("double"); err != nil {
		t.Fatal(err)
	}
	if err := pm.EnablePlugin("double"); err == nil {
		t.Error("expected error for already-enabled plugin")
	}
}
```

**Step 2: Run to confirm failure**

Run: `go test ./plugin_system/ --tags 'json1 fts5' -run "TestEnableDisable|TestEnableUnknown|TestEnableAlready" -v`
Expected: FAIL

**Step 3: Implement EnablePlugin and DisablePlugin**

Add to `plugin_system/manager.go`:

```go
// EnablePlugin activates a discovered plugin by creating a Lua VM and calling init().
func (pm *PluginManager) EnablePlugin(name string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	// Check already enabled
	for _, p := range pm.plugins {
		if p.Name == name {
			return fmt.Errorf("plugin %q is already enabled", name)
		}
	}

	// Find in discovered
	var dp *DiscoveredPlugin
	for i := range pm.discovered {
		if pm.discovered[i].Name == name {
			dp = &pm.discovered[i]
			break
		}
	}
	if dp == nil {
		return fmt.Errorf("plugin %q not found", name)
	}

	scriptPath := filepath.Join(dp.Dir, "plugin.lua")
	if err := pm.loadPlugin(dp.Dir, scriptPath); err != nil {
		return fmt.Errorf("loading plugin %q: %w", name, err)
	}

	return nil
}

// DisablePlugin deactivates a running plugin: removes all hooks, injections,
// pages, menu items, and closes the Lua VM.
func (pm *PluginManager) DisablePlugin(name string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	// Find the plugin's Lua state
	var targetState *lua.LState
	var pluginIdx int = -1
	for i, p := range pm.plugins {
		if p.Name == name {
			targetState = pm.states[i]
			pluginIdx = i
			break
		}
	}
	if targetState == nil {
		return fmt.Errorf("plugin %q is not enabled", name)
	}

	// Remove hooks belonging to this state
	for event, entries := range pm.hooks {
		var filtered []hookEntry
		for _, e := range entries {
			if e.state != targetState {
				filtered = append(filtered, e)
			}
		}
		pm.hooks[event] = filtered
	}

	// Remove injections belonging to this state
	for slot, entries := range pm.injections {
		var filtered []injectionEntry
		for _, e := range entries {
			if e.state != targetState {
				filtered = append(filtered, e)
			}
		}
		pm.injections[slot] = filtered
	}

	// Remove pages for this plugin
	delete(pm.pages, name)

	// Remove menu items for this plugin
	var filteredMenus []MenuRegistration
	for _, m := range pm.menuItems {
		if m.PluginName != name {
			filteredMenus = append(filteredMenus, m)
		}
	}
	pm.menuItems = filteredMenus

	// Remove from active lists
	pm.plugins = append(pm.plugins[:pluginIdx], pm.plugins[pluginIdx+1:]...)
	pm.states = append(pm.states[:pluginIdx], pm.states[pluginIdx+1:]...)

	// Remove VM lock and close state
	delete(pm.vmLocks, targetState)
	targetState.Close()

	// Remove in-memory settings
	delete(pm.pluginSettings, name)

	return nil
}

// IsEnabled returns whether a plugin is currently active.
func (pm *PluginManager) IsEnabled(name string) bool {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	for _, p := range pm.plugins {
		if p.Name == name {
			return true
		}
	}
	return false
}
```

**Step 4: Fix existing tests**

The existing tests in `manager_test.go` call `NewPluginManager` and expect plugins to be already loaded. Update them: after `NewPluginManager`, call `pm.EnablePlugin(name)` for each plugin that the test expects to be active. For example, `TestSingleValidPlugin` should become:

```go
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

	// Enable the plugin
	if err := pm.EnablePlugin("hello"); err != nil {
		t.Fatalf("EnablePlugin failed: %v", err)
	}

	plugins := pm.Plugins()
	// ... rest of test stays the same ...
```

Apply the same pattern to all existing tests that expect loaded plugins:
- `TestSingleValidPlugin` — add `pm.EnablePlugin("hello")`
- `TestBadSyntaxPluginSkipped` — bad plugin won't be in discovered list (or discovery fails); good plugin needs `pm.EnablePlugin("good")`
- `TestAlphabeticalLoadOrder` — enable all three, order should still be alphabetical
- `TestHookRegistration` — enable "hooker"
- `TestInjectionRegistration` — enable "injector"
- `TestAbortFunction` — enable "aborter"
- `TestMultipleHooksSameEvent` — enable both "alpha" and "bravo"
- `TestPluginWithoutInit` — enable "noinit"

Also update tests in other test files: `hooks_test.go`, `injections_test.go`, `pages_test.go`, `integration_test.go`, `db_api_test.go`, `http_api_test.go`.

**Step 5: Run all plugin tests**

Run: `go test ./plugin_system/ --tags 'json1 fts5' -v`
Expected: All PASS

**Step 6: Commit**

```bash
git add plugin_system/manager.go plugin_system/manager_test.go
git commit -m "feat(plugins): add runtime enable/disable with full teardown"
```

---

### Task 5: Settings Runtime Access (mah.get_setting)

**Files:**
- Modify: `plugin_system/manager.go` (add get_setting to registerMahModule)
- Test: `plugin_system/manager_test.go`

**Step 1: Write failing test**

Add to `plugin_system/manager_test.go`:

```go
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
```

**Step 2: Run to verify failure**

Run: `go test ./plugin_system/ --tags 'json1 fts5' -run "TestGetSetting" -v`
Expected: FAIL

**Step 3: Implement SetPluginSettings and mah.get_setting**

Add to `plugin_system/manager.go`:

```go
// SetPluginSettings stores settings for a plugin in memory.
// These are accessible via mah.get_setting() in Lua.
func (pm *PluginManager) SetPluginSettings(pluginName string, settings map[string]any) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.pluginSettings[pluginName] = settings
}

// GetPluginSettings returns the in-memory settings for a plugin.
func (pm *PluginManager) GetPluginSettings(pluginName string) map[string]any {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.pluginSettings[pluginName]
}
```

In `registerMahModule`, add the `get_setting` function (after the `abort` registration):

```go
	mahMod.RawSetString("get_setting", L.NewFunction(func(L *lua.LState) int {
		key := L.CheckString(1)
		name := *pluginNamePtr

		pm.mu.RLock()
		settings := pm.pluginSettings[name]
		pm.mu.RUnlock()

		if settings == nil {
			L.Push(lua.LNil)
			return 1
		}

		val, ok := settings[key]
		if !ok || val == nil {
			L.Push(lua.LNil)
			return 1
		}

		switch v := val.(type) {
		case string:
			L.Push(lua.LString(v))
		case float64:
			L.Push(lua.LNumber(v))
		case bool:
			L.Push(lua.LBool(v))
		default:
			L.Push(lua.LString(fmt.Sprintf("%v", v)))
		}
		return 1
	}))
```

**Step 4: Run tests**

Run: `go test ./plugin_system/ --tags 'json1 fts5' -run "TestGetSetting" -v`
Expected: PASS

**Step 5: Run all plugin tests**

Run: `go test ./plugin_system/ --tags 'json1 fts5' -v`
Expected: All PASS

**Step 6: Commit**

```bash
git add plugin_system/manager.go plugin_system/manager_test.go
git commit -m "feat(plugins): add mah.get_setting() for runtime settings access"
```

---

### Task 6: Application Context Integration

**Files:**
- Modify: `application_context/context.go` (lines 204-224)
- Create: `application_context/plugin_state_context.go`

This task connects the PluginManager's new discovery/enable/disable lifecycle to the database-backed `PluginState` model and wires it through the application context.

**Step 1: Create plugin state context methods**

Create `application_context/plugin_state_context.go`:

```go
package application_context

import (
	"encoding/json"
	"fmt"
	"mahresources/models"
	"mahresources/plugin_system"
)

// EnsurePluginStates creates PluginState rows for any discovered plugins
// that don't yet have one. Returns all plugin states.
func (ctx *MahresourcesContext) EnsurePluginStates() ([]models.PluginState, error) {
	if ctx.pluginManager == nil {
		return nil, nil
	}

	for _, dp := range ctx.pluginManager.DiscoveredPlugins() {
		var count int64
		ctx.db.Model(&models.PluginState{}).Where("plugin_name = ?", dp.Name).Count(&count)
		if count == 0 {
			state := models.PluginState{
				PluginName: dp.Name,
				Enabled:    false,
			}
			if err := ctx.db.Create(&state).Error; err != nil {
				return nil, fmt.Errorf("creating plugin state for %q: %w", dp.Name, err)
			}
		}
	}

	var states []models.PluginState
	if err := ctx.db.Order("plugin_name").Find(&states).Error; err != nil {
		return nil, err
	}
	return states, nil
}

// GetPluginStates returns all plugin states from the database.
func (ctx *MahresourcesContext) GetPluginStates() ([]models.PluginState, error) {
	var states []models.PluginState
	if err := ctx.db.Order("plugin_name").Find(&states).Error; err != nil {
		return nil, err
	}
	return states, nil
}

// GetPluginState returns the state for a specific plugin.
func (ctx *MahresourcesContext) GetPluginState(pluginName string) (*models.PluginState, error) {
	var state models.PluginState
	if err := ctx.db.Where("plugin_name = ?", pluginName).First(&state).Error; err != nil {
		return nil, err
	}
	return &state, nil
}

// SetPluginEnabled enables or disables a plugin and updates the database.
func (ctx *MahresourcesContext) SetPluginEnabled(pluginName string, enabled bool) error {
	if ctx.pluginManager == nil {
		return fmt.Errorf("plugin manager not initialized")
	}

	if enabled {
		// Check required settings before enabling
		dp := ctx.findDiscoveredPlugin(pluginName)
		if dp == nil {
			return fmt.Errorf("plugin %q not found", pluginName)
		}

		settings, _ := ctx.loadPluginSettingsMap(pluginName)
		missing := plugin_system.CheckRequiredSettings(dp.Settings, settings)
		if len(missing) > 0 {
			return fmt.Errorf("missing required settings: %v", missing)
		}

		// Load settings into plugin manager memory
		ctx.pluginManager.SetPluginSettings(pluginName, settings)

		if err := ctx.pluginManager.EnablePlugin(pluginName); err != nil {
			return err
		}
	} else {
		if err := ctx.pluginManager.DisablePlugin(pluginName); err != nil {
			return err
		}
	}

	return ctx.db.Model(&models.PluginState{}).
		Where("plugin_name = ?", pluginName).
		Update("enabled", enabled).Error
}

// SavePluginSettings validates and saves settings for a plugin.
func (ctx *MahresourcesContext) SavePluginSettings(pluginName string, values map[string]any) ([]plugin_system.ValidationError, error) {
	if ctx.pluginManager == nil {
		return nil, fmt.Errorf("plugin manager not initialized")
	}

	dp := ctx.findDiscoveredPlugin(pluginName)
	if dp == nil {
		return nil, fmt.Errorf("plugin %q not found", pluginName)
	}

	// Validate
	if errs := plugin_system.ValidateSettings(dp.Settings, values); len(errs) > 0 {
		return errs, nil
	}

	// Serialize to JSON
	jsonBytes, err := json.Marshal(values)
	if err != nil {
		return nil, fmt.Errorf("marshaling settings: %w", err)
	}

	// Save to DB
	if err := ctx.db.Model(&models.PluginState{}).
		Where("plugin_name = ?", pluginName).
		Update("settings_json", string(jsonBytes)).Error; err != nil {
		return nil, err
	}

	// Update in-memory settings if plugin is enabled
	if ctx.pluginManager.IsEnabled(pluginName) {
		ctx.pluginManager.SetPluginSettings(pluginName, values)
	}

	return nil, nil
}

// ActivateEnabledPlugins enables all plugins marked as enabled in the database.
// Called at startup after plugin discovery and DB initialization.
func (ctx *MahresourcesContext) ActivateEnabledPlugins() {
	if ctx.pluginManager == nil {
		return
	}

	states, err := ctx.GetPluginStates()
	if err != nil {
		return
	}

	for _, state := range states {
		if !state.Enabled {
			continue
		}

		// Load settings into memory
		settings, _ := ctx.loadPluginSettingsMap(state.PluginName)
		ctx.pluginManager.SetPluginSettings(state.PluginName, settings)

		if err := ctx.pluginManager.EnablePlugin(state.PluginName); err != nil {
			fmt.Printf("[plugin] warning: failed to enable %q at startup: %v\n", state.PluginName, err)
		}
	}
}

func (ctx *MahresourcesContext) findDiscoveredPlugin(name string) *plugin_system.DiscoveredPlugin {
	for _, dp := range ctx.pluginManager.DiscoveredPlugins() {
		if dp.Name == name {
			return &dp
		}
	}
	return nil
}

func (ctx *MahresourcesContext) loadPluginSettingsMap(pluginName string) (map[string]any, error) {
	state, err := ctx.GetPluginState(pluginName)
	if err != nil || state.SettingsJSON == "" {
		return make(map[string]any), err
	}

	var settings map[string]any
	if err := json.Unmarshal([]byte(state.SettingsJSON), &settings); err != nil {
		return make(map[string]any), err
	}
	return settings, nil
}
```

**Step 2: Update context initialization**

In `application_context/context.go`, update the plugin initialization section (lines 204-224). Replace the current plugin init block with:

```go
	// Initialize plugin manager unless disabled
	if !config.PluginsDisabled {
		pluginPath := config.PluginPath
		if pluginPath == "" {
			pluginPath = "./plugins"
		}
		pm, pmErr := plugin_system.NewPluginManager(pluginPath)
		if pmErr != nil {
			log.Printf("[plugin] WARNING: failed to initialize plugin system: %v", pmErr)
		} else {
			ctx.pluginManager = pm
			if discovered := pm.DiscoveredPlugins(); len(discovered) > 0 {
				log.Printf("[plugin] Discovered %d plugin(s)", len(discovered))
				for _, p := range discovered {
					log.Printf("[plugin]   - %s v%s", p.Name, p.Version)
				}
			}
			pm.SetEntityQuerier(NewPluginDBAdapter(ctx))
		}
	}
```

**Step 3: Update main.go to activate enabled plugins after AutoMigrate**

In `main.go`, after the AutoMigrate call and FK re-enablement (around line 255), add:

```go
	// Initialize plugin states in DB and activate enabled plugins
	if context.PluginManager() != nil {
		if _, err := context.EnsurePluginStates(); err != nil {
			log.Printf("[plugin] WARNING: failed to initialize plugin states: %v", err)
		}
		context.ActivateEnabledPlugins()
		if plugins := context.PluginManager().Plugins(); len(plugins) > 0 {
			log.Printf("[plugin] Activated %d plugin(s)", len(plugins))
		}
	}
```

**Step 4: Run tests**

Run: `go build --tags 'json1 fts5'`
Expected: Compiles

Run: `go test ./... --tags 'json1 fts5'`
Expected: All PASS

**Step 5: Commit**

```bash
git add application_context/plugin_state_context.go application_context/context.go main.go
git commit -m "feat(plugins): integrate plugin states with DB and application context"
```

---

### Task 7: Plugin Management API Handlers

**Files:**
- Create: `server/api_handlers/plugin_api_handlers.go`
- Modify: `server/routes.go`

**Step 1: Create API handlers**

Create `server/api_handlers/plugin_api_handlers.go`:

```go
package api_handlers

import (
	"encoding/json"
	"mahresources/constants"
	"mahresources/server/http_utils"
	"net/http"
	"strings"

	"mahresources/application_context"
)

type pluginListItem struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
	Settings    any    `json:"settings,omitempty"`
	Values      any    `json:"values,omitempty"`
}

func GetPluginsManageHandler(ctx *application_context.MahresourcesContext) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		pm := ctx.PluginManager()
		if pm == nil {
			w.Header().Set("Content-Type", constants.JSON)
			_ = json.NewEncoder(w).Encode([]pluginListItem{})
			return
		}

		discovered := pm.DiscoveredPlugins()
		states, _ := ctx.GetPluginStates()

		stateMap := make(map[string]*struct {
			enabled  bool
			settings string
		})
		for _, s := range states {
			stateMap[s.PluginName] = &struct {
				enabled  bool
				settings string
			}{s.Enabled, s.SettingsJSON}
		}

		var items []pluginListItem
		for _, dp := range discovered {
			item := pluginListItem{
				Name:        dp.Name,
				Version:     dp.Version,
				Description: dp.Description,
				Settings:    dp.Settings,
			}
			if s, ok := stateMap[dp.Name]; ok {
				item.Enabled = s.enabled
				if s.settings != "" {
					var vals map[string]any
					if err := json.Unmarshal([]byte(s.settings), &vals); err == nil {
						item.Values = vals
					}
				}
			}
			items = append(items, item)
		}

		w.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(w).Encode(items)
	}
}

func GetPluginEnableHandler(ctx *application_context.MahresourcesContext) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimSpace(r.FormValue("name"))
		if name == "" {
			http_utils.HandleError(
				http.ErrMissingFile, w, r, http.StatusBadRequest,
			)
			return
		}

		if err := ctx.SetPluginEnabled(name, true); err != nil {
			http_utils.HandleError(err, w, r, http.StatusBadRequest)
			return
		}

		if http_utils.RedirectIfHTMLAccepted(w, r, "/plugins/manage") {
			return
		}

		w.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "name": name, "enabled": true})
	}
}

func GetPluginDisableHandler(ctx *application_context.MahresourcesContext) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimSpace(r.FormValue("name"))
		if name == "" {
			http_utils.HandleError(
				http.ErrMissingFile, w, r, http.StatusBadRequest,
			)
			return
		}

		if err := ctx.SetPluginEnabled(name, false); err != nil {
			http_utils.HandleError(err, w, r, http.StatusBadRequest)
			return
		}

		if http_utils.RedirectIfHTMLAccepted(w, r, "/plugins/manage") {
			return
		}

		w.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "name": name, "enabled": false})
	}
}

func GetPluginSettingsHandler(ctx *application_context.MahresourcesContext) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimSpace(r.FormValue("name"))
		if name == "" {
			http_utils.HandleError(
				http.ErrMissingFile, w, r, http.StatusBadRequest,
			)
			return
		}

		var values map[string]any
		if err := json.NewDecoder(r.Body).Decode(&values); err != nil {
			http_utils.HandleError(err, w, r, http.StatusBadRequest)
			return
		}

		validationErrors, err := ctx.SavePluginSettings(name, values)
		if err != nil {
			http_utils.HandleError(err, w, r, http.StatusInternalServerError)
			return
		}
		if len(validationErrors) > 0 {
			w.Header().Set("Content-Type", constants.JSON)
			w.WriteHeader(http.StatusUnprocessableEntity)
			_ = json.NewEncoder(w).Encode(map[string]any{"errors": validationErrors})
			return
		}

		if http_utils.RedirectIfHTMLAccepted(w, r, "/plugins/manage") {
			return
		}

		w.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "name": name})
	}
}
```

**Step 2: Register routes**

In `server/routes.go`, add the plugin management routes before the plugin pages catch-all (before line 301):

```go
	// Plugin management API
	router.Methods(http.MethodGet).Path("/v1/plugins/manage").HandlerFunc(api_handlers.GetPluginsManageHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/plugin/enable").HandlerFunc(api_handlers.GetPluginEnableHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/plugin/disable").HandlerFunc(api_handlers.GetPluginDisableHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/plugin/settings").HandlerFunc(api_handlers.GetPluginSettingsHandler(appContext))
```

**Step 3: Compile and test**

Run: `go build --tags 'json1 fts5'`
Expected: Compiles

Run: `go test ./... --tags 'json1 fts5'`
Expected: All PASS

**Step 4: Commit**

```bash
git add server/api_handlers/plugin_api_handlers.go server/routes.go
git commit -m "feat(plugins): add plugin management API endpoints"
```

---

### Task 8: Plugin Management Template Page

**Files:**
- Create: `templates/managePlugins.tpl`
- Modify: `server/routes.go` (add template route for `/plugins/manage`)
- Create: `server/template_handlers/template_context_providers/plugin_manage_context.go`
- Modify: `templates/partials/menu.tpl` (add "Manage Plugins" link)

**Step 1: Create context provider**

Create `server/template_handlers/template_context_providers/plugin_manage_context.go`:

```go
package template_context_providers

import (
	"encoding/json"
	"net/http"

	"github.com/flosch/pongo2/v4"
	"mahresources/application_context"
	"mahresources/plugin_system"
)

type pluginDisplay struct {
	Name        string
	Version     string
	Description string
	Enabled     bool
	Settings    []plugin_system.SettingDefinition
	Values      map[string]any
}

func PluginManageContextProvider(appCtx *application_context.MahresourcesContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		ctx := staticTemplateCtx(request)
		ctx["pageTitle"] = "Manage Plugins"

		pm := appCtx.PluginManager()
		if pm == nil {
			ctx["plugins"] = []pluginDisplay{}
			return ctx
		}

		discovered := pm.DiscoveredPlugins()
		states, _ := appCtx.GetPluginStates()

		stateMap := make(map[string]struct {
			enabled  bool
			settings string
		})
		for _, s := range states {
			stateMap[s.PluginName] = struct {
				enabled  bool
				settings string
			}{s.Enabled, s.SettingsJSON}
		}

		var plugins []pluginDisplay
		for _, dp := range discovered {
			pd := pluginDisplay{
				Name:        dp.Name,
				Version:     dp.Version,
				Description: dp.Description,
				Settings:    dp.Settings,
				Values:      make(map[string]any),
			}
			if s, ok := stateMap[dp.Name]; ok {
				pd.Enabled = s.enabled
				if s.settings != "" {
					json.Unmarshal([]byte(s.settings), &pd.Values)
				}
			}
			plugins = append(plugins, pd)
		}

		ctx["plugins"] = plugins
		return ctx
	}
}
```

**Step 2: Create the template**

Create `templates/managePlugins.tpl`:

```django
{% extends "/layouts/base.tpl" %}

{% block head %}
    <title>Manage Plugins - mahresources</title>
{% endblock %}

{% block body %}
<div class="content-wrap">
    <h1 class="page-title">Manage Plugins</h1>

    {% if not plugins %}
    <p class="text-gray-500 italic">No plugins discovered. Place plugin directories in the plugins folder.</p>
    {% endif %}

    {% for plugin in plugins %}
    <div class="card mb-4" data-testid="plugin-card-{{ plugin.Name }}">
        <div class="card-header flex items-center justify-between">
            <div>
                <h2 class="text-lg font-semibold">{{ plugin.Name }}
                    <span class="text-sm text-gray-500 font-normal">v{{ plugin.Version }}</span>
                </h2>
                {% if plugin.Description %}
                <p class="text-sm text-gray-600">{{ plugin.Description }}</p>
                {% endif %}
            </div>
            <form method="POST"
                  action="{% if plugin.Enabled %}/v1/plugin/disable{% else %}/v1/plugin/enable{% endif %}"
                  class="inline">
                <input type="hidden" name="name" value="{{ plugin.Name }}">
                <input type="hidden" name="redirect" value="/plugins/manage">
                <button type="submit"
                        class="btn {% if plugin.Enabled %}btn-danger{% else %}btn-primary{% endif %}"
                        data-testid="plugin-toggle-{{ plugin.Name }}">
                    {% if plugin.Enabled %}Disable{% else %}Enable{% endif %}
                </button>
            </form>
        </div>

        {% if plugin.Settings %}
        <div class="card-body">
            <h3 class="text-sm font-semibold mb-2 text-gray-700">Settings</h3>
            <form method="POST" action="/v1/plugin/settings"
                  x-data="pluginSettings('{{ plugin.Name }}')"
                  @submit.prevent="saveSettings"
                  data-testid="plugin-settings-{{ plugin.Name }}">

                {% for setting in plugin.Settings %}
                <div class="mb-3">
                    <label class="form-label" for="setting-{{ plugin.Name }}-{{ setting.Name }}">
                        {{ setting.Label }}
                        {% if setting.Required %}<span class="text-red-500" title="Required">*</span>{% endif %}
                    </label>

                    {% if setting.Type == "boolean" %}
                    <input type="checkbox"
                           id="setting-{{ plugin.Name }}-{{ setting.Name }}"
                           name="{{ setting.Name }}"
                           {% if plugin.Values %}{% if plugin.Values|get:setting.Name %}checked{% endif %}{% elif setting.DefaultValue %}checked{% endif %}
                           class="form-checkbox"
                           data-testid="setting-{{ setting.Name }}">

                    {% elif setting.Type == "select" %}
                    <select id="setting-{{ plugin.Name }}-{{ setting.Name }}"
                            name="{{ setting.Name }}"
                            class="form-input"
                            data-testid="setting-{{ setting.Name }}">
                        {% for option in setting.Options %}
                        <option value="{{ option }}"
                                {% if plugin.Values %}{% if plugin.Values|get:setting.Name == option %}selected{% endif %}{% elif setting.DefaultValue == option %}selected{% endif %}>
                            {{ option }}
                        </option>
                        {% endfor %}
                    </select>

                    {% elif setting.Type == "password" %}
                    <input type="password"
                           id="setting-{{ plugin.Name }}-{{ setting.Name }}"
                           name="{{ setting.Name }}"
                           value="{{ plugin.Values|get:setting.Name }}"
                           class="form-input"
                           {% if setting.Required %}required{% endif %}
                           placeholder="{{ setting.Label }}"
                           data-testid="setting-{{ setting.Name }}">

                    {% elif setting.Type == "number" %}
                    <input type="number"
                           id="setting-{{ plugin.Name }}-{{ setting.Name }}"
                           name="{{ setting.Name }}"
                           value="{% if plugin.Values %}{{ plugin.Values|get:setting.Name }}{% elif setting.DefaultValue %}{{ setting.DefaultValue }}{% endif %}"
                           class="form-input"
                           step="any"
                           data-testid="setting-{{ setting.Name }}">

                    {% else %}
                    <input type="text"
                           id="setting-{{ plugin.Name }}-{{ setting.Name }}"
                           name="{{ setting.Name }}"
                           value="{% if plugin.Values %}{{ plugin.Values|get:setting.Name }}{% elif setting.DefaultValue %}{{ setting.DefaultValue }}{% endif %}"
                           class="form-input"
                           {% if setting.Required %}required{% endif %}
                           placeholder="{{ setting.Label }}"
                           data-testid="setting-{{ setting.Name }}">
                    {% endif %}
                </div>
                {% endfor %}

                <button type="submit" class="btn btn-primary" data-testid="save-settings-{{ plugin.Name }}">
                    Save Settings
                </button>
                <span x-show="saved" x-transition class="text-green-600 text-sm ml-2">Saved!</span>
                <span x-show="error" x-transition class="text-red-600 text-sm ml-2" x-text="error"></span>
            </form>
        </div>
        {% else %}
        <div class="card-body">
            <p class="text-sm text-gray-500 italic">No settings declared.</p>
        </div>
        {% endif %}
    </div>
    {% endfor %}
</div>
{% endblock %}
```

Note: The `pluginSettings` Alpine.js component will be added in the frontend task. For now, the form works as a standard POST form with page reload. The `x-data`, `@submit.prevent`, and Alpine directives will be progressively enhanced.

**Step 3: Register the template route**

In `server/routes.go`, add a new entry to the `templates` map:

```go
"/plugins/manage": {template_context_providers.PluginManageContextProviderFactory, "managePlugins.tpl", http.MethodGet},
```

Wait — the templates map uses a function signature `func(context *application_context.MahresourcesContext) func(request *http.Request) pongo2.Context`. So `PluginManageContextProvider` already matches this shape. Update the function name to match:

In `plugin_manage_context.go`, rename the function to match the expected pattern for the templates map. Actually, looking at the templates map more carefully, each entry's `contextFn` is `func(context *application_context.MahresourcesContext) func(request *http.Request) pongo2.Context`. The `PluginManageContextProvider` takes `appCtx` directly and returns the inner func. We need a factory wrapper.

Add to `plugin_manage_context.go`:

```go
func PluginManageContextProviderFactory(appCtx *application_context.MahresourcesContext) func(request *http.Request) pongo2.Context {
	return PluginManageContextProvider(appCtx)
}
```

Then, instead of adding to the templates map (which requires the factory signature), register the route directly. Add in `registerRoutes()`, **before** the plugin pages catch-all:

```go
	// Plugin management page
	manageCtxFn := wrapContextWithPlugins(appContext, template_context_providers.PluginManageContextProvider(appContext))
	router.Methods(http.MethodGet).Path("/plugins/manage").
		HandlerFunc(template_handlers.RenderTemplate("managePlugins.tpl", manageCtxFn))
```

This must come **before** the `PathPrefix("/plugins/")` catch-all so it takes priority.

**Step 4: Add "Manage Plugins" link to menu**

In `templates/partials/menu.tpl`, find the Plugins dropdown content (line 69-76). Add a "Manage Plugins" link and divider at the top of the dropdown:

```django
                <a href="/plugins/manage"
                   class="navbar-dropdown-item {% if '/plugins/manage' == path %}navbar-dropdown-item--active{% endif %}"
                   @click="pluginsOpen = false">
                    Manage Plugins
                </a>
                {% if pluginMenuItems %}
                <div class="navbar-dropdown-divider"></div>
                {% endif %}
```

Also update the mobile section similarly. Additionally, the Plugins dropdown should now always show (even when no pluginMenuItems exist) if the plugin manager exists. We need a new template variable `hasPluginManager` passed via `wrapContextWithPlugins`. Add it:

In `server/routes.go`, `wrapContextWithPlugins`:

```go
		ctx["hasPluginManager"] = true
```

Then update `templates/partials/menu.tpl` to show the Plugins dropdown when `hasPluginManager` is true (not just when `pluginMenuItems` exists):

Change `{% if pluginMenuItems %}` to `{% if hasPluginManager %}` for both desktop and mobile sections.

**Step 5: Compile and test**

Run: `go build --tags 'json1 fts5'`
Expected: Compiles

**Step 6: Commit**

```bash
git add templates/managePlugins.tpl \
    server/template_handlers/template_context_providers/plugin_manage_context.go \
    server/routes.go templates/partials/menu.tpl
git commit -m "feat(plugins): add plugin management page and navigation"
```

---

### Task 9: Template Filter for Settings Values

**Files:**
- Modify: Template filter registration (check how `plugin_slot` filter is registered)
- Possibly modify template code

The `managePlugins.tpl` uses `{{ plugin.Values|get:setting.Name }}` which requires a pongo2 filter called `get` that retrieves a key from a map. Check if this filter already exists. If not, add it.

**Step 1: Check existing filters**

Search for existing pongo2 filter registrations in the codebase. The `plugin_slot` is a tag, not a filter. Look for `pongo2.RegisterFilter`.

If no `get` filter exists, register one. This filter takes a map and a key argument:

```go
func init() {
	pongo2.RegisterFilter("get", func(in *pongo2.Value, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
		if in.IsNil() {
			return pongo2.AsValue(""), nil
		}
		key := param.String()
		// Try map[string]any
		if m, ok := in.Interface().(map[string]any); ok {
			if v, exists := m[key]; exists {
				return pongo2.AsValue(v), nil
			}
		}
		return pongo2.AsValue(""), nil
	})
}
```

Place this in a new file or in the existing filter registration file.

**Step 2: Test compilation**

Run: `go build --tags 'json1 fts5'`
Expected: Compiles

**Step 3: Commit**

```bash
git add <filter file>
git commit -m "feat(plugins): add pongo2 'get' filter for map key access"
```

---

### Task 10: Frontend Alpine.js Component

**Files:**
- Create or modify: `src/components/pluginSettings.js`
- Modify: `src/main.js` (import and register)

**Step 1: Create the Alpine.js data component**

Create `src/components/pluginSettings.js`:

```javascript
export default function pluginSettings(pluginName) {
    return {
        pluginName,
        saved: false,
        error: '',

        async saveSettings(event) {
            this.saved = false;
            this.error = '';

            const form = event.target;
            const formData = new FormData(form);
            const values = {};

            // Build values object from form
            for (const [key, value] of formData.entries()) {
                if (key === 'name') continue;
                values[key] = value;
            }

            // Handle checkboxes (unchecked ones aren't in FormData)
            form.querySelectorAll('input[type="checkbox"]').forEach(cb => {
                values[cb.name] = cb.checked;
            });

            // Handle number fields
            form.querySelectorAll('input[type="number"]').forEach(input => {
                if (values[input.name] !== undefined && values[input.name] !== '') {
                    values[input.name] = parseFloat(values[input.name]);
                }
            });

            try {
                const response = await fetch(`/v1/plugin/settings?name=${encodeURIComponent(this.pluginName)}`, {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify(values),
                });

                if (!response.ok) {
                    const data = await response.json();
                    if (data.errors) {
                        this.error = data.errors.map(e => e.message).join(', ');
                    } else {
                        this.error = 'Failed to save settings';
                    }
                    return;
                }

                this.saved = true;
                setTimeout(() => { this.saved = false; }, 3000);
            } catch (err) {
                this.error = err.message;
            }
        }
    };
}
```

**Step 2: Register in main.js**

In `src/main.js`, import and register:

```javascript
import pluginSettings from './components/pluginSettings.js';
// ... in the Alpine.data registrations:
Alpine.data('pluginSettings', pluginSettings);
```

**Step 3: Build**

Run: `npm run build-js`
Expected: Builds successfully

**Step 4: Commit**

```bash
git add src/components/pluginSettings.js src/main.js
git commit -m "feat(plugins): add Alpine.js component for plugin settings form"
```

---

### Task 11: Test Plugin with Settings

**Files:**
- Modify: `e2e/test-plugins/test-banner/plugin.lua`

Update the E2E test plugin to declare settings and display a setting value on a page.

**Step 1: Update the test plugin**

```lua
plugin = {
    name = "test-banner",
    version = "1.0",
    description = "Test plugin that injects a banner on every page",
    settings = {
        { name = "banner_text", type = "string", label = "Banner Text", default = "Plugin Banner Active" },
        { name = "api_key", type = "password", label = "API Key", required = true },
        { name = "show_banner", type = "boolean", label = "Show Banner", default = true },
        { name = "mode", type = "select", label = "Mode", options = {"simple", "advanced"}, default = "simple" },
        { name = "count", type = "number", label = "Count", default = 5 },
    }
}

function init()
    mah.inject("page_top", function(ctx)
        local text = mah.get_setting("banner_text") or "Plugin Banner Active"
        return '<div data-testid="plugin-banner" style="background:yellow;padding:8px;text-align:center;">' .. text .. '</div>'
    end)

    mah.on("before_note_create", function(data)
        data.name = "[Plugin] " .. data.name
        return data
    end)

    mah.page("test-page", function(ctx)
        return '<div data-testid="plugin-page-content"><h2>Test Plugin Page</h2><p>Method: ' .. ctx.method .. '</p><p>Path: ' .. ctx.path .. '</p></div>'
    end)

    mah.page("echo-query", function(ctx)
        local q = ctx.query.msg or "no message"
        return '<div data-testid="plugin-echo">' .. q .. '</div>'
    end)

    mah.page("show-settings", function(ctx)
        local key = mah.get_setting("api_key") or "not-set"
        local mode = mah.get_setting("mode") or "not-set"
        local count = mah.get_setting("count")
        local countStr = count and tostring(count) or "not-set"
        return '<div data-testid="plugin-settings-display">'
            .. '<span data-testid="setting-api-key">' .. key .. '</span>'
            .. '<span data-testid="setting-mode">' .. mode .. '</span>'
            .. '<span data-testid="setting-count">' .. countStr .. '</span>'
            .. '</div>'
    end)

    mah.menu("Test Page", "test-page")
    mah.menu("Echo Query", "echo-query")
    mah.menu("Show Settings", "show-settings")
end
```

**Step 2: Commit**

```bash
git add e2e/test-plugins/test-banner/plugin.lua
git commit -m "test(plugins): add settings declarations to test-banner plugin"
```

---

### Task 12: E2E Tests for Plugin Management

**Files:**
- Create: `e2e/tests/plugins/plugin-manage.spec.ts`
- Modify: `e2e/tests/plugins/plugin-pages.spec.ts` (update for disabled-by-default)
- Modify: `e2e/helpers/api-client.ts` (add plugin API helpers)

**Step 1: Add plugin helpers to API client**

Add to `e2e/helpers/api-client.ts`:

```typescript
  // Plugin management
  async getPlugins(): Promise<any[]> {
    const response = await this.withRetry(() =>
      this.request.get(`${this.baseUrl}/v1/plugins/manage`)
    );
    return response.json();
  }

  async enablePlugin(name: string): Promise<void> {
    await this.withRetry(() =>
      this.request.post(`${this.baseUrl}/v1/plugin/enable`, {
        form: { name },
      })
    );
  }

  async disablePlugin(name: string): Promise<void> {
    await this.withRetry(() =>
      this.request.post(`${this.baseUrl}/v1/plugin/disable`, {
        form: { name },
      })
    );
  }

  async savePluginSettings(name: string, values: Record<string, any>): Promise<any> {
    const response = await this.withRetry(() =>
      this.request.post(`${this.baseUrl}/v1/plugin/settings?name=${encodeURIComponent(name)}`, {
        data: values,
      })
    );
    return response.json();
  }
```

**Step 2: Update existing plugin-pages tests**

Since plugins are now disabled by default, the existing `plugin-pages.spec.ts` tests need to enable the test-banner plugin first. Add a `beforeEach` or `beforeAll` that enables it:

```typescript
import { test, expect } from '../../fixtures/base.fixture';

test.describe('Plugin Pages', () => {
  test.beforeEach(async ({ apiClient }) => {
    // Ensure plugin has required settings and is enabled
    await apiClient.savePluginSettings('test-banner', {
      banner_text: 'Plugin Banner Active',
      api_key: 'test-key-123',
      show_banner: true,
      mode: 'simple',
      count: 5,
    });
    await apiClient.enablePlugin('test-banner');
  });

  test.afterEach(async ({ apiClient }) => {
    try {
      await apiClient.disablePlugin('test-banner');
    } catch {
      // Ignore if already disabled
    }
  });

  // ... existing tests stay the same ...
});
```

**Step 3: Create plugin management E2E tests**

Create `e2e/tests/plugins/plugin-manage.spec.ts`:

```typescript
import { test, expect } from '../../fixtures/base.fixture';

test.describe('Plugin Management', () => {
  test.beforeEach(async ({ apiClient }) => {
    // Ensure plugin is disabled at test start
    try {
      await apiClient.disablePlugin('test-banner');
    } catch {
      // Ignore if already disabled
    }
  });

  test('management page shows discovered plugins', async ({ page }) => {
    await page.goto('/plugins/manage');
    await page.waitForLoadState('load');
    const card = page.getByTestId('plugin-card-test-banner');
    await expect(card).toBeVisible();
    await expect(card).toContainText('test-banner');
    await expect(card).toContainText('v1.0');
  });

  test('management page shows settings form', async ({ page }) => {
    await page.goto('/plugins/manage');
    await page.waitForLoadState('load');
    const form = page.getByTestId('plugin-settings-test-banner');
    await expect(form).toBeVisible();
    await expect(form.getByTestId('setting-banner_text')).toBeVisible();
    await expect(form.getByTestId('setting-api_key')).toBeVisible();
    await expect(form.getByTestId('setting-show_banner')).toBeVisible();
    await expect(form.getByTestId('setting-mode')).toBeVisible();
    await expect(form.getByTestId('setting-count')).toBeVisible();
  });

  test('can enable a plugin after configuring required settings', async ({ page, apiClient }) => {
    // First save required settings
    await apiClient.savePluginSettings('test-banner', {
      api_key: 'my-test-key',
      banner_text: 'Test Banner',
      show_banner: true,
      mode: 'simple',
      count: 5,
    });

    await page.goto('/plugins/manage');
    await page.waitForLoadState('load');

    // Click enable
    const enableButton = page.getByTestId('plugin-toggle-test-banner');
    await enableButton.click();
    await page.waitForLoadState('load');

    // Should now show Disable button
    const disableButton = page.getByTestId('plugin-toggle-test-banner');
    await expect(disableButton).toContainText('Disable');
  });

  test('enable fails without required settings', async ({ apiClient }) => {
    // Don't set any settings, try to enable
    const plugins = await apiClient.getPlugins();
    const banner = plugins.find(p => p.name === 'test-banner');
    expect(banner).toBeDefined();

    // Try to enable without required settings - should fail
    try {
      await apiClient.enablePlugin('test-banner');
      // If it didn't throw, check it's not enabled
    } catch {
      // Expected to fail
    }
  });

  test('disabled plugin does not inject banner', async ({ page }) => {
    await page.goto('/notes');
    await page.waitForLoadState('load');
    // Banner should NOT be visible when plugin is disabled
    await expect(page.getByTestId('plugin-banner')).not.toBeVisible();
  });

  test('enabled plugin injects banner', async ({ page, apiClient }) => {
    await apiClient.savePluginSettings('test-banner', {
      api_key: 'key',
      banner_text: 'My Custom Banner',
      show_banner: true,
      mode: 'simple',
      count: 5,
    });
    await apiClient.enablePlugin('test-banner');

    await page.goto('/notes');
    await page.waitForLoadState('load');
    const banner = page.getByTestId('plugin-banner');
    await expect(banner).toBeVisible();
    await expect(banner).toContainText('My Custom Banner');
  });

  test('disabling plugin removes banner', async ({ page, apiClient }) => {
    // Enable first
    await apiClient.savePluginSettings('test-banner', {
      api_key: 'key',
      banner_text: 'Banner',
      show_banner: true,
      mode: 'simple',
      count: 5,
    });
    await apiClient.enablePlugin('test-banner');

    // Verify banner is there
    await page.goto('/notes');
    await page.waitForLoadState('load');
    await expect(page.getByTestId('plugin-banner')).toBeVisible();

    // Disable
    await apiClient.disablePlugin('test-banner');

    // Reload and verify banner is gone
    await page.reload();
    await page.waitForLoadState('load');
    await expect(page.getByTestId('plugin-banner')).not.toBeVisible();
  });

  test('plugin can read settings at runtime', async ({ page, apiClient }) => {
    await apiClient.savePluginSettings('test-banner', {
      api_key: 'secret-api-key',
      banner_text: 'Banner',
      show_banner: true,
      mode: 'advanced',
      count: 42,
    });
    await apiClient.enablePlugin('test-banner');

    await page.goto('/plugins/test-banner/show-settings');
    await page.waitForLoadState('load');

    const display = page.getByTestId('plugin-settings-display');
    await expect(display).toBeVisible();
    await expect(page.getByTestId('setting-api-key')).toContainText('secret-api-key');
    await expect(page.getByTestId('setting-mode')).toContainText('advanced');
    await expect(page.getByTestId('setting-count')).toContainText('42');
  });

  test('settings persist after page reload', async ({ page, apiClient }) => {
    await apiClient.savePluginSettings('test-banner', {
      api_key: 'persistent-key',
      banner_text: 'Persistent Banner',
      show_banner: true,
      mode: 'simple',
      count: 10,
    });

    await page.goto('/plugins/manage');
    await page.waitForLoadState('load');

    // Check the password field has the value
    const apiKeyInput = page.getByTestId('setting-api_key');
    await expect(apiKeyInput).toHaveValue('persistent-key');
  });

  test('Plugins dropdown always visible with manage link', async ({ page }) => {
    await page.goto('/notes');
    await page.waitForLoadState('load');
    const desktopNav = page.locator('.navbar-links');
    const pluginsButton = desktopNav.locator('button', { hasText: 'Plugins' });
    await expect(pluginsButton).toBeVisible();
    await pluginsButton.click();
    await expect(desktopNav.locator('a[href="/plugins/manage"]')).toBeVisible();
  });
});
```

**Step 4: Run E2E tests**

Run: `cd e2e && npm run test:with-server`
Expected: All PASS (may need iteration to fix details)

**Step 5: Commit**

```bash
git add e2e/tests/plugins/plugin-manage.spec.ts \
    e2e/tests/plugins/plugin-pages.spec.ts \
    e2e/helpers/api-client.ts
git commit -m "test(plugins): add E2E tests for plugin management page"
```

---

### Task 13: Run All Tests and Fix Issues

**Files:** Any files that need fixing

**Step 1: Run Go unit tests**

Run: `go test ./... --tags 'json1 fts5'`
Expected: All PASS

Fix any failures.

**Step 2: Run E2E tests**

Run: `cd e2e && npm run test:with-server`
Expected: All PASS

Fix any failures.

**Step 3: Build the full application**

Run: `npm run build`
Expected: Builds successfully (CSS + JS + Go binary)

**Step 4: Commit any fixes**

```bash
git add -A
git commit -m "fix(plugins): fix test failures from plugin activation changes"
```

---

### Task 14: Update Example Plugin

**Files:**
- Modify: `plugins/example-plugin/plugin.lua`

Update the example plugin to demonstrate settings:

```lua
plugin = {
    name = "example-plugin",
    version = "1.0",
    description = "Example Lua plugin demonstrating all plugin capabilities",
    settings = {
        { name = "greeting", type = "string", label = "Greeting Message", default = "Hello from Example Plugin!" },
        { name = "show_footer", type = "boolean", label = "Show Footer Banner", default = true },
    }
}

function init()
    mah.inject("page_bottom", function(ctx)
        local show = mah.get_setting("show_footer")
        if show == false then return "" end
        local greeting = mah.get_setting("greeting") or "Powered by plugins"
        return '<div style="text-align:center;padding:4px;color:#888;font-size:12px;">' .. greeting .. '</div>'
    end)

    mah.on("after_note_create", function(note)
        mah.log("info", "Note created: " .. (note.name or "unknown"))
    end)

    mah.on("after_resource_create", function(resource)
        mah.log("info", "Resource created: " .. (resource.name or "unknown"))
    end)

    mah.page("info", function(ctx)
        local greeting = mah.get_setting("greeting") or "Hello!"
        return "<h2>Example Plugin</h2><p>" .. greeting .. "</p><p>This page is rendered by Lua.</p>"
    end)

    mah.menu("Plugin Info", "info")
end
```

**Step 1: Commit**

```bash
git add plugins/example-plugin/plugin.lua
git commit -m "docs(plugins): update example plugin with settings demonstration"
```

---

### Task 15: Final Verification

**Step 1: Full build**

Run: `npm run build`
Expected: Success

**Step 2: All Go tests**

Run: `go test ./... --tags 'json1 fts5'`
Expected: All PASS

**Step 3: All E2E tests**

Run: `cd e2e && npm run test:with-server`
Expected: All PASS

**Step 4: Manual smoke test**

Run: `./mahresources -ephemeral -bind-address=:8181`

1. Navigate to `/plugins/manage` — should see test plugins listed as disabled
2. Configure settings for a plugin
3. Enable it — should see banner appear
4. Disable it — banner should disappear
5. Verify settings persist across page reloads
