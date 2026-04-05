# Display Renderers: Built-in Shape Detection + Plugin API

## Summary

Extend the schema-driven metadata display with smart rendering for well-known object shapes (URL, GeoLocation, DateRange, Dimensions) and a plugin API that lets plugins register custom renderers for their own object types via `x-display` schema annotations.

## Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Plugin renderer declaration | `x-display` schema annotation | Explicit, no server round-trips for detection, schema author controls rendering |
| Built-in type detection | Automatic shape detection with opt-out | Works with existing data; `x-display: "raw"` disables it |
| Plugin render mechanism | Server-side HTML via API (POST) | Matches block-type pattern, gives plugins full Lua context |
| Built-in renderer location | Single file `src/schema-editor/display-renderers.ts` | Flat list of detect/render pairs, easy to extend |

## Built-in Shape Detectors

Four built-in shapes, checked in order. First match wins.

| Shape | Detection | Rendering |
|-------|-----------|-----------|
| URL/Location | has `href` + (`host` or `hostname`) | Clickable link showing href, host as subtitle |
| GeoLocation | has `latitude` + `longitude` (or `lat` + `lng`) | Coordinates text + OpenStreetMap link |
| Date Range | has `start` + `end`, both parseable as dates | Formatted range: "Mar 15, 2024 — Apr 1, 2024" |
| Dimensions | has `width` + `height`, both numbers | "1920 × 1080" |

**Opt-out:** `x-display: "raw"` or `x-display: "none"` skips detection and shows the key-value grid.

**Force built-in:** `x-display: "url"`, `x-display: "geo"`, `x-display: "daterange"`, `x-display: "dimensions"` forces a specific built-in renderer even if shape detection wouldn't match.

## Plugin Renderer API

### Schema Annotation

The schema author adds `x-display` to a property:

```json
{
  "type": "object",
  "x-display": "plugin:fal-ai:image-grid",
  "properties": { ... }
}
```

Naming convention: `plugin:{pluginName}:{type}`. Values without the `plugin:` prefix are reserved for built-in renderers.

### Plugin Registration

In `plugin.lua`:

```lua
mah.display_type({
    type = "image-grid",
    label = "Image Grid",
    render = function(ctx)
        -- ctx.value: the object value from Meta
        -- ctx.schema: the property's JSON schema
        -- ctx.field_path: dot-notation path (e.g., "images")
        -- ctx.field_label: display label (e.g., "Image Gallery")
        -- ctx.settings: plugin settings
        return '<div class="grid grid-cols-3 gap-2">...</div>'
    end
})
```

### Render Endpoint

`POST /v1/plugins/{pluginName}/display/render`

Request body:
```json
{
    "type": "image-grid",
    "value": { ... },
    "schema": { ... },
    "field_path": "images",
    "field_label": "Image Gallery"
}
```

Response: HTML string (plain text content type).

Timeout: 5 seconds (same as block types). On error: fall back to key-value grid with subtle error indicator.

## Renderer Pipeline in display-mode.ts

`_renderValue(field)` checks in order:

1. `x-display` starts with `plugin:` → render loading placeholder, POST to plugin endpoint, insert HTML on success, fall back to key-value grid on error
2. `x-display` is a built-in name (`url`, `geo`, `daterange`, `dimensions`) → use that renderer directly
3. `x-display` is `raw` or `none` → skip to key-value grid
4. Value is an object → run shape detectors in order (URL, Geo, DateRange, Dimensions), first match wins
5. Existing rendering (enums, booleans, numbers, strings, arrays, key-value grid fallback)

## Data Flow: DisplayField Changes

`flattenForDisplay` reads `x-display` from the raw schema property. If `x-display` is present on an object property, that property is NOT recursively flattened — it's emitted as a single `DisplayField` with the full object value, so the renderer receives the complete object. If `x-display` is absent, the existing flattening logic applies.

The `x-display` value is stored on `DisplayField`:

```typescript
interface DisplayField {
  // ... existing fields ...
  xDisplay: string;  // value of schema's x-display, empty string if absent
}
```

## Server-side Changes

### New Files

| File | Purpose |
|------|---------|
| `plugin_system/display_types.go` | `mah.display_type()` registration, type storage, lookup |
| `plugin_system/display_render.go` | Lua render function execution with context and 5s timeout |

### Modified Files

| File | Change |
|------|--------|
| `plugin_system/manager.go` | Register `display_type` in the `mah` Lua module table |
| `server/api_handlers/plugin_api_handlers.go` | Add `POST /v1/plugins/{name}/display/render` route |
| `server/routes.go` or equivalent | Register the new route |

### Render Context (Lua)

```lua
ctx = {
    value = { ... },
    schema = { ... },
    field_path = "url",
    field_label = "URL Details",
    settings = { ... },
}
```

## Frontend Changes

| File | Change |
|------|--------|
| `src/schema-editor/display-renderers.ts` | New file: built-in shape detectors + renderers |
| `src/schema-editor/modes/display-mode.ts` | Add `xDisplay` to `DisplayField`, integrate renderer pipeline |

## Edge Cases

| Scenario | Behavior |
|----------|----------|
| `x-display` references a plugin that isn't enabled | Fall back to key-value grid |
| `x-display` references a type the plugin doesn't have | Fall back to key-value grid |
| Plugin render times out (>5s) | Fall back to key-value grid, log warning |
| Plugin render returns error | Fall back to key-value grid, subtle error indicator |
| Object matches multiple shape detectors | First detector in list wins |
| `x-display: "raw"` on a URL-shaped object | Shows key-value grid, no link rendering |
| `x-display: "url"` on a non-URL object | Forces URL renderer, renders whatever `href` it finds (or empty) |
| Scalar value with `x-display` | Plugin renderer receives the scalar; it's the plugin's job to handle it |
| `x-display` on a nested flattened property | The `x-display` is read from the schema property at the point of flattening; nested objects with `x-display` are NOT flattened — they're passed as a whole object to the renderer |
