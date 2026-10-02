// @vitest-environment happy-dom
import { afterEach, describe, expect, it, vi } from 'vitest';

import { registerReductionReviewStore } from './reductionReview.js';

afterEach(() => vi.unstubAllGlobals());

function reviewStore() {
  let store: any;
  registerReductionReviewStore({ store: (_name: string, value: any) => { store = value; }, morph: vi.fn() } as any);
  return store;
}

const page = `
  <div data-reduction-clusters></div>
  <span data-reduction-count="3">3</span>
  <div data-reduction-version="2" data-reduction-checked="0" data-reduction-checked-losers="0"></div>`;

describe('reduction review refresh', () => {
  it('keeps the history state when it adopts a redirected page URL', async () => {
    document.body.innerHTML = page;
    // The media viewer's marker, on the entry it pushed while open.
    window.history.replaceState({ mahLightbox: 'open' }, '', '/resourceReduction?id=1&page=5');
    const redirected = new URL('/resourceReduction?id=1&page=3', window.location.href).href;
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: true, url: redirected, text: async () => page }));

    await reviewStore().refresh();

    expect(window.location.search).toBe('?id=1&page=3');
    expect(window.history.state).toEqual({ mahLightbox: 'open' });
  });
});
