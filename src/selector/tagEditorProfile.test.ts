import { afterEach, describe, expect, test, vi } from 'vitest';
import {
    createTagEditorProfile,
    type TagAssociationPersistenceAdapter,
} from './index';

interface TagValue {
    ID: number;
    Name: string;
}

interface Deferred<T> {
    readonly promise: Promise<T>;
    resolve(value: T): void;
    reject(error: unknown): void;
}

function deferred<T>(): Deferred<T> {
    let resolvePromise!: (value: T) => void;
    let rejectPromise!: (error: unknown) => void;
    const promise = new Promise<T>((resolve, reject) => {
        resolvePromise = resolve;
        rejectPromise = reject;
    });
    return { promise, resolve: resolvePromise, reject: rejectPromise };
}

function tag(ID: number, Name: string): TagValue {
    return { ID, Name };
}

function persistence(): {
    readonly adapter: TagAssociationPersistenceAdapter<TagValue>;
    readonly add: ReturnType<typeof vi.fn>;
    readonly remove: ReturnType<typeof vi.fn>;
} {
    const add = vi.fn<(value: TagValue, signal: AbortSignal) => Promise<void>>();
    const remove = vi.fn<(value: TagValue, signal: AbortSignal) => Promise<void>>();
    return { adapter: { add, remove }, add, remove };
}

async function flush(): Promise<void> {
    await Promise.resolve();
    await Promise.resolve();
}

afterEach(() => {
    vi.useRealTimers();
});

describe('tag editor profile', () => {
    test('persists optimistic additions and removals while exposing canonical pending keys', async () => {
        const alpha = tag(1, 'Alpha');
        const beta = tag(2, 'Beta');
        const writes = persistence();
        const add = deferred<void>();
        const remove = deferred<void>();
        writes.add.mockReturnValue(add.promise);
        writes.remove.mockReturnValue(remove.promise);
        const profile = createTagEditorProfile({
            usage: 'resource',
            selected: [alpha],
            association: writes.adapter,
        });

        profile.selector.dispatch({
            type: 'select-option',
            option: { key: beta.ID, label: beta.Name, raw: beta },
        });
        expect(profile.selector.getSnapshot().selected.map((option) => option.key)).toEqual([1, 2]);
        expect(writes.add).toHaveBeenCalledWith(beta, expect.any(AbortSignal));
        expect(profile.getSnapshot()).toEqual({ pendingKeys: ['2'], failedKeys: [], destroyed: false });

        add.resolve();
        await flush();
        expect(profile.getSnapshot()).toEqual({ pendingKeys: [], failedKeys: [], destroyed: false });

        profile.selector.dispatch({ type: 'remove-option', key: alpha.ID });
        expect(profile.selector.getSnapshot().selected.map((option) => option.key)).toEqual([2]);
        expect(writes.remove).toHaveBeenCalledWith(alpha, expect.any(AbortSignal));
        expect(profile.getSnapshot()).toEqual({ pendingKeys: ['1'], failedKeys: [], destroyed: false });

        remove.resolve();
        await flush();
        expect(profile.getSnapshot()).toEqual({ pendingKeys: [], failedKeys: [], destroyed: false });
    });

    test('rolls failed writes back through the selector and expires failure presentation', async () => {
        vi.useFakeTimers();
        const alpha = tag(1, 'Alpha');
        const beta = tag(2, 'Beta');
        const writes = persistence();
        const add = deferred<void>();
        const remove = deferred<void>();
        writes.add.mockReturnValue(add.promise);
        writes.remove.mockReturnValue(remove.promise);
        const profile = createTagEditorProfile({
            usage: 'resource',
            selected: [alpha],
            association: writes.adapter,
        });

        profile.selector.dispatch({
            type: 'select-option',
            option: { key: beta.ID, label: beta.Name, raw: beta },
        });
        add.reject(new Error('add failed'));
        await flush();
        expect(profile.selector.getSnapshot().selected.map((option) => option.key)).toEqual([1]);
        expect(profile.getSnapshot().failedKeys).toEqual(['2']);

        profile.selector.dispatch({ type: 'remove-option', key: alpha.ID });
        remove.reject(new Error('remove failed'));
        await flush();
        expect(profile.selector.getSnapshot().selected.map((option) => option.key)).toEqual([1]);
        expect(profile.getSnapshot().failedKeys).toEqual(['1', '2']);

        await vi.advanceTimersByTimeAsync(399);
        expect(profile.getSnapshot().failedKeys).toEqual(['1', '2']);
        await vi.advanceTimersByTimeAsync(1);
        expect(profile.getSnapshot().failedKeys).toEqual([]);
    });

    test('deduplicates same-key work while different keys persist concurrently', () => {
        const alpha = tag(1, 'Alpha');
        const beta = tag(2, 'Beta');
        const gamma = tag(3, 'Gamma');
        const writes = persistence();
        const betaWrite = deferred<void>();
        const gammaWrite = deferred<void>();
        writes.add.mockImplementation((value) => value.ID === beta.ID
            ? betaWrite.promise
            : gammaWrite.promise);
        const profile = createTagEditorProfile({ usage: 'resource', association: writes.adapter });

        profile.selector.dispatch({
            type: 'select-option',
            option: { key: beta.ID, label: beta.Name, raw: beta },
        });
        profile.selector.dispatch({
            type: 'select-option',
            option: { key: beta.ID, label: beta.Name, raw: beta },
        });
        profile.selector.dispatch({
            type: 'select-option',
            option: { key: gamma.ID, label: gamma.Name, raw: gamma },
        });

        expect(writes.add).toHaveBeenCalledTimes(2);
        expect(writes.add).toHaveBeenNthCalledWith(1, beta, expect.any(AbortSignal));
        expect(writes.add).toHaveBeenNthCalledWith(2, gamma, expect.any(AbortSignal));
        expect(profile.getSnapshot().pendingKeys).toEqual(['2', '3']);
    });

    test('does not let stale rollback overwrite a later change to the same key', async () => {
        const beta = tag(2, 'Beta');
        const writes = persistence();
        const add = deferred<void>();
        const remove = deferred<void>();
        writes.add.mockReturnValue(add.promise);
        writes.remove.mockReturnValue(remove.promise);
        const profile = createTagEditorProfile({ usage: 'resource', association: writes.adapter });

        profile.selector.dispatch({
            type: 'select-option',
            option: { key: beta.ID, label: beta.Name, raw: beta },
        });
        profile.selector.dispatch({ type: 'remove-option', key: beta.ID });
        add.reject(new Error('stale add failure'));
        await flush();

        expect(profile.selector.getSnapshot().selected).toEqual([]);
        expect(profile.getSnapshot().failedKeys).toEqual([]);
        expect(profile.getSnapshot().pendingKeys).toEqual(['2']);

        remove.resolve();
        await flush();
        expect(profile.getSnapshot().pendingKeys).toEqual([]);
    });

    test('does not roll back into an external silent reset while an association write is in flight', async () => {
        const alpha = tag(1, 'Alpha');
        const beta = tag(2, 'Beta');
        const writes = persistence();
        const add = deferred<void>();
        writes.add.mockReturnValue(add.promise);
        const profile = createTagEditorProfile({ usage: 'resource', association: writes.adapter });

        profile.selector.dispatch({
            type: 'select-option',
            option: { key: beta.ID, label: beta.Name, raw: beta },
        });
        profile.selector.dispatch({
            type: 'replace-selection',
            options: [{ key: alpha.ID, label: alpha.Name, raw: alpha }],
            reason: 'reset',
            silent: true,
        });
        add.reject(new Error('stale add failure'));
        await flush();

        expect(profile.selector.getSnapshot().selected.map((option) => option.key)).toEqual([1]);
        expect(profile.getSnapshot()).toEqual({ pendingKeys: [], failedKeys: [], destroyed: false });
    });

    test('invalidates an in-flight write before an identical external reset that core would ignore', async () => {
        const beta = tag(2, 'Beta');
        const writes = persistence();
        const add = deferred<void>();
        writes.add.mockReturnValue(add.promise);
        const profile = createTagEditorProfile({ usage: 'resource', association: writes.adapter });

        profile.selector.dispatch({
            type: 'select-option',
            option: { key: beta.ID, label: beta.Name, raw: beta },
        });
        const pendingOption = profile.selector.getSnapshot().selected[0];
        const signal = writes.add.mock.calls[0][1];

        profile.selector.dispatch({
            type: 'replace-selection',
            options: [pendingOption],
            reason: 'reset',
            silent: true,
        });
        expect(signal.aborted).toBe(true);
        expect(profile.getSnapshot().pendingKeys).toEqual([]);

        add.reject(new Error('old resource failed'));
        await flush();

        expect(profile.selector.getSnapshot().selected).toEqual([pendingOption]);
        expect(profile.getSnapshot().failedKeys).toEqual([]);
    });

    test('invalidates an in-flight write before a same-key external reset with new tag data', async () => {
        const beta = tag(2, 'Beta');
        const replacement = tag(2, 'Beta from new resource');
        const writes = persistence();
        const add = deferred<void>();
        writes.add.mockReturnValue(add.promise);
        const profile = createTagEditorProfile({ usage: 'resource', association: writes.adapter });

        profile.selector.dispatch({
            type: 'select-option',
            option: { key: beta.ID, label: beta.Name, raw: beta },
        });
        const signal = writes.add.mock.calls[0][1];
        const replacementOption = {
            key: String(replacement.ID),
            label: replacement.Name,
            raw: replacement,
        };

        profile.selector.dispatch({
            type: 'replace-selection',
            options: [replacementOption],
            reason: 'reset',
            silent: true,
        });
        expect(signal.aborted).toBe(true);
        expect(profile.getSnapshot().pendingKeys).toEqual([]);

        add.reject(new Error('old resource failed'));
        await flush();

        expect(profile.selector.getSnapshot().selected).toEqual([replacementOption]);
        expect(profile.getSnapshot().failedKeys).toEqual([]);
    });

    test('aborts profile-owned writes, clears presentation state, and ignores completions on destroy', async () => {
        const beta = tag(2, 'Beta');
        const writes = persistence();
        const add = deferred<void>();
        writes.add.mockReturnValue(add.promise);
        const profile = createTagEditorProfile({ usage: 'resource', association: writes.adapter });

        profile.selector.dispatch({
            type: 'select-option',
            option: { key: beta.ID, label: beta.Name, raw: beta },
        });
        const signal = writes.add.mock.calls[0][1];
        profile.destroy();
        add.reject(new Error('destroyed write'));
        await flush();

        expect(signal.aborted).toBe(true);
        expect(profile.getSnapshot()).toEqual({ pendingKeys: [], failedKeys: [], destroyed: true });
        expect(profile.selector.getSnapshot().destroyed).toBe(true);
    });
});
