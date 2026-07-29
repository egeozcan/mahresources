# Schema-Driven Metadata Display

## Summary

Render category-schema-aware metadata prominently on group and resource detail views. When a category defines a MetaSchema and the entity has Meta data, a beautiful read-only panel appears just below the description, replacing the need to dig through the raw JSON table in the sidebar.

## Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Placement | Below description, top of main content | Maximum visibility |
| Sidebar JSON table | Keep as-is | Power users, extra fields outside schema |
| No schema / no meta | No panel rendered | Display is schema-driven only |
| Layout | Smart hybrid grid | Short scalars in responsive grid, long values full-width below |
| Nested objects | Flat with dot notation | `address.city`, `address.state` in the same grid |
| Null/empty fields | Hidden by default, "Show N hidden fields" toggle | Clean default, full view on demand |
| Labels | Schema `title` as label (fallback: raw key), `description` as tooltip | Human-readable when available |
| Implementation | New Lit `<schema-display-mode>` component | Reuses schema-core.ts, matches form-mode pattern |

## Component: `<schema-display-mode>`

### Location

`src/schema-editor/modes/display-mode.ts`

Registered as a mode in `src/schema-editor/schema-editor.ts` — when `mode="display"`, renders `<schema-display-mode>`.

### Rendering

Uses **light DOM** (like form-mode) to inherit Tailwind styles from the host page.

### Inputs

| Property | Type | Description |
|----------|------|-------------|
| `.schema` | `JSONSchema` | Parsed JSON Schema from the category's MetaSchema |
| `.value` | `object` | The Meta JSON object from the group/resource |
| `.name` | `string` | Category name, shown in the panel header |

### Rendering Pipeline

1. **Walk schema properties** and flatten nested objects with dot-notation keys (e.g., `address.city`)
2. **Classify fields** as "short" or "long":
   - Short: strings <= ~80 chars, numbers, integers, booleans, enums
   - Long: strings > ~80 chars, arrays, deeply nested objects without sub-schema
3. **Render short fields** in a responsive CSS grid (3 cols wide, 2 medium, 1 narrow via `auto-fill` / `minmax()`)
4. **Render long fields** as full-width rows below the grid, separated by a subtle border
5. **Hide null/empty fields** by default; track count for toggle
6. **Resolve `$ref`** using existing `resolveRef()` from schema-core.ts

### Field Type Rendering

| Type | Display |
|------|---------|
| `string` | Plain text |
| `string` (long) | Full-width row, preserves whitespace |
| `string` with `enum` | Pill/badge |
| `string` with labeled enum (`oneOf`+`const`+`title`) | Pill showing title, raw value as tooltip |
| `integer` / `number` | Monospace text |
| `boolean` | "Yes" / "No" text |
| `string` with `format: "uri"` | Clickable link |
| `string` with `format: "email"` | Clickable `mailto:` link |
| `string` with `format: "date"` or `"date-time"` | Formatted date string |
| `array` of scalars | Comma-separated inline, or pills for short values |
| `array` of objects | Full-width compact table |
| Nested object | Flattened with dot-notation keys into the grid |

### Interactions

- **Click-to-copy:** Clicking any value copies it to clipboard (matches existing JSON table behavior). Subtle cursor change on hover.
- **Show/hide empty fields:** Internal Lit state toggle. Button text: "Show N hidden fields" / "Hide empty fields". Revealed fields show em-dash "—" in muted text.

## Panel Styling

Uses the existing `detail-panel` CSS pattern:
- `border: 1px solid #e7e5e4`, `border-radius: var(--radius-md)`
- Header: stone-50 background with monospace uppercase label "METADATA" and category name in muted text
- Body: padding, responsive grid
- Field labels: monospace, 10px uppercase, stone-400 color, `letter-spacing: 0.05em`
- Field values: 14px, stone-900 color
- Numeric values: monospace font
- Enum pills: rounded-full, light tinted background with darker text

## Template Integration

### Group Detail (`displayGroup.tpl`)

Insert below the description block. Condition: category has MetaSchema AND group has Meta data.

```html
{% if group.Category.MetaSchema and group.Meta %}
  <schema-editor mode="display"
    schema="{{ group.Category.MetaSchema|escapejs }}"
    value="{{ group.Meta|escapejs }}"
    name="{{ group.Category.Name }}">
  </schema-editor>
{% endif %}
```

### Resource Detail (`displayResource.tpl`)

Same pattern. A resource can have multiple categories — use the first ResourceCategory that has a MetaSchema.

### No Server-Side Changes

Templates already have access to category and meta data. The component does all rendering client-side.

### Sidebar JSON Table

Unchanged. Continues to show the raw Meta JSON for power users and for fields not covered by the schema.

## Edge Cases

| Scenario | Behavior |
|----------|----------|
| Schema exists, Meta is empty/null | No panel rendered |
| Meta has fields not in schema | Ignored in display panel (visible in sidebar JSON table) |
| Schema has fields not in Meta | Treated as empty, hidden by default, shown via toggle |
| Value doesn't match labeled enum entries | Show raw value plainly (no broken display) |
| Deeply nested objects | Flatten with dot notation up to 3 levels (e.g., `a.b.c`). Beyond that, render as inline JSON in a full-width row. |
| 20+ fields | Grid handles naturally, no pagination |
| No category / no MetaSchema | No panel rendered |

## Files Changed

| File | Change |
|------|--------|
| `src/schema-editor/modes/display-mode.ts` | New file: `<schema-display-mode>` component |
| `src/schema-editor/schema-editor.ts` | Add `mode="display"` case to render method |
| `templates/displayGroup.tpl` | Insert schema-editor display below description |
| `templates/displayResource.tpl` | Insert schema-editor display below description |
| `e2e/tests/` | New E2E test file for schema metadata display |
