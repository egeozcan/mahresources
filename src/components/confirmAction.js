import { selectionRequiredState } from './selectionRequired.js';

export function confirmAction({
    message = "Are you sure you want to delete?",
    // Findings 16/92: when set, the form additionally requires this selector
    // field to hold something. The destructive confirm must not fire over an
    // empty selection -- "Selected tags will be deleted and merged to X. Are you
    // sure?" with nothing selected is a scare with no action behind it, and
    // accepting it navigated the page to a raw API URL.
    requireSelection = null
} = {}) {
    const selectionGuard = requireSelection
        ? selectionRequiredState(requireSelection)
        : null;

    return {
        message,
        _shiftHeld: false,
        ...(selectionGuard || {}),
        init() {
            // Track shift state reliably via keydown/keyup since submit events
            // don't reliably carry modifier key state across all browsers
            this._onKeyDown = (e) => { if (e.key === 'Shift') this._shiftHeld = true; };
            this._onKeyUp = (e) => { if (e.key === 'Shift') this._shiftHeld = false; };
            document.addEventListener('keydown', this._onKeyDown);
            document.addEventListener('keyup', this._onKeyUp);
            // $el is the component root here, and the node is attached; a later
            // handler cannot rely on either.
            if (selectionGuard) {
                this.initSelectionRequired(this.$el);
            }
        },
        destroy() {
            document.removeEventListener('keydown', this._onKeyDown);
            document.removeEventListener('keyup', this._onKeyUp);
            if (selectionGuard) {
                this.destroySelectionRequired();
            }
        },
        events: {
            ["@submit"](e) {
                // Before the shift bypass and before the confirm: an empty
                // selection is not something to confirm, it is something to stop.
                if (selectionGuard && this.blockEmptySubmit(e)) {
                    return;
                }

                if (this._shiftHeld) {
                    return;
                }

                if (confirm(message)) {
                    return;
                }

                e.preventDefault();
            }
        }
    }
}
