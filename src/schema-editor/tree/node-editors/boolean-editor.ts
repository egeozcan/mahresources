import { LitElement, html, css } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import { sharedStyles } from '../../styles';
import type { JSONSchema } from '../../schema-core';

@customElement('schema-boolean-editor')
export class SchemaBooleanEditor extends LitElement {
  static override styles = [sharedStyles, css`
    .grid { display: grid; grid-template-columns: 1fr 1fr; gap: 10px; }
  `];

  @property({ type: Object }) schema: JSONSchema = {};

  private _emit(field: string, value: any) {
    this.dispatchEvent(new CustomEvent('constraint-change', {
      detail: { field, value },
      bubbles: true, composed: true,
    }));
  }

  override render() {
    return html`
      <div class="type-section">
        <h4>Boolean Constraints</h4>
        <div class="grid">
          <div>
            <label for="bool-default">Default</label>
            <select id="bool-default" .value=${this.schema.default === undefined ? '' : String(this.schema.default)} @change=${(e: Event) => {
              const v = (e.target as HTMLSelectElement).value;
              this._emit('default', v === '' ? undefined : v === 'true');
            }}>
              <option value="">-- none --</option>
              <option value="true" ?selected=${this.schema.default === true}>true</option>
              <option value="false" ?selected=${this.schema.default === false}>false</option>
            </select>
          </div>
          <div>
            <label for="bool-const">Const</label>
            <select id="bool-const" .value=${this.schema.const === undefined ? '' : String(this.schema.const)} @change=${(e: Event) => {
              const v = (e.target as HTMLSelectElement).value;
              this._emit('const', v === '' ? undefined : v === 'true');
            }}>
              <option value="">-- none --</option>
              <option value="true" ?selected=${this.schema.const === true}>true</option>
              <option value="false" ?selected=${this.schema.const === false}>false</option>
            </select>
          </div>
        </div>
      </div>
    `;
  }
}
