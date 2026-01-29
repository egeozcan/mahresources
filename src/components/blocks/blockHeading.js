// src/components/blocks/blockHeading.js
export function blockHeading() {
  return {
    get text() {
      return this.block?.content?.text || '';
    },
    get level() {
      return this.block?.content?.level || 2;
    },
    updateHeading(text, level) {
      this.$dispatch('update-content', { text, level: parseInt(level) });
    }
  };
}
