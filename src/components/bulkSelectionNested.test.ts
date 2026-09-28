// @vitest-environment happy-dom
import { beforeAll, expect, test, vi } from 'vitest';
import Alpine from 'alpinejs';
import { registerBulkSelectionStore, selectableItem } from './bulkSelection.js';

// The Job Center's cards sit inside the list's own component, so each card's
// component is one scope on a stack with the list's beneath it. A property a
// card assigns without declaring is written to the outermost scope that
// Alpine's merged proxy finds, which is the list's: every card then shares the
// last one's checkbox, and removing a card unregisters another card's.
beforeAll(() => {
    // Alpine made its own observer when it was imported. The one each card
    // watches its payload with is stubbed: happy-dom's has private fields that
    // Alpine's reactive proxy cannot reach, which a browser's native one does not.
    vi.stubGlobal('MutationObserver', class { observe() {} disconnect() {} });
    registerBulkSelectionStore(Alpine);
    Alpine.data('selectableItem', selectableItem);
    Alpine.data('outerList', () => ({}));
    Alpine.start();
});

function cardMarkup(id: string) {
    return `<article data-job-id="${id}" x-data="selectableItem({ itemId: '${id}' })">
        <input type="checkbox" :checked="selected() ? 'checked' : null" x-bind="events">
        <div data-entity='{"id":"${id}"}'></div>
    </article>`;
}

test('a card nested in another component unregisters its own checkbox when it leaves', async () => {
    document.body.innerHTML = `<div x-data="outerList"><section>${['a', 'b', 'c', 'd'].map(cardMarkup).join('')}</section></div>`;
    await vi.waitFor(() => expect(Alpine.store('bulkSelection').elements).toHaveLength(4));
    const store = Alpine.store('bulkSelection') as any;
    store.select('a');
    store.select('b');
    store.select('d');

    document.querySelector('[data-job-id="a"]')!.remove();
    document.querySelector('[data-job-id="b"]')!.remove();

    await vi.waitFor(() => expect(store.elements.map((option: any) => option.itemId)).toEqual(['c', 'd']));
    expect([...store.selectedIds]).toEqual(['d']);
    expect(store.elements.every((option: any) => option.el.isConnected)).toBe(true);
});
