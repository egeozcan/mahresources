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
            <label for="num-minimum">Minimum</label>
            <input id="num-minimum" type="number" step=${step} .value=${this.schema.minimum ?? ''} @change=${(e: Event) => this._emit('minimum', this._parseNum((e.target as HTMLInputElement).value))}>
          </div>
          <div>
            <label for="num-maximum">Maximum</label>
            <input id="num-maximum" type="number" step=${step} .value=${this.schema.maximum ?? ''} @change=${(e: Event) => this._emit('maximum', this._parseNum((e.target as HTMLInputElement).value))}>
          </div>
          <div>
            <label for="num-exclusive-minimum">Exclusive Minimum</label>
            <input id="num-exclusive-minimum" type="number" step=${step} .value=${this.schema.exclusiveMinimum ?? ''} @change=${(e: Event) => this._emit('exclusiveMinimum', this._parseNum((e.target as HTMLInputElement).value))}>
          </div>
          <div>
            <label for="num-exclusive-maximum">Exclusive Maximum</label>
            <input id="num-exclusive-maximum" type="number" step=${step} .value=${this.schema.exclusiveMaximum ?? ''} @change=${(e: Event) => this._emit('exclusiveMaximum', this._parseNum((e.target as HTMLInputElement).value))}>
          </div>
          <div>
            <label for="num-multiple-of">Multiple Of</label>
            <input id="num-multiple-of" type="number" step=${step} .value=${this.schema.multipleOf ?? ''} @change=${(e: Event) => this._emit('multipleOf', this._parseNum((e.target as HTMLInputElement).value))}>
          </div>
          <div>
            <label for="num-default">Default</label>
            <input id="num-default" type="number" step=${step} .value=${this.schema.default ?? ''} @change=${(e: Event) => this._emit('default', this._parseNum((e.target as HTMLInputElement).value))}>
          </div>
          <div>
            <label for="num-const">Const</label>
            <input id="num-const" type="number" step=${step} .value=${this.schema.const ?? ''} @change=${(e: Event) => this._emit('const', this._parseNum((e.target as HTMLInputElement).value))}>
          </div>
        </div>
      </div>
    `;
  }
}
