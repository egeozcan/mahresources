// src/components/lightbox/quickTagPanel.js

const STORAGE_KEY = 'mahresources_quickTags';

/**
 * Quick tag panel state/methods for the lightbox store.
 * All methods use `this` which is bound to the Alpine store.
 */
export const quickTagPanelState = {
  quickTagPanelOpen: false,
  quickTagSlots: Array(9).fill(null), // [{id, name} | null] x 9
  _quickTagTogglingIds: new Set(), // Not Alpine-reactive; used only as a guard in toggleQuickTag, not in templates
  recentTags: Array(9).fill(null), // [{id, name, ts} | null] x 9
};

export const quickTagPanelMethods = {
  // ==================== Persistence ====================

  _loadQuickTagsFromStorage() {
    try {
      const raw = localStorage.getItem(STORAGE_KEY);
      if (!raw) return;
      const data = JSON.parse(raw);
      if (Array.isArray(data.slots)) {
        const slots = data.slots.slice(0, 9);
        while (slots.length < 9) slots.push(null);
        this.quickTagSlots = slots;
      }
      if (typeof data.drawerOpen === 'boolean') {
        this.quickTagPanelOpen = data.drawerOpen;
      }
      if (Array.isArray(data.recentTags)) {
        const recent = data.recentTags.slice(0, 9);
        while (recent.length < 9) recent.push(null);
        this.recentTags = recent;
      }
    } catch {
      // Corrupted data — ignore
    }
  },

  _saveQuickTagsToStorage() {
    try {
      localStorage.setItem(STORAGE_KEY, JSON.stringify({
        slots: this.quickTagSlots,
        drawerOpen: this.quickTagPanelOpen,
        recentTags: this.recentTags,
      }));
    } catch {
      // Storage full or unavailable — ignore
    }
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
    // tag = { ID: number, Name: string } or null
    this.quickTagSlots[index] = tag ? { id: tag.ID, name: tag.Name } : null;
    // Force Alpine reactivity on array
    this.quickTagSlots = [...this.quickTagSlots];
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
    // Skip if this tag is in a quick-add slot
    if (this.quickTagSlots.some(s => s && s.id === tag.ID)) return;

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

  async toggleRecentTag(index) {
    const recent = this.recentTags[index];
    if (!recent) return;

    const tagId = recent.id;
    if (this._quickTagTogglingIds.has(tagId)) return;

    this._quickTagTogglingIds.add(tagId);
    try {
      const tag = { ID: tagId, Name: recent.name };

      if (this.isTagOnResource(tagId)) {
        await this.saveTagRemoval(tag);
      } else {
        await this.saveTagAddition(tag);
      }
    } finally {
      this._quickTagTogglingIds.delete(tagId);
    }
  },

  hasRecentTags() {
    return this.recentTags.some(r => r !== null);
  },

  recentTagKeyLabel(index) {
    return 'Shift+' + String(index + 1);
  },

  // ==================== Tag Toggle ====================

  isTagOnResource(tagId) {
    return (this.resourceDetails?.Tags || []).some(t => t.ID === tagId);
  },

  async toggleQuickTag(index) {
    const slot = this.quickTagSlots[index];
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
    // Resource details are fetched by editPanel's onResourceChange or by openQuickTagPanel.
    // The template reactively reads resourceDetails.Tags, so no extra work needed.
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
