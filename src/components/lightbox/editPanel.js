import { abortableFetch } from '../../index.js';

/**
 * Edit panel state/methods for the lightbox store.
 * All methods use `this` which is bound to the Alpine store.
 */
export const editPanelState = {
  // Edit panel state
  editPanelOpen: false,
  resourceDetails: null,
  detailsLoading: false,
  detailsCache: new Map(),
  detailsAborter: null,

  // Tag editing
  savingTag: false,

  // Track if changes were made that require refreshing the page content
  needsRefreshOnClose: false,
};

export const editPanelMethods = {
  handleEscape() {
    if (this.editPanelOpen) {
      this.closeEditPanel();
      return true;
    }
    if (this.isFullscreen) {
      this.toggleFullscreen();
      return true;
    }
    this.close();
    return true;
  },

  async openEditPanel() {
    this.editPanelOpen = true;
    this.needsRefreshOnClose = false;
    this.announce('Edit panel opened');
    await this.fetchResourceDetails();

    requestAnimationFrame(() => {
      const panel = document.querySelector('[data-edit-panel]');
      if (panel) {
        const firstInput = panel.querySelector('input, textarea');
        if (firstInput) {
          firstInput.focus();
        }
      }
    });
  },

  closeEditPanel() {
    this.editPanelOpen = false;

    if (this.detailsAborter) {
      this.detailsAborter();
      this.detailsAborter = null;
    }

    this.resourceDetails = null;

    if (this.needsRefreshOnClose) {
      this.needsRefreshOnClose = false;
      this.refreshPageContent();
    }

    this.announce('Edit panel closed');
  },

  async refreshPageContent() {
    const listContainer = document.querySelector('.list-container, .items-container');
    if (!listContainer) return;

    try {
      const url = new URL(window.location);
      url.pathname = url.pathname + '.body';

      const response = await fetch(url.toString());
      if (!response.ok) return;

      const html = await response.text();
      const parser = new DOMParser();
      const doc = parser.parseFromString(html, 'text/html');
      const newListContainer = doc.querySelector('.list-container, .items-container');

      if (newListContainer && window.Alpine) {
        const scrollX = window.scrollX;
        const scrollY = window.scrollY;

        window.Alpine.morph(listContainer, newListContainer, {
          updating(el, toEl, childrenOnly, skip) {
            if (el._x_dataStack) {
              toEl._x_dataStack = el._x_dataStack;
            }
          }
        });

        window.scrollTo(scrollX, scrollY);
        this.updateItemsFromDOM();
      }
    } catch (err) {
      console.error('Failed to refresh page content:', err);
    }
  },

  updateItemsFromDOM() {
    const container = document.querySelector('.list-container, .gallery');
    if (!container) return;

    const links = container.querySelectorAll('[data-lightbox-item]');
    const domItems = new Map();

    links.forEach(link => {
      const id = parseInt(link.dataset.resourceId, 10);
      const contentType = link.dataset.contentType || '';
      if (contentType.startsWith('image/') || contentType.startsWith('video/')) {
        const hash = link.dataset.resourceHash || '';
        const versionParam = hash ? `&v=${hash}` : '';
        domItems.set(id, {
          id,
          viewUrl: `/v1/resource/view?id=${id}${versionParam}`,
          contentType,
          name: link.dataset.resourceName || link.querySelector('img')?.alt || '',
          hash,
        });
      }
    });

    for (let i = 0; i < this.items.length; i++) {
      const updated = domItems.get(this.items[i].id);
      if (updated) {
        this.items[i] = { ...this.items[i], ...updated };
      }
    }
  },

  async fetchResourceDetails(id) {
    const resourceId = id ?? this.getCurrentItem()?.id;
    if (!resourceId) return;

    const cached = this.detailsCache.get(resourceId);
    if (cached) {
      this.resourceDetails = cached;
      return;
    }

    this.detailsLoading = true;

    if (this.detailsAborter) {
      this.detailsAborter();
    }

    try {
      const { abort, ready } = abortableFetch(`/resource.json?id=${resourceId}`);
      this.detailsAborter = abort;

      const response = await ready;
      if (!response.ok) {
        throw new Error(`Failed to fetch resource: ${response.status}`);
      }

      const data = await response.json();
      const fetchedDetails = data.resource || data;

      if (this.getCurrentItem()?.id === resourceId) {
        this.resourceDetails = fetchedDetails;
        this.detailsCache.set(resourceId, fetchedDetails);
      }
      this.detailsAborter = null;
    } catch (err) {
      if (err.name !== 'AbortError') {
        console.error('Failed to fetch resource details:', err);
        this.announce('Failed to load resource details');
      }
    } finally {
      this.detailsLoading = false;
    }
  },

  async onResourceChange() {
    if (!this.editPanelOpen) return;

    const focused = document.activeElement;
    const panel = document.querySelector('[data-edit-panel]');
    let focusSelector = null;
    if (focused && panel?.contains(focused)) {
      if (focused.id) {
        focusSelector = `#${focused.id}`;
      } else if (focused.matches('input[placeholder]')) {
        focusSelector = `input[placeholder="${focused.getAttribute('placeholder')}"]`;
      }
    }

    this.resourceDetails = null;
    const resourceId = this.getCurrentItem()?.id;
    if (resourceId) {
      this.detailsCache.delete(resourceId);
    }
    await this.fetchResourceDetails();

    if (focusSelector) {
      requestAnimationFrame(() => {
        const el = document.querySelector(`[data-edit-panel] ${focusSelector}`);
        if (el) el.focus();
      });
    }
  },

  async updateName(newName) {
    const resourceId = this.getCurrentItem()?.id;
    if (!resourceId || !this.resourceDetails) return;

    const oldName = this.resourceDetails.Name;
    if (newName === oldName) return;

    this.resourceDetails.Name = newName;

    const item = this.items[this.currentIndex];
    if (item) {
      item.name = newName;
    }

    try {
      const formData = new FormData();
      formData.append('Name', newName);

      const response = await fetch(`/v1/resource/editName?id=${resourceId}`, {
        method: 'POST',
        body: formData,
        headers: { 'Accept': 'application/json' }
      });

      if (!response.ok) {
        throw new Error(`Failed to update name: ${response.status}`);
      }

      this.detailsCache.set(resourceId, { ...this.resourceDetails });
      this.needsRefreshOnClose = true;
      this.announce('Name updated');
    } catch (err) {
      console.error('Failed to update name:', err);
      this.resourceDetails.Name = oldName;
      if (item) {
        item.name = oldName;
      }
      this.announce('Failed to update name');
    }
  },

  async updateDescription(newDescription) {
    const resourceId = this.getCurrentItem()?.id;
    if (!resourceId || !this.resourceDetails) return;

    const oldDescription = this.resourceDetails.Description;
    if (newDescription === oldDescription) return;

    this.resourceDetails.Description = newDescription;

    try {
      const formData = new FormData();
      formData.append('Description', newDescription);

      const response = await fetch(`/v1/resource/editDescription?id=${resourceId}`, {
        method: 'POST',
        body: formData,
        headers: { 'Accept': 'application/json' }
      });

      if (!response.ok) {
        throw new Error(`Failed to update description: ${response.status}`);
      }

      this.detailsCache.set(resourceId, { ...this.resourceDetails });
      this.needsRefreshOnClose = true;
      this.announce('Description updated');
    } catch (err) {
      console.error('Failed to update description:', err);
      this.resourceDetails.Description = oldDescription;
      this.announce('Failed to update description');
    }
  },

  // ==================== Tag API Methods ====================

  async saveTagAddition(tag) {
    const resourceId = this.getCurrentItem()?.id;
    if (!resourceId || this.savingTag) return;

    this.savingTag = true;

    if (this.resourceDetails) {
      if (!this.resourceDetails.Tags) {
        this.resourceDetails.Tags = [];
      }
      if (!this.resourceDetails.Tags.some(t => t.ID === tag.ID)) {
        this.resourceDetails.Tags.push(tag);
      }
    }

    try {
      const formData = new FormData();
      formData.append('ID', resourceId);
      formData.append('EditedId', tag.ID);

      const response = await fetch('/v1/resources/addTags', {
        method: 'POST',
        body: formData,
        headers: { 'Accept': 'application/json' }
      });

      if (!response.ok) {
        throw new Error(`Failed to add tag: ${response.status}`);
      }

      if (this.resourceDetails) {
        this.detailsCache.set(resourceId, { ...this.resourceDetails });
      }
      this.needsRefreshOnClose = true;
      this.announce(`Added tag: ${tag.Name}`);
    } catch (err) {
      console.error('Failed to add tag:', err);
      if (this.resourceDetails?.Tags) {
        const idx = this.resourceDetails.Tags.findIndex(t => t.ID === tag.ID);
        if (idx !== -1) {
          this.resourceDetails.Tags.splice(idx, 1);
        }
      }
      this.announce('Failed to add tag');
      throw err;
    } finally {
      this.savingTag = false;
    }
  },

  async saveTagRemoval(tag) {
    const resourceId = this.getCurrentItem()?.id;
    if (!resourceId) return;

    if (this.resourceDetails?.Tags) {
      const idx = this.resourceDetails.Tags.findIndex(t => t.ID === tag.ID);
      if (idx !== -1) {
        this.resourceDetails.Tags.splice(idx, 1);
      }
    }

    try {
      const formData = new FormData();
      formData.append('ID', resourceId);
      formData.append('EditedId', tag.ID);

      const response = await fetch('/v1/resources/removeTags', {
        method: 'POST',
        body: formData,
        headers: { 'Accept': 'application/json' }
      });

      if (!response.ok) {
        throw new Error(`Failed to remove tag: ${response.status}`);
      }

      if (this.resourceDetails) {
        this.detailsCache.set(resourceId, { ...this.resourceDetails });
      }
      this.needsRefreshOnClose = true;
      this.announce(`Removed tag: ${tag.Name}`);
    } catch (err) {
      console.error('Failed to remove tag:', err);
      if (this.resourceDetails?.Tags) {
        this.resourceDetails.Tags.push(tag);
      }
      this.announce('Failed to remove tag');
      throw err;
    }
  },

  getCurrentTags() {
    return this.resourceDetails?.Tags || [];
  },
};
