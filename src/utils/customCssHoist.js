// A category's stylesheet (style[data-mr-custom-css]) arrives inside the content that first
// needed it: a card, an [mrql] result, a list header, the hover card. The server writes each
// category's block once per page render, so later content on the page relies on that one copy.
// Much of the content it arrives in is replaced or cleared afterwards (card lists refreshed in
// the background, [reload] regions, [autorefresh] and other deferred bodies, note text blocks,
// the hover card), and the block would go with it. Moving every block into the head as it
// arrives keeps it for the life of the page. The lightbox's own copy (data-mr-lightbox-css)
// belongs to the panel and is left alone.

const BLOCK = 'style[data-mr-custom-css]';

/** Move the category blocks in or under `root` into `head`, dropping exact copies. */
export function hoistCustomCSS(root, head = document.head) {
  if (!root || !head) return;
  const blocks = root.matches?.(BLOCK) ? [root] : [...(root.querySelectorAll?.(BLOCK) || [])];
  for (const block of blocks) {
    if (block.parentNode === head) continue;
    const key = block.getAttribute('data-mr-custom-css');
    const css = block.textContent;
    const held = [...head.querySelectorAll(BLOCK)].some(
      (s) => s.getAttribute('data-mr-custom-css') === key && s.textContent === css,
    );
    if (held) {
      block.remove();
    } else {
      head.appendChild(block);
    }
  }
}

/** Hoist the blocks already on the page, then every block added to the body later. */
export function startCustomCSSHoist(doc = document) {
  hoistCustomCSS(doc.body, doc.head);
  const observer = new MutationObserver((records) => {
    for (const record of records) {
      for (const node of record.addedNodes) {
        if (node.nodeType === 1) hoistCustomCSS(node, doc.head);
      }
    }
  });
  observer.observe(doc.body, { childList: true, subtree: true });
  return observer;
}
