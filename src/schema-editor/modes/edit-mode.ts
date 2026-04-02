import { LitElement, html, css } from 'lit';
import { customElement, property, state } from 'lit/decorators.js';
import { sharedStyles } from '../styles';
import { schemaToTree, treeToSchema, detectDraft, getDefsPrefix } from '../schema-tree-model';
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

  /** Tracks the last schema we emitted so we can skip redundant reparses */
  private _lastEmittedSchema = '';

  override willUpdate(changed: Map<string, unknown>) {
    if (changed.has('schema') && this.schema) {
      // Skip reparse when the incoming schema matches what we just emitted.
      // Internal edits mutate the tree in-place and then emit; the parent echoes
      // the schema back as a prop change, which would reparse and regenerate IDs,
      // causing the selected node to become unfindable.
      const incoming = JSON.stringify(this.schema);
      if (incoming === this._lastEmittedSchema) {
        return;
      }
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
    for (const variant of node.variants || []) {
      const found = this._findNode(id, variant);
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
    for (const variant of node.variants || []) {
      const result = this._buildBreadcrumb(id, variant, current);
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
    const serialized = JSON.stringify(schema, null, 2);
    // Store a compact form for the willUpdate guard comparison
    this._lastEmittedSchema = JSON.stringify(schema);
    this.dispatchEvent(new CustomEvent('schema-change', {
      detail: { schema: serialized },
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
          const [, , arr] = parentAndIndex;
          const siblingNames = new Set(
            arr.filter(c => c.id !== selected.id).map(c => c.name)
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
      case 'enum':
        if (Array.isArray(value) && value.length === 0) {
          delete selected.schema.enum;
        } else {
          selected.schema.enum = value;
        }
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
    const [parent, index, arr] = parentAndIndex;
    arr.splice(index, 1);
    this._selectedId = parent.id;
    this.requestUpdate();
    this._emitSchemaChange();
  }

  private _handleNodeDuplicate() {
    if (!this._root) return;
    const parentAndIndex = this._findParentOf(this._selectedId);
    if (!parentAndIndex) return;
    const [, index, arr] = parentAndIndex;
    const original = arr[index];
    const clone = JSON.parse(JSON.stringify(original));
    // Deduplicate clone name among siblings
    let cloneName = original.name + '_copy';
    const siblingNames = new Set(arr.map(c => c.name));
    let counter = 1;
    while (siblingNames.has(cloneName)) {
      cloneName = `${original.name}_copy${counter++}`;
    }
    clone.name = cloneName;
    clone.id = `node-dup-${Date.now()}`;
    // Regenerate IDs for all children and variants
    const reId = (n: SchemaNode) => { n.id = `node-dup-${Date.now()}-${Math.random()}`; (n.children || []).forEach(reId); (n.variants || []).forEach(reId); };
    reId(clone);
    arr.splice(index + 1, 0, clone);
    this._selectedId = clone.id;
    this.requestUpdate();
    this._emitSchemaChange();
  }

  /**
   * Finds the parent node of the given id and returns the parent, the index,
   * and the array (children or variants) that contains the node.
   */
  private _findParentOf(id: string, node: SchemaNode | null = this._root): [SchemaNode, number, SchemaNode[]] | null {
    if (!node) return null;
    if (node.children) {
      for (let i = 0; i < node.children.length; i++) {
        if (node.children[i].id === id) return [node, i, node.children];
        const found = this._findParentOf(id, node.children[i]);
        if (found) return found;
      }
    }
    if (node.variants) {
      for (let i = 0; i < node.variants.length; i++) {
        if (node.variants[i].id === id) return [node, i, node.variants];
        const found = this._findParentOf(id, node.variants[i]);
        if (found) return found;
      }
    }
    return null;
  }

  private _handleAddProperty() {
    if (!this._root) return;

    // If the selected node is object-like, add the property as its child;
    // otherwise fall back to root. A node is object-like if it has type 'object'
    // OR if it already has children (e.g. typeless schemas with properties).
    const selected = this._findNode(this._selectedId);
    const isObjectLike = selected && selected !== this._root && (
      selected.type === 'object' ||
      selected.children != null
    );
    const target = isObjectLike ? selected! : this._root;

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
    if (!selected.variants) selected.variants = [];
    selected.variants.push({
      id: `node-variant-${Date.now()}`,
      name: `variant${selected.variants.length + 1}`,
      type: 'string',
      required: false,
      schema: {},
    });
    this.requestUpdate();
    this._emitSchemaChange();
  }

  private _handleRemoveVariant(e: CustomEvent) {
    const selected = this._findNode(this._selectedId);
    if (!selected?.variants) return;
    const { index } = e.detail;
    selected.variants.splice(index, 1);
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

    const [dragParent, dragIndex, dragArr] = draggedInfo;
    const [targetParent, , targetArr] = targetInfo;

    // Only reorder within the same parent and same array (children or variants)
    if (dragParent !== targetParent || dragArr !== targetArr) return;

    const [removed] = dragArr.splice(dragIndex, 1);
    // Recalculate target index after removal
    const insertIndex = dragArr.findIndex(c => c.id === targetId);
    if (insertIndex >= 0) {
      dragArr.splice(insertIndex, 0, removed);
    } else {
      dragArr.push(removed);
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
        // Capture children before overwriting — they belong to the first variant
        const originalChildren = node.children ? [...node.children] : undefined;
        // Set up the node as a composition node, keeping metadata on the wrapper
        // Property children move into the first variant; variants go in `node.variants`
        node.compositionKeyword = keyword;
        node.schema = metadata;
        node.type = '';
        node.children = undefined;
        node.variants = [
          { id: `node-variant-${Date.now()}-0`, name: variantName, type: originalType || '', required: false, schema: typeSchema, children: originalChildren },
          { id: `node-variant-${Date.now()}-1`, name: 'variant2', type: 'string', required: false, schema: {} },
        ];
        // Clean type from first variant's schema (it's stored in node.type)
        delete node.variants[0].schema.type;
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
          variants: node.variants ? [...node.variants] : undefined,
          compositionKeyword: node.compositionKeyword,
          ref: node.ref,
        };
        defsNode.children!.push(defNode);
        // Replace node with $ref (use correct prefix based on draft version)
        const defsPrefix = getDefsPrefix(this._root?.schema.$schema as string | undefined);
        node.type = '';
        node.schema = {};
        node.ref = `#/${defsPrefix}/${defName}`;
        node.children = undefined;
        node.variants = undefined;
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
          .defsPrefix=${getDefsPrefix(this._root?.schema.$schema as string | undefined)}
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
