# Plugin-Defined Note Block Types Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Allow Lua plugins to define custom note block types that register into the existing block type registry, with full HTML rendering, JSON Schema validation, content/state separation, and a JS bridge for interactivity.

**Architecture:** Plugin block types implement the existing `BlockType` interface via a `PluginBlockType` struct that uses JSON Schema for validation and delegates rendering to Lua functions. They register into the same `block_types` global registry, namespaced as `plugin:<pluginName>:<type>`. The frontend catches plugin types with a generic `blockPlugin` component that fetches rendered HTML from a new render endpoint.

**Tech Stack:** Go (gopher-lua, santhosh-tekuri/jsonschema), Alpine.js, Pongo2 templates

---

### Task 1: Add JSON Schema Validation Dependency

**Files:**
- Modify: `go.mod`

**Step 1: Add the dependency**

Run: `cd /Users/egecan/Code/mahresources && go get github.com/santhosh-tekuri/jsonschema/v6@latest`

**Step 2: Verify it was added**

Run: `grep santhosh go.mod`
Expected: `github.com/santhosh-tekuri/jsonschema/v6 vX.X.X`

**Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "deps: add santhosh-tekuri/jsonschema for plugin block validation"
```

---

### Task 2: Add UnregisterBlockType to Registry

**Files:**
- Modify: `models/block_types/registry.go:17` (after RegisterBlockType)
- Test: `models/block_types/registry_test.go`

**Step 1: Write the failing test**

Add to `models/block_types/registry_test.go`:

```go
func TestUnregisterBlockType(t *testing.T) {
	// Register a test type
	RegisterBlockType(TextBlockType{})
	assert.NotNil(t, GetBlockType("text"))

	// Unregister it
	UnregisterBlockType("text")
	assert.Nil(t, GetBlockType("text"))

	// Re-register for other tests
	RegisterBlockType(TextBlockType{})
}

func TestUnregisterBlockType_NonExistent(t *testing.T) {
	// Should not panic
	UnregisterBlockType("nonexistent_type_xyz")
}
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/egecan/Code/mahresources && go test ./models/block_types/ -run TestUnregisterBlockType -v`
Expected: FAIL — `UnregisterBlockType` not defined

**Step 3: Write minimal implementation**

Add to `models/block_types/registry.go` after `RegisterBlockType`:

```go
// UnregisterBlockType removes a block type from the registry.
// No-op if the type name is not registered.
func UnregisterBlockType(typeName string) {
	mu.Lock()
	defer mu.Unlock()
	delete(registry, typeName)
}
```

**Step 4: Run tests to verify they pass**

Run: `cd /Users/egecan/Code/mahresources && go test ./models/block_types/ -run TestUnregisterBlockType -v`
Expected: PASS

**Step 5: Commit**

```bash
git add models/block_types/registry.go models/block_types/registry_test.go
git commit -m "feat(blocks): add UnregisterBlockType to registry"
```

---

### Task 3: Add PluginBlockType Struct and Interface Methods

**Files:**
- Create: `plugin_system/block_types.go`
- Test: `plugin_system/block_types_test.go`

**Step 1: Write the failing test**

Create `plugin_system/block_types_test.go`:

```go
package plugin_system

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPluginBlockType_Type(t *testing.T) {
	pbt := &PluginBlockType{
		PluginName: "my-plugin",
		TypeName:   "plugin:my-plugin:kanban",
	}
	assert.Equal(t, "plugin:my-plugin:kanban", pbt.Type())
}

func TestPluginBlockType_Defaults(t *testing.T) {
	pbt := &PluginBlockType{
		DefContent: json.RawMessage(`{"columns":[]}`),
		DefState:   json.RawMessage(`{}`),
	}
	assert.Equal(t, json.RawMessage(`{"columns":[]}`), pbt.DefaultContent())
	assert.Equal(t, json.RawMessage(`{}`), pbt.DefaultState())
}

func TestPluginBlockType_ValidateContent_NoSchema(t *testing.T) {
	pbt := &PluginBlockType{}
	// No schema = accept all
	err := pbt.ValidateContent(json.RawMessage(`{"anything":"goes"}`))
	assert.NoError(t, err)
}

func TestPluginBlockType_ValidateContent_WithSchema(t *testing.T) {
	schema := `{"type":"object","properties":{"text":{"type":"string"}},"required":["text"]}`
	pbt, err := NewPluginBlockType("test-plugin", "mytype", "My Type", "", "", schema, "", json.RawMessage(`{"text":""}`), json.RawMessage(`{}`), nil)
	require.NoError(t, err)

	// Valid
	err = pbt.ValidateContent(json.RawMessage(`{"text":"hello"}`))
	assert.NoError(t, err)

	// Invalid — missing required field
	err = pbt.ValidateContent(json.RawMessage(`{}`))
	assert.Error(t, err)

	// Invalid — wrong type
	err = pbt.ValidateContent(json.RawMessage(`{"text":123}`))
	assert.Error(t, err)
}

func TestPluginBlockType_ValidateState_WithSchema(t *testing.T) {
	stateSchema := `{"type":"object","properties":{"collapsed":{"type":"array"}}}`
	pbt, err := NewPluginBlockType("test-plugin", "mytype", "My Type", "", "", "", stateSchema, json.RawMessage(`{}`), json.RawMessage(`{}`), nil)
	require.NoError(t, err)

	err = pbt.ValidateState(json.RawMessage(`{"collapsed":[]}`))
	assert.NoError(t, err)

	err = pbt.ValidateState(json.RawMessage(`{"collapsed":"not-array"}`))
	assert.Error(t, err)
}

func TestNewPluginBlockType_InvalidSchema(t *testing.T) {
	_, err := NewPluginBlockType("test-plugin", "mytype", "My Type", "", "", `{"type":"invalid_type_here"}`, "", json.RawMessage(`{}`), json.RawMessage(`{}`), nil)
	assert.Error(t, err)
}
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/egecan/Code/mahresources && go test ./plugin_system/ -run TestPluginBlockType -v`
Expected: FAIL — types not defined

**Step 3: Write the implementation**

Create `plugin_system/block_types.go`:

```go
package plugin_system

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
	lua "github.com/yuin/gopher-lua"
)

// BlockTypeFilter restricts which notes can use a plugin block type.
type BlockTypeFilter struct {
	NoteTypeIDs []uint `json:"note_type_ids,omitempty"`
	CategoryIDs []uint `json:"category_ids,omitempty"`
}

// PluginBlockType implements block_types.BlockType for plugin-defined blocks.
type PluginBlockType struct {
	PluginName    string
	TypeName      string // full namespaced: plugin:<pluginName>:<type>
	Label         string
	Icon          string
	Description   string
	contentSchema *jsonschema.Schema
	stateSchema   *jsonschema.Schema
	DefContent    json.RawMessage
	DefState      json.RawMessage
	Filters       BlockTypeFilter
	RenderView    *lua.LFunction
	RenderEdit    *lua.LFunction
	State         *lua.LState // Lua VM for rendering
}

// NewPluginBlockType creates a PluginBlockType, compiling JSON Schemas if provided.
// contentSchemaJSON and stateSchemaJSON are raw JSON Schema strings (empty = accept all).
func NewPluginBlockType(
	pluginName, typeName, label, icon, description string,
	contentSchemaJSON, stateSchemaJSON string,
	defContent, defState json.RawMessage,
	filters *BlockTypeFilter,
) (*PluginBlockType, error) {
	fullName := "plugin:" + pluginName + ":" + typeName

	pbt := &PluginBlockType{
		PluginName:  pluginName,
		TypeName:    fullName,
		Label:       label,
		Icon:        icon,
		Description: description,
		DefContent:  defContent,
		DefState:    defState,
	}

	if filters != nil {
		pbt.Filters = *filters
	}

	if contentSchemaJSON != "" {
		s, err := compileSchema(fullName+"/content", contentSchemaJSON)
		if err != nil {
			return nil, fmt.Errorf("invalid content_schema: %w", err)
		}
		pbt.contentSchema = s
	}

	if stateSchemaJSON != "" {
		s, err := compileSchema(fullName+"/state", stateSchemaJSON)
		if err != nil {
			return nil, fmt.Errorf("invalid state_schema: %w", err)
		}
		pbt.stateSchema = s
	}

	return pbt, nil
}

func compileSchema(id, schemaJSON string) (*jsonschema.Schema, error) {
	c := jsonschema.NewCompiler()
	if err := c.AddResource(id, strings.NewReader(schemaJSON)); err != nil {
		return nil, err
	}
	return c.Compile(id)
}

func (p *PluginBlockType) Type() string {
	return p.TypeName
}

func (p *PluginBlockType) ValidateContent(content json.RawMessage) error {
	if p.contentSchema == nil {
		return nil
	}
	return validateAgainstSchema(p.contentSchema, content)
}

func (p *PluginBlockType) ValidateState(state json.RawMessage) error {
	if p.stateSchema == nil {
		return nil
	}
	return validateAgainstSchema(p.stateSchema, state)
}

func validateAgainstSchema(schema *jsonschema.Schema, data json.RawMessage) error {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	return schema.Validate(v)
}

func (p *PluginBlockType) DefaultContent() json.RawMessage {
	if p.DefContent == nil {
		return json.RawMessage(`{}`)
	}
	return p.DefContent
}

func (p *PluginBlockType) DefaultState() json.RawMessage {
	if p.DefState == nil {
		return json.RawMessage(`{}`)
	}
	return p.DefState
}
```

**Step 4: Run tests to verify they pass**

Run: `cd /Users/egecan/Code/mahresources && go test ./plugin_system/ -run TestPluginBlockType -v`
Expected: PASS

**Step 5: Run all existing tests to verify no regressions**

Run: `cd /Users/egecan/Code/mahresources && go test ./plugin_system/ -v -count=1`
Expected: All PASS

**Step 6: Commit**

```bash
git add plugin_system/block_types.go plugin_system/block_types_test.go
git commit -m "feat(plugins): add PluginBlockType struct with JSON Schema validation"
```

---

### Task 4: Add Block Type Storage and Lifecycle to PluginManager

**Files:**
- Modify: `plugin_system/manager.go:84` (add blockTypes field to struct)
- Modify: `plugin_system/manager.go:128` (initialize in NewPluginManager)
- Modify: `plugin_system/manager.go:764` (cleanup in DisablePlugin, after actions cleanup)
- Modify: `plugin_system/manager.go:911` (cleanup in Close)
- Test: `plugin_system/block_types_test.go`

**Step 1: Write the failing test**

Add to `plugin_system/block_types_test.go`:

```go
func TestPluginManager_BlockTypeLifecycle(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "test-blocks", `
plugin = { name = "test-blocks", version = "1.0", description = "test" }

function init()
    mah.block_type({
        type = "kanban",
        label = "Kanban Board",
        icon = "board",
        description = "A kanban board",
        default_content = { columns = {} },
        default_state = {},
        render_view = function(ctx) return "<div>view</div>" end,
        render_edit = function(ctx) return "<div>edit</div>" end
    })
end
`)
	pm, err := NewPluginManager(dir)
	require.NoError(t, err)
	defer pm.Close()

	err = pm.EnablePlugin("test-blocks")
	require.NoError(t, err)

	// Verify block type is registered
	types := pm.GetBlockTypes()
	require.Len(t, types, 1)
	assert.Equal(t, "plugin:test-blocks:kanban", types[0].TypeName)
	assert.Equal(t, "Kanban Board", types[0].Label)
	assert.Equal(t, "board", types[0].Icon)

	// Disable and verify cleanup
	err = pm.DisablePlugin("test-blocks")
	require.NoError(t, err)
	assert.Empty(t, pm.GetBlockTypes())
}

func TestPluginManager_BlockTypeDuplicateID(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "dup-blocks", `
plugin = { name = "dup-blocks", version = "1.0", description = "test" }

function init()
    mah.block_type({
        type = "kanban",
        label = "Kanban",
        render_view = function(ctx) return "" end,
        render_edit = function(ctx) return "" end
    })
    mah.block_type({
        type = "kanban",
        label = "Kanban Again",
        render_view = function(ctx) return "" end,
        render_edit = function(ctx) return "" end
    })
end
`)
	pm, err := NewPluginManager(dir)
	require.NoError(t, err)
	defer pm.Close()

	err = pm.EnablePlugin("dup-blocks")
	assert.Error(t, err) // Should fail due to duplicate type
}

func TestPluginManager_BlockTypeMissingRequired(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "bad-blocks", `
plugin = { name = "bad-blocks", version = "1.0", description = "test" }

function init()
    mah.block_type({
        type = "kanban"
        -- missing label, render_view, render_edit
    })
end
`)
	pm, err := NewPluginManager(dir)
	require.NoError(t, err)
	defer pm.Close()

	err = pm.EnablePlugin("bad-blocks")
	assert.Error(t, err)
}
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/egecan/Code/mahresources && go test ./plugin_system/ -run TestPluginManager_BlockType -v`
Expected: FAIL — `mah.block_type` not defined, `GetBlockTypes` not defined

**Step 3: Implement the changes**

In `plugin_system/manager.go`, add to `PluginManager` struct (after line 85):

```go
	blockTypes map[string][]*PluginBlockType // pluginName -> block types
```

In `NewPluginManager`, add to initialization (after line 128):

```go
		blockTypes:      make(map[string][]*PluginBlockType),
```

In `DisablePlugin`, add after line 764 (`delete(pm.actions, name)`):

```go
	// Unregister plugin block types from global registry and remove from local map.
	for _, pbt := range pm.blockTypes[name] {
		block_types.UnregisterBlockType(pbt.TypeName)
	}
	delete(pm.blockTypes, name)
```

In `Close`, add after line 911 (`pm.actions = nil`):

```go
	pm.blockTypes = nil
```

Add the `mah.block_type()` binding in `registerMahModule` (after the `mah.action` block, around line 467):

```go
	mahMod.RawSetString("block_type", L.NewFunction(func(L *lua.LState) int {
		tbl := L.CheckTable(1)
		pbt, err := parseBlockTypeTable(L, tbl, *pluginNamePtr)
		if err != nil {
			L.ArgError(1, err.Error())
			return 0
		}
		pbt.State = L

		pm.mu.Lock()
		for _, existing := range pm.blockTypes[*pluginNamePtr] {
			if existing.TypeName == pbt.TypeName {
				pm.mu.Unlock()
				L.ArgError(1, fmt.Sprintf("duplicate block type %q", pbt.TypeName))
				return 0
			}
		}
		pm.blockTypes[*pluginNamePtr] = append(pm.blockTypes[*pluginNamePtr], pbt)
		pm.mu.Unlock()

		block_types.RegisterBlockType(pbt)
		return 0
	}))
```

Add a getter method (e.g. near `GetActions`):

```go
// GetBlockTypes returns all plugin-registered block types.
func (pm *PluginManager) GetBlockTypes() []*PluginBlockType {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	var result []*PluginBlockType
	for _, types := range pm.blockTypes {
		result = append(result, types...)
	}
	return result
}

// GetBlockType returns a specific plugin block type, or nil.
func (pm *PluginManager) GetPluginBlockType(fullTypeName string) *PluginBlockType {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	for _, types := range pm.blockTypes {
		for _, pbt := range types {
			if pbt.TypeName == fullTypeName {
				return pbt
			}
		}
	}
	return nil
}
```

Add the import for `block_types` at the top of `manager.go`:

```go
	"mahresources/models/block_types"
```

**Step 4: Implement `parseBlockTypeTable`**

Add to `plugin_system/block_types.go`:

```go
import (
	// add to existing imports
	"regexp"
)

var validBlockTypeName = regexp.MustCompile(`^[a-z][a-z0-9-]{0,49}$`)

// parseBlockTypeTable parses a Lua table into a PluginBlockType.
// Required fields: type, label, render_view, render_edit.
func parseBlockTypeTable(L *lua.LState, tbl *lua.LTable, pluginName string) (*PluginBlockType, error) {
	// Required: type
	typeName := ""
	if v := tbl.RawGetString("type"); v == lua.LNil {
		return nil, fmt.Errorf("missing required field 'type'")
	} else {
		typeName = strings.ToLower(v.String())
	}
	if !validBlockTypeName.MatchString(typeName) {
		return nil, fmt.Errorf("invalid block type name %q: must be lowercase alphanumeric with hyphens, max 50 chars", typeName)
	}

	// Required: label
	label := ""
	if v := tbl.RawGetString("label"); v == lua.LNil {
		return nil, fmt.Errorf("missing required field 'label'")
	} else {
		label = v.String()
	}

	// Required: render_view
	var renderView *lua.LFunction
	if v := tbl.RawGetString("render_view"); v == lua.LNil {
		return nil, fmt.Errorf("missing required field 'render_view'")
	} else if fn, ok := v.(*lua.LFunction); !ok {
		return nil, fmt.Errorf("'render_view' must be a function")
	} else {
		renderView = fn
	}

	// Required: render_edit
	var renderEdit *lua.LFunction
	if v := tbl.RawGetString("render_edit"); v == lua.LNil {
		return nil, fmt.Errorf("missing required field 'render_edit'")
	} else if fn, ok := v.(*lua.LFunction); !ok {
		return nil, fmt.Errorf("'render_edit' must be a function")
	} else {
		renderEdit = fn
	}

	// Optional: icon
	icon := ""
	if v := tbl.RawGetString("icon"); v != lua.LNil {
		icon = v.String()
	}

	// Optional: description
	description := ""
	if v := tbl.RawGetString("description"); v != lua.LNil {
		description = v.String()
	}

	// Optional: content_schema (Lua table → JSON string)
	contentSchemaJSON := ""
	if v := tbl.RawGetString("content_schema"); v != lua.LNil {
		if schemaTbl, ok := v.(*lua.LTable); ok {
			data := luaValueToGo(schemaTbl)
			b, err := json.Marshal(data)
			if err != nil {
				return nil, fmt.Errorf("content_schema: %w", err)
			}
			contentSchemaJSON = string(b)
		}
	}

	// Optional: state_schema
	stateSchemaJSON := ""
	if v := tbl.RawGetString("state_schema"); v != lua.LNil {
		if schemaTbl, ok := v.(*lua.LTable); ok {
			data := luaValueToGo(schemaTbl)
			b, err := json.Marshal(data)
			if err != nil {
				return nil, fmt.Errorf("state_schema: %w", err)
			}
			stateSchemaJSON = string(b)
		}
	}

	// Optional: default_content
	defContent := json.RawMessage(`{}`)
	if v := tbl.RawGetString("default_content"); v != lua.LNil {
		data := luaValueToGo(v)
		b, err := json.Marshal(data)
		if err != nil {
			return nil, fmt.Errorf("default_content: %w", err)
		}
		defContent = b
	}

	// Optional: default_state
	defState := json.RawMessage(`{}`)
	if v := tbl.RawGetString("default_state"); v != lua.LNil {
		data := luaValueToGo(v)
		b, err := json.Marshal(data)
		if err != nil {
			return nil, fmt.Errorf("default_state: %w", err)
		}
		defState = b
	}

	// Optional: filters
	var filters *BlockTypeFilter
	if v := tbl.RawGetString("filters"); v != lua.LNil {
		if filtersTbl, ok := v.(*lua.LTable); ok {
			filters = &BlockTypeFilter{}
			if ni := filtersTbl.RawGetString("note_type_ids"); ni != lua.LNil {
				if niTbl, ok := ni.(*lua.LTable); ok {
					niTbl.ForEach(func(_, val lua.LValue) {
						if n, ok := val.(lua.LNumber); ok {
							filters.NoteTypeIDs = append(filters.NoteTypeIDs, uint(n))
						}
					})
				}
			}
			if ci := filtersTbl.RawGetString("category_ids"); ci != lua.LNil {
				if ciTbl, ok := ci.(*lua.LTable); ok {
					ciTbl.ForEach(func(_, val lua.LValue) {
						if n, ok := val.(lua.LNumber); ok {
							filters.CategoryIDs = append(filters.CategoryIDs, uint(n))
						}
					})
				}
			}
		}
	}

	pbt, err := NewPluginBlockType(pluginName, typeName, label, icon, description, contentSchemaJSON, stateSchemaJSON, defContent, defState, filters)
	if err != nil {
		return nil, err
	}
	pbt.RenderView = renderView
	pbt.RenderEdit = renderEdit
	return pbt, nil
}
```

**Step 5: Run tests**

Run: `cd /Users/egecan/Code/mahresources && go test ./plugin_system/ -run TestPluginManager_BlockType -v`
Expected: PASS

**Step 6: Run all plugin system tests**

Run: `cd /Users/egecan/Code/mahresources && go test ./plugin_system/ -v -count=1`
Expected: All PASS

**Step 7: Commit**

```bash
git add plugin_system/manager.go plugin_system/block_types.go plugin_system/block_types_test.go
git commit -m "feat(plugins): add mah.block_type() Lua binding with lifecycle management"
```

---

### Task 5: Add Block Render Method to PluginManager

**Files:**
- Create: `plugin_system/block_render.go`
- Test: `plugin_system/block_render_test.go`

**Step 1: Write the failing test**

Create `plugin_system/block_render_test.go`:

```go
package plugin_system

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderBlockView(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "render-test", `
plugin = { name = "render-test", version = "1.0", description = "test" }

function init()
    mah.block_type({
        type = "simple",
        label = "Simple",
        render_view = function(ctx)
            return "<div>View: " .. ctx.block.id .. " note:" .. ctx.note.id .. "</div>"
        end,
        render_edit = function(ctx)
            return "<div>Edit: " .. ctx.block.id .. "</div>"
        end
    })
end
`)
	pm, err := NewPluginManager(dir)
	require.NoError(t, err)
	defer pm.Close()

	require.NoError(t, pm.EnablePlugin("render-test"))

	blockCtx := BlockRenderContext{
		Block: BlockRenderData{
			ID:       42,
			Content:  map[string]any{"text": "hello"},
			State:    map[string]any{},
			Position: "n",
		},
		Note: NoteRenderData{
			ID:         10,
			Name:       "Test Note",
			NoteTypeID: 1,
		},
		Settings: map[string]any{},
	}

	html, err := pm.RenderBlock("render-test", "plugin:render-test:simple", "view", blockCtx)
	require.NoError(t, err)
	assert.Contains(t, html, "View: 42")
	assert.Contains(t, html, "note:10")

	html, err = pm.RenderBlock("render-test", "plugin:render-test:simple", "edit", blockCtx)
	require.NoError(t, err)
	assert.Contains(t, html, "Edit: 42")
}

func TestRenderBlock_PluginDisabled(t *testing.T) {
	dir := t.TempDir()
	pm, err := NewPluginManager(dir)
	require.NoError(t, err)
	defer pm.Close()

	_, err = pm.RenderBlock("nonexistent", "plugin:nonexistent:x", "view", BlockRenderContext{})
	assert.Error(t, err)
}
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/egecan/Code/mahresources && go test ./plugin_system/ -run TestRenderBlock -v`
Expected: FAIL — types not defined

**Step 3: Write the implementation**

Create `plugin_system/block_render.go`:

```go
package plugin_system

import (
	"context"
	"fmt"
	"log"
	"time"

	lua "github.com/yuin/gopher-lua"
)

const luaBlockRenderTimeout = 5 * time.Second

// BlockRenderData holds block data for the render context.
type BlockRenderData struct {
	ID       uint           `json:"id"`
	Content  map[string]any `json:"content"`
	State    map[string]any `json:"state"`
	Position string         `json:"position"`
}

// NoteRenderData holds note data for the render context.
type NoteRenderData struct {
	ID         uint   `json:"id"`
	Name       string `json:"name"`
	NoteTypeID uint   `json:"note_type_id"`
}

// BlockRenderContext holds all context passed to the Lua render function.
type BlockRenderContext struct {
	Block    BlockRenderData `json:"block"`
	Note     NoteRenderData  `json:"note"`
	Settings map[string]any  `json:"settings"`
}

// RenderBlock executes the Lua render function for a plugin block type
// and returns the rendered HTML string.
func (pm *PluginManager) RenderBlock(pluginName, fullTypeName, mode string, ctx BlockRenderContext) (string, error) {
	if pm.closed.Load() {
		return "", fmt.Errorf("plugin manager is closed")
	}

	pbt := pm.GetPluginBlockType(fullTypeName)
	if pbt == nil {
		return "", fmt.Errorf("block type %q not found", fullTypeName)
	}
	if pbt.PluginName != pluginName {
		return "", fmt.Errorf("block type %q does not belong to plugin %q", fullTypeName, pluginName)
	}

	var fn *lua.LFunction
	switch mode {
	case "view":
		fn = pbt.RenderView
	case "edit":
		fn = pbt.RenderEdit
	default:
		return "", fmt.Errorf("invalid render mode %q: must be 'view' or 'edit'", mode)
	}
	if fn == nil {
		return "", fmt.Errorf("no render_%s function for block type %q", mode, fullTypeName)
	}

	L := pbt.State
	mu := pm.VMLock(L)
	mu.Lock()
	defer mu.Unlock()

	// Build context table
	ctxData := map[string]any{
		"block": map[string]any{
			"id":       ctx.Block.ID,
			"content":  ctx.Block.Content,
			"state":    ctx.Block.State,
			"position": ctx.Block.Position,
		},
		"note": map[string]any{
			"id":           ctx.Note.ID,
			"name":         ctx.Note.Name,
			"note_type_id": ctx.Note.NoteTypeID,
		},
	}
	if ctx.Settings != nil {
		ctxData["settings"] = ctx.Settings
	} else {
		ctxData["settings"] = map[string]any{}
	}

	tbl := goToLuaTable(L, ctxData)

	timeoutCtx, cancel := context.WithTimeout(context.Background(), luaBlockRenderTimeout)
	L.SetContext(timeoutCtx)

	err := L.CallByParam(lua.P{
		Fn:      fn,
		NRet:    1,
		Protect: true,
	}, tbl)

	L.RemoveContext()
	cancel()

	if err != nil {
		log.Printf("[plugin] warning: block render %q/%q returned error: %v", pluginName, fullTypeName, err)
		return "", fmt.Errorf("block render error: %w", err)
	}

	ret := L.Get(-1)
	L.Pop(1)

	if str, ok := ret.(lua.LString); ok {
		return string(str), nil
	}

	return "", nil
}
```

**Step 4: Run tests**

Run: `cd /Users/egecan/Code/mahresources && go test ./plugin_system/ -run TestRenderBlock -v`
Expected: PASS

**Step 5: Commit**

```bash
git add plugin_system/block_render.go plugin_system/block_render_test.go
git commit -m "feat(plugins): add RenderBlock method for plugin block type rendering"
```

---

### Task 6: Extend BlockTypeInfo and GetBlockTypesHandler

**Files:**
- Modify: `server/api_handlers/block_api_handlers.go:168-194`
- Test: existing block API tests or manual verification

**Step 1: Update BlockTypeInfo struct**

In `server/api_handlers/block_api_handlers.go`, modify `BlockTypeInfo` (around line 168):

```go
type BlockTypeInfo struct {
	Type           string                       `json:"type"`
	DefaultContent json.RawMessage              `json:"defaultContent"`
	DefaultState   json.RawMessage              `json:"defaultState"`
	Label          string                       `json:"label,omitempty"`
	Icon           string                       `json:"icon,omitempty"`
	Description    string                       `json:"description,omitempty"`
	Plugin         bool                         `json:"plugin,omitempty"`
	PluginName     string                       `json:"pluginName,omitempty"`
	Filters        *plugin_system.BlockTypeFilter `json:"filters,omitempty"`
}
```

**Step 2: Update GetBlockTypesHandler to populate plugin metadata**

Modify `GetBlockTypesHandler` (around line 178):

```go
func GetBlockTypesHandler() func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		allTypes := block_types.GetAllBlockTypes()
		result := make([]BlockTypeInfo, 0, len(allTypes))

		for _, bt := range allTypes {
			info := BlockTypeInfo{
				Type:           bt.Type(),
				DefaultContent: bt.DefaultContent(),
				DefaultState:   bt.DefaultState(),
			}

			// Check if it's a plugin block type and populate extra metadata
			if pbt, ok := bt.(*plugin_system.PluginBlockType); ok {
				info.Label = pbt.Label
				info.Icon = pbt.Icon
				info.Description = pbt.Description
				info.Plugin = true
				info.PluginName = pbt.PluginName
				if len(pbt.Filters.NoteTypeIDs) > 0 || len(pbt.Filters.CategoryIDs) > 0 {
					info.Filters = &pbt.Filters
				}
			}

			result = append(result, info)
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(result)
	}
}
```

Add import for `plugin_system` at the top of the file:

```go
	"mahresources/plugin_system"
```

**Step 3: Build to verify compilation**

Run: `cd /Users/egecan/Code/mahresources && go build --tags 'json1 fts5'`
Expected: Success

**Step 4: Commit**

```bash
git add server/api_handlers/block_api_handlers.go
git commit -m "feat(blocks): extend BlockTypeInfo with plugin metadata"
```

---

### Task 7: Add Block Render API Endpoint

**Files:**
- Modify: `server/api_handlers/plugin_api_handlers.go` (add handler)
- Modify: `server/routes.go` (add route)
- Modify: `server/interfaces/block_interfaces.go` (add interface if needed)

**Step 1: Add the render handler**

Add to `server/api_handlers/plugin_api_handlers.go`:

```go
// GetPluginBlockRenderHandler renders a plugin block type's HTML for view or edit mode.
func GetPluginBlockRenderHandler(ctx interfaces.BlockReader) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		pm := ctx.PluginManager()
		if pm == nil {
			http.Error(writer, "plugins not available", http.StatusServiceUnavailable)
			return
		}

		vars := mux.Vars(request)
		pluginName := vars["pluginName"]
		if pluginName == "" {
			http.Error(writer, "plugin name required", http.StatusBadRequest)
			return
		}

		blockID := uint(http_utils.GetIntQueryParameter(request, "blockId", 0))
		if blockID == 0 {
			http.Error(writer, "blockId required", http.StatusBadRequest)
			return
		}

		mode := request.URL.Query().Get("mode")
		if mode != "view" && mode != "edit" {
			http.Error(writer, "mode must be 'view' or 'edit'", http.StatusBadRequest)
			return
		}

		// Fetch block from DB
		block, err := ctx.GetBlock(blockID)
		if err != nil {
			http.Error(writer, "block not found", http.StatusNotFound)
			return
		}

		// Verify block type belongs to this plugin
		if !strings.HasPrefix(block.Type, "plugin:"+pluginName+":") {
			http.Error(writer, "block type does not belong to this plugin", http.StatusBadRequest)
			return
		}

		// Fetch note for context
		note, err := ctx.GetNote(block.NoteID)
		if err != nil {
			http.Error(writer, "note not found", http.StatusNotFound)
			return
		}

		// Build render context
		var contentMap map[string]any
		if block.Content != nil {
			_ = json.Unmarshal(block.Content, &contentMap)
		}
		if contentMap == nil {
			contentMap = map[string]any{}
		}

		var stateMap map[string]any
		if block.State != nil {
			_ = json.Unmarshal(block.State, &stateMap)
		}
		if stateMap == nil {
			stateMap = map[string]any{}
		}

		renderCtx := plugin_system.BlockRenderContext{
			Block: plugin_system.BlockRenderData{
				ID:       block.ID,
				Content:  contentMap,
				State:    stateMap,
				Position: block.Position,
			},
			Note: plugin_system.NoteRenderData{
				ID:         note.ID,
				Name:       note.Name,
				NoteTypeID: note.NoteTypeID,
			},
			Settings: pm.GetPluginSettings(pluginName),
		}

		html, err := pm.RenderBlock(pluginName, block.Type, mode, renderCtx)
		if err != nil {
			http.Error(writer, err.Error(), http.StatusInternalServerError)
			return
		}

		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = writer.Write([]byte(html))
	}
}
```

**Note:** This handler needs an interface that provides both `GetBlock` and `GetNote`. Check the existing interfaces. If there's no combined interface, the handler should accept `appContext` directly (matching the pattern used by other handlers that need multiple entity access). Adjust the interface parameter accordingly — look at how `GetCalendarBlockEventsHandler` or `GetPluginActionHandler` receive their dependencies.

**Step 2: Check how existing handlers access multiple entity types**

Read `server/api_handlers/plugin_api_handlers.go` to see how `GetPluginActionHandler` gets its context. It likely uses a combined interface or the full app context. Match that pattern.

**Step 3: Add the route**

In `server/routes.go`, add near the plugin routes section (around line 360):

```go
router.Methods(http.MethodGet).Path("/v1/plugins/{pluginName}/block/render").HandlerFunc(
    api_handlers.GetPluginBlockRenderHandler(appContext),
)
```

**Step 4: Build and verify**

Run: `cd /Users/egecan/Code/mahresources && go build --tags 'json1 fts5'`
Expected: Success

**Step 5: Commit**

```bash
git add server/api_handlers/plugin_api_handlers.go server/routes.go
git commit -m "feat(plugins): add block render API endpoint"
```

---

### Task 8: Add Filter Enforcement to CreateBlock

**Files:**
- Modify: `application_context/block_context.go:27-32` (in CreateBlock, after type validation)

**Step 1: Add filter check**

In `application_context/block_context.go`, in the `CreateBlock` method, after the block type is validated (around line 32), add filter enforcement:

```go
	// Check plugin block type filters against the parent note
	if pbt, ok := bt.(*plugin_system.PluginBlockType); ok {
		if len(pbt.Filters.NoteTypeIDs) > 0 || len(pbt.Filters.CategoryIDs) > 0 {
			note, err := ctx.GetNote(editor.NoteID)
			if err != nil {
				return nil, fmt.Errorf("cannot verify block type filters: %w", err)
			}
			if len(pbt.Filters.NoteTypeIDs) > 0 {
				found := false
				for _, id := range pbt.Filters.NoteTypeIDs {
					if note.NoteTypeID == id {
						found = true
						break
					}
				}
				if !found {
					return nil, fmt.Errorf("block type %q is not available for this note type", editor.Type)
				}
			}
			// CategoryIDs would check against note's groups' categories — skip for v1
			// as notes don't have a direct category field
		}
	}
```

Add import for `plugin_system` at the top of the file.

**Step 2: Build and verify**

Run: `cd /Users/egecan/Code/mahresources && go build --tags 'json1 fts5'`
Expected: Success

**Step 3: Run existing block tests**

Run: `cd /Users/egecan/Code/mahresources && go test ./... -count=1`
Expected: All PASS

**Step 4: Commit**

```bash
git add application_context/block_context.go
git commit -m "feat(blocks): enforce plugin block type filters on creation"
```

---

### Task 9: Add blockPlugin.js Frontend Component

**Files:**
- Create: `src/components/blocks/blockPlugin.js`
- Modify: `src/components/blocks/index.js:10` (add export)
- Modify: `src/main.js:39-40` (add import)
- Modify: `src/main.js:94-102` (add Alpine.data registration)

**Step 1: Create the component**

Create `src/components/blocks/blockPlugin.js`:

```js
// src/components/blocks/blockPlugin.js
export function blockPlugin(block, getEditMode) {
    return {
        block,
        renderedHtml: '',
        renderError: null,
        renderLoading: false,
        _lastMode: null,
        _lastContentKey: null,
        _lastStateKey: null,

        get editMode() {
            return getEditMode();
        },

        async loadRender() {
            const mode = this.editMode ? 'edit' : 'view';
            const contentKey = JSON.stringify(this.block.content);
            const stateKey = JSON.stringify(this.block.state);

            // Skip if nothing changed
            if (mode === this._lastMode && contentKey === this._lastContentKey && stateKey === this._lastStateKey) {
                return;
            }
            this._lastMode = mode;
            this._lastContentKey = contentKey;
            this._lastStateKey = stateKey;

            const pluginName = this.block.type.split(':')[1];
            this.renderLoading = true;
            this.renderError = null;

            try {
                const res = await fetch(
                    `/v1/plugins/${encodeURIComponent(pluginName)}/block/render?blockId=${this.block.id}&mode=${mode}`
                );
                if (!res.ok) {
                    throw new Error(await res.text());
                }
                this.renderedHtml = await res.text();
            } catch (err) {
                this.renderError = err.message;
            } finally {
                this.renderLoading = false;
            }
        }
    };
}
```

**Step 2: Add to index.js**

Add to `src/components/blocks/index.js`:

```js
export { blockPlugin } from './blockPlugin.js';
```

**Step 3: Add to main.js**

In `src/main.js`, add `blockPlugin` to the import from blocks/index.js (line 39-40):

```js
import { blockText, blockHeading, blockDivider, blockTodos, blockGallery, blockReferences, blockTable, blockCalendar, eventModal, blockPlugin } from './components/blocks/index.js';
```

Add registration (after line 102):

```js
Alpine.data('blockPlugin', blockPlugin);
```

**Step 4: Build JS**

Run: `cd /Users/egecan/Code/mahresources && npm run build-js`
Expected: Success

**Step 5: Commit**

```bash
git add src/components/blocks/blockPlugin.js src/components/blocks/index.js src/main.js
git commit -m "feat(frontend): add blockPlugin Alpine component for plugin block rendering"
```

---

### Task 10: Add JS Bridge (window.mahBlock)

**Files:**
- Modify: `src/components/blockEditor.js` (add bridge setup in init)

**Step 1: Add bridge initialization**

In `src/components/blockEditor.js`, in the `init()` method (around line 57), add after the existing code:

```js
    async init() {
      // Existing code...
      if (!this._blockTypesLoaded) {
        await this.loadBlockTypes();
      }
      if (this.blocks.length === 0 && this.noteId) {
        await this.loadBlocks();
      }

      // Set up JS bridge for plugin blocks
      const self = this;
      window.mahBlock = {
        saveContent(blockId, content) {
          return self.updateBlockContent(blockId, content);
        },
        updateState(blockId, state) {
          return self.updateBlockState(blockId, state);
        },
        getBlock(blockId) {
          return self.blocks.find(b => b.id === blockId) || null;
        }
      };
    },
```

**Step 2: Build JS**

Run: `cd /Users/egecan/Code/mahresources && npm run build-js`
Expected: Success

**Step 3: Commit**

```bash
git add src/components/blockEditor.js
git commit -m "feat(frontend): add window.mahBlock JS bridge for plugin block interactivity"
```

---

### Task 11: Update Block Editor Template for Plugin Blocks

**Files:**
- Modify: `templates/partials/blockEditor.tpl` (add plugin block catch-all before closing `</div>` of block content, and update type picker)

**Step 1: Add plugin block rendering**

In `templates/partials/blockEditor.tpl`, after the last built-in block type template (the calendar block's closing `</template>`, around line 816) but before the closing `</div>` of the block-content div (line 817), add:

```html
                    {# Plugin block (any type starting with "plugin:") #}
                    <template x-if="block.type.startsWith('plugin:')">
                        <div x-data="blockPlugin(block, () => editMode)"
                             x-effect="loadRender()">
                            <template x-if="renderLoading && !renderedHtml">
                                <div class="text-gray-400 text-sm py-4 text-center">Loading plugin block...</div>
                            </template>
                            <template x-if="renderError">
                                <div class="p-3 bg-red-50 border border-red-200 rounded text-red-700 text-sm" x-text="renderError"></div>
                            </template>
                            <template x-if="renderedHtml">
                                <div x-html="renderedHtml" class="plugin-block-content"></div>
                            </template>
                        </div>
                    </template>
```

**Step 2: Update the block type picker to handle plugin metadata**

In the `loadBlockTypes()` method of `blockEditor.js` (already handled in Task 9 - the API returns label/icon directly). The existing `_formatLabel` and `_getIconForType` serve as fallbacks. Update the mapping in `loadBlockTypes()`:

In `src/components/blockEditor.js`, update the `loadBlockTypes` method (around line 73):

```js
          this.blockTypes = types.map(bt => ({
            type: bt.type,
            label: bt.label || this._formatLabel(bt.type),
            icon: bt.icon || this._getIconForType(bt.type),
            defaultContent: bt.defaultContent,
            plugin: bt.plugin || false,
            pluginName: bt.pluginName || null,
            filters: bt.filters || null
          }));
```

**Step 3: Add "plugin unavailable" fallback**

The existing `_getIconForType` already returns '📦' for unknown types. Add a note about unavailable plugins. In the template, add after the plugin block template:

```html
                    {# Plugin block unavailable fallback #}
                    <template x-if="block.type.startsWith('plugin:') && !blockTypes.find(bt => bt.type === block.type)">
                        <div class="p-4 bg-gray-50 border border-gray-200 rounded text-gray-500 text-sm">
                            This block requires the "<span x-text="block.type.split(':')[1]"></span>" plugin which is not currently enabled.
                        </div>
                    </template>
```

**Note:** The `x-if` on the plugin template already handles enabled plugins. The fallback would only show when the type isn't in `blockTypes` but exists in the DB. However, Alpine evaluates `x-if` templates independently, so both may render. A cleaner approach is to make the plugin template check that the type exists in blockTypes:

```html
                    <template x-if="block.type.startsWith('plugin:') && blockTypes.find(bt => bt.type === block.type)">
                        ...plugin rendering...
                    </template>
                    <template x-if="block.type.startsWith('plugin:') && !blockTypes.find(bt => bt.type === block.type)">
                        ...unavailable fallback...
                    </template>
```

**Step 4: Build everything**

Run: `cd /Users/egecan/Code/mahresources && npm run build`
Expected: Success

**Step 5: Commit**

```bash
git add templates/partials/blockEditor.tpl src/components/blockEditor.js
git commit -m "feat(frontend): add plugin block rendering and fallback in block editor template"
```

---

### Task 12: Add GetPluginSettings Method (if missing)

**Files:**
- Check: `plugin_system/manager.go` for existing `GetPluginSettings`

**Step 1: Verify if method exists**

Search for `GetPluginSettings` in `plugin_system/manager.go`. If it exists, skip this task. If not, add:

```go
// GetPluginSettings returns the current settings for a plugin, or nil if none.
func (pm *PluginManager) GetPluginSettings(name string) map[string]any {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.pluginSettings[name]
}
```

**Step 2: Build and verify**

Run: `cd /Users/egecan/Code/mahresources && go build --tags 'json1 fts5'`
Expected: Success

**Step 3: Commit (if changes made)**

```bash
git add plugin_system/manager.go
git commit -m "feat(plugins): add GetPluginSettings accessor method"
```

---

### Task 13: Add PluginManager and GetNote to Render Handler Interface

**Files:**
- Check: `server/interfaces/` for which interface provides `PluginManager()` and `GetNote()`
- Modify: interface definitions if needed, or use existing combined interface / app context

**Step 1: Examine the existing interface pattern**

The render handler needs:
- `GetBlock(id uint) (*models.NoteBlock, error)` — from `BlockReader`
- `GetNote(id uint) (*models.Note, error)` — from note reader
- `PluginManager() *plugin_system.PluginManager` — from some context interface

Check how `GetPluginActionHandler` or `GetCalendarBlockEventsHandler` receive their context. They likely accept the full `appContext` or a combined interface. Match whichever pattern is used.

If the handler directly accepts `appContext` (which implements all interfaces), then the type parameter in the handler function signature should be whatever interface type `appContext` satisfies that includes these three methods. Create a small combined interface if needed:

```go
// In server/interfaces/block_interfaces.go
type PluginBlockRenderer interface {
    BlockReader
    GetNote(id uint) (*models.Note, error)
    PluginManager() *plugin_system.PluginManager
}
```

Verify `appContext` satisfies this and update the handler signature.

**Step 2: Build and verify**

Run: `cd /Users/egecan/Code/mahresources && go build --tags 'json1 fts5'`
Expected: Success

**Step 3: Commit**

```bash
git add server/interfaces/block_interfaces.go server/api_handlers/plugin_api_handlers.go
git commit -m "feat(interfaces): add PluginBlockRenderer interface for block render handler"
```

---

### Task 14: Create Example Plugin

**Files:**
- Create: `plugins/example-blocks/plugin.lua`

**Step 1: Write example plugin**

Create `plugins/example-blocks/plugin.lua`:

```lua
plugin = {
    name = "example-blocks",
    version = "1.0",
    description = "Example plugin demonstrating custom block types"
}

plugin.settings = {}

function init()
    mah.block_type({
        type = "counter",
        label = "Counter",
        icon = "🔢",
        description = "A simple click counter block",

        content_schema = {
            type = "object",
            properties = {
                label = { type = "string" }
            },
            required = { "label" }
        },

        state_schema = {
            type = "object",
            properties = {
                count = { type = "number" }
            }
        },

        default_content = { label = "My Counter" },
        default_state = { count = 0 },

        render_view = function(ctx)
            local count = ctx.block.state.count or 0
            local label = ctx.block.content.label or "Counter"
            local blockId = ctx.block.id

            return string.format([[
                <div style="text-align:center; padding:20px;">
                    <h3 style="margin:0 0 10px 0;">%s</h3>
                    <div style="font-size:2em; font-weight:bold; margin:10px 0;">%d</div>
                    <button onclick="mahBlock.updateState(%d, {count: %d})"
                            style="padding:8px 16px; background:#3b82f6; color:white; border:none; border-radius:4px; cursor:pointer;">
                        +1
                    </button>
                </div>
            ]], label, count, blockId, count + 1)
        end,

        render_edit = function(ctx)
            local label = ctx.block.content.label or "Counter"
            local blockId = ctx.block.id

            return string.format([[
                <div style="padding:10px;">
                    <label style="display:block; margin-bottom:4px; font-weight:500;">Counter Label</label>
                    <input type="text" value="%s"
                           onchange="mahBlock.saveContent(%d, {label: this.value})"
                           style="width:100%%; padding:8px; border:1px solid #d1d5db; border-radius:4px;" />
                </div>
            ]], label, blockId)
        end
    })
end
```

**Step 2: Commit**

```bash
git add plugins/example-blocks/plugin.lua
git commit -m "feat(plugins): add example-blocks plugin demonstrating custom block types"
```

---

### Task 15: Integration Testing

**Files:**
- Build and manually test the full flow

**Step 1: Build the full application**

Run: `cd /Users/egecan/Code/mahresources && npm run build`
Expected: Success

**Step 2: Run all Go tests**

Run: `cd /Users/egecan/Code/mahresources && go test ./... -count=1 --tags 'json1 fts5'`
Expected: All PASS

**Step 3: Run E2E tests**

Run: `cd /Users/egecan/Code/mahresources/e2e && npm run test:with-server`
Expected: All PASS (existing tests should not be affected)

**Step 4: Manual smoke test**

1. Start server: `./mahresources -ephemeral -bind-address=:8181`
2. Go to plugin management, enable "example-blocks"
3. Create a note, enter block edit mode
4. Verify "Counter" appears in the add block picker with 🔢 icon
5. Add a counter block
6. Verify view mode shows the counter at 0
7. Click "+1" button — counter should increment
8. Switch to edit mode — label editor should appear
9. Change label — view should update

**Step 5: Final commit (if any fixes needed)**

```bash
git add -A
git commit -m "fix: integration fixes for plugin block types"
```

---

### Task 16: Add OpenAPI Documentation for Render Endpoint

**Files:**
- Modify: `server/routes_openapi.go` (add render route registration)

**Step 1: Add route to OpenAPI registry**

In `server/routes_openapi.go`, in the plugin routes section, add:

```go
r.Register(openapi.RouteInfo{
    Method:               http.MethodGet,
    Path:                 "/v1/plugins/{pluginName}/block/render",
    OperationID:          "renderPluginBlock",
    Summary:              "Render a plugin block type's HTML",
    Tags:                 []string{"plugins", "blocks"},
    IDQueryParam:         "blockId",
    IDRequired:           true,
    ExtraQueryParams: []openapi.QueryParam{
        {Name: "mode", Type: "string", Required: true, Description: "Render mode: 'view' or 'edit'"},
    },
    ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeHTML},
})
```

**Step 2: Regenerate OpenAPI spec**

Run: `cd /Users/egecan/Code/mahresources && go run ./cmd/openapi-gen`
Expected: Success

**Step 3: Commit**

```bash
git add server/routes_openapi.go openapi.yaml
git commit -m "docs: add OpenAPI spec for plugin block render endpoint"
```

---

Plan complete and saved to `docs/plans/2026-03-07-plugin-block-types-impl.md`. Two execution options:

**1. Subagent-Driven (this session)** — I dispatch a fresh subagent per task, review between tasks, fast iteration

**2. Parallel Session (separate)** — Open new session with executing-plans, batch execution with checkpoints

Which approach?