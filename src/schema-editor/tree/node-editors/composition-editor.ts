import { LitElement, html, css } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import { sharedStyles } from '../../styles';
import type { SchemaNode } from '../../schema-tree-model';

@customElement('schema-composition-editor')
export class SchemaCompositionEditor extends LitElement {
  static override styles = [sharedStyles, css`
    .variant { padding: 8px; border: 1px solid #e5e7eb; border-radius: 4px; margin-bottom: 8px; display: flex; align-items: center; gap: 8px; }
    .variant-name { flex: 1; font-size: 12px; }
    .variant-type { font-size: 11px; color: #6b7280; }
  `];

  @property({ type: String }) keyword: string = 'oneOf';
  @property({ type: Array }) variants: SchemaNode[] = [];

  private _addVariant() {
    this.dispatchEvent(new CustomEvent('add-variant', { bubbles: true, composed: true }));
  }

  private _removeVariant(index: number) {
    this.dispatchEvent(new CustomEvent('remove-variant', {
      detail: { index },
      bubbles: true, composed: true,
    }));
  }

  override render() {
    return html`
      <div class="type-section">
        <h4>${this.keyword} — ${this.variants.length} variant${this.variants.length !== 1 ? 's' : ''}</h4>
        ${this.variants.map((v, i) => html`
          <div class="variant">
            <span class="variant-name">${v.schema.title || v.name || `Variant ${i + 1}`}</span>
            <span class="variant-type">(${v.type})</span>
            <button class="btn btn-danger" @click=${() => this._removeVariant(i)} aria-label="Remove variant ${i + 1}">×</button>
          </div>
        `)}
        <button class="btn-ghost" @click=${this._addVariant}>+ Add Variant</button>
      </div>
    `;
  }
}
