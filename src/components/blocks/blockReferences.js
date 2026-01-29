// src/components/blocks/blockReferences.js
// editMode is passed as a getter function to maintain reactivity with parent scope
export function blockReferences(block, saveContentFn, getEditMode) {
  return {
    block,
    saveContentFn,
    getEditMode,
    groupIds: [...(block?.content?.groupIds || [])],

    get editMode() {
      return this.getEditMode ? this.getEditMode() : false;
    },

    updateGroupIds(value) {
      // Parse comma-separated IDs
      this.groupIds = value
        .split(',')
        .map(s => parseInt(s.trim(), 10))
        .filter(n => !isNaN(n) && n > 0);
      this.saveContentFn(this.block.id, { groupIds: this.groupIds });
    },

    addGroups(ids) {
      this.groupIds = [...new Set([...this.groupIds, ...ids])];
      this.saveContentFn(this.block.id, { groupIds: this.groupIds });
    },

    removeGroup(id) {
      this.groupIds = this.groupIds.filter(gid => gid !== id);
      this.saveContentFn(this.block.id, { groupIds: this.groupIds });
    }
  };
}
