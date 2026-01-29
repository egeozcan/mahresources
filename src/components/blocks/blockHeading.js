// src/components/blocks/blockHeading.js
// Note: editMode is accessed from parent scope via $parent.editMode in the template
export function blockHeading(block, saveFn, saveDebouncedFn) {
  return {
    block,
    saveFn,
    saveDebouncedFn,
    text: block?.content?.text || '',
    level: block?.content?.level || 2,

    // Called on input for debounced auto-save
    onInput() {
      if (this.saveDebouncedFn) {
        this.saveDebouncedFn(this.block.id, { text: this.text, level: this.level });
      }
    },

    // Called on blur or select change for immediate save
    save() {
      this.saveFn(this.block.id, { text: this.text, level: this.level });
    }
  };
}
