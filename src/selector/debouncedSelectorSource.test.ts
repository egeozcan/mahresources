import { afterEach, describe, expect, test, vi } from 'vitest';
import {
    createDebouncedSelectorSource,
    type SelectorOption,
    type SelectorSource,
} from './index';

interface RawValue {
    id: number;
}

const emptyResults: readonly SelectorOption<RawValue>[] = [];

function abortError(): DOMException {
    return new DOMException('The operation was aborted', 'AbortError');
}

describe('createDebouncedSelectorSource', () => {
    afterEach(() => vi.useRealTimers());

    test('delays the underlying search without changing its original signal', async () => {
        vi.useFakeTimers();
        const source: SelectorSource<RawValue> = {
            search: vi.fn().mockResolvedValue(emptyResults),
        };
        const debounced = createDebouncedSelectorSource(source, 250);
        const controller = new AbortController();

        const result = debounced.search('alpha', controller.signal);
        expect(source.search).not.toHaveBeenCalled();

        await vi.advanceTimersByTimeAsync(249);
        expect(source.search).not.toHaveBeenCalled();
        await vi.advanceTimersByTimeAsync(1);

        await expect(result).resolves.toEqual(emptyResults);
        expect(source.search).toHaveBeenCalledWith('alpha', controller.signal);
    });

    test('cancels scheduled work when a newer search arrives', async () => {
        vi.useFakeTimers();
        const source: SelectorSource<RawValue> = {
            search: vi.fn().mockResolvedValue(emptyResults),
        };
        const debounced = createDebouncedSelectorSource(source, 250);
        const first = debounced.search('first', new AbortController().signal);
        const firstRejection = expect(first).rejects.toMatchObject({ name: 'AbortError' });
        const secondSignal = new AbortController().signal;
        const second = debounced.search('second', secondSignal);

        await firstRejection;
        await vi.advanceTimersByTimeAsync(250);

        await expect(second).resolves.toEqual(emptyResults);
        expect(source.search).toHaveBeenCalledTimes(1);
        expect(source.search).toHaveBeenCalledWith('second', secondSignal);
    });

    test('cancels scheduled work when its signal aborts', async () => {
        vi.useFakeTimers();
        const source: SelectorSource<RawValue> = {
            search: vi.fn().mockResolvedValue(emptyResults),
        };
        const debounced = createDebouncedSelectorSource(source, 250);
        const controller = new AbortController();
        const pending = debounced.search('alpha', controller.signal);
        const rejection = expect(pending).rejects.toEqual(abortError());

        controller.abort();
        await rejection;
        await vi.advanceTimersByTimeAsync(250);

        expect(source.search).not.toHaveBeenCalled();
    });

    test('propagates underlying errors after the delay', async () => {
        vi.useFakeTimers();
        const failure = new Error('network unavailable');
        const source: SelectorSource<RawValue> = {
            search: vi.fn().mockRejectedValue(failure),
        };
        const debounced = createDebouncedSelectorSource(source, 250);

        const pending = debounced.search('alpha', new AbortController().signal);
        const rejection = expect(pending).rejects.toBe(failure);
        await vi.advanceTimersByTimeAsync(250);

        await rejection;
    });
});
