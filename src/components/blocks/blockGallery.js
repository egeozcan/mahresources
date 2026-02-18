// src/components/blocks/blockGallery.js
// editMode is passed as a getter function to maintain reactivity with parent scope
export function blockGallery(block, saveContentFn, getEditMode, noteId) {
  return {
    block,
    saveContentFn,
    getEditMode,
    noteId,
    resourceIds: [...(block?.content?.resourceIds || [])],
    resourceMeta: {}, // Cache for resource metadata (contentType, name, hash)

    get editMode() {
      return this.getEditMode ? this.getEditMode() : false;
    },

    async init() {
      // Fetch metadata for all resources to enable lightbox
      await this.fetchResourceMeta();
    },

    async fetchResourceMeta() {
      if (this.resourceIds.length === 0) return;

      // Fetch metadata for resources we don't have yet
      const toFetch = this.resourceIds.filter(id => !this.resourceMeta[id]);
      if (toFetch.length === 0) return;

      // Fetch in chunks to limit concurrent requests
      const chunkSize = 5;
      try {
        for (let i = 0; i < toFetch.length; i += chunkSize) {
          const chunk = toFetch.slice(i, i + chunkSize);
          const results = await Promise.all(
            chunk.map(id =>
              fetch(`/v1/resource?id=${id}`).then(r => r.ok ? r.json() : null)
            )
          );
          results.forEach((res, j) => {
            if (res) {
              this.resourceMeta[chunk[j]] = {
                contentType: res.ContentType || '',
                name: res.Name || '',
                hash: res.Hash || ''
              };
            }
          });
        }
      } catch (err) {
        console.warn('Failed to fetch resource metadata for gallery:', err);
      }
    },

    openPicker() {
      const picker = Alpine.store('entityPicker');
      if (!picker) {
        console.error('entityPicker store not found');
        return;
      }
      picker.open({
        entityType: 'resource',
        noteId: this.noteId,
        existingIds: this.resourceIds,
        onConfirm: (selectedIds) => {
          this.addResources(selectedIds);
        }
      });
    },

    openGalleryLightbox(index) {
      const lightbox = Alpine.store('lightbox');
      if (!lightbox) return;

      // Build items array from resourceIds with metadata
      const items = this.resourceIds.map(id => {
        const meta = this.resourceMeta[id] || {};
        const hash = meta.hash || '';
        const versionParam = hash ? `&v=${hash}` : '';
        return {
          id,
          viewUrl: `/v1/resource/view?id=${id}${versionParam}`,
          contentType: meta.contentType || 'image/jpeg', // Default to image
          name: meta.name || '',
          hash: hash
        };
      }).filter(item =>
        item.contentType?.startsWith('image/') ||
        item.contentType?.startsWith('video/')
      );

      if (items.length === 0) return;

      // Set items and open lightbox
      lightbox.items = items;
      lightbox.loadedPages = new Set([1]);
      lightbox.hasNextPage = false;
      lightbox.hasPrevPage = false;
      lightbox.open(index);
    },

    updateResourceIds(value) {
      // Parse comma-separated IDs
      this.resourceIds = value
        .split(',')
        .map(s => parseInt(s.trim(), 10))
        .filter(n => !isNaN(n) && n > 0);
      this.saveContentFn(this.block.id, { resourceIds: this.resourceIds });
      // Fetch metadata for any new resources
      this.fetchResourceMeta();
    },

    addResources(ids) {
      this.resourceIds = [...new Set([...this.resourceIds, ...ids])];
      this.saveContentFn(this.block.id, { resourceIds: this.resourceIds });
      this.fetchResourceMeta();
    },

    removeResource(id) {
      this.resourceIds = this.resourceIds.filter(rid => rid !== id);
      this.saveContentFn(this.block.id, { resourceIds: this.resourceIds });
    }
  };
}
