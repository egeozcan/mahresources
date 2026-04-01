import { LitElement, html, css } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import { sharedStyles } from '../../styles';
import type { JSONSchema } from '../../schema-core';

@customElement('schema-number-editor')
export class SchemaNumberEditor extends LitElement {
  static override styles = [sharedStyles, css`
    .grid { display: grid; grid-template-columns: 1fr 1fr; gap: 10px; }
  `];

  @property({ type: Object }) schema: JSONSchema = {};
  @property({ type: Boolean }) integerOnly = false;

  private _emit(field: string, value: any) {
    this.dispatchEvent(new CustomEvent('constraint-change', {
      detail: { field, value: value === '' ? undefined : value },
      bubbles: true, composed: true,
    }));
  }

  private _parseNum(val: string): number | undefined {
    if (val === '') return undefined;
    return this.integerOnly ? parseInt(val) : parseFloat(val);
  }

  override render() {
    const step = this.integerOnly ? '1' : 'any';
    return html`
      <div class="type-section">
        <h4>${this.integerOnly ? 'Integer' : 'Number'} Constraints</h4>
        <div class="grid">
          <div>
            <label>Minimum</label>
            <input type="number" step=${step} .value=${this.schema.minimum ?? ''} @change=${(e: Event) => this._emit('minimum', this._parseNum((e.target as HTMLInputElement).value))}>
          </div>
          <div>
            <label>Maximum</label>
            <input type="number" step=${step} .value=${this.schema.maximum ?? ''} @change=${(e: Event) => this._emit('maximum', this._parseNum((e.target as HTMLInputElement).value))}>
          </div>
          <div>
            <label>Exclusive Minimum</label>
            <input type="number" step=${step} .value=${this.schema.exclusiveMinimum ?? ''} @change=${(e: Event) => this._emit('exclusiveMinimum', this._parseNum((e.target as HTMLInputElement).value))}>
          </div>
          <div>
            <label>Exclusive Maximum</label>
            <input type="number" step=${step} .value=${this.schema.exclusiveMaximum ?? ''} @change=${(e: Event) => this._emit('exclusiveMaximum', this._parseNum((e.target as HTMLInputElement).value))}>
          </div>
          <div>
            <label>Multiple Of</label>
            <input type="number" step=${step} .value=${this.schema.multipleOf ?? ''} @change=${(e: Event) => this._emit('multipleOf', this._parseNum((e.target as HTMLInputElement).value))}>
          </div>
          <div>
            <label>Default</label>
            <input type="number" step=${step} .value=${this.schema.default ?? ''} @change=${(e: Event) => this._emit('default', this._parseNum((e.target as HTMLInputElement).value))}>
          </div>
        </div>
      </div>
    `;
  }
}
