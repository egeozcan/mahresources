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

Block template always wins. The inner content is trimmed (`strings.TrimSpace`) before evaluation. When `sc.IsBlock` and the trimmed inner content is non-empty, it overrides any `CustomMRQLResult` set on the item's category. The `format` attribute is also ignored when a block template is present.

### Empty and whitespace-only blocks

`[mrql query="..."][/mrql]` and `[mrql query="..."]\n[/mrql]` (whitespace-only) both fall back to normal rendering — same as self-closing. The trim-then-check rule means a block is only treated as a template when it contains actual content.

### Result modes

- **flat**: Block template applied per entity item. Each item gets its own `MetaShortcodeContext`, so `[meta]`, `[property]`, `[conditional]`, nested `[mrql]`, and plugin shortcodes all work.
- **bucketed**: Block template applied per entity item within each bucket. Bucket headers render normally.
- **aggregated**: Block template ignored. Aggregated table renders as before.

### Empty results

No `[else]` support. Empty results render the existing "No results." default.

## Implementation

The change is entirely within `RenderMRQLShortcode` in `shortcodes/mrql_handler.go`.

After the executor returns results, trim `sc.InnerContent` with `strings.TrimSpace`. If `sc.IsBlock` and the trimmed content is non-empty:

1. For **flat** results: set every item's `CustomMRQLResult` to the trimmed inner content, then call `renderFlatWithCustom` with `forceCustom=true`.
2. For **bucketed** results: set every item's `CustomMRQLResult` to the trimmed inner content within each group. Override format to `"custom"` so `renderFlat` routes to the custom path.
3. For **aggregated** results: no change — inner content ignored even if non-empty.

### Files changed

- `shortcodes/mrql_handler.go` — add block template logic in `RenderMRQLShortcode`

### Files unchanged

- `shortcodes/parser.go` — `ParseWithBlocks` already handles `[mrql]...[/mrql]`
- `shortcodes/processor.go` — dispatch already passes full `Shortcode` struct with `IsBlock` and `InnerContent`

## Documentation

Two docs-site pages need updates:

### `docs-site/docs/features/shortcodes.md`

The `[mrql]` section (starting at line 112) documents only self-closing usage. Add a "Block Syntax" subsection after the existing examples showing:

- Block syntax with inner content as per-item template
- That `[meta]`, `[property]`, `[conditional]`, nested `[mrql]`, and plugin shortcodes work inside the block body
- Template precedence: block template overrides `CustomMRQLResult` and `format` attribute
- Empty/whitespace-only blocks fall back to normal rendering
- Works with flat and bucketed results; aggregated results ignore inner content

Also fix the stale nesting limit: the "Nesting" subsection (line 158) says `[mrql]` nests "up to 2 levels deep" — the actual runtime limit is 10 (`maxRecursionDepth` in `shortcodes/processor.go`). Correct this.

### `docs-site/docs/features/custom-templates.md`

The "Custom MRQL Result Templates" section (starting at line 367) contains unconditional statements like "the custom template is used" and "if any returned entity has a customMRQLResult, custom rendering is used." These need to be revised (not just appended to) to reflect the new precedence rule: when a block `[mrql]` provides a non-empty inner template, it overrides `customMRQLResult` for all items. Rewrite the "How It Works" and "Format Auto-Resolution" subsections to make this explicit.

## Testing

- Unit test: block `[mrql]` with flat results uses inner content as template
- Unit test: block `[mrql]` with bucketed results applies template per item in each bucket
- Unit test: block `[mrql]` with aggregated results ignores inner content
- Unit test: block template overrides `CustomMRQLResult` on items
- Unit test: self-closing `[mrql]` still works unchanged
- Unit test: block `[mrql]` with empty results shows default "No results."
- Unit test: whitespace-only block falls back to normal rendering
- Unit test: block with `format="table"` still uses inner content (block wins over format)
- Unit test: block body with `[property]` or `[meta]` renders item-specific values (proves child context)
- E2E test: block `[mrql]` renders correctly on a group detail page
