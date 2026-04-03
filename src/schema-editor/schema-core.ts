// ─── Type definitions ────────────────────────────────────────────────────────

export interface FlatField {
  path: string;
  label: string;
  type: string;
  enum: string[] | null;
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

export function mergeSchemas(base: JSONSchema, extension: JSONSchema): JSONSchema {
  const merged: JSONSchema = { ...base };
  for (const key in extension) {
    if (key === 'properties') {
      merged.properties = { ...(base.properties || {}), ...extension.properties };
    } else if (key === 'required') {
      merged.required = [...new Set([...(base.required || []), ...(extension.required || [])])];
    } else if (!['allOf', 'anyOf', 'oneOf', '$ref'].includes(key)) {
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
      const merged: JSONSchema = { ...resolved, ...schema };
      delete merged.$ref;
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
        if (resolved) merged = mergeSchemas(merged, resolved);
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

export function evaluateCondition(conditionSchema: JSONSchema | null | undefined, data: any): boolean {
  if (!conditionSchema) return true;

  // ── Top-level keyword checks (constrain the data value itself) ──────────

  // Top-level const — data itself must equal the value
  if (conditionSchema.const !== undefined && data !== conditionSchema.const) return false;

  // Top-level enum — data itself must be in the list
  if (conditionSchema.enum && !conditionSchema.enum.includes(data)) return false;

  // Top-level type check (only when there are no properties, to avoid
  // misinterpreting { type: "object", properties: {...} })
  if (conditionSchema.type && !conditionSchema.properties) {
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

  // Top-level string constraints
  if (conditionSchema.minLength !== undefined && (typeof data !== 'string' || data.length < conditionSchema.minLength)) return false;
  if (conditionSchema.maxLength !== undefined && (typeof data !== 'string' || data.length > conditionSchema.maxLength)) return false;

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
      const propSchema = conditionSchema.properties[key];
      const value = data?.[key];

      // Skip absent properties — they match vacuously per JSON Schema spec.
      if (value === undefined) continue;

      // Recurse into nested object properties
      if (propSchema.properties && typeof value === 'object' && value !== null && !Array.isArray(value)) {
        if (!evaluateCondition(propSchema, value)) return false;
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
    }
  }

  return true;
}

// ─── Schema match scoring ────────────────────────────────────────────────────

export function scoreSchemaMatch(schema: JSONSchema, data: unknown, rootSchema: JSONSchema): number {
  if (schema.$ref) {
    const resolved = resolveRef(schema.$ref, rootSchema);
    if (resolved) {
      schema = { ...resolved, ...schema };
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

    // Boost/penalize based on const/enum discriminator matches
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

  return 10;
}

// ─── Default values ──────────────────────────────────────────────────────────

export function getDefaultValue(schema: JSONSchema, rootSchema?: JSONSchema): any {
  if (schema.$ref) {
    const resolved = resolveRef(schema.$ref, rootSchema || schema);
    if (resolved) {
      return getDefaultValue({ ...resolved, ...schema, $ref: undefined }, rootSchema);
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
    const merged = mergeSchemas(baseSchema, schema.then || {});
    return getDefaultValue(merged, rootSchema);
  }

  if (schema.default !== undefined) return schema.default;
  if (schema.const !== undefined) return schema.const;
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

  if (schema.oneOf && schema.oneOf.length > 0) return getDefaultValue(schema.oneOf[0], rootSchema);
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
          continue;
        }
      }
      if (existing.enum && field.enum) {
        const a = [...existing.enum].sort();
        const b = [...field.enum].sort();
        if (JSON.stringify(a) !== JSON.stringify(b)) {
          existing.enum = null;
        }
      } else if (existing.enum !== field.enum) {
        existing.enum = null;
      }
    }
  }

  return Array.from(base.values());
}
