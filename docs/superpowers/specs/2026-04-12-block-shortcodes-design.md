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

### `[else]` Handling

`[else]` is not a parser-level token. It is a literal string inside block content. The `conditional` handler splits `InnerContent` on `[else]` internally. This keeps the parser simple and avoids special-casing in the general pipeline.

## Processing Pipeline Changes

`processWithDepth` switches to `ParseWithBlocks()`:

- **Self-closing shortcodes**: behavior identical to today.
- **Block shortcodes**: inner content is recursively processed through `processWithDepth` first (expanding nested shortcodes), then the handler receives the fully-rendered `InnerContent`.

This means handlers always see expanded inner content. A `[conditional]` wrapping `[meta path="x"]` receives the rendered `<meta-shortcode>` element, not the raw shortcode text.

### Plugin Renderer

No signature change needed. `PluginRenderer` already receives the full `Shortcode` struct, which now includes `InnerContent` and `IsBlock`. On the Lua side, the context table gains an `inner_content` string field (empty for self-closing shortcodes).

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

1. **Resolve value**: uses `extractValueAtPath` from `meta_handler.go` to read from entity meta JSON via the `path` attribute.
2. **Evaluate condition**: checks one operator attribute against the resolved value.
3. **Split on `[else]`**: if `InnerContent` contains `[else]`, split into true/false branches. Only the first `[else]` is recognized (subsequent ones are literal text in the else branch).
4. **Return**: matching branch content, or empty string if condition fails with no else branch.

### Supported Operators

| Attribute | Condition |
|-----------|-----------|
| `eq` | String equality: `tostring(value) == attr` |
| `neq` | String inequality: `tostring(value) != attr` |
| `gt` | Numeric greater than: `tonumber(value) > tonumber(attr)` |
| `lt` | Numeric less than: `tonumber(value) < tonumber(attr)` |
| `contains` | Substring match: `strings.Contains(tostring(value), attr)` |
| `empty` | Value is empty/missing (attribute value ignored, presence triggers) |
| `not-empty` | Value is non-empty (attribute value ignored, presence triggers) |

Only one operator per shortcode. If multiple are present, first match in the order above wins.

### Registration

- `conditional` added to the parser's shortcode name regex pattern.
- New case in `processWithDepth`'s switch statement.
- Non-block usage (`[conditional path="x" eq="y"]` without closing tag) is valid but does nothing useful (empty inner content, returns empty string).

## Plugin Removal

Remove the `conditional` shortcode from `plugins/data-views/plugin.lua` entirely. This includes the `render_conditional` function and its `mah.shortcode()` registration.

## Testing

### Unit Tests

- **`parser_test.go`**: block parsing (matched pairs, nesting, unmatched tags, mixed self-closing and block, `[else]` as literal content)
- **`processor_test.go`**: block processing (inner content expansion, nested blocks, depth limiting)
- **`conditional_handler_test.go`** (new): each operator, else branch, missing path, nested shortcodes in content, non-block usage

### E2E Tests

- Extend `e2e/tests/shortcodes.spec.ts` with conditional shortcode scenarios on group/note detail pages using meta values set via the API.

### Plugin Integration

- Verify plugin shortcodes receive `inner_content` in Lua context table.
- Existing plugin shortcode tests pass unchanged (self-closing behavior preserved).
