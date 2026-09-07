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

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>(r => { resolve = r; });
  return { promise, resolve };
}

function taggingStore() {
  const store = makeStore([item(1), item(2)]);
  store.quickTagPanelOpen = true;
  store.resourceDetails = { ID: 1, Name: 'image 1', Tags: [] };
  store.detailsCache.set(1, store.resourceDetails);
  store.recordRecentTag = vi.fn();
  store.suggestedTags = [{ ID: 5, Name: 'seed' }, { ID: 6, Name: 'remaining' }];
  store._suggestedCache.set(1, store.suggestedTags);
  return store;
}

const seedTag = { ID: 5, Name: 'seed' };
const relatedTag = { ID: 7, Name: 'related' };

describe('suggestions follow tag writes', () => {
  it.each(['suggestion', 'search add', 'search remove', 'batch add', 'batch remove', 'undo', 'carry-forward', 'quick slot'])(
    'refreshes after %s without navigating', async path => {
      const store = taggingStore();
      const urls = routeFetches({}, { 1: [relatedTag] });
      const post = vi.spyOn(globalThis, 'fetch').mockResolvedValue({ ok: true } as Response);
      try {
        if (path === 'suggestion') await store.applySuggestedTag(seedTag);
        if (path === 'search add') await store.saveTagAddition(seedTag);
        if (path === 'search remove') await store.saveTagRemoval(seedTag);
        if (path === 'batch add') await store._batchToggleTags([seedTag], 'add');
        if (path === 'batch remove') await store._batchToggleTags([seedTag], 'remove');
        if (path === 'undo') {
          store._undoRing.push({ resourceId: 1, tags: [seedTag], action: 'add', name: 'image 1' });
          await store.undoLastTagAction();
        }
        if (path === 'carry-forward') {
          store._carryForwardTags = [seedTag];
          await store.repeatPreviousTags();
        }
        if (path === 'quick slot') {
          store.quickSlots[0][0] = [{ id: seedTag.ID, name: seedTag.Name }];
          await store.toggleTabTag(0);
        }
        await vi.waitFor(() => expect(store.suggestedTags).toEqual([relatedTag]));
        expect(urls.filter(url => url.includes('suggestedTags'))).toEqual(['/v1/resource/suggestedTags?id=1']);
      } finally { post.mockRestore(); }
    },
  );

  it('coalesces concurrent writes and preserves remaining chips while refreshing', async () => {
    const store = taggingStore();
    const first = deferred<any>();
    const second = deferred<any>();
    const suggestions = deferred<any>();
    const post = vi.spyOn(globalThis, 'fetch').mockImplementationOnce(() => first.promise).mockImplementationOnce(() => second.promise);
    fetchMock.abortableFetch.mockReturnValue({ ready: suggestions.promise });
    store.fetchResourceDetails = vi.fn();
    try {
      const write1 = store.saveTagAddition(seedTag);
      const write2 = store.saveTagAddition({ ID: 8, Name: 'other seed' });
      expect(store.suggestedTags).toEqual([{ ID: 6, Name: 'remaining' }]);
      expect(store._suggestedCache.has(1)).toBe(false);
      first.resolve({ ok: true });
      await write1;
      expect(fetchMock.abortableFetch).not.toHaveBeenCalled();
      second.resolve({ ok: true });
      await write2;
      expect(fetchMock.abortableFetch).toHaveBeenCalledTimes(1);
      expect(store.suggestedTags).toEqual([{ ID: 6, Name: 'remaining' }]);
      suggestions.resolve(jsonResponse({ suggestions: [relatedTag] }));
      await vi.waitFor(() => expect(store.suggestedTags).toEqual([relatedTag]));
    } finally { post.mockRestore(); }
  });

  it('rejects a pre-edit response even if it arrives after the refreshed response', async () => {
    const store = taggingStore();
    const stale = deferred<any>();
    const fresh = deferred<any>();
    fetchMock.abortableFetch.mockReturnValueOnce({ ready: stale.promise }).mockReturnValueOnce({ ready: fresh.promise });
    const oldRequest = store.fetchSuggestedTags(1, true);
    const post = vi.spyOn(globalThis, 'fetch').mockResolvedValue({ ok: true } as Response);
    try {
      await store.saveTagAddition(seedTag);
      fresh.resolve(jsonResponse({ suggestions: [relatedTag] }));
      await vi.waitFor(() => expect(store.suggestedTags).toEqual([relatedTag]));
      stale.resolve(jsonResponse({ suggestions: [seedTag] }));
      await oldRequest;
      expect(store.suggestedTags).toEqual([relatedTag]);
      expect(store._suggestedCache.get(1)).toEqual([relatedTag]);
    } finally { post.mockRestore(); }
  });

  it('does not fetch during writes and reconciles a failed write after rollback', async () => {
    const store = taggingStore();
    const write = deferred<any>();
    const post = vi.spyOn(globalThis, 'fetch').mockReturnValue(write.promise);
    const log = vi.spyOn(console, 'error').mockImplementation(() => {});
    routeFetches({ 1: { ID: 1, Tags: [] } }, { 1: [seedTag] });
    try {
      const pending = store.saveTagAddition(seedTag);
      const rejected = expect(pending).rejects.toThrow('Failed to add tag');
      await store.fetchSuggestedTags(1, true);
      expect(fetchMock.abortableFetch).not.toHaveBeenCalled();
      write.resolve({ ok: false, status: 400 });
      await rejected;
      await vi.waitFor(() => expect(store.suggestedTags).toEqual([seedTag]));
      expect(store.resourceDetails.Tags).toEqual([]);
    } finally { post.mockRestore(); log.mockRestore(); }
  });

  it.each(['suggestion', 'search'])('does not prune the new image when a %s write finishes after navigation', async path => {
    const store = taggingStore();
    const write = deferred<any>();
    const post = vi.spyOn(globalThis, 'fetch').mockReturnValue(write.promise);
    try {
      const pending = path === 'suggestion' ? store.applySuggestedTag(seedTag) : store.saveTagAddition(seedTag);
      store.currentIndex = 1;
      store.suggestedTags = [seedTag, relatedTag];
      store._suggestedCache.set(2, store.suggestedTags);
      write.resolve({ ok: true });
      await pending;
      expect(store.suggestedTags).toEqual([seedTag, relatedTag]);
      expect(store._suggestedCache.get(2)).toEqual([seedTag, relatedTag]);
      expect(store._suggestedCache.has(1)).toBe(false);
      expect(fetchMock.abortableFetch).not.toHaveBeenCalled();
    } finally { post.mockRestore(); }
  });

  it('reschedules a post-tag refresh interrupted by a metadata write', async () => {
    const store = taggingStore();
    const interrupted = deferred<any>();
    const replacement = deferred<any>();
    fetchMock.abortableFetch.mockReturnValueOnce({ ready: interrupted.promise }).mockReturnValueOnce({ ready: replacement.promise });
    const post = vi.spyOn(globalThis, 'fetch').mockResolvedValue({ ok: true } as Response);
    try {
      await store.saveTagAddition(seedTag);
      store._beginDetailsWrite(1); // name/description writes use this shared guard
      interrupted.resolve(jsonResponse({ suggestions: [seedTag] }));
      await vi.waitFor(() => expect(store.suggestedTagsLoading).toBe(false));
      store._endDetailsWrite(1);
      expect(fetchMock.abortableFetch).toHaveBeenCalledTimes(2);
      replacement.resolve(jsonResponse({ suggestions: [relatedTag] }));
      await vi.waitFor(() => expect(store.suggestedTags).toEqual([relatedTag]));
    } finally { post.mockRestore(); }
  });

  it('runs a deferred initial suggestion fetch when metadata writes finish', async () => {
    const store = taggingStore();
    const urls = routeFetches({}, { 1: [relatedTag] });
    store._beginDetailsWrite(1);
    await store.fetchSuggestedTags(1, true); // opening the tag panel during the write
    expect(fetchMock.abortableFetch).not.toHaveBeenCalled();
    store._endDetailsWrite(1);
    await vi.waitFor(() => expect(store.suggestedTags).toEqual([relatedTag]));
    expect(urls).toEqual(['/v1/resource/suggestedTags?id=1']);
  });

  it('invalidates without refreshing when the tag panel is closed', async () => {
    const store = taggingStore();
    store.quickTagPanelOpen = false;
    const post = vi.spyOn(globalThis, 'fetch').mockResolvedValue({ ok: true } as Response);
    try {
      await store.saveTagAddition(seedTag);
      expect(store._suggestedCache.has(1)).toBe(false);
      expect(fetchMock.abortableFetch).not.toHaveBeenCalled();
    } finally { post.mockRestore(); }
  });
});
