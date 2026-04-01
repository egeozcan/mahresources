import { LitElement, html, nothing, type TemplateResult } from 'lit';
import { customElement, property, state } from 'lit/decorators.js';
import type { FlatField } from '../schema-core';
import { flattenSchema, intersectFields } from '../schema-core';
import { generateParamNameForMeta } from '../../components/freeFields.js';

// ─── Types ──────────────────────────────────────────────────────────────────

interface Operator {
  code: string;
  label: string;
}

interface HiddenInput {
  value: string;
}

interface ExistingValue {
  operator: string;
  value: string;
  enumValues: string[];
  boolValue: string;
}

interface SearchField extends FlatField {
  operator: string;
  value: string;
  enumValues: string[];
  boolValue: string;
  showOperator: boolean;
  operators: Operator[] | null;
}

interface MetaQueryEntry {
  name: string;
  value: any;
  operation?: string;
}

// ─── Helpers ────────────────────────────────────────────────────────────────

function defaultOperator(field: FlatField): string {
  if (field.enum) return 'EQ';
  if (field.type === 'boolean') return 'EQ';
  if (field.type === 'string') return 'LI';
  return 'EQ';
}

function operatorsForType(field: FlatField): Operator[] | null {
  if (field.enum || field.type === 'boolean') return null;
  if (field.type === 'string') {
    return [
      { code: 'LI', label: 'LIKE' },
      { code: 'EQ', label: '=' },
      { code: 'NE', label: '\u2260' },
      { code: 'NL', label: 'NOT LIKE' },
    ];
  }
  return [
    { code: 'EQ', label: '=' },
    { code: 'NE', label: '\u2260' },
    { code: 'GT', label: '>' },
    { code: 'GE', label: '\u2265' },
    { code: 'LT', label: '<' },
    { code: 'LE', label: '\u2264' },
  ];
}

const OPERATOR_SYMBOLS: Record<string, string> = {
  EQ: '=', NE: '\u2260', LI: '\u2248', NL: '\u2249',
  GT: '>', GE: '\u2265', LT: '<', LE: '\u2264',
};

function operatorSymbol(code: string): string {
  return OPERATOR_SYMBOLS[code] || code;
}

// ─── Component ──────────────────────────────────────────────────────────────

@customElement('schema-search-mode')
export class SchemaSearchMode extends LitElement {
  @property({ type: String }) schema = '';
  @property({ type: String, attribute: 'meta-query' }) metaQuery = '[]';
  @property({ type: String, attribute: 'field-name' }) fieldName = 'MetaQuery';

  @state() private _fields: SearchField[] = [];
  @state() private _hasFields = false;
  @state() private _fieldsCleared = false;

  private _existingMeta: MetaQueryEntry[] = [];

  // Light DOM for form submission and Tailwind inheritance
  override createRenderRoot() { return this; }

  // ─── Lifecycle ──────────────────────────────────────────────────────────

  override willUpdate(changed: Map<string, unknown>) {
    if (changed.has('schema')) {
      this._rebuildFields();
    }
    if (changed.has('metaQuery')) {
      this._parseMetaQuery();
    }
  }

  private _parseMetaQuery() {
    try {
      const parsed = JSON.parse(this.metaQuery);
      this._existingMeta = Array.isArray(parsed) ? parsed : [];
    } catch {
      this._existingMeta = [];
    }
  }

  private _rebuildFields() {
    const hadFields = this._hasFields;

    // Snapshot current field values so they survive schema changes
    const currentValues = new Map<string, {
      value: string;
      operator: string;
      enumValues: string[];
      boolValue: string;
    }>();
    for (const f of this._fields) {
      currentValues.set(f.path, {
        value: f.value,
        operator: f.operator,
        enumValues: f.enumValues,
        boolValue: f.boolValue,
      });
    }

    if (!this.schema) {
      this._clearFields(hadFields);
      return;
    }

    // Parse schema attribute — may be a single JSON Schema string or an array of schema strings
    let schemas: any[];
    try {
      const parsed = JSON.parse(this.schema);
      if (Array.isArray(parsed)) {
        // Array of schema strings — parse each one
        schemas = [];
        for (const item of parsed) {
          if (typeof item === 'string') {
            try {
              schemas.push(JSON.parse(item));
            } catch {
              this._clearFields(hadFields);
              return;
            }
          } else if (typeof item === 'object' && item !== null) {
            schemas.push(item);
          } else {
            this._clearFields(hadFields);
            return;
          }
        }
      } else if (typeof parsed === 'object' && parsed !== null) {
        schemas = [parsed];
      } else {
        this._clearFields(hadFields);
        return;
      }
    } catch {
      this._clearFields(hadFields);
      return;
    }

    if (schemas.length === 0) {
      this._clearFields(hadFields);
      return;
    }

    const fieldLists = schemas.map(s => flattenSchema(s));
    const merged = schemas.length === 1 ? fieldLists[0] : intersectFields(fieldLists);

    // Track paths we can't represent (e.g., range queries on non-enum fields)
    const unclaimable = new Set<string>();

    // Make sure _existingMeta is populated before building fields
    if (this._existingMeta.length === 0) {
      this._parseMetaQuery();
    }

    this._fields = merged.map(field => {
      const op = defaultOperator(field);
      // Prefer in-progress values (from current session), fall back to URL state
      const current = currentValues.get(field.path);
      let existing: ExistingValue | null = current
        ? { ...current }
        : this._findExistingValue(field.path);

      // Non-enum fields with multiple URL matches can't be represented in a single input.
      // Leave them for freeFields.
      if (!field.enum && existing && existing.enumValues.length > 0) {
        existing = null;
        unclaimable.add(field.path);
      }

      // Enum and boolean schema fields only support EQ semantics.
      // If any URL entry for this path uses a non-EQ operator, leave it for freeFields.
      if (!current && (field.enum || field.type === 'boolean')) {
        const rawMatches = this._existingMeta.filter(m => m.name === field.path);
        if (rawMatches.some(m => m.operation && m.operation !== 'EQ')) {
          existing = null;
          unclaimable.add(field.path);
        }
      }

      let enumValues = existing ? existing.enumValues : [];
      // For enum fields with a single existing value, route it into enumValues
      // so the checkbox/select UI shows it as checked.
      if (field.enum && existing && enumValues.length === 0 && existing.value) {
        enumValues = [existing.value];
      }

      return {
        ...field,
        operator: existing ? existing.operator : op,
        value: (existing && !field.enum) ? existing.value : '',
        enumValues,
        boolValue: existing ? existing.boolValue : 'any',
        showOperator: false,
        operators: operatorsForType(field),
      };
    });

    this._hasFields = this._fields.length > 0;
    this._fieldsCleared = hadFields && !this._hasFields;

    // Notify freeFields which paths are claimed by schema fields
    const claimedPaths = this._fields
      .filter(f => !unclaimable.has(f.path))
      .map(f => f.path);
    window.dispatchEvent(new CustomEvent('schema-fields-claimed', {
      detail: { paths: claimedPaths },
    }));
  }

  private _clearFields(hadFields: boolean) {
    this._fields = [];
    this._hasFields = false;
    this._fieldsCleared = hadFields;
    // Release all claimed paths so freeFields can restore them.
    window.dispatchEvent(new CustomEvent('schema-fields-claimed', {
      detail: { paths: [] },
    }));
  }

  private _findExistingValue(path: string): ExistingValue | null {
    const matches = this._existingMeta.filter(m => m.name === path);
    if (matches.length === 0) return null;

    if (matches.length > 1) {
      return {
        operator: matches[0].operation || 'EQ',
        value: '',
        enumValues: matches.map(m => String(m.value)),
        boolValue: 'any',
      };
    }

    const m = matches[0];
    if (typeof m.value === 'boolean') {
      return {
        operator: 'EQ',
        value: '',
        enumValues: [],
        boolValue: String(m.value),
      };
    }

    return {
      operator: m.operation || 'EQ',
      value: m.value != null ? String(m.value) : '',
      enumValues: [],
      boolValue: 'any',
    };
  }

  // ─── Hidden input generation ────────────────────────────────────────────

  private _getHiddenInputs(field: SearchField): HiddenInput[] {
    if (field.type === 'boolean') {
      if (field.boolValue === 'any') return [];
      return [{ value: generateParamNameForMeta({ name: field.path, value: field.boolValue, operation: 'EQ' }) }];
    }

    if (field.enum) {
      // String enums must be quoted so coercible values like "007", "true", "null"
      // are preserved as strings. Numeric enums must NOT be quoted.
      const quote = field.type === 'string';
      return field.enumValues.map(v => ({
        value: quote
          ? `${field.path}:EQ:"${v}"`
          : `${field.path}:EQ:${v}`,
      }));
    }

    if (!field.value && field.value !== '0') return [];

    // For string-typed schema fields, always quote the value so the backend
    // treats it as a string.
    if (field.type === 'string') {
      return [{ value: `${field.path}:${field.operator}:"${field.value}"` }];
    }

    return [{ value: generateParamNameForMeta({ name: field.path, value: field.value, operation: field.operator }) }];
  }

  // ─── Event handlers ─────────────────────────────────────────────────────

  private _toggleOperator(field: SearchField) {
    field.showOperator = !field.showOperator;
    this.requestUpdate();

    // Focus the appropriate element after render
    this.updateComplete.then(() => {
      const wrapper = this.querySelector(`[data-field-path="${field.path}"]`);
      if (!wrapper) return;
      const target = field.showOperator
        ? wrapper.querySelector('select')
        : wrapper.querySelector('button[data-operator-toggle]');
      if (target) (target as HTMLElement).focus();
    });
  }

  private _selectOperator(field: SearchField, e: Event) {
    field.operator = (e.target as HTMLSelectElement).value;
    field.showOperator = false;
    this.requestUpdate();

    this.updateComplete.then(() => {
      const wrapper = this.querySelector(`[data-field-path="${field.path}"]`);
      if (!wrapper) return;
      const btn = wrapper.querySelector('button[data-operator-toggle]');
      if (btn) (btn as HTMLElement).focus();
    });
  }

  private _onFieldValueInput(field: SearchField, e: Event) {
    field.value = (e.target as HTMLInputElement).value;
    this.requestUpdate();
  }

  private _onBoolChange(field: SearchField, value: string) {
    field.boolValue = value;
    this.requestUpdate();
  }

  private _onEnumCheckboxChange(field: SearchField, enumVal: string, checked: boolean) {
    if (checked) {
      if (!field.enumValues.includes(String(enumVal))) {
        field.enumValues = [...field.enumValues, String(enumVal)];
      }
    } else {
      field.enumValues = field.enumValues.filter(v => v !== String(enumVal));
    }
    this.requestUpdate();
  }

  private _onEnumSelectChange(field: SearchField, e: Event) {
    const select = e.target as HTMLSelectElement;
    field.enumValues = Array.from(select.selectedOptions).map(o => o.value);
    this.requestUpdate();
  }

  // ─── Render ─────────────────────────────────────────────────────────────

  override render() {
    const srText = this._hasFields
      ? `${this._fields.length} schema filter fields available`
      : (this._fieldsCleared ? 'Schema filter fields cleared' : '');

    return html`
      <div class="w-full" role="group" aria-label="Schema fields">
        <span class="sr-only" aria-live="polite" aria-atomic="true">${srText}</span>
        ${this._hasFields ? html`
          <div class="flex flex-col gap-2 w-full">
            ${this._fields.map((field, fIdx) => this._renderField(field, fIdx))}
          </div>
        ` : nothing}
      </div>
    `;
  }

  private _renderField(field: SearchField, fIdx: number): TemplateResult {
    const hiddenInputs = this._getHiddenInputs(field);
    const ariaLabel = field.label.replace(/ \u203a /g, ', ');

    return html`
      <div class="w-full" data-field-path=${field.path}>
        <!-- Hidden inputs for form submission -->
        ${hiddenInputs.map((hidden, hIdx) => html`
          <input type="hidden" name=${this.fieldName} .value=${hidden.value}
                 data-key="${fIdx}-h-${hIdx}">
        `)}

        ${field.type === 'boolean' ? this._renderBoolean(field, ariaLabel)
          : field.enum && field.enum.length <= 6 ? this._renderEnumCheckboxes(field, ariaLabel)
          : field.enum && field.enum.length > 6 ? this._renderEnumSelect(field, ariaLabel)
          : this._renderTextInput(field, ariaLabel)}
      </div>
    `;
  }

  private _renderBoolean(field: SearchField, ariaLabel: string): TemplateResult {
    const radioName = `search-bool-${field.path}`;
    return html`
      <fieldset class="w-full" aria-label=${ariaLabel}>
        <legend class="block text-xs font-mono font-medium text-stone-600 mt-1">${field.label}</legend>
        <div class="flex gap-3 mt-1">
          <label class="text-sm flex items-center gap-1">
            <input type="radio" name=${radioName} value="any"
                   .checked=${field.boolValue === 'any'}
                   @change=${() => this._onBoolChange(field, 'any')}>
            Any
          </label>
          <label class="text-sm flex items-center gap-1">
            <input type="radio" name=${radioName} value="true"
                   .checked=${field.boolValue === 'true'}
                   @change=${() => this._onBoolChange(field, 'true')}>
            Yes
          </label>
          <label class="text-sm flex items-center gap-1">
            <input type="radio" name=${radioName} value="false"
                   .checked=${field.boolValue === 'false'}
                   @change=${() => this._onBoolChange(field, 'false')}>
            No
          </label>
        </div>
      </fieldset>
    `;
  }

  private _renderEnumCheckboxes(field: SearchField, ariaLabel: string): TemplateResult {
    return html`
      <fieldset class="w-full" aria-label=${ariaLabel}>
        <legend class="block text-xs font-mono font-medium text-stone-600 mt-1">${field.label}</legend>
        <div class="flex flex-wrap gap-x-3 gap-y-1 mt-1">
          ${field.enum!.map(enumVal => html`
            <label class="text-sm flex items-center gap-1">
              <input type="checkbox" .value=${String(enumVal)}
                     .checked=${field.enumValues.includes(String(enumVal))}
                     @change=${(e: Event) => this._onEnumCheckboxChange(field, String(enumVal), (e.target as HTMLInputElement).checked)}>
              <span>${enumVal}</span>
            </label>
          `)}
        </div>
      </fieldset>
    `;
  }

  private _renderEnumSelect(field: SearchField, ariaLabel: string): TemplateResult {
    const selectSize = Math.min(field.enum!.length, 6);
    return html`
      <fieldset class="w-full" aria-label=${ariaLabel}>
        <legend class="block text-xs font-mono font-medium text-stone-600 mt-1">${field.label}</legend>
        <select multiple
                class="w-full text-sm border-stone-300 rounded mt-1 focus:ring-1 focus:ring-amber-600 focus:border-amber-600"
                size=${selectSize}
                @change=${(e: Event) => this._onEnumSelectChange(field, e)}>
          ${field.enum!.map(enumVal => html`
            <option value=${String(enumVal)} ?selected=${field.enumValues.includes(String(enumVal))}>${enumVal}</option>
          `)}
        </select>
      </fieldset>
    `;
  }

  private _renderTextInput(field: SearchField, ariaLabel: string): TemplateResult {
    const inputType = (field.type === 'number' || field.type === 'integer') ? 'number' : 'text';
    const step = field.type === 'integer' ? '1' : 'any';
    const inputId = `search-${field.path}`;
    const symbol = operatorSymbol(field.operator);

    return html`
      <div class="w-full">
        <label for=${inputId}
               class="block text-xs font-mono font-medium text-stone-600 mt-1">
          ${field.label}
        </label>
        <div class="flex gap-1 items-center w-full mt-1">
          ${!field.showOperator ? html`
            <button type="button"
                    data-operator-toggle
                    @click=${() => this._toggleOperator(field)}
                    class="text-xs text-stone-400 hover:text-amber-700 underline cursor-pointer flex-shrink-0 w-5 text-center focus:outline-none focus:ring-1 focus:ring-amber-600 rounded"
                    aria-label=${'Change operator, currently ' + symbol}
                    title=${'Operator: ' + symbol}>
              ${symbol}
            </button>
          ` : html`
            <select @change=${(e: Event) => this._selectOperator(field, e)}
                    aria-label=${'Operator for ' + field.label}
                    class="flex-shrink-0 w-16 text-sm border-stone-300 rounded focus:ring-1 focus:ring-amber-600 focus:border-amber-600">
              ${field.operators!.map(op => html`
                <option value=${op.code} ?selected=${field.operator === op.code}>${op.label}</option>
              `)}
            </select>
          `}
          <input type=${inputType}
                 step=${inputType === 'number' ? step : nothing}
                 .value=${field.value}
                 id=${inputId}
                 class="flex-grow w-full text-sm border-stone-300 rounded focus:ring-1 focus:ring-amber-600 focus:border-amber-600"
                 @input=${(e: Event) => this._onFieldValueInput(field, e)}>
        </div>
      </div>
    `;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'schema-search-mode': SchemaSearchMode;
  }
}
