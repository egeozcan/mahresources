# Schema-Driven Metadata Display Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Render category-schema-aware metadata as a beautiful read-only panel on group and resource detail views, just below the description.

**Architecture:** A new `<schema-display-mode>` Lit web component renders in light DOM (inheriting Tailwind styles) inside the existing `<schema-editor>` with `mode="display"`. It walks the JSON Schema, flattens nested properties with dot notation, classifies fields as short/long, and renders a smart hybrid layout — short scalars in a responsive grid, long values in full-width rows below. Templates pass the MetaSchema and Meta value via data attributes.

**Tech Stack:** Lit 3 (web components), Tailwind CSS, Pongo2 templates (Go), Playwright E2E tests

**Spec:** `docs/superpowers/specs/2026-04-04-schema-metadata-display.md`

---

### Task 1: Write E2E Tests for Schema Metadata Display

**Files:**
- Create: `e2e/tests/schema-metadata-display.spec.ts`

- [ ] **Step 1: Write failing E2E tests**

```typescript
/**
 * E2E tests for schema-driven metadata display on detail views.
 *
 * Tests that when a category has a MetaSchema and the entity has Meta data,
 * a structured metadata panel appears below the description on detail pages.
 */
import { test, expect } from '../fixtures/base.fixture';

test.describe('Schema metadata display on group detail', () => {
  let categoryId: number;
  let groupId: number;

  const schema = JSON.stringify({
    type: 'object',
    properties: {
      name: { type: 'string', title: 'Full Name', description: 'Legal name of the person' },
      age: { type: 'integer', title: 'Age' },
      status: { type: 'string', enum: ['active', 'inactive', 'pending'] },
      bio: { type: 'string', title: 'Biography' },
      email: { type: 'string', format: 'email', title: 'Email' },
      website: { type: 'string', format: 'uri', title: 'Website' },
      active: { type: 'boolean', title: 'Is Active' },
    },
  });

  const meta = JSON.stringify({
    name: 'Jane Doe',
    age: 30,
    status: 'active',
    bio: 'A photographer and content creator based in Berlin, known for urban landscape photography and creative visual storytelling across multiple platforms.',
    email: 'jane@example.com',
    website: 'https://janedoe.com',
    active: true,
  });

  test.beforeAll(async ({ apiClient }) => {
    const cat = await apiClient.createCategory(
      `Display Test ${Date.now()}`,
      'Category for metadata display tests',
      { MetaSchema: schema },
    );
    categoryId = cat.ID;
    const group = await apiClient.createGroup({
      name: `Display Group ${Date.now()}`,
      categoryId: cat.ID,
      meta,
    });
    groupId = group.ID;
  });

  test.afterAll(async ({ apiClient }) => {
    if (groupId) await apiClient.deleteGroup(groupId);
    if (categoryId) await apiClient.deleteCategory(categoryId);
  });

  test('renders metadata panel below description', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    await page.waitForLoadState('load');

    // The schema-editor in display mode should be visible
    const displayEditor = page.locator('schema-editor[mode="display"]');
    await expect(displayEditor).toBeVisible({ timeout: 5000 });

    // Panel should have header with "METADATA" text
    await expect(displayEditor).toContainText('Metadata');
  });

  test('shows field values from meta data', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    await page.waitForLoadState('load');

    const displayEditor = page.locator('schema-editor[mode="display"]');
    await expect(displayEditor).toBeVisible({ timeout: 5000 });

    // Short fields should be visible
    await expect(displayEditor).toContainText('Jane Doe');
    await expect(displayEditor).toContainText('30');
    await expect(displayEditor).toContainText('active');
  });

  test('uses schema title as label with description tooltip', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    await page.waitForLoadState('load');

    const displayEditor = page.locator('schema-editor[mode="display"]');
    await expect(displayEditor).toBeVisible({ timeout: 5000 });

    // Should use schema title "Full Name", not raw key "name"
    await expect(displayEditor).toContainText('Full Name');

    // Description should be in a title attribute for tooltip
    const label = displayEditor.locator('[title="Legal name of the person"]');
    await expect(label).toBeVisible();
  });

  test('renders long text in full-width row', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    await page.waitForLoadState('load');

    const displayEditor = page.locator('schema-editor[mode="display"]');
    await expect(displayEditor).toBeVisible({ timeout: 5000 });

    // Bio is long text — should be present and visible
    await expect(displayEditor).toContainText('photographer and content creator');
  });

  test('renders email as mailto link', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    await page.waitForLoadState('load');

    const displayEditor = page.locator('schema-editor[mode="display"]');
    const emailLink = displayEditor.locator('a[href="mailto:jane@example.com"]');
    await expect(emailLink).toBeVisible({ timeout: 5000 });
  });

  test('renders URI as clickable link', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    await page.waitForLoadState('load');

    const displayEditor = page.locator('schema-editor[mode="display"]');
    const uriLink = displayEditor.locator('a[href="https://janedoe.com"]');
    await expect(uriLink).toBeVisible({ timeout: 5000 });
  });

  test('renders boolean as Yes/No', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    await page.waitForLoadState('load');

    const displayEditor = page.locator('schema-editor[mode="display"]');
    await expect(displayEditor).toContainText('Yes');
  });
});

test.describe('Schema metadata display — empty/missing meta', () => {
  test('no panel when category has schema but group has no meta', async ({ page, apiClient }) => {
    const cat = await apiClient.createCategory(
      `Empty Meta Test ${Date.now()}`,
      'Category with schema but no meta data',
      { MetaSchema: JSON.stringify({ type: 'object', properties: { x: { type: 'string' } } }) },
    );
    const group = await apiClient.createGroup({
      name: `No Meta Group ${Date.now()}`,
      categoryId: cat.ID,
    });

    try {
      await page.goto(`/group?id=${group.ID}`);
      await page.waitForLoadState('load');

      const displayEditor = page.locator('schema-editor[mode="display"]');
      await expect(displayEditor).not.toBeVisible({ timeout: 3000 });
    } finally {
      await apiClient.deleteGroup(group.ID);
      await apiClient.deleteCategory(cat.ID);
    }
  });

  test('no panel when category has no schema', async ({ page, apiClient }) => {
    const cat = await apiClient.createCategory(
      `No Schema Test ${Date.now()}`,
      'Category without MetaSchema',
    );
    const group = await apiClient.createGroup({
      name: `No Schema Group ${Date.now()}`,
      categoryId: cat.ID,
      meta: JSON.stringify({ foo: 'bar' }),
    });

    try {
      await page.goto(`/group?id=${group.ID}`);
      await page.waitForLoadState('load');

      const displayEditor = page.locator('schema-editor[mode="display"]');
      await expect(displayEditor).not.toBeVisible({ timeout: 3000 });
    } finally {
      await apiClient.deleteGroup(group.ID);
      await apiClient.deleteCategory(cat.ID);
    }
  });
});

test.describe('Schema metadata display — show/hide empty fields', () => {
  test('hides empty fields by default and shows them on toggle', async ({ page, apiClient }) => {
    const schema = JSON.stringify({
      type: 'object',
      properties: {
        filled: { type: 'string', title: 'Filled Field' },
        empty: { type: 'string', title: 'Empty Field' },
      },
    });
    const cat = await apiClient.createCategory(
      `Toggle Test ${Date.now()}`,
      'Testing show/hide toggle',
      { MetaSchema: schema },
    );
    const group = await apiClient.createGroup({
      name: `Toggle Group ${Date.now()}`,
      categoryId: cat.ID,
      meta: JSON.stringify({ filled: 'has value' }),
    });

    try {
      await page.goto(`/group?id=${group.ID}`);
      await page.waitForLoadState('load');

      const displayEditor = page.locator('schema-editor[mode="display"]');
      await expect(displayEditor).toBeVisible({ timeout: 5000 });

      // Filled field should be visible
      await expect(displayEditor).toContainText('has value');

      // Empty field should NOT be visible by default
      await expect(displayEditor).not.toContainText('Empty Field');

      // Toggle button should show count
      const toggleBtn = displayEditor.locator('button', { hasText: /hidden field/ });
      await expect(toggleBtn).toBeVisible();
      await toggleBtn.click();

      // Now empty field should be visible with em-dash
      await expect(displayEditor).toContainText('Empty Field');
      await expect(displayEditor).toContainText('—');
    } finally {
      await apiClient.deleteGroup(group.ID);
      await apiClient.deleteCategory(cat.ID);
    }
  });
});

test.describe('Schema metadata display — nested objects', () => {
  test('flattens nested objects with dot notation', async ({ page, apiClient }) => {
    const schema = JSON.stringify({
      type: 'object',
      properties: {
        name: { type: 'string' },
        address: {
          type: 'object',
          properties: {
            city: { type: 'string', title: 'City' },
            zip: { type: 'string', title: 'ZIP Code' },
          },
        },
      },
    });
    const cat = await apiClient.createCategory(
      `Nested Test ${Date.now()}`,
      'Testing nested object display',
      { MetaSchema: schema },
    );
    const group = await apiClient.createGroup({
      name: `Nested Group ${Date.now()}`,
      categoryId: cat.ID,
      meta: JSON.stringify({ name: 'Alice', address: { city: 'Berlin', zip: '10115' } }),
    });

    try {
      await page.goto(`/group?id=${group.ID}`);
      await page.waitForLoadState('load');

      const displayEditor = page.locator('schema-editor[mode="display"]');
      await expect(displayEditor).toBeVisible({ timeout: 5000 });

      // Nested fields should be visible with their titles
      await expect(displayEditor).toContainText('Berlin');
      await expect(displayEditor).toContainText('10115');
      await expect(displayEditor).toContainText('City');
    } finally {
      await apiClient.deleteGroup(group.ID);
      await apiClient.deleteCategory(cat.ID);
    }
  });
});
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd e2e && npx playwright test tests/schema-metadata-display.spec.ts --reporter=list --retries=0`
Expected: All tests FAIL — `schema-editor[mode="display"]` doesn't exist yet.

- [ ] **Step 3: Commit test file**

```bash
git add e2e/tests/schema-metadata-display.spec.ts
git commit -m "test(e2e): add failing tests for schema metadata display"
```

---

### Task 2: Create `<schema-display-mode>` Component

**Files:**
- Create: `src/schema-editor/modes/display-mode.ts`

- [ ] **Step 1: Create the display-mode component**

This is the core component. It receives a parsed schema and value object, flattens properties, classifies them as short/long, and renders the smart hybrid layout.

```typescript
import { LitElement, html, nothing, type TemplateResult } from 'lit';
import { customElement, property, state } from 'lit/decorators.js';
import type { JSONSchema } from '../schema-core';
import {
  resolveRef,
  isLabeledEnum,
  getLabeledEnumEntries,
  titleCase,
} from '../schema-core';

/** Resolved schema — follows $ref chains and merges allOf. */
function resolveSchema(schema: JSONSchema, root: JSONSchema): JSONSchema | null {
  if (!schema) return null;
  if (schema.$ref) {
    const resolved = resolveRef(schema.$ref, root);
    return resolved ? resolveSchema(resolved, root) : null;
  }
  return schema;
}

interface DisplayField {
  path: string;       // dot-notation key, e.g. "address.city"
  label: string;      // schema title or titleCase(key)
  description: string; // schema description (for tooltip)
  type: string;       // resolved scalar type
  format: string;     // schema format (uri, email, date, etc.)
  value: any;         // actual value from Meta
  isEmpty: boolean;   // true if null/undefined/empty string
  isLong: boolean;    // true if should render full-width
  enum: any[] | null;
  enumLabels: string[] | null;
}

function getNestedValue(obj: any, path: string): any {
  const parts = path.split('.');
  let current = obj;
  for (const part of parts) {
    if (current == null || typeof current !== 'object') return undefined;
    current = current[part];
  }
  return current;
}

function isEmptyValue(val: any): boolean {
  if (val === null || val === undefined) return true;
  if (typeof val === 'string' && val.trim() === '') return true;
  return false;
}

const LONG_STRING_THRESHOLD = 80;

function classifyAsLong(field: DisplayField): boolean {
  if (field.type === 'array') return true;
  if (typeof field.value === 'string' && field.value.length > LONG_STRING_THRESHOLD) return true;
  return false;
}

function flattenForDisplay(
  schema: JSONSchema,
  value: any,
  root: JSONSchema,
  prefix = '',
  labelPrefix = '',
  depth = 0,
): DisplayField[] {
  if (depth > 3 || !schema) return [];
  const resolved = resolveSchema(schema, root);
  if (!resolved?.properties) return [];

  const fields: DisplayField[] = [];

  for (const [key, rawProp] of Object.entries(resolved.properties) as [string, JSONSchema][]) {
    const path = prefix ? `${prefix}.${key}` : key;
    const prop = resolveSchema(rawProp, root) || rawProp;
    const rawLabel = prop.title || titleCase(key);
    const label = labelPrefix ? `${labelPrefix} \u203A ${rawLabel}` : rawLabel;
    const description = prop.description || '';
    const format = prop.format || '';
    const val = getNestedValue(value, path);

    // Nested object with properties — flatten recursively
    if (prop.properties) {
      fields.push(...flattenForDisplay(prop, value, root, path, label, depth + 1));
      continue;
    }

    // Determine type
    let fieldType = prop.type || 'string';
    if (Array.isArray(fieldType)) {
      fieldType = fieldType.find((t: string) => t !== 'null') || 'string';
    }

    // Labeled enum detection
    let enumValues: any[] | null = null;
    let enumLabels: string[] | null = null;
    if (isLabeledEnum(prop)) {
      const entries = getLabeledEnumEntries(prop);
      enumValues = entries.map(e => e.value);
      enumLabels = entries.map(e => e.label);
    } else if (Array.isArray(prop.enum)) {
      enumValues = prop.enum;
    }

    const field: DisplayField = {
      path, label, description, type: fieldType, format,
      value: val,
      isEmpty: isEmptyValue(val),
      isLong: false,
      enum: enumValues,
      enumLabels,
    };
    field.isLong = classifyAsLong(field);
    fields.push(field);
  }

  return fields;
}

@customElement('schema-display-mode')
export class SchemaDisplayMode extends LitElement {
  @property({ type: Object }) schema: JSONSchema = {};
  @property({ type: Object }) value: any = {};
  @property({ type: String }) name = '';

  @state() private _showEmpty = false;

  // Light DOM to inherit Tailwind styles
  override createRenderRoot() {
    return this;
  }

  override render() {
    if (!this.schema?.properties || !this.value) return nothing;

    const allFields = flattenForDisplay(this.schema, this.value, this.schema);
    const filledFields = allFields.filter(f => !f.isEmpty);
    const emptyFields = allFields.filter(f => f.isEmpty);
    const visibleFields = this._showEmpty ? allFields : filledFields;
    const shortFields = visibleFields.filter(f => !f.isLong);
    const longFields = visibleFields.filter(f => f.isLong);

    if (filledFields.length === 0 && !this._showEmpty) return nothing;

    return html`
      <div class="detail-panel mb-6" aria-label="Schema metadata">
        <div class="detail-panel-header" style="background: #fafaf9;">
          <h2 class="detail-panel-title">Metadata</h2>
          ${this.name ? html`<span class="text-xs font-mono text-stone-400">${this.name}</span>` : nothing}
        </div>
        <div class="detail-panel-body" style="padding: 1rem;">
          ${shortFields.length > 0 ? html`
            <div class="grid gap-4" style="grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));">
              ${shortFields.map(f => this._renderShortField(f))}
            </div>
          ` : nothing}
          ${longFields.length > 0 ? html`
            <div class="${shortFields.length > 0 ? 'mt-4 pt-4 border-t border-stone-100' : ''}">
              ${longFields.map(f => this._renderLongField(f))}
            </div>
          ` : nothing}
          ${emptyFields.length > 0 ? html`
            <div class="mt-3 pt-3 border-t border-stone-100">
              <button
                class="text-xs font-mono text-stone-400 hover:text-stone-600 cursor-pointer bg-transparent border-none p-0"
                @click=${() => { this._showEmpty = !this._showEmpty; }}
              >${this._showEmpty
                ? 'Hide empty fields'
                : `Show ${emptyFields.length} hidden field${emptyFields.length !== 1 ? 's' : ''}`
              }</button>
            </div>
          ` : nothing}
        </div>
      </div>
    `;
  }

  private _renderShortField(field: DisplayField): TemplateResult {
    return html`
      <div class="group relative cursor-pointer"
        @click=${() => this._copyValue(field.value)}>
        <div class="text-[10px] font-mono uppercase text-stone-400 tracking-wider mb-1"
          style="letter-spacing: 0.05em;"
          title=${field.description || nothing}
        >${field.label}</div>
        <div class="text-sm text-stone-900">${this._renderValue(field)}</div>
      </div>
    `;
  }

  private _renderLongField(field: DisplayField): TemplateResult {
    return html`
      <div class="mb-3 last:mb-0 cursor-pointer"
        @click=${() => this._copyValue(field.value)}>
        <div class="text-[10px] font-mono uppercase text-stone-400 tracking-wider mb-1"
          style="letter-spacing: 0.05em;"
          title=${field.description || nothing}
        >${field.label}</div>
        <div class="text-sm text-stone-900">${this._renderValue(field)}</div>
      </div>
    `;
  }

  private _renderValue(field: DisplayField): TemplateResult | string {
    if (field.isEmpty) {
      return html`<span class="text-stone-300">—</span>`;
    }

    const val = field.value;

    // Enum with labels — pill with tooltip
    if (field.enumLabels && field.enum) {
      const idx = field.enum.indexOf(val);
      const label = idx >= 0 && field.enumLabels[idx] ? field.enumLabels[idx] : String(val);
      const tooltip = idx >= 0 && field.enumLabels[idx] ? String(val) : '';
      return html`<span
        class="inline-block text-xs px-2.5 py-0.5 rounded-full bg-indigo-50 text-indigo-700 font-medium"
        title=${tooltip || nothing}
      >${label}</span>`;
    }

    // Plain enum — pill
    if (field.enum) {
      return html`<span
        class="inline-block text-xs px-2.5 py-0.5 rounded-full bg-emerald-50 text-emerald-700 font-medium"
      >${String(val)}</span>`;
    }

    // Boolean
    if (field.type === 'boolean') {
      return val ? 'Yes' : 'No';
    }

    // Number / integer
    if (field.type === 'number' || field.type === 'integer') {
      return html`<span class="font-mono">${String(val)}</span>`;
    }

    // String with format
    if (typeof val === 'string') {
      if (field.format === 'uri' || field.format === 'url') {
        return html`<a href=${val} target="_blank" rel="noopener noreferrer"
          class="text-indigo-600 hover:text-indigo-800 underline decoration-indigo-300"
          @click=${(e: Event) => e.stopPropagation()}
        >${val}</a>`;
      }
      if (field.format === 'email') {
        return html`<a href="mailto:${val}"
          class="text-indigo-600 hover:text-indigo-800 underline decoration-indigo-300"
          @click=${(e: Event) => e.stopPropagation()}
        >${val}</a>`;
      }
      if (field.format === 'date' || field.format === 'date-time') {
        try {
          const d = new Date(val);
          return d.toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' });
        } catch {
          return val;
        }
      }
    }

    // Array of scalars
    if (field.type === 'array' && Array.isArray(val)) {
      if (val.length === 0) return html`<span class="text-stone-300">—</span>`;
      const allScalar = val.every(v => typeof v !== 'object' || v === null);
      if (allScalar) {
        return html`${val.map((v, i) => html`<span
          class="inline-block text-xs px-2 py-0.5 rounded-full bg-stone-100 text-stone-600 font-medium mr-1 mb-1"
        >${String(v)}</span>`)}`;
      }
      // Array of objects — compact JSON
      return html`<pre class="text-xs font-mono text-stone-600 bg-stone-50 p-2 rounded overflow-x-auto">${JSON.stringify(val, null, 2)}</pre>`;
    }

    // Default — plain string
    if (field.isLong && typeof val === 'string') {
      return html`<span style="white-space: pre-wrap;">${val}</span>`;
    }

    return String(val ?? '');
  }

  private _copyValue(val: any) {
    if (val === null || val === undefined) return;
    const text = typeof val === 'object' ? JSON.stringify(val) : String(val);
    navigator.clipboard?.writeText(text).catch(() => {});
  }
}
```

- [ ] **Step 2: Verify the file compiles**

Run: `cd /Users/egecan/Code/mahresources && npx vite build 2>&1 | tail -5`
Expected: Build succeeds (component is created but not yet imported).

- [ ] **Step 3: Commit**

```bash
git add src/schema-editor/modes/display-mode.ts
git commit -m "feat(schema-editor): add schema-display-mode component"
```

---

### Task 3: Register Display Mode in schema-editor.ts

**Files:**
- Modify: `src/schema-editor/schema-editor.ts`

- [ ] **Step 1: Add the import and mode type**

In `src/schema-editor/schema-editor.ts`, add the import for the new mode at the top (after the other mode imports):

```typescript
import './modes/display-mode';
```

Change the mode property type to include `'display'`:

```typescript
@property({ type: String }) mode: 'edit' | 'form' | 'search' | 'display' = 'edit';
```

- [ ] **Step 2: Add the display case to the render switch**

In the `render()` method's switch statement, add a new case after `case 'form'`:

```typescript
      case 'display': {
        let parsedValue = {};
        try { parsedValue = this.value ? JSON.parse(this.value) : {}; } catch { /* invalid JSON */ }
        return html`<schema-display-mode
          .schema=${this._parsedSchema}
          .value=${parsedValue}
          .name=${this.name}
        ></schema-display-mode>`;
      }
```

- [ ] **Step 3: Verify build succeeds**

Run: `cd /Users/egecan/Code/mahresources && npx vite build 2>&1 | tail -5`
Expected: Build succeeds.

- [ ] **Step 4: Commit**

```bash
git add src/schema-editor/schema-editor.ts
git commit -m "feat(schema-editor): register display mode in schema-editor"
```

---

### Task 4: Integrate into Group Detail Template

**Files:**
- Modify: `templates/displayGroup.tpl`

- [ ] **Step 1: Add schema-editor display block**

In `templates/displayGroup.tpl`, insert the following block **after line 11** (the description include) and **before line 13** (the `{% with %}` statement):

```html
    {% if group.Category.MetaSchema && group.Meta %}
    <schema-editor mode="display"
        schema='{{ group.Category.MetaSchema }}'
        value='{{ group.Meta }}'
        name="{{ group.Category.Name }}">
    </schema-editor>
    {% endif %}
```

The full context around the insertion should look like:

```html
    {% include "/partials/description.tpl" with description=group.Description descriptionEditUrl="/v1/group/editDescription" descriptionEditId=group.ID %}

    {% if group.Category.MetaSchema && group.Meta %}
    <schema-editor mode="display"
        schema='{{ group.Category.MetaSchema }}'
        value='{{ group.Meta }}'
        name="{{ group.Category.Name }}">
    </schema-editor>
    {% endif %}

    {% with hasOwn=(group.OwnNotes || group.OwnGroups || group.OwnResources) %}
```

- [ ] **Step 2: Commit**

```bash
git add templates/displayGroup.tpl
git commit -m "feat(templates): add schema metadata display to group detail view"
```

---

### Task 5: Integrate into Resource Detail Template

**Files:**
- Modify: `templates/displayResource.tpl`

- [ ] **Step 1: Add schema-editor display block**

In `templates/displayResource.tpl`, insert the following block **after line 11** (the description include) and **before line 13** (the existing "Metadata" detail panel):

```html
    {% if resource.ResourceCategory.MetaSchema && resource.Meta %}
    <schema-editor mode="display"
        schema='{{ resource.ResourceCategory.MetaSchema }}'
        value='{{ resource.Meta }}'
        name="{{ resource.ResourceCategory.Name }}">
    </schema-editor>
    {% endif %}
```

The full context around the insertion should look like:

```html
    {% include "/partials/description.tpl" with description=resource.Description descriptionEditUrl="/v1/resource/editDescription" descriptionEditId=resource.ID %}

    {% if resource.ResourceCategory.MetaSchema && resource.Meta %}
    <schema-editor mode="display"
        schema='{{ resource.ResourceCategory.MetaSchema }}'
        value='{{ resource.Meta }}'
        name="{{ resource.ResourceCategory.Name }}">
    </schema-editor>
    {% endif %}

    <div class="detail-panel" aria-label="Resource metadata">
```

- [ ] **Step 2: Commit**

```bash
git add templates/displayResource.tpl
git commit -m "feat(templates): add schema metadata display to resource detail view"
```

---

### Task 6: Build, Run E2E Tests, Fix Issues

**Files:**
- Possibly modify: `src/schema-editor/modes/display-mode.ts`, `templates/displayGroup.tpl`, `templates/displayResource.tpl`

- [ ] **Step 1: Build the JS bundle**

Run: `cd /Users/egecan/Code/mahresources && npx vite build 2>&1 | tail -5`
Expected: Build succeeds.

- [ ] **Step 2: Run the new E2E tests**

Run: `cd e2e && npx playwright test tests/schema-metadata-display.spec.ts --reporter=list --retries=0`
Expected: All tests pass. If any fail, investigate the failure, fix the code, and re-run.

- [ ] **Step 3: Run the full E2E suite to check for regressions**

Run: `cd e2e && npm run test:with-server:all`
Expected: All existing tests still pass alongside the new ones.

- [ ] **Step 4: Run Go unit tests**

Run: `go test --tags 'json1 fts5' ./...`
Expected: All pass (templates are Go-rendered but no Go code changed).

- [ ] **Step 5: Commit built assets and any fixes**

```bash
git add public/dist/
git commit -m "build: rebuild JS bundle with schema display mode"
```
