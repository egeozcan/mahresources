// src/components/blocks/blockGallery.js
// editMode is passed as a getter function to maintain reactivity with parent scope
export function blockGallery(block, saveContentFn, getEditMode) {
  return {
    block,
    saveContentFn,
    getEditMode,
    resourceIds: [...(block?.content?.resourceIds || [])],

    get editMode() {
      return this.getEditMode ? this.getEditMode() : false;
    },

    updateResourceIds(value) {
      // Parse comma-separated IDs
      this.resourceIds = value
        .split(',')
        .map(s => parseInt(s.trim(), 10))
        .filter(n => !isNaN(n) && n > 0);
      this.saveContentFn(this.block.id, { resourceIds: this.resourceIds });
    },

    addResources(ids) {
      this.resourceIds = [...new Set([...this.resourceIds, ...ids])];
      this.saveContentFn(this.block.id, { resourceIds: this.resourceIds });
    },

    removeResource(id) {
      this.resourceIds = this.resourceIds.filter(rid => rid !== id);
      this.saveContentFn(this.block.id, { resourceIds: this.resourceIds });
    }
  };
}
