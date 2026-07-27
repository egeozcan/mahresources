import { describe, expect, test } from 'vitest';
import {
    InMemorySelectorSource,
    createSelector,
    type SelectorOption,
    type SelectorSource,
} from './index';

interface RawValue {
    id: number;
    name: string;
}

function option(key: string | number, label: string): SelectorOption<RawValue> {
    return { key, label, raw: { id: Number(key), name: label } };
}

async function flushMicrotasks(): Promise<void> {
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
}

describe('selector lifecycle', () => {
    test('constructs its initial state from normalized options', () => {
        const available = option(1, 'Alpha');
        const selected = option(2, 'Beta');
        const selector = createSelector({
            source: new InMemorySelectorSource<RawValue>(),
            options: [available],
            selected: [selected],
        });

        expect(selector.getSnapshot()).toMatchObject({
            query: '',
            options: [available],
            selected: [selected],
            activeOptionIndex: null,
            isOpen: false,
            searchStatus: 'idle',
            creationStatus: 'idle',
            createCandidate: null,
            createConfirmationCandidate: null,
            error: null,
            destroyed: false,
        });
    });

    test('returns immutable snapshots without freezing raw domain values', () => {
        const available = option(1, 'Alpha');
        const selector = createSelector({
            source: new InMemorySelectorSource<RawValue>(),
            options: [available],
        });
        const snapshot = selector.getSnapshot();

        expect(Object.isFrozen(snapshot)).toBe(true);
        expect(Object.isFrozen(snapshot.options)).toBe(true);
        expect(Object.isFrozen(snapshot.options[0])).toBe(true);
        expect(Object.isFrozen(snapshot.options[0].raw)).toBe(false);
        expect(() => (snapshot.options as SelectorOption<RawValue>[]).push(option(2, 'Beta'))).toThrow();
        expect(() => {
            (snapshot.options[0] as { label: string }).label = 'Changed';
        }).toThrow();

        (available as { label: string }).label = 'Changed outside';
        expect(selector.getSnapshot().options[0].label).toBe('Alpha');
        expect(selector.getSnapshot().options[0].raw).toBe(available.raw);
    });

    test('notifies subscribers in registration order', () => {
        const selector = createSelector({ source: new InMemorySelectorSource<RawValue>() });
        const calls: string[] = [];
        selector.subscribe(() => calls.push('first'));
        selector.subscribe(() => calls.push('second'));

        selector.destroy();

        expect(calls).toEqual(['first', 'second']);
    });

    test('stops notifying a subscriber after unsubscribe', () => {
        const selector = createSelector({ source: new InMemorySelectorSource<RawValue>() });
        const calls: string[] = [];
        selector.subscribe(() => calls.push('kept'));
        const unsubscribe = selector.subscribe(() => calls.push('removed'));

        unsubscribe();
        unsubscribe();
        selector.destroy();

        expect(calls).toEqual(['kept']);
    });

    test('destroys idempotently and rejects later commands with a typed result', () => {
        const selector = createSelector({ source: new InMemorySelectorSource<RawValue>() });
        let notifications = 0;
        selector.subscribe(() => notifications += 1);

        selector.destroy();
        selector.destroy();

        expect(notifications).toBe(1);
        expect(selector.getSnapshot().destroyed).toBe(true);
        expect(selector.dispatch({ type: 'open' })).toEqual({
            ok: false,
            error: {
                code: 'destroyed',
                message: 'Selector has been destroyed',
            },
        });
    });
});

describe('selector selection', () => {
    test('canonicalizes numeric and string keys and prevents duplicate selection', () => {
        const numeric = option(1, 'Numeric');
        const serialized = option('1', 'Serialized');
        const selector = createSelector({
            source: new InMemorySelectorSource<RawValue>(),
            selected: [numeric, serialized],
        });
        let notifications = 0;
        selector.subscribe(() => notifications += 1);

        const result = selector.dispatch({ type: 'select-option', option: serialized });

        expect(selector.getSnapshot().selected).toEqual([numeric]);
        expect(result).toEqual({ ok: true });
        expect(notifications).toBe(0);
    });

    test('appends multi-selections in command order and emits one atomic change', () => {
        const alpha = option(1, 'Alpha');
        const beta = option(2, 'Beta');
        const selector = createSelector({
            source: new InMemorySelectorSource<RawValue>(),
            selected: [alpha],
        });
        const changes: unknown[] = [];
        selector.subscribe((_snapshot, change) => changes.push(change));

        const result = selector.dispatch({ type: 'select-option', option: beta });

        expect(selector.getSnapshot().selected).toEqual([alpha, beta]);
        expect(result).toMatchObject({
            ok: true,
            change: {
                previous: [alpha],
                current: [alpha, beta],
                added: [beta],
                removed: [],
                reason: 'select',
            },
        });
        expect(changes).toHaveLength(1);
        if (!result.ok) throw new Error('selection unexpectedly failed');
        expect(changes[0]).toEqual(result.change);
    });

    test('replaces a single selection atomically', () => {
        const alpha = option(1, 'Alpha');
        const beta = option(2, 'Beta');
        const selector = createSelector({
            source: new InMemorySelectorSource<RawValue>(),
            selected: [alpha],
            multiple: false,
        });
        const changes: unknown[] = [];
        selector.subscribe((_snapshot, change) => changes.push(change));

        const result = selector.dispatch({ type: 'select-option', option: beta });

        expect(selector.getSnapshot().selected).toEqual([beta]);
        expect(result).toMatchObject({
            ok: true,
            change: {
                previous: [alpha],
                current: [beta],
                added: [beta],
                removed: [alpha],
                reason: 'select',
            },
        });
        expect(changes).toHaveLength(1);
    });

    test('evicts the oldest selection when a multi-select maximum is reached', () => {
        const alpha = option(1, 'Alpha');
        const beta = option(2, 'Beta');
        const gamma = option(3, 'Gamma');
        const selector = createSelector({
            source: new InMemorySelectorSource<RawValue>(),
            selected: [alpha, beta],
            maxSelected: 2,
        });

        const result = selector.dispatch({ type: 'select-option', option: gamma });

        expect(selector.getSnapshot().selected).toEqual([beta, gamma]);
        expect(result).toMatchObject({
            change: {
                previous: [alpha, beta],
                current: [beta, gamma],
                added: [gamma],
                removed: [alpha],
            },
        });
    });

    test('removes by canonical key while missing removals are no-ops', () => {
        const alpha = option(1, 'Alpha');
        const beta = option(2, 'Beta');
        const selector = createSelector({
            source: new InMemorySelectorSource<RawValue>(),
            selected: [alpha, beta],
        });
        const changes: unknown[] = [];
        selector.subscribe((_snapshot, change) => changes.push(change));

        expect(selector.dispatch({ type: 'remove-option', key: 99 })).toEqual({ ok: true });
        const result = selector.dispatch({ type: 'remove-option', key: '1' });

        expect(selector.getSnapshot().selected).toEqual([beta]);
        expect(result).toMatchObject({
            change: {
                previous: [alpha, beta],
                current: [beta],
                added: [],
                removed: [alpha],
                reason: 'remove',
            },
        });
        expect(changes).toHaveLength(1);
    });

    test('replaces the complete selection once and preserves supplied ordering', () => {
        const alpha = option(1, 'Alpha');
        const beta = option(2, 'Beta');
        const gamma = option(3, 'Gamma');
        const selector = createSelector({
            source: new InMemorySelectorSource<RawValue>(),
            selected: [alpha, beta],
        });
        const changes: unknown[] = [];
        selector.subscribe((_snapshot, change) => changes.push(change));

        const result = selector.dispatch({
            type: 'replace-selection',
            options: [gamma, alpha, option('3', 'Duplicate Gamma')],
            reason: 'reset',
        });

        expect(selector.getSnapshot().selected).toEqual([gamma, alpha]);
        expect(result).toMatchObject({
            change: {
                previous: [alpha, beta],
                current: [gamma, alpha],
                added: [gamma],
                removed: [beta],
                reason: 'reset',
            },
        });
        expect(changes).toHaveLength(1);
    });

    test('emits one atomic change when replacement only reorders selected options', () => {
        const alpha = option(1, 'Alpha');
        const beta = option(2, 'Beta');
        const selector = createSelector({
            source: new InMemorySelectorSource<RawValue>(),
            selected: [alpha, beta],
        });
        const changes: unknown[] = [];
        selector.subscribe((_snapshot, change) => changes.push(change));

        const result = selector.dispatch({
            type: 'replace-selection',
            options: [beta, alpha],
            reason: 'reset',
        });

        expect(selector.getSnapshot().selected).toEqual([beta, alpha]);
        expect(result).toEqual({
            ok: true,
            change: {
                previous: [alpha, beta],
                current: [beta, alpha],
                added: [],
                removed: [],
                reason: 'reset',
            },
        });
        expect(changes).toEqual([result.ok ? result.change : undefined]);
    });

    test('emits one atomic change when replacement refreshes same-key option data', () => {
        const originalRaw = { id: 1, name: 'Alpha' };
        const refreshedRaw = { id: 1, name: 'Alpha refreshed' };
        const original: SelectorOption<RawValue> = { key: 1, label: 'Alpha', raw: originalRaw };
        const refreshed: SelectorOption<RawValue> = {
            key: '1',
            label: 'Alpha refreshed',
            raw: refreshedRaw,
        };
        const selector = createSelector({
            source: new InMemorySelectorSource<RawValue>(),
            selected: [original],
        });
        const changes: unknown[] = [];
        selector.subscribe((_snapshot, change) => changes.push(change));

        const result = selector.dispatch({
            type: 'replace-selection',
            options: [refreshed],
            reason: 'replace',
        });

        expect(selector.getSnapshot().selected).toEqual([refreshed]);
        expect(result).toEqual({
            ok: true,
            change: {
                previous: [original],
                current: [refreshed],
                added: [],
                removed: [],
                reason: 'replace',
            },
        });
        expect(changes).toEqual([result.ok ? result.change : undefined]);
    });

    test('does not transition or notify when maxSelected is zero', () => {
        const selector = createSelector({
            source: new InMemorySelectorSource<RawValue>(),
            maxSelected: 0,
        });
        const initialSnapshot = selector.getSnapshot();
        let notifications = 0;
        selector.subscribe(() => notifications += 1);

        const result = selector.dispatch({ type: 'select-option', option: option(1, 'Alpha') });

        expect(result).toEqual({ ok: true });
        expect(selector.getSnapshot()).toBe(initialSnapshot);
        expect(selector.getSnapshot().selected).toEqual([]);
        expect(notifications).toBe(0);
    });

    test('updates subscribers but emits no change for silent replacement', () => {
        const alpha = option(1, 'Alpha');
        const beta = option(2, 'Beta');
        const selector = createSelector({
            source: new InMemorySelectorSource<RawValue>(),
            selected: [alpha],
        });
        const notifications: unknown[] = [];
        selector.subscribe((_snapshot, change) => notifications.push(change));

        const result = selector.dispatch({
            type: 'replace-selection',
            options: [beta],
            silent: true,
            reason: 'reset',
        });

        expect(selector.getSnapshot().selected).toEqual([beta]);
        expect(result).toEqual({ ok: true });
        expect(notifications).toEqual([undefined]);
    });
});

class AbortIgnoringDeferredSource implements SelectorSource<RawValue> {
    private readonly pending = new Map<string, {
        promise: Promise<readonly SelectorOption<RawValue>[]>;
        resolve: (options: readonly SelectorOption<RawValue>[]) => void;
    }>();

    defer(query: string): { resolve: (options: readonly SelectorOption<RawValue>[]) => void } {
        let resolve!: (options: readonly SelectorOption<RawValue>[]) => void;
        const promise = new Promise<readonly SelectorOption<RawValue>[]>((resolvePromise) => {
            resolve = resolvePromise;
        });
        this.pending.set(query, { promise, resolve });
        return { resolve };
    }

    search(query: string, _signal: AbortSignal): Promise<readonly SelectorOption<RawValue>[]> {
        const deferred = this.pending.get(query);
        return deferred?.promise ?? Promise.reject(new Error(`No deferred search for ${query}`));
    }
}

describe('selector search orchestration', () => {
    test('updates the query synchronously and only applies latest results when a source ignores abort', async () => {
        const source = new AbortIgnoringDeferredSource();
        const stale = source.defer('old');
        const current = source.defer('new');
        const selector = createSelector({ source });

        expect(selector.dispatch({ type: 'set-query', query: 'old' })).toEqual({ ok: true });
        expect(selector.getSnapshot()).toMatchObject({ query: 'old', searchStatus: 'loading' });
        selector.dispatch({ type: 'set-query', query: 'new' });

        current.resolve([option(2, 'Current')]);
        await flushMicrotasks();
        expect(selector.getSnapshot()).toMatchObject({
            query: 'new',
            options: [option(2, 'Current')],
            searchStatus: 'success',
            error: null,
        });

        stale.resolve([option(1, 'Stale')]);
        await flushMicrotasks();
        expect(selector.getSnapshot()).toMatchObject({
            query: 'new',
            options: [option(2, 'Current')],
            searchStatus: 'success',
            error: null,
        });
    });

    test('filters selected options and treats aborted searches as a normal transition', async () => {
        const source = new InMemorySelectorSource<RawValue>();
        const alpha = option(1, 'Alpha');
        const beta = option(2, 'Beta');
        const first = source.deferSearch('first');
        source.setSearchResult('second', [alpha, beta]);
        const selector = createSelector({ source, selected: [alpha] });

        selector.dispatch({ type: 'set-query', query: 'first' });
        selector.dispatch({ type: 'set-query', query: 'second' });
        first.resolve([option(3, 'Ignored')]);
        await flushMicrotasks();

        expect(selector.getSnapshot()).toMatchObject({
            query: 'second',
            options: [beta],
            searchStatus: 'success',
            error: null,
        });
    });

    test('exposes a typed search error for a current non-abort failure', async () => {
        const source = new InMemorySelectorSource<RawValue>();
        const failure = new Error('lookup unavailable');
        source.setSearchFailure('broken', failure);
        const selector = createSelector({ source });

        selector.dispatch({ type: 'set-query', query: 'broken' });
        await flushMicrotasks();

        expect(selector.getSnapshot()).toMatchObject({
            query: 'broken',
            searchStatus: 'error',
            error: {
                operation: 'search',
                message: 'lookup unavailable',
                cause: failure,
            },
        });
    });
});

describe('selector creation queue foundation', () => {
    test('queues a creatable typed token and consumes the visible query before creation resolves', async () => {
        const source = new InMemorySelectorSource<RawValue>();
        const creation = source.deferCreate('New tag');
        const selector = createSelector({ source });
        const changes: unknown[] = [];
        selector.subscribe((_snapshot, change) => changes.push(change));

        selector.dispatch({ type: 'set-query', query: 'New tag' });
        await flushMicrotasks();
        const result = selector.dispatch({ type: 'commit-token', token: 'New tag' });

        expect(result).toEqual({ ok: true, consumed: true });
        expect(selector.getSnapshot()).toMatchObject({
            query: '',
            creationStatus: 'loading',
            selected: [],
        });

        creation.resolve(option(1, 'New tag'));
        await flushMicrotasks();

        expect(selector.getSnapshot()).toMatchObject({
            creationStatus: 'idle',
            selected: [option(1, 'New tag')],
            error: null,
        });
        expect(changes).toHaveLength(4);
        expect(changes[3]).toMatchObject({ reason: 'create' });
    });
});

describe('selector create candidates and token precedence', () => {
    test('exposes a trimmed create candidate only after its exact query completes without an exact match', async () => {
        const source = new InMemorySelectorSource<RawValue>();
        const pending = source.deferSearch(' New tag ');
        const selector = createSelector({ source });

        selector.dispatch({ type: 'set-query', query: ' New tag ' });
        expect(selector.getSnapshot().createCandidate).toBeNull();

        pending.resolve([]);
        await flushMicrotasks();
        expect(selector.getSnapshot().createCandidate).toEqual({ label: 'New tag' });
        selector.dispatch({ type: 'select-option', option: option(9, 'New tag') });
        expect(selector.getSnapshot().createCandidate).toBeNull();

        source.setSearchResult('Existing', [option(1, 'Existing')]);
        selector.dispatch({ type: 'set-query', query: 'Existing' });
        await flushMicrotasks();
        expect(selector.getSnapshot().createCandidate).toBeNull();
    });

    test('applies selected, available, creatable, and non-creatable token precedence', async () => {
        const selected = option(1, 'Selected');
        const available = option(2, 'Available');
        const created = option(3, 'Created');
        const source = new InMemorySelectorSource<RawValue>();
        source.setCreateResult('Created', created);
        const selector = createSelector({ source, selected: [selected], options: [available] });
        const changes: unknown[] = [];
        selector.subscribe((_snapshot, change) => changes.push(change));

        expect(selector.dispatch({ type: 'commit-token', token: 'Selected' })).toEqual({
            ok: true,
            consumed: true,
        });
        expect(selector.dispatch({ type: 'commit-token', token: 'Available' })).toMatchObject({
            ok: true,
            consumed: true,
            change: { reason: 'select', added: [available] },
        });
        expect(selector.dispatch({ type: 'commit-token', token: 'Created' })).toEqual({
            ok: true,
            consumed: true,
        });
        await flushMicrotasks();
        expect(selector.getSnapshot().selected).toEqual([selected, available, created]);
        expect(changes.filter(Boolean)).toHaveLength(2);

        const plain = createSelector<RawValue>({
            source: { search: async () => [] },
        });
        expect(plain.dispatch({ type: 'commit-token', token: 'Unknown' })).toEqual({
            ok: true,
            consumed: false,
        });
    });

    test('navigates to and commits the virtual create row', async () => {
        const source = new InMemorySelectorSource<RawValue>();
        source.setSearchResult('Virtual', []);
        source.setCreateResult('Virtual', option(1, 'Virtual'));
        const selector = createSelector({ source });

        selector.dispatch({ type: 'set-query', query: 'Virtual' });
        await flushMicrotasks();
        selector.dispatch({ type: 'move-active', direction: 'next' });

        expect(selector.getSnapshot()).toMatchObject({
            createCandidate: { label: 'Virtual' },
            activeOptionIndex: 0,
        });
        expect(selector.dispatch({ type: 'commit-active' })).toEqual({ ok: true, consumed: true });
        expect(selector.getSnapshot().query).toBe('');
        await flushMicrotasks();
        expect(selector.getSnapshot().selected).toEqual([option(1, 'Virtual')]);
    });
});

describe('selector confirmation and unified creation queue', () => {
    test('keeps confirmation in core and clears it on cancellation, replacement, and successful creation', async () => {
        const source = new InMemorySelectorSource<RawValue>();
        source.setSearchResult('Confirm me', []);
        const creation = source.deferCreate('Confirm me');
        const selector = createSelector({ source });

        selector.dispatch({ type: 'set-query', query: 'Confirm me' });
        await flushMicrotasks();
        expect(selector.dispatch({ type: 'request-create-confirmation' })).toEqual({
            ok: true,
            consumed: true,
        });
        expect(selector.getSnapshot().createConfirmationCandidate).toEqual({ label: 'Confirm me' });

        selector.dispatch({
            type: 'replace-selection',
            options: [option(9, 'External')],
            silent: true,
        });
        expect(selector.getSnapshot().createConfirmationCandidate).toBeNull();

        expect(selector.dispatch({
            type: 'request-create-confirmation',
            label: 'Confirm me',
        })).toEqual({ ok: true, consumed: true });
        expect(selector.dispatch({ type: 'cancel-create-confirmation' })).toEqual({
            ok: true,
            consumed: true,
        });
        expect(selector.getSnapshot().createConfirmationCandidate).toBeNull();

        selector.dispatch({ type: 'request-create-confirmation', label: 'Confirm me' });
        expect(selector.dispatch({ type: 'confirm-create' })).toEqual({ ok: true, consumed: true });
        creation.resolve(option(1, 'Confirm me'));
        await flushMicrotasks();
        expect(selector.getSnapshot()).toMatchObject({
            createConfirmationCandidate: null,
            selected: [option(9, 'External'), option(1, 'Confirm me')],
        });
    });

    test('processes queued entry paths in order, recovers from failures, and deduplicates labels', async () => {
        const source = new InMemorySelectorSource<RawValue>();
        const first = source.deferCreate('First');
        source.setCreateFailure('Broken', new Error('cannot create'));
        const last = source.deferCreate('Last');
        const selector = createSelector({ source });
        const observed = [] as Array<{ status: string; error: string | null }>;
        selector.subscribe((snapshot) => observed.push({
            status: snapshot.creationStatus,
            error: snapshot.error?.message ?? null,
        }));

        selector.dispatch({ type: 'commit-token', token: 'First' });
        selector.dispatch({ type: 'commit-token', token: 'First' });
        selector.dispatch({ type: 'commit-token', token: 'Broken' });
        selector.dispatch({ type: 'commit-token', token: 'Last' });
        first.resolve(option(1, 'First'));
        await flushMicrotasks();
        expect(selector.getSnapshot().selected).toEqual([option(1, 'First')]);
        expect(observed).toContainEqual({ status: 'loading', error: 'cannot create' });

        last.resolve(option(2, 'Last'));
        await flushMicrotasks();
        expect(selector.getSnapshot()).toMatchObject({
            selected: [option(1, 'First'), option(2, 'Last')],
            creationStatus: 'idle',
            error: null,
        });
    });

    test('serializes token, virtual-row, and confirmed creations through one public queue', async () => {
        const source = new InMemorySelectorSource<RawValue>();
        const token = source.deferCreate('Token');
        const virtual = source.deferCreate('Virtual');
        const confirmed = source.deferCreate('Confirmed');
        source.setSearchResult('Virtual', []);
        const selector = createSelector({ source });

        selector.dispatch({ type: 'commit-token', token: 'Token' });
        selector.dispatch({ type: 'set-query', query: 'Virtual' });
        await flushMicrotasks();
        selector.dispatch({ type: 'move-active', direction: 'next' });
        selector.dispatch({ type: 'commit-active' });
        selector.dispatch({ type: 'request-create-confirmation', label: 'Confirmed' });
        selector.dispatch({ type: 'confirm-create' });

        token.resolve(option(1, 'Token'));
        await flushMicrotasks();
        virtual.resolve(option(2, 'Virtual'));
        await flushMicrotasks();
        confirmed.resolve(option(3, 'Confirmed'));
        await flushMicrotasks();

        expect(selector.getSnapshot()).toMatchObject({
            selected: [option(1, 'Token'), option(2, 'Virtual'), option(3, 'Confirmed')],
            creationStatus: 'idle',
            error: null,
        });
    });

    test('does not recreate a completed label after selection limits later evict it', async () => {
        const source = new InMemorySelectorSource<RawValue>();
        source.setCreateResult('First', option(1, 'First'));
        source.setCreateResult('Second', option(2, 'Second'));
        const selector = createSelector({ source, multiple: false });

        selector.dispatch({ type: 'commit-token', token: 'First' });
        await flushMicrotasks();
        selector.dispatch({ type: 'commit-token', token: 'Second' });
        await flushMicrotasks();
        expect(selector.getSnapshot().selected).toEqual([option(2, 'Second')]);

        selector.dispatch({ type: 'set-query', query: 'First' });
        await flushMicrotasks();
        expect(selector.dispatch({ type: 'commit-token', token: 'First' })).toEqual({
            ok: true,
            consumed: true,
        });
        expect(selector.getSnapshot()).toMatchObject({
            query: 'First',
            creationStatus: 'idle',
            selected: [option(2, 'Second')],
        });
    });

    test('aborts creation and discards queued work on destruction without adding options', async () => {
        const source = new InMemorySelectorSource<RawValue>();
        const first = source.deferCreate('First');
        source.setCreateResult('Second', option(2, 'Second'));
        const selector = createSelector({ source });

        selector.dispatch({ type: 'commit-token', token: 'First' });
        selector.dispatch({ type: 'commit-token', token: 'Second' });
        selector.destroy();
        first.resolve(option(1, 'First'));
        await flushMicrotasks();

        expect(selector.getSnapshot()).toMatchObject({
            destroyed: true,
            selected: [],
            creationStatus: 'idle',
        });
        expect(selector.dispatch({ type: 'commit-token', token: 'Later' })).toEqual({
            ok: false,
            error: { code: 'destroyed', message: 'Selector has been destroyed' },
        });
    });
});

describe('selector end-to-end public contract flows', () => {
    test('drives a single selector from search through atomic active-option replacement', async () => {
        const alpha = option(1, 'Alpha');
        const beta = option(2, 'Beta');
        const source = new InMemorySelectorSource<RawValue>();
        source.setSearchResult('Alpha', [alpha]);
        source.setSearchResult('Beta', [beta]);
        const selector = createSelector({ source, multiple: false });
        const changes: unknown[] = [];
        selector.subscribe((_snapshot, change) => {
            if (change) changes.push(change);
        });

        selector.dispatch({ type: 'set-query', query: 'Alpha' });
        await flushMicrotasks();
        selector.dispatch({ type: 'move-active', direction: 'next' });
        selector.dispatch({ type: 'commit-active' });
        selector.dispatch({ type: 'set-query', query: 'Beta' });
        await flushMicrotasks();
        selector.dispatch({ type: 'move-active', direction: 'next' });
        selector.dispatch({ type: 'commit-active' });

        expect(selector.getSnapshot().selected).toEqual([beta]);
        expect(changes).toEqual([
            expect.objectContaining({ added: [alpha], removed: [], reason: 'select' }),
            expect.objectContaining({ added: [beta], removed: [alpha], reason: 'select' }),
        ]);
    });

    test('drives multi-selection limits and removals through public commands', () => {
        const alpha = option(1, 'Alpha');
        const beta = option(2, 'Beta');
        const gamma = option(3, 'Gamma');
        const selector = createSelector({
            source: new InMemorySelectorSource<RawValue>(),
            selected: [alpha],
            maxSelected: 2,
        });
        const changes: unknown[] = [];
        selector.subscribe((_snapshot, change) => {
            if (change) changes.push(change);
        });

        selector.dispatch({ type: 'select-option', option: beta });
        selector.dispatch({ type: 'select-option', option: gamma });
        selector.dispatch({ type: 'remove-option', key: gamma.key });

        expect(selector.getSnapshot().selected).toEqual([beta]);
        expect(changes).toEqual([
            expect.objectContaining({ added: [beta], removed: [], reason: 'select' }),
            expect.objectContaining({ added: [gamma], removed: [alpha], reason: 'select' }),
            expect.objectContaining({ added: [], removed: [gamma], reason: 'remove' }),
        ]);
    });

    test('recovers a creatable selector from a creation error through its virtual row', async () => {
        const source = new InMemorySelectorSource<RawValue>();
        source.setSearchResult('New', []);
        source.setCreateFailure('New', new Error('creation unavailable'));
        const selector = createSelector({ source });

        selector.dispatch({ type: 'set-query', query: 'New' });
        await flushMicrotasks();
        selector.dispatch({ type: 'move-active', direction: 'next' });
        selector.dispatch({ type: 'commit-active' });
        await flushMicrotasks();
        expect(selector.getSnapshot()).toMatchObject({
            creationStatus: 'error',
            error: { operation: 'create', message: 'creation unavailable' },
            selected: [],
        });

        source.setCreateResult('New', option(1, 'New'));
        selector.dispatch({ type: 'set-query', query: 'New' });
        await flushMicrotasks();
        selector.dispatch({ type: 'move-active', direction: 'next' });
        selector.dispatch({ type: 'commit-active' });
        await flushMicrotasks();
        expect(selector.getSnapshot()).toMatchObject({
            selected: [option(1, 'New')],
            creationStatus: 'idle',
            error: null,
        });
    });

    test('applies silent external reset without a change and recovers search state', async () => {
        const alpha = option(1, 'Alpha');
        const beta = option(2, 'Beta');
        const source = new InMemorySelectorSource<RawValue>();
        source.setSearchFailure('broken', new Error('search unavailable'));
        source.setSearchResult('recovered', [beta]);
        const selector = createSelector({ source, selected: [alpha] });
        const changes: unknown[] = [];
        selector.subscribe((_snapshot, change) => {
            if (change) changes.push(change);
        });

        selector.dispatch({
            type: 'replace-selection',
            options: [beta],
            reason: 'reset',
            silent: true,
        });
        selector.dispatch({ type: 'set-query', query: 'broken' });
        await flushMicrotasks();
        expect(selector.getSnapshot()).toMatchObject({
            selected: [beta],
            searchStatus: 'error',
            error: { operation: 'search', message: 'search unavailable' },
        });

        selector.dispatch({ type: 'set-query', query: 'recovered' });
        await flushMicrotasks();
        expect(selector.getSnapshot()).toMatchObject({
            selected: [beta],
            options: [],
            searchStatus: 'success',
            error: null,
        });
        expect(changes).toEqual([]);
    });
});

describe('selector open state and navigation', () => {
    test('opens and closes explicitly while clearing the active option on close', () => {
        const selector = createSelector({
            source: new InMemorySelectorSource<RawValue>(),
            options: [option(1, 'Alpha')],
        });

        expect(selector.dispatch({ type: 'open' })).toEqual({ ok: true });
        expect(selector.getSnapshot()).toMatchObject({
            isOpen: true,
            activeOptionIndex: null,
        });

        selector.dispatch({ type: 'move-active', direction: 'next' });
        expect(selector.getSnapshot().activeOptionIndex).toBe(0);
        expect(selector.dispatch({ type: 'close' })).toEqual({ ok: true });
        expect(selector.getSnapshot()).toMatchObject({
            isOpen: false,
            activeOptionIndex: null,
        });
    });

    test('keeps the active option empty when navigating an empty list', () => {
        const selector = createSelector({ source: new InMemorySelectorSource<RawValue>() });

        selector.dispatch({ type: 'move-active', direction: 'next' });
        expect(selector.getSnapshot()).toMatchObject({ isOpen: true, activeOptionIndex: null });

        selector.dispatch({ type: 'move-active', direction: 'previous' });
        expect(selector.getSnapshot()).toMatchObject({ isOpen: true, activeOptionIndex: null });
    });

    test('keeps navigation on the only option', () => {
        const selector = createSelector({
            source: new InMemorySelectorSource<RawValue>(),
            options: [option(1, 'Alpha')],
        });

        selector.dispatch({ type: 'move-active', direction: 'next' });
        expect(selector.getSnapshot().activeOptionIndex).toBe(0);
        selector.dispatch({ type: 'move-active', direction: 'next' });
        expect(selector.getSnapshot().activeOptionIndex).toBe(0);
        selector.dispatch({ type: 'move-active', direction: 'previous' });
        expect(selector.getSnapshot().activeOptionIndex).toBe(0);
    });

    test('wraps next and previous navigation across multiple options', () => {
        const selector = createSelector({
            source: new InMemorySelectorSource<RawValue>(),
            options: [option(1, 'Alpha'), option(2, 'Beta'), option(3, 'Gamma')],
        });

        selector.dispatch({ type: 'move-active', direction: 'next' });
        expect(selector.getSnapshot().activeOptionIndex).toBe(0);
        selector.dispatch({ type: 'move-active', direction: 'next' });
        expect(selector.getSnapshot().activeOptionIndex).toBe(1);
        selector.dispatch({ type: 'move-active', direction: 'next' });
        expect(selector.getSnapshot().activeOptionIndex).toBe(2);
        selector.dispatch({ type: 'move-active', direction: 'next' });
        expect(selector.getSnapshot().activeOptionIndex).toBe(0);

        selector.dispatch({ type: 'close' });
        selector.dispatch({ type: 'open' });
        selector.dispatch({ type: 'move-active', direction: 'previous' });
        expect(selector.getSnapshot().activeOptionIndex).toBe(2);
        selector.dispatch({ type: 'move-active', direction: 'previous' });
        expect(selector.getSnapshot().activeOptionIndex).toBe(1);
    });
});
