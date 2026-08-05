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
 * Note what this deliberately does not do: it does not decide a winner. The global
 * search dialog is `fixed inset-0 z-[60]` inside `.header`, which is itself a
 * stacking context at `z-index: 40`, so that `z-[60]` orders it against its header
 * siblings and not against the page. The jobs panel used to be in the same position
 * and is now teleported into `.overlays`, so the two are not even in the same
 * stacking context. "Who wins" is an accident of structure either way, which is
 * exactly why the rule is that neither opens over the other.
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
 * `ignoreWithin` is the caller's own root, or several of them: on the paths that can
 * be reached while a component is already open, finding its own dialog would make it
 * refuse to do anything — including close.
 *
 * It takes a list because a component's dialog need not be inside its component root
 * any more. The jobs cockpit teleports its panel into `.overlays` while its `x-data`
 * root stays in the header, so "the caller's own markup" is two disjoint subtrees.
 * Nullish entries are dropped, which is what lets a caller pass an `$refs` entry
 * that exists only while it is open.
 */
export function blockingModal(ignoreWithin = null) {
    const roots = (Array.isArray(ignoreWithin) ? ignoreWithin : [ignoreWithin]).filter(Boolean);
    for (const el of document.querySelectorAll('[aria-modal="true"]')) {
        if (roots.some((root) => root === el || root.contains?.(el))) continue;
        if (isRendered(el)) return el;
    }
    return null;
}
