# Block Shortcodes & Built-in Conditional

## Summary

Extend the shortcode system to support paired opening/closing tags (`[name]content[/name]`) alongside existing self-closing tags (`[name ...]`). Add a built-in `[conditional]` shortcode that replaces the plugin-provided version from `data-views`.

## Decisions

- **Syntax**: WordPress-style `[name attrs]content[/name]`
- **Nesting**: Arbitrary nesting with depth limit (10)
- **Opt-in per handler**: Parser always parses both forms; handlers decide whether to use `InnerContent`
- **Else support**: `[conditional ...]content[else]fallback[/conditional]`
- **Migration**: Remove `conditional` from `data-views` plugin entirely (not used in production)

## Parser Changes

### Shortcode Struct

```go
type Shortcode struct {
    Name         string
    Attrs        map[string]string
    Raw          string   // full matched text (opening+closing for blocks, tag for self-closing)
    Start        int      // byte offset of opening tag start
    End          int      // byte offset of closing tag end (or opening tag end if self-closing)
    InnerContent string   // content between [name]...[/name], empty for self-closing
    IsBlock      bool     // true if matched as [name]...[/name] pair
}
```

### Parsing Strategy

Two-phase approach extending the existing regex parser:

1. **Phase 1** — Tokenize: find all opening tags `[name ...]` and closing tags `[/name]` using regex. Opening tags use the existing `shortcodePattern` (extended to include `conditional`). Closing tags use a new pattern `\[/(meta|property|mrql|conditional|plugin:...)\]`.

2. **Phase 2** — Pair matching: iterate tokens inside-out (innermost pairs first) using a stack. When a closing tag is found, scan backward for the nearest unmatched opening tag with the same name. Matched pairs become block shortcodes. Unmatched opening tags remain self-closing. Unmatched closing tags are ignored (left as literal text).

### API

- Existing `Parse()` function continues to return self-closing shortcodes only (backward compatible).
- New `ParseWithBlocks()` function returns both self-closing and block shortcodes.
- The processor switches to `ParseWithBlocks()`.

**Output contract**: `ParseWithBlocks()` returns only top-level (non-overlapping) shortcodes. Nested shortcodes inside a block's `InnerContent` are not returned — they are left as raw text for recursive processing by the handler or the processor. This preserves the processor's linear Start/End splice algorithm unchanged.

### `[else]` Handling

`[else]` splitting is handler-level, not parser-level. The parser does not know about `[else]` — it returns the full content between `[name]` and `[/name]` as `InnerContent`. The `conditional` handler splits `InnerContent` on the first top-level `[else]` internally using a helper function `splitElse(content string) (ifBranch, elseBranch string)` that is aware of nested block depth (so `[else]` inside a nested block is not mistaken for the outer split point).

This keeps `[else]` specific to handlers that opt into it. Plugin block shortcodes and future built-ins receive the full `InnerContent` with any literal `[else]` text intact.

The `ElseContent` field is removed from the `Shortcode` struct — it is not needed since splitting is handler-side.

## Processing Pipeline Changes

`processWithDepth` switches to `ParseWithBlocks()`:

- **Self-closing shortcodes**: behavior identical to today.
- **Block shortcodes**: the handler receives raw `InnerContent` (the full text between `[name]` and `[/name]`). Handlers that need branching (like `conditional`) call `splitElse(InnerContent)` to get `(ifBranch, elseBranch)`, then expand only the selected branch via `processWithDepth`. Handlers that don't care about branching expand `InnerContent` unconditionally.

This means the conditional handler evaluates its condition first, splits on `[else]`, then only expands the taken branch. Shortcodes in the untaken branch are never executed — no wasted MRQL queries or plugin side effects.

### Plugin Renderer

No signature change needed. `PluginRenderer` already receives the full `Shortcode` struct, which now includes `InnerContent` and `IsBlock`. On the Lua side, the context table gains two fields: `inner_content` (string, empty for self-closing) and `is_block` (boolean). The `is_block` field lets plugins distinguish a self-closing `[name ...]` from an explicitly empty block `[name][/name]`.

Plugin block shortcodes receive raw (unexpanded) inner content. The processor applies a post-plugin expansion pass: after the plugin renderer returns, if the shortcode was a block (`IsBlock == true`), the processor runs `processWithDepth` on the plugin's output to expand any shortcodes the plugin left intact or injected. This mirrors how MRQL already gets a recursive expansion pass via `processWithDepth` in `RenderMRQLShortcode`. If a plugin wants to suppress expansion (treating inner content as a literal template), it can HTML-escape the bracket syntax in its output.

### Recursion Depth

The existing `maxRecursionDepth` (currently 2) is bumped to 10 to accommodate reasonable block nesting. Block nesting and MRQL recursive expansion share the same depth counter.

## Built-in Conditional Shortcode

### File

`shortcodes/conditional_handler.go`

### Syntax

```
[conditional path="status" eq="active"]
  <h2>Active Item</h2>
[/conditional]

[conditional path="status" eq="active"]
  <h2>Active</h2>
[else]
  <h2>Inactive</h2>
[/conditional]
```

### Handler Logic

1. **Resolve value**: three condition sources, checked in order: `mrql` > `field` > `path`.
   - **`path`**: uses a new `extractRawValueAtPath` helper that navigates the meta JSON by dot-notation path and returns the raw `any` value (string, float64, bool, nil) — not JSON-encoded text. The existing `extractValueAtPath` returns JSON-encoded strings (e.g., `"active"` with surrounding quotes) which is correct for the `[meta]` shortcode's data attributes but wrong for conditional comparisons.
   - **`field`**: reads an entity struct field by name via reflection (reuses the same approach as `RenderPropertyShortcode`). e.g., `field="Name"` reads the entity's `Name` field. Returns the raw Go value.
   - **`mrql`** (+ optional `scope`, `aggregate`): runs an MRQL query via the `QueryExecutor` callback (already threaded through `processWithDepth`). Scalar extraction by result mode: **flat** — item count; **aggregated** — first row's column named by `aggregate` (error if `aggregate` not provided); **bucketed** — number of groups. `scope` restricts results to a group subtree (same semantics as `[mrql scope="..."]`). This matches the current plugin's `resolve_scalar_from_mrql` behavior exactly.
2. **Evaluate condition**: checks one operator attribute against the resolved raw value. Strings are compared directly, numbers via `strconv.ParseFloat`.
3. **Select branch**: the handler calls `splitElse(InnerContent)` to get `(ifBranch, elseBranch)`. If condition is true, select `ifBranch`. If false, select `elseBranch` (may be empty).
4. **Expand and return**: the selected branch is recursively processed through `processWithDepth` to expand nested shortcodes. The unselected branch is never processed — no wasted work or side effects.

### Supported Operators

| Attribute | Condition |
|-----------|-----------|
| `eq` | String equality: `fmt.Sprint(value) == attr` (raw value, not JSON-encoded) |
| `neq` | String inequality: `fmt.Sprint(value) != attr` |
| `gt` | Numeric: `toFloat(value) > toFloat(attr)`, false if either non-numeric |
| `lt` | Numeric: `toFloat(value) < toFloat(attr)`, false if either non-numeric |
| `contains` | Substring: `strings.Contains(fmt.Sprint(value), attr)` |
| `empty` | Value is nil or empty string (attribute value ignored, presence triggers) |
| `not-empty` | Value is non-nil and non-empty (attribute value ignored, presence triggers) |

Only one operator per shortcode. If multiple are present, first match in the order above wins.

### Registration

- `conditional` added to the parser's shortcode name regex pattern.
- New case in `processWithDepth`'s switch statement. The handler receives the `MetaShortcodeContext` (for `path` and `field` resolution) and the `QueryExecutor` callback (for `mrql` resolution) — both already available in `processWithDepth`.
- Non-block usage (`[conditional path="x" eq="y"]` without closing tag) is valid but does nothing useful (empty inner content, returns empty string).

## Plugin Removal

Remove the `conditional` shortcode from `plugins/data-views/plugin.lua` entirely. This includes the `render_conditional` function and its `mah.shortcode()` registration.

### Feature delta vs. plugin version

| Plugin attribute | Built-in status | Notes |
|-----------------|----------------|-------|
| `path` | Kept | Same — dot-notation meta lookup as condition source |
| `field` | Kept | Same — entity struct field as condition source |
| `mrql` / `scope` / `aggregate` | Kept | Same — query-based condition source |
| `html` / `content` | Replaced | Block body replaces these entirely |
| `class` | Replaced | Use HTML directly in the block body: `<div class="...">` |

All condition sources are preserved. The block syntax replaces the content-delivery attributes (`html`, `content`, `class`) with something strictly more capable.

## Testing

### Unit Tests

- **`parser_test.go`**: block parsing (matched pairs, nesting, unmatched tags, mixed self-closing and block, `[else]` as literal content)
- **`processor_test.go`**: block processing (inner content expansion, nested blocks, depth limiting)
- **`conditional_handler_test.go`** (new): each operator, else branch, missing path, nested shortcodes in content, non-block usage

### E2E Tests

- Extend `e2e/tests/shortcodes.spec.ts` with conditional shortcode scenarios on group/note detail pages using meta values set via the API.

### Plugin Integration

- Verify plugin shortcodes receive `inner_content` in Lua context table.
- Existing self-closing plugin shortcode tests pass unchanged.

### Plugin Docs Preview

The plugin docs preview pipeline (`renderShortcodeForDocs` in `plugin_system/shortcodes.go`) currently parses examples using the self-closing-only path. This needs updating:

- **`plugin_system/shortcode_docs.go`** (or wherever example parsing happens): switch to `ParseWithBlocks()` so block shortcode examples preview correctly.
- **`plugin_system/shortcodes.go` `renderShortcodeForDocs`**: pass `inner_content` and `is_block` through to the Lua context table, same as the runtime path.
- **Post-render expansion**: docs preview does not support nested shortcode expansion. The preview environment lacks the full `MetaShortcodeContext` and `QueryExecutor` needed for built-in shortcode expansion, and the preview is scoped to a single plugin's shortcode. This is an acceptable limitation — docs examples show the plugin's own output, not a full rendering pipeline. If a plugin block shortcode returns raw nested shortcodes in its preview, they will appear as literal text.
- Add an explicit test that verifies a plugin block shortcode preview with nested shortcodes in its output renders those as literal text (not expanded). This documents the intentional divergence from runtime behavior.
- Add test coverage for a plugin block shortcode example rendering in docs preview.

### Downstream Updates from Plugin Removal

Removing `data-views:conditional` requires updates in:

- **`e2e/test-plugins/data-views/plugin.lua`**: remove the `conditional` shortcode registration (line 1684), `render_conditional` function (line 1363), and the help text reference (line 27).
- **`e2e/tests/plugins/plugin-data-views.spec.ts`**: remove or rewrite the `conditional` test (line 160) and the shortcode reference in group creation (line 28). The test should exercise the built-in `[conditional]` instead.
- **`plugins/data-views/plugin.lua`**: remove `render_conditional` (line 1694), `mah.shortcode` registration (line 2569), and examples (lines 2591-2595).

### Docs Site

Update the docs site (`docs-site/`) to document:

- Block shortcode syntax (`[name]...[/name]`) and nesting
- The built-in `[conditional]` shortcode with all condition sources (`path`, `field`, `mrql`), operators, and `[else]` support
- Examples showing block shortcodes with nested content and conditionals
- Note for plugin authors: docs preview does not expand nested shortcodes inside plugin block shortcode output (they render as literal text). Runtime rendering expands them normally.
