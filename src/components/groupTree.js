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
        const p = document.createElement('p');
        p.className = 'text-gray-500 p-4';
        p.textContent = 'No groups found.';
        container.replaceChildren(p);
        return;
      }

      const ul = document.createElement('ul');
      ul.className = 'tree-chart-list';
      for (const node of rootNodes) {
        ul.appendChild(this.renderNode(node, true));
      }

      container.replaceChildren(ul);
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

      const li = document.createElement('li');
      if (isRoot) li.className = 'tree-root-node';

      const a = document.createElement('a');
      a.href = `/group?id=${node.id}`;
      a.className = boxClass;
      a.title = node.name || '';

      const nameSpan = document.createElement('span');
      nameSpan.className = 'tree-node-name';
      nameSpan.textContent = node.name || '';
      a.appendChild(nameSpan);

      if (node.categoryName) {
        const catSpan = document.createElement('span');
        catSpan.className = 'tree-node-category';
        catSpan.textContent = node.categoryName;
        a.appendChild(catSpan);
      }

      li.appendChild(a);

      if (hasChildren) {
        if (isLoading) {
          const btn = document.createElement('button');
          btn.className = 'tree-node-expand';
          btn.disabled = true;
          btn.textContent = 'Loading...';
          li.appendChild(btn);
        } else if (isExpanded && children.length > 0) {
          const btn = document.createElement('button');
          btn.className = 'tree-node-expand';
          btn.dataset.nodeId = node.id;
          btn.dataset.action = 'collapse';

          const arrow = document.createElement('span');
          arrow.className = 'tree-node-arrow tree-node-arrow--down';
          btn.appendChild(arrow);
          btn.appendChild(document.createTextNode(` ${node.childCount}`));
          li.appendChild(btn);

          const childUl = document.createElement('ul');
          childUl.className = 'tree-chart-list';
          for (const child of children) {
            childUl.appendChild(this.renderNode(child, false));
          }

          // If we loaded fewer children than the total, show a "more" link
          if (children.length < node.childCount) {
            const moreLi = document.createElement('li');
            const moreA = document.createElement('a');
            moreA.href = `/groups?OwnerId=${node.id}`;
            moreA.className = 'tree-node-more';
            moreA.textContent = `+${node.childCount - children.length} more...`;
            moreLi.appendChild(moreA);
            childUl.appendChild(moreLi);
          }

          li.appendChild(childUl);
        } else {
          const btn = document.createElement('button');
          btn.className = 'tree-node-expand';
          btn.dataset.nodeId = node.id;
          btn.dataset.action = 'expand';

          const arrow = document.createElement('span');
          arrow.className = 'tree-node-arrow';
          btn.appendChild(arrow);
          btn.appendChild(document.createTextNode(` ${node.childCount}`));
          li.appendChild(btn);
        }
      }

      return li;
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

        const { abort, ready } = abortableFetch(`/v1/group/tree/children?parentId=${nodeId}&limit=50`);
        this.requestAborter = abort;

        const res = await ready;
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
    }
  };
}
