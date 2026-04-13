# Block MRQL Shortcode Design

**Date:** 2026-04-13

## Summary

Allow `[mrql]` shortcodes to be used as blocks where the inner content becomes the per-item template for query results.

## Syntax

```
[mrql query="FROM resources WHERE tag = 'recipe'" limit="5"]
  <div class="recipe-card">
    <h3>[property path="Name"]</h3>
    <p>Cooking time: [meta path="cooking.time"] minutes</p>
  </div>
[/mrql]
```

Self-closing `[mrql query="..."]` continues to work unchanged.

## Behavior

### Template precedence

Block template always wins. When `sc.IsBlock && sc.InnerContent != ""`, the inner content overrides any `CustomMRQLResult` set on the item's category. The `format` attribute is also ignored when a block template is present.

### Result modes

- **flat**: Block template applied per entity item. Each item gets its own `MetaShortcodeContext`, so `[meta]`, `[property]`, `[conditional]`, nested `[mrql]`, and plugin shortcodes all work.
- **bucketed**: Block template applied per entity item within each bucket. Bucket headers render normally.
- **aggregated**: Block template ignored. Aggregated table renders as before.

### Empty results

No `[else]` support. Empty results render the existing "No results." default.

## Implementation

The change is entirely within `RenderMRQLShortcode` in `shortcodes/mrql_handler.go`.

After the executor returns results, if `sc.IsBlock && sc.InnerContent != ""`:

1. For **flat** results: set every item's `CustomMRQLResult` to `sc.InnerContent`, then call `renderFlatWithCustom` with `forceCustom=true`.
2. For **bucketed** results: set every item's `CustomMRQLResult` to `sc.InnerContent` within each group. Override format to `"custom"` so `renderFlat` routes to the custom path.
3. For **aggregated** results: no change.

### Files changed

- `shortcodes/mrql_handler.go` — add block template logic in `RenderMRQLShortcode`

### Files unchanged

- `shortcodes/parser.go` — `ParseWithBlocks` already handles `[mrql]...[/mrql]`
- `shortcodes/processor.go` — dispatch already passes full `Shortcode` struct with `IsBlock` and `InnerContent`

## Testing

- Unit test: block `[mrql]` with flat results uses inner content as template
- Unit test: block `[mrql]` with bucketed results applies template per item in each bucket
- Unit test: block `[mrql]` with aggregated results ignores inner content
- Unit test: block template overrides `CustomMRQLResult` on items
- Unit test: self-closing `[mrql]` still works unchanged
- Unit test: block `[mrql]` with empty results shows default "No results."
- E2E test: block `[mrql]` renders correctly on a group detail page
