import { describe, expect, test, vi, beforeEach, afterEach } from 'vitest';
import { registerBulkSelectionStore, selectableItem } from './bulkSelection.js';

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
    test('repeated appearances select together and execute only once', () => {
        const store = makeStore();
        const first = card(10), second = card(10);
        store.registerOption(first);
        store.registerOption(second);
        store.select(10);
        expect([...store.selectedIds]).toEqual([10]);
        expect(first.el.checked).toBe(true);
        expect(second.el.checked).toBe(true);
        store.unregisterOption(first.el);
        expect(store.isSelected(10)).toBe(true);
        store.unregisterOption(second.el);
        expect(store.isSelected(10)).toBe(false);
    });

    test('equal IDs in different selections remain independent', () => {
        const resources = makeStore(), notes = makeStore();
        resources.registerOption(card(1)); notes.registerOption(card(1));
        resources.select(1);
        expect(notes.selectedIds.size).toBe(0);
        resources.reset();
        expect(resources.selectedIds.size).toBe(0);
        expect(resources.elements).toEqual([]);
    });
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

/**
 * The Select All row's emptiness guard.
 *
 * The row is rendered above the list, so Alpine evaluates its `x-show` before a
 * single card has run its own `init()` and registered. Answering that first
 * evaluation from the registry alone said "no rows", the registry filled a
 * moment later, and `x-collapse` animated the row open on page load — shoving
 * the list and the footer's pagination down 37px ~200ms after the page settled.
 */
describe('hasSelectableItems', () => {
    /** Rows are in the DOM from the first frame; only their registration is late. */
    function withRenderedRows(count: number) {
        const doc = (globalThis as any).document;
        doc.querySelector = (sel: string) =>
            sel === '[x-data^="selectableItem"]' && count > 0 ? {} : null;
    }

    test('is true before any card has registered, when rows are rendered', () => {
        const store = makeStore();
        withRenderedRows(50);

        expect(store.elements.length).toBe(0);
        expect(store.hasSelectableItems()).toBe(true);
    });

    test('stays true once the cards register, so the row never flips', () => {
        const store = makeStore();
        withRenderedRows(2);

        const before = store.hasSelectableItems();
        [10, 20].forEach(c => store.registerOption(card(c)));

        expect(before).toBe(true);
        expect(store.hasSelectableItems()).toBe(true);
    });

    test('is false on an empty list, so Select All is not offered (finding 68)', () => {
        const store = makeStore();
        withRenderedRows(0);

        expect(store.hasSelectableItems()).toBe(false);
    });

    test('answers from the registry once populated, without touching the DOM', () => {
        const store = makeStore();
        store.registerOption(card(10));
        (globalThis as any).document.querySelector = () => {
            throw new Error('should not query the DOM once the registry is populated');
        };

        expect(store.hasSelectableItems()).toBe(true);
    });
});


describe('selection across retained and repeated rows', () => {
    test('range endpoints are the clicked appearances rather than the last matching IDs', () => {
        const store = makeStore();
        const appearances = [card(10), card(20), card(30), card(10)];
        appearances.forEach(row => store.registerOption(row));
        store.toggle(10, appearances[0].el);
        store.selectUntil(20, appearances[1].el);
        expect([...store.selectedIds]).toEqual([10, 20]);
        store.selectedIds.clear();
        store.toggle(20, appearances[1].el);
        store.selectUntil(10, appearances[3].el);
        expect([...store.selectedIds]).toEqual([20, 30, 10]);
    });

    test('retained selectable rows refresh eligibility metadata after a morph', () => {
        const store = makeStore();
        const checkbox = card(10).el;
        const payload = {dataset: {entity: JSON.stringify({ID:10, resourceCategoryId:1})}};
        const root = {querySelector: (selector: string) => selector.includes('checkbox') ? checkbox : payload};
        let onMutation: () => void = () => {};
        const disconnect = vi.fn();
        vi.stubGlobal('MutationObserver', class {
            constructor(callback: () => void) { onMutation = callback; }
            observe() {}
            disconnect = disconnect;
        });
        const component = Object.assign(selectableItem({itemId:10}), {$root:root, $selection:store});
        component.init();
        store.select(10);
        expect(store.selectedEntities()[0].resourceCategoryId).toBe(1);
        payload.dataset.entity = JSON.stringify({ID:10, resourceCategoryId:2});
        onMutation();
        expect(store.selectedEntities()[0].resourceCategoryId).toBe(2);
        component.destroy();
        expect(disconnect).toHaveBeenCalledOnce();
    });
});
