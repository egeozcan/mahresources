// [adjacent] numbers a repeated landmark name within one server render, but it
// cannot see the page's own navigation landmarks (the main menu is "Main"), and
// a fragment inserted later (a deferred body, a re-rendered text block) is a
// render of its own. So the [adjacent] navs are numbered against every other
// named navigation landmark in the document: once on load, then on insertion.

const ADJACENT = 'nav.shortcode-adjacent[aria-label]';
const NAMED_NAV = 'nav[aria-label], [role="navigation"][aria-label]';

export function uniqueAdjacentLabels(container, doc = document) {
  const added = container.matches?.(ADJACENT) ? [container] : [...container.querySelectorAll(ADJACENT)];
  if (added.length === 0) return;
  const mine = new Set(added);
  const used = new Set();
  for (const nav of doc.querySelectorAll(NAMED_NAV)) {
    if (!mine.has(nav)) used.add(accessibleName(nav));
  }
  for (const nav of added) {
    const label = accessibleName(nav);
    if (used.has(label)) {
      const base = label.replace(/ \(\d+\)$/, '');
      let n = 2;
      while (used.has(`${base} (${n})`)) n++;
      nav.setAttribute('aria-label', `${base} (${n})`);
    }
    used.add(accessibleName(nav));
  }
}

// Whitespace collapses in an accessible name, so " Meetings " is "Meetings".
function accessibleName(nav) {
  return nav.getAttribute('aria-label').trim().replace(/\s+/g, ' ');
}

// One observer covers every insertion path, as startCustomCSSHoist does.
export function startAdjacentLabels(doc = document) {
  uniqueAdjacentLabels(doc.body, doc);
  const observer = new MutationObserver((records) => {
    for (const record of records) {
      for (const node of record.addedNodes) {
        if (node.nodeType === 1 && node.isConnected) uniqueAdjacentLabels(node, doc);
      }
    }
  });
  observer.observe(doc.body, { childList: true, subtree: true });
  return observer;
}
