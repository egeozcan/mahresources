import { LitElement, html, css } from 'lit';
import { customElement, property, state } from 'lit/decorators.js';
import { sharedStyles } from '../../styles';
import type { JSONSchema } from '../../schema-core';

const PROP_TYPES = ['string', 'number', 'integer', 'boolean', 'object', 'array'];

@customElement('schema-object-editor')
export class SchemaObjectEditor extends LitElement {
  static override styles = [sharedStyles, css`
    .grid { display: grid; grid-template-columns: 1fr 1fr; gap: 10px; }
    .pattern-row { display: flex; align-items: center; gap: 6px; margin-bottom: 4px; }
    .pattern-row input { flex: 1; min-width: 0; }
    .pattern-row select { flex: 0 0 100px; }
    .pattern-row button { padding: 2px 8px; font-size: 12px; flex-shrink: 0; }
    .subsection { margin-top: 14px; }
    .subsection h5 { font-size: 12px; font-weight: 600; color: #374151; margin: 0 0 6px 0; }
    .add-btn { margin-top: 4px; font-size: 12px; padding: 2px 10px; }
  `];

  @property({ type: Object }) schema: JSONSchema = {};

  @state() private _patternProps: Array<{ pattern: string; type: string }> = [];

  override updated(changed: Map<string, unknown>) {
    if (changed.has('schema')) {
      const pp = this.schema.patternProperties;
      if (pp && typeof pp === 'object') {
        this._patternProps = Object.entries(pp).map(([pattern, sub]: [string, any]) => ({
          pattern,
          type: sub?.type || 'string',
        }));
      } else {
        this._patternProps = [];
      }
    }
  }

  private _emit(field: string, value: any) {
    this.dispatchEvent(new CustomEvent('constraint-change', {
      detail: { field, value: value === '' ? undefined : value },
      bubbles: true, composed: true,
    }));
  }

  private _emitPatternProps() {
    if (this._patternProps.length === 0) {
      this._emit('patternProperties', undefined);
      return;
    }
    const obj: Record<string, JSONSchema> = {};
    for (const row of this._patternProps) {
      if (row.pattern) {
        obj[row.pattern] = { type: row.type };
      }
    }
    this._emit('patternProperties', Object.keys(obj).length > 0 ? obj : undefined);
  }

  private _addPatternProp() {
    this._patternProps = [...this._patternProps, { pattern: '', type: 'string' }];
    this._emitPatternProps();
  }

  private _removePatternProp(idx: number) {
    this._patternProps = this._patternProps.filter((_, i) => i !== idx);
    this._emitPatternProps();
  }

  private _updatePatternPropPattern(idx: number, pattern: string) {
    this._patternProps = this._patternProps.map((row, i) => i === idx ? { ...row, pattern } : row);
    this._emitPatternProps();
  }

  private _updatePatternPropType(idx: number, type: string) {
    this._patternProps = this._patternProps.map((row, i) => i === idx ? { ...row, type } : row);
    this._emitPatternProps();
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
              this._emit('minProperties', v !== '' ? parseInt(v, 10) : undefined);
            }}>
          </div>
          <div>
            <label for="obj-max-props">Max Properties</label>
            <input id="obj-max-props" type="number" min="0" .value=${this.schema.maxProperties ?? ''} @change=${(e: Event) => {
              const v = (e.target as HTMLInputElement).value;
              this._emit('maxProperties', v !== '' ? parseInt(v, 10) : undefined);
            }}>
          </div>
        </div>

        <div class="subsection">
          <h5>Pattern Properties</h5>
          ${this._patternProps.map((row, idx) => html`
            <div class="pattern-row">
              <input
                type="text"
                .value=${row.pattern}
                placeholder="regex pattern"
                aria-label="Pattern property ${idx + 1} regex"
                @change=${(e: Event) => this._updatePatternPropPattern(idx, (e.target as HTMLInputElement).value)}
              >
              <select
                aria-label="Pattern property ${idx + 1} type"
                .value=${row.type}
                @change=${(e: Event) => this._updatePatternPropType(idx, (e.target as HTMLSelectElement).value)}
              >
                ${PROP_TYPES.map(t => html`<option .value=${t} ?selected=${row.type === t}>${t}</option>`)}
              </select>
              <button type="button" @click=${() => this._removePatternProp(idx)} aria-label="Remove pattern property ${idx + 1}">-</button>
            </div>
          `)}
          <button type="button" class="add-btn" @click=${this._addPatternProp}>+ Add Pattern Property</button>
        </div>
      </div>
    `;
  }
}
