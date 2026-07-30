import { afterEach, describe, expect, test, vi } from 'vitest';
import {
    createMultiEntityFieldProfile,
    createSingleEntityFieldProfile,
    createTagFieldProfile,
} from './index';

/**
 * UI bug hunt 2026-07-29, finding 94: the merge pickers offered the entity being
 * merged into, so an impossible merge could be constructed. Verified live before
 * the fix: on /tag?id=78 ("modern"), typing "modern" into "Tags To Merge" returned
 * `["modern"]`, and submitting it produced HTTP 400 `winner cannot also be the
 * loser`.
 *
 * The exclusion is a callback rather than a value because the bulk toolbar's
 * "Merge Winner" picker has to exclude whatever rows are ticked *at search time*.
 */

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
    // Two flushes: the exclusion filter adds one `await` between the HTTP source's
    // promise and the core's publication.
    await Promise.resolve();
    await Promise.resolve();
}

afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
});

describe('selector excludeValues', () => {
    test('an excluded id is never offered, while the rest of the response is', async () => {
        vi.useFakeTimers();
        const self: EntityValue = { ID: 78, Name: 'modern' };
        const other: EntityValue = { ID: 79, Name: 'modernist' };
        vi.stubGlobal('fetch', vi.fn().mockResolvedValue(response([self, other])));

        const profile = createMultiEntityFieldProfile<EntityValue>({
            entity: 'tag',
            form: { name: 'losers', minimum: 1 },
            excludeValues: () => [78],
        });

        profile.selector.dispatch({ type: 'set-query', query: 'modern' });
        await completeSearch();

        const offered = profile.selector.getSnapshot().options.map((o) => o.label);
        // The positive control lives in the same assertion: if the filter dropped
        // everything, "modernist" would be missing too and this would fail.
        expect(offered).toEqual(['modernist']);
    });

    test('the exclusion is read per search, not captured once', async () => {
        vi.useFakeTimers();
        const alpha: EntityValue = { ID: 1, Name: 'alpha' };
        const beta: EntityValue = { ID: 2, Name: 'beta' };
        // A fresh Response per call: a Response body can be read only once, so a
        // single mockResolvedValue makes the *second* search fail with "body already
        // read" and leaves the previous options on the snapshot — which looks exactly
        // like "the exclusion was captured once".
        vi.stubGlobal('fetch', vi.fn(() => Promise.resolve(response([alpha, beta]))));

        let ticked: number[] = [];
        const profile = createSingleEntityFieldProfile<EntityValue>({
            entity: 'tag',
            form: { name: 'winner', minimum: 1 },
            excludeValues: () => ticked,
        });

        profile.selector.dispatch({ type: 'set-query', query: 'a' });
        await completeSearch();
        expect(profile.selector.getSnapshot().options.map((o) => o.key)).toEqual([1, 2]);

        // This is the bulk-toolbar case: the user ticks a row while the picker is on
        // screen, and the next search must stop offering it.
        ticked = [1];
        profile.selector.dispatch({ type: 'set-query', query: 'al' });
        await completeSearch();
        expect(profile.selector.getSnapshot().options.map((o) => o.key)).toEqual([2]);
    });

    test('ids compare across string and number spellings', async () => {
        vi.useFakeTimers();
        vi.stubGlobal('fetch', vi.fn().mockResolvedValue(response([{ ID: 5, Name: 'five' }])));

        const profile = createMultiEntityFieldProfile<EntityValue>({
            entity: 'tag',
            excludeValues: () => ['5'],
        });

        profile.selector.dispatch({ type: 'set-query', query: 'f' });
        await completeSearch();
        expect(profile.selector.getSnapshot().options).toEqual([]);
    });

    test('without an exclusion the source is untouched', async () => {
        vi.useFakeTimers();
        vi.stubGlobal('fetch', vi.fn().mockResolvedValue(response([{ ID: 5, Name: 'five' }])));

        const profile = createMultiEntityFieldProfile<EntityValue>({ entity: 'tag' });
        profile.selector.dispatch({ type: 'set-query', query: 'f' });
        await completeSearch();
        expect(profile.selector.getSnapshot().options.map((o) => o.key)).toEqual([5]);
    });

    test('a creatable profile keeps its create path when a filter is wrapped around it', async () => {
        // Exercised end to end, because the wrapper builds a *new* source object and
        // the only thing carrying creation across is one line forwarding
        // `source.create`. Asserting that the profile still has a dispatch function
        // (which it would either way) could not detect losing it — the wrapped source
        // would simply have no `create`, the core would offer no candidate, and every
        // tag field in the app would silently stop creating tags.
        vi.useFakeTimers();
        const created: EntityValue = { ID: 12, Name: 'fresh' };
        const fetch = vi.fn()
            .mockResolvedValueOnce(response([]))
            .mockResolvedValueOnce(response(created));
        vi.stubGlobal('fetch', fetch);

        const profile = createTagFieldProfile<EntityValue>({
            usage: 'resources',
            excludeValues: () => [99],
        });

        profile.selector.dispatch({ type: 'set-query', query: 'fresh' });
        await completeSearch();
        expect(profile.selector.getSnapshot().options).toEqual([]);
        // The core only offers to create when its source can: this is the first thing
        // a lost `create` would take away.
        expect(profile.selector.getSnapshot().createCandidate).toEqual({ label: 'fresh' });

        const result = profile.selector.dispatch({ type: 'commit-token', token: 'fresh' });
        expect(result).toMatchObject({ ok: true, consumed: true, creation: { label: 'fresh' } });
        if (!result.ok || !result.creation) throw new Error('the creation was not queued');
        expect(await result.creation.outcome).toMatchObject({ status: 'success' });

        expect(fetch).toHaveBeenNthCalledWith(
            2,
            '/v1/tag',
            expect.objectContaining({ method: 'POST', body: expect.stringContaining('"Name":"fresh"') }),
        );
        expect(profile.selector.getSnapshot().selected).toEqual([
            { key: 12, label: 'fresh', raw: created },
        ]);
    });

    test('the exclusion still applies to a creatable profile', async () => {
        // The control for the test above: forwarding `create` must not have been done
        // by handing back the unwrapped source.
        vi.useFakeTimers();
        vi.stubGlobal('fetch', vi.fn().mockResolvedValue(
            response([{ ID: 99, Name: 'stale' }, { ID: 100, Name: 'staler' }])));

        const profile = createTagFieldProfile<EntityValue>({
            usage: 'resources',
            excludeValues: () => [99],
        });

        profile.selector.dispatch({ type: 'set-query', query: 'stal' });
        await completeSearch();
        expect(profile.selector.getSnapshot().options.map((o) => o.key)).toEqual([100]);
    });
});
