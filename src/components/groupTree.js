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
    requestAborters: new Map(),

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

      // Preserve the current tabindex=0 treeitem's group id across re-renders
      // so keyboard users don't get bounced back to the first node on expand.
      const prevStop = container.querySelector('li[role="treeitem"][tabindex="0"]');
      const prevStopId = prevStop ? prevStop.getAttribute('data-group-id') : null;

      const ul = document.createElement('ul');
      ul.className = 'tree-chart-list';
      ul.setAttribute('role', 'tree');
      ul.setAttribute('aria-label', 'Group hierarchy');

      rootNodes.forEach((node, idx) => {
        ul.appendChild(this.renderNode(node, true, {
          level: 1,
          posinset: idx + 1,
          setsize: rootNodes.length,
        }));
      });

      container.replaceChildren(ul);

      // BH-029: roving tabindex — exactly one treeitem is tab-stoppable.
      this._applyRovingTabindex(container, prevStopId);
    },

    _applyRovingTabindex(container, preferredId = null) {
      const treeitems = container.querySelectorAll('li[role="treeitem"]');
      if (treeitems.length === 0) return;
      treeitems.forEach(li => li.setAttribute('tabindex', '-1'));
      let target = null;
      if (preferredId) {
        target = container.querySelector(`li[role="treeitem"][data-group-id="${preferredId}"]`);
      }
      if (!target) target = treeitems[0];
      target.setAttribute('tabindex', '0');
    },

    renderNode(node, isRoot, { level = 1, posinset = 1, setsize = 1 } = {}) {
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

      // BH-029: WAI-ARIA Tree View pattern
      li.setAttribute('role', 'treeitem');
      li.setAttribute('aria-level', String(level));
      li.setAttribute('aria-posinset', String(posinset));
      li.setAttribute('aria-setsize', String(setsize));
      li.setAttribute('data-group-id', String(node.id));
      if (hasChildren) {
        li.setAttribute('aria-expanded', isExpanded ? 'true' : 'false');
      }

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
        const childLabel = node.childCount === 1 ? '1 child' : `${node.childCount} children`;
        if (isLoading) {
          const btn = document.createElement('button');
          btn.className = 'tree-node-expand';
          btn.disabled = true;
          btn.setAttribute('aria-label', `Loading children of ${node.name || 'group'}`);
          btn.textContent = 'Loading...';
          li.appendChild(btn);
        } else if (isExpanded && children.length > 0) {
          const btn = document.createElement('button');
          btn.className = 'tree-node-expand';
          btn.dataset.nodeId = node.id;
          btn.dataset.action = 'collapse';
          btn.setAttribute('aria-expanded', 'true');
          btn.setAttribute('aria-label', `Collapse ${node.name || 'group'}, ${childLabel}`);

          const arrow = document.createElement('span');
          arrow.className = 'tree-node-arrow tree-node-arrow--down';
          arrow.setAttribute('aria-hidden', 'true');
          btn.appendChild(arrow);
          btn.appendChild(document.createTextNode(` ${node.childCount}`));
          li.appendChild(btn);

          const childUl = document.createElement('ul');
          childUl.className = 'tree-chart-list';
          // Nested groups per WAI-ARIA Tree View: role="group".
          childUl.setAttribute('role', 'group');
          children.forEach((child, idx) => {
            childUl.appendChild(this.renderNode(child, false, {
              level: level + 1,
              posinset: idx + 1,
              setsize: children.length,
            }));
          });

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
          btn.setAttribute('aria-expanded', 'false');
          btn.setAttribute('aria-label', `Expand ${node.name || 'group'}, ${childLabel}`);

          const arrow = document.createElement('span');
          arrow.className = 'tree-node-arrow';
          arrow.setAttribute('aria-hidden', 'true');
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

    /**
     * BH-029: WAI-ARIA Tree View keyboard pattern.
     * https://www.w3.org/WAI/ARIA/apg/patterns/treeview/
     *
     * Implemented keys:
     *   ArrowUp / ArrowDown — move focus between visible treeitems
     *   ArrowRight          — expand (if collapsed) or move to first child (if expanded)
     *   ArrowLeft           — collapse (if expanded) or move to parent
     *   Home / End          — first / last visible treeitem
     */
    handleKeyDown(e) {
      const target = e.target.closest('li[role="treeitem"]');
      if (!target) return;
      const container = this.$refs.treeContainer;
      if (!container || !container.contains(target)) return;

      const all = Array.from(container.querySelectorAll('li[role="treeitem"]'));
      const idx = all.indexOf(target);
      if (idx < 0) return;

      let next = null;
      switch (e.key) {
        case 'ArrowDown':
          next = all[idx + 1] || target;
          break;
        case 'ArrowUp':
          next = all[idx - 1] || target;
          break;
        case 'Home':
          next = all[0];
          break;
        case 'End':
          next = all[all.length - 1];
          break;
        case 'ArrowRight': {
          const expanded = target.getAttribute('aria-expanded');
          if (expanded === 'false') {
            const nodeId = parseInt(target.dataset.groupId, 10);
            if (!Number.isNaN(nodeId)) {
              e.preventDefault();
              this.expandNode(nodeId);
            }
            return;
          }
          if (expanded === 'true') {
            const firstChild = target.querySelector(':scope > ul > li[role="treeitem"]');
            if (firstChild) next = firstChild;
          }
          break;
        }
        case 'ArrowLeft': {
          const expanded = target.getAttribute('aria-expanded');
          if (expanded === 'true') {
            const nodeId = parseInt(target.dataset.groupId, 10);
            if (!Number.isNaN(nodeId)) {
              e.preventDefault();
              this.expandedNodes.delete(nodeId);
              this.render();
              const refocused = container.querySelector(`li[role="treeitem"][data-group-id="${nodeId}"]`);
              if (refocused) {
                // _applyRovingTabindex already set tabindex=0 on the same node via preferredId.
                refocused.focus();
              }
            }
            return;
          }
          const parent = target.parentElement?.closest('li[role="treeitem"]');
          if (parent) next = parent;
          break;
        }
        default:
          return;
      }

      if (next && next !== target) {
        e.preventDefault();
        all.forEach(li => li.setAttribute('tabindex', '-1'));
        next.setAttribute('tabindex', '0');
        next.focus();
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
        if (this.requestAborters.has(nodeId)) {
          this.requestAborters.get(nodeId)();
        }

        const { abort, ready } = abortableFetch(`/v1/group/tree/children?parentId=${nodeId}&limit=50`);
        this.requestAborters.set(nodeId, abort);

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
        this.requestAborters.delete(nodeId);
        this.render();
      }
    }
  };
}
