import { generateParamNameForMeta } from './freeFields.js';

/**
 * Resolve a JSON Pointer $ref (e.g., "#/definitions/address") against a root schema.
 */
function resolveRef(ref, root) {
  if (typeof ref !== 'string' || !ref.startsWith('#/')) return null;
  let current = root;
  for (const part of ref.split('/').slice(1)) {
    if (current && typeof current === 'object' && part in current) {
      current = current[part];
    } else {
      return null;
    }
  }
  return current;
}

/**
 * Merge two schemas (for allOf). Combines properties and required arrays,
 * copies other keys from the extension, and strips composition keywords.
 */
function mergeSchemas(base, extension) {
  const merged = { ...base };
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

/**
 * Resolve a schema that may use composition keywords ($ref, allOf).
 * Returns a plain schema with type/properties ready for flattening.
 */
function resolveSchema(schema, rootSchema) {
  if (!schema) return schema;

  // Resolve $ref
  if (schema.$ref) {
    const resolved = resolveRef(schema.$ref, rootSchema);
    if (resolved) {
      const merged = { ...resolved, ...schema };
      delete merged.$ref;
      return resolveSchema(merged, rootSchema);
    }
    return null;
  }

  // Merge allOf / oneOf / anyOf — for search we union all variant properties
  // so users can filter on any field from any variant.
  for (const keyword of ['allOf', 'oneOf', 'anyOf']) {
    if (schema[keyword] && Array.isArray(schema[keyword])) {
      let merged = { ...schema };
      delete merged[keyword];
      for (const sub of schema[keyword]) {
        let resolved;
        if (sub.$ref) {
          // Resolve the $ref, then merge sibling properties from the original
          // sub-schema so { $ref: '...', properties: { extra: ... } } keeps extra.
          const refResult = resolveRef(sub.$ref, rootSchema);
          const siblings = { ...sub };
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

/**
 * Recursively flatten a JSON Schema into a list of searchable field descriptors.
 * Supports $ref, allOf, and nested objects.
 *
 * @param {object} schema - Parsed JSON Schema object
 * @param {string} prefix - Dot-separated path prefix for nested fields
 * @param {string} labelPrefix - Human-readable label prefix
 * @param {number} depth - Current recursion depth (max 10)
 * @param {object} rootSchema - Top-level schema for $ref resolution
 * @returns {Array<{path: string, label: string, type: string, enum: string[]|null}>}
 */
export function flattenSchema(schema, prefix = '', labelPrefix = '', depth = 0, rootSchema = null) {
  if (depth > 10 || !schema) return [];

  const root = rootSchema || schema;
  const resolved = resolveSchema(schema, root);
  if (!resolved || !resolved.properties) {
    return [];
  }

  const fields = [];

  for (const [key, rawProp] of Object.entries(resolved.properties)) {
    const path = prefix ? `${prefix}.${key}` : key;
    // Resolve $ref on individual properties before inspecting type
    const prop = resolveSchema(rawProp, root) || rawProp;
    const rawLabel = prop.title || titleCase(key);
    const label = labelPrefix ? `${labelPrefix} › ${rawLabel}` : rawLabel;

    if (prop.properties) {
      fields.push(...flattenSchema(prop, path, label, depth + 1, root));
    } else if (prop.type === 'array') {
      continue;
    } else {
      fields.push({
        path,
        label,
        type: prop.type || 'string',
        enum: Array.isArray(prop.enum) ? prop.enum : null,
      });
    }
  }

  return fields;
}

/**
 * Intersect multiple flattened field lists. Keep only fields present in ALL lists.
 * Type conflicts fall back to "string". Enum conflicts drop the enum.
 *
 * @param {Array<Array<{path: string, label: string, type: string, enum: string[]|null}>>} fieldLists
 * @returns {Array<{path: string, label: string, type: string, enum: string[]|null}>}
 */
export function intersectFields(fieldLists) {
  if (fieldLists.length === 0) return [];
  if (fieldLists.length === 1) return fieldLists[0];

  // Index first list by path — safe to delete during forward iteration
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
          // integer and number are compatible — merge to number,
          // but still reconcile enums (one side may lack an enum)
          existing.type = 'number';
        } else {
          existing.type = 'string';
          existing.enum = null;
          continue;
        }
      }
      // Reconcile enums (runs for same-type AND numeric-merge cases)
      if (existing.enum && field.enum) {
        // Sort before comparing so order doesn't matter
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

function titleCase(key) {
  return key
    .replace(/([a-z])([A-Z])/g, '$1 $2')
    .replace(/[_-]/g, ' ')
    .replace(/\b\w/g, c => c.toUpperCase());
}

function defaultOperator(field) {
  if (field.enum) return 'EQ';
  if (field.type === 'boolean') return 'EQ';
  if (field.type === 'string') return 'LI';
  return 'EQ';
}

function operatorsForType(field) {
  if (field.enum || field.type === 'boolean') return null;
  if (field.type === 'string') {
    return [
      { code: 'LI', label: 'LIKE' },
      { code: 'EQ', label: '=' },
      { code: 'NE', label: '≠' },
      { code: 'NL', label: 'NOT LIKE' },
    ];
  }
  return [
    { code: 'EQ', label: '=' },
    { code: 'NE', label: '≠' },
    { code: 'GT', label: '>' },
    { code: 'GE', label: '≥' },
    { code: 'LT', label: '<' },
    { code: 'LE', label: '≤' },
  ];
}

function operatorSymbol(code) {
  const symbols = { EQ: '=', NE: '≠', LI: '≈', NL: '≉', GT: '>', GE: '≥', LT: '<', LE: '≤' };
  return symbols[code] || code;
}

/**
 * Alpine.js data component for schema-driven search fields.
 *
 * @param {object} opts
 * @param {string} opts.elName - The autocompleter element name to listen for
 * @param {Array} opts.existingMetaQuery - Pre-parsed MetaQuery from URL
 * @param {Array} opts.initialCategories - Categories already selected on page load (from URL params)
 * @param {string} opts.id - Unique ID prefix for form elements
 */
export function schemaSearchFields({ elName, existingMetaQuery, initialCategories, id }) {
  return {
    elName,
    id,
    /** @type {Array<{path: string, label: string, type: string, enum: string[]|null, operator: string, value: string, enumValues: string[], showOperator: boolean}>} */
    fields: [],
    hasFields: false,
    /** Whether fields were just cleared (for aria-live announcement) */
    fieldsCleared: false,

    init() {
      this._existingMeta = existingMetaQuery || [];
      // If categories were pre-selected on page load (restored from URL params),
      // call handleCategoryChange immediately so schema fields and pre-filled values render.
      // Without this, the autocompleter only dispatches multiple-input on later mutations
      // (dropdown.js line 59), so schema fields stay empty after form submit or saved URLs.
      const initial = initialCategories || [];
      if (initial.length > 0) {
        this.$nextTick(() => this.handleCategoryChange(initial));
      }
    },

    clearFields(hadFields) {
      this.fields = [];
      this.hasFields = false;
      this.fieldsCleared = hadFields;
      // Release all claimed paths so freeFields can restore them.
      window.dispatchEvent(new CustomEvent('schema-fields-claimed', {
        detail: { paths: [] },
      }));
    },

    handleCategoryChange(items) {
      const hadFields = this.hasFields;

      // Snapshot current field values so they survive category changes.
      // Keyed by field path → { value, operator, enumValues, boolValue }.
      const currentValues = new Map();
      for (const f of this.fields) {
        currentValues.set(f.path, {
          value: f.value,
          operator: f.operator,
          enumValues: f.enumValues,
          boolValue: f.boolValue,
        });
      }

      const selectedItems = items || [];
      if (selectedItems.length === 0) {
        this.clearFields(hadFields);
        return;
      }

      const schemas = [];
      for (const item of selectedItems) {
        // If any selected category lacks a usable schema, the safe fallback is
        // to hide schema-driven filters rather than imply they apply to all.
        if (!item?.MetaSchema) {
          this.clearFields(hadFields);
          return;
        }

        try {
          schemas.push(JSON.parse(item.MetaSchema));
        } catch {
          this.clearFields(hadFields);
          return;
        }
      }

      if (schemas.length === 0) {
        this.clearFields(hadFields);
        return;
      }

      const fieldLists = schemas.map(s => flattenSchema(s));
      const merged = schemas.length === 1 ? fieldLists[0] : intersectFields(fieldLists);

      // Track paths we can't represent (e.g., range queries on non-enum fields)
      // so we don't claim them from freeFields.
      const unclaimable = new Set();

      this.fields = merged.map(field => {
        const op = defaultOperator(field);
        // Prefer in-progress values (from current session), fall back to URL state
        const current = currentValues.get(field.path);
        let existing = current || this._findExistingValue(field.path);

        // Non-enum fields with multiple URL matches (e.g., weight:GT:5 + weight:LT:10,
        // or active:EQ:true + active:EQ:false) can't be represented in a single input.
        // Leave them for freeFields.
        if (!field.enum && existing && existing.enumValues.length > 0) {
          existing = null;
          unclaimable.add(field.path);
        }

        // Enum and boolean schema fields only support EQ semantics.
        // If any URL entry for this path uses a non-EQ operator (NE, NL, GT, etc.),
        // the schema UI can't represent it — leave it for freeFields.
        if (!current && (field.enum || field.type === 'boolean')) {
          const rawMatches = this._existingMeta.filter(m => m.name === field.path);
          if (rawMatches.some(m => m.operation && m.operation !== 'EQ')) {
            existing = null;
            unclaimable.add(field.path);
          }
        }

        let enumValues = existing ? existing.enumValues : [];
        // For enum fields with a single existing value, _findExistingValue puts
        // it in `value` (not `enumValues`). Route it into enumValues so the
        // checkbox/select UI shows it as checked.
        if (field.enum && existing && enumValues.length === 0 && existing.value) {
          enumValues = [existing.value];
        }

        return {
          ...field,
          operator: existing ? existing.operator : op,
          value: (existing && !field.enum) ? existing.value : '',
          enumValues,
          boolValue: existing ? existing.boolValue : 'any',
          showOperator: false,
          operators: operatorsForType(field),
        };
      });

      this.hasFields = this.fields.length > 0;
      this.fieldsCleared = hadFields && !this.hasFields;

      // Notify freeFields which paths are claimed by schema fields so it
      // can exclude them and avoid duplicate MetaQuery submissions.
      // Exclude paths we can't represent (range queries, etc.).
      const claimedPaths = this.fields
        .filter(f => !unclaimable.has(f.path))
        .map(f => f.path);
      window.dispatchEvent(new CustomEvent('schema-fields-claimed', {
        detail: { paths: claimedPaths },
      }));
    },

    _findExistingValue(path) {
      // ColumnMeta is JSON-serialised with lowercase keys matching its json struct tags:
      // Key → "name", Value → "value", Operation → "operation"
      const matches = this._existingMeta.filter(m => m.name === path);
      if (matches.length === 0) return null;

      if (matches.length > 1) {
        return {
          operator: matches[0].operation || 'EQ',
          value: '',
          enumValues: matches.map(m => String(m.value)),
          boolValue: 'any',
        };
      }

      const m = matches[0];
      if (typeof m.value === 'boolean') {
        return {
          operator: 'EQ',
          value: '',
          enumValues: [],
          boolValue: String(m.value),
        };
      }

      return {
        operator: m.operation || 'EQ',
        value: m.value != null ? String(m.value) : '',
        enumValues: [],
        boolValue: 'any',
      };
    },

    getSymbol(field) {
      return operatorSymbol(field.operator);
    },

    toggleOperator(field) {
      field.showOperator = !field.showOperator;
      this.$nextTick(() => {
        const wrapper = this.$el.querySelector(`[data-field-path="${field.path}"]`);
        if (!wrapper) return;
        const target = field.showOperator
          ? wrapper.querySelector('select')
          : wrapper.querySelector('button[data-operator-toggle]');
        if (target) target.focus();
      });
    },

    selectOperator(field) {
      field.showOperator = false;
      this.$nextTick(() => {
        const wrapper = this.$el.querySelector(`[data-field-path="${field.path}"]`);
        if (!wrapper) return;
        const btn = wrapper.querySelector('button[data-operator-toggle]');
        if (btn) btn.focus();
      });
    },

    getHiddenInputs(field) {
      if (field.type === 'boolean') {
        if (field.boolValue === 'any') return [];
        return [{ value: generateParamNameForMeta({ name: field.path, value: field.boolValue, operation: 'EQ' }) }];
      }

      if (field.enum) {
        // String enums must be quoted so coercible values like "007", "true", "null"
        // are preserved as strings. Numeric enums must NOT be quoted so the backend
        // matches them as numbers.
        const quote = field.type === 'string';
        return field.enumValues.map(v => ({
          value: quote
            ? `${field.path}:EQ:"${v}"`
            : `${field.path}:EQ:${v}`,
        }));
      }

      if (!field.value && field.value !== 0) return [];

      // For string-typed schema fields, always quote the value so the backend
      // treats it as a string. Without quotes, generateParamNameForMeta routes
      // through getJSONValue which coerces "42" → 42, "false" → false, etc.
      if (field.type === 'string') {
        return [{ value: `${field.path}:${field.operator}:"${field.value}"` }];
      }

      return [{ value: generateParamNameForMeta({ name: field.path, value: field.value, operation: field.operator }) }];
    },
  };
}
