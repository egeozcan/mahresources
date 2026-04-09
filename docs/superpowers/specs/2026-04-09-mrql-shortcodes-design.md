# MRQL Shortcodes Design

**Date:** 2026-04-09

## Summary

Add two new built-in shortcodes — `[mrql]` and `[property]` — that enable embedding live MRQL query results and entity field values directly in note descriptions, group descriptions, and other shortcode-processed content. Custom rendering templates per category/type allow full control over how results appear.

## New Built-in Shortcodes

### `[property]` Shortcode

Accesses entity model fields from the current entity context.

```
[property path="Name"]
[property path="CreatedAt"]
[property path="Tags"]
```

- Uses the existing `MetaShortcodeContext`, extended with a full `Entity` field.
- Outputs raw values: strings as-is, numbers/bools as string form, slices as comma-separated, nested objects as JSON.

### `[mrql]` Shortcode

Executes an MRQL query and renders results inline.

```
[mrql query="type = 'resource' AND tags = 'photo'" limit="10"]
[mrql saved="my-saved-query" format="table"]
[mrql query="type = 'note' AND category = 'recipe'" format="custom"]
```

**Attributes:**

| Attribute | Description |
|-----------|-------------|
| `query`   | Inline MRQL query string |
| `saved`   | Name of a saved MRQL query (mutually exclusive with `query`) |
| `limit`   | Max results to render (default: 20) |
| `format`  | Rendering format: `table`, `list`, `compact`, `custom` |

**Format resolution order:**

1. If `format` attribute is explicitly set, use that format.
2. If not set and the result entity's category/type has a `CustomMRQLResult` template, use custom.
3. If not set and no custom template exists, use the default rendering (same as MRQL page results).

Each result item resolves its format independently — a single result set can have mixed rendering when items span multiple categories.

## Architecture

### Shortcode Processor Changes

**New callback type** (follows the existing `PluginRenderer` pattern):

```go
type QueryExecutor func(query string, savedName string, limit int) (*QueryResult, error)
```

**New result types:**

```go
type QueryResult struct {
    EntityType string
    Items      []QueryResultItem
}

type QueryResultItem struct {
    EntityType       string
    EntityID         uint
    Entity           any
    Meta             json.RawMessage
    MetaSchema       string
    CustomMRQLResult string
}
```

**Updated `Process()` signature:**

```go
func Process(input string, ctx MetaShortcodeContext, renderer PluginRenderer, executor QueryExecutor) string
```

The `executor` can be nil — if nil, `[mrql]` shortcodes are left as-is (same pattern as `renderer`).

**Extended `MetaShortcodeContext`:**

```go
type MetaShortcodeContext struct {
    EntityType string
    EntityID   uint
    Meta       json.RawMessage
    MetaSchema string
    Entity     any    // full model struct, for [property] shortcode
}
```

### Rendering Flow for `[mrql]`

1. Parse shortcode attributes.
2. Call `QueryExecutor` with query/saved name + limit.
3. For each result item, build a child `MetaShortcodeContext` from the item.
4. Resolve format per item (explicit format > custom template > default).
5. If custom: process the `CustomMRQLResult` template through `Process()` recursively with the item's context, same renderer/executor.
6. If default/table/list/compact: render with the built-in format.
7. Wrap all rendered items in a container element.

## Database Model Changes

New `CustomMRQLResult` string field on:

- `models.Category` (groups)
- `models.ResourceCategory` (resources)
- `models.NoteType` (notes)

Contains HTML + shortcodes, e.g.:

```html
<div class="recipe-card">
  <h3>[property path="Name"]</h3>
  <p>Cook time: [meta path="cooking.time"]</p>
</div>
```

GORM auto-migrates the new column. Editable via textarea in each category/type editor.

## MRQL Page Integration

The existing MRQL page shares the same rendering logic:

- Check each result entity's category/type for `CustomMRQLResult`.
- If present, render using its custom template (shortcodes processed server-side).
- If absent, use the current default rendering.
- Same mixed-rendering behavior as in `[mrql]` shortcodes.

The render function is shared between the MRQL template handler and the shortcode processor.

## Security and Performance

**Recursion guard:** Custom templates processed through `Process()` could contain nested `[mrql]` shortcodes. Cap recursion depth at 2 levels to prevent infinite loops.

**Query limits:** Default limit of 20 results if none specified. Prevents accidentally rendering thousands of items inline.

**No new auth concerns:** The app has no auth. MRQL already executes arbitrary queries via the API, so shortcodes don't expand the attack surface.
