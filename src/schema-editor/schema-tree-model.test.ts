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
