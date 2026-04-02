import { LitElement, html, css } from 'lit';
import { customElement, property, state } from 'lit/decorators.js';
import { sharedStyles } from '../../styles';
import type { JSONSchema } from '../../schema-core';

const ITEM_TYPES = ['string', 'number', 'integer', 'boolean', 'object', 'array'];

@customElement('schema-array-editor')
export class SchemaArrayEditor extends LitElement {
  static override styles = [sharedStyles, css`
    .grid { display: grid; grid-template-columns: 1fr 1fr; gap: 10px; }
    .prefix-row { display: flex; align-items: center; gap: 6px; margin-bottom: 4px; }
    .prefix-row select { flex: 1; }
    .prefix-row button { padding: 2px 8px; font-size: 12px; }
    .subsection { margin-top: 14px; }
    .subsection h5 { font-size: 12px; font-weight: 600; color: #374151; margin: 0 0 6px 0; }
    .add-btn { margin-top: 4px; font-size: 12px; padding: 2px 10px; }
  `];

  @property({ type: Object }) schema: JSONSchema = {};

  @state() private _prefixItems: Array<{ type: string }> = [];

  override updated(changed: Map<string, unknown>) {
    if (changed.has('schema')) {
      const pi = this.schema.prefixItems;
      if (Array.isArray(pi)) {
        this._prefixItems = pi.map((s: JSONSchema) => ({ type: s?.type || 'string' }));
      } else {
        this._prefixItems = [];
      }
    }
  }

  private _emit(field: string, value: any) {
    this.dispatchEvent(new CustomEvent('constraint-change', {
      detail: { field, value: value === '' ? undefined : value },
      bubbles: true, composed: true,
    }));
  }

  private _emitPrefixItems() {
    const value = this._prefixItems.length > 0
      ? this._prefixItems.map(row => ({ type: row.type }))
      : undefined;
    this._emit('prefixItems', value);
  }

  private _addPrefixItem() {
    this._prefixItems = [...this._prefixItems, { type: 'string' }];
    this._emitPrefixItems();
  }

  private _removePrefixItem(idx: number) {
    this._prefixItems = this._prefixItems.filter((_, i) => i !== idx);
    this._emitPrefixItems();
  }

  private _updatePrefixItemType(idx: number, type: string) {
    this._prefixItems = this._prefixItems.map((row, i) => i === idx ? { type } : row);
    this._emitPrefixItems();
  }

  override render() {
    const itemsType = (this.schema.items && typeof this.schema.items === 'object' && !Array.isArray(this.schema.items))
      ? (this.schema.items.type || '')
      : '';
    const containsType = (this.schema.contains && typeof this.schema.contains === 'object')
      ? (this.schema.contains.type || '')
      : '';

    return html`
      <div class="type-section">
        <h4>Array Constraints</h4>
        <div class="grid">
          <div>
            <label for="arr-min-items">Min Items</label>
            <input id="arr-min-items" type="number" min="0" .value=${this.schema.minItems ?? ''} @change=${(e: Event) => {
              const v = (e.target as HTMLInputElement).value;
              this._emit('minItems', v !== '' ? parseInt(v, 10) : undefined);
            }}>
          </div>
          <div>
            <label for="arr-max-items">Max Items</label>
            <input id="arr-max-items" type="number" min="0" .value=${this.schema.maxItems ?? ''} @change=${(e: Event) => {
              const v = (e.target as HTMLInputElement).value;
              this._emit('maxItems', v !== '' ? parseInt(v, 10) : undefined);
            }}>
          </div>
          <div>
            <label><input type="checkbox" ?checked=${this.schema.uniqueItems} @change=${(e: Event) => this._emit('uniqueItems', (e.target as HTMLInputElement).checked || undefined)}> Unique Items</label>
          </div>
        </div>

        <div class="subsection">
          <h5><label for="arr-items-type">Items Type</label></h5>
          <select id="arr-items-type" .value=${itemsType} @change=${(e: Event) => {
            const v = (e.target as HTMLSelectElement).value;
            this._emit('items', v ? { type: v } : undefined);
          }}>
            <option value="">-- any --</option>
            ${ITEM_TYPES.map(t => html`<option .value=${t} ?selected=${itemsType === t}>${t}</option>`)}
          </select>
        </div>

        <div class="subsection">
          <h5><label for="arr-contains-type">Contains</label></h5>
          <select id="arr-contains-type" .value=${containsType} @change=${(e: Event) => {
            const v = (e.target as HTMLSelectElement).value;
            this._emit('contains', v ? { type: v } : undefined);
          }}>
            <option value="">-- none --</option>
            ${ITEM_TYPES.map(t => html`<option .value=${t} ?selected=${containsType === t}>${t}</option>`)}
          </select>
        </div>

        <div class="subsection">
          <h5>Prefix Items</h5>
          ${this._prefixItems.map((row, idx) => html`
            <div class="prefix-row">
              <label>
                <span class="sr-only">Prefix item ${idx + 1} type</span>
                <select aria-label="Prefix item ${idx + 1} type" .value=${row.type} @change=${(e: Event) => this._updatePrefixItemType(idx, (e.target as HTMLSelectElement).value)}>
                  ${ITEM_TYPES.map(t => html`<option .value=${t} ?selected=${row.type === t}>${t}</option>`)}
                </select>
              </label>
              <button type="button" @click=${() => this._removePrefixItem(idx)} aria-label="Remove prefix item ${idx + 1}">-</button>
            </div>
          `)}
          <button type="button" class="add-btn" @click=${this._addPrefixItem}>+ Add Prefix Item</button>
        </div>
      </div>
    `;
  }
}
