import { LitElement, html, css, nothing } from 'lit';
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
      .tree-node.drop-target { border-top: 2px solid #6366f1; }
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

      /* Context menu */
      .context-menu {
        position: fixed;
        background: white;
        border: 1px solid #d1d5db;
        border-radius: 6px;
        box-shadow: 0 4px 12px rgba(0,0,0,0.15);
        padding: 4px 0;
        min-width: 180px;
        z-index: 1000;
        font-size: 12px;
      }
      .context-menu-item {
        padding: 6px 12px;
        cursor: pointer;
        display: block;
        width: 100%;
        border: none;
        background: none;
        text-align: left;
        font-size: 12px;
        font-family: inherit;
        color: #374151;
      }
      .context-menu-item:hover { background: #f3f4f6; }
      .context-menu-item:focus-visible { background: #eef2ff; outline: none; }
      .context-menu-separator {
        height: 1px;
        background: #e5e7eb;
        margin: 4px 0;
      }
    `,
  ];

  @property({ type: Object }) root: SchemaNode | null = null;
  @property({ type: String }) selectedId = '';
  @property({ type: String }) draft: string | null = null;

  @state() private _expanded = new Set<string>();
  @state() private _draggedNodeId: string | null = null;
  @state() private _dropTargetId: string | null = null;
  @state() private _contextMenu: { x: number; y: number; nodeId: string } | null = null;

  override connectedCallback() {
    super.connectedCallback();
    // Expand root by default
    if (this.root) {
      this._expanded.add(this.root.id);
    }
    // Close context menu on outside click
    this._boundCloseContextMenu = this._closeContextMenu.bind(this);
    document.addEventListener('click', this._boundCloseContextMenu);
  }

  override disconnectedCallback() {
    super.disconnectedCallback();
    document.removeEventListener('click', this._boundCloseContextMenu);
  }

  private _boundCloseContextMenu: (() => void) = () => {};

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
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      const items = this.shadowRoot?.querySelectorAll('[role="treeitem"]');
      if (items) {
        const arr = Array.from(items);
        const idx = arr.indexOf(e.target as Element);
        if (idx >= 0 && idx < arr.length - 1) {
          const next = arr[idx + 1] as HTMLElement;
          next.focus();
          const nodeId = next.dataset.nodeId;
          if (nodeId) this._selectNode(nodeId);
        }
      }
    }
    if (e.key === 'ArrowUp') {
      e.preventDefault();
      const items = this.shadowRoot?.querySelectorAll('[role="treeitem"]');
      if (items) {
        const arr = Array.from(items);
        const idx = arr.indexOf(e.target as Element);
        if (idx > 0) {
          const prev = arr[idx - 1] as HTMLElement;
          prev.focus();
          const nodeId = prev.dataset.nodeId;
          if (nodeId) this._selectNode(nodeId);
        }
      }
    }
    if (e.key === 'Home') {
      e.preventDefault();
      const first = this.shadowRoot?.querySelector('[role="treeitem"]') as HTMLElement;
      if (first) {
        first.focus();
        const nodeId = first.dataset.nodeId;
        if (nodeId) this._selectNode(nodeId);
      }
    }
    if (e.key === 'End') {
      e.preventDefault();
      const items = this.shadowRoot?.querySelectorAll('[role="treeitem"]');
      if (items?.length) {
        const last = items[items.length - 1] as HTMLElement;
        last.focus();
        const nodeId = last.dataset.nodeId;
        if (nodeId) this._selectNode(nodeId);
      }
    }
  }

  private _addProperty() {
    this.dispatchEvent(new CustomEvent('add-property', { bubbles: true, composed: true }));
  }

  private _addDefs() {
    this.dispatchEvent(new CustomEvent('add-defs', { bubbles: true, composed: true }));
  }

  // ─── Drag and drop ──────────────────────────────────────────────────────────

  private _handleDragStart(e: DragEvent, nodeId: string) {
    this._draggedNodeId = nodeId;
    if (e.dataTransfer) {
      e.dataTransfer.effectAllowed = 'move';
      e.dataTransfer.setData('text/plain', nodeId);
    }
  }

  private _handleDragOver(e: DragEvent, nodeId: string) {
    e.preventDefault();
    if (this._draggedNodeId && this._draggedNodeId !== nodeId) {
      this._dropTargetId = nodeId;
    }
  }

  private _handleDragLeave() {
    this._dropTargetId = null;
  }

  private _handleDrop(e: DragEvent, nodeId: string) {
    e.preventDefault();
    this._dropTargetId = null;
    if (this._draggedNodeId && this._draggedNodeId !== nodeId) {
      this.dispatchEvent(new CustomEvent('reorder', {
        detail: { draggedId: this._draggedNodeId, targetId: nodeId },
        bubbles: true,
        composed: true,
      }));
    }
    this._draggedNodeId = null;
  }

  private _handleDragEnd() {
    this._draggedNodeId = null;
    this._dropTargetId = null;
  }

  // ─── Context menu ───────────────────────────────────────────────────────────

  private _handleContextMenu(e: MouseEvent, nodeId: string) {
    e.preventDefault();
    this._contextMenu = { x: e.clientX, y: e.clientY, nodeId };
  }

  private _closeContextMenu() {
    if (this._contextMenu) {
      this._contextMenu = null;
    }
  }

  private _contextMenuAction(action: string) {
    if (!this._contextMenu) return;
    const { nodeId } = this._contextMenu;
    this._contextMenu = null;

    this.dispatchEvent(new CustomEvent('context-action', {
      detail: { nodeId, action },
      bubbles: true,
      composed: true,
    }));
  }

  // ─── Badge helpers ──────────────────────────────────────────────────────────

  private _getBadgeClass(node: SchemaNode): string {
    if (node.isDef) return 'badge badge-def';
    if (node.ref) return 'badge badge-ref';
    if (node.compositionKeyword) return 'badge badge-composition';
    if (node.schema?.enum) return 'badge badge-enum';
    if (node.schema?.if) return 'badge badge-conditional';
    return `badge badge-${node.type || 'string'}`;
  }

  private _getBadgeText(node: SchemaNode): string {
    if (node.isDef) return node.type || 'object';
    if (node.ref) return '$ref';
    if (node.compositionKeyword) return node.compositionKeyword;
    if (node.schema?.enum) return 'enum';
    if (node.schema?.if) return 'if/then';
    return node.type || 'string';
  }

  private _hasChildren(node: SchemaNode): boolean {
    const propChildren = (node.children || []).filter(c => c.name !== '$defs');
    const variantChildren = node.variants || [];
    return propChildren.length > 0 || variantChildren.length > 0;
  }

  private _renderNode(node: SchemaNode, isRoot = false): unknown {
    const propChildren = (node.children || []).filter(c => c.name !== '$defs' && !c.isDef);
    const variantChildren = node.variants || [];
    const defsNode = (node.children || []).find(c => c.name === '$defs');
    const expanded = this._expanded.has(node.id);
    const selected = this.selectedId === node.id;
    const hasChildren = propChildren.length > 0 || variantChildren.length > 0;
    const isDropTarget = this._dropTargetId === node.id;

    return html`
      <div
        class=${classMap({ 'tree-node': true, selected, 'drop-target': isDropTarget })}
        data-node-id=${node.id}
        role="treeitem"
        tabindex=${selected ? '0' : '-1'}
        aria-selected=${selected}
        aria-expanded=${hasChildren ? String(expanded) : nothing}
        @click=${() => this._selectNode(node.id)}
        @dblclick=${() => hasChildren && this._toggleExpand(node.id)}
        @keydown=${(e: KeyboardEvent) => this._handleKeydown(e, node)}
        @contextmenu=${(e: MouseEvent) => this._handleContextMenu(e, node.id)}
        @dragover=${(e: DragEvent) => this._handleDragOver(e, node.id)}
        @dragleave=${() => this._handleDragLeave()}
        @drop=${(e: DragEvent) => this._handleDrop(e, node.id)}
        @dragend=${() => this._handleDragEnd()}
      >
        ${hasChildren
          ? html`<span class="expand-icon" aria-hidden="true" @click=${(e: Event) => { e.stopPropagation(); this._toggleExpand(node.id); }}>${expanded ? '\u25BC' : '\u25B6'}</span>`
          : html`<span class="expand-icon" aria-hidden="true"></span>`}
        ${!isRoot ? html`<span
          class="drag-handle"
          aria-hidden="true"
          draggable="true"
          @dragstart=${(e: DragEvent) => this._handleDragStart(e, node.id)}
        >\u2630</span>` : ''}
        <span class="node-name">${isRoot ? 'root' : node.name}</span>
        ${node.required ? html`<span class="required-marker" aria-label="required">*</span>` : ''}
        <span class=${this._getBadgeClass(node)}>${this._getBadgeText(node)}</span>
      </div>
      ${hasChildren && expanded
        ? html`<div class="children" role="group">
            ${repeat(propChildren, c => c.id, c => this._renderNode(c))}
            ${repeat(variantChildren, c => c.id, c => this._renderNode(c))}
          </div>`
        : ''}
      ${isRoot && defsNode && defsNode.children?.length
        ? html`
          <div class="defs-section">
            <div class="defs-header">
              <span class="expand-icon" aria-hidden="true" @click=${() => this._toggleExpand(defsNode.id)}>${this._expanded.has(defsNode.id) ? '\u25BC' : '\u25B6'}</span>
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

  private _renderContextMenu(): unknown {
    if (!this._contextMenu) return nothing;
    const { x, y } = this._contextMenu;
    return html`
      <div class="context-menu" style="left:${x}px;top:${y}px" @click=${(e: Event) => e.stopPropagation()}>
        <button class="context-menu-item" @click=${() => this._contextMenuAction('wrap-oneOf')}>Wrap in oneOf</button>
        <button class="context-menu-item" @click=${() => this._contextMenuAction('wrap-anyOf')}>Wrap in anyOf</button>
        <button class="context-menu-item" @click=${() => this._contextMenuAction('wrap-allOf')}>Wrap in allOf</button>
        <div class="context-menu-separator"></div>
        <button class="context-menu-item" @click=${() => this._contextMenuAction('add-if-then-else')}>Add if/then/else</button>
        <div class="context-menu-separator"></div>
        <button class="context-menu-item" @click=${() => this._contextMenuAction('convert-to-ref')}>Convert to $ref</button>
      </div>
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
      ${this._renderContextMenu()}
    `;
  }
}
