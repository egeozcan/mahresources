const SHORTCODE_CUSTOM_ELEMENT_TAGS = new Set([
  'LAZY-SHORTCODE',
  'DETAILS-SHORTCODE',
  'META-SHORTCODE',
]);

function isShortcodeCustomElement(el) {
  return el?.nodeType === 1 && SHORTCODE_CUSTOM_ELEMENT_TAGS.has(el.tagName);
}

export function morphOptionsWithShortcodeElements(options = {}) {
  const { updating, updated, ...rest } = options;

  return {
    ...rest,
    updating(el, toEl, childrenOnly, skip, skipChildren) {
      updating?.(el, toEl, childrenOnly, skip, skipChildren);

      if (isShortcodeCustomElement(el) && isShortcodeCustomElement(toEl)) {
        if (typeof skipChildren !== 'function') {
          skip?.();
          return;
        }
        skipChildren();
      }
    },
    updated(el, toEl) {
      if (isShortcodeCustomElement(el) && typeof el.refreshFromMorph === 'function') {
        el.refreshFromMorph(toEl);
      }

      updated?.(el, toEl);
    },
  };
}

export const morphOptionsWithDeferredShortcodes = morphOptionsWithShortcodeElements;
