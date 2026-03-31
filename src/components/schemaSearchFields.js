import { generateParamNameForMeta } from './freeFields.js';

/**
 * Recursively flatten a JSON Schema into a list of searchable field descriptors.
 *
 * @param {object} schema - Parsed JSON Schema object
 * @param {string} prefix - Dot-separated path prefix for nested fields
 * @param {string} labelPrefix - Human-readable label prefix
 * @param {number} depth - Current recursion depth (max 10)
 * @returns {Array<{path: string, label: string, type: string, enum: string[]|null}>}
 */
export function flattenSchema(schema, prefix = '', labelPrefix = '', depth = 0) {
  if (depth > 10 || !schema || schema.type !== 'object' || !schema.properties) {
    return [];
  }

  const fields = [];

  for (const [key, prop] of Object.entries(schema.properties)) {
    const path = prefix ? `${prefix}.${key}` : key;
    const rawLabel = prop.title || titleCase(key);
    const label = labelPrefix ? `${labelPrefix} › ${rawLabel}` : rawLabel;

    if (prop.type === 'object' && prop.properties) {
      fields.push(...flattenSchema(prop, path, label, depth + 1));
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

      if (existing.type !== field.type) {
        existing.type = 'string';
        existing.enum = null;
      } else if (existing.enum && field.enum) {
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

      const schemas = items
        .filter(item => item.MetaSchema)
        .map(item => {
          try { return JSON.parse(item.MetaSchema); }
          catch { return null; }
        })
        .filter(Boolean);

      if (schemas.length === 0) {
        this.fields = [];
        this.hasFields = false;
        this.fieldsCleared = hadFields;
        return;
      }

      const fieldLists = schemas.map(s => flattenSchema(s));
      const merged = schemas.length === 1 ? fieldLists[0] : intersectFields(fieldLists);

      this.fields = merged.map(field => {
        const op = defaultOperator(field);
        // Prefer in-progress values (from current session), fall back to URL state
        const current = currentValues.get(field.path);
        const existing = current || this._findExistingValue(field.path);

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
      window.dispatchEvent(new CustomEvent('schema-fields-claimed', {
        detail: { paths: this.fields.map(f => f.path) },
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
        return field.enumValues.map(v => ({
          value: generateParamNameForMeta({ name: field.path, value: v, operation: 'EQ' }),
        }));
      }

      if (!field.value && field.value !== 0) return [];

      return [{ value: generateParamNameForMeta({ name: field.path, value: field.value, operation: field.operator }) }];
    },
  };
}
