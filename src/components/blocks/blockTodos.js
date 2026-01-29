// src/components/blocks/blockTodos.js
// editMode is passed as a getter function to maintain reactivity with parent scope
export function blockTodos(block, saveContentFn, saveStateFn, getEditMode) {
  return {
    block,
    saveContentFn,
    saveStateFn,
    getEditMode,
    items: [...(block?.content?.items || [])],
    checked: [...(block?.state?.checked || [])],

    get editMode() {
      return this.getEditMode ? this.getEditMode() : false;
    },

    isChecked(itemId) {
      return this.checked.includes(itemId);
    },

    toggleCheck(itemId) {
      if (this.isChecked(itemId)) {
        this.checked = this.checked.filter(id => id !== itemId);
      } else {
        this.checked = [...this.checked, itemId];
      }
      this.saveStateFn(this.block.id, { checked: this.checked });
    },

    saveContent() {
      this.saveContentFn(this.block.id, { items: this.items });
    },

    addItem() {
      const newItem = { id: crypto.randomUUID(), label: 'New item' };
      this.items = [...this.items, newItem];
      this.saveContentFn(this.block.id, { items: this.items });
    },

    removeItem(idx) {
      const removedItem = this.items[idx];
      this.items = this.items.filter((_, i) => i !== idx);
      if (removedItem) {
        this.checked = this.checked.filter(id => id !== removedItem.id);
      }
      this.saveContentFn(this.block.id, { items: this.items });
      this.saveStateFn(this.block.id, { checked: this.checked });
    }
  };
}
