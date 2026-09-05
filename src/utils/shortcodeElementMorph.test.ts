import { describe, expect, it, vi } from 'vitest';
import {
  morphOptionsWithDeferredShortcodes,
  morphOptionsWithShortcodeElements,
} from './shortcodeElementMorph.js';

function el(tagName: string, extra: Record<string, unknown> = {}) {
  return { nodeType: 1, tagName, ...extra };
}

describe('morphOptionsWithShortcodeElements', () => {
  it.each(['LAZY-SHORTCODE', 'DETAILS-SHORTCODE', 'META-SHORTCODE', 'SCHEMA-EDITOR'])(
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

  it.each(['LAZY-SHORTCODE', 'DETAILS-SHORTCODE', 'META-SHORTCODE', 'SCHEMA-EDITOR', 'RELOAD-SHORTCODE'])(
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

// B2. <schema-editor> is the metadata panel on every group, note and resource
// detail page. In every mode but `edit` it renders into light DOM, and the
// server sends the element empty — so morph walking into it swapped Lit's part
// markers for the server's whitespace and removed the rendered subtree. The
// panel disappeared on any detail page that also carries data-paste-context,
// which is exactly displayGroup and displayNote, on every paste-upload refresh.
describe('schema-editor is treated as client-owned', () => {
  it('never lets morph walk into a schema-editor subtree', () => {
    const skip = vi.fn();
    const skipChildren = vi.fn();
    const options = morphOptionsWithShortcodeElements();

    options.updating(el('SCHEMA-EDITOR'), el('SCHEMA-EDITOR'), false, skip, skipChildren);

    expect(skipChildren).toHaveBeenCalledOnce();
    // skip() would stop the attributes being patched too, which is the thing
    // that must still happen: a morph is how the panel learns the entity's meta
    // changed underneath it.
    expect(skip).not.toHaveBeenCalled();
  });

  it('falls back to skipping the whole element when the morph build has no skipChildren', () => {
    const skip = vi.fn();
    const options = morphOptionsWithShortcodeElements();

    options.updating(el('SCHEMA-EDITOR'), el('SCHEMA-EDITOR'), false, skip, undefined);

    expect(skip).toHaveBeenCalledOnce();
  });
});

it('preserves the children of a plugin element opting into client ownership', () => {
  const skipChildren = vi.fn();
  const refreshFromMorph = vi.fn();
  const plugin = el('PM-STATUS-CONTROL', { hasAttribute: (name: string) => name === 'data-morph-client-owned', refreshFromMorph });
  const options = morphOptionsWithShortcodeElements();
  options.updating(plugin, plugin, false, vi.fn(), skipChildren);
  options.updated(plugin, plugin);
  expect(skipChildren).toHaveBeenCalledOnce();
  expect(refreshFromMorph).toHaveBeenCalledOnce();
});
