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

function fakeEl(descendants: any[] = []) {
  const attrs = new Map<string, string>();
  const classes = new Set<string>();
  return {
    innerHTML: 'original',
    isConnected: true,
    // A region contains the deferred blocks inside it; reloadInto has to be able
    // to see that overlap.
    contains: (other: any) => descendants.includes(other),
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

describe('reloadInto across nested containers', () => {
  it('lets a reload inside a block supersede the region reload enclosing it', async () => {
    const inner = fakeEl();
    const region = fakeEl([inner]);

    // The reader asks for the whole region, then changes their mind and refreshes
    // just the block inside it. The region's answer is the slower one.
    const outer = reloadInto(region, region, 'region-token');
    const nested = reloadInto(inner, inner, 'inner-token');

    pending[1].resolve('fresh block');
    await expect(nested).resolves.toBe(true);

    pending[0].resolve('stale region');
    // Replacing the region would throw away the block the reader just refreshed
    // and put back a render that predates it.
    await expect(outer).resolves.toBe(false);
    expect(region.innerHTML).toBe('original');
    expect(inner.innerHTML).toBe('fresh block');
  });

  it('lets a region reload supersede a block reload inside it', async () => {
    const inner = fakeEl();
    const region = fakeEl([inner]);

    const nested = reloadInto(inner, inner, 'inner-token');
    const outer = reloadInto(region, region, 'region-token');

    pending[1].resolve('fresh region');
    await expect(outer).resolves.toBe(true);

    pending[0].resolve('stale block');
    // The region render already contains this block, freshly rendered.
    await expect(nested).resolves.toBe(false);
    expect(inner.innerHTML).toBe('original');
    expect(region.innerHTML).toBe('fresh region');
  });

  it('clears the busy state of the overlapping reload it superseded', async () => {
    const inner = fakeEl();
    const region = fakeEl([inner]);

    reloadInto(region, region, 'region-token');
    expect(region.getAttribute('aria-busy')).toBe('true');

    // Nothing else will ever clear this: the abandoned reload keeps its hands off
    // shared state, and its host is not the one taking over.
    reloadInto(inner, inner, 'inner-token');
    expect(region.getAttribute('aria-busy')).toBeNull();
    expect(region.classList.contains('deferred-reloading')).toBe(false);
  });

  it('leaves unrelated containers alone', async () => {
    const a = fakeEl();
    const b = fakeEl();

    const first = reloadInto(a, a, 'token-a');
    const second = reloadInto(b, b, 'token-b');

    pending[1].resolve('from b');
    pending[0].resolve('from a');

    await expect(second).resolves.toBe(true);
    await expect(first).resolves.toBe(true);
    expect(a.innerHTML).toBe('from a');
    expect(b.innerHTML).toBe('from b');
  });
});

describe('reloadInto supersession releases the caller', () => {
  it('resolves a superseded reload without waiting for its response', async () => {
    const inner = fakeEl();
    const region = fakeEl([inner]);

    const outer = reloadInto(region, region, 'region-token');
    reloadInto(inner, inner, 'inner-token');

    // The region's request may be slow, or may never answer at all. Its outcome
    // is already decided, so the button that started it must be released now
    // rather than kept spinning on a reply nobody is waiting for.
    await expect(outer).resolves.toBe(false);
    expect(pending.every((p) => p !== undefined)).toBe(true);
  });

  it('resolves a reload superseded on its own container without waiting', async () => {
    const el = fakeEl();

    const first = reloadInto(el, el, 'token-a');
    reloadInto(el, el, 'token-b');

    // Two [reload] buttons in one region target the same container.
    await expect(first).resolves.toBe(false);
  });

  it('still reports a live reload only when its response lands', async () => {
    const el = fakeEl();
    let settled = false;

    const only = reloadInto(el, el, 'token-a').then((v) => {
      settled = true;
      return v;
    });
    await Promise.resolve();
    expect(settled).toBe(false);

    pending[0].resolve('payload');
    await expect(only).resolves.toBe(true);
  });
});

