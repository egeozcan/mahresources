// @vitest-environment happy-dom
import { afterEach, describe, expect, it, vi } from 'vitest';

import { registerReductionReviewStore } from './reductionReview.js';

afterEach(() => vi.unstubAllGlobals());

function reviewStore() {
  let store: any;
  registerReductionReviewStore({ store: (_name: string, value: any) => { store = value; }, morph: vi.fn() } as any);
  return store;
}

const state = (extra: Record<string, unknown> = {}) => JSON.stringify({
  id: 1, version: 2, status: 'ready', jobId: '', checked: 3, checkedLosers: 4, open: 5, tab: 'open',
  figures: { identical: { clusters: 2, losers: 3, keepAsVersion: false }, near: { clusters: 1, losers: 1, keepAsVersion: true }, keepsSmaller: 1 },
  bulk: { identicalApprovable: 2, identicalOpen: 2 }, applying: false, ...extra,
});

const page = (extra: Record<string, unknown> = {}) => `
  <div data-reduction-live="head"></div>
  <div data-reduction-live="body"><div data-reduction-clusters></div></div>
  <script type="application/json" data-reduction-state>${state(extra)}</script>`;

describe('reduction review refresh', () => {
  it('keeps the history state when it adopts a redirected page URL', async () => {
    document.body.innerHTML = page();
    // The media viewer's marker, on the entry it pushed while open.
    window.history.replaceState({ mahLightbox: 'open' }, '', '/reduction?id=1&page=5');
    const redirected = new URL('/reduction?id=1&page=3', window.location.href).href;
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: true, url: redirected, text: async () => page() }));

    const store = reviewStore();
    await store.refresh();

    expect(window.location.search).toBe('?id=1&page=3');
    expect(window.history.state).toEqual({ mahLightbox: 'open' });
  });

  it('takes the figures and counts the refreshed page carries', async () => {
    document.body.innerHTML = page();
    window.history.replaceState(null, '', '/reduction?id=1');
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: true,
      url: new URL('/reduction?id=1', window.location.href).href,
      text: async () => page({ version: 9, checked: 7, checkedLosers: 8, bulk: { identicalApprovable: 0, identicalOpen: 1 } }),
    }));

    const store = reviewStore();
    await store.refresh();

    expect(store.version).toBe(9);
    expect(store.checkedCount).toBe(7);
    expect(store.checkedLoserCount).toBe(8);
    expect(store.bulkFigures).toEqual({ identicalApprovable: 0, identicalOpen: 1 });
  });
});

describe('reduction apply dialog figures', () => {
  it('states each tier from the server figures, hedged by the keep-as-version flag', () => {
    document.body.innerHTML = page();
    const store = reviewStore();
    store.seed();
    store.confirmed = { figures: store.figures };

    const [identical, near] = store.tierSummaries();
    expect(identical.headline).toBe('Identical: 2 Clusters, 3 copies removed.');
    expect(identical.detail).toContain('They have the same content as the kept copy.');
    expect(identical.detail).toContain('move to the deleted-files backup unless a version still uses them');
    expect(near.headline).toBe('Near-Identical: 1 Cluster, 1 copy removed.');
    expect(near.detail).toContain('added as versions of the kept copy');
    expect(near.detail).not.toContain('same content');
  });

  it('leaves out a tier with nothing approved', () => {
    document.body.innerHTML = page({ figures: { identical: { clusters: 0, losers: 0, keepAsVersion: false }, near: { clusters: 1, losers: 2, keepAsVersion: false }, keepsSmaller: 0 } });
    const store = reviewStore();
    store.seed();
    store.confirmed = { figures: store.figures };

    expect(store.tierSummaries().map((t: { key: string }) => t.key)).toEqual(['near']);
  });
});

describe('reduction compute error handoff', () => {
  it('shows the message the create flow stored, once', () => {
    document.body.innerHTML = page();
    window.sessionStorage.setItem('reduction-compute-error-1', 'no capacity');
    window.history.replaceState(null, '', '/reduction?id=1&startFailed=1');

    const store = reviewStore();
    store.seed();

    expect(store.startError).toBe('no capacity');
    expect(window.sessionStorage.getItem('reduction-compute-error-1')).toBeNull();
    expect(window.location.search).toBe('?id=1');
  });

  it('shows the generic failure from the URL marker alone when storage is blocked', () => {
    document.body.innerHTML = page();
    window.history.replaceState(null, '', '/reduction?id=1&startFailed=1');
    const original = Object.getOwnPropertyDescriptor(window, 'sessionStorage');
    Object.defineProperty(window, 'sessionStorage', { configurable: true, get() { throw new Error('blocked'); } });
    try {
      const store = reviewStore();
      store.seed();
      expect(store.startError).toBe(true);
      expect(window.location.search).toBe('?id=1');
    } finally {
      if (original) Object.defineProperty(window, 'sessionStorage', original);
    }
  });
});

describe('reduction store figures and versions', () => {
  it('marks the figures stale from a write until the page is read again', () => {
    document.body.innerHTML = page();
    const store = reviewStore();
    store.seed();
    expect(store.figuresStale).toBe(false);
    store.adoptVersion(9);
    expect(store.version).toBe(9);
    expect(store.figuresStale).toBe(true);
    store._readState(document);
    expect(store.figuresStale).toBe(false);
  });
});
