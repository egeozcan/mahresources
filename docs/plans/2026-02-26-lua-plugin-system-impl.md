# Lua Plugin System Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a Lua-based plugin system that lets plugins inject HTML/JS/CSS at named template slots and hook into entity CRUD lifecycle events, with read-only database access.

**Architecture:** Each plugin lives in its own directory with a `plugin.lua` entry point. At startup, a plugin manager scans the plugin directory, creates an isolated gopher-lua VM per plugin, registers the `mah` Go→Lua API, and calls each plugin's `init()`. The plugin manager is stored on `MahresourcesContext` and called from CRUD methods (hooks) and template rendering (injection slots).

**Tech Stack:** gopher-lua (pure Go Lua 5.1 VM), pongo2 (template integration), existing GORM context layer

---

### Task 1: Add gopher-lua dependency

**Files:**
- Modify: `go.mod`

**Step 1: Add the dependency**

Run:
```bash
go get github.com/yuin/gopher-lua
```

**Step 2: Verify it resolves**

Run:
```bash
go mod tidy
```

**Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "deps: add gopher-lua for plugin system"
```

---

### Task 2: Plugin manager core — loading, lifecycle, hook registration

This is the heart of the system. Build the plugin manager that scans a directory for plugins, creates Lua VMs, and collects hook/injection registrations.

**Files:**
- Create: `plugin_system/manager.go`
- Create: `plugin_system/manager_test.go`

**Step 1: Write the test file**

```go
package plugin_system

import (
	"os"
	"path/filepath"
	"testing"
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

func TestLoadPlugins_Empty(t *testing.T) {
	dir := t.TempDir()
	mgr, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()
	if len(mgr.Plugins()) != 0 {
		t.Errorf("expected 0 plugins, got %d", len(mgr.Plugins()))
	}
}

func TestLoadPlugins_SinglePlugin(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "test-plugin", `
plugin = {
	name = "test-plugin",
	version = "1.0",
	description = "A test plugin"
}

function init()
end
`)

	mgr, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()

	if len(mgr.Plugins()) != 1 {
		t.Fatalf("expected 1 plugin, got %d", len(mgr.Plugins()))
	}
	p := mgr.Plugins()[0]
	if p.Name != "test-plugin" {
		t.Errorf("expected name 'test-plugin', got %q", p.Name)
	}
}

func TestLoadPlugins_BadSyntaxSkipped(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "bad-plugin", `this is not valid lua !!!`)
	writePlugin(t, dir, "good-plugin", `
plugin = { name = "good", version = "1.0", description = "ok" }
function init() end
`)

	mgr, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()

	if len(mgr.Plugins()) != 1 {
		t.Errorf("expected 1 plugin (bad skipped), got %d", len(mgr.Plugins()))
	}
}

func TestLoadPlugins_AlphabeticalOrder(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "charlie", `
plugin = { name = "charlie", version = "1.0", description = "c" }
function init() end
`)
	writePlugin(t, dir, "alpha", `
plugin = { name = "alpha", version = "1.0", description = "a" }
function init() end
`)
	writePlugin(t, dir, "bravo", `
plugin = { name = "bravo", version = "1.0", description = "b" }
function init() end
`)

	mgr, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()

	plugins := mgr.Plugins()
	if len(plugins) != 3 {
		t.Fatalf("expected 3, got %d", len(plugins))
	}
	if plugins[0].Name != "alpha" || plugins[1].Name != "bravo" || plugins[2].Name != "charlie" {
		t.Errorf("wrong order: %v, %v, %v", plugins[0].Name, plugins[1].Name, plugins[2].Name)
	}
}

func TestHookRegistration(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "hooker", `
plugin = { name = "hooker", version = "1.0", description = "hooks" }
function my_handler(entity)
end
function init()
	mah.on("before_note_create", my_handler)
	mah.on("after_note_create", my_handler)
end
`)

	mgr, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()

	if len(mgr.GetHooks("before_note_create")) != 1 {
		t.Error("expected 1 before_note_create hook")
	}
	if len(mgr.GetHooks("after_note_create")) != 1 {
		t.Error("expected 1 after_note_create hook")
	}
	if len(mgr.GetHooks("before_resource_create")) != 0 {
		t.Error("expected 0 before_resource_create hooks")
	}
}

func TestInjectionRegistration(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "injector", `
plugin = { name = "injector", version = "1.0", description = "injects" }
function init()
	mah.inject("head", function(ctx)
		return "<style>body { color: red; }</style>"
	end)
	mah.inject("resource_detail_sidebar", function(ctx)
		return "<div>custom</div>"
	end)
end
`)

	mgr, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()

	if len(mgr.GetInjections("head")) != 1 {
		t.Error("expected 1 head injection")
	}
	if len(mgr.GetInjections("resource_detail_sidebar")) != 1 {
		t.Error("expected 1 resource_detail_sidebar injection")
	}
	if len(mgr.GetInjections("page_bottom")) != 0 {
		t.Error("expected 0 page_bottom injections")
	}
}

func TestNonexistentDirReturnsEmpty(t *testing.T) {
	mgr, err := NewPluginManager("/nonexistent/path/that/does/not/exist")
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()
	if len(mgr.Plugins()) != 0 {
		t.Errorf("expected 0 plugins for nonexistent dir, got %d", len(mgr.Plugins()))
	}
}
```

**Step 2: Run the tests — verify they fail**

Run:
```bash
go test ./plugin_system/... --tags 'json1 fts5'
```
Expected: Compilation failure — `plugin_system` package doesn't exist yet.

**Step 3: Implement the plugin manager**

Create `plugin_system/manager.go`:

```go
package plugin_system

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"sync"

	lua "github.com/yuin/gopher-lua"
)

// PluginInfo holds metadata about a loaded plugin.
type PluginInfo struct {
	Name        string
	Version     string
	Description string
	Dir         string
}

// hookEntry ties a Lua callback to the VM it lives in.
type hookEntry struct {
	state *lua.LState
	fn    *lua.LFunction
}

// injectionEntry ties a Lua render function to its VM.
type injectionEntry struct {
	state *lua.LState
	fn    *lua.LFunction
}

// PluginManager loads and manages Lua plugins.
type PluginManager struct {
	plugins    []PluginInfo
	states     []*lua.LState
	hooks      map[string][]hookEntry
	injections map[string][]injectionEntry
	mu         sync.RWMutex
}

// NewPluginManager scans dir for plugin directories, loads each plugin.lua,
// and collects hook/injection registrations. Plugins that fail to load are
// skipped with a warning. If dir does not exist, returns an empty manager.
func NewPluginManager(dir string) (*PluginManager, error) {
	mgr := &PluginManager{
		hooks:      make(map[string][]hookEntry),
		injections: make(map[string][]injectionEntry),
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return mgr, nil
		}
		return nil, fmt.Errorf("reading plugin directory %s: %w", dir, err)
	}

	// Sort alphabetically for deterministic load order
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		pluginFile := filepath.Join(dir, entry.Name(), "plugin.lua")
		if _, err := os.Stat(pluginFile); err != nil {
			continue
		}

		if loadErr := mgr.loadPlugin(pluginFile, entry.Name()); loadErr != nil {
			log.Printf("[plugin] WARNING: failed to load plugin %q: %v", entry.Name(), loadErr)
		}
	}

	return mgr, nil
}

func (mgr *PluginManager) loadPlugin(pluginFile, dirName string) error {
	L := lua.NewState()

	// Register the mah module before executing plugin code
	mgr.registerMahModule(L)

	if err := L.DoFile(pluginFile); err != nil {
		L.Close()
		return fmt.Errorf("executing %s: %w", pluginFile, err)
	}

	// Read plugin metadata table
	pluginTable := L.GetGlobal("plugin")
	tbl, ok := pluginTable.(*lua.LTable)
	if !ok {
		L.Close()
		return fmt.Errorf("plugin global is not a table in %s", pluginFile)
	}

	info := PluginInfo{
		Name:        lua.LVAsString(tbl.RawGetString("name")),
		Version:     lua.LVAsString(tbl.RawGetString("version")),
		Description: lua.LVAsString(tbl.RawGetString("description")),
		Dir:         dirName,
	}

	if info.Name == "" {
		L.Close()
		return fmt.Errorf("plugin.name is empty in %s", pluginFile)
	}

	// Call init() if it exists
	initFn := L.GetGlobal("init")
	if initFn != lua.LNil {
		if err := L.CallByParam(lua.P{Fn: initFn, NRet: 0, Protect: true}); err != nil {
			L.Close()
			return fmt.Errorf("calling init() in %s: %w", pluginFile, err)
		}
	}

	mgr.plugins = append(mgr.plugins, info)
	mgr.states = append(mgr.states, L)

	log.Printf("[plugin] Loaded %q v%s", info.Name, info.Version)
	return nil
}

func (mgr *PluginManager) registerMahModule(L *lua.LState) {
	mahMod := L.NewTable()

	// mah.on(event_name, handler_function)
	L.SetField(mahMod, "on", L.NewFunction(func(L *lua.LState) int {
		event := L.CheckString(1)
		fn := L.CheckFunction(2)
		mgr.mu.Lock()
		mgr.hooks[event] = append(mgr.hooks[event], hookEntry{state: L, fn: fn})
		mgr.mu.Unlock()
		return 0
	}))

	// mah.inject(slot_name, render_function)
	L.SetField(mahMod, "inject", L.NewFunction(func(L *lua.LState) int {
		slot := L.CheckString(1)
		fn := L.CheckFunction(2)
		mgr.mu.Lock()
		mgr.injections[slot] = append(mgr.injections[slot], injectionEntry{state: L, fn: fn})
		mgr.mu.Unlock()
		return 0
	}))

	// mah.log(level, message)
	L.SetField(mahMod, "log", L.NewFunction(func(L *lua.LState) int {
		level := L.CheckString(1)
		message := L.CheckString(2)
		log.Printf("[plugin:%s] %s", level, message)
		return 0
	}))

	// mah.abort(reason) — used inside before-hooks to cancel operations
	L.SetField(mahMod, "abort", L.NewFunction(func(L *lua.LState) int {
		reason := L.CheckString(1)
		L.RaiseError("PLUGIN_ABORT:%s", reason)
		return 0
	}))

	L.SetGlobal("mah", mahMod)
}

// Plugins returns metadata for all loaded plugins.
func (mgr *PluginManager) Plugins() []PluginInfo {
	return mgr.plugins
}

// GetHooks returns all registered hooks for an event name.
func (mgr *PluginManager) GetHooks(event string) []hookEntry {
	mgr.mu.RLock()
	defer mgr.mu.RUnlock()
	return mgr.hooks[event]
}

// GetInjections returns all registered injections for a slot name.
func (mgr *PluginManager) GetInjections(slot string) []injectionEntry {
	mgr.mu.RLock()
	defer mgr.mu.RUnlock()
	return mgr.injections[slot]
}

// Close shuts down all Lua VMs.
func (mgr *PluginManager) Close() {
	for _, L := range mgr.states {
		L.Close()
	}
}
```

**Step 4: Run the tests — verify they pass**

Run:
```bash
go test ./plugin_system/... --tags 'json1 fts5' -v
```
Expected: All tests pass.

**Step 5: Commit**

```bash
git add plugin_system/
git commit -m "feat: add plugin manager core — loading, hooks, injections"
```

---

### Task 3: Hook execution — RunBeforeHooks and RunAfterHooks

Add methods to PluginManager that execute registered hooks, converting Go data to/from Lua tables.

**Files:**
- Modify: `plugin_system/manager.go` (add hook execution methods)
- Create: `plugin_system/hooks.go` (entity↔Lua conversion and hook runners)
- Create: `plugin_system/hooks_test.go`

**Step 1: Write the test file**

```go
package plugin_system

import (
	"testing"
)

func TestRunBeforeHooks_ModifiesFields(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "modifier", `
plugin = { name = "modifier", version = "1.0", description = "modifies" }
function handler(entity)
	entity.name = entity.name .. " [modified]"
end
function init()
	mah.on("before_note_create", handler)
end
`)

	mgr, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()

	data := map[string]any{
		"name":        "Test Note",
		"description": "A note",
	}

	result, err := mgr.RunBeforeHooks("before_note_create", data)
	if err != nil {
		t.Fatal(err)
	}

	if result["name"] != "Test Note [modified]" {
		t.Errorf("expected modified name, got %q", result["name"])
	}
}

func TestRunBeforeHooks_Abort(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "blocker", `
plugin = { name = "blocker", version = "1.0", description = "blocks" }
function handler(entity)
	mah.abort("not allowed")
end
function init()
	mah.on("before_note_create", handler)
end
`)

	mgr, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()

	data := map[string]any{"name": "Test"}

	_, err = mgr.RunBeforeHooks("before_note_create", data)
	if err == nil {
		t.Fatal("expected abort error, got nil")
	}

	abortErr, ok := err.(*PluginAbortError)
	if !ok {
		t.Fatalf("expected PluginAbortError, got %T: %v", err, err)
	}
	if abortErr.Reason != "not allowed" {
		t.Errorf("expected reason 'not allowed', got %q", abortErr.Reason)
	}
}

func TestRunBeforeHooks_RuntimeErrorSkipped(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "bad", `
plugin = { name = "bad", version = "1.0", description = "crashes" }
function handler(entity)
	error("oops")
end
function init()
	mah.on("before_note_create", handler)
end
`)

	mgr, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()

	data := map[string]any{"name": "Test"}
	result, err := mgr.RunBeforeHooks("before_note_create", data)
	if err != nil {
		t.Fatalf("runtime errors should be skipped, got: %v", err)
	}
	if result["name"] != "Test" {
		t.Errorf("data should be unchanged after skipped hook, got %q", result["name"])
	}
}

func TestRunAfterHooks_NoError(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "logger", `
plugin = { name = "logger", version = "1.0", description = "logs" }
logged = false
function handler(entity)
	logged = true
end
function init()
	mah.on("after_note_create", handler)
end
`)

	mgr, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()

	data := map[string]any{"id": float64(42), "name": "Test"}
	mgr.RunAfterHooks("after_note_create", data)
	// No error expected — after-hooks are fire-and-forget
}

func TestRunBeforeHooks_MultiplePluginsOrder(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "01-first", `
plugin = { name = "first", version = "1.0", description = "1st" }
function handler(entity)
	entity.name = entity.name .. "-first"
end
function init()
	mah.on("before_note_create", handler)
end
`)
	writePlugin(t, dir, "02-second", `
plugin = { name = "second", version = "1.0", description = "2nd" }
function handler(entity)
	entity.name = entity.name .. "-second"
end
function init()
	mah.on("before_note_create", handler)
end
`)

	mgr, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()

	data := map[string]any{"name": "base"}
	result, err := mgr.RunBeforeHooks("before_note_create", data)
	if err != nil {
		t.Fatal(err)
	}

	if result["name"] != "base-first-second" {
		t.Errorf("expected 'base-first-second', got %q", result["name"])
	}
}

func TestRunBeforeHooks_NoHooksRegistered(t *testing.T) {
	dir := t.TempDir()
	mgr, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()

	data := map[string]any{"name": "Test"}
	result, err := mgr.RunBeforeHooks("before_note_create", data)
	if err != nil {
		t.Fatal(err)
	}
	if result["name"] != "Test" {
		t.Errorf("data should be unchanged, got %q", result["name"])
	}
}
```

**Step 2: Run tests — verify they fail**

Run:
```bash
go test ./plugin_system/... --tags 'json1 fts5'
```
Expected: Compilation failure — `RunBeforeHooks`, `RunAfterHooks`, `PluginAbortError` don't exist.

**Step 3: Implement hooks.go**

Create `plugin_system/hooks.go`:

```go
package plugin_system

import (
	"fmt"
	"log"
	"strings"

	lua "github.com/yuin/gopher-lua"
)

// PluginAbortError is returned when a before-hook calls mah.abort().
type PluginAbortError struct {
	Reason string
}

func (e *PluginAbortError) Error() string {
	return fmt.Sprintf("plugin abort: %s", e.Reason)
}

// goToLuaTable converts a Go map to a Lua table.
func goToLuaTable(L *lua.LState, data map[string]any) *lua.LTable {
	tbl := L.NewTable()
	for k, v := range data {
		tbl.RawSetString(k, goToLuaValue(L, v))
	}
	return tbl
}

// goToLuaValue converts a Go value to a Lua value.
func goToLuaValue(L *lua.LState, v any) lua.LValue {
	if v == nil {
		return lua.LNil
	}
	switch val := v.(type) {
	case string:
		return lua.LString(val)
	case float64:
		return lua.LNumber(val)
	case float32:
		return lua.LNumber(float64(val))
	case int:
		return lua.LNumber(float64(val))
	case int64:
		return lua.LNumber(float64(val))
	case uint:
		return lua.LNumber(float64(val))
	case uint64:
		return lua.LNumber(float64(val))
	case bool:
		return lua.LBool(val)
	case map[string]any:
		return goToLuaTable(L, val)
	case []any:
		tbl := L.NewTable()
		for i, item := range val {
			tbl.RawSetInt(i+1, goToLuaValue(L, item))
		}
		return tbl
	default:
		return lua.LString(fmt.Sprintf("%v", val))
	}
}

// luaTableToGoMap converts a Lua table back to a Go map.
func luaTableToGoMap(tbl *lua.LTable) map[string]any {
	result := make(map[string]any)
	tbl.ForEach(func(key lua.LValue, value lua.LValue) {
		if keyStr, ok := key.(lua.LString); ok {
			result[string(keyStr)] = luaValueToGo(value)
		}
	})
	return result
}

// luaValueToGo converts a Lua value to a Go value.
func luaValueToGo(v lua.LValue) any {
	switch val := v.(type) {
	case lua.LBool:
		return bool(val)
	case lua.LNumber:
		return float64(val)
	case lua.LString:
		return string(val)
	case *lua.LTable:
		return luaTableToGoMap(val)
	case *lua.LNilType:
		return nil
	default:
		return val.String()
	}
}

// RunBeforeHooks runs all before-hooks for an event. Returns the (possibly modified)
// entity data. Returns a PluginAbortError if a hook calls mah.abort().
// Runtime errors in hooks are logged and skipped.
func (mgr *PluginManager) RunBeforeHooks(event string, data map[string]any) (map[string]any, error) {
	hooks := mgr.GetHooks(event)
	if len(hooks) == 0 {
		return data, nil
	}

	current := data
	for _, hook := range hooks {
		tbl := goToLuaTable(hook.state, current)
		if err := hook.state.CallByParam(lua.P{
			Fn:      hook.fn,
			NRet:    0,
			Protect: true,
		}, tbl); err != nil {
			// Check if this is an abort
			if isAbort, reason := parseAbortError(err); isAbort {
				return nil, &PluginAbortError{Reason: reason}
			}
			// Runtime error — log and skip
			log.Printf("[plugin] WARNING: hook %q error: %v", event, err)
			continue
		}
		// Read back potentially modified data
		current = luaTableToGoMap(tbl)
	}

	return current, nil
}

// RunAfterHooks runs all after-hooks for an event. Errors are logged and ignored.
func (mgr *PluginManager) RunAfterHooks(event string, data map[string]any) {
	hooks := mgr.GetHooks(event)
	for _, hook := range hooks {
		tbl := goToLuaTable(hook.state, data)
		if err := hook.state.CallByParam(lua.P{
			Fn:      hook.fn,
			NRet:    0,
			Protect: true,
		}, tbl); err != nil {
			log.Printf("[plugin] WARNING: after-hook %q error: %v", event, err)
		}
	}
}

// parseAbortError checks if a Lua error is a mah.abort() call.
func parseAbortError(err error) (bool, string) {
	msg := err.Error()
	const prefix = "PLUGIN_ABORT:"
	if idx := strings.Index(msg, prefix); idx >= 0 {
		return true, strings.TrimSpace(msg[idx+len(prefix):])
	}
	return false, ""
}
```

**Step 4: Run tests — verify they pass**

Run:
```bash
go test ./plugin_system/... --tags 'json1 fts5' -v
```
Expected: All tests pass.

**Step 5: Commit**

```bash
git add plugin_system/
git commit -m "feat: add hook execution with before/after, abort, and error handling"
```

---

### Task 4: Injection rendering — RenderSlot

Add the method that executes injection render functions and concatenates their output.

**Files:**
- Create: `plugin_system/injections.go`
- Create: `plugin_system/injections_test.go`

**Step 1: Write the test file**

```go
package plugin_system

import (
	"testing"
)

func TestRenderSlot_SinglePlugin(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "sidebar", `
plugin = { name = "sidebar", version = "1.0", description = "sidebar" }
function init()
	mah.inject("resource_detail_sidebar", function(ctx)
		return "<div>Hello " .. ctx.path .. "</div>"
	end)
end
`)

	mgr, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()

	ctx := map[string]any{"path": "/resource?id=42"}
	result := mgr.RenderSlot("resource_detail_sidebar", ctx)
	expected := "<div>Hello /resource?id=42</div>"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestRenderSlot_MultiplePlugins(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "01-first", `
plugin = { name = "first", version = "1.0", description = "1" }
function init()
	mah.inject("head", function(ctx) return "<style>a{}</style>" end)
end
`)
	writePlugin(t, dir, "02-second", `
plugin = { name = "second", version = "1.0", description = "2" }
function init()
	mah.inject("head", function(ctx) return "<script></script>" end)
end
`)

	mgr, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()

	result := mgr.RenderSlot("head", map[string]any{})
	if result != "<style>a{}</style><script></script>" {
		t.Errorf("expected concatenated output, got %q", result)
	}
}

func TestRenderSlot_ErrorSkipped(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "01-bad", `
plugin = { name = "bad", version = "1.0", description = "crashes" }
function init()
	mah.inject("head", function(ctx) error("boom") end)
end
`)
	writePlugin(t, dir, "02-good", `
plugin = { name = "good", version = "1.0", description = "ok" }
function init()
	mah.inject("head", function(ctx) return "<meta>" end)
end
`)

	mgr, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()

	result := mgr.RenderSlot("head", map[string]any{})
	if result != "<meta>" {
		t.Errorf("expected only good plugin output, got %q", result)
	}
}

func TestRenderSlot_EmptySlot(t *testing.T) {
	dir := t.TempDir()
	mgr, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()

	result := mgr.RenderSlot("nonexistent", map[string]any{})
	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

func TestRenderSlot_WithEntityContext(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "entity-aware", `
plugin = { name = "entity-aware", version = "1.0", description = "reads entity" }
function init()
	mah.inject("note_detail_after", function(ctx)
		if ctx.entity then
			return "<p>" .. ctx.entity.name .. "</p>"
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

	ctx := map[string]any{
		"entity": map[string]any{
			"id":   float64(1),
			"name": "My Note",
		},
		"path": "/note?id=1",
	}
	result := mgr.RenderSlot("note_detail_after", ctx)
	if result != "<p>My Note</p>" {
		t.Errorf("expected '<p>My Note</p>', got %q", result)
	}
}
```

**Step 2: Run tests — verify they fail**

Run:
```bash
go test ./plugin_system/... --tags 'json1 fts5'
```
Expected: Compilation failure — `RenderSlot` doesn't exist.

**Step 3: Implement injections.go**

Create `plugin_system/injections.go`:

```go
package plugin_system

import (
	"log"
	"strings"

	lua "github.com/yuin/gopher-lua"
)

// RenderSlot executes all render functions registered for a slot and
// concatenates their output. Errors in individual renderers are logged
// and their output is skipped.
func (mgr *PluginManager) RenderSlot(slot string, ctx map[string]any) string {
	injections := mgr.GetInjections(slot)
	if len(injections) == 0 {
		return ""
	}

	var sb strings.Builder

	for _, inj := range injections {
		ctxTable := goToLuaTable(inj.state, ctx)
		if err := inj.state.CallByParam(lua.P{
			Fn:      inj.fn,
			NRet:    1,
			Protect: true,
		}, ctxTable); err != nil {
			log.Printf("[plugin] WARNING: injection %q render error: %v", slot, err)
			continue
		}

		result := inj.state.Get(-1)
		inj.state.Pop(1)

		if str, ok := result.(lua.LString); ok {
			sb.WriteString(string(str))
		}
	}

	return sb.String()
}
```

**Step 4: Run tests — verify they pass**

Run:
```bash
go test ./plugin_system/... --tags 'json1 fts5' -v
```
Expected: All tests pass.

**Step 5: Commit**

```bash
git add plugin_system/
git commit -m "feat: add injection slot rendering"
```

---

### Task 5: Configuration — add plugin flags

Add `-plugin-path` and `-plugins-disabled` flags to main.go and the config structs.

**Files:**
- Modify: `main.go` (add flags, ~lines 84-135, and config building ~lines 173-196)
- Modify: `application_context/context.go` (add fields to `MahresourcesInputConfig` ~line 63 and `MahresourcesConfig` ~line 33)

**Step 1: Add fields to `MahresourcesInputConfig`**

In `application_context/context.go`, add to the end of `MahresourcesInputConfig` struct (before the closing brace ~line 98):

```go
PluginPath      string
PluginsDisabled bool
```

**Step 2: Add fields to `MahresourcesConfig`**

In `application_context/context.go`, add to `MahresourcesConfig` struct:

```go
PluginPath      string
PluginsDisabled bool
```

**Step 3: Add flags in `main.go`**

Add after existing flag definitions (~line 135):

```go
pluginPath := flag.String("plugin-path", getEnvOrDefault("PLUGIN_PATH", "./plugins"), "path to plugin directory")
pluginsDisabled := flag.Bool("plugins-disabled", os.Getenv("PLUGINS_DISABLED") == "1", "disable all plugins")
```

**Step 4: Wire into config struct**

In the `MahresourcesInputConfig` initialization block (~lines 173-196), add:

```go
PluginPath:      *pluginPath,
PluginsDisabled: *pluginsDisabled,
```

**Step 5: Pass through to `MahresourcesConfig`**

In `context.go` where `MahresourcesConfig` is built inside `CreateContextWithConfig` (or `NewMahresourcesContext`), pass through:

```go
PluginPath:      cfg.PluginPath,
PluginsDisabled: cfg.PluginsDisabled,
```

**Step 6: Verify it compiles**

Run:
```bash
go build --tags 'json1 fts5'
```
Expected: Compiles successfully.

**Step 7: Commit**

```bash
git add main.go application_context/context.go
git commit -m "feat: add plugin-path and plugins-disabled configuration flags"
```

---

### Task 6: Wire plugin manager into MahresourcesContext

Initialize the PluginManager at startup and store it on the context so all layers can access it.

**Files:**
- Modify: `application_context/context.go` (add PluginManager field, initialize in constructor)
- Modify: `main.go` (close plugin manager on shutdown)

**Step 1: Add PluginManager to MahresourcesContext**

In `application_context/context.go`, add to the `MahresourcesContext` struct (~line 108):

```go
pluginManager *plugin_system.PluginManager
```

Add import for `mahresources/plugin_system`.

**Step 2: Add accessor method**

Add to `context.go`:

```go
// PluginManager returns the plugin manager, or nil if plugins are disabled.
func (ctx *MahresourcesContext) PluginManager() *plugin_system.PluginManager {
	return ctx.pluginManager
}
```

**Step 3: Initialize in context creation**

In `NewMahresourcesContext` or `CreateContextWithConfig`, after the context is created, add:

```go
if !cfg.PluginsDisabled {
	pm, pmErr := plugin_system.NewPluginManager(cfg.PluginPath)
	if pmErr != nil {
		log.Printf("[plugin] WARNING: failed to initialize plugin system: %v", pmErr)
	} else {
		ctx.pluginManager = pm
	}
}
```

**Step 4: Add Close to main.go**

In `main.go`, in the shutdown path (look for `srv.Shutdown` or deferred cleanup), add:

```go
if context.PluginManager() != nil {
	context.PluginManager().Close()
}
```

**Step 5: Verify it compiles**

Run:
```bash
go build --tags 'json1 fts5'
```

**Step 6: Run existing tests to check nothing is broken**

Run:
```bash
go test ./... --tags 'json1 fts5'
```

**Step 7: Commit**

```bash
git add application_context/context.go main.go
git commit -m "feat: wire plugin manager into application context"
```

---

### Task 7: Integrate hooks into entity CRUD methods

Add before/after hook calls to the existing CRUD methods for all entities. This is the largest integration task.

**Files:**
- Modify: `application_context/note_context.go` (CreateOrUpdateNote ~line 17, DeleteNote ~line 186)
- Modify: `application_context/group_crud_context.go` (CreateGroup ~line 14, UpdateGroup ~line 83, DeleteGroup ~line 228)
- Modify: `application_context/tags_context.go` (CreateTag ~line 55, UpdateTag ~line 75, DeleteTag ~line 96)
- Modify: `application_context/category_context.go` (CreateCategory ~line 48, UpdateCategory ~line 73, DeleteCategory ~line 99)
- Modify: `application_context/resource_upload_context.go` (AddResource ~line 387)
- Modify: `application_context/resource_crud_context.go` (EditResource ~line 146)
- Modify: `application_context/resource_bulk_context.go` (DeleteResource ~line 22)

**Pattern for each method:**

Add a helper method to `context.go` to avoid repetition:

```go
// runBeforeHooks runs before-hooks if the plugin manager is active.
// Returns the (possibly modified) data, or an error if a plugin aborted.
func (ctx *MahresourcesContext) runBeforeHooks(event string, data map[string]any) (map[string]any, error) {
	if ctx.pluginManager == nil {
		return data, nil
	}
	return ctx.pluginManager.RunBeforeHooks(event, data)
}

// runAfterHooks runs after-hooks if the plugin manager is active.
func (ctx *MahresourcesContext) runAfterHooks(event string, data map[string]any) {
	if ctx.pluginManager == nil {
		return
	}
	ctx.pluginManager.RunAfterHooks(event, data)
}
```

**For each entity, the pattern is:**

**Before-hooks** — call before the DB operation, convert the query DTO or model to a `map[string]any`, run hooks, apply any modifications back.

**After-hooks** — call after the commit and logging, convert the saved model to a `map[string]any`.

Example for `CreateOrUpdateNote` at line 17 of `note_context.go`:

Before `tx := ctx.db.Begin()` (line 33), add:
```go
// Determine if this is create or update
hookEvent := "before_note_create"
if noteQuery.ID != 0 {
	hookEvent = "before_note_update"
}
hookData := map[string]any{
	"id":          float64(noteQuery.ID),
	"name":        noteQuery.Name,
	"description": noteQuery.Description,
	"meta":        noteQuery.Meta,
}
hookData, hookErr := ctx.runBeforeHooks(hookEvent, hookData)
if hookErr != nil {
	return nil, hookErr
}
// Apply modifications from plugins
if name, ok := hookData["name"].(string); ok {
	noteQuery.Name = name
}
if desc, ok := hookData["description"].(string); ok {
	noteQuery.Description = desc
}
```

After the logging block (~line 131), add:
```go
afterEvent := "after_note_create"
if noteQuery.ID != 0 {
	afterEvent = "after_note_update"
}
ctx.runAfterHooks(afterEvent, map[string]any{
	"id":          float64(note.ID),
	"name":        note.Name,
	"description": note.Description,
})
```

Apply the same pattern to:
- `CreateGroup` / `UpdateGroup` / `DeleteGroup` in `group_crud_context.go`
- `CreateTag` / `UpdateTag` / `DeleteTag` in `tags_context.go`
- `CreateCategory` / `UpdateCategory` / `DeleteCategory` in `category_context.go`
- `AddResource` in `resource_upload_context.go`
- `EditResource` in `resource_crud_context.go`
- `DeleteResource` in `resource_bulk_context.go`

For **delete** methods, the before-hook receives `{"id": float64(id)}` and can abort. The after-hook also receives `{"id": float64(id), "name": savedName}`.

**Step 1: Add helper methods to context.go**

Add the `runBeforeHooks` and `runAfterHooks` helpers as shown above.

**Step 2: Integrate into each CRUD method**

Follow the pattern above for each method. This is mechanical but must be done carefully.

**Step 3: Verify it compiles**

Run:
```bash
go build --tags 'json1 fts5'
```

**Step 4: Run all tests**

Run:
```bash
go test ./... --tags 'json1 fts5'
```

**Step 5: Commit**

```bash
git add application_context/
git commit -m "feat: integrate plugin hooks into all entity CRUD methods"
```

---

### Task 8: Template integration — plugin_slot function

Register a pongo2 global function `plugin_slot` that calls `PluginManager.RenderSlot`.

**Files:**
- Create: `server/template_handlers/template_filters/plugin_slot.go`
- Modify: `server/template_handlers/render_template.go` (~line 29, to pass plugin manager into template context)
- Modify: `server/routes.go` (~line 81, to pass plugin manager to RenderTemplate)

**Implementation approach:** The cleanest way is to add a `plugin_slot` pongo2 filter or use a context variable that's a callable. Since pongo2 doesn't natively support function calls with arguments in `{{ }}` syntax, we'll use a **pongo2 custom tag** instead: `{% plugin_slot "head" %}`.

Alternatively, and more simply, we can pass the plugin manager's RenderSlot output for each relevant slot as pre-computed template context variables. But this is wasteful — we'd compute all slots even if a page doesn't use them.

The best approach: **register a pongo2 custom tag** `plugin_slot` that takes a slot name and renders it.

**Step 1: Create the custom tag**

Create `server/template_handlers/template_filters/plugin_slot.go`:

```go
package template_filters

import (
	"github.com/flosch/pongo2/v4"
	"mahresources/plugin_system"
)

type pluginSlotNode struct {
	slotName string
}

func (node *pluginSlotNode) Execute(ctx *pongo2.ExecutionContext, writer pongo2.TemplateWriter) *pongo2.Error {
	// Get plugin manager from template context
	pmVal, ok := ctx.Public["_pluginManager"]
	if !ok || pmVal == nil {
		return nil
	}
	pm, ok := pmVal.(*plugin_system.PluginManager)
	if !ok || pm == nil {
		return nil
	}

	// Build slot context from template context
	slotCtx := make(map[string]any)
	if path, ok := ctx.Public["currentPath"].(string); ok {
		slotCtx["path"] = path
	}

	// Pass entity context for detail pages
	for _, key := range []string{"resource", "note", "group", "tag", "category"} {
		if entity, ok := ctx.Public[key]; ok && entity != nil {
			slotCtx["entity"] = entityToMap(entity)
			break
		}
	}

	// Pass entities for list pages
	for _, key := range []string{"resources", "notes", "groups", "tags", "categories"} {
		if entities, ok := ctx.Public[key]; ok && entities != nil {
			slotCtx["entities_key"] = key
			break
		}
	}

	html := pm.RenderSlot(node.slotName, slotCtx)
	if html != "" {
		writer.WriteString(html)
	}
	return nil
}

// entityToMap converts common entity fields to a map for Lua.
// Uses type assertion to handle different model types.
func entityToMap(entity any) map[string]any {
	result := make(map[string]any)

	// Use reflection or type switches for the known model types
	// For now, use a simple interface check
	type named interface {
		GetName() string
	}
	type identified interface {
		GetID() uint
	}

	// We'll refine this in implementation — the key fields are id, name, description
	return result
}

func pluginSlotTagParser(doc *pongo2.Parser, start *pongo2.Token, arguments *pongo2.Parser) (pongo2.INodeTag, *pongo2.Error) {
	slotNameToken := arguments.MatchType(pongo2.TokenString)
	if slotNameToken == nil {
		return nil, arguments.Error("plugin_slot tag requires a string argument (slot name)", nil)
	}

	return &pluginSlotNode{slotName: slotNameToken.Val}, nil
}

func init() {
	// Register will be called only if the import is included
	pongo2.RegisterTag("plugin_slot", pluginSlotTagParser)
}
```

**Step 2: Pass plugin manager into template context**

In `server/template_handlers/render_template.go`, modify the `RenderTemplate` function. After `context := templateContextGenerator(request)` (~line 41), add:

```go
// Plugin manager is already in context as _pluginManager (set by context providers)
```

The plugin manager needs to get into the template context. The cleanest place is in `server/routes.go` where `registerRoutes` is called. Add the plugin manager to the template context generation chain.

Modify `registerRoutes` in `routes.go` to wrap the context function:

```go
// After getting contextFn from templateInfo
originalContextFn := templateInfo.contextFn(appContext)
wrappedContextFn := func(request *http.Request) pongo2.Context {
	ctx := originalContextFn(request)
	if appContext.PluginManager() != nil {
		ctx["_pluginManager"] = appContext.PluginManager()
		ctx["currentPath"] = request.URL.String()
	}
	return ctx
}
```

Then pass `wrappedContextFn` instead of `templateInfo.contextFn(appContext)` to `RenderTemplate`.

**Step 3: Verify it compiles**

Run:
```bash
go build --tags 'json1 fts5'
```

**Step 4: Commit**

```bash
git add server/
git commit -m "feat: add plugin_slot template tag and wire plugin manager into template context"
```

---

### Task 9: Add plugin_slot tags to templates

Place `{% plugin_slot "..." %}` calls at all the named injection points defined in the design.

**Files:**
- Modify: `templates/layouts/base.tpl` (global slots: head, page_top, page_bottom, sidebar_top, sidebar_bottom, scripts)
- Modify: `templates/displayResource.tpl` (resource_detail_before, resource_detail_after, resource_detail_sidebar)
- Modify: `templates/displayNote.tpl` (note_detail_before, note_detail_after, note_detail_sidebar)
- Modify: `templates/displayGroup.tpl` (group_detail_before, group_detail_after, group_detail_sidebar)
- Modify: `templates/listResources.tpl` (resource_list_before, resource_list_after)
- Modify: `templates/listNotes.tpl` (note_list_before, note_list_after)
- Modify: `templates/listGroups.tpl` (group_list_before, group_list_after)

**Step 1: base.tpl**

After `{% block head %}{% endblock %}` (line 27):
```html
{% plugin_slot "head" %}
```

After `<body ...>` and the skip-to-content link (line 30), before `<header>`:
```html
{% plugin_slot "page_top" %}
```

Inside `<aside class="sidebar">`, after the Updated/Created timestamps and before `{% block sidebar %}` (~line 54):
```html
{% plugin_slot "sidebar_top" %}
```

After `{% block sidebar %}{% endblock %}` (line 55):
```html
{% plugin_slot "sidebar_bottom" %}
```

Before `</footer>` (line 64), after `{% block footer %}`:
```html
{% plugin_slot "page_bottom" %}
```

Before `</body>` (line 69):
```html
{% plugin_slot "scripts" %}
```

**Step 2: displayResource.tpl**

At the start of `{% block body %}`, before existing content:
```html
{% plugin_slot "resource_detail_before" %}
```

At the end of `{% block body %}`, before `{% endblock %}`:
```html
{% plugin_slot "resource_detail_after" %}
```

Inside `{% block sidebar %}`, at an appropriate position:
```html
{% plugin_slot "resource_detail_sidebar" %}
```

**Step 3: displayNote.tpl** — same pattern with `note_detail_*` slots

**Step 4: displayGroup.tpl** — same pattern with `group_detail_*` slots

**Step 5: listResources.tpl** — `resource_list_before` before the loop, `resource_list_after` after

**Step 6: listNotes.tpl** — `note_list_before`/`note_list_after`

**Step 7: listGroups.tpl** — `group_list_before`/`group_list_after`

**Step 8: Verify it compiles and runs**

Run:
```bash
go build --tags 'json1 fts5' && ./mahresources -ephemeral -bind-address=:8181
```

Visit http://localhost:8181 — pages should render normally with no visible changes (no plugins installed yet).

**Step 9: Commit**

```bash
git add templates/
git commit -m "feat: add plugin_slot injection points to all templates"
```

---

### Task 10: Read-only database API for Lua

Expose `mah.db.*` functions that let plugins query entities.

**Files:**
- Create: `plugin_system/db_api.go`
- Create: `plugin_system/db_api_test.go`
- Modify: `plugin_system/manager.go` (accept context for DB access)

**Step 1: Design the integration**

The plugin manager needs access to `MahresourcesContext` for DB queries. Since the manager is created at startup, pass the context to it. Add a `SetContext` method or accept it in the constructor.

Modify `NewPluginManager` signature:
```go
func NewPluginManager(dir string, dbProvider EntityQuerier) (*PluginManager, error)
```

Where `EntityQuerier` is an interface:
```go
type EntityQuerier interface {
	GetNote(id uint) (*NoteData, error)
	GetResource(id uint) (*ResourceData, error)
	GetGroup(id uint) (*GroupData, error)
	GetTag(id uint) (*TagData, error)
	GetCategory(id uint) (*CategoryData, error)
	QueryNotes(filter map[string]any) ([]NoteData, error)
	QueryResources(filter map[string]any) ([]ResourceData, error)
	QueryGroups(filter map[string]any) ([]GroupData, error)
	GetResourceTags(resourceID uint) ([]TagData, error)
	GetResourceNotes(resourceID uint) ([]NoteData, error)
	GetResourceGroups(resourceID uint) ([]GroupData, error)
	GetNoteResources(noteID uint) ([]ResourceData, error)
	GetGroupChildren(groupID uint) ([]GroupData, error)
}
```

The data types (`NoteData`, `ResourceData`, etc.) are simple structs with exported fields, defined in the plugin_system package to avoid importing models directly. The adapter that implements `EntityQuerier` lives in `application_context/` and bridges to the real GORM queries.

**Step 2: Create the interface and adapter**

Create `plugin_system/db_api.go` with the interface definition and the `mah.db` Lua module registration.

Create `application_context/plugin_db_adapter.go` that implements `EntityQuerier` using the existing context methods.

**Step 3: Register mah.db in the Lua VM**

In `registerMahModule`, add a `db` sub-table:

```go
dbMod := L.NewTable()
L.SetField(dbMod, "get_note", L.NewFunction(func(L *lua.LState) int {
	id := L.CheckNumber(1)
	note, err := mgr.dbProvider.GetNote(uint(id))
	if err != nil {
		L.Push(lua.LNil)
		return 1
	}
	L.Push(noteToLuaTable(L, note))
	return 1
}))
// ... similar for all entity types
L.SetField(mahMod, "db", dbMod)
```

**Step 4: Write tests with a mock EntityQuerier**

**Step 5: Verify tests pass**

Run:
```bash
go test ./plugin_system/... --tags 'json1 fts5' -v
```

**Step 6: Commit**

```bash
git add plugin_system/ application_context/
git commit -m "feat: add read-only database API for Lua plugins"
```

---

### Task 11: Entity-to-map conversion for hook integration

The hooks in Task 7 need to convert entity models and query DTOs to `map[string]any` and back. The `plugin_slot` tag in Task 8 also needs entity-to-map conversion. Create a shared conversion layer.

**Files:**
- Create: `plugin_system/entity_convert.go`
- Create: `plugin_system/entity_convert_test.go`

**Step 1: Implement converters**

These are straightforward struct-to-map functions. Use explicit field mapping (not reflection) for type safety and to control what's exposed:

```go
package plugin_system

// NoteToMap converts a note-like entity to a plugin-friendly map.
func NoteToMap(id uint, name, description string, meta []byte) map[string]any {
	return map[string]any{
		"id":          float64(id),
		"name":        name,
		"description": description,
		"meta":        string(meta),
	}
}

// Similar for ResourceToMap, GroupToMap, TagToMap, CategoryToMap
```

**Step 2: Write tests, verify, commit**

```bash
git add plugin_system/
git commit -m "feat: add entity-to-map converters for plugin hooks"
```

---

### Task 12: End-to-end integration test

Create a test that loads a real plugin, creates an entity, and verifies the hook fired and injection renders.

**Files:**
- Create: `plugin_system/integration_test.go`

**Step 1: Write an integration test**

```go
func TestEndToEnd_HookAndInjection(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "e2e-plugin", `
plugin = { name = "e2e", version = "1.0", description = "end to end test" }

function init()
	mah.on("before_note_create", function(entity)
		entity.name = entity.name .. " [via plugin]"
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

	// Test hook modifies data
	data := map[string]any{"name": "Test Note", "description": "desc"}
	result, err := mgr.RunBeforeHooks("before_note_create", data)
	if err != nil {
		t.Fatal(err)
	}
	if result["name"] != "Test Note [via plugin]" {
		t.Errorf("hook didn't modify name: %q", result["name"])
	}

	// Test injection renders with entity context
	html := mgr.RenderSlot("note_detail_after", map[string]any{
		"entity": map[string]any{"name": "Test Note [via plugin]"},
	})
	if html != "<div class='plugin-injected'>Plugin: Test Note [via plugin]</div>" {
		t.Errorf("unexpected injection output: %q", html)
	}
}
```

**Step 2: Run, verify, commit**

```bash
go test ./plugin_system/... --tags 'json1 fts5' -v
git add plugin_system/
git commit -m "test: add end-to-end plugin system integration test"
```

---

### Task 13: E2E Playwright test

Add a Playwright test that starts the server with a test plugin and verifies injection appears in the browser.

**Files:**
- Create: `e2e/tests/plugins/plugin-injection.spec.ts`
- Create: `e2e/test-plugins/test-banner/plugin.lua`

**Step 1: Create a test plugin**

Create `e2e/test-plugins/test-banner/plugin.lua`:
```lua
plugin = {
    name = "test-banner",
    version = "1.0",
    description = "Test plugin that injects a banner"
}

function init()
    mah.inject("page_top", function(ctx)
        return '<div data-testid="plugin-banner" style="background:yellow;padding:8px;text-align:center;">Plugin Banner Active</div>'
    end)
end
```

**Step 2: Write the Playwright test**

```typescript
import { test, expect } from '../fixtures/base.fixture';

test.describe('Plugin System', () => {
    test('plugin injection renders on page', async ({ page, baseURL }) => {
        await page.goto(`${baseURL}/resources`);
        const banner = page.locator('[data-testid="plugin-banner"]');
        await expect(banner).toBeVisible();
        await expect(banner).toContainText('Plugin Banner Active');
    });
});
```

**Step 3: Update the test:with-server script** to pass `-plugin-path=./test-plugins` to the server.

**Step 4: Run, verify, commit**

```bash
cd e2e && npm run test:with-server -- --grep "Plugin System"
git add e2e/
git commit -m "test: add E2E test for plugin injection"
```

---

### Task 14: Thread safety — mutex protection for Lua VMs

gopher-lua's `LState` is not goroutine-safe. Since HTTP handlers run concurrently, we need synchronization.

**Files:**
- Modify: `plugin_system/manager.go` (add per-VM mutexes)
- Modify: `plugin_system/hooks.go` (lock VM before calling hooks)
- Modify: `plugin_system/injections.go` (lock VM before calling injections)

**Step 1: Add per-VM locks**

Each `hookEntry` and `injectionEntry` should reference a mutex for its VM. Add a `mu *sync.Mutex` to each, or maintain a `map[*lua.LState]*sync.Mutex` on the manager.

```go
type PluginManager struct {
	// ... existing fields
	vmLocks map[*lua.LState]*sync.Mutex
}
```

**Step 2: Lock before Lua calls**

In `RunBeforeHooks`, `RunAfterHooks`, and `RenderSlot`, acquire the VM lock before calling into Lua:

```go
mgr.vmLocks[hook.state].Lock()
// ... call Lua
mgr.vmLocks[hook.state].Unlock()
```

**Step 3: Run tests with race detector**

```bash
go test ./plugin_system/... --tags 'json1 fts5' -race -v
```

**Step 4: Commit**

```bash
git add plugin_system/
git commit -m "feat: add per-VM mutex for thread-safe Lua execution"
```

---

### Task 15: Documentation and example plugin

Create a sample plugin and brief documentation.

**Files:**
- Create: `plugins/.gitkeep` (ensure the directory exists in the repo)
- Create: `plugins/example-plugin/plugin.lua`

**Step 1: Create the example plugin**

```lua
-- Example plugin for mahresources
-- Place plugin directories in the plugins/ folder (or your configured -plugin-path)

plugin = {
    name = "example-plugin",
    version = "1.0",
    description = "Demonstrates the plugin API — inject HTML and hook into entity events"
}

function init()
    -- Inject a small footer note on every page
    mah.inject("page_bottom", function(ctx)
        return '<div style="text-align:center;padding:4px;font-size:12px;color:#999;">Powered by mahresources plugins</div>'
    end)

    -- Log when a note is created
    mah.on("after_note_create", function(note)
        mah.log("info", "Note created: " .. note.name)
    end)
end
```

**Step 2: Commit**

```bash
git add plugins/
git commit -m "docs: add example plugin demonstrating hook and injection API"
```

---

### Task 16: Final verification

**Step 1: Run all Go tests**
```bash
go test ./... --tags 'json1 fts5' -race
```

**Step 2: Run E2E tests**
```bash
cd e2e && npm run test:with-server
```

**Step 3: Manual smoke test**

```bash
npm run build
./mahresources -ephemeral -bind-address=:8181 -plugin-path=./plugins
```

Visit http://localhost:8181 — verify the example plugin's footer note appears. Create a note — check server logs for the plugin log message.

**Step 4: Final commit if any fixes needed**

```bash
git add -A
git commit -m "fix: address issues found in final verification"
```
