# Data-Views Extended Data Sources Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extend the data-views plugin so shortcodes can read entity properties (`field="FileSize"`) and MRQL query results (`mrql="type=resource"`) in addition to existing metadata paths.

**Architecture:** Hybrid Lua-Go approach. Entity properties are pre-populated in the Lua context table (`ctx.entity`). MRQL queries are executed via a new `mah.db.mrql_query()` Lua function backed by a Go `MRQLExecutor` interface with per-request caching and owner-based scoping.

**Tech Stack:** Go (gopher-lua, GORM, reflection), Lua (plugin shortcode render functions)

**Spec:** `docs/superpowers/specs/2026-04-10-data-views-extended-sources-design.md`

---

### Task 1: MRQLExecutor Interface and PluginManager Plumbing

**Files:**
- Modify: `plugin_system/db_api.go` (after line 156 — add interface, types, setter/getter)
- Modify: `plugin_system/manager.go:94` (add atomic.Value field)
- Test: `plugin_system/db_api_test.go` (existing test file, add setter test)

- [ ] **Step 1: Define MRQLExecutor interface and result types in db_api.go**

Add after the `getDbWriter` function (after line 156):

```go
// MRQLExecutor provides MRQL query execution for plugins.
type MRQLExecutor interface {
	ExecuteMRQL(ctx context.Context, query string, opts MRQLExecOptions) (*MRQLResult, error)
}

// MRQLExecOptions carries execution parameters including scope.
type MRQLExecOptions struct {
	Limit   int  // max items (default 20)
	Buckets int  // max GROUP BY buckets (default 5)
	ScopeID uint // resolved owner_id for scoping (0 = no scope filter)
}

// MRQLResult holds query results in a plugin_system-safe form (no model imports).
type MRQLResult struct {
	EntityType string
	Mode       string // "flat", "aggregated", "bucketed"
	Items      []map[string]any
	Rows       []map[string]any
	Groups     []MRQLResultGroup
}

// MRQLResultGroup is a bucket of items sharing a common key.
type MRQLResultGroup struct {
	Key   map[string]any
	Items []map[string]any
}
```

Add import `"context"` to the import block (currently only imports `lua`).

- [ ] **Step 2: Add setter/getter for MRQLExecutor in db_api.go**

Add after the MRQLResultGroup type:

```go
// SetMRQLExecutor sets the MRQL query executor for plugins.
func (pm *PluginManager) SetMRQLExecutor(me MRQLExecutor) {
	pm.mrqlExecutor.Store(me)
}

// getMRQLExecutor returns the current MRQLExecutor, or nil if not yet set.
func (pm *PluginManager) getMRQLExecutor() MRQLExecutor {
	v := pm.mrqlExecutor.Load()
	if v == nil {
		return nil
	}
	return v.(MRQLExecutor)
}
```

- [ ] **Step 3: Add mrqlExecutor field to PluginManager struct in manager.go**

In the PluginManager struct, add after the `kvStore` field (line 97):

```go
	mrqlExecutor atomic.Value
```

- [ ] **Step 4: Run tests to verify compilation**

Run: `go build --tags 'json1 fts5' ./...`
Expected: clean build, no errors.

- [ ] **Step 5: Commit**

```bash
git add plugin_system/db_api.go plugin_system/manager.go
git commit -m "feat: add MRQLExecutor interface and PluginManager plumbing"
```

---

### Task 2: Extend RenderShortcode to Accept Request Context and Entity

**Files:**
- Modify: `plugin_system/shortcodes.go:190` (RenderShortcode signature + ctx.entity building)

- [ ] **Step 1: Write test for ctx.entity in shortcodes_test.go**

Add to `plugin_system/shortcodes_test.go`:

```go
func TestShortcodeEntityContext(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "sc-entity", `
		plugin = { name = "sc-entity", version = "1.0" }
		function init()
			mah.shortcode({
				name = "entfield",
				label = "Entity Field",
				render = function(ctx)
					if ctx.entity == nil then return "no entity" end
					return tostring(ctx.entity.Name) .. ":" .. tostring(ctx.entity.FileSize)
				end
			})
		end
	`)

	pm, err := NewPluginManager(dir)
	require.NoError(t, err)
	defer pm.Close()

	require.NoError(t, pm.EnablePlugin("sc-entity"))

	html, err := pm.RenderShortcode(
		context.Background(),
		"sc-entity",
		"plugin:sc-entity:entfield",
		"resource", 1,
		json.RawMessage(`{"rating": 5}`),
		map[string]string{},
		&testResourceEntity{Name: "photo.jpg", FileSize: 1024},
	)
	require.NoError(t, err)
	assert.Equal(t, "photo.jpg:1024", html)
}

// testResourceEntity is a minimal struct for testing ctx.entity reflection.
type testResourceEntity struct {
	Name        string
	FileSize    int64
	ContentType string
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test --tags 'json1 fts5' ./plugin_system/... -run TestShortcodeEntityContext -v`
Expected: FAIL — `RenderShortcode` does not accept these parameters yet.

- [ ] **Step 3: Update RenderShortcode signature and build ctx.entity**

Change the `RenderShortcode` function in `plugin_system/shortcodes.go` (line 190):

Old signature:
```go
func (pm *PluginManager) RenderShortcode(pluginName, fullTypeName, entityType string, entityID uint, meta json.RawMessage, attrs map[string]string) (string, error) {
```

New signature:
```go
func (pm *PluginManager) RenderShortcode(reqCtx context.Context, pluginName, fullTypeName, entityType string, entityID uint, meta json.RawMessage, attrs map[string]string, entity any) (string, error) {
```

In the body, change line 244 from:
```go
	timeoutCtx, cancel := context.WithTimeout(context.Background(), luaShortcodeRenderTimeout)
```
to:
```go
	timeoutCtx, cancel := context.WithTimeout(reqCtx, luaShortcodeRenderTimeout)
```

Before the `tbl := goToLuaTable(L, ctxData)` line (line 242), add entity table building:

```go
	if entity != nil {
		ctxData["entity"] = entityToMap(entity)
	}
```

Add the `entityToMap` helper function after `RenderShortcode`:

```go
// entityToMap converts an entity struct to a map[string]any using reflection.
// Fields are PascalCase keys (Name, FileSize, etc.). time.Time values are
// formatted as RFC3339 strings. Nil pointer fields become nil (Lua nil).
func entityToMap(entity any) map[string]any {
	v := reflect.ValueOf(entity)
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return nil
	}

	result := make(map[string]any)
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}
		fv := v.Field(i)
		val := entityFieldValue(fv)
		if val != nil {
			result[field.Name] = val
		}
	}
	return result
}

// entityFieldValue extracts a Lua-compatible value from a reflect.Value.
// Returns nil for unsupported types (slices, nested structs, etc.).
func entityFieldValue(fv reflect.Value) any {
	if fv.Kind() == reflect.Ptr {
		if fv.IsNil() {
			return nil
		}
		fv = fv.Elem()
	}

	iface := fv.Interface()

	// time.Time -> RFC3339 string
	if t, ok := iface.(time.Time); ok {
		return t.Format(time.RFC3339)
	}
	// json.RawMessage -> keep as string
	if raw, ok := iface.(json.RawMessage); ok {
		return string(raw)
	}
	// fmt.Stringer (covers types.URL, url.URL, and similar wrapper types)
	if s, ok := iface.(fmt.Stringer); ok {
		return s.String()
	}

	switch fv.Kind() {
	case reflect.String:
		return fv.String()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return float64(fv.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return float64(fv.Uint())
	case reflect.Float32, reflect.Float64:
		return fv.Float()
	case reflect.Bool:
		return fv.Bool()
	default:
		return nil // skip slices, structs, maps, etc.
	}
}
```

Add `"reflect"`, `"time"`, and `"fmt"` to the imports (fmt is needed for the `fmt.Stringer` check on types like `types.URL`).

- [ ] **Step 4: Run test to verify it passes**

Run: `go test --tags 'json1 fts5' ./plugin_system/... -run TestShortcodeEntityContext -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add plugin_system/shortcodes.go plugin_system/shortcodes_test.go
git commit -m "feat: extend RenderShortcode with request context and entity"
```

---

### Task 3: Update All RenderShortcode Call Sites

**Files:**
- Modify: `server/routes.go:189-190`
- Modify: `server/api_handlers/mrql_api_handlers.go:60-61`
- Modify: `server/template_handlers/template_filters/shortcode_tag.go:49-50`
- Modify: `plugin_system/shortcodes_test.go` (6 existing test calls)

- [ ] **Step 1: Update routes.go pluginRenderer closure**

In `server/routes.go`, change the closure at line 189-190:

```go
		pluginRenderer = func(pluginName string, sc shortcodes.Shortcode, mctx shortcodes.MetaShortcodeContext) (string, error) {
			return pm.RenderShortcode(reqCtx, pluginName, sc.Name, mctx.EntityType, mctx.EntityID, mctx.Meta, sc.Attrs, mctx.Entity)
		}
```

- [ ] **Step 2: Update mrql_api_handlers.go pluginRenderer closure**

In `server/api_handlers/mrql_api_handlers.go`, change the closure at line 60-61:

```go
	return func(pluginName string, sc shortcodes.Shortcode, mctx shortcodes.MetaShortcodeContext) (string, error) {
		return pm.RenderShortcode(reqCtx, pluginName, sc.Name, mctx.EntityType, mctx.EntityID, mctx.Meta, sc.Attrs, mctx.Entity)
	}
```

Note: `buildPluginRenderer` needs to also accept `reqCtx context.Context` as a parameter. Check the function signature — it currently takes only `appCtx`. You'll need to update it and all its callers (search for `buildPluginRenderer` in the file). Alternatively, since `renderMRQLCustomTemplates` already receives `reqCtx`, thread it through.

- [ ] **Step 3: Update shortcode_tag.go pluginRenderer closure**

In `server/template_handlers/template_filters/shortcode_tag.go`, change the closure at line 49-50:

```go
			pluginRenderer = func(pluginName string, sc shortcodes.Shortcode, mctx shortcodes.MetaShortcodeContext) (string, error) {
				return pm.RenderShortcode(reqCtx, pluginName, sc.Name, mctx.EntityType, mctx.EntityID, mctx.Meta, sc.Attrs, mctx.Entity)
			}
```

`reqCtx` is already available in this function (line 63-68).

- [ ] **Step 4: Update existing test calls in shortcodes_test.go**

Update all 6 existing `pm.RenderShortcode(...)` calls to include `context.Background()` as the first arg and `nil` as the last arg (no entity). For example, line 62-67:

```go
	html, err := pm.RenderShortcode(
		context.Background(),
		"sc-render",
		"plugin:sc-render:stars",
		"group", 1,
		json.RawMessage(`{"rating": 4}`),
		map[string]string{"max": "3"},
		nil,
	)
```

Repeat for line 94 (TestShortcodeRenderContext) and lines 133, 136, 139 (TestShortcodeNonStringReturnErrors). Add `"context"` to the import block.

- [ ] **Step 5: Run all plugin_system tests**

Run: `go test --tags 'json1 fts5' ./plugin_system/... -v`
Expected: all pass.

- [ ] **Step 6: Run full build**

Run: `go build --tags 'json1 fts5' ./...`
Expected: clean build.

- [ ] **Step 7: Commit**

```bash
git add server/routes.go server/api_handlers/mrql_api_handlers.go server/template_handlers/template_filters/shortcode_tag.go plugin_system/shortcodes_test.go
git commit -m "feat: update all RenderShortcode call sites for new signature"
```

---

### Task 4: MRQLExecutor Adapter and Startup Wiring

**Files:**
- Create: `application_context/plugin_mrql_adapter.go`
- Modify: `application_context/context.go:274` (add SetMRQLExecutor call)
- Test: `application_context/plugin_mrql_adapter_test.go`

- [ ] **Step 1: Write test for the adapter**

Create `application_context/plugin_mrql_adapter_test.go`:

```go
package application_context

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"mahresources/plugin_system"
)

func TestPluginMRQLAdapterFlat(t *testing.T) {
	ctx := setupEphemeralTestContext(t)

	adapter := &pluginMRQLAdapter{ctx: ctx}

	result, err := adapter.ExecuteMRQL(context.Background(), "type=resource", plugin_system.MRQLExecOptions{
		Limit: 10,
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "flat", result.Mode)
	assert.Equal(t, "resource", result.EntityType)
}

func TestPluginMRQLAdapterAggregated(t *testing.T) {
	ctx := setupEphemeralTestContext(t)

	adapter := &pluginMRQLAdapter{ctx: ctx}

	result, err := adapter.ExecuteMRQL(context.Background(), "type=resource GROUP BY contentType COUNT()", plugin_system.MRQLExecOptions{
		Limit:   10,
		Buckets: 5,
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	// Result mode depends on data — "aggregated" or "bucketed"
	assert.Contains(t, []string{"aggregated", "bucketed"}, result.Mode)
}

func TestPluginMRQLAdapterScoped(t *testing.T) {
	ctx := setupEphemeralTestContext(t)

	adapter := &pluginMRQLAdapter{ctx: ctx}

	// ScopeID=999999 should match nothing (empty result, no error)
	result, err := adapter.ExecuteMRQL(context.Background(), "type=resource", plugin_system.MRQLExecOptions{
		Limit:   10,
		ScopeID: 999999,
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Empty(t, result.Items)
}
```

Note: `setupEphemeralTestContext` may not exist yet. Check if there is an existing test helper in `application_context/` that creates an ephemeral context. If not, create one using `NewMahresourcesContext` with memory-db and memory-fs config. Look at existing tests in the package for patterns.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test --tags 'json1 fts5' ./application_context/... -run TestPluginMRQLAdapter -v`
Expected: FAIL — `pluginMRQLAdapter` not defined.

- [ ] **Step 3: Implement the adapter**

Create `application_context/plugin_mrql_adapter.go`:

```go
package application_context

import (
	"context"
	"encoding/json"
	"fmt"

	"mahresources/models"
	"mahresources/mrql"
	"mahresources/plugin_system"
)

// pluginMRQLAdapter implements plugin_system.MRQLExecutor using MahresourcesContext.
type pluginMRQLAdapter struct {
	ctx *MahresourcesContext
}

func (a *pluginMRQLAdapter) ExecuteMRQL(reqCtx context.Context, query string, opts plugin_system.MRQLExecOptions) (*plugin_system.MRQLResult, error) {
	parsed, err := mrql.Parse(query)
	if err != nil {
		return nil, err
	}
	if err := mrql.Validate(parsed); err != nil {
		return nil, err
	}

	if opts.Limit > 0 {
		parsed.Limit = opts.Limit
	}

	entityType := mrql.ExtractEntityType(parsed)
	if entityType == mrql.EntityUnspecified {
		return nil, fmt.Errorf("MRQL query must specify an entity type (e.g. type=resource)")
	}
	parsed.EntityType = entityType

	// GROUP BY path
	if parsed.GroupBy != nil {
		if opts.Buckets > 0 {
			parsed.BucketLimit = opts.Buckets
		}
		grouped, err := a.ctx.ExecuteMRQLGrouped(reqCtx, parsed)
		if err != nil {
			return nil, err
		}
		return a.convertGrouped(grouped, opts.ScopeID), nil
	}

	// Flat path — use TranslateOptions for scoping
	translateOpts := mrql.TranslateOptions{}
	result, err := a.ctx.ExecuteSingleEntityWithScope(reqCtx, parsed, entityType, translateOpts, opts.ScopeID)
	if err != nil {
		return nil, err
	}
	return a.convertFlat(result), nil
}

func (a *pluginMRQLAdapter) convertFlat(result *MRQLResult) *plugin_system.MRQLResult {
	pr := &plugin_system.MRQLResult{
		EntityType: result.EntityType,
		Mode:       "flat",
	}
	for _, r := range result.Resources {
		pr.Items = append(pr.Items, resourceToMap(&r))
	}
	for _, n := range result.Notes {
		pr.Items = append(pr.Items, noteToMap(&n))
	}
	for _, g := range result.Groups {
		pr.Items = append(pr.Items, groupToMap(&g))
	}
	return pr
}

func (a *pluginMRQLAdapter) convertGrouped(result *MRQLGroupedResult, scopeID uint) *plugin_system.MRQLResult {
	pr := &plugin_system.MRQLResult{
		EntityType: result.EntityType,
	}
	if result.Mode == "aggregated" {
		pr.Mode = "aggregated"
		pr.Rows = result.Rows
		return pr
	}
	pr.Mode = "bucketed"
	for _, bucket := range result.Groups {
		group := plugin_system.MRQLResultGroup{Key: bucket.Key}
		switch items := bucket.Items.(type) {
		case []models.Resource:
			for i := range items {
				group.Items = append(group.Items, resourceToMap(&items[i]))
			}
		case []models.Note:
			for i := range items {
				group.Items = append(group.Items, noteToMap(&items[i]))
			}
		case []models.Group:
			for i := range items {
				group.Items = append(group.Items, groupToMap(&items[i]))
			}
		}
		pr.Groups = append(pr.Groups, group)
	}
	return pr
}

// resourceToMap converts a Resource to a map with lowercase/camelCase keys
// matching MRQL field naming conventions.
func resourceToMap(r *models.Resource) map[string]any {
	m := map[string]any{
		"id":           float64(r.ID),
		"name":         r.Name,
		"description":  r.Description,
		"contentType":  r.ContentType,
		"fileSize":     float64(r.FileSize),
		"width":        float64(r.Width),
		"height":       float64(r.Height),
		"originalName": r.OriginalName,
		"hash":         r.Hash,
		"category":     r.Category,
		"entity_type":  "resource",
		"createdAt":    r.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		"updatedAt":    r.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
	if r.OwnerId != nil {
		m["ownerId"] = float64(*r.OwnerId)
	}
	if len(r.Meta) > 0 {
		var meta map[string]any
		if err := json.Unmarshal(r.Meta, &meta); err == nil {
			m["meta"] = meta
		}
	}
	return m
}

// noteToMap converts a Note to a map with lowercase/camelCase keys.
func noteToMap(n *models.Note) map[string]any {
	m := map[string]any{
		"id":          float64(n.ID),
		"name":        n.Name,
		"description": n.Description,
		"entity_type": "note",
		"createdAt":   n.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		"updatedAt":   n.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
	if n.OwnerId != nil {
		m["ownerId"] = float64(*n.OwnerId)
	}
	if n.StartDate != nil {
		m["startDate"] = n.StartDate.Format("2006-01-02T15:04:05Z07:00")
	}
	if n.EndDate != nil {
		m["endDate"] = n.EndDate.Format("2006-01-02T15:04:05Z07:00")
	}
	if len(n.Meta) > 0 {
		var meta map[string]any
		if err := json.Unmarshal(n.Meta, &meta); err == nil {
			m["meta"] = meta
		}
	}
	return m
}

// groupToMap converts a Group to a map with lowercase/camelCase keys.
func groupToMap(g *models.Group) map[string]any {
	m := map[string]any{
		"id":          float64(g.ID),
		"name":        g.Name,
		"description": g.Description,
		"entity_type": "group",
		"createdAt":   g.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		"updatedAt":   g.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
	if g.OwnerId != nil {
		m["ownerId"] = float64(*g.OwnerId)
	}
	if g.URL != nil {
		m["url"] = g.URL.String()
	}
	if len(g.Meta) > 0 {
		var meta map[string]any
		if err := json.Unmarshal(g.Meta, &meta); err == nil {
			m["meta"] = meta
		}
	}
	return m
}
```

Note: `ExecuteSingleEntityWithScope` does not exist yet. This is a thin wrapper around `executeSingleEntity` that applies the scope filter. Add it to `mrql_context.go`:

```go
// ExecuteSingleEntityWithScope is like executeSingleEntity but applies an
// additional owner_id scope filter before execution.
func (ctx *MahresourcesContext) ExecuteSingleEntityWithScope(reqCtx context.Context, q *mrql.Query, entityType mrql.EntityType, opts mrql.TranslateOptions, scopeID uint) (*MRQLResult, error) {
	if scopeID > 0 {
		// Wrap the existing WHERE with an owner_id filter via GORM scope
		return ctx.executeSingleEntityScoped(reqCtx, q, entityType, opts, scopeID)
	}
	return ctx.executeSingleEntity(reqCtx, q, entityType, opts)
}
```

The `executeSingleEntityScoped` function should clone the GORM DB, add `.Where("owner_id = ?", scopeID)`, and proceed with translation. This is implemented in Task 5.

- [ ] **Step 4: Wire up the adapter in context.go**

In `application_context/context.go`, after line 274 (`pm.SetKVStore(adapter)`), add:

```go
			mrqlAdapter := &pluginMRQLAdapter{ctx: ctx}
			pm.SetMRQLExecutor(mrqlAdapter)
```

- [ ] **Step 5: Run tests**

Run: `go test --tags 'json1 fts5' ./application_context/... -run TestPluginMRQLAdapter -v`
Expected: may fail if `ExecuteSingleEntityWithScope` is not yet implemented — that's ok, it's in Task 5.

- [ ] **Step 6: Commit**

```bash
git add application_context/plugin_mrql_adapter.go application_context/plugin_mrql_adapter_test.go application_context/context.go
git commit -m "feat: add MRQLExecutor adapter and wire up during startup"
```

---

### Task 5: Scope Resolution and Scoped Execution

**Files:**
- Modify: `application_context/mrql_context.go` (add ExecuteSingleEntityWithScope, executeSingleEntityScoped)
- Modify: `application_context/plugin_mrql_adapter.go` (scope resolution for parent/root)

- [ ] **Step 1: Write test for scoped execution**

Add to `application_context/plugin_mrql_adapter_test.go`:

```go
func TestPluginMRQLAdapterScopeResolution(t *testing.T) {
	ctx := setupEphemeralTestContext(t)

	adapter := &pluginMRQLAdapter{ctx: ctx}

	// Test scope resolution: looking up parent of a nonexistent entity
	// should return a sentinel (max uint >> 1) that matches nothing in DB.
	// NOT 0, because 0 means "no scope filter" = global fan-out.
	scopeID := adapter.resolveScope("parent", 999999, "group")
	assert.Equal(t, ^uint(0)>>1, scopeID)

	// scope="global" always returns 0
	scopeID = adapter.resolveScope("global", 1, "group")
	assert.Equal(t, uint(0), scopeID)

	// scope="entity" returns the entity ID itself
	scopeID = adapter.resolveScope("entity", 42, "group")
	assert.Equal(t, uint(42), scopeID)

	// scope="" (empty) defaults to entity
	scopeID = adapter.resolveScope("", 42, "group")
	assert.Equal(t, uint(42), scopeID)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test --tags 'json1 fts5' ./application_context/... -run TestPluginMRQLAdapterScope -v`
Expected: FAIL — `resolveScope` not defined.

- [ ] **Step 3: Implement scope resolution in plugin_mrql_adapter.go**

Add the `resolveScope` method to `pluginMRQLAdapter`:

```go
const maxScopeTraversalDepth = 50

// resolveScope converts a scope string + entity ID into a concrete owner_id for filtering.
// Returns 0 for "global" or unresolvable scopes (which means no scope filter is applied
// by the executor — but unresolvable non-global scopes result in empty results via a
// sentinel approach: the caller passes scopeID=0 and the executor skips the filter,
// but for unresolvable parent/root we use a nonexistent ID to guarantee empty results).
func (a *pluginMRQLAdapter) resolveScope(scope string, entityID uint, entityType string) uint {
	switch scope {
	case "global":
		return 0
	case "parent":
		ownerID := a.lookupOwnerID(entityID, entityType)
		if ownerID == 0 {
			// Entity has no parent — return a sentinel that will match nothing.
			// Using max uint32 as a nonexistent ID is safe for this purpose.
			return ^uint(0) >> 1
		}
		return ownerID
	case "root":
		// First hop: use the actual entity type (resource/note/group) to get
		// the entity's OwnerId. After that, we're walking groups.
		ownerID := a.lookupOwnerID(entityID, entityType)
		if ownerID == 0 {
			// Entity has no owner — "root" falls back to "entity" per spec
			return entityID
		}
		current := ownerID
		for i := 0; i < maxScopeTraversalDepth; i++ {
			parentID := a.lookupOwnerID(current, "group")
			if parentID == 0 {
				return current // this group is the root
			}
			current = parentID
		}
		return current // hit depth limit, use last found
	default: // "entity" or empty
		return entityID
	}
}

// lookupOwnerID returns the OwnerId of the given entity, or 0 if not found/nil.
func (a *pluginMRQLAdapter) lookupOwnerID(entityID uint, entityType string) uint {
	switch entityType {
	case "group":
		data, err := a.ctx.GetGroup(entityID)
		if err != nil || data == nil || data.OwnerId == nil {
			return 0
		}
		return *data.OwnerId
	case "resource":
		data, err := a.ctx.GetResource(entityID)
		if err != nil || data == nil || data.OwnerId == nil {
			return 0
		}
		return *data.OwnerId
	case "note":
		data, err := a.ctx.GetNote(entityID)
		if err != nil || data == nil || data.OwnerId == nil {
			return 0
		}
		return *data.OwnerId
	}
	return 0
}
```

- [ ] **Step 4: Implement ExecuteSingleEntityWithScope in mrql_context.go**

The scope filter MUST be applied at the GORM level before `.Find()`, not as in-memory filtering. In-memory filtering breaks LIMIT (you'd get fewer results than requested) and ordering (wrong rows survive). The injection point is between `TranslateWithOptions()` and the `.Find()` call — exactly where the translated `*gorm.DB` is ready but not yet executed.

Add to `application_context/mrql_context.go`:

```go
// ExecuteSingleEntityWithScope executes a single-entity MRQL query with an
// optional owner_id scope filter applied at the GORM level before execution.
// This ensures LIMIT, ORDER BY, and pagination operate on the scoped dataset.
func (ctx *MahresourcesContext) ExecuteSingleEntityWithScope(reqCtx context.Context, q *mrql.Query, entityType mrql.EntityType, opts mrql.TranslateOptions, scopeID uint) (*MRQLResult, error) {
	q.EntityType = entityType

	queryCtx, cancel := context.WithTimeout(reqCtx, MRQLQueryTimeout)
	defer cancel()

	db, err := mrql.TranslateWithOptions(q, ctx.db.WithContext(queryCtx), opts)
	if err != nil {
		return nil, err
	}

	if q.Limit < 0 {
		db = db.Limit(defaultMRQLLimit)
	}

	// Apply scope filter BEFORE execution so LIMIT/ORDER operate on scoped data
	if scopeID > 0 {
		db = db.Where("owner_id = ?", scopeID)
	}

	result := &MRQLResult{EntityType: entityType.String()}

	switch entityType {
	case mrql.EntityResource:
		var resources []models.Resource
		if err := db.Find(&resources).Error; err != nil {
			return nil, err
		}
		result.Resources = resources
	case mrql.EntityNote:
		var notes []models.Note
		if err := db.Find(&notes).Error; err != nil {
			return nil, err
		}
		result.Notes = notes
	case mrql.EntityGroup:
		var groups []models.Group
		if err := db.Find(&groups).Error; err != nil {
			return nil, err
		}
		result.Groups = groups
	}

	return result, nil
}
```

This duplicates the execution logic from `executeSingleEntity` rather than wrapping it — the scope MUST be injected between translation and execution, and `executeSingleEntity` doesn't expose that seam.

- [ ] **Step 5: Implement ExecuteMRQLGroupedWithScope in mrql_context.go**

The grouped path also needs scoping. Add:

```go
// ExecuteMRQLGroupedWithScope executes a GROUP BY MRQL query with an optional
// owner_id scope filter applied at the GORM level before aggregation/bucketing.
func (ctx *MahresourcesContext) ExecuteMRQLGroupedWithScope(reqCtx context.Context, parsed *mrql.Query, scopeID uint) (*MRQLGroupedResult, error) {
	if scopeID == 0 {
		return ctx.ExecuteMRQLGrouped(reqCtx, parsed)
	}

	queryCtx, cancel := context.WithTimeout(reqCtx, MRQLQueryTimeout)
	defer cancel()

	if parsed.Limit < 0 {
		parsed.Limit = defaultMRQLLimit
	}

	if len(parsed.GroupBy.Aggregates) > 0 {
		return ctx.executeAggregatedQueryScoped(queryCtx, parsed, scopeID)
	}

	if parsed.Limit > maxBucketedTotalItems {
		parsed.Limit = maxBucketedTotalItems
	}
	return ctx.executeBucketedQueryScoped(queryCtx, parsed, scopeID)
}
```

For `executeAggregatedQueryScoped` and `executeBucketedQueryScoped`, these are thin wrappers that call `TranslateGroupBy` / the bucketed path and inject `.Where("owner_id = ?", scopeID)` on the GORM DB before execution. Look at the existing `executeAggregatedQuery` and `executeBucketedQuery` implementations and add the scope injection at the same seam point (after translation, before execution).

- [ ] **Step 6: Update adapter to use scoped grouped execution**

In `plugin_mrql_adapter.go`, update the GROUP BY path in `ExecuteMRQL`:

```go
	// GROUP BY path
	if parsed.GroupBy != nil {
		if opts.Buckets > 0 {
			parsed.BucketLimit = opts.Buckets
		}
		grouped, err := a.ctx.ExecuteMRQLGroupedWithScope(reqCtx, parsed, opts.ScopeID)
		if err != nil {
			return nil, err
		}
		return a.convertGrouped(grouped, opts.ScopeID), nil
	}
```

- [ ] **Step 6: Run tests**

Run: `go test --tags 'json1 fts5' ./application_context/... -run TestPluginMRQLAdapter -v`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add application_context/mrql_context.go application_context/plugin_mrql_adapter.go application_context/plugin_mrql_adapter_test.go
git commit -m "feat: add scope resolution and scoped MRQL execution"
```

---

### Task 6: Request-Scoped MRQL Cache

**Files:**
- Create: `plugin_system/mrql_cache.go`

- [ ] **Step 1: Write test for the cache**

Create `plugin_system/mrql_cache_test.go`:

```go
package plugin_system

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

type cacheKey = string

func TestMRQLCacheHitAndMiss(t *testing.T) {
	ctx := context.Background()
	ctx = WithMRQLCache(ctx)

	cache := MRQLCacheFromContext(ctx)
	assert.NotNil(t, cache)

	key := MRQLCacheKey("type=resource", 0, 10, 5)

	// Miss
	result, ok := cache.Get(key)
	assert.False(t, ok)
	assert.Nil(t, result)

	// Store
	expected := &MRQLResult{Mode: "flat", EntityType: "resource"}
	cache.Put(key, expected)

	// Hit
	result, ok = cache.Get(key)
	assert.True(t, ok)
	assert.Equal(t, expected, result)
}

func TestMRQLCacheFromContextNil(t *testing.T) {
	// No cache in context — should return nil
	cache := MRQLCacheFromContext(context.Background())
	assert.Nil(t, cache)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test --tags 'json1 fts5' ./plugin_system/... -run TestMRQLCache -v`
Expected: FAIL — types not defined.

- [ ] **Step 3: Implement the cache**

Create `plugin_system/mrql_cache.go`:

```go
package plugin_system

import (
	"context"
	"fmt"
	"sync"
)

type mrqlCacheKey struct{}

// MRQLCache is a per-request cache for MRQL query results.
type MRQLCache struct {
	mu    sync.Mutex
	store map[string]*MRQLResult
}

// WithMRQLCache returns a new context with an empty MRQL cache attached.
func WithMRQLCache(ctx context.Context) context.Context {
	return context.WithValue(ctx, mrqlCacheKey{}, &MRQLCache{
		store: make(map[string]*MRQLResult),
	})
}

// MRQLCacheFromContext retrieves the MRQL cache from the context, or nil.
func MRQLCacheFromContext(ctx context.Context) *MRQLCache {
	v := ctx.Value(mrqlCacheKey{})
	if v == nil {
		return nil
	}
	return v.(*MRQLCache)
}

// MRQLCacheKey builds a deterministic cache key from query parameters.
func MRQLCacheKey(query string, scopeID uint, limit, buckets int) string {
	return fmt.Sprintf("%s|%d|%d|%d", query, scopeID, limit, buckets)
}

// Get returns a cached result and true, or nil and false.
func (c *MRQLCache) Get(key string) (*MRQLResult, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	r, ok := c.store[key]
	return r, ok
}

// Put stores a result in the cache.
func (c *MRQLCache) Put(key string, result *MRQLResult) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.store[key] = result
}
```

- [ ] **Step 4: Run tests**

Run: `go test --tags 'json1 fts5' ./plugin_system/... -run TestMRQLCache -v`
Expected: PASS

- [ ] **Step 5: Attach cache to request context in routes.go**

In `server/routes.go`, in the `processShortcodesForJSON` function (line 180), add cache setup at the top:

```go
func processShortcodesForJSON(ctx pongo2.Context, pm *plugin_system.PluginManager, appCtx *application_context.MahresourcesContext, reqCtx context.Context) {
	reqCtx = plugin_system.WithMRQLCache(reqCtx)
	// ... rest of function
```

Similarly, in `shortcode_tag.go`, when building `reqCtx` (line 62-68), wrap it:

```go
	reqCtx = plugin_system.WithMRQLCache(reqCtx)
```

And in `mrql_api_handlers.go`, where `renderMRQLCustomTemplates` is called, wrap the context.

- [ ] **Step 6: Commit**

```bash
git add plugin_system/mrql_cache.go plugin_system/mrql_cache_test.go server/routes.go server/template_handlers/template_filters/shortcode_tag.go server/api_handlers/mrql_api_handlers.go
git commit -m "feat: add request-scoped MRQL cache"
```

---

### Task 7: Register mah.db.mrql_query() Lua Function

**Files:**
- Modify: `plugin_system/db_api.go` (add mrql_query registration inside `registerDbModule`)

- [ ] **Step 1: Write test for the Lua function**

Add to `plugin_system/db_api_test.go`:

```go
func TestMRQLQueryLuaFunction(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "mrql-test", `
		plugin = { name = "mrql-test", version = "1.0" }
		function init()
			mah.shortcode({
				name = "mrqltest",
				label = "MRQL Test",
				render = function(ctx)
					local result, err = mah.db.mrql_query("type=resource", {
						scope_entity_id = 0,
						scope = "global",
						limit = 5,
					})
					if err then return "error:" .. err end
					if result == nil then return "nil" end
					return result.mode .. ":" .. result.entity_type
				end
			})
		end
	`)

	pm, err := NewPluginManager(dir)
	require.NoError(t, err)
	defer pm.Close()

	pm.SetMRQLExecutor(&mockMRQLExecutor{})

	require.NoError(t, pm.EnablePlugin("mrql-test"))

	html, err := pm.RenderShortcode(
		context.Background(),
		"mrql-test",
		"plugin:mrql-test:mrqltest",
		"group", 1,
		json.RawMessage(`{}`),
		map[string]string{},
		nil,
	)
	require.NoError(t, err)
	assert.Equal(t, "flat:resource", html)
}

type mockMRQLExecutor struct{}

func (m *mockMRQLExecutor) ExecuteMRQL(ctx context.Context, query string, opts MRQLExecOptions) (*MRQLResult, error) {
	return &MRQLResult{
		EntityType: "resource",
		Mode:       "flat",
		Items:      []map[string]any{{"id": float64(1), "name": "test"}},
	}, nil
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test --tags 'json1 fts5' ./plugin_system/... -run TestMRQLQueryLuaFunction -v`
Expected: FAIL — `mah.db.mrql_query` not registered.

- [ ] **Step 3: Register mah.db.mrql_query in registerDbModule**

In `plugin_system/db_api.go`, inside `registerDbModule`, before `mahMod.RawSetString("db", dbMod)` (line 698), add:

```go
	// mah.db.mrql_query(query, opts) -> result_table or (nil, error_string)
	dbMod.RawSetString("mrql_query", L.NewFunction(func(L *lua.LState) int {
		executor := pm.getMRQLExecutor()
		if executor == nil {
			L.Push(lua.LNil)
			L.Push(lua.LString("MRQL executor not available"))
			return 2
		}

		query := L.CheckString(1)
		optsTbl := L.OptTable(2, L.NewTable())
		optsMap := luaTableToGoMap(optsTbl)

		// Extract options
		limit := 20
		if v, ok := optsMap["limit"].(float64); ok && v > 0 {
			limit = int(v)
		}
		buckets := 5
		if v, ok := optsMap["buckets"].(float64); ok && v > 0 {
			buckets = int(v)
		}

		// Scope resolution
		var scopeID uint
		scopeEntityID := uint(0)
		if v, ok := optsMap["scope_entity_id"].(float64); ok {
			scopeEntityID = uint(v)
		}
		scopeStr := "entity"
		if v, ok := optsMap["scope"].(string); ok && v != "" {
			scopeStr = v
		}

		switch scopeStr {
		case "global":
			scopeID = 0
		case "parent":
			// Look up parent via EntityQuerier
			db := pm.getDbProvider()
			if db != nil && scopeEntityID > 0 {
				entityType := ""
				if v, ok := optsMap["entity_type"].(string); ok {
					entityType = v
				}
				scopeID = resolveParentScope(db, scopeEntityID, entityType)
			}
		case "root":
			db := pm.getDbProvider()
			if db != nil && scopeEntityID > 0 {
				scopeID = resolveRootScope(db, scopeEntityID)
			}
		default: // "entity"
			scopeID = scopeEntityID
		}

		execOpts := MRQLExecOptions{
			Limit:   limit,
			Buckets: buckets,
			ScopeID: scopeID,
		}

		// Check cache
		reqCtx := L.Context()
		if reqCtx == nil {
			reqCtx = context.Background()
		}
		cacheKey := MRQLCacheKey(query, scopeID, limit, buckets)
		if cache := MRQLCacheFromContext(reqCtx); cache != nil {
			if cached, ok := cache.Get(cacheKey); ok {
				L.Push(mrqlResultToLua(L, cached))
				return 1
			}
		}

		result, err := executor.ExecuteMRQL(reqCtx, query, execOpts)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}

		// Cache the result
		if cache := MRQLCacheFromContext(reqCtx); cache != nil {
			cache.Put(cacheKey, result)
		}

		L.Push(mrqlResultToLua(L, result))
		return 1
	}))
```

Add the helper functions:

```go
// resolveParentScope looks up the owner_id of an entity.
func resolveParentScope(db EntityQuerier, entityID uint, entityType string) uint {
	var data map[string]any
	var err error
	switch entityType {
	case "group":
		data, err = db.GetGroupData(entityID)
	case "resource":
		data, err = db.GetResourceData(entityID)
	case "note":
		data, err = db.GetNoteData(entityID)
	default:
		data, err = db.GetGroupData(entityID)
	}
	if err != nil || data == nil {
		return ^uint(0) >> 1 // sentinel: ensures empty results
	}
	if ownerID, ok := data["owner_id"].(float64); ok && ownerID > 0 {
		return uint(ownerID)
	}
	return ^uint(0) >> 1
}

// resolveRootScope walks the ownership chain to find the root.
func resolveRootScope(db EntityQuerier, entityID uint) uint {
	current := entityID
	for i := 0; i < 50; i++ {
		data, err := db.GetGroupData(current)
		if err != nil || data == nil {
			return current
		}
		ownerID, ok := data["owner_id"].(float64)
		if !ok || ownerID <= 0 {
			return current
		}
		current = uint(ownerID)
	}
	return current
}

// mrqlResultToLua converts an MRQLResult to a Lua table.
func mrqlResultToLua(L *lua.LState, result *MRQLResult) *lua.LTable {
	tbl := L.NewTable()
	tbl.RawSetString("mode", lua.LString(result.Mode))
	tbl.RawSetString("entity_type", lua.LString(result.EntityType))

	switch result.Mode {
	case "flat":
		items := L.NewTable()
		for i, item := range result.Items {
			items.RawSetInt(i+1, goToLuaTable(L, item))
		}
		tbl.RawSetString("items", items)
	case "aggregated":
		rows := L.NewTable()
		for i, row := range result.Rows {
			rows.RawSetInt(i+1, goToLuaTable(L, row))
		}
		tbl.RawSetString("rows", rows)
	case "bucketed":
		groups := L.NewTable()
		for i, g := range result.Groups {
			groupTbl := L.NewTable()
			groupTbl.RawSetString("key", goToLuaTable(L, g.Key))
			items := L.NewTable()
			for j, item := range g.Items {
				items.RawSetInt(j+1, goToLuaTable(L, item))
			}
			groupTbl.RawSetString("items", items)
			groups.RawSetInt(i+1, groupTbl)
		}
		tbl.RawSetString("groups", groups)
	}

	return tbl
}
```

Add `"context"` to the import block if not already present.

- [ ] **Step 4: Run test**

Run: `go test --tags 'json1 fts5' ./plugin_system/... -run TestMRQLQueryLuaFunction -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add plugin_system/db_api.go plugin_system/db_api_test.go
git commit -m "feat: register mah.db.mrql_query() Lua function with scope and cache"
```

---

### Task 8: Plugin Lua — resolve_data_source Helper

**Files:**
- Modify: `plugins/data-views/plugin.lua` (add helper after existing helpers, ~line 100)

- [ ] **Step 1: Add resolve_data_source helper function**

After the existing helper functions (around line 100, before the first shortcode), add:

```lua
-- ---------------------------------------------------------------------------
-- Data source resolution
-- ---------------------------------------------------------------------------

-- resolve_data_source(ctx) returns the data value for a shortcode.
-- Checks mrql > field > path. Returns (value, nil) or (nil, error_string).
local function resolve_data_source(ctx)
    local attrs = ctx.attrs or {}
    if attrs.mrql then
        local result, err = mah.db.mrql_query(attrs.mrql, {
            scope_entity_id = ctx.entity_id,
            scope = attrs.scope,
            entity_type = ctx.entity_type,
            limit = tonumber(attrs.limit),
            buckets = tonumber(attrs.buckets),
        })
        if err then return nil, err end
        return result, nil
    elseif attrs.field then
        if ctx.entity == nil then return nil, nil end
        return ctx.entity[attrs.field], nil
    else
        local path = attrs["path"]
        if not path then return nil, nil end
        return get_nested(ctx.value, path), nil
    end
end

-- resolve_scalar_from_mrql(result, aggregate_attr) extracts a single value
-- from an MRQL result for scalar shortcodes. For aggregated results, requires
-- the aggregate= attribute to select which column. For flat results, returns
-- the count of items.
local function resolve_scalar_from_mrql(result, aggregate_attr)
    if result == nil then return nil end
    if result.mode == "aggregated" and result.rows then
        if not aggregate_attr or aggregate_attr == "" then
            return nil, 'mrql aggregated results require aggregate="column_name" attribute'
        end
        local first_row = result.rows[1]
        if not first_row then return nil end
        return first_row[aggregate_attr], nil
    elseif result.mode == "flat" and result.items then
        return #result.items, nil
    elseif result.mode == "bucketed" and result.groups then
        return #result.groups, nil
    end
    return nil, nil
end

-- render_mrql_error(err) renders a styled error div for MRQL errors.
local function render_mrql_error(err)
    return string.format(
        '<div class="mrql-results mrql-error text-sm text-red-700 bg-red-50 '
        .. 'border border-red-200 rounded-md p-3 font-mono">%s</div>',
        html_escape(tostring(err)))
end
```

- [ ] **Step 2: Verify plugin loads**

Run: `go test --tags 'json1 fts5' ./plugin_system/... -v` (to verify Lua parses cleanly)

Alternatively, if there's a faster check: build and start the server in ephemeral mode, check logs for plugin load errors:
```bash
npm run build && timeout 5 ./mahresources -ephemeral 2>&1 | grep -i plugin || true
```

- [ ] **Step 3: Commit**

```bash
git add plugins/data-views/plugin.lua
git commit -m "feat: add resolve_data_source helper to data-views plugin"
```

---

### Task 9: Update Single-Value Shortcodes

**Files:**
- Modify: `plugins/data-views/plugin.lua` (badge, format, stat-card, meter, barcode, qr-code, link-preview, conditional)

Each single-value shortcode currently reads `path` and calls `get_nested(ctx.value, path)`. Update each to use `resolve_data_source` instead. The pattern is the same for all — replace the path-reading block with the new helper.

- [ ] **Step 1: Update render_badge (line ~479)**

Replace the path-reading block:
```lua
local function render_badge(ctx)
    local attrs = ctx.attrs or {}
    local path = attrs["path"]
    if not path then return shortcode_error("badge", '"path" attribute is required') end

    local val = get_nested(ctx.value, path)
```

With:
```lua
local function render_badge(ctx)
    local attrs = ctx.attrs or {}

    local val, err = resolve_data_source(ctx)
    if err then return render_mrql_error(err) end
    if val == nil then return '<div class="py-1.5"></div>' end

    -- If MRQL result, extract scalar
    if type(val) == "table" and val.mode then
        val, err = resolve_scalar_from_mrql(val, attrs.aggregate)
        if err then return render_mrql_error(err) end
    end

    if val == nil then return '<div class="py-1.5"></div>' end
```

Remove the old nil check (`if val == nil then return '<div class="py-1.5"></div>' end`) that followed `get_nested` — it's now handled above.

- [ ] **Step 2: Update render_format (line ~514)**

Replace:
```lua
    local path = attrs["path"]
    if not path then return shortcode_error("format", '"path" attribute is required') end

    local val = get_nested(ctx.value, path)
```

With:
```lua
    local val, err = resolve_data_source(ctx)
    if err then return render_mrql_error(err) end

    -- If MRQL result, extract scalar
    if type(val) == "table" and val.mode then
        val, err = resolve_scalar_from_mrql(val, attrs.aggregate)
        if err then return render_mrql_error(err) end
    end
```

Also update the `title` attribute in the format output: replace `html_escape(path)` with `html_escape(attrs.path or attrs.field or attrs.mrql or "")`.

- [ ] **Step 3: Update render_stat_card (line ~534)**

Same pattern — replace path-reading with `resolve_data_source` + `resolve_scalar_from_mrql`.

- [ ] **Step 4: Update render_meter (line ~562)**

Same pattern.

- [ ] **Step 5: Update render_barcode (line ~1025)**

Same pattern.

- [ ] **Step 6: Update render_qr_code (line ~1056)**

Same pattern.

- [ ] **Step 7: Update render_link_preview (line ~1090)**

Same pattern.

- [ ] **Step 8: Update render_conditional (line ~1344)**

The conditional shortcode checks a field value against conditions. Replace path-reading with `resolve_data_source` + scalar extraction.

- [ ] **Step 9: Verify plugin loads and existing shortcodes still work**

Run: `npm run build && cd e2e && npm run test:with-server`
Expected: existing tests pass (shortcodes using `path` still work since `resolve_data_source` falls through to `get_nested` when no `field` or `mrql` is present).

- [ ] **Step 10: Commit**

```bash
git add plugins/data-views/plugin.lua
git commit -m "feat: update single-value shortcodes to support field and mrql"
```

---

### Task 10: Update Collection Shortcodes

**Files:**
- Modify: `plugins/data-views/plugin.lua` (table, list, sparkline, bar-chart, pie-chart, count-badge, timeline-chart)

Collection shortcodes work differently — they need to handle MRQL result tables (arrays of items or aggregated rows), not just single values.

- [ ] **Step 1: Update render_table (line ~699)**

The table shortcode currently queries entities via `mah.db.query_*`. Add MRQL support:

After the existing attrs parsing, add before the `query_fn` selection:

```lua
    -- MRQL data source
    if attrs.mrql then
        local result, err = resolve_data_source(ctx)
        if err then return render_mrql_error(err) end
        if result == nil then return "" end
        return render_mrql_table(result, cols, labels, attrs)
    end
```

Add a helper above the shortcode definitions:

```lua
-- render_mrql_table renders an MRQL result as an HTML table.
local function render_mrql_table(result, cols, labels, attrs)
    local rows = {}
    if result.mode == "aggregated" and result.rows then
        rows = result.rows
        -- Auto-detect columns from first row if not specified
        if #cols == 0 and rows[1] then
            for k, _ in pairs(rows[1]) do
                cols[#cols + 1] = k
                labels[#labels + 1] = k
            end
        end
    elseif result.mode == "flat" and result.items then
        rows = result.items
        if #cols == 0 then
            cols = {"name", "entity_type", "createdAt"}
            labels = {"Name", "Type", "Created"}
        end
    elseif result.mode == "bucketed" and result.groups then
        -- Flatten bucketed results
        for _, group in ipairs(result.groups) do
            for _, item in ipairs(group.items or {}) do
                rows[#rows + 1] = item
            end
        end
        if #cols == 0 then
            cols = {"name", "entity_type", "createdAt"}
            labels = {"Name", "Type", "Created"}
        end
    end

    if #rows == 0 then
        return '<p class="text-sm text-stone-500 py-2 text-center">No results.</p>'
    end

    -- Fill missing labels
    for i = #labels + 1, #cols do labels[i] = cols[i] end

    local parts = {}
    parts[#parts + 1] = '<div class="overflow-x-auto"><table class="min-w-full text-sm border border-stone-200 rounded-md">'
    parts[#parts + 1] = '<thead class="bg-stone-100"><tr>'
    for _, label in ipairs(labels) do
        parts[#parts + 1] = '<th class="px-3 py-2 text-left text-xs font-semibold text-stone-600 uppercase border-b border-stone-200">' .. html_escape(label) .. '</th>'
    end
    parts[#parts + 1] = '</tr></thead><tbody class="divide-y divide-stone-100">'
    for _, row in ipairs(rows) do
        parts[#parts + 1] = '<tr class="hover:bg-stone-50">'
        for _, col in ipairs(cols) do
            local cell = row[col]
            if cell == nil then cell = "" end
            parts[#parts + 1] = '<td class="px-3 py-2 text-stone-800">' .. html_escape(tostring(cell)) .. '</td>'
        end
        parts[#parts + 1] = '</tr>'
    end
    parts[#parts + 1] = '</tbody></table></div>'
    return table.concat(parts)
end
```

- [ ] **Step 2: Update render_list (line ~800)**

Add MRQL support at the top of the function, after attrs parsing:

```lua
    if attrs.mrql then
        local result, err = resolve_data_source(ctx)
        if err then return render_mrql_error(err) end
        if result == nil then return "" end
        -- Extract items from result
        local items = {}
        if result.mode == "flat" and result.items then
            for _, item in ipairs(result.items) do
                items[#items + 1] = item.name or tostring(item.id or "")
            end
        elseif result.mode == "aggregated" and result.rows then
            for _, row in ipairs(result.rows) do
                local parts = {}
                for k, v in pairs(row) do parts[#parts + 1] = k .. ": " .. tostring(v) end
                items[#items + 1] = table.concat(parts, ", ")
            end
        end
        -- Render with existing list rendering logic (reuse the format variable)
        local val = items
        -- Fall through to existing rendering with val as array
        -- (set val and skip the path-based resolution below)
        return render_list_from_array(val, attrs)
    end
```

Add `render_list_from_array` helper that extracts the list rendering logic from the existing function.

- [ ] **Step 3: Update render_sparkline, render_bar_chart, render_pie_chart, render_count_badge**

Same pattern for each — add MRQL branch at the top that calls `resolve_data_source`, extracts the appropriate data shape, and feeds it to the existing rendering logic.

- [ ] **Step 4: Update render_timeline_chart**

Add MRQL support — uses flat results with date fields from entity items.

- [ ] **Step 5: Run E2E tests**

Run: `cd e2e && npm run test:with-server`
Expected: all existing tests pass.

- [ ] **Step 6: Commit**

```bash
git add plugins/data-views/plugin.lua
git commit -m "feat: update collection shortcodes to support mrql data source"
```

---

### Task 11: E2E Tests

**Files:**
- Create: `e2e/tests/data-views-sources.spec.ts`

- [ ] **Step 1: Write E2E test for field data source**

Create `e2e/tests/data-views-sources.spec.ts`:

```typescript
import { test, expect } from '../fixtures/base.fixture';

test.describe('Data-views extended data sources', () => {
  test('field attribute reads entity property in stat-card', async ({ page, apiClient }) => {
    // Create a resource category with a CustomMRQLResult using field=
    const category = await apiClient.createResourceCategory({
      name: 'Field Test Category',
      customMRQLResult: '[plugin:data-views:format field="FileSize" type="filesize"]',
    });

    // Create a resource in that category
    const resource = await apiClient.createResource({
      name: 'test-file.txt',
      resourceCategoryId: category.id,
      // resource will have a FileSize from the uploaded file
    });

    // Visit the resource page
    await page.goto(`/resource?id=${resource.id}`);

    // The format shortcode should render the file size
    await expect(page.locator('.font-mono')).toBeVisible();
  });

  test('mrql attribute renders query results in table', async ({ page, apiClient }) => {
    // Create a group category with CustomHeader using mrql=
    const category = await apiClient.createCategory({
      name: 'MRQL Test Category',
      customHeader: '[plugin:data-views:table mrql="type=resource" cols="name,contentType"]',
    });

    // Create a group and some resources owned by it
    const group = await apiClient.createGroup({
      name: 'Test Group',
      categoryId: category.id,
    });

    await apiClient.createResource({
      name: 'photo.jpg',
      ownerId: group.id,
    });

    // Visit the group page
    await page.goto(`/group?id=${group.id}`);

    // The table should render with resource data
    await expect(page.locator('table')).toBeVisible();
    await expect(page.getByText('photo.jpg')).toBeVisible();
  });
});
```

Adjust test details based on the actual API client helpers available in `e2e/helpers/`.

- [ ] **Step 2: Run the E2E tests**

Run: `cd e2e && npm run test:with-server -- --grep "Data-views extended"`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add e2e/tests/data-views-sources.spec.ts
git commit -m "test: add E2E tests for data-views field and mrql sources"
```

---

### Task 12: Full Test Suite Verification

- [ ] **Step 1: Run Go unit tests**

Run: `go test --tags 'json1 fts5' ./...`
Expected: all pass.

- [ ] **Step 2: Run full E2E test suite**

Run: `cd e2e && npm run test:with-server:all`
Expected: all pass.

- [ ] **Step 3: Run Postgres tests**

Run: `go test --tags 'json1 fts5 postgres' ./mrql/... ./server/api_tests/... -count=1 && cd e2e && npm run test:with-server:postgres`
Expected: all pass.

- [ ] **Step 4: Final commit if any fixes were needed**

```bash
git add -A
git commit -m "fix: address test failures from data-views extended sources"
```
