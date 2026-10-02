import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

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
import { cropPanelMethods } from './cropPanel.js';

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
    _tagWriteChains: new Map(),
    writeErrors: { name: {}, description: {}, tags: {} },
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

describe('lightbox review fixes', () => {
  it('sends a remove only after the add of the same tag on the same resource settles', async () => {
    const store = taggingStore();
    routeFetches({ 1: { ID: 1, Tags: [] } });
    const add = deferred<any>();
    const urls: string[] = [];
    const post = vi.spyOn(globalThis, 'fetch').mockImplementation(((url: string) => {
      urls.push(url);
      return url.includes('addTags') ? add.promise : Promise.resolve({ ok: true });
    }) as any);
    try {
      const adding = store.saveTagAddition(seedTag);
      const removing = store.saveTagRemoval(seedTag);
      await Promise.resolve();
      await Promise.resolve();
      // The remove must not overtake the add: the server applies them in arrival order.
      expect(urls).toEqual(['/v1/resources/addTags']);
      add.resolve({ ok: true });
      await adding;
      await removing;
      expect(urls).toEqual(['/v1/resources/addTags', '/v1/resources/removeTags']);
    } finally { post.mockRestore(); }
  });

  it('still saves a tag on the next image while the same tag is in flight for the previous one', async () => {
    const store = taggingStore();
    routeFetches({ 1: { ID: 1, Tags: [] }, 2: { ID: 2, Tags: [] } });
    const first = deferred<any>();
    const bodies: string[] = [];
    const post = vi.spyOn(globalThis, 'fetch').mockImplementation(((_url: string, init: any) => {
      bodies.push(init.body.get('ID'));
      return bodies.length === 1 ? first.promise : Promise.resolve({ ok: true });
    }) as any);
    try {
      const onFirst = store.saveTagAddition(seedTag);
      store.currentIndex = 1;
      await store.saveTagAddition(seedTag);
      expect(bodies).toEqual(['1', '2']);
      first.resolve({ ok: true });
      await onFirst;
    } finally { post.mockRestore(); }
  });

  it('drops the previous image\'s suggestions before the navigation fetch resolves', () => {
    const store = taggingStore();
    fetchMock.abortableFetch.mockReturnValue({ abort: vi.fn(), ready: new Promise(() => {}) });
    store.currentIndex = 1;
    store.onResourceChange();
    // Synchronously: Shift+digit in this window must not apply image 1's chips to image 2.
    expect(store.suggestedTags).toEqual([]);
  });

  it('does not let a details prefetch that resolves after close() repopulate the cache', async () => {
    const store = makeStore([item(1), item(2)]);
    store.quickTagPanelOpen = true;
    const pending = deferred<any>();
    fetchMock.abortableFetch.mockReturnValue({ abort: vi.fn(), ready: pending.promise });
    store._preloadDetailsUpcoming();
    store.quickTagPanelOpen = false;
    store.close();
    pending.resolve(jsonResponse({ resource: { ID: 2, Name: 'image 2', Tags: [] } }));
    // Let the whole .then chain run; close() already emptied _detailsInFlight, so waiting on it
    // would not wait at all.
    await new Promise(r => setTimeout(r, 0));
    expect(store.detailsCache.has(2)).toBe(false);
  });

  it('lets the next session warm an item a stale prefetch is still out for, and keeps its marker', async () => {
    const store = makeStore([item(1), item(2)]);
    store.quickTagPanelOpen = true;
    const stale = deferred<any>();
    fetchMock.abortableFetch.mockReturnValueOnce({ abort: vi.fn(), ready: stale.promise });
    store._preloadDetailsUpcoming();
    store.close();

    // Reopened: the same item is warmed again rather than skipped as "in flight".
    store.quickTagPanelOpen = true;
    fetchMock.abortableFetch.mockReturnValueOnce({ abort: vi.fn(), ready: new Promise(() => {}) });
    store._preloadDetailsUpcoming();
    expect(fetchMock.abortableFetch).toHaveBeenCalledTimes(2);

    // The stale one landing must not clear the new session's marker.
    stale.resolve(jsonResponse({ resource: { ID: 2, Name: 'image 2', Tags: [] } }));
    await new Promise(r => setTimeout(r, 0));
    expect(store._detailsInFlight.has(2)).toBe(true);
  });

  it('keeps the video playing and the zoom when there is nowhere to go', async () => {
    const store = makeStore([item(1), item(2)]);
    store.currentIndex = 1;
    await store.next();
    store.currentIndex = 0;
    await store.prev();
    expect(store.pauseCurrentVideo).not.toHaveBeenCalled();
    expect(store.resetZoom).not.toHaveBeenCalled();

    // And they still happen when the index does move.
    await store.next();
    expect(store.currentIndex).toBe(1);
    expect(store.pauseCurrentVideo).toHaveBeenCalledTimes(1);
    expect(store.resetZoom).toHaveBeenCalledTimes(1);
  });

  it('clears the rotate spinner when the follow-up refresh fails', async () => {
    const store = { ...makeStore([item(1)]), ...cropPanelMethods, isHistoricalVersion: () => false };
    const post = vi.spyOn(globalThis, 'fetch').mockImplementation(((url: string) =>
      Promise.resolve(url.startsWith('/resource.json') ? { ok: false, status: 500 } : { ok: true })) as any);
    const error = vi.spyOn(console, 'error').mockImplementation(() => {});
    try {
      await store.rotateCurrent(90);
      expect(store.loading).toBe(false);
      expect(store.rotating).toBe(false);
      expect(store.needsRefreshOnClose).toBe(true);
      expect(store.announce).toHaveBeenLastCalledWith('Image rotated, but the viewer could not show the new version');
    } finally { post.mockRestore(); error.mockRestore(); }
  });
});

describe('batch tag writes share the per-tag chain', () => {
  it.each([
    ['quick slot', async (store: any) => {
      store.quickSlots[0][0] = [{ id: seedTag.ID, name: seedTag.Name }];
      await store.toggleTabTag(0);
    }],
    ['suggestion', (store: any) => store.applySuggestedTag(seedTag)],
  ])('holds a %s add until the tag editor\'s remove of the same tag settles', async (_name, write) => {
    const store = taggingStore();
    store.resourceDetails.Tags = [seedTag];
    routeFetches({ 1: { ID: 1, Tags: [] } });
    const remove = deferred<any>();
    const urls: string[] = [];
    const post = vi.spyOn(globalThis, 'fetch').mockImplementation(((url: string) => {
      urls.push(url);
      return url.includes('removeTags') ? remove.promise : Promise.resolve({ ok: true });
    }) as any);
    try {
      const removing = store.saveTagRemoval(seedTag);
      const adding = write(store);
      await new Promise(r => setTimeout(r, 0));
      // The add must not overtake the remove: the server applies them in arrival order.
      expect(urls).toEqual(['/v1/resources/removeTags']);
      remove.resolve({ ok: true });
      await removing;
      await adding;
      expect(urls).toEqual(['/v1/resources/removeTags', '/v1/resources/addTags']);
    } finally { post.mockRestore(); }
  });

  it('holds a tag-editor remove until every retry of a batch add has finished', async () => {
    const store = taggingStore();
    routeFetches({ 1: { ID: 1, Tags: [] } });
    const urls: string[] = [];
    let addAttempts = 0;
    const post = vi.spyOn(globalThis, 'fetch').mockImplementation(((url: string) => {
      urls.push(url);
      if (url.includes('addTags') && ++addAttempts === 1) return Promise.resolve({ ok: false, status: 503 });
      return Promise.resolve({ ok: true });
    }) as any);
    try {
      const adding = store._batchToggleTags([seedTag, relatedTag], 'add');
      const removing = store.saveTagRemoval(relatedTag);
      await adding;
      await removing;
      expect(urls).toEqual(['/v1/resources/addTags', '/v1/resources/addTags', '/v1/resources/removeTags']);
      expect(store._tagWriteChains.size).toBe(0);
    } finally { post.mockRestore(); }
  });
});

describe('visible write errors', () => {
  it('shows a failed name save on its own image only, and clears it after a later save', async () => {
    const store = taggingStore();
    const error = vi.spyOn(console, 'error').mockImplementation(() => {});
    const post = vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce({ ok: false, status: 500 } as Response);
    try {
      await store.updateName('renamed');
      expect(store.resourceDetails.Name).toBe('image 1');
      expect(store.writeError('name')).toBe('Could not save the name "renamed". The previous name is back.');
      // Said in the same words: after Enter, focus has left the field the message describes.
      expect(store.announce).toHaveBeenCalledWith('Could not save the name "renamed". The previous name is back.');
      store.currentIndex = 1;
      expect(store.writeError('name')).toBe('');
      store.currentIndex = 0;
      post.mockResolvedValueOnce({ ok: true } as Response);
      await store.updateName('renamed');
      expect(store.writeError('name')).toBe('');
    } finally { post.mockRestore(); error.mockRestore(); }
  });

  it('keeps a failure on each image: one on image 2 does not replace image 1\'s', async () => {
    const store = taggingStore();
    store._setWriteError('name', 1, 'failed on 1', store._session, ++store._writeSeq);
    store._setWriteError('name', 2, 'failed on 2', store._session, ++store._writeSeq);
    expect(store.writeError('name')).toBe('failed on 1');
    store.currentIndex = 1;
    expect(store.writeError('name')).toBe('failed on 2');
    // A later success on image 1 clears its message only.
    store.currentIndex = 0;
    store._clearWriteError('name', 1, store._session, ++store._writeSeq);
    expect(store.writeError('name')).toBe('');
    store.currentIndex = 1;
    expect(store.writeError('name')).toBe('failed on 2');
  });

  it('shows the newest failure when two on different images both concern this one', () => {
    const store = taggingStore();
    store.items.push(item(3));
    // An undo for image 3 failed while the user was on image 1, then a quick slot on image 1.
    store._setWriteError('tags', 1, 'undo for 3 failed', store._session, ++store._writeSeq, 3, [seedTag.ID]);
    store._setWriteError('tags', 1, 'slot on 1 failed', store._session, ++store._writeSeq, 1, [seedTag.ID]);
    expect(store.writeError('tags')).toBe('slot on 1 failed');
    store.currentIndex = 2;
    expect(store.writeError('tags')).toBe('undo for 3 failed');
  });

  it('keeps a tag failure until every tag it names has been saved', () => {
    const store = taggingStore();
    store._setWriteError('tags', 1, 'could not add seed, related', store._session, ++store._writeSeq, 1, [seedTag.ID, relatedTag.ID]);
    store._clearWriteError('tags', 1, store._session, ++store._writeSeq, [seedTag.ID]);
    expect(store.writeError('tags')).toBe('could not add seed, related');
    store._clearWriteError('tags', 1, store._session, ++store._writeSeq, [relatedTag.ID]);
    expect(store.writeError('tags')).toBe('');
  });

  it('dismisses the tag failures shown on this image and keeps the others', () => {
    const store = taggingStore();
    store.items.push(item(3));
    store._setWriteError('tags', 1, 'undo for 3 failed', store._session, ++store._writeSeq, 3, [seedTag.ID]);
    store._setWriteError('tags', 1, 'slot on 1 failed', store._session, ++store._writeSeq, 1, [seedTag.ID]);
    store._setWriteError('tags', 2, 'slot on 2 failed', store._session, ++store._writeSeq, 2, [seedTag.ID]);
    store.dismissWriteError('tags');
    // Both concerned image 1, so neither shows there any more, nor on image 3.
    expect(store.writeError('tags')).toBe('');
    store.currentIndex = 2;
    expect(store.writeError('tags')).toBe('');
    store.currentIndex = 1;
    expect(store.writeError('tags')).toBe('slot on 2 failed');
  });

  it('shows a failed description save', async () => {
    const store = taggingStore();
    const error = vi.spyOn(console, 'error').mockImplementation(() => {});
    const post = vi.spyOn(globalThis, 'fetch').mockResolvedValue({ ok: false, status: 500 } as Response);
    try {
      await store.updateDescription('new text');
      expect(store.writeError('description')).toBe('Could not save the description. The previous text is back.');
      expect(store.announce).toHaveBeenCalledWith('Could not save the description. The previous text is back.');
    } finally { post.mockRestore(); error.mockRestore(); }
  });

  it('shows a failed quick-slot write, and a failed undo on the image the user is on', async () => {
    const store = taggingStore();
    routeFetches({ 1: { ID: 1, Tags: [] } });
    const error = vi.spyOn(console, 'error').mockImplementation(() => {});
    const post = vi.spyOn(globalThis, 'fetch').mockResolvedValue({ ok: false, status: 400 } as Response);
    try {
      store.quickSlots[0][0] = [{ id: seedTag.ID, name: seedTag.Name }];
      await store.toggleTabTag(0);
      expect(store.writeError('tags')).toBe('Could not add tag seed. Try again.');

      store._undoRing.push({ resourceId: 1, tags: [seedTag, relatedTag], action: 'add', name: 'image 1' });
      store.currentIndex = 1;
      await store.undoLastTagAction();
      expect(store.writeError('tags')).toBe('Could not remove tags seed, related from image 1. Try again.');
      expect(store.announce).toHaveBeenLastCalledWith('Undo failed. Could not remove tags seed, related from image 1. Try again.');

      post.mockResolvedValue({ ok: true } as Response);
      // A success on another image says nothing about the failed write...
      store.items.push(item(3));
      await store._batchToggleTags([relatedTag], 'add', { targetResourceId: 3, fromUndo: true });
      expect(store.writeError('tags')).toBe('Could not remove tags seed, related from image 1. Try again.');
      // ...but retrying it on the image the message is about does.
      await store.undoLastTagAction();
      expect(store.writeError('tags')).toBe('');
    } finally { post.mockRestore(); error.mockRestore(); }
  });

  it('forgets write errors when the viewer closes', async () => {
    const store = taggingStore();
    store._setWriteError('name', 1, 'failed', store._session);
    store.close();
    expect(store.writeError('name')).toBe('');
    expect(store.writeErrors.name).toEqual({});
  });

  it('does not carry a failure that lands after close() into the next session', async () => {
    const store = taggingStore();
    const error = vi.spyOn(console, 'error').mockImplementation(() => {});
    const response = deferred<any>();
    const post = vi.spyOn(globalThis, 'fetch').mockReturnValue(response.promise);
    try {
      const saving = store.updateName('renamed');
      store.close();
      response.resolve({ ok: false, status: 500 });
      await saving;
      expect(store.writeErrors.name).toEqual({});
    } finally { post.mockRestore(); error.mockRestore(); }
  });

  it.each([
    ['its gallery is unchanged', () => {}],
    ['a standalone viewer gave the page its own gallery back', (store: any) => { store._itemsBeforeStandalone = [item(2)]; }],
  ])('announces a batch failure that lands after close(), naming its image, when %s', async (_name, setup) => {
    const store = taggingStore();
    const error = vi.spyOn(console, 'error').mockImplementation(() => {});
    const response = deferred<any>();
    const post = vi.spyOn(globalThis, 'fetch').mockReturnValue(response.promise);
    try {
      const adding = store._batchToggleTags([seedTag], 'add');
      setup(store);
      store.close();
      response.resolve({ ok: false, status: 400 });
      await adding;
      expect(store.announce).toHaveBeenLastCalledWith('Could not add tag seed to image 1. Try again.');
      expect(store.writeErrors.tags).toEqual({});
    } finally { post.mockRestore(); error.mockRestore(); }
  });

  it('does not talk over a failed undo that lands after close()', async () => {
    const store = taggingStore();
    const error = vi.spyOn(console, 'error').mockImplementation(() => {});
    const response = deferred<any>();
    const post = vi.spyOn(globalThis, 'fetch').mockReturnValue(response.promise);
    try {
      store._undoRing.push({ resourceId: 1, tags: [seedTag], action: 'add', name: 'image 1' });
      const undoing = store.undoLastTagAction();
      store.close();
      response.resolve({ ok: false, status: 400 });
      await undoing;
      expect(store.announce).toHaveBeenLastCalledWith('Could not remove tag seed from image 1. Try again.');
    } finally { post.mockRestore(); error.mockRestore(); }
  });

  it.each([
    ['name', (store: any) => store.updateName('renamed')],
    ['description', (store: any) => store.updateDescription('new text')],
    ['tag add', (store: any) => store.saveTagAddition(seedTag)],
    ['tag remove', (store: any) => store.saveTagRemoval(seedTag)],
    ['batch tag', (store: any) => store._batchToggleTags([seedTag], 'add')],
  ])('refreshes the page behind the viewer when a %s write lands after close()', async (_name, write) => {
    const store = taggingStore();
    store.editPanelOpen = true;
    const response = deferred<any>();
    const post = vi.spyOn(globalThis, 'fetch').mockReturnValue(response.promise);
    try {
      const writing = write(store);
      store.close();
      expect(store.refreshPageContent).not.toHaveBeenCalled();
      response.resolve({ ok: true });
      await writing;
      expect(store.refreshPageContent).toHaveBeenCalledTimes(1);
      // Nor left set for the next session to act on.
      expect(store.needsRefreshOnClose).toBe(false);
    } finally { post.mockRestore(); }
  });

  it('does not let a save from a closed session clear a failure in the next one', async () => {
    const store = taggingStore();
    const response = deferred<any>();
    const post = vi.spyOn(globalThis, 'fetch').mockReturnValue(response.promise);
    try {
      const saving = store.updateName('renamed');
      store.close();
      store._setWriteError('name', 1, 'failed in the next session', store._session);
      response.resolve({ ok: true });
      await saving;
      expect(store.writeError('name')).toBe('failed in the next session');
    } finally { post.mockRestore(); }
  });

  it('keeps a tag failure until a later write on its image touches a tag that failed', async () => {
    const store = taggingStore();
    routeFetches({ 1: { ID: 1, Tags: [] }, 2: { ID: 2, Tags: [] } });
    const error = vi.spyOn(console, 'error').mockImplementation(() => {});
    const post = vi.spyOn(globalThis, 'fetch').mockResolvedValue({ ok: false, status: 400 } as Response);
    try {
      await store._batchToggleTags([seedTag], 'add');
      expect(store.writeError('tags')).toBe('Could not add tag seed. Try again.');
      post.mockResolvedValue({ ok: true } as Response);
      // Other tags on the same image say nothing about seed...
      await store._batchToggleTags([relatedTag], 'add');
      await store.saveTagAddition({ ID: 8, Name: 'other' });
      expect(store.writeError('tags')).toBe('Could not add tag seed. Try again.');
      // ...a write that touches seed does.
      await store.saveTagAddition(seedTag);
      expect(store.writeError('tags')).toBe('');

      // An undo failure about image 1, shown on image 2, outlasts a success on image 2.
      store._undoRing.push({ resourceId: 1, tags: [seedTag], action: 'add', name: 'image 1' });
      store.currentIndex = 1;
      post.mockResolvedValue({ ok: false, status: 400 } as Response);
      await store.undoLastTagAction();
      post.mockResolvedValue({ ok: true } as Response);
      await store._batchToggleTags([seedTag], 'add');
      expect(store.writeError('tags')).toBe('Could not remove tag seed from image 1. Try again.');
    } finally { post.mockRestore(); error.mockRestore(); }
  });

  it.each([
    ['name', 'updateName', 'Could not save the name "B". The previous name is back.'],
    ['description', 'updateDescription', 'Could not save the description. The previous text is back.'],
  ])('does not let an older %s save clear a newer one\'s failure', async (field, method, message) => {
    const store = taggingStore();
    routeFetches({ 1: { ID: 1, Name: 'image 1', Tags: [] } });
    const error = vi.spyOn(console, 'error').mockImplementation(() => {});
    const older = deferred<any>();
    const newer = deferred<any>();
    const post = vi.spyOn(globalThis, 'fetch').mockReturnValueOnce(older.promise).mockReturnValueOnce(newer.promise);
    try {
      const savingA = store[method]('A');
      const savingB = store[method]('B');
      newer.resolve({ ok: false, status: 500 });
      await savingB;
      expect(store.writeError(field)).toBe(message);
      older.resolve({ ok: true });
      await savingA;
      expect(store.writeError(field)).toBe(message);
    } finally { post.mockRestore(); error.mockRestore(); }
  });

  it('does not let an older tag write clear a newer one\'s failure', async () => {
    const store = taggingStore();
    routeFetches({ 1: { ID: 1, Tags: [] } });
    const error = vi.spyOn(console, 'error').mockImplementation(() => {});
    const older = deferred<any>();
    const newer = deferred<any>();
    // The per-tag chain already orders two writes of one tag; stubbing it out pins the rule itself.
    store._postTagsWithRetry = vi.fn().mockReturnValueOnce(older.promise).mockReturnValueOnce(newer.promise);
    try {
      const first = store._batchToggleTags([seedTag], 'add');
      const second = store._batchToggleTags([seedTag], 'add');
      newer.resolve({ ok: false, status: 400 });
      await second;
      expect(store.writeError('tags')).toBe('Could not add tag seed. Try again.');
      older.resolve({ ok: true });
      await first;
      expect(store.writeError('tags')).toBe('Could not add tag seed. Try again.');
    } finally { error.mockRestore(); }
  });

  it('does not announce a failed undo from a closed session with the next session\'s error', async () => {
    const store = taggingStore();
    const error = vi.spyOn(console, 'error').mockImplementation(() => {});
    const response = deferred<any>();
    const post = vi.spyOn(globalThis, 'fetch').mockReturnValue(response.promise);
    try {
      store._undoRing.push({ resourceId: 1, tags: [seedTag], action: 'add', name: 'image 1' });
      const undoing = store.undoLastTagAction();
      store.close();
      store._setWriteError('tags', 1, 'The next session\'s own failure.', store._session, ++store._writeSeq, 1, [relatedTag.ID]);
      response.resolve({ ok: false, status: 400 });
      await undoing;
      expect(store.announce).toHaveBeenLastCalledWith('Could not remove tag seed from image 1. Try again.');
    } finally { post.mockRestore(); error.mockRestore(); }
  });

  it('names an undo\'s image from its entry when the viewer no longer lists it', async () => {
    const store = taggingStore();
    routeFetches({ 1: { ID: 1, Tags: [] } });
    const error = vi.spyOn(console, 'error').mockImplementation(() => {});
    const post = vi.spyOn(globalThis, 'fetch').mockResolvedValue({ ok: false, status: 400 } as Response);
    try {
      store._undoRing.push({ resourceId: 9, tags: [seedTag], action: 'add', name: 'image 9' });
      await store.undoLastTagAction();
      expect(store.writeError('tags')).toBe('Could not remove tag seed from image 9. Try again.');
    } finally { post.mockRestore(); error.mockRestore(); }
  });

  it('shows a tag failure on the image it was about too, where the user goes back to retry', async () => {
    const store = taggingStore();
    routeFetches({ 1: { ID: 1, Tags: [] }, 2: { ID: 2, Tags: [] } });
    const error = vi.spyOn(console, 'error').mockImplementation(() => {});
    const response = deferred<any>();
    const post = vi.spyOn(globalThis, 'fetch').mockReturnValue(response.promise);
    try {
      const adding = store._batchToggleTags([seedTag], 'add');
      store.currentIndex = 1;
      response.resolve({ ok: false, status: 400 });
      await adding;
      expect(store.writeError('tags')).toBe('Could not add tag seed to image 1. Try again.');
      store.currentIndex = 0;
      expect(store.writeError('tags')).toBe('Could not add tag seed to image 1. Try again.');
    } finally { post.mockRestore(); error.mockRestore(); }
  });

  it('does not post an older name save\'s failure over a newer save that went through', async () => {
    const store = taggingStore();
    routeFetches({ 1: { ID: 1, Name: 'B', Tags: [] } });
    const error = vi.spyOn(console, 'error').mockImplementation(() => {});
    const older = deferred<any>();
    const newer = deferred<any>();
    const post = vi.spyOn(globalThis, 'fetch').mockReturnValueOnce(older.promise).mockReturnValueOnce(newer.promise);
    try {
      const savingA = store.updateName('A');
      const savingB = store.updateName('B');
      newer.resolve({ ok: true });
      await savingB;
      older.resolve({ ok: false, status: 500 });
      await savingA;
      expect(store.writeError('name')).toBe('');
      expect(store.announce).not.toHaveBeenCalledWith(expect.stringContaining('Could not save the name "A"'));
    } finally { post.mockRestore(); error.mockRestore(); }
  });

  it('does not post an older tag write\'s failure over a newer write of that tag, only of that tag', async () => {
    const store = taggingStore();
    routeFetches({ 1: { ID: 1, Tags: [] } });
    const error = vi.spyOn(console, 'error').mockImplementation(() => {});
    const writes = [deferred<any>(), deferred<any>(), deferred<any>(), deferred<any>()];
    // The per-tag chain already orders two writes of one tag; stubbing it out pins the rule itself.
    store._postTagsWithRetry = vi.fn()
      .mockReturnValueOnce(writes[0].promise).mockReturnValueOnce(writes[1].promise)
      .mockReturnValueOnce(writes[2].promise).mockReturnValueOnce(writes[3].promise);
    try {
      const olderSeed = store._batchToggleTags([seedTag], 'add');
      const newerSeed = store._batchToggleTags([seedTag], 'add');
      writes[1].resolve({ ok: true });
      await newerSeed;
      writes[0].resolve({ ok: false, status: 400 });
      await olderSeed;
      expect(store.writeError('tags')).toBe('');

      // A later success on another tag says nothing about this one.
      const olderRelated = store._batchToggleTags([relatedTag], 'add');
      const newerOther = store._batchToggleTags([{ ID: 8, Name: 'other' }], 'add');
      writes[3].resolve({ ok: true });
      await newerOther;
      writes[2].resolve({ ok: false, status: 400 });
      await olderRelated;
      expect(store.writeError('tags')).toBe('Could not add tag related. Try again.');
    } finally { error.mockRestore(); }
  });

  it.each([
    ['name', 'updateName', (store: any) => [store.items[0].name, store.detailsCache.get(1)?.Name]],
    ['description', 'updateDescription', (store: any) => [store.resourceDetails.Description, store.detailsCache.get(1)?.Description]],
  ])('leaves a newer saved %s in place when an older save fails after it', async (_field, method, shown) => {
    const store = taggingStore();
    const error = vi.spyOn(console, 'error').mockImplementation(() => {});
    const older = deferred<any>();
    const newer = deferred<any>();
    const post = vi.spyOn(globalThis, 'fetch').mockReturnValueOnce(older.promise).mockReturnValueOnce(newer.promise);
    try {
      const savingA = store[method]('A');
      const savingB = store[method]('B');
      // No panel open, so nothing would refetch to correct a rolled-back value.
      store.quickTagPanelOpen = false;
      newer.resolve({ ok: true });
      await savingB;
      older.resolve({ ok: false, status: 500 });
      await savingA;
      expect(shown(store)).toEqual(['B', 'B']);
    } finally { post.mockRestore(); error.mockRestore(); }
  });

  it('keeps a newer save\'s failure when an older one fails after it', async () => {
    const store = taggingStore();
    routeFetches({ 1: { ID: 1, Name: 'image 1', Tags: [] } });
    const error = vi.spyOn(console, 'error').mockImplementation(() => {});
    const older = deferred<any>();
    const newer = deferred<any>();
    const post = vi.spyOn(globalThis, 'fetch').mockReturnValueOnce(older.promise).mockReturnValueOnce(newer.promise);
    try {
      const savingA = store.updateName('A');
      const savingB = store.updateName('B');
      newer.resolve({ ok: false, status: 500 });
      await savingB;
      older.resolve({ ok: false, status: 500 });
      await savingA;
      expect(store.writeError('name')).toBe('Could not save the name "B". The previous name is back.');
    } finally { post.mockRestore(); error.mockRestore(); }
  });

  it('never counts a write with no keys as superseded', () => {
    const store = makeStore([item(1)]);
    expect(store._isSuperseded('tags', 1, 1, [])).toBe(false);
    expect(store._isSuperseded('tags', 1, 1, null)).toBe(false);
  });
});

describe('page refresh', () => {
  it('runs one refresh at a time, with one trailing run for every call made meanwhile', async () => {
    const store = makeStore([item(1)]);
    store.refreshPageContent = editPanelMethods.refreshPageContent;
    const runs = [deferred<void>(), deferred<void>()];
    let started = 0;
    store._listSelection = { refresh: vi.fn(() => runs[started++]?.promise) };
    const first = store.refreshPageContent();
    const second = store.refreshPageContent();
    const third = store.refreshPageContent();
    expect(store._listSelection.refresh).toHaveBeenCalledTimes(1);

    let secondSettled = false;
    second.then(() => { secondSettled = true; });
    runs[0].resolve();
    await vi.waitFor(() => expect(store._listSelection.refresh).toHaveBeenCalledTimes(2));
    // A caller made during the first run waits for the run that covers it.
    expect(secondSettled).toBe(false);
    runs[1].resolve();
    await Promise.all([first, second, third]);
    expect(secondSettled).toBe(true);
    expect(store._listSelection.refresh).toHaveBeenCalledTimes(2);
  });

  it('parks focus on the list when the refreshed list no longer has the focused thumbnail', async () => {
    const store = makeStore([item(1)]);
    store.refreshPageContent = editPanelMethods.refreshPageContent;
    (globalThis as any).requestAnimationFrame = (callback: () => void) => { callback(); return 0; };
    const doc = (globalThis as any).document;
    const thumbnail: any = { isConnected: true, dataset: { resourceId: '1' } };
    thumbnail.closest = () => thumbnail;
    const list: any = {
      isConnected: true,
      contains: () => true,
      querySelector: () => null,
      querySelectorAll: () => [],
      matches: () => false,
      setAttribute: vi.fn(),
      removeAttribute: vi.fn(),
      focus: () => { doc.activeElement = list; },
    };
    doc.activeElement = thumbnail;
    doc.querySelector = (selector: string) => (selector.includes('[data-list-container]') ? list : null);
    // The edit took the resource out of the results: its card is gone and nothing replaces it.
    store._listSelection = { refresh: async () => { thumbnail.isConnected = false; doc.activeElement = doc.body; } };
    await store.refreshPageContent();
    expect(doc.activeElement).toBe(list);
  });

  it('parks focus on the rebuilt list when it was on something else in the old one', async () => {
    const store = makeStore([item(1)]);
    store.refreshPageContent = editPanelMethods.refreshPageContent;
    (globalThis as any).requestAnimationFrame = (callback: () => void) => { callback(); return 0; };
    const doc = (globalThis as any).document;
    // A card's title link: in the list, but not a thumbnail.
    const link: any = { isConnected: true, closest: () => null };
    const oldList: any = { contains: (el: any) => el === link };
    const newList: any = {
      isConnected: true,
      querySelector: () => null,
      querySelectorAll: () => [],
      matches: () => false,
      setAttribute: vi.fn(),
      removeAttribute: vi.fn(),
      focus: () => { doc.activeElement = newList; },
    };
    let list = oldList;
    doc.activeElement = link;
    doc.querySelector = (selector: string) => (selector.includes('[data-list-container]') ? list : null);
    // MRQL re-runs its query: the whole list is rebuilt and focus falls to <body>.
    store._listSelection = { refresh: async () => { link.isConnected = false; list = newList; doc.activeElement = doc.body; } };
    await store.refreshPageContent();
    expect(doc.activeElement).toBe(newList);
  });
});

describe('focus after close', () => {
  it('lands on the rebuilt thumbnail when a save\'s refresh beat close()\'s focus return', async () => {
    const store = makeStore([item(1)]);
    store.refreshPageContent = editPanelMethods.refreshPageContent;
    const frames: Array<() => void> = [];
    (globalThis as any).requestAnimationFrame = (callback: () => void) => { frames.push(callback); return 0; };
    const flushFrames = () => { while (frames.length) frames.shift()!(); };
    const doc = (globalThis as any).document;
    const focusable = (el: any) => Object.assign(el, {
      matches: () => true, setAttribute: vi.fn(), removeAttribute: vi.fn(), focus: () => { doc.activeElement = el; },
    });
    const oldThumb: any = focusable({ isConnected: true, dataset: { resourceId: '1' } });
    oldThumb.closest = () => oldThumb;
    const newThumb: any = focusable({ isConnected: true, dataset: { resourceId: '1' } });
    newThumb.closest = () => newThumb;
    const emptyList: any = focusable({ isConnected: true, contains: () => false, querySelector: () => null, querySelectorAll: () => [] });
    const newList: any = focusable({
      isConnected: true,
      contains: (el: any) => el === newThumb,
      querySelector: (selector: string) => (selector.includes('data-resource-id="1"') ? newThumb : null),
      querySelectorAll: () => [],
    });
    let list = emptyList;
    doc.querySelector = (selector: string) => (selector.includes('[data-list-container]') ? list : null);
    doc.activeElement = doc.body;
    store.isOpen = true;
    store.triggerElement = oldThumb;

    // Back saved the Name, and on a fast server the save landed and started MRQL's re-run,
    // which tore the cards down, before close()'s two-frame focus return came round.
    let finishRun!: () => void;
    store._listSelection = {
      refresh: () => {
        oldThumb.isConnected = false;
        return new Promise<void>(resolve => { finishRun = () => { list = newList; resolve(); }; });
      },
    };
    const refreshing = store.refreshPageContent();
    store.close();
    flushFrames();
    flushFrames();
    finishRun();
    await refreshing;
    await Promise.resolve();
    flushFrames();
    await Promise.resolve();
    flushFrames();
    expect(doc.activeElement).toBe(newThumb);
  });
});

describe('focus after close, superseded', () => {
  it('drops the focus return of a session the reader has reopened and closed since', async () => {
    const store = makeStore([item(1), item(2)]);
    store.refreshPageContent = editPanelMethods.refreshPageContent;
    store._preloadUpcoming = vi.fn();
    const frames: Array<() => void> = [];
    (globalThis as any).requestAnimationFrame = (callback: () => void) => { frames.push(callback); return 0; };
    const flushFrames = () => { while (frames.length) frames.shift()!(); };
    const doc = (globalThis as any).document;
    const thumb = (id: number) => {
      const el: any = { isConnected: true, dataset: { resourceId: String(id) }, matches: () => true,
        setAttribute: vi.fn(), removeAttribute: vi.fn(), focus: () => { doc.activeElement = el; } };
      el.closest = () => el;
      return el;
    };
    const a = thumb(1);
    const b = thumb(2);
    // MRQL's re-run tears every card down at once and renders new ones when it lands.
    const rebuilt: Record<string, any> = { 1: thumb(1), 2: thumb(2) };
    let list: any = null;
    doc.querySelector = (selector: string) => (selector.includes('[data-list-container]') ? list : null);
    doc.activeElement = doc.body;
    let finishRun!: () => void;
    store._listSelection = {
      refresh: () => {
        a.isConnected = false;
        b.isConnected = false;
        return new Promise<void>(resolve => {
          finishRun = () => {
            list = { isConnected: true, contains: () => false, querySelectorAll: () => [],
              querySelector: (selector: string) => rebuilt[selector.match(/data-resource-id="(\d+)"/)?.[1] ?? ''] ?? null };
            resolve();
          };
        });
      },
    };
    const refreshing = store.refreshPageContent();

    store.triggerElement = a;
    store.isOpen = true;
    store.close();
    flushFrames(); flushFrames();
    store.triggerElement = b;
    store.open(1);
    store.close();
    flushFrames(); flushFrames();
    finishRun();
    await refreshing;
    await Promise.resolve();
    flushFrames();
    await Promise.resolve();
    flushFrames();
    // B is the session the reader closed last.
    expect(doc.activeElement).toBe(rebuilt[2]);
  });
});

describe('browser history', () => {
  let state: any;
  let back: any;

  beforeEach(() => {
    state = { page: 'own' };
    back = vi.fn();
    (globalThis as any).history = {
      get state() { return state; },
      pushState: vi.fn((next: any) => { state = next; }),
      replaceState: vi.fn((next: any) => { state = next; }),
      back,
    };
  });

  afterEach(() => { delete (globalThis as any).history; });

  it('defers the push of a viewer reopened before close()\'s Back has landed', () => {
    const store = makeStore([item(1)]);
    store._preloadUpcoming = vi.fn();
    store.open(0);
    const marker = state;
    store.close();
    store.open(0);
    expect(history.pushState).toHaveBeenCalledTimes(1);

    // The traversal close() asked for lands now: it must not close the new viewer.
    state = { page: 'own' };
    store._onHistoryPop();
    expect(store.isOpen).toBe(true);
    expect(history.pushState).toHaveBeenCalledTimes(2);
    expect(state.mahLightbox).not.toBe(marker.mahLightbox);

    // And the new entry behaves like any other.
    state = { page: 'own' };
    store._onHistoryPop();
    expect(store.isOpen).toBe(false);
  });

  it('stops waiting for a Back traversal that never arrives', () => {
    vi.useFakeTimers();
    try {
      const store = makeStore([item(1)]);
      store._preloadUpcoming = vi.fn();
      store.open(0);
      const first = state.mahLightbox;
      store.close();
      store.open(0);
      expect(history.pushState).toHaveBeenCalledTimes(1);
      vi.advanceTimersByTime(1000);
      expect(store._historyBackPending).toBe(false);
      // Still on the old entry (the traversal never happened), so it is reused for this viewer.
      expect(state.mahLightbox).not.toBe(first);
      state = { page: 'own' };
      store._onHistoryPop();
      expect(store.isOpen).toBe(false);
    } finally { vi.useRealTimers(); }
  });

  it('reuses a marker entry a previous page load left behind instead of stacking another', () => {
    const store = makeStore([item(1)]);
    store._preloadUpcoming = vi.fn();
    state = { page: 'own', mahLightbox: 'left-by-reload' };
    store.open(0);
    expect(history.pushState).not.toHaveBeenCalled();
    expect(state).toEqual({ page: 'own', mahLightbox: expect.not.stringMatching(/^left-by-reload$/), mahLightboxItem: 1 });
    // Going back from it could reload the page; the marker is stripped in place instead.
    store.close();
    expect(back).not.toHaveBeenCalled();
    expect(state).toEqual({ page: 'own' });
    store.open(0);
    expect(history.pushState).toHaveBeenCalledTimes(1);
  });

  it('pushes one entry on open and closes on Back without going back again', () => {
    const store = makeStore([item(1), item(2)]);
    store._preloadUpcoming = vi.fn();
    store.open(0);
    store.open(1);
    expect(history.pushState).toHaveBeenCalledTimes(1);
    expect(state).toEqual({ page: 'own', mahLightbox: expect.any(String), mahLightboxItem: 2 });

    state = { page: 'own' };
    store._onHistoryPop();
    expect(store.isOpen).toBe(false);
    expect(back).not.toHaveBeenCalled();
  });

  // Forward lands on the entry a closed viewer left ahead of the page.
  function thumbnailsFor(ids: number[]) {
    const thumbs = new Map(ids.map(id => [id, { dataset: { resourceId: String(id), contentType: 'image/png' }, closest: () => null, nodeType: 1 }]));
    (globalThis as any).document.querySelector = (selector: string) => {
      const match = selector.match(/data-resource-id="(\d+)"/);
      return match ? thumbs.get(Number(match[1])) ?? null : null;
    };
    return thumbs;
  }

  it('records the image on screen in its entry as the reader steps', () => {
    const store = makeStore([item(1), item(2)]);
    store._preloadUpcoming = vi.fn();
    store.onResourceChange = vi.fn();
    store.open(0);
    expect(state.mahLightboxItem).toBe(1);
    store._stepTo(1);
    expect(state.mahLightboxItem).toBe(2);
    expect(state.page).toBe('own');
  });

  it.each([
    ['Back', (store: any) => { state = { page: 'own' }; store._onHistoryPop(); }],
    ['Escape', (store: any) => { store.close(); state = { page: 'own' }; store._onHistoryPop(); }],
  ])('reopens on Forward after %s, on the image it was closed on, without a new entry', (_name, closeIt) => {
    const store = makeStore([item(1), item(2)]);
    store._preloadUpcoming = vi.fn();
    store.onResourceChange = vi.fn();
    const thumbs = thumbnailsFor([1, 2]);
    store.open(0);
    store._stepTo(1);
    const marker = state;
    closeIt(store);
    expect(store.isOpen).toBe(false);
    back.mockClear();

    state = marker;
    store._onHistoryPop();
    expect(store.isOpen).toBe(true);
    expect(store.getCurrentItem().id).toBe(2);
    expect(store.triggerElement).toBe(thumbs.get(2));
    expect(history.pushState).toHaveBeenCalledTimes(1);
    expect(back).not.toHaveBeenCalled();

    // Back closes it again like any other session.
    state = { page: 'own' };
    store._onHistoryPop();
    expect(store.isOpen).toBe(false);
    expect(back).not.toHaveBeenCalled();
  });

  it('reopens in the gallery the session was opened from when the image is listed twice', () => {
    const store = makeStore([item(1)]);
    store._preloadUpcoming = vi.fn();
    const thumbs = thumbnailsFor([1]);
    // The same resource in a second gallery (a Group's related resources) further down.
    const related: any = { dataset: { resourceId: '1', contentType: 'image/png' }, nodeType: 1, closest: () => null, querySelector: () => null };
    const relatedGallery = {
      isConnected: true,
      querySelector: () => related,
      querySelectorAll: () => [related],
      dataset: { lightboxSource: '/resources', lightboxParamName: 'Groups', lightboxParamValue: '5' },
    };
    (globalThis as any).window.location = { origin: 'http://localhost' };
    related.closest = (selector: string) => (selector.includes('data-lightbox-source') ? relatedGallery : null);
    store.triggerElement = related;
    store.open(0);
    const marker = state;
    state = { page: 'own' };
    store._onHistoryPop();

    state = marker;
    store._onHistoryPop();
    expect(store.isOpen).toBe(true);
    expect(store.triggerElement).toBe(related);
    expect(store.triggerElement).not.toBe(thumbs.get(1));
  });

  it('steps back off the entry when its image is no longer on the page', () => {
    const store = makeStore([item(1)]);
    store._preloadUpcoming = vi.fn();
    thumbnailsFor([]);
    store.open(0);
    const marker = state;
    state = { page: 'own' };
    store._onHistoryPop();

    state = marker;
    store._onHistoryPop();
    expect(store.isOpen).toBe(false);
    expect(back).toHaveBeenCalledTimes(1);
    // That traversal's own popstate changes nothing.
    state = { page: 'own' };
    store._onHistoryPop();
    expect(store.isOpen).toBe(false);
    expect(back).toHaveBeenCalledTimes(1);
  });

  it('drops its entry when closed from the viewer', () => {
    const store = makeStore([item(1)]);
    store.open(0);
    store.close();
    expect(back).toHaveBeenCalledTimes(1);
    // The pop that history.back() fires arrives after close and changes nothing.
    state = { page: 'own' };
    store._onHistoryPop();
    expect(store.isOpen).toBe(false);
  });

  it('keeps a non-object page state intact under its marker', () => {
    const store = makeStore([item(1)]);
    store._preloadUpcoming = vi.fn();
    state = 'page-string';
    store.open(0);
    expect(state).toEqual({ mahLightbox: expect.any(String), mahLightboxItem: 1, previousState: 'page-string' });
  });

  it('marks a page with no state of its own with the marker alone', () => {
    const store = makeStore([item(1)]);
    store._preloadUpcoming = vi.fn();
    state = null;
    store.open(0);
    expect(state).toEqual({ mahLightbox: expect.any(String), mahLightboxItem: 1 });
  });

  it('closes the entity picker a quick slot opened, so it is not left over the page', () => {
    const store = makeStore([item(1)]);
    store._preloadUpcoming = vi.fn();
    const picker = { isOpen: true, close: vi.fn() };
    (globalThis as any).window.Alpine = { store: (name: string) => (name === 'entityPicker' ? picker : undefined) };
    store.open(0);
    state = { page: 'own' };
    store._onHistoryPop();
    expect(store.isOpen).toBe(false);
    expect(picker.close).toHaveBeenCalledTimes(1);
  });

  it('saves a Name edit in progress before Back drops the details', async () => {
    const store = taggingStore();
    store._preloadUpcoming = vi.fn();
    store.open(0);
    store.resourceDetails = { ID: 1, Name: 'image 1', Tags: [] };
    const saves: string[] = [];
    const input = {
      tagName: 'INPUT',
      closest: (sel: string) => (sel === '[data-edit-panel]' ? {} : null),
      blur: () => { saves.push('blur'); store.updateName('typed'); },
    };
    (globalThis as any).document.activeElement = input;
    const post = vi.spyOn(globalThis, 'fetch').mockResolvedValue({ ok: true } as Response);
    try {
      state = { page: 'own' };
      store._onHistoryPop();
      expect(saves).toEqual(['blur']);
      expect(post).toHaveBeenCalledWith('/v1/resource/editName?id=1', expect.anything());
    } finally { post.mockRestore(); }
  });

  it('leaves history alone when another entry was pushed on top of its own', () => {
    const store = makeStore([item(1)]);
    store.open(0);
    state = { q: 'other' };
    store.close();
    expect(back).not.toHaveBeenCalled();
  });
});
