import { readFileSync } from 'fs';
import { resolve } from 'path';
import { fileURLToPath } from 'url';
import { describe, it, expect } from 'vitest';
import { resolveRef, mergeSchemas, resolveSchema, flattenSchema, intersectFields, getDefaultValue, scoreSchemaMatch, evaluateCondition, inferType, inferSchema, titleCase, escapeJsonPointer, unescapeJsonPointer } from './schema-core';

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

  // Bug 2: infer type from enum values when schema omits explicit type
  it('infers integer type from numeric enum without explicit type', () => {
    const schema = {
      type: 'object',
      properties: {
        code: { enum: [100, 200, 404] },
      },
    };
    const fields = flattenSchema(schema);
    expect(fields[0].type).toBe('integer');
  });

  it('infers number type from float enum without explicit type', () => {
    const schema = {
      type: 'object',
      properties: {
        ratio: { enum: [0.5, 1.5, 2.5] },
      },
    };
    const fields = flattenSchema(schema);
    expect(fields[0].type).toBe('number');
  });

  it('infers boolean type from boolean enum without explicit type', () => {
    const schema = {
      type: 'object',
      properties: {
        flag: { enum: [true, false] },
      },
    };
    const fields = flattenSchema(schema);
    expect(fields[0].type).toBe('boolean');
  });

  it('keeps string default for string enum without explicit type', () => {
    const schema = {
      type: 'object',
      properties: {
        status: { enum: ['active', 'inactive'] },
      },
    };
    const fields = flattenSchema(schema);
    expect(fields[0].type).toBe('string');
  });

  it('preserves explicit type even when enum values differ', () => {
    const schema = {
      type: 'object',
      properties: {
        code: { type: 'string', enum: [100, 200, 404] },
      },
    };
    const fields = flattenSchema(schema);
    expect(fields[0].type).toBe('string');
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

  it('populates all declared properties for object type with properties', () => {
    const schema = {
      type: 'object',
      properties: {
        name: { type: 'string' },
        age: { type: 'integer' },
        active: { type: 'boolean' },
      },
    };
    const result = getDefaultValue(schema);
    expect(result).toEqual({ name: '', age: 0, active: false });
  });

  it('scoreSchemaMatch distinguishes oneOf variants with populated defaults', () => {
    const variants = [
      { type: 'object', properties: { journal: { type: 'string' } }, required: ['journal'] },
      { type: 'object', properties: { conference: { type: 'string' } }, required: ['conference'] },
    ];
    const journalData = getDefaultValue(variants[0]);
    const confData = getDefaultValue(variants[1]);
    // Each variant's default should score highest against its own schema
    expect(scoreSchemaMatch(variants[0], journalData, {})).toBeGreaterThan(
      scoreSchemaMatch(variants[1], journalData, {}),
    );
    expect(scoreSchemaMatch(variants[1], confData, {})).toBeGreaterThan(
      scoreSchemaMatch(variants[0], confData, {}),
    );
  });

  it('returns empty object for object type without properties', () => {
    expect(getDefaultValue({ type: 'object' })).toEqual({});
  });

  // Bug 3: getDefaultValue should return first enum value instead of type default
  it('returns first enum value as default instead of type default', () => {
    expect(getDefaultValue({ type: 'string', enum: ['active', 'inactive'] })).toBe('active');
    expect(getDefaultValue({ type: 'integer', enum: [1, 2, 3] })).toBe(1);
    expect(getDefaultValue({ type: 'boolean', enum: [true] })).toBe(true);
  });

  it('prefers explicit default over enum[0]', () => {
    expect(getDefaultValue({ type: 'string', enum: ['a', 'b'], default: 'b' })).toBe('b');
  });

  it('returns first enum value for string enum without explicit type', () => {
    expect(getDefaultValue({ enum: ['red', 'green', 'blue'] })).toBe('red');
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

  // Bug 1: evaluateCondition should handle required, type, range, length, pattern
  it('evaluates required condition — present', () => {
    expect(evaluateCondition({ required: ['email'] }, { email: 'a@b.com' })).toBe(true);
  });
  it('evaluates required condition — missing', () => {
    expect(evaluateCondition({ required: ['email'] }, {})).toBe(false);
  });
  it('evaluates required condition — undefined value', () => {
    expect(evaluateCondition({ required: ['email'] }, { email: undefined })).toBe(false);
  });
  it('evaluates required-only condition (no properties key)', () => {
    expect(evaluateCondition({ required: ['x'] }, { x: 1 })).toBe(true);
    expect(evaluateCondition({ required: ['x'] }, {})).toBe(false);
  });

  it('evaluates type condition — number match', () => {
    expect(evaluateCondition({ properties: { age: { type: 'number' } } }, { age: 42 })).toBe(true);
  });
  it('evaluates type condition — number mismatch', () => {
    expect(evaluateCondition({ properties: { age: { type: 'number' } } }, { age: 'old' })).toBe(false);
  });
  it('evaluates type condition — integer matches number', () => {
    expect(evaluateCondition({ properties: { age: { type: 'number' } } }, { age: 7 })).toBe(true);
  });
  it('evaluates type condition — array type', () => {
    expect(evaluateCondition({ properties: { v: { type: ['string', 'null'] } } }, { v: 'hi' })).toBe(true);
    expect(evaluateCondition({ properties: { v: { type: ['string', 'null'] } } }, { v: 42 })).toBe(false);
  });

  it('evaluates minimum condition — pass', () => {
    expect(evaluateCondition({ properties: { age: { minimum: 18 } } }, { age: 21 })).toBe(true);
  });
  it('evaluates minimum condition — fail', () => {
    expect(evaluateCondition({ properties: { age: { minimum: 18 } } }, { age: 15 })).toBe(false);
  });
  it('evaluates maximum condition', () => {
    expect(evaluateCondition({ properties: { age: { maximum: 65 } } }, { age: 70 })).toBe(false);
    expect(evaluateCondition({ properties: { age: { maximum: 65 } } }, { age: 60 })).toBe(true);
  });
  it('evaluates exclusiveMinimum condition', () => {
    expect(evaluateCondition({ properties: { age: { exclusiveMinimum: 18 } } }, { age: 18 })).toBe(false);
    expect(evaluateCondition({ properties: { age: { exclusiveMinimum: 18 } } }, { age: 19 })).toBe(true);
  });
  it('evaluates exclusiveMaximum condition', () => {
    expect(evaluateCondition({ properties: { age: { exclusiveMaximum: 65 } } }, { age: 65 })).toBe(false);
    expect(evaluateCondition({ properties: { age: { exclusiveMaximum: 65 } } }, { age: 64 })).toBe(true);
  });

  it('evaluates minLength condition', () => {
    expect(evaluateCondition({ properties: { name: { minLength: 3 } } }, { name: 'ab' })).toBe(false);
    expect(evaluateCondition({ properties: { name: { minLength: 3 } } }, { name: 'abc' })).toBe(true);
  });
  it('evaluates maxLength condition', () => {
    expect(evaluateCondition({ properties: { name: { maxLength: 5 } } }, { name: 'abcdef' })).toBe(false);
    expect(evaluateCondition({ properties: { name: { maxLength: 5 } } }, { name: 'abc' })).toBe(true);
  });

  it('evaluates pattern condition', () => {
    expect(evaluateCondition({ properties: { code: { pattern: '^[A-Z]{3}$' } } }, { code: 'ABC' })).toBe(true);
    expect(evaluateCondition({ properties: { code: { pattern: '^[A-Z]{3}$' } } }, { code: 'ab' })).toBe(false);
  });
  it('pattern condition ignores non-strings', () => {
    // pattern only applies to strings; non-string values should not fail on pattern alone
    expect(evaluateCondition({ properties: { code: { pattern: '^[A-Z]+$' } } }, { code: 42 })).toBe(true);
  });

  it('evaluates minimum on non-number value — fails', () => {
    expect(evaluateCondition({ properties: { age: { minimum: 18 } } }, { age: 'old' })).toBe(false);
  });
  it('evaluates minLength on non-string value — fails', () => {
    expect(evaluateCondition({ properties: { name: { minLength: 1 } } }, { name: 42 })).toBe(false);
  });

  // Bug 1: evaluateCondition must handle composition keywords (allOf, anyOf, oneOf)
  describe('composition keywords in conditions', () => {
    it('evaluates allOf condition — all sub-conditions must match', () => {
      const cond = {
        allOf: [
          { properties: { x: { const: 'a' } } },
          { required: ['x'] },
        ],
      };
      expect(evaluateCondition(cond, { x: 'a' })).toBe(true);
      expect(evaluateCondition(cond, { x: 'b' })).toBe(false); // const mismatch
      expect(evaluateCondition(cond, {})).toBe(false); // required missing
    });

    it('evaluates anyOf condition — at least one must match', () => {
      const cond = {
        anyOf: [
          { properties: { x: { const: 'a' } } },
          { properties: { x: { const: 'b' } } },
        ],
      };
      expect(evaluateCondition(cond, { x: 'a' })).toBe(true);
      expect(evaluateCondition(cond, { x: 'b' })).toBe(true);
      expect(evaluateCondition(cond, { x: 'c' })).toBe(false);
    });

    it('evaluates oneOf condition — exactly one must match', () => {
      const cond = {
        oneOf: [
          { properties: { x: { minimum: 0 } } },
          { properties: { x: { maximum: 10 } } },
        ],
      };
      // x=5 matches both => oneOf fails (not exactly one)
      expect(evaluateCondition(cond, { x: 5 })).toBe(false);
      // x=-1 matches only maximum<=10 => oneOf passes
      expect(evaluateCondition(cond, { x: -1 })).toBe(true);
      // x=15 matches only minimum>=0 => oneOf passes
      expect(evaluateCondition(cond, { x: 15 })).toBe(true);
    });

    it('evaluates nested composition in condition', () => {
      const cond = {
        allOf: [
          { properties: { addr: { properties: { country: { const: 'US' } } } } },
        ],
      };
      expect(evaluateCondition(cond, { addr: { country: 'US' } })).toBe(true);
      expect(evaluateCondition(cond, { addr: { country: 'UK' } })).toBe(false);
    });

    it('evaluates composition alongside direct properties', () => {
      const cond = {
        allOf: [{ required: ['x'] }],
        properties: { x: { const: 'a' } },
      };
      // allOf requires x present, AND direct properties checks x === 'a'
      expect(evaluateCondition(cond, { x: 'a' })).toBe(true);
      expect(evaluateCondition(cond, {})).toBe(false); // allOf fails (required)
      expect(evaluateCondition(cond, { x: 'b' })).toBe(false); // properties const fails
    });
  });
});

describe('titleCase', () => {
  it('converts camelCase', () => expect(titleCase('firstName')).toBe('First Name'));
  it('converts snake_case', () => expect(titleCase('first_name')).toBe('First Name'));
  it('converts kebab-case', () => expect(titleCase('first-name')).toBe('First Name'));
});

describe('scoreSchemaMatch', () => {
  it('returns 100 for exact const match', () => {
    expect(scoreSchemaMatch({ const: 'hello' }, 'hello', {})).toBe(100);
  });
  it('returns 0 for const mismatch', () => {
    expect(scoreSchemaMatch({ const: 'hello' }, 'world', {})).toBe(0);
  });
  it('returns 10 for matching type', () => {
    expect(scoreSchemaMatch({ type: 'string' }, 'hello', {})).toBe(10);
  });
  it('returns 0 for type mismatch', () => {
    expect(scoreSchemaMatch({ type: 'string' }, 42, {})).toBe(0);
  });
  it('returns 9 for integer data matching number type', () => {
    expect(scoreSchemaMatch({ type: 'number' }, 42, {})).toBe(9);
  });
  it('scores object by property key overlap', () => {
    const schema = { type: 'object', properties: { a: {}, b: {}, c: {} } };
    expect(scoreSchemaMatch(schema, { a: 1, b: 2 }, {})).toBe(12); // 2 matches + 10
  });
  it('handles array type with matching type', () => {
    expect(scoreSchemaMatch({ type: ['string', 'null'] }, 'hello', {})).toBe(10);
  });
  it('handles array type with integer/number compat', () => {
    expect(scoreSchemaMatch({ type: ['number', 'null'] }, 42, {})).toBe(9);
  });
  it('resolves $ref before scoring', () => {
    const root = { $defs: { name: { type: 'string' } } };
    expect(scoreSchemaMatch({ $ref: '#/$defs/name' }, 'hello', root)).toBe(10);
  });

  // Bug 2: scoreSchemaMatch must resolve allOf-wrapped variant schemas before scoring
  it('scores allOf-wrapped variant with discriminator correctly', () => {
    const root = {
      $defs: { base: { type: 'object', properties: { name: { type: 'string' } } } },
    };
    const variant = {
      allOf: [
        { $ref: '#/$defs/base' },
        { properties: { type: { const: 'email' }, address: { type: 'string' } } },
      ],
    };
    const emailData = { name: 'test', type: 'email', address: 'a@b.com' };
    const phoneData = { name: 'test', type: 'phone', number: '555' };

    // emailData should score high (const match on type='email')
    expect(scoreSchemaMatch(variant, emailData, root)).toBeGreaterThan(10);
    // phoneData should score 0 (const mismatch on type)
    expect(scoreSchemaMatch(variant, phoneData, root)).toBe(0);
  });

  it('scores allOf-wrapped variant without $ref', () => {
    const variant = {
      allOf: [
        { type: 'object', properties: { name: { type: 'string' } } },
        { properties: { kind: { const: 'book' } }, required: ['kind'] },
      ],
    };
    const bookData = { name: 'My Book', kind: 'book' };
    const movieData = { name: 'My Movie', kind: 'movie' };

    expect(scoreSchemaMatch(variant, bookData, {})).toBeGreaterThan(10);
    expect(scoreSchemaMatch(variant, movieData, {})).toBe(0);
  });
});

// ─── Bug 1 (P1): Invalid MetaSchema shows empty box ──────────────────────────

describe('Bug 1: templates validate MetaSchema JSON before setting currentSchema', () => {
  const thisDir = fileURLToPath(new URL('.', import.meta.url));

  it('createGroup.tpl init() validates JSON with JSON.parse', () => {
    const tplSource = readFileSync(
      resolve(thisDir, '../../templates/createGroup.tpl'),
      'utf8',
    );
    // Extract the init() method body
    const initStart = tplSource.indexOf('init()');
    const initEnd = tplSource.indexOf('handleCategoryChange');
    const initBody = tplSource.slice(initStart, initEnd);
    expect(initBody).toContain('JSON.parse');
  });

  it('createResource.tpl init() validates JSON with JSON.parse', () => {
    const tplSource = readFileSync(
      resolve(thisDir, '../../templates/createResource.tpl'),
      'utf8',
    );
    const initStart = tplSource.indexOf('init()');
    const initEnd = tplSource.indexOf('handleCategoryChange');
    const initBody = tplSource.slice(initStart, initEnd);
    expect(initBody).toContain('JSON.parse');
  });

  it('createGroup.tpl handleCategoryChange validates MetaSchema JSON', () => {
    const tplSource = readFileSync(
      resolve(thisDir, '../../templates/createGroup.tpl'),
      'utf8',
    );
    const handlerStart = tplSource.indexOf('handleCategoryChange');
    const handlerEnd = tplSource.indexOf('handleMetaChange');
    const handlerBody = tplSource.slice(handlerStart, handlerEnd);
    expect(handlerBody).toContain('JSON.parse');
  });

  it('createResource.tpl handleCategoryChange validates MetaSchema JSON', () => {
    const tplSource = readFileSync(
      resolve(thisDir, '../../templates/createResource.tpl'),
      'utf8',
    );
    const handlerStart = tplSource.indexOf('handleCategoryChange');
    const handlerEnd = tplSource.indexOf('handleMetaChange');
    const handlerBody = tplSource.slice(handlerStart, handlerEnd);
    expect(handlerBody).toContain('JSON.parse');
  });
});

// ─── Bug 4 (P2): $ref paths not JSON Pointer-escaped ─────────────────────────

describe('escapeJsonPointer', () => {
  it('escapes ~ before / per RFC 6901', () => {
    expect(escapeJsonPointer('foo~bar')).toBe('foo~0bar');
  });

  it('escapes / to ~1', () => {
    expect(escapeJsonPointer('foo/bar')).toBe('foo~1bar');
  });

  it('escapes both ~ and / in correct order', () => {
    expect(escapeJsonPointer('a~/b')).toBe('a~0~1b');
  });

  it('returns unchanged string when no special chars', () => {
    expect(escapeJsonPointer('foobar')).toBe('foobar');
  });
});

describe('unescapeJsonPointer', () => {
  it('unescapes ~0 to ~', () => {
    expect(unescapeJsonPointer('foo~0bar')).toBe('foo~bar');
  });

  it('unescapes ~1 to /', () => {
    expect(unescapeJsonPointer('foo~1bar')).toBe('foo/bar');
  });

  it('unescapes in correct order (~1 before ~0)', () => {
    expect(unescapeJsonPointer('a~0~1b')).toBe('a~/b');
  });
});

describe('resolveRef handles escaped JSON Pointer tokens', () => {
  it('resolves ref with ~1 (escaped /)', () => {
    const root = { $defs: { 'foo/bar': { type: 'string' } } };
    expect(resolveRef('#/$defs/foo~1bar', root)).toEqual({ type: 'string' });
  });

  it('resolves ref with ~0 (escaped ~)', () => {
    const root = { $defs: { 'foo~bar': { type: 'number' } } };
    expect(resolveRef('#/$defs/foo~0bar', root)).toEqual({ type: 'number' });
  });

  it('resolves ref with both ~0 and ~1', () => {
    const root = { $defs: { 'a~/b': { type: 'boolean' } } };
    expect(resolveRef('#/$defs/a~0~1b', root)).toEqual({ type: 'boolean' });
  });

  it('still resolves simple refs without escaping', () => {
    const root = { $defs: { simple: { type: 'integer' } } };
    expect(resolveRef('#/$defs/simple', root)).toEqual({ type: 'integer' });
  });
});
