import { createLiveRegion } from '../utils/ariaLiveRegion.js';
import { navigationState, navigationMethods } from './lightbox/navigation.js';
import { zoomState, zoomMethods } from './lightbox/zoom.js';
import { gestureState, gestureMethods } from './lightbox/gestures.js';
import { editPanelState, editPanelMethods } from './lightbox/editPanel.js';
import { quickTagPanelState, quickTagPanelMethods } from './lightbox/quickTagPanel.js';
import { cropPanelState, cropPanelMethods } from './lightbox/cropPanel.js';

/**
 * Register the lightbox Alpine store
 * @param {import('alpinejs').Alpine} Alpine
 */
export function registerLightboxStore(Alpine) {
  Alpine.store('lightbox', {
    // Compose state from modules
    ...navigationState,
    ...zoomState,
    ...gestureState,
    ...editPanelState,
    ...quickTagPanelState,
    ...cropPanelState,

    // Live region for screen reader announcements
    _liveRegion: null,
    liveRegion: null,

    init() {
      // Guard against multiple initializations (prevents memory leak)
      if (this._liveRegion) return;
      // Async: hydrates from the server-backed user-settings store when it resolves.
      this._loadQuickTagsFromStorage();

      this._liveRegion = createLiveRegion();
      this.liveRegion = this._liveRegion.element;

      // Listen for fullscreen changes
      this._handleFullscreenChange = () => {
        this.isFullscreen = !!(document.fullscreenElement || document.webkitFullscreenElement);
        this.announce(this.isFullscreen ? 'Entered fullscreen' : 'Exited fullscreen');
      };
      document.addEventListener('fullscreenchange', this._handleFullscreenChange);
      document.addEventListener('webkitfullscreenchange', this._handleFullscreenChange);

      // Add non-passive wheel listener to allow preventDefault for browser back/forward
      this._handleWheelEvent = (event) => {
        if (!this.isOpen) return;
        // Let edit panel handle its own scrolling
        if (event.target.closest('[data-edit-panel]')) return;
        if (event.target.closest('[data-quick-tag-panel]')) return;
        // The crop overlay sits above the image; wheeling over it must not zoom
        // the underlying image (it scrolls the crop UI instead).
        if (event.target.closest('[data-crop-overlay]')) return;
        this.handleWheel(event);
      };
      document.addEventListener('wheel', this._handleWheelEvent, { passive: false });

      // Re-clamp pan when the viewport changes size while zoomed, so a panned image cannot
      // be stranded off-screen after a resize or device rotation (BH: L2). Debounced.
      this._handleViewportResize = () => {
        if (!this.isOpen || !this.isZoomed()) return;
        if (this._resizeDebounce) clearTimeout(this._resizeDebounce);
        this._resizeDebounce = setTimeout(() => {
          this._resizeDebounce = null;
          if (this.isOpen && this.isZoomed()) this.constrainPan();
        }, 150);
      };
      window.addEventListener('resize', this._handleViewportResize);
      window.addEventListener('orientationchange', this._handleViewportResize);

      // Details go stale while the viewer sits still: the user edits the same resource on its
      // page in another tab, a collaborator retags it, a background job re-versions it. Nothing
      // in the viewer would ever ask again, because nothing navigated. Revalidating whenever
      // the tab comes back to the front means what is on screen was checked against the server
      // no earlier than the moment the user returned to it. Cheap: only while a panel that
      // shows this data is actually open.
      this._handleDetailsRevalidate = () => {
        if (document.hidden || !this.isOpen) return;
        if (!this.editPanelOpen && !this.quickTagPanelOpen) return;
        this.fetchResourceDetails(undefined, true);
        this.fetchSuggestedTags(undefined, true);
      };
      document.addEventListener('visibilitychange', this._handleDetailsRevalidate);
      window.addEventListener('focus', this._handleDetailsRevalidate);
    },

    destroy() {
      if (this._handleFullscreenChange) {
        document.removeEventListener('fullscreenchange', this._handleFullscreenChange);
        document.removeEventListener('webkitfullscreenchange', this._handleFullscreenChange);
      }
      if (this._handleWheelEvent) {
        document.removeEventListener('wheel', this._handleWheelEvent);
      }
      if (this._handleViewportResize) {
        window.removeEventListener('resize', this._handleViewportResize);
        window.removeEventListener('orientationchange', this._handleViewportResize);
      }
      if (this._handleDetailsRevalidate) {
        document.removeEventListener('visibilitychange', this._handleDetailsRevalidate);
        window.removeEventListener('focus', this._handleDetailsRevalidate);
      }
      if (this._resizeDebounce) {
        clearTimeout(this._resizeDebounce);
      }
      if (this._liveRegion) {
        this._liveRegion.destroy();
        this._liveRegion = null;
        this.liveRegion = null;
      }
      if (this.animationTimeout) {
        clearTimeout(this.animationTimeout);
      }
    },

    announce(message) {
      this._liveRegion?.announce(message);
    },

    // Compose methods from modules
    ...navigationMethods,
    ...zoomMethods,
    ...gestureMethods,
    ...editPanelMethods,
    ...quickTagPanelMethods,
    ...cropPanelMethods,
  });
}
