import type { JSONSchema } from './schema-core';

// ─── Schema Node ─────────────────────────────────────────────────────────────

export interface SchemaNode {
  /** Property name (empty string for root) */
  name: string;
  /** JSON Schema type: string, number, integer, boolean, object, array, null */
  type: string;
  /** Is this property in the parent's required array? */
  required: boolean;
  /** The raw schema keywords for this node (title, description, constraints, etc.) */
  schema: JSONSchema;
  /** Children: object properties, array items schema, composition variants */
  children?: SchemaNode[];
  /** For composition nodes (oneOf/anyOf/allOf): which keyword */
  compositionKeyword?: 'oneOf' | 'anyOf' | 'allOf' | 'not';
  /** For $ref nodes: the ref string */
  ref?: string;
  /** For $defs: each def is a child of a virtual $defs node */
  isDef?: boolean;
  /** Unique ID for tree rendering (assigned at parse time) */
  id: string;
}

let nextId = 0;
function uid(): string {
  return `node-${++nextId}`;
}

/** Reset ID counter (for tests) */
export function resetIdCounter(): void {
  nextId = 0;
}

// ─── Draft detection ─────────────────────────────────────────────────────────

export function detectDraft(schema: JSONSchema): string | null {
  const s = schema.$schema;
  if (!s || typeof s !== 'string') return null;
  if (s.includes('2020-12')) return '2020-12';
  if (s.includes('2019-09')) return '2019-09';
  if (s.includes('draft-07')) return 'draft-07';
  if (s.includes('draft-06')) return 'draft-06';
  if (s.includes('draft-04')) return 'draft-04';
  return null;
}

// ─── Schema → Tree ───────────────────────────────────────────────────────────

export function schemaToTree(schema: JSONSchema, name = '', parentRequired: string[] = []): SchemaNode {
  const node: SchemaNode = {
    id: uid(),
    name,
    // Preserve absence of type — use empty string to signal "no explicit type"
    type: schema.type || '',
    required: parentRequired.includes(name),
    schema: { ...schema },
  };

  // Clean up children-related keys from stored schema — they're represented in the tree
  delete node.schema.properties;
  delete node.schema.required;
  delete node.schema.$defs;
  delete node.schema.definitions;
  delete node.schema.not;
  // Also strip type — it's stored on node.type and restored in treeToSchema
  delete node.schema.type;

  // `not` keyword → composition node with one child
  if (schema.not && typeof schema.not === 'object') {
    node.compositionKeyword = 'not';
    const child = schemaToTree(schema.not as JSONSchema, 'not');
    node.children = [...(node.children || []), child];
  }

  // Object with properties → children
  if (schema.properties) {
    const reqSet = schema.required || [];
    const propChildren = Object.entries(schema.properties).map(([key, propSchema]) =>
      schemaToTree(propSchema as JSONSchema, key, reqSet),
    );
    node.children = [...(node.children || []), ...propChildren];
  }

  // $defs / definitions
  const defs = schema.$defs || schema.definitions;
  if (defs && typeof defs === 'object') {
    const defsNode: SchemaNode = {
      id: uid(),
      name: '$defs',
      type: 'object',
      required: false,
      schema: {},
      isDef: true,
      children: Object.entries(defs).map(([key, defSchema]) => {
        const child = schemaToTree(defSchema as JSONSchema, key);
        child.isDef = true;
        return child;
      }),
    };
    node.children = [...(node.children || []), defsNode];
  }

  return node;
}

// ─── Tree → Schema ───────────────────────────────────────────────────────────

export function treeToSchema(node: SchemaNode): JSONSchema {
  const schema: JSONSchema = { ...node.schema };

  // Restore type only when explicitly set (non-empty)
  if (node.type) {
    schema.type = node.type;
  } else {
    delete schema.type;
  }

  // Serialize `not` composition keyword
  if (node.compositionKeyword === 'not') {
    const notChild = (node.children || []).find(c => c.name === 'not');
    if (notChild) {
      schema.not = treeToSchema(notChild);
    }
  }

  // Separate property children from $defs node and `not` child
  const propChildren = (node.children || []).filter(c => !c.isDef && c.name !== '$defs' && !(node.compositionKeyword === 'not' && c.name === 'not'));
  const defsNode = (node.children || []).find(c => c.isDef && c.name === '$defs');

  // Restore properties (for object type or when type is unset/empty)
  if (propChildren.length > 0 && (node.type === 'object' || node.type === '')) {
    schema.properties = {};
    const required: string[] = [];

    for (const child of propChildren) {
      schema.properties[child.name] = treeToSchema(child);
      if (child.required) {
        required.push(child.name);
      }
    }

    if (required.length > 0) {
      schema.required = required;
    }
  }

  // Restore $defs
  if (defsNode && defsNode.children && defsNode.children.length > 0) {
    const defsKey = node.schema.$schema?.includes('draft-04') ? 'definitions' : '$defs';
    schema[defsKey] = {};
    for (const defChild of defsNode.children) {
      schema[defsKey][defChild.name] = treeToSchema(defChild);
    }
  }

  return schema;
}
