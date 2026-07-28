# Shortcode System for Custom Render Locations

**Date:** 2026-04-05
**Status:** Draft

## Overview

Add shortcode parsing to category custom render locations (`CustomHeader`, `CustomSidebar`, `CustomSummary`, `CustomAvatar`). Shortcodes are inline directives like `[meta path="cooking.time" editable=true]` that expand into rendered widgets. Built-in shortcodes handle metadata display and inline editing. Plugins can register custom shortcodes via `mah.shortcode()`.

## Syntax

Non-nesting, self-closing: `[shortcodeName attr1="value1" attr2="value2"]`

- Only recognized shortcode names are matched; unrecognized `[...]` patterns are left as-is.
- Regular HTML around shortcodes is preserved untouched.
- Attribute values may be quoted (`"value"`) or unquoted for simple values (`true`, `false`, numbers).

## Architecture: Server-Side Parse, Client-Side Hydrate

- **Go** parses shortcode syntax at template render time via a pongo2 filter.
- **Built-in `meta` shortcode**: Go extracts entity context (type, ID, schema slice, current value) and emits a `<meta-shortcode>` custom element with data attributes. Client-side JS hydrates it using existing schema-editor rendering logic.
- **Plugin shortcodes**: Go calls the plugin's Lua render function server-side and inlines the returned HTML directly. No client-side hydration needed.

## Built-in Shortcode: `[meta]`

### Attributes

| Attribute | Type | Default | Description |
|-----------|------|---------|-------------|
| `path` | string | required | Dot-notation meta path (e.g., `cooking.time`, `a.b.c.d`) |
| `editable` | boolean | `false` | Show pencil edit button |
| `hide-empty` | boolean | `false` | Hide entirely when value is absent |

### Server-Side Expansion

`[meta path="cooking.time" editable=true]` expands to:

```html
<meta-shortcode
    data-path="cooking.time"
    data-editable="true"
    data-hide-empty="false"
    data-entity-type="group"
    data-entity-id="123"
    data-schema='{"type":"integer","title":"Cooking Time (min)"}'
    data-value="30">
</meta-shortcode>
```

The Go layer:
1. Resolves entity type and ID from the template render context.
2. Extracts the schema slice for the given path from the category's MetaSchema. If the path is not in the schema or no schema exists, `data-schema` is empty.
3. Reads the current value from the entity's Meta at the given path. If absent, `data-value` is empty.

## `<meta-shortcode>` Web Component

A Lit-based custom element rendered in light DOM (for Tailwind style inheritance).

### Data Attributes

| Attribute | Description |
|-----------|-------------|
| `data-path` | Dot-notation meta path |
| `data-editable` | `"true"` / `"false"` |
| `data-hide-empty` | `"true"` / `"false"` |
| `data-entity-type` | `"group"`, `"resource"`, or `"note"` |
| `data-entity-id` | Entity ID |
| `data-schema` | JSON schema slice for this path (empty if none) |
| `data-value` | Current value as JSON (empty if not set) |

### Display Mode (Default)

- **Value exists + schema exists**: Reuse `schema-display-mode` rendering logic — type-aware formatting, shape detection, x-display pipeline — for the single field.
- **Value exists + no schema**: Render as formatted JSON value.
- **Value empty + `hide-empty=false`**: Render label (schema `title` or last path segment in title case) with a dash/empty indicator.
- **Value empty + `hide-empty=true`**: Render nothing.

### Edit Mode (when `editable=true`)

1. A pencil button appears beside the displayed value (same SVG/style as existing `<inline-edit>`).
2. Clicking the pencil swaps display for a form:
   - **With schema**: Renders `schema-form-mode` scoped to the path's schema slice.
   - **Without schema**: Renders a standard key-value field editor for the value at that path.
3. Save and Cancel buttons below the form.
4. **Save** constructs the full nested object path (e.g., path `a.b.c.d` + value `30` → `{"a":{"b":{"c":{"d":30}}}}`) and POSTs to `editMeta`.
5. On success: green flash, swap back to display mode with updated value.
6. On failure: red flash, keep form open.

### Deep Path Creation

When editing a path like `a.b.c.d` and the entity has no `a` at all:
- The save operation constructs the full nested chain.
- If a schema exists, intermediate objects are created according to schema types.
- If no schema, plain nested objects are created.

## Meta Path Edit API Endpoint

### `POST /v1/{entityType}/editMeta`

Three routes sharing one generic handler (same `EntityWriter[T]` pattern as `editName`/`editDescription`):
- `POST /v1/group/editMeta`
- `POST /v1/resource/editMeta`
- `POST /v1/note/editMeta`

### Parameters

| Parameter | Source | Description |
|-----------|--------|-------------|
| `id` | query param | Entity ID |
| `path` | form field | Dot-notation path |
| `value` | form field | JSON-encoded value |

### Behavior

1. Validate entity exists.
2. If the entity's category has a MetaSchema, validate the value against the schema slice for the path.
3. Build the nested object from the path: `cooking.time` + `30` → `{"cooking":{"time":30}}`.
4. Deep-merge into the entity's existing Meta:
   - Walk path segments, creating intermediate objects as needed.
   - If intermediate segments exist as objects, merge into them.
   - If an intermediate segment exists but is not an object, overwrite it.
   - **Postgres**: construct nested JSON object, `||` merge.
   - **SQLite**: construct nested JSON object in Go, `json_patch`.
5. Return `{"ok": true, "id": <id>, "meta": <full updated meta>}`.

## Plugin Shortcode API: `mah.shortcode()`

### Lua Registration

```lua
mah.shortcode({
    name = "rating",              -- required, ^[a-z][a-z0-9-]{0,49}$
    label = "Star Rating",        -- required
    render = function(ctx)        -- required, returns HTML string
        local stars = tonumber(ctx.attrs.max) or 5
        return '<div>...</div>'
    end
})
```

### Render Context

| Field | Description |
|-------|-------------|
| `ctx.entity_type` | `"group"`, `"resource"`, or `"note"` |
| `ctx.entity_id` | Entity ID |
| `ctx.value` | Entity's full Meta as Lua table |
| `ctx.attrs` | Shortcode attributes as key-value table |
| `ctx.settings` | Plugin settings |

### Type Naming

- Plugin registers `name = "rating"`.
- System expands to `plugin:my-plugin:rating`.
- Usage in custom render field: `[plugin:my-plugin:rating max="5"]`.

### Execution

- Server-side at template render time (same as built-in shortcodes).
- 5-second timeout per render (matching display renderer timeout).
- Returned HTML is inlined directly — no client-side hydration.

### Storage

`pm.shortcodes[pluginName]` map, parallel to `pm.displayTypes`. Cleaned up on plugin disable.

## Template Integration: Pongo2 Filter

### Filter

`processShortcodes` — takes the entity as argument. Parses shortcodes in the string and expands them with the entity's context.

### Usage

Replace existing raw output:

```html
<!-- Before -->
{% autoescape off %}
    {{ entity.Category.CustomSummary }}
{% endautoescape %}

<!-- After -->
{% autoescape off %}
    {{ entity.Category.CustomSummary|processShortcodes:entity }}
{% endautoescape %}
```

### Affected Templates (~14 locations)

**Group detail page — `displayGroup.tpl`:**
- `CustomHeader` (line 7, via `group.Category`)
- `CustomSidebar` (line 60, via `group.Category`)

**Resource detail page — `displayResource.tpl`:**
- `CustomHeader` (line 7, via `resource.ResourceCategory`)
- `CustomSidebar` (line 236, via `resource.ResourceCategory`)

**Note detail pages — `displayNote.tpl` / `displayNoteText.tpl`:**
- `CustomHeader` (displayNote.tpl line 7, via `note.NoteType`)
- `CustomSidebar` (displayNote.tpl line 48, displayNoteText.tpl line 25, via `note.NoteType`)

**List/card partials:**
- `partials/group.tpl` — `CustomSummary` (line 60, via `entity.Category`)
- `partials/resource.tpl` — `CustomSummary` (line 46), `CustomAvatar` (line 37, via `entity.ResourceCategory`)
- `partials/note.tpl` — `CustomSummary` (line 17), `CustomAvatar` (line 7, via `entity.NoteType`)

Each invocation receives the specific entity, so shortcodes resolve with per-entity Meta and ID even in list views.

### Edge Case: Lightbox

`partials/lightbox.tpl` (line 763) renders `CustomSidebar` client-side via Alpine's `x-html` from API-fetched resource data. The pongo2 filter doesn't apply here.

**Solution:** Pre-process shortcodes in the Custom* fields when the API returns entity details (the handler already has entity context). The expanded `<meta-shortcode>` elements in the HTML string are recognized as custom elements when `x-html` inserts them into the DOM, so hydration works automatically.

## What Does NOT Change

- Existing schema-editor components (reused, not modified).
- Existing `<inline-edit>` web component.
- Category / ResourceCategory / NoteType models (no new fields).
- Existing bulk meta edit endpoints (`addMeta`).

## New Files

| File | Purpose |
|------|---------|
| `shortcodes/parser.go` | Shortcode syntax parser |
| `shortcodes/meta_shortcode.go` | Built-in `meta` shortcode handler |
| `shortcodes/processor.go` | Top-level `ProcessShortcodes` function, pongo2 filter registration |
| `plugin_system/shortcodes.go` | `mah.shortcode()` registration, types, storage |
| `src/webcomponents/meta-shortcode.ts` | `<meta-shortcode>` Lit web component |
| `server/api_handlers/meta_edit_handler.go` | `editMeta` generic handler |
