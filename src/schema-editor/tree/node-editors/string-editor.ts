import { LitElement, html, css } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import { sharedStyles } from '../../styles';
import type { JSONSchema } from '../../schema-core';

const STRING_FORMATS = [
  '', 'date', 'date-time', 'time', 'email', 'uri', 'uri-reference',
  'uuid', 'hostname', 'ipv4', 'ipv6', 'regex', 'json-pointer',
];

@customElement('schema-string-editor')
export class SchemaStringEditor extends LitElement {
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
        <h4>String Constraints</h4>
        <div class="grid">
          <div>
            <label>Min Length</label>
            <input type="number" min="0" .value=${this.schema.minLength ?? ''} @change=${(e: Event) => {
              const v = (e.target as HTMLInputElement).value;
              this._emit('minLength', v ? parseInt(v) : undefined);
            }}>
          </div>
          <div>
            <label>Max Length</label>
            <input type="number" min="0" .value=${this.schema.maxLength ?? ''} @change=${(e: Event) => {
              const v = (e.target as HTMLInputElement).value;
              this._emit('maxLength', v ? parseInt(v) : undefined);
            }}>
          </div>
          <div>
            <label>Pattern (regex)</label>
            <input .value=${this.schema.pattern || ''} @change=${(e: Event) => this._emit('pattern', (e.target as HTMLInputElement).value)}>
          </div>
          <div>
            <label>Format</label>
            <select .value=${this.schema.format || ''} @change=${(e: Event) => this._emit('format', (e.target as HTMLSelectElement).value)}>
              ${STRING_FORMATS.map(f => html`<option .value=${f} ?selected=${f === (this.schema.format || '')}>${f || '(none)'}</option>`)}
            </select>
          </div>
          <div>
            <label>Default</label>
            <input .value=${this.schema.default ?? ''} @change=${(e: Event) => this._emit('default', (e.target as HTMLInputElement).value)}>
          </div>
          <div>
            <label>Const</label>
            <input .value=${this.schema.const ?? ''} @change=${(e: Event) => this._emit('const', (e.target as HTMLInputElement).value)}>
          </div>
        </div>
      </div>
    `;
  }
}
