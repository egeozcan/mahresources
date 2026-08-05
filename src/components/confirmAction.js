import { selectionRequiredState } from './selectionRequired.js';
import { selectorRegistry } from '../selector/selectorRegistry.ts';
import { askToConfirm } from './confirmDialog.js';

const DEFAULT_MESSAGE = 'Are you sure you want to delete?';

/**
 * Placeholders a confirm message may use. They are resolved at submit time,
 * because that is the only moment the numbers are known.
 *
 *   {count}   how many rows the bulk selection holds
 *   {s}       "" when {count} is 1, "s" otherwise
 *   {winner}  the Name of the single value in the form's `winner` selector
 *
 * Findings 78 and 153: the bulk toolbars all authored their own message —
 * "Are you sure you want to delete the selected resources?", "Selected tags will
 * be merged. Are you sure?" — and every one of them was discarded, so a mis-click
 * on Select All and a single-row delete produced the identical, count-less
 * "Are you sure you want to delete?". See the argument normalization below for
 * why. Once the messages arrived, they still could not state the blast radius,
 * because a string written into an x-data attribute cannot know what is ticked.
 */
function resolvePlaceholders(message, form) {
    if (!/\{(count|s|winner)\}/.test(message)) return message;

    const count = window.Alpine?.store('bulkSelection')?.selectedIds?.size ?? 0;
    return message
        .replace(/\{count\}/g, String(count))
        .replace(/\{s\}/g, count === 1 ? '' : 's')
        .replace(/\{winner\}/g, () => winnerName(form));
}

/** The Name of the single item held by the form's `winner` selector field. */
function winnerName(form) {
    if (!form) return 'the selected item';
    const values = selectorRegistry.get(form, 'winner')?.getRawValues() ?? [];
    return values[0]?.Name || 'the selected item';
}

/**
 * @param {string|object} [options] Either the message itself or an options
 *   object. **The string form is load-bearing, not a convenience.** This
 *   function destructures its argument, and destructuring a string yields
 *   `undefined` for every named property — so `confirmAction('Delete the
 *   selected notes?')` silently fell back to the default message, in all four
 *   bulk toolbars, for as long as they have existed. Accepting both shapes means
 *   the mistake cannot be made again rather than being fixed once at four call
 *   sites.
 */
export function confirmAction(options = {}) {
    const {
        message = DEFAULT_MESSAGE,
        // Findings 16/92: when set, the form additionally requires this selector
        // field to hold something. The destructive confirm must not fire over an
        // empty selection -- "Selected tags will be deleted and merged to X. Are you
        // sure?" with nothing selected is a scare with no action behind it, and
        // accepting it navigated the page to a raw API URL.
        requireSelection = null,
    } = typeof options === 'string' ? { message: options } : (options || {});

    const selectionGuard = requireSelection
        ? selectionRequiredState(requireSelection)
        : null;

    // Whether a message was actually supplied, as opposed to defaulted. Needed so
    // `data-confirm-message` can fill in without overriding a real argument.
    const messageWasGiven = typeof options === 'string'
        ? options !== ''
        : !!(options && options.message);

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
            this._formEl = this.$el.closest?.('form') || (this.$el.tagName === 'FORM' ? this.$el : null);

            // A message carrying user-controlled text belongs in a data attribute,
            // not in the x-data expression.
            //
            // Pongo2 autoescapes, so `{{ note.Name }}` inside an attribute becomes
            // `&#39;` for an apostrophe — and the HTML parser decodes that back to a
            // real quote *before* any JS in that attribute is parsed. A name like
            // "it's" would therefore terminate the string literal in
            // `x-data="confirmAction({message: '…'})"`. Read through `dataset` the
            // value is only ever a DOM string, so there is no literal to break.
            if (!messageWasGiven) {
                const fromAttr = this.$el.dataset?.confirmMessage;
                if (fromAttr) this.message = fromAttr;
            }
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
        /** The message a reader is actually shown, with the numbers filled in. */
        confirmMessage(form) {
            return resolvePlaceholders(this.message, form || this._formEl);
        },
        events: {
            async ["@submit"](e) {
                // The re-submit fired after the reader confirms passes straight
                // through — and, critically, without calling preventDefault. The
                // delegated AJAX listener in bulkSelection.js reads
                // `event.defaultPrevented` synchronously to decide whether to fetch,
                // so preventing this one would turn every confirmed bulk operation
                // into a no-op.
                if (this._confirmed) {
                    this._confirmed = false;
                    return;
                }

                // Before the shift bypass and before the confirm: an empty
                // selection is not something to confirm, it is something to stop.
                if (selectionGuard && this.blockEmptySubmit(e)) {
                    return;
                }

                if (this._shiftHeld) {
                    return;
                }

                const form = e.target;

                // Always stop this submit. The answer arrives from a dialog rather
                // than from `window.confirm`, so it cannot arrive synchronously, and
                // a submit left to proceed while we wait is a submit that happened
                // without a confirmation.
                e.preventDefault();

                // Re-entry guard: a second Enter while the dialog is up must not
                // stack a second dialog on the same form.
                if (this._awaiting) return;
                this._awaiting = true;
                let confirmed = false;
                try {
                    confirmed = await askToConfirm(this.confirmMessage(form));
                } finally {
                    this._awaiting = false;
                }

                if (!confirmed) return;

                // requestSubmit(), not submit(): submit() does not fire a submit
                // event at all, so the delegated AJAX listener would never see the
                // form and a bulk operation would go out as a full-page POST.
                // requestSubmit() also runs constraint validation, which is what the
                // old native-submit path did.
                //
                // The flag is cleared in `finally` and not by the re-entry above,
                // because validation can reject the submit and dispatch no event —
                // which would leave the flag set and let the *next* submit through
                // unconfirmed.
                this._confirmed = true;
                try {
                    form.requestSubmit();
                } finally {
                    this._confirmed = false;
                }
            }
        }
    }
}
