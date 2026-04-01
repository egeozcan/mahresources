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
});
