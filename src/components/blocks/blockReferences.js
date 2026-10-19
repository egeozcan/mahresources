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
    _metaGeneration: 0,

    get editMode() {
      return this.getEditMode ? this.getEditMode() : false;
    },

    async init() {
      await this.fetchGroupMeta();
    },

    async fetchGroupMeta() {
      if (this.groupIds.length === 0) return;

      // A slower, older response is dropped: it could only mark a group added
      // since as unavailable and overwrite that group's real metadata.
      const generation = ++this._metaGeneration;
      const requested = [...this.groupIds];
      this.loadingMeta = true;
      try {
        const meta = await fetchEntityMeta('group', requested);
        if (generation !== this._metaGeneration) return;
        // Mark groups that were fetched but returned no data (deleted / 404) as unavailable
        for (const id of requested) {
          if (!meta[id]) {
            meta[id] = { __unavailable: true };
          }
        }
        this.groupMeta = meta;
      } catch (err) {
        console.warn('Failed to fetch group metadata:', err);
      } finally {
        if (generation === this._metaGeneration) this.loadingMeta = false;
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
      if (!meta) return { name: `Group ${id}`, breadcrumb: '', unavailable: false };
      if (meta.__unavailable) return { name: `Group #${id} unavailable`, breadcrumb: '', unavailable: true };
      return {
        name: meta.name || `Group ${id}`,
        breadcrumb: meta.breadcrumb || '',
        unavailable: false
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
