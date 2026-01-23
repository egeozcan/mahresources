import { abortableFetch } from '../index.js';

/**
 * Register the lightbox Alpine store
 * @param {import('alpinejs').Alpine} Alpine
 */
export function registerLightboxStore(Alpine) {
  Alpine.store('lightbox', {
    // State
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

    // Touch handling
    touchStartX: null,
    touchStartY: null,

    // Request aborter for canceling in-flight requests
    requestAborter: null,

    // Reference to trigger element for focus restoration
    triggerElement: null,

    // Live region for screen reader announcements
    liveRegion: null,

    init() {
      // Guard against multiple initializations (prevents memory leak)
      if (this.liveRegion) return;

      // Create ARIA live region for screen reader announcements
      this.liveRegion = document.createElement('div');
      this.liveRegion.setAttribute('role', 'status');
      this.liveRegion.setAttribute('aria-live', 'polite');
      this.liveRegion.setAttribute('aria-atomic', 'true');
      Object.assign(this.liveRegion.style, {
        position: 'absolute',
        width: '1px',
        height: '1px',
        padding: '0',
        margin: '-1px',
        overflow: 'hidden',
        clip: 'rect(0, 0, 0, 0)',
        whiteSpace: 'nowrap',
        border: '0'
      });
      document.body.appendChild(this.liveRegion);
    },

    announce(message) {
      if (this.liveRegion) {
        this.liveRegion.textContent = '';
        setTimeout(() => {
          this.liveRegion.textContent = message;
        }, 50);
      }
    },

    /**
     * Initialize lightbox from DOM elements
     * Call this on DOMContentLoaded to set up items from existing links
     */
    initFromDOM() {
      const container = document.querySelector('.list-container, .gallery');
      if (!container) return;

      // Parse lightbox items from DOM
      const links = container.querySelectorAll('[data-lightbox-item]');
      this.items = Array.from(links).map((link, index) => ({
        id: parseInt(link.dataset.resourceId, 10),
        viewUrl: `/v1/resource/view?id=${link.dataset.resourceId}`,
        contentType: link.dataset.contentType || '',
        name: link.dataset.resourceName || link.querySelector('img')?.alt || '',
        domIndex: index
      })).filter(item =>
        item.contentType?.startsWith('image/') ||
        item.contentType?.startsWith('video/')
      );

      // Parse pagination state from URL
      const urlParams = new URLSearchParams(window.location.search);
      this.currentPage = parseInt(urlParams.get('page'), 10) || 1;

      // Store base URL for fetching pages
      this.baseUrl = window.location.pathname + window.location.search;

      // Check pagination state from data attributes (reliable method)
      const paginationNav = document.querySelector('nav[aria-label="Pagination"]');
      if (paginationNav) {
        this.hasNextPage = paginationNav.dataset.hasNext === 'true';
        this.hasPrevPage = paginationNav.dataset.hasPrev === 'true';
      } else {
        // Fallback: infer from current page
        this.hasPrevPage = this.currentPage > 1;
        this.hasNextPage = false;
      }

      // Mark current page as loaded
      this.loadedPages.add(this.currentPage);

      // Parse page size from data attribute or fall back to path-based detection
      const pageSizeAttr = container.dataset.pageSize;
      if (pageSizeAttr) {
        this.pageSize = parseInt(pageSizeAttr, 10) || 50;
      } else if (window.location.pathname.includes('/simple')) {
        // Fallback for templates without data attribute
        this.pageSize = 200;
      }
    },

    /**
     * Open lightbox from a click event
     * @param {MouseEvent} event
     * @param {number} resourceId
     * @param {string} contentType
     */
    openFromClick(event, resourceId, contentType) {
      // Only handle images and videos
      if (!contentType?.startsWith('image/') && !contentType?.startsWith('video/')) {
        window.location.href = event.currentTarget.href;
        return;
      }

      event.preventDefault();
      this.triggerElement = event.currentTarget;

      const index = this.items.findIndex(item => item.id === resourceId);
      if (index !== -1) {
        this.open(index);
      }
    },

    /**
     * Open lightbox at specific index
     * @param {number} index
     */
    open(index) {
      this.currentIndex = index;
      this.isOpen = true;
      this.loading = true;
      document.body.style.overflow = 'hidden';

      const item = this.getCurrentItem();
      this.announce(`Opened ${this.isVideo(item?.contentType) ? 'video' : 'image'}: ${item?.name || 'media'}. ${this.currentIndex + 1} of ${this.items.length}`);

      // Schedule a check for cached media after Alpine renders the new element
      this.scheduleMediaCheck();
    },

    /**
     * Close lightbox
     */
    close() {
      // Pause any playing video before closing
      this.pauseCurrentVideo();

      this.isOpen = false;
      this.loading = false;
      document.body.style.overflow = '';

      // Cancel any pending requests
      if (this.requestAborter) {
        this.requestAborter();
        this.requestAborter = null;
      }

      // Restore focus to trigger element
      if (this.triggerElement) {
        this.triggerElement.focus();
        this.triggerElement = null;
      }

      this.announce('Media viewer closed');
    },

    /**
     * Navigate to next item
     */
    async next() {
      if (this.pageLoading) return;

      // Pause any playing video before navigating
      this.pauseCurrentVideo();

      if (this.currentIndex < this.items.length - 1) {
        this.currentIndex++;
        this.loading = true;
        this.announcePosition();
        this.scheduleMediaCheck();
      } else if (this.hasNextPage) {
        await this.loadNextPage();
        if (this.currentIndex < this.items.length - 1) {
          this.currentIndex++;
          this.loading = true;
          this.announcePosition();
          this.scheduleMediaCheck();
        }
      }
    },

    /**
     * Navigate to previous item
     */
    async prev() {
      if (this.pageLoading) return;

      // Pause any playing video before navigating
      this.pauseCurrentVideo();

      if (this.currentIndex > 0) {
        this.currentIndex--;
        this.loading = true;
        this.announcePosition();
        this.scheduleMediaCheck();
      } else if (this.hasPrevPage) {
        const prevItemCount = await this.loadPrevPage();
        if (prevItemCount > 0) {
          // After prepending items, the index shifts
          this.currentIndex = prevItemCount - 1;
          this.loading = true;
          this.announcePosition();
          this.scheduleMediaCheck();
        }
      }
    },

    announcePosition() {
      const item = this.getCurrentItem();
      this.announce(`${item?.name || 'Media'}, ${this.currentIndex + 1} of ${this.items.length}`);
    },

    /**
     * Load next page of items
     */
    async loadNextPage() {
      const nextPage = this.currentPage + 1;

      // Check if already loaded
      if (this.loadedPages.has(nextPage)) {
        this.currentPage = nextPage;
        return;
      }

      this.pageLoading = true;
      this.announce('Loading more items...');

      try {
        const newItems = await this.fetchPage(nextPage);

        // Handle empty page (end of results)
        if (newItems.length === 0) {
          this.hasNextPage = false;
          this.announce('No more items');
          return;
        }

        this.items = [...this.items, ...newItems];
        this.loadedPages.add(nextPage);
        this.currentPage = nextPage;

        // Only assume more pages if we got a full page of items
        // This may still cause one extra fetch when total is exactly divisible,
        // but the empty page check above handles that gracefully
        this.hasNextPage = newItems.length >= this.pageSize;

        this.announce(`Loaded ${newItems.length} more items`);
      } catch (err) {
        if (err.name !== 'AbortError') {
          console.error('Failed to load next page:', err);
          this.announce('Failed to load more items');
        }
      } finally {
        this.pageLoading = false;
      }
    },

    /**
     * Load previous page of items
     * @returns {Promise<number>} Number of items loaded
     */
    async loadPrevPage() {
      if (this.currentPage <= 1) return 0;

      const prevPage = this.currentPage - 1;

      // Check if already loaded (unlikely for prev, but be safe)
      // If already loaded, items are already in array - just update pagination state
      if (this.loadedPages.has(prevPage)) {
        this.currentPage = prevPage;
        this.hasPrevPage = prevPage > 1;
        // Return 0 since no new items were prepended - caller won't change index
        // which is correct since we can't navigate further back
        return 0;
      }

      this.pageLoading = true;
      this.announce('Loading previous items...');

      try {
        const newItems = await this.fetchPage(prevPage);
        const prevItemCount = newItems.length;

        // Prepend items and adjust current index
        this.items = [...newItems, ...this.items];
        this.currentIndex += prevItemCount;
        this.loadedPages.add(prevPage);

        // Update pagination state
        this.hasPrevPage = prevPage > 1;

        this.announce(`Loaded ${prevItemCount} previous items`);
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

    /**
     * Fetch a specific page of resources
     * @param {number} pageNum
     * @returns {Promise<Array>}
     */
    async fetchPage(pageNum) {
      // Cancel any existing request
      if (this.requestAborter) {
        this.requestAborter();
      }

      // Build URL with same filters but different page
      const url = new URL(this.baseUrl, window.location.origin);
      url.searchParams.set('page', pageNum);

      // Fetch JSON version of the endpoint
      const jsonUrl = url.pathname + '.json' + url.search;

      const { abort, ready } = abortableFetch(jsonUrl);
      this.requestAborter = abort;

      const response = await ready;
      if (!response.ok) {
        throw new Error(`Failed to fetch page: ${response.status}`);
      }
      const resources = await response.json();
      this.requestAborter = null;

      // Map to lightbox item format, filtering to only images/videos
      return resources
        .filter(r => r.ContentType?.startsWith('image/') || r.ContentType?.startsWith('video/'))
        .map(r => ({
          id: r.ID,
          viewUrl: `/v1/resource/view?id=${r.ID}`,
          contentType: r.ContentType,
          name: r.Name || ''
        }));
    },

    /**
     * Check if content type is an image
     * @param {string} contentType
     * @returns {boolean}
     */
    isImage(contentType) {
      return contentType?.startsWith('image/');
    },

    /**
     * Check if content type is a video
     * @param {string} contentType
     * @returns {boolean}
     */
    isVideo(contentType) {
      return contentType?.startsWith('video/');
    },

    /**
     * Get current item
     * @returns {Object|undefined}
     */
    getCurrentItem() {
      return this.items[this.currentIndex];
    },

    /**
     * Handle media load complete
     */
    onMediaLoaded() {
      this.loading = false;
    },

    /**
     * Check if media element is already loaded (handles cached media)
     * Called via x-init to detect if image/video loaded before Alpine wired up @load handler
     * @param {HTMLElement} el - The img or video element
     */
    checkIfMediaLoaded(el) {
      if (!el) return;
      // Images: check 'complete' property and naturalWidth > 0
      if (el.tagName === 'IMG' && el.complete && el.naturalWidth > 0) {
        this.loading = false;
        return;
      }
      // Videos: check readyState >= 3 (HAVE_FUTURE_DATA)
      if (el.tagName === 'VIDEO' && el.readyState >= 3) {
        this.loading = false;
      }
    },

    /**
     * Schedule a check for cached media after Alpine renders
     * Uses double requestAnimationFrame to ensure DOM is fully updated
     */
    scheduleMediaCheck() {
      requestAnimationFrame(() => {
        requestAnimationFrame(() => {
          const el = document.querySelector('[role="dialog"] img, [role="dialog"] video');
          if (el) {
            this.checkIfMediaLoaded(el);
          }
        });
      });
    },

    /**
     * Pause any currently playing video
     * Called before navigation to prevent videos playing in background
     */
    pauseCurrentVideo() {
      const video = document.querySelector('[x-show="$store.lightbox.isOpen"] video');
      if (video && !video.paused) {
        video.pause();
      }
    },

    /**
     * Handle touch start for swipe gestures
     * @param {TouchEvent} event
     */
    handleTouchStart(event) {
      this.touchStartX = event.touches[0].clientX;
      this.touchStartY = event.touches[0].clientY;
    },

    /**
     * Handle touch end for swipe gestures
     * @param {TouchEvent} event
     */
    handleTouchEnd(event) {
      if (this.touchStartX === null) return;

      const touchEndX = event.changedTouches[0].clientX;
      const touchEndY = event.changedTouches[0].clientY;
      const diffX = this.touchStartX - touchEndX;
      const diffY = this.touchStartY - touchEndY;

      // Only handle horizontal swipes (ignore vertical scrolling)
      if (Math.abs(diffX) > Math.abs(diffY) && Math.abs(diffX) > 50) {
        if (diffX > 0) {
          this.next();
        } else {
          this.prev();
        }
      }

      this.touchStartX = null;
      this.touchStartY = null;
    }
  });
}
