import { LitElement, html, nothing, type TemplateResult } from 'lit';
import { customElement, property, state } from 'lit/decorators.js';
import type { JSONSchema } from '../schema-core';
import {
  resolveSchema,
  isLabeledEnum,
  getLabeledEnumEntries,
  titleCase,
} from '../schema-core';
import { detectShape, getBuiltinRenderer } from '../display-renderers';

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
  xDisplay: string;
  rawSchema: JSONSchema;
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
  if (typeof field.value === 'object' && field.value !== null && !Array.isArray(field.value)) return true;
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

    // Read x-display annotation
    const xDisplay = (rawProp['x-display'] || prop['x-display'] || '') as string;

    // If x-display is set on an object, do NOT flatten — emit as whole field
    if (xDisplay && prop.properties) {
      // fall through to emit as single field
    } else if (prop.properties) {
      // If the value matches a known shape, keep it whole for the renderer pipeline
      if (val != null && typeof val === 'object' && !Array.isArray(val) && detectShape(val)) {
        // fall through to emit as single field (shape detection in _renderValue)
      } else {
        // Nested object with properties — flatten recursively
        fields.push(...flattenForDisplay(prop, value, root, path, label, depth + 1));
        continue;
      }
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
      xDisplay,
      rawSchema: prop,
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
  @state() private _pluginHtml: Record<string, string> = {};
  @state() private _pluginErrors: Record<string, boolean> = {};

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
      <div class="group relative">
        <div class="text-[10px] font-mono uppercase text-stone-400 tracking-wider mb-1 cursor-pointer hover:text-stone-600"
          style="letter-spacing: 0.05em;"
          title=${field.description || nothing}
          @click=${() => this._copyText(field.path)}
        >${field.label}</div>
        <div class="text-sm text-stone-900 cursor-pointer"
          @click=${() => this._copyValue(field.value)}>${this._renderValue(field)}</div>
      </div>
    `;
  }

  private _renderLongField(field: DisplayField): TemplateResult {
    return html`
      <div class="mb-3 last:mb-0">
        <div class="text-[10px] font-mono uppercase text-stone-400 tracking-wider mb-1 cursor-pointer hover:text-stone-600"
          style="letter-spacing: 0.05em;"
          title=${field.description || nothing}
          @click=${() => this._copyText(field.path)}
        >${field.label}</div>
        <div class="text-sm text-stone-900 cursor-pointer"
          @click=${() => this._copyValue(field.value)}>${this._renderValue(field)}</div>
      </div>
    `;
  }

  private _renderValue(field: DisplayField): TemplateResult | string {
    if (field.isEmpty) {
      return html`<span class="text-stone-300">\u2014</span>`;
    }

    const val = field.value;

    // ── Renderer pipeline ──────────────────────────────────────────────
    const xd = field.xDisplay;

    // 1. Plugin renderer (x-display: "plugin:name:type")
    if (xd.startsWith('plugin:')) {
      return this._renderPluginDisplay(field);
    }

    // 2. Forced built-in renderer (x-display: "url", "geo", etc.)
    if (xd && xd !== 'raw' && xd !== 'none') {
      const renderer = getBuiltinRenderer(xd);
      if (renderer) return renderer.render(val);
    }

    // 3. Opt-out: x-display "raw" or "none" skips shape detection
    // (falls through to existing object/scalar handling below)

    // 4. Auto shape detection for objects (when no x-display set)
    if (!xd && typeof val === 'object' && val !== null && !Array.isArray(val)) {
      const detected = detectShape(val);
      if (detected) return detected.render(val);
    }
    // ── End renderer pipeline ──────────────────────────────────────────

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

    // Array
    if (field.type === 'array' && Array.isArray(val)) {
      if (val.length === 0) return html`<span class="text-stone-300">\u2014</span>`;
      const allScalar = val.every(v => typeof v !== 'object' || v === null);
      if (allScalar) {
        return html`${val.map((v, i) => html`<span
          class="inline-block text-xs px-2 py-0.5 rounded-full bg-stone-100 text-stone-600 font-medium mr-1 mb-1"
        >${String(v)}</span>`)}`;
      }
      // Array of objects — render each as a key-value sub-grid
      return html`${val.map((item, i) => html`
        ${i > 0 ? html`<hr class="my-2 border-stone-100">` : nothing}
        ${this._renderObjectValue(item)}
      `)}`;
    }

    // Object — render as inline key-value grid
    if (typeof val === 'object' && val !== null && !Array.isArray(val)) {
      return this._renderObjectValue(val);
    }

    // Default — plain string
    if (field.isLong && typeof val === 'string') {
      return html`<span style="white-space: pre-wrap;">${val}</span>`;
    }

    return String(val ?? '');
  }

  private _renderObjectValue(obj: Record<string, any>): TemplateResult {
    const entries = Object.entries(obj).filter(([, v]) => !isEmptyValue(v));
    if (entries.length === 0) return html`<span class="text-stone-300">\u2014</span>`;
    return html`
      <div class="grid gap-x-4 gap-y-1 bg-stone-50 rounded p-2" style="grid-template-columns: auto 1fr;">
        ${entries.map(([k, v]) => {
          const display = typeof v === 'object' && v !== null
            ? JSON.stringify(v)
            : String(v);
          return html`
            <span class="text-[10px] font-mono uppercase text-stone-400 tracking-wider self-baseline" style="letter-spacing:0.05em;">${titleCase(k)}</span>
            <span class="text-sm text-stone-700 break-all self-baseline">${display}</span>
          `;
        })}
      </div>
    `;
  }

  private _renderPluginDisplay(field: DisplayField): TemplateResult {
    const key = field.path;

    if (this._pluginHtml[key] !== undefined) {
      const wrapper = document.createElement('div');
      wrapper.innerHTML = this._pluginHtml[key];
      return html`${wrapper}`;
    }

    if (this._pluginErrors[key]) {
      if (typeof field.value === 'object' && field.value !== null) {
        return this._renderObjectValue(field.value);
      }
      return html`<span class="text-stone-400 text-xs italic">Render error</span>`;
    }

    const parts = field.xDisplay.split(':');
    if (parts.length < 3) {
      return this._renderObjectValue(field.value);
    }
    const pluginName = parts[1];
    const typeName = parts[2];

    this._fetchPluginDisplay(key, pluginName, typeName, field);
    return html`<span class="text-stone-400 text-xs animate-pulse">Loading...</span>`;
  }

  private async _fetchPluginDisplay(key: string, pluginName: string, typeName: string, field: DisplayField) {
    try {
      const resp = await fetch(`/v1/plugins/${pluginName}/display/render`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          type: typeName,
          value: field.value,
          schema: field.rawSchema || {},
          field_path: field.path,
          field_label: field.label,
        }),
      });
      if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
      const htmlStr = await resp.text();
      this._pluginHtml = { ...this._pluginHtml, [key]: htmlStr };
    } catch {
      this._pluginErrors = { ...this._pluginErrors, [key]: true };
    }
  }

  private _copyText(text: string) {
    navigator.clipboard?.writeText(text).catch(() => {});
  }

  private _copyValue(val: any) {
    if (val === null || val === undefined) return;
    const text = typeof val === 'object' ? JSON.stringify(val) : String(val);
    navigator.clipboard?.writeText(text).catch(() => {});
  }
}
