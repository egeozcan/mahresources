import { beforeEach, describe, expect, it, vi } from 'vitest';

const userSettingsMock = vi.hoisted(() => ({
  whenLoaded: vi.fn(),
  get: vi.fn(),
  set: vi.fn(),
}));

vi.mock('../../userSettings.js', () => userSettingsMock);

import { quickTagPanelMethods, quickTagPanelState } from './quickTagPanel.js';

function emptySlots() {
  return [
    Array(9).fill(null),
    Array(9).fill(null),
    Array(9).fill(null),
    Array(9).fill(null),
  ];
}

function makeStore() {
  return {
    ...quickTagPanelState,
    ...quickTagPanelMethods,
    quickSlots: emptySlots(),
    recentTags: Array(9).fill(null),
    _suggestedCache: new Map(),
    announce: vi.fn(),
  } as any;
}

describe('quickTagPanel settings hydration', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    userSettingsMock.whenLoaded.mockResolvedValue(undefined);
    (globalThis as any).document = { querySelectorAll: () => [] };
  });

  it('keeps quick-slot edits made before server settings hydrate', async () => {
    const store = makeStore();
    const serverSlots = emptySlots();
    serverSlots[0][1] = [{ id: 2, name: 'server' }];
    userSettingsMock.get.mockReturnValue({
      version: 3,
      quickSlots: serverSlots,
      recentTags: Array(9).fill(null),
      flowMode: false,
    });

    store.addTagToSlot(0, { ID: 1, Name: 'local' });
    await store._loadQuickTagsFromStorage();

    expect(store.quickSlots[0][0]).toEqual([{ id: 1, name: 'local' }]);
    expect(store.quickSlots[0][1]).toEqual([{ id: 2, name: 'server' }]);
    expect(userSettingsMock.set).toHaveBeenCalledWith(
      'quickTags',
      expect.objectContaining({
        quickSlots: expect.arrayContaining([
          expect.arrayContaining([
            [{ id: 1, name: 'local' }],
            [{ id: 2, name: 'server' }],
          ]),
        ]),
      }),
    );
  });

  it('merges pre-hydration edits with tags already saved in the same slot', async () => {
    const store = makeStore();
    const serverSlots = emptySlots();
    serverSlots[0][0] = [{ id: 2, name: 'server' }];
    userSettingsMock.get.mockReturnValue({
      version: 3,
      quickSlots: serverSlots,
      recentTags: Array(9).fill(null),
      flowMode: false,
    });

    store.addTagToSlot(0, { ID: 1, Name: 'local' });
    await store._loadQuickTagsFromStorage();

    expect(store.quickSlots[0][0]).toEqual([
      { id: 2, name: 'server' },
      { id: 1, name: 'local' },
    ]);
  });
});
