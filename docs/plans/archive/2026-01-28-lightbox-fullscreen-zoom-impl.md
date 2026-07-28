# Lightbox Fullscreen & Zoom Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add fullscreen mode, pinch zoom, and momentum-based gesture navigation to the Alpine.js lightbox.

**Architecture:** Extend the existing lightbox Alpine store with new state for zoom/pan/fullscreen. Use CSS transforms for GPU-accelerated zoom/pan. Detect gestures via touch events (pinch, swipe) and wheel events (trackpad pinch via ctrlKey). Navigation vs pan behavior switches based on zoom level.

**Tech Stack:** Alpine.js store, CSS transforms, Fullscreen API, Touch Events API, Wheel events

---

## Task 1: Add Fullscreen State and Methods

**Files:**
- Modify: `src/components/lightbox.js:8-50` (add state properties)
- Modify: `src/components/lightbox.js:180-215` (modify close method, add fullscreen methods)

**Step 1: Add fullscreen state properties**

In `src/components/lightbox.js`, add these properties to the Alpine store after line 50 (after `needsRefreshOnClose`):

```javascript
    // Fullscreen state
    isFullscreen: false,
```

**Step 2: Add fullscreen detection method**

Add this method after the `init()` method:

```javascript
    /**
     * Check if Fullscreen API is supported
     * @returns {boolean}
     */
    fullscreenSupported() {
      return !!(document.fullscreenEnabled || document.webkitFullscreenEnabled);
    },
```

**Step 3: Add toggleFullscreen method**

Add this method:

```javascript
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
```

**Step 4: Add fullscreen event listener in init()**

Modify the `init()` method to add the fullscreen change listener. After the live region setup, add:

```javascript
      // Listen for fullscreen changes
      const handleFullscreenChange = () => {
        this.isFullscreen = !!(document.fullscreenElement || document.webkitFullscreenElement);
        this.announce(this.isFullscreen ? 'Entered fullscreen' : 'Exited fullscreen');
      };
      document.addEventListener('fullscreenchange', handleFullscreenChange);
      document.addEventListener('webkitfullscreenchange', handleFullscreenChange);
```

**Step 5: Update close() to exit fullscreen**

In the `close()` method, add at the beginning (after pausing video):

```javascript
      // Exit fullscreen if active
      if (this.isFullscreen) {
        if (document.exitFullscreen) {
          document.exitFullscreen().catch(() => {});
        } else if (document.webkitExitFullscreen) {
          document.webkitExitFullscreen();
        }
        this.isFullscreen = false;
      }
```

**Step 6: Update handleEscape() for layered escape**

Modify `handleEscape()` to handle fullscreen:

```javascript
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
```

**Step 7: Build and verify no errors**

Run: `cd /Users/egecan/Code/mahresources/.worktrees/lightbox-fullscreen-zoom && npm run build-js`
Expected: Build completes without errors

**Step 8: Commit**

```bash
git add src/components/lightbox.js
git commit -m "feat(lightbox): add fullscreen state and toggle methods"
```

---

## Task 2: Add Fullscreen Button and Keyboard Shortcuts to Template

**Files:**
- Modify: `templates/partials/lightbox.tpl:12-19` (add keyboard handlers)
- Modify: `templates/partials/lightbox.tpl:376-410` (add fullscreen button to bottom bar)

**Step 1: Add Enter and e/F2 keyboard handlers**

In `templates/partials/lightbox.tpl`, after line 16 (the page-down handler), add:

```html
    @keydown.enter.window="$store.lightbox.isOpen && canNavigate() && $store.lightbox.toggleFullscreen()"
    @keydown.e.window="$store.lightbox.isOpen && canNavigate() && ($store.lightbox.editPanelOpen ? $store.lightbox.closeEditPanel() : $store.lightbox.openEditPanel())"
    @keydown.f2.window.prevent="$store.lightbox.isOpen && ($store.lightbox.editPanelOpen ? $store.lightbox.closeEditPanel() : $store.lightbox.openEditPanel())"
```

**Step 2: Add fullscreen button to bottom bar**

In the bottom bar section (around line 380), after the counter div and before the name div, add:

```html
        <!-- Fullscreen button -->
        <button
            x-show="$store.lightbox.fullscreenSupported()"
            @click.stop="$store.lightbox.toggleFullscreen()"
            class="bg-black/50 px-3 py-1.5 rounded hover:bg-white/20 transition-colors focus:outline-none focus:ring-2 focus:ring-white/50 flex items-center gap-1.5"
            :title="$store.lightbox.isFullscreen ? 'Exit fullscreen' : 'Enter fullscreen'"
        >
            <!-- Expand icon (not fullscreen) -->
            <svg x-show="!$store.lightbox.isFullscreen" class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 8V4m0 0h4M4 4l5 5m11-1V4m0 0h-4m4 0l-5 5M4 16v4m0 0h4m-4 0l5-5m11 5l-5-5m5 5v-4m0 4h-4"></path>
            </svg>
            <!-- Compress icon (fullscreen) -->
            <svg x-show="$store.lightbox.isFullscreen" x-cloak class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 9L4 4m0 0v4m0-4h4m6 0l5-5m0 0v4m0-4h-4M9 15l-5 5m0 0v-4m0 4h4m6 0l5 5m0 0v-4m0 4h-4"></path>
            </svg>
        </button>
```

**Step 3: Build and verify**

Run: `npm run build`
Expected: Build completes, no template errors

**Step 4: Commit**

```bash
git add templates/partials/lightbox.tpl
git commit -m "feat(lightbox): add fullscreen button and keyboard shortcuts (Enter, e, F2)"
```

---

## Task 3: Add Zoom State and Basic Zoom Methods

**Files:**
- Modify: `src/components/lightbox.js` (add zoom state and methods)

**Step 1: Add zoom state properties**

After the `isFullscreen` property, add:

```javascript
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
    pinchCenterX: null,
    pinchCenterY: null,

    // Image dimensions for pan bounds
    imageRect: null,
    containerRect: null,
```

**Step 2: Add isZoomed helper**

Add this computed-like method:

```javascript
    /**
     * Check if currently zoomed in
     * @returns {boolean}
     */
    isZoomed() {
      return this.zoomLevel > 1;
    },
```

**Step 3: Add zoom level setter with indicator**

```javascript
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
```

**Step 4: Add resetZoom method**

```javascript
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
```

**Step 5: Update next() and prev() to reset zoom**

In both `next()` and `prev()` methods, add `this.resetZoom();` after `this.pauseCurrentVideo();`

**Step 6: Add announceZoom for accessibility**

```javascript
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
```

**Step 7: Build and verify**

Run: `npm run build-js`
Expected: No errors

**Step 8: Commit**

```bash
git add src/components/lightbox.js
git commit -m "feat(lightbox): add zoom state, setZoomLevel, and resetZoom methods"
```

---

## Task 4: Add Zoom Indicator to Template

**Files:**
- Modify: `templates/partials/lightbox.tpl` (add zoom indicator UI)

**Step 1: Add zoom indicator element**

After the page loading indicator div (around line 374), add:

```html
    <!-- Zoom indicator -->
    <div
        x-show="$store.lightbox.zoomIndicatorVisible && $store.lightbox.zoomLevel > 1"
        x-transition:enter="transition ease-out duration-150"
        x-transition:enter-start="opacity-0"
        x-transition:enter-end="opacity-100"
        x-transition:leave="transition ease-in duration-150"
        x-transition:leave-start="opacity-100"
        x-transition:leave-end="opacity-0"
        class="absolute bottom-20 left-4 px-3 py-1.5 bg-black/50 rounded text-white text-sm z-20"
        x-text="$store.lightbox.zoomLevel.toFixed(1) + 'x'"
    ></div>
```

**Step 2: Build and verify**

Run: `npm run build`
Expected: No errors

**Step 3: Commit**

```bash
git add templates/partials/lightbox.tpl
git commit -m "feat(lightbox): add zoom level indicator UI"
```

---

## Task 5: Add CSS Transform for Zoom/Pan

**Files:**
- Modify: `templates/partials/lightbox.tpl` (add transform styles to media elements)

**Step 1: Add transform style to image element**

Find the `<img>` element inside `<template x-if="$store.lightbox.isImage(...)">` and add the `:style` binding:

```html
                <img
                    :src="$store.lightbox.getCurrentItem()?.viewUrl"
                    :alt="$store.lightbox.getCurrentItem()?.name || 'Image'"
                    class="max-h-[90vh] object-contain transition-all duration-300"
                    :class="$store.lightbox.editPanelOpen ? 'md:max-w-[calc(100vw-450px)]' : 'max-w-[90vw]'"
                    :style="{ transform: `scale(${$store.lightbox.zoomLevel}) translate(${$store.lightbox.panX}px, ${$store.lightbox.panY}px)`, transformOrigin: 'center center' }"
                    x-init="$nextTick(() => $store.lightbox.checkIfMediaLoaded($el))"
                    @load="$store.lightbox.onMediaLoaded()"
                    @error="$store.lightbox.onMediaLoaded()"
                >
```

**Step 2: Add transform style to SVG object element**

Find the `<object>` element for SVG and add the same `:style` binding:

```html
                <object
                    :data="$store.lightbox.getCurrentItem()?.viewUrl"
                    type="image/svg+xml"
                    :aria-label="$store.lightbox.getCurrentItem()?.name || 'SVG Image'"
                    class="max-h-[90vh] max-w-[90vw] min-h-[50vh] min-w-[50vw] transition-all duration-300"
                    :class="$store.lightbox.editPanelOpen ? 'md:max-w-[calc(100vw-450px)]' : ''"
                    :style="{ transform: `scale(${$store.lightbox.zoomLevel}) translate(${$store.lightbox.panX}px, ${$store.lightbox.panY}px)`, transformOrigin: 'center center' }"
                    x-init="$nextTick(() => $store.lightbox.checkIfMediaLoaded($el))"
                    @load="$store.lightbox.onMediaLoaded()"
                    @error="$store.lightbox.onMediaLoaded()"
                >
```

**Step 3: Video does NOT get transform (per design - native controls handle zoom)**

No changes to video element.

**Step 4: Build and verify**

Run: `npm run build`
Expected: No errors

**Step 5: Commit**

```bash
git add templates/partials/lightbox.tpl
git commit -m "feat(lightbox): add CSS transform bindings for zoom/pan on images"
```

---

## Task 6: Add Double-Click/Double-Tap Zoom Toggle

**Files:**
- Modify: `src/components/lightbox.js` (add double-click handler)
- Modify: `templates/partials/lightbox.tpl` (bind double-click event)

**Step 1: Add handleDoubleClick method**

```javascript
    /**
     * Handle double-click/double-tap to toggle zoom
     * @param {MouseEvent|TouchEvent} event
     */
    handleDoubleClick(event) {
      // Skip for videos
      if (this.isVideo(this.getCurrentItem()?.contentType)) return;

      event.preventDefault();

      if (this.zoomLevel === 1) {
        // Zoom in to 2x centered on click point
        this.setZoomLevel(2);
        this.announceZoom();
      } else {
        // Zoom out to 1x
        this.setZoomLevel(1);
        this.announceZoom();
      }
    },
```

**Step 2: Add dblclick binding to media container**

In the template, find the media content div (the one with `class="relative max-h-[90vh] max-w-[90vw]..."`) and add:

```html
            @dblclick="$store.lightbox.handleDoubleClick($event)"
```

**Step 3: Build and verify**

Run: `npm run build`
Expected: No errors

**Step 4: Commit**

```bash
git add src/components/lightbox.js templates/partials/lightbox.tpl
git commit -m "feat(lightbox): add double-click/tap to toggle 1x/2x zoom"
```

---

## Task 7: Add Pinch Zoom Gesture Handling

**Files:**
- Modify: `src/components/lightbox.js` (add pinch handlers)
- Modify: `templates/partials/lightbox.tpl` (bind touch events for pinch)

**Step 1: Add pinch distance calculator**

```javascript
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
```

**Step 2: Modify handleTouchStart for pinch detection**

Replace the existing `handleTouchStart` method:

```javascript
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
        this.pinchCenterX = center.x;
        this.pinchCenterY = center.y;
      } else if (event.touches.length === 1) {
        // Single touch - swipe or drag
        this.touchStartX = event.touches[0].clientX;
        this.touchStartY = event.touches[0].clientY;
      }
    },
```

**Step 3: Add handleTouchMove for pinch zoom**

```javascript
    /**
     * Handle touch move for pinch zoom
     * @param {TouchEvent} event
     */
    handleTouchMove(event) {
      // Skip for videos
      if (this.isVideo(this.getCurrentItem()?.contentType)) return;

      if (event.touches.length === 2 && this.pinchStartDistance !== null) {
        // Pinch zoom
        event.preventDefault();
        const currentDistance = this.getPinchDistance(event.touches);
        const scale = currentDistance / this.pinchStartDistance;
        this.setZoomLevel(this.pinchStartZoom * scale);
      }
    },
```

**Step 4: Update handleTouchEnd for pinch end and snap-back**

Replace the existing `handleTouchEnd` method:

```javascript
    /**
     * Handle touch end for swipe and pinch gestures
     * @param {TouchEvent} event
     */
    handleTouchEnd(event) {
      // Handle pinch end
      if (this.pinchStartDistance !== null) {
        // Snap back if below minimum
        if (this.zoomLevel < this.minZoom) {
          this.setZoomLevel(this.minZoom);
        }
        this.pinchStartDistance = null;
        this.pinchStartZoom = null;
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
          // Pan when zoomed (handled by handleTouchMove in future task)
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
```

**Step 5: Add touchmove binding to template**

Find the main container div (`x-show="$store.lightbox.isOpen"`) and add:

```html
    @touchmove="$store.lightbox.handleTouchMove($event)"
```

**Step 6: Build and verify**

Run: `npm run build`
Expected: No errors

**Step 7: Commit**

```bash
git add src/components/lightbox.js templates/partials/lightbox.tpl
git commit -m "feat(lightbox): add pinch-to-zoom gesture handling"
```

---

## Task 8: Add Trackpad Pinch Zoom (Wheel + ctrlKey)

**Files:**
- Modify: `src/components/lightbox.js` (update handleWheel method)

**Step 1: Update handleWheel to detect trackpad pinch**

Replace the existing `handleWheel` method:

```javascript
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

        // deltaY is positive when pinching out (zoom in), negative for pinch in
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
```

**Step 2: Add constrainPan method stub (will be implemented in Task 10)**

```javascript
    /**
     * Constrain pan to image bounds
     */
    constrainPan() {
      // Will be implemented with proper bounds checking
      // For now, allow free pan
    },
```

**Step 3: Build and verify**

Run: `npm run build-js`
Expected: No errors

**Step 4: Commit**

```bash
git add src/components/lightbox.js
git commit -m "feat(lightbox): add trackpad pinch zoom via wheel+ctrlKey"
```

---

## Task 9: Add Mouse Drag for Navigation and Pan

**Files:**
- Modify: `src/components/lightbox.js` (add mouse drag handlers)
- Modify: `templates/partials/lightbox.tpl` (bind mouse events)

**Step 1: Add mouse drag state**

Add after the pinch tracking state:

```javascript
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
```

**Step 2: Add handleMouseDown method**

```javascript
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
```

**Step 3: Add handleMouseMove method**

```javascript
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
```

**Step 4: Add handleMouseUp method**

```javascript
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
    },
```

**Step 5: Add mouse event bindings to template**

Find the main content area div (the one with `class="relative flex-1 flex items-center justify-center..."`) and add:

```html
        @mousedown="$store.lightbox.handleMouseDown($event)"
        @mousemove="$store.lightbox.handleMouseMove($event)"
        @mouseup="$store.lightbox.handleMouseUp($event)"
        @mouseleave="$store.lightbox.handleMouseUp($event)"
```

**Step 6: Add cursor style for dragging**

On the same div, add:

```html
        :class="[$store.lightbox.editPanelOpen ? 'md:mr-[400px]' : '', $store.lightbox.isDragging ? 'cursor-grabbing' : 'cursor-grab']"
```

**Step 7: Build and verify**

Run: `npm run build`
Expected: No errors

**Step 8: Commit**

```bash
git add src/components/lightbox.js templates/partials/lightbox.tpl
git commit -m "feat(lightbox): add mouse drag for navigation and pan with momentum"
```

---

## Task 10: Implement Pan Boundary Constraints

**Files:**
- Modify: `src/components/lightbox.js` (implement constrainPan properly)

**Step 1: Add method to get current media element dimensions**

```javascript
    /**
     * Get current media element and its dimensions
     * @returns {{element: HTMLElement, rect: DOMRect}|null}
     */
    getMediaElement() {
      const el = document.querySelector('[role="dialog"] img, [role="dialog"] object');
      if (!el) return null;
      return { element: el, rect: el.getBoundingClientRect() };
    },

    /**
     * Get container dimensions for pan bounds
     * @returns {DOMRect|null}
     */
    getContainerRect() {
      const container = document.querySelector('[role="dialog"] .relative.max-h-\\[90vh\\]');
      return container?.getBoundingClientRect() || null;
    },
```

**Step 2: Implement constrainPan properly**

Replace the stub:

```javascript
    /**
     * Constrain pan to image bounds so edges don't show empty space
     */
    constrainPan() {
      const media = this.getMediaElement();
      if (!media) return;

      // Get the base (unzoomed) dimensions
      const baseWidth = media.rect.width / this.zoomLevel;
      const baseHeight = media.rect.height / this.zoomLevel;

      // Calculate how much the image extends beyond its original size
      const scaledWidth = baseWidth * this.zoomLevel;
      const scaledHeight = baseHeight * this.zoomLevel;

      // Maximum pan is half the overflow on each side
      const maxPanX = Math.max(0, (scaledWidth - baseWidth) / 2 / this.zoomLevel);
      const maxPanY = Math.max(0, (scaledHeight - baseHeight) / 2 / this.zoomLevel);

      // Constrain pan values
      this.panX = Math.max(-maxPanX, Math.min(maxPanX, this.panX));
      this.panY = Math.max(-maxPanY, Math.min(maxPanY, this.panY));
    },
```

**Step 3: Build and verify**

Run: `npm run build-js`
Expected: No errors

**Step 4: Commit**

```bash
git add src/components/lightbox.js
git commit -m "feat(lightbox): implement pan boundary constraints"
```

---

## Task 11: Add Two-Finger Touch Pan When Zoomed

**Files:**
- Modify: `src/components/lightbox.js` (update touch handlers for pan)

**Step 1: Add two-finger pan tracking state**

Add after pinch tracking:

```javascript
    // Two-finger pan tracking (when zoomed)
    twoFingerPanStartX: null,
    twoFingerPanStartY: null,
    twoFingerPanStartPanX: null,
    twoFingerPanStartPanY: null,
```

**Step 2: Update handleTouchStart for two-finger pan**

In `handleTouchStart`, update the two-finger case to also track pan start:

```javascript
      if (event.touches.length === 2) {
        // Pinch gesture start
        event.preventDefault();
        this.pinchStartDistance = this.getPinchDistance(event.touches);
        this.pinchStartZoom = this.zoomLevel;
        const center = this.getPinchCenter(event.touches);
        this.pinchCenterX = center.x;
        this.pinchCenterY = center.y;

        // Also track for pan if zoomed
        if (this.isZoomed()) {
          this.twoFingerPanStartX = center.x;
          this.twoFingerPanStartY = center.y;
          this.twoFingerPanStartPanX = this.panX;
          this.twoFingerPanStartPanY = this.panY;
        }
      }
```

**Step 3: Update handleTouchMove for two-finger pan**

Update to handle pan alongside pinch:

```javascript
    handleTouchMove(event) {
      if (this.isVideo(this.getCurrentItem()?.contentType)) return;

      if (event.touches.length === 2) {
        event.preventDefault();

        if (this.pinchStartDistance !== null) {
          // Pinch zoom
          const currentDistance = this.getPinchDistance(event.touches);
          const scale = currentDistance / this.pinchStartDistance;
          this.setZoomLevel(this.pinchStartZoom * scale);
        }

        // Two-finger pan when zoomed
        if (this.isZoomed() && this.twoFingerPanStartX !== null) {
          const center = this.getPinchCenter(event.touches);
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
```

**Step 4: Update handleTouchStart for single-finger pan setup**

Update the single touch case to store initial pan values:

```javascript
      } else if (event.touches.length === 1) {
        // Single touch - swipe or drag
        this.touchStartX = event.touches[0].clientX;
        this.touchStartY = event.touches[0].clientY;
        if (this.isZoomed()) {
          this.dragStartPanX = this.panX;
          this.dragStartPanY = this.panY;
        }
      }
```

**Step 5: Update handleTouchEnd to reset two-finger pan state**

Add to the pinch end handling:

```javascript
      if (this.pinchStartDistance !== null) {
        if (this.zoomLevel < this.minZoom) {
          this.setZoomLevel(this.minZoom);
        }
        this.pinchStartDistance = null;
        this.pinchStartZoom = null;
        this.twoFingerPanStartX = null;
        this.twoFingerPanStartY = null;
        this.announceZoom();
        return;
      }
```

**Step 6: Build and verify**

Run: `npm run build-js`
Expected: No errors

**Step 7: Commit**

```bash
git add src/components/lightbox.js
git commit -m "feat(lightbox): add two-finger touch pan when zoomed"
```

---

## Task 12: Final Polish and Testing

**Files:**
- All modified files

**Step 1: Run full build**

Run: `npm run build`
Expected: CSS and JS build complete without errors

**Step 2: Run Go tests**

Run: `go test ./...`
Expected: All tests pass

**Step 3: Manual testing checklist**

Start the server: `./mahresources -ephemeral -bind-address=:8181`

Test the following:

1. **Fullscreen**
   - [ ] Enter key toggles fullscreen (not when in input)
   - [ ] Fullscreen button shows/hides based on API support
   - [ ] Escape exits fullscreen before closing lightbox
   - [ ] Exiting lightbox exits fullscreen

2. **Edit panel shortcuts**
   - [ ] 'e' key toggles edit panel (not when in input)
   - [ ] F2 key toggles edit panel (works even in input)
   - [ ] Typing 'e' in name input types the letter

3. **Zoom**
   - [ ] Pinch zoom on touchscreen works
   - [ ] Trackpad pinch zoom works (Cmd+scroll or pinch gesture)
   - [ ] Double-click toggles between 1x and 2x
   - [ ] Zoom indicator appears and auto-hides
   - [ ] Zoom resets when navigating to next/prev

4. **Pan**
   - [ ] Mouse drag pans when zoomed
   - [ ] Two-finger pan on touch when zoomed
   - [ ] Trackpad scroll pans when zoomed
   - [ ] Pan is constrained to image bounds

5. **Navigation**
   - [ ] Mouse drag navigates when not zoomed (momentum)
   - [ ] Two-finger swipe navigates when not zoomed
   - [ ] Single finger swipe still works

6. **Edge cases**
   - [ ] Video: no zoom/pan (native controls work)
   - [ ] SVG: zoom/pan works

**Step 4: Commit any final fixes**

```bash
git add -A
git commit -m "fix(lightbox): polish and fixes from testing"
```

**Step 5: Final commit summary**

```bash
git log --oneline -10
```

---

## Summary

This plan adds:
1. Fullscreen mode with Enter key and button
2. F2 and 'e' keyboard shortcuts for edit panel
3. Pinch zoom (touch and trackpad)
4. Double-click/tap to toggle 1x/2x zoom
5. Zoom level indicator
6. Pan when zoomed (mouse drag, touch, trackpad)
7. Momentum-based navigation when not zoomed
8. Pan boundary constraints
