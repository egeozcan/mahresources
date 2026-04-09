# MRQL Shortcodes Design

**Date:** 2026-04-09

## Summary

Add two new built-in shortcodes — `[mrql]` and `[property]` — that enable embedding live MRQL query results and entity field values directly in note descriptions, group descriptions, and other shortcode-processed content. Custom rendering templates per category/type allow full control over how results appear.

## Integration Points

Shortcodes are already processed wherever `{% process_shortcodes %}` is used in templates. The new `[mrql]` and `[property]` shortcodes work anywhere shortcodes work today. The key integration points:

- **Note descriptions** — the `note_display.html` template already processes shortcodes on `note.Description`.
- **Group descriptions** — the `group_display.html` template already processes shortcodes on `group.Description`.
- **Resource descriptions** — the `resource_display.html` template already processes shortcodes on `resource.Description`.
- **MRQL results page** — new integration: result entities rendered with custom templates when their category defines `CustomMRQLResult`.

The template filter (`shortcode_tag.go`) must be updated to pass the new `QueryExecutor` callback and populate the `Entity` field on `MetaShortcodeContext`. This is the single wiring point — all existing `{% process_shortcodes %}` call sites automatically gain `[mrql]` and `[property]` support.

## New Built-in Shortcodes

### `[property]` Shortcode

Accesses entity model fields from the current entity context.

```
[property path="Name"]
[property path="CreatedAt"]
[property path="Tags"]
```

- Uses the existing `MetaShortcodeContext`, extended with a full `Entity` field.
- **HTML-escaped by default.** All output is passed through `html.EscapeString()` to prevent markup injection. An optional `raw="true"` attribute opts into unescaped output for cases where the field intentionally contains HTML.
- Outputs: strings as escaped text, numbers/bools as string form, slices as comma-separated (each element escaped), nested objects as JSON.

```
[property path="Name"]                  <!-- escaped -->
[property path="Description" raw="true"] <!-- unescaped, opt-in -->
```

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

**Format resolution:**

Format applies at the **result set level**, not per item:

1. If `format` attribute is explicitly set, use that format for the entire result set.
2. If not set, use the default rendering (same as MRQL page results).

**Exception — custom templates:** When using the default rendering or `format="custom"`, each result item checks its own category/type for a `CustomMRQLResult` template. Items with a custom template render using it; items without one fall back to the default row rendering. This is the only case where mixed rendering occurs within a single result set.

Set-level formats like `table` render the entire result set as a unified table — individual row customization does not apply.

## Architecture

### Shortcode Processor Changes

**New callback type** (follows the existing `PluginRenderer` pattern):

```go
type QueryExecutor func(ctx context.Context, query string, savedName string, limit int) (*QueryResult, error)
```

The callback receives `context.Context` for request-scoped timeout and cancellation — MRQL execution already uses `MRQLQueryTimeout` and the executor must propagate that.

**New result types:**

```go
type QueryResult struct {
    EntityType string
    Mode       string              // "flat", "aggregated", or "bucketed"
    Items      []QueryResultItem   // flat mode
    Rows       []map[string]any    // aggregated mode (GROUP BY with aggregates)
    Groups     []QueryResultGroup  // bucketed mode (GROUP BY without aggregates)
}

type QueryResultItem struct {
    EntityType       string
    EntityID         uint
    Entity           any
    Meta             json.RawMessage
    MetaSchema       string
    CustomMRQLResult string
}

type QueryResultGroup struct {
    Key   map[string]any
    Items []QueryResultItem
}
```

**Updated `Process()` signature:**

```go
func Process(ctx context.Context, input string, mctx MetaShortcodeContext, renderer PluginRenderer, executor QueryExecutor) string
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
2. Call `QueryExecutor` with context, query/saved name, and limit.
3. Branch on result mode:
   - **Flat:** for each result item, build a child `MetaShortcodeContext`. Apply format resolution (explicit format > custom template per item > default).
   - **Aggregated:** render rows as a table (columns from aggregate keys/values). Custom templates do not apply — aggregated results are summary data, not entities.
   - **Bucketed:** render each group header (key), then render items within each group using the same flat-mode logic.
4. If custom: process the `CustomMRQLResult` template through `Process()` recursively with the item's context, same renderer/executor.
5. If set-level format (table/list/compact): render the entire result set uniformly.
6. Wrap all rendered output in a container element.

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
