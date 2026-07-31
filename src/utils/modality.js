/**
 * Whether another modal dialog is already open.
 *
 * Two `aria-modal` dialogs open at once is a defect whichever way it paints: each
 * arms its own `x-trap`, and the reader ends up held by one while looking at the
 * other. This app has ten dialogs across `downloadCockpit.tpl`, `globalSearch.tpl`,
 * `pluginActionModal.tpl`, `pasteUpload.tpl`, `entityPicker.tpl`, `mrql.tpl`,
 * `json.tpl`, `menu.tpl`, `blockEditor.tpl` and `schemaEditorModal.tpl`.
 *
 * This lives here rather than on a component because it was on a component once.
 * The jobs cockpit grew the guard, and its own comment named `globalSearch.tpl` as a
 * dialog it could collide with — but the guard stayed on the cockpit, so Cmd+K with
 * the panel open still mounted a second dialog and a second trap. A rule that only
 * one side enforces is not a rule; sharing the implementation is what keeps the next
 * dialog from re-learning this.
 *
 * Note what this deliberately does not do: it does not decide a winner. Both of the
 * two dialogs in `.header` are `fixed inset-0 z-[60]`, and `.header` is itself a
 * stacking context at `z-index: 40`, so `z-[60]` orders them against each other and
 * not against the page — the later DOM sibling paints on top. "Who wins" is
 * therefore an accident of template order, which is exactly why the rule is that
 * neither opens over the other.
 */

/**
 * Whether an element is actually painted.
 *
 * Several of this app's overlays use `x-show`, so they stay in the document with
 * `display: none`. Querying for `[aria-modal]` alone would find them and refuse to
 * open anything, forever.
 */
export function isRendered(el) {
    if (!el || !el.isConnected) return false;
    if (typeof el.checkVisibility === 'function') return el.checkVisibility();
    return !!(el.offsetWidth || el.offsetHeight || el.getClientRects?.().length);
}

/**
 * The first painted `aria-modal` dialog, or null.
 *
 * `ignoreWithin` is the caller's own root: on the paths that can be reached while a
 * component is already open, finding its own dialog would make it refuse to do
 * anything — including close.
 */
export function blockingModal(ignoreWithin = null) {
    for (const el of document.querySelectorAll('[aria-modal="true"]')) {
        if (ignoreWithin?.contains?.(el)) continue;
        if (isRendered(el)) return el;
    }
    return null;
}
