import { describe, expect, test, vi } from 'vitest';
import {
    HttpSelectorSourceError,
    createHttpSelectorSource,
    type SelectorOption,
} from './index';

interface RawValue {
    id: number;
    name: string;
}

function response(body: unknown, status = 200): Response {
    return {
        ok: status >= 200 && status < 300,
        status,
        json: vi.fn().mockResolvedValue(body),
    } as unknown as Response;
}

function mapOption(raw: RawValue): SelectorOption<RawValue> {
    return { key: raw.id, label: raw.name, raw };
}

describe('createHttpSelectorSource', () => {
    test('encodes the query and dynamic parameters for every search', async () => {
        let side = 'left/right';
        const fetch = vi.fn().mockResolvedValue(response([]));
        const source = createHttpSelectorSource({
            searchUrl: '/v1/items?maxResults=8',
            parameters: () => ({ side, includeArchived: false }),
            mapOption,
            fetch,
        });

        await source.search('a & b', new AbortController().signal);
        side = 'other';
        await source.search('again', new AbortController().signal);

        expect(fetch).toHaveBeenNthCalledWith(
            1,
            '/v1/items?maxResults=8&side=left%2Fright&includeArchived=false&name=a+%26+b',
            expect.objectContaining({ signal: expect.any(AbortSignal) }),
        );
        expect(fetch).toHaveBeenNthCalledWith(
            2,
            '/v1/items?maxResults=8&side=other&includeArchived=false&name=again',
            expect.objectContaining({ signal: expect.any(AbortSignal) }),
        );
    });

    test('checks a failed status before parsing the response', async () => {
        const failed = response({ unexpected: true }, 503);
        const fetch = vi.fn().mockResolvedValue(failed);
        const source = createHttpSelectorSource({ searchUrl: '/v1/items', mapOption, fetch });

        await expect(source.search('offline', new AbortController().signal)).rejects.toEqual(
            new HttpSelectorSourceError('search', 503),
        );
        expect(failed.json).not.toHaveBeenCalled();
    });

    test('rejects malformed search and creation response shapes', async () => {
        const fetch = vi.fn()
            .mockResolvedValueOnce(response({ results: [] }))
            .mockResolvedValueOnce(response(null));
        const source = createHttpSelectorSource({
            searchUrl: '/v1/items',
            createUrl: '/v1/item',
            mapOption,
            fetch,
        });

        await expect(source.search('alpha', new AbortController().signal)).rejects.toThrow(
            'Expected search response to be a list',
        );
        await expect(source.create?.('Alpha', new AbortController().signal)).rejects.toThrow(
            'Expected create response to be one value',
        );
    });

    test('maps search lists and a created value through the supplied option mapper', async () => {
        const alpha = { id: 1, name: 'Alpha' };
        const beta = { id: 2, name: 'Beta' };
        const fetch = vi.fn()
            .mockResolvedValueOnce(response([alpha]))
            .mockResolvedValueOnce(response(beta));
        const source = createHttpSelectorSource({
            searchUrl: '/v1/items',
            createUrl: '/v1/item',
            parameters: () => ({ groupId: 7 }),
            mapOption,
            fetch,
        });

        await expect(source.search('Alpha', new AbortController().signal)).resolves.toEqual([
            { key: 1, label: 'Alpha', raw: alpha },
        ]);
        await expect(source.create?.('Beta', new AbortController().signal)).resolves.toEqual({
            key: 2,
            label: 'Beta',
            raw: beta,
        });
        expect(fetch).toHaveBeenLastCalledWith('/v1/item', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ groupId: 7, Name: 'Beta' }),
            signal: expect.any(AbortSignal),
        });
    });

    test('passes abort signals through to fetch', async () => {
        const fetch = vi.fn((_url: string, init?: RequestInit) => new Promise<Response>((_resolve, reject) => {
            init?.signal?.addEventListener('abort', () => reject(new DOMException('aborted', 'AbortError')));
        }));
        const source = createHttpSelectorSource({ searchUrl: '/v1/items', mapOption, fetch });
        const controller = new AbortController();
        const pending = source.search('alpha', controller.signal);
        const rejection = expect(pending).rejects.toMatchObject({ name: 'AbortError' });

        controller.abort();
        await rejection;
        expect(fetch).toHaveBeenCalledWith('/v1/items?name=alpha', {
            signal: controller.signal,
        });
    });
});
