# Plugin KV Store Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a per-plugin key-value store so plugins can persist runtime state (e.g., "my AI category ID is 5") across restarts.

**Architecture:** New `plugin_kv` table with `(plugin_name, key)` unique index and JSON text values. GORM model, app context CRUD methods, `KVStore` interface on PluginManager (atomic.Value like EntityQuerier), Lua `mah.kv.*` sub-module, purge endpoint + UI button.

**Tech Stack:** Go, GORM, gopher-lua, Pongo2 templates

---

### Task 1: GORM Model

**Files:**
- Create: `models/plugin_kv_model.go`

**Step 1: Create model file**

```go
package models

import "time"

// PluginKV stores per-plugin key-value data.
type PluginKV struct {
	ID         uint      `gorm:"primarykey"`
	CreatedAt  time.Time `gorm:"index"`
	UpdatedAt  time.Time `gorm:"index"`
	PluginName string    `gorm:"uniqueIndex:idx_plugin_kv_key;not null"`
	Key        string    `gorm:"uniqueIndex:idx_plugin_kv_key;not null"`
	Value      string    `gorm:"type:text;not null"`
}
```

**Step 2: Add to auto-migrate in main.go**

In `main.go`, find the `db.AutoMigrate(...)` call (around line 233) and add `&models.PluginKV{}` after `&models.PluginState{}`:

```go
&models.PluginState{},
&models.PluginKV{},
```

**Step 3: Build to verify migration compiles**

Run: `go build --tags 'json1 fts5'`
Expected: clean build, no errors

**Step 4: Commit**

```
feat: add PluginKV model for plugin key-value storage
```

---

### Task 2: KVStore Interface + App Context Implementation

**Files:**
- Modify: `plugin_system/db_api.go` — add `KVStore` interface and setter/getter
- Create: `application_context/plugin_kv_context.go` — GORM implementation
- Modify: `application_context/plugin_db_adapter.go` — implement interface on adapter
- Modify: `application_context/context.go` — wire up `SetKVStore`

**Step 1: Write failing test**

Create `application_context/plugin_kv_context_test.go`:

```go
//go:build json1 && fts5

package application_context

import (
	"testing"
)

func TestPluginKV_SetGetDelete(t *testing.T) {
	ctx := createTestContext(t)

	// Set a value
	if err := ctx.PluginKVSet("test-plugin", "my_key", `"hello"`); err != nil {
		t.Fatalf("KVSet failed: %v", err)
	}

	// Get it back
	val, found, err := ctx.PluginKVGet("test-plugin", "my_key")
	if err != nil {
		t.Fatalf("KVGet failed: %v", err)
	}
	if !found {
		t.Fatal("expected key to be found")
	}
	if val != `"hello"` {
		t.Errorf("expected %q, got %q", `"hello"`, val)
	}

	// Overwrite (upsert)
	if err := ctx.PluginKVSet("test-plugin", "my_key", `42`); err != nil {
		t.Fatalf("KVSet upsert failed: %v", err)
	}
	val, _, _ = ctx.PluginKVGet("test-plugin", "my_key")
	if val != `42` {
		t.Errorf("expected %q after upsert, got %q", `42`, val)
	}

	// Delete
	if err := ctx.PluginKVDelete("test-plugin", "my_key"); err != nil {
		t.Fatalf("KVDelete failed: %v", err)
	}
	_, found, _ = ctx.PluginKVGet("test-plugin", "my_key")
	if found {
		t.Fatal("expected key to be deleted")
	}

	// Delete non-existent key (should not error)
	if err := ctx.PluginKVDelete("test-plugin", "nope"); err != nil {
		t.Fatalf("KVDelete of missing key failed: %v", err)
	}
}

func TestPluginKV_ListWithPrefix(t *testing.T) {
	ctx := createTestContext(t)

	ctx.PluginKVSet("test-plugin", "cat:images", `1`)
	ctx.PluginKVSet("test-plugin", "cat:docs", `2`)
	ctx.PluginKVSet("test-plugin", "other", `3`)

	// List all
	keys, err := ctx.PluginKVList("test-plugin", "")
	if err != nil {
		t.Fatalf("KVList failed: %v", err)
	}
	if len(keys) != 3 {
		t.Errorf("expected 3 keys, got %d", len(keys))
	}

	// List with prefix
	keys, err = ctx.PluginKVList("test-plugin", "cat:")
	if err != nil {
		t.Fatalf("KVList with prefix failed: %v", err)
	}
	if len(keys) != 2 {
		t.Errorf("expected 2 keys with prefix 'cat:', got %d", len(keys))
	}
	// Should be sorted
	if keys[0] != "cat:docs" || keys[1] != "cat:images" {
		t.Errorf("unexpected key order: %v", keys)
	}
}

func TestPluginKV_Isolation(t *testing.T) {
	ctx := createTestContext(t)

	ctx.PluginKVSet("plugin-a", "key", `"a-value"`)
	ctx.PluginKVSet("plugin-b", "key", `"b-value"`)

	val, found, _ := ctx.PluginKVGet("plugin-a", "key")
	if !found || val != `"a-value"` {
		t.Errorf("plugin-a got wrong value: %q", val)
	}

	val, found, _ = ctx.PluginKVGet("plugin-b", "key")
	if !found || val != `"b-value"` {
		t.Errorf("plugin-b got wrong value: %q", val)
	}

	// List only shows own keys
	keys, _ := ctx.PluginKVList("plugin-a", "")
	if len(keys) != 1 {
		t.Errorf("plugin-a should see 1 key, got %d", len(keys))
	}
}

func TestPluginKV_Purge(t *testing.T) {
	ctx := createTestContext(t)

	ctx.PluginKVSet("doomed", "k1", `1`)
	ctx.PluginKVSet("doomed", "k2", `2`)
	ctx.PluginKVSet("survivor", "k1", `3`)

	if err := ctx.PluginKVPurge("doomed"); err != nil {
		t.Fatalf("KVPurge failed: %v", err)
	}

	keys, _ := ctx.PluginKVList("doomed", "")
	if len(keys) != 0 {
		t.Errorf("expected 0 keys after purge, got %d", len(keys))
	}

	// Other plugin unaffected
	keys, _ = ctx.PluginKVList("survivor", "")
	if len(keys) != 1 {
		t.Errorf("survivor should still have 1 key, got %d", len(keys))
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test --tags 'json1 fts5' ./application_context/ -run TestPluginKV -v`
Expected: FAIL — methods don't exist yet

**Step 3: Add KVStore interface to plugin_system/db_api.go**

After the `PluginLogger` interface (around line 77), add:

```go
// KVStore provides per-plugin key-value storage for plugins.
type KVStore interface {
	KVGet(pluginName, key string) (string, bool, error)
	KVSet(pluginName, key, value string) error
	KVDelete(pluginName, key string) error
	KVList(pluginName, prefix string) ([]string, error)
	KVPurge(pluginName string) error
}
```

Add setter/getter on PluginManager (same pattern as SetEntityQuerier):

```go
// SetKVStore sets the key-value store provider for plugins.
func (pm *PluginManager) SetKVStore(kv KVStore) {
	pm.kvStore.Store(kv)
}

func (pm *PluginManager) getKVStore() KVStore {
	v := pm.kvStore.Load()
	if v == nil {
		return nil
	}
	return v.(KVStore)
}
```

Add `kvStore atomic.Value` to the PluginManager struct in `manager.go` (alongside `dbProvider`, `dbWriter`, `logger`).

**Step 4: Create application_context/plugin_kv_context.go**

```go
package application_context

import (
	"mahresources/models"

	"gorm.io/gorm/clause"
)

// PluginKVGet retrieves a value by plugin name and key.
// Returns the JSON string value, whether it was found, and any error.
func (ctx *MahresourcesContext) PluginKVGet(pluginName, key string) (string, bool, error) {
	var kv models.PluginKV
	err := ctx.db.Where("plugin_name = ? AND key = ?", pluginName, key).First(&kv).Error
	if err != nil {
		if err.Error() == "record not found" {
			return "", false, nil
		}
		return "", false, err
	}
	return kv.Value, true, nil
}

// PluginKVSet upserts a key-value pair for a plugin.
func (ctx *MahresourcesContext) PluginKVSet(pluginName, key, value string) error {
	kv := models.PluginKV{
		PluginName: pluginName,
		Key:        key,
		Value:      value,
	}
	return ctx.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "plugin_name"}, {Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value", "updated_at"}),
	}).Create(&kv).Error
}

// PluginKVDelete removes a key-value pair for a plugin.
func (ctx *MahresourcesContext) PluginKVDelete(pluginName, key string) error {
	return ctx.db.Where("plugin_name = ? AND key = ?", pluginName, key).
		Delete(&models.PluginKV{}).Error
}

// PluginKVList returns all keys for a plugin, optionally filtered by prefix, sorted alphabetically.
func (ctx *MahresourcesContext) PluginKVList(pluginName, prefix string) ([]string, error) {
	var keys []string
	q := ctx.db.Model(&models.PluginKV{}).Where("plugin_name = ?", pluginName)
	if prefix != "" {
		q = q.Where("key LIKE ?", prefix+"%")
	}
	if err := q.Order("key").Pluck("key", &keys).Error; err != nil {
		return nil, err
	}
	return keys, nil
}

// PluginKVPurge deletes all key-value data for a plugin.
func (ctx *MahresourcesContext) PluginKVPurge(pluginName string) error {
	return ctx.db.Where("plugin_name = ?", pluginName).
		Delete(&models.PluginKV{}).Error
}
```

**Step 5: Implement KVStore on pluginDBAdapter**

In `application_context/plugin_db_adapter.go`, add (the adapter already has a `ctx *MahresourcesContext`):

```go
// Compile-time check
var _ plugin_system.KVStore = (*pluginDBAdapter)(nil)

func (a *pluginDBAdapter) KVGet(pluginName, key string) (string, bool, error) {
	return a.ctx.PluginKVGet(pluginName, key)
}

func (a *pluginDBAdapter) KVSet(pluginName, key, value string) error {
	return a.ctx.PluginKVSet(pluginName, key, value)
}

func (a *pluginDBAdapter) KVDelete(pluginName, key string) error {
	return a.ctx.PluginKVDelete(pluginName, key)
}

func (a *pluginDBAdapter) KVList(pluginName, prefix string) ([]string, error) {
	return a.ctx.PluginKVList(pluginName, prefix)
}

func (a *pluginDBAdapter) KVPurge(pluginName string) error {
	return a.ctx.PluginKVPurge(pluginName)
}
```

**Step 6: Wire up in context.go**

In `application_context/context.go`, after `pm.SetPluginLogger(adapter)` (line 224), add:

```go
pm.SetKVStore(adapter)
```

**Step 7: Run tests**

Run: `go test --tags 'json1 fts5' ./application_context/ -run TestPluginKV -v`
Expected: all PASS

**Step 8: Build**

Run: `go build --tags 'json1 fts5'`
Expected: clean build

**Step 9: Commit**

```
feat: add KVStore interface and app context implementation
```

---

### Task 3: Lua `mah.kv` Module

**Files:**
- Create: `plugin_system/kv_api.go` — Lua module registration
- Modify: `plugin_system/manager.go` — call `registerKvModule` from `registerMahModule`

**Step 1: Write failing test**

Create `plugin_system/kv_api_test.go`:

```go
//go:build json1 && fts5

package plugin_system

import (
	"testing"
)

// mockKVStore implements KVStore for testing.
type mockKVStore struct {
	data map[string]map[string]string // pluginName -> key -> value
}

func newMockKVStore() *mockKVStore {
	return &mockKVStore{data: make(map[string]map[string]string)}
}

func (m *mockKVStore) KVGet(pluginName, key string) (string, bool, error) {
	if keys, ok := m.data[pluginName]; ok {
		if val, ok := keys[key]; ok {
			return val, true, nil
		}
	}
	return "", false, nil
}

func (m *mockKVStore) KVSet(pluginName, key, value string) error {
	if m.data[pluginName] == nil {
		m.data[pluginName] = make(map[string]string)
	}
	m.data[pluginName][key] = value
	return nil
}

func (m *mockKVStore) KVDelete(pluginName, key string) error {
	if keys, ok := m.data[pluginName]; ok {
		delete(keys, key)
	}
	return nil
}

func (m *mockKVStore) KVList(pluginName, prefix string) ([]string, error) {
	var result []string
	if keys, ok := m.data[pluginName]; ok {
		for k := range keys {
			if prefix == "" || len(k) >= len(prefix) && k[:len(prefix)] == prefix {
				result = append(result, k)
			}
		}
	}
	// Sort for consistency
	sort.Strings(result)
	return result, nil
}

func (m *mockKVStore) KVPurge(pluginName string) error {
	delete(m.data, pluginName)
	return nil
}
```

Add the actual test (same file):

```go
func TestKV_SetAndGet(t *testing.T) {
	mgr := createTestPluginManager(t, `
		plugin = { name = "test-kv", version = "1.0.0", description = "test" }
		function init()
			mah.kv.set("count", 42)
			mah.kv.set("name", "hello")
			mah.kv.set("config", {theme = "dark", size = 3})
		end
	`)
	store := newMockKVStore()
	mgr.SetKVStore(store)

	if err := mgr.EnablePlugin("test-kv"); err != nil {
		t.Fatalf("EnablePlugin failed: %v", err)
	}

	// Verify stored as JSON
	val, ok := store.data["test-kv"]["count"]
	if !ok || val != "42" {
		t.Errorf("expected count=42, got %q (found=%v)", val, ok)
	}
	val, ok = store.data["test-kv"]["name"]
	if !ok || val != `"hello"` {
		t.Errorf("expected name=\"hello\", got %q", val)
	}
}

func TestKV_GetReturnsNilForMissing(t *testing.T) {
	mgr := createTestPluginManager(t, `
		plugin = { name = "test-kv-nil", version = "1.0.0", description = "test" }
		function init()
			local v = mah.kv.get("nonexistent")
			if v ~= nil then
				error("expected nil, got: " .. tostring(v))
			end
		end
	`)
	store := newMockKVStore()
	mgr.SetKVStore(store)

	if err := mgr.EnablePlugin("test-kv-nil"); err != nil {
		t.Fatalf("EnablePlugin failed: %v", err)
	}
}

func TestKV_Delete(t *testing.T) {
	mgr := createTestPluginManager(t, `
		plugin = { name = "test-kv-del", version = "1.0.0", description = "test" }
		function init()
			mah.kv.set("temp", "value")
			mah.kv.delete("temp")
			local v = mah.kv.get("temp")
			if v ~= nil then
				error("expected nil after delete")
			end
		end
	`)
	store := newMockKVStore()
	mgr.SetKVStore(store)

	if err := mgr.EnablePlugin("test-kv-del"); err != nil {
		t.Fatalf("EnablePlugin failed: %v", err)
	}
}

func TestKV_List(t *testing.T) {
	mgr := createTestPluginManager(t, `
		plugin = { name = "test-kv-list", version = "1.0.0", description = "test" }
		function init()
			mah.kv.set("cat:images", 1)
			mah.kv.set("cat:docs", 2)
			mah.kv.set("other", 3)

			local all = mah.kv.list()
			if #all ~= 3 then
				error("expected 3 keys, got " .. #all)
			end

			local cats = mah.kv.list("cat:")
			if #cats ~= 2 then
				error("expected 2 keys with prefix, got " .. #cats)
			end
		end
	`)
	store := newMockKVStore()
	mgr.SetKVStore(store)

	if err := mgr.EnablePlugin("test-kv-list"); err != nil {
		t.Fatalf("EnablePlugin failed: %v", err)
	}
}
```

Note: `createTestPluginManager` is a helper that already exists in the test suite. Check existing test files to confirm its exact signature. It writes the Lua code to a temp `plugin.lua`, creates a PluginManager, and discovers it.

**Step 2: Run test to verify it fails**

Run: `go test --tags 'json1 fts5' ./plugin_system/ -run TestKV -v`
Expected: FAIL — `mah.kv` doesn't exist yet

**Step 3: Create plugin_system/kv_api.go**

```go
package plugin_system

import (
	"encoding/json"
	"fmt"

	lua "github.com/yuin/gopher-lua"
)

// registerKvModule registers the mah.kv sub-table in the Lua VM.
// Provides mah.kv.get(key), mah.kv.set(key, value), mah.kv.delete(key), mah.kv.list([prefix]).
func (pm *PluginManager) registerKvModule(L *lua.LState, mahMod *lua.LTable, pluginNamePtr *string) {
	kvMod := L.NewTable()

	// mah.kv.get(key) -> value or nil
	kvMod.RawSetString("get", L.NewFunction(func(L *lua.LState) int {
		key := L.CheckString(1)
		kv := pm.getKVStore()
		if kv == nil {
			L.Push(lua.LNil)
			return 1
		}
		val, found, err := kv.KVGet(*pluginNamePtr, key)
		if err != nil || !found {
			L.Push(lua.LNil)
			return 1
		}
		// Deserialize JSON to Lua value
		var goVal any
		if err := json.Unmarshal([]byte(val), &goVal); err != nil {
			L.Push(lua.LNil)
			return 1
		}
		L.Push(goToLuaValue(L, goVal))
		return 1
	}))

	// mah.kv.set(key, value)
	kvMod.RawSetString("set", L.NewFunction(func(L *lua.LState) int {
		key := L.CheckString(1)
		val := L.CheckAny(2)
		kv := pm.getKVStore()
		if kv == nil {
			L.RaiseError("kv store not available")
			return 0
		}
		goVal := luaValueToGoForJson(val)
		jsonBytes, err := json.Marshal(goVal)
		if err != nil {
			L.RaiseError("failed to serialize value: %s", err.Error())
			return 0
		}
		if err := kv.KVSet(*pluginNamePtr, key, string(jsonBytes)); err != nil {
			L.RaiseError("kv set failed: %s", err.Error())
			return 0
		}
		return 0
	}))

	// mah.kv.delete(key)
	kvMod.RawSetString("delete", L.NewFunction(func(L *lua.LState) int {
		key := L.CheckString(1)
		kv := pm.getKVStore()
		if kv == nil {
			L.RaiseError("kv store not available")
			return 0
		}
		if err := kv.KVDelete(*pluginNamePtr, key); err != nil {
			L.RaiseError("kv delete failed: %s", err.Error())
			return 0
		}
		return 0
	}))

	// mah.kv.list([prefix]) -> table of key strings
	kvMod.RawSetString("list", L.NewFunction(func(L *lua.LState) int {
		prefix := ""
		if L.GetTop() >= 1 {
			prefix = L.CheckString(1)
		}
		kv := pm.getKVStore()
		if kv == nil {
			L.Push(L.NewTable())
			return 1
		}
		keys, err := kv.KVList(*pluginNamePtr, prefix)
		if err != nil {
			L.Push(L.NewTable())
			return 1
		}
		tbl := L.NewTable()
		for _, k := range keys {
			tbl.Append(lua.LString(k))
		}
		L.Push(tbl)
		return 1
	}))

	mahMod.RawSetString("kv", kvMod)

	// Suppress unused import if fmt isn't needed elsewhere
	_ = fmt.Sprintf
}
```

Remove the `_ = fmt.Sprintf` line if `fmt` is not needed — it's just a safety net. The file likely won't need `fmt` at all; remove the import if so.

**Step 4: Wire into registerMahModule in manager.go**

In `plugin_system/manager.go`, in the `registerMahModule` function, after the `registerJsonModule` call (around line 551) add:

```go
pm.registerKvModule(L, mahMod, pluginNamePtr)
```

**Step 5: Run tests**

Run: `go test --tags 'json1 fts5' ./plugin_system/ -run TestKV -v`
Expected: all PASS

**Step 6: Build**

Run: `go build --tags 'json1 fts5'`
Expected: clean build

**Step 7: Commit**

```
feat: add mah.kv Lua module for plugin key-value storage
```

---

### Task 4: Purge Endpoint and Management UI

**Files:**
- Modify: `server/api_handlers/plugin_api_handlers.go` — add purge handler
- Modify: `server/routes.go` — register purge route
- Modify: `templates/managePlugins.tpl` — add purge button

**Step 1: Add purge handler**

In `server/api_handlers/plugin_api_handlers.go`, add a new handler function following the existing enable/disable pattern:

```go
// GetPluginPurgeDataHandler deletes all KV data for a disabled plugin.
func GetPluginPurgeDataHandler(ctx *application_context.MahresourcesContext) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimSpace(r.FormValue("name"))
		if name == "" {
			http_utils.HandleError(fmt.Errorf("missing plugin name"), w, r, http.StatusBadRequest)
			return
		}

		pm := ctx.PluginManager()
		if pm != nil && pm.IsEnabled(name) {
			http_utils.HandleError(fmt.Errorf("cannot purge data for enabled plugin %q — disable it first", name), w, r, http.StatusBadRequest)
			return
		}

		if err := ctx.PluginKVPurge(name); err != nil {
			http_utils.HandleError(err, w, r, http.StatusInternalServerError)
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

**Step 2: Register route**

In `server/routes.go`, near the other plugin routes (around line 366), add:

```go
router.Methods(http.MethodPost).Path("/v1/plugin/purge-data").HandlerFunc(api_handlers.GetPluginPurgeDataHandler(appContext))
```

**Step 3: Add purge button to template**

In `templates/managePlugins.tpl`, after the enable/disable form button (around line 34), add a purge form that only shows for disabled plugins:

```html
{% if not plugin.Enabled %}
<form method="POST" action="/v1/plugin/purge-data" class="ml-2"
      onsubmit="return confirm('Purge all stored data for {{ plugin.Name }}? This cannot be undone.')">
    <input type="hidden" name="name" value="{{ plugin.Name }}">
    <button type="submit" class="btn btn-outline text-sm"
            data-testid="plugin-purge-{{ plugin.Name }}">
        Purge Data
    </button>
</form>
{% endif %}
```

Wrap the existing enable/disable form and the new purge form in a `flex` container so they sit side by side. The card-header `div` already has `flex items-center justify-between`, so wrap both forms in a `<div class="flex gap-2">` on the right side.

**Step 4: Build and verify**

Run: `go build --tags 'json1 fts5'`
Expected: clean build

**Step 5: Commit**

```
feat: add plugin data purge endpoint and management UI button
```

---

### Task 5: Full Integration Test

**Files:**
- Modify: `plugin_system/kv_api_test.go` — add integration-style test

**Step 1: Write an end-to-end test**

Add to `plugin_system/kv_api_test.go`:

```go
func TestKV_RoundTrip_ComplexValues(t *testing.T) {
	mgr := createTestPluginManager(t, `
		plugin = { name = "test-kv-rt", version = "1.0.0", description = "test" }
		function init()
			-- Store various types
			mah.kv.set("str", "hello world")
			mah.kv.set("num", 3.14)
			mah.kv.set("bool", true)
			mah.kv.set("obj", {name = "test", count = 5})
			mah.kv.set("arr", {10, 20, 30})

			-- Read back and verify types
			local s = mah.kv.get("str")
			if type(s) ~= "string" or s ~= "hello world" then
				error("str roundtrip failed: " .. tostring(s))
			end

			local n = mah.kv.get("num")
			if type(n) ~= "number" or n ~= 3.14 then
				error("num roundtrip failed: " .. tostring(n))
			end

			local b = mah.kv.get("bool")
			if type(b) ~= "boolean" or b ~= true then
				error("bool roundtrip failed: " .. tostring(b))
			end

			local o = mah.kv.get("obj")
			if type(o) ~= "table" or o.name ~= "test" or o.count ~= 5 then
				error("obj roundtrip failed")
			end

			local a = mah.kv.get("arr")
			if type(a) ~= "table" or #a ~= 3 or a[1] ~= 10 then
				error("arr roundtrip failed")
			end
		end
	`)
	store := newMockKVStore()
	mgr.SetKVStore(store)

	if err := mgr.EnablePlugin("test-kv-rt"); err != nil {
		t.Fatalf("EnablePlugin failed: %v", err)
	}
}
```

**Step 2: Run all KV tests**

Run: `go test --tags 'json1 fts5' ./plugin_system/ -run TestKV -v`
Expected: all PASS

**Step 3: Run full test suite**

Run: `go test --tags 'json1 fts5' ./...`
Expected: all PASS

**Step 4: Commit**

```
test: add KV store round-trip integration test
```

---

Plan complete and saved to `docs/plans/2026-03-05-plugin-kv-store-impl.md`. Two execution options:

**1. Subagent-Driven (this session)** — I dispatch a fresh subagent per task, review between tasks, fast iteration

**2. Parallel Session (separate)** — Open new session with executing-plans, batch execution with checkpoints

Which approach?