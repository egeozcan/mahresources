import { describe, expect, it, beforeEach, afterEach, vi } from 'vitest';
import { readFileSync } from 'fs';
import { resolve, dirname } from 'path';
import { fileURLToPath } from 'url';
import { PluginNodeCache } from './pluginNodeCache.js';

/**
 * Node identity for plugin-rendered HTML.
 *
 * Both plugin display renderers used to build a fresh wrapper on every render
 * and hand the detached node to Lit, so Lit replaced the live node each time
 * and any state inside a plugin display was destroyed. The bug is invisible in
 * a snapshot test — the markup is identical either way — so what is asserted
 * here is *node identity*, which is the thing that was wrong.
 *
 * There is no jsdom in this project, so document.createElement is stubbed. That
 * is enough: the cache's whole job is deciding when to call it.
 */

const __dirname = dirname(fileURLToPath(import.meta.url));

function fakeElement(tagName: string) {
  return { tagName, innerHTML: '' } as unknown as HTMLElement;
}

describe('PluginNodeCache', () => {
  let createElement: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    createElement = vi.fn((tagName: string) => fakeElement(tagName));
    vi.stubGlobal('document', { createElement });
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('returns the same node while the HTML is unchanged', () => {
    const cache = new PluginNodeCache();
    const first = cache.nodeFor('meta.badge', '<b>ok</b>');
    const second = cache.nodeFor('meta.badge', '<b>ok</b>');

    expect(second).toBe(first);
    expect(createElement).toHaveBeenCalledTimes(1);
  });

  it('builds a new node when the HTML changes', () => {
    const cache = new PluginNodeCache();
    const first = cache.nodeFor('meta.badge', '<b>ok</b>');
    const second = cache.nodeFor('meta.badge', '<b>changed</b>');

    expect(second).not.toBe(first);
    expect(second.innerHTML).toBe('<b>changed</b>');
    expect(createElement).toHaveBeenCalledTimes(2);
  });

  it('keys by field, so two fields never share a node', () => {
    const cache = new PluginNodeCache();
    const a = cache.nodeFor('meta.one', '<b>same</b>');
    const b = cache.nodeFor('meta.two', '<b>same</b>');

    expect(b).not.toBe(a);
  });

  it('builds a fresh node after clear(), even for identical HTML', () => {
    const cache = new PluginNodeCache();
    const first = cache.nodeFor('meta.badge', '<b>ok</b>');
    cache.clear();
    const second = cache.nodeFor('meta.badge', '<b>ok</b>');

    // A node surviving clear() would be the stale-render bug pointing the other
    // way: the cached HTML has been dropped, so the node must go with it.
    expect(second).not.toBe(first);
  });

  it('honours the requested tag name', () => {
    const cache = new PluginNodeCache();
    cache.nodeFor('meta.badge', '<b>ok</b>');
    cache.nodeFor('other', '<b>ok</b>', 'span');

    expect(createElement).toHaveBeenNthCalledWith(1, 'div');
    expect(createElement).toHaveBeenNthCalledWith(2, 'span');
  });
});

/**
 * Both consumers must drop cached nodes wherever they drop the cached HTML.
 * The cache is per-component private state, and neither component can be
 * instantiated without a DOM, so this is asserted against the source: every
 * site that nulls _pluginHtml must clear the node cache too.
 */
describe('plugin node cache invalidation, at both call sites', () => {
  const cases = [
    { file: '../webcomponents/meta-shortcode.ts', reset: /this\._pluginHtml = null;/g },
    { file: '../schema-editor/modes/display-mode.ts', reset: /this\._pluginHtml = \{\};/g },
  ];

  it.each(cases)('$file clears the node cache wherever it clears the HTML', ({ file, reset }) => {
    const src = readFileSync(resolve(__dirname, file), 'utf-8');
    const htmlResets = src.match(reset) ?? [];
    const nodeResets = src.match(/this\._pluginNodes\.clear\(\);/g) ?? [];

    expect(htmlResets.length).toBeGreaterThan(0);
    expect(nodeResets.length).toBe(htmlResets.length);
  });
});

/**
 * window.mahBlock is the only bridge a plugin block's own script has back into
 * the editor, and it was assigned after two awaited fetches — so it was
 * undefined for the first two round-trips of every note page, which is exactly
 * when a freshly inserted block runs its script.
 *
 * Asserted on source order because the defect is an ordering one: a behavioural
 * test would need the whole Alpine component plus a DOM.
 */
describe('blockEditor: the mahBlock bridge is installed before any await', () => {
  it('assigns window.mahBlock before the first await in init()', () => {
    const src = readFileSync(resolve(__dirname, '../components/blockEditor.js'), 'utf-8');

    const bridgeAt = src.indexOf('window.mahBlock = {');
    const firstAwaitAt = src.indexOf('await this.loadBlockTypes()');

    expect(bridgeAt).toBeGreaterThan(-1);
    expect(firstAwaitAt).toBeGreaterThan(-1);
    expect(bridgeAt).toBeLessThan(firstAwaitAt);
  });
});
