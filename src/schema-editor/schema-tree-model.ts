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
  /** Children: object properties, $defs node */
  children?: SchemaNode[];
  /** Composition variant children (oneOf/anyOf/allOf variants), kept separate from property children */
  variants?: SchemaNode[];
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

/** Returns the correct definitions key based on the $schema URI,
 *  falling back to whichever key the original schema already used. */
export function getDefsPrefix(schemaUri: string | undefined, originalSchema?: JSONSchema): string {
  if (schemaUri && typeof schemaUri === 'string') {
    // draft-04, draft-06, and draft-07 all use "definitions"
    if (schemaUri.includes('draft-04') || schemaUri.includes('draft-06') || schemaUri.includes('draft-07')) {
      return 'definitions';
    }
    // 2019-09, 2020-12, and anything else use "$defs"
    return '$defs';
  }
  // No $schema declared — preserve whatever key the original schema used
  if (originalSchema) {
    if (originalSchema.definitions && !originalSchema.$defs) return 'definitions';
    if (originalSchema.$defs) return '$defs';
  }
  return '$defs'; // true default for brand-new schemas
}

// ─── Schema → Tree ───────────────────────────────────────────────────────────

export function schemaToTree(schema: JSONSchema, name = '', parentRequired: string[] = []): SchemaNode {
  // Handle nullable type arrays: ["string", "null"] → base type + preserved array
  let baseType: string = '';
  let nullableTypeArray: string[] | null = null;
  if (Array.isArray(schema.type)) {
    nullableTypeArray = schema.type;
    baseType = schema.type.find((t: string) => t !== 'null') || '';
  } else {
    baseType = schema.type || '';
  }

  const node: SchemaNode = {
    id: uid(),
    name,
    // Store the scalar base type for display / switch logic
    type: baseType,
    required: parentRequired.includes(name),
    schema: { ...schema },
  };

  // Clean up children-related keys from stored schema — they're represented in the tree
  delete node.schema.properties;
  delete node.schema.required;
  delete node.schema.$defs;
  delete node.schema.definitions;
  // Strip type — it's stored on node.type and restored in treeToSchema.
  // For nullable arrays, preserve the union in node.schema.type so treeToSchema
  // can emit the correct ["type", "null"] array.
  if (nullableTypeArray) {
    node.schema.type = nullableTypeArray;
  } else {
    delete node.schema.type;
  }

  // $ref → reference node
  if (schema.$ref && typeof schema.$ref === 'string') {
    node.ref = schema.$ref;
    delete node.schema.$ref;
  }

  // oneOf / anyOf / allOf → composition node with variant children (stored in `variants`)
  // Only extract the FIRST matching keyword into compositionKeyword/variants.
  // Additional composition keywords stay in node.schema and pass through
  // treeToSchema via the spread, preserving multi-keyword schemas.
  for (const kw of ['oneOf', 'anyOf', 'allOf'] as const) {
    if (Array.isArray(schema[kw])) {
      node.compositionKeyword = kw;
      const variantNodes = (schema[kw] as JSONSchema[]).map((variant, i) =>
        schemaToTree(variant, variant.title || `variant${i + 1}`),
      );
      node.variants = [...(node.variants || []), ...variantNodes];
      delete node.schema[kw];
      break;
    }
  }

  // `not` keyword → composition node with one variant child.
  // Only extract `not` into compositionKeyword/variants if no other composition
  // keyword was already extracted (first-wins logic). Otherwise leave it in
  // node.schema so it round-trips via the spread in treeToSchema.
  if (schema.not && typeof schema.not === 'object' && !node.compositionKeyword) {
    node.compositionKeyword = 'not';
    const child = schemaToTree(schema.not as JSONSchema, 'not');
    node.variants = [...(node.variants || []), child];
    delete node.schema.not;
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
    // Remember which key the original schema used so treeToSchema can
    // emit the same key even when $schema is absent.
    node.schema._originalDefsKey = schema.$defs ? '$defs' : 'definitions';
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

  // Restore type: prefer the union array in node.schema.type (e.g. ["string","null"])
  // which is kept in sync by the nullable toggle, then fall back to the scalar node.type.
  if (Array.isArray(node.schema.type)) {
    schema.type = node.schema.type;
  } else if (node.type) {
    schema.type = node.type;
  } else {
    delete schema.type;
  }

  // $ref nodes
  if (node.ref) {
    schema.$ref = node.ref;
  }

  // Composition keywords: oneOf / anyOf / allOf → serialize variant children from `variants`
  if (node.compositionKeyword === 'oneOf' || node.compositionKeyword === 'anyOf' || node.compositionKeyword === 'allOf') {
    schema[node.compositionKeyword] = (node.variants || []).map(c => treeToSchema(c));
  }

  // Serialize `not` composition keyword from `variants`.
  // `not` takes a single schema (not an array), so use the first variant.
  // If variants is empty (the constraint was removed via unwrap), omit `not`.
  if (node.compositionKeyword === 'not') {
    const notChild = (node.variants || [])[0];
    if (notChild) {
      schema.not = treeToSchema(notChild);
    }
  }

  // Property children are always in node.children (separate from variants)
  const propChildren = (node.children || []).filter(c =>
    !c.isDef && c.name !== '$defs'
  );
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
    // Use the stored original key when $schema is absent
    const originalSchema = node.schema._originalDefsKey
      ? { [node.schema._originalDefsKey]: true } as JSONSchema
      : undefined;
    const defsKey = getDefsPrefix(node.schema.$schema as string | undefined, originalSchema);
    schema[defsKey] = {};
    for (const defChild of defsNode.children) {
      schema[defsKey][defChild.name] = treeToSchema(defChild);
    }
  }
  // Clean up internal tracking property from output
  delete schema._originalDefsKey;

  return schema;
}
