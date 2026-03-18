// src/components/blocks/blockText.js
// Note: editMode is accessed from parent scope via Alpine v3 scope merging in the template
export function blockText(block, saveFn, saveDebouncedFn) {
  return {
    block,
    saveFn,
    saveDebouncedFn,
    text: block?.content?.text || '',

    // Called on input for debounced auto-save
    // Named onBlockInput to avoid collision with mentionTextarea's onInput
    onBlockInput() {
      if (this.saveDebouncedFn) {
        this.saveDebouncedFn(this.block.id, { text: this.text });
      }
    },

    // Called on blur for immediate save
    // Named saveBlock to avoid collision with inner scope names
    saveBlock() {
      this.saveFn(this.block.id, { text: this.text });
    }
  };
}
