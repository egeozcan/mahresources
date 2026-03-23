// src/components/lightbox/quickTagPanel.js

const STORAGE_KEY = 'mahresources_quickTags';

const TAB_LABELS = [
  { name: 'QUICK 1', key: 'Z' },
  { name: 'QUICK 2', key: 'X' },
  { name: 'QUICK 3', key: 'C' },
  { name: 'QUICK 4', key: 'V' },
  { name: 'RECENT',  key: 'B' },
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
  activeTab: 0, // 0-3=QUICK, 4=RECENT
  quickSlots: [
    Array(9).fill(null),
    Array(9).fill(null),
    Array(9).fill(null),
    Array(9).fill(null),
  ],
  _quickTagTogglingSlot: null,
  editingSlotIndex: null,
  expandedSlotIndex: null,
  _longPressTimer: null,
  _longPressThreshold: 400,
  _longPressSlotIdx: null, // tracks which slot started the long press (for progress bar)
  _expandedClickOutsideHandler: null,
  recentTags: Array(9).fill(null),
  tabLabels: TAB_LABELS,
};

export const quickTagPanelMethods = {
  // ==================== Persistence ====================

  _loadQuickTagsFromStorage() {
    try {
      const raw = localStorage.getItem(STORAGE_KEY);
      if (!raw) return;
      const data = JSON.parse(raw);

      // Migration v1 → v2: flat `slots` array to nested quickSlots
      if (Array.isArray(data.slots) && !Array.isArray(data.quickSlots)) {
        data.quickSlots = [
          padArray(data.slots, 9),
          Array(9).fill(null),
          Array(9).fill(null),
        ];
        data.activeTab = 0;
        data.version = 2;
      }

      // Migration v2 → v3: single-tag slots to multi-tag arrays, 3→4 tabs, remap activeTab
      if (!data.version || data.version < 3) {
        if (Array.isArray(data.quickSlots)) {
          // Wrap each non-null single-tag {id, name} in [{ id, name }]
          data.quickSlots = data.quickSlots.map(tab =>
            (tab || []).map(slot => slot && !Array.isArray(slot) ? [slot] : slot)
          );
          // Extend from 3 to 4 inner arrays
          while (data.quickSlots.length < 4) {
            data.quickSlots.push(Array(9).fill(null));
          }
        }
        // Remap activeTab: v2 3(RECENT)→4, v2 4(LAST)→0
        if (data.activeTab === 3) data.activeTab = 4;
        else if (data.activeTab === 4) data.activeTab = 0;
        data.version = 3;
      }

      // Load each field independently
      try {
        if (Array.isArray(data.quickSlots)) {
          this.quickSlots = [
            padArray(data.quickSlots[0], 9),
            padArray(data.quickSlots[1], 9),
            padArray(data.quickSlots[2], 9),
            padArray(data.quickSlots[3], 9),
          ];
        }
      } catch (e) {
        console.warn('Failed to load quickSlots from storage:', e);
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
    } catch (e) {
      console.warn('Failed to load quick tags from storage:', e);
    }
  },

  _saveQuickTagsToStorage() {
    const payload = JSON.stringify({
      version: 3,
      quickSlots: this.quickSlots,
      drawerOpen: this.quickTagPanelOpen,
      activeTab: this.activeTab,
      recentTags: this.recentTags,
    });
    try {
      localStorage.setItem(STORAGE_KEY, payload);
    } catch (e) {
      console.warn('Failed to save quick tags to localStorage:', e);
      try {
        const date = new Date().toISOString().slice(0, 10);
        localStorage.setItem(`${STORAGE_KEY}_recover_${date}`, payload);
      } catch { /* recovery save also failed — nothing more to do */ }
    }
  },

  _initStorageSync() {
    window.addEventListener('storage', (event) => {
      if (event.key === STORAGE_KEY) {
        this._loadQuickTagsFromStorage();
      }
    });
  },

  // ==================== Tab Management ====================

  switchTab(tabIndex) {
    if (tabIndex < 0 || tabIndex > 4) return;
    if (this.expandedSlotIndex !== null) {
      this.expandedSlotIndex = null;
      this._cancelLongPress();
    }
    this.activeTab = tabIndex;
    this.editingSlotIndex = null;
    this._saveQuickTagsToStorage();
    this.announce(`Switched to ${TAB_LABELS[tabIndex].name} tab`);
  },

  getActiveTabSlots() {
    if (this.activeTab < 4) return this.quickSlots[this.activeTab];
    return this.recentTags;
  },

  isQuickTab() {
    return this.activeTab < 4;
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
    this.editingSlotIndex = null;
    this.expandedSlotIndex = null;
    this._cancelLongPress();
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

  addTagToSlot(index, tag) {
    if (!this.isQuickTab()) return;
    const tabIdx = this.activeTab;
    // tag = { ID: number, Name: string }
    if (!tag) return;
    const entry = { id: tag.ID, name: tag.Name };
    const current = this.quickSlots[tabIdx][index];
    if (current) {
      // Skip if tag already in slot
      if (current.some(t => t.id === tag.ID)) return;
      current.push(entry);
    } else {
      this.quickSlots[tabIdx][index] = [entry];
    }
    // Force Alpine reactivity
    this.quickSlots = [...this.quickSlots];
    // Remove from recents if this tag was there
    const recentIdx = this.recentTags.findIndex(r => r && r.id === tag.ID);
    if (recentIdx !== -1) {
      this.recentTags[recentIdx] = null;
      this.recentTags = [...this.recentTags];
    }
    this._saveQuickTagsToStorage();

    // Dismiss any open popovers in the quick-tag panel
    document.querySelectorAll('[data-quick-tag-panel] [popover]').forEach(p => {
      try { p.hidePopover(); } catch {}
    });
  },

  removeTagFromSlot(index, tagId) {
    if (!this.isQuickTab()) return;
    const tabIdx = this.activeTab;
    const current = this.quickSlots[tabIdx][index];
    if (!current) return;
    const filtered = current.filter(t => t.id !== tagId);
    this.quickSlots[tabIdx][index] = filtered.length > 0 ? filtered : null;
    this.quickSlots = [...this.quickSlots];
    this._saveQuickTagsToStorage();
  },

  clearQuickTagSlot(index) {
    if (!this.isQuickTab()) return;
    const tabIdx = this.activeTab;
    this.quickSlots[tabIdx][index] = null;
    this.quickSlots = [...this.quickSlots];
    this._saveQuickTagsToStorage();
  },

  // ==================== Recent Tags ====================

  recordRecentTag(tag) {
    // tag = { ID: number, Name: string }
    // Skip if this tag is in any quick-add slot
    if (this.quickSlots.some(slots => slots.some(s => s && s.some(t => t.id === tag.ID)))) return;

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

  // ==================== Tag Toggle ====================

  isTagOnResource(tagId) {
    return (this.resourceDetails?.Tags || []).some(t => t.ID === tagId);
  },

  slotMatchState(index) {
    const slots = this.getActiveTabSlots();
    const slot = slots[index];
    if (!slot) return 'none';
    if (!this.resourceDetails) return 'none';

    // Normalize: RECENT entries are single {id, name, ts}, QUICK entries are arrays
    const tags = Array.isArray(slot) ? slot : [slot];
    if (tags.length === 0) return 'none';

    const presentCount = tags.filter(t => this.isTagOnResource(t.id ?? t.ID)).length;
    if (presentCount === tags.length) return 'all';
    if (presentCount > 0) return 'some';
    return 'none';
  },

  async toggleTabTag(index) {
    const slots = this.getActiveTabSlots();
    const slot = slots[index];
    if (!slot) return;

    if (this._quickTagTogglingSlot === index) return;
    this._quickTagTogglingSlot = index;

    try {
      // Normalize: RECENT entries are {id, name, ts}, QUICK entries are [{id, name}, ...]
      const tags = (Array.isArray(slot) ? slot : [slot]).map(t => ({
        ID: t.id ?? t.ID,
        Name: t.name ?? t.Name,
      }));

      const state = this.slotMatchState(index);

      if (state === 'all') {
        await this._batchToggleTags(tags, 'remove');
      } else {
        const missing = tags.filter(tag => !this.isTagOnResource(tag.ID));
        if (missing.length > 0) {
          await this._batchToggleTags(missing, 'add');
        }
      }
    } finally {
      this._quickTagTogglingSlot = null;
    }
  },

  async _batchToggleTags(tags, action) {
    const resourceId = this.getCurrentItem()?.id;
    if (!resourceId) return;

    const endpoint = action === 'add' ? '/v1/resources/addTags' : '/v1/resources/removeTags';

    // Optimistic UI update
    if (this.resourceDetails) {
      if (!this.resourceDetails.Tags) this.resourceDetails.Tags = [];
      for (const tag of tags) {
        if (action === 'add') {
          if (!this.resourceDetails.Tags.some(t => t.ID === tag.ID)) {
            this.resourceDetails.Tags.push(tag);
          }
        } else {
          const idx = this.resourceDetails.Tags.findIndex(t => t.ID === tag.ID);
          if (idx !== -1) this.resourceDetails.Tags.splice(idx, 1);
        }
      }
    }

    try {
      const formData = new FormData();
      formData.append('ID', resourceId);
      for (const tag of tags) {
        formData.append('EditedId', tag.ID);
      }

      const response = await fetch(endpoint, {
        method: 'POST',
        body: formData,
        headers: { 'Accept': 'application/json' },
      });

      if (!response.ok) {
        throw new Error(`Failed to ${action} tags: ${response.status}`);
      }

      if (this.resourceDetails) {
        this.detailsCache.set(resourceId, { ...this.resourceDetails });
      }
      this.needsRefreshOnClose = true;

      const names = tags.map(t => t.Name).join(', ');
      this.announce(`${action === 'add' ? 'Added' : 'Removed'} tags: ${names}`);

      // Record each as recent tag
      if (action === 'add') {
        for (const tag of tags) {
          this.recordRecentTag(tag);
        }
      }
    } catch (err) {
      console.error(`Failed to ${action} tags:`, err);
      // Roll back optimistic update
      this.detailsCache.delete(resourceId);
      await this.fetchResourceDetails();
      this.announce(`Failed to ${action} tags`);
    }
  },

  // ==================== Resource Change Hook ====================

  onQuickTagResourceChange() {
    if (!this.quickTagPanelOpen) return;
    if (this.expandedSlotIndex !== null) {
      this.expandedSlotIndex = null;
      this._cancelLongPress();
    }
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

  // ==================== Slot Expansion ====================

  isExpanded() {
    return this.expandedSlotIndex !== null;
  },

  expandedTags() {
    if (this.expandedSlotIndex === null) return [];
    const slot = this.getActiveTabSlots()[this.expandedSlotIndex];
    if (!slot) return [];
    const tags = Array.isArray(slot) ? slot : [slot];
    return tags.slice(0, 9);
  },

  collapseExpanded() {
    if (this.expandedSlotIndex === null) return;
    this.expandedSlotIndex = null;
    this._cancelLongPress();
    if (this._expandedClickOutsideHandler) {
      document.removeEventListener('click', this._expandedClickOutsideHandler, true);
      this._expandedClickOutsideHandler = null;
    }
    this.announce('Back to quick slots');
  },

  _expandSlot(index) {
    this.expandedSlotIndex = index;
    this._longPressTimer = null;
    this._longPressSlotIdx = null;
    const tags = this.expandedTags();
    const label = this.quickTagKeyLabel(index);
    this.announce(`Expanded slot ${label}: ${tags.length} tags. Press Escape to go back.`);
  },

  _cancelLongPress() {
    if (this._longPressTimer) {
      clearTimeout(this._longPressTimer);
      this._longPressTimer = null;
    }
    this._longPressSlotIdx = null;
  },

  _slotTagCount(index) {
    const slots = this.getActiveTabSlots();
    const slot = slots[index];
    if (!slot) return 0;
    return Array.isArray(slot) ? slot.length : 1;
  },

  // ==================== Expanded Tag Toggle ====================

  async toggleExpandedTag(index) {
    const tags = this.expandedTags();
    if (index >= tags.length) return;
    const tag = tags[index];
    const tagObj = { ID: tag.id ?? tag.ID, Name: tag.name ?? tag.Name };
    const isOn = this.isTagOnResource(tagObj.ID);
    await this._batchToggleTags([tagObj], isOn ? 'remove' : 'add');
  },

  // ==================== Keyboard Dispatch ====================

  handleSlotKeydown(idx, event) {
    // Guard against key repeat
    if (event.repeat && this._longPressTimer) return;

    if (this.isExpanded()) {
      // In expanded mode: toggle individual tag at this index
      this.toggleExpandedTag(idx);
      return;
    }

    const tagCount = this._slotTagCount(idx);
    if (tagCount <= 1) {
      // Single-tag or empty: fire immediately (existing behavior)
      this.toggleTabTag(idx);
      return;
    }

    // Multi-tag: start long-press timer
    this._longPressSlotIdx = idx;
    this._longPressTimer = setTimeout(() => {
      this._expandSlot(idx);
    }, this._longPressThreshold);
  },

  handleSlotKeyup(idx) {
    if (this.isExpanded()) return; // expansion already happened

    const tagCount = this._slotTagCount(idx);
    if (tagCount <= 1) return; // already fired on keydown

    if (this._longPressTimer) {
      // Short press: cancel timer, fire batch toggle
      this._cancelLongPress();
      this.toggleTabTag(idx);
    }
  },

  // ==================== Mouse Dispatch ====================

  handleSlotMousedown(idx) {
    if (this.isExpanded()) return; // in expanded mode, click on slot cards toggles individually

    const tagCount = this._slotTagCount(idx);
    if (tagCount <= 1) return; // single-tag: normal click handler fires

    this._longPressSlotIdx = idx;
    this._longPressTimer = setTimeout(() => {
      this._expandSlot(idx);
    }, this._longPressThreshold);
  },

  handleSlotMouseup(idx) {
    if (this.isExpanded()) return;

    const tagCount = this._slotTagCount(idx);
    if (tagCount <= 1) return;

    if (this._longPressTimer) {
      this._cancelLongPress();
      this.toggleTabTag(idx);
    }
  },

  handleSlotMouseleave(idx) {
    if (this._longPressTimer) {
      this._cancelLongPress();
    }
  },

  _setupExpandedClickOutside() {
    // Called via x-effect when isExpanded() changes
    if (this._expandedClickOutsideHandler) {
      document.removeEventListener('click', this._expandedClickOutsideHandler, true);
      this._expandedClickOutsideHandler = null;
    }
    if (this.isExpanded()) {
      this._expandedClickOutsideHandler = (e) => {
        const panel = document.querySelector('[data-quick-tag-panel]');
        if (panel && !panel.contains(e.target)) {
          this.collapseExpanded();
        }
      };
      // Use capture + nextTick to avoid triggering on the same click that caused expansion
      setTimeout(() => {
        if (this._expandedClickOutsideHandler) {
          document.addEventListener('click', this._expandedClickOutsideHandler, true);
        }
      }, 0);
    }
  },
};
