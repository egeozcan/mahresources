// [adjacent] numbers a repeated landmark name within one server render, but a
// fragment inserted later (a deferred body, a hover card, a re-rendered text
// block) is a render of its own, so its <nav> can repeat a name the page
// already holds. Newcomers are numbered against the rest of the document.

const SELECTOR = 'nav.shortcode-adjacent[aria-label]';

export function uniqueAdjacentLabels(container, doc = document) {
  const added = container.matches?.(SELECTOR) ? [container] : [...container.querySelectorAll(SELECTOR)];
  if (added.length === 0) return;
  const used = new Set();
  for (const nav of doc.querySelectorAll(SELECTOR)) {
    if (!container.contains(nav)) used.add(accessibleName(nav));
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
