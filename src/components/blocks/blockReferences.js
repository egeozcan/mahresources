// src/components/blocks/blockReferences.js
export function blockReferences() {
  return {
    get groupIds() {
      return this.block?.content?.groupIds || [];
    },
    addGroups(ids) {
      const newIds = [...new Set([...this.groupIds, ...ids])];
      this.$dispatch('update-content', { groupIds: newIds });
    },
    removeGroup(id) {
      const newIds = this.groupIds.filter(gid => gid !== id);
      this.$dispatch('update-content', { groupIds: newIds });
    }
  };
}
