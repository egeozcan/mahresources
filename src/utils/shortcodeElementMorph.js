// Elements whose children are CLIENT-owned: the server only ever sends a
// placeholder inside them, so morph must patch their attributes but never walk
// into their subtree, or live content is overwritten with that placeholder.
const CLIENT_OWNED_CHILDREN_TAGS = new Set([
  'LAZY-SHORTCODE',
  'DETAILS-SHORTCODE',
  'META-SHORTCODE',
  // <schema-editor> is the metadata panel on every group, note and resource
  // detail page. In every mode but `edit` it renders into light DOM, and the
  // server sends the element empty -- exactly the condition described above.
  // Without this, paste-upload's whole-`.main` morph swapped Lit's part markers
  // for the server's whitespace and removed the rendered subtree, so the panel
  // vanished on any detail page that also carries data-paste-context.
  'SCHEMA-EDITOR',
]);

// Elements that want to re-derive state once morph has finished patching them.
// That is everything above, plus <reload-shortcode>, whose children are
// server-owned and so are morphed normally: it only needs to re-check the
// button's accessible name afterwards.
const MORPH_AWARE_TAGS = new Set([...CLIENT_OWNED_CHILDREN_TAGS, 'RELOAD-SHORTCODE']);

function hasClientOwnedChildren(el) {
  return el?.nodeType === 1 && (CLIENT_OWNED_CHILDREN_TAGS.has(el.tagName) || el.hasAttribute?.('data-morph-client-owned'));
}

function isMorphAware(el) {
  return el?.nodeType === 1 && (MORPH_AWARE_TAGS.has(el.tagName) || el.hasAttribute?.('data-morph-client-owned'));
}

export function morphOptionsWithShortcodeElements(options = {}) {
  const { updating, updated, ...rest } = options;

  return {
    ...rest,
    updating(el, toEl, childrenOnly, skip, skipChildren) {
      updating?.(el, toEl, childrenOnly, skip, skipChildren);

      if (hasClientOwnedChildren(el) && hasClientOwnedChildren(toEl)) {
        if (typeof skipChildren !== 'function') {
          skip?.();
          return;
        }
        skipChildren();
      }
    },
    updated(el, toEl) {
      if (isMorphAware(el) && typeof el.refreshFromMorph === 'function') {
        el.refreshFromMorph(toEl);
      }

      updated?.(el, toEl);
    },
  };
}

export const morphOptionsWithDeferredShortcodes = morphOptionsWithShortcodeElements;

/**
 * Morph `from` into `to`, then repair the Alpine components the morph left holding stale
 * state.
 *
 * Alpine's morph copies `_x_dataStack` across matched elements and then patches the `x-data`
 * *attribute*, but it never re-reads that attribute — so an element whose x-data expression
 * changed keeps running the component built from the old one. Destroying and re-initialising
 * exactly those elements is the repair. Doing it for the whole tree instead would throw away
 * the live state the morph exists to preserve.
 *
 * Every "re-fetch the page and morph the list back in" path needs this, not just the first
 * one that hit the problem.
 */
export function morphAndReinitChangedComponents(from, to, options = {}) {
  const before = new Map();
  from.querySelectorAll('[x-data]').forEach((el) => {
    before.set(el, el.getAttribute('x-data'));
  });

  window.Alpine.morph(from, to, morphOptionsWithShortcodeElements(options));

  before.forEach((oldValue, el) => {
    if (!el.isConnected) return; // removed by the morph
    if (el.getAttribute('x-data') === oldValue) return;
    window.Alpine.destroyTree(el);
    window.Alpine.initTree(el);
  });
}
