import { abortableFetch } from '../../index.js';
import { captureTrigger, restoreFocus } from '../../utils/focus.js';
import { findListContainer } from '../../utils/listContainer.js';

/**
 * Navigation and pagination state/methods for the lightbox store.
 * All methods use `this` which is bound to the Alpine store.
 */
export const navigationState = {
  // Core state
  isOpen: false,
  currentIndex: 0,
  items: [],
  loading: false,
  pageLoading: false,
  // Id of the item whose media failed to load. Reconciliation deliberately keeps entries the
  // DOM no longer lists, because "paged off the first page" and "deleted elsewhere" are
  // indistinguishable there — so a resource deleted from another session stays in the list
  // and 404s when reached. Announcing that was not enough; the viewer showed a broken image
  // with no on-screen explanation.
  mediaErrorId: null,

  // Pagination state
  currentPage: 1,
  hasNextPage: false,
  hasPrevPage: false,
  baseUrl: '',
  pageSize: 50,

  // Track loaded page ranges to avoid re-fetching
  loadedPages: new Set(),

  // Request aborter for canceling in-flight requests
  requestAborter: null,

  // Reference to trigger element for focus restoration
  triggerElement: null,

  // The page's own gallery, parked while a standalone item is open. See
  // openFromClick's fallback branch and the restore in close().
  _itemsBeforeStandalone: null,
  // Its pagination context, parked alongside. Restoring `items` without this leaves the
  // page's gallery holding the standalone item's "one page, no neighbours" cursor.
  _paginationBeforeStandalone: null,

  // True while `items` came from a page's small inline preview (a `[data-lightbox-source]`
  // container) rather than from a page of the paging endpoint. The first fetch must then ask
  // for page 1 and replace the preview, not ask for page 2 and skip everything in between.
  _previewSeeded: false,

  // True while `items` is the page's own gallery, so a DOM re-scan describes the same list.
  // Cleared for the standalone and source-container sessions, whose lists come from
  // elsewhere. See initFromDOM.
  _ownsPageGallery: true,

  // Cache of preloaded image URLs (kept in-memory by the Image objects we hold)
  _preloadedUrls: new Set(),
  _preloadedImages: [],
  _preloadAheadCount: 5,
};

export const navigationMethods = {
  _extractItemsFromLinks(links) {
    return Array.from(links).map(link => {
      const hash = link.dataset.resourceHash || '';
      const versionParam = hash ? `&v=${hash}` : '';
      return {
        id: parseInt(link.dataset.resourceId, 10),
        viewUrl: `/v1/resource/view?id=${link.dataset.resourceId}${versionParam}`,
        contentType: link.dataset.contentType || '',
        name: link.dataset.resourceName || link.querySelector('img')?.alt || '',
        hash: hash,
        width: parseInt(link.dataset.resourceWidth, 10) || 0,
        height: parseInt(link.dataset.resourceHeight, 10) || 0,
        ownerName: link.dataset.ownerName || '',
        ownerId: parseInt(link.dataset.ownerId, 10) || 0,
      };
    }).filter(item =>
      item.contentType?.startsWith('image/') ||
      item.contentType?.startsWith('video/')
    );
  },

  initFromDOM() {
    const containers = document.querySelectorAll('.list-container, .gallery, .dashboard-grid');
    if (containers.length === 0) return;

    const allItems = [];
    containers.forEach(container => {
      const links = container.querySelectorAll('[data-lightbox-item]');
      allItems.push(...this._extractItemsFromLinks(links));
    });

    // A background refresh must not take the gallery away from a viewer that is already
    // open. `items` legitimately holds more than the DOM does once loadNextPage has appended
    // a page the document never rendered, and `_extractItemsFromLinks` drops non-media cards,
    // so a wholesale rebuild here can leave the list shorter than a `currentIndex` that was
    // valid a moment ago. getCurrentItem() then returns undefined, every media branch in
    // lightbox.tpl unmounts, and the viewer sits open over nothing until it is reopened.
    // Every caller that can land here mid-session is fire-and-forget: the download-completed
    // listener in main.js (dispatched off an SSE stream, so with no user action at all),
    // plus pasteUpload, timeline and mrqlEditor.
    if (this.isOpen) {
      // Only a viewer that is actually paging the page's own gallery may take this scan.
      // A standalone item and a source-container preview both hold a different list from a
      // different endpoint, so merging these cards in would grow a deliberately narrow
      // viewer with resources the user never opened. Leaving them untouched is also safe:
      // nothing shrinks, so nothing can strand the index.
      if (this._ownsPageGallery) {
        this._reconcileOpenItems(allItems);
      }
      return;
    }

    this.items = allItems;
    this._readPaginationFromDOM(containers);
  },

  _readPaginationFromDOM(containers) {
    const urlParams = new URLSearchParams(window.location.search);
    this.currentPage = parseInt(urlParams.get('page'), 10) || 1;
    this.baseUrl = window.location.pathname + window.location.search;
    this._previewSeeded = false;
    this._ownsPageGallery = true;

    const paginationNav = document.querySelector('nav[aria-label="Pagination"]');
    if (paginationNav) {
      this.hasNextPage = paginationNav.dataset.hasNext === 'true';
      this.hasPrevPage = paginationNav.dataset.hasPrev === 'true';
    } else {
      this.hasPrevPage = this.currentPage > 1;
      this.hasNextPage = false;
    }

    // A fresh Set, not an add. `loadedPages` describes which pages are inside `items`, and
    // `items` was just rebuilt from one page of DOM. Carrying entries over from an earlier
    // paging session makes loadNextPage's already-loaded check fire for a page whose items
    // are no longer there, so Next does nothing at all for one press per stale page.
    this.loadedPages = new Set([this.currentPage]);

    const pageSizeAttr = containers[0].dataset.pageSize;
    if (pageSizeAttr) {
      this.pageSize = parseInt(pageSizeAttr, 10) || 50;
    } else if (window.location.pathname.includes('/simple')) {
      this.pageSize = 200;
    }
  },

  // Merge a fresh DOM scan into the list a live viewer is paging through, without ever
  // shortening it. Entries the scan still shows are patched in place (a crop or rotate
  // rewrites viewUrl and hash), genuinely new cards are inserted where the DOM puts them, and
  // entries the DOM no longer shows are kept: dropping them is what strands currentIndex, and
  // we cannot tell "scrolled off page 1" from "deleted" here anyway. The viewer then
  // re-anchors on the resource it was actually displaying, so an insert above it cannot
  // silently slide the user onto a neighbour.
  _reconcileOpenItems(freshItems) {
    const anchor = this.getCurrentItem();
    const anchorId = anchor?.id;
    const anchorUrl = anchor?.viewUrl;

    const existingIds = new Set(this.items.map(item => item.id));
    const patches = new Map();
    const insertBefore = new Map();
    const trailing = [];

    freshItems.forEach((fresh, index) => {
      if (existingIds.has(fresh.id)) {
        patches.set(fresh.id, fresh);
        return;
      }
      // Anchor each new card to the first card after it that we already hold, so a
      // newest-first list keeps its order instead of collecting arrivals at the tail.
      let successorId = null;
      for (let i = index + 1; i < freshItems.length; i++) {
        if (existingIds.has(freshItems[i].id)) {
          successorId = freshItems[i].id;
          break;
        }
      }
      if (successorId === null) {
        trailing.push(fresh);
        return;
      }
      const bucket = insertBefore.get(successorId);
      if (bucket) bucket.push(fresh);
      else insertBefore.set(successorId, [fresh]);
    });

    const merged = [];
    for (const item of this.items) {
      const pending = insertBefore.get(item.id);
      if (pending) {
        merged.push(...pending);
        insertBefore.delete(item.id);
      }
      const patch = patches.get(item.id);
      merged.push(patch ? { ...item, ...patch } : item);
    }
    merged.push(...trailing);

    this.items = merged;
    this._reanchorIndex(anchorId);

    // Zoom and pan are clamped against the bitmap that was on screen. If the merge put a
    // different resource (or a new version of the same one) under the viewer, a pan that was
    // legal a moment ago can translate the new image clear out of the overflow-hidden media
    // column: loaded, present in the DOM, and invisible. cropPanel drops zoom on its own
    // image swap for exactly this reason.
    const current = this.getCurrentItem();
    if (current?.id !== anchorId || current?.viewUrl !== anchorUrl) {
      this.resetZoom();
    }
  },

  // Keep the viewer on the resource it was showing after `items` is rebuilt or reordered.
  // Clamping alone would silently move the user to a different image, so the clamp is only
  // the fallback for when the resource really has gone.
  _reanchorIndex(anchorId) {
    if (!this.items.length) {
      this.currentIndex = 0;
      return;
    }
    if (anchorId != null) {
      const index = this.items.findIndex(item => item.id === anchorId);
      if (index !== -1) {
        this.currentIndex = index;
        return;
      }
    }
    this.currentIndex = Math.max(0, Math.min(this.currentIndex, this.items.length - 1));
  },

  openFromClick(event, resourceId, contentType) {
    if (!contentType?.startsWith('image/') && !contentType?.startsWith('video/')) {
      window.location.href = event.currentTarget.href;
      return;
    }

    event.preventDefault();
    this.triggerElement = captureTrigger(event);

    // Check if inside a container with a lightbox source (multi-section pages)
    const sourceContainer = event.currentTarget.closest('[data-lightbox-source]');
    if (sourceContainer) {
      this._openFromSourceContainer(sourceContainer, resourceId);
      return;
    }

    const index = this.items.findIndex(item => item.id === resourceId);
    if (index !== -1) {
      this._ownsPageGallery = true;
      this.open(index);
      return;
    }

    // Not in the page's gallery: open it as a gallery of one.
    //
    // The resource detail page's own main preview is absent from `items` by
    // construction — GetSeriesSiblings excludes the resource itself, and the
    // sidebar is not a scanned lightbox container — so without this branch the
    // preview would call openFromClick, find nothing, and do nothing at all. A
    // dead click is worse than the plain link it replaced.
    const single = this._extractItemsFromLinks([event.currentTarget]);
    if (single.length === 0) return;

    this._itemsBeforeStandalone = this.items;
    this._paginationBeforeStandalone = this._capturePaginationContext();
    this.items = single;
    this.baseUrl = '';
    this.currentPage = 1;
    this.loadedPages = new Set([1]);
    this.hasNextPage = false;
    this.hasPrevPage = false;
    this._previewSeeded = false;
    this._ownsPageGallery = false;
    this.open(0);
  },

  _openFromSourceContainer(container, resourceId) {
    const links = container.querySelectorAll('[data-lightbox-item]');
    const containerItems = this._extractItemsFromLinks(links);
    if (containerItems.length === 0) return;

    // Build pagination URL from container's data attributes
    const basePath = container.dataset.lightboxSource;
    const paramName = container.dataset.lightboxParamName;
    const paramValue = container.dataset.lightboxParamValue;
    const url = new URL(basePath, window.location.origin);
    url.searchParams.set(paramName, paramValue);

    // Configure lightbox for this container's resources
    this.items = containerItems;
    this.baseUrl = url.pathname + url.search;
    this.pageSize = 50;
    // These items came from the page's 5-resource preview (pageLimitCustom(5) server-side),
    // not from a page of the endpoint we are about to page through. Treating them as page 1
    // made the first Next ask for page 2 and skip resources 6..50 outright. `loadedPages`
    // therefore starts empty and loadNextPage fetches page 1, which is a superset of the
    // preview in the same order, and replaces it.
    this.currentPage = 1;
    this.loadedPages = new Set();
    this._previewSeeded = true;
    this._ownsPageGallery = false;
    // Counted BEFORE filtering out non-media. Deriving this from the filtered
    // `containerItems` would hide collection media whenever the preview mixes media and
    // non-media (BH: H1).
    this.hasNextPage = links.length >= 5;
    this.hasPrevPage = false;

    const index = containerItems.findIndex(item => item.id === resourceId);
    if (index !== -1) {
      this.open(index);
    }
  },

  open(index) {
    // Guard against an empty list or an out-of-bounds index (e.g. a gallery whose
    // clicked thumbnail index does not map 1:1 onto the filtered media list — BH: L1).
    if (!this.items || this.items.length === 0) return;
    const safeIndex = Math.max(0, Math.min(index | 0, this.items.length - 1));

    this._savedScrollY = window.scrollY;
    document.body.style.position = 'fixed';
    document.body.style.top = `-${this._savedScrollY}px`;
    document.body.style.left = '0';
    document.body.style.right = '0';
    document.body.style.overflow = 'hidden';
    // Prevent browser back/forward swipe gesture on Mac trackpads
    this._savedOverscrollBehaviorX = document.body.style.overscrollBehaviorX;
    document.body.style.overscrollBehaviorX = 'none';

    this.currentIndex = safeIndex;
    this.isOpen = true;
    this.loading = true;

    // WS4 finding 74. There used to be a pre-emptive
    //   document.activeElement.blur()
    // here, commented "so x-trap can move focus into the lightbox". It is not
    // needed — measured, the trap takes focus with the trigger still focused —
    // and it was actively harmful: x-trap activates on a setTimeout(15) and
    // records whatever has focus at that moment as its return node, so blurring
    // first made it record <body>. close()'s own restore below then ran, landed
    // correctly, and was overwritten when the trap deactivated and "returned"
    // focus to <body>. The trap now carries .noreturn (lightbox.tpl) and this
    // blur is gone, so the restore in close() is the only thing deciding.

    const item = this.getCurrentItem();
    const mediaType = this.isVideo(item?.contentType) ? 'video' : this.isSvg(item?.contentType) ? 'SVG' : 'image';
    this.announce(`Opened ${mediaType}: ${item?.name || 'media'}. ${this.currentIndex + 1} of ${this.items.length}`);

    this.scheduleMediaCheck();
    this._preloadUpcoming();

    // Restore quick tag panel from localStorage if it was previously open. Forced: whatever
    // is cached predates this session, and the resource may have been edited since.
    if (this.quickTagPanelOpen) {
      this.fetchResourceDetails(undefined, true);
      this.fetchSuggestedTags(undefined, true);
    }
  },

  _preloadUpcoming() {
    const ahead = this._preloadAheadCount || 5;
    const end = Math.min(this.items.length, this.currentIndex + 1 + ahead);
    for (let i = this.currentIndex + 1; i < end; i++) {
      const item = this.items[i];
      if (!item || !this.isImage(item.contentType)) continue;
      if (this._preloadedUrls.has(item.viewUrl)) continue;
      this._preloadedUrls.add(item.viewUrl);
      const img = new Image();
      img.decoding = 'async';
      // Track the (relative) viewUrl separately: img.src resolves to an absolute URL,
      // so we cannot use it to delete the matching entry from _preloadedUrls (BH: L6).
      img._viewUrl = item.viewUrl;
      img.src = item.viewUrl;
      this._preloadedImages.push(img);
    }
    // Cap retained references so the cache cannot grow unboundedly during long sessions.
    // Keep _preloadedUrls in lock-step with the retained images: evicting a URL also lets
    // it be re-preloaded later if the user navigates back to it (BH: L6).
    const cap = (this._preloadAheadCount || 5) * 6;
    if (this._preloadedImages.length > cap) {
      const removed = this._preloadedImages.splice(0, this._preloadedImages.length - cap);
      for (const img of removed) {
        this._preloadedUrls.delete(img._viewUrl);
      }
    }

    // Warm tag details for the same upcoming window so navigating to a neighbour
    // paints quick-slot colors instantly instead of round-tripping per image.
    this._preloadDetailsUpcoming();
  },

  _preloadDetailsUpcoming() {
    // Only warm details that the tagging UI will actually display. When no panel is
    // open these fetches would be pure waste (mirrors open()'s panel-gated fetch).
    if (!this.quickTagPanelOpen && !this.editPanelOpen) return;

    const ahead = this._preloadAheadCount || 5;
    const end = Math.min(this.items.length, this.currentIndex + 1 + ahead);
    for (let i = this.currentIndex + 1; i < end; i++) {
      const item = this.items[i];
      if (!item || !item.id) continue;
      // Tags apply to images and videos alike, so unlike bitmap preload this does
      // not filter by content type.
      if (this.detailsCache.has(item.id)) continue;
      if (this._detailsInFlight.has(item.id)) continue;

      this._detailsInFlight.add(item.id);
      const generation = this._detailsGeneration(item.id);
      const { ready } = abortableFetch(`/resource.json?id=${item.id}`);
      ready
        .then(response => (response.ok ? response.json() : null))
        .then(data => {
          if (!data) return;
          // A write to this resource landed while the prefetch was in flight, so this
          // response predates it. Caching it would quietly put the pre-write tags back, and
          // the panel would show the user's change undoing itself when they navigate here.
          if (!this._mayCommitDetails(item.id, generation)) return;
          const details = data.resource || data;
          // Respect the same cache cap fetchResourceDetails enforces.
          if (this.detailsCache.size > 100) {
            this.detailsCache.delete(this.detailsCache.keys().next().value);
          }
          this.detailsCache.set(item.id, details);
          // An upcoming item may have been re-versioned or renamed since the DOM was rendered;
          // patch it now so arriving there shows the current bitmap and name, not last page
          // load's. Never the on-screen item, so this cannot disturb what is being viewed.
          this._syncItemFromDetails(details);
        })
        .catch(() => { /* background prefetch: a real navigation will refetch on demand */ })
        .finally(() => this._detailsInFlight.delete(item.id));
    }
  },

  close() {
    this.pauseCurrentVideo();

    if (this.isFullscreen) {
      if (document.exitFullscreen) {
        document.exitFullscreen().catch(() => {});
      } else if (document.webkitExitFullscreen) {
        document.webkitExitFullscreen();
      }
      this.isFullscreen = false;
    }

    if (this.editPanelOpen) {
      this.closeEditPanel();
    }

    if (this.quickTagPanelOpen) {
      this.closeQuickTagPanel();
    }

    // Defensive: never leave the crop overlay's focus trap active behind a
    // hidden viewer. Not reachable through the UI today (the overlay covers the
    // close affordances), but keeps close() self-healing like the panels above.
    if (this.cropOpen) {
      this.closeCrop();
    }

    // When an in-place edit (crop/rotate) happened with no panel open, the
    // panel-close paths above never ran, so refresh the underlying gallery here
    // so its now-stale thumbnail reflects the new version.
    if (!this.editPanelOpen && !this.quickTagPanelOpen && this.needsRefreshOnClose) {
      this.needsRefreshOnClose = false;
      this.refreshPageContent();
    }

    // Give the page its own gallery back after a standalone item.
    //
    // Without this, opening the detail page's main preview and closing it leaves
    // `items` holding that one resource — so a later click on a series-sibling
    // card would miss the lookup, take the standalone branch itself, and lose
    // sibling-to-sibling navigation for the rest of the page's life.
    if (this._itemsBeforeStandalone) {
      const restore = this._itemsBeforeStandalone;
      this._itemsBeforeStandalone = null;
      this.items = restore;
    }
    // The items alone are not the whole context: openFromClick also blanked `baseUrl` and
    // collapsed the page cursor to a single page with no neighbours. Restoring only `items`
    // leaves the page's own gallery unable to load any further page for the rest of its life.
    if (this._paginationBeforeStandalone) {
      const parked = this._paginationBeforeStandalone;
      this._paginationBeforeStandalone = null;
      this._restorePaginationContext(parked);
    }

    this.isOpen = false;
    this.loading = false;
    this.resetZoom();

    const savedY = this._savedScrollY;
    document.body.style.position = '';
    document.body.style.top = '';
    document.body.style.left = '';
    document.body.style.right = '';
    document.body.style.overflow = '';
    // Restore overscroll behavior
    document.body.style.overscrollBehaviorX = this._savedOverscrollBehaviorX ?? '';
    window.scrollTo(0, savedY);

    if (this.requestAborter) {
      this.requestAborter();
      this.requestAborter = null;
    }

    // Release the preloaded-image cache so decoded bitmaps are not held between sessions.
    // (`items`/`loadedPages` are reassigned wholesale by every open() path, so they do not
    // leak across sessions; they only grow within a single uninterrupted paging session — BH: L7.)
    this._preloadedUrls.clear();
    this._preloadedImages = [];

    // Detail and suggestion snapshots describe a moment in time, and the next session can be
    // an hour and a dozen edits later — made on the resource page, in another tab, by a bulk
    // action or by a background job. Revalidation would correct them a round-trip after they
    // were painted; dropping them means the next session never paints them at all.
    // The write-generation maps are deliberately left alone: a write may still be in flight,
    // and clearing its generation is what would let its stale in-flight GET commit.
    this.detailsCache.clear();
    this._suggestedCache.clear();

    // A bare .focus() is silently a no-op on a detached node, and the thumbnail
    // that opened the viewer is often gone by now — an in-place crop or rotate
    // calls refreshPageContent(), which morphs the whole list. restoreFocus
    // checks isConnected and parks on the list instead of dropping to <body>.
    //
    // Deferred, and that is load-bearing: `this.isOpen = false` above only
    // schedules Alpine's teardown, so a synchronous restore lands while the
    // viewer is still mounted and x-trap is still active — the trap's focusin
    // guard pulls focus straight back in and the reader ends on <body> when the
    // subtree finally goes. Measured settling on BODY for 1.6s before this.
    // Two frames, because the trap releases on its own timer.
    if (this.triggerElement) {
      const trigger = this.triggerElement;
      this.triggerElement = null;
      requestAnimationFrame(() => {
        requestAnimationFrame(() => {
          restoreFocus(trigger, findListContainer(document) ?? document.querySelector('main'));
        });
      });
    }

    this.announce('Media viewer closed');

    requestAnimationFrame(() => {
      window.scrollTo(0, savedY);
    });
  },

  async next() {
    if (this.pageLoading) return;

    this.pauseCurrentVideo();
    this.resetZoom();

    if (this.currentIndex < this.items.length - 1) {
      this.currentIndex++;
      this.loading = true;
      this.announcePosition();
      this.scheduleMediaCheck();
      this._preloadUpcoming();
      this.onResourceChange();
    } else if (this.hasNextPage) {
      const loaded = await this.loadNextPage();
      if (this.currentIndex < this.items.length - 1) {
        this.currentIndex++;
        this.loading = true;
        // Combine the "loaded more" status with the position so the shared (single-slot)
        // live region does not clobber the page-load message with the position (BH: M9).
        this.announcePosition(loaded > 0 ? `Loaded ${loaded} more items. ` : '');
        this.scheduleMediaCheck();
        this._preloadUpcoming();
        this.onResourceChange();
      }
    }
  },

  async prev() {
    if (this.pageLoading) return;

    this.pauseCurrentVideo();
    this.resetZoom();

    if (this.currentIndex > 0) {
      this.currentIndex--;
      this.loading = true;
      this.announcePosition();
      this.scheduleMediaCheck();
      this._preloadUpcoming();
      this.onResourceChange();
    } else if (this.hasPrevPage) {
      const prevItemCount = await this.loadPrevPage();
      if (prevItemCount > 0) {
        this.currentIndex = prevItemCount - 1;
        this.loading = true;
        // Combine the load status with the position so it isn't clobbered (BH: M9).
        this.announcePosition(`Loaded ${prevItemCount} previous items. `);
        this.scheduleMediaCheck();
        this._preloadUpcoming();
        this.onResourceChange();
      }
    }
  },

  _capturePaginationContext() {
    return {
      baseUrl: this.baseUrl,
      currentPage: this.currentPage,
      loadedPages: this.loadedPages,
      hasNextPage: this.hasNextPage,
      hasPrevPage: this.hasPrevPage,
      pageSize: this.pageSize,
      previewSeeded: this._previewSeeded,
      ownsPageGallery: this._ownsPageGallery,
    };
  },

  _restorePaginationContext(parked) {
    this.baseUrl = parked.baseUrl;
    this.currentPage = parked.currentPage;
    this.loadedPages = parked.loadedPages;
    this.hasNextPage = parked.hasNextPage;
    this.hasPrevPage = parked.hasPrevPage;
    this.pageSize = parked.pageSize;
    this._previewSeeded = parked.previewSeeded;
    this._ownsPageGallery = parked.ownsPageGallery;
  },

  // The cursor for growing the buffer is the edge of what is loaded, not `currentPage`.
  // `currentPage` moves backwards when the user pages back across a boundary, which made the
  // next forward press ask for a page already inside `items` and silently do nothing.
  _nextPageNumber() {
    if (!this.loadedPages.size) return (this.currentPage || 1) + 1;
    return Math.max(...this.loadedPages) + 1;
  },

  _prevPageNumber() {
    if (!this.loadedPages.size) return (this.currentPage || 1) - 1;
    return Math.min(...this.loadedPages) - 1;
  },

  announcePosition(prefix = '') {
    const item = this.getCurrentItem();
    // Consume any pending flow-mode prefix once so the applied tag(s) and the new
    // position are announced together in a single live-region message (Item 5).
    const flow = this._pendingFlowPrefix || '';
    this._pendingFlowPrefix = '';
    this.announce(`${flow}${prefix}${item?.name || 'Media'}, ${this.currentIndex + 1} of ${this.items.length}`);
  },

  async loadNextPage() {
    // A preview-seeded buffer holds the page's 5-resource teaser, not a page of the endpoint,
    // so its first fetch is page 1 and it replaces rather than appends.
    const seeded = this._previewSeeded;

    this.pageLoading = true;
    this.announce('Loading more items...');

    try {
      const known = new Set(this.items.map(item => item.id));
      let page = seeded ? 1 : this._nextPageNumber();
      let addedCount = 0;
      let serverHasNext = false;
      let replacedPreview = false;
      let hopsExhausted = true;

      // A whole page can filter down to nothing when its resources are all non-media (PDFs,
      // archives). That is not the end of the gallery, so keep walking while the server still
      // reports a next page, rather than declaring "No more items" over the top of it.
      const MAX_HOPS = 20;
      for (let hop = 0; hop < MAX_HOPS; hop++) {
        const { items: newItems, hasNextPage } = await this.fetchPage(page);
        this.loadedPages.add(page);
        this.currentPage = page;
        serverHasNext = hasNextPage;

        if (seeded && !replacedPreview) {
          // Page 1 usually contains the preview, so replacing beats appending: the user gets
          // the endpoint's real ordering instead of a five-item teaser followed by a jump.
          //
          // "Usually" is doing work there. The preview comes from a bare Limit(5) with no
          // ORDER BY (pageLimitCustom, application_context/context.go:894) while the paging
          // endpoint sorts created_at desc, so on a collection larger than a page the two
          // need not overlap at all. Replacing then throws away the very item the user
          // clicked and drops them somewhere unrelated, so fall back to appending whenever
          // page 1 does not actually contain the anchor.
          const anchorId = this.getCurrentItem()?.id;
          const fresh = newItems.filter(item => !known.has(item.id));
          replacedPreview = true;
          this._previewSeeded = false;

          if (anchorId == null || newItems.some(item => item.id === anchorId)) {
            this.items = newItems;
            this._reanchorIndex(anchorId);
            known.clear();
            newItems.forEach(item => known.add(item.id));
          } else {
            this.items = [...this.items, ...fresh];
            fresh.forEach(item => known.add(item.id));
          }

          addedCount += fresh.length;
          // Only stop if this actually produced a forward step. Page 1 can contain the
          // anchor at its very last position, in which case the caller still cannot advance
          // and the keypress would be wasted; keep hopping to page 2 instead.
          if (fresh.length && this.currentIndex < this.items.length - 1) {
            hopsExhausted = false;
            break;
          }
        } else {
          const added = newItems.filter(item => !known.has(item.id));
          added.forEach(item => known.add(item.id));
          if (added.length) {
            this.items = [...this.items, ...added];
            addedCount += added.length;
            hopsExhausted = false;
            break;
          }
        }

        if (!hasNextPage) {
          hopsExhausted = false;
          break;
        }
        page++;
      }

      this.hasNextPage = serverHasNext;

      // Bounded so a pathological run of non-media pages cannot spin. Say so rather than
      // presenting the give-up as the end of the gallery; hasNextPage is still set above, so
      // pressing again resumes from where this left off.
      if (hopsExhausted) {
        this.announce(`No media found in the next ${MAX_HOPS} pages. Press again to keep looking.`);
        return 0;
      }

      if (addedCount === 0) {
        if (!serverHasNext) this.announce('No more items');
        return 0;
      }

      // The "loaded N more" status is announced (combined with position) by next() so it
      // is not immediately clobbered by the position announcement (BH: M9).
      return addedCount;
    } catch (err) {
      if (err.name !== 'AbortError') {
        console.error('Failed to load next page:', err);
        this.announce('Failed to load more items');
      }
      return 0;
    } finally {
      this.pageLoading = false;
    }
  },

  async loadPrevPage() {
    const prevPage = this._prevPageNumber();
    if (prevPage < 1) {
      this.hasPrevPage = false;
      return 0;
    }

    this.pageLoading = true;
    this.announce('Loading previous items...');

    try {
      const known = new Set(this.items.map(item => item.id));
      const { items: newItems } = await this.fetchPage(prevPage);
      // Page boundaries shift when a resource is added or removed while the viewer is open,
      // so the same resource can come back on two different pages. Prepending it twice would
      // show it twice and put the anchor arithmetic below out by one.
      const added = newItems.filter(item => !known.has(item.id));

      this.loadedPages.add(prevPage);
      // Track the page the user is now viewing, matching loadNextPage. Without this the
      // backward-pagination boundary stalls for one keypress (BH: M1).
      this.currentPage = prevPage;
      this.hasPrevPage = prevPage > 1;

      if (added.length === 0) return 0;

      this.items = [...added, ...this.items];
      this.currentIndex += added.length;

      // Status announced (combined with position) by prev() to avoid clobbering (BH: M9).
      return added.length;
    } catch (err) {
      if (err.name !== 'AbortError') {
        console.error('Failed to load previous page:', err);
        this.announce('Failed to load previous items');
      }
      return 0;
    } finally {
      this.pageLoading = false;
    }
  },

  async fetchPage(pageNum) {
    if (this.requestAborter) {
      this.requestAborter();
    }

    const url = new URL(this.baseUrl, window.location.origin);
    url.searchParams.set('page', pageNum);

    const jsonUrl = url.pathname + '.json' + url.search;

    const { abort, ready } = abortableFetch(jsonUrl);
    this.requestAborter = abort;

    const response = await ready;
    if (!response.ok) {
      throw new Error(`Failed to fetch page: ${response.status}`);
    }
    const data = await response.json();
    this.requestAborter = null;

    const resources = data.resources || [];
    const pagination = data.pagination || {};
    const hasNextPage = pagination.NextLink?.Selected === true;

    const items = resources
      .filter(r => r.ContentType?.startsWith('image/') || r.ContentType?.startsWith('video/'))
      .map(r => {
        const versionParam = r.Hash ? `&v=${r.Hash}` : '';
        return {
          id: r.ID,
          viewUrl: `/v1/resource/view?id=${r.ID}${versionParam}`,
          contentType: r.ContentType,
          name: r.Name || '',
          hash: r.Hash || '',
          width: r.Width || 0,
          height: r.Height || 0,
          ownerName: r.Owner?.Name || '',
          ownerId: r.Owner?.ID || 0
        };
      });

    return { items, hasNextPage };
  },

  // Content type helpers
  isImage(contentType) {
    return contentType?.startsWith('image/') && !this.isSvg(contentType);
  },

  isSvg(contentType) {
    return contentType === 'image/svg+xml';
  },

  isVideo(contentType) {
    return contentType?.startsWith('video/');
  },

  getCurrentItem() {
    return this.items[this.currentIndex];
  },

  // True when the item under currentIndex is the one that failed to load. Keyed on the id so
  // it clears by itself the moment the user navigates elsewhere.
  hasMediaError() {
    const item = this.getCurrentItem();
    return !!item && this.mediaErrorId === item.id;
  },

  // The URL a media element is actually showing, as an absolute URL. <object> carries it on
  // `data` rather than `src`, so reading only src/currentSrc left every SVG event unfenced.
  // Use the IDL property, never getAttribute('data'): the attribute is the raw relative
  // string from the template, which never equals the resolved URL we compare it against.
  _mediaEventUrl(event) {
    return this._mediaElementUrl(event?.target);
  },

  _mediaElementUrl(el) {
    if (!el) return '';
    return el.currentSrc || el.src || el.data || '';
  },

  // The media element is reused across navigation, so an event queued for the previous item
  // can run after the swap. Acting on it would clear the new item's spinner early or flag a
  // perfectly good image as broken. Fails open: when either URL is unknown we handle the
  // event, because a stuck spinner is worse than a mis-attributed one.
  _isEventForCurrentMedia(event) {
    const wanted = this.getCurrentItem()?.viewUrl;
    const actual = this._mediaEventUrl(event);
    if (!wanted || !actual) return true;
    return new URL(wanted, document.baseURI).href === actual;
  },

  onMediaLoaded(event) {
    if (!this._isEventForCurrentMedia(event)) return;
    this.loading = false;
    this.mediaErrorId = null;
    // Re-clamp against the bitmap that just arrived. Pan bounds come from the live media
    // element, so a pan still holding limits computed for the previous image can translate
    // this one outside the media column and read as a blank viewer.
    this.constrainPan();
  },

  onMediaError(event) {
    if (!this._isEventForCurrentMedia(event)) return;
    const item = this.getCurrentItem();
    // A broken/404 media file otherwise leaves the spinner silently vanishing with no
    // signal to assistive tech that the load failed (BH: M8).
    this.loading = false;
    this.mediaErrorId = item?.id ?? null;
    const mediaType = this.isVideo(item?.contentType) ? 'video' : this.isSvg(item?.contentType) ? 'SVG' : 'image';
    this.announce(`Failed to load ${mediaType}: ${item?.name || 'media'}`);
  },

  // This runs when the element was already complete before any @load could fire (a cached
  // bitmap), so it is a second success path and has to clear the error state too — otherwise
  // an error left by a previous item with the same id shows over media that did load.
  checkIfMediaLoaded(el) {
    if (!el) return;
    if (el.tagName === 'IMG' && el.complete) {
      // Same fence as the event handlers: a complete element may still be showing the
      // previous item's bitmap for a frame after the swap.
      if (!this._isElementShowingCurrentMedia(el)) return;
      const isSvg = this.isSvg(this.getCurrentItem()?.contentType);
      if (isSvg || el.naturalWidth > 0) {
        this.loading = false;
        this.mediaErrorId = null;
        return;
      }
    }
    if (el.tagName === 'OBJECT') {
      // The <object> is reused across navigation, so contentDocument still holds the PREVIOUS
      // SVG while the new one loads; clearing the spinner off that would present the old
      // drawing as the new one. Gate on the element's own `data` rather than on
      // contentDocument.URL: /v1/resource/view redirects to the stored file, so the loaded
      // document's URL is never the viewUrl we would be comparing it to, and that check could
      // only ever reject — which is exactly how it stranded the SVG spinner.
      if (el.contentDocument && this._isElementShowingCurrentMedia(el)) {
        this.loading = false;
        this.mediaErrorId = null;
      }
      return;
    }
    if (el.tagName === 'VIDEO' && el.readyState >= 3) {
      if (!this._isElementShowingCurrentMedia(el)) return;
      this.loading = false;
      this.mediaErrorId = null;
    }
  },

  _isElementShowingCurrentMedia(el) {
    const wanted = this.getCurrentItem()?.viewUrl;
    const actual = this._mediaElementUrl(el);
    if (!wanted || !actual) return true;
    return new URL(wanted, document.baseURI).href === actual;
  },

  scheduleMediaCheck() {
    requestAnimationFrame(() => {
      requestAnimationFrame(() => {
        const el = document.querySelector('[role="dialog"] img, [role="dialog"] video, [role="dialog"] object');
        if (el) {
          this.checkIfMediaLoaded(el);
        }
      });
    });
  },

  restartVideo() {
    const video = document.querySelector('[role="dialog"] video');
    if (!video) return;
    video.currentTime = 0;
    video.play();
  },

  pauseCurrentVideo() {
    const video = document.querySelector('[x-show="$store.lightbox.isOpen"] video');
    if (video && !video.paused) {
      video.pause();
    }
  },
};
