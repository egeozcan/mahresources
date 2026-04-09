# MRQL Shortcodes Design

**Date:** 2026-04-09

## Summary

Add two new built-in shortcodes — `[mrql]` and `[property]` — that enable embedding live MRQL query results and entity field values directly in note descriptions, group descriptions, and other shortcode-processed content. Custom rendering templates per category/type allow full control over how results appear.

## Integration Points

The new `[mrql]` and `[property]` shortcodes work anywhere `{% process_shortcodes %}` is used. Two changes are needed to wire them up:

**1. Shortcode filter wiring** — the template filter (`shortcode_tag.go`) must be updated to pass the new `QueryExecutor` callback and populate the `Entity` field on `MetaShortcodeContext`. All existing `{% process_shortcodes %}` call sites automatically gain `[mrql]` and `[property]` support.

**2. Description partial** — the shared `templates/partials/description.tpl` currently applies `markdown2|render_mentions` but does **not** call `process_shortcodes`. This partial must be updated to add shortcode processing to the filter chain so that shortcodes in note, group, and resource descriptions are expanded. This is a template change, not a Go change.

**MRQL results page** — new integration, handled differently (see MRQL Page Integration section below).

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
| `limit`   | Max items per result set (flat) or per bucket (bucketed). Default: 20 |
| `buckets` | Max groups to render in bucketed GROUP BY mode. Default: 5 |
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
type QueryExecutor func(ctx context.Context, query string, savedName string, limit int, buckets int) (*QueryResult, error)
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

The MRQL page currently renders results entirely client-side: `mrqlEditor.js` fetches JSON from `POST /v1/mrql` and Alpine.js templates render aggregated, bucketed, and flat results in the browser. There is no server-side HTML generation for MRQL results today.

To support `CustomMRQLResult` on the MRQL page, the API response is extended rather than changing the rendering architecture:

- The `/v1/mrql` endpoint gains an optional `render=1` query parameter.
- When `render=1` is set, the API checks each result entity's category/type for `CustomMRQLResult`. If present, it processes the template server-side (shortcodes expanded) and includes the rendered HTML as a `renderedHTML` field on each entity in the JSON response.
- The frontend checks for `renderedHTML` on each result item. If present, it inserts the pre-rendered HTML. If absent, it uses the existing client-side rendering.
- This preserves the current client-side architecture while enabling custom templates without duplicating the shortcode engine in JavaScript.
- The `render=1` parameter is opt-in so existing API consumers are unaffected.

For bucketed mode, custom rendering applies to items within each bucket — bucket headers remain client-side rendered.

## Security and Performance

**Recursion guard:** Custom templates processed through `Process()` could contain nested `[mrql]` shortcodes. Cap recursion depth at 2 levels to prevent infinite loops.

**Query limits:** Default limit of 20 items (flat mode or per-bucket in bucketed mode). Default bucket cap of 5 groups in bucketed mode. These two limits together bound total output: a bucketed query renders at most 5 groups x 20 items = 100 entities by default.

**No new auth concerns:** The app has no auth. MRQL already executes arbitrary queries via the API, so shortcodes don't expand the attack surface.
