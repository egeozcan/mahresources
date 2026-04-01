import { LitElement, html, css, nothing } from 'lit';
import { customElement, property, state } from 'lit/decorators.js';
import { sharedStyles } from './styles';
import type { JSONSchema } from './schema-core';
import './modes/edit-mode';
import './modes/form-mode';

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
      case 'form':
        return html`<schema-form-mode
          .schema=${this._parsedSchema}
          .value=${this.value ? JSON.parse(this.value) : {}}
          .name=${this.name}
          @value-change=${(e: CustomEvent) => {
            this.value = JSON.stringify(e.detail.value);
            this.dispatchEvent(new CustomEvent('value-change', { detail: e.detail, bubbles: true, composed: true }));
          }}
        ></schema-form-mode>`;
      case 'search':
        return html`<div class="search-mode-placeholder">Search mode — Task 12+</div>`;
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
