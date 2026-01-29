// src/components/blocks/blockTodos.js
export function blockTodos() {
  return {
    newItemText: '',

    get items() {
      return this.block?.content?.items || [];
    },
    get checked() {
      return this.block?.state?.checked || [];
    },
    isChecked(itemId) {
      return this.checked.includes(itemId);
    },
    toggleItem(itemId) {
      const newChecked = this.isChecked(itemId)
        ? this.checked.filter(id => id !== itemId)
        : [...this.checked, itemId];
      this.$dispatch('update-state', { checked: newChecked });
    },
    addItem() {
      if (!this.newItemText.trim()) return;
      const newItem = { id: crypto.randomUUID(), label: this.newItemText.trim() };
      const newItems = [...this.items, newItem];
      this.$dispatch('update-content', { items: newItems });
      this.newItemText = '';
    },
    removeItem(itemId) {
      const newItems = this.items.filter(i => i.id !== itemId);
      const newChecked = this.checked.filter(id => id !== itemId);
      this.$dispatch('update-content', { items: newItems });
      this.$dispatch('update-state', { checked: newChecked });
    }
  };
}
