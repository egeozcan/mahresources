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
