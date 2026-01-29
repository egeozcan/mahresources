// src/components/blocks/blockGallery.js
export function blockGallery() {
  return {
    get resourceIds() {
      return this.block?.content?.resourceIds || [];
    },
    get layout() {
      return this.block?.state?.layout || 'grid';
    },
    setLayout(layout) {
      this.$dispatch('update-state', { layout });
    },
    addResources(ids) {
      const newIds = [...new Set([...this.resourceIds, ...ids])];
      this.$dispatch('update-content', { resourceIds: newIds });
    },
    removeResource(id) {
      const newIds = this.resourceIds.filter(rid => rid !== id);
      this.$dispatch('update-content', { resourceIds: newIds });
    }
  };
}
