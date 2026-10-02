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
    writeErrors: { name: null, description: null, tags: null },
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
      store.currentIndex = 1;
      expect(store.writeError('name')).toBe('');
      store.currentIndex = 0;
      post.mockResolvedValueOnce({ ok: true } as Response);
      await store.updateName('renamed');
      expect(store.writeError('name')).toBe('');
    } finally { post.mockRestore(); error.mockRestore(); }
  });

  it('shows a failed description save', async () => {
    const store = taggingStore();
    const error = vi.spyOn(console, 'error').mockImplementation(() => {});
    const post = vi.spyOn(globalThis, 'fetch').mockResolvedValue({ ok: false, status: 500 } as Response);
    try {
      await store.updateDescription('new text');
      expect(store.writeError('description')).toBe('Could not save the description. The previous text is back.');
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
      expect(store.writeError('tags')).toBe('Could not remove tags seed, related on image 1. Try again.');

      post.mockResolvedValue({ ok: true } as Response);
      await store.undoLastTagAction();
      // A success on image 1 says nothing about what is shown on image 2.
      expect(store.writeError('tags')).toBe('Could not remove tags seed, related on image 1. Try again.');
      await store._batchToggleTags([relatedTag], 'add');
      expect(store.writeError('tags')).toBe('');
    } finally { post.mockRestore(); error.mockRestore(); }
  });

  it('forgets write errors when the viewer closes', async () => {
    const store = taggingStore();
    store._setWriteError('name', 1, 'failed');
    store.close();
    expect(store.writeError('name')).toBe('');
    expect(store.writeErrors.name).toBe(null);
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
      back,
    };
  });

  it('pushes one entry on open and closes on Back without going back again', () => {
    const store = makeStore([item(1), item(2)]);
    store._preloadUpcoming = vi.fn();
    store.open(0);
    store.open(1);
    expect(history.pushState).toHaveBeenCalledTimes(1);
    expect(state).toEqual({ page: 'own', mahLightbox: expect.any(String) });

    state = { page: 'own' };
    store._onHistoryPop();
    expect(store.isOpen).toBe(false);
    expect(back).not.toHaveBeenCalled();
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

  it('leaves history alone when another entry was pushed on top of its own', () => {
    const store = makeStore([item(1)]);
    store.open(0);
    state = { q: 'other' };
    store.close();
    expect(back).not.toHaveBeenCalled();
  });
});
