import { generateParamNameForMeta } from './freeFields.js';

/**
 * Recursively flatten a JSON Schema into a list of searchable field descriptors.
 */
export function flattenSchema(schema, prefix = '', labelPrefix = '') {
  if (!schema || schema.type !== 'object' || !schema.properties) {
    return [];
  }

  const fields = [];

  for (const [key, prop] of Object.entries(schema.properties)) {
    const path = prefix ? `${prefix}.${key}` : key;
    const rawLabel = prop.title || titleCase(key);
    const label = labelPrefix ? `${labelPrefix} › ${rawLabel}` : rawLabel;

    if (prop.type === 'object' && prop.properties) {
      fields.push(...flattenSchema(prop, path, label));
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
 */
export function intersectFields(fieldLists) {
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

      if (existing.type !== field.type) {
        existing.type = 'string';
        existing.enum = null;
      } else if (existing.enum && field.enum) {
        if (JSON.stringify(existing.enum) !== JSON.stringify(field.enum)) {
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

export function schemaSearchFields({ elName, existingMetaQuery, id }) {
  return {
    elName,
    id,
    fields: [],
    hasFields: false,

    init() {
      this._existingMeta = existingMetaQuery || [];
    },

    handleCategoryChange(items) {
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
        return;
      }

      const fieldLists = schemas.map(s => flattenSchema(s));
      const merged = schemas.length === 1 ? fieldLists[0] : intersectFields(fieldLists);

      this.fields = merged.map(field => {
        const op = defaultOperator(field);
        const existing = this._findExistingValue(field.path);

        return {
          ...field,
          operator: existing ? existing.operator : op,
          value: existing ? existing.value : '',
          enumValues: existing ? existing.enumValues : [],
          boolValue: existing ? existing.boolValue : 'any',
          showOperator: false,
          operators: operatorsForType(field),
        };
      });

      this.hasFields = this.fields.length > 0;
    },

    _findExistingValue(path) {
      const matches = this._existingMeta.filter(m => m.Key === path);
      if (matches.length === 0) return null;

      if (matches.length > 1) {
        return {
          operator: matches[0].Operation || 'EQ',
          value: '',
          enumValues: matches.map(m => String(m.Value)),
          boolValue: 'any',
        };
      }

      const m = matches[0];
      if (typeof m.Value === 'boolean') {
        return {
          operator: 'EQ',
          value: '',
          enumValues: [],
          boolValue: String(m.Value),
        };
      }

      return {
        operator: m.Operation || 'EQ',
        value: m.Value != null ? String(m.Value) : '',
        enumValues: [],
        boolValue: 'any',
      };
    },

    getSymbol(field) {
      return operatorSymbol(field.operator);
    },

    toggleOperator(field) {
      field.showOperator = !field.showOperator;
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
