import { beforeEach, describe, expect, it, vi } from 'vitest';

import { navigationMethods, navigationState } from './navigation.js';

function makeStore(items: any[]) {
  const store: any = {
    ...navigationState,
    ...navigationMethods,
    items,
    currentIndex: 0,
    loadedPages: new Set<number>(),
    detailsCache: new Map(),
    _suggestedCache: new Map(),
    _preloadedUrls: new Set(),
    _preloadedImages: [],
    quickTagPanelOpen: false,
    editPanelOpen: false,
    versionPanelOpen: false,
    // Collaborators from modules this test does not exercise.
    announce: vi.fn(),
    resetZoom: vi.fn(),
    constrainPan: vi.fn(),
    scheduleMediaCheck: vi.fn(),
    pauseCurrentVideo: vi.fn(),
    _preloadUpcoming: vi.fn(),
    _preloadDetailsUpcoming: vi.fn(),
    onResourceChange: vi.fn(),
    _loadQuickTagsFromStorage: vi.fn(),
  };
  return store;
}

function item(id: number, extra: Record<string, unknown> = {}) {
  return {
    id,
    viewUrl: `/v1/resource/view?id=${id}`,
    contentType: 'image/png',
    name: `image ${id}`,
    hash: '',
    width: 10,
    height: 10,
    ownerName: '',
    ownerId: 0,
    ...extra,
  };
}

beforeEach(() => {
  vi.clearAllMocks();
  (globalThis as any).document = {
    baseURI: 'http://localhost/',
    body: { style: {} },
    querySelector: () => null,
    querySelectorAll: () => [],
    activeElement: null,
  };
  (globalThis as any).window = { innerWidth: 1400, scrollY: 0, scrollTo: () => {} };
  (globalThis as any).requestAnimationFrame = () => 0;
});

describe('lightbox media load arming', () => {
  it('arms the spinner for an item that has not been shown yet', () => {
    const store = makeStore([item(1), item(2)]);

    store.open(1);
    expect(store.loading).toBe(true);
    expect(store.hasMediaError()).toBe(false);

    store.open(0);
    expect(store.loading).toBe(true);
  });

  it('does not arm the spinner for an item whose load already failed', () => {
    const store = makeStore([item(1), item(2), item(3)]);
    store.open(1);
    store.onMediaError({ target: { src: 'http://localhost/v1/resource/view?id=2' } });
    expect(store.hasMediaError()).toBe(true);
    expect(store.loading).toBe(false);

    // The media element is reused and the URL is unchanged on a reopen, so no request goes
    // out and no load or error event arrives. Arming here left the spinner turning on top
    // of the error message for good.
    store.close();
    store.open(1);
    expect(store.loading).toBe(false);
    expect(store.hasMediaError()).toBe(true);

    // Navigating away still re-arms for an item that has not been shown yet.
    store.next();
    expect(store.loading).toBe(true);
    expect(store.hasMediaError()).toBe(false);

    // Coming back re-requests (the URL changed), but the item stays marked as failed until
    // the request answers, so there is no spinner over a message that is already up. A file
    // restored in the meantime clears the failure through the load event.
    store.prev();
    expect(store.loading).toBe(false);
    expect(store.hasMediaError()).toBe(true);
    store.onMediaLoaded({ target: { src: 'http://localhost/v1/resource/view?id=2' } });
    expect(store.hasMediaError()).toBe(false);
  });

  it('never reports loading and a media error for the same item at once', () => {
    const store = makeStore([item(1), item(2)]);

    store.open(0);
    store.onMediaError({ target: { src: 'http://localhost/v1/resource/view?id=1' } });
    expect(store.hasMediaError() && store.loading).toBe(false);

    // A stale event for the item that was left behind must not strand the flag either.
    store.next();
    store.onMediaError({ target: { src: 'http://localhost/v1/resource/view?id=1' } });
    expect(store.hasMediaError() && store.loading).toBe(false);
  });
});
