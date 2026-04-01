import { LitElement, html, css } from 'lit';
import { customElement, property, state } from 'lit/decorators.js';
import { sharedStyles } from '../styles';
import { schemaToTree, treeToSchema, detectDraft, resetIdCounter } from '../schema-tree-model';
import type { SchemaNode } from '../schema-tree-model';
import type { JSONSchema } from '../schema-core';
import '../tree/tree-panel';
import '../tree/detail-panel';

@customElement('schema-edit-mode')
export class SchemaEditMode extends LitElement {
  static override styles = [
    sharedStyles,
    css`
      :host { display: flex; height: 100%; }
      .tree-side {
        width: 260px;
        border-right: 1px solid #e5e7eb;
        background: #f9fafb;
        flex-shrink: 0;
        overflow: hidden;
        display: flex;
        flex-direction: column;
      }
      .detail-side {
        flex: 1;
        overflow: hidden;
      }
    `,
  ];

  @property({ type: Object }) schema: JSONSchema = {};

  @state() private _root: SchemaNode | null = null;
  @state() private _selectedId = '';
  @state() private _draft: string | null = null;

  override willUpdate(changed: Map<string, unknown>) {
    if (changed.has('schema') && this.schema) {
      resetIdCounter();
      this._root = schemaToTree(this.schema);
      this._draft = detectDraft(this.schema);
      if (this._root && !this._selectedId) {
        this._selectedId = this._root.id;
      }
    }
  }

  private _findNode(id: string, node: SchemaNode | null = this._root): SchemaNode | null {
    if (!node) return null;
    if (node.id === id) return node;
    for (const child of node.children || []) {
      const found = this._findNode(id, child);
      if (found) return found;
    }
    return null;
  }

  private _buildBreadcrumb(id: string, node: SchemaNode | null = this._root, path: string[] = []): string[] {
    if (!node) return [];
    const current = [...path, node.name || 'root'];
    if (node.id === id) return current;
    for (const child of node.children || []) {
      const result = this._buildBreadcrumb(id, child, current);
      if (result.length) return result;
    }
    return [];
  }

  private _getDefsNames(): string[] {
    if (!this._root) return [];
    const defsNode = (this._root.children || []).find(c => c.name === '$defs');
    return (defsNode?.children || []).map(c => c.name);
  }

  private _emitSchemaChange() {
    if (!this._root) return;
    const schema = treeToSchema(this._root);
    this.dispatchEvent(new CustomEvent('schema-change', {
      detail: { schema: JSON.stringify(schema, null, 2) },
      bubbles: true,
      composed: true,
    }));
  }

  private _handleNodeSelect(e: CustomEvent) {
    this._selectedId = e.detail.nodeId;
  }

  private _handleNodeChange(e: CustomEvent) {
    const selected = this._findNode(this._selectedId);
    if (!selected) return;

    const { field, value } = e.detail;

    switch (field) {
      case 'name':
        selected.name = value;
        break;
      case 'type':
        selected.type = value;
        // Reset type-specific constraints
        for (const key of ['minLength', 'maxLength', 'pattern', 'format', 'minimum', 'maximum',
          'exclusiveMinimum', 'exclusiveMaximum', 'multipleOf', 'minItems', 'maxItems',
          'uniqueItems', 'additionalProperties', 'minProperties', 'maxProperties', 'items', 'enum']) {
          delete selected.schema[key];
        }
        break;
      case 'required':
        selected.required = value;
        break;
      default:
        if (value === undefined) {
          delete selected.schema[field];
        } else {
          selected.schema[field] = value;
        }
    }

    this.requestUpdate();
    this._emitSchemaChange();
  }

  private _handleNodeDelete() {
    if (!this._root) return;
    const parentAndIndex = this._findParentOf(this._selectedId);
    if (!parentAndIndex) return;
    const [parent, index] = parentAndIndex;
    parent.children!.splice(index, 1);
    this._selectedId = parent.id;
    this.requestUpdate();
    this._emitSchemaChange();
  }

  private _handleNodeDuplicate() {
    if (!this._root) return;
    const parentAndIndex = this._findParentOf(this._selectedId);
    if (!parentAndIndex) return;
    const [parent, index] = parentAndIndex;
    const original = parent.children![index];
    const clone = JSON.parse(JSON.stringify(original));
    clone.name = original.name + '_copy';
    clone.id = `node-dup-${Date.now()}`;
    // Regenerate IDs for all children
    const reId = (n: SchemaNode) => { n.id = `node-dup-${Date.now()}-${Math.random()}`; (n.children || []).forEach(reId); };
    reId(clone);
    parent.children!.splice(index + 1, 0, clone);
    this._selectedId = clone.id;
    this.requestUpdate();
    this._emitSchemaChange();
  }

  private _findParentOf(id: string, node: SchemaNode | null = this._root): [SchemaNode, number] | null {
    if (!node || !node.children) return null;
    for (let i = 0; i < node.children.length; i++) {
      if (node.children[i].id === id) return [node, i];
      const found = this._findParentOf(id, node.children[i]);
      if (found) return found;
    }
    return null;
  }

  private _handleAddProperty() {
    if (!this._root) return;
    if (!this._root.children) this._root.children = [];
    let name = 'newProperty';
    let counter = 1;
    const existing = new Set((this._root.children || []).map(c => c.name));
    while (existing.has(name)) name = `newProperty${counter++}`;
    const newNode: SchemaNode = {
      id: `node-new-${Date.now()}`,
      name,
      type: 'string',
      required: false,
      schema: {},
    };
    // Insert before $defs node if present
    const defsIndex = this._root.children.findIndex(c => c.name === '$defs');
    if (defsIndex >= 0) {
      this._root.children.splice(defsIndex, 0, newNode);
    } else {
      this._root.children.push(newNode);
    }
    this._selectedId = newNode.id;
    this.requestUpdate();
    this._emitSchemaChange();
  }

  private _handleAddDefs() {
    if (!this._root) return;
    if (!this._root.children) this._root.children = [];
    let defsNode = this._root.children.find(c => c.name === '$defs');
    if (!defsNode) {
      defsNode = {
        id: `node-defs-${Date.now()}`,
        name: '$defs',
        type: 'object',
        required: false,
        schema: {},
        isDef: true,
        children: [],
      };
      this._root.children.push(defsNode);
    }
    const newDef: SchemaNode = {
      id: `node-def-${Date.now()}`,
      name: 'newDefinition',
      type: 'object',
      required: false,
      schema: {},
      isDef: true,
    };
    defsNode.children!.push(newDef);
    this._selectedId = newDef.id;
    this.requestUpdate();
    this._emitSchemaChange();
  }

  override render() {
    const selected = this._findNode(this._selectedId);
    const breadcrumb = this._buildBreadcrumb(this._selectedId);
    const isRoot = selected === this._root;

    return html`
      <div class="tree-side">
        <schema-tree-panel
          .root=${this._root}
          .selectedId=${this._selectedId}
          .draft=${this._draft}
          @node-select=${this._handleNodeSelect}
          @add-property=${this._handleAddProperty}
          @add-defs=${this._handleAddDefs}
        ></schema-tree-panel>
      </div>
      <div class="detail-side">
        <schema-detail-panel
          .node=${selected}
          .breadcrumb=${breadcrumb}
          .defsNames=${this._getDefsNames()}
          .isRoot=${isRoot}
          @node-change=${this._handleNodeChange}
          @node-delete=${this._handleNodeDelete}
          @node-duplicate=${this._handleNodeDuplicate}
        ></schema-detail-panel>
      </div>
    `;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'schema-edit-mode': SchemaEditMode;
  }
}
