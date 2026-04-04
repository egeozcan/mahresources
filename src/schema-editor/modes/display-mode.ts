import { LitElement, html, nothing, type TemplateResult } from 'lit';
import { customElement, property, state } from 'lit/decorators.js';
import type { JSONSchema } from '../schema-core';
import {
  resolveRef,
  isLabeledEnum,
  getLabeledEnumEntries,
  titleCase,
} from '../schema-core';

/** Resolved schema — follows $ref chains and merges allOf. */
function resolveSchema(schema: JSONSchema, root: JSONSchema): JSONSchema | null {
  if (!schema) return null;
  if (schema.$ref) {
    const resolved = resolveRef(schema.$ref, root);
    return resolved ? resolveSchema(resolved, root) : null;
  }
  return schema;
}

interface DisplayField {
  path: string;
  label: string;
  description: string;
  type: string;
  format: string;
  value: any;
  isEmpty: boolean;
  isLong: boolean;
  enum: any[] | null;
  enumLabels: string[] | null;
}

function getNestedValue(obj: any, path: string): any {
  const parts = path.split('.');
  let current = obj;
  for (const part of parts) {
    if (current == null || typeof current !== 'object') return undefined;
    current = current[part];
  }
  return current;
}

function isEmptyValue(val: any): boolean {
  if (val === null || val === undefined) return true;
  if (typeof val === 'string' && val.trim() === '') return true;
  return false;
}

const LONG_STRING_THRESHOLD = 80;

function classifyAsLong(field: DisplayField): boolean {
  if (field.type === 'array') return true;
  if (typeof field.value === 'string' && field.value.length > LONG_STRING_THRESHOLD) return true;
  return false;
}

function flattenForDisplay(
  schema: JSONSchema,
  value: any,
  root: JSONSchema,
  prefix = '',
  labelPrefix = '',
  depth = 0,
): DisplayField[] {
  if (depth > 3 || !schema) return [];
  const resolved = resolveSchema(schema, root);
  if (!resolved?.properties) return [];

  const fields: DisplayField[] = [];

  for (const [key, rawProp] of Object.entries(resolved.properties) as [string, JSONSchema][]) {
    const path = prefix ? `${prefix}.${key}` : key;
    const prop = resolveSchema(rawProp, root) || rawProp;
    const rawLabel = prop.title || titleCase(key);
    const label = labelPrefix ? `${labelPrefix} \u203A ${rawLabel}` : rawLabel;
    const description = prop.description || '';
    const format = prop.format || '';
    const val = getNestedValue(value, path);

    // Nested object with properties — flatten recursively
    if (prop.properties) {
      fields.push(...flattenForDisplay(prop, value, root, path, label, depth + 1));
      continue;
    }

    // Determine type
    let fieldType = prop.type || 'string';
    if (Array.isArray(fieldType)) {
      fieldType = fieldType.find((t: string) => t !== 'null') || 'string';
    }

    // Labeled enum detection
    let enumValues: any[] | null = null;
    let enumLabels: string[] | null = null;
    if (isLabeledEnum(prop)) {
      const entries = getLabeledEnumEntries(prop);
      enumValues = entries.map(e => e.value);
      enumLabels = entries.map(e => e.label);
    } else if (Array.isArray(prop.enum)) {
      enumValues = prop.enum;
    }

    const field: DisplayField = {
      path, label, description, type: fieldType, format,
      value: val,
      isEmpty: isEmptyValue(val),
      isLong: false,
      enum: enumValues,
      enumLabels,
    };
    field.isLong = classifyAsLong(field);
    fields.push(field);
  }

  return fields;
}

@customElement('schema-display-mode')
export class SchemaDisplayMode extends LitElement {
  @property({ type: Object }) schema: JSONSchema = {};
  @property({ type: Object }) value: any = {};
  @property({ type: String }) name = '';

  @state() private _showEmpty = false;

  // Light DOM to inherit Tailwind styles
  override createRenderRoot() {
    return this;
  }

  override render() {
    if (!this.schema?.properties || !this.value) return nothing;

    const allFields = flattenForDisplay(this.schema, this.value, this.schema);
    const filledFields = allFields.filter(f => !f.isEmpty);
    const emptyFields = allFields.filter(f => f.isEmpty);
    const visibleFields = this._showEmpty ? allFields : filledFields;
    const shortFields = visibleFields.filter(f => !f.isLong);
    const longFields = visibleFields.filter(f => f.isLong);

    if (filledFields.length === 0 && !this._showEmpty) return nothing;

    return html`
      <div class="detail-panel mb-6" aria-label="Schema metadata">
        <div class="detail-panel-header" style="background: #fafaf9;">
          <h2 class="detail-panel-title">Metadata</h2>
          ${this.name ? html`<span class="text-xs font-mono text-stone-400">${this.name}</span>` : nothing}
        </div>
        <div class="detail-panel-body" style="padding: 1rem;">
          ${shortFields.length > 0 ? html`
            <div class="grid gap-4" style="grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));">
              ${shortFields.map(f => this._renderShortField(f))}
            </div>
          ` : nothing}
          ${longFields.length > 0 ? html`
            <div class="${shortFields.length > 0 ? 'mt-4 pt-4 border-t border-stone-100' : ''}">
              ${longFields.map(f => this._renderLongField(f))}
            </div>
          ` : nothing}
          ${emptyFields.length > 0 ? html`
            <div class="mt-3 pt-3 border-t border-stone-100">
              <button
                class="text-xs font-mono text-stone-400 hover:text-stone-600 cursor-pointer bg-transparent border-none p-0"
                @click=${() => { this._showEmpty = !this._showEmpty; }}
              >${this._showEmpty
                ? 'Hide empty fields'
                : `Show ${emptyFields.length} hidden field${emptyFields.length !== 1 ? 's' : ''}`
              }</button>
            </div>
          ` : nothing}
        </div>
      </div>
    `;
  }

  private _renderShortField(field: DisplayField): TemplateResult {
    return html`
      <div class="group relative cursor-pointer"
        @click=${() => this._copyValue(field.value)}>
        <div class="text-[10px] font-mono uppercase text-stone-400 tracking-wider mb-1"
          style="letter-spacing: 0.05em;"
          title=${field.description || nothing}
        >${field.label}</div>
        <div class="text-sm text-stone-900">${this._renderValue(field)}</div>
      </div>
    `;
  }

  private _renderLongField(field: DisplayField): TemplateResult {
    return html`
      <div class="mb-3 last:mb-0 cursor-pointer"
        @click=${() => this._copyValue(field.value)}>
        <div class="text-[10px] font-mono uppercase text-stone-400 tracking-wider mb-1"
          style="letter-spacing: 0.05em;"
          title=${field.description || nothing}
        >${field.label}</div>
        <div class="text-sm text-stone-900">${this._renderValue(field)}</div>
      </div>
    `;
  }

  private _renderValue(field: DisplayField): TemplateResult | string {
    if (field.isEmpty) {
      return html`<span class="text-stone-300">\u2014</span>`;
    }

    const val = field.value;

    // Enum with labels — pill with tooltip
    if (field.enumLabels && field.enum) {
      const idx = field.enum.indexOf(val);
      const label = idx >= 0 && field.enumLabels[idx] ? field.enumLabels[idx] : String(val);
      const tooltip = idx >= 0 && field.enumLabels[idx] ? String(val) : '';
      return html`<span
        class="inline-block text-xs px-2.5 py-0.5 rounded-full bg-indigo-50 text-indigo-700 font-medium"
        title=${tooltip || nothing}
      >${label}</span>`;
    }

    // Plain enum — pill
    if (field.enum) {
      return html`<span
        class="inline-block text-xs px-2.5 py-0.5 rounded-full bg-emerald-50 text-emerald-700 font-medium"
      >${String(val)}</span>`;
    }

    // Boolean
    if (field.type === 'boolean') {
      return val ? 'Yes' : 'No';
    }

    // Number / integer
    if (field.type === 'number' || field.type === 'integer') {
      return html`<span class="font-mono">${String(val)}</span>`;
    }

    // String with format
    if (typeof val === 'string') {
      if (field.format === 'uri' || field.format === 'url') {
        return html`<a href=${val} target="_blank" rel="noopener noreferrer"
          class="text-indigo-600 hover:text-indigo-800 underline decoration-indigo-300"
          @click=${(e: Event) => e.stopPropagation()}
        >${val}</a>`;
      }
      if (field.format === 'email') {
        return html`<a href="mailto:${val}"
          class="text-indigo-600 hover:text-indigo-800 underline decoration-indigo-300"
          @click=${(e: Event) => e.stopPropagation()}
        >${val}</a>`;
      }
      if (field.format === 'date' || field.format === 'date-time') {
        try {
          const d = new Date(val);
          return d.toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' });
        } catch {
          return val;
        }
      }
    }

    // Array of scalars
    if (field.type === 'array' && Array.isArray(val)) {
      if (val.length === 0) return html`<span class="text-stone-300">\u2014</span>`;
      const allScalar = val.every(v => typeof v !== 'object' || v === null);
      if (allScalar) {
        return html`${val.map((v, i) => html`<span
          class="inline-block text-xs px-2 py-0.5 rounded-full bg-stone-100 text-stone-600 font-medium mr-1 mb-1"
        >${String(v)}</span>`)}`;
      }
      return html`<pre class="text-xs font-mono text-stone-600 bg-stone-50 p-2 rounded overflow-x-auto">${JSON.stringify(val, null, 2)}</pre>`;
    }

    // Default — plain string
    if (field.isLong && typeof val === 'string') {
      return html`<span style="white-space: pre-wrap;">${val}</span>`;
    }

    return String(val ?? '');
  }

  private _copyValue(val: any) {
    if (val === null || val === undefined) return;
    const text = typeof val === 'object' ? JSON.stringify(val) : String(val);
    navigator.clipboard?.writeText(text).catch(() => {});
  }
}
