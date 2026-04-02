import type { JSONSchema } from './schema-core';
import { resolveRef, mergeSchemas, unescapeJsonPointer } from './schema-core';

/**
 * Determines whether a JSON Schema represents a "leaf" field -- one that
 * renders as a single form control (input, select, textarea, checkbox).
 *
 * Container types (object, array) and composition keywords (oneOf, anyOf,
 * if/then/else) render nested sub-forms, so the first descendant form
 * control found inside them is NOT the control for the container itself.
 * Applying `id`, `required`, `aria-required`, and `aria-describedby` to
 * that random child input is incorrect.
 *
 * However, `$ref` and `allOf` may resolve to a simple primitive type, in
 * which case the schema IS a leaf. This function resolves those before
 * checking.
 */
export function isLeafSchema(schema: JSONSchema, rootSchema?: JSONSchema): boolean {
  // $ref: resolve and check the resolved schema
  if (schema.$ref) {
    if (!rootSchema) return false; // cannot resolve without root
    const resolved = resolveRef(schema.$ref, rootSchema);
    if (!resolved) return false;
    // Merge any sibling properties (e.g. title, description) with resolved
    const merged: JSONSchema = { ...resolved, ...schema };
    delete merged.$ref;
    return isLeafSchema(merged, rootSchema);
  }

  // oneOf / anyOf render variant selectors with nested sub-forms — never a leaf
  if (schema.oneOf && Array.isArray(schema.oneOf)) return false;
  if (schema.anyOf && Array.isArray(schema.anyOf)) return false;

  // allOf: merge all entries and check the merged result
  if (schema.allOf && Array.isArray(schema.allOf)) {
    let merged: JSONSchema = { ...schema };
    delete merged.allOf;
    for (const sub of schema.allOf) {
      let resolved: JSONSchema | null;
      if (sub.$ref) {
        const refResult = rootSchema ? resolveRef(sub.$ref, rootSchema) : null;
        const siblings: JSONSchema = { ...sub };
        delete siblings.$ref;
        resolved = refResult ? mergeSchemas(refResult, siblings) : siblings;
      } else {
        resolved = sub;
      }
      if (resolved) merged = mergeSchemas(merged, resolved);
    }
    return isLeafSchema(merged, rootSchema);
  }

  // Conditional schemas render nested sub-forms
  if (schema.if) return false;

  // Check type
  let type = schema.type;

  // Handle type arrays like ["string", "null"]
  if (Array.isArray(type)) {
    // If any non-null type is a container, it's not a leaf
    const nonNullTypes = type.filter((t: string) => t !== 'null');
    if (nonNullTypes.some((t: string) => t === 'object' || t === 'array')) return false;
    // All non-null types are primitive — it's a leaf
    return true;
  }

  // Scalar type check
  if (type === 'object' || type === 'array') return false;

  // Everything else: string, number, integer, boolean, null, enum, const
  return true;
}

// ─── Recursive stale-key stripping ──────────────────────────────────────────

/**
 * Recursively strips keys from `data` that are not declared in the schema's
 * properties when `additionalProperties` is false.  Handles `$ref` and
 * `allOf` composition so that nested schemas with strict additional-properties
 * rules also get cleaned.
 */
export function stripStaleKeys(data: any, schema: JSONSchema, rootSchema?: JSONSchema): void {
  if (!data || typeof data !== 'object' || Array.isArray(data)) return;

  // Resolve $ref if present
  let resolved = schema;
  if (schema.$ref && rootSchema) {
    const r = resolveRef(schema.$ref, rootSchema);
    if (r) {
      resolved = { ...r, ...schema, $ref: undefined };
    }
  }

  // Merge allOf if present
  if (resolved.allOf && Array.isArray(resolved.allOf)) {
    let merged: JSONSchema = { ...resolved };
    delete merged.allOf;
    for (const sub of resolved.allOf) {
      let s: JSONSchema;
      if (sub.$ref && rootSchema) {
        const refResult = resolveRef(sub.$ref, rootSchema);
        // Merge resolved $ref with sibling properties on the same allOf member
        const siblings = { ...sub };
        delete siblings.$ref;
        s = refResult ? mergeSchemas(refResult, siblings) : siblings;
      } else {
        s = sub;
      }
      merged = mergeSchemas(merged, s);
    }
    resolved = merged;
  }

  // Strip keys not in declared properties when additionalProperties is false
  if (resolved.additionalProperties === false) {
    const allowed = new Set(Object.keys(resolved.properties || {}));
    for (const key of Object.keys(data)) {
      if (!allowed.has(key)) delete data[key];
    }
  }

  // Recurse into declared properties
  if (resolved.properties) {
    for (const [key, propSchema] of Object.entries(resolved.properties)) {
      if (data[key] && typeof data[key] === 'object' && !Array.isArray(data[key])) {
        stripStaleKeys(data[key], propSchema as JSONSchema, rootSchema || schema);
      }
    }
  }
}
