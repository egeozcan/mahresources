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

    async removeItem(idx) {
      const removedItem = this.items[idx];
      this.items = this.items.filter((_, i) => i !== idx);
      if (removedItem) {
        this.checked = this.checked.filter(id => id !== removedItem.id);
      }
      // Sequence the two writes. The content PUT and the state PATCH hit the
      // same row, and each server response carries both fields, so firing them
      // concurrently lets the later response clobber the earlier field
      // (last-write-wins). Awaiting content before state keeps them ordered.
      await this.saveContentFn(this.block.id, { items: this.items });
      await this.saveStateFn(this.block.id, { checked: this.checked });
    }
  };
}
