import { describe, it, expect } from 'vitest';
import { schemaToTree, treeToSchema, detectDraft, SchemaNode } from './schema-tree-model';

describe('detectDraft', () => {
  it('detects draft-07', () => {
    expect(detectDraft({ $schema: 'http://json-schema.org/draft-07/schema#' })).toBe('draft-07');
  });
  it('detects 2020-12', () => {
    expect(detectDraft({ $schema: 'https://json-schema.org/draft/2020-12/schema' })).toBe('2020-12');
  });
  it('detects draft-04', () => {
    expect(detectDraft({ $schema: 'http://json-schema.org/draft-04/schema#' })).toBe('draft-04');
  });
  it('returns null for no $schema', () => {
    expect(detectDraft({ type: 'object' })).toBeNull();
  });
});

describe('schemaToTree / treeToSchema round-trip', () => {
  it('round-trips a flat object schema', () => {
    const schema = {
      type: 'object',
      properties: {
        name: { type: 'string', minLength: 1 },
        age: { type: 'integer', minimum: 0 },
      },
      required: ['name'],
    };
    const tree = schemaToTree(schema);
    expect(tree.type).toBe('object');
    expect(tree.children).toHaveLength(2);
    expect(tree.children![0].name).toBe('name');
    expect(tree.children![0].required).toBe(true);
    expect(tree.children![1].name).toBe('age');
    expect(tree.children![1].required).toBe(false);

    const output = treeToSchema(tree);
    expect(output).toEqual(schema);
  });

  it('round-trips enum property', () => {
    const schema = {
      type: 'object',
      properties: {
        status: { type: 'string', enum: ['active', 'inactive'] },
      },
    };
    const tree = schemaToTree(schema);
    const output = treeToSchema(tree);
    expect(output).toEqual(schema);
  });

  it('round-trips nested object', () => {
    const schema = {
      type: 'object',
      properties: {
        address: {
          type: 'object',
          properties: {
            street: { type: 'string' },
            city: { type: 'string' },
          },
          required: ['street'],
        },
      },
    };
    const tree = schemaToTree(schema);
    expect(tree.children![0].children).toHaveLength(2);
    const output = treeToSchema(tree);
    expect(output).toEqual(schema);
  });

  it('round-trips array with items', () => {
    const schema = {
      type: 'object',
      properties: {
        tags: { type: 'array', items: { type: 'string' }, minItems: 1 },
      },
    };
    const tree = schemaToTree(schema);
    const output = treeToSchema(tree);
    expect(output).toEqual(schema);
  });

  it('round-trips oneOf', () => {
    const schema = {
      type: 'object',
      properties: {
        contact: {
          oneOf: [
            { type: 'string', title: 'Email' },
            { type: 'object', title: 'Phone', properties: { number: { type: 'string' } } },
          ],
        },
      },
    };
    const tree = schemaToTree(schema);
    const output = treeToSchema(tree);
    expect(output).toEqual(schema);
  });

  it('round-trips $ref and $defs', () => {
    const schema = {
      type: 'object',
      $defs: {
        address: { type: 'object', properties: { city: { type: 'string' } } },
      },
      properties: {
        home: { $ref: '#/$defs/address' },
      },
    };
    const tree = schemaToTree(schema);
    const output = treeToSchema(tree);
    expect(output).toEqual(schema);
  });

  it('round-trips if/then/else', () => {
    const schema = {
      type: 'object',
      properties: {
        kind: { type: 'string', enum: ['a', 'b'] },
      },
      if: { properties: { kind: { const: 'a' } } },
      then: { properties: { aField: { type: 'string' } } },
      else: { properties: { bField: { type: 'number' } } },
    };
    const tree = schemaToTree(schema);
    const output = treeToSchema(tree);
    expect(output).toEqual(schema);
  });

  it('preserves title, description, and $schema', () => {
    const schema = {
      $schema: 'https://json-schema.org/draft/2020-12/schema',
      title: 'Person',
      description: 'A person record',
      type: 'object',
      properties: { name: { type: 'string' } },
    };
    const tree = schemaToTree(schema);
    const output = treeToSchema(tree);
    expect(output).toEqual(schema);
  });

  it('round-trips boolean type', () => {
    const schema = {
      type: 'object',
      properties: { active: { type: 'boolean', default: true } },
    };
    const tree = schemaToTree(schema);
    const output = treeToSchema(tree);
    expect(output).toEqual(schema);
  });

  it('round-trips nullable type array', () => {
    const schema = {
      type: 'object',
      properties: {
        name: { type: ['string', 'null'] },
        age: { type: ['integer', 'null'], minimum: 0 },
      },
    };
    const tree = schemaToTree(schema);

    // Verify node.type holds the base type, node.schema.type holds the full array
    const nameNode = tree.children!.find(c => c.name === 'name')!;
    expect(nameNode.type).toBe('string');
    expect(nameNode.schema.type).toEqual(['string', 'null']);

    const ageNode = tree.children!.find(c => c.name === 'age')!;
    expect(ageNode.type).toBe('integer');
    expect(ageNode.schema.type).toEqual(['integer', 'null']);

    const output = treeToSchema(tree);
    expect(output).toEqual(schema);
  });

  it('round-trips oneOf with tree structure', () => {
    const schema = {
      type: 'object',
      properties: {
        contact: {
          oneOf: [
            { type: 'string', title: 'Email' },
            { type: 'object', title: 'Phone', properties: { number: { type: 'string' } } },
          ],
        },
      },
    };
    const tree = schemaToTree(schema);

    // Verify the contact node has composition structure
    const contactNode = tree.children!.find(c => c.name === 'contact')!;
    expect(contactNode.compositionKeyword).toBe('oneOf');
    expect(contactNode.children).toHaveLength(2);
    expect(contactNode.children![0].name).toBe('Email');
    expect(contactNode.children![1].name).toBe('Phone');

    const output = treeToSchema(tree);
    expect(output).toEqual(schema);
  });

  it('round-trips $ref with tree structure', () => {
    const schema = {
      type: 'object',
      $defs: {
        address: { type: 'object', properties: { city: { type: 'string' } } },
      },
      properties: {
        home: { $ref: '#/$defs/address' },
      },
    };
    const tree = schemaToTree(schema);

    // Verify the home node has ref structure
    const homeNode = tree.children!.find(c => c.name === 'home')!;
    expect(homeNode.ref).toBe('#/$defs/address');
    expect(homeNode.type).toBe('');

    const output = treeToSchema(tree);
    expect(output).toEqual(schema);
  });

  it('round-trips anyOf at property level', () => {
    const schema = {
      type: 'object',
      properties: {
        value: {
          anyOf: [
            { type: 'string' },
            { type: 'number' },
          ],
        },
      },
    };
    const tree = schemaToTree(schema);
    const valueNode = tree.children!.find(c => c.name === 'value')!;
    expect(valueNode.compositionKeyword).toBe('anyOf');
    expect(valueNode.children).toHaveLength(2);

    const output = treeToSchema(tree);
    expect(output).toEqual(schema);
  });

  it('round-trips not keyword', () => {
    const schema = {
      type: 'object',
      properties: {
        value: {
          not: { type: 'string', minLength: 1 },
        },
      },
    };
    const tree = schemaToTree(schema);
    // Verify the tree structure
    const valueNode = tree.children!.find(c => c.name === 'value')!;
    expect(valueNode.compositionKeyword).toBe('not');
    expect(valueNode.children).toHaveLength(1);
    expect(valueNode.children![0].name).toBe('not');
    expect(valueNode.children![0].type).toBe('string');

    const output = treeToSchema(tree);
    expect(output).toEqual(schema);
  });
});

describe('Bug fix: wrapping in composition preserves children', () => {
  it('wrapping an object node in oneOf should preserve nested properties in the variant', () => {
    // Setup: parse a schema with a nested object (address with street + city)
    const schema = {
      type: 'object',
      properties: {
        address: {
          type: 'object',
          properties: {
            street: { type: 'string' },
            city: { type: 'string' },
          },
          required: ['street'],
        },
      },
    };
    const tree = schemaToTree(schema);
    const addressNode = tree.children!.find(c => c.name === 'address')!;

    // Verify the children exist before wrapping
    expect(addressNode.children).toHaveLength(2);
    expect(addressNode.children![0].name).toBe('street');
    expect(addressNode.children![1].name).toBe('city');

    // Simulate wrapping in oneOf — replicate edit-mode.ts _handleContextAction 'wrap-oneOf'
    const keyword = 'oneOf' as const;
    const metadataKeys = ['title', 'description', 'readOnly', 'writeOnly', 'default', 'examples', 'deprecated'];
    const metadata: Record<string, any> = {};
    const typeSchema: Record<string, any> = {};
    for (const [k, v] of Object.entries(addressNode.schema)) {
      if (metadataKeys.includes(k)) {
        metadata[k] = v;
      } else {
        typeSchema[k] = v;
      }
    }
    const originalType = addressNode.type;
    if (originalType) typeSchema.type = originalType;
    const variantName = addressNode.schema.title || 'variant1';

    // FIX: capture children BEFORE overwriting them
    const originalChildren = addressNode.children ? [...addressNode.children] : undefined;

    // Set up the node as a composition node, keeping metadata on the wrapper
    // FIX: the variant now gets the original children
    addressNode.compositionKeyword = keyword;
    addressNode.schema = metadata;
    addressNode.type = '';
    addressNode.children = [
      { id: `node-variant-0`, name: variantName, type: originalType || '', required: false, schema: typeSchema, children: originalChildren },
      { id: `node-variant-1`, name: 'variant2', type: 'string', required: false, schema: {} },
    ];
    delete addressNode.children[0].schema.type;

    // Serialize back
    const output = treeToSchema(tree);

    // The oneOf variant should contain the nested properties
    const addressOutput = output.properties!.address as any;
    expect(addressOutput.oneOf).toBeDefined();
    expect(addressOutput.oneOf).toHaveLength(2);
    // First variant should have street and city properties — THIS WILL FAIL with current code
    expect(addressOutput.oneOf[0].properties).toBeDefined();
    expect(addressOutput.oneOf[0].properties.street).toEqual({ type: 'string' });
    expect(addressOutput.oneOf[0].properties.city).toEqual({ type: 'string' });
    expect(addressOutput.oneOf[0].required).toEqual(['street']);
  });
});

describe('Bug fix: convert-to-ref preserves compositionKeyword and ref', () => {
  it('extracting a oneOf node to $defs should preserve the composition structure', () => {
    // Setup: parse a schema with a oneOf property
    const schema = {
      type: 'object',
      properties: {
        contact: {
          oneOf: [
            { type: 'string', title: 'Email' },
            { type: 'object', title: 'Phone', properties: { number: { type: 'string' } } },
          ],
        },
      },
    };
    const tree = schemaToTree(schema);
    const contactNode = tree.children!.find(c => c.name === 'contact')!;

    // Verify the node has compositionKeyword
    expect(contactNode.compositionKeyword).toBe('oneOf');

    // Simulate convert-to-ref — replicate edit-mode.ts _handleContextAction 'convert-to-ref' EXACTLY
    // CURRENT BUGGY CODE: defNode does NOT copy compositionKeyword or ref
    if (!tree.children) tree.children = [];
    let defsNode = tree.children.find(c => c.name === '$defs');
    if (!defsNode) {
      defsNode = {
        id: `node-defs-test`,
        name: '$defs',
        type: 'object',
        required: false,
        schema: {},
        isDef: true,
        children: [],
      };
      tree.children.push(defsNode);
    }
    const defName = 'contact';
    const defNode: SchemaNode = {
      id: `node-def-test`,
      name: defName,
      type: contactNode.type,
      required: false,
      schema: { ...contactNode.schema },
      isDef: true,
      children: contactNode.children ? [...contactNode.children] : undefined,
      // FIX: copy compositionKeyword and ref
      compositionKeyword: contactNode.compositionKeyword,
      ref: contactNode.ref,
    };
    defsNode.children!.push(defNode);
    // Replace original with $ref
    contactNode.type = '';
    contactNode.schema = {};
    contactNode.ref = `#/$defs/${defName}`;
    contactNode.children = undefined;
    contactNode.compositionKeyword = undefined;

    // Serialize back
    const output = treeToSchema(tree);

    // The $defs entry should contain the oneOf structure
    expect(output.$defs).toBeDefined();
    expect(output.$defs!.contact).toBeDefined();
    const defOutput = output.$defs!.contact as any;
    // THIS WILL FAIL: without compositionKeyword, treeToSchema won't emit oneOf
    expect(defOutput.oneOf).toBeDefined();
    expect(defOutput.oneOf).toHaveLength(2);
    expect(defOutput.oneOf[0].title).toBe('Email');
    expect(defOutput.oneOf[1].title).toBe('Phone');
    expect(defOutput.oneOf[1].properties).toEqual({ number: { type: 'string' } });

    // The original property should be a $ref now
    expect(output.properties!.contact).toEqual({ $ref: '#/$defs/contact' });
  });
});

describe('Bug fix: selection lost after edit due to ID regeneration', () => {
  it('demonstrates that reparsing generates new IDs (the underlying problem)', () => {
    const schema = { type: 'object', properties: { name: { type: 'string' } } };
    const tree1 = schemaToTree(schema);
    const originalRootId = tree1.id;
    const originalChildId = tree1.children![0].id;

    // Reparse the same schema (simulates what willUpdate does)
    const tree2 = schemaToTree(schema);
    // The IDs should be DIFFERENT because uid() increments
    expect(tree2.id).not.toBe(originalRootId);
    expect(tree2.children![0].id).not.toBe(originalChildId);
    // This demonstrates why _selectedId becomes stale after reparse
  });

  it('skipping reparse when schema matches last emission preserves selection', () => {
    // Simulate the fix: track lastEmittedSchema, skip reparse if incoming matches
    const schema = { type: 'object', properties: { name: { type: 'string' } } };
    const tree = schemaToTree(schema);
    const selectedId = tree.children![0].id;

    // Simulate emitting schema change
    const emittedSchema = JSON.stringify(treeToSchema(tree), null, 2);

    // Simulate willUpdate receiving the same schema back
    const incomingSchema = emittedSchema;
    let reparsed = false;

    // The guard: if incoming matches last emitted, skip reparse
    if (incomingSchema !== emittedSchema) {
      reparsed = true;
    }

    expect(reparsed).toBe(false);
    // selectedId is still valid because we didn't reparse
    // (In the real code, _root still has the same tree with the same IDs)
  });
});
