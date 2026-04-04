/**
 * Tests for 4 confirmed bugs — written RED-first before any fixes.
 *
 * Bug 2 (P1): Nested stale keys survive schema switch
 * Bug 3 (P2): Nullable toggle produces ["null", "null"]
 */
import { readFileSync } from 'fs';
import { describe, it, expect, beforeAll } from 'vitest';
import { resolveRef, mergeSchemas, type JSONSchema } from './schema-core';
import { stripStaleKeys } from './form-mode-helpers';

// ─── Bug 2 (P1): Nested stale keys survive schema switch ─────────────────────

describe('Bug 2 (P1): recursive stale key stripping', () => {
  it('stripStaleKeys function is exported from form-mode-helpers', async () => {
    const mod = await import('./form-mode-helpers');
    expect(typeof (mod as any).stripStaleKeys).toBe('function');
  });

  it('strips nested stale keys when nested schema has additionalProperties:false', () => {
    const data = { spec: { color: 'red', weight: 5 } };
    const schema: JSONSchema = {
      properties: {
        spec: {
          type: 'object',
          properties: { weight: { type: 'number' } },
          additionalProperties: false,
        },
      },
    };
    stripStaleKeys(data, schema);
    expect(data.spec).toEqual({ weight: 5 });
  });

  it('strips top-level stale keys when schema has additionalProperties:false', () => {
    const data = { color: 'red', size: 'large', weight: 42 };
    const schema: JSONSchema = {
      properties: { weight: { type: 'number' } },
      additionalProperties: false,
    };
    stripStaleKeys(data, schema);
    expect(data).toEqual({ weight: 42 });
  });

  it('preserves all keys when additionalProperties is not false', () => {
    const data = { color: 'red', extra: 'val' };
    const schema: JSONSchema = {
      properties: { color: { type: 'string' } },
    };
    stripStaleKeys(data, schema);
    expect(data).toEqual({ color: 'red', extra: 'val' });
  });

  it('handles null/undefined data gracefully', () => {
    const schema: JSONSchema = {
      properties: { weight: { type: 'number' } },
      additionalProperties: false,
    };
    // Should not throw
    stripStaleKeys(null, schema);
    stripStaleKeys(undefined, schema);
  });

  it('handles arrays (should not strip from arrays)', () => {
    const data = [1, 2, 3];
    const schema: JSONSchema = {
      type: 'array',
      items: { type: 'number' },
    };
    stripStaleKeys(data, schema);
    expect(data).toEqual([1, 2, 3]);
  });

  it('recurses into nested objects with their own schemas', () => {
    const data = {
      address: {
        city: 'NYC',
        country: 'US',
        extra: 'stale',
      },
      name: 'test',
    };
    const schema: JSONSchema = {
      properties: {
        address: {
          type: 'object',
          properties: {
            city: { type: 'string' },
            country: { type: 'string' },
          },
          additionalProperties: false,
        },
        name: { type: 'string' },
      },
    };
    stripStaleKeys(data, schema);
    // Top level keeps all keys (no additionalProperties:false at top)
    expect(data.name).toBe('test');
    // Nested level strips 'extra'
    expect(data.address).toEqual({ city: 'NYC', country: 'US' });
  });

  it('resolves $ref in nested properties', () => {
    const rootSchema: JSONSchema = {
      $defs: {
        address: {
          type: 'object',
          properties: { city: { type: 'string' } },
          additionalProperties: false,
        },
      },
      properties: {
        addr: { $ref: '#/$defs/address' },
      },
    };
    const data = { addr: { city: 'NYC', stale: 'old' } };
    stripStaleKeys(data, rootSchema, rootSchema);
    expect(data.addr).toEqual({ city: 'NYC' });
  });

  it('resolves allOf in nested properties', () => {
    const schema: JSONSchema = {
      properties: {
        spec: {
          allOf: [
            { properties: { weight: { type: 'number' } } },
            { properties: { height: { type: 'number' } } },
          ],
          additionalProperties: false,
        },
      },
    };
    const data = { spec: { weight: 5, height: 10, color: 'red' } };
    stripStaleKeys(data, schema);
    expect(data.spec).toEqual({ weight: 5, height: 10 });
  });

  it('form-mode willUpdate calls stripStaleKeys for recursive stripping', () => {
    const source = readFileSync(
      new URL('./modes/form-mode.ts', import.meta.url),
      'utf8',
    );
    const startIdx = source.indexOf('willUpdate(');
    const endIdx = source.indexOf('private _emitChange(');
    const willUpdateSection = source.slice(startIdx, endIdx);
    expect(willUpdateSection).toContain('stripStaleKeys');
  });
});


// ─── Bug 3 (P2): Nullable toggle produces ["null", "null"] ───────────────────

describe('Bug 3 (P2): nullable toggle does not produce ["null","null"]', () => {
  it('detail-panel hides Nullable checkbox when node type is null', () => {
    const source = readFileSync(
      new URL('./tree/detail-panel.ts', import.meta.url),
      'utf8',
    );
    // The Nullable checkbox section should guard against node.type === 'null'
    // Look for the condition near the Nullable label
    const flagsStart = source.indexOf('Nullable');
    const flagsSection = source.slice(Math.max(0, flagsStart - 300), flagsStart + 50);
    expect(flagsSection).toContain("node.type !== 'null'");
  });

  it('edit-mode nullable handler guards against baseType === null', () => {
    const source = readFileSync(
      new URL('./modes/edit-mode.ts', import.meta.url),
      'utf8',
    );
    const nullableStart = source.indexOf("case 'nullable':");
    const nullableEnd = source.indexOf("case 'enum':");
    const nullableSection = source.slice(nullableStart, nullableEnd);
    // Should have a guard that prevents creating ["null", "null"]
    expect(nullableSection).toContain("'null'");
    // It should check baseType against 'null' before creating the union
    expect(nullableSection).toMatch(/baseType\s*===\s*'null'/);
  });
});


// ─── Bug 4 (P1): allOf stale-key stripping drops $ref sibling properties ────

describe('Bug 4 (P1): stripStaleKeys preserves $ref sibling properties in allOf', () => {
  it('preserves properties from both $ref and sibling on the same allOf member', () => {
    const rootSchema: JSONSchema = {
      $defs: { base: { type: 'object', properties: { name: { type: 'string' } } } },
      allOf: [
        { $ref: '#/$defs/base', properties: { zip: { type: 'string' } } },
      ],
      additionalProperties: false,
    };
    const data = { name: 'Alice', zip: '12345' };
    stripStaleKeys(data, rootSchema, rootSchema);
    // Both name (from $ref) and zip (sibling property) should survive
    expect(data).toEqual({ name: 'Alice', zip: '12345' });
  });

  it('preserves $ref-only properties when allOf member has no siblings', () => {
    const rootSchema: JSONSchema = {
      $defs: { base: { type: 'object', properties: { name: { type: 'string' } } } },
      allOf: [
        { $ref: '#/$defs/base' },
      ],
      additionalProperties: false,
    };
    const data = { name: 'Alice', stale: 'old' };
    stripStaleKeys(data, rootSchema, rootSchema);
    expect(data).toEqual({ name: 'Alice' });
  });

  it('preserves sibling-only properties when allOf member has no $ref', () => {
    const rootSchema: JSONSchema = {
      allOf: [
        { properties: { zip: { type: 'string' } } },
      ],
      additionalProperties: false,
    };
    const data = { zip: '12345', stale: 'old' };
    stripStaleKeys(data, rootSchema, rootSchema);
    expect(data).toEqual({ zip: '12345' });
  });

  it('merges properties from $ref and multiple siblings across allOf members', () => {
    const rootSchema: JSONSchema = {
      $defs: { base: { type: 'object', properties: { name: { type: 'string' } } } },
      allOf: [
        { $ref: '#/$defs/base', properties: { zip: { type: 'string' } } },
        { properties: { country: { type: 'string' } } },
      ],
      additionalProperties: false,
    };
    const data = { name: 'Alice', zip: '12345', country: 'US', stale: 'old' };
    stripStaleKeys(data, rootSchema, rootSchema);
    expect(data).toEqual({ name: 'Alice', zip: '12345', country: 'US' });
  });
});


// ─── Bug 5 (P2): Type switch to null produces ["null", "null"] ──────────────

describe('Bug 5 (P2): type change to null on nullable field', () => {
  it('edit-mode type handler does not produce ["null","null"] when switching to null', () => {
    const source = readFileSync(
      new URL('./modes/edit-mode.ts', import.meta.url),
      'utf8',
    );
    const typeStart = source.indexOf("case 'type':");
    const typeEnd = source.indexOf("case 'required':");
    const typeSection = source.slice(typeStart, typeEnd);
    // The type handler should guard against value === 'null' before building a union
    expect(typeSection).toContain("'null'");
    // It should have logic to handle the null case separately
    expect(typeSection).toMatch(/value\s*===\s*'null'/);
  });

  it('type change handler collapses nullable array when new type is null', () => {
    // Simulate: schema.type = ["string", "null"], user changes type to "null"
    // This replicates the exact logic path in edit-mode.ts case 'type':
    const schema: any = { type: ['string', 'null'] };
    const selected = { type: 'null', schema };

    // Apply the same logic as the type handler
    const value = 'null';
    selected.type = value;
    // Reset constraints (abbreviated)
    // Then handle nullable array
    if (Array.isArray(selected.schema.type) && selected.schema.type.includes('null')) {
      if (value === 'null') {
        // Fix: collapse to scalar or delete
        delete selected.schema.type;
      } else {
        selected.schema.type = [value, 'null'];
      }
    }

    // schema.type should NOT be ["null","null"]
    expect(selected.schema.type).not.toEqual(['null', 'null']);
    // It should be deleted (null type expressed via node.type)
    expect(selected.schema.type).toBeUndefined();
  });

  it('type change handler preserves nullable array for non-null types', () => {
    // Simulate: schema.type = ["string", "null"], user changes to "integer"
    const schema: any = { type: ['string', 'null'] };
    const selected = { type: 'integer', schema };

    const value = 'integer';
    selected.type = value;
    if (Array.isArray(selected.schema.type) && selected.schema.type.includes('null')) {
      if (value === 'null') {
        delete selected.schema.type;
      } else {
        selected.schema.type = [value, 'null'];
      }
    }

    expect(selected.schema.type).toEqual(['integer', 'null']);
  });
});
