import { abortableFetch } from '../../index.js';
import { morphAndReinitChangedComponents } from '../../utils/shortcodeElementMorph.js';
import { findListContainer, LIST_CONTAINER_SELECTOR } from '../../utils/listContainer.js';
import { focusFirstIn, focusOn, focusedElement, restoreFocus } from '../../utils/focus.js';

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

  // Tag editing: the tail of each `resourceId:tagId` write chain (see _serializeTagWrite)
  _tagWriteChains: new Map(),

  // Track if changes were made that require refreshing the page content
  needsRefreshOnClose: false,

  // The last failed write per field, as { resourceId, message }. The live region alone is
  // gone the moment it is spoken, and a sighted user saw a typed name silently snap back.
  // Read through writeError(), which shows a message only on the image it belongs to.
  writeErrors: { name: null, description: null, tags: null },
  // Drawn by every name, description and tag write as it starts, so a failure records which
  // write it was and an older save that lands later cannot clear it (see _clearWriteError).
  _writeSeq: 0,
};

export const editPanelMethods = {
  // Shown on the image the user was on when it failed and on the image the write was for: a
  // failure that lands after they moved on names that image, and they go back there to retry.
  writeError(field) {
    const error = this.writeErrors[field];
    const currentId = this.getCurrentItem()?.id;
    return error && (error.resourceId === currentId || error.targetId === currentId) ? error.message : '';
  },

  // `resourceId` is the image the message shows on; `targetId` the one the failed write was
  // for, when they differ; `tagIds` the tags a failed tag write was changing. `session` is the
  // viewing session the write started in; a failure landing after close() must not show up in
  // the next one. `seq` is the write's _writeSeq.
  _setWriteError(field, resourceId, message, session, seq, targetId = resourceId, tagIds = null) {
    if (session !== this._session || this._isSuperseded(field, targetId, seq, tagIds)) return;
    // Nor replace the failure of a write that started later: rename to A, then to B, and when
    // both fail the message is about B, the name the reader last typed.
    const existing = this.writeErrors[field];
    if (existing?.targetId === targetId && existing.seq > seq) return;
    this.writeErrors[field] = { resourceId, targetId, message, tagIds, seq };
  },

  // A write that started later on the same field of the same image (for tags, on every tag
  // that failed) has already gone through, so this one's failure lost nothing: rename to A,
  // then to B, and B saving first leaves B in place, which A's message would call lost.
  _isSuperseded(field, resourceId, seq, tagIds = null) {
    const keys = this._writeKeys(field, resourceId, tagIds);
    return keys.length > 0 && keys.every(key => this._writeSucceeded?.get(key) > seq);
  },

  // Tags count one by one: saving Y says nothing about the X that failed.
  _writeKeys(field, resourceId, tagIds) {
    return field === 'tags' ? (tagIds ?? []).map(id => `tags:${resourceId}:${id}`) : [`${field}:${resourceId}`];
  },

  // Called by every write that succeeds, which it records for _isSuperseded.
  // Only a later success in the same session, on the image the failed write was for, replaces
  // its message; a success elsewhere says nothing about it. Later means started later: rename
  // to A, then to B, and A's save landing after B failed has not saved B. A tag failure also
  // needs the success to touch one of its tags: adding Y says nothing about the X that failed.
  _clearWriteError(field, resourceId, session, seq, tagIds = []) {
    // Lazily allocated per store, like _suggestedDirty, so stores do not share one Map.
    this._writeSucceeded ??= new Map();
    for (const key of this._writeKeys(field, resourceId, tagIds)) {
      if (!(this._writeSucceeded.get(key) > seq)) this._writeSucceeded.set(key, seq);
    }
    const error = this.writeErrors[field];
    if (session !== this._session || error?.targetId !== resourceId || seq <= error.seq) return;
    if (error.tagIds && !tagIds.some(id => error.tagIds.includes(id))) return;
    this.writeErrors[field] = null;
  },

  // A write that lands after close() (a Name edit saved by Back, a tag write still out) has
  // missed the refresh close() makes, so it refreshes the page now rather than leave the flag
  // set on a closed viewer for the next session to act on.
  _refreshOnClose(session) {
    if (session === this._session) this.needsRefreshOnClose = true;
    else this.refreshPageContent();
  },

  _queueSuggestedRefresh(resourceId) {
    this._suggestedDirty ??= new Set();
    this._suggestedDirty.add(resourceId);
  },

  // Tag edits also invalidate recommendations. Lazily allocate per store so
  // independent stores/tests do not share a module-level mutable collection.
  _beginTagWrite(resourceId, tags, action) {
    this._suggestedCache?.delete(resourceId);
    this._queueSuggestedRefresh(resourceId);
    if (this.getCurrentItem()?.id === resourceId) {
      ++this._suggestedReq;
      this.suggestedTagsLoading = false;
      if (action === 'add') {
        const added = new Set(tags.map(tag => tag.ID));
        this.suggestedTags = (this.suggestedTags || []).filter(tag => !added.has(tag.ID));
      }
    }
    return this._beginDetailsWrite(resourceId);
  },

  _detailsGeneration(resourceId) {
    return this._detailsGen.get(resourceId) ?? 0;
  },

  // Call before mutating `resourceId`, and pair with _endDetailsWrite in a finally. Every
  // read in flight across the write now describes the past, so neither fetchResourceDetails
  // nor _preloadDetailsUpcoming may commit its result on top of the change.
  // Returns the generation issued to this write, for _settleDetailsCache.
  _beginDetailsWrite(resourceId) {
    if (resourceId == null) return 0;
    // Name/description edits share this generation too. If they invalidate a
    // suggestion GET, queue its replacement before the old response is dropped.
    if (this.suggestedTagsLoading && this.getCurrentItem()?.id === resourceId) {
      this._queueSuggestedRefresh(resourceId);
    }
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
    if ((this.editPanelOpen || this.quickTagPanelOpen || this.versionPanelOpen) &&
        this.getCurrentItem()?.id === resourceId &&
        !this.detailsCache.has(resourceId) &&
        !this._detailsWrites.has(resourceId)) {
      this.fetchResourceDetails(resourceId, true);
    }
    // Refresh once after overlapping edits (including rollback/undo) settle.
    // Navigation leaves the edited resource invalidated without touching the new row.
    if (!this._detailsWrites.has(resourceId) && this._suggestedDirty?.delete(resourceId) &&
        this.quickTagPanelOpen && this.getCurrentItem()?.id === resourceId) {
      this.fetchSuggestedTags(resourceId, true);
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
    const panel = document.querySelector('[data-edit-panel]');
    const hadFocus = !!panel?.contains(document.activeElement);
    this.editPanelOpen = false;
    // Two frames: x-show reveals the toggle in a frame of its own, queued after this one, so a
    // single frame tried to focus a still-hidden button and focus fell to <body>. Only when
    // focus was in the panel and is still lost by then: opening the Tags panel closes this one
    // on narrow viewports, and must not have its own focus move taken back.
    if (toggle && hadFocus) {
      requestAnimationFrame(() => requestAnimationFrame(() => {
        const now = focusedElement();
        if (!now || panel.contains(now)) focusOn(toggle);
      }));
    }
    // The media viewport widens again — re-clamp pan to the new bounds (BH: M7).
    requestAnimationFrame(() => this.constrainPan());

    if (!this.quickTagPanelOpen && !this.versionPanelOpen) {
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

  // One refresh at a time. Overlapping ones morphed in whatever order their responses came
  // back, so a slow early one could bring back a name a later one had shown, and MRQL's
  // selection refresh drops a call made while its query is still running. A call made during
  // a refresh asks for one more run after it, and gets a promise that settles once that is done.
  refreshPageContent() {
    if (this._pageRefresh) {
      this._pageRefreshAgain = true;
      return this._pageRefresh;
    }
    this._pageRefresh = (async () => {
      try {
        do {
          this._pageRefreshAgain = false;
          // MRQL refreshes by re-running its query, which rebuilds the whole list: whatever had
          // focus in it goes (the thumbnail close() gave focus back to, a title link, the list
          // itself), and the reader is left on <body>. Once it renders, put them on the same
          // resource's new thumbnail if they were on one and it is still listed, otherwise on
          // the list, unless they have moved on since.
          const focused = focusedElement();
          const inList = findListContainer(document)?.contains(focused) ? focused : null;
          await this._refreshPageContentOnce();
          if (!inList) continue;
          await new Promise(resolve => requestAnimationFrame(resolve));
          if (focusedElement() || inList.isConnected) continue;
          const list = findListContainer(document);
          const id = inList.closest('[data-lightbox-item]')?.dataset.resourceId;
          restoreFocus(id && list?.querySelector(`[data-lightbox-item][data-resource-id="${id}"]`),
            list ?? document.querySelector('main'));
        } while (this._pageRefreshAgain);
      } finally {
        this._pageRefresh = null;
      }
    })();
    return this._pageRefresh;
  },

  async _refreshPageContentOnce() {
    if (this._listSelection?.refresh) {
      await this._listSelection.refresh();
      this.updateItemsFromDOM();
      return;
    }
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
          width: parseInt(link.dataset.resourceWidth, 10) || 0,
          height: parseInt(link.dataset.resourceHeight, 10) || 0,
          hash,
        });
      }
    });

    for (let i = 0; i < this.items.length; i++) {
      const updated = domItems.get(this.items[i].id);
      if (updated) {
        const next = { ...this.items[i], ...updated };
        this.items[i] = this._preserveDisplayedVersion?.(this.items[i], next) ?? next;
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
      const oldestId = this.detailsCache.keys().next().value;
      this.detailsCache.delete(oldestId);
      this.versionsCache?.delete(oldestId);
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
        this._cacheVersions?.(resourceId, data.versions);
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
    let next = {
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

    next = this._preserveDisplayedVersion?.(item, next) ?? next;
    const mediaChanged = next.viewUrl !== item.viewUrl || next.contentType !== item.contentType;
    const changed = mediaChanged || next.name !== item.name ||
      next.width !== item.width || next.height !== item.height;
    if (!changed && !forceMedia && !item.displayedVersion) return false;

    this.items[idx] = next;

    if ((mediaChanged || forceMedia) && idx === this.currentIndex) {
      // A different bitmap is coming: zoom and pan are clamped against the one that was on
      // screen, so drop them and show the spinner until the new one loads.
      this.resetZoom();
      this._armMediaLoad();
      this.scheduleMediaCheck();
    }
    return true;
  },

  async onResourceChange() {
    this.resetDisplayedVersion?.();
    if (!this.editPanelOpen && !this.quickTagPanelOpen && !this.versionPanelOpen) return;

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

    // The suggestion row and its Shift+digit keys act on whatever image is current, so the
    // previous image's chips must go before the await below, not after it: for a whole
    // round-trip they offered A's suggestions as one-tap writes onto B. The request token
    // drops any response still in flight for the previous image.
    if (this.quickTagPanelOpen) {
      ++this._suggestedReq;
      this.suggestedTagsLoading = false;
      this.suggestedTags = [];
    }

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
    if (this.versionPanelOpen) this.scrollDisplayedVersion();
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
    const session = this._session;
    const seq = ++this._writeSeq;
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
      this._refreshOnClose(session);
      this._clearWriteError('name', resourceId, session, seq);
      this.announce('Name updated');
    } catch (err) {
      console.error('Failed to update name:', err);
      // A rename that started later has already saved. Rolling back would put a name on screen
      // the server no longer has, and with no panel open nothing would refetch to correct it.
      if (this._isSuperseded('name', resourceId, seq)) return;
      details.Name = oldName;
      if (item) {
        item.name = oldName;
      }
      // The cached copy for this resource is now uncertain — drop it so a later view refetches.
      this.detailsCache.delete(resourceId);
      const message = `Could not save the name "${newName}". The previous name is back.`;
      this._setWriteError('name', resourceId, message, session, seq);
      // The words on screen: after Enter, focus has left the field the message describes.
      this.announce(message);
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
    const session = this._session;
    const seq = ++this._writeSeq;
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
      this._refreshOnClose(session);
      this._clearWriteError('description', resourceId, session, seq);
      this.announce('Description updated');
    } catch (err) {
      console.error('Failed to update description:', err);
      if (this._isSuperseded('description', resourceId, seq)) return;
      details.Description = oldDescription;
      this.detailsCache.delete(resourceId);
      const message = 'Could not save the description. The previous text is back.';
      this._setWriteError('description', resourceId, message, session, seq);
      this.announce(message);
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

  // Writes for one tag on one resource reach the server in the order the reader made them.
  // The profile discards a superseded operation's result, but it cannot recall a request the
  // server may already have applied: an add followed quickly by a remove used to race, and
  // the row kept a tag the panel showed as gone. Mirrors tagAssociationFromUrls.
  // A batch write (quick slot, suggestion, carry-forward, undo) names several tags in one
  // request, so it waits for every chain it touches and becomes the tail of each of them.
  _serializeTagWrite(resourceId, tagIds, run) {
    const keys = tagIds.map(tagId => `${resourceId}:${tagId}`);
    const previous = Promise.all(keys.map(key => this._tagWriteChains.get(key)));
    const next = previous.then(run);
    // A swallowed tail, so one failed write cannot poison the writes queued behind it.
    const tail = next.catch(() => undefined);
    for (const key of keys) this._tagWriteChains.set(key, tail);
    tail.then(() => {
      for (const key of keys) {
        if (this._tagWriteChains.get(key) === tail) this._tagWriteChains.delete(key);
      }
    });
    return next;
  },

  _postTagWrite(url, resourceId, tagId) {
    return this._serializeTagWrite(resourceId, [tagId], () => {
      const formData = new FormData();
      formData.append('ID', resourceId);
      formData.append('EditedId', tagId);
      return fetch(url, {
        method: 'POST',
        body: formData,
        headers: { 'Accept': 'application/json' }
      });
    });
  },

  async saveTagAddition(tag) {
    const resourceId = this.getCurrentItem()?.id;
    if (!resourceId) return;

    const writeGeneration = this._beginTagWrite(resourceId, [tag], 'add');
    const session = this._session;
    const seq = ++this._writeSeq;

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
      const response = await this._postTagWrite('/v1/resources/addTags', resourceId, tag.ID);

      if (!response.ok) {
        throw new Error(`Failed to add tag: ${response.status}`);
      }

      // A null `details` means the write happened during a cache-miss load window, where
      // resourceDetails still described the previous image: there is no trustworthy snapshot
      // to cache, so _settleDetailsCache drops the entry and a later view refetches.
      this._settleDetailsCache(resourceId, writeGeneration, details ? { ...details } : null);
      this._refreshOnClose(session);
      this._clearWriteError('tags', resourceId, session, seq, [tag.ID]);
      this.announce(`Added tag: ${tag.Name}`);

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
      this._endDetailsWrite(resourceId);
    }
  },

  async saveTagRemoval(tag) {
    const resourceId = this.getCurrentItem()?.id;
    if (!resourceId) return;

    const writeGeneration = this._beginTagWrite(resourceId, [tag], 'remove');
    const session = this._session;
    const seq = ++this._writeSeq;

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
      const response = await this._postTagWrite('/v1/resources/removeTags', resourceId, tag.ID);

      if (!response.ok) {
        throw new Error(`Failed to remove tag: ${response.status}`);
      }

      // Null `details` (a cache-miss load window) has no trustworthy snapshot to cache, so
      // _settleDetailsCache drops the entry and a later view refetches the authoritative set.
      this._settleDetailsCache(resourceId, writeGeneration, details ? { ...details } : null);
      this._refreshOnClose(session);
      this._clearWriteError('tags', resourceId, session, seq, [tag.ID]);
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
