# Visual JSON Schema Editor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a `<schema-editor>` Lit web component with three modes (edit, form, search) that unifies all JSON Schema UI in the application behind a single, testable-in-isolation web component.

**Architecture:** A single Lit+TypeScript web component with mode-specific renderers sharing a common `schema-core.ts` utility module. The edit mode provides a tree+detail-panel visual builder. Form and search modes replace existing Alpine.js components (`schemaForm.js`, `schemaSearchFields.js`). A modal wrapper on category/resource-type forms hosts the editor with Edit/Preview/Raw JSON tabs.

**Tech Stack:** Lit 3.x, TypeScript, Vite (existing), Vitest (new, for TS unit tests), Playwright (existing, for E2E)

---

## File Structure

```
src/
  schema-editor/
    schema-editor.ts            # <schema-editor> main Lit element — delegates to mode renderers
    schema-core.ts              # Shared: resolveRef, mergeSchemas, resolveSchema, flattenSchema,
                                #   intersectFields, getDefaultValue, scoreSchemaMatch,
                                #   evaluateCondition, inferType, inferSchema, titleCase
    schema-tree-model.ts        # SchemaNode tree model, schemaToTree(), treeToSchema(), detectDraft()
    schema-core.test.ts         # Vitest unit tests for schema-core
    schema-tree-model.test.ts   # Vitest unit tests for tree model round-trips
    modes/
      edit-mode.ts              # Edit mode top-level: tree + detail split layout
      form-mode.ts              # Form mode: data-entry form renderer (port of schemaForm.js)
      search-mode.ts            # Search mode: filter field renderer (port of schemaSearchFields.js)
    tree/
      tree-panel.ts             # Left sidebar: collapsible tree of schema nodes
      detail-panel.ts           # Right panel: property editor with type-specific sections
      node-editors/
        string-editor.ts        # String constraints: minLength, maxLength, pattern, format, enum, const, default
        number-editor.ts        # Number/integer: min, max, exclusiveMin/Max, multipleOf, enum, const, default
        boolean-editor.ts       # Boolean: const, default
        object-editor.ts        # Object: additionalProperties, minProperties, maxProperties, patternProperties
        array-editor.ts         # Array: items, prefixItems, minItems, maxItems, uniqueItems, contains
        enum-editor.ts          # Enum value list: add/remove/reorder values
        composition-editor.ts   # oneOf, anyOf, allOf, not: variant list management
        conditional-editor.ts   # if/then/else: three sub-schema slots
        ref-editor.ts           # $ref: dropdown picker from $defs
    styles.ts                   # Shared CSS-in-JS styles (Lit css tagged template)
    test.html                   # Standalone test page — all three modes, no Go/Alpine needed
  components/
    schemaEditorModal.ts        # Alpine.js data component for the modal wrapper
```

**Modified files:**
- `src/main.js` — import new web component + modal Alpine component
- `vite.config.js` — no changes needed (Vite handles .ts natively)
- `package.json` — add `lit`, `vitest` dependencies
- `tsconfig.json` — new file for TypeScript config
- `templates/createCategory.tpl` — add Visual Editor button next to MetaSchema textarea
- `templates/createResourceCategory.tpl` — same
- `templates/createGroup.tpl` — replace schemaForm Alpine include with `<schema-editor mode="form">`
- `templates/createResource.tpl` — add schema-driven form (currently only has freeFields)
- `templates/partials/form/schemaSearchFields.tpl` — replace with `<schema-editor mode="search">`
- `templates/listGroups.tpl`, `listGroupsText.tpl`, `listGroupsTimeline.tpl` — update search field include
- `templates/partials/form/searchFormResource.tpl` — update search field include
- `docs-site/docs/features/meta-schemas.md` — add visual editor documentation
- `docs-site/static/img/screenshot-manifest.json` — add 4 new screenshot entries

---

## Phase 1: Foundation — schema-core.ts, tree model, and edit mode

### Task 1: Project setup — TypeScript, Lit, Vitest

**Files:**
- Create: `tsconfig.json`
- Modify: `package.json`

- [ ] **Step 1: Install dependencies**

Run:
```bash
cd /Users/egecan/Code/mahresources && npm install lit && npm install -D vitest typescript
```

- [ ] **Step 2: Create tsconfig.json**

```json
{
  "compilerOptions": {
    "target": "ES2021",
    "module": "ESNext",
    "moduleResolution": "bundler",
    "lib": ["ES2021", "DOM", "DOM.Iterable"],
    "strict": true,
    "noEmit": true,
    "skipLibCheck": true,
    "isolatedModules": true,
    "esModuleInterop": true,
    "forceConsistentCasingInFileNames": true,
    "useDefineForClassFields": false,
    "experimentalDecorators": true,
    "declaration": true,
    "sourceMap": true,
    "outDir": "./dist"
  },
  "include": ["src/**/*.ts"],
  "exclude": ["node_modules", "public"]
}
```

- [ ] **Step 3: Add vitest test script to package.json**

Add to the `"scripts"` section in `package.json`:
```json
"test:unit": "vitest run",
"test:unit:watch": "vitest"
```

- [ ] **Step 4: Verify setup compiles**

Run:
```bash
npx tsc --noEmit
```
Expected: exits cleanly (no .ts files yet, so nothing to check)

- [ ] **Step 5: Commit**

```bash
git add tsconfig.json package.json package-lock.json
git commit -m "chore: add TypeScript, Lit, and Vitest dependencies"
```

---

### Task 2: Extract schema-core.ts from existing JS

**Files:**
- Create: `src/schema-editor/schema-core.ts`
- Create: `src/schema-editor/schema-core.test.ts`

This extracts the duplicated utility functions from `src/components/schemaForm.js` and `src/components/schemaSearchFields.js` into a single typed module. The old files are NOT yet modified — they'll be replaced in later phases.

- [ ] **Step 1: Write failing unit tests for resolveRef**

Create `src/schema-editor/schema-core.test.ts`:
```typescript
import { describe, it, expect } from 'vitest';
import { resolveRef, mergeSchemas, resolveSchema, flattenSchema, intersectFields, getDefaultValue, scoreSchemaMatch, evaluateCondition, inferType, inferSchema, titleCase } from './schema-core';

describe('resolveRef', () => {
  it('resolves a simple $ref pointer', () => {
    const root = {
      definitions: { address: { type: 'object', properties: { street: { type: 'string' } } } },
    };
    const result = resolveRef('#/definitions/address', root);
    expect(result).toEqual({ type: 'object', properties: { street: { type: 'string' } } });
  });

  it('returns null for invalid ref', () => {
    expect(resolveRef('#/missing/path', { definitions: {} })).toBeNull();
  });

  it('returns null for non-string input', () => {
    expect(resolveRef(42 as any, {})).toBeNull();
  });

  it('returns null for non-hash ref', () => {
    expect(resolveRef('http://example.com/schema', {})).toBeNull();
  });
});

describe('mergeSchemas', () => {
  it('merges properties from two schemas', () => {
    const base = { type: 'object', properties: { a: { type: 'string' } } };
    const ext = { properties: { b: { type: 'number' } } };
    const merged = mergeSchemas(base, ext);
    expect(merged.properties).toEqual({ a: { type: 'string' }, b: { type: 'number' } });
  });

  it('unions required arrays', () => {
    const base = { required: ['a'] };
    const ext = { required: ['b', 'a'] };
    const merged = mergeSchemas(base, ext);
    expect(merged.required).toEqual(['a', 'b']);
  });

  it('does not copy composition keywords', () => {
    const base = {};
    const ext = { allOf: [{ type: 'string' }], title: 'test' };
    const merged = mergeSchemas(base, ext);
    expect(merged).not.toHaveProperty('allOf');
    expect(merged.title).toBe('test');
  });
});

describe('inferType', () => {
  it('detects array', () => expect(inferType([])).toBe('array'));
  it('detects null', () => expect(inferType(null)).toBe('null'));
  it('detects integer', () => expect(inferType(42)).toBe('integer'));
  it('detects number', () => expect(inferType(3.14)).toBe('number'));
  it('detects string', () => expect(inferType('hi')).toBe('string'));
  it('detects boolean', () => expect(inferType(true)).toBe('boolean'));
  it('detects object', () => expect(inferType({})).toBe('object'));
});

describe('inferSchema', () => {
  it('infers object schema', () => {
    expect(inferSchema({})).toEqual({ type: 'object', properties: {} });
  });
  it('infers array schema from first element', () => {
    expect(inferSchema([42])).toEqual({ type: 'array', items: { type: 'integer' } });
  });
  it('infers empty array as string items', () => {
    expect(inferSchema([])).toEqual({ type: 'array', items: { type: 'string' } });
  });
});

describe('resolveSchema', () => {
  it('resolves $ref in schema', () => {
    const root = { $defs: { name: { type: 'string' } } };
    const schema = { $ref: '#/$defs/name' };
    const result = resolveSchema(schema, root);
    expect(result).toEqual({ type: 'string' });
  });

  it('merges allOf schemas', () => {
    const schema = {
      allOf: [
        { properties: { a: { type: 'string' } } },
        { properties: { b: { type: 'number' } }, required: ['b'] },
      ],
    };
    const result = resolveSchema(schema, schema);
    expect(result!.properties).toEqual({ a: { type: 'string' }, b: { type: 'number' } });
    expect(result!.required).toEqual(['b']);
  });

  it('unions oneOf variant properties for search', () => {
    const schema = {
      oneOf: [
        { properties: { a: { type: 'string' } } },
        { properties: { b: { type: 'number' } } },
      ],
    };
    const result = resolveSchema(schema, schema);
    expect(result!.properties).toHaveProperty('a');
    expect(result!.properties).toHaveProperty('b');
  });
});

describe('flattenSchema', () => {
  it('flattens top-level properties', () => {
    const schema = {
      type: 'object',
      properties: {
        name: { type: 'string', title: 'Full Name' },
        age: { type: 'integer' },
      },
    };
    const fields = flattenSchema(schema);
    expect(fields).toEqual([
      { path: 'name', label: 'Full Name', type: 'string', enum: null },
      { path: 'age', label: 'Age', type: 'integer', enum: null },
    ]);
  });

  it('flattens nested object properties with dot paths', () => {
    const schema = {
      type: 'object',
      properties: {
        address: {
          type: 'object',
          properties: {
            city: { type: 'string' },
          },
        },
      },
    };
    const fields = flattenSchema(schema);
    expect(fields).toEqual([
      { path: 'address.city', label: 'Address › City', type: 'string', enum: null },
    ]);
  });

  it('includes enum values', () => {
    const schema = {
      type: 'object',
      properties: {
        status: { type: 'string', enum: ['active', 'inactive'] },
      },
    };
    const fields = flattenSchema(schema);
    expect(fields[0].enum).toEqual(['active', 'inactive']);
  });

  it('skips array-typed properties', () => {
    const schema = {
      type: 'object',
      properties: {
        tags: { type: 'array', items: { type: 'string' } },
        name: { type: 'string' },
      },
    };
    const fields = flattenSchema(schema);
    expect(fields).toHaveLength(1);
    expect(fields[0].path).toBe('name');
  });
});

describe('intersectFields', () => {
  it('returns common fields only', () => {
    const list1 = [
      { path: 'name', label: 'Name', type: 'string' as const, enum: null },
      { path: 'age', label: 'Age', type: 'integer' as const, enum: null },
    ];
    const list2 = [
      { path: 'name', label: 'Name', type: 'string' as const, enum: null },
      { path: 'email', label: 'Email', type: 'string' as const, enum: null },
    ];
    const result = intersectFields([list1, list2]);
    expect(result).toHaveLength(1);
    expect(result[0].path).toBe('name');
  });

  it('falls back to string on type conflict', () => {
    const list1 = [{ path: 'x', label: 'X', type: 'string' as const, enum: null }];
    const list2 = [{ path: 'x', label: 'X', type: 'number' as const, enum: null }];
    const result = intersectFields([list1, list2]);
    expect(result[0].type).toBe('string');
  });

  it('merges integer and number to number', () => {
    const list1 = [{ path: 'x', label: 'X', type: 'integer' as const, enum: null }];
    const list2 = [{ path: 'x', label: 'X', type: 'number' as const, enum: null }];
    const result = intersectFields([list1, list2]);
    expect(result[0].type).toBe('number');
  });
});

describe('getDefaultValue', () => {
  it('returns default if specified', () => {
    expect(getDefaultValue({ type: 'string', default: 'hello' })).toBe('hello');
  });
  it('returns const if specified', () => {
    expect(getDefaultValue({ const: 42 })).toBe(42);
  });
  it('returns empty object for object type', () => {
    expect(getDefaultValue({ type: 'object' })).toEqual({});
  });
  it('returns empty array for array type', () => {
    expect(getDefaultValue({ type: 'array' })).toEqual([]);
  });
  it('returns empty string for string type', () => {
    expect(getDefaultValue({ type: 'string' })).toBe('');
  });
  it('returns 0 for number type', () => {
    expect(getDefaultValue({ type: 'number' })).toBe(0);
  });
  it('returns false for boolean type', () => {
    expect(getDefaultValue({ type: 'boolean' })).toBe(false);
  });
});

describe('evaluateCondition', () => {
  it('returns true when const matches', () => {
    const cond = { properties: { status: { const: 'active' } } };
    expect(evaluateCondition(cond, { status: 'active' })).toBe(true);
  });
  it('returns false when const does not match', () => {
    const cond = { properties: { status: { const: 'active' } } };
    expect(evaluateCondition(cond, { status: 'inactive' })).toBe(false);
  });
  it('returns true when enum includes value', () => {
    const cond = { properties: { status: { enum: ['a', 'b'] } } };
    expect(evaluateCondition(cond, { status: 'a' })).toBe(true);
  });
});

describe('titleCase', () => {
  it('converts camelCase', () => expect(titleCase('firstName')).toBe('First Name'));
  it('converts snake_case', () => expect(titleCase('first_name')).toBe('First Name'));
  it('converts kebab-case', () => expect(titleCase('first-name')).toBe('First Name'));
});
```

- [ ] **Step 2: Run tests to verify they fail**

Run:
```bash
cd /Users/egecan/Code/mahresources && npx vitest run src/schema-editor/schema-core.test.ts
```
Expected: FAIL — `schema-core` module does not exist yet.

- [ ] **Step 3: Implement schema-core.ts**

Create `src/schema-editor/schema-core.ts`:
```typescript
// ─── Type definitions ────────────────────────────────────────────────────────

export interface FlatField {
  path: string;
  label: string;
  type: string;
  enum: string[] | null;
}

export type JSONSchema = Record<string, any>;

// ─── Ref resolution ──────────────────────────────────────────────────────────

export function resolveRef(ref: unknown, root: JSONSchema): JSONSchema | null {
  if (typeof ref !== 'string' || !ref.startsWith('#/')) return null;
  const parts = ref.split('/').slice(1);
  let current: any = root;
  for (const part of parts) {
    if (current && typeof current === 'object' && part in current) {
      current = current[part];
    } else {
      return null;
    }
  }
  return current;
}

// ─── Schema merging ──────────────────────────────────────────────────────────

export function mergeSchemas(base: JSONSchema, extension: JSONSchema): JSONSchema {
  const merged: JSONSchema = { ...base };
  for (const key in extension) {
    if (key === 'properties') {
      merged.properties = { ...(base.properties || {}), ...extension.properties };
    } else if (key === 'required') {
      merged.required = [...new Set([...(base.required || []), ...(extension.required || [])])];
    } else if (!['allOf', 'anyOf', 'oneOf', '$ref'].includes(key)) {
      merged[key] = extension[key];
    }
  }
  return merged;
}

// ─── Schema resolution (composition keywords) ───────────────────────────────

export function resolveSchema(schema: JSONSchema | null, rootSchema: JSONSchema): JSONSchema | null {
  if (!schema) return schema;

  if (schema.$ref) {
    const resolved = resolveRef(schema.$ref, rootSchema);
    if (resolved) {
      const merged: JSONSchema = { ...resolved, ...schema };
      delete merged.$ref;
      return resolveSchema(merged, rootSchema);
    }
    return null;
  }

  for (const keyword of ['allOf', 'oneOf', 'anyOf'] as const) {
    if (schema[keyword] && Array.isArray(schema[keyword])) {
      let merged: JSONSchema = { ...schema };
      delete merged[keyword];
      for (const sub of schema[keyword]) {
        let resolved: JSONSchema | null;
        if (sub.$ref) {
          const refResult = resolveRef(sub.$ref, rootSchema);
          const siblings: JSONSchema = { ...sub };
          delete siblings.$ref;
          resolved = refResult ? mergeSchemas(refResult, siblings) : siblings;
        } else {
          resolved = sub;
        }
        if (resolved) merged = mergeSchemas(merged, resolved);
      }
      return resolveSchema(merged, rootSchema);
    }
  }

  return schema;
}

// ─── Type inference ──────────────────────────────────────────────────────────

export function inferType(val: unknown): string {
  if (Array.isArray(val)) return 'array';
  if (val === null) return 'null';
  const t = typeof val;
  if (t === 'number') {
    return Number.isInteger(val) ? 'integer' : 'number';
  }
  return t;
}

export function inferSchema(val: unknown): JSONSchema {
  const type = inferType(val);
  if (type === 'object') return { type: 'object', properties: {} };
  if (type === 'array') {
    const arr = val as unknown[];
    return { type: 'array', items: arr.length ? inferSchema(arr[0]) : { type: 'string' } };
  }
  return { type };
}

// ─── Condition evaluation ────────────────────────────────────────────────────

export function evaluateCondition(conditionSchema: JSONSchema | null | undefined, data: any): boolean {
  if (!conditionSchema || !conditionSchema.properties) return true;
  for (const key in conditionSchema.properties) {
    const propSchema = conditionSchema.properties[key];
    if (propSchema.const !== undefined && data?.[key] !== propSchema.const) return false;
    if (propSchema.enum && !propSchema.enum.includes(data?.[key])) return false;
  }
  return true;
}

// ─── Schema match scoring ────────────────────────────────────────────────────

export function scoreSchemaMatch(schema: JSONSchema, data: unknown, rootSchema: JSONSchema): number {
  if (schema.$ref) {
    const resolved = resolveRef(schema.$ref, rootSchema);
    if (resolved) {
      schema = { ...resolved, ...schema };
    }
  }

  if (schema.const !== undefined) return schema.const === data ? 100 : 0;

  const dataType = inferType(data);
  let schemaType = schema.type;

  if (Array.isArray(schemaType)) {
    if (schemaType.includes(dataType)) return 10;
    if (dataType === 'integer' && schemaType.includes('number')) return 9;
    if (dataType === 'null' && (schemaType.includes('string') || schemaType.includes('number'))) return 5;
    return 0;
  }

  if (schemaType && schemaType !== dataType) {
    if (schemaType === 'number' && dataType === 'integer') return 9;
    return 0;
  }

  if (dataType === 'object' && schema.properties) {
    const dataKeys = Object.keys(data as object);
    const schemaKeys = Object.keys(schema.properties);
    const matchCount = dataKeys.filter(k => schemaKeys.includes(k)).length;
    return matchCount + 10;
  }

  return 10;
}

// ─── Default values ──────────────────────────────────────────────────────────

export function getDefaultValue(schema: JSONSchema, rootSchema?: JSONSchema): any {
  if (schema.$ref) {
    const resolved = resolveRef(schema.$ref, rootSchema || schema);
    if (resolved) {
      return getDefaultValue({ ...resolved, ...schema, $ref: undefined }, rootSchema);
    }
  }

  if (schema.allOf && Array.isArray(schema.allOf)) {
    let merged: JSONSchema = { ...schema };
    delete merged.allOf;
    for (const sub of schema.allOf) {
      const resolved = sub.$ref ? resolveRef(sub.$ref, rootSchema || schema) : sub;
      if (resolved) merged = mergeSchemas(merged, resolved);
    }
    return getDefaultValue(merged, rootSchema);
  }

  if (schema.if) {
    const baseSchema: JSONSchema = { ...schema };
    delete baseSchema.if;
    delete baseSchema.then;
    delete baseSchema.else;
    const merged = mergeSchemas(baseSchema, schema.then || {});
    return getDefaultValue(merged, rootSchema);
  }

  if (schema.default !== undefined) return schema.default;
  if (schema.const !== undefined) return schema.const;
  if (schema.type === 'object') return {};
  if (schema.type === 'array') return [];
  if (schema.type === 'boolean') return false;
  if (schema.type === 'number' || schema.type === 'integer') return 0;

  if (Array.isArray(schema.type)) {
    if (schema.type.includes('string')) return '';
    if (schema.type.includes('number') || schema.type.includes('integer')) return 0;
    if (schema.type.includes('boolean')) return false;
    if (schema.type.includes('object')) return {};
    if (schema.type.includes('array')) return [];
    if (schema.type.includes('null')) return null;
  }

  if (schema.oneOf && schema.oneOf.length > 0) return getDefaultValue(schema.oneOf[0], rootSchema);
  if (schema.anyOf && schema.anyOf.length > 0) return getDefaultValue(schema.anyOf[0], rootSchema);

  return '';
}

// ─── Title case ──────────────────────────────────────────────────────────────

export function titleCase(key: string): string {
  return key
    .replace(/([a-z])([A-Z])/g, '$1 $2')
    .replace(/[_-]/g, ' ')
    .replace(/\b\w/g, c => c.toUpperCase());
}

// ─── Schema flattening (for search mode) ─────────────────────────────────────

export function flattenSchema(
  schema: JSONSchema,
  prefix = '',
  labelPrefix = '',
  depth = 0,
  rootSchema: JSONSchema | null = null,
): FlatField[] {
  if (depth > 10 || !schema) return [];

  const root = rootSchema || schema;
  const resolved = resolveSchema(schema, root);
  if (!resolved || !resolved.properties) return [];

  const fields: FlatField[] = [];

  for (const [key, rawProp] of Object.entries(resolved.properties) as [string, JSONSchema][]) {
    const path = prefix ? `${prefix}.${key}` : key;
    const prop = resolveSchema(rawProp, root) || rawProp;
    const rawLabel = prop.title || titleCase(key);
    const label = labelPrefix ? `${labelPrefix} › ${rawLabel}` : rawLabel;

    if (prop.properties) {
      fields.push(...flattenSchema(prop, path, label, depth + 1, root));
    } else if (prop.type === 'array') {
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

// ─── Field intersection (multi-schema search) ────────────────────────────────

export function intersectFields(fieldLists: FlatField[][]): FlatField[] {
  if (fieldLists.length === 0) return [];
  if (fieldLists.length === 1) return fieldLists[0];

  const base = new Map(fieldLists[0].map(f => [f.path, { ...f }]));

  for (let i = 1; i < fieldLists.length; i++) {
    const currentPaths = new Set(fieldLists[i].map(f => f.path));

    for (const path of base.keys()) {
      if (!currentPaths.has(path)) {
        base.delete(path);
      }
    }

    for (const field of fieldLists[i]) {
      const existing = base.get(field.path);
      if (!existing) continue;

      const numericTypes = new Set(['number', 'integer']);
      if (existing.type !== field.type) {
        if (numericTypes.has(existing.type) && numericTypes.has(field.type)) {
          existing.type = 'number';
        } else {
          existing.type = 'string';
          existing.enum = null;
          continue;
        }
      }
      if (existing.enum && field.enum) {
        const a = [...existing.enum].sort();
        const b = [...field.enum].sort();
        if (JSON.stringify(a) !== JSON.stringify(b)) {
          existing.enum = null;
        }
      } else if (existing.enum !== field.enum) {
        existing.enum = null;
      }
    }
  }

  return Array.from(base.values());
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run:
```bash
npx vitest run src/schema-editor/schema-core.test.ts
```
Expected: ALL PASS

- [ ] **Step 5: Commit**

```bash
git add src/schema-editor/schema-core.ts src/schema-editor/schema-core.test.ts
git commit -m "feat: extract schema-core.ts with shared schema utilities and tests"
```

---

### Task 3: Schema tree model — schemaToTree / treeToSchema

**Files:**
- Create: `src/schema-editor/schema-tree-model.ts`
- Create: `src/schema-editor/schema-tree-model.test.ts`

- [ ] **Step 1: Write failing unit tests for tree model**

Create `src/schema-editor/schema-tree-model.test.ts`:
```typescript
import { describe, it, expect } from 'vitest';
import { schemaToTree, treeToSchema, detectDraft, SchemaNode } from './schema-tree-model';

describe('detectDraft', () => {
  it('detects draft-07', () => {
    expect(detectDraft({ $schema: 'http://json-schema.org/draft-07/schema#' })).toBe('draft-07');
  });
  it('detects 2020-12', () => {
    expect(detectDraft({ $schema: 'https://json-schema.org/draft/2020-12/schema' })).toBe('2020-12');
  });
  it('detects draft-04', () => {
    expect(detectDraft({ $schema: 'http://json-schema.org/draft-04/schema#' })).toBe('draft-04');
  });
  it('returns null for no $schema', () => {
    expect(detectDraft({ type: 'object' })).toBeNull();
  });
});

describe('schemaToTree / treeToSchema round-trip', () => {
  it('round-trips a flat object schema', () => {
    const schema = {
      type: 'object',
      properties: {
        name: { type: 'string', minLength: 1 },
        age: { type: 'integer', minimum: 0 },
      },
      required: ['name'],
    };
    const tree = schemaToTree(schema);
    expect(tree.type).toBe('object');
    expect(tree.children).toHaveLength(2);
    expect(tree.children![0].name).toBe('name');
    expect(tree.children![0].required).toBe(true);
    expect(tree.children![1].name).toBe('age');
    expect(tree.children![1].required).toBe(false);

    const output = treeToSchema(tree);
    expect(output).toEqual(schema);
  });

  it('round-trips enum property', () => {
    const schema = {
      type: 'object',
      properties: {
        status: { type: 'string', enum: ['active', 'inactive'] },
      },
    };
    const tree = schemaToTree(schema);
    const output = treeToSchema(tree);
    expect(output).toEqual(schema);
  });

  it('round-trips nested object', () => {
    const schema = {
      type: 'object',
      properties: {
        address: {
          type: 'object',
          properties: {
            street: { type: 'string' },
            city: { type: 'string' },
          },
          required: ['street'],
        },
      },
    };
    const tree = schemaToTree(schema);
    expect(tree.children![0].children).toHaveLength(2);
    const output = treeToSchema(tree);
    expect(output).toEqual(schema);
  });

  it('round-trips array with items', () => {
    const schema = {
      type: 'object',
      properties: {
        tags: { type: 'array', items: { type: 'string' }, minItems: 1 },
      },
    };
    const tree = schemaToTree(schema);
    const output = treeToSchema(tree);
    expect(output).toEqual(schema);
  });

  it('round-trips oneOf', () => {
    const schema = {
      type: 'object',
      properties: {
        contact: {
          oneOf: [
            { type: 'string', title: 'Email' },
            { type: 'object', title: 'Phone', properties: { number: { type: 'string' } } },
          ],
        },
      },
    };
    const tree = schemaToTree(schema);
    const output = treeToSchema(tree);
    expect(output).toEqual(schema);
  });

  it('round-trips $ref and $defs', () => {
    const schema = {
      type: 'object',
      $defs: {
        address: { type: 'object', properties: { city: { type: 'string' } } },
      },
      properties: {
        home: { $ref: '#/$defs/address' },
      },
    };
    const tree = schemaToTree(schema);
    const output = treeToSchema(tree);
    expect(output).toEqual(schema);
  });

  it('round-trips if/then/else', () => {
    const schema = {
      type: 'object',
      properties: {
        kind: { type: 'string', enum: ['a', 'b'] },
      },
      if: { properties: { kind: { const: 'a' } } },
      then: { properties: { aField: { type: 'string' } } },
      else: { properties: { bField: { type: 'number' } } },
    };
    const tree = schemaToTree(schema);
    const output = treeToSchema(tree);
    expect(output).toEqual(schema);
  });

  it('preserves title, description, and $schema', () => {
    const schema = {
      $schema: 'https://json-schema.org/draft/2020-12/schema',
      title: 'Person',
      description: 'A person record',
      type: 'object',
      properties: { name: { type: 'string' } },
    };
    const tree = schemaToTree(schema);
    const output = treeToSchema(tree);
    expect(output).toEqual(schema);
  });

  it('round-trips boolean type', () => {
    const schema = {
      type: 'object',
      properties: { active: { type: 'boolean', default: true } },
    };
    const tree = schemaToTree(schema);
    const output = treeToSchema(tree);
    expect(output).toEqual(schema);
  });
});
```

- [ ] **Step 2: Run tests to verify they fail**

Run:
```bash
npx vitest run src/schema-editor/schema-tree-model.test.ts
```
Expected: FAIL — module does not exist

- [ ] **Step 3: Implement schema-tree-model.ts**

Create `src/schema-editor/schema-tree-model.ts`:
```typescript
import type { JSONSchema } from './schema-core';

// ─── Schema Node ─────────────────────────────────────────────────────────────

export interface SchemaNode {
  /** Property name (empty string for root) */
  name: string;
  /** JSON Schema type: string, number, integer, boolean, object, array, null */
  type: string;
  /** Is this property in the parent's required array? */
  required: boolean;
  /** The raw schema keywords for this node (title, description, constraints, etc.) */
  schema: JSONSchema;
  /** Children: object properties, array items schema, composition variants */
  children?: SchemaNode[];
  /** For composition nodes (oneOf/anyOf/allOf): which keyword */
  compositionKeyword?: 'oneOf' | 'anyOf' | 'allOf' | 'not';
  /** For $ref nodes: the ref string */
  ref?: string;
  /** For $defs: each def is a child of a virtual $defs node */
  isDef?: boolean;
  /** Unique ID for tree rendering (assigned at parse time) */
  id: string;
}

let nextId = 0;
function uid(): string {
  return `node-${++nextId}`;
}

/** Reset ID counter (for tests) */
export function resetIdCounter(): void {
  nextId = 0;
}

// ─── Draft detection ─────────────────────────────────────────────────────────

export function detectDraft(schema: JSONSchema): string | null {
  const s = schema.$schema;
  if (!s || typeof s !== 'string') return null;
  if (s.includes('2020-12')) return '2020-12';
  if (s.includes('2019-09')) return '2019-09';
  if (s.includes('draft-07')) return 'draft-07';
  if (s.includes('draft-06')) return 'draft-06';
  if (s.includes('draft-04')) return 'draft-04';
  return null;
}

// ─── Schema → Tree ───────────────────────────────────────────────────────────

export function schemaToTree(schema: JSONSchema, name = '', parentRequired: string[] = []): SchemaNode {
  const node: SchemaNode = {
    id: uid(),
    name,
    type: schema.type || 'object',
    required: parentRequired.includes(name),
    schema: { ...schema },
  };

  // Clean up children-related keys from stored schema — they're represented in the tree
  delete node.schema.properties;
  delete node.schema.required;
  delete node.schema.$defs;
  delete node.schema.definitions;

  // Object with properties → children
  if (schema.properties) {
    const reqSet = schema.required || [];
    node.children = Object.entries(schema.properties).map(([key, propSchema]) =>
      schemaToTree(propSchema as JSONSchema, key, reqSet),
    );
  }

  // $defs / definitions
  const defs = schema.$defs || schema.definitions;
  if (defs && typeof defs === 'object') {
    const defsNode: SchemaNode = {
      id: uid(),
      name: '$defs',
      type: 'object',
      required: false,
      schema: {},
      isDef: true,
      children: Object.entries(defs).map(([key, defSchema]) => {
        const child = schemaToTree(defSchema as JSONSchema, key);
        child.isDef = true;
        return child;
      }),
    };
    node.children = [...(node.children || []), defsNode];
  }

  return node;
}

// ─── Tree → Schema ───────────────────────────────────────────────────────────

export function treeToSchema(node: SchemaNode): JSONSchema {
  const schema: JSONSchema = { ...node.schema };

  // Restore type for root and all nodes
  if (node.type) {
    schema.type = node.type;
  }

  // Separate property children from $defs node
  const propChildren = (node.children || []).filter(c => !c.isDef && c.name !== '$defs');
  const defsNode = (node.children || []).find(c => c.isDef && c.name === '$defs');

  // Restore properties
  if (propChildren.length > 0 && (node.type === 'object' || !node.type)) {
    schema.properties = {};
    const required: string[] = [];

    for (const child of propChildren) {
      schema.properties[child.name] = treeToSchema(child);
      if (child.required) {
        required.push(child.name);
      }
    }

    if (required.length > 0) {
      schema.required = required;
    }
  }

  // Restore $defs
  if (defsNode && defsNode.children && defsNode.children.length > 0) {
    const defsKey = node.schema.$schema?.includes('draft-04') ? 'definitions' : '$defs';
    schema[defsKey] = {};
    for (const defChild of defsNode.children) {
      schema[defsKey][defChild.name] = treeToSchema(defChild);
    }
  }

  return schema;
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run:
```bash
npx vitest run src/schema-editor/schema-tree-model.test.ts
```
Expected: ALL PASS

- [ ] **Step 5: Commit**

```bash
git add src/schema-editor/schema-tree-model.ts src/schema-editor/schema-tree-model.test.ts
git commit -m "feat: add schema tree model with schemaToTree/treeToSchema and tests"
```

---

### Task 4: Shared Lit styles

**Files:**
- Create: `src/schema-editor/styles.ts`

- [ ] **Step 1: Create shared styles module**

Create `src/schema-editor/styles.ts`:
```typescript
import { css } from 'lit';

export const sharedStyles = css`
  :host {
    display: block;
    font-family: system-ui, -apple-system, sans-serif;
    font-size: 13px;
    color: #1f2937;
  }

  * {
    box-sizing: border-box;
  }

  /* ─── Tree badges ─────────────────────────── */
  .badge {
    display: inline-block;
    padding: 0 6px;
    border-radius: 3px;
    font-size: 10px;
    font-weight: 600;
    line-height: 18px;
  }
  .badge-string { background: #d1fae5; color: #065f46; }
  .badge-number, .badge-integer { background: #dbeafe; color: #1e40af; }
  .badge-boolean { background: #fef3c7; color: #92400e; }
  .badge-object { background: #e0e7ff; color: #3730a3; }
  .badge-array { background: #ede9fe; color: #5b21b6; }
  .badge-enum { background: #fef3c7; color: #92400e; }
  .badge-composition { background: #fce7f3; color: #9d174d; }
  .badge-ref { background: #f3f4f6; color: #6b7280; }
  .badge-conditional { background: #fff1f2; color: #9f1239; }

  /* ─── Form elements ───────────────────────── */
  input, select, textarea {
    width: 100%;
    padding: 5px 8px;
    border: 1px solid #d1d5db;
    border-radius: 4px;
    font-size: 12px;
    font-family: inherit;
  }
  input:focus, select:focus, textarea:focus {
    outline: none;
    border-color: #6366f1;
    box-shadow: 0 0 0 2px rgba(99, 102, 241, 0.2);
  }
  input[type="checkbox"] {
    width: auto;
    margin-right: 4px;
  }
  input[type="number"] {
    -moz-appearance: textfield;
  }
  label {
    display: block;
    font-size: 11px;
    color: #6b7280;
    margin-bottom: 2px;
    font-weight: 600;
  }

  /* ─── Buttons ─────────────────────────────── */
  button {
    cursor: pointer;
    font-family: inherit;
  }
  .btn {
    padding: 4px 12px;
    border: 1px solid #d1d5db;
    border-radius: 4px;
    background: white;
    font-size: 11px;
    color: #374151;
  }
  .btn:hover { background: #f9fafb; }
  .btn:focus-visible { outline: 2px solid #6366f1; outline-offset: 2px; }
  .btn-primary {
    background: #4338ca;
    color: white;
    border-color: #4338ca;
  }
  .btn-primary:hover { background: #3730a3; }
  .btn-danger {
    background: #fee2e2;
    color: #dc2626;
    border-color: #fecaca;
  }
  .btn-danger:hover { background: #fecaca; }
  .btn-ghost {
    background: transparent;
    border: 1px dashed #d1d5db;
    color: #6b7280;
    width: 100%;
    text-align: center;
  }

  /* ─── Utility ─────────────────────────────── */
  .sr-only {
    position: absolute;
    width: 1px;
    height: 1px;
    padding: 0;
    margin: -1px;
    overflow: hidden;
    clip: rect(0, 0, 0, 0);
    white-space: nowrap;
    border-width: 0;
  }
  .required-marker {
    color: #dc2626;
    font-size: 9px;
    margin-left: 2px;
  }
  .breadcrumb {
    font-size: 11px;
    color: #9ca3af;
    margin-bottom: 8px;
  }
  .breadcrumb .current {
    color: #4338ca;
  }
`;
```

- [ ] **Step 2: Commit**

```bash
git add src/schema-editor/styles.ts
git commit -m "feat: add shared Lit CSS styles for schema editor"
```

---

### Task 5: `<schema-editor>` shell component with mode routing

**Files:**
- Create: `src/schema-editor/schema-editor.ts`
- Modify: `src/main.js`

- [ ] **Step 1: Create the main component**

Create `src/schema-editor/schema-editor.ts`:
```typescript
import { LitElement, html, css, nothing } from 'lit';
import { customElement, property, state } from 'lit/decorators.js';
import { sharedStyles } from './styles';
import type { JSONSchema } from './schema-core';

@customElement('schema-editor')
export class SchemaEditor extends LitElement {
  static override styles = [
    sharedStyles,
    css`
      :host {
        display: block;
      }
    `,
  ];

  @property({ type: String }) mode: 'edit' | 'form' | 'search' = 'edit';
  @property({ type: String }) schema = '';
  @property({ type: String }) value = '';
  @property({ type: String }) name = 'Meta';
  @property({ type: String, attribute: 'meta-query' }) metaQuery = '';
  @property({ type: String, attribute: 'field-name' }) fieldName = 'MetaQuery';

  @state() private _parsedSchema: JSONSchema | null = null;

  override willUpdate(changed: Map<string, unknown>) {
    if (changed.has('schema')) {
      this._parseSchema();
    }
  }

  private _parseSchema() {
    if (!this.schema) {
      this._parsedSchema = null;
      return;
    }
    try {
      this._parsedSchema = JSON.parse(this.schema);
    } catch {
      this._parsedSchema = null;
    }
  }

  // ─── Public API ──────────────────────────────────────────────────────────

  getSchema(): string {
    return this.schema;
  }

  getValue(): object {
    if (!this.value) return {};
    try {
      return JSON.parse(this.value);
    } catch {
      return {};
    }
  }

  validate(): boolean {
    // Placeholder — will be implemented in form-mode
    return true;
  }

  // ─── Render ──────────────────────────────────────────────────────────────

  override render() {
    if (!this._parsedSchema) {
      return html`<slot></slot>`;
    }

    switch (this.mode) {
      case 'edit':
        return html`<div class="edit-mode-placeholder">Edit mode — Task 7+</div>`;
      case 'form':
        return html`<div class="form-mode-placeholder">Form mode — Task 10+</div>`;
      case 'search':
        return html`<div class="search-mode-placeholder">Search mode — Task 12+</div>`;
      default:
        return nothing;
    }
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'schema-editor': SchemaEditor;
  }
}
```

- [ ] **Step 2: Register the component in main.js**

Add this import at the top of `src/main.js` (after the existing web component imports around line 59):
```javascript
import './schema-editor/schema-editor.ts';
```

- [ ] **Step 3: Build and verify no errors**

Run:
```bash
npm run build-js
```
Expected: Build succeeds with no errors.

- [ ] **Step 4: Commit**

```bash
git add src/schema-editor/schema-editor.ts src/main.js
git commit -m "feat: add <schema-editor> shell component with mode routing"
```

---

### Task 6: Standalone test page

**Files:**
- Create: `src/schema-editor/test.html`

- [ ] **Step 1: Create the test page**

Create `src/schema-editor/test.html`:
```html
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Schema Editor — Standalone Test</title>
  <script type="module" src="./schema-editor.ts"></script>
  <style>
    body { font-family: system-ui, sans-serif; max-width: 1200px; margin: 0 auto; padding: 20px; background: #f5f5f5; }
    h1 { font-size: 18px; }
    h2 { font-size: 14px; margin-top: 24px; }
    section { background: white; border: 1px solid #e5e7eb; border-radius: 8px; padding: 16px; margin-bottom: 16px; }
    pre { background: #f9fafb; padding: 8px; border-radius: 4px; font-size: 11px; overflow-x: auto; }
    #event-log { max-height: 200px; overflow-y: auto; }
  </style>
</head>
<body>
  <h1>Schema Editor — Standalone Test Page</h1>
  <p>This page loads the <code>&lt;schema-editor&gt;</code> component via Vite dev server. No Go backend or Alpine.js required.</p>

  <h2>Edit Mode</h2>
  <section>
    <schema-editor
      id="editor-edit"
      mode="edit"
      schema='{"type":"object","properties":{"name":{"type":"string","minLength":1},"status":{"type":"string","enum":["active","inactive","pending"]},"age":{"type":"integer","minimum":0,"maximum":150},"email":{"type":"string","format":"email"}},"required":["name","status"]}'
    ></schema-editor>
  </section>

  <h2>Form Mode</h2>
  <section>
    <schema-editor
      id="editor-form"
      mode="form"
      schema='{"type":"object","properties":{"name":{"type":"string","minLength":1},"status":{"type":"string","enum":["active","inactive","pending"]},"age":{"type":"integer","minimum":0}},"required":["name"]}'
      value='{"name":"Alice","status":"active","age":30}'
      name="Meta"
    ></schema-editor>
  </section>

  <h2>Search Mode</h2>
  <section>
    <schema-editor
      id="editor-search"
      mode="search"
      schema='{"type":"object","properties":{"name":{"type":"string"},"status":{"type":"string","enum":["active","inactive","pending"]},"weight":{"type":"number"}}}'
      meta-query='[]'
      field-name="MetaQuery"
    ></schema-editor>
  </section>

  <h2>Event Log</h2>
  <section>
    <pre id="event-log"></pre>
  </section>

  <script type="module">
    const log = document.getElementById('event-log');
    function logEvent(name, detail) {
      const line = `[${new Date().toISOString().slice(11,19)}] ${name}: ${JSON.stringify(detail)}\n`;
      log.textContent = line + log.textContent;
    }
    document.querySelectorAll('schema-editor').forEach(el => {
      el.addEventListener('schema-change', e => logEvent('schema-change', e.detail));
      el.addEventListener('value-change', e => logEvent('value-change', e.detail));
      el.addEventListener('schema-fields-claimed', e => logEvent('schema-fields-claimed', e.detail));
    });
  </script>
</body>
</html>
```

- [ ] **Step 2: Verify the test page works with Vite dev server**

Run:
```bash
cd /Users/egecan/Code/mahresources && npx vite src/schema-editor --open
```
Expected: Browser opens showing the test page with three sections. Each shows a placeholder for its mode. No console errors.

Stop the dev server with Ctrl+C.

- [ ] **Step 3: Commit**

```bash
git add src/schema-editor/test.html
git commit -m "feat: add standalone test page for schema editor component"
```

---

### Task 7: Tree panel component

**Files:**
- Create: `src/schema-editor/tree/tree-panel.ts`

- [ ] **Step 1: Implement tree-panel.ts**

Create `src/schema-editor/tree/tree-panel.ts`:
```typescript
import { LitElement, html, css } from 'lit';
import { customElement, property, state } from 'lit/decorators.js';
import { repeat } from 'lit/directives/repeat.js';
import { classMap } from 'lit/directives/class-map.js';
import { sharedStyles } from '../styles';
import type { SchemaNode } from '../schema-tree-model';

@customElement('schema-tree-panel')
export class SchemaTreePanel extends LitElement {
  static override styles = [
    sharedStyles,
    css`
      :host { display: flex; flex-direction: column; height: 100%; }
      .toolbar {
        padding: 8px 12px;
        border-bottom: 1px solid #e5e7eb;
        display: flex;
        gap: 4px;
        align-items: center;
      }
      .toolbar .spacer { flex: 1; }
      .toolbar .draft { font-size: 10px; color: #9ca3af; }
      .tree { flex: 1; overflow-y: auto; padding: 8px 0; }
      .tree-node {
        padding: 4px 12px;
        display: flex;
        align-items: center;
        gap: 6px;
        cursor: pointer;
        border-radius: 4px;
        margin: 1px 4px;
        font-size: 12px;
        user-select: none;
      }
      .tree-node:hover { background: #f3f4f6; }
      .tree-node:focus-visible { outline: 2px solid #6366f1; outline-offset: -2px; }
      .tree-node.selected { background: #eef2ff; border: 1px solid #c7d2fe; }
      .tree-node.selected .node-name { font-weight: 600; color: #4338ca; }
      .node-name { color: #1f2937; flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
      .expand-icon { color: #9ca3af; font-size: 10px; width: 12px; text-align: center; flex-shrink: 0; }
      .drag-handle { color: #9ca3af; font-size: 10px; cursor: grab; flex-shrink: 0; }
      .children { margin-left: 16px; }
      .defs-section { margin-top: 8px; border-top: 1px solid #e5e7eb; padding-top: 8px; }
      .defs-header {
        padding: 4px 12px;
        font-size: 12px;
        color: #6b7280;
        display: flex;
        align-items: center;
        gap: 4px;
      }
      .defs-header .count { font-size: 10px; color: #9ca3af; margin-left: auto; }
    `,
  ];

  @property({ type: Object }) root: SchemaNode | null = null;
  @property({ type: String }) selectedId = '';
  @property({ type: String }) draft: string | null = null;

  @state() private _expanded = new Set<string>();

  override connectedCallback() {
    super.connectedCallback();
    // Expand root by default
    if (this.root) {
      this._expanded.add(this.root.id);
    }
  }

  override willUpdate(changed: Map<string, unknown>) {
    if (changed.has('root') && this.root) {
      this._expanded.add(this.root.id);
    }
  }

  private _toggleExpand(nodeId: string) {
    if (this._expanded.has(nodeId)) {
      this._expanded.delete(nodeId);
    } else {
      this._expanded.add(nodeId);
    }
    this.requestUpdate();
  }

  private _selectNode(nodeId: string) {
    this.dispatchEvent(new CustomEvent('node-select', { detail: { nodeId }, bubbles: true, composed: true }));
  }

  private _handleKeydown(e: KeyboardEvent, node: SchemaNode) {
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault();
      this._selectNode(node.id);
    }
    if (e.key === 'ArrowRight' && node.children?.length) {
      e.preventDefault();
      this._expanded.add(node.id);
      this.requestUpdate();
    }
    if (e.key === 'ArrowLeft') {
      e.preventDefault();
      this._expanded.delete(node.id);
      this.requestUpdate();
    }
  }

  private _addProperty() {
    this.dispatchEvent(new CustomEvent('add-property', { bubbles: true, composed: true }));
  }

  private _addDefs() {
    this.dispatchEvent(new CustomEvent('add-defs', { bubbles: true, composed: true }));
  }

  private _getBadgeClass(node: SchemaNode): string {
    if (node.ref) return 'badge badge-ref';
    if (node.compositionKeyword) return 'badge badge-composition';
    if (node.schema?.enum) return 'badge badge-enum';
    if (node.schema?.if) return 'badge badge-conditional';
    return `badge badge-${node.type || 'string'}`;
  }

  private _getBadgeText(node: SchemaNode): string {
    if (node.ref) return '$ref';
    if (node.compositionKeyword) return node.compositionKeyword;
    if (node.schema?.enum) return 'enum';
    if (node.schema?.if) return 'if/then';
    return node.type || 'string';
  }

  private _hasChildren(node: SchemaNode): boolean {
    const propChildren = (node.children || []).filter(c => c.name !== '$defs');
    return propChildren.length > 0;
  }

  private _renderNode(node: SchemaNode, isRoot = false): unknown {
    const propChildren = (node.children || []).filter(c => c.name !== '$defs' && !c.isDef);
    const defsNode = (node.children || []).find(c => c.name === '$defs');
    const expanded = this._expanded.has(node.id);
    const selected = this.selectedId === node.id;
    const hasChildren = propChildren.length > 0;

    return html`
      <div
        class=${classMap({ 'tree-node': true, selected })}
        role="treeitem"
        tabindex=${selected ? '0' : '-1'}
        aria-selected=${selected}
        aria-expanded=${hasChildren ? String(expanded) : 'undefined'}
        @click=${() => this._selectNode(node.id)}
        @dblclick=${() => hasChildren && this._toggleExpand(node.id)}
        @keydown=${(e: KeyboardEvent) => this._handleKeydown(e, node)}
      >
        ${hasChildren
          ? html`<span class="expand-icon" @click=${(e: Event) => { e.stopPropagation(); this._toggleExpand(node.id); }}>${expanded ? '▼' : '▶'}</span>`
          : html`<span class="expand-icon"></span>`}
        ${!isRoot ? html`<span class="drag-handle" aria-hidden="true">☰</span>` : ''}
        <span class="node-name">${isRoot ? 'root' : node.name}</span>
        ${node.required ? html`<span class="required-marker" aria-label="required">*</span>` : ''}
        <span class=${this._getBadgeClass(node)}>${this._getBadgeText(node)}</span>
      </div>
      ${hasChildren && expanded
        ? html`<div class="children" role="group">
            ${repeat(propChildren, c => c.id, c => this._renderNode(c))}
          </div>`
        : ''}
      ${isRoot && defsNode && defsNode.children?.length
        ? html`
          <div class="defs-section">
            <div class="defs-header">
              <span class="expand-icon" @click=${() => this._toggleExpand(defsNode.id)}>${this._expanded.has(defsNode.id) ? '▼' : '▶'}</span>
              <span style="font-weight:600;color:#6b7280;">$defs</span>
              <span class="count">${defsNode.children.length} definition${defsNode.children.length !== 1 ? 's' : ''}</span>
            </div>
            ${this._expanded.has(defsNode.id)
              ? html`<div class="children" role="group">
                  ${repeat(defsNode.children, c => c.id, c => this._renderNode(c))}
                </div>`
              : ''}
          </div>`
        : ''}
    `;
  }

  override render() {
    if (!this.root) return html`<div>No schema loaded</div>`;

    return html`
      <div class="toolbar">
        <button class="btn" @click=${this._addProperty}>+ Property</button>
        <button class="btn" @click=${this._addDefs}>+ $defs</button>
        <span class="spacer"></span>
        ${this.draft ? html`<span class="draft">${this.draft}</span>` : ''}
      </div>
      <div class="tree" role="tree" aria-label="Schema structure">
        ${this._renderNode(this.root, true)}
      </div>
    `;
  }
}
```

- [ ] **Step 2: Build and verify**

Run:
```bash
npm run build-js
```
Expected: Build succeeds.

- [ ] **Step 3: Commit**

```bash
git add src/schema-editor/tree/tree-panel.ts
git commit -m "feat: add tree panel component for schema editor"
```

---

### Task 8: Detail panel and node editors

**Files:**
- Create: `src/schema-editor/tree/detail-panel.ts`
- Create: `src/schema-editor/tree/node-editors/string-editor.ts`
- Create: `src/schema-editor/tree/node-editors/number-editor.ts`
- Create: `src/schema-editor/tree/node-editors/boolean-editor.ts`
- Create: `src/schema-editor/tree/node-editors/object-editor.ts`
- Create: `src/schema-editor/tree/node-editors/array-editor.ts`
- Create: `src/schema-editor/tree/node-editors/enum-editor.ts`
- Create: `src/schema-editor/tree/node-editors/composition-editor.ts`
- Create: `src/schema-editor/tree/node-editors/conditional-editor.ts`
- Create: `src/schema-editor/tree/node-editors/ref-editor.ts`

This is a large task. Due to its size and the number of files, the implementation details for each node editor follow the same pattern. I'll show the detail panel and two representative editors (string and composition). The remaining editors follow the same Lit component pattern with type-specific fields.

- [ ] **Step 1: Create detail-panel.ts**

Create `src/schema-editor/tree/detail-panel.ts`:
```typescript
import { LitElement, html, css, nothing } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import { sharedStyles } from '../styles';
import type { SchemaNode } from '../schema-tree-model';

// Import all node editors (registers them as custom elements)
import './node-editors/string-editor';
import './node-editors/number-editor';
import './node-editors/boolean-editor';
import './node-editors/object-editor';
import './node-editors/array-editor';
import './node-editors/enum-editor';
import './node-editors/composition-editor';
import './node-editors/conditional-editor';
import './node-editors/ref-editor';

@customElement('schema-detail-panel')
export class SchemaDetailPanel extends LitElement {
  static override styles = [
    sharedStyles,
    css`
      :host { display: block; padding: 20px; overflow-y: auto; height: 100%; }
      .header { margin-bottom: 16px; }
      .header h3 { margin: 0; font-size: 16px; }
      .grid { display: grid; grid-template-columns: 1fr 1fr; gap: 12px; margin-bottom: 16px; }
      .grid-full { grid-column: span 2; }
      .flags {
        display: flex; gap: 16px; padding: 12px;
        background: #f9fafb; border-radius: 6px; margin-bottom: 16px;
      }
      .flags label { display: flex; align-items: center; gap: 6px; font-size: 12px; color: #374151; font-weight: normal; }
      .type-section { border: 1px solid #e5e7eb; border-radius: 6px; padding: 16px; margin-bottom: 16px; }
      .type-section h4 { margin: 0 0 12px; font-size: 13px; font-weight: 600; }
      .actions {
        display: flex; gap: 8px; padding-top: 12px;
        border-top: 1px solid #e5e7eb;
      }
    `,
  ];

  @property({ type: Object }) node: SchemaNode | null = null;
  @property({ type: Array }) breadcrumb: string[] = [];
  @property({ type: Array }) defsNames: string[] = [];
  @property({ type: Boolean }) isRoot = false;

  private _dispatchChange(field: string, value: any) {
    this.dispatchEvent(new CustomEvent('node-change', {
      detail: { field, value },
      bubbles: true,
      composed: true,
    }));
  }

  private _dispatchDelete() {
    this.dispatchEvent(new CustomEvent('node-delete', { bubbles: true, composed: true }));
  }

  private _dispatchDuplicate() {
    this.dispatchEvent(new CustomEvent('node-duplicate', { bubbles: true, composed: true }));
  }

  private _renderTypeEditor() {
    if (!this.node) return nothing;
    const schema = this.node.schema;

    // Enum editor is type-independent (any type can have enum)
    if (schema.enum) {
      return html`<schema-enum-editor .values=${schema.enum} .valueType=${this.node.type} @enum-change=${(e: CustomEvent) => this._dispatchChange('enum', e.detail.values)}></schema-enum-editor>`;
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

  override render() {
    if (!this.node) {
      return html`<div style="display:flex;align-items:center;justify-content:center;height:100%;color:#9ca3af;">Select a node from the tree</div>`;
    }

    const node = this.node;
    const schema = node.schema;

    // $ref nodes get a special editor
    if (node.ref) {
      return html`
        <div class="header">
          <div class="breadcrumb">${this.breadcrumb.join(' → ')}</div>
          <h3>Reference: ${node.name}</h3>
        </div>
        <schema-ref-editor .ref=${node.ref} .defsNames=${this.defsNames} @ref-change=${(e: CustomEvent) => this._dispatchChange('$ref', e.detail.ref)}></schema-ref-editor>
      `;
    }

    // Composition nodes
    if (node.compositionKeyword) {
      return html`
        <div class="header">
          <div class="breadcrumb">${this.breadcrumb.join(' → ')}</div>
          <h3>${node.compositionKeyword}: ${node.name}</h3>
        </div>
        <schema-composition-editor .keyword=${node.compositionKeyword} .variants=${node.children || []}></schema-composition-editor>
      `;
    }

    // Conditional nodes
    if (schema.if) {
      return html`
        <div class="header">
          <div class="breadcrumb">${this.breadcrumb.join(' → ')}</div>
          <h3>Conditional: ${node.name}</h3>
        </div>
        <schema-conditional-editor .schema=${schema}></schema-conditional-editor>
      `;
    }

    const allTypes = ['string', 'integer', 'number', 'boolean', 'object', 'array', 'null'];

    return html`
      <div class="header">
        <div class="breadcrumb">${this.breadcrumb.slice(0, -1).join(' → ')}${this.breadcrumb.length > 1 ? ' → ' : ''}<span class="current">${this.breadcrumb.at(-1) || 'root'}</span></div>
        <h3>${this.isRoot ? 'Root Schema' : `Property: ${node.name}`}</h3>
      </div>

      <div class="grid">
        ${!this.isRoot ? html`
          <div>
            <label for="prop-name">Property Name</label>
            <input id="prop-name" .value=${node.name} @change=${(e: Event) => this._dispatchChange('name', (e.target as HTMLInputElement).value)}>
          </div>
        ` : ''}
        <div>
          <label for="prop-type">Type</label>
          <select id="prop-type" .value=${node.type} @change=${(e: Event) => this._dispatchChange('type', (e.target as HTMLSelectElement).value)}>
            ${allTypes.map(t => html`<option .value=${t} ?selected=${t === node.type}>${t}</option>`)}
          </select>
        </div>
        <div>
          <label for="prop-title">Title</label>
          <input id="prop-title" .value=${schema.title || ''} @change=${(e: Event) => this._dispatchChange('title', (e.target as HTMLInputElement).value)}>
        </div>
        <div>
          <label for="prop-desc">Description</label>
          <input id="prop-desc" .value=${schema.description || ''} @change=${(e: Event) => this._dispatchChange('description', (e.target as HTMLInputElement).value)}>
        </div>
      </div>

      <div class="flags">
        ${!this.isRoot ? html`
          <label><input type="checkbox" ?checked=${node.required} @change=${(e: Event) => this._dispatchChange('required', (e.target as HTMLInputElement).checked)}> Required</label>
        ` : ''}
        <label><input type="checkbox" ?checked=${schema.readOnly} @change=${(e: Event) => this._dispatchChange('readOnly', (e.target as HTMLInputElement).checked)}> Read Only</label>
        <label><input type="checkbox" ?checked=${schema.writeOnly} @change=${(e: Event) => this._dispatchChange('writeOnly', (e.target as HTMLInputElement).checked)}> Write Only</label>
      </div>

      ${this._renderTypeEditor()}

      ${!this.isRoot ? html`
        <div class="actions">
          <button class="btn btn-danger" @click=${this._dispatchDelete}>Delete Property</button>
          <button class="btn" @click=${this._dispatchDuplicate}>Duplicate</button>
        </div>
      ` : ''}
    `;
  }
}
```

- [ ] **Step 2: Create string-editor.ts**

Create `src/schema-editor/tree/node-editors/string-editor.ts`:
```typescript
import { LitElement, html, css } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import { sharedStyles } from '../../styles';
import type { JSONSchema } from '../../schema-core';

const STRING_FORMATS = [
  '', 'date', 'date-time', 'time', 'email', 'uri', 'uri-reference',
  'uuid', 'hostname', 'ipv4', 'ipv6', 'regex', 'json-pointer',
];

@customElement('schema-string-editor')
export class SchemaStringEditor extends LitElement {
  static override styles = [sharedStyles, css`
    .grid { display: grid; grid-template-columns: 1fr 1fr; gap: 10px; }
  `];

  @property({ type: Object }) schema: JSONSchema = {};

  private _emit(field: string, value: any) {
    this.dispatchEvent(new CustomEvent('constraint-change', {
      detail: { field, value: value === '' ? undefined : value },
      bubbles: true, composed: true,
    }));
  }

  override render() {
    return html`
      <div class="type-section">
        <h4>String Constraints</h4>
        <div class="grid">
          <div>
            <label>Min Length</label>
            <input type="number" min="0" .value=${this.schema.minLength ?? ''} @change=${(e: Event) => {
              const v = (e.target as HTMLInputElement).value;
              this._emit('minLength', v ? parseInt(v) : undefined);
            }}>
          </div>
          <div>
            <label>Max Length</label>
            <input type="number" min="0" .value=${this.schema.maxLength ?? ''} @change=${(e: Event) => {
              const v = (e.target as HTMLInputElement).value;
              this._emit('maxLength', v ? parseInt(v) : undefined);
            }}>
          </div>
          <div>
            <label>Pattern (regex)</label>
            <input .value=${this.schema.pattern || ''} @change=${(e: Event) => this._emit('pattern', (e.target as HTMLInputElement).value)}>
          </div>
          <div>
            <label>Format</label>
            <select .value=${this.schema.format || ''} @change=${(e: Event) => this._emit('format', (e.target as HTMLSelectElement).value)}>
              ${STRING_FORMATS.map(f => html`<option .value=${f} ?selected=${f === (this.schema.format || '')}>${f || '(none)'}</option>`)}
            </select>
          </div>
          <div>
            <label>Default</label>
            <input .value=${this.schema.default ?? ''} @change=${(e: Event) => this._emit('default', (e.target as HTMLInputElement).value)}>
          </div>
          <div>
            <label>Const</label>
            <input .value=${this.schema.const ?? ''} @change=${(e: Event) => this._emit('const', (e.target as HTMLInputElement).value)}>
          </div>
        </div>
      </div>
    `;
  }
}
```

- [ ] **Step 3: Create number-editor.ts**

Create `src/schema-editor/tree/node-editors/number-editor.ts`:
```typescript
import { LitElement, html, css } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import { sharedStyles } from '../../styles';
import type { JSONSchema } from '../../schema-core';

@customElement('schema-number-editor')
export class SchemaNumberEditor extends LitElement {
  static override styles = [sharedStyles, css`
    .grid { display: grid; grid-template-columns: 1fr 1fr; gap: 10px; }
  `];

  @property({ type: Object }) schema: JSONSchema = {};
  @property({ type: Boolean }) integerOnly = false;

  private _emit(field: string, value: any) {
    this.dispatchEvent(new CustomEvent('constraint-change', {
      detail: { field, value: value === '' ? undefined : value },
      bubbles: true, composed: true,
    }));
  }

  private _parseNum(val: string): number | undefined {
    if (val === '') return undefined;
    return this.integerOnly ? parseInt(val) : parseFloat(val);
  }

  override render() {
    const step = this.integerOnly ? '1' : 'any';
    return html`
      <div class="type-section">
        <h4>${this.integerOnly ? 'Integer' : 'Number'} Constraints</h4>
        <div class="grid">
          <div>
            <label>Minimum</label>
            <input type="number" step=${step} .value=${this.schema.minimum ?? ''} @change=${(e: Event) => this._emit('minimum', this._parseNum((e.target as HTMLInputElement).value))}>
          </div>
          <div>
            <label>Maximum</label>
            <input type="number" step=${step} .value=${this.schema.maximum ?? ''} @change=${(e: Event) => this._emit('maximum', this._parseNum((e.target as HTMLInputElement).value))}>
          </div>
          <div>
            <label>Exclusive Minimum</label>
            <input type="number" step=${step} .value=${this.schema.exclusiveMinimum ?? ''} @change=${(e: Event) => this._emit('exclusiveMinimum', this._parseNum((e.target as HTMLInputElement).value))}>
          </div>
          <div>
            <label>Exclusive Maximum</label>
            <input type="number" step=${step} .value=${this.schema.exclusiveMaximum ?? ''} @change=${(e: Event) => this._emit('exclusiveMaximum', this._parseNum((e.target as HTMLInputElement).value))}>
          </div>
          <div>
            <label>Multiple Of</label>
            <input type="number" step=${step} .value=${this.schema.multipleOf ?? ''} @change=${(e: Event) => this._emit('multipleOf', this._parseNum((e.target as HTMLInputElement).value))}>
          </div>
          <div>
            <label>Default</label>
            <input type="number" step=${step} .value=${this.schema.default ?? ''} @change=${(e: Event) => this._emit('default', this._parseNum((e.target as HTMLInputElement).value))}>
          </div>
        </div>
      </div>
    `;
  }
}
```

- [ ] **Step 4: Create boolean-editor.ts**

Create `src/schema-editor/tree/node-editors/boolean-editor.ts`:
```typescript
import { LitElement, html, css } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import { sharedStyles } from '../../styles';
import type { JSONSchema } from '../../schema-core';

@customElement('schema-boolean-editor')
export class SchemaBooleanEditor extends LitElement {
  static override styles = [sharedStyles, css`
    .grid { display: grid; grid-template-columns: 1fr 1fr; gap: 10px; }
  `];

  @property({ type: Object }) schema: JSONSchema = {};

  private _emit(field: string, value: any) {
    this.dispatchEvent(new CustomEvent('constraint-change', {
      detail: { field, value },
      bubbles: true, composed: true,
    }));
  }

  override render() {
    return html`
      <div class="type-section">
        <h4>Boolean Constraints</h4>
        <div class="grid">
          <div>
            <label>Default</label>
            <select .value=${this.schema.default === undefined ? '' : String(this.schema.default)} @change=${(e: Event) => {
              const v = (e.target as HTMLSelectElement).value;
              this._emit('default', v === '' ? undefined : v === 'true');
            }}>
              <option value="">-- none --</option>
              <option value="true" ?selected=${this.schema.default === true}>true</option>
              <option value="false" ?selected=${this.schema.default === false}>false</option>
            </select>
          </div>
          <div>
            <label>Const</label>
            <select .value=${this.schema.const === undefined ? '' : String(this.schema.const)} @change=${(e: Event) => {
              const v = (e.target as HTMLSelectElement).value;
              this._emit('const', v === '' ? undefined : v === 'true');
            }}>
              <option value="">-- none --</option>
              <option value="true" ?selected=${this.schema.const === true}>true</option>
              <option value="false" ?selected=${this.schema.const === false}>false</option>
            </select>
          </div>
        </div>
      </div>
    `;
  }
}
```

- [ ] **Step 5: Create object-editor.ts**

Create `src/schema-editor/tree/node-editors/object-editor.ts`:
```typescript
import { LitElement, html, css } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import { sharedStyles } from '../../styles';
import type { JSONSchema } from '../../schema-core';

@customElement('schema-object-editor')
export class SchemaObjectEditor extends LitElement {
  static override styles = [sharedStyles, css`
    .grid { display: grid; grid-template-columns: 1fr 1fr; gap: 10px; }
  `];

  @property({ type: Object }) schema: JSONSchema = {};

  private _emit(field: string, value: any) {
    this.dispatchEvent(new CustomEvent('constraint-change', {
      detail: { field, value: value === '' ? undefined : value },
      bubbles: true, composed: true,
    }));
  }

  override render() {
    const addlProps = this.schema.additionalProperties;
    const addlValue = addlProps === false ? 'false' : addlProps === true ? 'true' : '';

    return html`
      <div class="type-section">
        <h4>Object Constraints</h4>
        <div class="grid">
          <div>
            <label>Additional Properties</label>
            <select .value=${addlValue} @change=${(e: Event) => {
              const v = (e.target as HTMLSelectElement).value;
              this._emit('additionalProperties', v === '' ? undefined : v === 'true');
            }}>
              <option value="">-- default (true) --</option>
              <option value="true" ?selected=${addlValue === 'true'}>Allowed</option>
              <option value="false" ?selected=${addlValue === 'false'}>Forbidden</option>
            </select>
          </div>
          <div>
            <label>Min Properties</label>
            <input type="number" min="0" .value=${this.schema.minProperties ?? ''} @change=${(e: Event) => {
              const v = (e.target as HTMLInputElement).value;
              this._emit('minProperties', v ? parseInt(v) : undefined);
            }}>
          </div>
          <div>
            <label>Max Properties</label>
            <input type="number" min="0" .value=${this.schema.maxProperties ?? ''} @change=${(e: Event) => {
              const v = (e.target as HTMLInputElement).value;
              this._emit('maxProperties', v ? parseInt(v) : undefined);
            }}>
          </div>
        </div>
      </div>
    `;
  }
}
```

- [ ] **Step 6: Create array-editor.ts**

Create `src/schema-editor/tree/node-editors/array-editor.ts`:
```typescript
import { LitElement, html, css } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import { sharedStyles } from '../../styles';
import type { JSONSchema } from '../../schema-core';

@customElement('schema-array-editor')
export class SchemaArrayEditor extends LitElement {
  static override styles = [sharedStyles, css`
    .grid { display: grid; grid-template-columns: 1fr 1fr; gap: 10px; }
  `];

  @property({ type: Object }) schema: JSONSchema = {};

  private _emit(field: string, value: any) {
    this.dispatchEvent(new CustomEvent('constraint-change', {
      detail: { field, value: value === '' ? undefined : value },
      bubbles: true, composed: true,
    }));
  }

  override render() {
    return html`
      <div class="type-section">
        <h4>Array Constraints</h4>
        <div class="grid">
          <div>
            <label>Min Items</label>
            <input type="number" min="0" .value=${this.schema.minItems ?? ''} @change=${(e: Event) => {
              const v = (e.target as HTMLInputElement).value;
              this._emit('minItems', v ? parseInt(v) : undefined);
            }}>
          </div>
          <div>
            <label>Max Items</label>
            <input type="number" min="0" .value=${this.schema.maxItems ?? ''} @change=${(e: Event) => {
              const v = (e.target as HTMLInputElement).value;
              this._emit('maxItems', v ? parseInt(v) : undefined);
            }}>
          </div>
          <div>
            <label><input type="checkbox" ?checked=${this.schema.uniqueItems} @change=${(e: Event) => this._emit('uniqueItems', (e.target as HTMLInputElement).checked || undefined)}> Unique Items</label>
          </div>
        </div>
      </div>
    `;
  }
}
```

- [ ] **Step 7: Create enum-editor.ts**

Create `src/schema-editor/tree/node-editors/enum-editor.ts`:
```typescript
import { LitElement, html, css } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import { repeat } from 'lit/directives/repeat.js';
import { sharedStyles } from '../../styles';

@customElement('schema-enum-editor')
export class SchemaEnumEditor extends LitElement {
  static override styles = [sharedStyles, css`
    .enum-row { display: flex; align-items: center; gap: 6px; margin-bottom: 6px; }
    .enum-row input { flex: 1; }
    .drag { color: #9ca3af; cursor: grab; font-size: 10px; }
    .remove { color: #dc2626; background: none; border: none; font-size: 14px; padding: 0 4px; }
  `];

  @property({ type: Array }) values: any[] = [];
  @property({ type: String }) valueType = 'string';

  private _emit() {
    this.dispatchEvent(new CustomEvent('enum-change', {
      detail: { values: [...this.values] },
      bubbles: true, composed: true,
    }));
  }

  private _updateValue(index: number, raw: string) {
    if (this.valueType === 'number' || this.valueType === 'integer') {
      this.values[index] = this.valueType === 'integer' ? parseInt(raw) : parseFloat(raw);
    } else {
      this.values[index] = raw;
    }
    this._emit();
  }

  private _removeValue(index: number) {
    this.values.splice(index, 1);
    this._emit();
    this.requestUpdate();
  }

  private _addValue() {
    this.values.push(this.valueType === 'number' || this.valueType === 'integer' ? 0 : '');
    this._emit();
    this.requestUpdate();
  }

  override render() {
    return html`
      <div class="type-section">
        <h4>Enum Values</h4>
        ${repeat(this.values, (_v, i) => i, (v, i) => html`
          <div class="enum-row">
            <span class="drag" aria-hidden="true">☰</span>
            <input
              .value=${String(v)}
              type=${this.valueType === 'number' || this.valueType === 'integer' ? 'number' : 'text'}
              step=${this.valueType === 'integer' ? '1' : 'any'}
              @change=${(e: Event) => this._updateValue(i, (e.target as HTMLInputElement).value)}
              aria-label="Enum value ${i + 1}"
            >
            <button class="remove" @click=${() => this._removeValue(i)} aria-label="Remove value ${v}">×</button>
          </div>
        `)}
        <button class="btn-ghost" @click=${this._addValue}>+ Add Value</button>
      </div>
    `;
  }
}
```

- [ ] **Step 8: Create composition-editor.ts**

Create `src/schema-editor/tree/node-editors/composition-editor.ts`:
```typescript
import { LitElement, html, css } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import { sharedStyles } from '../../styles';
import type { SchemaNode } from '../../schema-tree-model';

@customElement('schema-composition-editor')
export class SchemaCompositionEditor extends LitElement {
  static override styles = [sharedStyles, css`
    .variant { padding: 8px; border: 1px solid #e5e7eb; border-radius: 4px; margin-bottom: 8px; display: flex; align-items: center; gap: 8px; }
    .variant-name { flex: 1; font-size: 12px; }
    .variant-type { font-size: 11px; color: #6b7280; }
  `];

  @property({ type: String }) keyword: string = 'oneOf';
  @property({ type: Array }) variants: SchemaNode[] = [];

  private _addVariant() {
    this.dispatchEvent(new CustomEvent('add-variant', { bubbles: true, composed: true }));
  }

  private _removeVariant(index: number) {
    this.dispatchEvent(new CustomEvent('remove-variant', {
      detail: { index },
      bubbles: true, composed: true,
    }));
  }

  override render() {
    return html`
      <div class="type-section">
        <h4>${this.keyword} — ${this.variants.length} variant${this.variants.length !== 1 ? 's' : ''}</h4>
        ${this.variants.map((v, i) => html`
          <div class="variant">
            <span class="variant-name">${v.schema.title || v.name || `Variant ${i + 1}`}</span>
            <span class="variant-type">(${v.type})</span>
            <button class="btn btn-danger" @click=${() => this._removeVariant(i)} aria-label="Remove variant ${i + 1}">×</button>
          </div>
        `)}
        <button class="btn-ghost" @click=${this._addVariant}>+ Add Variant</button>
      </div>
    `;
  }
}
```

- [ ] **Step 9: Create conditional-editor.ts**

Create `src/schema-editor/tree/node-editors/conditional-editor.ts`:
```typescript
import { LitElement, html, css } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import { sharedStyles } from '../../styles';
import type { JSONSchema } from '../../schema-core';

@customElement('schema-conditional-editor')
export class SchemaConditionalEditor extends LitElement {
  static override styles = [sharedStyles, css`
    .slot { padding: 12px; border: 1px solid #e5e7eb; border-radius: 4px; margin-bottom: 8px; }
    .slot-label { font-size: 11px; font-weight: 600; color: #6b7280; margin-bottom: 4px; }
    .slot-content { font-size: 12px; color: #374151; font-family: monospace; white-space: pre-wrap; max-height: 100px; overflow-y: auto; }
  `];

  @property({ type: Object }) schema: JSONSchema = {};

  override render() {
    return html`
      <div class="type-section">
        <h4>Conditional (if / then / else)</h4>
        <div class="slot">
          <div class="slot-label">if</div>
          <div class="slot-content">${JSON.stringify(this.schema.if, null, 2)}</div>
        </div>
        ${this.schema.then ? html`
          <div class="slot">
            <div class="slot-label">then</div>
            <div class="slot-content">${JSON.stringify(this.schema.then, null, 2)}</div>
          </div>
        ` : ''}
        ${this.schema.else ? html`
          <div class="slot">
            <div class="slot-label">else</div>
            <div class="slot-content">${JSON.stringify(this.schema.else, null, 2)}</div>
          </div>
        ` : ''}
        <p style="font-size:11px;color:#9ca3af;margin-top:8px;">Edit conditional schemas via the Raw JSON tab for full control.</p>
      </div>
    `;
  }
}
```

- [ ] **Step 10: Create ref-editor.ts**

Create `src/schema-editor/tree/node-editors/ref-editor.ts`:
```typescript
import { LitElement, html, css } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import { sharedStyles } from '../../styles';

@customElement('schema-ref-editor')
export class SchemaRefEditor extends LitElement {
  static override styles = [sharedStyles, css``];

  @property({ type: String }) ref = '';
  @property({ type: Array }) defsNames: string[] = [];

  private _emit(ref: string) {
    this.dispatchEvent(new CustomEvent('ref-change', {
      detail: { ref },
      bubbles: true, composed: true,
    }));
  }

  override render() {
    return html`
      <div class="type-section">
        <h4>$ref Target</h4>
        <label>Reference</label>
        ${this.defsNames.length > 0
          ? html`
            <select .value=${this.ref} @change=${(e: Event) => this._emit((e.target as HTMLSelectElement).value)}>
              <option value="">-- select --</option>
              ${this.defsNames.map(name => html`<option .value=${'#/$defs/' + name} ?selected=${this.ref === '#/$defs/' + name}>${name}</option>`)}
            </select>
          `
          : html`<input .value=${this.ref} @change=${(e: Event) => this._emit((e.target as HTMLInputElement).value)}>`
        }
      </div>
    `;
  }
}
```

- [ ] **Step 11: Build and verify**

Run:
```bash
npm run build-js
```
Expected: Build succeeds.

- [ ] **Step 12: Commit**

```bash
git add src/schema-editor/tree/
git commit -m "feat: add detail panel and all node type editors for schema editor"
```

---

### Task 9: Edit mode — wire tree + detail into edit-mode.ts

**Files:**
- Create: `src/schema-editor/modes/edit-mode.ts`
- Modify: `src/schema-editor/schema-editor.ts`

- [ ] **Step 1: Create edit-mode.ts**

Create `src/schema-editor/modes/edit-mode.ts`:
```typescript
import { LitElement, html, css } from 'lit';
import { customElement, property, state } from 'lit/decorators.js';
import { sharedStyles } from '../styles';
import { schemaToTree, treeToSchema, detectDraft, resetIdCounter } from '../schema-tree-model';
import type { SchemaNode } from '../schema-tree-model';
import type { JSONSchema } from '../schema-core';
import '../tree/tree-panel';
import '../tree/detail-panel';

@customElement('schema-edit-mode')
export class SchemaEditMode extends LitElement {
  static override styles = [
    sharedStyles,
    css`
      :host { display: flex; height: 100%; }
      .tree-side {
        width: 260px;
        border-right: 1px solid #e5e7eb;
        background: #f9fafb;
        flex-shrink: 0;
        overflow: hidden;
        display: flex;
        flex-direction: column;
      }
      .detail-side {
        flex: 1;
        overflow: hidden;
      }
    `,
  ];

  @property({ type: Object }) schema: JSONSchema = {};

  @state() private _root: SchemaNode | null = null;
  @state() private _selectedId = '';
  @state() private _draft: string | null = null;

  override willUpdate(changed: Map<string, unknown>) {
    if (changed.has('schema') && this.schema) {
      resetIdCounter();
      this._root = schemaToTree(this.schema);
      this._draft = detectDraft(this.schema);
      if (this._root && !this._selectedId) {
        this._selectedId = this._root.id;
      }
    }
  }

  private _findNode(id: string, node: SchemaNode | null = this._root): SchemaNode | null {
    if (!node) return null;
    if (node.id === id) return node;
    for (const child of node.children || []) {
      const found = this._findNode(id, child);
      if (found) return found;
    }
    return null;
  }

  private _buildBreadcrumb(id: string, node: SchemaNode | null = this._root, path: string[] = []): string[] {
    if (!node) return [];
    const current = [...path, node.name || 'root'];
    if (node.id === id) return current;
    for (const child of node.children || []) {
      const result = this._buildBreadcrumb(id, child, current);
      if (result.length) return result;
    }
    return [];
  }

  private _getDefsNames(): string[] {
    if (!this._root) return [];
    const defsNode = (this._root.children || []).find(c => c.name === '$defs');
    return (defsNode?.children || []).map(c => c.name);
  }

  private _emitSchemaChange() {
    if (!this._root) return;
    const schema = treeToSchema(this._root);
    this.dispatchEvent(new CustomEvent('schema-change', {
      detail: { schema: JSON.stringify(schema, null, 2) },
      bubbles: true,
      composed: true,
    }));
  }

  private _handleNodeSelect(e: CustomEvent) {
    this._selectedId = e.detail.nodeId;
  }

  private _handleNodeChange(e: CustomEvent) {
    const selected = this._findNode(this._selectedId);
    if (!selected) return;

    const { field, value } = e.detail;

    switch (field) {
      case 'name':
        selected.name = value;
        break;
      case 'type':
        selected.type = value;
        // Reset type-specific constraints
        for (const key of ['minLength', 'maxLength', 'pattern', 'format', 'minimum', 'maximum',
          'exclusiveMinimum', 'exclusiveMaximum', 'multipleOf', 'minItems', 'maxItems',
          'uniqueItems', 'additionalProperties', 'minProperties', 'maxProperties', 'items', 'enum']) {
          delete selected.schema[key];
        }
        break;
      case 'required':
        selected.required = value;
        break;
      default:
        if (value === undefined) {
          delete selected.schema[field];
        } else {
          selected.schema[field] = value;
        }
    }

    this.requestUpdate();
    this._emitSchemaChange();
  }

  private _handleNodeDelete() {
    if (!this._root) return;
    const parentAndIndex = this._findParentOf(this._selectedId);
    if (!parentAndIndex) return;
    const [parent, index] = parentAndIndex;
    parent.children!.splice(index, 1);
    this._selectedId = parent.id;
    this.requestUpdate();
    this._emitSchemaChange();
  }

  private _handleNodeDuplicate() {
    if (!this._root) return;
    const parentAndIndex = this._findParentOf(this._selectedId);
    if (!parentAndIndex) return;
    const [parent, index] = parentAndIndex;
    const original = parent.children![index];
    const clone = JSON.parse(JSON.stringify(original));
    clone.name = original.name + '_copy';
    clone.id = `node-dup-${Date.now()}`;
    // Regenerate IDs for all children
    const reId = (n: SchemaNode) => { n.id = `node-dup-${Date.now()}-${Math.random()}`; (n.children || []).forEach(reId); };
    reId(clone);
    parent.children!.splice(index + 1, 0, clone);
    this._selectedId = clone.id;
    this.requestUpdate();
    this._emitSchemaChange();
  }

  private _findParentOf(id: string, node: SchemaNode | null = this._root): [SchemaNode, number] | null {
    if (!node || !node.children) return null;
    for (let i = 0; i < node.children.length; i++) {
      if (node.children[i].id === id) return [node, i];
      const found = this._findParentOf(id, node.children[i]);
      if (found) return found;
    }
    return null;
  }

  private _handleAddProperty() {
    if (!this._root) return;
    if (!this._root.children) this._root.children = [];
    let name = 'newProperty';
    let counter = 1;
    const existing = new Set((this._root.children || []).map(c => c.name));
    while (existing.has(name)) name = `newProperty${counter++}`;
    const newNode: SchemaNode = {
      id: `node-new-${Date.now()}`,
      name,
      type: 'string',
      required: false,
      schema: {},
    };
    // Insert before $defs node if present
    const defsIndex = this._root.children.findIndex(c => c.name === '$defs');
    if (defsIndex >= 0) {
      this._root.children.splice(defsIndex, 0, newNode);
    } else {
      this._root.children.push(newNode);
    }
    this._selectedId = newNode.id;
    this.requestUpdate();
    this._emitSchemaChange();
  }

  private _handleAddDefs() {
    if (!this._root) return;
    if (!this._root.children) this._root.children = [];
    let defsNode = this._root.children.find(c => c.name === '$defs');
    if (!defsNode) {
      defsNode = {
        id: `node-defs-${Date.now()}`,
        name: '$defs',
        type: 'object',
        required: false,
        schema: {},
        isDef: true,
        children: [],
      };
      this._root.children.push(defsNode);
    }
    const newDef: SchemaNode = {
      id: `node-def-${Date.now()}`,
      name: 'newDefinition',
      type: 'object',
      required: false,
      schema: {},
      isDef: true,
    };
    defsNode.children!.push(newDef);
    this._selectedId = newDef.id;
    this.requestUpdate();
    this._emitSchemaChange();
  }

  override render() {
    const selected = this._findNode(this._selectedId);
    const breadcrumb = this._buildBreadcrumb(this._selectedId);
    const isRoot = selected === this._root;

    return html`
      <div class="tree-side">
        <schema-tree-panel
          .root=${this._root}
          .selectedId=${this._selectedId}
          .draft=${this._draft}
          @node-select=${this._handleNodeSelect}
          @add-property=${this._handleAddProperty}
          @add-defs=${this._handleAddDefs}
        ></schema-tree-panel>
      </div>
      <div class="detail-side">
        <schema-detail-panel
          .node=${selected}
          .breadcrumb=${breadcrumb}
          .defsNames=${this._getDefsNames()}
          .isRoot=${isRoot}
          @node-change=${this._handleNodeChange}
          @node-delete=${this._handleNodeDelete}
          @node-duplicate=${this._handleNodeDuplicate}
        ></schema-detail-panel>
      </div>
    `;
  }
}
```

- [ ] **Step 2: Wire edit mode into schema-editor.ts**

In `src/schema-editor/schema-editor.ts`, add the import at the top:
```typescript
import './modes/edit-mode';
```

And replace the edit mode placeholder in the `render()` method:
```typescript
case 'edit':
  return html`<schema-edit-mode .schema=${this._parsedSchema} @schema-change=${(e: CustomEvent) => {
    this.schema = e.detail.schema;
    this.dispatchEvent(new CustomEvent('schema-change', { detail: e.detail, bubbles: true, composed: true }));
  }}></schema-edit-mode>`;
```

- [ ] **Step 3: Build and verify**

Run:
```bash
npm run build-js
```
Expected: Build succeeds.

- [ ] **Step 4: Test on standalone page**

Run:
```bash
npx vite src/schema-editor --open
```
Expected: The edit mode section shows the tree panel on the left with schema nodes, and the detail panel on the right. Clicking nodes selects them and shows their properties. Stop with Ctrl+C.

- [ ] **Step 5: Run all unit tests**

Run:
```bash
npx vitest run
```
Expected: All tests pass.

- [ ] **Step 6: Commit**

```bash
git add src/schema-editor/modes/edit-mode.ts src/schema-editor/schema-editor.ts
git commit -m "feat: implement edit mode with tree + detail panel wiring"
```

---

### Task 10: Modal integration — Alpine wrapper + category/resource-type templates

**Files:**
- Create: `src/components/schemaEditorModal.ts`
- Modify: `src/main.js`
- Modify: `templates/createCategory.tpl`
- Modify: `templates/createResourceCategory.tpl`

- [ ] **Step 1: Create the modal Alpine component**

Create `src/components/schemaEditorModal.ts`:
```typescript
/**
 * Alpine.js data component for the schema editor modal.
 * Manages open/close state, tab switching, and sync between
 * the <schema-editor> component and the MetaSchema textarea.
 */
export function schemaEditorModal() {
  return {
    open: false,
    tab: 'edit' as 'edit' | 'preview' | 'raw',
    rawJson: '',
    currentSchema: '',
    /** The textarea element this modal reads/writes to */
    _textareaEl: null as HTMLTextAreaElement | null,

    openModal(textareaId: string) {
      this._textareaEl = document.getElementById(textareaId) as HTMLTextAreaElement;
      this.currentSchema = this._textareaEl?.value || '{"type":"object","properties":{}}';
      this.rawJson = this.currentSchema;
      try {
        // Pretty-print for raw tab
        this.rawJson = JSON.stringify(JSON.parse(this.currentSchema), null, 2);
      } catch { /* keep as-is */ }
      this.tab = 'edit';
      this.open = true;
      // Trap focus after render
      this.$nextTick(() => {
        const modal = this.$refs.modalContent as HTMLElement;
        modal?.querySelector<HTMLElement>('[autofocus], button, input, select')?.focus();
      });
    },

    closeModal() {
      this.open = false;
      // Return focus to trigger button
      this._textareaEl?.closest('.meta-schema-field')?.querySelector<HTMLElement>('.visual-editor-btn')?.focus();
    },

    handleSchemaChange(e: CustomEvent) {
      this.currentSchema = e.detail.schema;
      try {
        this.rawJson = JSON.stringify(JSON.parse(this.currentSchema), null, 2);
      } catch {
        this.rawJson = this.currentSchema;
      }
    },

    handleRawChange() {
      try {
        JSON.parse(this.rawJson);
        this.currentSchema = this.rawJson;
      } catch { /* invalid JSON — don't sync */ }
    },

    applySchema() {
      if (this._textareaEl) {
        // Minify for storage
        try {
          this._textareaEl.value = JSON.stringify(JSON.parse(this.currentSchema));
        } catch {
          this._textareaEl.value = this.currentSchema;
        }
        // Trigger input event for any watchers
        this._textareaEl.dispatchEvent(new Event('input', { bubbles: true }));
      }
      this.closeModal();
    },

    handleKeydown(e: KeyboardEvent) {
      if (e.key === 'Escape') {
        this.closeModal();
      }
    },

    getPropertyCount() {
      try {
        const schema = JSON.parse(this.currentSchema);
        const props = schema.properties ? Object.keys(schema.properties).length : 0;
        const req = schema.required ? schema.required.length : 0;
        return `${props} propert${props !== 1 ? 'ies' : 'y'} · ${req} required`;
      } catch { return ''; }
    },
  };
}
```

- [ ] **Step 2: Register in main.js**

Add import in `src/main.js` (near the other component imports):
```javascript
import { schemaEditorModal } from './components/schemaEditorModal.ts';
```

And register it with Alpine (near the other `Alpine.data()` calls):
```javascript
Alpine.data('schemaEditorModal', schemaEditorModal);
```

- [ ] **Step 3: Update createCategory.tpl**

In `templates/createCategory.tpl`, replace line 16:
```
{% include "/partials/form/createFormTextareaInput.tpl" with title="Meta JSON Schema" name="MetaSchema" value=category.MetaSchema big=true %}
```
with:
```html
<div class="meta-schema-field" x-data="schemaEditorModal()">
    <div class="flex gap-2 items-start">
        <div class="flex-1">
            {% include "/partials/form/createFormTextareaInput.tpl" with title="Meta JSON Schema" name="MetaSchema" value=category.MetaSchema big=true id="metaSchemaTextarea" %}
        </div>
        <button type="button" class="visual-editor-btn mt-6 inline-flex items-center px-3 py-2 border border-stone-300 shadow-sm text-sm font-medium font-mono rounded-md text-stone-700 bg-white hover:bg-stone-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-amber-600" @click="openModal('metaSchemaTextarea')">
            Visual Editor
        </button>
    </div>

    <!-- Modal -->
    <template x-if="open">
        <div class="fixed inset-0 z-50 flex items-center justify-center" @keydown.escape="closeModal()">
            <div class="absolute inset-0 bg-black/40" @click="closeModal()"></div>
            <div x-ref="modalContent" class="relative bg-white rounded-lg shadow-2xl flex flex-col" style="width:90vw;max-width:1400px;height:80vh;" role="dialog" aria-modal="true" aria-label="Meta JSON Schema Editor">
                <!-- Header -->
                <div class="flex items-center border-b border-stone-200 px-4 bg-stone-50 rounded-t-lg">
                    <h3 class="text-sm font-medium font-mono text-stone-700 py-3 mr-6">Meta JSON Schema</h3>
                    <div class="flex gap-0 -mb-px">
                        <button type="button" class="px-4 py-2.5 text-xs font-medium font-mono" :class="tab === 'edit' ? 'text-indigo-700 border border-stone-200 border-b-white bg-white rounded-t-md' : 'text-stone-500 bg-transparent border-none'" @click="tab = 'edit'">Edit Schema</button>
                        <button type="button" class="px-4 py-2.5 text-xs font-medium font-mono" :class="tab === 'preview' ? 'text-indigo-700 border border-stone-200 border-b-white bg-white rounded-t-md' : 'text-stone-500 bg-transparent border-none'" @click="tab = 'preview'">Preview Form</button>
                        <button type="button" class="px-4 py-2.5 text-xs font-medium font-mono" :class="tab === 'raw' ? 'text-indigo-700 border border-stone-200 border-b-white bg-white rounded-t-md' : 'text-stone-500 bg-transparent border-none'" @click="tab = 'raw'">Raw JSON</button>
                    </div>
                    <div class="flex-1"></div>
                    <button type="button" class="text-stone-400 hover:text-stone-600 text-lg" @click="closeModal()" aria-label="Close">&times;</button>
                </div>
                <!-- Body -->
                <div class="flex-1 overflow-hidden">
                    <template x-if="tab === 'edit'">
                        <schema-editor mode="edit" :schema="currentSchema" @schema-change="handleSchemaChange($event)" style="height:100%;"></schema-editor>
                    </template>
                    <template x-if="tab === 'preview'">
                        <div class="p-6 overflow-y-auto h-full">
                            <schema-editor mode="form" :schema="currentSchema" value="{}" name="_preview"></schema-editor>
                        </div>
                    </template>
                    <template x-if="tab === 'raw'">
                        <textarea x-model="rawJson" @input="handleRawChange()" class="w-full h-full p-4 font-mono text-xs border-none resize-none focus:ring-0" spellcheck="false"></textarea>
                    </template>
                </div>
                <!-- Footer -->
                <div class="flex items-center gap-3 px-4 py-3 border-t border-stone-200 bg-stone-50 rounded-b-lg">
                    <span class="text-xs text-stone-400 font-mono" x-text="getPropertyCount()"></span>
                    <div class="flex-1"></div>
                    <button type="button" class="px-4 py-2 border border-stone-300 rounded-md text-sm font-mono text-stone-700 bg-white hover:bg-stone-50" @click="closeModal()">Cancel</button>
                    <button type="button" class="px-4 py-2 border-none rounded-md text-sm font-mono text-white bg-indigo-700 hover:bg-indigo-800" @click="applySchema()">Apply Schema</button>
                </div>
            </div>
        </div>
    </template>
</div>
```

- [ ] **Step 4: Update createResourceCategory.tpl**

Apply the identical change to `templates/createResourceCategory.tpl` line 16, with the same HTML structure but with `id="rcMetaSchemaTextarea"` in the include and `openModal('rcMetaSchemaTextarea')` in the button click.

- [ ] **Step 5: Verify the textarea partial uses the id parameter**

The partial `templates/partials/form/createFormTextareaInput.tpl` already supports an `id` parameter (defaults to `name` if not provided). Passing `id="metaSchemaTextarea"` in the include gives the textarea a known ID for the modal. No partial changes needed.

- [ ] **Step 6: Build the full application**

Run:
```bash
npm run build
```
Expected: CSS + JS + Go binary all build successfully.

- [ ] **Step 7: Smoke test**

Run:
```bash
./mahresources -ephemeral -bind-address=:8181 &
```
Navigate to `http://localhost:8181/category/new`. Verify:
1. The MetaSchema textarea is visible with a "Visual Editor" button next to it.
2. Clicking "Visual Editor" opens the modal.
3. The modal has three tabs.
4. "Apply Schema" writes back to the textarea.
5. "Cancel" and Escape close the modal.

Kill the server: `kill %1`

- [ ] **Step 8: Commit**

```bash
git add src/components/schemaEditorModal.ts src/main.js templates/createCategory.tpl templates/createResourceCategory.tpl
git commit -m "feat: add schema editor modal to category and resource category forms"
```

---

## Phase 2: Form mode — replace schemaForm.js

### Task 11: Port form rendering to form-mode.ts

**Files:**
- Create: `src/schema-editor/modes/form-mode.ts`
- Modify: `src/schema-editor/schema-editor.ts`

This is the largest single task — porting ~900 lines of `generateFormElement()` from imperative DOM manipulation to Lit templates. The logic stays identical; only the rendering approach changes.

- [ ] **Step 1: Create form-mode.ts**

Create `src/schema-editor/modes/form-mode.ts`. This file ports the `generateFormElement()` function from `src/components/schemaForm.js` to Lit's declarative `html` templates. The full implementation is too large to include inline in this plan — the implementer should:

1. Read `src/components/schemaForm.js` lines 156–995 (the `generateFormElement` function)
2. Convert each `createElement`/`innerHTML`/`appendChild` pattern to equivalent Lit `html` template
3. Convert `onchange`/`oninput`/`onblur` handlers to Lit `@change`/`@input`/`@blur` syntax
4. Replace the `uniqueIdCounter` with Lit's key-based rendering
5. Use `schema-core.ts` imports for `resolveRef`, `mergeSchemas`, `getDefaultValue`, `scoreSchemaMatch`, `evaluateCondition`, `inferType`, `inferSchema`

The component signature:
```typescript
import { LitElement, html, css } from 'lit';
import { customElement, property, state } from 'lit/decorators.js';
import { sharedStyles } from '../styles';
import { resolveRef, mergeSchemas, getDefaultValue, scoreSchemaMatch, evaluateCondition, inferType, inferSchema } from '../schema-core';
import type { JSONSchema } from '../schema-core';

@customElement('schema-form-mode')
export class SchemaFormMode extends LitElement {
  static override styles = [sharedStyles, css`/* form-specific styles */`];

  @property({ type: Object }) schema: JSONSchema = {};
  @property({ type: Object }) value: any = {};
  @property({ type: String }) name = 'Meta';

  // Light DOM hidden input for form submission
  private _hiddenInput: HTMLInputElement | null = null;

  override connectedCallback() {
    super.connectedCallback();
    this._hiddenInput = document.createElement('input');
    this._hiddenInput.type = 'hidden';
    this._hiddenInput.name = this.name;
    this._hiddenInput.value = JSON.stringify(this.value);
    this.appendChild(this._hiddenInput);
  }

  override disconnectedCallback() {
    this._hiddenInput?.remove();
    super.disconnectedCallback();
  }

  private _updateValue(newValue: any) {
    this.value = newValue;
    if (this._hiddenInput) {
      this._hiddenInput.value = JSON.stringify(newValue);
    }
    this.dispatchEvent(new CustomEvent('value-change', {
      detail: { value: newValue },
      bubbles: true, composed: true,
    }));
    this.requestUpdate();
  }

  // ... generateFormElement port as private _renderField(schema, data, onChange, rootSchema) returning TemplateResult
  // Each type handler (object, array, string, number, boolean, enum, const, oneOf, anyOf, allOf, if/then/else, $ref)
  // becomes a private method returning html`` template.

  override render() {
    return this._renderField(this.schema, this.value, (v: any) => this._updateValue(v), this.schema);
  }
}
```

The key pattern for each type: the implementer converts `createElement` → `html` templates, `container.appendChild(el)` → nested `html` returns, and event handlers from `el.onchange = (e) => {}` → `@change=${(e) => {}}`.

- [ ] **Step 2: Wire form mode into schema-editor.ts**

In `src/schema-editor/schema-editor.ts`, add the import:
```typescript
import './modes/form-mode';
```

Replace the form mode placeholder in `render()`:
```typescript
case 'form':
  return html`<schema-form-mode
    .schema=${this._parsedSchema}
    .value=${this.value ? JSON.parse(this.value) : {}}
    .name=${this.name}
    @value-change=${(e: CustomEvent) => {
      this.value = JSON.stringify(e.detail.value);
      this.dispatchEvent(new CustomEvent('value-change', { detail: e.detail, bubbles: true, composed: true }));
    }}
  ></schema-form-mode>`;
```

- [ ] **Step 3: Build and test on standalone page**

Run:
```bash
npm run build-js && npx vite src/schema-editor --open
```
Expected: The form mode section renders input fields matching the schema. Filling values updates the event log.

- [ ] **Step 4: Commit**

```bash
git add src/schema-editor/modes/form-mode.ts src/schema-editor/schema-editor.ts
git commit -m "feat: implement form mode — port schemaForm.js to Lit"
```

---

### Task 12: Integrate form mode into group/resource templates

**Files:**
- Modify: `templates/createGroup.tpl`
- Modify: `templates/createResource.tpl`

- [ ] **Step 1: Update createGroup.tpl**

Replace lines 77–89 in `templates/createGroup.tpl` (the `x-if="currentSchema"` block) with:
```html
<template x-if="currentSchema">
    <div class="border p-4 rounded-md bg-stone-50 mt-5">
        <h2 class="text-sm font-medium font-mono text-stone-700 mb-3">Meta Data (Schema Enforced)</h2>
        <schema-editor
            mode="form"
            :schema="currentSchema"
            value='{{ group.Meta|json }}'
            name="Meta"
        ></schema-editor>
    </div>
</template>
```

- [ ] **Step 2: Add schema-driven metadata to createResource.tpl**

In `templates/createResource.tpl`, around line 166 where the freeFields include is, wrap it in a category-aware Alpine block similar to createGroup.tpl. The resource form needs to react to the `ResourceCategoryId` autocompleter:

```html
<div x-data="{
         currentSchema: null,
         handleCategoryChange(e) {
             if (e.detail.value.length > 0) {
                 this.currentSchema = e.detail.value[0].MetaSchema;
             } else {
                 this.currentSchema = null;
             }
         }
    }"
    @multiple-input.window="if ($event.detail.name === 'ResourceCategoryId') handleCategoryChange($event)"
    class="w-full"
>
    <template x-if="currentSchema">
        <div class="border p-4 rounded-md bg-stone-50 mt-5">
            <h2 class="text-sm font-medium font-mono text-stone-700 mb-3">Meta Data (Schema Enforced)</h2>
            <schema-editor
                mode="form"
                :schema="currentSchema"
                value='{{ resource.Meta|json }}'
                name="Meta"
            ></schema-editor>
        </div>
    </template>
    <template x-if="!currentSchema">
        {% include "/partials/form/freeFields.tpl" with name="Meta" url='/v1/resources/meta/keys' fromJSON=resource.Meta jsonOutput="true" id=getNextId("freeField") %}
    </template>
</div>
```

- [ ] **Step 3: Build and smoke test**

Run:
```bash
npm run build && ./mahresources -ephemeral -bind-address=:8181 &
```
1. Create a category with a MetaSchema.
2. Create a group in that category — verify the schema-driven form appears.
3. Create a resource category with a MetaSchema.
4. Create a resource in that category — verify the schema-driven form appears.

Kill: `kill %1`

- [ ] **Step 4: Commit**

```bash
git add templates/createGroup.tpl templates/createResource.tpl
git commit -m "feat: integrate schema-editor form mode into group and resource templates"
```

---

## Phase 3: Search mode — replace schemaSearchFields.js

### Task 13: Port search fields to search-mode.ts

**Files:**
- Create: `src/schema-editor/modes/search-mode.ts`
- Modify: `src/schema-editor/schema-editor.ts`

- [ ] **Step 1: Create search-mode.ts**

Create `src/schema-editor/modes/search-mode.ts`. This ports the `schemaSearchFields` Alpine component to Lit. The implementer should:

1. Read `src/components/schemaSearchFields.js` lines 232–461 (the `schemaSearchFields` function)
2. Read `templates/partials/form/schemaSearchFields.tpl` for the template rendering
3. Port the field rendering (boolean radio, enum checkboxes/multi-select, string/number inputs with operators) to Lit templates
4. Port the `_findExistingValue`, `getHiddenInputs`, operator toggle logic
5. Handle multi-schema intersection: when `schema` attribute is a JSON array, parse each, flatten, intersect
6. Emit `schema-fields-claimed` event
7. Render hidden `<input name="MetaQuery">` elements in light DOM for form submission

The component signature:
```typescript
import { LitElement, html, css } from 'lit';
import { customElement, property, state } from 'lit/decorators.js';
import { sharedStyles } from '../styles';
import { flattenSchema, intersectFields, titleCase } from '../schema-core';
import type { JSONSchema, FlatField } from '../schema-core';

interface SearchField extends FlatField {
  operator: string;
  value: string;
  enumValues: string[];
  boolValue: string;
  showOperator: boolean;
  operators: { code: string; label: string }[] | null;
}

@customElement('schema-search-mode')
export class SchemaSearchMode extends LitElement {
  static override styles = [sharedStyles, css`/* search-specific styles */`];

  @property({ type: String }) schema = '';
  @property({ type: String }) metaQuery = '[]';
  @property({ type: String }) fieldName = 'MetaQuery';

  @state() private _fields: SearchField[] = [];

  // ... port of schemaSearchFields logic
  // Hidden inputs rendered in light DOM via connectedCallback pattern
}
```

Import `generateParamNameForMeta` and `getJSONValue` from `src/components/freeFields.js` (these stay in place since freeFields is not being replaced).

- [ ] **Step 2: Wire search mode into schema-editor.ts**

Add import and replace placeholder in `render()`:
```typescript
import './modes/search-mode';

case 'search':
  return html`<schema-search-mode
    .schema=${this.schema}
    .metaQuery=${this.metaQuery}
    .fieldName=${this.fieldName}
  ></schema-search-mode>`;
```

- [ ] **Step 3: Build and test on standalone page**

Run:
```bash
npm run build-js && npx vite src/schema-editor --open
```
Expected: The search mode section renders filter fields with operators.

- [ ] **Step 4: Commit**

```bash
git add src/schema-editor/modes/search-mode.ts src/schema-editor/schema-editor.ts
git commit -m "feat: implement search mode — port schemaSearchFields.js to Lit"
```

---

### Task 14: Integrate search mode into list page templates

**Files:**
- Modify: `templates/partials/form/schemaSearchFields.tpl`
- Modify: `templates/partials/form/searchFormResource.tpl`

- [ ] **Step 1: Replace schemaSearchFields.tpl content**

Replace the contents of `templates/partials/form/schemaSearchFields.tpl` with:
```html
<div
    x-data="{
        schemas: [],
        handleCategoryChange(items) {
            if (!items || items.length === 0) {
                this.schemas = [];
                this.$refs.searchEditor.setAttribute('schema', '');
                return;
            }
            this.schemas = items.map(i => i.MetaSchema).filter(Boolean);
            if (this.schemas.length === 1) {
                this.$refs.searchEditor.setAttribute('schema', this.schemas[0]);
            } else if (this.schemas.length > 1) {
                this.$refs.searchEditor.setAttribute('schema', JSON.stringify(this.schemas));
            } else {
                this.$refs.searchEditor.setAttribute('schema', '');
            }
        }
    }"
    @multiple-input.window="if ($event.detail.name === '{{ elName }}') handleCategoryChange($event.detail.value)"
    class="w-full"
>
    <schema-editor
        x-ref="searchEditor"
        mode="search"
        schema=""
        meta-query='{{ existingMetaQuery|json }}'
        field-name="MetaQuery"
    ></schema-editor>
</div>
```

Note: The initial category handling (for page load from URL params) needs the same `initialCategories` logic. Add an `x-init` block:
```
x-init="$nextTick(() => { const initial = {{ initialCategories|json }} || []; if (initial.length > 0) handleCategoryChange(initial); })"
```

- [ ] **Step 2: Update searchFormResource.tpl**

In `templates/partials/form/searchFormResource.tpl`, the `schemaSearchFields` include at line 31 already uses the partial — it will pick up the changes automatically. Verify that the `elName` is `ResourceCategoryId` which matches the resource category autocompleter.

- [ ] **Step 3: Build and smoke test**

Run:
```bash
npm run build && ./mahresources -ephemeral -bind-address=:8181 &
```
1. Create a category with MetaSchema.
2. Go to Groups list page, select that category — verify schema filter fields appear.
3. Go to Resources list page, select a resource category with MetaSchema — verify fields appear.

Kill: `kill %1`

- [ ] **Step 4: Commit**

```bash
git add templates/partials/form/schemaSearchFields.tpl templates/partials/form/searchFormResource.tpl
git commit -m "feat: integrate schema-editor search mode into list page templates"
```

---

## Phase 4: Cleanup, tests, and docs

### Task 15: Remove old Alpine components

**Files:**
- Delete: `src/components/schemaForm.js`
- Delete: `src/components/schemaSearchFields.js`
- Modify: `src/main.js`

- [ ] **Step 1: Remove imports and registrations from main.js**

Remove these lines from `src/main.js`:
```javascript
import { schemaForm } from './components/schemaForm.js';
import { schemaSearchFields } from './components/schemaSearchFields.js';
```
And:
```javascript
Alpine.data('schemaForm', schemaForm);
Alpine.data('schemaSearchFields', schemaSearchFields);
```

- [ ] **Step 2: Delete the old files**

```bash
rm src/components/schemaForm.js src/components/schemaSearchFields.js
```

- [ ] **Step 3: Build and verify**

Run:
```bash
npm run build-js
```
Expected: Build succeeds — no remaining imports of these files.

- [ ] **Step 4: Run all unit tests**

Run:
```bash
npx vitest run
```
Expected: All pass.

- [ ] **Step 5: Commit**

```bash
git add -A src/components/schemaForm.js src/components/schemaSearchFields.js src/main.js
git commit -m "refactor: remove old Alpine schemaForm and schemaSearchFields components"
```

---

### Task 16: E2E tests

**Files:**
- Create: `e2e/tests/schema-editor.spec.ts`

- [ ] **Step 1: Write E2E test for modal integration**

Create `e2e/tests/schema-editor.spec.ts`:
```typescript
import { test, expect } from '../fixtures/base.fixture';

test.describe('Schema Editor Modal', () => {
  let categoryId: number;

  test.beforeAll(async ({ apiClient }) => {
    const cat = await apiClient.createCategory({
      Name: 'Schema Editor Test',
      Description: 'Category for schema editor E2E tests',
    });
    categoryId = cat.ID;
  });

  test.afterAll(async ({ apiClient }) => {
    await apiClient.deleteCategory(categoryId);
  });

  test('opens modal from category edit form', async ({ page }) => {
    await page.goto(`/category?id=${categoryId}`);
    await page.click('.visual-editor-btn');
    await expect(page.locator('[role="dialog"]')).toBeVisible();
    await expect(page.locator('[role="dialog"]')).toContainText('Meta JSON Schema');
  });

  test('builds schema visually and applies it', async ({ page }) => {
    await page.goto(`/category?id=${categoryId}`);
    await page.click('.visual-editor-btn');

    // Wait for editor to load
    const dialog = page.locator('[role="dialog"]');
    await expect(dialog).toBeVisible();

    // The tree should show a root node
    await expect(dialog.locator('[role="treeitem"]').first()).toBeVisible();

    // Click "+ Property" to add a property
    await dialog.locator('button', { hasText: '+ Property' }).click();

    // New property should appear in tree
    await expect(dialog.locator('[role="treeitem"]', { hasText: 'newProperty' })).toBeVisible();

    // Apply schema
    await dialog.locator('button', { hasText: 'Apply Schema' }).click();
    await expect(dialog).not.toBeVisible();

    // Verify textarea was updated
    const textarea = page.locator('#metaSchemaTextarea');
    const value = await textarea.inputValue();
    expect(value).toContain('newProperty');
  });

  test('tab switching works', async ({ page }) => {
    await page.goto(`/category?id=${categoryId}`);

    // First set a schema via textarea
    await page.locator('#metaSchemaTextarea').fill('{"type":"object","properties":{"name":{"type":"string"}}}');
    await page.click('.visual-editor-btn');

    const dialog = page.locator('[role="dialog"]');

    // Switch to Preview tab
    await dialog.locator('button', { hasText: 'Preview Form' }).click();
    await expect(dialog.locator('schema-editor[mode="form"]')).toBeVisible();

    // Switch to Raw JSON tab
    await dialog.locator('button', { hasText: 'Raw JSON' }).click();
    await expect(dialog.locator('textarea')).toBeVisible();
    const rawContent = await dialog.locator('textarea').inputValue();
    expect(rawContent).toContain('"name"');

    // Close
    await dialog.locator('button', { hasText: 'Cancel' }).click();
    await expect(dialog).not.toBeVisible();
  });

  test('escape closes modal', async ({ page }) => {
    await page.goto(`/category?id=${categoryId}`);
    await page.click('.visual-editor-btn');
    await expect(page.locator('[role="dialog"]')).toBeVisible();
    await page.keyboard.press('Escape');
    await expect(page.locator('[role="dialog"]')).not.toBeVisible();
  });
});

test.describe('Schema Editor Form Mode', () => {
  let categoryId: number;
  const testSchema = JSON.stringify({
    type: 'object',
    properties: {
      name: { type: 'string', minLength: 1 },
      status: { type: 'string', enum: ['active', 'inactive'] },
      age: { type: 'integer', minimum: 0 },
    },
    required: ['name'],
  });

  test.beforeAll(async ({ apiClient }) => {
    const cat = await apiClient.createCategory({
      Name: 'Form Mode Test',
      MetaSchema: testSchema,
    });
    categoryId = cat.ID;
  });

  test.afterAll(async ({ apiClient }) => {
    await apiClient.deleteCategory(categoryId);
  });

  test('renders schema-driven form when category selected on group create', async ({ page }) => {
    await page.goto('/group/new');
    // Select the category via autocompleter (implementation depends on existing autocompleter helpers)
    // After selection, the <schema-editor mode="form"> should render
    await expect(page.locator('schema-editor[mode="form"]')).toBeVisible({ timeout: 5000 });
  });
});
```

- [ ] **Step 2: Run E2E tests**

Run:
```bash
cd e2e && npm run test:with-server -- --grep "Schema Editor"
```
Expected: All tests pass.

- [ ] **Step 3: Commit**

```bash
git add e2e/tests/schema-editor.spec.ts
git commit -m "test: add E2E tests for schema editor modal, form mode, and tab switching"
```

---

### Task 17: Accessibility E2E tests

**Files:**
- Create: `e2e/tests/accessibility/schema-editor-a11y.spec.ts`

- [ ] **Step 1: Write accessibility test**

Create `e2e/tests/accessibility/schema-editor-a11y.spec.ts`:
```typescript
import { test, expect } from '../../fixtures/a11y.fixture';

test.describe('Schema Editor Accessibility', () => {
  let categoryId: number;
  const testSchema = JSON.stringify({
    type: 'object',
    properties: {
      name: { type: 'string' },
      status: { type: 'string', enum: ['active', 'inactive'] },
    },
  });

  test.beforeAll(async ({ apiClient }) => {
    const cat = await apiClient.createCategory({
      Name: 'A11y Test Category',
      MetaSchema: testSchema,
    });
    categoryId = cat.ID;
  });

  test.afterAll(async ({ apiClient }) => {
    await apiClient.deleteCategory(categoryId);
  });

  test('schema editor modal has no axe violations', async ({ page, makeAxeBuilder }) => {
    await page.goto(`/category?id=${categoryId}`);
    await page.click('.visual-editor-btn');
    await expect(page.locator('[role="dialog"]')).toBeVisible();

    const results = await makeAxeBuilder().analyze();
    expect(results.violations).toEqual([]);
  });

  test('tree panel has proper ARIA roles', async ({ page }) => {
    await page.goto(`/category?id=${categoryId}`);
    await page.locator('#metaSchemaTextarea').fill(testSchema);
    await page.click('.visual-editor-btn');

    const tree = page.locator('[role="tree"]');
    await expect(tree).toBeVisible();
    await expect(tree.locator('[role="treeitem"]').first()).toBeVisible();
  });
});
```

- [ ] **Step 2: Run accessibility tests**

Run:
```bash
cd e2e && npm run test:with-server:a11y -- --grep "Schema Editor"
```
Expected: All pass.

- [ ] **Step 3: Commit**

```bash
git add e2e/tests/accessibility/schema-editor-a11y.spec.ts
git commit -m "test: add accessibility tests for schema editor"
```

---

### Task 18: Documentation updates

**Files:**
- Modify: `docs-site/docs/features/meta-schemas.md`
- Modify: `docs-site/static/img/screenshot-manifest.json`

- [ ] **Step 1: Update meta-schemas.md**

Add the following sections after the existing "Form Generation" section in `docs-site/docs/features/meta-schemas.md`:

```markdown
## Visual Schema Editor

Instead of writing JSON Schema by hand, you can use the visual editor to build schemas interactively.

### Opening the Editor

1. Navigate to **Categories** (or **Resource Categories**)
2. Create or edit a Category
3. Click the **Visual Editor** button next to the Meta Schema field
4. The editor opens in a modal with three tabs

### Editor Tabs

**Edit Schema** — The visual builder with a tree view on the left and a property editor on the right. Click nodes in the tree to edit their type, constraints, and metadata. Use the "+ Property" button to add new fields.

![Schema Editor Modal](/img/schema-editor-modal.png)

**Preview Form** — Shows a live preview of the form that will be generated from your schema. This is exactly what users will see when creating or editing entities in this category.

![Schema Editor Preview](/img/schema-editor-preview.png)

**Raw JSON** — The full JSON Schema as editable text. Changes here sync with the visual editor. Use this for advanced schemas that the visual editor doesn't fully support.

### Building a Schema

1. Click **+ Property** in the tree toolbar
2. Select the new property in the tree
3. Set its name, type, and constraints in the detail panel
4. Check **Required** if the field is mandatory
5. For enum fields: choose "string" type, then add enum values in the Enum Values section
6. For nested objects: choose "object" type, then add child properties
7. Click **Preview Form** to verify the form looks right
8. Click **Apply Schema** to save

### Composition Keywords

The editor supports `oneOf`, `anyOf`, `allOf`, and `$ref` for advanced schema patterns:

- Use `$defs` to define reusable schema fragments
- Use `$ref` to reference definitions
- Use `oneOf`/`anyOf` for variant types (e.g., a "contact" field that can be email or phone)

![Schema Composition](/img/schema-editor-composition.png)

## Search Integration

When a Category has a schema defined, the list page search form automatically renders typed filter fields based on the schema properties.

![Schema Search Fields](/img/schema-search-fields.png)

- **String fields** render as text inputs with a LIKE operator by default
- **Number fields** render as numeric inputs with comparison operators (=, ≠, >, ≥, <, ≤)
- **Enum fields** render as checkboxes (≤6 values) or multi-select dropdowns (>6 values)
- **Boolean fields** render as three-state radio buttons (Any / Yes / No)

When multiple categories are selected, only fields common to all selected categories are shown. Fields that exist in some but not all categories are hidden.

Schema-driven filter fields appear alongside the existing free-form metadata filters. The free-form filters are automatically adjusted to exclude fields already covered by the schema filters.
```

- [ ] **Step 2: Update the "Setting a Schema → Via the UI" section**

Find the existing section and update it:
```markdown
### Via the UI

1. Navigate to **Categories** (or **Resource Categories**)
2. Create or edit a Category
3. Enter the JSON Schema in the **Meta Schema** field, or click **Visual Editor** to build the schema interactively
4. Save
```

- [ ] **Step 3: Add screenshot entries to manifest**

Add these entries to `docs-site/static/img/screenshot-manifest.json`:
```json
"schema-editor-modal.png": {
  "page": "/category?id=1",
  "description": "Schema editor modal showing tree panel with 5 properties and detail panel editing a string property",
  "seedDependencies": ["categories"],
  "seedDetails": "Person category with rich MetaSchema (name, email, status enum, age, address object)",
  "viewport": { "width": 1200, "height": 800 },
  "interactions": ["Click Visual Editor button to open modal"],
  "capturedDate": ""
},
"schema-editor-preview.png": {
  "page": "/category?id=1",
  "description": "Schema editor modal on Preview Form tab showing rendered form fields for the Person schema",
  "seedDependencies": ["categories"],
  "seedDetails": "Same as schema-editor-modal",
  "viewport": { "width": 1200, "height": 800 },
  "interactions": ["Click Visual Editor, switch to Preview Form tab"],
  "capturedDate": ""
},
"schema-editor-composition.png": {
  "page": "/category?id=1",
  "description": "Schema editor modal showing a oneOf composition node with two variants in the detail panel",
  "seedDependencies": ["categories"],
  "seedDetails": "Category with schema containing oneOf (email string or phone object)",
  "viewport": { "width": 1200, "height": 800 },
  "interactions": ["Click Visual Editor, select the oneOf node in tree"],
  "capturedDate": ""
},
"schema-search-fields.png": {
  "page": "/groups?categories=1",
  "description": "Groups list page with schema-driven search fields showing enum checkboxes, text input with operator, and boolean radio",
  "seedDependencies": ["categories", "groups"],
  "seedDetails": "Person category selected, schema fields rendered in search sidebar",
  "viewport": { "width": 1200, "height": 800 },
  "interactions": ["Select Person category in sidebar"],
  "capturedDate": ""
}
```

- [ ] **Step 4: Commit**

```bash
git add docs-site/docs/features/meta-schemas.md docs-site/static/img/screenshot-manifest.json
git commit -m "docs: add visual schema editor documentation and screenshot manifest entries"
```

---

### Task 19: Run full test suite

**Files:** None — verification only.

- [ ] **Step 1: Run Go unit tests**

Run:
```bash
go test --tags 'json1 fts5' ./...
```
Expected: All pass.

- [ ] **Step 2: Run TypeScript unit tests**

Run:
```bash
npx vitest run
```
Expected: All pass.

- [ ] **Step 3: Run E2E browser + CLI tests**

Run:
```bash
cd e2e && npm run test:with-server:all
```
Expected: All pass.

- [ ] **Step 4: Run Postgres tests**

Run:
```bash
go test --tags 'json1 fts5 postgres' ./mrql/... ./server/api_tests/... -count=1 && cd e2e && npm run test:with-server:postgres
```
Expected: All pass.

- [ ] **Step 5: If any tests fail, fix them before proceeding.**

---

### Task 20: Final build verification

**Files:** None — verification only.

- [ ] **Step 1: Full clean build**

Run:
```bash
npm run build
```
Expected: CSS + JS + Go binary all build successfully.

- [ ] **Step 2: Smoke test all integration points**

Run:
```bash
./mahresources -ephemeral -bind-address=:8181 &
```

Verify:
1. Category form → Visual Editor button → modal opens → edit schema → apply → textarea updated
2. Resource Category form → same flow
3. Group create → select category with schema → schema-driven form renders
4. Resource create → select resource category with schema → schema-driven form renders
5. Groups list → select category → schema search fields appear
6. Resources list → select resource category → schema search fields appear

Kill: `kill %1`

- [ ] **Step 3: Commit any remaining fixes**
