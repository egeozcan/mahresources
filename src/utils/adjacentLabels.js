// [adjacent] numbers a repeated landmark name within one server render, but it
// cannot see the page's own navigation landmarks (the main menu is "Main"), and
// a fragment inserted later (a deferred body, a re-rendered text block) is a
// render of its own. So the [adjacent] navs are numbered against every other
// named navigation landmark in the document: once on load, then whenever a
// navigation landmark is inserted.

const ADJACENT = 'nav.shortcode-adjacent[aria-label]';
const NAVIGATION = 'nav, [role="navigation"]';

// uniqueAdjacentLabels renames [adjacent] navs whose accessible name another
// navigation landmark already holds. The other landmarks keep their names;
// among [adjacent] navs, those outside `inserted` keep theirs first, so a
// newly inserted fragment yields to what the reader already had.
export function uniqueAdjacentLabels(inserted = null, doc = document) {
  const adjacent = [...doc.querySelectorAll(ADJACENT)];
  if (adjacent.length === 0) return;
  const isNew = (nav) => inserted !== null && inserted.contains(nav);
  const used = new Set();
  for (const nav of doc.querySelectorAll(NAVIGATION)) {
    if (nav.matches(ADJACENT)) continue;
    const name = accessibleName(nav, doc);
    if (name) used.add(name);
  }
  for (const nav of [...adjacent.filter((n) => !isNew(n)), ...adjacent.filter(isNew)]) {
    const label = accessibleName(nav, doc);
    if (used.has(label)) {
      const base = label.replace(/ \(\d+\)$/, '');
      let n = 2;
      while (used.has(`${base} (${n})`)) n++;
      nav.setAttribute('aria-label', `${base} (${n})`);
    }
    used.add(accessibleName(nav, doc));
  }
}

// The accessible name as a screen reader computes it for a landmark:
// aria-labelledby wins over aria-label, and whitespace collapses.
function accessibleName(el, doc) {
  let name = '';
  const ids = el.getAttribute('aria-labelledby');
  if (ids) {
    name = ids
      .split(/\s+/)
      .map((id) => doc.getElementById(id)?.textContent ?? '')
      .join(' ');
  }
  if (!name.trim()) name = el.getAttribute('aria-label') ?? '';
  return name.trim().replace(/\s+/g, ' ');
}

// One observer covers every insertion path, as startCustomCSSHoist does.
export function startAdjacentLabels(doc = document) {
  uniqueAdjacentLabels(null, doc);
  const observer = new MutationObserver((records) => {
    for (const record of records) {
      for (const node of record.addedNodes) {
        if (node.nodeType !== 1 || !node.isConnected) continue;
        if (node.matches(NAVIGATION) || node.querySelector(NAVIGATION)) uniqueAdjacentLabels(node, doc);
      }
    }
  });
  observer.observe(doc.body, { childList: true, subtree: true });
  return observer;
}
