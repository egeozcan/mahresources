import { beforeEach, describe, expect, test, vi } from 'vitest';
import { registerEntityPickerStore } from './entityPicker.js';

interface FilterStore {
    config: unknown;
    filterValues: Record<string, unknown>;
    loadResults: () => void;
    applyFilterChange: (key: string, multiple: boolean, change: unknown) => void;
    setFilter: (key: string, value: unknown) => void;
}

function option(id: number, name: string) {
    return { key: id, label: name, raw: { ID: id, Name: name } };
}

function createStore(): FilterStore {
    let store: FilterStore | null = null;
    registerEntityPickerStore({
        store: (_name: string, value: FilterStore) => {
            store = value;
        },
    });
    const created = store as FilterStore | null;
    if (!created) throw new Error('entityPicker store was not registered');
    created.loadResults = vi.fn();
    return created;
}

describe('entity picker filter changes', () => {
    let store: FilterStore;

    beforeEach(() => {
        store = createStore();
    });

    test('a single-value filter takes the replacement selection in one reload', () => {
        store.applyFilterChange('group', false, {
            previous: [option(1, 'Alpha')],
            current: [option(2, 'Beta')],
            removed: [option(1, 'Alpha')],
            added: [option(2, 'Beta')],
            reason: 'select',
        });

        expect(store.filterValues).toEqual({ group: 2 });
        expect(store.loadResults).toHaveBeenCalledTimes(1);
    });

    test('clearing a single-value filter removes the key', () => {
        store.setFilter('group', 5);
        store.applyFilterChange('group', false, {
            previous: [option(5, 'Alpha')],
            current: [],
            removed: [option(5, 'Alpha')],
            added: [],
            reason: 'remove',
        });

        expect(store.filterValues).toEqual({});
    });

    test('a multi-value filter accumulates additions and drops removals', () => {
        store.applyFilterChange('tags', true, {
            previous: [],
            current: [option(1, 'Alpha')],
            removed: [],
            added: [option(1, 'Alpha')],
            reason: 'select',
        });
        store.applyFilterChange('tags', true, {
            previous: [option(1, 'Alpha')],
            current: [option(1, 'Alpha'), option(2, 'Beta')],
            removed: [],
            added: [option(2, 'Beta')],
            reason: 'select',
        });
        expect(store.filterValues).toEqual({ tags: [1, 2] });

        store.applyFilterChange('tags', true, {
            previous: [option(1, 'Alpha'), option(2, 'Beta')],
            current: [option(2, 'Beta')],
            removed: [option(1, 'Alpha')],
            added: [],
            reason: 'remove',
        });
        expect(store.filterValues).toEqual({ tags: [2] });
    });
});
