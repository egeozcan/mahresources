// @vitest-environment happy-dom
import { beforeEach, afterEach, describe, expect, it, vi } from 'vitest';
import { savedSearches } from './savedSearches.js';
import { askToConfirm } from './confirmDialog.js';

vi.mock('./confirmDialog.js', () => ({ askToConfirm: vi.fn() }));

describe('saved searches', () => {
    let component;
    beforeEach(() => {
        vi.stubGlobal('fetch', vi.fn());
        vi.mocked(askToConfirm).mockResolvedValue(true);
        window.history.replaceState(null, '', '/tags?Name=applied');
        document.body.innerHTML = '<button id="trigger">Saved searches</button><input name="Name" value="pending">';
        component = savedSearches('tags');
        component.$refs = { trigger: document.querySelector('button'), save: document.querySelector('button') };
        component.$nextTick = callback => callback();
    });
    afterEach(() => vi.unstubAllGlobals());

    it('saves applied URL state and waits for server success', async () => {
        let resolve;
        vi.mocked(fetch).mockReturnValue(new Promise(r => { resolve = r; }));
        component.startDialog();
        component.name = 'Search';
        const saving = component.save();
        expect(component.status).toBe('');
        expect(component.busy).toBe(true);
        expect(JSON.parse(vi.mocked(fetch).mock.calls[0][1]!.body as string)).toEqual({ name: 'Search', url: '/tags?Name=applied' });
        resolve(new Response(JSON.stringify({ id: 1, name: 'Search', url: '/tags?Name=applied' }), { status: 201 }));
        await saving;
        expect(component.dialog).toBe('');
        expect(component.searches).toHaveLength(1);
        expect(component.status).toBe('Search saved.');
    });

    it('retains the dialog and entered name when saving fails', async () => {
        vi.mocked(fetch).mockResolvedValue(new Response('{"error":"Database unavailable"}', { status: 500 }));
        component.startDialog();
        component.name = 'Keep this';
        await component.save();
        expect(component.dialog).toBe('Save current search');
        expect(component.name).toBe('Keep this');
        expect(component.error).toContain('Database unavailable');
        expect(component.status).toBe('');
        expect(component.searches).toEqual([]);
    });

    it('does not overwrite the URL during rename', async () => {
        const search = { id: 4, name: 'Old', url: '/tags?Name=old' };
        vi.mocked(fetch).mockResolvedValue(new Response(JSON.stringify({ ...search, name: 'New' })));
        component.startDialog(search);
        component.name = 'New';
        await component.save();
        expect(JSON.parse(vi.mocked(fetch).mock.calls[0][1]!.body as string)).toEqual({ name: 'New' });
    });

    it('does not mutate when replacement or deletion is cancelled', async () => {
        vi.mocked(askToConfirm).mockResolvedValue(false);
        const search = { id: 4, name: 'Keep' };
        await component.replace(search);
        await component.remove(search);
        expect(fetch).not.toHaveBeenCalled();
    });

    it('keeps searches when deletion fails', async () => {
        vi.mocked(fetch).mockResolvedValue(new Response('{}', { status: 500 }));
        const search = { id: 4, name: 'Keep' };
        component.searches = [search];
        await component.remove(search);
        expect(component.searches).toEqual([search]);
        expect(component.error).toContain('Could not delete');
    });

    it('reloads from the server when reopening the menu', async () => {
        vi.mocked(fetch).mockResolvedValue(new Response('[]'));
        await component.toggle();
        component.close();
        vi.mocked(fetch).mockResolvedValue(new Response('[{"id":8,"name":"Other browser"}]'));
        await component.toggle();
        expect(component.searches[0].name).toBe('Other browser');
    });

    it('ignores an older load after a newer load and successful save', async () => {
        const pending: Array<(response: Response) => void> = [];
        vi.mocked(fetch).mockImplementation(() => new Promise(resolve => pending.push(resolve)));
        const firstOpen = component.toggle();
        component.close();
        const secondOpen = component.toggle();
        pending[1](new Response('[]'));
        await secondOpen;
        component.startDialog();
        component.name = 'Just saved';
        const saving = component.save();
        pending[2](new Response(JSON.stringify({ id: 1, name: 'Just saved', url: '/tags' }), { status: 201 }));
        await saving;
        pending[0](new Response('[]'));
        await firstOpen;
        expect(component.searches.map(search => search.name)).toEqual(['Just saved']);
        expect(component.status).toBe('Search saved.');
    });

    it('keeps the latest load pending when an obsolete request fails', async () => {
        const pending: Array<(response: Response) => void> = [];
        vi.mocked(fetch).mockImplementation(() => new Promise(resolve => pending.push(resolve)));
        const firstOpen = component.toggle();
        component.close();
        const secondOpen = component.toggle();
        pending[0](new Response('{"error":"Old request failed"}', { status: 500 }));
        await firstOpen;
        expect(component.loading).toBe(true);
        expect(component.error).toBe('');
        pending[1](new Response('[{"id":2,"name":"Latest"}]'));
        await secondOpen;
        expect(component.loading).toBe(false);
        expect(component.searches[0].name).toBe('Latest');
    });
});
