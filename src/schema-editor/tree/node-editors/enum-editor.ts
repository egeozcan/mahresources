import { LitElement, html, css } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import { repeat } from 'lit/directives/repeat.js';
import { sharedStyles } from '../../styles';

@customElement('schema-enum-editor')
export class SchemaEnumEditor extends LitElement {
  static override styles = [sharedStyles, css`
    .enum-row { display: flex; align-items: center; gap: 6px; margin-bottom: 6px; }
    .enum-row input { flex: 1; }
    .drag { color: #9ca3af; cursor: grab; font-size: 10px; }
    .remove { color: #dc2626; background: none; border: none; font-size: 14px; padding: 0 4px; }
  `];

  @property({ type: Array }) values: any[] = [];
  @property({ type: String }) valueType = 'string';

  private _emit() {
    this.dispatchEvent(new CustomEvent('enum-change', {
      detail: { values: [...this.values] },
      bubbles: true, composed: true,
    }));
  }

  private _updateValue(index: number, raw: string) {
    const updated = [...this.values];
    if (this.valueType === 'number' || this.valueType === 'integer') {
      updated[index] = this.valueType === 'integer' ? parseInt(raw, 10) : parseFloat(raw);
    } else if (this.valueType === 'boolean') {
      updated[index] = raw === 'true';
    } else {
      updated[index] = raw;
    }
    this.values = updated;
    this._emit();
  }

  private _removeValue(index: number) {
    this.values = this.values.filter((_, i) => i !== index);
    this._emit();
    this.requestUpdate();
  }

  private _addValue() {
    let newVal: any;
    if (this.valueType === 'number' || this.valueType === 'integer') newVal = 0;
    else if (this.valueType === 'boolean') newVal = false;
    else newVal = '';
    this.values = [...this.values, newVal];
    this._emit();
    this.requestUpdate();
  }

  override render() {
    return html`
      <div class="type-section">
        <h4>Enum Values</h4>
        ${repeat(this.values, (_v, i) => i, (v, i) => html`
          <div class="enum-row">
            <span class="drag" aria-hidden="true">☰</span>
            ${this.valueType === 'boolean'
              ? html`<select
                  .value=${String(v)}
                  @change=${(e: Event) => this._updateValue(i, (e.target as HTMLSelectElement).value)}
                  aria-label="Enum value ${i + 1}"
                >
                  <option value="true" ?selected=${v === true}>true</option>
                  <option value="false" ?selected=${v === false}>false</option>
                </select>`
              : html`<input
                  .value=${String(v)}
                  type=${this.valueType === 'number' || this.valueType === 'integer' ? 'number' : 'text'}
                  step=${this.valueType === 'integer' ? '1' : 'any'}
                  @change=${(e: Event) => this._updateValue(i, (e.target as HTMLInputElement).value)}
                  aria-label="Enum value ${i + 1}"
                >`}
            <button class="remove" @click=${() => this._removeValue(i)} aria-label="Remove value ${v}">×</button>
          </div>
        `)}
        <button class="btn-ghost" @click=${this._addValue}>+ Add Value</button>
      </div>
    `;
  }
}
