import { describe, expect, it, vi } from 'vitest';
import {
  morphOptionsWithDeferredShortcodes,
  morphOptionsWithShortcodeElements,
} from './shortcodeElementMorph.js';

function el(tagName: string, extra: Record<string, unknown> = {}) {
  return { nodeType: 1, tagName, ...extra };
}

describe('morphOptionsWithShortcodeElements', () => {
  it.each(['LAZY-SHORTCODE', 'DETAILS-SHORTCODE', 'META-SHORTCODE'])(
    'keeps %s attributes patchable by skipping only children',
    (tagName) => {
      const skip = vi.fn();
      const skipChildren = vi.fn();
      const options = morphOptionsWithShortcodeElements();

      options.updating(el(tagName), el(tagName), false, skip, skipChildren);

      expect(skipChildren).toHaveBeenCalledOnce();
      expect(skip).not.toHaveBeenCalled();
    },
  );

  it.each(['LAZY-SHORTCODE', 'DETAILS-SHORTCODE', 'META-SHORTCODE', 'RELOAD-SHORTCODE'])(
    'refreshes %s after attributes have been patched',
    (tagName) => {
      const refreshFromMorph = vi.fn();
      const toEl = el(tagName, { patchedValue: 'fresh' });
      const options = morphOptionsWithShortcodeElements();

      options.updated(el(tagName, { refreshFromMorph }), toEl);

      expect(refreshFromMorph).toHaveBeenCalledOnce();
      expect(refreshFromMorph).toHaveBeenCalledWith(toEl);
    },
  );

  // <reload-shortcode> owns server-rendered children, so morph must patch them as
  // usual; it only wants the callback afterwards to re-check the button's name.
  it('lets morph patch reload-shortcode children while still notifying it', () => {
    const skip = vi.fn();
    const skipChildren = vi.fn();
    const refreshFromMorph = vi.fn();
    const options = morphOptionsWithShortcodeElements();

    options.updating(el('RELOAD-SHORTCODE'), el('RELOAD-SHORTCODE'), false, skip, skipChildren);
    options.updated(el('RELOAD-SHORTCODE', { refreshFromMorph }), el('RELOAD-SHORTCODE'));

    expect(skipChildren).not.toHaveBeenCalled();
    expect(skip).not.toHaveBeenCalled();
    expect(refreshFromMorph).toHaveBeenCalledOnce();
  });

  it('leaves ordinary elements to Alpine morph', () => {
    const refreshFromMorph = vi.fn();
    const skip = vi.fn();
    const skipChildren = vi.fn();
    const options = morphOptionsWithShortcodeElements();

    options.updating(el('DIV'), el('DIV'), false, skip, skipChildren);
    options.updated(el('DIV', { refreshFromMorph }), el('DIV'));

    expect(skipChildren).not.toHaveBeenCalled();
    expect(skip).not.toHaveBeenCalled();
    expect(refreshFromMorph).not.toHaveBeenCalled();
  });

  it('falls back to skipping the whole element only for old morph hook signatures', () => {
    const skip = vi.fn();
    const options = morphOptionsWithShortcodeElements();

    options.updating(el('LAZY-SHORTCODE'), el('LAZY-SHORTCODE'), false, skip);

    expect(skip).toHaveBeenCalledOnce();
  });

  it('keeps the old deferred-shortcode export as a compatibility alias', () => {
    expect(morphOptionsWithDeferredShortcodes).toBe(morphOptionsWithShortcodeElements);
  });
});
