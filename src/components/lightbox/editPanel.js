import { abortableFetch } from '../../index.js';
import { morphOptionsWithShortcodeElements } from '../../utils/shortcodeElementMorph.js';

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
  // Ids whose tag details are being prefetched in the background, so fast paging
  // does not stampede duplicate /resource.json requests for the same resource.
  _detailsInFlight: new Set(),
  detailsAborter: null,
  // Monotonic token so a stale/aborted fetch cannot flip detailsLoading off while a
  // newer fetch is still in flight (BH: M2).
  _detailsReq: 0,

  // Tag editing
  _savingTagIds: new Set(),

  // Track if changes were made that require refreshing the page content
  needsRefreshOnClose: false,
};

export const editPanelMethods = {
  handleEscape() {
    this.close();
    return true;
  },

  async openEditPanel() {
    // Responsive exclusivity: close quick tag panel on narrow viewports
    if (window.innerWidth < 1024 && this.quickTagPanelOpen) {
      this.closeQuickTagPanel();
    }

    this.editPanelOpen = true;
    this.announce('Info panel opened');
    // The panel narrows the media viewport — re-clamp any existing pan so a zoomed image
    // does not slide off-screen (BH: M7). rAF lets the new width class apply first.
    requestAnimationFrame(() => this.constrainPan());
    // Revalidate against the server on (re)open so stale cached details are refreshed (BH: L5).
    await this.fetchResourceDetails(undefined, true);

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
    // The media viewport widens again — re-clamp pan to the new bounds (BH: M7).
    requestAnimationFrame(() => this.constrainPan());

    if (!this.quickTagPanelOpen) {
      if (this.detailsAborter) {
        this.detailsAborter();
        this.detailsAborter = null;
      }
      this.resourceDetails = null;
    }

    // Only refresh when both panels are closed — the last panel to close triggers the refresh
    if (!this.quickTagPanelOpen && this.needsRefreshOnClose) {
      this.needsRefreshOnClose = false;
      this.refreshPageContent();
    }

    this.announce('Info panel closed');
  },

  formatBytes(bytes) {
    const n = Number(bytes);
    if (!Number.isFinite(n) || n <= 0) return '';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.min(sizes.length - 1, Math.floor(Math.log(n) / Math.log(k)));
    return parseFloat((n / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i];
  },

  formatDateTime(value) {
    if (!value) return '';
    const d = value instanceof Date ? value : new Date(value);
    if (Number.isNaN(d.getTime())) return '';
    try {
      return d.toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' });
    } catch {
      return d.toLocaleString();
    }
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

        window.Alpine.morph(listContainer, newListContainer, morphOptionsWithShortcodeElements({
          updating(el, toEl, childrenOnly, skip) {
            if (el._x_dataStack) {
              toEl._x_dataStack = el._x_dataStack;
            }
          }
        }));

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

  async fetchResourceDetails(id, forceRefresh = false) {
    const resourceId = id ?? this.getCurrentItem()?.id;
    if (!resourceId) return;

    const cached = this.detailsCache.get(resourceId);
    if (cached) {
      this.resourceDetails = cached;
      // Fast path: use the cache for an instant paint. When forceRefresh is set (a panel
      // was (re)opened) we still fall through to revalidate against the server so an
      // out-of-band change made elsewhere is not shown stale forever (BH: L5).
      if (!forceRefresh) return;
    }

    // Evict oldest entry if cache exceeds max size
    if (this.detailsCache.size > 100) {
      this.detailsCache.delete(this.detailsCache.keys().next().value);
    }

    const reqId = ++this._detailsReq;
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
      // Only the most recent request may clear the loading flag — an earlier aborted
      // request must not turn the spinner off while this newer one is still pending (BH: M2).
      if (reqId === this._detailsReq) {
        this.detailsLoading = false;
      }
    }
  },

  // Return the live resourceDetails object ONLY when it authoritatively describes
  // resourceId. During a cache-miss navigation load window resourceDetails still holds
  // the PREVIOUS image (onResourceChange deliberately does not blank it to avoid a color
  // flash) while getCurrentItem() already points at the new one. Optimistically mutating
  // that stale object — or caching it under the new id — would poison the cache with the
  // previous image's data (the BH: H5 class of bug). Callers that get null must fall back
  // to a server + cache-invalidation path instead of an optimistic in-place update.
  _currentDetails(resourceId) {
    return this.resourceDetails?.ID === resourceId ? this.resourceDetails : null;
  },

  async onResourceChange() {
    if (!this.editPanelOpen && !this.quickTagPanelOpen) return;

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

    // Snapshot the just-left image's tags for carry-forward (Item 4) BEFORE the refetch
    // below replaces resourceDetails. currentIndex has already advanced, but resourceDetails
    // still holds the previous image here, which is exactly what R should repeat.
    this._snapshotCarryForward();

    // Do NOT blank resourceDetails or evict the incoming resource's cache here.
    // Blanking made every quick-slot color flash neutral on each next/prev
    // (slotMatchState returns 'none' while resourceDetails is null), and evicting
    // the entry we are about to need forced a network round-trip per image.
    // fetchResourceDetails paints instantly on a cache hit (the hit path is fully
    // synchronous) and, on a miss, holds the prior details visible under aria-busy
    // until the fetch resolves. Optimistic tag writes keep the cache correct, and
    // openEditPanel still force-revalidates on explicit (re)open. The post-await id
    // guard in fetchResourceDetails (BH: H5) prevents cross-resource cache poisoning.
    await this.fetchResourceDetails();

    if (focusSelector) {
      requestAnimationFrame(() => {
        const el = document.querySelector(`[data-edit-panel] ${focusSelector}`);
        if (el) el.focus();
      });
    }

    this.onQuickTagResourceChange();
  },

  async updateName(newName) {
    const resourceId = this.getCurrentItem()?.id;
    // Only edit the live details when they authoritatively describe the current resource.
    // During a cache-miss navigation load window resourceDetails still holds the previous
    // image, so writing this edit through would poison its cache entry (BH: H5). After the
    // await the user may also navigate away, which _currentDetails' captured reference plus
    // the resourceId-keyed cache write below likewise guard against.
    const details = this._currentDetails(resourceId);
    if (!resourceId || !details) return;

    const item = this.items[this.currentIndex];

    const oldName = details.Name;
    if (newName === oldName) return;

    details.Name = newName;
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

      this.detailsCache.set(resourceId, { ...details });
      this.needsRefreshOnClose = true;
      this.announce('Name updated');
    } catch (err) {
      console.error('Failed to update name:', err);
      details.Name = oldName;
      if (item) {
        item.name = oldName;
      }
      // The cached copy for this resource is now uncertain — drop it so a later view refetches.
      this.detailsCache.delete(resourceId);
      this.announce('Failed to update name');
    }
  },

  async updateDescription(newDescription) {
    const resourceId = this.getCurrentItem()?.id;
    // Only edit the live details when they belong to the current resource; a cache-miss
    // load window would otherwise misdirect this write onto the previous image (BH: H5).
    const details = this._currentDetails(resourceId);
    if (!resourceId || !details) return;

    const oldDescription = details.Description;
    if (newDescription === oldDescription) return;

    details.Description = newDescription;

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

      this.detailsCache.set(resourceId, { ...details });
      this.needsRefreshOnClose = true;
      this.announce('Description updated');
    } catch (err) {
      console.error('Failed to update description:', err);
      details.Description = oldDescription;
      this.detailsCache.delete(resourceId);
      this.announce('Failed to update description');
    }
  },

  // ==================== Tag API Methods ====================

  async saveTagAddition(tag) {
    const resourceId = this.getCurrentItem()?.id;
    if (!resourceId || this._savingTagIds.has(tag.ID)) return;

    this._savingTagIds.add(tag.ID);

    // Only mutate/cache the live details when they belong to the current resource. During a
    // cache-miss load window resourceDetails still describes the previous image, so caching
    // it under this id would poison the entry; a post-await navigation is likewise guarded by
    // the captured reference and the resourceId-keyed cache write below (BH: H5).
    const details = this._currentDetails(resourceId);
    if (details) {
      if (!details.Tags) {
        details.Tags = [];
      }
      if (!details.Tags.some(t => t.ID === tag.ID)) {
        details.Tags.push(tag);
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

      if (details) {
        this.detailsCache.set(resourceId, { ...details });
      } else {
        // Written during a cache-miss load window (details still described the previous
        // image): an in-flight details fetch for this id may cache a pre-write snapshot, so
        // drop any entry and force a later refetch — mirrors _batchToggleTags' non-current path.
        this.detailsCache.delete(resourceId);
      }
      this.needsRefreshOnClose = true;
      this.announce(`Added tag: ${tag.Name}`);

      // Mirror applySuggestedTag(): drop the now-applied tag from the Suggested row (if
      // showing) and invalidate its cache entry so a later view refetches without it.
      this.suggestedTags = this.suggestedTags.filter(s => s.ID !== tag.ID);
      this._suggestedCache.delete(resourceId);

      // Record as recent tag (skips if in a quick-add slot)
      this.recordRecentTag(tag);
    } catch (err) {
      console.error('Failed to add tag:', err);
      if (details?.Tags) {
        const idx = details.Tags.findIndex(t => t.ID === tag.ID);
        if (idx !== -1) {
          details.Tags.splice(idx, 1);
        }
      }
      this.detailsCache.delete(resourceId);
      this.announce('Failed to add tag');
      throw err;
    } finally {
      this._savingTagIds.delete(tag.ID);
    }
  },

  async saveTagRemoval(tag) {
    const resourceId = this.getCurrentItem()?.id;
    if (!resourceId) return;

    // Only mutate/cache the live details when they belong to the current resource — a
    // cache-miss load window otherwise misdirects this onto the previous image (BH: H5).
    const details = this._currentDetails(resourceId);
    if (details?.Tags) {
      const idx = details.Tags.findIndex(t => t.ID === tag.ID);
      if (idx !== -1) {
        details.Tags.splice(idx, 1);
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

      if (details) {
        this.detailsCache.set(resourceId, { ...details });
      } else {
        // Written during a cache-miss load window: drop any stale entry so a later view
        // refetches the authoritative tag set (mirrors _batchToggleTags' non-current path).
        this.detailsCache.delete(resourceId);
      }
      this.needsRefreshOnClose = true;
      this.announce(`Removed tag: ${tag.Name}`);
    } catch (err) {
      console.error('Failed to remove tag:', err);
      if (details?.Tags && !details.Tags.some(t => t.ID === tag.ID)) {
        details.Tags.push(tag);
      }
      this.detailsCache.delete(resourceId);
      this.announce('Failed to remove tag');
      throw err;
    }
  },

  getCurrentTags() {
    return this.resourceDetails?.Tags || [];
  },
};
