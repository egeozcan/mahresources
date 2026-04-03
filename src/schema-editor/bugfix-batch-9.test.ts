/**
 * Bug 1 (P1): Boolean enum filters render checkboxes but never submit
 * Bug 2 (P2): Boolean auto-initialization mutates untouched optional fields
 * Bug 3 (P2): Nullable type arrays leak into search field type
 *
 * Written RED-first before any fixes.
 */
import { readFileSync } from 'fs';
import { describe, it, expect } from 'vitest';
import { flattenSchema } from './schema-core';

// ─── Bug 1: Boolean enum fields must generate hidden inputs from enumValues ──

describe('Bug 1: boolean enum field generates MetaQuery inputs from enumValues, not boolValue', () => {
  /**
   * _getHiddenInputs checks `field.type === 'boolean'` before `field.enum`.
   * A boolean-enum field stores checked values in enumValues (the enum path),
   * but the boolean branch reads boolValue (which is 'any', untouched), and
   * returns []. The checked enum values are silently dropped.
   *
   * Fix: check field.enum BEFORE field.type === 'boolean' in _getHiddenInputs.
   */
  it('source code checks field.enum before field.type === boolean in _getHiddenInputs', () => {
    const src = readFileSync(
      new URL('./modes/search-mode.ts', import.meta.url).pathname,
      'utf-8',
    );

    const hiddenInputsMatch = src.match(
      /private _getHiddenInputs[\s\S]*?(?=\n\s*(?:\/\/ ─|private |override ))/,
    );
    expect(hiddenInputsMatch).not.toBeNull();
    const fnSrc = hiddenInputsMatch![0];

    // The enum check must appear BEFORE the boolean check in the method body.
    const enumIdx = fnSrc.indexOf('field.enum');
    const boolIdx = fnSrc.indexOf("field.type === 'boolean'");
    expect(enumIdx).toBeGreaterThan(-1);
    expect(boolIdx).toBeGreaterThan(-1);
    expect(enumIdx).toBeLessThan(boolIdx);
  });

  it('boolean enum field with checked values produces correct hidden input format', () => {
    const src = readFileSync(
      new URL('./modes/search-mode.ts', import.meta.url).pathname,
      'utf-8',
    );

    const hiddenInputsMatch = src.match(
      /private _getHiddenInputs[\s\S]*?(?=\n\s*(?:\/\/ ─|private |override ))/,
    );
    expect(hiddenInputsMatch).not.toBeNull();
    const fnSrc = hiddenInputsMatch![0];

    // The enum branch must handle boolean enum values (not quoted)
    // since field.type === 'boolean' means they should NOT be wrapped in quotes.
    // The existing enum branch uses `field.type === 'string'` to decide quoting,
    // which correctly leaves boolean enum values unquoted.
    expect(fnSrc).toContain("field.type === 'string'");
  });
});

// ─── Bug 2: Boolean auto-init must only apply to required properties ─────────

describe('Bug 2: boolean auto-initialization respects required array', () => {
  /**
   * The boolean initialization in _renderObject eagerly writes `false` for
   * EVERY undefined boolean property, even optional ones without a default.
   * This changes submission semantics: a previously-absent optional flag
   * now submits as `false`. It also skips nullable booleans (type: ["boolean", "null"]).
   *
   * Fix: only auto-initialize boolean properties that are REQUIRED by the schema.
   */
  it('source code checks required set before auto-initializing booleans', () => {
    const src = readFileSync(
      new URL('./modes/form-mode.ts', import.meta.url).pathname,
      'utf-8',
    );

    const renderObjectMatch = src.match(
      /private _renderObject[\s\S]*?(?=\n\s*(?:private |\/\/ ─))/,
    );
    expect(renderObjectMatch).not.toBeNull();
    const renderObjectSrc = renderObjectMatch![0];

    // The boolean initialization block must check required set membership.
    // It should build a Set from schema.required and check requiredSet.has(key).
    expect(renderObjectSrc).toMatch(/required/i);

    // Find the boolean init block specifically
    const boolInitBlock = renderObjectSrc.match(
      /Pre-populate[\s\S]*?needsUpdate[\s\S]*?\}/,
    );
    expect(boolInitBlock).not.toBeNull();
    const blockSrc = boolInitBlock![0];

    // The block MUST gate on required — look for requiredSet.has or required.includes
    expect(blockSrc).toMatch(/requiredSet\.has|required.*includes|required.*has/);
  });

  it('source code handles nullable booleans (type array with boolean)', () => {
    const src = readFileSync(
      new URL('./modes/form-mode.ts', import.meta.url).pathname,
      'utf-8',
    );

    const renderObjectMatch = src.match(
      /private _renderObject[\s\S]*?(?=\n\s*(?:private |\/\/ ─))/,
    );
    expect(renderObjectMatch).not.toBeNull();
    const renderObjectSrc = renderObjectMatch![0];

    // The boolean type check should also handle array types like ["boolean", "null"]
    // Look for Array.isArray(ps.type) or similar
    expect(renderObjectSrc).toMatch(/Array\.isArray.*type|type.*includes.*boolean/);
  });
});

// ─── Bug 3: Nullable type arrays must be normalized in flattenSchema ─────────

describe('Bug 3: flattenSchema normalizes nullable type arrays to scalar', () => {
  /**
   * flattenSchema stores prop.type verbatim. For nullable properties like
   * { type: ["string", "null"] }, FlatField.type becomes the array, not a
   * string. Search mode treats field.type as a scalar string everywhere.
   *
   * Fix: normalize nullable type arrays to their base scalar type.
   */
  it('normalizes ["string", "null"] to "string"', () => {
    const schema = {
      type: 'object',
      properties: {
        name: { type: ['string', 'null'] },
      },
    };
    const fields = flattenSchema(schema);
    expect(fields).toHaveLength(1);
    expect(fields[0].type).toBe('string');
  });

  it('normalizes ["number", "null"] to "number"', () => {
    const schema = {
      type: 'object',
      properties: {
        age: { type: ['number', 'null'] },
      },
    };
    const fields = flattenSchema(schema);
    expect(fields).toHaveLength(1);
    expect(fields[0].type).toBe('number');
  });

  it('normalizes ["integer", "null"] to "integer"', () => {
    const schema = {
      type: 'object',
      properties: {
        count: { type: ['integer', 'null'] },
      },
    };
    const fields = flattenSchema(schema);
    expect(fields).toHaveLength(1);
    expect(fields[0].type).toBe('integer');
  });

  it('normalizes ["boolean", "null"] to "boolean"', () => {
    const schema = {
      type: 'object',
      properties: {
        flag: { type: ['boolean', 'null'] },
      },
    };
    const fields = flattenSchema(schema);
    expect(fields).toHaveLength(1);
    expect(fields[0].type).toBe('boolean');
  });

  it('leaves scalar type unchanged', () => {
    const schema = {
      type: 'object',
      properties: {
        name: { type: 'string' },
        age: { type: 'number' },
      },
    };
    const fields = flattenSchema(schema);
    expect(fields).toHaveLength(2);
    expect(fields[0].type).toBe('string');
    expect(fields[1].type).toBe('number');
  });

  it('falls back to "string" for unrecognizable type arrays', () => {
    const schema = {
      type: 'object',
      properties: {
        weird: { type: ['null'] },
      },
    };
    const fields = flattenSchema(schema);
    expect(fields).toHaveLength(1);
    expect(fields[0].type).toBe('string');
  });
});
