# Schema-Driven Search Fields for List Views

**Date:** 2026-03-31
**Status:** Approved

## Problem

List view search forms have a generic `freeFields` component for metadata queries that requires users to manually type field names, pick operators, and enter values. When entities have a category/type with a defined MetaSchema, the search form should offer structured, type-appropriate input fields derived from that schema — reducing friction and making metadata-based filtering discoverable.

## Scope

**In scope:**
- Groups (via `Category.MetaSchema`)
- Resources (via `ResourceCategory.MetaSchema`)

**Out of scope:**
- Notes (`NoteType` has no MetaSchema field)

## Design

### Triggering

When a user selects one or more categories in the search form autocompleter:

1. The autocompleter dispatches a `multiple-input` event (line 65 of `src/components/dropdown.js`) with `{ value: selectedResults, name: elName }`. Each selected category object includes its `MetaSchema` string field.
2. An event listener on the form captures this event, extracts `MetaSchema` from each selected category, and parses them as JSON Schema objects.
3. The parsed schemas are flattened and (if multiple) intersected, producing a list of searchable fields.
4. These fields are passed to a new `schemaSearchFields` Alpine.js component that renders the appropriate inputs.
5. If no categories are selected, or none have a MetaSchema, the schema search section is hidden entirely.

### Schema Flattening

The component recursively walks each JSON Schema and produces a flat list of field descriptors:

```
Input:
{
  "type": "object",
  "properties": {
    "color": { "type": "string", "enum": ["red", "green", "blue"] },
    "weight": { "type": "number" },
    "dimensions": {
      "type": "object",
      "properties": {
        "width": { "type": "number" },
        "height": { "type": "number" }
      }
    }
  }
}

Output:
[
  { path: "color",            label: "Color",              type: "string", enum: ["red","green","blue"] },
  { path: "weight",           label: "Weight",             type: "number", enum: null },
  { path: "dimensions.width", label: "Dimensions › Width", type: "number", enum: null },
  { path: "dimensions.height",label: "Dimensions › Height",type: "number", enum: null }
]
```

Rules:
- Object-typed properties are recursed into, not rendered as fields themselves.
- The `path` uses dot notation matching the existing `JSONQueryExpression` key format (e.g., `$.dimensions.width`).
- Labels use `›` separator for nested paths. If the schema property has a `title`, use that; otherwise title-case the key.
- Array-typed properties are skipped (not meaningful for search).

### Multi-Category Intersection

When multiple categories are selected:

1. Flatten each schema independently.
2. Keep only fields whose `path` appears in ALL schemas.
3. If types differ for the same path: fall back to `"string"` (text input).
4. If enums differ for the same path: drop the enum, render as text input.
5. If enums are identical: keep the enum.

### Field Rendering

| Schema Type | Input Element | Default Operator |
|---|---|---|
| `string` | Text input | LIKE |
| `number` / `integer` | Number input | EQ |
| `boolean` | Three-state radio group (Any / Yes / No) | EQ |
| `string` with `enum` ≤ 6 values | Checkboxes | EQ (OR logic across checked values) |
| `string` with `enum` > 6 values | Multi-select dropdown | EQ (OR logic across selected values) |

**Operator override:** String and number fields display a small operator symbol next to the input (`≈` for LIKE, `=` for EQ). Clicking it expands into a `<select>` dropdown with all applicable operators:
- String: LIKE, =, ≠, NOT LIKE
- Number/Integer: =, ≠, >, ≥, <, ≤

Boolean and enum fields have no operator override (EQ is the only meaningful operator).

### URL Serialization

Schema fields output standard `MetaQuery` parameters, identical to what `freeFields` produces:

```
MetaQuery.0=color:LI:blue
MetaQuery.1=dimensions.width:GE:100
MetaQuery.2=status:EQ:"active"
MetaQuery.3=status:EQ:"draft"
MetaQuery.4=published:EQ:true
```

This requires zero backend changes. The existing `ParseMeta` / `FillMetaQueryFromRequest` / `JSONQueryExpression` pipeline handles all of these already.

**State restoration:** On page load, if MetaQuery params exist in the URL AND a category is selected, the component parses those params and pre-fills matching schema fields. MetaQuery params that don't match any schema field path are left alone for the `freeFields` component to handle.

**Empty fields:** Not included in form submission. Standard browser form behavior.

### Form Integration

**Alpine.js scope:** The list view forms (`listGroups.tpl`, `searchFormResource.tpl`) currently have no `x-data` wrapper. The `schemaSearchFields.tpl` partial introduces its own self-contained `x-data="schemaSearchFields({...})"` scope on a wrapper `<div>`. It listens for the `multiple-input` event from the category autocompleter using `@multiple-input.window` with a name filter (e.g., `$event.detail.name === 'categories'` for groups, `$event.detail.name === 'ResourceCategoryId'` for resources). This is the same pattern used in `createGroup.tpl` (line 74). No modifications needed on the parent `<form>` element.

**Placement:** Schema search fields appear directly above the existing `freeFields` section. When no applicable schemas are available, the section is hidden with no empty container or layout shift.

**Styling:** Subtle label-based field names (e.g., "Color", "Dimensions › Width") matching the rest of the form. No bordered section or special grouping.

### Accessibility

- All fields get proper `<label>` elements associated via `for`/`id`.
- Operator override dropdown is keyboard-accessible (focusable, operable via Enter/Space).
- Container uses `aria-live="polite"` so screen readers announce when schema fields appear/disappear.
- Enum checkboxes wrapped in `<fieldset>` with `<legend>`.
- Boolean radio groups wrapped in `<fieldset>` with `<legend>`.
- Nested field labels include full path in `aria-label` (e.g., `aria-label="Dimensions, Width"`).
- Tab order follows visual field order.

## File Changes

### New Files

- **`src/components/schemaSearchFields.js`** — Alpine.js component (~200 lines). Handles schema parsing, flattening, intersection, field rendering, operator override, and MetaQuery serialization.
- **`templates/partials/form/schemaSearchFields.tpl`** — Template partial for the schema search section. Shared between groups and resources to avoid markup duplication.

### Modified Files

- **`templates/listGroups.tpl`** — Add `x-on:multiple-input` listener on the categories autocompleter to capture schema data. Include the new `schemaSearchFields.tpl` partial between the categories autocompleter and the `freeFields` include (between current lines 47 and 52).
- **`templates/partials/form/searchFormResource.tpl`** — Same pattern: listener on ResourceCategory autocompleter, include new partial between current lines 30 and 31.
- **`src/main.js`** — Import and register `schemaSearchFields` via `Alpine.data('schemaSearchFields', schemaSearchFields)`.

### Unchanged

- All backend Go code (handlers, query models, database scopes, API endpoints).
- `src/components/freeFields.js` — untouched.
- `src/components/schemaForm.js` — untouched (create/edit stays as-is).
- Note templates (no MetaSchema support on NoteType).

## Testing

### E2E Tests (Playwright)

- Select a category with a MetaSchema on the groups list page; verify schema fields appear.
- Select a ResourceCategory with a MetaSchema on the resources list page; verify schema fields appear.
- Deselect the category; verify schema fields disappear.
- Select two categories with overlapping schemas; verify only common fields appear.
- Select two categories with no common fields; verify schema section is hidden.
- Fill schema fields and submit; verify URL contains correct MetaQuery params.
- Load a URL with MetaQuery params and a selected category; verify fields are pre-filled.
- Test operator override: click operator symbol, select a different operator, submit, verify URL.
- Test enum checkboxes: check multiple values, submit, verify multiple MetaQuery entries with OR.
- Test boolean radio: select Yes/No, submit, verify MetaQuery entry.

### Accessibility Tests

- Verify all schema fields have associated labels.
- Verify fieldset/legend for enum and boolean groups.
- Verify keyboard navigation through operator override.
- Verify aria-live announcement on schema section show/hide.
