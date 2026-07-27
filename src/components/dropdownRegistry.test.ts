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
        removeEventListener: vi.fn(),
    } as unknown as HTMLFormElement;
}

function mountSelector(selector: LegacySelector, form: HTMLFormElement | null) {
    const root = createNode() as unknown as HTMLElement & { closest: (selector: string) => HTMLFormElement | null };
    root.closest = vi.fn(() => form);
    selector.$el = root;
    selector.$refs = {};
    selector.$watch = vi.fn();
    selector.$nextTick = (callback) => callback ? callback() : Promise.resolve();
    selector.$dispatch = vi.fn();
    return { form, selector };
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
    return mountSelector(autocompleter({
        selectedResults,
        max: 0,
        min: 0,
        ownerId: 0,
        url,
        elName,
    }) as LegacySelector, form);
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
        expect(markup).toContain(
            "removeItem(result); $nextTick(() => root.querySelector('input[role=combobox]')?.focus())",
        );
        expect(markup).not.toContain('selectedResults.splice');
    });

    test('keeps mouse active state in the core command path for shared result rows', () => {
        const markup = readFileSync(
            new URL('../../templates/partials/form/formParts/dropDownResults.tpl', import.meta.url),
            'utf8',
        );

        expect(markup).toContain('@mouseover="setActiveIndex(index)"');
        expect(markup).toContain('@mousedown="startSelecting(); setActiveIndex(index); {{ action }}($event)"');
        expect(markup).toContain('@mousedown="startSelecting(); setActiveIndex(results.length); {{ action }}($event)"');
        expect(markup).not.toContain('selectedIndex = index');
    });

    test('routes chip removal through the initialized core and calls the compatibility callback once', () => {
        const onRemove = vi.fn();
        const { selector } = mountSelector(autocompleter({
            selectedResults: [{ ID: 1, Name: 'Alpha' }],
            max: 0,
            min: 0,
            ownerId: 0,
            url: '/v1/tags',
            elName: 'tags',
            onRemove,
        }) as LegacySelector, createForm());
        selector.init();

        selector.removeItem({ ID: 1, Name: 'Alpha' });

        expect(selector.selectedResults).toEqual([]);
        expect(onRemove).toHaveBeenCalledTimes(1);
        expect(onRemove).toHaveBeenCalledWith({ ID: 1, Name: 'Alpha' });
        selector.destroy();
    });

    test('mirrors normalized search options, status, and errors from the core', async () => {
        vi.useFakeTimers();
        vi.mocked(fetch)
            .mockResolvedValueOnce({
                ok: true,
                json: async () => [{ ID: 1, Name: 'Alpha' }],
            } as Response)
            .mockRejectedValueOnce(new Error('lookup unavailable'));
        const { selector } = createSelector({ selectedResults: [] });
        selector.init();

        selector._core.dispatch({ type: 'set-query', query: 'alpha' });
        expect(selector).toMatchObject({
            query: 'alpha',
            searchStatus: 'loading',
            searchError: null,
            results: [],
        });
        await vi.advanceTimersByTimeAsync(200);
        await Promise.resolve();
        expect(selector).toMatchObject({
            searchStatus: 'success',
            searchError: null,
            results: [{ ID: 1, Name: 'Alpha' }],
        });

        selector._core.dispatch({ type: 'set-query', query: 'broken' });
        await vi.advanceTimersByTimeAsync(200);
        await Promise.resolve();
        expect(selector).toMatchObject({
            searchStatus: 'error',
            searchError: expect.objectContaining({ message: 'lookup unavailable' }),
        });
        selector.destroy();
        vi.useRealTimers();
    });

    test('reevaluates dynamic adapter parameters for every core-owned HTTP search', async () => {
        vi.useFakeTimers();
        vi.mocked(fetch).mockResolvedValue({ ok: true, json: async () => [] } as Response);
        const { selector } = createSelector({ selectedResults: [], url: '/v1/tags' });
        selector.init();

        selector.ownerId = 7;
        selector._core.dispatch({ type: 'set-query', query: 'first' });
        await vi.advanceTimersByTimeAsync(200);
        selector.ownerId = 8;
        selector._core.dispatch({ type: 'set-query', query: 'second' });
        await vi.advanceTimersByTimeAsync(200);

        expect(fetch).toHaveBeenNthCalledWith(
            1,
            '/v1/tags?ownerId=7&name=first',
            expect.objectContaining({ signal: expect.any(AbortSignal) }),
        );
        expect(fetch).toHaveBeenNthCalledWith(
            2,
            '/v1/tags?ownerId=8&name=second',
            expect.objectContaining({ signal: expect.any(AbortSignal) }),
        );
        selector.destroy();
        vi.useRealTimers();
    });

    test('clears token input before core creation starts and queues a virtual row while creation is loading', async () => {
        vi.useFakeTimers();
        const postedInputValues: string[] = [];
        let resolveFirstCreate!: (response: Response) => void;
        vi.mocked(fetch).mockImplementation((request, init) => {
            if (init?.method === 'POST') {
                postedInputValues.push((input as HTMLInputElement).value);
                return new Promise<Response>((resolve) => {
                    resolveFirstCreate = resolve;
                });
            }
            return Promise.resolve({ ok: true, json: async () => [] } as Response);
        });
        const { selector } = mountSelector(autocompleter({
            selectedResults: [],
            max: 0,
            min: 0,
            ownerId: 0,
            url: '/v1/tags',
            addUrl: '/v1/tag',
        }) as LegacySelector, createForm());
        const input = { value: 'First', dispatchEvent: vi.fn() };
        selector.$refs = { autocompleter: input as unknown as HTMLInputElement };
        selector.init();

        selector.commitToken('First');
        expect(postedInputValues).toEqual(['']);

        input.value = 'Virtual';
        selector._core.dispatch({ type: 'set-query', query: 'Virtual' });
        await vi.advanceTimersByTimeAsync(200);
        await Promise.resolve();
        selector._core.dispatch({ type: 'open' });
        selector._core.dispatch({ type: 'move-active', direction: 'next' });
        selector.pushVal();

        expect(selector._core.getSnapshot()).toMatchObject({
            query: '',
            creationStatus: 'loading',
        });
        resolveFirstCreate({
            ok: true,
            json: async () => ({ ID: 1, Name: 'First' }),
        } as Response);
        selector.destroy();
        vi.useRealTimers();
    });

    test('clears confirmation input before its core queue request starts', () => {
        const postedInputValues: string[] = [];
        vi.mocked(fetch).mockImplementation((_request, init) => {
            if (init?.method === 'POST') {
                postedInputValues.push((input as HTMLInputElement).value);
                return new Promise<Response>(() => undefined);
            }
            return Promise.resolve({ ok: true, json: async () => [] } as Response);
        });
        const { selector } = mountSelector(autocompleter({
            selectedResults: [],
            max: 0,
            min: 0,
            ownerId: 0,
            url: '/v1/tags',
            addUrl: '/v1/tag',
        }) as LegacySelector, createForm());
        const input = { value: 'Confirmed', dispatchEvent: vi.fn() };
        selector.$refs = { autocompleter: input as unknown as HTMLInputElement };
        selector.init();
        selector._core.dispatch({
            type: 'request-create-confirmation',
            label: 'Confirmed',
        });

        selector.addVal();

        expect(postedInputValues).toEqual(['']);
        selector.destroy();
    });

    test('does not bypass core candidate validity when Enter arrives during search', () => {
        const { selector } = mountSelector(autocompleter({
            selectedResults: [],
            max: 0,
            min: 0,
            ownerId: 0,
            url: '/v1/tags',
            addUrl: '/v1/tag',
        }) as LegacySelector, createForm());
        selector.$refs = {
            autocompleter: { value: 'kbtag_a' } as HTMLInputElement,
        };
        selector.init();
        selector._core.dispatch({ type: 'set-query', query: 'kbtag_a' });

        selector.pushVal();

        expect(selector._core.getSnapshot()).toMatchObject({
            searchStatus: 'loading',
            createCandidate: null,
            createConfirmationCandidate: null,
        });
        expect(selector.addModeForTag).toBe('');
        selector.destroy();
    });

    test('does not select stale results when a newer query is still loading', async () => {
        vi.useFakeTimers();
        vi.mocked(fetch).mockResolvedValue({
            ok: true,
            json: async () => [{ ID: 1, Name: 'Alpha' }],
        } as Response);
        const { selector } = mountSelector(autocompleter({
            selectedResults: [],
            max: 0,
            min: 0,
            ownerId: 0,
            url: '/v1/tags',
        }) as LegacySelector, createForm());
        selector.init();
        selector._core.dispatch({ type: 'open' });
        selector._core.dispatch({ type: 'set-query', query: 'alpha' });
        await vi.advanceTimersByTimeAsync(200);
        await Promise.resolve();
        expect(selector._core.getSnapshot()).toMatchObject({
            query: 'alpha',
            searchStatus: 'success',
            activeOptionIndex: 0,
        });

        selector._core.dispatch({ type: 'set-query', query: 'beta' });
        selector.pushVal();

        expect(selector._core.getSnapshot()).toMatchObject({
            query: 'beta',
            searchStatus: 'loading',
            selected: [],
        });
        selector.destroy();
        vi.useRealTimers();
    });

    test('does not select a hidden result when Enter follows Escape', async () => {
        vi.useFakeTimers();
        vi.mocked(fetch).mockResolvedValue({
            ok: true,
            json: async () => [{ ID: 1, Name: 'Alpha' }],
        } as Response);
        const { selector } = mountSelector(autocompleter({
            selectedResults: [],
            max: 0,
            min: 0,
            ownerId: 0,
            url: '/v1/tags',
        }) as LegacySelector, createForm());
        selector.init();
        selector._core.dispatch({ type: 'open' });
        selector._core.dispatch({ type: 'set-query', query: 'alpha' });
        await vi.advanceTimersByTimeAsync(200);
        await Promise.resolve();
        selector._core.dispatch({ type: 'close' });

        selector.pushVal();

        expect(selector._core.getSnapshot()).toMatchObject({
            isOpen: false,
            activeOptionIndex: null,
            selected: [],
        });
        selector.destroy();
        vi.useRealTimers();
    });

    test('does not reactivate after closing before the current search completes', async () => {
        vi.useFakeTimers();
        let resolveResponse!: (response: Response) => void;
        vi.mocked(fetch).mockImplementation(() => new Promise<Response>((resolve) => {
            resolveResponse = resolve;
        }));
        const { selector } = mountSelector(autocompleter({
            selectedResults: [],
            max: 0,
            min: 0,
            ownerId: 0,
            url: '/v1/tags',
        }) as LegacySelector, createForm());
        selector.init();
        selector._core.dispatch({ type: 'open' });
        selector._core.dispatch({ type: 'set-query', query: 'alpha' });
        await vi.advanceTimersByTimeAsync(200);
        selector._core.dispatch({ type: 'close' });
        const completed = new Promise<void>((resolve) => {
            const unsubscribe = selector._core.subscribe((snapshot) => {
                if (snapshot.searchStatus === 'success') {
                    unsubscribe();
                    resolve();
                }
            });
        });
        resolveResponse({
            ok: true,
            json: async () => [{ ID: 1, Name: 'Alpha' }],
        } as Response);
        await completed;

        expect(selector._core.getSnapshot()).toMatchObject({
            searchStatus: 'success',
            isOpen: false,
            activeOptionIndex: null,
        });
        selector.destroy();
        vi.useRealTimers();
    });

    test('does not announce a silent registry replacement after search success', async () => {
        vi.useFakeTimers();
        vi.mocked(fetch).mockResolvedValue({
            ok: true,
            json: async () => [{ ID: 1, Name: 'Alpha' }],
        } as Response);
        const { form, selector } = createSelector({ selectedResults: [] });
        const input = { value: '', dispatchEvent: vi.fn() } as unknown as HTMLInputElement;
        selector.$refs = { autocompleter: input };
        Object.defineProperty(document, 'activeElement', { configurable: true, value: input });
        selector.init();
        const announce = vi.fn();
        selector._liveRegion = { announce, destroy: vi.fn() };
        selector._core.dispatch({ type: 'open' });
        selector._core.dispatch({ type: 'set-query', query: 'alpha' });
        await vi.advanceTimersByTimeAsync(200);
        await Promise.resolve();
        expect(announce).toHaveBeenCalledWith('1 result available. Use arrow keys to navigate.');
        announce.mockClear();

        selectorRegistry.get(form!, 'tags')?.replaceRawValues(
            [{ ID: 2, Name: 'Beta' }],
            { silent: true },
        );

        expect(selector.selectedResults).toEqual([{ ID: 2, Name: 'Beta' }]);
        expect(announce).not.toHaveBeenCalled();
        selector.destroy();
        vi.useRealTimers();
    });

    test('keeps an explicit core close closed after successful search activation', async () => {
        vi.useFakeTimers();
        vi.mocked(fetch).mockResolvedValue({
            ok: true,
            json: async () => [{ ID: 1, Name: 'Alpha' }],
        } as Response);
        const { selector } = mountSelector(autocompleter({
            selectedResults: [],
            max: 0,
            min: 0,
            ownerId: 0,
            url: '/v1/tags',
        }) as LegacySelector, createForm());
        selector.init();
        selector._core.dispatch({ type: 'open' });
        selector._core.dispatch({ type: 'set-query', query: 'alp' });
        await vi.advanceTimersByTimeAsync(200);
        await Promise.resolve();

        expect(selector._core.getSnapshot()).toMatchObject({
            searchStatus: 'success',
            isOpen: true,
            activeOptionIndex: 0,
        });
        selector._core.dispatch({ type: 'close' });

        expect(selector._core.getSnapshot()).toMatchObject({
            isOpen: false,
            activeOptionIndex: null,
        });
        selector.destroy();
        vi.useRealTimers();
    });

    test('translates maximum-one replacement atomically with removals before additions and one aggregate event', () => {
        const calls: string[] = [];
        const onRemove = vi.fn((item) => calls.push(`remove:${item.Name}`));
        const onSelect = vi.fn((item) => calls.push(`select:${item.Name}`));
        const { selector } = mountSelector(autocompleter({
            selectedResults: [{ ID: 1, Name: 'Alpha' }],
            max: 1,
            min: 0,
            ownerId: 0,
            url: '/v1/tags',
            onRemove,
            onSelect,
        }) as LegacySelector, createForm());
        selector.init();

        selector.selectResult({ ID: 2, Name: 'Beta' });

        expect(selector.selectedResults).toEqual([{ ID: 2, Name: 'Beta' }]);
        expect(calls).toEqual(['remove:Alpha', 'select:Beta']);
        expect(selector.$dispatch).toHaveBeenCalledTimes(1);
        expect(selector.$dispatch).toHaveBeenCalledWith('multiple-input', {
            value: [{ ID: 2, Name: 'Beta' }],
            name: undefined,
        });
        selector.destroy();
    });

    test('uses core replacement for nullable clear and keeps a silent reset callback/event-free', () => {
        const onRemove = vi.fn();
        const { selector } = mountSelector(autocompleter({
            selectedResults: [{ ID: 1, Name: 'Alpha' }],
            max: 0,
            min: 0,
            ownerId: 0,
            url: '/v1/tags',
            onRemove,
        }) as LegacySelector, createForm());
        selector.init();

        selector.resetSelectedResults(null, { silent: true });

        expect(selector.selectedResults).toEqual([]);
        expect(onRemove).not.toHaveBeenCalled();
        expect(selector.$dispatch).not.toHaveBeenCalled();
        selector.destroy();
    });

    test('cleans core, registry, and DOM listeners so remount handles each event once', () => {
        const formListeners = new Map<string, Set<EventListener>>();
        const popoverListeners = new Map<string, Set<EventListener>>();
        const listenerTarget = (listeners: Map<string, Set<EventListener>>) => ({
            addEventListener: vi.fn((event: string, listener: EventListener) => {
                const registered = listeners.get(event) ?? new Set<EventListener>();
                registered.add(listener);
                listeners.set(event, registered);
            }),
            removeEventListener: vi.fn((event: string, listener: EventListener) => {
                listeners.get(event)?.delete(listener);
            }),
        });
        const form = listenerTarget(formListeners) as unknown as HTMLFormElement;
        const popover = {
            ...listenerTarget(popoverListeners),
            matches: vi.fn(() => false),
            hidePopover: vi.fn(),
            showPopover: vi.fn(),
            style: {},
        } as unknown as HTMLElement;
        const makeMounted = () => {
            const mounted = mountSelector(autocompleter({
                selectedResults: [{ ID: 1, Name: 'Alpha' }],
                max: 0,
                min: 0,
                ownerId: 0,
                url: '/v1/tags',
                elName: 'tags',
            }) as LegacySelector, form);
            mounted.selector.$refs = { dropdown: popover };
            return mounted.selector;
        };
        const first = makeMounted();
        first.init();
        first.destroy();

        expect(formListeners.get('submit')).toHaveLength(0);
        expect(formListeners.get('reset')).toHaveLength(0);
        expect(popoverListeners.get('mousedown')).toHaveLength(0);
        expect(window.removeEventListener).toHaveBeenCalledWith(
            'scroll', expect.any(Function), true,
        );
        expect(window.removeEventListener).toHaveBeenCalledWith(
            'resize', expect.any(Function),
        );
        expect(selectorRegistry.get(form, 'tags')).toBeUndefined();

        const remounted = makeMounted();
        remounted.init();
        expect(formListeners.get('submit')).toHaveLength(1);
        expect(formListeners.get('reset')).toHaveLength(1);
        expect(popoverListeners.get('mousedown')).toHaveLength(1);

        for (const listener of formListeners.get('reset') ?? []) listener(new Event('reset'));
        expect(remounted.$dispatch).toHaveBeenCalledTimes(1);
        remounted.destroy();
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

    test('uses the reset command for external selector synchronization', () => {
        const entityPickerMarkup = readFileSync(
            new URL('../../templates/partials/entityPicker.tpl', import.meta.url),
            'utf8',
        );
        const lightboxMarkup = readFileSync(
            new URL('../../templates/partials/lightbox.tpl', import.meta.url),
            'utf8',
        );

        expect(entityPickerMarkup).toContain('@entity-picker-closed.window="resetSelectedResults([])"');
        expect(entityPickerMarkup).not.toContain('@entity-picker-closed.window="selectedResults = []"');
        // The lightbox tag editor routes its external synchronization through the tag-editor
        // profile, which resets on navigation and reconciles same-resource changes per key.
        expect(lightboxMarkup).toContain(
            'x-effect="syncEntityTags($store.lightbox.resourceDetails?.ID, $store.lightbox.resourceDetails?.Tags)"',
        );
        expect(lightboxMarkup).not.toContain('selectedResults = ');
    });

    test('keeps user-triggered form resets non-silent', () => {
        let reset: (() => void) | undefined;
        const form = {
            addEventListener: vi.fn((event: string, callback: () => void) => {
                if (event === 'reset') reset = callback;
            }),
            removeEventListener: vi.fn(),
        } as unknown as HTMLFormElement;
        const { selector } = createSelector({ form });
        selector.init();

        reset?.();

        expect(selector.selectedResults).toEqual([]);
        expect(selector._suppressNextAnnounce).toBe(false);
        selector.destroy();
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
        expect(selector._suppressNextAnnounce).toBe(false);

        handle.replaceRawValues([{ ID: 4, Name: 'Delta' }], { silent: true });
        expect(selector.selectedResults).toEqual([{ ID: 4, Name: 'Delta' }]);
        expect(selector._suppressNextAnnounce).toBe(false);

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
        expect(selector._suppressNextAnnounce).toBe(false);

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
        expect(selector._suppressNextAnnounce).toBe(false);

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
