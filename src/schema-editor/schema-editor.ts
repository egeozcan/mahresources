import { LitElement, html, css, nothing } from 'lit';
import { customElement, property, state } from 'lit/decorators.js';
import { sharedStyles } from './styles';
import type { JSONSchema } from './schema-core';
import './modes/edit-mode';
import './modes/form-mode';
import './modes/search-mode';

@customElement('schema-editor')
export class SchemaEditor extends LitElement {
  static override styles = [
    sharedStyles,
    css`
      :host {
        display: block;
      }
    `,
  ];

  /**
   * Form and search modes need light DOM so that hidden inputs are visible
   * to the parent `<form>` element and Tailwind styles are inherited.
   * Edit mode uses shadow DOM for style isolation inside the modal.
   *
   * Note: `createRenderRoot` is called once during first render and cached.
   * In practice each `<schema-editor>` element has a fixed `mode` attribute
   * set in the template, so this works correctly.
   */
  override createRenderRoot() {
    if (this.mode === 'edit') {
      return super.createRenderRoot();
    }
    return this;
  }

  @property({ type: String }) mode: 'edit' | 'form' | 'search' = 'edit';
  @property({ type: String }) schema = '';
  @property({ type: String }) value = '';
  @property({ type: String }) name = 'Meta';
  @property({ type: String, attribute: 'meta-query' }) metaQuery = '';
  @property({ type: String, attribute: 'field-name' }) fieldName = 'MetaQuery';

  @state() private _parsedSchema: JSONSchema | null = null;

  override willUpdate(changed: Map<string, unknown>) {
    if (changed.has('schema')) {
      this._parseSchema();
    }
  }

  private _parseSchema() {
    if (!this.schema) {
      this._parsedSchema = null;
      return;
    }
    // When Alpine binds with :schema="currentSchema", the value may arrive
    // as an already-parsed object (not a JSON string).  Handle both cases.
    if (typeof this.schema === 'object') {
      this._parsedSchema = this.schema as any;
      return;
    }
    try {
      this._parsedSchema = JSON.parse(this.schema);
    } catch {
      this._parsedSchema = null;
    }
  }

  // ─── Public API ──────────────────────────────────────────────────────────

  getSchema(): string {
    return this.schema;
  }

  getValue(): object {
    if (!this.value) return {};
    try {
      return JSON.parse(this.value);
    } catch {
      return {};
    }
  }

  validate(): boolean {
    // Placeholder — will be implemented in form-mode
    return true;
  }

  // ─── Render ──────────────────────────────────────────────────────────────

  override render() {
    if (!this._parsedSchema) {
      return html`<slot></slot>`;
    }

    switch (this.mode) {
      case 'edit':
        return html`<schema-edit-mode .schema=${this._parsedSchema} @schema-change=${(e: CustomEvent) => {
          this.schema = e.detail.schema;
          this.dispatchEvent(new CustomEvent('schema-change', { detail: e.detail, bubbles: true, composed: true }));
        }}></schema-edit-mode>`;
      case 'form': {
        let parsedValue = {};
        try { parsedValue = this.value ? JSON.parse(this.value) : {}; } catch { /* invalid JSON */ }
        return html`<schema-form-mode
          .schema=${this._parsedSchema}
          .value=${parsedValue}
          .name=${this.name}
          @value-change=${(e: CustomEvent) => {
            this.value = JSON.stringify(e.detail.value);
            this.dispatchEvent(new CustomEvent('value-change', { detail: e.detail, bubbles: true, composed: true }));
          }}
        ></schema-form-mode>`;
      }
      case 'search':
        return html`<schema-search-mode
          .schema=${this.schema}
          .metaQuery=${this.metaQuery}
          .fieldName=${this.fieldName}
        ></schema-search-mode>`;
      default:
        return nothing;
    }
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'schema-editor': SchemaEditor;
  }
}
