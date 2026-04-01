import { LitElement, html, css } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import { sharedStyles } from '../../styles';
import type { JSONSchema } from '../../schema-core';

@customElement('schema-array-editor')
export class SchemaArrayEditor extends LitElement {
  static override styles = [sharedStyles, css`
    .grid { display: grid; grid-template-columns: 1fr 1fr; gap: 10px; }
  `];

  @property({ type: Object }) schema: JSONSchema = {};

  private _emit(field: string, value: any) {
    this.dispatchEvent(new CustomEvent('constraint-change', {
      detail: { field, value: value === '' ? undefined : value },
      bubbles: true, composed: true,
    }));
  }

  override render() {
    return html`
      <div class="type-section">
        <h4>Array Constraints</h4>
        <div class="grid">
          <div>
            <label>Min Items</label>
            <input type="number" min="0" .value=${this.schema.minItems ?? ''} @change=${(e: Event) => {
              const v = (e.target as HTMLInputElement).value;
              this._emit('minItems', v ? parseInt(v) : undefined);
            }}>
          </div>
          <div>
            <label>Max Items</label>
            <input type="number" min="0" .value=${this.schema.maxItems ?? ''} @change=${(e: Event) => {
              const v = (e.target as HTMLInputElement).value;
              this._emit('maxItems', v ? parseInt(v) : undefined);
            }}>
          </div>
          <div>
            <label><input type="checkbox" ?checked=${this.schema.uniqueItems} @change=${(e: Event) => this._emit('uniqueItems', (e.target as HTMLInputElement).checked || undefined)}> Unique Items</label>
          </div>
        </div>
      </div>
    `;
  }
}
