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
    if (this.valueType === 'number' || this.valueType === 'integer') {
      this.values[index] = this.valueType === 'integer' ? parseInt(raw) : parseFloat(raw);
    } else {
      this.values[index] = raw;
    }
    this._emit();
  }

  private _removeValue(index: number) {
    this.values.splice(index, 1);
    this._emit();
    this.requestUpdate();
  }

  private _addValue() {
    this.values.push(this.valueType === 'number' || this.valueType === 'integer' ? 0 : '');
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
            <input
              .value=${String(v)}
              type=${this.valueType === 'number' || this.valueType === 'integer' ? 'number' : 'text'}
              step=${this.valueType === 'integer' ? '1' : 'any'}
              @change=${(e: Event) => this._updateValue(i, (e.target as HTMLInputElement).value)}
              aria-label="Enum value ${i + 1}"
            >
            <button class="remove" @click=${() => this._removeValue(i)} aria-label="Remove value ${v}">×</button>
          </div>
        `)}
        <button class="btn-ghost" @click=${this._addValue}>+ Add Value</button>
      </div>
    `;
  }
}
