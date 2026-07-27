import { afterEach, describe, expect, test, vi } from 'vitest';
import {
    createCreatableEntityFieldProfile,
    createMultiEntityFieldProfile,
    createTagFieldProfile,
} from './index';

interface EntityValue {
    ID: number;
    Name: string;
}

function response(payload: unknown): Response {
    return new Response(JSON.stringify(payload), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
    });
}

async function completeSearch(): Promise<void> {
    await vi.advanceTimersByTimeAsync(200);
    await Promise.resolve();
}

afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
});

describe('creatable entity field profile', () => {
    test('creates and selects a series through the catalog creation source', async () => {
        vi.useFakeTimers();
        const created: EntityValue = { ID: 12, Name: 'New series' };
        const fetch = vi.fn()
            .mockResolvedValueOnce(response([]))
            .mockResolvedValueOnce(response(created));
        vi.stubGlobal('fetch', fetch);
        const profile = createCreatableEntityFieldProfile<EntityValue>({
            entity: 'series',
            form: { name: 'SeriesId' },
        });

        profile.selector.dispatch({ type: 'set-query', query: ' New series ' });
        await completeSearch();
        expect(profile.selector.getSnapshot().createCandidate).toEqual({ label: 'New series' });

        const result = profile.selector.dispatch({ type: 'commit-token', token: 'New series' });
        expect(result).toMatchObject({
            ok: true,
            consumed: true,
            creation: { label: 'New series' },
        });
        if (!result.ok || !result.creation) throw new Error('series creation was not queued');
        expect(await result.creation.outcome).toMatchObject({ status: 'success' });

        expect(fetch).toHaveBeenNthCalledWith(
            1,
            '/v1/seriesList?name=+New+series+',
            expect.objectContaining({ signal: expect.any(AbortSignal) }),
        );
        expect(fetch).toHaveBeenNthCalledWith(
            2,
            '/v1/series/create',
            expect.objectContaining({
                method: 'POST',
                body: JSON.stringify({ Name: 'New series' }),
                signal: expect.any(AbortSignal),
            }),
        );
        expect(profile.selector.getSnapshot().selected).toEqual([
            { key: 12, label: 'New series', raw: created },
        ]);
    });
});

describe('tag field profile', () => {
    test('uses lean suggestions, tag creation, usage sorting, and tag delimiter behavior', async () => {
        vi.useFakeTimers();
        const created: EntityValue = { ID: 8, Name: 'sunset orange' };
        const fetch = vi.fn()
            .mockResolvedValueOnce(response([]))
            .mockResolvedValueOnce(response(created));
        vi.stubGlobal('fetch', fetch);
        const profile = createTagFieldProfile<EntityValue>({
            usage: 'resource',
            form: { name: 'tags' },
            parameters: () => ({ ownerId: 4 }),
        });

        expect(profile.interaction).toEqual({ tokenDelimiters: [','], commitOnSpace: false });
        profile.selector.dispatch({ type: 'set-query', query: 'sunset orange' });
        await completeSearch();
        const result = profile.selector.dispatch({
            type: 'commit-token',
            token: 'sunset orange',
        });
        if (!result.ok || !result.creation) throw new Error('tag creation was not queued');
        expect(await result.creation.outcome).toMatchObject({ status: 'success' });

        expect(result).toMatchObject({ ok: true, consumed: true });
        expect(fetch).toHaveBeenNthCalledWith(
            1,
            '/v1/tags/suggest?ownerId=4&SortBy=most_used_resource&name=sunset+orange',
            expect.objectContaining({ signal: expect.any(AbortSignal) }),
        );
        expect(fetch).toHaveBeenNthCalledWith(
            2,
            '/v1/tag',
            expect.objectContaining({
                method: 'POST',
                body: JSON.stringify({
                    ownerId: 4,
                    SortBy: 'most_used_resource',
                    Name: 'sunset orange',
                }),
            }),
        );
        expect(profile.selector.getSnapshot().selected).toEqual([
            { key: 8, label: 'sunset orange', raw: created },
        ]);
    });

    test('keeps a non-creatable multi-tag picker in the ordinary entity profile family', async () => {
        vi.useFakeTimers();
        const fetch = vi.fn().mockResolvedValue(response([]));
        vi.stubGlobal('fetch', fetch);
        const profile = createMultiEntityFieldProfile<EntityValue>({
            entity: 'tag',
            tagSuggestions: { usage: 'group' },
            form: { name: 'tags' },
        });

        profile.selector.dispatch({ type: 'set-query', query: 'plain tag' });
        await completeSearch();

        expect(fetch).toHaveBeenCalledWith(
            '/v1/tags/suggest?SortBy=most_used_group&name=plain+tag',
            expect.objectContaining({ signal: expect.any(AbortSignal) }),
        );
        expect(profile.selector.getSnapshot().createCandidate).toBeNull();
        expect(profile.selector.dispatch({ type: 'commit-token', token: 'plain tag' })).toEqual({
            ok: true,
            consumed: false,
        });
    });
});
