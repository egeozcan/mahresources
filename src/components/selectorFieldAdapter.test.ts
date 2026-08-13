import { readFileSync } from 'node:fs';
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import { selectorRegistry, type SelectorFieldChange } from '../selector/selectorRegistry';
import { multiEntitySelector, singleEntitySelector, tagFieldSelector } from './profiledAutocompleter.js';

type MountedSelector = ReturnType<typeof multiEntitySelector> & {
    $el: HTMLElement;
    $refs: Record<string, HTMLElement>;
    $watch: (name: string, callback: (...args: unknown[]) => void) => void;
    $nextTick: (callback: () => void) => void;
    $dispatch: ReturnType<typeof vi.fn>;
};

/** A non-creatable multi-tag field: searches /v1/tags, never offers a create row. */
function tagPicker(options: Record<string, unknown> = {}) {
    return multiEntitySelector({ entity: 'tag', ...options }) as MountedSelector;
}

/** A creatable tag field: searches the lean suggestion source and can create through /v1/tag. */
function creatableTagField(options: Record<string, unknown> = {}) {
    return tagFieldSelector({ usage: 'resource', ...options }) as MountedSelector;
}

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

function mountSelector(selector: MountedSelector, form: HTMLFormElement | null) {
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
}: {
    form?: HTMLFormElement | null;
    elName?: string;
    selectedResults?: Array<Record<string, unknown>>;
} = {}) {
    return mountSelector(tagPicker({ selected: selectedResults, form: { name: elName } }), form);
}

describe('selector rendering adapter and registry integration', () => {
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
        // A result row commits the row it names, so the click survives a newer in-flight
        // search; the virtual create row stays on the index-based action.
        expect(markup).toContain('@mousedown="startSelecting(); setActiveIndex(index); selectResult(result)"');
        expect(markup).toContain('@mousedown="startSelecting(); setActiveIndex(results.length); {{ action }}($event)"');
        expect(markup).not.toContain('selectedIndex = index');
    });

    test('routes chip removal through the initialized core and reports one atomic change', () => {
        const onChange = vi.fn();
        const { selector } = mountSelector(tagPicker({
            selected: [{ ID: 1, Name: 'Alpha' }],
            form: { name: 'tags' },
            onChange,
        }), createForm());
        selector.init();

        selector.removeItem({ ID: 1, Name: 'Alpha' });

        expect(selector.selectedResults).toEqual([]);
        expect(onChange).toHaveBeenCalledTimes(1);
        expect(onChange.mock.calls[0][0]).toMatchObject({
            added: [],
            removed: [{ raw: { ID: 1, Name: 'Alpha' } }],
        });
        selector.destroy();
    });

    test('renders normalized search options and surfaces a failed search as an error message', async () => {
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
        expect(selector).toMatchObject({ query: 'alpha', results: [], errorMessage: false });
        await vi.advanceTimersByTimeAsync(200);
        await Promise.resolve();
        expect(selector).toMatchObject({
            results: [{ ID: 1, Name: 'Alpha' }],
            errorMessage: false,
        });

        selector._core.dispatch({ type: 'set-query', query: 'broken' });
        await vi.advanceTimersByTimeAsync(200);
        await Promise.resolve();
        expect(selector.errorMessage).toBe('lookup unavailable');
        selector.destroy();
        vi.useRealTimers();
    });

    test('reevaluates the profile parameter callback for every core-owned HTTP search', async () => {
        vi.useFakeTimers();
        vi.mocked(fetch).mockResolvedValue({ ok: true, json: async () => [] } as Response);
        let ownerId = 7;
        const { selector } = mountSelector(
            tagPicker({ selected: [], parameters: () => ({ ownerId }) }),
            createForm(),
        );
        selector.init();

        selector._core.dispatch({ type: 'set-query', query: 'first' });
        await vi.advanceTimersByTimeAsync(200);
        ownerId = 8;
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
        const { selector } = mountSelector(creatableTagField({ selected: [] }), createForm());
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
        const { selector } = mountSelector(creatableTagField({ selected: [] }), createForm());
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
        const { selector } = mountSelector(creatableTagField({ selected: [] }), createForm());
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
        // Still the untouched sentinel: the confirmation has never been shown, so the shared
        // field's `addModeForTag !== false` autofocus guard must not fire.
        expect(selector.addModeForTag).toBe(false);
        selector.destroy();
    });

    test('keeps the add-mode sentinel false until the confirmation has been shown', () => {
        const { selector } = mountSelector(creatableTagField({ selected: [] }), createForm());
        selector.$refs = { autocompleter: { value: '' } as HTMLInputElement };
        selector.init();

        expect(selector.addModeForTag).toBe(false);

        selector._core.dispatch({ type: 'request-create-confirmation', label: 'brand new' });
        expect(selector.addModeForTag).toBe('brand new');

        // Once the flow has been entered, leaving it yields '' -- which is what re-focuses the
        // input after Cancel.
        selector._core.dispatch({ type: 'cancel-create-confirmation' });
        expect(selector.addModeForTag).toBe('');
        selector.destroy();
    });

    test('does not select stale results when a newer query is still loading', async () => {
        vi.useFakeTimers();
        vi.mocked(fetch).mockResolvedValue({
            ok: true,
            json: async () => [{ ID: 1, Name: 'Alpha' }],
        } as Response);
        const { selector } = mountSelector(tagPicker({ selected: [] }), createForm());
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
        const { selector } = mountSelector(tagPicker({ selected: [] }), createForm());
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
        const { selector } = mountSelector(tagPicker({ selected: [] }), createForm());
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
        const { selector } = mountSelector(tagPicker({ selected: [] }), createForm());
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

    test('translates maximum-one replacement atomically into one field change for its form', () => {
        const calls: string[] = [];
        const form = createForm();
        const observed: SelectorFieldChange[] = [];
        const stopObserving = selectorRegistry.observe(
            form, 'callbackCategoryId', (change) => observed.push(change),
        );
        const { selector } = mountSelector(singleEntitySelector({
            entity: 'category',
            selected: [{ ID: 1, Name: 'Alpha' }],
            form: { name: 'callbackCategoryId' },
            onChange: (change) => {
                for (const option of change.removed) calls.push(`remove:${option.raw.Name}`);
                for (const option of change.added) calls.push(`select:${option.raw.Name}`);
            },
        }) as MountedSelector, form);
        selector.init();

        selector.selectResult({ ID: 2, Name: 'Beta' });

        expect(selector.selectedResults).toEqual([{ ID: 2, Name: 'Beta' }]);
        expect(calls).toEqual(['remove:Alpha', 'select:Beta']);
        expect(observed).toEqual([{
            values: [{ ID: 2, Name: 'Beta' }],
            added: [{ ID: 2, Name: 'Beta' }],
            removed: [{ ID: 1, Name: 'Alpha' }],
        }]);
        expect(selector.$dispatch).not.toHaveBeenCalled();
        stopObserving();
        selector.destroy();
    });

    test('a change to one form does not reach an identically named field in another form', () => {
        const ownForm = createForm();
        const otherForm = createForm();
        const neighbour = vi.fn();
        const stopObserving = selectorRegistry.observe(otherForm, 'tags', neighbour);
        const { selector } = mountSelector(
            tagPicker({ selected: [], form: { name: 'tags' } }), ownForm,
        );
        selector.init();

        selector.selectResult({ ID: 7, Name: 'Beta' });

        expect(selector.selectedResults).toEqual([{ ID: 7, Name: 'Beta' }]);
        expect(neighbour).not.toHaveBeenCalled();
        stopObserving();
        selector.destroy();
    });

    test('uses core replacement for nullable clear and keeps a silent reset callback/change-free', () => {
        const onChange = vi.fn();
        const form = createForm();
        const observer = vi.fn();
        const stopObserving = selectorRegistry.observe(form, 'tags', observer);
        const { selector } = mountSelector(tagPicker({
            selected: [{ ID: 1, Name: 'Alpha' }],
            form: { name: 'tags' },
            onChange,
        }), form);
        selector.init();

        selector.clearSelection();

        expect(selector.selectedResults).toEqual([]);
        expect(onChange).not.toHaveBeenCalled();
        expect(observer).not.toHaveBeenCalled();
        stopObserving();
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
            const mounted = mountSelector(
                tagPicker({ selected: [{ ID: 1, Name: 'Alpha' }], form: { name: 'tags' } }), form,
            );
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

        const observer = vi.fn();
        const stopObserving = selectorRegistry.observe(form, 'tags', observer);
        for (const listener of formListeners.get('reset') ?? []) listener(new Event('reset'));
        expect(observer).toHaveBeenCalledTimes(1);
        stopObserving();
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

        expect(entityPickerMarkup).toContain('@entity-picker-closed.window="clearSelection()"');
        expect(entityPickerMarkup).not.toContain('@entity-picker-closed.window="selectedResults = []"');
        // The lightbox tag editor routes its external synchronization through the tag-editor
        // profile, which resets on navigation and reconciles same-resource changes per key.
        // Read through displayDetails(), never the raw resourceDetails: during a navigation
        // load window the latter still holds the previous image's tags.
        expect(lightboxMarkup).toContain(
            'x-effect="syncEntityTags($store.lightbox.displayDetails()?.ID, $store.lightbox.displayDetails()?.Tags)"',
        );
        expect(lightboxMarkup).not.toContain('$store.lightbox.resourceDetails');
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
        selector.destroy();
    });

    test('replaces the whole selection whether the replacement is announced or silent', () => {
        const { form, selector } = createSelector();
        const observer = vi.fn();
        const stopObserving = selectorRegistry.observe(form!, 'tags', observer);
        selector.init();
        const handle = selectorRegistry.get(form!, 'tags')!;

        handle.replaceRawValues([{ ID: 2, Name: 'Beta' }]);
        expect(selector.selectedResults).toEqual([{ ID: 2, Name: 'Beta' }]);
        expect(observer).toHaveBeenCalledTimes(1);

        handle.replaceRawValues([{ ID: 4, Name: 'Delta' }], { silent: true });
        expect(selector.selectedResults).toEqual([{ ID: 4, Name: 'Delta' }]);
        expect(observer).toHaveBeenCalledTimes(1);

        stopObserving();
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
        expect(fetchMock).toHaveBeenNthCalledWith(1, '/v1/tags?Name=beta');
        expect(fetchMock).toHaveBeenNthCalledWith(2, '/v1/tags?Name=gamma');
        expect(selector.selectedResults).toEqual([
            { ID: 2, Name: 'Beta' },
            { ID: 3, Name: 'GAMMA' },
        ]);

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
    /**
     * WS14 finding 57. The submit guard sets "Please select at least 1 value"
     * when a required field is empty, and nothing ever retired it: on
     * /relationType/new the red message and aria-invalid="true" stayed under
     * From Category after the user had chosen one, so a form the user had
     * already fixed went on looking broken.
     */
    test('retires the minimum-not-met message once the field holds enough', () => {
        const form = createForm();
        const { selector } = mountSelector(
            tagPicker({ selected: [], form: { name: 'tags', minimum: 1 } }),
            form,
        );
        selector.init();

        const submitHandler = vi.mocked(form.addEventListener).mock.calls
            .find(([type]) => type === 'submit')?.[1] as (e: { preventDefault: () => void }) => void;
        expect(submitHandler, 'the field registers no submit guard').toBeTruthy();

        const event = { preventDefault: vi.fn() };
        submitHandler(event);
        expect(event.preventDefault).toHaveBeenCalled();
        expect(selector.errorMessage).toBe('Please select at least 1 value');

        selector.selectResult({ ID: 1, Name: 'Alpha' });

        expect(selector.selectedResults).toEqual([{ ID: 1, Name: 'Alpha' }]);
        expect(selector.errorMessage).toBe('');

        selector.destroy();
    });

    /**
     * The other half of the same rule: a selection that still does not reach the
     * minimum must keep the message. Without this control the fix could be
     * "clear it on any change" and nobody would notice.
     */
    test('keeps the message while the selection is still short of the minimum', () => {
        const form = createForm();
        const { selector } = mountSelector(
            tagPicker({ selected: [], form: { name: 'tags', minimum: 2 } }),
            form,
        );
        selector.init();

        const submitHandler = vi.mocked(form.addEventListener).mock.calls
            .find(([type]) => type === 'submit')?.[1] as (e: { preventDefault: () => void }) => void;
        submitHandler({ preventDefault: vi.fn() });
        expect(selector.errorMessage).toBe('Please select at least 2 values');

        selector.selectResult({ ID: 1, Name: 'Alpha' });
        expect(selector.errorMessage).toBe('Please select at least 2 values');

        selector.selectResult({ ID: 2, Name: 'Beta' });
        expect(selector.errorMessage).toBe('');

        selector.destroy();
    });
});
