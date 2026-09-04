import { afterEach, describe, expect, test, vi } from 'vitest';

import { blockPlugin } from './blockPlugin.js';

describe('blockPlugin render batching', () => {
  afterEach(() => vi.restoreAllMocks());

  test('renders sibling plugin blocks in one request per note and mode', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify({
      renders: { 11: '<p>criteria</p>', 12: '<p>update</p>' },
    }), { status: 200, headers: { 'Content-Type': 'application/json' } }));

    const first = blockPlugin({ id: 11, noteId: 7, type: 'plugin:pm:criteria', content: {}, state: {} }, () => false);
    const second = blockPlugin({ id: 12, noteId: 7, type: 'plugin:pm:update', content: {}, state: {} }, () => false);
    await Promise.all([first.loadRender(), second.loadRender()]);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(fetchMock).toHaveBeenCalledWith('/v1/plugins/block/render-batch', expect.objectContaining({
      method: 'POST',
      body: JSON.stringify({ noteId: 7, mode: 'view', blockIds: [11, 12] }),
    }));
    expect(first.renderedHtml).toBe('<p>criteria</p>');
    expect(second.renderedHtml).toBe('<p>update</p>');
  });
});
