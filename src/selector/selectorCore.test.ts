import { describe, expect, test } from 'vitest';
import {
    InMemorySelectorSource,
    createSelector,
    type SelectorOption,
} from './index';

interface RawValue {
    id: number;
    name: string;
}

function option(key: string | number, label: string): SelectorOption<RawValue> {
    return { key, label, raw: { id: Number(key), name: label } };
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
