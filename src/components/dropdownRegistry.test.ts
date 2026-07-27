import { readFileSync } from 'node:fs';
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import { selectorRegistry } from '../selector/selectorRegistry';
import { autocompleter } from './dropdown.js';

type LegacySelector = ReturnType<typeof autocompleter> & {
    $el: HTMLElement;
    $refs: Record<string, HTMLElement>;
    $watch: (name: string, callback: (...args: unknown[]) => void) => void;
    $nextTick: (callback: () => void) => void;
    $dispatch: ReturnType<typeof vi.fn>;
};

function createNode() {
    return {
        style: {},
        parentNode: null as ReturnType<typeof createNode> | null,
        setAttribute: vi.fn(),
        appendChild(child: ReturnType<typeof createNode>) {
            child.parentNode = this;
        },
        removeChild(child: ReturnType<typeof createNode>) {
            child.parentNode = null;
        },
    };
}

function createForm() {
    return {
        addEventListener: vi.fn(),
    } as unknown as HTMLFormElement;
}

function createSelector({
    form = createForm(),
    elName = 'tags',
    selectedResults = [{ ID: 1, Name: 'Alpha', MetaSchema: 'hydrated' }],
    url = '/v1/tags?limit=20',
}: {
    form?: HTMLFormElement | null;
    elName?: string;
    selectedResults?: Array<Record<string, unknown>>;
    url?: string;
} = {}) {
    const root = createNode() as unknown as HTMLElement & { closest: (selector: string) => HTMLFormElement | null };
    root.closest = vi.fn(() => form);
    const selector = autocompleter({
        selectedResults,
        max: 0,
        min: 0,
        ownerId: 0,
        url,
        elName,
    }) as LegacySelector;
    selector.$el = root;
    selector.$refs = {};
    selector.$watch = vi.fn();
    selector.$nextTick = (callback) => callback();
    selector.$dispatch = vi.fn();
    return { form, selector };
}

describe('legacy autocompleter selector registry integration', () => {
    beforeEach(() => {
        vi.stubGlobal('document', {
            createElement: vi.fn(() => createNode()),
            body: createNode(),
            querySelectorAll: vi.fn(() => []),
        });
        vi.stubGlobal('window', {
            addEventListener: vi.fn(),
            removeEventListener: vi.fn(),
        });
        vi.stubGlobal('fetch', vi.fn());
    });

    afterEach(() => {
        vi.restoreAllMocks();
        vi.unstubAllGlobals();
    });

    test('routes shared chip removal through the legacy command and restores combobox focus', () => {
        const markup = readFileSync(
            new URL('../../templates/partials/form/formParts/dropDownSelectedResults.tpl', import.meta.url),
            'utf8',
        );

        expect(markup).toContain('@click="removeItem(result)"');
        expect(markup).toContain('removeItem(result); $nextTick(() => $refs.autocompleter?.focus())');
        expect(markup).not.toContain('selectedResults.splice');
    });

    test('removeItem calls the compatibility callback exactly once', () => {
        const onRemove = vi.fn();
        const selector = autocompleter({
            selectedResults: [{ ID: 1, Name: 'Alpha' }],
            max: 0,
            min: 0,
            ownerId: 0,
            url: '/v1/tags',
            elName: 'tags',
            onRemove,
        });

        selector.removeItem({ ID: 1, Name: 'Alpha' });

        expect(selector.selectedResults).toEqual([]);
        expect(onRemove).toHaveBeenCalledTimes(1);
        expect(onRemove).toHaveBeenCalledWith({ ID: 1, Name: 'Alpha' });
    });

    test('registers by owning form and field name, then unregisters on destruction', () => {
        const { form, selector } = createSelector();

        selector.init();
        const handle = selectorRegistry.get(form!, 'tags');

        expect(handle?.getRawValues()).toEqual([
            { ID: 1, Name: 'Alpha', MetaSchema: 'hydrated' },
        ]);

        selector.destroy();
        expect(selectorRegistry.get(form!, 'tags')).toBeUndefined();
    });

    test('does not register without both an owning form and field name', () => {
        const withoutForm = createSelector({ form: null });
        const withoutName = createSelector({ elName: '' });

        withoutForm.selector.init();
        withoutName.selector.init();

        expect(withoutForm.form).toBeNull();
        expect(selectorRegistry.get(withoutName.form!, '')).toBeUndefined();

        withoutForm.selector.destroy();
        withoutName.selector.destroy();
    });

    test('adapts non-silent replacement and the legacy silent reset behavior', () => {
        const { form, selector } = createSelector();
        selector.init();
        const handle = selectorRegistry.get(form!, 'tags')!;

        handle.replaceRawValues([{ ID: 2, Name: 'Beta' }]);
        expect(selector.selectedResults).toEqual([{ ID: 2, Name: 'Beta' }]);
        expect(selector._suppressNextAnnounce).toBe(false);

        selector.resetSelectedResults([{ ID: 3, Name: 'Gamma' }]);
        expect(selector.selectedResults).toEqual([{ ID: 3, Name: 'Gamma' }]);
        expect(selector._suppressNextAnnounce).toBe(true);

        handle.replaceRawValues([{ ID: 4, Name: 'Delta' }], { silent: true });
        expect(selector.selectedResults).toEqual([{ ID: 4, Name: 'Delta' }]);
        expect(selector._suppressNextAnnounce).toBe(true);

        selector.destroy();
    });

    test('preserves hydrated selected values when replacing by key', () => {
        const { form, selector } = createSelector();
        selector.init();
        const handle = selectorRegistry.get(form!, 'tags')!;

        handle.replaceByKeys(['1', '2'], { silent: true });

        expect(selector.selectedResults).toEqual([
            { ID: 1, Name: 'Alpha', MetaSchema: 'hydrated' },
            { ID: 2, Name: '#2' },
        ]);
        expect(selector._suppressNextAnnounce).toBe(true);

        selector.destroy();
    });

    test('resolves every exact label before atomically replacing the selection', async () => {
        const { form, selector } = createSelector();
        const fetchMock = vi.mocked(fetch);
        fetchMock
            .mockResolvedValueOnce({
                ok: true,
                json: async () => [{ ID: 2, Name: 'Beta' }, { ID: 20, Name: 'Betamax' }],
            } as Response)
            .mockResolvedValueOnce({
                ok: true,
                json: async () => [{ ID: 3, Name: 'GAMMA' }],
            } as Response);
        selector.init();
        const handle = selectorRegistry.get(form!, 'tags')!;

        await expect(handle.resolveExactLabels(['beta', 'gamma'], { silent: true }))
            .resolves.toBe(true);
        expect(fetchMock).toHaveBeenNthCalledWith(1, '/v1/tags?limit=20&Name=beta');
        expect(fetchMock).toHaveBeenNthCalledWith(2, '/v1/tags?limit=20&Name=gamma');
        expect(selector.selectedResults).toEqual([
            { ID: 2, Name: 'Beta' },
            { ID: 3, Name: 'GAMMA' },
        ]);
        expect(selector._suppressNextAnnounce).toBe(true);

        selector.destroy();
    });

    test('leaves the current selection unchanged when an exact label cannot be resolved', async () => {
        const { form, selector } = createSelector();
        vi.mocked(fetch).mockResolvedValue({ ok: true, json: async () => [] } as unknown as Response);
        selector.init();
        const handle = selectorRegistry.get(form!, 'tags')!;

        await expect(handle.resolveExactLabels(['missing'])).resolves.toBe(false);
        expect(selector.selectedResults).toEqual([
            { ID: 1, Name: 'Alpha', MetaSchema: 'hydrated' },
        ]);

        selector.destroy();
    });
});
