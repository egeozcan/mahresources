/**
 * Bug 1 (P2): Single saved enum filters with falsy values don't restore
 * Bug 2 (P3): Preview wrong for composition/ref root schemas
 *
 * Written RED-first before any fixes.
 */
import { readFileSync } from 'fs';
import { resolve, dirname } from 'path';
import { fileURLToPath } from 'url';
import { describe, it, expect } from 'vitest';

const __dirname = dirname(fileURLToPath(import.meta.url));

function readSource(relativePath: string): string {
  return readFileSync(resolve(__dirname, relativePath), 'utf-8');
}

// ─── Bug 1 (P2): Boolean enum filters with falsy values don't restore ──────

describe('Bug 1: falsy enum filter values restore from URL params', () => {
  /**
   * When a boolean-typed field has enum:[true,false], the search-mode
   * renders checkboxes (not bool radios). But _findExistingValue routes
   * boolean values into boolValue (not value/enumValues), and the
   * rehydration guard at ~line 216 checks `existing.value` which is ''
   * for booleans. The fix must ensure boolean enum values end up in
   * enumValues so checkboxes show them as checked.
   */

  it('rehydrates boolean enum value into enumValues, not boolValue', () => {
    const src = readSource('./modes/search-mode.ts');
    // After _findExistingValue, the _rebuildFields code must detect
    // when a field has an enum AND existing has a boolValue that was
    // routed by _findExistingValue for boolean typeof values. It should
    // redirect that boolValue into enumValues for enum fields.
    //
    // The fix area is the enum rehydration block around line 213-218.
    // It must handle the case where existing.value is empty but
    // existing.boolValue contains the actual value (e.g. 'false' or 'true').

    const rebuildSection = src.slice(
      src.indexOf('let enumValues = existing ? existing.enumValues : [];'),
      src.indexOf('return {'),
    );

    // The code should check boolValue for enum fields when value is empty
    expect(rebuildSection).toContain('boolValue');
  });

  it('the enum rehydration guard does not rely on truthiness of existing.value', () => {
    const src = readSource('./modes/search-mode.ts');
    // The old code: `if (field.enum && existing && enumValues.length === 0 && existing.value)`
    // This fails when existing.value is '' (empty string) which happens for
    // boolean values routed through _findExistingValue. The fix should not
    // use a bare truthiness check on existing.value.

    const rebuildSection = src.slice(
      src.indexOf('let enumValues = existing ? existing.enumValues : [];'),
      src.indexOf('return {'),
    );

    // Should NOT have the old pattern that fails for falsy values
    expect(rebuildSection).not.toMatch(
      /if\s*\(\s*field\.enum\s*&&\s*existing\s*&&\s*enumValues\.length\s*===\s*0\s*&&\s*existing\.value\s*\)/,
    );
  });

  it('handles numeric zero enum value (value:"0" is truthy but worth verifying)', () => {
    const src = readSource('./modes/search-mode.ts');
    // For numeric enum with value 0, _findExistingValue returns value: '0'
    // (truthy string). The old code would work but the fix should use
    // explicit null/undefined checks rather than truthiness, covering all
    // falsy value cases consistently.
    const rebuildSection = src.slice(
      src.indexOf('let enumValues = existing ? existing.enumValues : [];'),
      src.indexOf('return {'),
    );

    // Should use explicit null/undefined/empty check, not bare truthiness
    // This could be != null, !== undefined, !== null, etc.
    expect(rebuildSection).toMatch(/!==?\s*(null|undefined|'')/);
  });
});


// ─── Bug 2 (P3): Preview wrong for composition/ref root schemas ────────────

import { getPreviewValue } from '../components/schemaEditorModal';

describe('Bug 2: getPreviewValue handles composition and ref root schemas', () => {
  it('returns correct default for oneOf root schema', () => {
    const schema = JSON.stringify({
      oneOf: [{ type: 'string' }, { type: 'number' }],
    });
    const result = getPreviewValue(schema);
    // First variant is string, default is ''
    expect(JSON.parse(result)).toBe('');
  });

  it('returns correct default for anyOf root schema', () => {
    const schema = JSON.stringify({
      anyOf: [{ type: 'number' }, { type: 'string' }],
    });
    const result = getPreviewValue(schema);
    // First variant is number, default is 0
    expect(JSON.parse(result)).toBe(0);
  });

  it('returns correct default for allOf root schema', () => {
    const schema = JSON.stringify({
      allOf: [{ type: 'array' }, { minItems: 1 }],
    });
    const result = getPreviewValue(schema);
    expect(JSON.parse(result)).toEqual([]);
  });

  it('returns correct default for $ref root schema', () => {
    const schema = JSON.stringify({
      $ref: '#/$defs/person',
      $defs: {
        person: {
          type: 'object',
          properties: { name: { type: 'string' } },
        },
      },
    });
    const result = getPreviewValue(schema);
    const parsed = JSON.parse(result);
    expect(parsed).toEqual({ name: '' });
  });

  it('returns correct default for allOf with $ref', () => {
    const schema = JSON.stringify({
      allOf: [
        { $ref: '#/$defs/base' },
        { properties: { extra: { type: 'number' } } },
      ],
      $defs: {
        base: {
          type: 'object',
          properties: { id: { type: 'integer' } },
        },
      },
    });
    const result = getPreviewValue(schema);
    const parsed = JSON.parse(result);
    expect(parsed).toHaveProperty('id');
    expect(parsed).toHaveProperty('extra');
  });

  it('returns correct default for if/then/else root schema', () => {
    const schema = JSON.stringify({
      type: 'object',
      properties: { kind: { type: 'string' } },
      if: { properties: { kind: { const: 'a' } } },
      then: { properties: { aField: { type: 'string' } } },
      else: { properties: { bField: { type: 'number' } } },
    });
    const result = getPreviewValue(schema);
    const parsed = JSON.parse(result);
    // getDefaultValue evaluates the condition against base defaults:
    // kind defaults to '' which doesn't match const 'a', so else branch is chosen
    expect(parsed).toHaveProperty('kind');
    expect(parsed).toHaveProperty('bField');
  });

  // Existing behavior should be preserved
  it('still returns {} for plain object type', () => {
    const result = getPreviewValue('{"type":"object"}');
    expect(JSON.parse(result)).toEqual({});
  });

  it('still returns "" for plain string type', () => {
    const result = getPreviewValue('{"type":"string"}');
    expect(JSON.parse(result)).toBe('');
  });

  it('still returns 0 for plain number type', () => {
    const result = getPreviewValue('{"type":"number"}');
    expect(JSON.parse(result)).toBe(0);
  });

  it('still returns [] for plain array type', () => {
    const result = getPreviewValue('{"type":"array"}');
    expect(JSON.parse(result)).toEqual([]);
  });

  it('still returns false for plain boolean type', () => {
    const result = getPreviewValue('{"type":"boolean"}');
    expect(JSON.parse(result)).toBe(false);
  });

  it('still returns null for plain null type', () => {
    const result = getPreviewValue('{"type":"null"}');
    expect(JSON.parse(result)).toBe(null);
  });

  it('still handles nullable type arrays', () => {
    const result = getPreviewValue('{"type":["string","null"]}');
    expect(JSON.parse(result)).toBe('');
  });

  it('returns {} for invalid JSON', () => {
    const result = getPreviewValue('not json');
    expect(JSON.parse(result)).toEqual({});
  });
});
