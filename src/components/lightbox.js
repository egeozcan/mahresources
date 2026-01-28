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

    // Edit panel state
    editPanelOpen: false,
    resourceDetails: null,
    detailsLoading: false,
    detailsCache: new Map(),
    detailsAborter: null,

    // Tag editing - API methods only, UI state handled by autocompleter component
    savingTag: false,

    // Track if changes were made that require refreshing the page content
    needsRefreshOnClose: false,

    // Fullscreen state
    isFullscreen: false,

    // Zoom state
    zoomLevel: 1,
    minZoom: 1,
    maxZoom: 5,
    panX: 0,
    panY: 0,
    zoomIndicatorVisible: false,
    zoomIndicatorTimeout: null,

    // Pinch tracking
    pinchStartDistance: null,
    pinchStartZoom: null,
    pinchStartCenterX: null,
    pinchStartCenterY: null,
    pinchCenterX: null,
    pinchCenterY: null,

    // Two-finger pan tracking (when zoomed)
    twoFingerPanStartX: null,
    twoFingerPanStartY: null,
    twoFingerPanStartPanX: null,
    twoFingerPanStartPanY: null,

    // Mouse drag tracking
    isDragging: false,
    dragStartX: null,
    dragStartY: null,
    dragStartPanX: null,
    dragStartPanY: null,
    dragVelocityX: 0,
    dragVelocityY: 0,
    lastDragTime: null,
    lastDragX: null,
    lastDragY: null,

    // Image dimensions for pan bounds
    imageRect: null,
    containerRect: null,

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

      // Listen for fullscreen changes
      const handleFullscreenChange = () => {
        this.isFullscreen = !!(document.fullscreenElement || document.webkitFullscreenElement);
        this.announce(this.isFullscreen ? 'Entered fullscreen' : 'Exited fullscreen');
      };
      document.addEventListener('fullscreenchange', handleFullscreenChange);
      document.addEventListener('webkitfullscreenchange', handleFullscreenChange);
    },

    /**
     * Check if Fullscreen API is supported
     * @returns {boolean}
     */
    fullscreenSupported() {
      return !!(document.fullscreenEnabled || document.webkitFullscreenEnabled);
    },

    /**
     * Toggle fullscreen mode
     */
    async toggleFullscreen() {
      if (!this.fullscreenSupported()) return;

      const container = document.querySelector('[role="dialog"][aria-modal="true"]');
      if (!container) return;

      try {
        if (!this.isFullscreen) {
          if (container.requestFullscreen) {
            await container.requestFullscreen();
          } else if (container.webkitRequestFullscreen) {
            await container.webkitRequestFullscreen();
          }
        } else {
          if (document.exitFullscreen) {
            await document.exitFullscreen();
          } else if (document.webkitExitFullscreen) {
            await document.webkitExitFullscreen();
          }
        }
      } catch (err) {
        console.error('Fullscreen toggle failed:', err);
      }
    },

    /**
     * Check if currently zoomed in
     * @returns {boolean}
     */
    isZoomed() {
      return this.zoomLevel > 1;
    },

    /**
     * Set zoom level with bounds checking and indicator display
     * @param {number} level
     */
    setZoomLevel(level) {
      const oldLevel = this.zoomLevel;
      this.zoomLevel = Math.max(this.minZoom, Math.min(this.maxZoom, level));

      if (this.zoomLevel !== oldLevel) {
        this.showZoomIndicator();
        if (this.zoomLevel === 1) {
          this.panX = 0;
          this.panY = 0;
        }
      }
    },

    /**
     * Show zoom indicator and auto-hide after delay
     */
    showZoomIndicator() {
      this.zoomIndicatorVisible = true;

      if (this.zoomIndicatorTimeout) {
        clearTimeout(this.zoomIndicatorTimeout);
      }

      this.zoomIndicatorTimeout = setTimeout(() => {
        this.zoomIndicatorVisible = false;
        this.zoomIndicatorTimeout = null;
      }, 1500);
    },

    /**
     * Reset zoom and pan to default
     */
    resetZoom() {
      this.zoomLevel = 1;
      this.panX = 0;
      this.panY = 0;
      this.zoomIndicatorVisible = false;
      if (this.zoomIndicatorTimeout) {
        clearTimeout(this.zoomIndicatorTimeout);
        this.zoomIndicatorTimeout = null;
      }
    },

    /**
     * Announce zoom level to screen readers
     */
    announceZoom() {
      if (this.zoomLevel === 1) {
        this.announce('Zoom reset to 100%');
      } else {
        this.announce(`Zoomed to ${Math.round(this.zoomLevel * 100)}%`);
      }
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
      this.items = Array.from(links).map((link, index) => {
        const hash = link.dataset.resourceHash || '';
        const versionParam = hash ? `&v=${hash}` : '';
        return {
          id: parseInt(link.dataset.resourceId, 10),
          viewUrl: `/v1/resource/view?id=${link.dataset.resourceId}${versionParam}`,
          contentType: link.dataset.contentType || '',
          name: link.dataset.resourceName || link.querySelector('img')?.alt || '',
          hash: hash,
          domIndex: index
        };
      }).filter(item =>
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
      const mediaType = this.isVideo(item?.contentType) ? 'video' : this.isSvg(item?.contentType) ? 'SVG' : 'image';
      this.announce(`Opened ${mediaType}: ${item?.name || 'media'}. ${this.currentIndex + 1} of ${this.items.length}`);

      // Schedule a check for cached media after Alpine renders the new element
      this.scheduleMediaCheck();
    },

    /**
     * Close lightbox
     */
    close() {
      // Pause any playing video before closing
      this.pauseCurrentVideo();

      // Exit fullscreen if active
      if (this.isFullscreen) {
        if (document.exitFullscreen) {
          document.exitFullscreen().catch(() => {});
        } else if (document.webkitExitFullscreen) {
          document.webkitExitFullscreen();
        }
        this.isFullscreen = false;
      }

      // Close edit panel if open
      if (this.editPanelOpen) {
        this.closeEditPanel();
      }

      this.isOpen = false;
      this.loading = false;
      document.body.style.overflow = '';

      // Clear resource details to prevent stale data when reopening
      this.resourceDetails = null;

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
      this.resetZoom();

      if (this.currentIndex < this.items.length - 1) {
        this.currentIndex++;
        this.loading = true;
        this.announcePosition();
        this.scheduleMediaCheck();
        this.onResourceChange();
      } else if (this.hasNextPage) {
        await this.loadNextPage();
        if (this.currentIndex < this.items.length - 1) {
          this.currentIndex++;
          this.loading = true;
          this.announcePosition();
          this.scheduleMediaCheck();
          this.onResourceChange();
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
      this.resetZoom();

      if (this.currentIndex > 0) {
        this.currentIndex--;
        this.loading = true;
        this.announcePosition();
        this.scheduleMediaCheck();
        this.onResourceChange();
      } else if (this.hasPrevPage) {
        const prevItemCount = await this.loadPrevPage();
        if (prevItemCount > 0) {
          // After prepending items, the index shifts
          this.currentIndex = prevItemCount - 1;
          this.loading = true;
          this.announcePosition();
          this.scheduleMediaCheck();
          this.onResourceChange();
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
        const { items: newItems, hasNextPage } = await this.fetchPage(nextPage);

        // Handle empty page (end of results)
        if (newItems.length === 0) {
          this.hasNextPage = false;
          this.announce('No more items');
          return;
        }

        this.items = [...this.items, ...newItems];
        this.loadedPages.add(nextPage);
        this.currentPage = nextPage;

        // Use the server's pagination info to determine if there are more pages
        this.hasNextPage = hasNextPage;

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
        const { items: newItems } = await this.fetchPage(prevPage);
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
     * @returns {Promise<{items: Array, hasNextPage: boolean}>}
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
      const data = await response.json();
      this.requestAborter = null;

      // The JSON response is the full template context with 'resources' and 'pagination' keys
      const resources = data.resources || [];
      const pagination = data.pagination || {};

      // Determine if there's a next page from the server's pagination info
      // NextLink.Selected is true when there's a next page available
      const hasNextPage = pagination.NextLink?.Selected === true;

      // Map to lightbox item format, filtering to only images/videos
      const items = resources
        .filter(r => r.ContentType?.startsWith('image/') || r.ContentType?.startsWith('video/'))
        .map(r => {
          const versionParam = r.Hash ? `&v=${r.Hash}` : '';
          return {
            id: r.ID,
            viewUrl: `/v1/resource/view?id=${r.ID}${versionParam}`,
            contentType: r.ContentType,
            name: r.Name || '',
            hash: r.Hash || ''
          };
        });

      return { items, hasNextPage };
    },

    /**
     * Check if content type is an image (excluding SVG which gets special handling)
     * @param {string} contentType
     * @returns {boolean}
     */
    isImage(contentType) {
      return contentType?.startsWith('image/') && !this.isSvg(contentType);
    },

    /**
     * Check if content type is an SVG
     * @param {string} contentType
     * @returns {boolean}
     */
    isSvg(contentType) {
      return contentType === 'image/svg+xml';
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
      // For SVGs, naturalWidth may be 0 if no explicit dimensions, so just check complete
      if (el.tagName === 'IMG' && el.complete) {
        const isSvg = this.isSvg(this.getCurrentItem()?.contentType);
        if (isSvg || el.naturalWidth > 0) {
          this.loading = false;
          return;
        }
      }
      // Object elements (used for SVG): check if contentDocument is accessible
      if (el.tagName === 'OBJECT' && el.contentDocument) {
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
          const el = document.querySelector('[role="dialog"] img, [role="dialog"] video, [role="dialog"] object');
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
     * Calculate distance between two touch points
     * @param {TouchList} touches
     * @returns {number}
     */
    getPinchDistance(touches) {
      const dx = touches[0].clientX - touches[1].clientX;
      const dy = touches[0].clientY - touches[1].clientY;
      return Math.sqrt(dx * dx + dy * dy);
    },

    /**
     * Get center point between two touches
     * @param {TouchList} touches
     * @returns {{x: number, y: number}}
     */
    getPinchCenter(touches) {
      return {
        x: (touches[0].clientX + touches[1].clientX) / 2,
        y: (touches[0].clientY + touches[1].clientY) / 2
      };
    },

    /**
     * Handle touch start for swipe and pinch gestures
     * @param {TouchEvent} event
     */
    handleTouchStart(event) {
      // Ignore touches that start within the edit panel
      if (event.target.closest('[data-edit-panel]')) {
        this.touchStartX = null;
        return;
      }

      if (event.touches.length === 2) {
        // Pinch gesture start
        event.preventDefault();
        this.pinchStartDistance = this.getPinchDistance(event.touches);
        this.pinchStartZoom = this.zoomLevel;
        const center = this.getPinchCenter(event.touches);
        this.pinchStartCenterX = center.x;
        this.pinchStartCenterY = center.y;
        this.pinchCenterX = center.x;
        this.pinchCenterY = center.y;

        // Also track for pan if zoomed
        if (this.isZoomed()) {
          this.twoFingerPanStartX = center.x;
          this.twoFingerPanStartY = center.y;
          this.twoFingerPanStartPanX = this.panX;
          this.twoFingerPanStartPanY = this.panY;
        }
      } else if (event.touches.length === 1) {
        // Single touch - swipe or drag
        this.touchStartX = event.touches[0].clientX;
        this.touchStartY = event.touches[0].clientY;
        if (this.isZoomed()) {
          this.dragStartPanX = this.panX;
          this.dragStartPanY = this.panY;
        }
      }
    },

    /**
     * Handle touch move for pinch zoom
     * @param {TouchEvent} event
     */
    handleTouchMove(event) {
      // Skip for videos
      if (this.isVideo(this.getCurrentItem()?.contentType)) return;

      if (event.touches.length === 2) {
        event.preventDefault();

        const center = this.getPinchCenter(event.touches);

        if (this.pinchStartDistance !== null) {
          // Pinch zoom
          const currentDistance = this.getPinchDistance(event.touches);
          const scale = currentDistance / this.pinchStartDistance;
          this.setZoomLevel(this.pinchStartZoom * scale);

          // Track current center for two-finger swipe navigation
          this.pinchCenterX = center.x;
          this.pinchCenterY = center.y;
        }

        // Two-finger pan when zoomed
        if (this.isZoomed() && this.twoFingerPanStartX !== null) {
          const dx = center.x - this.twoFingerPanStartX;
          const dy = center.y - this.twoFingerPanStartY;
          this.panX = this.twoFingerPanStartPanX + dx / this.zoomLevel;
          this.panY = this.twoFingerPanStartPanY + dy / this.zoomLevel;
          this.constrainPan();
        }
      } else if (event.touches.length === 1 && this.isZoomed()) {
        // Single finger pan when zoomed
        if (this.touchStartX !== null) {
          event.preventDefault();
          const dx = event.touches[0].clientX - this.touchStartX;
          const dy = event.touches[0].clientY - this.touchStartY;
          this.panX = (this.dragStartPanX || 0) + dx / this.zoomLevel;
          this.panY = (this.dragStartPanY || 0) + dy / this.zoomLevel;
          this.constrainPan();
        }
      }
    },

    /**
     * Handle touch end for swipe and pinch gestures
     * @param {TouchEvent} event
     */
    handleTouchEnd(event) {
      // Handle pinch/two-finger gesture end
      if (this.pinchStartDistance !== null) {
        // Snap back if below minimum
        if (this.zoomLevel < this.minZoom) {
          this.setZoomLevel(this.minZoom);
        }

        // Two-finger swipe navigation when not zoomed
        if (!this.isZoomed() && this.pinchStartCenterX !== null && this.pinchCenterX !== null) {
          // Calculate how far the center point moved from start to end
          const diffX = this.pinchStartCenterX - this.pinchCenterX;
          if (Math.abs(diffX) > 50) {
            if (diffX > 0) {
              this.next();
            } else {
              this.prev();
            }
          }
        }

        this.pinchStartDistance = null;
        this.pinchStartZoom = null;
        this.pinchStartCenterX = null;
        this.pinchStartCenterY = null;
        this.pinchCenterX = null;
        this.pinchCenterY = null;
        this.twoFingerPanStartX = null;
        this.twoFingerPanStartY = null;
        this.twoFingerPanStartPanX = null;
        this.twoFingerPanStartPanY = null;
        this.announceZoom();
        return;
      }

      // Handle swipe (existing logic)
      if (this.touchStartX === null) return;

      const touchEndX = event.changedTouches[0].clientX;
      const touchEndY = event.changedTouches[0].clientY;
      const diffX = this.touchStartX - touchEndX;
      const diffY = this.touchStartY - touchEndY;

      // Only handle horizontal swipes (ignore vertical scrolling)
      if (Math.abs(diffX) > Math.abs(diffY) && Math.abs(diffX) > 50) {
        if (this.isZoomed()) {
          // Pan is handled by handleTouchMove, swipe ignored when zoomed
        } else {
          // Navigate when not zoomed
          if (diffX > 0) {
            this.next();
          } else {
            this.prev();
          }
        }
      }

      this.touchStartX = null;
      this.touchStartY = null;
    },

    /**
     * Handle wheel events for trackpad gestures
     * @param {WheelEvent} event
     */
    handleWheel(event) {
      // Ignore wheel events within the edit panel
      if (event.target.closest('[data-edit-panel]')) {
        return;
      }

      // Skip for videos
      if (this.isVideo(this.getCurrentItem()?.contentType)) return;

      // Trackpad pinch zoom (ctrlKey is set by browser for pinch gestures)
      if (event.ctrlKey) {
        event.preventDefault();

        // Negate deltaY so pinch-out zooms in and pinch-in zooms out
        const zoomDelta = -event.deltaY * 0.01;
        this.setZoomLevel(this.zoomLevel + zoomDelta);

        // Announce on debounced basis
        if (!this._zoomAnnounceDebounce) {
          this._zoomAnnounceDebounce = true;
          setTimeout(() => {
            this.announceZoom();
            this._zoomAnnounceDebounce = false;
          }, 500);
        }
        return;
      }

      // Horizontal scrolling (trackpad swipe) for navigation when not zoomed
      if (!this.isZoomed()) {
        if (Math.abs(event.deltaX) > Math.abs(event.deltaY) && Math.abs(event.deltaX) > 10) {
          event.preventDefault();

          // Debounce to prevent multiple navigations from a single swipe
          if (this._wheelDebounce) return;
          this._wheelDebounce = true;
          setTimeout(() => { this._wheelDebounce = false; }, 300);

          if (event.deltaX > 0) {
            this.next();
          } else {
            this.prev();
          }
        }
      } else {
        // Pan when zoomed (using trackpad scroll)
        event.preventDefault();
        this.panX -= event.deltaX / this.zoomLevel;
        this.panY -= event.deltaY / this.zoomLevel;
        this.constrainPan();
      }
    },

    /**
     * Get the current media element and its dimensions
     * @returns {{element: HTMLElement, rect: DOMRect}|null}
     */
    getMediaElement() {
      const el = document.querySelector('[role="dialog"] img, [role="dialog"] object');
      if (!el) return null;
      return { element: el, rect: el.getBoundingClientRect() };
    },

    /**
     * Get the container element dimensions
     * @returns {DOMRect|null}
     */
    getContainerRect() {
      const container = document.querySelector('[role="dialog"] .relative.max-h-\\[90vh\\]');
      return container?.getBoundingClientRect() || null;
    },

    /**
     * Constrain pan to keep image edges within or beyond viewport edges
     * When zoomed, you should be able to pan just enough to see all edges
     */
    constrainPan() {
      if (this.zoomLevel <= 1) {
        this.panX = 0;
        this.panY = 0;
        return;
      }

      const media = this.getMediaElement();
      const containerRect = this.getContainerRect();
      if (!media || !containerRect) return;

      const el = media.element;

      // Get displayed dimensions (before zoom transform)
      // The image is displayed at its fitted size within the container
      const displayedWidth = el.clientWidth;
      const displayedHeight = el.clientHeight;

      // Calculate the zoomed dimensions
      const zoomedWidth = displayedWidth * this.zoomLevel;
      const zoomedHeight = displayedHeight * this.zoomLevel;

      // Calculate max pan distances
      // Pan is applied after scale, so we need to divide by zoom level
      const maxPanX = Math.max(0, (zoomedWidth - containerRect.width) / 2 / this.zoomLevel);
      const maxPanY = Math.max(0, (zoomedHeight - containerRect.height) / 2 / this.zoomLevel);

      // Constrain pan
      this.panX = Math.max(-maxPanX, Math.min(maxPanX, this.panX));
      this.panY = Math.max(-maxPanY, Math.min(maxPanY, this.panY));
    },

    // ==================== Edit Panel Methods ====================

    /**
     * Handle escape key - close edit panel first, then fullscreen, then lightbox
     * @returns {boolean} true if escape was handled
     */
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

    /**
     * Open the edit panel and fetch resource details
     */
    async openEditPanel() {
      this.editPanelOpen = true;
      this.needsRefreshOnClose = false; // Reset on open
      this.announce('Edit panel opened');
      await this.fetchResourceDetails();

      // Focus the panel after opening
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

    /**
     * Close the edit panel
     */
    closeEditPanel() {
      this.editPanelOpen = false;

      // Cancel any pending detail request
      if (this.detailsAborter) {
        this.detailsAborter();
        this.detailsAborter = null;
      }

      // Trigger background refresh if changes were made
      if (this.needsRefreshOnClose) {
        this.needsRefreshOnClose = false;
        this.refreshPageContent();
      }

      this.announce('Edit panel closed');
    },

    /**
     * Refresh the page content using Alpine morph
     * Called when edit panel closes after changes were made
     */
    async refreshPageContent() {
      const listContainer = document.querySelector('.list-container, .items-container');
      if (!listContainer) return;

      try {
        // Fetch the current page with .body suffix to get just the body content
        const url = new URL(window.location);
        url.pathname = url.pathname + '.body';

        const response = await fetch(url.toString());
        if (!response.ok) return;

        const html = await response.text();
        const parser = new DOMParser();
        const doc = parser.parseFromString(html, 'text/html');
        const newListContainer = doc.querySelector('.list-container, .items-container');

        if (newListContainer && window.Alpine) {
          // Save scroll position before morph
          const scrollX = window.scrollX;
          const scrollY = window.scrollY;

          window.Alpine.morph(listContainer, newListContainer, {
            updating(el, toEl, childrenOnly, skip) {
              // Preserve Alpine state where possible
              if (el._x_dataStack) {
                toEl._x_dataStack = el._x_dataStack;
              }
            }
          });

          // Restore scroll position after morph
          window.scrollTo(scrollX, scrollY);

          // Re-initialize lightbox items from the updated DOM
          this.initFromDOM();
        }
      } catch (err) {
        console.error('Failed to refresh page content:', err);
      }
    },

    /**
     * Fetch full resource details including tags
     * @param {number} [id] - Resource ID (defaults to current item)
     */
    async fetchResourceDetails(id) {
      const resourceId = id ?? this.getCurrentItem()?.id;
      if (!resourceId) return;

      // Check cache first
      const cached = this.detailsCache.get(resourceId);
      if (cached) {
        this.resourceDetails = cached;
        return;
      }

      this.detailsLoading = true;

      // Cancel any existing request
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

        // Only update if we're still on the same resource (prevents race conditions)
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

    /**
     * Called when navigating to a new resource while edit panel is open
     */
    async onResourceChange() {
      if (!this.editPanelOpen) return;
      // Clear current details to show loading state and ensure autocompleter
      // recreates with fresh data when fetch completes
      this.resourceDetails = null;
      // Force fresh fetch by clearing cache for this resource
      const resourceId = this.getCurrentItem()?.id;
      if (resourceId) {
        this.detailsCache.delete(resourceId);
      }
      await this.fetchResourceDetails();
    },

    /**
     * Update resource name
     * @param {string} newName
     */
    async updateName(newName) {
      const resourceId = this.getCurrentItem()?.id;
      if (!resourceId || !this.resourceDetails) return;

      const oldName = this.resourceDetails.Name;
      if (newName === oldName) return;

      // Optimistically update UI
      this.resourceDetails.Name = newName;

      // Also update the items array for the bottom bar
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

        // Update cache and mark for refresh
        this.detailsCache.set(resourceId, { ...this.resourceDetails });
        this.needsRefreshOnClose = true;
        this.announce('Name updated');
      } catch (err) {
        console.error('Failed to update name:', err);
        // Revert on error
        this.resourceDetails.Name = oldName;
        if (item) {
          item.name = oldName;
        }
        this.announce('Failed to update name');
      }
    },

    /**
     * Update resource description
     * @param {string} newDescription
     */
    async updateDescription(newDescription) {
      const resourceId = this.getCurrentItem()?.id;
      if (!resourceId || !this.resourceDetails) return;

      const oldDescription = this.resourceDetails.Description;
      if (newDescription === oldDescription) return;

      // Optimistically update UI
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

        // Update cache and mark for refresh
        this.detailsCache.set(resourceId, { ...this.resourceDetails });
        this.needsRefreshOnClose = true;
        this.announce('Description updated');
      } catch (err) {
        console.error('Failed to update description:', err);
        // Revert on error
        this.resourceDetails.Description = oldDescription;
        this.announce('Failed to update description');
      }
    },

    // ==================== Tag API Methods (for autocompleter callbacks) ====================

    /**
     * Save a tag addition to the server
     * Called by autocompleter onSelect callback
     * @param {Object} tag - Tag object with ID and Name
     */
    async saveTagAddition(tag) {
      const resourceId = this.getCurrentItem()?.id;
      if (!resourceId || this.savingTag) return;

      this.savingTag = true;

      // The autocompleter has its own array copy, so also update our resourceDetails.Tags
      if (this.resourceDetails) {
        if (!this.resourceDetails.Tags) {
          this.resourceDetails.Tags = [];
        }
        // Only add if not already present
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

        // Update cache and mark for refresh
        if (this.resourceDetails) {
          this.detailsCache.set(resourceId, { ...this.resourceDetails });
        }
        this.needsRefreshOnClose = true;
        this.announce(`Added tag: ${tag.Name}`);
      } catch (err) {
        console.error('Failed to add tag:', err);
        // Revert: remove the tag from our resourceDetails.Tags
        if (this.resourceDetails?.Tags) {
          const idx = this.resourceDetails.Tags.findIndex(t => t.ID === tag.ID);
          if (idx !== -1) {
            this.resourceDetails.Tags.splice(idx, 1);
          }
        }
        this.announce('Failed to add tag');
        throw err; // Re-throw so autocompleter can handle rollback
      } finally {
        this.savingTag = false;
      }
    },

    /**
     * Save a tag removal to the server
     * Called by autocompleter onRemove callback
     * @param {Object} tag - Tag object with ID and Name
     */
    async saveTagRemoval(tag) {
      const resourceId = this.getCurrentItem()?.id;
      if (!resourceId) return;

      // The autocompleter has its own array copy, so also update our resourceDetails.Tags
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

        // Update cache and mark for refresh
        if (this.resourceDetails) {
          this.detailsCache.set(resourceId, { ...this.resourceDetails });
        }
        this.needsRefreshOnClose = true;
        this.announce(`Removed tag: ${tag.Name}`);
      } catch (err) {
        console.error('Failed to remove tag:', err);
        // Revert: add the tag back to our resourceDetails.Tags
        if (this.resourceDetails?.Tags) {
          this.resourceDetails.Tags.push(tag);
        }
        this.announce('Failed to remove tag');
        throw err; // Re-throw so autocompleter can handle rollback
      }
    },

    /**
     * Get current resource tags (for autocompleter selectedResults)
     * @returns {Array}
     */
    getCurrentTags() {
      return this.resourceDetails?.Tags || [];
    },

    /**
     * Handle double-click/double-tap to toggle zoom
     * @param {MouseEvent} event
     */
    handleDoubleClick(event) {
      // Skip for videos
      if (this.isVideo(this.getCurrentItem()?.contentType)) return;

      event.preventDefault();

      if (this.zoomLevel === 1) {
        // Zoom in to 2x
        this.setZoomLevel(2);
        this.announceZoom();
      } else {
        // Zoom out to 1x
        this.setZoomLevel(1);
        this.announceZoom();
      }
    },

    /**
     * Handle mouse down for drag start
     * @param {MouseEvent} event
     */
    handleMouseDown(event) {
      // Skip for videos or if clicking on controls
      if (this.isVideo(this.getCurrentItem()?.contentType)) return;
      if (event.target.closest('button')) return;
      if (event.target.closest('[data-edit-panel]')) return;

      event.preventDefault();
      this.isDragging = true;
      this.dragStartX = event.clientX;
      this.dragStartY = event.clientY;
      this.dragStartPanX = this.panX;
      this.dragStartPanY = this.panY;
      this.lastDragX = event.clientX;
      this.lastDragY = event.clientY;
      this.lastDragTime = performance.now();
      this.dragVelocityX = 0;
      this.dragVelocityY = 0;
    },

    /**
     * Handle mouse move for dragging
     * @param {MouseEvent} event
     */
    handleMouseMove(event) {
      if (!this.isDragging) return;

      const now = performance.now();
      const dt = now - this.lastDragTime;

      if (dt > 0) {
        // Calculate velocity for momentum
        this.dragVelocityX = (event.clientX - this.lastDragX) / dt;
        this.dragVelocityY = (event.clientY - this.lastDragY) / dt;
      }

      this.lastDragX = event.clientX;
      this.lastDragY = event.clientY;
      this.lastDragTime = now;

      if (this.isZoomed()) {
        // Pan when zoomed
        const dx = event.clientX - this.dragStartX;
        const dy = event.clientY - this.dragStartY;
        this.panX = this.dragStartPanX + dx / this.zoomLevel;
        this.panY = this.dragStartPanY + dy / this.zoomLevel;
        this.constrainPan();
      }
    },

    /**
     * Handle mouse up for drag end
     * @param {MouseEvent} event
     */
    handleMouseUp(event) {
      if (!this.isDragging) return;

      const dx = event.clientX - this.dragStartX;
      const dy = event.clientY - this.dragStartY;
      const distance = Math.sqrt(dx * dx + dy * dy);
      const speed = Math.sqrt(this.dragVelocityX ** 2 + this.dragVelocityY ** 2);

      this.isDragging = false;

      if (!this.isZoomed()) {
        // Navigate based on momentum - quick flick triggers navigation
        const threshold = 0.3; // pixels per ms
        const minDistance = 30;

        if (Math.abs(this.dragVelocityX) > Math.abs(this.dragVelocityY)) {
          if (speed > threshold || distance > minDistance) {
            if (dx < 0) {
              this.next();
            } else if (dx > 0) {
              this.prev();
            }
          }
        }
      }

      this.dragStartX = null;
      this.dragStartY = null;
    }
  });
}
