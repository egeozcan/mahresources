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
            <label for="str-min-length">Min Length</label>
            <input id="str-min-length" type="number" min="0" .value=${this.schema.minLength ?? ''} @change=${(e: Event) => {
              const v = (e.target as HTMLInputElement).value;
              this._emit('minLength', v !== '' ? parseInt(v, 10) : undefined);
            }}>
          </div>
          <div>
            <label for="str-max-length">Max Length</label>
            <input id="str-max-length" type="number" min="0" .value=${this.schema.maxLength ?? ''} @change=${(e: Event) => {
              const v = (e.target as HTMLInputElement).value;
              this._emit('maxLength', v !== '' ? parseInt(v, 10) : undefined);
            }}>
          </div>
          <div>
            <label for="str-pattern">Pattern (regex)</label>
            <input id="str-pattern" .value=${this.schema.pattern || ''} @change=${(e: Event) => this._emit('pattern', (e.target as HTMLInputElement).value)}>
          </div>
          <div>
            <label for="str-format">Format</label>
            <select id="str-format" .value=${this.schema.format || ''} @change=${(e: Event) => this._emit('format', (e.target as HTMLSelectElement).value)}>
              ${STRING_FORMATS.map(f => html`<option .value=${f} ?selected=${f === (this.schema.format || '')}>${f || '(none)'}</option>`)}
            </select>
          </div>
          <div>
            <label for="str-default">Default</label>
            <input id="str-default" .value=${this.schema.default ?? ''} @change=${(e: Event) => this._emit('default', (e.target as HTMLInputElement).value)}>
          </div>
          <div>
            <label for="str-const">Const</label>
            <input id="str-const" .value=${this.schema.const ?? ''} @change=${(e: Event) => this._emit('const', (e.target as HTMLInputElement).value)}>
          </div>
        </div>
      </div>
    `;
  }
}
