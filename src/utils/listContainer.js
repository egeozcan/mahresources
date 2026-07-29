/**
 * The element that holds a list page's items — the target every
 * "re-fetch the page and morph the list back in" path needs to find.
 *
 * This is a JS hook, not a style. `.list-container` and `.items-container` are
 * layout classes (a CSS grid and a flex column respectively), so a view whose
 * layout is neither cannot join simply by taking one of them: giving the
 * details table's `.detail-table-wrap` a `.list-container` makes it
 * `display: grid` and the table stops filling its own box. That coupling is
 * what left `/resources/details` unfindable, so every bulk operation there
 * threw "Could not find refreshed list" after the write had already succeeded
 * (UI bug hunt 2026-07-29, finding 9).
 *
 * Templates therefore opt in with `data-list-container`. The two classes stay
 * in the selector because the templates that already carry them are correct as
 * they are, and a page must resolve to the same element before and after a
 * refresh for the morph to be meaningful.
 */
export const LIST_CONTAINER_SELECTOR = '[data-list-container], .list-container, .items-container';

/**
 * @param {Document|DocumentFragment|Element} [root=document]
 * @returns {Element|null}
 */
export function findListContainer(root = document) {
    return root.querySelector(LIST_CONTAINER_SELECTOR);
}
