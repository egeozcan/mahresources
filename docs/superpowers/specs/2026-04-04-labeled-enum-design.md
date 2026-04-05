# Labeled Enums in JSON Schema Editor

## Problem

Integer enum properties (e.g., `active` with values 0, 1, 2) rely on the `description` field to document meaning ("-1 all, 1 active, 0 inactive, 2 hidden"). Forms and search render raw numbers with no labels, making the UI hard to use.

## Solution

Support labeled enums using the standard JSON Schema `oneOf` + `const` + `title` pattern. Provide a conversion path from plain enums, and render labels in forms and search.

### Schema Representation

Plain enum (current):
```json
{ "type": "integer", "enum": [0, 1, 2] }
```

Labeled enum (new):
```json
{
  "type": "integer",
  "oneOf": [
    { "const": 0, "title": "Inactive" },
    { "const": 1, "title": "Active" },
    { "const": 2, "title": "Hidden" }
  ]
}
```

## Design

### 1. Labeled Enum Detection

A helper function `isLabeledEnum(schema)` that returns `true` when:
- Schema has `oneOf` (array, length >= 1)
- Every `oneOf` entry has a `const` property
- No `oneOf` entry contains complex keywords (`type`, `properties`, `items`, `oneOf`, `anyOf`, `allOf`, `if`)

This distinguishes labeled enums from complex variant selection. Used by the editor, form mode, and search mode to decide rendering strategy.

### 2. Schema Editor (Edit Mode) — Enum Editor Changes

The enum editor gains an optional **Label** column alongside values:

```
Value    Label
-----    ----------
0        Inactive
1        Active
2        Hidden

[+ Add Value]
```

**Storage rules:**
- If any value has a label: store as `oneOf` + `const` + `title`
- If no values have labels: store as plain `enum: [values]`
- Clearing all labels collapses back to plain `enum`

**"Add Labels" conversion button:**
- Displayed when the editor detects a plain `enum` with no labels
- Converts `enum: [0, 1, 2]` to `oneOf: [{ const: 0 }, { const: 1 }, { const: 2 }]` with empty title fields
- Pre-fills the values, user fills in labels

### 3. Form Mode — Dropdown Rendering

When `isLabeledEnum()` returns true, render a `<select>` showing labels but submitting values:

```html
<select>
  <option value="0">Inactive</option>
  <option value="1">Active</option>
  <option value="2">Hidden</option>
</select>
```

- Show `title` if present, fall back to raw value if not
- Nullable fields get an empty option at the top (existing behavior)
- This bypasses the complex `oneOf` variant selector — no "which variant?" dropdown needed for simple constants

### 4. Search Mode — Labeled Rendering

Same label lookup applies to search:
- Checkboxes (<=6 values): show label text instead of raw number
- Multi-select (7+ values): show label text in options
- Submitted query values remain the raw integers

### 5. Backward Compatibility

- Existing plain `enum` schemas continue to work unchanged
- No data migration needed — the stored data values don't change (still integers)
- Only the schema definition changes when a user adds labels
- The `oneOf` + `const` pattern is standard JSON Schema; external tools understand it

## Files to Modify

| File | Change |
|------|--------|
| `src/schema-editor/schema-core.ts` | Add `isLabeledEnum()` helper |
| `src/schema-editor/tree/node-editors/enum-editor.ts` | Add label column, "Add Labels" button, bidirectional conversion |
| `src/schema-editor/modes/form-mode.ts` | Detect labeled enum in `_renderField`, render labeled dropdown |
| `src/schema-editor/modes/search-mode.ts` | Detect labeled enum, render labels in checkboxes/multi-select |

## Out of Scope

- Migration tool for bulk-converting existing schemas
- Backend validation changes (the `oneOf` + `const` pattern is already valid JSON Schema)
- Changes to API responses or Go code
