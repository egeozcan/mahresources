# Schema-Driven Search Fields Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Dynamically render type-appropriate search fields in list view sidebars based on selected category MetaSchema definitions, with multi-category intersection and smart operator defaults.

**Architecture:** A new Alpine.js component (`schemaSearchFields`) listens for category selection events, recursively flattens JSON Schema definitions into searchable fields, intersects multiple schemas, and renders type-appropriate inputs that serialize as standard `MetaQuery` URL parameters. Zero backend changes — the existing `ParseMeta`/`JSONQueryExpression` pipeline handles everything.

**Tech Stack:** Alpine.js, Pongo2 templates, Playwright (E2E tests)

---

### Task 1: Schema Flattening and Intersection Logic

**Files:**
- Create: `src/components/schemaSearchFields.js`
- Modify: `src/main.js:30,89-122`

This task builds the pure-logic core: schema flattening, multi-schema intersection, and label generation. No DOM rendering yet.

- [ ] **Step 1: Create `schemaSearchFields.js` with flattening logic**

Create `src/components/schemaSearchFields.js`:

```javascript
import { generateParamNameForMeta } from './freeFields.js';

/**
 * Recursively flatten a JSON Schema into a list of searchable field descriptors.
 *
 * @param {object} schema - Parsed JSON Schema object
 * @param {string} prefix - Dot-separated path prefix for nested fields
 * @param {string} labelPrefix - Human-readable label prefix (e.g., "Dimensions › ")
 * @returns {Array<{path: string, label: string, type: string, enum: string[]|null}>}
 */
export function flattenSchema(schema, prefix = '', labelPrefix = '') {
  if (!schema || schema.type !== 'object' || !schema.properties) {
    return [];
  }

  const fields = [];

  for (const [key, prop] of Object.entries(schema.properties)) {
    const path = prefix ? `${prefix}.${key}` : key;
    const rawLabel = prop.title || titleCase(key);
    const label = labelPrefix ? `${labelPrefix} › ${rawLabel}` : rawLabel;

    if (prop.type === 'object' && prop.properties) {
      // Recurse into nested objects
      fields.push(...flattenSchema(prop, path, label));
    } else if (prop.type === 'array') {
      // Skip arrays — not meaningful for search
      continue;
    } else {
      fields.push({
        path,
        label,
        type: prop.type || 'string',
        enum: Array.isArray(prop.enum) ? prop.enum : null,
      });
    }
  }

  return fields;
}

/**
 * Intersect multiple flattened field lists. Keep only fields present in ALL lists.
 * Type conflicts fall back to "string". Enum conflicts drop the enum.
 *
 * @param {Array<Array<{path: string, label: string, type: string, enum: string[]|null}>>} fieldLists
 * @returns {Array<{path: string, label: string, type: string, enum: string[]|null}>}
 */
export function intersectFields(fieldLists) {
  if (fieldLists.length === 0) return [];
  if (fieldLists.length === 1) return fieldLists[0];

  // Index first list by path
  const base = new Map(fieldLists[0].map(f => [f.path, { ...f }]));

  // Intersect with each subsequent list
  for (let i = 1; i < fieldLists.length; i++) {
    const currentPaths = new Set(fieldLists[i].map(f => f.path));

    // Remove paths not in current list
    for (const path of base.keys()) {
      if (!currentPaths.has(path)) {
        base.delete(path);
      }
    }

    // Merge types/enums for remaining paths
    for (const field of fieldLists[i]) {
      const existing = base.get(field.path);
      if (!existing) continue;

      if (existing.type !== field.type) {
        existing.type = 'string';
        existing.enum = null;
      } else if (existing.enum && field.enum) {
        if (JSON.stringify(existing.enum) !== JSON.stringify(field.enum)) {
          existing.enum = null;
        }
      } else if (existing.enum !== field.enum) {
        existing.enum = null;
      }
    }
  }

  return Array.from(base.values());
}

/**
 * Title-case a camelCase or snake_case key.
 * "birthDate" → "Birth Date", "first_name" → "First Name"
 */
function titleCase(key) {
  return key
    .replace(/([a-z])([A-Z])/g, '$1 $2')  // camelCase → spaced
    .replace(/[_-]/g, ' ')                  // snake_case → spaced
    .replace(/\b\w/g, c => c.toUpperCase()); // capitalize words
}
```

- [ ] **Step 2: Register the component in `main.js`**

In `src/main.js`, add the import after line 26 (the `freeFields` import):

```javascript
import { schemaSearchFields } from './components/schemaSearchFields.js';
```

Add the Alpine.data registration after line 95 (the `schemaForm` registration):

```javascript
Alpine.data('schemaSearchFields', schemaSearchFields);
```

- [ ] **Step 3: Build the JS bundle and verify no errors**

Run: `npm run build-js`
Expected: Clean build with no errors.

- [ ] **Step 4: Commit**

```bash
git add src/components/schemaSearchFields.js src/main.js
git commit -m "feat: add schema flattening and intersection logic for search fields"
```

---

### Task 2: Alpine.js Component — State, Event Handling, and Serialization

**Files:**
- Modify: `src/components/schemaSearchFields.js`

This task adds the Alpine.js component function that manages state: listening for category selection events, computing fields, tracking user input values, and serializing to MetaQuery params.

- [ ] **Step 1: Add the Alpine.js component function**

Append to the end of `src/components/schemaSearchFields.js`:

```javascript
/**
 * Default operator for a given field type.
 */
function defaultOperator(field) {
  if (field.enum) return 'EQ';
  if (field.type === 'boolean') return 'EQ';
  if (field.type === 'string') return 'LI';
  return 'EQ'; // number, integer
}

/**
 * Available operators for a given field type (for the override dropdown).
 * Boolean and enum fields return null (no override allowed).
 */
function operatorsForType(field) {
  if (field.enum || field.type === 'boolean') return null;
  if (field.type === 'string') {
    return [
      { code: 'LI', label: 'LIKE' },
      { code: 'EQ', label: '=' },
      { code: 'NE', label: '≠' },
      { code: 'NL', label: 'NOT LIKE' },
    ];
  }
  // number, integer
  return [
    { code: 'EQ', label: '=' },
    { code: 'NE', label: '≠' },
    { code: 'GT', label: '>' },
    { code: 'GE', label: '≥' },
    { code: 'LT', label: '<' },
    { code: 'LE', label: '≤' },
  ];
}

/**
 * Operator display symbol for the collapsed state.
 */
function operatorSymbol(code) {
  const symbols = { EQ: '=', NE: '≠', LI: '≈', NL: '≉', GT: '>', GE: '≥', LT: '<', LE: '≤' };
  return symbols[code] || code;
}

/**
 * Alpine.js data component for schema-driven search fields.
 *
 * @param {object} opts
 * @param {string} opts.elName - The autocompleter element name to listen for (e.g., 'categories')
 * @param {Array} opts.existingMetaQuery - Pre-parsed MetaQuery from URL (parsedQuery.MetaQuery)
 * @param {string} opts.id - Unique ID prefix for form elements
 */
export function schemaSearchFields({ elName, existingMetaQuery, id }) {
  return {
    elName,
    id,
    /** @type {Array<{path: string, label: string, type: string, enum: string[]|null, operator: string, value: string, enumValues: string[], showOperator: boolean}>} */
    fields: [],
    /** Whether any schema fields are visible */
    hasFields: false,

    init() {
      // Pre-fill from URL if MetaQuery params exist
      this._existingMeta = existingMetaQuery || [];
    },

    /**
     * Handle category selection changes. Called via @multiple-input.window.
     */
    handleCategoryChange(items) {
      const schemas = items
        .filter(item => item.MetaSchema)
        .map(item => {
          try { return JSON.parse(item.MetaSchema); }
          catch { return null; }
        })
        .filter(Boolean);

      if (schemas.length === 0) {
        this.fields = [];
        this.hasFields = false;
        return;
      }

      const fieldLists = schemas.map(s => flattenSchema(s));
      const merged = schemas.length === 1 ? fieldLists[0] : intersectFields(fieldLists);

      this.fields = merged.map(field => {
        const op = defaultOperator(field);
        const existing = this._findExistingValue(field.path);

        return {
          ...field,
          operator: existing ? existing.operator : op,
          value: existing ? existing.value : '',
          // For enum fields: track which values are checked (array of strings)
          enumValues: existing ? existing.enumValues : [],
          // For boolean: 'any', 'true', 'false'
          boolValue: existing ? existing.boolValue : 'any',
          showOperator: false,
          operators: operatorsForType(field),
        };
      });

      this.hasFields = this.fields.length > 0;
    },

    /**
     * Look up an existing MetaQuery value for a given path.
     * Returns { operator, value, enumValues, boolValue } or null.
     */
    _findExistingValue(path) {
      const matches = this._existingMeta.filter(m => m.Key === path);
      if (matches.length === 0) return null;

      // Multiple matches for same key → enum multi-select
      if (matches.length > 1) {
        return {
          operator: matches[0].Operation || 'EQ',
          value: '',
          enumValues: matches.map(m => String(m.Value)),
          boolValue: 'any',
        };
      }

      const m = matches[0];
      if (typeof m.Value === 'boolean') {
        return {
          operator: 'EQ',
          value: '',
          enumValues: [],
          boolValue: String(m.Value),
        };
      }

      return {
        operator: m.Operation || 'EQ',
        value: m.Value != null ? String(m.Value) : '',
        enumValues: [],
        boolValue: 'any',
      };
    },

    /**
     * Get the operator display symbol for a field.
     */
    getSymbol(field) {
      return operatorSymbol(field.operator);
    },

    /**
     * Toggle operator dropdown visibility.
     */
    toggleOperator(field) {
      field.showOperator = !field.showOperator;
    },

    /**
     * Generate hidden input entries for a field.
     * Returns an array of {name, value} for hidden inputs.
     */
    getHiddenInputs(field) {
      if (field.type === 'boolean') {
        if (field.boolValue === 'any') return [];
        return [{ value: generateParamNameForMeta({ name: field.path, value: field.boolValue, operation: 'EQ' }) }];
      }

      if (field.enum) {
        return field.enumValues.map(v => ({
          value: generateParamNameForMeta({ name: field.path, value: `"${v}"`, operation: 'EQ' }),
        }));
      }

      if (!field.value && field.value !== 0) return [];

      return [{ value: generateParamNameForMeta({ name: field.path, value: field.value, operation: field.operator }) }];
    },
  };
}
```

- [ ] **Step 2: Build the JS bundle and verify no errors**

Run: `npm run build-js`
Expected: Clean build with no errors.

- [ ] **Step 3: Commit**

```bash
git add src/components/schemaSearchFields.js
git commit -m "feat: add schemaSearchFields Alpine.js component with state management"
```

---

### Task 3: Template Partial — Rendering Schema Fields

**Files:**
- Create: `templates/partials/form/schemaSearchFields.tpl`

This task creates the shared template partial that renders the schema-driven fields. It will be included by both `listGroups.tpl` and `searchFormResource.tpl`.

- [ ] **Step 1: Create `schemaSearchFields.tpl`**

Create `templates/partials/form/schemaSearchFields.tpl`:

```html
<div
    x-data="schemaSearchFields({
        elName: '{{ elName }}',
        existingMetaQuery: {{ existingMetaQuery|json }} || [],
        id: '{{ id }}',
    })"
    @multiple-input.window="if ($event.detail.name === '{{ elName }}') handleCategoryChange($event.detail.value)"
    class="w-full"
    aria-live="polite"
>
    <template x-if="hasFields">
        <div class="flex flex-col gap-2 w-full">
            <template x-for="(field, fIdx) in fields" :key="field.path">
                <div class="w-full">
                    <!-- Hidden inputs for form submission -->
                    <template x-for="(hidden, hIdx) in getHiddenInputs(field)" :key="fIdx + '-h-' + hIdx">
                        <input type="hidden" name="MetaQuery" :value="hidden.value">
                    </template>

                    <!-- Boolean: three-state radio -->
                    <template x-if="field.type === 'boolean'">
                        <fieldset class="w-full">
                            <legend
                                class="block text-xs font-mono font-medium text-stone-600 mt-1"
                                x-text="field.label"
                                :aria-label="field.label.replace(/ › /g, ', ')"
                            ></legend>
                            <div class="flex gap-3 mt-1">
                                <label class="text-sm flex items-center gap-1">
                                    <input type="radio" :name="id + '-bool-' + field.path" value="any"
                                           x-model="field.boolValue">
                                    Any
                                </label>
                                <label class="text-sm flex items-center gap-1">
                                    <input type="radio" :name="id + '-bool-' + field.path" value="true"
                                           x-model="field.boolValue">
                                    Yes
                                </label>
                                <label class="text-sm flex items-center gap-1">
                                    <input type="radio" :name="id + '-bool-' + field.path" value="false"
                                           x-model="field.boolValue">
                                    No
                                </label>
                            </div>
                        </fieldset>
                    </template>

                    <!-- Enum ≤ 6: checkboxes -->
                    <template x-if="field.enum && field.enum.length <= 6">
                        <fieldset class="w-full">
                            <legend
                                class="block text-xs font-mono font-medium text-stone-600 mt-1"
                                x-text="field.label"
                                :aria-label="field.label.replace(/ › /g, ', ')"
                            ></legend>
                            <div class="flex flex-wrap gap-x-3 gap-y-1 mt-1">
                                <template x-for="enumVal in field.enum" :key="enumVal">
                                    <label class="text-sm flex items-center gap-1">
                                        <input type="checkbox" :value="enumVal"
                                               x-model="field.enumValues">
                                        <span x-text="enumVal"></span>
                                    </label>
                                </template>
                            </div>
                        </fieldset>
                    </template>

                    <!-- Enum > 6: multi-select dropdown -->
                    <template x-if="field.enum && field.enum.length > 6">
                        <fieldset class="w-full">
                            <legend
                                class="block text-xs font-mono font-medium text-stone-600 mt-1"
                                x-text="field.label"
                                :aria-label="field.label.replace(/ › /g, ', ')"
                            ></legend>
                            <select multiple
                                    x-model="field.enumValues"
                                    :id="id + '-enum-' + field.path"
                                    :aria-label="field.label.replace(/ › /g, ', ')"
                                    class="w-full text-sm border-stone-300 rounded mt-1 focus:ring-1 focus:ring-amber-600 focus:border-amber-600"
                                    :size="Math.min(field.enum.length, 6)"
                            >
                                <template x-for="enumVal in field.enum" :key="enumVal">
                                    <option :value="enumVal" x-text="enumVal"></option>
                                </template>
                            </select>
                        </fieldset>
                    </template>

                    <!-- String / Number / Integer: text or number input with operator -->
                    <template x-if="!field.enum && field.type !== 'boolean'">
                        <div class="w-full">
                            <label
                                :for="id + '-' + field.path"
                                class="block text-xs font-mono font-medium text-stone-600 mt-1"
                                x-text="field.label"
                                :aria-label="field.label.replace(/ › /g, ', ')"
                            ></label>
                            <div class="flex gap-1 items-center w-full mt-1">
                                <!-- Collapsed operator symbol (clickable) -->
                                <template x-if="!field.showOperator">
                                    <button
                                        type="button"
                                        @click="toggleOperator(field)"
                                        class="text-xs text-stone-400 hover:text-amber-700 underline cursor-pointer flex-shrink-0 w-5 text-center"
                                        :aria-label="'Change operator, currently ' + getSymbol(field)"
                                        :title="'Operator: ' + getSymbol(field)"
                                        x-text="getSymbol(field)"
                                    ></button>
                                </template>
                                <!-- Expanded operator dropdown -->
                                <template x-if="field.showOperator">
                                    <select
                                        x-model="field.operator"
                                        @change="field.showOperator = false"
                                        :aria-label="'Operator for ' + field.label"
                                        class="flex-shrink-0 w-16 text-sm border-stone-300 rounded focus:ring-1 focus:ring-amber-600 focus:border-amber-600"
                                    >
                                        <template x-for="op in field.operators" :key="op.code">
                                            <option :value="op.code" x-text="op.label"></option>
                                        </template>
                                    </select>
                                </template>
                                <input
                                    :type="(field.type === 'number' || field.type === 'integer') ? 'number' : 'text'"
                                    :step="field.type === 'integer' ? '1' : 'any'"
                                    x-model="field.value"
                                    :id="id + '-' + field.path"
                                    :aria-label="field.label.replace(/ › /g, ', ')"
                                    class="flex-grow w-full text-sm border-stone-300 rounded focus:ring-1 focus:ring-amber-600 focus:border-amber-600"
                                >
                            </div>
                        </div>
                    </template>
                </div>
            </template>
        </div>
    </template>
</div>
```

- [ ] **Step 2: Commit**

```bash
git add templates/partials/form/schemaSearchFields.tpl
git commit -m "feat: add schemaSearchFields template partial"
```

---

### Task 4: Integrate into Group and Resource List Views

**Files:**
- Modify: `templates/listGroups.tpl:47-52`
- Modify: `templates/partials/form/searchFormResource.tpl:30-31`

- [ ] **Step 1: Add the schema search fields partial to `listGroups.tpl`**

In `templates/listGroups.tpl`, insert the include between the categories autocompleter (line 47) and the freeFields include (line 52). The `elName` must match the autocompleter's `elName` which is `'categories'`:

Find this in `templates/listGroups.tpl`:

```
            {% include "/partials/form/autocompleter.tpl" with url='/v1/categories' elName='categories' title='Categories' selectedItems=categories id=getNextId("autocompleter") %}
            {% include "/partials/form/autocompleter.tpl" with url='/v1/notes' elName='notes' title='Notes' selectedItems=notes id=getNextId("autocompleter") %}
```

Replace with:

```
            {% include "/partials/form/autocompleter.tpl" with url='/v1/categories' elName='categories' title='Categories' selectedItems=categories id=getNextId("autocompleter") %}
            {% include "/partials/form/schemaSearchFields.tpl" with elName='categories' existingMetaQuery=parsedQuery.MetaQuery id=getNextId("schemaSearch") %}
            {% include "/partials/form/autocompleter.tpl" with url='/v1/notes' elName='notes' title='Notes' selectedItems=notes id=getNextId("autocompleter") %}
```

- [ ] **Step 2: Add the schema search fields partial to `searchFormResource.tpl`**

In `templates/partials/form/searchFormResource.tpl`, insert between the ResourceCategory autocompleter (line 30) and the freeFields include (line 31). The `elName` must match `'ResourceCategoryId'`:

Find this in `templates/partials/form/searchFormResource.tpl`:

```
        {% include "/partials/form/autocompleter.tpl" with url='/v1/resourceCategories' max=1 elName='ResourceCategoryId' title='Resource Category' selectedItems=selectedResourceCategory id=getNextId("autocompleter") %}
        {% include "/partials/form/freeFields.tpl" with name="MetaQuery" url='/v1/resources/meta/keys' fields=parsedQuery.MetaQuery id=getNextId("freeField") %}
```

Replace with:

```
        {% include "/partials/form/autocompleter.tpl" with url='/v1/resourceCategories' max=1 elName='ResourceCategoryId' title='Resource Category' selectedItems=selectedResourceCategory id=getNextId("autocompleter") %}
        {% include "/partials/form/schemaSearchFields.tpl" with elName='ResourceCategoryId' existingMetaQuery=parsedQuery.MetaQuery id=getNextId("schemaSearch") %}
        {% include "/partials/form/freeFields.tpl" with name="MetaQuery" url='/v1/resources/meta/keys' fields=parsedQuery.MetaQuery id=getNextId("freeField") %}
```

- [ ] **Step 3: Build full application and verify**

Run: `npm run build`
Expected: Clean build. No Go compilation errors, no JS bundle errors.

- [ ] **Step 4: Commit**

```bash
git add templates/listGroups.tpl templates/partials/form/searchFormResource.tpl
git commit -m "feat: integrate schema search fields into group and resource list views"
```

---

### Task 5: E2E Tests — Groups Schema Search

**Files:**
- Create: `e2e/tests/schema-search-fields.spec.ts`

- [ ] **Step 1: Create the E2E test file for schema search fields**

Create `e2e/tests/schema-search-fields.spec.ts`:

```typescript
import { test, expect } from '../fixtures/base.fixture';

test.describe('Schema-Driven Search Fields', () => {
  let categoryWithSchemaId: number;
  let categoryNoSchemaId: number;
  let categoryOverlapId: number;
  let groupId: number;
  let resourceCategoryId: number;

  test.beforeAll(async ({ apiClient }) => {
    // Create a category with a MetaSchema
    const cat1 = await apiClient.createCategory('Schema Test Category', 'For search field tests', {
      MetaSchema: JSON.stringify({
        type: 'object',
        properties: {
          color: { type: 'string', enum: ['red', 'green', 'blue'] },
          weight: { type: 'number' },
          active: { type: 'boolean' },
          dimensions: {
            type: 'object',
            properties: {
              width: { type: 'number' },
              height: { type: 'number' },
            },
          },
        },
      }),
    });
    categoryWithSchemaId = cat1.ID;

    // Create a category without a MetaSchema
    const cat2 = await apiClient.createCategory('No Schema Category', 'No schema');
    categoryNoSchemaId = cat2.ID;

    // Create a category with overlapping schema (shares 'color' and 'weight')
    const cat3 = await apiClient.createCategory('Overlap Category', 'Overlapping fields', {
      MetaSchema: JSON.stringify({
        type: 'object',
        properties: {
          color: { type: 'string' },  // same key, no enum this time
          weight: { type: 'number' }, // same key, same type
          material: { type: 'string' },
        },
      }),
    });
    categoryOverlapId = cat3.ID;

    // Create a group with metadata for pre-fill testing
    const group = await apiClient.createGroup({
      name: 'Schema Search Test Group',
      description: 'Group for testing schema search',
      categoryId: categoryWithSchemaId,
      meta: { color: 'blue', weight: 42 },
    });
    groupId = group.ID;

    // Create a resource category with a MetaSchema
    const resCat = await apiClient.createResourceCategory('Schema Res Category', 'For resource search tests', {
      MetaSchema: JSON.stringify({
        type: 'object',
        properties: {
          format: { type: 'string', enum: ['jpg', 'png', 'gif', 'webp'] },
          quality: { type: 'integer' },
        },
      }),
    });
    resourceCategoryId = resCat.ID;
  });

  test.describe('Groups List View', () => {
    test('schema fields appear when selecting a category with MetaSchema', async ({ page, groupPage }) => {
      await groupPage.gotoList();

      // Select the category with schema
      await page.getByRole('group').filter({ hasText: 'Categories' }).getByRole('textbox').fill('Schema Test');
      await page.getByRole('option', { name: 'Schema Test Category' }).click();

      // Verify schema fields appear
      await expect(page.getByLabel('Color')).toBeVisible();
      await expect(page.getByLabel('Weight')).toBeVisible();
      await expect(page.getByText('Active')).toBeVisible();
      await expect(page.getByLabel('Dimensions, Width')).toBeVisible();
      await expect(page.getByLabel('Dimensions, Height')).toBeVisible();
    });

    test('schema fields disappear when deselecting the category', async ({ page, groupPage }) => {
      await groupPage.gotoList();

      // Select then deselect
      await page.getByRole('group').filter({ hasText: 'Categories' }).getByRole('textbox').fill('Schema Test');
      await page.getByRole('option', { name: 'Schema Test Category' }).click();
      await expect(page.getByLabel('Color')).toBeVisible();

      // Remove the selection (click the remove button on the selected item)
      await page.getByRole('button', { name: /remove.*Schema Test/i }).click();

      // Fields should disappear
      await expect(page.getByLabel('Color')).not.toBeVisible();
    });

    test('enum field renders as checkboxes when ≤ 6 values', async ({ page, groupPage }) => {
      await groupPage.gotoList();

      await page.getByRole('group').filter({ hasText: 'Categories' }).getByRole('textbox').fill('Schema Test');
      await page.getByRole('option', { name: 'Schema Test Category' }).click();

      // Color has 3 enum values → should be checkboxes
      const colorFieldset = page.locator('fieldset').filter({ hasText: 'Color' });
      await expect(colorFieldset.getByRole('checkbox', { name: 'red' })).toBeVisible();
      await expect(colorFieldset.getByRole('checkbox', { name: 'green' })).toBeVisible();
      await expect(colorFieldset.getByRole('checkbox', { name: 'blue' })).toBeVisible();
    });

    test('boolean field renders as three-state radio', async ({ page, groupPage }) => {
      await groupPage.gotoList();

      await page.getByRole('group').filter({ hasText: 'Categories' }).getByRole('textbox').fill('Schema Test');
      await page.getByRole('option', { name: 'Schema Test Category' }).click();

      const activeFieldset = page.locator('fieldset').filter({ hasText: 'Active' });
      await expect(activeFieldset.getByRole('radio', { name: 'Any' })).toBeVisible();
      await expect(activeFieldset.getByRole('radio', { name: 'Yes' })).toBeVisible();
      await expect(activeFieldset.getByRole('radio', { name: 'No' })).toBeVisible();
    });

    test('submitting schema fields produces correct MetaQuery URL params', async ({ page, groupPage }) => {
      await groupPage.gotoList();

      await page.getByRole('group').filter({ hasText: 'Categories' }).getByRole('textbox').fill('Schema Test');
      await page.getByRole('option', { name: 'Schema Test Category' }).click();

      // Fill in weight
      await page.getByLabel('Weight').fill('42');

      // Check a color enum
      const colorFieldset = page.locator('fieldset').filter({ hasText: 'Color' });
      await colorFieldset.getByRole('checkbox', { name: 'blue' }).check();

      // Submit the form
      await page.getByRole('button', { name: /search/i }).click();

      // Verify URL params
      const url = new URL(page.url());
      const metaParams = url.searchParams.getAll('MetaQuery');
      expect(metaParams).toContainEqual(expect.stringContaining('weight:EQ:42'));
      expect(metaParams).toContainEqual(expect.stringContaining('color:EQ:"blue"'));
    });

    test('multi-category intersection shows only common fields', async ({ page, groupPage }) => {
      await groupPage.gotoList();

      // Select first category
      await page.getByRole('group').filter({ hasText: 'Categories' }).getByRole('textbox').fill('Schema Test');
      await page.getByRole('option', { name: 'Schema Test Category' }).click();

      // Verify all fields visible
      await expect(page.getByLabel('Weight')).toBeVisible();
      await expect(page.getByLabel('Dimensions, Width')).toBeVisible();

      // Select second overlapping category
      await page.getByRole('group').filter({ hasText: 'Categories' }).getByRole('textbox').fill('Overlap');
      await page.getByRole('option', { name: 'Overlap Category' }).click();

      // Only common fields should remain (color and weight)
      await expect(page.getByLabel('Weight')).toBeVisible();
      // Color should now be a text input (enum was dropped due to mismatch)
      await expect(page.getByLabel('Color')).toBeVisible();
      // Non-common fields should disappear
      await expect(page.getByLabel('Dimensions, Width')).not.toBeVisible();
    });

    test('selecting a category without MetaSchema shows no schema fields', async ({ page, groupPage }) => {
      await groupPage.gotoList();

      await page.getByRole('group').filter({ hasText: 'Categories' }).getByRole('textbox').fill('No Schema');
      await page.getByRole('option', { name: 'No Schema Category' }).click();

      // No schema fields should appear — just the regular freeFields
      await expect(page.locator('[aria-live="polite"]').filter({ hasText: 'Color' })).not.toBeVisible();
    });

    test('operator override works', async ({ page, groupPage }) => {
      await groupPage.gotoList();

      await page.getByRole('group').filter({ hasText: 'Categories' }).getByRole('textbox').fill('Schema Test');
      await page.getByRole('option', { name: 'Schema Test Category' }).click();

      // Click operator symbol next to Weight
      await page.getByRole('button', { name: /change operator.*=/ }).first().click();

      // Select ≥ operator
      await page.getByLabel(/operator for weight/i).selectOption('GE');

      // Fill value
      await page.getByLabel('Weight').fill('10');

      // Submit
      await page.getByRole('button', { name: /search/i }).click();

      // Verify URL
      const url = new URL(page.url());
      const metaParams = url.searchParams.getAll('MetaQuery');
      expect(metaParams).toContainEqual(expect.stringContaining('weight:GE:10'));
    });
  });

  test.describe('Resources List View', () => {
    test('schema fields appear when selecting a resource category with MetaSchema', async ({ page, resourcePage }) => {
      await resourcePage.gotoList();

      await page.getByRole('group').filter({ hasText: 'Resource Category' }).getByRole('textbox').fill('Schema Res');
      await page.getByRole('option', { name: 'Schema Res Category' }).click();

      // Format has 4 enum values → checkboxes
      const formatFieldset = page.locator('fieldset').filter({ hasText: 'Format' });
      await expect(formatFieldset.getByRole('checkbox', { name: 'jpg' })).toBeVisible();

      // Quality should be a number input
      await expect(page.getByLabel('Quality')).toBeVisible();
    });
  });
});
```

- [ ] **Step 2: Run the E2E tests**

Run: `cd e2e && npm run test:with-server -- --grep "Schema-Driven"`
Expected: All tests should pass. Some tests may need selector adjustments based on actual DOM output — fix any locator issues.

- [ ] **Step 3: Commit**

```bash
git add e2e/tests/schema-search-fields.spec.ts
git commit -m "test: add E2E tests for schema-driven search fields"
```

---

### Task 6: Accessibility Tests

**Files:**
- Create: `e2e/tests/accessibility/schema-search-a11y.spec.ts`

- [ ] **Step 1: Create the accessibility test file**

Check the existing accessibility test pattern first by reading `e2e/fixtures/a11y.fixture.ts` and one existing a11y test to follow the same structure.

Create `e2e/tests/accessibility/schema-search-a11y.spec.ts`:

```typescript
import { test, expect } from '../../fixtures/a11y.fixture';

test.describe('Schema Search Fields Accessibility', () => {
  let categoryWithSchemaId: number;

  test.beforeAll(async ({ apiClient }) => {
    const cat = await apiClient.createCategory('A11y Schema Category', 'For a11y tests', {
      MetaSchema: JSON.stringify({
        type: 'object',
        properties: {
          status: { type: 'string', enum: ['active', 'archived', 'draft'] },
          priority: { type: 'integer' },
          published: { type: 'boolean' },
        },
      }),
    });
    categoryWithSchemaId = cat.ID;
  });

  test('schema search fields pass axe accessibility checks', async ({ page, groupPage, expectA11y }) => {
    await groupPage.gotoList();

    // Select category to show schema fields
    await page.getByRole('group').filter({ hasText: 'Categories' }).getByRole('textbox').fill('A11y Schema');
    await page.getByRole('option', { name: 'A11y Schema Category' }).click();

    // Wait for schema fields to render
    await expect(page.getByLabel('Priority')).toBeVisible();

    // Run axe on the sidebar form
    await expectA11y(page.locator('form[aria-label="Filter groups"]'));
  });
});
```

- [ ] **Step 2: Run accessibility tests**

Run: `cd e2e && npm run test:with-server:a11y -- --grep "Schema Search"`
Expected: All a11y tests pass.

- [ ] **Step 3: Commit**

```bash
git add e2e/tests/accessibility/schema-search-a11y.spec.ts
git commit -m "test: add accessibility tests for schema search fields"
```

---

### Task 7: Manual Verification and Polish

**Files:** No new files — this is a verification task.

- [ ] **Step 1: Build the full application**

Run: `npm run build`
Expected: Clean build.

- [ ] **Step 2: Run all Go unit tests**

Run: `go test --tags 'json1 fts5' ./...`
Expected: All pass (no backend changes, so these should be unaffected).

- [ ] **Step 3: Run full E2E test suite (browser + CLI)**

Run: `cd e2e && npm run test:with-server:all`
Expected: All existing tests still pass, plus the new schema search tests.

- [ ] **Step 4: Run Postgres tests**

Run: `go test --tags 'json1 fts5 postgres' ./mrql/... ./server/api_tests/... -count=1 && cd e2e && npm run test:with-server:postgres`
Expected: All pass.

- [ ] **Step 5: Final commit if any polish was needed**

If any adjustments were made during verification:

```bash
git add -A
git commit -m "fix: polish schema search fields based on manual verification"
```
