# Visual JSON Schema Editor — Design Spec

**Date:** 2026-04-01
**Status:** Draft

## Overview

Add a visual JSON Schema editor to group categories (`Category.MetaSchema`) and resource types (`ResourceCategory.MetaSchema`). The editor replaces the current raw textarea with a modal-based visual builder, and unifies three existing schema-consuming UIs into a single `<schema-editor>` Lit web component with TypeScript.

## Goals

1. **Visual schema authoring** — build and edit full JSON Schema documents without writing JSON, supporting all keywords across all drafts the `jsonschema/v6` library supports.
2. **Unified component** — a single `<schema-editor>` web component that handles three modes: schema editing, form rendering (data entry), and search field rendering (filtering).
3. **Testable in isolation** — the component works standalone in a plain HTML file with no Go server or Alpine.js dependency.
4. **Replace existing implementations** — subsume `schemaForm.js` (Alpine) and `schemaSearchFields.js` (Alpine) with the web component's `form` and `search` modes.

## Non-Goals

- No Go backend changes — `MetaSchema` field, API, models remain unchanged.
- No server-side JSON Schema validation on entity create/update (remains UI-driven).
- No changes to `freeFields.js` — it continues to handle free-form metadata when no schema exists.

## Architecture

### Module Structure

```
src/
  schema-editor/
    schema-editor.ts          # <schema-editor> Lit web component (entry point)
    schema-core.ts             # Shared schema utilities (extracted + new)
    modes/
      edit-mode.ts             # Tree + detail panel renderer (new)
      form-mode.ts             # Data entry form renderer (from schemaForm.js)
      search-mode.ts           # Search filter renderer (from schemaSearchFields.js)
    tree/
      schema-tree-model.ts     # Internal tree representation of a schema
      tree-panel.ts            # Left sidebar tree component
      detail-panel.ts          # Right detail editor component
      node-editors/
        string-editor.ts       # String type constraints
        number-editor.ts       # Number/integer type constraints
        boolean-editor.ts      # Boolean type constraints
        object-editor.ts       # Object type (properties, additionalProperties, etc.)
        array-editor.ts        # Array type (items, prefixItems, min/maxItems, etc.)
        enum-editor.ts         # Enum value list management
        composition-editor.ts  # oneOf, anyOf, allOf, not
        conditional-editor.ts  # if/then/else
        ref-editor.ts          # $ref picker (from $defs)
    test.html                  # Standalone test page for all three modes
```

### Technology Choices

- **Lit** (`lit` package) — lightweight web component library for reactive rendering. ~5KB gzipped. Provides declarative templates, reactive properties, and efficient DOM updates. Encapsulated in Shadow DOM so it doesn't interfere with the Alpine.js host page.
- **TypeScript** — type safety for the recursive schema manipulation logic. Vite has built-in TS support, no config changes needed.
- **No new Go dependencies** — the component is purely frontend.

### schema-core.ts — Shared Utilities

Extracted from the current duplicated logic in `schemaForm.js` and `schemaSearchFields.js`:

| Function | Current Location | Used By |
|----------|-----------------|---------|
| `resolveRef()` | Both files (duplicated) | All modes |
| `mergeSchemas()` | Both files (duplicated) | All modes |
| `resolveSchema()` | `schemaSearchFields.js` | Search + edit modes |
| `flattenSchema()` | `schemaSearchFields.js` | Search mode |
| `intersectFields()` | `schemaSearchFields.js` | Search mode |
| `getDefaultValue()` | `schemaForm.js` | Form mode |
| `scoreSchemaMatch()` | `schemaForm.js` | Form mode |
| `evaluateCondition()` | `schemaForm.js` | Form mode |
| `inferType()` / `inferSchema()` | `schemaForm.js` | Form mode |

New additions for edit mode:

| Function | Purpose |
|----------|---------|
| `schemaToTree()` | Parse JSON Schema into internal tree model |
| `treeToSchema()` | Serialize tree model back to JSON Schema |
| `detectDraft()` | Auto-detect JSON Schema draft from `$schema` keyword |

## Component API

### `<schema-editor>` Web Component

```html
<!-- Edit mode: visual schema builder (used in modal) -->
<schema-editor
  mode="edit"
  schema='{"type":"object","properties":{"name":{"type":"string"}}}'
></schema-editor>

<!-- Form mode: data entry form (replaces schemaForm Alpine component) -->
<schema-editor
  mode="form"
  schema='...'
  value='{"name":"Alice"}'
  name="Meta"
></schema-editor>

<!-- Search mode: filter fields with operators (replaces schemaSearchFields) -->
<schema-editor
  mode="search"
  schema='...'
  meta-query='[{"name":"status","value":"active","operation":"EQ"}]'
></schema-editor>
```

### Attributes

| Attribute | Type | Modes | Description |
|-----------|------|-------|-------------|
| `mode` | `"edit" \| "form" \| "search"` | All | Rendering mode |
| `schema` | JSON string | All | The JSON Schema document |
| `value` | JSON string | `form` | Current data value for form rendering |
| `name` | string | `form` | Hidden input name for form submission |
| `meta-query` | JSON string | `search` | Pre-parsed MetaQuery for restoring search state |
| `field-name` | string | `search` | Form field name for hidden inputs (default: `"MetaQuery"`) |

### Events

| Event | Modes | Detail | Description |
|-------|-------|--------|-------------|
| `schema-change` | `edit` | `{ schema: string }` | Schema modified in editor |
| `value-change` | `form` | `{ value: object }` | Data value changed in form |
| `schema-fields-claimed` | `search` | `{ paths: string[] }` | Claimed field paths (for freeFields coordination) |

### Methods

| Method | Modes | Description |
|--------|-------|-------------|
| `getSchema(): string` | `edit` | Returns current schema as JSON string |
| `getValue(): object` | `form` | Returns current form data |
| `validate(): boolean` | `form` | Validates current data against schema |

## Editor Mode (mode="edit")

### Layout

**Tree + Detail Panel** split view:

- **Left panel (tree):** Collapsible outline of the schema structure. Each node shows: drag handle, property name, required indicator, type badge. Supports drag-and-drop reordering within the same parent. `$defs` section at the bottom for reusable definitions.
- **Right panel (detail):** Edits the selected node. Shows breadcrumb path, basic fields (name, type, title, description), flags (required, nullable, readOnly, writeOnly), and type-specific constraint editors.

### Tree Node Types

Each schema construct maps to a tree node:

| JSON Schema | Tree Display | Badge Color |
|-------------|-------------|-------------|
| `"type": "string"` | Property name | Green |
| `"type": "integer"` / `"number"` | Property name | Blue |
| `"type": "boolean"` | Property name | Yellow |
| `"type": "object"` | Expandable, shows children | Indigo |
| `"type": "array"` | Expandable, shows `items` | Purple |
| `"enum": [...]` | Property name | Amber |
| `"oneOf"` / `"anyOf"` / `"allOf"` | Expandable, shows variants | Pink |
| `"if"` / `"then"` / `"else"` | Expandable, shows branches | Rose |
| `"$ref"` | Shows reference target | Gray |
| `"$defs"` entry | Under $defs section | Slate |

### Detail Panel — Type-Specific Editors

**string:** title, description, minLength, maxLength, pattern, format (dropdown: date, date-time, email, uri, uuid, etc.), enum values, const, default.

**number / integer:** title, description, minimum, maximum, exclusiveMinimum, exclusiveMaximum, multipleOf, enum values, const, default.

**boolean:** title, description, const, default.

**object:** title, description, properties (shown in tree), required (checkboxes), additionalProperties (boolean or schema), minProperties, maxProperties, patternProperties (key-pattern → schema pairs).

**array:** title, description, items (schema), prefixItems (ordered list of schemas), minItems, maxItems, uniqueItems, contains (schema).

**composition (oneOf/anyOf/allOf/not):** list of sub-schemas, each editable. Add/remove variants. `not` takes a single sub-schema.

**conditional (if/then/else):** three sub-schema slots. `if` is the condition, `then`/`else` are applied schemas.

**$ref:** dropdown picker populated from `$defs` entries in the current schema.

### Tree Operations

- **Add property** — button in tree toolbar or context menu. Creates a new string property with auto-generated name.
- **Delete property** — button in detail panel footer.
- **Duplicate property** — button in detail panel footer. Copies node with suffixed name.
- **Reorder** — drag-and-drop within same parent level.
- **Change type** — dropdown in detail panel. Changing type resets type-specific constraints but preserves name, title, description.
- **Add composition keyword** — context menu on any node to wrap it in oneOf/anyOf/allOf or add if/then/else.
- **Add $defs entry** — button in tree toolbar. Creates a reusable definition.
- **Convert to $ref** — context menu to extract a node into `$defs` and replace with `$ref`.

## Form Mode (mode="form")

Replaces the current `schemaForm.js` Alpine component. Same rendering logic, ported to Lit:

- Renders input fields based on schema type and constraints.
- Supports `$ref`, `allOf`, `anyOf`, `oneOf`, `if/then/else`, `enum`, `const`.
- Handles nested objects, arrays, additional properties.
- Validates constraints client-side (min/max, pattern, required).
- Outputs data as JSON to a hidden `<input>` for form submission.
- Emits `value-change` events on data modification.

### Migration from schemaForm.js

The form rendering logic in `schemaForm.js` (~1000 lines) moves into `form-mode.ts`. Key changes:

- DOM manipulation (`createElement`, `innerHTML`) → Lit `html` templates with reactive rendering.
- Alpine.js `$refs` and reactive data → Lit reactive properties and `@event` handlers.
- Output mechanism stays the same: hidden `<input>` with `name` attribute containing JSON string.

### Integration with Group/Resource Create Forms

The template include changes from:
```html
<div x-data="schemaForm({schema: currentSchema, value: ..., name: 'Meta'})">
  <div x-ref="container"></div>
</div>
```
to:
```html
<schema-editor
  mode="form"
  :schema="currentSchema"
  value='{{ group.Meta|json }}'
  name="Meta"
></schema-editor>
```

The Alpine `x-data` wrapper for schema form switching (in `createGroup.tpl`) continues to manage the toggle between schema-enforced and free-form modes, now showing/hiding the `<schema-editor>` component instead of the Alpine component.

## Search Mode (mode="search")

Replaces the current `schemaSearchFields.js` Alpine component. Same functionality, ported to Lit:

- Flattens schema into searchable field descriptors via `flattenSchema()`.
- Renders filter inputs with operator selection (EQ, LI, NE, GT, GE, LT, LE).
- Enum fields → checkboxes (≤6 values) or multi-select (>6 values).
- Boolean fields → three-state radio (Any/Yes/No).
- Outputs `MetaQuery` hidden inputs for form submission.
- Multi-schema intersection via `intersectFields()` when multiple categories selected.
- Emits `schema-fields-claimed` for freeFields coordination.

### Migration from schemaSearchFields.js

The search field rendering logic moves into `search-mode.ts`. Key changes:

- Alpine reactive data → Lit reactive properties.
- Template `x-for` loops → Lit `repeat()` directive.
- `@multiple-input.window` event listener → component listens for attribute changes on `schema` (host page updates attribute when category selection changes).
- Hidden input generation → same mechanism, rendered in Lit templates.

### Integration with List Pages

The template include changes from:
```html
<div x-data="schemaSearchFields({...})" @multiple-input.window="...">
```
to:
```html
<schema-editor
  mode="search"
  x-bind:schema="currentSchemas"
  :meta-query='{{ existingMetaQuery|json }}'
  field-name="MetaQuery"
></schema-editor>
```

The Alpine wrapper on the list page still manages the category autocompleter events. When categories are selected, it collects their `MetaSchema` strings into a JSON array and passes them via the `schema` attribute. The component's search mode handles multi-schema intersection internally — when `schema` is a JSON array of schema strings, it flattens each and runs `intersectFields()` to show only common fields. When `schema` is a single schema string, it uses it directly.

### Multi-Schema Search Attribute

| `schema` value | Behavior |
|----------------|----------|
| Single JSON Schema string | Flatten and render fields directly |
| JSON array of schema strings | Flatten each, intersect, render common fields |
| Empty string / `"[]"` / `"null"` | Clear fields, release claimed paths |

## Modal Integration

### Category / ResourceCategory Edit Forms

The existing `MetaSchema` textarea remains the actual form field. A "Visual Editor" button next to it opens a modal containing:

**Modal header:** Title ("Meta JSON Schema") + three tabs (Edit Schema, Preview Form, Raw JSON) + close button.

**Modal body:**
- **Edit Schema tab:** `<schema-editor mode="edit">` with the tree + detail panel.
- **Preview Form tab:** `<schema-editor mode="form">` rendering a live preview of the form the schema would produce. Uses sample/default data.
- **Raw JSON tab:** A `<textarea>` showing the full JSON Schema. Editable — changes sync bidirectionally with the visual editor.

**Modal footer:** Property/required count summary, Cancel button, "Apply Schema" button.

**Apply Schema** reads the current schema from the editor and writes it into the MetaSchema textarea. The modal is implemented as a thin Alpine.js wrapper that manages open/close state and coordinates between the textarea and the component.

### Modal Sizing

The modal should be large — roughly 90% viewport width and 80% viewport height — to give the tree and detail panel enough room. Responsive: on smaller screens, the tree panel collapses to an icon bar or stacks above the detail panel.

## Testing

### Standalone Test Page

`src/schema-editor/test.html` — a plain HTML file that loads the component via Vite dev server. Contains:

- All three modes with sample schemas.
- Controls to switch schemas, modify attributes, observe events.
- No Go server or Alpine required.

### E2E Tests

Playwright tests covering:

- **Edit mode:** Create a schema from scratch, add properties of each type, set constraints, verify generated JSON.
- **Edit mode:** Load an existing complex schema, modify properties, verify round-trip fidelity.
- **Form mode:** Render a schema, fill in data, verify hidden input value.
- **Search mode:** Select categories, verify filter fields render, fill values, verify MetaQuery hidden inputs.
- **Modal integration:** Open editor from category form, build schema, apply, verify textarea updated.
- **Tab switching:** Edit → Preview → Raw JSON, verify consistency.
- **Accessibility:** axe-core scans on all three modes.

### Unit Tests

TypeScript unit tests for `schema-core.ts`:

- `schemaToTree()` / `treeToSchema()` round-trip for all node types.
- `resolveRef()`, `mergeSchemas()`, `resolveSchema()` — existing behavior preserved.
- `flattenSchema()`, `intersectFields()` — existing behavior preserved.
- `detectDraft()` — correct draft detection from `$schema` keyword.

## Accessibility

- Tree panel: `role="tree"` with `role="treeitem"` nodes, arrow key navigation, expand/collapse with Enter/Space.
- Detail panel: proper `<label>` associations, `aria-describedby` for descriptions, `aria-required` for required fields.
- Modal: focus trap, Escape to close, `role="dialog"`, `aria-modal="true"`, focus returns to trigger button on close.
- Form mode: inherits current accessibility from `schemaForm.js` (labels, aria-required, error announcements).
- Search mode: inherits current accessibility from `schemaSearchFields.js` (fieldsets, legends, aria-live).
- All modes: keyboard-operable, no mouse-only interactions.

## Migration Strategy

The migration is incremental — both old and new implementations can coexist during transition:

1. **Phase 1:** Build `<schema-editor>` with edit mode. Add modal to category/resource-type forms. Old schemaForm and schemaSearchFields continue working.
2. **Phase 2:** Port form mode. Replace `schemaForm.js` Alpine component with `<schema-editor mode="form">` in `createGroup.tpl` and `createResource.tpl`.
3. **Phase 3:** Port search mode. Replace `schemaSearchFields.js` Alpine component with `<schema-editor mode="search">` in list page templates.
4. **Phase 4:** Remove old `schemaForm.js` and `schemaSearchFields.js`. Remove duplicated utilities from those files.

Each phase is independently deployable and testable.

## Shadow DOM and Form Submission

Lit components use Shadow DOM by default, which means `<input>` elements inside the shadow root are not included in the parent `<form>` submission. The component handles this by using **slotted light DOM inputs** for form-participating elements:

- **Form mode:** Renders a hidden `<input name="Meta">` in the light DOM (outside shadow root) via Lit's `createRenderRoot()` override or by appending to `this` directly. The shadow DOM contains the visible form UI; the light DOM input holds the serialized JSON value.
- **Search mode:** Renders hidden `<input name="MetaQuery">` elements in the light DOM for each active filter. Same pattern — visible UI in shadow, form-participating inputs in light DOM.
- **Edit mode:** No form submission needed — the modal reads the schema via `getSchema()` method and writes it to the external textarea.

This pattern is standard for form-associated custom elements and avoids the need for the `ElementInternals` API (which has limited browser support for form participation).

## Dependencies

**New npm packages:**
- `lit` — web component library (~5KB gzipped)

**No new Go dependencies.**

**Vite config:** No changes needed — Vite handles `.ts` files and Lit imports out of the box.
