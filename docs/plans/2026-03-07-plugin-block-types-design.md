# Plugin-Defined Note Block Types

**Date:** 2026-03-07
**Status:** Approved

## Overview

Allow Lua plugins to define custom note block types that integrate with the existing block editor. Plugin blocks are first-class citizens in the block type registry, using full HTML rendering from Lua, JSON Schema-based validation, content + state separation, and a JS bridge for interactivity.

## Design Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Rendering approach | Full HTML from Lua | Maximum flexibility, consistent with `mah.page` |
| Content/State | Full separation (like built-in blocks) | Consistent contract across all block types |
| Render context | Rich (block + note + settings) | Avoids extra round-trips for common data |
| Availability | Filterable by note type/category | Consistent with plugin action filters |
| Interactivity | Static HTML + JS bridge | Cleaner and more secure than eval'd JS |
| Validation | JSON Schema at registration time | Fast (no Lua call per save), self-documenting |

## Registration API

Plugins register block types during `init()` via `mah.block_type()`:

```lua
mah.block_type({
    type = "kanban",                    -- required, unique per plugin
    label = "Kanban Board",             -- required, display name
    icon = "icon-name-or-emoji",        -- optional
    description = "A kanban task board", -- optional

    -- JSON Schema for validation (optional, nil = accept any JSON)
    content_schema = {
        type = "object",
        properties = {
            columns = { type = "array" }
        },
        required = { "columns" }
    },
    state_schema = {
        type = "object",
        properties = {
            collapsed = { type = "array" }
        }
    },

    -- Defaults for new blocks
    default_content = { columns = {} },
    default_state = { collapsed = {} },

    -- Optional filters: restrict to specific note types/categories
    filters = {
        note_type_ids = { 3, 5 },
        category_ids = { 1 }
    },

    -- Render functions (required)
    render_view = function(ctx)
        -- ctx.block = { id, content, state, position }
        -- ctx.note = { id, name, note_type_id }
        -- ctx.settings = plugin settings table
        return '<div class="kanban">...</div>'
    end,
    render_edit = function(ctx)
        return '<div class="kanban-editor">...</div>'
    end
})
```

### Type Naming

Types are namespaced automatically: `plugin:<pluginName>:<type>` (e.g., `plugin:my-kanban:kanban`). This prevents collisions with built-in types (which never contain `:`) and between plugins.

Type name validation: lowercase alphanumeric + hyphens only, max 50 chars.

## Go Data Model

### `PluginBlockType` struct

```go
// In plugin_system/block_types.go
type PluginBlockType struct {
    PluginName     string
    TypeName       string              // full namespaced: plugin:<pluginName>:<type>
    Label          string
    Icon           string
    Description    string
    ContentSchema  *jsonschema.Schema  // compiled JSON Schema, nil = accept all
    StateSchema    *jsonschema.Schema  // compiled JSON Schema, nil = accept all
    DefContent     json.RawMessage
    DefState       json.RawMessage
    Filters        BlockTypeFilter
    RenderView     *lua.LFunction
    RenderEdit     *lua.LFunction
}

type BlockTypeFilter struct {
    NoteTypeIDs []uint `json:"note_type_ids,omitempty"`
    CategoryIDs []uint `json:"category_ids,omitempty"`
}
```

Implements `block_types.BlockType` interface:
- `Type()` returns the full namespaced name
- `ValidateContent()` validates against compiled ContentSchema (nil = pass)
- `ValidateState()` validates against compiled StateSchema (nil = pass)
- `DefaultContent()` returns DefContent
- `DefaultState()` returns DefState

### Registry Changes

Add `UnregisterBlockType` to `models/block_types/registry.go`:

```go
func UnregisterBlockType(typeName string) {
    mu.Lock()
    defer mu.Unlock()
    delete(registry, typeName)
}
```

Plugin manager tracks registered types per plugin (`pm.blockTypes map[string][]string`) for clean teardown on disable.

### JSON Schema Validation

Library: `github.com/santhosh-tekuri/jsonschema/v5`

- Schemas compiled once at registration time via `jsonschema.CompileString()`
- Invalid schemas cause registration failure (error logged)
- Validation at create/update time calls `compiledSchema.Validate()` — fast, no Lua overhead
- No schema (nil) = accept any JSON

## API Changes

### Extended `GET /v1/note/block/types`

Response struct extended with optional metadata fields:

```go
type BlockTypeInfo struct {
    Type           string           `json:"type"`
    DefaultContent json.RawMessage  `json:"defaultContent"`
    DefaultState   json.RawMessage  `json:"defaultState"`
    // New fields (omitted for built-in types)
    Label       string           `json:"label,omitempty"`
    Icon        string           `json:"icon,omitempty"`
    Description string           `json:"description,omitempty"`
    Plugin      bool             `json:"plugin,omitempty"`
    PluginName  string           `json:"pluginName,omitempty"`
    Filters     *BlockTypeFilter `json:"filters,omitempty"`
}
```

Built-in types continue returning only `type`, `defaultContent`, `defaultState` — no breaking change.

### New Render Endpoint

```
GET /v1/plugins/{pluginName}/block/render?blockId={id}&mode=view|edit
```

- Looks up block from DB (content, state, note ID)
- Looks up parent note for context (id, name, note_type_id)
- Calls plugin's `render_view` or `render_edit` Lua function
- Returns `Content-Type: text/html`
- 5-second timeout (same as injections)
- Returns 404 if plugin disabled or block type mismatch

### Render Context

The Lua render functions receive a context table:

```lua
ctx = {
    block = {
        id = 42,
        content = { columns = {...} },
        state = { collapsed = {...} },
        position = "n"
    },
    note = {
        id = 10,
        name = "My Note",
        note_type_id = 3
    },
    settings = {
        api_key = "...",
        -- plugin settings from PluginState
    }
}
```

### Filter Enforcement

On block creation: if the `PluginBlockType` has filters, the handler checks the parent note's type/category. Mismatch returns 400.

## Frontend Integration

### Block Type Picker

`loadBlockTypes()` in `blockEditor.js` uses the extended API response. Plugin types arrive with `label`, `icon`, `plugin: true`. The existing `_formatLabel`/`_getIconForType` serve as fallbacks for built-in types; plugin types use their provided metadata.

When the note has a `noteTypeId`, the picker filters out plugin block types whose `filters.note_type_ids` doesn't include it.

### Plugin Block Rendering

In `blockEditor.tpl`, after all built-in `x-if` checks, a catch-all handles plugin blocks:

```html
<template x-if="block.type.startsWith('plugin:')">
    <div x-data="blockPlugin(block, editMode)"
         x-effect="loadRender(block, editMode)">
        <div x-html="renderedHtml" class="plugin-block-content"></div>
        <div x-show="renderError" class="text-red-500 text-sm" x-text="renderError"></div>
        <div x-show="renderLoading" class="text-gray-400 text-sm">Loading...</div>
    </div>
</template>
```

### `blockPlugin.js` Component

```js
export function blockPlugin(block, editMode) {
    return {
        renderedHtml: '',
        renderError: null,
        renderLoading: false,
        _lastMode: null,
        _lastContent: null,

        async loadRender(block, editMode) {
            const mode = editMode ? 'edit' : 'view';
            const contentKey = JSON.stringify(block.content);
            if (mode === this._lastMode && contentKey === this._lastContent) return;
            this._lastMode = mode;
            this._lastContent = contentKey;

            const pluginName = block.type.split(':')[1];
            this.renderLoading = true;
            this.renderError = null;
            try {
                const res = await fetch(
                    `/v1/plugins/${pluginName}/block/render?blockId=${block.id}&mode=${mode}`
                );
                if (!res.ok) throw new Error(await res.text());
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

### JS Bridge

Plugin HTML interacts with the block editor via global functions:

```js
window.mahBlock = {
    saveContent(blockId, content) { /* calls updateBlockContent on Alpine component */ },
    updateState(blockId, state) { /* calls updateBlockState on Alpine component */ },
    getBlock(blockId) { /* returns block data from Alpine state */ }
};
```

Plugin HTML usage:
```html
<button onclick="mahBlock.saveContent(42, {columns: updatedData})">Save</button>
```

After save/state updates, the block editor re-renders (content/state change triggers `x-effect` → `loadRender`).

### Plugin Unavailable Fallback

If a block type starts with `plugin:` but the plugin isn't in the loaded `blockTypes`, the template shows: "This block requires the [pluginName] plugin which is not currently enabled."

## Lifecycle

1. **Plugin enabled** → `mah.block_type()` calls during `init()` → `PluginBlockType` created → `block_types.RegisterBlockType()` called → type appears in registry
2. **Plugin disabled** → `block_types.UnregisterBlockType()` called for each type → types removed from registry → existing DB blocks preserved → render with fallback UI
3. **Plugin re-enabled** → types re-registered → existing blocks render again

## Error Handling

| Scenario | Behavior |
|----------|----------|
| Create block with disabled plugin type | 400 "unknown block type" (existing behavior) |
| Update block with disabled plugin type | 400 (GetBlockType returns nil) |
| Lua render throws error | Render endpoint returns 500, `blockPlugin.js` shows error |
| Lua render times out (5s) | Same 500 path |
| Plugin returns empty HTML | Empty div rendered (valid) |
| Invalid JSON Schema at registration | Registration fails, error logged |
| Content fails schema validation | Block create/update returns 400 |

## Dependencies

- New Go dependency: `github.com/santhosh-tekuri/jsonschema/v5` (pure Go, no CGo)
