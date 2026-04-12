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
    InnerContent string   // content between [name]...[else] or [/name], empty for self-closing
    ElseContent  string   // content after [else] in a block shortcode, empty if no [else]
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

`[else]` is a parser-level token recognized inside block shortcodes. When the parser matches a block pair, it scans the inner content for the first top-level `[else]` (not nested inside a deeper block) and splits the content into `InnerContent` (true branch) and `ElseContent` (false branch). This is structural, not a post-processing string split.

The `Shortcode` struct gets an additional field:

```go
ElseContent string // content after [else] in a block shortcode, empty if no [else]
```

## Processing Pipeline Changes

`processWithDepth` switches to `ParseWithBlocks()`:

- **Self-closing shortcodes**: behavior identical to today.
- **Block shortcodes**: the handler receives raw `InnerContent` and `ElseContent`. Handlers that need expansion call `processWithDepth` on the branch they select. Handlers that don't care about branching can expand `InnerContent` unconditionally.

This means the conditional handler evaluates its condition first, then only expands the taken branch. Shortcodes in the untaken branch are never executed — no wasted MRQL queries or plugin side effects.

### Plugin Renderer

No signature change needed. `PluginRenderer` already receives the full `Shortcode` struct, which now includes `InnerContent`, `ElseContent`, and `IsBlock`. On the Lua side, the context table gains `inner_content` and `else_content` string fields (empty for self-closing shortcodes).

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

1. **Resolve value**: uses a new `extractRawValueAtPath` helper that navigates the meta JSON by dot-notation path and returns the raw `any` value (string, float64, bool, nil) — not JSON-encoded text. The existing `extractValueAtPath` returns JSON-encoded strings (e.g., `"active"` with surrounding quotes) which is correct for the `[meta]` shortcode's data attributes but wrong for conditional string comparisons.
2. **Evaluate condition**: checks one operator attribute against the resolved raw value. Strings are compared directly, numbers via `strconv.ParseFloat`.
3. **Select branch**: if condition is true, select `InnerContent`. If false, select `ElseContent` (may be empty). The parser has already split these structurally.
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
- New case in `processWithDepth`'s switch statement.
- Non-block usage (`[conditional path="x" eq="y"]` without closing tag) is valid but does nothing useful (empty inner content, returns empty string).

## Plugin Removal

Remove the `conditional` shortcode from `plugins/data-views/plugin.lua` entirely. This includes the `render_conditional` function and its `mah.shortcode()` registration.

### Feature delta vs. plugin version

The plugin version supports attributes the built-in intentionally does not carry forward:

| Plugin attribute | Built-in status | Notes |
|-----------------|----------------|-------|
| `path` | Kept | Same — dot-notation meta lookup as condition source |
| `field` | Removed | Was a condition source (entity struct field). Not carried forward — `path` covers the primary use case. Could be added later if needed. |
| `mrql` / `scope` / `aggregate` | Removed | Were condition sources (run a query, compare the result). Not carried forward — these are genuinely lost capabilities, not replaceable by composition since `[mrql]` inside the block only affects rendered content, not the condition evaluation. Could be added later as additional condition source attributes. |
| `html` / `content` | Replaced | Block body replaces these entirely |
| `class` | Replaced | Use HTML directly in the block body: `<div class="...">` |

The built-in version trades two condition source modes (`field`, `mrql`) for block syntax with nesting. The `path`-based condition covers the primary use case. The dropped condition sources can be added as future attributes if demand arises.

## Testing

### Unit Tests

- **`parser_test.go`**: block parsing (matched pairs, nesting, unmatched tags, mixed self-closing and block, `[else]` as literal content)
- **`processor_test.go`**: block processing (inner content expansion, nested blocks, depth limiting)
- **`conditional_handler_test.go`** (new): each operator, else branch, missing path, nested shortcodes in content, non-block usage

### E2E Tests

- Extend `e2e/tests/shortcodes.spec.ts` with conditional shortcode scenarios on group/note detail pages using meta values set via the API.

### Plugin Integration

- Verify plugin shortcodes receive `inner_content` and `else_content` in Lua context table.
- Existing self-closing plugin shortcode tests pass unchanged.

### Downstream Updates from Plugin Removal

Removing `data-views:conditional` requires updates in:

- **`e2e/test-plugins/data-views/plugin.lua`**: remove the `conditional` shortcode registration (line 1684), `render_conditional` function (line 1363), and the help text reference (line 27).
- **`e2e/tests/plugins/plugin-data-views.spec.ts`**: remove or rewrite the `conditional` test (line 160) and the shortcode reference in group creation (line 28). The test should exercise the built-in `[conditional]` instead.
- **`plugins/data-views/plugin.lua`**: remove `render_conditional` (line 1694), `mah.shortcode` registration (line 2569), and examples (lines 2591-2595).
