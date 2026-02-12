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
};

export const navigationMethods = {
  initFromDOM() {
    const container = document.querySelector('.list-container, .gallery');
    if (!container) return;

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
        width: parseInt(link.dataset.resourceWidth, 10) || 0,
        height: parseInt(link.dataset.resourceHeight, 10) || 0,
        domIndex: index
      };
    }).filter(item =>
      item.contentType?.startsWith('image/') ||
      item.contentType?.startsWith('video/')
    );

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

    const pageSizeAttr = container.dataset.pageSize;
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

    const index = this.items.findIndex(item => item.id === resourceId);
    if (index !== -1) {
      this.open(index);
    }
  },

  open(index) {
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

    this.scheduleMediaCheck();
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

    this.isOpen = false;
    this.loading = false;
    this.resetZoom();

    const savedY = this._savedScrollY;
    document.body.style.position = '';
    document.body.style.top = '';
    document.body.style.left = '';
    document.body.style.right = '';
    document.body.style.overflow = '';
    window.scrollTo(0, savedY);

    this.resourceDetails = null;

    if (this.requestAborter) {
      this.requestAborter();
      this.requestAborter = null;
    }

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

  async prev() {
    if (this.pageLoading) return;

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

  async loadNextPage() {
    const nextPage = this.currentPage + 1;

    if (this.loadedPages.has(nextPage)) {
      this.currentPage = nextPage;
      return;
    }

    this.pageLoading = true;
    this.announce('Loading more items...');

    try {
      const { items: newItems, hasNextPage } = await this.fetchPage(nextPage);

      if (newItems.length === 0) {
        this.hasNextPage = false;
        this.announce('No more items');
        return;
      }

      this.items = [...this.items, ...newItems];
      this.loadedPages.add(nextPage);
      this.currentPage = nextPage;
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
          height: r.Height || 0
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

  pauseCurrentVideo() {
    const video = document.querySelector('[x-show="$store.lightbox.isOpen"] video');
    if (video && !video.paused) {
      video.pause();
    }
  },
};
