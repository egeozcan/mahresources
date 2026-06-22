import { abortableFetch } from '../../index.js';

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
    this.items = allItems;

    const urlParams = new URLSearchParams(window.location.search);
    this.currentPage = parseInt(urlParams.get('page'), 10) || 1;
    this.baseUrl = window.location.pathname + window.location.search;

    const paginationNav = document.querySelector('nav[aria-label="Pagination"]');
    if (paginationNav) {
      this.hasNextPage = paginationNav.dataset.hasNext === 'true';
      this.hasPrevPage = paginationNav.dataset.hasPrev === 'true';
    } else {
      this.hasPrevPage = this.currentPage > 1;
      this.hasNextPage = false;
    }

    this.loadedPages.add(this.currentPage);

    const pageSizeAttr = containers[0].dataset.pageSize;
    if (pageSizeAttr) {
      this.pageSize = parseInt(pageSizeAttr, 10) || 50;
    } else if (window.location.pathname.includes('/simple')) {
      this.pageSize = 200;
    }
  },

  openFromClick(event, resourceId, contentType) {
    if (!contentType?.startsWith('image/') && !contentType?.startsWith('video/')) {
      window.location.href = event.currentTarget.href;
      return;
    }

    event.preventDefault();
    this.triggerElement = event.currentTarget;

    // Check if inside a container with a lightbox source (multi-section pages)
    const sourceContainer = event.currentTarget.closest('[data-lightbox-source]');
    if (sourceContainer) {
      this._openFromSourceContainer(sourceContainer, resourceId);
      return;
    }

    const index = this.items.findIndex(item => item.id === resourceId);
    if (index !== -1) {
      this.open(index);
    }
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
    this.currentPage = 1;
    this.loadedPages = new Set([1]);
    this.pageSize = 50;
    // The group/series page preloads up to 5 resources (pageLimitCustom(5) server-side),
    // counted BEFORE filtering out non-media. Deriving this from the filtered `containerItems`
    // would hide collection media whenever the preview mixes media and non-media (BH: H1).
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

    // Blur the trigger element so x-trap can move focus into the lightbox
    if (document.activeElement && document.activeElement !== document.body) {
      document.activeElement.blur();
    }

    const item = this.getCurrentItem();
    const mediaType = this.isVideo(item?.contentType) ? 'video' : this.isSvg(item?.contentType) ? 'SVG' : 'image';
    this.announce(`Opened ${mediaType}: ${item?.name || 'media'}. ${this.currentIndex + 1} of ${this.items.length}`);

    this.scheduleMediaCheck();
    this._preloadUpcoming();

    // Restore quick tag panel from localStorage if it was previously open
    if (this.quickTagPanelOpen) {
      this.fetchResourceDetails();
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

    if (this.triggerElement) {
      this.triggerElement.focus({ preventScroll: true });
      this.triggerElement = null;
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

  announcePosition(prefix = '') {
    const item = this.getCurrentItem();
    this.announce(`${prefix}${item?.name || 'Media'}, ${this.currentIndex + 1} of ${this.items.length}`);
  },

  async loadNextPage() {
    const nextPage = this.currentPage + 1;

    if (this.loadedPages.has(nextPage)) {
      this.currentPage = nextPage;
      return 0;
    }

    this.pageLoading = true;
    this.announce('Loading more items...');

    try {
      const { items: newItems, hasNextPage } = await this.fetchPage(nextPage);

      if (newItems.length === 0) {
        this.hasNextPage = false;
        this.announce('No more items');
        return 0;
      }

      this.items = [...this.items, ...newItems];
      this.loadedPages.add(nextPage);
      this.currentPage = nextPage;
      this.hasNextPage = hasNextPage;

      // The "loaded N more" status is announced (combined with position) by next() so it
      // is not immediately clobbered by the position announcement (BH: M9).
      return newItems.length;
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
    if (this.currentPage <= 1) return 0;

    const prevPage = this.currentPage - 1;

    if (this.loadedPages.has(prevPage)) {
      this.currentPage = prevPage;
      this.hasPrevPage = prevPage > 1;
      return 0;
    }

    this.pageLoading = true;
    this.announce('Loading previous items...');

    try {
      const { items: newItems } = await this.fetchPage(prevPage);
      const prevItemCount = newItems.length;

      this.items = [...newItems, ...this.items];
      this.currentIndex += prevItemCount;
      this.loadedPages.add(prevPage);
      // Track the page the user is now viewing, matching loadNextPage. Without this the
      // backward-pagination boundary stalls for one keypress (BH: M1).
      this.currentPage = prevPage;
      this.hasPrevPage = prevPage > 1;

      // Status announced (combined with position) by prev() to avoid clobbering (BH: M9).
      return prevItemCount;
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

  onMediaLoaded() {
    this.loading = false;
  },

  onMediaError() {
    // A broken/404 media file otherwise leaves the spinner silently vanishing with no
    // signal to assistive tech that the load failed (BH: M8).
    this.loading = false;
    const item = this.getCurrentItem();
    const mediaType = this.isVideo(item?.contentType) ? 'video' : this.isSvg(item?.contentType) ? 'SVG' : 'image';
    this.announce(`Failed to load ${mediaType}: ${item?.name || 'media'}`);
  },

  checkIfMediaLoaded(el) {
    if (!el) return;
    if (el.tagName === 'IMG' && el.complete) {
      const isSvg = this.isSvg(this.getCurrentItem()?.contentType);
      if (isSvg || el.naturalWidth > 0) {
        this.loading = false;
        return;
      }
    }
    if (el.tagName === 'OBJECT' && el.contentDocument) {
      this.loading = false;
      return;
    }
    if (el.tagName === 'VIDEO' && el.readyState >= 3) {
      this.loading = false;
    }
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
