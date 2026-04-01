import { LitElement, html, css } from 'lit';
import { customElement, property, state } from 'lit/decorators.js';
import { repeat } from 'lit/directives/repeat.js';
import { classMap } from 'lit/directives/class-map.js';
import { sharedStyles } from '../styles';
import type { SchemaNode } from '../schema-tree-model';

@customElement('schema-tree-panel')
export class SchemaTreePanel extends LitElement {
  static override styles = [
    sharedStyles,
    css`
      :host { display: flex; flex-direction: column; height: 100%; }
      .toolbar {
        padding: 8px 12px;
        border-bottom: 1px solid #e5e7eb;
        display: flex;
        gap: 4px;
        align-items: center;
      }
      .toolbar .spacer { flex: 1; }
      .toolbar .draft { font-size: 10px; color: #9ca3af; }
      .tree { flex: 1; overflow-y: auto; padding: 8px 0; }
      .tree-node {
        padding: 4px 12px;
        display: flex;
        align-items: center;
        gap: 6px;
        cursor: pointer;
        border-radius: 4px;
        margin: 1px 4px;
        font-size: 12px;
        user-select: none;
      }
      .tree-node:hover { background: #f3f4f6; }
      .tree-node:focus-visible { outline: 2px solid #6366f1; outline-offset: -2px; }
      .tree-node.selected { background: #eef2ff; border: 1px solid #c7d2fe; }
      .tree-node.selected .node-name { font-weight: 600; color: #4338ca; }
      .node-name { color: #1f2937; flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
      .expand-icon { color: #9ca3af; font-size: 10px; width: 12px; text-align: center; flex-shrink: 0; }
      .drag-handle { color: #9ca3af; font-size: 10px; cursor: grab; flex-shrink: 0; }
      .children { margin-left: 16px; }
      .defs-section { margin-top: 8px; border-top: 1px solid #e5e7eb; padding-top: 8px; }
      .defs-header {
        padding: 4px 12px;
        font-size: 12px;
        color: #6b7280;
        display: flex;
        align-items: center;
        gap: 4px;
      }
      .defs-header .count { font-size: 10px; color: #9ca3af; margin-left: auto; }
    `,
  ];

  @property({ type: Object }) root: SchemaNode | null = null;
  @property({ type: String }) selectedId = '';
  @property({ type: String }) draft: string | null = null;

  @state() private _expanded = new Set<string>();

  override connectedCallback() {
    super.connectedCallback();
    // Expand root by default
    if (this.root) {
      this._expanded.add(this.root.id);
    }
  }

  override willUpdate(changed: Map<string, unknown>) {
    if (changed.has('root') && this.root) {
      this._expanded.add(this.root.id);
    }
  }

  private _toggleExpand(nodeId: string) {
    if (this._expanded.has(nodeId)) {
      this._expanded.delete(nodeId);
    } else {
      this._expanded.add(nodeId);
    }
    this.requestUpdate();
  }

  private _selectNode(nodeId: string) {
    this.dispatchEvent(new CustomEvent('node-select', { detail: { nodeId }, bubbles: true, composed: true }));
  }

  private _handleKeydown(e: KeyboardEvent, node: SchemaNode) {
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault();
      this._selectNode(node.id);
    }
    if (e.key === 'ArrowRight' && node.children?.length) {
      e.preventDefault();
      this._expanded.add(node.id);
      this.requestUpdate();
    }
    if (e.key === 'ArrowLeft') {
      e.preventDefault();
      this._expanded.delete(node.id);
      this.requestUpdate();
    }
  }

  private _addProperty() {
    this.dispatchEvent(new CustomEvent('add-property', { bubbles: true, composed: true }));
  }

  private _addDefs() {
    this.dispatchEvent(new CustomEvent('add-defs', { bubbles: true, composed: true }));
  }

  private _getBadgeClass(node: SchemaNode): string {
    if (node.ref) return 'badge badge-ref';
    if (node.compositionKeyword) return 'badge badge-composition';
    if (node.schema?.enum) return 'badge badge-enum';
    if (node.schema?.if) return 'badge badge-conditional';
    return `badge badge-${node.type || 'string'}`;
  }

  private _getBadgeText(node: SchemaNode): string {
    if (node.ref) return '$ref';
    if (node.compositionKeyword) return node.compositionKeyword;
    if (node.schema?.enum) return 'enum';
    if (node.schema?.if) return 'if/then';
    return node.type || 'string';
  }

  private _hasChildren(node: SchemaNode): boolean {
    const propChildren = (node.children || []).filter(c => c.name !== '$defs');
    return propChildren.length > 0;
  }

  private _renderNode(node: SchemaNode, isRoot = false): unknown {
    const propChildren = (node.children || []).filter(c => c.name !== '$defs' && !c.isDef);
    const defsNode = (node.children || []).find(c => c.name === '$defs');
    const expanded = this._expanded.has(node.id);
    const selected = this.selectedId === node.id;
    const hasChildren = propChildren.length > 0;

    return html`
      <div
        class=${classMap({ 'tree-node': true, selected })}
        role="treeitem"
        tabindex=${selected ? '0' : '-1'}
        aria-selected=${selected}
        aria-expanded=${hasChildren ? String(expanded) : 'undefined'}
        @click=${() => this._selectNode(node.id)}
        @dblclick=${() => hasChildren && this._toggleExpand(node.id)}
        @keydown=${(e: KeyboardEvent) => this._handleKeydown(e, node)}
      >
        ${hasChildren
          ? html`<span class="expand-icon" @click=${(e: Event) => { e.stopPropagation(); this._toggleExpand(node.id); }}>${expanded ? '▼' : '▶'}</span>`
          : html`<span class="expand-icon"></span>`}
        ${!isRoot ? html`<span class="drag-handle" aria-hidden="true">☰</span>` : ''}
        <span class="node-name">${isRoot ? 'root' : node.name}</span>
        ${node.required ? html`<span class="required-marker" aria-label="required">*</span>` : ''}
        <span class=${this._getBadgeClass(node)}>${this._getBadgeText(node)}</span>
      </div>
      ${hasChildren && expanded
        ? html`<div class="children" role="group">
            ${repeat(propChildren, c => c.id, c => this._renderNode(c))}
          </div>`
        : ''}
      ${isRoot && defsNode && defsNode.children?.length
        ? html`
          <div class="defs-section">
            <div class="defs-header">
              <span class="expand-icon" @click=${() => this._toggleExpand(defsNode.id)}>${this._expanded.has(defsNode.id) ? '▼' : '▶'}</span>
              <span style="font-weight:600;color:#6b7280;">$defs</span>
              <span class="count">${defsNode.children.length} definition${defsNode.children.length !== 1 ? 's' : ''}</span>
            </div>
            ${this._expanded.has(defsNode.id)
              ? html`<div class="children" role="group">
                  ${repeat(defsNode.children, c => c.id, c => this._renderNode(c))}
                </div>`
              : ''}
          </div>`
        : ''}
    `;
  }

  override render() {
    if (!this.root) return html`<div>No schema loaded</div>`;

    return html`
      <div class="toolbar">
        <button class="btn" @click=${this._addProperty}>+ Property</button>
        <button class="btn" @click=${this._addDefs}>+ $defs</button>
        <span class="spacer"></span>
        ${this.draft ? html`<span class="draft">${this.draft}</span>` : ''}
      </div>
      <div class="tree" role="tree" aria-label="Schema structure">
        ${this._renderNode(this.root, true)}
      </div>
    `;
  }
}
