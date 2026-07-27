import { afterEach, describe, expect, test, vi } from 'vitest';
import {
    createDynamicEntitySelectorProfile,
    createMultiEntityFieldProfile,
    createSingleEntityFieldProfile,
    type SelectorOption,
} from './index';

interface EntityValue {
    ID: number;
    Name: string;
    Category?: { Name: string };
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

describe('entity field profiles', () => {
    test('single entity fields map catalog results and atomically replace the selected entity', async () => {
        vi.useFakeTimers();
        const alpha: EntityValue = {
            ID: 1,
            Name: 'Alpha',
            Category: { Name: 'First category' },
        };
        const beta: EntityValue = {
            ID: 2,
            Name: 'Beta',
            Category: { Name: 'Second category' },
        };
        const fetch = vi.fn().mockResolvedValue(response([beta]));
        vi.stubGlobal('fetch', fetch);
        let ownerId = 7;

        const profile = createSingleEntityFieldProfile({
            entity: 'group',
            selected: [alpha],
            form: { name: 'ownerId', minimum: 1 },
            parameters: () => ({ ownerId }),
            categoryDecoration: true,
        });

        expect(Object.keys(profile).sort()).toEqual([
            'form',
            'interaction',
            'lookup',
            'presentation',
            'selector',
        ]);
        expect(profile.lookup).toEqual({ searchUrl: '/v1/groups' });
        expect(profile.form).toEqual({
            name: 'ownerId',
            minimum: 1,
            includeEmptyControl: true,
        });
        expect(profile.interaction).toEqual({ tokenDelimiters: [','], commitOnSpace: false });
        expect(profile.presentation).toEqual({ decoration: 'category-name' });
        expect(profile.selector.getSnapshot().selected).toEqual([
            { key: 1, label: 'Alpha', raw: alpha },
        ]);

        profile.selector.dispatch({ type: 'set-query', query: 'Beta value' });
        await completeSearch();

        expect(fetch).toHaveBeenCalledWith(
            '/v1/groups?ownerId=7&name=Beta+value',
            expect.objectContaining({ signal: expect.any(AbortSignal) }),
        );
        const available = profile.selector.getSnapshot().options[0];
        expect(available).toEqual({ key: 2, label: 'Beta', raw: beta });

        ownerId = 9;
        const result = profile.selector.dispatch({ type: 'select-option', option: available });
        expect(profile.selector.getSnapshot().selected).toEqual([available]);
        expect(result).toMatchObject({
            change: {
                removed: [{ key: 1, label: 'Alpha', raw: alpha }],
                added: [available],
            },
        });

        profile.selector.dispatch({ type: 'set-query', query: 'again' });
        await completeSearch();
        expect(fetch).toHaveBeenLastCalledWith(
            '/v1/groups?ownerId=9&name=again',
            expect.objectContaining({ signal: expect.any(AbortSignal) }),
        );
    });

    test('multi entity fields use form and interaction defaults without exposing creation', async () => {
        vi.useFakeTimers();
        const alpha: EntityValue = { ID: 1, Name: 'Alpha' };
        const beta: EntityValue = { ID: 2, Name: 'Beta' };
        vi.stubGlobal('fetch', vi.fn().mockResolvedValue(response([])));

        const profile = createMultiEntityFieldProfile({
            entity: 'note',
            selected: [alpha],
            form: { name: 'notes' },
        });

        expect(profile.form).toEqual({
            name: 'notes',
            minimum: 0,
            includeEmptyControl: true,
        });
        expect(profile.interaction).toEqual({ tokenDelimiters: [','], commitOnSpace: false });
        expect(profile.presentation).toEqual({ decoration: null });

        const betaOption: SelectorOption<EntityValue> = { key: 2, label: 'Beta', raw: beta };
        profile.selector.dispatch({ type: 'select-option', option: betaOption });
        expect(profile.selector.getSnapshot().selected).toEqual([
            { key: 1, label: 'Alpha', raw: alpha },
            betaOption,
        ]);

        profile.selector.dispatch({ type: 'set-query', query: 'Missing' });
        await completeSearch();
        expect(profile.selector.getSnapshot().createCandidate).toBeNull();
        expect(profile.selector.dispatch({ type: 'commit-token', token: 'Missing' })).toEqual({
            ok: true,
            consumed: false,
        });
    });
});

describe('non-creatable multi-tag profiles', () => {
    test('rank by usage and hide already configured tags without offering creation', async () => {
        vi.useFakeTimers();
        const configured: EntityValue = { ID: 1, Name: 'Landscape' };
        const other: EntityValue = { ID: 2, Name: 'Landmark' };
        const fetch = vi.fn().mockResolvedValue(response([configured, other]));
        vi.stubGlobal('fetch', fetch);

        const profile = createMultiEntityFieldProfile({
            entity: 'tag',
            tagSuggestions: { usage: 'resource' },
            selected: [configured],
        });

        profile.selector.dispatch({ type: 'set-query', query: 'Land' });
        await completeSearch();

        expect(fetch).toHaveBeenCalledWith(
            '/v1/tags/suggest?SortBy=most_used_resource&name=Land',
            expect.objectContaining({ signal: expect.any(AbortSignal) }),
        );
        // The seeded slot tag is already selected, so it is excluded from the offered options.
        expect(profile.selector.getSnapshot().options).toEqual([
            { key: 2, label: 'Landmark', raw: other },
        ]);

        profile.selector.dispatch({ type: 'set-query', query: 'Brand new' });
        await completeSearch();
        expect(profile.selector.getSnapshot().createCandidate).toBeNull();
    });
});

describe('dynamic entity selector profile', () => {
    test('searches a runtime endpoint and keeps at most one selection when not multiple', async () => {
        vi.useFakeTimers();
        const alpha: EntityValue = { ID: 1, Name: 'Alpha' };
        const beta: EntityValue = { ID: 2, Name: 'Beta' };
        const fetch = vi.fn().mockResolvedValue(response([beta]));
        vi.stubGlobal('fetch', fetch);

        const profile = createDynamicEntitySelectorProfile<EntityValue>({
            searchUrl: '/v1/categories',
            multiple: false,
        });

        // A runtime selector never serializes into a form, so it carries no form metadata.
        expect(profile.form).toBeNull();
        expect(profile.presentation).toEqual({ decoration: null });

        profile.selector.dispatch({ type: 'select-option', option: { key: 1, label: 'Alpha', raw: alpha } });
        profile.selector.dispatch({ type: 'set-query', query: 'Be' });
        await completeSearch();

        expect(fetch).toHaveBeenCalledWith(
            '/v1/categories?name=Be',
            expect.objectContaining({ signal: expect.any(AbortSignal) }),
        );

        const available = profile.selector.getSnapshot().options[0];
        const result = profile.selector.dispatch({ type: 'select-option', option: available });
        expect(profile.selector.getSnapshot().selected).toEqual([available]);
        expect(result).toMatchObject({
            change: {
                removed: [{ key: 1, label: 'Alpha', raw: alpha }],
                added: [available],
            },
        });
    });

    test('accumulates selections when multiple and never offers creation', async () => {
        vi.useFakeTimers();
        const alpha: EntityValue = { ID: 1, Name: 'Alpha' };
        const beta: EntityValue = { ID: 2, Name: 'Beta' };
        vi.stubGlobal('fetch', vi.fn().mockResolvedValue(response([])));

        const profile = createDynamicEntitySelectorProfile<EntityValue>({
            searchUrl: '/v1/tags',
            multiple: true,
        });

        profile.selector.dispatch({ type: 'select-option', option: { key: 1, label: 'Alpha', raw: alpha } });
        profile.selector.dispatch({ type: 'select-option', option: { key: 2, label: 'Beta', raw: beta } });
        expect(profile.selector.getSnapshot().selected).toHaveLength(2);

        profile.selector.dispatch({ type: 'set-query', query: 'Missing' });
        await completeSearch();
        expect(profile.selector.getSnapshot().createCandidate).toBeNull();
        expect(profile.selector.dispatch({ type: 'commit-token', token: 'Missing' })).toEqual({
            ok: true,
            consumed: false,
        });
    });
});
