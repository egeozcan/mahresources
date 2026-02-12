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

    // Pinch tracking
    pinchStartDistance: null,
    pinchStartZoom: null,
    pinchStartCenterX: null,
    pinchStartCenterY: null,
    pinchCenterX: null,
    pinchCenterY: null,

    // Pinch zoom-toward-center tracking
    pinchOriginX: null,
    pinchOriginY: null,
    pinchImageX: null,
    pinchImageY: null,

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

    // Track when to disable CSS transitions for smooth real-time interaction
    animationsDisabled: false,
    animationTimeout: null,

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

      // Add non-passive wheel listener to allow preventDefault for browser back/forward
      document.addEventListener('wheel', (event) => {
        if (!this.isOpen) return;
        // Let edit panel handle its own scrolling
        if (event.target.closest('[data-edit-panel]')) return;
        this.handleWheel(event);
      }, { passive: false });
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
        if (this.zoomLevel === 1) {
          this.panX = 0;
          this.panY = 0;
        }
      }
    },

    /**
     * Reset zoom and pan to default
     */
    resetZoom() {
      this.zoomLevel = 1;
      this.panX = 0;
      this.panY = 0;
      this.hideZoomPresets();
    },

    /**
     * Hide the zoom preset popover if open
     */
    hideZoomPresets() {
      const p = document.getElementById('zoom-preset-popover');
      if (p?.matches(':popover-open')) p.hidePopover();
    },

    /**
     * Build and show the zoom preset popover anchored to a button element.
     * @param {HTMLElement} btn - The button that triggered the popover
     */
    showZoomPresets(btn) {
      const p = document.getElementById('zoom-preset-popover');
      if (!p) return;

      if (p.matches(':popover-open')) {
        p.hidePopover();
        return;
      }

      const presets = this.zoomPresets();
      if (!presets.length) return;

      const self = this;
      p.innerHTML = '';
      Object.assign(p.style, {
        background: 'rgba(0,0,0,0.8)',
        backdropFilter: 'blur(8px)',
        border: '1px solid rgba(255,255,255,0.1)',
        borderRadius: '0.375rem',
        padding: '0.25rem 0',
        margin: '0',
        minWidth: '7rem',
        textAlign: 'center',
        color: 'white',
        fontSize: '0.875rem',
      });

      for (const preset of presets) {
        const item = document.createElement('button');
        item.textContent = preset.label;
        item.style.cssText = 'display:block;width:100%;padding:0.375rem 0.75rem;transition:background 150ms;font-variant-numeric:tabular-nums;background:none;border:none;color:inherit;cursor:pointer;font-size:inherit;';
        item.addEventListener('mouseenter', () => item.style.background = 'rgba(255,255,255,0.2)');
        item.addEventListener('mouseleave', () => item.style.background = 'none');
        item.addEventListener('click', (e) => {
          e.stopPropagation();
          self.setNativeZoom(preset.nativePct);
        });
        p.appendChild(item);
      }

      // Position above the button, centered
      const rect = btn.getBoundingClientRect();
      p.showPopover();
      const popRect = p.getBoundingClientRect();
      p.style.position = 'fixed';
      p.style.left = (rect.left + rect.width / 2 - popRect.width / 2) + 'px';
      p.style.top = (rect.top - popRect.height - 4) + 'px';
      p.style.bottom = 'auto';
      p.style.right = 'auto';
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
      // Lock body scroll using position:fixed with negative top offset.
      // This preserves visual position unlike overflow:hidden on <html>,
      // which Chrome resets scrollY to 0.
      this._savedScrollY = window.scrollY;
      document.body.style.position = 'fixed';
      document.body.style.top = `-${this._savedScrollY}px`;
      document.body.style.left = '0';
      document.body.style.right = '0';
      document.body.style.overflow = 'hidden';

      this.currentIndex = index;
      this.isOpen = true;
      this.loading = true;

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
      this.resetZoom();

      // Unlock body scroll and restore position
      const savedY = this._savedScrollY;
      document.body.style.position = '';
      document.body.style.top = '';
      document.body.style.left = '';
      document.body.style.right = '';
      document.body.style.overflow = '';
      window.scrollTo(0, savedY);

      // Clear resource details to prevent stale data when reopening
      this.resourceDetails = null;

      // Cancel any pending requests
      if (this.requestAborter) {
        this.requestAborter();
        this.requestAborter = null;
      }

      // Restore focus to trigger element without scrolling the page
      if (this.triggerElement) {
        this.triggerElement.focus({ preventScroll: true });
        this.triggerElement = null;
      }

      this.announce('Media viewer closed');

      // Restore scroll again after Alpine's x-trap deactivation settles,
      // in case focus-trap's returnFocus causes a scroll jump.
      requestAnimationFrame(() => {
        window.scrollTo(0, savedY);
      });
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
     * Get zoom level relative to native image resolution as a percentage string.
     * Returns e.g. "50%" when the image is displayed at half its native pixels.
     * Returns null for videos or when the image element is unavailable.
     * @returns {string|null}
     */
    nativeZoomPercent() {
      // Read reactive properties so Alpine re-evaluates when they change
      const loading = this.loading;
      const zoom = this.zoomLevel;

      const item = this.getCurrentItem();
      if (!item || this.isVideo(item.contentType) || loading) return null;

      const media = this.getMediaElement();
      if (!media) return null;

      const el = media.element;
      const naturalW = el.naturalWidth;
      const displayedW = el.clientWidth;
      if (!naturalW || !displayedW) return null;

      const pct = (zoom * displayedW / naturalW) * 100;
      return Math.round(pct) + '%';
    },

    /**
     * Get available zoom preset percentages (relative to native resolution).
     * Returns presets that fall within the allowed zoom range, plus "Fit".
     * @returns {Array<{label: string, nativePct: number|null}>}
     */
    zoomPresets() {
      const media = this.getMediaElement();
      if (!media) return [];

      const el = media.element;
      const naturalW = el.naturalWidth;
      const displayedW = el.clientWidth;
      const displayedH = el.clientHeight;
      if (!naturalW || !displayedW) return [];

      const fitNativePct = Math.round((displayedW / naturalW) * 100);
      const candidates = [25, 50, 100, 200, 300, 500];

      const presets = [{label: 'Fit (' + fitNativePct + '%)', nativePct: null}];

      // Add "Stretch" option when the image at fit size is smaller than the available space
      const availW = window.innerWidth * 0.9;
      const availH = window.innerHeight * 0.9;
      if (displayedW && displayedH) {
        const stretchZoom = Math.min(availW / displayedW, availH / displayedH);
        if (stretchZoom > 1.01 && stretchZoom <= this.maxZoom) {
          const stretchNativePct = Math.round(stretchZoom * displayedW / naturalW * 100);
          presets.push({label: 'Stretch (' + stretchNativePct + '%)', nativePct: 'stretch'});
        }
      }

      for (const pct of candidates) {
        // Convert native pct to zoom level and check if it's within bounds
        const zoomForPct = (pct / 100) * (naturalW / displayedW);
        if (zoomForPct >= this.minZoom && zoomForPct <= this.maxZoom && pct !== fitNativePct) {
          presets.push({label: pct + '%', nativePct: pct});
        }
      }

      return presets;
    },

    /**
     * Set zoom level to show the image at a specific native resolution percentage,
     * or 'stretch' to fill the container.
     * @param {number|string|null} nativePct - Percentage (100 = 1:1), null for fit, 'stretch' to fill container.
     */
    setNativeZoom(nativePct) {
      this.hideZoomPresets();

      if (nativePct === null) {
        this.setZoomLevel(1);
        this.announceZoom();
        return;
      }

      const media = this.getMediaElement();
      if (!media) return;

      const el = media.element;
      const displayedW = el.clientWidth;
      const displayedH = el.clientHeight;
      if (!displayedW || !displayedH) return;

      let targetZoom;
      if (nativePct === 'stretch') {
        const availW = window.innerWidth * 0.9;
        const availH = window.innerHeight * 0.9;
        targetZoom = Math.min(availW / displayedW, availH / displayedH);
      } else {
        const naturalW = el.naturalWidth;
        if (!naturalW) return;
        targetZoom = (nativePct / 100) * (naturalW / displayedW);
      }

      this.panX = 0;
      this.panY = 0;
      this.setZoomLevel(targetZoom);
      this.constrainPan();
      this.announceZoom();
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

        // Compute transform origin and image point under pinch center
        // so zoom tracks toward the pinch point
        const media = this.getMediaElement();
        if (media) {
          const rect = media.rect;
          const rectCenterX = rect.left + rect.width / 2;
          const rectCenterY = rect.top + rect.height / 2;
          this.pinchOriginX = rectCenterX - this.zoomLevel * this.panX;
          this.pinchOriginY = rectCenterY - this.zoomLevel * this.panY;
          this.pinchImageX = (center.x - this.pinchOriginX) / this.zoomLevel - this.panX;
          this.pinchImageY = (center.y - this.pinchOriginY) / this.zoomLevel - this.panY;
        } else {
          this.pinchOriginX = null;
          this.pinchOriginY = null;
          this.pinchImageX = null;
          this.pinchImageY = null;
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
          this.disableAnimations();
          const currentDistance = this.getPinchDistance(event.touches);
          const scale = currentDistance / this.pinchStartDistance;
          this.setZoomLevel(this.pinchStartZoom * scale);

          // Adjust pan to keep the initial image point under the current pinch center
          if (this.pinchOriginX !== null) {
            this.panX = (center.x - this.pinchOriginX) / this.zoomLevel - this.pinchImageX;
            this.panY = (center.y - this.pinchOriginY) / this.zoomLevel - this.pinchImageY;
            this.constrainPan();
          }

          // Track current center for two-finger swipe navigation
          this.pinchCenterX = center.x;
          this.pinchCenterY = center.y;
        }
      } else if (event.touches.length === 1 && this.isZoomed()) {
        // Single finger pan when zoomed
        if (this.touchStartX !== null) {
          event.preventDefault();
          this.disableAnimations();
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
        this.pinchOriginX = null;
        this.pinchOriginY = null;
        this.pinchImageX = null;
        this.pinchImageY = null;
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
      // Skip for videos
      if (this.isVideo(this.getCurrentItem()?.contentType)) return;

      // Trackpad pinch zoom (ctrlKey is set by browser for pinch gestures)
      if (event.ctrlKey) {
        event.preventDefault();
        this.disableAnimations();

        // Capture state before zoom change
        const media = this.getMediaElement();
        const oldZoom = this.zoomLevel;
        const oldPanX = this.panX;
        const oldPanY = this.panY;

        // Negate deltaY so pinch-out zooms in and pinch-in zooms out
        const zoomDelta = -event.deltaY * 0.01;
        this.setZoomLevel(this.zoomLevel + zoomDelta);
        const newZoom = this.zoomLevel;

        // Adjust pan so the point under the cursor stays fixed
        if (newZoom !== oldZoom && media) {
          const rect = media.rect;
          const rectCenterX = rect.left + rect.width / 2;
          const rectCenterY = rect.top + rect.height / 2;
          // Transform origin (element center before transforms)
          const originX = rectCenterX - oldZoom * oldPanX;
          const originY = rectCenterY - oldZoom * oldPanY;
          // Cursor position relative to transform origin
          const cursorRelX = event.clientX - originX;
          const cursorRelY = event.clientY - originY;
          this.panX = oldPanX + cursorRelX * (1 / newZoom - 1 / oldZoom);
          this.panY = oldPanY + cursorRelY * (1 / newZoom - 1 / oldZoom);
          this.constrainPan();
        }

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
        // Prevent browser back/forward navigation on any horizontal swipe
        if (Math.abs(event.deltaX) > Math.abs(event.deltaY)) {
          event.preventDefault();

          // Only trigger navigation after threshold is met
          if (Math.abs(event.deltaX) > 10) {
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
        }
      } else {
        // Pan when zoomed (using trackpad scroll)
        event.preventDefault();
        this.disableAnimations();
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

    /**
     * Temporarily disable CSS transitions for smooth real-time interaction
     * Auto-restores after a brief delay when interaction stops
     */
    disableAnimations() {
      this.animationsDisabled = true;

      // Clear any existing timeout
      if (this.animationTimeout) {
        clearTimeout(this.animationTimeout);
      }

      // Re-enable animations after interaction stops
      this.animationTimeout = setTimeout(() => {
        this.animationsDisabled = false;
        this.animationTimeout = null;
      }, 100);
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

      // Clear stale details so reopening on a different resource doesn't flash old data
      this.resourceDetails = null;

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

          // Update lightbox items from the refreshed DOM without losing
          // items loaded from other pages via pagination.
          this.updateItemsFromDOM();
        }
      } catch (err) {
        console.error('Failed to refresh page content:', err);
      }
    },

    /**
     * Update existing lightbox items from the current DOM without
     * rebuilding the entire items array. This preserves items loaded
     * from other pages and keeps currentIndex valid.
     */
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

      // Update properties of existing items that appear in the refreshed DOM
      for (let i = 0; i < this.items.length; i++) {
        const updated = domItems.get(this.items[i].id);
        if (updated) {
          this.items[i] = { ...this.items[i], ...updated };
        }
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

      // Remember which input was focused so we can restore it after loading
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

      // Clear current details to show loading state and ensure autocompleter
      // recreates with fresh data when fetch completes
      this.resourceDetails = null;
      // Force fresh fetch by clearing cache for this resource
      const resourceId = this.getCurrentItem()?.id;
      if (resourceId) {
        this.detailsCache.delete(resourceId);
      }
      await this.fetchResourceDetails();

      // Restore focus to the same input after new details render
      if (focusSelector) {
        requestAnimationFrame(() => {
          const el = document.querySelector(`[data-edit-panel] ${focusSelector}`);
          if (el) el.focus();
        });
      }
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
        const media = this.getMediaElement();
        if (!media) return;

        const el = media.element;
        const displayedWidth = el.clientWidth;
        const displayedHeight = el.clientHeight;

        // Calculate native resolution zoom level, clamped to maxZoom
        const nativeWidth = el.naturalWidth || displayedWidth;
        const nativeHeight = el.naturalHeight || displayedHeight;
        const nativeZoom = Math.max(nativeWidth / displayedWidth, nativeHeight / displayedHeight);
        const targetZoom = Math.max(this.minZoom + 0.01, Math.min(this.maxZoom, nativeZoom));

        // Calculate click position relative to image center (in displayed pixels)
        const rect = media.rect;
        const clickRelX = event.clientX - (rect.left + rect.width / 2);
        const clickRelY = event.clientY - (rect.top + rect.height / 2);

        // Pan so the clicked point moves to viewport center.
        // CSS transform: scale(Z) translate(tx,ty) applies translate first, then scale,
        // so screen offset = Z * (displayedOffset + pan). For screen offset = 0: pan = -displayedOffset.
        this.panX = -clickRelX;
        this.panY = -clickRelY;

        this.setZoomLevel(targetZoom);
        this.constrainPan();
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
        this.disableAnimations();
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

      // Refocus the dialog to ensure keyboard shortcuts work after clicking
      const dialog = document.querySelector('[role="dialog"][aria-modal="true"]');
      if (dialog) dialog.focus();
    }
  });
}
