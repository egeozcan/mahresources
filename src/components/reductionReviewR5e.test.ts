// @vitest-environment happy-dom

import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import { registerReductionReviewStore } from './reductionReview.js';

// Round 5E review fixes for the review store: each test is one finding.

type Store = Record<string, any>;

function makeStore(): Store {
  let store: Store = {};
  registerReductionReviewStore({ store: (_name: string, value: Store) => { store = value; } });
  store.reductionId = 7;
  return store;
}

function cardHtml(opts: { version?: string; shown?: number; memberCount?: number } = {}): string {
  const shown = opts.shown ?? 1;
  const members = Array.from({ length: shown }, (_, i) =>
    `<div class="reduction-member" tabindex="-1" data-testid="reduction-member" data-resource-id="${i + 1}"></div>`).join('');
  const version = opts.version === undefined ? '' : ` data-cluster-version="${opts.version}"`;
  return `
    <article data-cluster-id="c1" tabindex="-1" data-member-count="${opts.memberCount ?? 5}" data-cluster-state="open"${version}>
      <h3 id="cluster-c1-heading">Near-Identical</h3>
      <div class="reduction-cluster-members">${members}</div>
      <div data-testid="cluster-more-members-wrap">
        <p data-testid="cluster-shown-count">Showing ${shown} of ${opts.memberCount ?? 5} copies.</p>
        <button type="button" data-testid="cluster-more-members" data-members-after="">Show more members</button>
      </div>
    </article>`;
}

beforeEach(() => {
  document.body.innerHTML = '';
  (window as any).mahAnnounce = vi.fn();
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('Show more members', () => {
  test('asks for the version the card was rendered at, not the store version a failed refresh moved on', async () => {
    document.body.innerHTML = cardHtml({ version: '4' });
    const store = makeStore();
    store.version = 9; // adopted from a write; the refresh that would re-render the card failed
    const trigger = document.querySelector<HTMLButtonElement>('[data-testid="cluster-more-members"]')!;
    const fetchMock = vi.fn(async () => ({ ok: true, status: 200, json: async () => ({ members: [] }) }));
    vi.stubGlobal('fetch', fetchMock);

    await store.loadMoreMembers('c1', trigger);

    const url = new URL(String((fetchMock.mock.calls[0] as unknown[])[0]));
    expect(url.searchParams.get('version')).toBe('4');
  });

  test('falls back to the store version for a card that carries none', async () => {
    document.body.innerHTML = cardHtml();
    const store = makeStore();
    store.version = 9;
    const trigger = document.querySelector<HTMLButtonElement>('[data-testid="cluster-more-members"]')!;
    const fetchMock = vi.fn(async () => ({ ok: true, status: 200, json: async () => ({ members: [] }) }));
    vi.stubGlobal('fetch', fetchMock);

    await store.loadMoreMembers('c1', trigger);

    expect(new URL(String((fetchMock.mock.calls[0] as unknown[])[0])).searchParams.get('version')).toBe('9');
  });

  test('counts merged-away placeholders it appended in "Showing N of M"', async () => {
    document.body.innerHTML = cardHtml({ shown: 1, memberCount: 340 });
    const store = makeStore();
    const trigger = document.querySelector<HTMLButtonElement>('[data-testid="cluster-more-members"]')!;
    const placeholders = Array.from({ length: 199 }, (_, i) => ({ resourceId: 100 + i, Resource: null, IsWinner: false }));
    vi.stubGlobal('fetch', vi.fn(async () => ({ ok: true, status: 200, json: async () => ({ members: placeholders, after: 'next' }) })));

    await store.loadMoreMembers('c1', trigger);

    expect(document.querySelector('[data-testid="cluster-shown-count"]')!.textContent).toBe('Showing 200 of 340 copies.');
  });
});

describe('apply during a migration', () => {
  function stubApply(response: Record<string, unknown>) {
    const calls: string[] = [];
    vi.stubGlobal('fetch', vi.fn(async (input: unknown) => {
      calls.push(String(input));
      return response;
    }));
    return calls;
  }

  test('a definitive 503 upgrade refusal is shown, not polled for as a lost response', async () => {
    document.body.innerHTML = '<div data-reduction-live="body"></div>';
    const store = makeStore();
    store.confirmed = { checked: 1, figures: {} };
    store.refresh = vi.fn(async () => {});
    store._recoverApply = vi.fn(async () => ({ message: 'polled' }));
    const calls = stubApply({
      ok: false,
      status: 503,
      headers: { get: (name: string) => (name.toLowerCase() === 'content-type' ? 'application/json' : null) },
      text: async () => JSON.stringify({ error: 'upgrading' }),
    });
    (window as any).errorMessageFromResponse = async () => 'upgrading';

    await store.confirmApply();

    expect(calls).toEqual(['/v1/reduction/apply']);
    expect(store._recoverApply).not.toHaveBeenCalled();
    expect(store.error).toMatch(/being upgraded/);
    expect(store.applyUnknown).toBe('');
  });

  test('a gateway 503 (not the server answering) is still treated as a lost response', async () => {
    document.body.innerHTML = '<div data-reduction-live="body"></div>';
    const store = makeStore();
    store.confirmed = { checked: 1, figures: {} };
    store.refresh = vi.fn(async () => {});
    store._recoverApply = vi.fn(async () => ({ message: 'polled' }));
    stubApply({
      ok: false,
      status: 503,
      headers: { get: () => 'text/html' },
      text: async () => '<html>Service Unavailable</html>',
    });

    await store.confirmApply();

    expect(store._recoverApply).toHaveBeenCalled();
  });
});
