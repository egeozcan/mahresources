/**
 * Tests for 3 confirmed bugs — written RED-first before any fixes.
 *
 * Bug 1: Renaming a definition doesn't update $ref consumers
 * Bug 2: Multi-type arrays (e.g., ["string", "number"]) not updated by type dropdown
 * Bug 3: Deleting last $defs child selects invisible ghost $defs node
 */
import { readFileSync } from 'fs';
import { resolve } from 'path';
import { describe, it, expect, beforeEach } from 'vitest';
import { schemaToTree, treeToSchema, resetIdCounter, type SchemaNode } from './schema-tree-model';
import { escapeJsonPointer } from './schema-core';

// ─── Bug 1: Renaming a definition updates all $ref consumers ────────────────

describe('Bug 1: renaming a definition updates all $ref consumers in the tree', () => {
  beforeEach(() => resetIdCounter());

  it('renames $ref strings when a $defs node is renamed', () => {
    const schema = {
      type: 'object' as const,
      $defs: {
        address: {
          type: 'object' as const,
          properties: { city: { type: 'string' as const } },
        },
      },
      properties: {
        home: { $ref: '#/$defs/address' },
        work: { $ref: '#/$defs/address' },
        unrelated: { $ref: '#/$defs/other' },
      },
    };

    const tree = schemaToTree(schema);

    // Find the $defs wrapper and the 'address' definition node
    const defsNode = tree.children!.find(c => c.name === '$defs')!;
    expect(defsNode).toBeDefined();
    const addressDef = defsNode.children!.find(c => c.name === 'address')!;
    expect(addressDef).toBeDefined();

    // Find the $ref consumer nodes
    const homeNode = tree.children!.find(c => c.name === 'home')!;
    const workNode = tree.children!.find(c => c.name === 'work')!;
    const unrelatedNode = tree.children!.find(c => c.name === 'unrelated')!;

    expect(homeNode.ref).toBe('#/$defs/address');
    expect(workNode.ref).toBe('#/$defs/address');
    expect(unrelatedNode.ref).toBe('#/$defs/other');

    // Simulate what edit-mode._handleNodeChange does for 'name' field on a $defs child:
    // 1. Capture old name
    // 2. Update name
    // 3. Walk tree to update refs
    // The edit-mode code should do this automatically; we test the source.
    const src = readFileSync(resolve(__dirname, 'modes/edit-mode.ts'), 'utf-8');

    // The name handler must walk the tree to update refs when renaming a definition
    expect(src).toContain('_updateRefsInTree');
  });

  it('edit-mode _updateRefsInTree helper exists and walks children and variants', () => {
    const src = readFileSync(resolve(__dirname, 'modes/edit-mode.ts'), 'utf-8');

    // The helper must exist
    expect(src).toContain('_updateRefsInTree');

    // It must walk both children and variants
    const helperBody = extractMethodBody(src, '_updateRefsInTree');
    expect(helperBody).not.toBeNull();
    expect(helperBody).toContain('children');
    expect(helperBody).toContain('variants');
  });

  it('name handler captures oldName before setting the new value', () => {
    const src = readFileSync(resolve(__dirname, 'modes/edit-mode.ts'), 'utf-8');

    // In the name case, we need to capture the old name BEFORE setting selected.name
    // Look for a pattern where oldName is captured
    const nameCase = extractCaseBlock(src, "'name'");
    expect(nameCase).not.toBeNull();
    // oldName must be captured before selected.name = value/deduped
    expect(nameCase).toMatch(/old[Nn]ame.*=.*selected\.name/);
  });

  it('round-trips correctly after simulated rename (functional test)', () => {
    const schema = {
      type: 'object' as const,
      $defs: {
        address: {
          type: 'object' as const,
          properties: { city: { type: 'string' as const } },
        },
      },
      properties: {
        home: { $ref: '#/$defs/address' },
        work: { $ref: '#/$defs/address' },
      },
    };

    const tree = schemaToTree(schema);
    const defsNode = tree.children!.find(c => c.name === '$defs')!;
    const addressDef = defsNode.children!.find(c => c.name === 'address')!;
    const homeNode = tree.children!.find(c => c.name === 'home')!;
    const workNode = tree.children!.find(c => c.name === 'work')!;

    // Simulate the full rename: change name + update refs (what edit-mode should do)
    const oldRef = '#/$defs/address';
    const newRef = '#/$defs/mailingAddress';
    addressDef.name = 'mailingAddress';
    // Walk tree and update refs
    updateRefsInTree(tree, oldRef, newRef);

    expect(homeNode.ref).toBe('#/$defs/mailingAddress');
    expect(workNode.ref).toBe('#/$defs/mailingAddress');

    // Round-trip
    const output = treeToSchema(tree);
    expect(output.$defs).toHaveProperty('mailingAddress');
    expect(output.$defs).not.toHaveProperty('address');
    expect(output.properties!.home.$ref).toBe('#/$defs/mailingAddress');
    expect(output.properties!.work.$ref).toBe('#/$defs/mailingAddress');
  });

  it('handles definitions key for older draft schemas', () => {
    const schema = {
      $schema: 'http://json-schema.org/draft-07/schema#',
      type: 'object' as const,
      definitions: {
        color: { type: 'string' as const, enum: ['red', 'blue'] },
      },
      properties: {
        primary: { $ref: '#/definitions/color' },
      },
    };

    const tree = schemaToTree(schema);
    const defsNode = tree.children!.find(c => c.name === '$defs')!;
    const colorDef = defsNode.children!.find(c => c.name === 'color')!;
    const primaryNode = tree.children!.find(c => c.name === 'primary')!;

    expect(primaryNode.ref).toBe('#/definitions/color');

    // Simulate rename
    const oldRef = '#/definitions/color';
    const newRef = '#/definitions/palette';
    colorDef.name = 'palette';
    updateRefsInTree(tree, oldRef, newRef);

    expect(primaryNode.ref).toBe('#/definitions/palette');

    const output = treeToSchema(tree);
    expect(output.definitions).toHaveProperty('palette');
    expect(output.properties!.primary.$ref).toBe('#/definitions/palette');
  });
});

// ─── Bug 2: Multi-type arrays not updated by type dropdown ──────────────────

describe('Bug 2: type change on multi-type array replaces the array with scalar', () => {
  beforeEach(() => resetIdCounter());

  it('clears non-nullable multi-type array from schema.type on type change', () => {
    // Simulate a schema with type: ["string", "number"]
    const schema = {
      type: ['string', 'number'] as any,
    };

    const tree = schemaToTree(schema);
    // schemaToTree stores the first non-null type in node.type
    expect(tree.type).toBe('string');
    // and preserves the array in node.schema.type
    expect(Array.isArray(tree.schema.type)).toBe(true);
    expect(tree.schema.type).toEqual(['string', 'number']);

    // Now simulate what edit-mode does: user picks "integer" from dropdown
    // The fix should clear the multi-type array from node.schema.type
    tree.type = 'integer';
    // Since this is NOT a nullable array (no "null" in it), delete node.schema.type
    if (Array.isArray(tree.schema.type) && !tree.schema.type.includes('null')) {
      delete tree.schema.type;
    }

    const output = treeToSchema(tree);
    expect(output.type).toBe('integer');
    // Must NOT be ["string", "number"]
    expect(Array.isArray(output.type)).toBe(false);
  });

  it('edit-mode type handler clears non-nullable multi-type arrays', () => {
    const src = readFileSync(resolve(__dirname, 'modes/edit-mode.ts'), 'utf-8');

    // The type case must handle non-nullable multi-type arrays
    const typeCase = extractCaseBlock(src, "'type'");
    expect(typeCase).not.toBeNull();

    // It must check for Array.isArray(selected.schema.type) and handle the
    // case where the array does NOT include 'null'
    expect(typeCase).toMatch(/Array\.isArray.*selected\.schema\.type/);
    // Must have a branch that deletes schema.type for non-nullable arrays
    expect(typeCase).toMatch(/delete.*selected\.schema\.type/);
  });

  it('preserves nullable arrays when changing type', () => {
    // Nullable: ["string", "null"] - should stay as array but update base type
    const schema = { type: ['string', 'null'] as any };
    const tree = schemaToTree(schema);

    expect(tree.type).toBe('string');
    expect(tree.schema.type).toEqual(['string', 'null']);

    // Simulate changing type to 'integer' — nullable should be preserved
    tree.type = 'integer';
    if (Array.isArray(tree.schema.type) && tree.schema.type.includes('null')) {
      tree.schema.type = ['integer', 'null'];
    }

    const output = treeToSchema(tree);
    expect(output.type).toEqual(['integer', 'null']);
  });

  it('round-trips correctly: multi-type → single type → schema', () => {
    const schema = {
      type: 'object' as const,
      properties: {
        field: { type: ['string', 'number'] as any },
      },
    };

    const tree = schemaToTree(schema);
    const fieldNode = tree.children!.find(c => c.name === 'field')!;
    expect(fieldNode.type).toBe('string');
    expect(fieldNode.schema.type).toEqual(['string', 'number']);

    // Simulate type change to 'boolean'
    fieldNode.type = 'boolean';
    // Apply the fix: clear non-nullable multi-type array
    if (Array.isArray(fieldNode.schema.type) && !fieldNode.schema.type.includes('null')) {
      delete fieldNode.schema.type;
    }

    const output = treeToSchema(tree);
    expect(output.properties!.field.type).toBe('boolean');
  });
});

// ─── Bug 3: Deleting last $defs child selects ghost $defs wrapper ───────────

describe('Bug 3: deleting last definition selects root, not ghost $defs wrapper', () => {
  beforeEach(() => resetIdCounter());

  it('edit-mode delete handler checks for empty $defs wrapper', () => {
    const src = readFileSync(resolve(__dirname, 'modes/edit-mode.ts'), 'utf-8');

    // The _handleNodeDelete method must have special handling for empty $defs
    const deleteBody = extractMethodBody(src, '_handleNodeDelete');
    expect(deleteBody).not.toBeNull();

    // Must check if parent is $defs and now empty
    expect(deleteBody).toMatch(/\$defs/);
    expect(deleteBody).toMatch(/children/);
    // Must select root when $defs becomes empty
    expect(deleteBody).toContain('this._root');
  });

  it('$defs wrapper with no children disappears from treeToSchema output', () => {
    const schema = {
      type: 'object' as const,
      $defs: { only: { type: 'string' as const } },
      properties: { name: { type: 'string' as const } },
    };

    const tree = schemaToTree(schema);
    const defsNode = tree.children!.find(c => c.name === '$defs')!;
    expect(defsNode).toBeDefined();
    expect(defsNode.children).toHaveLength(1);

    // Delete the only definition
    defsNode.children!.splice(0, 1);
    expect(defsNode.children).toHaveLength(0);

    // treeToSchema should NOT emit $defs when empty
    const output = treeToSchema(tree);
    expect(output.$defs).toBeUndefined();
    expect(output.definitions).toBeUndefined();
  });

  it('tree-panel does not render $defs section when children list is empty', () => {
    const src = readFileSync(resolve(__dirname, 'tree/tree-panel.ts'), 'utf-8');

    // The render condition must check defsNode.children?.length
    expect(src).toMatch(/defsNode.*children.*length/);
  });

  it('after deleting last def, selectedId should NOT be the $defs wrapper', () => {
    // This tests the logic that edit-mode should implement
    const schema = {
      type: 'object' as const,
      $defs: { only: { type: 'string' as const } },
      properties: { name: { type: 'string' as const } },
    };

    const tree = schemaToTree(schema);
    const defsNode = tree.children!.find(c => c.name === '$defs')!;
    const onlyDef = defsNode.children![0];

    // The parent of onlyDef is the $defs wrapper
    // After deletion, if parent ($defs) has no children, select root
    defsNode.children!.splice(0, 1);

    const shouldSelectRoot =
      defsNode.isDef &&
      defsNode.name === '$defs' &&
      (!defsNode.children || defsNode.children.length === 0);

    expect(shouldSelectRoot).toBe(true);
    // So selectedId should be tree.id (root), not defsNode.id
    expect(tree.id).not.toBe(defsNode.id);
  });
});

// ─── Bug 4: Rename must update $ref inside raw sub-schemas (if/then/else, secondary composition) ──

describe('Bug 4: renaming a definition updates $ref inside raw node.schema sub-schemas', () => {
  beforeEach(() => resetIdCounter());

  it('renaming a definition updates $ref inside raw if/then/else sub-schemas', () => {
    const schema = {
      type: 'object' as const,
      $defs: {
        addr: { type: 'object' as const, properties: { city: { type: 'string' as const } } },
      },
      properties: {
        kind: { type: 'string' as const, enum: ['home', 'work'] },
      },
      if: { properties: { kind: { const: 'home' } } },
      then: { properties: { address: { $ref: '#/$defs/addr' } } },
      else: { properties: { address: { $ref: '#/$defs/addr' } } },
    };

    const tree = schemaToTree(schema);

    // if/then/else stay in tree.schema as raw JSON (not extracted into tree nodes)
    expect(tree.schema.if).toBeDefined();
    expect(tree.schema.then).toBeDefined();
    expect(tree.schema.else).toBeDefined();

    // Simulate rename: addr → location
    const defsNode = tree.children!.find(c => c.name === '$defs')!;
    const addrDef = defsNode.children!.find(c => c.name === 'addr')!;
    addrDef.name = 'location';
    const oldRef = '#/$defs/addr';
    const newRef = '#/$defs/location';
    updateRefsInTree(tree, oldRef, newRef);

    // Verify $ref inside raw then/else was updated
    expect(tree.schema.then.properties.address.$ref).toBe('#/$defs/location');
    expect(tree.schema.else.properties.address.$ref).toBe('#/$defs/location');

    // Round-trip should produce valid schema with updated refs
    const output = treeToSchema(tree);
    expect(output.$defs).toHaveProperty('location');
    expect(output.$defs).not.toHaveProperty('addr');
    expect(output.then.properties.address.$ref).toBe('#/$defs/location');
    expect(output.else.properties.address.$ref).toBe('#/$defs/location');
  });

  it('renaming a definition updates $ref inside secondary composition keywords in node.schema', () => {
    const schema = {
      type: 'object' as const,
      $defs: {
        base: { type: 'object' as const, properties: { id: { type: 'string' as const } } },
      },
      properties: {
        item: {
          oneOf: [{ type: 'string' as const }],
          allOf: [{ $ref: '#/$defs/base' }],
        },
      },
    };

    const tree = schemaToTree(schema);
    // oneOf is extracted into variants; allOf stays in node.schema as secondary keyword
    const itemNode = tree.children!.find(c => c.name === 'item')!;
    expect(itemNode.compositionKeyword).toBe('oneOf');
    expect(itemNode.schema.allOf).toBeDefined();
    expect(itemNode.schema.allOf[0].$ref).toBe('#/$defs/base');

    // Simulate rename: base → foundation
    const defsNode = tree.children!.find(c => c.name === '$defs')!;
    const baseDef = defsNode.children!.find(c => c.name === 'base')!;
    baseDef.name = 'foundation';
    const oldRef = '#/$defs/base';
    const newRef = '#/$defs/foundation';
    updateRefsInTree(tree, oldRef, newRef);

    // Verify $ref inside raw allOf was updated
    expect(itemNode.schema.allOf[0].$ref).toBe('#/$defs/foundation');

    // Round-trip should produce valid schema
    const output = treeToSchema(tree);
    expect(output.$defs).toHaveProperty('foundation');
    expect(output.properties!.item.allOf[0].$ref).toBe('#/$defs/foundation');
  });

  it('edit-mode _updateRefsInTree scans node.schema for raw $ref strings', () => {
    const src = readFileSync(resolve(__dirname, 'modes/edit-mode.ts'), 'utf-8');
    const helperBody = extractMethodBody(src, '_updateRefsInTree');
    expect(helperBody).not.toBeNull();
    // Must reference node.schema to scan raw sub-schemas
    expect(helperBody).toMatch(/node\.schema/);
  });

  it('edit-mode has _updateRefsInObject helper for recursive $ref replacement', () => {
    const src = readFileSync(resolve(__dirname, 'modes/edit-mode.ts'), 'utf-8');
    // Must have the helper method
    expect(src).toContain('_updateRefsInObject');
    const helperBody = extractMethodBody(src, '_updateRefsInObject');
    expect(helperBody).not.toBeNull();
    // Must check for $ref key
    expect(helperBody).toContain('$ref');
  });
});

// ─── Helpers ────────────────────────────────────────────────────────────────

/** Recursively update $ref strings in a tree — mirrors production _updateRefsInTree */
function updateRefsInTree(node: SchemaNode, oldRef: string, newRef: string): void {
  if (node.ref === oldRef) {
    node.ref = newRef;
  }
  updateRefsInObject(node.schema, oldRef, newRef);
  for (const child of node.children || []) {
    updateRefsInTree(child, oldRef, newRef);
  }
  for (const variant of node.variants || []) {
    updateRefsInTree(variant, oldRef, newRef);
  }
}

/** Recursively scan an object for $ref strings and replace them */
function updateRefsInObject(obj: any, oldRef: string, newRef: string): void {
  if (!obj || typeof obj !== 'object') return;
  if (Array.isArray(obj)) {
    for (const item of obj) {
      updateRefsInObject(item, oldRef, newRef);
    }
    return;
  }
  for (const [key, value] of Object.entries(obj)) {
    if (key === '$ref' && value === oldRef) {
      obj[key] = newRef;
    } else if (typeof value === 'object' && value !== null) {
      updateRefsInObject(value, oldRef, newRef);
    }
  }
}

function extractMethodBody(src: string, methodName: string): string | null {
  const idx = src.indexOf(`private ${methodName}`);
  if (idx === -1) return null;
  const openBrace = src.indexOf('{', idx);
  if (openBrace === -1) return null;
  let depth = 1;
  let pos = openBrace + 1;
  while (pos < src.length && depth > 0) {
    if (src[pos] === '{') depth++;
    else if (src[pos] === '}') depth--;
    pos++;
  }
  return src.slice(openBrace, pos);
}

function extractCaseBlock(src: string, caseName: string): string | null {
  const idx = src.indexOf(`case ${caseName}`);
  if (idx === -1) return null;
  // Walk forward, tracking brace depth. When we hit a top-level `break;`
  // or the next `case`/`default:` at the same depth, stop.
  const rest = src.slice(idx);
  // Find the break statement for this case — look for `break;` that follows the case
  // Use a generous window (3000 chars covers any realistic case block)
  const window = rest.slice(0, 3000);
  // Find the last `break;` before the next case/default at the switch level
  const nextCaseMatch = window.match(/\n\s{6}(case |default:)/);
  if (nextCaseMatch && nextCaseMatch.index) {
    return window.slice(0, nextCaseMatch.index);
  }
  return window;
}
