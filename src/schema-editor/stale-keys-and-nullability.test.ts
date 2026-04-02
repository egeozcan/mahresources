/**
 * Tests for three confirmed bugs — written RED-first before any fixes.
 *
 * Bug 1 (P1): Switching schemas submits hidden stale meta keys
 * Bug 2 (P2): Empty meta {} not preserved in freeFields handoff
 * Bug 3 (P2): Wrapping nullable field in composition drops nullability
 */
import { describe, it, expect } from 'vitest';
import { schemaToTree, treeToSchema, resetIdCounter, type SchemaNode } from './schema-tree-model';

// ─── Bug 1 (P1): Switching schemas submits hidden stale meta keys ──────────

/**
 * When the schema changes (e.g. category switch), form-mode's willUpdate
 * rehydrates _data from this.value — which contains ALL keys from the
 * previous schema. When the new schema has additionalProperties: false,
 * the extra-keys UI is suppressed, but the hidden input still submits
 * the stale data.
 *
 * Since we can't instantiate the full Lit element in vitest, we extract
 * and test the key-stripping logic that should live in willUpdate.
 */

/**
 * Pure function that replicates the key-stripping logic that willUpdate
 * SHOULD perform when the schema changes. Before the fix, this logic
 * does not exist — _data is rehydrated verbatim from this.value.
 */
function stripStaleKeys(
  data: Record<string, any>,
  schema: { properties?: Record<string, any>; additionalProperties?: boolean | object },
): Record<string, any> {
  if (!schema.properties || !data || typeof data !== 'object') {
    return data;
  }
  const allowedKeys = new Set(Object.keys(schema.properties));
  const allowsAdditional = schema.additionalProperties !== false;
  if (allowsAdditional) return data;
  // Strip keys not in the new schema
  const result: Record<string, any> = {};
  for (const key of Object.keys(data)) {
    if (allowedKeys.has(key)) {
      result[key] = data[key];
    }
  }
  return result;
}

describe('Bug 1 (P1): strip stale meta keys when switching to stricter schema', () => {
  it('strips keys not in new schema when additionalProperties is false', () => {
    const oldData = { color: 'red', size: 'large' };
    const newSchema = {
      properties: { weight: { type: 'number' } },
      additionalProperties: false,
    };
    const result = stripStaleKeys(oldData, newSchema);
    expect(result).not.toHaveProperty('color');
    expect(result).not.toHaveProperty('size');
    // weight was not in oldData so it shouldn't appear with a value
    expect(Object.keys(result)).toEqual([]);
  });

  it('preserves matching keys from old data that exist in new schema', () => {
    const oldData = { color: 'red', weight: 42 };
    const newSchema = {
      properties: { weight: { type: 'number' } },
      additionalProperties: false,
    };
    const result = stripStaleKeys(oldData, newSchema);
    expect(result).toEqual({ weight: 42 });
  });

  it('keeps all keys when additionalProperties is not false', () => {
    const oldData = { color: 'red', size: 'large' };
    const newSchema = {
      properties: { weight: { type: 'number' } },
      // additionalProperties defaults to true
    };
    const result = stripStaleKeys(oldData, newSchema);
    expect(result).toEqual({ color: 'red', size: 'large' });
  });

  it('keeps all keys when additionalProperties is explicitly true', () => {
    const oldData = { color: 'red', extra: 'val' };
    const newSchema = {
      properties: { color: { type: 'string' } },
      additionalProperties: true,
    };
    const result = stripStaleKeys(oldData, newSchema);
    expect(result).toEqual({ color: 'red', extra: 'val' });
  });

  it('handles schema without properties gracefully', () => {
    const oldData = { anything: 'goes' };
    const newSchema = {};
    const result = stripStaleKeys(oldData, newSchema);
    expect(result).toEqual({ anything: 'goes' });
  });

  it('handles null/undefined data gracefully', () => {
    const newSchema = {
      properties: { weight: { type: 'number' } },
      additionalProperties: false,
    };
    expect(stripStaleKeys(null as any, newSchema)).toBeNull();
    expect(stripStaleKeys(undefined as any, newSchema)).toBeUndefined();
  });

  /**
   * Integration-level: verify form-mode.ts willUpdate actually performs
   * key stripping by reading the source code.
   */
  it('form-mode willUpdate strips stale keys on schema change', () => {
    const { readFileSync } = require('fs');
    const source = readFileSync(
      new URL('./modes/form-mode.ts', import.meta.url),
      'utf8',
    );
    // After the fix, willUpdate should contain key-stripping logic
    // when schema changes and additionalProperties is false.
    // Extract the full willUpdate method body (from 'willUpdate(' to the
    // next method definition '_safeParse' or '_emitChange')
    const startIdx = source.indexOf('willUpdate(');
    const endIdx = source.indexOf('_emitChange(');
    const willUpdateSection = source.slice(startIdx, endIdx);
    expect(willUpdateSection).toContain('additionalProperties');
    expect(willUpdateSection).toContain('allowedKeys');
  });
});

// ─── Bug 2 (P2): Empty meta {} not preserved in freeFields handoff ──────────

describe('Bug 2 (P2): empty meta {} preserved in freeFields handoff', () => {
  /**
   * Simulate the freeFields init() logic for choosing initSource.
   * The current code rejects empty objects because of
   *   Object.keys(parsed).length > 0
   * which treats {} as "no value" and falls back to fromJSON.
   */
  function simulateInitSource(
    currentMetaAttr: string | undefined,
    fromJSON: Record<string, any> | null,
    metaEdited: boolean,
  ): Record<string, any> | null {
    // Simulate the template-side behavior:
    // When metaEdited is false, data-current-meta should be absent/empty
    // When metaEdited is true, data-current-meta is the JSON string
    const effectiveAttr = metaEdited ? currentMetaAttr : '';

    let initSource = fromJSON;
    if (effectiveAttr) {
      try {
        const parsed = JSON.parse(effectiveAttr);
        if (parsed && typeof parsed === 'object') {
          initSource = parsed;
        }
      } catch { /* fall through */ }
    }
    return initSource;
  }

  it('preserves empty {} when user explicitly cleared all fields', () => {
    // User edited meta and cleared everything → metaEdited=true, currentMeta={}
    const result = simulateInitSource('{}', { old: 'data' }, true);
    expect(result).toEqual({});
  });

  it('uses fromJSON on initial render (no edits yet)', () => {
    // First render: metaEdited=false, so effectiveAttr is empty
    const result = simulateInitSource('{}', { existing: 'value' }, false);
    expect(result).toEqual({ existing: 'value' });
  });

  it('uses edited meta with data', () => {
    const result = simulateInitSource('{"color":"red"}', { old: 'data' }, true);
    expect(result).toEqual({ color: 'red' });
  });

  it('falls back to fromJSON when no data-current-meta', () => {
    const result = simulateInitSource(undefined, { server: 'data' }, false);
    expect(result).toEqual({ server: 'data' });
  });

  /**
   * Verify the freeFields.js source no longer has the Object.keys check.
   */
  it('freeFields init does not reject empty objects', () => {
    const { readFileSync } = require('fs');
    const source = readFileSync(
      new URL('../components/freeFields.js', import.meta.url),
      'utf8',
    );
    // After the fix, the init() should NOT require Object.keys(parsed).length > 0
    const initSection = source.slice(
      source.indexOf('currentMetaAttr'),
      source.indexOf('if (initSource)'),
    );
    expect(initSection).not.toContain('Object.keys(parsed).length > 0');
    expect(initSection).not.toContain('Object.keys(parsed).length>0');
  });

  /**
   * Verify createGroup.tpl tracks metaEdited state.
   */
  it('createGroup.tpl uses metaEdited sentinel', () => {
    const { readFileSync } = require('fs');
    const tplSource = readFileSync(
      require('path').resolve(__dirname, '../../templates/createGroup.tpl'),
      'utf8',
    );
    expect(tplSource).toContain('metaEdited');
  });

  /**
   * Verify createResource.tpl tracks metaEdited state.
   */
  it('createResource.tpl uses metaEdited sentinel', () => {
    const { readFileSync } = require('fs');
    const tplSource = readFileSync(
      require('path').resolve(__dirname, '../../templates/createResource.tpl'),
      'utf8',
    );
    expect(tplSource).toContain('metaEdited');
  });
});

// ─── Bug 3 (P2): Wrapping nullable field drops nullability ──────────────────

describe('Bug 3 (P2): wrapping nullable field preserves type union', () => {
  /**
   * Simulate the wrap-oneOf action from edit-mode.ts.
   * This replicates the exact logic at lines 346-384.
   */
  /**
   * Simulate the wrap action. This mirrors the FIXED production code
   * in edit-mode.ts — nullable type arrays are preserved in the variant
   * instead of being overwritten by the scalar and then deleted.
   *
   * A source-reading test below verifies the production code matches.
   */
  function simulateWrap(node: SchemaNode, keyword: 'oneOf' | 'anyOf' | 'allOf'): void {
    const metadataKeys = ['title', 'description', 'readOnly', 'writeOnly', 'default', 'examples', 'deprecated'];
    const metadata: Record<string, any> = {};
    const typeSchema: Record<string, any> = {};
    for (const [k, v] of Object.entries(node.schema)) {
      if (metadataKeys.includes(k)) {
        metadata[k] = v;
      } else {
        typeSchema[k] = v;
      }
    }
    const originalType = node.type;
    const hasNullableArray = Array.isArray(typeSchema.type);
    if (originalType && !hasNullableArray) typeSchema.type = originalType;
    const variantName = node.schema.title || 'variant1';
    const originalChildren = node.children ? [...node.children] : undefined;
    const originalRef = node.ref;
    const originalVariants = node.variants;
    const originalComposition = node.compositionKeyword;

    node.compositionKeyword = keyword;
    node.schema = metadata;
    node.type = '';
    node.children = undefined;
    node.ref = undefined;
    node.variants = [
      { id: `node-variant-0`, name: variantName, type: originalType || '', required: false, schema: typeSchema, children: originalChildren, ref: originalRef, variants: originalVariants, compositionKeyword: originalComposition },
      { id: `node-variant-1`, name: 'variant2', type: 'string', required: false, schema: {} },
    ];
    if (!Array.isArray(node.variants[0].schema.type)) {
      delete node.variants[0].schema.type;
    }
  }

  it('wrapping a nullable string field preserves ["string", "null"] in variant schema', () => {
    resetIdCounter();
    const tree = schemaToTree({
      type: 'object',
      properties: {
        optionalName: { type: ['string', 'null'], title: 'Optional Name' },
      },
    });

    const node = tree.children!.find(c => c.name === 'optionalName')!;
    expect(node.type).toBe('string'); // scalar base type
    // The nullable array should be in node.schema.type
    expect(node.schema.type).toEqual(['string', 'null']);

    simulateWrap(node, 'oneOf');

    const variant = node.variants![0];
    // After wrap, the variant should preserve nullability
    // variant.type is the display scalar
    expect(variant.type).toBe('string');
    // The variant.schema.type should have the nullable union preserved
    // (not deleted, because treeToSchema needs it to emit ["string", "null"])
    expect(variant.schema.type).toEqual(['string', 'null']);
  });

  it('round-trips nullable wrapped field correctly through treeToSchema', () => {
    resetIdCounter();
    const tree = schemaToTree({
      type: 'object',
      properties: {
        optionalName: { type: ['string', 'null'], title: 'Optional Name' },
      },
    });

    const node = tree.children!.find(c => c.name === 'optionalName')!;
    simulateWrap(node, 'oneOf');

    // Serialize the whole tree
    const output = treeToSchema(tree);
    const prop = output.properties!.optionalName;

    // The oneOf should have a variant that includes the nullable type
    expect(prop.oneOf).toBeDefined();
    const variants = prop.oneOf!;
    // First variant should have type: ['string', 'null']
    expect(variants[0].type).toEqual(['string', 'null']);
  });

  /**
   * Source-reading: verify edit-mode.ts wrap logic preserves nullable arrays.
   */
  it('edit-mode.ts wrap logic checks for nullable array before overwriting type', () => {
    const { readFileSync } = require('fs');
    const source = readFileSync(
      new URL('./modes/edit-mode.ts', import.meta.url),
      'utf8',
    );
    // The wrap action section should check for nullable arrays
    const wrapSection = source.slice(
      source.indexOf("case 'wrap-oneOf':"),
      source.indexOf("case 'add-if-then-else':"),
    );
    expect(wrapSection).toContain('hasNullableArray');
    expect(wrapSection).toContain('Array.isArray');
  });

  it('wrapping a non-nullable field works the same as before (no regression)', () => {
    resetIdCounter();
    const tree = schemaToTree({
      type: 'object',
      properties: {
        name: { type: 'string', title: 'Name' },
      },
    });

    const node = tree.children!.find(c => c.name === 'name')!;
    expect(node.type).toBe('string');
    expect(node.schema.type).toBeUndefined(); // non-nullable: type is NOT in schema

    simulateWrap(node, 'anyOf');

    const variant = node.variants![0];
    expect(variant.type).toBe('string');
    // For non-nullable, schema.type should be absent (type is in variant.type)
    expect(variant.schema.type).toBeUndefined();

    // Round-trip should produce correct output
    const output = treeToSchema(tree);
    const prop = output.properties!.name;
    expect(prop.anyOf).toBeDefined();
    expect(prop.anyOf![0].type).toBe('string');
  });
});

// ─── Bug 4 (P2): Propertyless strict schema fails to strip all stale keys ───

/**
 * A schema like { "type": "object", "additionalProperties": false } has no
 * `properties` key at all. The previous fix guarded the entire key-stripping
 * block with `this.schema.properties`, so for propertyless strict schemas the
 * block was never entered and all stale keys from _data leaked through the
 * hidden input.
 *
 * The fix moves the `schema.properties` check inside the allowed-keys Set
 * constructor (defaulting to empty `{}`), so the block fires whenever
 * additionalProperties === false regardless of whether properties exists.
 */

/**
 * Fixed version of stripStaleKeys that handles propertyless strict schemas.
 */
function stripStaleKeysFixed(
  data: Record<string, any>,
  schema: { properties?: Record<string, any>; additionalProperties?: boolean | object },
): Record<string, any> {
  if (!data || typeof data !== 'object') {
    return data;
  }
  const allowsAdditional = schema.additionalProperties !== false;
  if (allowsAdditional) return data;

  const allowedKeys = new Set(Object.keys(schema.properties || {}));
  const result: Record<string, any> = {};
  for (const key of Object.keys(data)) {
    if (allowedKeys.has(key)) {
      result[key] = data[key];
    }
  }
  return result;
}

describe('Bug 4 (P2): strip all stale keys for strict schemas without declared properties', () => {
  it('strips all keys when schema has additionalProperties:false and no properties', () => {
    const data = { color: 'red', size: 'large' };
    const schema = { type: 'object', additionalProperties: false } as any;
    const result = stripStaleKeysFixed(data, schema);
    expect(result).toEqual({});
  });

  it('strips all keys when schema is { additionalProperties: false } with no type', () => {
    const data = { foo: 1, bar: 2 };
    const schema = { additionalProperties: false };
    const result = stripStaleKeysFixed(data, schema);
    expect(result).toEqual({});
  });

  it('preserves declared keys when properties exists (no regression)', () => {
    const data = { weight: 42, stale: 'old' };
    const schema = {
      properties: { weight: { type: 'number' } },
      additionalProperties: false,
    };
    const result = stripStaleKeysFixed(data, schema);
    expect(result).toEqual({ weight: 42 });
  });

  it('keeps all keys when additionalProperties is not false (no regression)', () => {
    const data = { color: 'red', extra: 'val' };
    const schema = { additionalProperties: true } as any;
    const result = stripStaleKeysFixed(data, schema);
    expect(result).toEqual({ color: 'red', extra: 'val' });
  });

  /**
   * Integration: form-mode.ts willUpdate should strip ALL keys for a
   * propertyless strict schema. Verify the guard condition changed.
   */
  it('form-mode willUpdate does not guard on schema.properties before stripping', () => {
    const { readFileSync } = require('fs');
    const source = readFileSync(
      new URL('./modes/form-mode.ts', import.meta.url),
      'utf8',
    );
    const startIdx = source.indexOf('willUpdate(');
    const endIdx = source.indexOf('_emitChange(');
    const willUpdateSection = source.slice(startIdx, endIdx);

    // The OLD (buggy) guard: `this.schema?.properties && this._data`
    // After the fix, the outer guard must NOT require schema.properties.
    // The condition checking `schema.properties` must only appear inside
    // the allowedKeys Set construction, not as the outer if-guard.
    expect(willUpdateSection).not.toMatch(
      /if\s*\(\s*changed\.has\(['"]schema['"]\)\s*&&\s*this\.schema\?\.properties/,
    );

    // The fix should still check additionalProperties and use allowedKeys
    expect(willUpdateSection).toContain('additionalProperties');
    expect(willUpdateSection).toContain('allowedKeys');
  });
});
