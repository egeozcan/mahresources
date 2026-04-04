import { LitElement, html, css, nothing } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import { sharedStyles } from '../styles';
import type { SchemaNode } from '../schema-tree-model';
import { isLabeledEnum } from '../schema-core';
import type { EnumEntry } from './node-editors/enum-editor';

// Import all node editors (registers them as custom elements)
import './node-editors/string-editor';
import './node-editors/number-editor';
import './node-editors/boolean-editor';
import './node-editors/object-editor';
import './node-editors/array-editor';
import './node-editors/enum-editor';
import './node-editors/composition-editor';
import './node-editors/conditional-editor';
import './node-editors/ref-editor';

@customElement('schema-detail-panel')
export class SchemaDetailPanel extends LitElement {
  static override styles = [
    sharedStyles,
    css`
      :host { display: block; padding: 20px; overflow-y: auto; height: 100%; }
      .header { margin-bottom: 16px; }
      .header h3 { margin: 0; font-size: 16px; }
      .grid { display: grid; grid-template-columns: 1fr 1fr; gap: 12px; margin-bottom: 16px; }
      .grid-full { grid-column: span 2; }
      .flags {
        display: flex; gap: 16px; padding: 12px;
        background: #f9fafb; border-radius: 6px; margin-bottom: 16px;
      }
      .flags label { display: flex; align-items: center; gap: 6px; font-size: 12px; color: #374151; font-weight: normal; }
      .type-section { border: 1px solid #e5e7eb; border-radius: 6px; padding: 16px; margin-bottom: 16px; }
      .type-section h4 { margin: 0 0 12px; font-size: 13px; font-weight: 600; }
      .actions {
        display: flex; gap: 8px; padding-top: 12px;
        border-top: 1px solid #e5e7eb;
      }
    `,
  ];

  @property({ type: Object }) node: SchemaNode | null = null;
  @property({ type: Array }) breadcrumb: string[] = [];
  @property({ type: Array }) defsNames: string[] = [];
  @property({ type: String }) defsPrefix = '$defs';
  @property({ type: Boolean }) isRoot = false;

  private _dispatchChange(field: string, value: any) {
    this.dispatchEvent(new CustomEvent('node-change', {
      detail: { field, value },
      bubbles: true,
      composed: true,
    }));
  }

  private _dispatchDelete() {
    this.dispatchEvent(new CustomEvent('node-delete', { bubbles: true, composed: true }));
  }

  private _dispatchDuplicate() {
    this.dispatchEvent(new CustomEvent('node-duplicate', { bubbles: true, composed: true }));
  }

  private _renderActions() {
    if (this.isRoot) return nothing;
    return html`
      <div class="actions">
        <button class="btn btn-danger" @click=${this._dispatchDelete}>Delete Property</button>
        <button class="btn" @click=${this._dispatchDuplicate}>Duplicate</button>
      </div>
    `;
  }

  /**
   * Shared header + metadata fields rendered for ALL node types: regular
   * properties, $ref nodes, composition nodes, and conditional nodes.
   *
   * Includes: breadcrumb, property name input, title, description, and
   * the required checkbox (when not root).
   */
  private _renderCommonFields() {
    if (!this.node) return nothing;
    const node = this.node;
    const schema = node.schema;

    return html`
      <div class="header">
        <div class="breadcrumb">${this.breadcrumb.slice(0, -1).join(' → ')}${this.breadcrumb.length > 1 ? ' → ' : ''}<span class="current">${this.breadcrumb.at(-1) || 'root'}</span></div>
        <h3>${this.isRoot ? 'Root Schema' : `Property: ${node.name}`}</h3>
      </div>

      <div class="grid">
        ${!this.isRoot ? html`
          <div>
            <label for="prop-name">Property Name</label>
            <input id="prop-name" .value=${node.name} @change=${(e: Event) => this._dispatchChange('name', (e.target as HTMLInputElement).value)}>
          </div>
        ` : ''}
        <div>
          <label for="prop-title">Title</label>
          <input id="prop-title" .value=${schema.title || ''} @change=${(e: Event) => this._dispatchChange('title', (e.target as HTMLInputElement).value)}>
        </div>
        <div>
          <label for="prop-desc">Description</label>
          <input id="prop-desc" .value=${schema.description || ''} @change=${(e: Event) => this._dispatchChange('description', (e.target as HTMLInputElement).value)}>
        </div>
      </div>

      <div class="flags">
        ${!this.isRoot ? html`
          <label><input type="checkbox" ?checked=${node.required} @change=${(e: Event) => this._dispatchChange('required', (e.target as HTMLInputElement).checked)}> Required</label>
        ` : ''}
        <label><input type="checkbox" ?checked=${schema.readOnly} @change=${(e: Event) => this._dispatchChange('readOnly', (e.target as HTMLInputElement).checked)}> Read Only</label>
        <label><input type="checkbox" ?checked=${schema.writeOnly} @change=${(e: Event) => this._dispatchChange('writeOnly', (e.target as HTMLInputElement).checked)}> Write Only</label>
      </div>
    `;
  }

  private _renderTypeEditor() {
    if (!this.node) return nothing;
    const schema = this.node.schema;

    // Labeled enum: oneOf with const+title entries
    if (isLabeledEnum(schema)) {
      const entries: EnumEntry[] = (schema.oneOf as any[]).map((e: any) => ({
        value: e.const,
        label: e.title || '',
      }));
      return html`<schema-enum-editor
        .entries=${entries}
        .labeled=${true}
        .valueType=${this.node.type}
        @enum-change=${(e: CustomEvent) => {
          if (e.detail.labeled) {
            this._dispatchChange('labeledEnum', e.detail.entries);
          } else {
            this._dispatchChange('enum', e.detail.values);
          }
        }}
      ></schema-enum-editor>`;
    }

    // Plain enum editor (any type can have enum)
    if (schema.enum) {
      return html`<schema-enum-editor
        .values=${schema.enum}
        .valueType=${this.node.type}
        @enum-change=${(e: CustomEvent) => {
          if (e.detail.labeled) {
            this._dispatchChange('labeledEnum', e.detail.entries);
          } else {
            this._dispatchChange('enum', e.detail.values);
          }
        }}
      ></schema-enum-editor>`;
    }

    switch (this.node.type) {
      case 'string':
        return html`<schema-string-editor .schema=${schema} @constraint-change=${(e: CustomEvent) => this._dispatchChange(e.detail.field, e.detail.value)}></schema-string-editor>`;
      case 'number':
      case 'integer':
        return html`<schema-number-editor .schema=${schema} .integerOnly=${this.node.type === 'integer'} @constraint-change=${(e: CustomEvent) => this._dispatchChange(e.detail.field, e.detail.value)}></schema-number-editor>`;
      case 'boolean':
        return html`<schema-boolean-editor .schema=${schema} @constraint-change=${(e: CustomEvent) => this._dispatchChange(e.detail.field, e.detail.value)}></schema-boolean-editor>`;
      case 'object':
        return html`<schema-object-editor .schema=${schema} @constraint-change=${(e: CustomEvent) => this._dispatchChange(e.detail.field, e.detail.value)}></schema-object-editor>`;
      case 'array':
        return html`<schema-array-editor .schema=${schema} @constraint-change=${(e: CustomEvent) => this._dispatchChange(e.detail.field, e.detail.value)}></schema-array-editor>`;
      default:
        return nothing;
    }
  }

  override render() {
    if (!this.node) {
      return html`<div style="display:flex;align-items:center;justify-content:center;height:100%;color:#9ca3af;">Select a node from the tree</div>`;
    }

    const node = this.node;
    const schema = node.schema;

    // $ref nodes get a special editor
    if (node.ref) {
      return html`
        ${this._renderCommonFields()}
        <schema-ref-editor .ref=${node.ref} .defsNames=${this.defsNames} .defsPrefix=${this.defsPrefix} @ref-change=${(e: CustomEvent) => this._dispatchChange('$ref', e.detail.ref)}></schema-ref-editor>
        ${this._renderActions()}
      `;
    }

    // Composition nodes
    if (node.compositionKeyword) {
      return html`
        ${this._renderCommonFields()}
        <schema-composition-editor .keyword=${node.compositionKeyword} .variants=${node.variants || []}></schema-composition-editor>
        ${this._renderActions()}
      `;
    }

    // Conditional nodes
    if (schema.if) {
      return html`
        ${this._renderCommonFields()}
        <schema-conditional-editor .schema=${schema}></schema-conditional-editor>
        ${this._renderActions()}
      `;
    }

    const allTypes = ['string', 'integer', 'number', 'boolean', 'object', 'array', 'null'];

    return html`
      <div class="header">
        <div class="breadcrumb">${this.breadcrumb.slice(0, -1).join(' → ')}${this.breadcrumb.length > 1 ? ' → ' : ''}<span class="current">${this.breadcrumb.at(-1) || 'root'}</span></div>
        <h3>${this.isRoot ? 'Root Schema' : `Property: ${node.name}`}</h3>
      </div>

      <div class="grid">
        ${!this.isRoot ? html`
          <div>
            <label for="prop-name">Property Name</label>
            <input id="prop-name" .value=${node.name} @change=${(e: Event) => this._dispatchChange('name', (e.target as HTMLInputElement).value)}>
          </div>
        ` : ''}
        <div>
          <label for="prop-type">Type</label>
          <select id="prop-type" .value=${node.type} @change=${(e: Event) => this._dispatchChange('type', (e.target as HTMLSelectElement).value)}>
            ${allTypes.map(t => html`<option .value=${t} ?selected=${t === node.type}>${t}</option>`)}
          </select>
        </div>
        <div>
          <label for="prop-title">Title</label>
          <input id="prop-title" .value=${schema.title || ''} @change=${(e: Event) => this._dispatchChange('title', (e.target as HTMLInputElement).value)}>
        </div>
        <div>
          <label for="prop-desc">Description</label>
          <input id="prop-desc" .value=${schema.description || ''} @change=${(e: Event) => this._dispatchChange('description', (e.target as HTMLInputElement).value)}>
        </div>
      </div>

      <div class="flags">
        ${!this.isRoot ? html`
          <label><input type="checkbox" ?checked=${node.required} @change=${(e: Event) => this._dispatchChange('required', (e.target as HTMLInputElement).checked)}> Required</label>
        ` : ''}
        <label><input type="checkbox" ?checked=${schema.readOnly} @change=${(e: Event) => this._dispatchChange('readOnly', (e.target as HTMLInputElement).checked)}> Read Only</label>
        <label><input type="checkbox" ?checked=${schema.writeOnly} @change=${(e: Event) => this._dispatchChange('writeOnly', (e.target as HTMLInputElement).checked)}> Write Only</label>
        ${!this.isRoot && node.type !== 'null' ? html`
          <label><input type="checkbox" ?checked=${Array.isArray(node.schema.type) && node.schema.type.includes('null')} @change=${(e: Event) => this._dispatchChange('nullable', (e.target as HTMLInputElement).checked)}> Nullable</label>
        ` : ''}
      </div>

      ${this._renderTypeEditor()}

      ${this._renderActions()}
    `;
  }
}
