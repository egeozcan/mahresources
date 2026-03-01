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
    this.announce('Edit tags panel opened');

    // Ensure resource details are loaded (reuses editPanel cache)
    this.fetchResourceDetails();
  },

  closeQuickTagPanel() {
    this.quickTagPanelOpen = false;
    this._saveQuickTagsToStorage();

    // If both panels are closed and changes were made, refresh page content
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
    this._saveQuickTagsToStorage();

    // Dismiss any open popovers in the quick-tag panel (autocompleter dropdowns)
    document.querySelectorAll('[data-quick-tag-panel] [popover]').forEach(p => {
      try { p.hidePopover(); } catch {}
    });
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

  async focusTagEditor() {
    if (!this.quickTagPanelOpen) {
      this.openQuickTagPanel();
    }
    // Wait for resource details to load (input is inside x-if="resourceDetails")
    await this.fetchResourceDetails();
    // Poll for the input element (Alpine needs a tick to render the template)
    const poll = (attempts) => {
      const panel = document.querySelector('[data-quick-tag-panel]');
      const input = panel?.querySelector('[data-tag-editor-input]');
      if (input) {
        input.focus();
      } else if (attempts > 0) {
        requestAnimationFrame(() => poll(attempts - 1));
      }
    };
    requestAnimationFrame(() => poll(10));
  },

  // ==================== Keyboard Shortcut Label ====================

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
