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
import { versionPanelMethods, versionPanelState } from './versionPanel.js';
import { navigationMethods, navigationState } from './navigation.js';
import { cropPanelMethods } from './cropPanel.js';

function jsonResponse(body: unknown) {
  return { ok: true, status: 200, json: async () => body };
}

function makeStore(items: any[] = []) {
  const store: any = {
    ...navigationState,
    ...versionPanelState,
    ...versionPanelMethods,
    versionsCache: new Map(),
    ...editPanelState,
    ...quickTagPanelState,
    ...navigationMethods,
    ...editPanelMethods,
    ...quickTagPanelMethods,
    ...cropPanelMethods,
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


const historical = { id: 11, resourceId: 1, versionNumber: 1, contentType: 'image/png', width: 80, height: 40, hash: 'historical', createdAt: '2026-01-02T00:00:00Z' };
const current = { ...historical, id: 12, versionNumber: 2, hash: 'current' };

function versionResponse(versions = [current, historical]) {
  fetchMock.abortableFetch.mockReturnValue({ abort: vi.fn(), ready: Promise.resolve(jsonResponse({
    resource: { ID: 1, Name: 'Updated name', ContentType: 'image/jpeg', Width: 200, Height: 100, Hash: 'current', currentVersionId: 12 }, versions,
  })) });
}

describe('Displayed Version', () => {
  it('preserves historical pixels and dimensions while revalidating resource metadata', async () => {
    const store = makeStore([item(1)]);
    versionResponse();
    await store.openVersionPanel();
    store.selectVersion(historical);
    await store.fetchResourceDetails(undefined, true);
    expect(store.getCurrentItem()).toMatchObject({ id: 1, name: 'Updated name', width: 80, height: 40, contentType: 'image/png', viewUrl: '/v1/resource/version/file?versionId=11' });
    expect(store.isHistoricalVersion()).toBe(true);
    expect(store.versionEntries()).toHaveLength(2);
  });
});

describe('Version selection lifecycle', () => {
  it('falls back and announces when the Displayed Version was deleted elsewhere', async () => {
    const store = makeStore([item(1)]);
    versionResponse();
    await store.openVersionPanel();
    store.selectVersion(historical);
    versionResponse([current]);
    await store.fetchResourceDetails(undefined, true);
    expect(store.isHistoricalVersion()).toBe(false);
    expect(store.getCurrentItem().viewUrl).toBe('/v1/resource/view?id=1&v=current');
    expect(store.announce).toHaveBeenLastCalledWith('Displayed version is no longer available. Showing current version');
  });

  it('resets the just-left Resource even when the version panel is closed', async () => {
    const store = makeStore([item(1), item(2)]);
    versionResponse();
    await store.openVersionPanel();
    store.selectVersion(historical);
    store.versionPanelOpen = false;
    store.currentIndex = 1;
    await store.onResourceChange();
    store.currentIndex = 0;
    expect(store.isHistoricalVersion()).toBe(false);
    expect(store.getCurrentItem().viewUrl).toBe('/v1/resource/view?id=1&v=current');
  });

  it('lists non-displayable Versions and refuses their selection', async () => {
    const store = makeStore([item(1)]);
    const pdf = { ...historical, id: 13, contentType: 'application/pdf' };
    versionResponse([current, pdf, historical]);
    await store.openVersionPanel();
    expect(store.versionEntries()).toHaveLength(3);
    expect(store.isVersionDisplayable(pdf)).toBe(false);
    store.selectVersion(pdf);
    expect(store.getCurrentItem().viewUrl).toBe('/v1/resource/view?id=1&v=current');
    expect(store.isVersionDisplayable({ ...historical, contentType: 'image/avif', width: 0, height: 0 })).toBe(true);
  });

  it('preserves history across a DOM rescan and resets to the refreshed Current Version', async () => {
    const store = makeStore([item(1)]);
    versionResponse();
    await store.openVersionPanel();
    store.selectVersion(historical);
    store._reconcileOpenItems([item(1, { hash: 'new', viewUrl: '/v1/resource/view?id=1&v=new', width: 600, height: 300 })]);
    expect(store.getCurrentItem().viewUrl).toBe('/v1/resource/version/file?versionId=11');
    store.resetDisplayedVersion();
    expect(store.getCurrentItem()).toMatchObject({ viewUrl: '/v1/resource/view?id=1&v=new', width: 600 });
  });

  it('rejects a version list fetched across a Resource write', async () => {
    const store = makeStore([item(1)]);
    versionResponse();
    await store.openVersionPanel();
    let release: (value: unknown) => void = () => {};
    fetchMock.abortableFetch.mockReturnValue({ abort: vi.fn(), ready: new Promise(resolve => { release = resolve; }) });
    const pending = store.fetchResourceDetails(undefined, true);
    const generation = store._beginDetailsWrite(1);
    store._settleDetailsCache(1, generation, { ID: 1, Name: 'Written' });
    store._endDetailsWrite(1);
    release(jsonResponse({ resource: { ID: 1, Name: 'Stale' }, versions: [] }));
    await pending;
    expect(store.versionEntries()).toHaveLength(2);
  });
});


it('restores Current Version media and clears version state on close', async () => {
  const store = makeStore([item(1)]);
  versionResponse();
  await store.openVersionPanel();
  store.selectVersion(historical);
  store.close();
  expect(store.getCurrentItem().viewUrl).toBe('/v1/resource/view?id=1&v=current');
  expect(store.versionsCache.size).toBe(0);
  expect(store.versionPanelOpen).toBe(false);
});

it('a rescan that omits the displayed Resource preserves its reset target', async () => {
  const store = makeStore([item(1)]);
  versionResponse();
  await store.openVersionPanel();
  store.selectVersion(historical);
  store._reconcileOpenItems([]);
  store.resetDisplayedVersion();
  expect(store.getCurrentItem().viewUrl).toBe('/v1/resource/view?id=1&v=current');
});

it('opens a Resource Version as one Resource and announces after the existing details fetch', async () => {
  const store = makeStore([item(2)]);
  versionResponse();
  await store.openResourceAtVersion(1, 11, 'image/png', 80, 40);
  expect(store.items).toHaveLength(1);
  expect(store.getCurrentItem()).toMatchObject({ id: 1, width: 80, height: 40 });
  expect(store.versionPanelOpen).toBe(true);
  expect(fetchMock.abortableFetch).toHaveBeenCalledTimes(1);
  expect(store.announce).toHaveBeenLastCalledWith('Showing version 1 of 2, uploaded Jan 02 2026');
  store.close();
  expect(store.items.map((i: any) => i.id)).toEqual([2]);
});

it('uses Current Version media for an unmigrated Resource with a virtual v1', async () => {
  const store = makeStore([]);
  const virtual = { ...historical, id: 0 };
  fetchMock.abortableFetch.mockReturnValue({ abort: vi.fn(), ready: Promise.resolve(jsonResponse({ resource: { ID: 1, Hash: 'current', ContentType: 'image/png', currentVersionId: null }, versions: [virtual] })) });
  await store.openResourceAtVersion(1, 0, 'image/png', 80, 40);
  expect(store.getCurrentItem().viewUrl).toBe('/v1/resource/view?id=1&v=current');
  expect(store.displayedVersionId()).toBe(0);
  expect(store.isHistoricalVersion()).toBe(false);
  expect(store.versionThumbnailUrl(virtual, 64)).toBe('/v1/resource/preview?id=1&height=64&v=historical');
});

it('opens history with one details request when Edit Tags was already open', async () => {
  const store = makeStore([]);
  store.quickTagPanelOpen = true;
  versionResponse();
  await store.openResourceAtVersion(1, 11, 'image/png', 80, 40);
  expect(fetchMock.abortableFetch.mock.calls.filter(([url]) => url.startsWith('/resource.json'))).toHaveLength(1);
});

it('keeps the shared details request alive when Info closes above an open version panel', () => {
  const store = makeStore([item(1)]);
  store.versionPanelOpen = true;
  store.editPanelOpen = true;
  store.resourceDetails = { ID: 1, Name: 'Resource' };
  const abort = vi.fn();
  store.detailsAborter = abort;
  store.closeEditPanel();
  expect(abort).not.toHaveBeenCalled();
  expect(store.displayDetails()).toMatchObject({ ID: 1 });
});

it('opens the explicitly requested Version after Current changes between viewer sessions', async () => {
  const store = makeStore([]);
  versionResponse();
  await store.openResourceAtVersion(1, 12, 'image/png', 80, 40);
  store.close();
  const newest = { ...current, id: 13, versionNumber: 3, hash: 'newest' };
  fetchMock.abortableFetch.mockReturnValue({ abort: vi.fn(), ready: Promise.resolve(jsonResponse({
    resource: { ID: 1, ContentType: 'image/png', Hash: 'newest', currentVersionId: 13 },
    versions: [newest, current, historical],
  })) });
  await store.openResourceAtVersion(1, 12, 'image/png', 80, 40);
  expect(store.getCurrentItem().viewUrl).toBe('/v1/resource/version/file?versionId=12');
  expect(store.isHistoricalVersion()).toBe(true);
});

it.each(['versions', 'info', 'tags'])('refreshes versions after editing Current with %s open and can select the former Current', async (panel) => {
  const store = makeStore([item(1)]);
  versionResponse();
  await store.openVersionPanel();
  store.editPanelOpen = panel === 'info';
  store.quickTagPanelOpen = panel === 'tags';
  const rotated = { ...current, id: 13, versionNumber: 3, hash: 'rotated' };
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse({
    resource: { ID: 1, ContentType: 'image/jpeg', Hash: 'rotated', currentVersionId: 13 },
    versions: [rotated, current, historical],
  })));
  try {
    await store.refreshCurrentItem(1);
    expect(store.versionEntries()).toHaveLength(3);
    expect(store.displayedVersionId()).toBe(13);
    store.selectVersion(current);
    expect(store.isHistoricalVersion()).toBe(true);
    expect(store.getCurrentItem().viewUrl).toBe('/v1/resource/version/file?versionId=12');
  } finally {
    vi.unstubAllGlobals();
  }
});
