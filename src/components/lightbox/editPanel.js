import { abortableFetch } from '../../index.js';
import { morphAndReinitChangedComponents } from '../../utils/shortcodeElementMorph.js';
import { findListContainer, LIST_CONTAINER_SELECTOR } from '../../utils/listContainer.js';
import { focusFirstIn, focusOn } from '../../utils/focus.js';

/**
 * Edit panel state/methods for the lightbox store.
 * All methods use `this` which is bound to the Alpine store.
 */
export const editPanelState = {
  // Edit panel state
  editPanelOpen: false,
  resourceDetails: null,
  detailsLoading: false,
  // Id of the resource whose details could not be loaded. With the display fence in place a
  // failed fetch leaves nothing paintable, and without this the panel would sit on its loading
  // placeholder forever — indistinguishable from a slow network, and offering no way out.
  detailsErrorId: null,
  // True while a refresh runs BEHIND details that already describe the resource on screen.
  // Kept apart from detailsLoading, which means "nothing trustworthy is painted yet" and drives
  // the panel's blocking spinner: a stale-while-revalidate refresh happens on every navigation,
  // and flashing that overlay over content the user is reading each time would be noise.
  detailsRevalidating: false,
  detailsCache: new Map(),
  // Ids whose tag details are being prefetched in the background, so fast paging
  // does not stampede duplicate /resource.json requests for the same resource.
  _detailsInFlight: new Set(),
  detailsAborter: null,
  // Monotonic token so a stale/aborted fetch cannot flip detailsLoading off while a
  // newer fetch is still in flight (BH: M2).
  _detailsReq: 0,
  // Per-resource write counter, bumped when a write starts AND when it finishes. A GET that
  // was in flight across either edge describes the pre-write state, so committing it would
  // visually undo the edit; comparing generations lets both the foreground fetch and the
  // background prefetch discard those. Kept separate from _detailsReq on purpose: that token
  // also owns the loading flag, and bumping it from a write would strand the spinner.
  _detailsGen: new Map(),
  // Writes currently in flight, per resource. The generation alone cannot catch a GET that
  // both starts and finishes inside the write window, because the generation does not move
  // while the write is open.
  _detailsWrites: new Map(),
  // Generations are drawn from one process-wide sequence rather than counting per resource,
  // so an entry evicted by the cap below and later recreated can never reissue a number a
  // still-pending read is holding.
  _detailsSeq: 0,

  // Tag editing
  _savingTagIds: new Set(),

  // Track if changes were made that require refreshing the page content
  needsRefreshOnClose: false,
};

export const editPanelMethods = {
  _detailsGeneration(resourceId) {
    return this._detailsGen.get(resourceId) ?? 0;
  },

  // Call before mutating `resourceId`, and pair with _endDetailsWrite in a finally. Every
  // read in flight across the write now describes the past, so neither fetchResourceDetails
  // nor _preloadDetailsUpcoming may commit its result on top of the change.
  // Returns the generation issued to this write, for _settleDetailsCache.
  _beginDetailsWrite(resourceId) {
    if (resourceId == null) return 0;
    const generation = ++this._detailsSeq;
    this._detailsGen.set(resourceId, generation);
    this._detailsWrites.set(resourceId, (this._detailsWrites.get(resourceId) ?? 0) + 1);
    // Unbounded growth would outlive the details cache it guards; the cap matches it.
    if (this._detailsGen.size > 500) {
      this._detailsGen.delete(this._detailsGen.keys().next().value);
    }
    return generation;
  },

  _endDetailsWrite(resourceId) {
    if (resourceId == null) return;
    const outstanding = (this._detailsWrites.get(resourceId) ?? 0) - 1;
    if (outstanding > 0) this._detailsWrites.set(resourceId, outstanding);
    else this._detailsWrites.delete(resourceId);
    // Bump again: a read that started AFTER the write began is just as stale as one that
    // started before it, and only this second edge rejects it.
    this._detailsGen.set(resourceId, ++this._detailsSeq);

    // A write that could not leave an authoritative cache entry dropped it instead, so the
    // panel may still be showing the pre-write copy. Converge now, but only in that case:
    // a write that did update the cache has already left resourceDetails correct, which
    // keeps batch tagging from firing a request per toggle.
    if ((this.editPanelOpen || this.quickTagPanelOpen) &&
        this.getCurrentItem()?.id === resourceId &&
        !this.detailsCache.has(resourceId) &&
        !this._detailsWrites.has(resourceId)) {
      this.fetchResourceDetails(resourceId, true);
    }
  },

  // Commit a writer's own snapshot, unless another write to the same resource landed while
  // this one was open. That snapshot was captured before the other write, so caching it
  // would quietly undo the newer edit; dropping the entry makes the next read fetch the
  // authoritative version instead.
  // Returns whether the snapshot was accepted, so callers that also paint it can tell.
  _settleDetailsCache(resourceId, writeGeneration, snapshot) {
    if (snapshot && this._detailsGeneration(resourceId) === writeGeneration) {
      this.detailsCache.set(resourceId, snapshot);
      return true;
    }
    this.detailsCache.delete(resourceId);
    return false;
  },

  // A fetched snapshot may be committed only when no write has touched this resource since
  // the fetch started, and none is open right now.
  _mayCommitDetails(resourceId, generation) {
    return this._detailsGeneration(resourceId) === generation &&
      !this._detailsWrites.has(resourceId);
  },

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

    // WS4 finding 123. This used to be querySelector('input, textarea'), which
    // is #lightbox-edit-name — and canNavigate() in lightbox.tpl deliberately
    // makes ArrowLeft/ArrowRight inert while a text field has focus, so simply
    // opening the panel killed image navigation with no indication why.
    // focusFirstIn lands on the panel's own "Close info panel" button, which is
    // a BUTTON, so paging keeps working and a screen reader lands on the
    // dismiss control. Do not re-target an input here: the selector, not the
    // markup order, is the bug.
    requestAnimationFrame(() => focusFirstIn(document.querySelector('[data-edit-panel]')));
  },

  closeEditPanel() {
    // The "Resource info" toggle is x-show'd on !editPanelOpen, so it is hidden
    // while the panel is open and reappears as the panel goes. Hand focus back
    // to it rather than letting the removed panel drop the reader on <body>.
    const toggle = document.querySelector('button[title="Resource info"]');
    this.editPanelOpen = false;
    if (toggle) requestAnimationFrame(() => focusOn(toggle));
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
    const listContainer = findListContainer(document);
    if (!listContainer) return;

    try {
      const url = new URL(window.location);
      url.pathname = url.pathname + '.body';

      const response = await fetch(url.toString());
      if (!response.ok) return;

      const html = await response.text();
      const parser = new DOMParser();
      const doc = parser.parseFromString(html, 'text/html');
      const newListContainer = findListContainer(doc);

      if (newListContainer && window.Alpine) {
        const scrollX = window.scrollX;
        const scrollY = window.scrollY;

        morphAndReinitChangedComponents(listContainer, newListContainer, {
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
    const container = document.querySelector(`${LIST_CONTAINER_SELECTOR}, .gallery, .dashboard-grid`);
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

    // A write is open on this resource, so any cached copy predates it. Painting it would
    // show — and let the user act on — values the write is about to change, and the write
    // itself may end by dropping the entry rather than replacing it.
    const cached = this._detailsWrites.has(resourceId) ? null : this.detailsCache.get(resourceId);
    if (cached) {
      this.resourceDetails = cached;
      // Fast path: use the cache for an instant paint. When forceRefresh is set — every
      // navigation, and every panel (re)open — we still fall through to revalidate against the
      // server, so a change made anywhere else (another tab, another session, a background job)
      // corrects itself within one round-trip instead of being shown stale for the whole
      // session (BH: L5).
      //
      // Note there is deliberately no _syncItemFromDetails here: only a server response is
      // authoritative enough to re-point an item. The item can be FRESHER than this cache entry
      // — refreshPageContent's updateItemsFromDOM patches items from a newly rendered page
      // while the entry keeps whatever hash it was stored with — so syncing from the cache
      // could roll the media back a version, remount it, and remount it again when the
      // revalidation below lands.
      if (!forceRefresh) return;
    }

    // Evict oldest entry if cache exceeds max size
    if (this.detailsCache.size > 100) {
      this.detailsCache.delete(this.detailsCache.keys().next().value);
    }

    const reqId = ++this._detailsReq;
    const generation = this._detailsGeneration(resourceId);
    // Something trustworthy for this resource is already on screen, so this is a revalidation.
    if (cached) this.detailsRevalidating = true;
    else this.detailsLoading = true;
    // This attempt supersedes any earlier verdict on the same resource.
    if (this.detailsErrorId === resourceId) this.detailsErrorId = null;

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

      // The id check alone is not enough: a write to this same resource can land while the
      // GET is in flight, and this response predates it. Committing it would reinstate the
      // pre-write name/tags and read as the user's edit reverting itself a moment later.
      if (this._mayCommitDetails(resourceId, generation)) {
        if (this.getCurrentItem()?.id === resourceId) {
          this.resourceDetails = fetchedDetails;
        }
        this.detailsCache.set(resourceId, fetchedDetails);
        // The item entry is metadata too — and the one the <img> src is built from.
        this._syncItemFromDetails(fetchedDetails);
      }
      this.detailsAborter = null;
    } catch (err) {
      if (err.name !== 'AbortError') {
        console.error('Failed to fetch resource details:', err);
        // Only a failure that leaves the panel with nothing to show is worth flagging: when a
        // revalidation fails behind a painted copy, that copy is still this resource's.
        if (!this._currentDetails(resourceId)) this.detailsErrorId = resourceId;
        this.announce('Failed to load resource details');
      }
    } finally {
      // Only the most recent request may clear the loading flags — an earlier aborted
      // request must not turn the spinner off while this newer one is still pending (BH: M2).
      if (reqId === this._detailsReq) {
        this.detailsLoading = false;
        this.detailsRevalidating = false;
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

  // The details the UI may PAINT, and the single gate every display site goes through.
  //
  // `resourceDetails` deliberately keeps holding the PREVIOUS image while the incoming one
  // loads (see onResourceChange) — carry-forward and the add/remove decision poll both read
  // that snapshot — but rendering it means showing one resource's name, size, dimensions, hash
  // and tags underneath a different resource's picture. Reads go through here so the panel
  // shows its loading state instead, and the quick slots read neutral, until the details on
  // hand actually describe what is on screen.
  displayDetails() {
    return this._currentDetails(this.getCurrentItem()?.id);
  },

  // True while the panel is waiting on details it may paint — either a first load or a
  // revalidation behind already-painted content. Drives aria-busy so assistive tech is told
  // the panel's values are being checked, without the visual overlay that detailsLoading owns.
  detailsBusy() {
    return this.detailsLoading || this.detailsRevalidating;
  },

  // The current resource has nothing paintable and nothing in flight to change that: the fetch
  // failed (or the resource is gone). The panel says so and offers a retry instead of showing a
  // placeholder that will never resolve — or, as it did before the fence, another image's data.
  detailsFailed() {
    const currentId = this.getCurrentItem()?.id;
    return currentId != null && this.detailsErrorId === currentId &&
      !this.displayDetails() && !this.detailsBusy();
  },

  retryDetails() {
    this.detailsErrorId = null;
    this.fetchResourceDetails(undefined, true);
    this.fetchSuggestedTags(undefined, true);
  },

  // Keep the viewer's own item entry in step with the authoritative details for that resource.
  // The item carries the name in the toolbar, the dimensions readout, and — through `hash` —
  // the cache-busting `viewUrl` the <img>/<video> src is built from. A version created anywhere
  // else (a crop in another tab, an uploaded replacement, a plugin action) changes Hash, so
  // without this the panel would report the new metadata over the OLD bitmap for as long as the
  // session lasts. No-ops unless something actually changed, so ordinary navigation never
  // remounts media. `forceMedia` is for an in-place edit that must re-show the spinner even
  // when the fields happen to compare equal.
  _syncItemFromDetails(details, { forceMedia = false } = {}) {
    const id = details?.ID;
    if (id == null) return false;
    const idx = this.items.findIndex(i => i.id === id);
    if (idx === -1) return false;

    const item = this.items[idx];
    const hash = details.Hash || '';
    const next = {
      ...item,
      hash,
      viewUrl: `/v1/resource/view?id=${id}${hash ? `&v=${hash}` : ''}`,
      // Crop/rotate re-encode the image, so the content type can change (e.g. rotate always
      // re-encodes to JPEG); keep the item in sync.
      contentType: details.ContentType || item.contentType,
      name: typeof details.Name === 'string' ? details.Name : item.name,
      width: details.Width || 0,
      height: details.Height || 0,
    };

    const mediaChanged = next.viewUrl !== item.viewUrl || next.contentType !== item.contentType;
    const changed = mediaChanged || next.name !== item.name ||
      next.width !== item.width || next.height !== item.height;
    if (!changed && !forceMedia) return false;

    this.items[idx] = next;

    if ((mediaChanged || forceMedia) && idx === this.currentIndex) {
      // A different bitmap is coming: zoom and pan are clamped against the one that was on
      // screen, so drop them and show the spinner until the new one loads.
      this.resetZoom();
      this.loading = true;
      this.scheduleMediaCheck();
    }
    return true;
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
    // Blanking would throw away the previous image's snapshot that _snapshotCarryForward
    // and the decision poll still need, and evicting the entry we are about to need forces
    // a blocking round-trip per image. Instead this is stale-while-revalidate: the cache
    // paints instantly (its hit path is fully synchronous) and the forced refresh below
    // checks it against the server, so a change made outside this viewer corrects itself
    // within one round-trip rather than persisting for the session. What must never be
    // painted — the PREVIOUS image's details during a cache miss — is fenced off at the
    // read sites by displayDetails(), not by nulling the state. The post-await id guard in
    // fetchResourceDetails (BH: H5) prevents cross-resource cache poisoning.
    await this.fetchResourceDetails(undefined, true);

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

    const writeGeneration = this._beginDetailsWrite(resourceId);
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

      this._settleDetailsCache(resourceId, writeGeneration, { ...details });
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
    } finally {
      this._endDetailsWrite(resourceId);
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

    const writeGeneration = this._beginDetailsWrite(resourceId);
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

      this._settleDetailsCache(resourceId, writeGeneration, { ...details });
      this.needsRefreshOnClose = true;
      this.announce('Description updated');
    } catch (err) {
      console.error('Failed to update description:', err);
      details.Description = oldDescription;
      this.detailsCache.delete(resourceId);
      this.announce('Failed to update description');
    } finally {
      this._endDetailsWrite(resourceId);
    }
  },

  // ==================== Tag API Methods ====================

  // Association persistence for the lightbox tag-editor profile. The profile owns optimistic
  // selection state; the domain keeps the detail cache, suggested-tag invalidation, recent-tag
  // tracking and announcements below. The abort signal is deliberately unused: each write
  // captures its resource id up front, so a write started before navigation must still land.
  tagAssociationAdapter() {
    return {
      add: (tag) => this.saveTagAddition(tag),
      remove: (tag) => this.saveTagRemoval(tag),
    };
  },

  async saveTagAddition(tag) {
    const resourceId = this.getCurrentItem()?.id;
    if (!resourceId || this._savingTagIds.has(tag.ID)) return;

    this._savingTagIds.add(tag.ID);
    const writeGeneration = this._beginDetailsWrite(resourceId);

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

      // A null `details` means the write happened during a cache-miss load window, where
      // resourceDetails still described the previous image: there is no trustworthy snapshot
      // to cache, so _settleDetailsCache drops the entry and a later view refetches.
      this._settleDetailsCache(resourceId, writeGeneration, details ? { ...details } : null);
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
      this._endDetailsWrite(resourceId);
    }
  },

  async saveTagRemoval(tag) {
    const resourceId = this.getCurrentItem()?.id;
    if (!resourceId) return;

    const writeGeneration = this._beginDetailsWrite(resourceId);

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

      // Null `details` (a cache-miss load window) has no trustworthy snapshot to cache, so
      // _settleDetailsCache drops the entry and a later view refetches the authoritative set.
      this._settleDetailsCache(resourceId, writeGeneration, details ? { ...details } : null);
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
    } finally {
      this._endDetailsWrite(resourceId);
    }
  },

  // Display-side read: never the previous image's tags (see displayDetails).
  getCurrentTags() {
    return this.displayDetails()?.Tags || [];
  },
};
