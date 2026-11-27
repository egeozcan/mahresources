// @vitest-environment happy-dom
import { afterEach, describe, expect, it, vi } from 'vitest';
import { _resetDocsCache, shortcodeCompletionSource } from './shortcodeCompletion.js';

const docs = [
  { name: 'excerpt', attrs: [{ name: 'path', type: 'string' }, { name: 'field', type: 'string' }] },
];
const schema = JSON.stringify({ type: 'object', properties: { recipe: { type: 'object', properties: { intro: { type: 'string' } } } } });

async function complete(text: string) {
  const context = { pos: text.length, state: { sliceDoc: (from: number, to: number) => text.slice(from, to) } };
  return shortcodeCompletionSource(() => schema)(context as never);
}

describe('[excerpt] attribute value completion', () => {
  afterEach(() => {
    _resetDocsCache();
    vi.unstubAllGlobals();
  });

  it('offers the MetaSchema paths for path=', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => ({ ok: true, json: async () => docs })));
    const result = await complete('[excerpt path="');
    expect(result?.options.map((o: { label: string }) => o.label)).toEqual(['recipe', 'recipe.intro']);
  });

  it('offers no Meta paths for field=, which names an entity field', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => ({ ok: true, json: async () => docs })));
    expect(await complete('[excerpt field="')).toBeNull();
  });
});
