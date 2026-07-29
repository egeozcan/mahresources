import { selectorRegistry, observeSelectorField } from '../selector/selectorRegistry.ts';

/**
 * Form-level guard for a submit that is only meaningful once a selector holds
 * something. Findings 16/92 and 56/91: the tag-merge and Add Tags forms are plain
 * HTML POSTs to `/v1/…` with a `?redirect=`, so submitting one empty navigated the
 * whole page to a raw API URL and left the reader on an error page.
 *
 * Exposes `hasSelection` for the submit button's `:disabled`, and
 * `blockEmptySubmit(event)` for the submit handler. Both are needed: a disabled
 * button does not stop `form.requestSubmit()`, which the selector itself calls
 * when the user presses Enter in the combobox.
 *
 * The state lives on the form rather than in the selector because the submit
 * button is the selector's sibling, not its child — and because the confirm that
 * must not fire (`confirmAction`) is bound to the form too.
 */
export function selectionRequiredState({ field, message = 'Choose at least one item first.' } = {}) {
    return {
        field,
        selectionRequiredMessage: message,
        selectionCount: 0,
        // A plain property, not `get hasSelection()`. confirmAction composes this
        // state with the object spread, and spreading *invokes* a getter and
        // copies its result — so a getter would have frozen at `false` and the
        // submit button would never have re-enabled.
        hasSelection: false,

        /**
         * `root` is captured here rather than read from `$el` later: in an Alpine
         * method `$el` is the element whose directive is evaluating (the button,
         * the form, the input), and `$root` walks up from that same element, so
         * either can be wrong or detached by the time a handler runs.
         */
        initSelectionRequired(root) {
            this._selectionRoot = root;
            this._setSelectionCount(this._readSelectionCount());
            this._stopObserving = observeSelectorField(root, this.field, (change) => {
                this._setSelectionCount(change.values.length);
            });
        },

        _setSelectionCount(count) {
            this.selectionCount = count;
            this.hasSelection = count > 0;
        },

        destroySelectionRequired() {
            this._stopObserving?.();
            this._stopObserving = null;
        },

        /**
         * Reads through the registry handle when one is available. The guard is on
         * the form and the selector is a descendant, so at init time the selector
         * has not registered yet and the observed count is the only source; by
         * submit time the handle exists and is authoritative.
         */
        _readSelectionCount() {
            const form = this._selectionRoot?.closest?.('form');
            if (!form) return this.selectionCount;
            const handle = selectorRegistry.get(form, this.field);
            return handle ? handle.getRawValues().length : this.selectionCount;
        },

        /** Returns true when the submit was blocked. */
        blockEmptySubmit(event) {
            this._setSelectionCount(this._readSelectionCount());
            if (this.hasSelection) return false;
            event.preventDefault();
            return true;
        },
    };
}

/**
 * Alpine factory for a form whose only guard is the selection requirement (the
 * Add Tags form). Merge forms compose the same state into `confirmAction`, so the
 * destructive confirmation is skipped rather than shown over an empty selection.
 */
export function selectionRequired(options = {}) {
    return {
        ...selectionRequiredState(options),
        init() {
            this.initSelectionRequired(this.$el);
        },
        destroy() {
            this.destroySelectionRequired();
        },
        events: {
            ['@submit'](event) {
                this.blockEmptySubmit(event);
            },
        },
    };
}
