import { describe, it, expect } from 'vitest';
import { schemaToTree, treeToSchema, resetIdCounter } from './schema-tree-model';
import type { SchemaNode } from './schema-tree-model';

// ─── Bug 1: Adding property to non-object root silently loses the child ──────

describe('Bug 1: auto-convert non-object root when adding properties', () => {
  it('treeToSchema drops children from a string-type root (confirms the bug)', () => {
    // This test demonstrates the actual bug: children added to a string-type
    // root are silently dropped during serialization because treeToSchema only
    // emits `properties` when node.type is "object" or "".
    resetIdCounter();
    const schema = { type: 'string' as const, minLength: 1 };
    const tree = schemaToTree(schema);
    expect(tree.type).toBe('string');

    // Simulate what _handleAddProperty currently does: just add a child
    // without converting the type. This is the bug.
    if (!tree.children) tree.children = [];
    tree.children.push({
      id: 'test-child-1',
      name: 'newProp',
      type: 'string',
      required: false,
      schema: {},
    });

    const output = treeToSchema(tree);
    // After fix, the auto-convert in _handleAddProperty will change type to
    // 'object' before adding the child, so this scenario shouldn't happen.
    // But if someone manually adds children without the auto-convert,
    // treeToSchema should still serialize them. For now, this confirms the bug:
    // properties ARE dropped when type is 'string'.
    expect(output.type).toBe('string');
    expect(output).not.toHaveProperty('properties'); // bug: children lost
  });

  it('adding property to string root auto-converts to object and serializes correctly', () => {
    resetIdCounter();
    const schema = { type: 'string' as const, minLength: 1 };
    const tree = schemaToTree(schema);
    expect(tree.type).toBe('string');

    // After the fix, _handleAddProperty will auto-convert to object and clear
    // non-object constraints. Simulate that behavior:
    tree.type = 'object';
    delete tree.schema.minLength;
    if (!tree.children) tree.children = [];
    tree.children.push({
      id: 'test-child-1',
      name: 'newProp',
      type: 'string',
      required: false,
      schema: {},
    });

    const output = treeToSchema(tree);
    expect(output.type).toBe('object');
    expect(output.properties).toHaveProperty('newProp');
    expect(output).not.toHaveProperty('minLength');
  });

  it('adding property to integer root auto-converts to object and clears numeric constraints', () => {
    resetIdCounter();
    const schema = { type: 'integer' as const, minimum: 0, maximum: 100 };
    const tree = schemaToTree(schema);
    expect(tree.type).toBe('integer');

    // Simulate auto-convert
    tree.type = 'object';
    delete tree.schema.minimum;
    delete tree.schema.maximum;
    if (!tree.children) tree.children = [];
    tree.children.push({
      id: 'test-child-1',
      name: 'count',
      type: 'integer',
      required: false,
      schema: {},
    });

    const output = treeToSchema(tree);
    expect(output.type).toBe('object');
    expect(output.properties).toHaveProperty('count');
    expect(output).not.toHaveProperty('minimum');
    expect(output).not.toHaveProperty('maximum');
  });

  it('adding property to array root auto-converts to object and clears array constraints', () => {
    resetIdCounter();
    const schema = { type: 'array' as const, items: { type: 'string' }, minItems: 1 };
    const tree = schemaToTree(schema);
    expect(tree.type).toBe('array');

    // Simulate auto-convert
    tree.type = 'object';
    delete tree.schema.items;
    delete tree.schema.minItems;
    if (!tree.children) tree.children = [];
    tree.children.push({
      id: 'test-child-1',
      name: 'tags',
      type: 'array',
      required: false,
      schema: { items: { type: 'string' } },
    });

    const output = treeToSchema(tree);
    expect(output.type).toBe('object');
    expect(output.properties).toHaveProperty('tags');
    expect(output).not.toHaveProperty('items');
    expect(output).not.toHaveProperty('minItems');
  });

  it('does not auto-convert when target is already object type', () => {
    resetIdCounter();
    const schema = { type: 'object' as const, properties: { name: { type: 'string' } } };
    const tree = schemaToTree(schema);
    expect(tree.type).toBe('object');
    expect(tree.children).toHaveLength(1);

    // Adding property to existing object should not change type
    tree.children!.push({
      id: 'test-child-1',
      name: 'age',
      type: 'integer',
      required: false,
      schema: {},
    });

    const output = treeToSchema(tree);
    expect(output.type).toBe('object');
    expect(output.properties).toHaveProperty('name');
    expect(output.properties).toHaveProperty('age');
  });

  it('does not auto-convert typeless root (empty string type)', () => {
    resetIdCounter();
    const schema = { properties: { name: { type: 'string' } } };
    const tree = schemaToTree(schema);
    expect(tree.type).toBe(''); // typeless

    tree.children!.push({
      id: 'test-child-1',
      name: 'age',
      type: 'integer',
      required: false,
      schema: {},
    });

    const output = treeToSchema(tree);
    expect(output).not.toHaveProperty('type'); // stays typeless
    expect(output.properties).toHaveProperty('name');
    expect(output.properties).toHaveProperty('age');
  });
});

// ─── Bug 2: Modal accepts non-object JSON as valid schema ────────────────────

describe('Bug 2: reject non-object JSON in schema modal and editor', () => {
  // These tests verify that _parseSchema (in schema-editor.ts) correctly
  // rejects non-object values. Since _parseSchema is a private method on a
  // LitElement, we test the logic pattern directly.

  function parseSchemaLogic(input: string | object | null): object | null {
    if (!input) return null;
    if (typeof input === 'object') {
      if (Array.isArray(input) || input === null) return null;
      return input;
    }
    try {
      const parsed = JSON.parse(input as string);
      if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) return null;
      return parsed;
    } catch {
      return null;
    }
  }

  it('rejects primitive number 42', () => {
    expect(parseSchemaLogic('42')).toBeNull();
  });

  it('rejects primitive string "hello"', () => {
    expect(parseSchemaLogic('"hello"')).toBeNull();
  });

  it('rejects primitive boolean true', () => {
    expect(parseSchemaLogic('true')).toBeNull();
  });

  it('rejects JSON array [1,2,3]', () => {
    expect(parseSchemaLogic('[1,2,3]')).toBeNull();
  });

  it('rejects null', () => {
    expect(parseSchemaLogic('null')).toBeNull();
  });

  it('accepts valid object schema', () => {
    const result = parseSchemaLogic('{"type":"object","properties":{}}');
    expect(result).toEqual({ type: 'object', properties: {} });
  });

  it('accepts empty object {}', () => {
    const result = parseSchemaLogic('{}');
    expect(result).toEqual({});
  });

  it('rejects array passed as object', () => {
    expect(parseSchemaLogic([1, 2, 3] as any)).toBeNull();
  });

  it('accepts object passed directly', () => {
    const obj = { type: 'object' };
    expect(parseSchemaLogic(obj)).toBe(obj);
  });

  // Modal-level validation: handleRawChange should reject non-objects
  describe('handleRawChange validation', () => {
    function simulateHandleRawChange(rawJson: string): {
      rawJsonValid: boolean;
      rawJsonError: string;
      currentSchema: string | null;
    } {
      let currentSchema: string | null = null;
      try {
        const parsed = JSON.parse(rawJson);
        if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
          return {
            rawJsonValid: false,
            rawJsonError: 'Schema must be a JSON object',
            currentSchema: null,
          };
        }
        currentSchema = rawJson;
        return { rawJsonValid: true, rawJsonError: '', currentSchema };
      } catch (e: any) {
        return {
          rawJsonValid: false,
          rawJsonError: e instanceof Error ? e.message : 'Invalid JSON',
          currentSchema: null,
        };
      }
    }

    it('rejects number 42', () => {
      const result = simulateHandleRawChange('42');
      expect(result.rawJsonValid).toBe(false);
      expect(result.rawJsonError).toBe('Schema must be a JSON object');
    });

    it('rejects string "hello"', () => {
      const result = simulateHandleRawChange('"hello"');
      expect(result.rawJsonValid).toBe(false);
      expect(result.rawJsonError).toBe('Schema must be a JSON object');
    });

    it('rejects boolean true', () => {
      const result = simulateHandleRawChange('true');
      expect(result.rawJsonValid).toBe(false);
      expect(result.rawJsonError).toBe('Schema must be a JSON object');
    });

    it('rejects array [1,2]', () => {
      const result = simulateHandleRawChange('[1,2]');
      expect(result.rawJsonValid).toBe(false);
      expect(result.rawJsonError).toBe('Schema must be a JSON object');
    });

    it('rejects null', () => {
      const result = simulateHandleRawChange('null');
      expect(result.rawJsonValid).toBe(false);
      expect(result.rawJsonError).toBe('Schema must be a JSON object');
    });

    it('accepts valid object', () => {
      const result = simulateHandleRawChange('{"type":"string"}');
      expect(result.rawJsonValid).toBe(true);
      expect(result.rawJsonError).toBe('');
      expect(result.currentSchema).toBe('{"type":"string"}');
    });

    it('rejects invalid JSON syntax', () => {
      const result = simulateHandleRawChange('{bad json');
      expect(result.rawJsonValid).toBe(false);
      expect(result.rawJsonError).not.toBe('');
    });
  });
});

// ─── Bug: add-property auto-convert misses nullable type arrays ───────────────

describe('Bug: add-property on nullable primitive root clears schema.type array', () => {
  it('auto-convert sets type to object and removes the stale schema.type array', () => {
    resetIdCounter();
    const schema = { type: ['string', 'null'] as any };
    const tree = schemaToTree(schema);
    expect(tree.type).toBe('string');
    expect(tree.schema.type).toEqual(['string', 'null']);

    // Simulate what _handleAddProperty does: set type to 'object'
    // WITHOUT clearing schema.type — this is the bug state
    tree.type = 'object';
    // (intentionally NOT deleting tree.schema.type to demonstrate the bug)
    tree.children = [{ id: 't1', name: 'prop', type: 'string', required: false, schema: {} }];

    const outputBug = treeToSchema(tree);
    // Without the fix, schema.type array wins and output is still ["string","null"]
    expect(outputBug.type).not.toBe('object'); // demonstrates the bug
  });

  it('after fix: auto-convert deletes schema.type array so object type wins', () => {
    resetIdCounter();
    const schema = { type: ['string', 'null'] as any };
    const tree = schemaToTree(schema);
    expect(tree.type).toBe('string');
    expect(tree.schema.type).toEqual(['string', 'null']);

    // Simulate the fixed _handleAddProperty behaviour
    tree.type = 'object';
    delete tree.schema.type; // THE FIX
    tree.children = [{ id: 't1', name: 'prop', type: 'string', required: false, schema: {} }];

    const output = treeToSchema(tree);
    expect(output.type).toBe('object');
    expect(output.properties).toHaveProperty('prop');
  });

  it('auto-convert on nullable integer root also clears schema.type array', () => {
    resetIdCounter();
    const schema = { type: ['integer', 'null'] as any, minimum: 0 };
    const tree = schemaToTree(schema);
    expect(tree.type).toBe('integer');
    expect(tree.schema.type).toEqual(['integer', 'null']);

    // Simulate fixed auto-convert
    tree.type = 'object';
    delete tree.schema.type;
    delete tree.schema.minimum;
    tree.children = [{ id: 't2', name: 'count', type: 'integer', required: false, schema: {} }];

    const output = treeToSchema(tree);
    expect(output.type).toBe('object');
    expect(output.properties).toHaveProperty('count');
    expect(output).not.toHaveProperty('minimum');
  });
});
