import { LitElement, html, css } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import { sharedStyles } from '../../styles';

@customElement('schema-ref-editor')
export class SchemaRefEditor extends LitElement {
  static override styles = [sharedStyles, css``];

  @property({ type: String }) ref = '';
  @property({ type: Array }) defsNames: string[] = [];
  @property({ type: String }) defsPrefix = '$defs';

  private _emit(ref: string) {
    this.dispatchEvent(new CustomEvent('ref-change', {
      detail: { ref },
      bubbles: true, composed: true,
    }));
  }

  override render() {
    return html`
      <div class="type-section">
        <h4>$ref Target</h4>
        <label for="ref-target">Reference</label>
        ${this.defsNames.length > 0
          ? html`
            <select id="ref-target" .value=${this.ref} @change=${(e: Event) => this._emit((e.target as HTMLSelectElement).value)}>
              <option value="">-- select --</option>
              ${this.defsNames.map(name => html`<option .value=${'#/' + this.defsPrefix + '/' + name} ?selected=${this.ref === '#/' + this.defsPrefix + '/' + name}>${name}</option>`)}
            </select>
          `
          : html`<input id="ref-target" .value=${this.ref} @change=${(e: Event) => this._emit((e.target as HTMLInputElement).value)}>`
        }
      </div>
    `;
  }
}
