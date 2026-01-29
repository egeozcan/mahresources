// src/components/blocks/blockText.js
export function blockText() {
  return {
    get text() {
      return this.block?.content?.text || '';
    },
    updateText(newText) {
      this.$dispatch('update-content', { text: newText });
    }
  };
}
