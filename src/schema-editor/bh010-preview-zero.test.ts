/**
 * BH-010: Schema-editor Preview Form tab seeds numeric fields with `0` instead
 * of leaving them empty. Verify the new `getPreviewDefaultValue()` returns
 * `undefined` for number/integer/string types that have no explicit `default`,
 * so the preview form renders those inputs as empty (not `0`).
 */
import { describe, it, expect } from 'vitest';
import { getPreviewDefaultValue } from './schema-core';

describe('BH-010: getPreviewDefaultValue', () => {
  it('returns undefined for numeric field with no default', () => {
    expect(getPreviewDefaultValue({ type: 'number' })).toBeUndefined();
    expect(getPreviewDefaultValue({ type: 'integer' })).toBeUndefined();
  });

  it('returns undefined for string with no default', () => {
    expect(getPreviewDefaultValue({ type: 'string' })).toBeUndefined();
  });

  it('honors explicit numeric default', () => {
    expect(getPreviewDefaultValue({ type: 'number', default: 42 })).toBe(42);
    expect(getPreviewDefaultValue({ type: 'integer', default: 7 })).toBe(7);
  });

  it('honors explicit string default', () => {
    expect(getPreviewDefaultValue({ type: 'string', default: 'hello' })).toBe('hello');
  });

  it('returns false for boolean (only empty-state meaningful choice)', () => {
    expect(getPreviewDefaultValue({ type: 'boolean' })).toBe(false);
  });

  it('for object schemas, recurses with preview semantics (numeric props stay undefined)', () => {
    const schema = {
      type: 'object',
      properties: {
        year: { type: 'integer', minimum: 1900, maximum: 2100 },
        title: { type: 'string' },
        active: { type: 'boolean' },
      },
    };
    const got = getPreviewDefaultValue(schema);
    expect(got.year).toBeUndefined();
    expect(got.title).toBeUndefined();
    expect(got.active).toBe(false);
  });

  it('honors const and first enum value', () => {
    expect(getPreviewDefaultValue({ const: 'fixed' })).toBe('fixed');
    expect(getPreviewDefaultValue({ enum: ['a', 'b', 'c'] })).toBe('a');
  });

  it('returns [] for arrays and {} for empty objects', () => {
    expect(getPreviewDefaultValue({ type: 'array' })).toEqual([]);
    expect(getPreviewDefaultValue({ type: 'object' })).toEqual({});
  });

  it('resolves $ref with preview semantics', () => {
    const root = {
      $defs: { yr: { type: 'integer', minimum: 1900 } },
      type: 'object',
      properties: { y: { $ref: '#/$defs/yr' } },
    };
    const got = getPreviewDefaultValue(root, root);
    expect(got.y).toBeUndefined();
  });

  it('handles nullable type arrays by preferring null then boolean then undefined', () => {
    expect(getPreviewDefaultValue({ type: ['null', 'integer'] })).toBe(null);
    expect(getPreviewDefaultValue({ type: ['integer', 'number'] })).toBeUndefined();
  });
});
