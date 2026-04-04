import { LitElement, html, css } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import { repeat } from 'lit/directives/repeat.js';
import { sharedStyles } from '../../styles';

export interface EnumEntry {
  value: any;
  label: string;
}

@customElement('schema-enum-editor')
export class SchemaEnumEditor extends LitElement {
  static override styles = [sharedStyles, css`
    .enum-row { display: flex; align-items: center; gap: 6px; margin-bottom: 6px; }
    .enum-row input { flex: 1; }
    .drag { color: #9ca3af; cursor: grab; font-size: 10px; }
    .remove { color: #dc2626; background: none; border: none; font-size: 14px; padding: 0 4px; }
    .label-header { display: grid; grid-template-columns: 20px 1fr 1fr 24px; gap: 6px; margin-bottom: 4px; font-size: 11px; color: #6b7280; font-weight: 600; }
    .labeled-row { display: grid; grid-template-columns: 20px 1fr 1fr 24px; gap: 6px; margin-bottom: 6px; align-items: center; }
    .convert-btn { font-size: 12px; color: #4f46e5; background: none; border: 1px solid #c7d2fe; border-radius: 4px; padding: 4px 10px; cursor: pointer; margin-bottom: 8px; }
    .convert-btn:hover { background: #eef2ff; }
  `];

  /** Plain enum values (when no labels) */
  @property({ type: Array }) values: any[] = [];
  /** Labeled entries (when labels are present) */
  @property({ type: Array }) entries: EnumEntry[] = [];
  @property({ type: String }) valueType = 'string';
  /** Whether the editor is in labeled mode */
  @property({ type: Boolean }) labeled = false;

  /** Index of the row currently being dragged */
  private _dragIndex = -1;

  private _onDragStart(index: number, e: DragEvent) {
    this._dragIndex = index;
    e.dataTransfer!.effectAllowed = 'move';
    (e.target as HTMLElement).closest('.enum-row, .labeled-row')?.classList.add('dragging');
  }

  private _onDragOver(e: DragEvent) {
    e.preventDefault();
    e.dataTransfer!.dropEffect = 'move';
  }

  private _onDrop(targetIndex: number, e: DragEvent) {
    e.preventDefault();
    if (this._dragIndex < 0 || this._dragIndex === targetIndex) return;

    if (this.labeled) {
      const updated = [...this.entries];
      const [moved] = updated.splice(this._dragIndex, 1);
      updated.splice(targetIndex, 0, moved);
      this.entries = updated;
    } else {
      const updated = [...this.values];
      const [moved] = updated.splice(this._dragIndex, 1);
      updated.splice(targetIndex, 0, moved);
      this.values = updated;
    }

    this._dragIndex = -1;
    this._emit();
    this.requestUpdate();
  }

  private _onDragEnd(e: DragEvent) {
    this._dragIndex = -1;
    (e.target as HTMLElement).closest('.enum-row, .labeled-row')?.classList.remove('dragging');
  }

  private _emit() {
    if (this.labeled) {
      this.dispatchEvent(new CustomEvent('enum-change', {
        detail: { labeled: true, entries: [...this.entries] },
        bubbles: true, composed: true,
      }));
    } else {
      this.dispatchEvent(new CustomEvent('enum-change', {
        detail: { labeled: false, values: [...this.values] },
        bubbles: true, composed: true,
      }));
    }
  }

  private _parseValue(raw: string): any {
    if (this.valueType === 'number' || this.valueType === 'integer') {
      return this.valueType === 'integer' ? parseInt(raw, 10) : parseFloat(raw);
    }
    if (this.valueType === 'boolean') return raw === 'true';
    return raw;
  }

  private _defaultValue(): any {
    if (this.valueType === 'number' || this.valueType === 'integer') return 0;
    if (this.valueType === 'boolean') return false;
    return '';
  }

  // --- Plain enum (no labels) ------------------------------------------------

  private _updateValue(index: number, raw: string) {
    const updated = [...this.values];
    updated[index] = this._parseValue(raw);
    this.values = updated;
    this._emit();
  }

  private _removeValue(index: number) {
    this.values = this.values.filter((_, i) => i !== index);
    this._emit();
    this.requestUpdate();
  }

  private _addValue() {
    this.values = [...this.values, this._defaultValue()];
    this._emit();
    this.requestUpdate();
  }

  // --- Labeled enum ----------------------------------------------------------

  private _updateEntryValue(index: number, raw: string) {
    const updated = [...this.entries];
    updated[index] = { ...updated[index], value: this._parseValue(raw) };
    this.entries = updated;
    this._emit();
  }

  private _updateEntryLabel(index: number, label: string) {
    const updated = [...this.entries];
    updated[index] = { ...updated[index], label };
    this.entries = updated;
    this._emit();
  }

  private _removeEntry(index: number) {
    this.entries = this.entries.filter((_, i) => i !== index);
    if (this.entries.length === 0) {
      this.labeled = false;
      this.values = [];
    }
    this._emit();
    this.requestUpdate();
  }

  private _addEntry() {
    this.entries = [...this.entries, { value: this._defaultValue(), label: '' }];
    this._emit();
    this.requestUpdate();
  }

  // --- Conversion ------------------------------------------------------------

  private _convertToLabeled() {
    this.entries = this.values.map(v => ({ value: v, label: '' }));
    this.labeled = true;
    this.values = [];
    this._emit();
    this.requestUpdate();
  }

  private _convertToPlain() {
    this.values = this.entries.map(e => e.value);
    this.labeled = false;
    this.entries = [];
    this._emit();
    this.requestUpdate();
  }

  // --- Render ----------------------------------------------------------------

  override render() {
    if (this.labeled) return this._renderLabeled();
    return this._renderPlain();
  }

  private _renderPlain() {
    return html`
      <div class="type-section">
        <h4>Enum Values</h4>
        <button class="convert-btn" @click=${this._convertToLabeled} title="Convert to labeled enum with display names">+ Add Labels</button>
        ${repeat(this.values, (_v, i) => i, (v, i) => html`
          <div class="enum-row" draggable="true"
            @dragstart=${(e: DragEvent) => this._onDragStart(i, e)}
            @dragover=${this._onDragOver}
            @drop=${(e: DragEvent) => this._onDrop(i, e)}
            @dragend=${this._onDragEnd}>
            <span class="drag" aria-hidden="true">\u2630</span>
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
            <button class="remove" @click=${() => this._removeValue(i)} aria-label="Remove value ${v}">\u00d7</button>
          </div>
        `)}
        <button class="btn-ghost" @click=${this._addValue}>+ Add Value</button>
      </div>
    `;
  }

  private _renderLabeled() {
    return html`
      <div class="type-section">
        <h4>Enum Values</h4>
        <button class="convert-btn" @click=${this._convertToPlain} title="Remove labels and convert back to plain enum">Remove Labels</button>
        <div class="label-header">
          <span></span>
          <span>Value</span>
          <span>Label</span>
          <span></span>
        </div>
        ${repeat(this.entries, (_e, i) => i, (entry, i) => html`
          <div class="labeled-row" draggable="true"
            @dragstart=${(e: DragEvent) => this._onDragStart(i, e)}
            @dragover=${this._onDragOver}
            @drop=${(e: DragEvent) => this._onDrop(i, e)}
            @dragend=${this._onDragEnd}>
            <span class="drag" aria-hidden="true">\u2630</span>
            ${this.valueType === 'boolean'
              ? html`<select
                  .value=${String(entry.value)}
                  @change=${(e: Event) => this._updateEntryValue(i, (e.target as HTMLSelectElement).value)}
                  aria-label="Enum value ${i + 1}"
                >
                  <option value="true" ?selected=${entry.value === true}>true</option>
                  <option value="false" ?selected=${entry.value === false}>false</option>
                </select>`
              : html`<input
                  .value=${String(entry.value)}
                  type=${this.valueType === 'number' || this.valueType === 'integer' ? 'number' : 'text'}
                  step=${this.valueType === 'integer' ? '1' : 'any'}
                  @change=${(e: Event) => this._updateEntryValue(i, (e.target as HTMLInputElement).value)}
                  aria-label="Enum value ${i + 1}"
                >`}
            <input
              .value=${entry.label}
              type="text"
              placeholder="Display label"
              @change=${(e: Event) => this._updateEntryLabel(i, (e.target as HTMLInputElement).value)}
              aria-label="Label for value ${entry.value}">
            <button class="remove" @click=${() => this._removeEntry(i)} aria-label="Remove value ${entry.value}">\u00d7</button>
          </div>
        `)}
        <button class="btn-ghost" @click=${this._addEntry}>+ Add Value</button>
      </div>
    `;
  }
}
