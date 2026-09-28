// Shared open/close behaviour for the sidebar buttons that open a popup.
//
// There are two of them on the resource detail page — Custom Thumbnail and
// Image Actions — and between them they carry three rules that are easy to get
// subtly wrong and impossible to notice when wrong: the opener has to be
// captured at open and not at close, a second aria-modal dialog must never be
// mounted over a first, and focus has to go back a tick after close rather than
// synchronously. Written twice, the second copy is the one that drifts.
//
// The rules and the reasoning come from the two components that already had
// them, `customThumbnail` and `pluginActionModal`; this is the extraction, not a
// redesign. `onOpen` is the hook a component overrides — `customThumbnail` uses
// it to drop the previous run's outcome, which described a dialog that no longer
// exists.

import { captureTrigger, focusedElement, restoreFocus } from '../utils/focus.js';
import { blockingModal } from '../utils/modality.js';

export function sidebarPopup() {
  return {
    isOpen: false,

    // The control the reader pressed. Captured at open: by the time the popup
    // closes, focus is on one of its own controls, which the x-if has removed.
    _opener: null,

    open(event) {
      if (this.isOpen) return;
      // Two aria-modal dialogs open at once is a defect whichever way it paints:
      // each arms its own x-trap, and the reader ends up held by one while
      // looking at the other. Shared with every other overlay in the app, so
      // this cannot become the rule that only one side enforces.
      if (blockingModal(this.$root)) return;
      // `currentTarget` first, focus second. A click lands on the button in
      // every browser, but a mouse press does not always *focus* a button, and
      // then `focusedElement()` reports whatever was focused before — possibly
      // nothing, which is a reader dropped on <body> on close.
      this._opener = captureTrigger(event) ?? focusedElement();
      this.onOpen();
      this.isOpen = true;
    },

    // The hook. A component overrides this; the default does nothing.
    onOpen() {},

    /**
     * Close the popup and hand focus back to the opener.
     *
     * `handOff` is for the caller that is about to put something else where this
     * was: it owns the focus from here, and a restore from under it would fight
     * for it.
     */
    close({ handOff = false } = {}) {
      const opener = this._opener;
      this._opener = null;
      this.isOpen = false;
      if (handOff) return;
      // Deferred a tick, as pluginActionModal and massEditModal do: restoring
      // synchronously happens while x-trap is still armed, which pulls focus
      // straight back in before the x-if has torn the subtree down.
      this.$nextTick(() => restoreFocus(opener));
    },
  };
}
