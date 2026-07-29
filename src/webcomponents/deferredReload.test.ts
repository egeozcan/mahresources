// Deterministic coverage for reloadInto's staleness handling. These orderings
// (a slow reload landing after a fast one, a superseded request failing, a page
// morph re-rendering a region mid-flight) are the cases an E2E test cannot
// reliably produce, and they are exactly where a wrong guard corrupts content or
// reports an outcome that contradicts what the reader sees.
import { beforeAll, beforeEach, describe, expect, it, vi } from 'vitest';

// deferredShortcodes.js registers custom elements at module scope, so the DOM
// globals it touches have to exist before it is imported. vitest runs in node.
beforeAll(() => {
  vi.stubGlobal('HTMLElement', class {});
  vi.stubGlobal('customElements', { get: () => undefined, define: () => {} });
  vi.stubGlobal('window', {});
});

let reloadInto: (host: any, contentEl: any, token: string, isStale?: () => boolean) => Promise<any>;

beforeAll(async () => {
  ({ reloadInto } = await import('./deferredShortcodes.js'));
});

function fakeEl() {
  const attrs = new Map<string, string>();
  const classes = new Set<string>();
  return {
    innerHTML: 'original',
    isConnected: true,
    setAttribute: (k: string, v: string) => void attrs.set(k, v),
    removeAttribute: (k: string) => void attrs.delete(k),
    hasAttribute: (k: string) => attrs.has(k),
    getAttribute: (k: string) => attrs.get(k) ?? null,
    classList: {
      add: (c: string) => void classes.add(c),
      remove: (c: string) => void classes.delete(c),
      contains: (c: string) => classes.has(c),
    },
  };
}

// Each fetch call parks its settle function so a test can land responses in any
// order it likes.
let pending: Array<{ resolve: (html: string) => void; reject: (err: Error) => void }>;

beforeEach(() => {
  pending = [];
  vi.stubGlobal(
    'fetch',
    vi.fn(
      () =>
        new Promise((res, rej) => {
          pending.push({
            resolve: (html: string) => res({ ok: true, json: async () => ({ html }) }),
            reject: (err: Error) => rej(err),
          });
        }),
    ),
  );
});

describe('reloadInto', () => {
  it('lets only the newest reload write, whatever order the responses land in', async () => {
    const el = fakeEl();

    const first = reloadInto(el, el, 'token-a');
    const second = reloadInto(el, el, 'token-b');

    // The slower first request comes back last and must not win.
    pending[1].resolve('from second');
    pending[0].resolve('from first');

    await expect(second).resolves.toBe(true);
    await expect(first).resolves.toBe(false);
    expect(el.innerHTML).toBe('from second');
  });

  it('keeps the busy state owned by the newest reload', async () => {
    const el = fakeEl();

    const first = reloadInto(el, el, 'token-a');
    const second = reloadInto(el, el, 'token-b');

    pending[0].resolve('from first');
    await first;
    // The superseded response must not clear a busy state it no longer owns.
    expect(el.getAttribute('aria-busy')).toBe('true');
    expect(el.classList.contains('deferred-reloading')).toBe(true);

    pending[1].resolve('from second');
    await second;
    expect(el.getAttribute('aria-busy')).toBeNull();
    expect(el.classList.contains('deferred-reloading')).toBe(false);
  });

  it('swallows a superseded failure instead of reporting it', async () => {
    const el = fakeEl();

    const first = reloadInto(el, el, 'token-a');
    const second = reloadInto(el, el, 'token-b');

    pending[1].resolve('from second');
    await second;
    pending[0].reject(new Error('deferred render failed: 400'));

    // Announcing this failure would contradict the content already on screen.
    await expect(first).resolves.toBe(false);
  });

  it('rejects a failure that is still the current attempt', async () => {
    const el = fakeEl();

    const only = reloadInto(el, el, 'token-a');
    pending[0].reject(new Error('deferred render failed: 400'));

    await expect(only).rejects.toThrow('deferred render failed');
    // A failed reload must leave what the reader was looking at untouched.
    expect(el.innerHTML).toBe('original');
    expect(el.getAttribute('aria-busy')).toBeNull();
  });

  it('abandons the swap when the container was re-rendered underneath', async () => {
    const el = fakeEl();
    el.setAttribute('data-shortcode-region', 'token-a');

    const run = reloadInto(el, el, 'token-a', () => el.getAttribute('data-shortcode-region') !== 'token-a');
    // A page morph re-renders the region: the seal is nonced, so the token differs.
    el.setAttribute('data-shortcode-region', 'token-b');
    pending[0].resolve('stale payload');

    await expect(run).resolves.toBe(false);
    expect(el.innerHTML).toBe('original');
  });

  it('swallows a failure for a container that was re-rendered underneath', async () => {
    const el = fakeEl();
    el.setAttribute('data-shortcode-region', 'token-a');

    const run = reloadInto(el, el, 'token-a', () => el.getAttribute('data-shortcode-region') !== 'token-a');
    // Same abandonment as the success path, reached the other way: the region was
    // morphed and only then did the obsolete request fail. Reporting it would put
    // "Reload failed" over content that has just been rendered fresh.
    el.setAttribute('data-shortcode-region', 'token-b');
    pending[0].reject(new Error('deferred render failed: 400'));

    await expect(run).resolves.toBe(false);
  });

  it('swallows a failure for a container that left the document', async () => {
    const el = fakeEl();

    const run = reloadInto(el, el, 'token-a');
    el.isConnected = false;
    pending[0].reject(new Error('deferred render failed: 400'));

    // Nobody is looking at this content any more, so there is nothing to report.
    await expect(run).resolves.toBe(false);
  });

  it('does not write into a container that left the document', async () => {
    const el = fakeEl();

    const run = reloadInto(el, el, 'token-a');
    el.isConnected = false;
    pending[0].resolve('payload');

    await expect(run).resolves.toBe(false);
    expect(el.innerHTML).toBe('original');
  });
});
