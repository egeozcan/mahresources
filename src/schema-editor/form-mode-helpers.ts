import type { JSONSchema } from './schema-core';

/**
 * Determines whether a JSON Schema represents a "leaf" field -- one that
 * renders as a single form control (input, select, textarea, checkbox).
 *
 * Container types (object, array) and composition keywords (oneOf, anyOf,
 * allOf, if/then/else, $ref) render nested sub-forms, so the first
 * descendant form control found inside them is NOT the control for the
 * container itself. Applying `id`, `required`, `aria-required`, and
 * `aria-describedby` to that random child input is incorrect.
 */
export function isLeafSchema(schema: JSONSchema): boolean {
  // $ref delegates to another schema — not a leaf
  if (schema.$ref) return false;

  // Composition keywords render variant selectors with nested sub-forms
  if (schema.oneOf && Array.isArray(schema.oneOf)) return false;
  if (schema.anyOf && Array.isArray(schema.anyOf)) return false;
  if (schema.allOf && Array.isArray(schema.allOf)) return false;

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
