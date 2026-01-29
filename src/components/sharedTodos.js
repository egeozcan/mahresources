// src/components/sharedTodos.js
// A simplified todos component for shared notes that only allows checking/unchecking items
// (no add/remove/edit functionality) and syncs state with the share server.

export function sharedTodos(blockId, initialState, shareToken) {
  return {
    blockId,
    shareToken,
    checked: [...(initialState?.checked || [])],
    saving: false,
    error: null,

    isChecked(itemId) {
      return this.checked.includes(itemId);
    },

    async toggleItem(itemId) {
      // Optimistic update
      const wasChecked = this.isChecked(itemId);
      if (wasChecked) {
        this.checked = this.checked.filter(id => id !== itemId);
      } else {
        this.checked = [...this.checked, itemId];
      }

      // Sync with server
      this.saving = true;
      this.error = null;

      try {
        const response = await fetch(`/s/${this.shareToken}/block/${this.blockId}/state`, {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
          },
          body: JSON.stringify({ checked: this.checked }),
        });

        if (!response.ok) {
          throw new Error(`Failed to save: ${response.status}`);
        }
      } catch (err) {
        // Rollback on error
        if (wasChecked) {
          this.checked = [...this.checked, itemId];
        } else {
          this.checked = this.checked.filter(id => id !== itemId);
        }
        this.error = err.message;
        console.error('Failed to save todo state:', err);
      } finally {
        this.saving = false;
      }
    }
  };
}
