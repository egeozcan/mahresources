import { abortableFetch } from '../index.js';

export function groupTree({ initialRows = [], highlightedPath = null, containingId = 0, rootId = 0 }) {
  const pathArray = highlightedPath || [];
  return {
    tree: {},              // Map<parentId|'root', GroupTreeNode[]>
    expandedNodes: new Set(),
    loadingNodes: new Set(),
    highlightedSet: new Set(pathArray),
    containingId,
    rootId,
    requestAborter: null,

    init() {
      this.buildTree(initialRows);

      // Auto-expand nodes on the highlighted path
      for (const id of pathArray) {
        this.expandedNodes.add(id);
      }

      this.render();
    },

    buildTree(rows) {
      for (const row of rows) {
        const parentKey = row.ownerId || 'root';
        if (!this.tree[parentKey]) {
          this.tree[parentKey] = [];
        }
        // Avoid duplicates
        if (!this.tree[parentKey].some(n => n.id === row.id)) {
          this.tree[parentKey].push(row);
        }
      }
    },

    render() {
      const container = this.$refs.treeContainer;
      if (!container) return;

      // Find root node(s)
      const rootNodes = this.tree['root'] || this.tree[0] || [];

      if (rootNodes.length === 0) {
        container.innerHTML = '<p class="text-gray-500 p-4">No groups found.</p>';
        return;
      }

      let html = '<ul class="tree-chart-list">';
      for (const node of rootNodes) {
        html += this.renderNode(node, true);
      }
      html += '</ul>';

      container.innerHTML = html;
    },

    renderNode(node, isRoot) {
      const isHighlighted = this.highlightedSet.has(node.id);
      const isFocused = node.id === this.containingId;
      const isExpanded = this.expandedNodes.has(node.id);
      const isLoading = this.loadingNodes.has(node.id);
      const children = this.tree[node.id] || [];
      const hasChildren = node.childCount > 0;

      let boxClass = 'tree-node-box';
      if (isFocused) boxClass += ' tree-node-box--focused';
      else if (isHighlighted) boxClass += ' tree-node-box--path';

      let html = `<li class="${isRoot ? 'tree-root-node' : ''}">`;
      html += `<a href="/group?id=${node.id}" class="${boxClass}" title="${this.escapeHtml(node.name)}">`;
      html += `<span class="tree-node-name">${this.escapeHtml(node.name)}</span>`;
      if (node.categoryName) {
        html += `<span class="tree-node-category">${this.escapeHtml(node.categoryName)}</span>`;
      }
      html += '</a>';

      if (hasChildren) {
        if (isLoading) {
          html += '<button class="tree-node-expand" disabled>Loading...</button>';
        } else if (isExpanded && children.length > 0) {
          html += `<button class="tree-node-expand" data-node-id="${node.id}" data-action="collapse">`;
          html += `<span class="tree-node-arrow tree-node-arrow--down"></span> ${node.childCount}`;
          html += '</button>';

          html += '<ul class="tree-chart-list">';
          for (const child of children) {
            html += this.renderNode(child, false);
          }

          // If we loaded fewer children than the total, show a "more" link
          if (children.length < node.childCount) {
            html += `<li><a href="/groups?OwnerId=${node.id}" class="tree-node-more">+${node.childCount - children.length} more...</a></li>`;
          }

          html += '</ul>';
        } else {
          html += `<button class="tree-node-expand" data-node-id="${node.id}" data-action="expand">`;
          html += `<span class="tree-node-arrow"></span> ${node.childCount}`;
          html += '</button>';
        }
      }

      html += '</li>';
      return html;
    },

    handleClick(e) {
      const btn = e.target.closest('[data-node-id]');
      if (!btn) return;

      e.preventDefault();
      const nodeId = parseInt(btn.dataset.nodeId, 10);
      const action = btn.dataset.action;

      if (action === 'collapse') {
        this.expandedNodes.delete(nodeId);
        this.render();
      } else if (action === 'expand') {
        this.expandNode(nodeId);
      }
    },

    async expandNode(nodeId) {
      this.expandedNodes.add(nodeId);

      // If we already have children loaded, just re-render
      if (this.tree[nodeId] && this.tree[nodeId].length > 0) {
        this.render();
        return;
      }

      // Fetch children
      this.loadingNodes.add(nodeId);
      this.render();

      try {
        if (this.requestAborter) {
          this.requestAborter();
        }

        const [response, abort] = abortableFetch(`/v1/group/tree/children?parentId=${nodeId}&limit=50`);
        this.requestAborter = abort;

        const res = await response;
        if (!res.ok) throw new Error('Failed to load children');

        const children = await res.json();

        // Add to tree
        this.tree[nodeId] = children;
      } catch (err) {
        if (err.name !== 'AbortError') {
          console.error('Failed to load tree children:', err);
        }
      } finally {
        this.loadingNodes.delete(nodeId);
        this.requestAborter = null;
        this.render();
      }
    },

    escapeHtml(str) {
      if (!str) return '';
      const div = document.createElement('div');
      div.textContent = str;
      return div.innerHTML;
    }
  };
}
