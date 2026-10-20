// src/components/blocks/blockText.js
// Note: editMode is accessed from parent scope via Alpine v3 scope merging in the template
export function blockText(block, saveFn, saveDebouncedFn) {
  return {
    block,
    saveFn,
    saveDebouncedFn,
    text: block?.content?.text || '',

    // Called on input for debounced auto-save. markdownEditor writes every
    // edit back to the textarea and fires input there.
    onBlockInput() {
      if (this.saveDebouncedFn) {
        this.saveDebouncedFn(this.block.id, { text: this.text });
      }
    },

    // Called when focus leaves the block's editor, for an immediate save.
    // Named saveBlock to avoid collision with inner scope names
    saveBlock() {
      this.saveFn(this.block.id, { text: this.text });
    }
  };
}
