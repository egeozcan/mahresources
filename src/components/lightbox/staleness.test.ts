import { beforeEach, describe, expect, it, vi } from 'vitest';

const fetchMock = vi.hoisted(() => ({
  abortableFetch: vi.fn(),
}));

vi.mock('../../index.js', () => fetchMock);
vi.mock('../../userSettings.js', () => ({
  whenLoaded: vi.fn().mockResolvedValue(undefined),
  get: vi.fn(),
  set: vi.fn(),
}));

import { editPanelMethods, editPanelState } from './editPanel.js';
import { quickTagPanelMethods, quickTagPanelState } from './quickTagPanel.js';
import { navigationMethods, navigationState } from './navigation.js';

function jsonResponse(body: unknown) {
  return { ok: true, status: 200, json: async () => body };
}

// Serve /resource.json and /v1/resource/suggestedTags from a per-id map, recording every
// request so a test can assert that a revalidation actually went to the server.
function routeFetches(details: Record<number, any>, suggestions: Record<number, any[]> = {}) {
  const urls: string[] = [];
  fetchMock.abortableFetch.mockImplementation((url: string) => {
    urls.push(url);
    const id = Number(new URLSearchParams(url.split('?')[1]).get('id'));
    if (url.startsWith('/v1/resource/suggestedTags')) {
      return { abort: vi.fn(), ready: Promise.resolve(jsonResponse({ suggestions: suggestions[id] ?? [] })) };
    }
    return { abort: vi.fn(), ready: Promise.resolve(jsonResponse({ resource: details[id] ?? null })) };
  });
  return urls;
}

function makeStore(items: any[] = []) {
  const store: any = {
    ...navigationState,
    ...editPanelState,
    ...quickTagPanelState,
    ...navigationMethods,
    ...editPanelMethods,
    ...quickTagPanelMethods,
    // Fresh per-store collections: the module state objects are shared module singletons,
    // so spreading them hands every store the same Map/Set instance.
    items,
    currentIndex: 0,
    loadedPages: new Set<number>(),
    detailsCache: new Map(),
    _detailsGen: new Map(),
    _detailsWrites: new Map(),
    _detailsInFlight: new Set(),
    _suggestedCache: new Map(),
    _preloadedUrls: new Set(),
    _preloadedImages: [],
    quickSlots: [Array(9).fill(null), Array(9).fill(null), Array(9).fill(null), Array(9).fill(null)],
    recentTags: Array(9).fill(null),
    _undoRing: [],
    // Collaborators from modules this test does not exercise.
    announce: vi.fn(),
    resetZoom: vi.fn(),
    constrainPan: vi.fn(),
    scheduleMediaCheck: vi.fn(),
    pauseCurrentVideo: vi.fn(),
    refreshPageContent: vi.fn(),
    isZoomed: () => false,
  };
  return store;
}

function item(id: number, extra: Record<string, unknown> = {}) {
  return {
    id,
    viewUrl: `/v1/resource/view?id=${id}&v=hash-${id}`,
    contentType: 'image/png',
    name: `image ${id}`,
    hash: `hash-${id}`,
    width: 10,
    height: 10,
    ...extra,
  };
}

beforeEach(() => {
  vi.clearAllMocks();
  (globalThis as any).document = {
    body: { style: {} },
    querySelector: () => null,
    querySelectorAll: () => [],
    activeElement: null,
  };
  (globalThis as any).window = { innerWidth: 1400, scrollY: 0, scrollTo: () => {} };
  (globalThis as any).requestAnimationFrame = () => 0;
});

describe('lightbox display fence', () => {
  it('hides details that describe a different resource than the one on screen', () => {
    const store = makeStore([item(1), item(2)]);
    store.resourceDetails = { ID: 1, Name: 'first', Tags: [{ ID: 7, Name: 'seven' }] };

    expect(store.displayDetails()).toEqual(store.resourceDetails);

    // Navigation moved on but the incoming details have not landed yet.
    store.currentIndex = 1;
    expect(store.displayDetails()).toBeNull();
  });

  it('reports no tags and no slot match while the details on hand are the previous image\'s', () => {
    const store = makeStore([item(1), item(2)]);
    store.quickTagPanelOpen = true;
    store.quickSlots[0][0] = [{ id: 7, name: 'seven' }];
    store.resourceDetails = { ID: 1, Name: 'first', Tags: [{ ID: 7, Name: 'seven' }] };

    expect(store.slotMatchState(0)).toBe('all');
    expect(store.getCurrentTags()).toHaveLength(1);

    store.currentIndex = 1;
    expect(store.slotMatchState(0)).toBe('none');
    expect(store.isTagOnResource(7)).toBe(false);
    expect(store.getCurrentTags()).toEqual([]);
  });
});

describe('lightbox detail revalidation', () => {
  it('revalidates against the server on navigation even when the resource is cached', async () => {
    const store = makeStore([item(1), item(2)]);
    store.quickTagPanelOpen = true;
    store.resourceDetails = { ID: 1, Name: 'first', Tags: [] };
    store.detailsCache.set(2, { ID: 2, Name: 'stale second', Tags: [] });

    const urls = routeFetches({ 2: { ID: 2, Name: 'fresh second', Tags: [{ ID: 9, Name: 'nine' }] } });

    store.currentIndex = 1;
    await store.onResourceChange();

    expect(urls.filter(u => u.startsWith('/resource.json?id=2'))).toHaveLength(1);
    expect(store.displayDetails()?.Name).toBe('fresh second');
    expect(store.detailsCache.get(2).Name).toBe('fresh second');
  });

  it('paints the cached copy immediately and keeps the foreground spinner down while revalidating', async () => {
    const store = makeStore([item(1), item(2)]);
    store.quickTagPanelOpen = true;
    store.currentIndex = 1;
    store.detailsCache.set(2, { ID: 2, Name: 'cached second', Tags: [] });

    let release: (value: unknown) => void = () => {};
    fetchMock.abortableFetch.mockImplementation(() => ({
      abort: vi.fn(),
      ready: new Promise(resolve => {
        release = resolve;
      }),
    }));

    const pending = store.fetchResourceDetails(undefined, true);
    expect(store.displayDetails()?.Name).toBe('cached second');
    expect(store.detailsLoading).toBe(false);
    expect(store.detailsRevalidating).toBe(true);

    release(jsonResponse({ resource: { ID: 2, Name: 'fresh second', Tags: [] } }));
    await pending;
    expect(store.detailsRevalidating).toBe(false);
    expect(store.displayDetails()?.Name).toBe('fresh second');
  });

  it('revalidates suggested tags for a cached resource on navigation', async () => {
    const store = makeStore([item(1), item(2)]);
    store.quickTagPanelOpen = true;
    store.currentIndex = 1;
    store._suggestedCache.set(2, [{ ID: 3, Name: 'stale suggestion' }]);

    const urls = routeFetches({ 2: { ID: 2, Name: 'second', Tags: [] } }, { 2: [{ ID: 4, Name: 'fresh suggestion' }] });

    await store.onResourceChange();
    await new Promise(r => setTimeout(r, 0));

    expect(urls.filter(u => u.startsWith('/v1/resource/suggestedTags?id=2'))).toHaveLength(1);
    expect(store.suggestedTags).toEqual([{ ID: 4, Name: 'fresh suggestion' }]);
  });

  it('drops the detail and suggestion caches when the viewer closes', () => {
    const store = makeStore([item(1)]);
    store.isOpen = true;
    store.detailsCache.set(1, { ID: 1 });
    store._suggestedCache.set(1, [{ ID: 2, Name: 'two' }]);

    store.close();

    expect(store.detailsCache.size).toBe(0);
    expect(store._suggestedCache.size).toBe(0);
  });
});

describe('lightbox failed details', () => {
  it('reports a failure instead of an endless placeholder, and clears it on retry', async () => {
    const store = makeStore([item(1), item(2)]);
    store.editPanelOpen = true;
    store.currentIndex = 1;
    store.resourceDetails = { ID: 1, Name: 'first', Tags: [] };

    fetchMock.abortableFetch.mockImplementation(() => ({
      abort: vi.fn(),
      ready: Promise.resolve({ ok: false, status: 404, json: async () => ({}) }),
    }));
    await store.fetchResourceDetails();

    expect(store.displayDetails()).toBeNull();
    expect(store.detailsBusy()).toBe(false);
    expect(store.detailsFailed()).toBe(true);

    routeFetches({ 2: { ID: 2, Name: 'second', Tags: [] } });
    store.retryDetails();
    expect(store.detailsFailed()).toBe(false);
    await new Promise(r => setTimeout(r, 0));
    expect(store.displayDetails()?.Name).toBe('second');
    expect(store.detailsFailed()).toBe(false);
  });

  it('does not flag a failed revalidation that still has this resource\'s details painted', async () => {
    const store = makeStore([item(1)]);
    store.editPanelOpen = true;
    store.detailsCache.set(1, { ID: 1, Name: 'painted', Tags: [] });

    fetchMock.abortableFetch.mockImplementation(() => ({
      abort: vi.fn(),
      ready: Promise.resolve({ ok: false, status: 500, json: async () => ({}) }),
    }));
    await store.fetchResourceDetails(undefined, true);

    expect(store.displayDetails()?.Name).toBe('painted');
    expect(store.detailsFailed()).toBe(false);
  });
});

describe('lightbox media freshness', () => {
  it('re-points the on-screen item at the new version when a fetch reports a different hash', async () => {
    const store = makeStore([item(1), item(2)]);
    store.editPanelOpen = true;
    store.currentIndex = 1;
    routeFetches({
      2: { ID: 2, Name: 'renamed', Tags: [], Hash: 'hash-2-v2', ContentType: 'image/jpeg', Width: 20, Height: 30 },
    });

    await store.fetchResourceDetails();

    expect(store.items[1].hash).toBe('hash-2-v2');
    expect(store.items[1].viewUrl).toBe('/v1/resource/view?id=2&v=hash-2-v2');
    expect(store.items[1].contentType).toBe('image/jpeg');
    expect(store.items[1].name).toBe('renamed');
    expect(store.loading).toBe(true);
    expect(store.resetZoom).toHaveBeenCalled();
  });

  it('leaves the item alone when the version is unchanged', async () => {
    const store = makeStore([item(1)]);
    store.editPanelOpen = true;
    routeFetches({ 1: { ID: 1, Name: 'image 1', Tags: [], Hash: 'hash-1', ContentType: 'image/png' } });

    await store.fetchResourceDetails();

    expect(store.items[0].viewUrl).toBe('/v1/resource/view?id=1&v=hash-1');
    expect(store.loading).toBe(false);
    expect(store.resetZoom).not.toHaveBeenCalled();
  });
});
