import { focusedElement, restoreFocus } from '../utils/focus.js';

/**
 * The app's destructive-confirmation dialog.
 *
 * This replaces `window.confirm` at every destructive site. `window.confirm` is
 * modal to the *browser*, not to the page, and that bought three properties for
 * free which a replacement has to re-earn or it is a downgrade:
 *
 *   1. It cannot be missed — no z-index, stacking context or scroll position hides it.
 *   2. It cannot be dismissed by accident — no stray click, no click-through.
 *   3. It traps focus by construction and returns focus where it came from.
 *
 * So: the dialog lives in `.overlays` (z-index 41, above the `.header` stacking
 * context at 40), it carries `x-trap`, a backdrop click does **not** dismiss it,
 * and `_applyInert` makes the rest of the page genuinely inert rather than merely
 * covered. `x-trap.inert` is *not* enough for that last one — it sets
 * `aria-hidden="true"` on siblings, which hides the page from the accessibility
 * tree but leaves it clickable and focusable.
 *
 * The 2026-07-29 UI bug hunt found two focus-trap defects in this app's own
 * modals, which is why the focus return is owned here rather than by `x-trap`:
 * `x-trap` restores to whatever had focus when it armed, and for an action inside
 * a menu that has since closed, that node is unfocusable and the reader lands on
 * `<body>`. `.noreturn` plus a component-owned restore is the pattern this
 * codebase settled on (see `templates/partials/pluginActionModal.tpl`).
 */

const DEFAULT_TITLE = 'Confirm';
const DEFAULT_CONFIRM_LABEL = 'Confirm';
const DEFAULT_CANCEL_LABEL = 'Cancel';

export function registerConfirmDialogStore(Alpine) {
    Alpine.store('confirmDialog', {
        isOpen: false,
        message: '',
        title: DEFAULT_TITLE,
        confirmLabel: DEFAULT_CONFIRM_LABEL,
        cancelLabel: DEFAULT_CANCEL_LABEL,

        _resolve: null,
        _opener: null,
        // Only the elements *this* dialog made inert, so an element that was
        // already inert for another reason is left exactly as it was found.
        _inerted: [],

        /**
         * Ask the reader to confirm, and resolve true only if they accept.
         *
         * Resolves false for every other outcome — Escape, the Cancel button, or a
         * second `ask()` arriving while one is already open. False meaning "do not
         * proceed" is the safe default for a destructive action: the failure mode
         * of a bug here must be that nothing happens, never that something is
         * destroyed without being confirmed.
         */
        ask(message, { title, confirmLabel, cancelLabel } = {}) {
            if (this.isOpen) return Promise.resolve(false);

            this.message = message || 'Are you sure?';
            this.title = title || DEFAULT_TITLE;
            this.confirmLabel = confirmLabel || DEFAULT_CONFIRM_LABEL;
            this.cancelLabel = cancelLabel || DEFAULT_CANCEL_LABEL;
            // No trigger event to capture: `ask` is reached from a submit handler,
            // not from a click, so `captureTrigger` has no `currentTarget` to read.
            // After a click on a submit button that button is the active element,
            // which is exactly the control to come back to.
            this._opener = focusedElement();
            this.isOpen = true;

            return new Promise((resolve) => {
                this._resolve = resolve;
            });
        },

        accept() {
            this._settle(true);
        },

        cancel() {
            this._settle(false);
        },

        _settle(value) {
            if (!this.isOpen) return;
            const resolve = this._resolve;
            const opener = this._opener;

            this.isOpen = false;
            this._resolve = null;
            this._opener = null;
            this._releaseInert();

            // Restore focus only once Alpine has torn the dialog down.
            //
            // Measured, not assumed: restoring synchronously here left the reader on
            // <body>. At this point in the tick `x-trap` is still armed and the
            // dialog is still mounted, so focus moved now is pulled straight back
            // inside by the trap — and then `x-if` removes the subtree and focus
            // falls to <body>, which is precisely the defect this dialog exists to
            // avoid. A macrotask runs after Alpine's microtask-flushed effects, so
            // by then the trap has released and the subtree is gone.
            setTimeout(() => restoreFocus(opener, null), 0);

            if (resolve) resolve(value);
        },

        /**
         * Make everything except this dialog inert.
         *
         * Walks `<body>`'s children, and descends into the one that contains the
         * dialog (`.overlays`) so that a modal the confirm was opened *from* — the
         * block editor, the jobs panel — goes inert too. Without that descent the
         * confirm would be a dialog floating over a still-interactive dialog.
         */
        _applyInert(dialogRoot) {
            if (!dialogRoot) return;
            this._releaseInert();

            const ancestors = new Set();
            for (let el = dialogRoot; el && el !== document.body; el = el.parentElement) {
                ancestors.add(el);
            }

            const mark = (el) => {
                // Already inert for another reason: leave it alone, and leave it out
                // of the restore list so releasing does not clear someone else's.
                if (ancestors.has(el) || el.inert) return;
                el.inert = true;
                this._inerted.push(el);
            };

            for (const child of document.body.children) {
                if (ancestors.has(child)) {
                    for (const grandchild of child.children) mark(grandchild);
                } else {
                    mark(child);
                }
            }
        },

        _releaseInert() {
            for (const el of this._inerted) el.inert = false;
            this._inerted = [];
        },
    });
}

/**
 * Ask for confirmation from a plain module, outside an Alpine expression.
 *
 * Fails **closed**: if the store is missing the answer is "no". Every page layout
 * extends `layouts/base.tpl`, which includes the dialog, so a missing store means
 * something is broken — and the right behaviour when the confirmation mechanism is
 * broken is that the destructive action does not happen.
 */
export function askToConfirm(message, options) {
    // globalThis, not window: identical in the browser, and it keeps this reachable
    // from the unit suite, which runs in node with no `window` defined.
    const store = globalThis.Alpine?.store('confirmDialog');
    if (!store) {
        console.error('confirmDialog store is not registered; refusing to confirm', message);
        return Promise.resolve(false);
    }
    return store.ask(message, options);
}
