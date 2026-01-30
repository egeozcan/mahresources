// src/components/blocks/blockReferences.js
import { fetchEntityMeta } from '../picker/index.js';

// editMode is passed as a getter function to maintain reactivity with parent scope
export function blockReferences(block, saveContentFn, getEditMode) {
  return {
    block,
    saveContentFn,
    getEditMode,
    groupIds: [...(block?.content?.groupIds || [])],
    groupMeta: {},
    loadingMeta: false,

    get editMode() {
      return this.getEditMode ? this.getEditMode() : false;
    },

    async init() {
      await this.fetchGroupMeta();
    },

    async fetchGroupMeta() {
      if (this.groupIds.length === 0) return;

      this.loadingMeta = true;
      try {
        this.groupMeta = await fetchEntityMeta('group', this.groupIds);
      } catch (err) {
        console.warn('Failed to fetch group metadata:', err);
      } finally {
        this.loadingMeta = false;
      }
    },

    openPicker() {
      const picker = Alpine.store('entityPicker');
      if (!picker) {
        console.error('entityPicker store not found');
        return;
      }
      picker.open({
        entityType: 'group',
        existingIds: this.groupIds,
        onConfirm: (selectedIds) => {
          this.addGroups(selectedIds);
        }
      });
    },

    getGroupDisplay(id) {
      const meta = this.groupMeta[id];
      if (!meta) return { name: `Group ${id}`, breadcrumb: '' };
      return {
        name: meta.name || `Group ${id}`,
        breadcrumb: meta.breadcrumb || ''
      };
    },

    addGroups(ids) {
      this.groupIds = [...new Set([...this.groupIds, ...ids])];
      this.saveContentFn(this.block.id, { groupIds: this.groupIds });
      this.fetchGroupMeta();
    },

    removeGroup(id) {
      this.groupIds = this.groupIds.filter(gid => gid !== id);
      this.saveContentFn(this.block.id, { groupIds: this.groupIds });
    }
  };
}
