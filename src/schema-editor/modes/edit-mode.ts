import { LitElement, html, css } from 'lit';
import { customElement, property, state } from 'lit/decorators.js';
import { sharedStyles } from '../styles';
import { schemaToTree, treeToSchema, detectDraft } from '../schema-tree-model';
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
      case 'name': {
        // Prevent duplicate names among all siblings (properties and $defs alike)
        const parentAndIndex = this._findParentOf(this._selectedId);
        if (parentAndIndex) {
          const [parent] = parentAndIndex;
          const siblingNames = new Set(
            (parent.children || []).filter(c => c.id !== selected.id).map(c => c.name)
          );
          if (siblingNames.has(value)) {
            let deduped = value;
            let counter = 1;
            while (siblingNames.has(deduped)) deduped = `${value}${counter++}`;
            selected.name = deduped;
            break;
          }
        }
        selected.name = value;
        break;
      }
      case 'type':
        selected.type = value;
        // Reset type-specific constraints
        for (const key of ['minLength', 'maxLength', 'pattern', 'format', 'minimum', 'maximum',
          'exclusiveMinimum', 'exclusiveMaximum', 'multipleOf', 'minItems', 'maxItems',
          'uniqueItems', 'additionalProperties', 'minProperties', 'maxProperties',
          'items', 'enum', 'prefixItems', 'contains', 'patternProperties']) {
          delete selected.schema[key];
        }
        // Update nullable type array if present (keep "null" but swap the base type)
        if (Array.isArray(selected.schema.type) && selected.schema.type.includes('null')) {
          selected.schema.type = [value, 'null'];
        }
        break;
      case 'required':
        selected.required = value;
        break;
      case '$ref':
        selected.ref = value;
        break;
      case 'nullable': {
        const baseType = selected.type || 'string';
        if (value) {
          // Store nullable union in node.schema.type so treeToSchema emits it
          selected.schema.type = [baseType, 'null'];
        } else {
          // Remove the nullable union — let treeToSchema fall through to node.type
          delete selected.schema.type;
        }
        break;
      }
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

    // If the selected node is an object, add the property as its child;
    // otherwise fall back to root.
    const selected = this._findNode(this._selectedId);
    const target = (selected && selected.type === 'object' && selected !== this._root)
      ? selected
      : this._root;

    if (!target.children) target.children = [];
    let name = 'newProperty';
    let counter = 1;
    const existing = new Set((target.children || []).map(c => c.name));
    while (existing.has(name)) name = `newProperty${counter++}`;
    const newNode: SchemaNode = {
      id: `node-new-${Date.now()}`,
      name,
      type: 'string',
      required: false,
      schema: {},
    };
    // Insert before $defs node if present
    const defsIndex = target.children.findIndex(c => c.name === '$defs');
    if (defsIndex >= 0) {
      target.children.splice(defsIndex, 0, newNode);
    } else {
      target.children.push(newNode);
    }
    this._selectedId = newNode.id;
    this.requestUpdate();
    this._emitSchemaChange();
  }

  private _handleAddVariant() {
    const selected = this._findNode(this._selectedId);
    if (!selected || !selected.compositionKeyword) return;
    if (!selected.children) selected.children = [];
    selected.children.push({
      id: `node-variant-${Date.now()}`,
      name: `variant${selected.children.length + 1}`,
      type: 'string',
      required: false,
      schema: {},
    });
    this.requestUpdate();
    this._emitSchemaChange();
  }

  private _handleRemoveVariant(e: CustomEvent) {
    const selected = this._findNode(this._selectedId);
    if (!selected?.children) return;
    const { index } = e.detail;
    selected.children.splice(index, 1);
    this.requestUpdate();
    this._emitSchemaChange();
  }

  private _handleReorder(e: CustomEvent) {
    if (!this._root) return;
    const { draggedId, targetId } = e.detail;

    // Find dragged node's parent and index
    const draggedInfo = this._findParentOf(draggedId);
    const targetInfo = this._findParentOf(targetId);
    if (!draggedInfo || !targetInfo) return;

    const [dragParent, dragIndex] = draggedInfo;
    const [targetParent, targetIndex] = targetInfo;

    // Only reorder within the same parent
    if (dragParent !== targetParent) return;

    const [removed] = dragParent.children!.splice(dragIndex, 1);
    // Recalculate target index after removal
    const insertIndex = dragParent.children!.findIndex(c => c.id === targetId);
    if (insertIndex >= 0) {
      dragParent.children!.splice(insertIndex, 0, removed);
    } else {
      dragParent.children!.push(removed);
    }

    this.requestUpdate();
    this._emitSchemaChange();
  }

  private _handleContextAction(e: CustomEvent) {
    const { nodeId, action } = e.detail;
    const node = this._findNode(nodeId);
    if (!node) return;

    switch (action) {
      case 'wrap-oneOf':
      case 'wrap-anyOf':
      case 'wrap-allOf': {
        const keyword = action.replace('wrap-', '') as 'oneOf' | 'anyOf' | 'allOf';
        // Split schema into metadata (stays on wrapper) and type-specific (goes into variant)
        const metadataKeys = ['title', 'description', 'readOnly', 'writeOnly', 'default', 'examples', 'deprecated'];
        const metadata: Record<string, any> = {};
        const typeSchema: Record<string, any> = {};
        for (const [k, v] of Object.entries(node.schema)) {
          if (metadataKeys.includes(k)) {
            metadata[k] = v;
          } else {
            typeSchema[k] = v;
          }
        }
        // Build the first variant from the type-specific schema
        const originalType = node.type;
        if (originalType) typeSchema.type = originalType;
        const variantName = node.schema.title || 'variant1';
        // Set up the node as a composition node, keeping metadata on the wrapper
        node.compositionKeyword = keyword;
        node.schema = metadata;
        node.type = '';
        node.children = [
          { id: `node-variant-${Date.now()}-0`, name: variantName, type: originalType || '', required: false, schema: typeSchema },
          { id: `node-variant-${Date.now()}-1`, name: 'variant2', type: 'string', required: false, schema: {} },
        ];
        // Clean type from first variant's schema (it's stored in node.type)
        delete node.children[0].schema.type;
        break;
      }
      case 'add-if-then-else':
        node.schema.if = { properties: {} };
        node.schema.then = { properties: {} };
        node.schema.else = { properties: {} };
        break;
      case 'convert-to-ref': {
        if (!this._root) break;
        // Find or create $defs node
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
        // Create a definition from the node's current schema (deduplicate name)
        let defName = node.name || 'extracted';
        let defCounter = 1;
        const existingDefs = new Set((defsNode.children || []).map(c => c.name));
        while (existingDefs.has(defName)) defName = `${node.name || 'extracted'}${defCounter++}`;
        const defNode: SchemaNode = {
          id: `node-def-${Date.now()}`,
          name: defName,
          type: node.type,
          required: false,
          schema: { ...node.schema },
          isDef: true,
          children: node.children ? [...node.children] : undefined,
        };
        defsNode.children!.push(defNode);
        // Replace node with $ref
        node.type = '';
        node.schema = {};
        node.ref = `#/$defs/${defName}`;
        node.children = undefined;
        node.compositionKeyword = undefined;
        break;
      }
    }

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
    let name = 'newDefinition';
    let counter = 1;
    const existing = new Set((defsNode.children || []).map(c => c.name));
    while (existing.has(name)) name = `newDefinition${counter++}`;
    const newDef: SchemaNode = {
      id: `node-def-${Date.now()}`,
      name,
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
          @reorder=${this._handleReorder}
          @context-action=${this._handleContextAction}
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
          @add-variant=${this._handleAddVariant}
          @remove-variant=${this._handleRemoveVariant}
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
