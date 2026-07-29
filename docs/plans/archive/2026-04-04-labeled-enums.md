# Labeled Enums Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Support labeled enums in the JSON Schema editor using the standard `oneOf` + `const` + `title` pattern, with conversion from plain enums and labeled rendering in forms and search.

**Architecture:** Add an `isLabeledEnum()` detection helper to schema-core.ts. Extend the enum editor to show value+label pairs and emit `oneOf`/`const`/`title` schemas. Add a detection gate in form-mode and search-mode that renders labeled dropdowns/checkboxes when the pattern is detected. Extend `flattenSchema` to extract labeled enum info for search mode.

**Tech Stack:** Lit, TypeScript, existing schema-editor web component system.

---

### Task 1: Add `isLabeledEnum` helper to schema-core.ts

**Files:**
- Modify: `src/schema-editor/schema-core.ts`

The helper detects schemas where `oneOf` is an array of simple `{ const, title?, description? }` entries — no complex subschemas. This function is used by the enum editor, form mode, search mode, and `flattenSchema`.

- [ ] **Step 1: Add `isLabeledEnum` function**

Add after the `titleCase` function (line 656) in `src/schema-editor/schema-core.ts`:

```typescript
// ─── Labeled enum detection ─────────────────────────────────────────────────

/**
 * Returns true when a schema represents a "labeled enum" — a `oneOf` array
 * where every entry is a simple `{ const, title?, description? }` object
 * with no complex subschema keywords.
 */
export function isLabeledEnum(schema: JSONSchema): boolean {
  if (!schema.oneOf || !Array.isArray(schema.oneOf) || schema.oneOf.length === 0) return false;
  const complexKeys = new Set(['type', 'properties', 'items', 'oneOf', 'anyOf', 'allOf', 'if', '$ref', 'enum']);
  return schema.oneOf.every((entry: JSONSchema) => {
    if (!entry || typeof entry !== 'object' || entry.const === undefined) return false;
    return !Object.keys(entry).some(k => complexKeys.has(k));
  });
}

/**
 * Given a labeled-enum schema, returns the label for a specific value.
 * Falls back to stringifying the value if no title is found.
 */
export function getLabeledEnumTitle(schema: JSONSchema, value: any): string {
  if (!schema.oneOf) return String(value);
  const entry = schema.oneOf.find((e: JSONSchema) => e.const === value);
  return entry?.title || String(value);
}

/**
 * Extracts the enum values and labels from a labeled-enum schema.
 * Returns an array of { value, label } objects.
 */
export function getLabeledEnumEntries(schema: JSONSchema): Array<{ value: any; label: string }> {
  if (!schema.oneOf) return [];
  return schema.oneOf.map((entry: JSONSchema) => ({
    value: entry.const,
    label: entry.title || String(entry.const),
  }));
}
```

- [ ] **Step 2: Export `isLabeledEnum` and helpers**

The functions are already exported inline via `export function`. Verify by checking that the file has no separate export block that would need updating. (It doesn't — all functions in schema-core.ts use inline `export`.)

- [ ] **Step 3: Build to verify no TypeScript errors**

Run: `cd /Users/egecan/Code/mahresources && npm run build-js`
Expected: Build succeeds with no errors.

- [ ] **Step 4: Commit**

```bash
git add src/schema-editor/schema-core.ts
git commit -m "feat: add isLabeledEnum helper for oneOf+const+title detection"
```

---

### Task 2: Extend `flattenSchema` to handle labeled enums in search mode

**Files:**
- Modify: `src/schema-editor/schema-core.ts` (the `flattenSchema` function, lines 660-715)

Currently `flattenSchema` skips properties with `oneOf` because they have nested `properties` (complex objects). For labeled enums, we need to detect the pattern and emit a `FlatField` with enum values and labels.

- [ ] **Step 1: Update `FlatField` interface to include labels**

At the top of `src/schema-editor/schema-core.ts`, modify the `FlatField` interface (line 3-8):

```typescript
export interface FlatField {
  path: string;
  label: string;
  type: string;
  enum: string[] | null;
  /** Optional labels for enum values (parallel array with `enum`). null if no labels. */
  enumLabels: string[] | null;
}
```

- [ ] **Step 2: Update `flattenSchema` to detect labeled enums**

In the `flattenSchema` function, after resolving `prop` (line 677), add labeled enum detection before the existing `if (prop.properties)` check. Modify lines 681-711:

```typescript
    // Labeled enum: oneOf with const+title entries
    if (isLabeledEnum(prop)) {
      const entries = getLabeledEnumEntries(prop);
      let fieldType = prop.type || 'string';
      if (Array.isArray(fieldType)) {
        fieldType = fieldType.find((t: string) => t !== 'null') || 'string';
      }
      // Infer type from const values if no explicit type
      if (!prop.type && entries.length > 0) {
        const representative = entries.find(e => e.value !== null)?.value ?? entries[0].value;
        if (representative === null) fieldType = 'null';
        else if (typeof representative === 'number') fieldType = Number.isInteger(representative) ? 'integer' : 'number';
        else if (typeof representative === 'boolean') fieldType = 'boolean';
      }
      fields.push({
        path,
        label,
        type: fieldType,
        enum: entries.map(e => e.value),
        enumLabels: entries.map(e => e.label),
      });
      continue;
    }

    if (prop.properties) {
```

- [ ] **Step 3: Add `enumLabels: null` to the existing non-labeled enum field push**

In the existing `fields.push` call (around line 705), add `enumLabels: null`:

```typescript
      fields.push({
        path,
        label,
        type: fieldType,
        enum: Array.isArray(prop.enum) ? prop.enum : null,
        enumLabels: null,
      });
```

- [ ] **Step 4: Update `intersectFields` to handle `enumLabels`**

In the `intersectFields` function, when creating the base map (line 720), `enumLabels` is already spread via `{ ...f }`. But when enums get nulled out due to mismatch, also null out labels. After line 753 (`existing.enum = null;`), add:

```typescript
                existing.enumLabels = null;
```

And in the `else if` branch (line 755, `existing.enum !== field.enum`):

```typescript
        existing.enumLabels = null;
```

These two lines go inside the existing conditions that already null out `existing.enum`.

- [ ] **Step 5: Build to verify no TypeScript errors**

Run: `cd /Users/egecan/Code/mahresources && npm run build-js`
Expected: Build succeeds with no errors.

- [ ] **Step 6: Commit**

```bash
git add src/schema-editor/schema-core.ts
git commit -m "feat: flattenSchema extracts labeled enum values and labels"
```

---

### Task 3: Extend enum editor with label column and conversion

**Files:**
- Modify: `src/schema-editor/tree/node-editors/enum-editor.ts`
- Modify: `src/schema-editor/tree/detail-panel.ts`
- Modify: `src/schema-editor/modes/edit-mode.ts`

The enum editor gets a label column. When any label is non-empty, the detail panel emits the `oneOf`+`const`+`title` schema. The detail panel also handles rendering the labeled enum editor when it detects the pattern in an incoming schema.

- [ ] **Step 1: Add labels support to enum-editor.ts**

Replace the entire content of `src/schema-editor/tree/node-editors/enum-editor.ts`:

```typescript
import { LitElement, html, css } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import { repeat } from 'lit/directives/repeat.js';
import { sharedStyles } from '../../styles';

export interface EnumEntry {
  value: any;
  label: string;
}

@customElement('schema-enum-editor')
export class SchemaEnumEditor extends LitElement {
  static override styles = [sharedStyles, css`
    .enum-row { display: flex; align-items: center; gap: 6px; margin-bottom: 6px; }
    .enum-row input { flex: 1; }
    .drag { color: #9ca3af; cursor: grab; font-size: 10px; }
    .remove { color: #dc2626; background: none; border: none; font-size: 14px; padding: 0 4px; }
    .label-header { display: grid; grid-template-columns: 20px 1fr 1fr 24px; gap: 6px; margin-bottom: 4px; font-size: 11px; color: #6b7280; font-weight: 600; }
    .labeled-row { display: grid; grid-template-columns: 20px 1fr 1fr 24px; gap: 6px; margin-bottom: 6px; align-items: center; }
    .convert-btn { font-size: 12px; color: #4f46e5; background: none; border: 1px solid #c7d2fe; border-radius: 4px; padding: 4px 10px; cursor: pointer; margin-bottom: 8px; }
    .convert-btn:hover { background: #eef2ff; }
  `];

  /** Plain enum values (when no labels) */
  @property({ type: Array }) values: any[] = [];
  /** Labeled entries (when labels are present) */
  @property({ type: Array }) entries: EnumEntry[] = [];
  @property({ type: String }) valueType = 'string';
  /** Whether the editor is in labeled mode */
  @property({ type: Boolean }) labeled = false;

  private _emit() {
    if (this.labeled) {
      this.dispatchEvent(new CustomEvent('enum-change', {
        detail: { labeled: true, entries: [...this.entries] },
        bubbles: true, composed: true,
      }));
    } else {
      this.dispatchEvent(new CustomEvent('enum-change', {
        detail: { labeled: false, values: [...this.values] },
        bubbles: true, composed: true,
      }));
    }
  }

  private _parseValue(raw: string): any {
    if (this.valueType === 'number' || this.valueType === 'integer') {
      return this.valueType === 'integer' ? parseInt(raw, 10) : parseFloat(raw);
    }
    if (this.valueType === 'boolean') return raw === 'true';
    return raw;
  }

  private _defaultValue(): any {
    if (this.valueType === 'number' || this.valueType === 'integer') return 0;
    if (this.valueType === 'boolean') return false;
    return '';
  }

  // ─── Plain enum (no labels) ──────────────────────────────────────────────

  private _updateValue(index: number, raw: string) {
    const updated = [...this.values];
    updated[index] = this._parseValue(raw);
    this.values = updated;
    this._emit();
  }

  private _removeValue(index: number) {
    this.values = this.values.filter((_, i) => i !== index);
    this._emit();
    this.requestUpdate();
  }

  private _addValue() {
    this.values = [...this.values, this._defaultValue()];
    this._emit();
    this.requestUpdate();
  }

  // ─── Labeled enum ────────────────────────────────────────────────────────

  private _updateEntryValue(index: number, raw: string) {
    const updated = [...this.entries];
    updated[index] = { ...updated[index], value: this._parseValue(raw) };
    this.entries = updated;
    this._emit();
  }

  private _updateEntryLabel(index: number, label: string) {
    const updated = [...this.entries];
    updated[index] = { ...updated[index], label };
    this.entries = updated;
    this._emit();
  }

  private _removeEntry(index: number) {
    this.entries = this.entries.filter((_, i) => i !== index);
    if (this.entries.length === 0) {
      this.labeled = false;
      this.values = [];
    }
    this._emit();
    this.requestUpdate();
  }

  private _addEntry() {
    this.entries = [...this.entries, { value: this._defaultValue(), label: '' }];
    this._emit();
    this.requestUpdate();
  }

  // ─── Conversion ──────────────────────────────────────────────────────────

  private _convertToLabeled() {
    this.entries = this.values.map(v => ({ value: v, label: '' }));
    this.labeled = true;
    this.values = [];
    this._emit();
    this.requestUpdate();
  }

  private _convertToPlain() {
    this.values = this.entries.map(e => e.value);
    this.labeled = false;
    this.entries = [];
    this._emit();
    this.requestUpdate();
  }

  // ─── Render ──────────────────────────────────────────────────────────────

  override render() {
    if (this.labeled) return this._renderLabeled();
    return this._renderPlain();
  }

  private _renderPlain() {
    return html`
      <div class="type-section">
        <h4>Enum Values</h4>
        <button class="convert-btn" @click=${this._convertToLabeled} title="Convert to labeled enum with display names">+ Add Labels</button>
        ${repeat(this.values, (_v, i) => i, (v, i) => html`
          <div class="enum-row">
            <span class="drag" aria-hidden="true">\u2630</span>
            ${this.valueType === 'boolean'
              ? html`<select
                  .value=${String(v)}
                  @change=${(e: Event) => this._updateValue(i, (e.target as HTMLSelectElement).value)}
                  aria-label="Enum value ${i + 1}"
                >
                  <option value="true" ?selected=${v === true}>true</option>
                  <option value="false" ?selected=${v === false}>false</option>
                </select>`
              : html`<input
                  .value=${String(v)}
                  type=${this.valueType === 'number' || this.valueType === 'integer' ? 'number' : 'text'}
                  step=${this.valueType === 'integer' ? '1' : 'any'}
                  @change=${(e: Event) => this._updateValue(i, (e.target as HTMLInputElement).value)}
                  aria-label="Enum value ${i + 1}"
                >`}
            <button class="remove" @click=${() => this._removeValue(i)} aria-label="Remove value ${v}">\u00d7</button>
          </div>
        `)}
        <button class="btn-ghost" @click=${this._addValue}>+ Add Value</button>
      </div>
    `;
  }

  private _renderLabeled() {
    return html`
      <div class="type-section">
        <h4>Enum Values</h4>
        <button class="convert-btn" @click=${this._convertToPlain} title="Remove labels and convert back to plain enum">Remove Labels</button>
        <div class="label-header">
          <span></span>
          <span>Value</span>
          <span>Label</span>
          <span></span>
        </div>
        ${repeat(this.entries, (_e, i) => i, (entry, i) => html`
          <div class="labeled-row">
            <span class="drag" aria-hidden="true">\u2630</span>
            ${this.valueType === 'boolean'
              ? html`<select
                  .value=${String(entry.value)}
                  @change=${(e: Event) => this._updateEntryValue(i, (e.target as HTMLSelectElement).value)}
                  aria-label="Enum value ${i + 1}"
                >
                  <option value="true" ?selected=${entry.value === true}>true</option>
                  <option value="false" ?selected=${entry.value === false}>false</option>
                </select>`
              : html`<input
                  .value=${String(entry.value)}
                  type=${this.valueType === 'number' || this.valueType === 'integer' ? 'number' : 'text'}
                  step=${this.valueType === 'integer' ? '1' : 'any'}
                  @change=${(e: Event) => this._updateEntryValue(i, (e.target as HTMLInputElement).value)}
                  aria-label="Enum value ${i + 1}"
                >`}
            <input
              .value=${entry.label}
              type="text"
              placeholder="Display label"
              @change=${(e: Event) => this._updateEntryLabel(i, (e.target as HTMLInputElement).value)}
              aria-label="Label for value ${entry.value}">
            <button class="remove" @click=${() => this._removeEntry(i)} aria-label="Remove value ${entry.value}">\u00d7</button>
          </div>
        `)}
        <button class="btn-ghost" @click=${this._addEntry}>+ Add Value</button>
      </div>
    `;
  }
}
```

- [ ] **Step 2: Update detail-panel.ts to handle labeled enum schemas**

In `src/schema-editor/tree/detail-panel.ts`, add the import at the top (after line 4):

```typescript
import { isLabeledEnum } from '../schema-core';
import type { EnumEntry } from './node-editors/enum-editor';
```

Then modify `_renderTypeEditor()` (lines 118-141). Replace the existing enum check and add labeled enum detection:

```typescript
  private _renderTypeEditor() {
    if (!this.node) return nothing;
    const schema = this.node.schema;

    // Labeled enum: oneOf with const+title entries
    if (isLabeledEnum(schema)) {
      const entries: EnumEntry[] = (schema.oneOf as any[]).map((e: any) => ({
        value: e.const,
        label: e.title || '',
      }));
      return html`<schema-enum-editor
        .entries=${entries}
        .labeled=${true}
        .valueType=${this.node.type}
        @enum-change=${(e: CustomEvent) => {
          if (e.detail.labeled) {
            this._dispatchChange('labeledEnum', e.detail.entries);
          } else {
            this._dispatchChange('enum', e.detail.values);
          }
        }}
      ></schema-enum-editor>`;
    }

    // Plain enum editor (any type can have enum)
    if (schema.enum) {
      return html`<schema-enum-editor
        .values=${schema.enum}
        .valueType=${this.node.type}
        @enum-change=${(e: CustomEvent) => {
          if (e.detail.labeled) {
            this._dispatchChange('labeledEnum', e.detail.entries);
          } else {
            this._dispatchChange('enum', e.detail.values);
          }
        }}
      ></schema-enum-editor>`;
    }

    switch (this.node.type) {
      case 'string':
        return html`<schema-string-editor .schema=${schema} @constraint-change=${(e: CustomEvent) => this._dispatchChange(e.detail.field, e.detail.value)}></schema-string-editor>`;
      case 'number':
      case 'integer':
        return html`<schema-number-editor .schema=${schema} .integerOnly=${this.node.type === 'integer'} @constraint-change=${(e: CustomEvent) => this._dispatchChange(e.detail.field, e.detail.value)}></schema-number-editor>`;
      case 'boolean':
        return html`<schema-boolean-editor .schema=${schema} @constraint-change=${(e: CustomEvent) => this._dispatchChange(e.detail.field, e.detail.value)}></schema-boolean-editor>`;
      case 'object':
        return html`<schema-object-editor .schema=${schema} @constraint-change=${(e: CustomEvent) => this._dispatchChange(e.detail.field, e.detail.value)}></schema-object-editor>`;
      case 'array':
        return html`<schema-array-editor .schema=${schema} @constraint-change=${(e: CustomEvent) => this._dispatchChange(e.detail.field, e.detail.value)}></schema-array-editor>`;
      default:
        return nothing;
    }
  }
```

- [ ] **Step 3: Handle `labeledEnum` in edit-mode.ts `_handleNodeChange`**

In `src/schema-editor/modes/edit-mode.ts`, add a new case after the `'enum'` case (after line 213). Find the switch statement that handles `node-change` events and add:

```typescript
      case 'labeledEnum': {
        // Convert entries to oneOf + const + title schema
        const entries = value as Array<{ value: any; label: string }>;
        if (entries.length === 0) {
          delete selected.schema.oneOf;
          delete selected.schema.enum;
        } else {
          delete selected.schema.enum;
          selected.schema.oneOf = entries.map(e => {
            const entry: any = { const: e.value };
            if (e.label) entry.title = e.label;
            return entry;
          });
        }
        // Ensure this node is NOT treated as a composition node
        // (labeledEnum oneOf lives in schema, not in compositionKeyword/variants)
        break;
      }
```

Also update the `'enum'` case to clean up any leftover `oneOf` from a previous labeled state:

```typescript
      case 'enum':
        if (Array.isArray(value) && value.length === 0) {
          delete selected.schema.enum;
        } else {
          selected.schema.enum = value;
        }
        // Clean up oneOf from previous labeled enum state
        if (selected.schema.oneOf && !selected.compositionKeyword) {
          delete selected.schema.oneOf;
        }
        break;
```

- [ ] **Step 4: Prevent `schemaToTree` from treating labeled enum oneOf as composition**

In `src/schema-editor/schema-tree-model.ts`, the loop at lines 116-126 extracts `oneOf` into `compositionKeyword`/`variants`. We need to skip this when the schema is a labeled enum. Add the import and guard:

At the top of `src/schema-editor/schema-tree-model.ts`, add:

```typescript
import { isLabeledEnum } from './schema-core';
```

Then modify the composition keyword loop (lines 116-126). Wrap it to skip labeled enums:

```typescript
  // oneOf / anyOf / allOf → composition node with variant children (stored in `variants`)
  // Only extract the FIRST matching keyword into compositionKeyword/variants.
  // Additional composition keywords stay in node.schema and pass through
  // treeToSchema via the spread, preserving multi-keyword schemas.
  // Skip labeled enums — their oneOf is not a composition, it's a value list.
  if (!isLabeledEnum(schema)) {
    for (const kw of ['oneOf', 'anyOf', 'allOf'] as const) {
      if (Array.isArray(schema[kw])) {
        node.compositionKeyword = kw;
        const variantNodes = (schema[kw] as JSONSchema[]).map((variant, i) =>
          schemaToTree(variant, variant.title || `variant${i + 1}`),
        );
        node.variants = [...(node.variants || []), ...variantNodes];
        delete node.schema[kw];
        break;
      }
    }
  }
```

- [ ] **Step 5: Build to verify no TypeScript errors**

Run: `cd /Users/egecan/Code/mahresources && npm run build-js`
Expected: Build succeeds with no errors.

- [ ] **Step 6: Commit**

```bash
git add src/schema-editor/tree/node-editors/enum-editor.ts src/schema-editor/tree/detail-panel.ts src/schema-editor/modes/edit-mode.ts src/schema-editor/schema-tree-model.ts
git commit -m "feat: enum editor with label column and oneOf+const+title storage"
```

---

### Task 4: Render labeled enums in form mode

**Files:**
- Modify: `src/schema-editor/modes/form-mode.ts`
- Modify: `src/schema-editor/form-mode-helpers.ts`

Form mode needs to detect labeled enums and render them as `<select>` with label text instead of raw values. The key change is in `_renderField`: detect labeled enum before the existing `oneOf` handler, and render a specialized dropdown.

- [ ] **Step 1: Add `isLabeledEnum` import to form-mode.ts**

At the top of `src/schema-editor/modes/form-mode.ts`, add to the import from `schema-core` (line 4-12):

```typescript
import {
  resolveRef,
  mergeSchemas,
  getDefaultValue,
  scoreSchemaMatch,
  evaluateCondition,
  inferType,
  inferSchema,
  isLabeledEnum,
  getLabeledEnumEntries,
} from '../schema-core';
```

- [ ] **Step 2: Add labeled enum detection before oneOf handler in `_renderField`**

In `_renderField` (line 144), add the labeled enum check just before the `if (schema.oneOf)` check at line 158. Insert between the `$ref` handler (lines 146-155) and the `oneOf` handler:

```typescript
    // Handle labeled enum (oneOf with const+title)
    if (isLabeledEnum(schema)) {
      return this._renderLabeledEnum(schema, data, onChange, fieldId, describedBy, isRequired);
    }

    // Handle oneOf
```

- [ ] **Step 3: Add `_renderLabeledEnum` method**

Add after the existing `_renderEnum` method (after line 464):

```typescript
  // ─── labeled enum (oneOf + const + title) ──────────────────────────────

  private _renderLabeledEnum(schema: JSONSchema, data: any, onChange: (val: any) => void, fieldId?: string, describedBy?: string | null, isRequired?: boolean): TemplateResult {
    const entries = getLabeledEnumEntries(schema);
    const hasValue = entries.some(e => e.value === data);
    const isNull = data === null || data === undefined;

    const onSelectChange = (e: Event) => {
      const valStr = (e.target as HTMLSelectElement).value;
      // Find matching entry by stringified value
      const match = entries.find(entry => String(entry.value) === valStr);
      if (match !== undefined) {
        onChange(match.value);
      } else if (schema.type === 'integer' || schema.type === 'number') {
        onChange(parseFloat(valStr));
      } else {
        onChange(valStr);
      }
    };

    return html`
      <select class="shadow-sm focus:ring-indigo-500 focus:border-indigo-500 block w-full sm:text-sm border-gray-300 rounded-md mt-1"
        id=${fieldId || nothing}
        aria-label=${schema.title ? `Select ${schema.title}` : 'Select value'}
        aria-describedby=${describedBy || nothing}
        ?required=${!!isRequired}
        aria-required=${isRequired ? 'true' : nothing}
        @change=${onSelectChange}>
        ${isNull ? html`<option value="" selected>-- select --</option>` : nothing}
        ${entries.map(entry => html`
          <option value=${entry.value} ?selected=${entry.value === data}>${entry.label}</option>
        `)}
        ${!isNull && !hasValue ? html`
          <option value=${data} selected>${data} (current)</option>
        ` : nothing}
      </select>
    `;
  }
```

- [ ] **Step 4: Update `isLeafSchema` in form-mode-helpers.ts to treat labeled enums as leaves**

In `src/schema-editor/form-mode-helpers.ts`, add the import (line 1):

```typescript
import type { JSONSchema } from './schema-core';
import { resolveRef, mergeSchemas, unescapeJsonPointer, isLabeledEnum } from './schema-core';
```

Then add a check before the existing `oneOf` check at line 31:

```typescript
  // Labeled enum (oneOf with const+title) renders as a simple dropdown — it's a leaf
  if (isLabeledEnum(schema)) return true;

  // oneOf / anyOf render variant selectors with nested sub-forms — never a leaf
```

- [ ] **Step 5: Build to verify no TypeScript errors**

Run: `cd /Users/egecan/Code/mahresources && npm run build-js`
Expected: Build succeeds with no errors.

- [ ] **Step 6: Commit**

```bash
git add src/schema-editor/modes/form-mode.ts src/schema-editor/form-mode-helpers.ts
git commit -m "feat: render labeled enums as dropdowns with labels in form mode"
```

---

### Task 5: Render labeled enums in search mode

**Files:**
- Modify: `src/schema-editor/modes/search-mode.ts`

Search mode reads `FlatField` data from `flattenSchema`. Since Task 2 already emits `enumLabels` in `FlatField`, search mode just needs to use them when rendering checkboxes and multi-selects.

- [ ] **Step 1: Update `SearchField` to include `enumLabels`**

In `src/schema-editor/modes/search-mode.ts`, add `enumLabels` to the `SearchField` interface (after line 33):

```typescript
interface SearchField extends FlatField {
  operator: string;
  value: string;
  enumValues: string[];
  boolValue: string;
  showOperator: boolean;
  operators: Operator[] | null;
  enumLabels: string[] | null;
}
```

Note: `enumLabels` is already inherited from `FlatField` via the `extends`, but it's good to be explicit. Actually, since `SearchField extends FlatField` and `FlatField` already has `enumLabels`, this is already inherited. No change needed to the interface itself — just make sure the field is passed through in `_rebuildFields`.

The `_rebuildFields` method uses `{ ...field }` spread when creating `SearchField` objects (line 231), so `enumLabels` is automatically included. No code change needed here.

- [ ] **Step 2: Update `_renderEnumCheckboxes` to show labels**

Modify `_renderEnumCheckboxes` (lines 472-488) to use `enumLabels` when available:

```typescript
  private _renderEnumCheckboxes(field: SearchField, ariaLabel: string): TemplateResult {
    return html`
      <fieldset class="w-full" aria-label=${ariaLabel}>
        <legend class="block text-xs font-mono font-medium text-stone-600 mt-1">${field.label}</legend>
        <div class="flex flex-wrap gap-x-3 gap-y-1 mt-1">
          ${field.enum!.map((enumVal, idx) => {
            const displayLabel = field.enumLabels?.[idx] || String(enumVal);
            return html`
              <label class="text-sm flex items-center gap-1">
                <input type="checkbox" .value=${String(enumVal)}
                       .checked=${field.enumValues.includes(String(enumVal))}
                       @change=${(e: Event) => this._onEnumCheckboxChange(field, String(enumVal), (e.target as HTMLInputElement).checked)}>
                <span>${displayLabel}</span>
              </label>
            `;
          })}
        </div>
      </fieldset>
    `;
  }
```

- [ ] **Step 3: Update `_renderEnumSelect` to show labels**

Modify `_renderEnumSelect` (lines 490-512) to use `enumLabels` when available:

```typescript
  private _renderEnumSelect(field: SearchField, ariaLabel: string): TemplateResult {
    const selectSize = Math.min(field.enum!.length, 6);
    return html`
      <fieldset class="w-full" aria-label=${ariaLabel}>
        <legend class="block text-xs font-mono font-medium text-stone-600 mt-1">${field.label}</legend>
        <select multiple
                class="w-full text-sm border-stone-300 rounded mt-1 focus:ring-1 focus:ring-amber-600 focus:border-amber-600"
                size=${selectSize}
                @change=${(e: Event) => this._onEnumSelectChange(field, e)}>
          ${field.enum!.map((enumVal, idx) => {
            const displayLabel = field.enumLabels?.[idx] || String(enumVal);
            return html`
              <option value=${String(enumVal)} ?selected=${field.enumValues.includes(String(enumVal))}>${displayLabel}</option>
            `;
          })}
        </select>
        ${field.enumValues.length > 0 ? html`
          <button type="button"
                  class="text-xs text-stone-400 hover:text-amber-700 mt-1 underline"
                  aria-label="Clear ${field.label} selection"
                  @click=${() => { field.enumValues = []; this.requestUpdate(); }}>
            Clear selection
          </button>
        ` : nothing}
      </fieldset>
    `;
  }
```

- [ ] **Step 4: Build to verify no TypeScript errors**

Run: `cd /Users/egecan/Code/mahresources && npm run build-js`
Expected: Build succeeds with no errors.

- [ ] **Step 5: Commit**

```bash
git add src/schema-editor/modes/search-mode.ts
git commit -m "feat: render labeled enum labels in search mode checkboxes and selects"
```

---

### Task 6: Handle `getDefaultValue` for labeled enums

**Files:**
- Modify: `src/schema-editor/schema-core.ts`

Currently `getDefaultValue` for `oneOf` returns `getDefaultValue(schema.oneOf[0], rootSchema)` (line 634). For labeled enums, it should return the first `const` value directly, not try to resolve a complex schema.

- [ ] **Step 1: Add labeled enum check to `getDefaultValue`**

In `src/schema-editor/schema-core.ts`, find the existing `oneOf` handler in `getDefaultValue` at line 634:

```typescript
  if (schema.oneOf && schema.oneOf.length > 0) return getDefaultValue(schema.oneOf[0], rootSchema);
```

Replace it with:

```typescript
  if (schema.oneOf && schema.oneOf.length > 0) {
    if (isLabeledEnum(schema)) return schema.oneOf[0].const;
    return getDefaultValue(schema.oneOf[0], rootSchema);
  }
```

- [ ] **Step 2: Build to verify no TypeScript errors**

Run: `cd /Users/egecan/Code/mahresources && npm run build-js`
Expected: Build succeeds with no errors.

- [ ] **Step 3: Commit**

```bash
git add src/schema-editor/schema-core.ts
git commit -m "fix: getDefaultValue returns const value for labeled enums"
```

---

### Task 7: Full build, manual verification, and E2E tests

**Files:**
- No new files

- [ ] **Step 1: Full build**

Run: `cd /Users/egecan/Code/mahresources && npm run build`
Expected: CSS, JS, and Go binary all build successfully.

- [ ] **Step 2: Run Go unit tests**

Run: `cd /Users/egecan/Code/mahresources && go test --tags 'json1 fts5' ./...`
Expected: All tests pass (no Go code was changed).

- [ ] **Step 3: Run E2E tests (browser + CLI)**

Run: `cd /Users/egecan/Code/mahresources/e2e && npm run test:with-server:all`
Expected: All existing tests pass. The schema editor tests should still pass since we didn't break any existing functionality.

- [ ] **Step 4: Manual verification in browser**

Start an ephemeral server: `cd /Users/egecan/Code/mahresources && ./mahresources -ephemeral -bind-address=:8181`

Verify:
1. Open schema editor for a category
2. Add an integer property with enum values 0, 1, 2
3. Click "Add Labels" button — values are preserved, label fields appear
4. Fill in labels (Inactive, Active, Hidden)
5. Switch to Form tab — dropdown shows "Inactive", "Active", "Hidden" instead of 0, 1, 2
6. Switch to Search tab — checkboxes show labels instead of raw numbers
7. Switch back to Edit tab — labeled enum is preserved
8. Click "Remove Labels" — reverts to plain enum
9. Save and reload — labeled enum persists in the JSON

- [ ] **Step 5: Run Postgres E2E tests**

Run: `cd /Users/egecan/Code/mahresources/e2e && npm run test:with-server:postgres`
Expected: All tests pass.

- [ ] **Step 6: Final commit if any cleanup needed**

Only if manual testing reveals issues that need fixing.
