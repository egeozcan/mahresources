// src/components/lightbox/quickTagPanel.js

const STORAGE_KEY = 'mahresources_quickTags';

/**
 * Quick tag panel state/methods for the lightbox store.
 * All methods use `this` which is bound to the Alpine store.
 */
export const quickTagPanelState = {
  quickTagPanelOpen: false,
  quickTagSlots: Array(10).fill(null), // [{id, name} | null] x 10
  _quickTagTogglingIds: new Set(),
};

export const quickTagPanelMethods = {
  // ==================== Persistence ====================

  _loadQuickTagsFromStorage() {
    try {
      const raw = localStorage.getItem(STORAGE_KEY);
      if (!raw) return;
      const data = JSON.parse(raw);
      if (Array.isArray(data.slots) && data.slots.length === 10) {
        this.quickTagSlots = data.slots;
      }
      if (typeof data.drawerOpen === 'boolean') {
        this.quickTagPanelOpen = data.drawerOpen;
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
    this.announce('Quick tag panel opened');

    // Ensure resource details are loaded (reuses editPanel cache)
    this.fetchResourceDetails();
  },

  closeQuickTagPanel() {
    this.quickTagPanelOpen = false;
    this._saveQuickTagsToStorage();
    this.announce('Quick tag panel closed');
  },

  // ==================== Slot Management ====================

  setQuickTagSlot(index, tag) {
    // tag = { ID: number, Name: string } or null
    this.quickTagSlots[index] = tag ? { id: tag.ID, name: tag.Name } : null;
    // Force Alpine reactivity on array
    this.quickTagSlots = [...this.quickTagSlots];
    this._saveQuickTagsToStorage();
  },

  clearQuickTagSlot(index) {
    this.setQuickTagSlot(index, null);
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

  // ==================== Keyboard Shortcut Label ====================

  quickTagKeyLabel(index) {
    // index 0-8 → '1'-'9', index 9 → '0'
    return index < 9 ? String(index + 1) : '0';
  },
};
