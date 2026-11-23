import { containFocus } from '../utils/containFocus.js';

/**
 * Make a dialog modal: everything outside it inert, Tab kept inside.
 *
 * The same two moves confirmDialog.js makes, without its store, because the
 * review page's dialogs are their own stores' business. `inert` rather than
 * `aria-hidden`: the rest of the page has to be unreachable by click and Tab as
 * well as hidden from a screen reader. It walks every ancestor of the dialog and
 * marks each sibling along the way, so a dialog teleported into `.overlays` and
 * one rendered deeper both leave only themselves live.
 *
 * Returns a release function. Elements already inert for another reason are left
 * as they were found.
 */
export function makeModal(root) {
  const ancestors = new Set();
  for (let el = root; el && el !== document.documentElement; el = el.parentElement) ancestors.add(el);

  const inerted = [];
  for (const ancestor of ancestors) {
    const parent = ancestor.parentElement;
    if (!parent) continue;
    for (const sibling of parent.children) {
      if (ancestors.has(sibling) || sibling.inert) continue;
      sibling.inert = true;
      inerted.push(sibling);
    }
  }
  const containment = containFocus(root);

  return () => {
    containment?.stop();
    for (const el of inerted) el.inert = false;
  };
}
