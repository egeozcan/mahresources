// src/components/blocks/blockText.js
// Note: editMode is accessed from parent scope via $parent.editMode in the template
export function blockText(block, saveFn, saveDebouncedFn) {
  return {
    block,
    saveFn,
    saveDebouncedFn,
    text: block?.content?.text || '',

    // Called on input for debounced auto-save
    onInput() {
      if (this.saveDebouncedFn) {
        this.saveDebouncedFn(this.block.id, { text: this.text });
      }
    },

    // Called on blur for immediate save
    save() {
      this.saveFn(this.block.id, { text: this.text });
    }
  };
}
