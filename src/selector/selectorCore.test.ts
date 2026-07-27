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
