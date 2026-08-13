import { describe, expect, test, vi, beforeEach, afterEach } from 'vitest';
import { registerBulkSelectionStore } from './bulkSelection.js';

/**
 * The shared list-page selection store.
 *
 * Every entity list registers its cards here, so a defect in the range anchor is
 * a defect on /notes, /tags, /groups, /resources, /queries and /downloads at
 * once — which is why it is pinned here rather than in any one page's tests.
 */

function makeStore() {
    let store: any;
    // `any`: registerBulkSelectionStore only ever calls Alpine.store(name, value).
    registerBulkSelectionStore({ store: (_name: string, value: any) => { store = value; } } as any);
    store.init();
    return store;
}

/** A card checkbox, as `selectableItem` hands one to the store. */
function card(id: number, checked = false) {
    return { itemId: id, el: { checked, setAttribute() {}, removeAttribute() {} } };
}

beforeEach(() => {
    // createLiveRegion runs in the store's init(); this is the smallest DOM that
    // lets it build its announcer without a full jsdom environment.
    vi.stubGlobal('document', {
        body: { appendChild() {} },
        createElement: () => ({ setAttribute() {}, style: {}, textContent: '', parentNode: null }),
    });
});

afterEach(() => {
    vi.unstubAllGlobals();
});

describe('shift-range selection', () => {
    test('registering the page does not become the reader\'s last click', () => {
        const store = makeStore();
        [10, 20, 30].forEach(c => store.registerOption(card(c)));

        // The first interaction on the page is a shift-click on the first card.
        // With the anchor left where registration put it — the *last* card — this
        // selected all three, which on /downloads is one keystroke from deleting
        // every row on the page.
        store.selectUntil(10);

        expect([...store.selectedIds]).toEqual([10]);
    });

    test('a range still runs from the previous click to the shift-click', () => {
        const store = makeStore();
        [10, 20, 30, 40].forEach(c => store.registerOption(card(c)));

        store.toggle(20);
        store.selectUntil(40);

        expect([...store.selectedIds].sort((a, b) => a - b)).toEqual([20, 30, 40]);
    });

    test('a card that arrives already checked is still selected', () => {
        // The anchor is restored around registration, but the selection it syncs is
        // not: a pre-checked card must still land in the set.
        const store = makeStore();
        store.registerOption(card(10));
        store.registerOption(card(20, true));

        expect([...store.selectedIds]).toEqual([20]);
    });
});
