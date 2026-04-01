import { LitElement, html, css } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import { sharedStyles } from '../../styles';
import type { JSONSchema } from '../../schema-core';

@customElement('schema-object-editor')
export class SchemaObjectEditor extends LitElement {
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
    const addlProps = this.schema.additionalProperties;
    const addlValue = addlProps === false ? 'false' : addlProps === true ? 'true' : '';

    return html`
      <div class="type-section">
        <h4>Object Constraints</h4>
        <div class="grid">
          <div>
            <label for="obj-addl-props">Additional Properties</label>
            <select id="obj-addl-props" .value=${addlValue} @change=${(e: Event) => {
              const v = (e.target as HTMLSelectElement).value;
              this._emit('additionalProperties', v === '' ? undefined : v === 'true');
            }}>
              <option value="">-- default (true) --</option>
              <option value="true" ?selected=${addlValue === 'true'}>Allowed</option>
              <option value="false" ?selected=${addlValue === 'false'}>Forbidden</option>
            </select>
          </div>
          <div>
            <label for="obj-min-props">Min Properties</label>
            <input id="obj-min-props" type="number" min="0" .value=${this.schema.minProperties ?? ''} @change=${(e: Event) => {
              const v = (e.target as HTMLInputElement).value;
              this._emit('minProperties', v ? parseInt(v) : undefined);
            }}>
          </div>
          <div>
            <label for="obj-max-props">Max Properties</label>
            <input id="obj-max-props" type="number" min="0" .value=${this.schema.maxProperties ?? ''} @change=${(e: Event) => {
              const v = (e.target as HTMLInputElement).value;
              this._emit('maxProperties', v ? parseInt(v) : undefined);
            }}>
          </div>
        </div>
      </div>
    `;
  }
}
