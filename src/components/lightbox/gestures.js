/**
 * Touch, mouse drag, and wheel gesture handling for the lightbox store.
 * All methods use `this` which is bound to the Alpine store.
 */
export const gestureState = {
  // Touch handling
  touchStartX: null,
  touchStartY: null,

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
};

export const gestureMethods = {
  getPinchDistance(touches) {
    const dx = touches[0].clientX - touches[1].clientX;
    const dy = touches[0].clientY - touches[1].clientY;
    return Math.sqrt(dx * dx + dy * dy);
  },

  getPinchCenter(touches) {
    return {
      x: (touches[0].clientX + touches[1].clientX) / 2,
      y: (touches[0].clientY + touches[1].clientY) / 2
    };
  },

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
      this.touchStartX = event.touches[0].clientX;
      this.touchStartY = event.touches[0].clientY;
      if (this.isZoomed()) {
        this.dragStartPanX = this.panX;
        this.dragStartPanY = this.panY;
      }
    }
  },

  handleTouchMove(event) {
    if (this.isVideo(this.getCurrentItem()?.contentType)) return;

    if (event.touches.length === 2) {
      event.preventDefault();

      const center = this.getPinchCenter(event.touches);

      if (this.pinchStartDistance !== null) {
        this.disableAnimations();
        const currentDistance = this.getPinchDistance(event.touches);
        const scale = currentDistance / this.pinchStartDistance;
        this.setZoomLevel(this.pinchStartZoom * scale);

        if (this.pinchOriginX !== null) {
          this.panX = (center.x - this.pinchOriginX) / this.zoomLevel - this.pinchImageX;
          this.panY = (center.y - this.pinchOriginY) / this.zoomLevel - this.pinchImageY;
          this.constrainPan();
        }

        this.pinchCenterX = center.x;
        this.pinchCenterY = center.y;
      }
    } else if (event.touches.length === 1 && this.isZoomed()) {
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

  handleTouchEnd(event) {
    // Handle pinch/two-finger gesture end
    if (this.pinchStartDistance !== null) {
      if (this.zoomLevel < this.minZoom) {
        this.setZoomLevel(this.minZoom);
      }

      if (!this.isZoomed() && this.pinchStartCenterX !== null && this.pinchCenterX !== null) {
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

    if (this.touchStartX === null) return;

    const touchEndX = event.changedTouches[0].clientX;
    const touchEndY = event.changedTouches[0].clientY;
    const diffX = this.touchStartX - touchEndX;
    const diffY = this.touchStartY - touchEndY;

    if (Math.abs(diffX) > Math.abs(diffY) && Math.abs(diffX) > 50) {
      if (this.isZoomed()) {
        // Pan is handled by handleTouchMove, swipe ignored when zoomed
      } else {
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

  handleWheel(event) {
    if (this.isVideo(this.getCurrentItem()?.contentType)) return;

    if (event.ctrlKey) {
      event.preventDefault();
      this.disableAnimations();

      const media = this.getMediaElement();
      const oldZoom = this.zoomLevel;
      const oldPanX = this.panX;
      const oldPanY = this.panY;

      const zoomDelta = -event.deltaY * 0.01;
      this.setZoomLevel(this.zoomLevel + zoomDelta);
      const newZoom = this.zoomLevel;

      if (newZoom !== oldZoom && media) {
        const rect = media.rect;
        const rectCenterX = rect.left + rect.width / 2;
        const rectCenterY = rect.top + rect.height / 2;
        const originX = rectCenterX - oldZoom * oldPanX;
        const originY = rectCenterY - oldZoom * oldPanY;
        const cursorRelX = event.clientX - originX;
        const cursorRelY = event.clientY - originY;
        this.panX = oldPanX + cursorRelX * (1 / newZoom - 1 / oldZoom);
        this.panY = oldPanY + cursorRelY * (1 / newZoom - 1 / oldZoom);
        this.constrainPan();
      }

      if (!this._zoomAnnounceDebounce) {
        this._zoomAnnounceDebounce = true;
        setTimeout(() => {
          this.announceZoom();
          this._zoomAnnounceDebounce = false;
        }, 500);
      }
      return;
    }

    if (!this.isZoomed()) {
      if (Math.abs(event.deltaX) > Math.abs(event.deltaY)) {
        event.preventDefault();

        if (Math.abs(event.deltaX) > 10) {
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
      event.preventDefault();
      this.disableAnimations();
      this.panX -= event.deltaX / this.zoomLevel;
      this.panY -= event.deltaY / this.zoomLevel;
      this.constrainPan();
    }
  },

  handleDoubleClick(event) {
    if (this.isVideo(this.getCurrentItem()?.contentType)) return;

    event.preventDefault();

    if (this.zoomLevel === 1) {
      const media = this.getMediaElement();
      if (!media) return;

      const el = media.element;
      const displayedWidth = el.clientWidth;
      const displayedHeight = el.clientHeight;

      const nativeWidth = el.naturalWidth || displayedWidth;
      const nativeHeight = el.naturalHeight || displayedHeight;
      const nativeZoom = Math.max(nativeWidth / displayedWidth, nativeHeight / displayedHeight);
      const targetZoom = Math.max(this.minZoom + 0.01, Math.min(this.maxZoom, nativeZoom));

      const rect = media.rect;
      const clickRelX = event.clientX - (rect.left + rect.width / 2);
      const clickRelY = event.clientY - (rect.top + rect.height / 2);

      this.panX = -clickRelX;
      this.panY = -clickRelY;

      this.setZoomLevel(targetZoom);
      this.constrainPan();
      this.announceZoom();
    } else {
      this.setZoomLevel(1);
      this.announceZoom();
    }
  },

  handleMouseDown(event) {
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

  handleMouseMove(event) {
    if (!this.isDragging) return;

    const now = performance.now();
    const dt = now - this.lastDragTime;

    if (dt > 0) {
      this.dragVelocityX = (event.clientX - this.lastDragX) / dt;
      this.dragVelocityY = (event.clientY - this.lastDragY) / dt;
    }

    this.lastDragX = event.clientX;
    this.lastDragY = event.clientY;
    this.lastDragTime = now;

    if (this.isZoomed()) {
      this.disableAnimations();
      const dx = event.clientX - this.dragStartX;
      const dy = event.clientY - this.dragStartY;
      this.panX = this.dragStartPanX + dx / this.zoomLevel;
      this.panY = this.dragStartPanY + dy / this.zoomLevel;
      this.constrainPan();
    }
  },

  handleMouseUp(event) {
    if (!this.isDragging) return;

    const dx = event.clientX - this.dragStartX;
    const dy = event.clientY - this.dragStartY;
    const distance = Math.sqrt(dx * dx + dy * dy);
    const speed = Math.sqrt(this.dragVelocityX ** 2 + this.dragVelocityY ** 2);

    this.isDragging = false;

    if (!this.isZoomed()) {
      const threshold = 0.3;
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

    const dialog = document.querySelector('[role="dialog"][aria-modal="true"]');
    if (dialog) dialog.focus();
  },
};
