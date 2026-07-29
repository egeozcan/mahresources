// Elements whose children are CLIENT-owned: the server only ever sends a
// placeholder inside them, so morph must patch their attributes but never walk
// into their subtree, or live content is overwritten with that placeholder.
const CLIENT_OWNED_CHILDREN_TAGS = new Set([
  'LAZY-SHORTCODE',
  'DETAILS-SHORTCODE',
  'META-SHORTCODE',
]);

// Elements that want to re-derive state once morph has finished patching them.
// That is everything above, plus <reload-shortcode>, whose children are
// server-owned and so are morphed normally: it only needs to re-check the
// button's accessible name afterwards.
const MORPH_AWARE_TAGS = new Set([...CLIENT_OWNED_CHILDREN_TAGS, 'RELOAD-SHORTCODE']);

function hasClientOwnedChildren(el) {
  return el?.nodeType === 1 && CLIENT_OWNED_CHILDREN_TAGS.has(el.tagName);
}

function isMorphAware(el) {
  return el?.nodeType === 1 && MORPH_AWARE_TAGS.has(el.tagName);
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
