export function confirmAction({
    message = "Are you sure you want to delete?"
} = {}) {
    return {
        message,
        _shiftHeld: false,
        init() {
            // Track shift state reliably via keydown/keyup since submit events
            // don't reliably carry modifier key state across all browsers
            this._onKeyDown = (e) => { if (e.key === 'Shift') this._shiftHeld = true; };
            this._onKeyUp = (e) => { if (e.key === 'Shift') this._shiftHeld = false; };
            document.addEventListener('keydown', this._onKeyDown);
            document.addEventListener('keyup', this._onKeyUp);
        },
        destroy() {
            document.removeEventListener('keydown', this._onKeyDown);
            document.removeEventListener('keyup', this._onKeyUp);
        },
        events: {
            ["@submit"](e) {
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
