import { createLiveRegion } from '../utils/ariaLiveRegion.js';
import { navigationState, navigationMethods } from './lightbox/navigation.js';
import { zoomState, zoomMethods } from './lightbox/zoom.js';
import { gestureState, gestureMethods } from './lightbox/gestures.js';
import { editPanelState, editPanelMethods } from './lightbox/editPanel.js';

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

    // Live region for screen reader announcements
    _liveRegion: null,
    liveRegion: null,

    init() {
      // Guard against multiple initializations (prevents memory leak)
      if (this._liveRegion) return;

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
        this.handleWheel(event);
      };
      document.addEventListener('wheel', this._handleWheelEvent, { passive: false });
    },

    destroy() {
      if (this._handleFullscreenChange) {
        document.removeEventListener('fullscreenchange', this._handleFullscreenChange);
        document.removeEventListener('webkitfullscreenchange', this._handleFullscreenChange);
      }
      if (this._handleWheelEvent) {
        document.removeEventListener('wheel', this._handleWheelEvent);
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
  });
}
