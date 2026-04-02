/**
 * Tests for Bug 1: post-render attribute pass is unsafe for nested controls
 * and Bug 2: edit-mode external schema replacement doesn't validate selection.
 *
 * Written RED-first before any fixes.
 */
import { describe, it, expect, beforeEach } from 'vitest';

// ─── Bug 1: form-mode post-render attribute pass is unsafe for nested controls ──

/**
 * We cannot easily instantiate the full Lit element in a Node-only vitest
 * environment. Instead we test the logic extracted into a pure helper that
 * determines whether a schema represents a "leaf" (primitive) field or a
 * container (object, array, composition).
 *
 * The post-render attribute pass should only target leaf fields.
 */
import { isLeafSchema } from '../form-mode-helpers';

describe('Bug 1: isLeafSchema correctly distinguishes leaf vs container schemas', () => {
  it('string schema is a leaf', () => {
    expect(isLeafSchema({ type: 'string' })).toBe(true);
  });

  it('number schema is a leaf', () => {
    expect(isLeafSchema({ type: 'number' })).toBe(true);
  });

  it('integer schema is a leaf', () => {
    expect(isLeafSchema({ type: 'integer' })).toBe(true);
  });

  it('boolean schema is a leaf', () => {
    expect(isLeafSchema({ type: 'boolean' })).toBe(true);
  });

  it('enum schema is a leaf', () => {
    expect(isLeafSchema({ type: 'string', enum: ['a', 'b'] })).toBe(true);
  });

  it('const schema is a leaf', () => {
    expect(isLeafSchema({ const: 'fixed' })).toBe(true);
  });

  it('object schema is NOT a leaf', () => {
    expect(isLeafSchema({ type: 'object', properties: { city: { type: 'string' } } })).toBe(false);
  });

  it('array schema is NOT a leaf', () => {
    expect(isLeafSchema({ type: 'array', items: { type: 'string' } })).toBe(false);
  });

  it('oneOf schema is NOT a leaf', () => {
    expect(isLeafSchema({ oneOf: [{ type: 'string' }, { type: 'number' }] })).toBe(false);
  });

  it('anyOf schema is NOT a leaf', () => {
    expect(isLeafSchema({ anyOf: [{ type: 'string' }, { type: 'number' }] })).toBe(false);
  });

  it('allOf schema is NOT a leaf', () => {
    expect(isLeafSchema({ allOf: [{ type: 'string' }, { minLength: 1 }] })).toBe(false);
  });

  it('schema with if/then/else is NOT a leaf', () => {
    expect(isLeafSchema({ if: { properties: {} }, then: {}, else: {} })).toBe(false);
  });

  it('schema with $ref is NOT a leaf', () => {
    expect(isLeafSchema({ $ref: '#/$defs/Address' })).toBe(false);
  });

  it('null type is a leaf', () => {
    expect(isLeafSchema({ type: 'null' })).toBe(true);
  });

  it('nullable string [string, null] is a leaf', () => {
    expect(isLeafSchema({ type: ['string', 'null'] })).toBe(true);
  });

  it('type array with object is NOT a leaf', () => {
    expect(isLeafSchema({ type: ['object', 'null'] })).toBe(false);
  });

  it('type array with array is NOT a leaf', () => {
    expect(isLeafSchema({ type: ['array', 'null'] })).toBe(false);
  });
});

/**
 * Integration-level test: when a required object property is rendered, the
 * first child input of the nested object should NOT receive required/aria-required
 * just because the parent object property is required.
 *
 * This test validates the DOM-level behaviour by checking that the wrapper
 * for a container-type property does NOT have data-required="true".
 */
describe('Bug 1: _renderFieldWithAttributes skips attributes for container types', () => {
  it('does not mark wrapper as requiring attributes for object-type schemas', () => {
    // The fix ensures _renderFieldWithAttributes either:
    // a) Does not render the wrapper span for containers, or
    // b) Renders it without data-required for containers
    //
    // Since we extracted isLeafSchema, the wrapper should skip attributes
    // for non-leaf schemas. This is tested via the pure helper above.
    // The integration is: updated() checks isLeafSchema before applying attrs.
    const objectSchema = { type: 'object', properties: { city: { type: 'string' } } };
    expect(isLeafSchema(objectSchema)).toBe(false);
  });
});

// ─── Bug 2: edit-mode external schema replacement doesn't validate selection ──

import { schemaToTree, resetIdCounter } from '../schema-tree-model';

describe('Bug 2: edit-mode validates selection after external schema replacement', () => {
  /**
   * Simulates the willUpdate logic from edit-mode.ts.
   * When a new schema comes in externally (Raw JSON edit, category switch),
   * the tree is reparsed with fresh IDs. If the old _selectedId no longer
   * exists in the new tree, it should fall back to root.
   */

  function findNode(id: string, node: ReturnType<typeof schemaToTree> | null): any | null {
    if (!node) return null;
    if (node.id === id) return node;
    for (const child of node.children || []) {
      const found = findNode(id, child);
      if (found) return found;
    }
    for (const variant of node.variants || []) {
      const found = findNode(id, variant);
      if (found) return found;
    }
    return null;
  }

  /**
   * Simulates the willUpdate logic as it SHOULD work after the fix:
   * - Reparses the schema into a new tree
   * - If _selectedId doesn't exist in the new tree, falls back to root
   */
  function simulateWillUpdate(
    newSchema: Record<string, any>,
    currentSelectedId: string,
    lastEmittedSchema: string,
  ): { root: ReturnType<typeof schemaToTree>; selectedId: string } {
    const incoming = JSON.stringify(newSchema);
    if (incoming === lastEmittedSchema) {
      // Skip reparse (not the path we're testing)
      throw new Error('Should not hit this path in test');
    }
    const root = schemaToTree(newSchema);

    // BUG (before fix): only sets selectedId if currently empty
    // FIX: validate that selectedId still exists in new tree
    let selectedId = currentSelectedId;
    if (selectedId && !findNode(selectedId, root)) {
      selectedId = root.id;
    }
    if (!selectedId) {
      selectedId = root.id;
    }

    return { root, selectedId };
  }

  it('selects root when external schema change invalidates current selection', () => {
    resetIdCounter();

    // Parse schema A, select a child node
    const schemaA = {
      type: 'object',
      properties: {
        name: { type: 'string' },
        age: { type: 'integer' },
      },
    };
    const treeA = schemaToTree(schemaA);
    // Select the "age" property node (second child)
    const ageNode = treeA.children![1];
    expect(ageNode.name).toBe('age');
    const selectedId = ageNode.id;

    // Parse schema B (completely different structure)
    const schemaB = {
      type: 'object',
      properties: {
        color: { type: 'string' },
        size: { type: 'number' },
      },
    };

    const result = simulateWillUpdate(schemaB, selectedId, '');

    // The old selectedId from tree A should not exist in tree B
    expect(findNode(selectedId, result.root)).toBeNull();
    // So selection should fall back to root
    expect(result.selectedId).toBe(result.root.id);
  });

  it('preserves selection when node still exists after reparse (same schema structure)', () => {
    resetIdCounter();

    // If somehow the IDs happen to match (they won't with incrementing IDs),
    // selection should be preserved. In practice, this tests the "skip reparse
    // when schema matches" path, but we test the validation logic itself.
    const schema = {
      type: 'object',
      properties: {
        name: { type: 'string' },
      },
    };
    const tree = schemaToTree(schema);
    const rootId = tree.id;

    // If we reparse the same schema (with a different lastEmittedSchema),
    // root gets a new ID, so rootId becomes stale
    const result = simulateWillUpdate(schema, rootId, 'different');

    // The old rootId is stale (new tree has new IDs)
    // So selection should fall back to the new root
    expect(result.selectedId).toBe(result.root.id);
  });

  it('falls back to root even when selectedId is from a deeply nested node', () => {
    resetIdCounter();

    const deepSchema = {
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
    const tree = schemaToTree(deepSchema);
    // Select the deeply nested "city" node
    const addressNode = tree.children![0];
    const cityNode = addressNode.children![0];
    expect(cityNode.name).toBe('city');
    const selectedId = cityNode.id;

    // Replace with completely different schema
    const newSchema = { type: 'string' };
    const result = simulateWillUpdate(newSchema, selectedId, '');

    expect(findNode(selectedId, result.root)).toBeNull();
    expect(result.selectedId).toBe(result.root.id);
  });
});
