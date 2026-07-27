import { describe, expect, test, vi } from 'vitest';
import {
    observeSelectorField,
    SelectorRegistry,
    selectorRegistry,
    type SelectorFieldChange,
    type SelectorIntegrationHandle,
    type SelectorReplacementOptions,
    type SelectorValue,
} from './selectorRegistry';

function form(): HTMLFormElement {
    return {} as HTMLFormElement;
}

function change(overrides: Partial<SelectorFieldChange> = {}): SelectorFieldChange {
    return { values: [], added: [], removed: [], ...overrides };
}

function handle(values: SelectorValue[] = []): SelectorIntegrationHandle {
    return {
        getRawValues: () => values,
        replaceRawValues: vi.fn(),
        replaceByKeys: vi.fn(),
        resolveExactLabels: vi.fn(async () => true),
    };
}

describe('SelectorRegistry', () => {
    test('registers and looks up a selector by owning form and field name', () => {
        const registry = new SelectorRegistry();
        const owner = form();
        const selector = handle();

        registry.register(owner, 'tags', selector);

        expect(registry.get(owner, 'tags')).toBe(selector);
        expect(registry.get(owner, 'groups')).toBeUndefined();
    });

    test('keeps duplicate field names isolated between forms', () => {
        const registry = new SelectorRegistry();
        const firstForm = form();
        const secondForm = form();
        const firstSelector = handle([{ ID: 1, Name: 'first' }]);
        const secondSelector = handle([{ ID: 2, Name: 'second' }]);

        registry.register(firstForm, 'tags', firstSelector);
        registry.register(secondForm, 'tags', secondSelector);

        expect(registry.get(firstForm, 'tags')).toBe(firstSelector);
        expect(registry.get(secondForm, 'tags')).toBe(secondSelector);
    });

    test('replacement registration is not removed by stale cleanup', () => {
        const registry = new SelectorRegistry();
        const owner = form();
        const first = handle();
        const replacement = handle();
        const cleanupFirst = registry.register(owner, 'tags', first);
        const cleanupReplacement = registry.register(owner, 'tags', replacement);

        expect(registry.get(owner, 'tags')).toBe(replacement);

        cleanupFirst();
        expect(registry.get(owner, 'tags')).toBe(replacement);

        cleanupReplacement();
        expect(registry.get(owner, 'tags')).toBeUndefined();
    });

    test('cleanup only removes its own registration and is idempotent', () => {
        const registry = new SelectorRegistry();
        const owner = form();
        const cleanup = registry.register(owner, 'tags', handle());

        cleanup();
        cleanup();

        expect(registry.get(owner, 'tags')).toBeUndefined();
    });

    test('exposes exact-label resolution and silent replacement through an in-memory handle', async () => {
        const registry = new SelectorRegistry();
        const owner = form();
        const catalog: SelectorValue[] = [
            { ID: 1, Name: 'Alpha' },
            { ID: 2, Name: 'Beta' },
        ];
        let selected: SelectorValue[] = [{ ID: 1, Name: 'Alpha', MetaSchema: 'hydrated' }];
        const replacements: SelectorReplacementOptions[] = [];
        const inMemoryHandle: SelectorIntegrationHandle = {
            getRawValues: () => selected,
            replaceRawValues(values, options = {}) {
                replacements.push(options);
                selected = [...values];
            },
            replaceByKeys(keys, options = {}) {
                replacements.push(options);
                const existing = new Map(selected.map((value) => [String(value.ID), value]));
                selected = keys.map((key) => existing.get(String(key)) ?? { ID: key, Name: `#${key}` });
            },
            async resolveExactLabels(labels, options = {}) {
                const matches = labels.map((label) => catalog.find(
                    (value) => value.Name?.toLowerCase() === label.toLowerCase(),
                ));
                if (matches.some((value) => !value)) return false;
                this.replaceRawValues(matches as SelectorValue[], options);
                return true;
            },
        };
        registry.register(owner, 'tags', inMemoryHandle);
        const selector = registry.get(owner, 'tags');

        expect(await selector?.resolveExactLabels(['beta'], { silent: true })).toBe(true);
        expect(selector?.getRawValues()).toEqual([{ ID: 2, Name: 'Beta' }]);
        expect(replacements).toEqual([{ silent: true }]);

        selector?.replaceRawValues([{ ID: 1, Name: 'Alpha', MetaSchema: 'hydrated' }]);
        selector?.replaceByKeys([1], { silent: true });
        expect(selector?.getRawValues()).toEqual([
            { ID: 1, Name: 'Alpha', MetaSchema: 'hydrated' },
        ]);
        expect(replacements.at(-1)).toEqual({ silent: true });
    });
});

describe('SelectorRegistry field observation', () => {
    test('delivers a field change to observers of that form and field only', () => {
        const registry = new SelectorRegistry();
        const owner = form();
        const other = form();
        const observed: SelectorFieldChange[] = [];
        const wrongField = vi.fn();
        const wrongForm = vi.fn();

        registry.observe(owner, 'categoryId', (next) => observed.push(next));
        registry.observe(owner, 'tags', wrongField);
        registry.observe(other, 'categoryId', wrongForm);

        const emitted = change({
            values: [{ ID: 2, Name: 'Beta' }],
            added: [{ ID: 2, Name: 'Beta' }],
            removed: [{ ID: 1, Name: 'Alpha' }],
        });
        registry.notifyChange(owner, 'categoryId', emitted);

        expect(observed).toEqual([emitted]);
        expect(wrongField).not.toHaveBeenCalled();
        expect(wrongForm).not.toHaveBeenCalled();
    });

    test('observation does not depend on the selector registering first', () => {
        const registry = new SelectorRegistry();
        const owner = form();
        const observer = vi.fn();

        registry.observe(owner, 'categoryId', observer);
        registry.register(owner, 'categoryId', handle());
        registry.notifyChange(owner, 'categoryId', change());

        expect(observer).toHaveBeenCalledTimes(1);
    });

    test('notifying a field nobody observes is a no-op', () => {
        const registry = new SelectorRegistry();

        expect(() => registry.notifyChange(form(), 'categoryId', change())).not.toThrow();
    });

    test('cleanup stops delivery and is idempotent', () => {
        const registry = new SelectorRegistry();
        const owner = form();
        const first = vi.fn();
        const second = vi.fn();
        const stopFirst = registry.observe(owner, 'categoryId', first);
        registry.observe(owner, 'categoryId', second);

        stopFirst();
        stopFirst();
        registry.notifyChange(owner, 'categoryId', change());

        expect(first).not.toHaveBeenCalled();
        expect(second).toHaveBeenCalledTimes(1);
    });

    test('one failing observer does not starve the others', () => {
        const registry = new SelectorRegistry();
        const owner = form();
        const survivor = vi.fn();
        registry.observe(owner, 'categoryId', () => { throw new Error('observer blew up'); });
        registry.observe(owner, 'categoryId', survivor);

        expect(() => registry.notifyChange(owner, 'categoryId', change())).not.toThrow();
        expect(survivor).toHaveBeenCalledTimes(1);
    });

    test('observeSelectorField resolves the owning form of a DOM consumer', () => {
        const owner = form();
        const element = { closest: vi.fn(() => owner) } as unknown as Element;
        const observer = vi.fn();

        const stop = observeSelectorField(element, 'categoryId', observer);
        selectorRegistry.notifyChange(owner, 'categoryId', change());
        stop();
        selectorRegistry.notifyChange(owner, 'categoryId', change());

        expect(element.closest).toHaveBeenCalledWith('form');
        expect(observer).toHaveBeenCalledTimes(1);
    });

    test('observeSelectorField returns an inert cleanup when there is no owning form', () => {
        const element = { closest: vi.fn(() => null) } as unknown as Element;
        const observer = vi.fn();

        const stop = observeSelectorField(element, 'categoryId', observer);

        expect(stop).toBeTypeOf('function');
        expect(() => stop()).not.toThrow();
        expect(observer).not.toHaveBeenCalled();
    });
});
