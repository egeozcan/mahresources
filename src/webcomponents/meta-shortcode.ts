// src/webcomponents/meta-shortcode.ts
import { LitElement, html, nothing, type TemplateResult } from 'lit';
import { customElement, property, state } from 'lit/decorators.js';
import { detectShape, getBuiltinRenderer } from '../schema-editor/display-renderers';
import type { JSONSchema } from '../schema-editor/schema-core';
import { titleCase } from '../schema-editor/schema-core';

@customElement('meta-shortcode')
export class MetaShortcode extends LitElement {
  @property({ attribute: 'data-path' }) path = '';
  @property({ attribute: 'data-editable' }) editable = 'false';
  @property({ attribute: 'data-hide-empty' }) hideEmpty = 'false';
  @property({ attribute: 'data-entity-type' }) entityType = '';
  @property({ attribute: 'data-entity-id' }) entityId = '';
  @property({ attribute: 'data-schema' }) schemaStr = '';
  @property({ attribute: 'data-value' }) valueStr = '';

  @state() private _editing = false;
  @state() private _saving = false;
  @state() private _currentValue: any = undefined;
  @state() private _flash: 'success' | 'error' | null = null;
  @state() private _pluginHtml: string | null = null;
  @state() private _pluginError = false;
  private _pluginFetchVersion = 0;

  private _metaUpdateHandler = (e: Event) => {
    const detail = (e as CustomEvent).detail;
    if (detail?.entityType === this.entityType && detail?.entityId === this.entityId && detail?.meta) {
      // Another meta-shortcode on the same entity saved — refresh our value
      const parts = this.path.split('.');
      let current: any = detail.meta;
      for (const part of parts) {
        if (current == null || typeof current !== 'object') { current = undefined; break; }
        current = current[part];
      }
      this._currentValue = current;
      this.valueStr = current !== undefined ? JSON.stringify(current) : '';
      // Clear plugin display cache and advance fetch version so any
      // in-flight request from before this update is discarded.
      this._pluginHtml = null;
      this._pluginError = false;
      this._pluginFetchVersion++;
    }
  };

  override connectedCallback() {
    super.connectedCallback();
    document.addEventListener('meta-shortcode-updated', this._metaUpdateHandler);
  }

  override disconnectedCallback() {
    document.removeEventListener('meta-shortcode-updated', this._metaUpdateHandler);
    super.disconnectedCallback();
  }

  override createRenderRoot() {
    return this;
  }

  private get _schema(): JSONSchema | null {
    if (!this.schemaStr) return null;
    try {
      return JSON.parse(this.schemaStr);
    } catch {
      return null;
    }
  }

  private get _value(): any {
    if (this._currentValue !== undefined) return this._currentValue;
    if (!this.valueStr) return undefined;
    try {
      return JSON.parse(this.valueStr);
    } catch {
      return this.valueStr;
    }
  }

  private get _isEmpty(): boolean {
    const v = this._value;
    return v === undefined || v === null || (typeof v === 'string' && v.trim() === '');
  }

  private get _label(): string {
    const schema = this._schema;
    if (schema?.title) return schema.title;
    const parts = this.path.split('.');
    return titleCase(parts[parts.length - 1]);
  }

  private get _isEditable(): boolean {
    return this.editable === 'true';
  }

  override render(): TemplateResult | typeof nothing {
    if (this._isEmpty && this.hideEmpty === 'true' && !this._editing) {
      return nothing;
    }

    const flashClass = this._flash === 'success'
      ? 'bg-green-100 transition-colors duration-300'
      : this._flash === 'error'
        ? 'bg-red-100 transition-colors duration-300'
        : '';

    return html`
      <span class="meta-shortcode inline-flex items-center gap-1 ${flashClass}">
        ${this._editing ? this._renderEditMode() : this._renderDisplayMode()}
      </span>
    `;
  }

  private _renderDisplayMode(): TemplateResult {
    const value = this._value;
    const schema = this._schema;
    const editButton = this._isEditable
      ? html`<button
          type="button"
          class="inline-flex items-center p-0.5 border-0 bg-transparent cursor-pointer"
          aria-label="Edit ${this._label}"
          @click=${this._enterEditMode}
        >${this._pencilIcon()}</button>`
      : nothing;

    if (this._isEmpty) {
      return html`
        <span class="text-stone-400 text-sm">${this._label}: —</span>
        ${editButton}
      `;
    }

    return html`
      <span class="text-sm">${this._renderValue(value, schema)}</span>
      ${editButton}
    `;
  }

  private _renderValue(value: any, schema: JSONSchema | null): TemplateResult | string {
    const xDisplay = schema?.['x-display'] as string | undefined;
    if (xDisplay?.startsWith('plugin:')) {
      return this._renderPluginDisplay(value, xDisplay);
    }

    if (xDisplay) {
      const renderer = getBuiltinRenderer(xDisplay);
      if (renderer) {
        return html`<span>${renderer.render(value)}</span>`;
      }
    }

    if (value != null && typeof value === 'object' && !Array.isArray(value)) {
      const shape = detectShape(value);
      if (shape) {
        return html`<span>${shape.render(value)}</span>`;
      }
    }

    if (schema) {
      const type = Array.isArray(schema.type)
        ? schema.type.find((t: string) => t !== 'null') || 'string'
        : schema.type || 'string';

      if (type === 'boolean') return value ? 'Yes' : 'No';
      if (type === 'integer' || type === 'number') {
        return html`<span class="font-mono">${value}</span>`;
      }
    }

    if (typeof value === 'object') {
      return JSON.stringify(value);
    }
    return String(value);
  }

  private _renderPluginDisplay(value: any, xDisplay: string): TemplateResult {
    if (this._pluginHtml !== null) {
      const wrapper = document.createElement('span');
      wrapper.innerHTML = this._pluginHtml;
      return html`${wrapper}`;
    }
    if (this._pluginError) {
      return html`<span class="text-stone-400 text-xs italic">Render error</span>`;
    }

    const parts = xDisplay.split(':');
    if (parts.length < 3) {
      // Malformed x-display (e.g., "plugin:name" missing type) — fall back to raw value
      return typeof value === 'object' ? html`${JSON.stringify(value)}` : html`${String(value)}`;
    }
    this._fetchPluginDisplay(parts[1], parts[2], value);
    return html`<span class="text-stone-400 text-xs animate-pulse">Loading...</span>`;
  }

  private async _fetchPluginDisplay(pluginName: string, typeName: string, value: any) {
    const version = ++this._pluginFetchVersion;
    try {
      const resp = await fetch(`/v1/plugins/${pluginName}/display/render`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          type: typeName,
          value,
          schema: this._schema || {},
          field_path: this.path,
          field_label: this._label,
        }),
      });
      if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
      const html = await resp.text();
      // Discard if a newer fetch was started (value changed while in-flight)
      if (version === this._pluginFetchVersion) {
        this._pluginHtml = html;
      }
    } catch {
      if (version === this._pluginFetchVersion) {
        this._pluginError = true;
      }
    }
  }

  private _renderEditMode(): TemplateResult {
    const schema = this._schema;
    const value = this._value;

    return html`
      <div class="meta-shortcode-edit border border-stone-300 rounded p-2 my-1">
        ${schema
          ? html`<schema-editor
              mode="form"
              .schema=${JSON.stringify(schema)}
              .value=${JSON.stringify(value ?? this._defaultValue(schema))}
              name="_meta_shortcode_value"
              @value-change=${this._onFormValueChange}
            ></schema-editor>`
          : html`<input
              type="text"
              class="border border-stone-300 rounded px-2 py-1 text-sm w-full"
              .value=${value != null ? (typeof value === 'object' ? JSON.stringify(value) : String(value)) : ''}
              @input=${this._onInputChange}
            />`
        }
        <div class="flex gap-2 mt-2">
          <button
            type="button"
            class="px-3 py-1 text-sm bg-indigo-600 text-white rounded hover:bg-indigo-700 disabled:opacity-50"
            ?disabled=${this._saving}
            @click=${this._save}
          >${this._saving ? 'Saving...' : 'Save'}</button>
          <button
            type="button"
            class="px-3 py-1 text-sm bg-stone-200 text-stone-700 rounded hover:bg-stone-300"
            ?disabled=${this._saving}
            @click=${this._cancelEdit}
          >Cancel</button>
        </div>
      </div>
    `;
  }

  private _editValue: any = undefined;

  private _onFormValueChange(e: CustomEvent) {
    this._editValue = e.detail.value;
  }

  private _onInputChange(e: Event) {
    const input = e.target as HTMLInputElement;
    try {
      this._editValue = JSON.parse(input.value);
    } catch {
      this._editValue = input.value;
    }
  }

  private _defaultValue(schema: JSONSchema): any {
    const type = schema.type;
    if (type === 'object') return {};
    if (type === 'array') return [];
    if (type === 'string') return '';
    if (type === 'number' || type === 'integer') return 0;
    if (type === 'boolean') return false;
    return null;
  }

  private _enterEditMode() {
    this._editValue = this._value;
    this._editing = true;
  }

  private _cancelEdit() {
    this._editing = false;
    this._editValue = undefined;
  }

  private async _save() {
    this._saving = true;

    const value = this._editValue !== undefined ? this._editValue : this._value;
    const valueJSON = JSON.stringify(value);

    const formData = new FormData();
    formData.append('path', this.path);
    formData.append('value', valueJSON);

    try {
      const resp = await fetch(
        `/v1/${this.entityType}/editMeta?id=${this.entityId}`,
        { method: 'POST', body: formData }
      );

      if (!resp.ok) throw new Error(`HTTP ${resp.status}`);

      const result = await resp.json();
      if (result.meta) {
        const parts = this.path.split('.');
        let current: any = result.meta;
        for (const part of parts) {
          if (current == null || typeof current !== 'object') {
            current = undefined;
            break;
          }
          current = current[part];
        }
        this._currentValue = current;
        this.valueStr = current !== undefined ? JSON.stringify(current) : '';
      }

      this._editing = false;
      this._flash = 'success';
      setTimeout(() => { this._flash = null; }, 1000);

      // Notify other meta-shortcode elements on the same entity to refresh
      document.dispatchEvent(new CustomEvent('meta-shortcode-updated', {
        detail: { entityType: this.entityType, entityId: this.entityId, meta: result.meta },
      }));
    } catch (err) {
      console.error('Meta shortcode save error:', err);
      this._flash = 'error';
      setTimeout(() => { this._flash = null; }, 1000);
    } finally {
      this._saving = false;
    }
  }

  private _pencilIcon(): TemplateResult {
    return html`
      <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="text-stone-400 hover:text-stone-600">
        <path d="M17 3a2.85 2.83 0 1 1 4 4L7.5 20.5 2 22l1.5-5.5Z"/>
        <path d="m15 5 4 4"/>
      </svg>
    `;
  }
}
