const DEFERRED_SHORTCODE_TAGS = new Set(['LAZY-SHORTCODE', 'DETAILS-SHORTCODE']);

function isDeferredShortcode(el) {
  return el?.nodeType === 1 && DEFERRED_SHORTCODE_TAGS.has(el.tagName);
}

export function morphOptionsWithDeferredShortcodes(options = {}) {
  const { updating, updated, ...rest } = options;

  return {
    ...rest,
    updating(el, toEl, childrenOnly, skip, skipChildren) {
      updating?.(el, toEl, childrenOnly, skip, skipChildren);

      if (isDeferredShortcode(el) && isDeferredShortcode(toEl)) {
        if (typeof skipChildren !== 'function') {
          skip?.();
          return;
        }
        skipChildren();
      }
    },
    updated(el, toEl) {
      if (isDeferredShortcode(el) && typeof el.refreshFromMorph === 'function') {
        el.refreshFromMorph(toEl);
      }

      updated?.(el, toEl);
    },
  };
}
