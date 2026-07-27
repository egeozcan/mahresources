import { describe, expect, expectTypeOf, test } from 'vitest';
import {
    InMemorySelectorSource,
    type SelectorCommand,
    type SelectorHandle,
    type SelectorOption,
    type SelectorSource,
    type SelectorState,
} from './index';

interface RawValue {
    id: number;
    name: string;
}

function option(id: number, label: string): SelectorOption<RawValue> {
    return { key: id, label, raw: { id, name: label } };
}

describe('InMemorySelectorSource', () => {
    test('provides deterministic search and creation success', async () => {
        const source = new InMemorySelectorSource<RawValue>();
        const alpha = option(1, 'Alpha');
        const beta = option(2, 'Beta');
        source.setSearchResult('alp', [alpha]);
        source.setCreateResult('Beta', beta);

        await expect(source.search('alp', new AbortController().signal)).resolves.toEqual([alpha]);
        await expect(source.create('Beta', new AbortController().signal)).resolves.toEqual(beta);
    });

    test('provides deterministic search and creation failures', async () => {
        const source = new InMemorySelectorSource<RawValue>();
        const searchError = new Error('search failed');
        const createError = new Error('create failed');
        source.setSearchFailure('bad search', searchError);
        source.setCreateFailure('bad create', createError);

        await expect(source.search('bad search', new AbortController().signal)).rejects.toBe(searchError);
        await expect(source.create('bad create', new AbortController().signal)).rejects.toBe(createError);
    });

    test('allows deferred searches to complete without sleeps', async () => {
        const source = new InMemorySelectorSource<RawValue>();
        const alpha = option(1, 'Alpha');
        const deferred = source.deferSearch('later');
        const pending = source.search('later', new AbortController().signal);

        deferred.resolve([alpha]);

        await expect(pending).resolves.toEqual([alpha]);
    });

    test('rejects deferred work with AbortError when cancelled', async () => {
        const source = new InMemorySelectorSource<RawValue>();
        source.deferSearch('cancel me');
        const controller = new AbortController();
        const pending = source.search('cancel me', controller.signal);

        controller.abort();

        await expect(pending).rejects.toMatchObject({ name: 'AbortError' });
    });

    test('publishes the source and headless contract types from the module entry', () => {
        expectTypeOf<InMemorySelectorSource<RawValue>>().toMatchTypeOf<SelectorSource<RawValue>>();
        expectTypeOf<SelectorHandle<RawValue>['getSnapshot']>().returns.toMatchTypeOf<SelectorState<RawValue>>();
        expectTypeOf<SelectorCommand<RawValue>>().not.toBeNever();
    });
});
