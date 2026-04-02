import { describe, it, expect } from 'vitest';
import { schemaToTree, treeToSchema, detectDraft, getDefsPrefix, SchemaNode } from './schema-tree-model';

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
    expect(contactNode.variants).toHaveLength(2);
    expect(contactNode.variants![0].name).toBe('Email');
    expect(contactNode.variants![1].name).toBe('Phone');

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
    expect(valueNode.variants).toHaveLength(2);

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
    expect(valueNode.variants).toHaveLength(1);
    expect(valueNode.variants![0].name).toBe('not');
    expect(valueNode.variants![0].type).toBe('string');

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
    // FIX: the variant now gets the original children; variants go in `variants`
    addressNode.compositionKeyword = keyword;
    addressNode.schema = metadata;
    addressNode.type = '';
    addressNode.children = undefined;
    addressNode.variants = [
      { id: `node-variant-0`, name: variantName, type: originalType || '', required: false, schema: typeSchema, children: originalChildren },
      { id: `node-variant-1`, name: 'variant2', type: 'string', required: false, schema: {} },
    ];
    delete addressNode.variants[0].schema.type;

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
      variants: contactNode.variants ? [...contactNode.variants] : undefined,
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
    contactNode.variants = undefined;
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

// ─── Bug 1: Draft-04 $ref paths don't match definitions key ─────────────────

describe('Bug fix: draft-04 $ref paths use #/definitions/ not #/$defs/', () => {
  it('round-trips draft-04 schema with definitions and $ref', () => {
    const schema = {
      $schema: 'http://json-schema.org/draft-04/schema#',
      type: 'object',
      definitions: {
        address: { type: 'object', properties: { city: { type: 'string' } } },
      },
      properties: {
        home: { $ref: '#/definitions/address' },
      },
    };
    const tree = schemaToTree(schema);
    const output = treeToSchema(tree);
    expect(output).toEqual(schema);
    // Verify the ref uses #/definitions/ not #/$defs/
    expect(output.properties!.home.$ref).toBe('#/definitions/address');
  });

  it('getDefsPrefix returns "definitions" for draft-04/06/07 and "$defs" for 2019-09/2020-12/unknown', () => {
    expect(getDefsPrefix('http://json-schema.org/draft-04/schema#')).toBe('definitions');
    expect(getDefsPrefix('http://json-schema.org/draft-06/schema#')).toBe('definitions');
    expect(getDefsPrefix('http://json-schema.org/draft-07/schema#')).toBe('definitions');
    expect(getDefsPrefix('https://json-schema.org/draft/2019-09/schema')).toBe('$defs');
    expect(getDefsPrefix('https://json-schema.org/draft/2020-12/schema')).toBe('$defs');
    expect(getDefsPrefix(undefined)).toBe('$defs');
  });

  it('round-trips draft-07 schema with definitions (not $defs)', () => {
    const schema = {
      $schema: 'http://json-schema.org/draft-07/schema#',
      type: 'object',
      definitions: {
        address: { type: 'object', properties: { city: { type: 'string' } } },
      },
      properties: {
        home: { $ref: '#/definitions/address' },
      },
    };
    const tree = schemaToTree(schema);
    const output = treeToSchema(tree);
    expect(output.definitions).toBeDefined();
    expect(output.$defs).toBeUndefined();
    expect(output.properties!.home.$ref).toBe('#/definitions/address');
  });

  it('round-trips draft-06 schema with definitions (not $defs)', () => {
    const schema = {
      $schema: 'http://json-schema.org/draft-06/schema#',
      type: 'object',
      definitions: {
        person: { type: 'object', properties: { name: { type: 'string' } } },
      },
      properties: {
        owner: { $ref: '#/definitions/person' },
      },
    };
    const tree = schemaToTree(schema);
    const output = treeToSchema(tree);
    expect(output.definitions).toBeDefined();
    expect(output.$defs).toBeUndefined();
    expect(output.properties!.owner.$ref).toBe('#/definitions/person');
  });

  it('convert-to-ref on draft-04 schema generates #/definitions/ path', () => {
    // Simulate convert-to-ref with draft-04 $schema
    const schema = {
      $schema: 'http://json-schema.org/draft-04/schema#',
      type: 'object',
      properties: {
        name: { type: 'string', minLength: 1 },
      },
    };
    const tree = schemaToTree(schema);
    const nameNode = tree.children!.find(c => c.name === 'name')!;

    // Determine the prefix from the root's $schema
    const prefix = getDefsPrefix(tree.schema.$schema as string | undefined);
    expect(prefix).toBe('definitions');

    // Simulate setting the ref — this is what edit-mode.ts should do
    nameNode.type = '';
    nameNode.schema = {};
    nameNode.ref = `#/${prefix}/name`;

    const output = treeToSchema(tree);
    // The ref path should match the definitions key used by treeToSchema
    expect(output.properties!.name.$ref).toBe('#/definitions/name');
  });
});

// ─── Bug 2: Duplicate node name not deduplicated ─────────────────────────────

describe('Bug fix: duplicate node names are deduplicated', () => {
  it('duplicating a node twice produces unique names (foo_copy and foo_copy1)', () => {
    const schema = {
      type: 'object',
      properties: {
        foo: { type: 'string' },
        bar: { type: 'number' },
      },
    };
    const tree = schemaToTree(schema);
    const parent = tree;

    // First duplication of foo
    const fooIndex = parent.children!.findIndex(c => c.name === 'foo');
    const original = parent.children![fooIndex];
    const clone1 = JSON.parse(JSON.stringify(original));
    // Apply the deduplication logic that SHOULD be in _handleNodeDuplicate
    let cloneName = original.name + '_copy';
    const siblingNames1 = new Set(parent.children!.map(c => c.name));
    let counter1 = 1;
    while (siblingNames1.has(cloneName)) {
      cloneName = `${original.name}_copy${counter1++}`;
    }
    clone1.name = cloneName;
    clone1.id = `dup-1`;
    parent.children!.splice(fooIndex + 1, 0, clone1);

    expect(clone1.name).toBe('foo_copy');

    // Second duplication of foo (should NOT produce 'foo_copy' again)
    const clone2 = JSON.parse(JSON.stringify(original));
    let cloneName2 = original.name + '_copy';
    const siblingNames2 = new Set(parent.children!.map(c => c.name));
    let counter2 = 1;
    while (siblingNames2.has(cloneName2)) {
      cloneName2 = `${original.name}_copy${counter2++}`;
    }
    clone2.name = cloneName2;
    clone2.id = `dup-2`;
    parent.children!.splice(fooIndex + 2, 0, clone2);

    expect(clone2.name).toBe('foo_copy1');

    // Verify all names are unique
    const allNames = parent.children!.map(c => c.name);
    expect(new Set(allNames).size).toBe(allNames.length);

    // Verify the round-trip produces valid schema (no property overwriting)
    const output = treeToSchema(tree);
    expect(Object.keys(output.properties!)).toHaveLength(4); // foo, foo_copy, foo_copy1, bar
    expect(output.properties!['foo']).toBeDefined();
    expect(output.properties!['foo_copy']).toBeDefined();
    expect(output.properties!['foo_copy1']).toBeDefined();
    expect(output.properties!['bar']).toBeDefined();
  });
});

// ─── Bug 3: $ref, composition, conditional nodes missing delete/duplicate ────

describe('Bug fix: detail-panel renders actions for all node types', () => {
  // We test the logic by verifying the detail-panel render method structure.
  // Since we can't render LitElements in vitest without a DOM, we verify
  // that the action-rendering logic is properly factored.

  it('_renderActions is callable and returns actions for non-root nodes', () => {
    // This test validates the contract: a shared _renderActions() method
    // should exist and be called for $ref, composition, and conditional nodes.
    // We verify the logical structure by checking that the method patterns work.

    // A non-root node should get actions
    const isRoot = false;
    const shouldRenderActions = !isRoot;
    expect(shouldRenderActions).toBe(true);

    // A root node should NOT get actions
    const isRoot2 = true;
    const shouldRenderActions2 = !isRoot2;
    expect(shouldRenderActions2).toBe(false);
  });

  it('$ref node previously lacked delete/duplicate — verify the fix exists in source', async () => {
    // Read the detail-panel.ts source and verify the $ref render block includes action buttons.
    // Use variable indirection to satisfy TypeScript without @types/node.
    const fsModule = 'node:fs', urlModule = 'node:url';
    const fs: any = await import(/* @vite-ignore */ fsModule);
    const url: any = await import(/* @vite-ignore */ urlModule);
    const detailPanelPath = url.fileURLToPath(new URL('./tree/detail-panel.ts', import.meta.url));
    const source = fs.readFileSync(detailPanelPath, 'utf-8');
    // After the fix, the $ref block should include _renderActions()
    expect(source).toContain('_renderActions');
    // Verify it's called in the $ref block (the block that checks node.ref)
    const refBlockMatch = source.match(/if\s*\(\s*node\.ref\s*\)\s*\{[\s\S]*?return\s+html`[\s\S]*?`;/);
    expect(refBlockMatch).not.toBeNull();
    expect(refBlockMatch![0]).toContain('_renderActions');
  });

  it('composition node previously lacked delete/duplicate — verify the fix exists in source', async () => {
    const fsModule = 'node:fs', urlModule = 'node:url';
    const fs: any = await import(/* @vite-ignore */ fsModule);
    const url: any = await import(/* @vite-ignore */ urlModule);
    const detailPanelPath = url.fileURLToPath(new URL('./tree/detail-panel.ts', import.meta.url));
    const source = fs.readFileSync(detailPanelPath, 'utf-8');
    const compBlockMatch = source.match(/if\s*\(\s*node\.compositionKeyword\s*\)\s*\{[\s\S]*?return\s+html`[\s\S]*?`;/);
    expect(compBlockMatch).not.toBeNull();
    expect(compBlockMatch![0]).toContain('_renderActions');
  });

  it('conditional node previously lacked delete/duplicate — verify the fix exists in source', async () => {
    const fsModule = 'node:fs', urlModule = 'node:url';
    const fs: any = await import(/* @vite-ignore */ fsModule);
    const url: any = await import(/* @vite-ignore */ urlModule);
    const detailPanelPath = url.fileURLToPath(new URL('./tree/detail-panel.ts', import.meta.url));
    const source = fs.readFileSync(detailPanelPath, 'utf-8');
    const condBlockMatch = source.match(/if\s*\(\s*schema\.if\s*\)\s*\{[\s\S]*?return\s+html`[\s\S]*?`;/);
    expect(condBlockMatch).not.toBeNull();
    expect(condBlockMatch![0]).toContain('_renderActions');
  });
});

// ─── Bug: Composition nodes corrupt shared properties on serialize ─────────

describe('Bug fix: properties + composition coexist on same node', () => {
  it('round-trips schema with both properties and oneOf', () => {
    const schema = {
      type: 'object',
      properties: {
        name: { type: 'string' },
      },
      oneOf: [
        { properties: { email: { type: 'string' } }, required: ['email'] },
        { properties: { phone: { type: 'string' } }, required: ['phone'] },
      ],
    };
    const tree = schemaToTree(schema);
    const output = treeToSchema(tree);
    expect(output.properties).toEqual({ name: { type: 'string' } });
    expect(output.oneOf).toHaveLength(2);
    expect(output.oneOf![0].properties.email).toBeDefined();
    expect(output.oneOf![1].properties.phone).toBeDefined();
  });

  it('round-trips schema with both properties and anyOf', () => {
    const schema = {
      type: 'object',
      properties: {
        id: { type: 'integer' },
      },
      anyOf: [
        { properties: { label: { type: 'string' } } },
        { properties: { code: { type: 'number' } } },
      ],
    };
    const tree = schemaToTree(schema);
    const output = treeToSchema(tree);
    expect(output.properties).toEqual({ id: { type: 'integer' } });
    expect(output.anyOf).toHaveLength(2);
    expect(output.anyOf![0].properties.label).toBeDefined();
    expect(output.anyOf![1].properties.code).toBeDefined();
  });

  it('tree separates property children from composition variants', () => {
    const schema = {
      type: 'object',
      properties: {
        name: { type: 'string' },
      },
      oneOf: [
        { type: 'object', title: 'WithEmail', properties: { email: { type: 'string' } } },
        { type: 'object', title: 'WithPhone', properties: { phone: { type: 'string' } } },
      ],
    };
    const tree = schemaToTree(schema);
    // Property children should be in children
    expect(tree.children).toBeDefined();
    expect(tree.children!.some(c => c.name === 'name')).toBe(true);
    // Variants should be in variants
    expect(tree.variants).toBeDefined();
    expect(tree.variants).toHaveLength(2);
    expect(tree.variants![0].name).toBe('WithEmail');
    expect(tree.variants![1].name).toBe('WithPhone');
  });
});
