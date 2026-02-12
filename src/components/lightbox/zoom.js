/**
 * Zoom and pan state/methods for the lightbox store.
 * All methods use `this` which is bound to the Alpine store.
 */
export const zoomState = {
  // Fullscreen state
  isFullscreen: false,

  // Zoom state
  zoomLevel: 1,
  minZoom: 1,
  maxZoom: 5,
  panX: 0,
  panY: 0,

  // Image dimensions for pan bounds
  imageRect: null,
  containerRect: null,

  // Track when to disable CSS transitions for smooth real-time interaction
  animationsDisabled: false,
  animationTimeout: null,
};

export const zoomMethods = {
  fullscreenSupported() {
    return !!(document.fullscreenEnabled || document.webkitFullscreenEnabled);
  },

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

  isZoomed() {
    return this.zoomLevel > 1;
  },

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

  resetZoom() {
    this.zoomLevel = 1;
    this.panX = 0;
    this.panY = 0;
    this.hideZoomPresets();
  },

  hideZoomPresets() {
    const p = document.getElementById('zoom-preset-popover');
    if (p?.matches(':popover-open')) p.hidePopover();
  },

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

  announceZoom() {
    if (this.zoomLevel === 1) {
      this.announce('Zoom reset to 100%');
    } else {
      this.announce(`Zoomed to ${Math.round(this.zoomLevel * 100)}%`);
    }
  },

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
      const zoomForPct = (pct / 100) * (naturalW / displayedW);
      if (zoomForPct >= this.minZoom && zoomForPct <= this.maxZoom && pct !== fitNativePct) {
        presets.push({label: pct + '%', nativePct: pct});
      }
    }

    return presets;
  },

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

  getMediaElement() {
    const el = document.querySelector('[role="dialog"] img, [role="dialog"] object');
    if (!el) return null;
    return { element: el, rect: el.getBoundingClientRect() };
  },

  getContainerRect() {
    const container = document.querySelector('[role="dialog"] .relative.max-h-\\[90vh\\]');
    return container?.getBoundingClientRect() || null;
  },

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
    const displayedWidth = el.clientWidth;
    const displayedHeight = el.clientHeight;
    const zoomedWidth = displayedWidth * this.zoomLevel;
    const zoomedHeight = displayedHeight * this.zoomLevel;
    const maxPanX = Math.max(0, (zoomedWidth - containerRect.width) / 2 / this.zoomLevel);
    const maxPanY = Math.max(0, (zoomedHeight - containerRect.height) / 2 / this.zoomLevel);

    this.panX = Math.max(-maxPanX, Math.min(maxPanX, this.panX));
    this.panY = Math.max(-maxPanY, Math.min(maxPanY, this.panY));
  },

  disableAnimations() {
    this.animationsDisabled = true;

    if (this.animationTimeout) {
      clearTimeout(this.animationTimeout);
    }

    this.animationTimeout = setTimeout(() => {
      this.animationsDisabled = false;
      this.animationTimeout = null;
    }, 100);
  },
};
