// src/components/lightbox/quickTagPanel.js

const STORAGE_KEY = 'mahresources_quickTags';

const TAB_LABELS = [
  { name: 'QUICK 1', key: 'Z' },
  { name: 'QUICK 2', key: 'X' },
  { name: 'QUICK 3', key: 'C' },
  { name: 'RECENT',  key: 'V' },
  { name: 'LAST',    key: 'B' },
];

function padArray(arr, len) {
  const result = (arr || []).slice(0, len);
  while (result.length < len) result.push(null);
  return result;
}

/**
 * Quick tag panel state/methods for the lightbox store.
 * All methods use `this` which is bound to the Alpine store.
 */
export const quickTagPanelState = {
  quickTagPanelOpen: false,
  activeTab: 0, // 0=QUICK1, 1=QUICK2, 2=QUICK3, 3=RECENT, 4=LAST
  quickSlots: [
    Array(9).fill(null),
    Array(9).fill(null),
    Array(9).fill(null),
  ],
  _quickTagTogglingIds: new Set(),
  _activeTagResourceId: null, // resource currently being tagged (not reactive)
  _pendingLastTags: null, // latest snapshot of current resource's tags (not reactive)
  _tagsModifiedOnResource: false, // true when tags were changed via the panel (not reactive)
  recentTags: Array(9).fill(null), // [{id, name, ts} | null] x 9
  lastResourceTags: Array(9).fill(null), // [{id, name} | null] x 9 — frozen on resource switch
  tabLabels: TAB_LABELS,
};

export const quickTagPanelMethods = {
  // ==================== Persistence ====================

  _loadQuickTagsFromStorage() {
    try {
      const raw = localStorage.getItem(STORAGE_KEY);
      if (!raw) return;
      const data = JSON.parse(raw);

      // Migration: old schema had flat `slots` array
      if (Array.isArray(data.slots) && !Array.isArray(data.quickSlots)) {
        data.quickSlots = [
          padArray(data.slots, 9),
          Array(9).fill(null),
          Array(9).fill(null),
        ];
        data.activeTab = 0;
        data.lastResourceTags = Array(9).fill(null);
      }

      if (Array.isArray(data.quickSlots)) {
        this.quickSlots = [
          padArray(data.quickSlots[0], 9),
          padArray(data.quickSlots[1], 9),
          padArray(data.quickSlots[2], 9),
        ];
      }
      if (typeof data.drawerOpen === 'boolean') {
        this.quickTagPanelOpen = data.drawerOpen;
      }
      if (typeof data.activeTab === 'number' && data.activeTab >= 0 && data.activeTab <= 4) {
        this.activeTab = data.activeTab;
      }
      if (Array.isArray(data.recentTags)) {
        this.recentTags = padArray(data.recentTags, 9);
      }
      if (Array.isArray(data.lastResourceTags)) {
        this.lastResourceTags = padArray(data.lastResourceTags, 9);
      }
    } catch {
      // Corrupted data — ignore
    }
  },

  _saveQuickTagsToStorage() {
    try {
      localStorage.setItem(STORAGE_KEY, JSON.stringify({
        version: 2,
        quickSlots: this.quickSlots,
        drawerOpen: this.quickTagPanelOpen,
        activeTab: this.activeTab,
        recentTags: this.recentTags,
        lastResourceTags: this.lastResourceTags,
      }));
    } catch {
      // Storage full or unavailable — ignore
    }
  },

  // ==================== Tab Management ====================

  switchTab(tabIndex) {
    if (tabIndex < 0 || tabIndex > 4) return;
    this.activeTab = tabIndex;
    this._saveQuickTagsToStorage();
    this.announce(`Switched to ${TAB_LABELS[tabIndex].name} tab`);
  },

  getActiveTabSlots() {
    if (this.activeTab < 3) return this.quickSlots[this.activeTab];
    if (this.activeTab === 3) return this.recentTags;
    return this.lastResourceTags;
  },

  isQuickTab() {
    return this.activeTab < 3;
  },

  // ==================== Open / Close ====================

  openQuickTagPanel() {
    // Responsive exclusivity: close edit panel on narrow viewports
    if (window.innerWidth < 1024 && this.editPanelOpen) {
      this.closeEditPanel();
    }
    this.quickTagPanelOpen = true;
    this._saveQuickTagsToStorage();
    this.announce('Edit tags panel opened');

    // Ensure resource details are loaded (reuses editPanel cache)
    this.fetchResourceDetails();
  },

  closeQuickTagPanel() {
    this.quickTagPanelOpen = false;
    this._saveQuickTagsToStorage();

    // Only refresh when both panels are closed — the last panel to close triggers the refresh
    if (!this.editPanelOpen && this.needsRefreshOnClose) {
      this.needsRefreshOnClose = false;
      this.refreshPageContent();
    }

    // Clear resource details if edit panel is also closed
    if (!this.editPanelOpen) {
      if (this.detailsAborter) {
        this.detailsAborter();
        this.detailsAborter = null;
      }
      this.resourceDetails = null;
    }

    this.announce('Edit tags panel closed');
  },

  // ==================== Slot Management ====================

  setQuickTagSlot(index, tag) {
    if (!this.isQuickTab()) return;
    const tabIdx = this.activeTab;
    // tag = { ID: number, Name: string } or null
    this.quickSlots[tabIdx][index] = tag ? { id: tag.ID, name: tag.Name } : null;
    // Force Alpine reactivity
    this.quickSlots = [...this.quickSlots];
    // Remove from recents if this tag was there
    if (tag) {
      const recentIdx = this.recentTags.findIndex(r => r && r.id === tag.ID);
      if (recentIdx !== -1) {
        this.recentTags[recentIdx] = null;
        this.recentTags = [...this.recentTags];
      }
    }
    this._saveQuickTagsToStorage();

    // Dismiss any open popovers in the quick-tag panel (autocompleter dropdowns)
    document.querySelectorAll('[data-quick-tag-panel] [popover]').forEach(p => {
      try { p.hidePopover(); } catch {}
    });
  },

  clearQuickTagSlot(index) {
    this.setQuickTagSlot(index, null);
  },

  // ==================== Recent Tags ====================

  recordRecentTag(tag) {
    // tag = { ID: number, Name: string }
    // Skip if this tag is in any quick-add slot
    if (this.quickSlots.some(slots => slots.some(s => s && s.id === tag.ID))) return;

    const now = Date.now();

    // If already in recents, update ts in place
    const existingIdx = this.recentTags.findIndex(r => r && r.id === tag.ID);
    if (existingIdx !== -1) {
      this.recentTags[existingIdx] = { id: tag.ID, name: tag.Name, ts: now };
      this.recentTags = [...this.recentTags];
      this._saveQuickTagsToStorage();
      return;
    }

    // Find the position to replace: first null, or oldest ts
    let targetIdx = this.recentTags.indexOf(null);
    if (targetIdx === -1) {
      // All filled — find oldest (smallest ts)
      targetIdx = 0;
      for (let i = 1; i < this.recentTags.length; i++) {
        if (this.recentTags[i].ts < this.recentTags[targetIdx].ts) {
          targetIdx = i;
        }
      }
    }

    this.recentTags[targetIdx] = { id: tag.ID, name: tag.Name, ts: now };
    this.recentTags = [...this.recentTags];
    this._saveQuickTagsToStorage();
  },

  // ==================== Last Resource Tags ====================

  // Snapshot current resource's tags into the pending buffer (called after tag add/remove)
  _snapshotCurrentTags() {
    const currentId = this.getCurrentItem()?.id;
    if (!currentId) return;

    const tags = (this.resourceDetails?.Tags || []).slice(0, 9);
    const snapshot = Array(9).fill(null);
    tags.forEach((t, i) => {
      snapshot[i] = { id: t.ID, name: t.Name };
    });

    this._activeTagResourceId = currentId;
    this._pendingLastTags = snapshot;
    this._tagsModifiedOnResource = true;
  },

  // Promote pending tags to LAST tab if tags were modified (called on navigation/close)
  _promoteLastTags() {
    if (this._tagsModifiedOnResource && this._pendingLastTags) {
      this.lastResourceTags = this._pendingLastTags;
      this._saveQuickTagsToStorage();
    }
    this._tagsModifiedOnResource = false;
  },

  // ==================== Tag Toggle ====================

  isTagOnResource(tagId) {
    return (this.resourceDetails?.Tags || []).some(t => t.ID === tagId);
  },

  async toggleTabTag(index) {
    const slots = this.getActiveTabSlots();
    const slot = slots[index];
    if (!slot) return;

    const tagId = slot.id;
    if (this._quickTagTogglingIds.has(tagId)) return;

    this._quickTagTogglingIds.add(tagId);
    try {
      const tag = { ID: tagId, Name: slot.name };

      if (this.isTagOnResource(tagId)) {
        await this.saveTagRemoval(tag);
      } else {
        await this.saveTagAddition(tag);
      }
    } finally {
      this._quickTagTogglingIds.delete(tagId);
    }
  },

  // ==================== Resource Change Hook ====================

  onQuickTagResourceChange() {
    if (!this.quickTagPanelOpen) return;
    this.fetchResourceDetails();
  },

  async focusTagEditor() {
    if (!this.quickTagPanelOpen) {
      this.openQuickTagPanel();
    }
    // Wait for resource details to load (input is inside x-if="resourceDetails")
    await this.fetchResourceDetails();
    // Try immediately (e.g. details came from cache), otherwise poll for Alpine to render
    const findAndFocus = () => {
      const panel = document.querySelector('[data-quick-tag-panel]');
      return panel?.querySelector('[data-tag-editor-input]');
    };
    const input = findAndFocus();
    if (input) {
      input.focus();
      return;
    }
    const poll = (attempts) => {
      const el = findAndFocus();
      if (el) {
        el.focus();
      } else if (attempts > 0) {
        requestAnimationFrame(() => poll(attempts - 1));
      }
    };
    requestAnimationFrame(() => poll(10));
  },

  // ==================== Numpad Layout ====================

  // Numpad visual order: top row = 7,8,9 → mid = 4,5,6 → bottom = 1,2,3
  _numpadOrder: [6, 7, 8, 3, 4, 5, 0, 1, 2],

  numpadIndex(visualIndex) {
    return this._numpadOrder[visualIndex];
  },

  quickTagKeyLabel(index) {
    // index 0-8 → '1'-'9'
    return String(index + 1);
  },

  _mediaMaxWidthClass() {
    const bothOpen = this.editPanelOpen && this.quickTagPanelOpen;
    const editOnly = this.editPanelOpen && !this.quickTagPanelOpen;
    const tagsOnly = !this.editPanelOpen && this.quickTagPanelOpen;
    if (bothOpen) return 'lg:max-w-[calc(100vw-690px)] max-w-[90vw]';
    if (editOnly || tagsOnly) return 'lg:max-w-[calc(100vw-450px)] max-w-[90vw]';
    return 'max-w-[90vw]';
  },
};
