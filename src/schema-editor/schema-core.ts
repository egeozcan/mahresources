// ─── Type definitions ────────────────────────────────────────────────────────

export interface FlatField {
  path: string;
  label: string;
  type: string;
  enum: string[] | null;
  /** Optional labels for enum values (parallel array with `enum`). null if no labels. */
  enumLabels: string[] | null;
}

export type JSONSchema = Record<string, any>;

// ─── JSON Pointer escaping (RFC 6901) ────────────────────────────────────────

export function escapeJsonPointer(token: string): string {
  return token.replace(/~/g, '~0').replace(/\//g, '~1');
}

export function unescapeJsonPointer(token: string): string {
  return token.replace(/~1/g, '/').replace(/~0/g, '~');
}

// ─── Ref resolution ──────────────────────────────────────────────────────────

export function resolveRef(ref: unknown, root: JSONSchema): JSONSchema | null {
  if (typeof ref !== 'string' || !ref.startsWith('#/')) return null;
  const parts = ref.split('/').slice(1);
  let current: any = root;
  for (const part of parts) {
    const unescaped = unescapeJsonPointer(part);
    if (current && typeof current === 'object' && unescaped in current) {
      current = current[unescaped];
    } else {
      return null;
    }
  }
  return current;
}

// ─── Schema merging ──────────────────────────────────────────────────────────

export function mergeSchemas(base: JSONSchema, extension: JSONSchema, mode: 'intersect' | 'union' = 'intersect'): JSONSchema {
  const merged: JSONSchema = { ...base };
  for (const key in extension) {
    if (key === 'properties') {
      const baseProps = base.properties || {};
      const extProps = extension.properties || {};
      merged.properties = { ...baseProps };
      for (const propKey in extProps) {
        if (baseProps[propKey] && extProps[propKey]) {
          const baseProp = baseProps[propKey];
          const extProp = extProps[propKey];

          // Deep merge when both sides have nested properties or required
          if ((baseProp.properties && extProp.properties) ||
              (baseProp.required && extProp.required)) {
            merged.properties[propKey] = mergeSchemas(baseProp, extProp, mode);
          } else {
            // Shallow merge with type/enum conflict resolution
            const baseType = baseProp.type;
            const extType = extProp.type;
            const numericTypes = new Set(['number', 'integer']);
            const mergedProp = { ...baseProp, ...extProp };
            // Resolve type conflicts
            if (baseType && extType && baseType !== extType) {
              mergedProp.type = (numericTypes.has(baseType) && numericTypes.has(extType))
                ? 'number'
                : 'string';
            }
            // Merge enum values: intersect for allOf (default), union for oneOf/anyOf
            const baseEnum = baseProp.enum;
            const extEnum = extProp.enum;
            if (baseEnum && extEnum) {
              if (mode === 'union') {
                const combined = [...baseEnum];
                for (const v of extEnum) {
                  if (!combined.some((existing: any) => existing === v && typeof existing === typeof v)) {
                    combined.push(v);
                  }
                }
                mergedProp.enum = combined;
              } else {
                // Intersect: keep only values present in both (allOf semantics)
                const intersected = baseEnum.filter((v: any) =>
                  extEnum.some((ev: any) => ev === v && typeof ev === typeof v)
                );
                // Preserve the intersection even if empty — an empty enum
                // means the constraint is unsatisfiable, which is the correct
                // allOf semantic for disjoint enums.
                mergedProp.enum = intersected;
              }
            }
            // When only one side has enum, the spread already put whichever
            // exists on mergedProp. This is correct for allOf (the constraint
            // applies). For oneOf/anyOf, resolveSchema handles enum dropping
            // after merging all branches.
            merged.properties[propKey] = mergedProp;
          }
        } else {
          merged.properties[propKey] = extProps[propKey];
        }
      }
    } else if (key === 'required') {
      merged.required = [...new Set([...(base.required || []), ...(extension.required || [])])];
    } else if (key === '$ref') {
      // $ref is never copied — it must be resolved before merging
    } else if (['allOf', 'anyOf', 'oneOf'].includes(key)) {
      // Composition keywords: if base already has the same keyword, concat.
      // Otherwise copy from extension. This preserves nested constraints
      // like spec.allOf:[{required:['height']}] during deep property merge.
      if (merged[key] && Array.isArray(merged[key])) {
        merged[key] = [...merged[key], ...extension[key]];
      } else {
        merged[key] = extension[key];
      }
    } else {
      merged[key] = extension[key];
    }
  }
  return merged;
}

// ─── Schema resolution (composition keywords) ───────────────────────────────

export function resolveSchema(schema: JSONSchema | null, rootSchema: JSONSchema): JSONSchema | null {
  if (!schema) return schema;

  if (schema.$ref) {
    const resolved = resolveRef(schema.$ref, rootSchema);
    if (resolved) {
      // Use mergeSchemas to properly combine ref target with sibling keywords,
      // especially properties which need to be merged, not overwritten.
      const siblings: JSONSchema = { ...schema };
      delete siblings.$ref;
      const merged = mergeSchemas(resolved, siblings);
      return resolveSchema(merged, rootSchema);
    }
    return null;
  }

  for (const keyword of ['allOf', 'oneOf', 'anyOf'] as const) {
    if (schema[keyword] && Array.isArray(schema[keyword])) {
      let merged: JSONSchema = { ...schema };
      delete merged[keyword];
      for (const sub of schema[keyword]) {
        let resolved: JSONSchema | null;
        if (sub.$ref) {
          const refResult = resolveRef(sub.$ref, rootSchema);
          const siblings: JSONSchema = { ...sub };
          delete siblings.$ref;
          resolved = refResult ? mergeSchemas(refResult, siblings) : siblings;
        } else {
          resolved = sub;
        }
        if (resolved) {
          // For oneOf/anyOf, branches are alternatives — union their enums.
          // For allOf, branches are constraints — intersect (the default).
          const branchMode = keyword === 'allOf' ? 'intersect' : 'union' as const;
          merged = mergeSchemas(merged, resolved, branchMode);
        }
      }

      // For oneOf/anyOf (alternatives), drop enum on properties where any
      // branch DECLARES the property WITHOUT an enum — that branch allows
      // arbitrary values for that field. Branches that omit the property
      // entirely (or have additionalProperties: false) are not saying
      // "any value is fine" — they're saying "this field doesn't exist
      // in this variant," which doesn't weaken the enum constraint from
      // branches that do declare it.
      if (keyword !== 'allOf' && merged.properties) {
        for (const [propKey, propSchema] of Object.entries(merged.properties)) {
          if (!(propSchema as JSONSchema).enum) continue;
          const anyBranchDeclaresWithoutEnum = schema[keyword].some((sub: JSONSchema) => {
            let resolved = sub;
            if (sub.$ref) {
              const r = resolveRef(sub.$ref, rootSchema);
              if (r) {
                // Properly merge ref target with sibling keywords
                const siblings: JSONSchema = { ...sub };
                delete siblings.$ref;
                resolved = mergeSchemas(r, siblings);
              }
            }
            // Branch declares the property but without an enum → unrestricted
            const prop = resolved.properties?.[propKey];
            return prop && !prop.enum;
          });
          if (anyBranchDeclaresWithoutEnum) {
            delete (propSchema as JSONSchema).enum;
          }
        }
      }

      return resolveSchema(merged, rootSchema);
    }
  }

  return schema;
}

// ─── Type inference ──────────────────────────────────────────────────────────

export function inferType(val: unknown): string {
  if (Array.isArray(val)) return 'array';
  if (val === null) return 'null';
  const t = typeof val;
  if (t === 'number') {
    return Number.isInteger(val) ? 'integer' : 'number';
  }
  return t;
}

export function inferSchema(val: unknown): JSONSchema {
  const type = inferType(val);
  if (type === 'object') return { type: 'object', properties: {} };
  if (type === 'array') {
    const arr = val as unknown[];
    return { type: 'array', items: arr.length ? inferSchema(arr[0]) : { type: 'string' } };
  }
  return { type };
}

// ─── Condition evaluation ────────────────────────────────────────────────────

export function evaluateCondition(conditionSchema: JSONSchema | null | undefined, data: any, rootSchema?: JSONSchema): boolean {
  if (!conditionSchema) return true;

  // ── Resolve $ref before any checks ────────────────────────────────────
  if (conditionSchema.$ref && rootSchema) {
    const resolved = resolveRef(conditionSchema.$ref, rootSchema);
    if (resolved) {
      const siblings: JSONSchema = { ...conditionSchema };
      delete siblings.$ref;
      return evaluateCondition(mergeSchemas(resolved, siblings), data, rootSchema);
    }
  }

  // ── Composition keywords (allOf, anyOf, oneOf) ────────────────────────
  // Resolve these recursively, then continue to check any remaining direct
  // keywords (properties, required, etc.) on the same condition schema.

  if (conditionSchema.allOf && Array.isArray(conditionSchema.allOf)) {
    for (const sub of conditionSchema.allOf) {
      if (!evaluateCondition(sub as JSONSchema, data, rootSchema)) return false;
    }
  }

  if (conditionSchema.anyOf && Array.isArray(conditionSchema.anyOf)) {
    if (!conditionSchema.anyOf.some((sub: JSONSchema) => evaluateCondition(sub, data, rootSchema))) return false;
  }

  if (conditionSchema.oneOf && Array.isArray(conditionSchema.oneOf)) {
    if (conditionSchema.oneOf.filter((sub: JSONSchema) => evaluateCondition(sub, data, rootSchema)).length !== 1) return false;
  }

  if (conditionSchema.not) {
    // not: the sub-schema must NOT match — if it evaluates true, the condition fails
    if (evaluateCondition(conditionSchema.not as JSONSchema, data, rootSchema)) return false;
  }

  // ── Top-level keyword checks (constrain the data value itself) ──────────

  // Top-level const — data itself must equal the value
  if (conditionSchema.const !== undefined && data !== conditionSchema.const) return false;

  // Top-level enum — data itself must be in the list
  if (conditionSchema.enum && !conditionSchema.enum.includes(data)) return false;

  // Top-level type check — always validate, including when properties exist.
  // A condition like { type: "object", properties: { x: { const: "a" } } }
  // should fail if data is a string, not silently pass into the properties loop.
  if (conditionSchema.type) {
    const actualType = inferType(data);
    const expected = conditionSchema.type;
    if (typeof expected === 'string') {
      if (expected !== actualType) {
        if (!(expected === 'number' && actualType === 'integer')) return false;
      }
    } else if (Array.isArray(expected)) {
      if (!expected.includes(actualType) &&
          !(actualType === 'integer' && expected.includes('number'))) return false;
    }
  }

  // Top-level numeric constraints
  if (conditionSchema.minimum !== undefined && (typeof data !== 'number' || data < conditionSchema.minimum)) return false;
  if (conditionSchema.maximum !== undefined && (typeof data !== 'number' || data > conditionSchema.maximum)) return false;
  if (conditionSchema.exclusiveMinimum !== undefined && (typeof data !== 'number' || data <= conditionSchema.exclusiveMinimum)) return false;
  if (conditionSchema.exclusiveMaximum !== undefined && (typeof data !== 'number' || data >= conditionSchema.exclusiveMaximum)) return false;

  // Top-level string constraints
  if (conditionSchema.minLength !== undefined && (typeof data !== 'string' || data.length < conditionSchema.minLength)) return false;
  if (conditionSchema.maxLength !== undefined && (typeof data !== 'string' || data.length > conditionSchema.maxLength)) return false;
  if (conditionSchema.pattern) {
    if (typeof data === 'string') {
      try { if (!new RegExp(conditionSchema.pattern).test(data)) return false; } catch { /* invalid regex — skip */ }
    }
  }

  // Top-level array constraints
  if (conditionSchema.minItems !== undefined && (!Array.isArray(data) || data.length < conditionSchema.minItems)) return false;
  if (conditionSchema.maxItems !== undefined && (!Array.isArray(data) || data.length > conditionSchema.maxItems)) return false;

  // Check required — all listed keys must be present and non-undefined
  if (conditionSchema.required && Array.isArray(conditionSchema.required)) {
    for (const key of conditionSchema.required) {
      if (data?.[key] === undefined) return false;
    }
  }

  // Check properties constraints
  // Per JSON Schema spec, property constraints in `if` only apply to properties
  // that are PRESENT in the data. Absent properties match vacuously — use
  // `required` to demand their presence.
  if (conditionSchema.properties) {
    for (const key in conditionSchema.properties) {
      let propSchema = conditionSchema.properties[key];
      const value = data?.[key];

      // Resolve property-level $ref
      if (propSchema.$ref && rootSchema) {
        const resolved = resolveRef(propSchema.$ref, rootSchema);
        if (resolved) {
          const siblings: JSONSchema = { ...propSchema };
          delete siblings.$ref;
          propSchema = mergeSchemas(resolved, siblings);
        }
      }

      // Skip absent properties — they match vacuously per JSON Schema spec.
      if (value === undefined) continue;

      // Recurse into nested object properties or composition schemas
      if (propSchema.properties || propSchema.allOf || propSchema.anyOf || propSchema.oneOf || propSchema.not) {
        if (!evaluateCondition(propSchema, value, rootSchema)) return false;
        continue;
      }

      // const match
      if (propSchema.const !== undefined && value !== propSchema.const) return false;

      // enum match
      if (propSchema.enum && !propSchema.enum.includes(value)) return false;

      // type match
      if (propSchema.type) {
        const actualType = inferType(value);
        const expectedType = propSchema.type;
        if (typeof expectedType === 'string') {
          if (expectedType !== actualType) {
            if (!(expectedType === 'number' && actualType === 'integer')) return false;
          }
        } else if (Array.isArray(expectedType)) {
          if (!expectedType.includes(actualType) &&
              !(actualType === 'integer' && expectedType.includes('number'))) return false;
        }
      }

      // minimum/maximum for numbers
      if (propSchema.minimum !== undefined && (typeof value !== 'number' || value < propSchema.minimum)) return false;
      if (propSchema.maximum !== undefined && (typeof value !== 'number' || value > propSchema.maximum)) return false;
      if (propSchema.exclusiveMinimum !== undefined && (typeof value !== 'number' || value <= propSchema.exclusiveMinimum)) return false;
      if (propSchema.exclusiveMaximum !== undefined && (typeof value !== 'number' || value >= propSchema.exclusiveMaximum)) return false;

      // minLength/maxLength for strings
      if (propSchema.minLength !== undefined && (typeof value !== 'string' || value.length < propSchema.minLength)) return false;
      if (propSchema.maxLength !== undefined && (typeof value !== 'string' || value.length > propSchema.maxLength)) return false;

      // pattern for strings
      if (propSchema.pattern && typeof value === 'string') {
        try { if (!new RegExp(propSchema.pattern).test(value)) return false; } catch { /* invalid regex — skip */ }
      }

      // minItems/maxItems for arrays
      if (propSchema.minItems !== undefined && (!Array.isArray(value) || value.length < propSchema.minItems)) return false;
      if (propSchema.maxItems !== undefined && (!Array.isArray(value) || value.length > propSchema.maxItems)) return false;
    }
  }

  return true;
}

// ─── Schema match scoring ────────────────────────────────────────────────────

export function scoreSchemaMatch(schema: JSONSchema, data: unknown, rootSchema: JSONSchema): number {
  if (schema.$ref) {
    const resolved = resolveRef(schema.$ref, rootSchema);
    if (resolved) {
      const siblings: JSONSchema = { ...schema };
      delete siblings.$ref;
      schema = mergeSchemas(resolved, siblings);
    }
  }

  // Resolve composition so we score against the merged schema.
  // allOf: merge all branches (all constraints apply simultaneously).
  // oneOf/anyOf: score each branch independently, return best score.
  if (schema.allOf && Array.isArray(schema.allOf)) {
    let merged: JSONSchema = { ...schema };
    delete merged.allOf;
    for (const sub of schema.allOf) {
      const resolved = sub.$ref ? resolveRef(sub.$ref, rootSchema) : sub;
      if (resolved) {
        const siblings = sub.$ref ? (() => { const s = {...sub}; delete s.$ref; return s; })() : {};
        merged = mergeSchemas(merged, sub.$ref ? mergeSchemas(resolved, siblings) : resolved);
      }
    }
    schema = merged;
  } else {
    for (const kw of ['oneOf', 'anyOf'] as const) {
      if (schema[kw] && Array.isArray(schema[kw])) {
        // Collect sibling properties (declared alongside oneOf/anyOf)
        const siblings: JSONSchema = { ...schema };
        delete siblings[kw];

        let bestScore = -1;
        for (const sub of schema[kw]) {
          const resolved = sub.$ref ? (() => {
            const r = resolveRef(sub.$ref, rootSchema);
            const s: JSONSchema = { ...sub }; delete s.$ref;
            return r ? mergeSchemas(r, s) : s;
          })() : sub;
          // Merge branch with sibling properties for scoring
          const branchSchema = mergeSchemas(siblings, resolved);
          const branchScore = scoreSchemaMatch(branchSchema, data, rootSchema);
          if (branchScore > bestScore) bestScore = branchScore;
        }
        return bestScore >= 0 ? bestScore : 0;
      }
    }
  }

  if (schema.const !== undefined) return schema.const === data ? 100 : 0;

  const dataType = inferType(data);
  let schemaType = schema.type;

  if (Array.isArray(schemaType)) {
    if (schemaType.includes(dataType)) return 10;
    if (dataType === 'integer' && schemaType.includes('number')) return 9;
    if (dataType === 'null' && (schemaType.includes('string') || schemaType.includes('number'))) return 5;
    return 0;
  }

  if (schemaType && schemaType !== dataType) {
    if (schemaType === 'number' && dataType === 'integer') return 9;
    return 0;
  }

  if (dataType === 'object' && schema.properties) {
    const dataKeys = Object.keys(data as object);
    const schemaKeys = Object.keys(schema.properties);
    const matchCount = dataKeys.filter(k => schemaKeys.includes(k)).length;
    let score = matchCount + 10;

    // Boost/penalize based on const/enum discriminator matches,
    // recursing into nested objects for deep discriminators.
    for (const [key, propSchema] of Object.entries(schema.properties)) {
      const ps = propSchema as Record<string, any>;
      const val = (data as Record<string, any>)[key];
      if (val === undefined) continue;

      // const match: strong signal for discriminated unions
      if (ps.const !== undefined) {
        if (val === ps.const) score += 50;
        else return 0; // Definite mismatch — this variant cannot match
      }

      // enum match
      if (ps.enum && Array.isArray(ps.enum)) {
        if (ps.enum.includes(val)) score += 20;
        else return 0; // Value not in allowed enum — cannot match
      }

      // Resolve the property schema before checking for nested discriminators
      let resolvedProp = ps;
      if (ps.$ref) {
        const r = resolveRef(ps.$ref, rootSchema);
        if (r) {
          const siblings: Record<string, any> = { ...ps };
          delete siblings.$ref;
          resolvedProp = mergeSchemas(r, siblings);
        }
      }

      // Handle composition on the property schema
      if (resolvedProp.allOf && Array.isArray(resolvedProp.allOf)) {
        // allOf: merge all (all constraints apply simultaneously)
        let merged: JSONSchema = { ...resolvedProp };
        delete merged.allOf;
        for (const sub of resolvedProp.allOf) {
          const r = sub.$ref ? resolveRef(sub.$ref, rootSchema) : sub;
          if (r) {
            const siblings = sub.$ref ? (() => { const s = {...sub}; delete s.$ref; return s; })() : {};
            merged = mergeSchemas(merged, sub.$ref ? mergeSchemas(r, siblings) : r);
          }
        }
        resolvedProp = merged;
      } else if (typeof val === 'object' && val !== null && !Array.isArray(val)) {
        // oneOf/anyOf: score each branch independently, take best
        // Also check sibling constraints (properties alongside the composition keyword)
        for (const kw of ['oneOf', 'anyOf'] as const) {
          if (resolvedProp[kw] && Array.isArray(resolvedProp[kw])) {
            // Score sibling constraints (properties declared alongside oneOf/anyOf)
            const siblingSchema: JSONSchema = { ...resolvedProp };
            delete siblingSchema[kw];

            // Check if sibling has any meaningful constraints (not just metadata)
            const metadataOnly = ['title', 'description', 'examples', 'deprecated', 'readOnly', 'writeOnly', 'default', '$comment'];
            const hasSiblingConstraints = Object.keys(siblingSchema).some(k => !metadataOnly.includes(k));
            if (hasSiblingConstraints) {
              const siblingScore = scoreSchemaMatch(siblingSchema, val, rootSchema);
              if (siblingScore === 0) return 0; // Sibling constraint mismatch
              score += siblingScore - 10; // Subtract base to avoid double-counting
            }

            // Score composition branches independently
            let bestBranchScore = -1;
            for (const sub of resolvedProp[kw]) {
              const branchScore = scoreSchemaMatch(sub, val, rootSchema);
              if (branchScore > bestBranchScore) bestBranchScore = branchScore;
            }
            if (bestBranchScore === 0) return 0;
            if (bestBranchScore > 0) score += bestBranchScore - 10;
            resolvedProp = {}; // Both siblings and branches are scored
            break;
          }
        }
      }

      // Recurse into nested objects for deep discriminators
      if (resolvedProp.properties && typeof val === 'object' && val !== null && !Array.isArray(val)) {
        const nestedScore = scoreSchemaMatch(resolvedProp, val, rootSchema);
        if (nestedScore === 0) return 0; // Nested discriminator mismatch
        score += nestedScore - 10; // Subtract base score to avoid double-counting
      }
    }

    // Penalize missing required fields
    if (schema.required && Array.isArray(schema.required)) {
      for (const reqKey of schema.required) {
        if ((data as Record<string, any>)[reqKey] === undefined) {
          score -= 5;
        }
      }
    }

    return score;
  }

  // Handle object schemas with required but no properties (e.g., sibling schemas)
  if (dataType === 'object' && schema.required && Array.isArray(schema.required)) {
    let score = 10;
    for (const reqKey of schema.required) {
      if ((data as Record<string, any>)[reqKey] === undefined) {
        score -= 5;
      }
    }
    return score;
  }

  return 10;
}

// ─── Default values ──────────────────────────────────────────────────────────

export function getDefaultValue(schema: JSONSchema, rootSchema?: JSONSchema): any {
  if (schema.$ref) {
    const resolved = resolveRef(schema.$ref, rootSchema || schema);
    if (resolved) {
      const siblings: JSONSchema = { ...schema };
      delete siblings.$ref;
      return getDefaultValue(mergeSchemas(resolved, siblings), rootSchema);
    }
  }

  if (schema.allOf && Array.isArray(schema.allOf)) {
    let merged: JSONSchema = { ...schema };
    delete merged.allOf;
    for (const sub of schema.allOf) {
      let resolved: JSONSchema;
      if (sub.$ref) {
        const refResult = resolveRef(sub.$ref, rootSchema || schema);
        const siblings: JSONSchema = { ...sub };
        delete siblings.$ref;
        resolved = refResult ? mergeSchemas(refResult, siblings) : siblings;
      } else {
        resolved = sub;
      }
      if (resolved) merged = mergeSchemas(merged, resolved);
    }
    return getDefaultValue(merged, rootSchema);
  }

  if (schema.if) {
    const baseSchema: JSONSchema = { ...schema };
    delete baseSchema.if;
    delete baseSchema.then;
    delete baseSchema.else;
    // Evaluate the condition against the base defaults to pick the right branch
    const baseDefault = getDefaultValue(baseSchema, rootSchema);
    const conditionMet = evaluateCondition(schema.if, baseDefault, rootSchema);
    const branch = conditionMet ? (schema.then || {}) : (schema.else || {});
    const merged = mergeSchemas(baseSchema, branch);
    return getDefaultValue(merged, rootSchema);
  }

  if (schema.default !== undefined) return schema.default;
  if (schema.const !== undefined) return schema.const;
  if (schema.enum && Array.isArray(schema.enum) && schema.enum.length > 0) {
    return schema.enum[0];
  }
  if (schema.type === 'object') {
    if (!schema.properties) return {};
    const obj: any = {};
    for (const [key, propSchema] of Object.entries(schema.properties)) {
      obj[key] = getDefaultValue(propSchema as JSONSchema, rootSchema);
    }
    return obj;
  }
  if (schema.type === 'array') return [];
  if (schema.type === 'boolean') return false;
  if (schema.type === 'number' || schema.type === 'integer') return 0;
  if (schema.type === 'null') return null;

  if (Array.isArray(schema.type)) {
    if (schema.type.includes('string')) return '';
    if (schema.type.includes('number') || schema.type.includes('integer')) return 0;
    if (schema.type.includes('boolean')) return false;
    if (schema.type.includes('object')) return {};
    if (schema.type.includes('array')) return [];
    if (schema.type.includes('null')) return null;
  }

  if (schema.oneOf && schema.oneOf.length > 0) {
    if (isLabeledEnum(schema)) return schema.oneOf[0].const;
    return getDefaultValue(schema.oneOf[0], rootSchema);
  }
  if (schema.anyOf && schema.anyOf.length > 0) return getDefaultValue(schema.anyOf[0], rootSchema);

  // Schemas with properties but no explicit type are implicitly objects
  if (schema.properties) {
    const obj: any = {};
    for (const [key, propSchema] of Object.entries(schema.properties)) {
      obj[key] = getDefaultValue(propSchema as JSONSchema, rootSchema);
    }
    return obj;
  }

  return '';
}

// ─── Title case ──────────────────────────────────────────────────────────────

export function titleCase(key: string): string {
  return key
    .replace(/([a-z])([A-Z])/g, '$1 $2')
    .replace(/[_-]/g, ' ')
    .replace(/\b\w/g, c => c.toUpperCase());
}

// ─── Labeled enum detection ─────────────────────────────────────────────────

/**
 * Returns true when a schema represents a "labeled enum" — a `oneOf` array
 * where every entry is a simple `{ const, title?, description? }` object
 * with no complex subschema keywords.
 */
export function isLabeledEnum(schema: JSONSchema): boolean {
  if (!schema.oneOf || !Array.isArray(schema.oneOf) || schema.oneOf.length === 0) return false;
  const complexKeys = new Set(['type', 'properties', 'items', 'oneOf', 'anyOf', 'allOf', 'if', '$ref', 'enum']);
  return schema.oneOf.every((entry: JSONSchema) => {
    if (!entry || typeof entry !== 'object' || entry.const === undefined) return false;
    return !Object.keys(entry).some(k => complexKeys.has(k));
  });
}

/**
 * Given a labeled-enum schema, returns the label for a specific value.
 * Falls back to stringifying the value if no title is found.
 */
export function getLabeledEnumTitle(schema: JSONSchema, value: any): string {
  if (!schema.oneOf) return String(value);
  const entry = schema.oneOf.find((e: JSONSchema) => e.const === value);
  return entry?.title || String(value);
}

/**
 * Extracts the enum values and labels from a labeled-enum schema.
 * Returns an array of { value, label } objects.
 */
export function getLabeledEnumEntries(schema: JSONSchema): Array<{ value: any; label: string }> {
  if (!schema.oneOf) return [];
  return schema.oneOf.map((entry: JSONSchema) => ({
    value: entry.const,
    label: entry.title || String(entry.const),
  }));
}

// ─── Schema flattening (for search mode) ─────────────────────────────────────

export function flattenSchema(
  schema: JSONSchema,
  prefix = '',
  labelPrefix = '',
  depth = 0,
  rootSchema: JSONSchema | null = null,
): FlatField[] {
  if (depth > 10 || !schema) return [];

  const root = rootSchema || schema;
  const resolved = resolveSchema(schema, root);
  if (!resolved || !resolved.properties) return [];

  const fields: FlatField[] = [];

  for (const [key, rawProp] of Object.entries(resolved.properties) as [string, JSONSchema][]) {
    const path = prefix ? `${prefix}.${key}` : key;
    const prop = resolveSchema(rawProp, root) || rawProp;
    const rawLabel = prop.title || titleCase(key);
    const label = labelPrefix ? `${labelPrefix} › ${rawLabel}` : rawLabel;

    // Labeled enum: oneOf with const+title entries
    if (isLabeledEnum(prop)) {
      const entries = getLabeledEnumEntries(prop);
      let fieldType = prop.type || 'string';
      if (Array.isArray(fieldType)) {
        fieldType = fieldType.find((t: string) => t !== 'null') || 'string';
      }
      // Infer type from const values if no explicit type
      if (!prop.type && entries.length > 0) {
        const representative = entries.find(e => e.value !== null)?.value ?? entries[0].value;
        if (representative === null) fieldType = 'null';
        else if (typeof representative === 'number') fieldType = Number.isInteger(representative) ? 'integer' : 'number';
        else if (typeof representative === 'boolean') fieldType = 'boolean';
      }
      fields.push({
        path,
        label,
        type: fieldType,
        enum: entries.map(e => e.value),
        enumLabels: entries.map(e => e.label),
      });
      continue;
    }

    if (prop.properties) {
      fields.push(...flattenSchema(prop, path, label, depth + 1, root));
    } else if (prop.type === 'array') {
      continue;
    } else {
      // Normalize nullable type arrays (e.g. ["string", "null"]) to their
      // base scalar type so search-mode operators and serialization work.
      let fieldType = prop.type || 'string';
      if (Array.isArray(fieldType)) {
        fieldType = fieldType.find((t: string) => t !== 'null') || 'string';
      }
      // If type is defaulted (no explicit type) and enum exists, infer from enum values
      if (!prop.type && Array.isArray(prop.enum) && prop.enum.length > 0) {
        // Find the first non-null value to infer type, since null alone isn't searchable
        const representative = prop.enum.find((v: any) => v !== null) ?? prop.enum[0];
        if (representative === null) {
          fieldType = 'null';
        } else if (typeof representative === 'number') {
          fieldType = Number.isInteger(representative) ? 'integer' : 'number';
        } else if (typeof representative === 'boolean') {
          fieldType = 'boolean';
        }
        // string is already the default
      }
      fields.push({
        path,
        label,
        type: fieldType,
        enum: Array.isArray(prop.enum) ? prop.enum : null,
        enumLabels: null,
      });
    }
  }

  return fields;
}

// ─── Field intersection (multi-schema search) ────────────────────────────────

export function intersectFields(fieldLists: FlatField[][]): FlatField[] {
  if (fieldLists.length === 0) return [];
  if (fieldLists.length === 1) return fieldLists[0];

  const base = new Map(fieldLists[0].map(f => [f.path, { ...f }]));

  for (let i = 1; i < fieldLists.length; i++) {
    const currentPaths = new Set(fieldLists[i].map(f => f.path));

    for (const path of base.keys()) {
      if (!currentPaths.has(path)) {
        base.delete(path);
      }
    }

    for (const field of fieldLists[i]) {
      const existing = base.get(field.path);
      if (!existing) continue;

      const numericTypes = new Set(['number', 'integer']);
      if (existing.type !== field.type) {
        if (numericTypes.has(existing.type) && numericTypes.has(field.type)) {
          existing.type = 'number';
        } else {
          existing.type = 'string';
          existing.enum = null;
          existing.enumLabels = null;
          continue;
        }
      }
      if (existing.enum && field.enum) {
        const a = [...existing.enum].sort();
        const b = [...field.enum].sort();
        if (JSON.stringify(a) !== JSON.stringify(b)) {
          existing.enum = null;
          existing.enumLabels = null;
        }
      } else if (existing.enum !== field.enum) {
        existing.enum = null;
        existing.enumLabels = null;
      }
    }
  }

  return Array.from(base.values());
}
