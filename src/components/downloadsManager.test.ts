import { describe, expect, test, vi, beforeEach, afterEach } from 'vitest';
import { downloadsStore } from './downloadsManager.js';
import { downloadCockpit } from './downloadCockpit.js';

/**
 * The /downloads store, and the jobs panel's row cap.
 *
 * Both are pure enough to pin here: the action calls are one fetch whose response
 * shape decides what the user is told, and the cap is a slice. Selection is no
 * longer the store's business — it reads the shared `bulkSelection` store, which
 * every entity list already exercises — so what is left to test is that it reads
 * it, that Delete asks first, and that the outcome reporting survived the move.
 * The Playwright spec covers that the template is wired to it.
 */

let confirmAnswer: boolean;
let confirmCalls: { message: string; options: any }[];
let selectedIds: Set<number>;

function mountStore() {
    confirmAnswer = true;
    confirmCalls = [];
    selectedIds = new Set();

    const stores: Record<string, any> = {
        bulkSelection: { get selectedIds() { return selectedIds; } },
        confirmDialog: {
            ask: (message: string, options: any) => {
                confirmCalls.push({ message, options });
                return Promise.resolve(confirmAnswer);
            },
        },
    };
    // `any` because the store is a plain JS object Alpine augments at runtime.
    const store: any = downloadsStore({ store: (name: string) => stores[name] } as any);
    store._liveRegion = { announce: vi.fn(), destroy: vi.fn() };
    return store;
}

let reloadCalls: number;

beforeEach(() => {
    reloadCalls = 0;
    const store = new Map<string, string>();
    vi.stubGlobal('sessionStorage', {
        store,
        getItem: (k: string) => store.get(k) ?? null,
        setItem: (k: string, v: string) => { store.set(k, v); },
        removeItem: (k: string) => { store.delete(k); },
    });
    vi.stubGlobal('window', { location: { reload: () => { reloadCalls++; } } });
});

afterEach(() => {
    vi.unstubAllGlobals();
});

describe('selection', () => {
    test('the selection is the shared bulkSelection store, not a private copy', () => {
        const c = mountStore();
        expect(c.selectedCount).toBe(0);

        selectedIds.add(1);
        selectedIds.add(2);
        expect(c.selectedIds).toEqual([1, 2]);
        expect(c.selectedCount).toBe(2);
    });

    test('bulk actions send exactly what is selected', async () => {
        const c = mountStore();
        selectedIds.add(4);
        selectedIds.add(5);
        const fetchMock = vi.fn(async (_path: string, _init: any) => ({ ok: true, json: async () => ({ retried: 2 }) }));
        vi.stubGlobal('fetch', fetchMock);

        await c.retrySelected();

        expect(JSON.parse(fetchMock.mock.calls[0][1].body)).toEqual({ ids: [4, 5] });
    });
});

describe('delete confirmation', () => {
    test('deleting asks first, and names the count and what survives', async () => {
        const c = mountStore();
        const fetchMock = vi.fn(async () => ({ ok: true, json: async () => ({ deleted: 2 }) }));
        vi.stubGlobal('fetch', fetchMock);

        await c.remove([1, 2]);

        expect(confirmCalls).toHaveLength(1);
        expect(confirmCalls[0].message).toContain('Delete 2 download records');
        // The obvious fear about deleting a download row is that it deletes the file.
        expect(confirmCalls[0].message).toContain('not affected');
        expect(fetchMock).toHaveBeenCalledTimes(1);
    });

    test('a dismissed confirm deletes nothing', async () => {
        const c = mountStore();
        confirmAnswer = false;
        const fetchMock = vi.fn();
        vi.stubGlobal('fetch', fetchMock);

        await c.remove([1]);

        expect(fetchMock).not.toHaveBeenCalled();
        expect(reloadCalls).toBe(0);
    });

    test('retry is not gated behind a confirm', async () => {
        const c = mountStore();
        vi.stubGlobal('fetch', vi.fn(async () => ({ ok: true, json: async () => ({ retried: 1 }) })));

        await c.retry([1]);

        expect(confirmCalls).toHaveLength(0);
    });
});

describe('actions', () => {
    test('a partly refused batch reports both halves', async () => {
        const c = mountStore();
        vi.stubGlobal('fetch', vi.fn(async () => ({
            ok: true,
            json: async () => ({
                retried: 1,
                results: [
                    { id: 1, ok: true, jobId: 'j1' },
                    { id: 2, ok: false, reason: 'a completed download cannot be retried' },
                ],
            }),
        })));

        await c.send('/v1/downloads/retry', [1, 2], 'retried');

        const message = sessionStorage.getItem('downloads-flash');
        expect(message).toContain('1 download retried');
        expect(message).toContain('1 skipped');
        expect(message).toContain('completed download cannot be retried');
        expect(reloadCalls).toBe(1);
    });

    test('a wholly refused batch reports each distinct reason, not just the summary', async () => {
        const c = mountStore();
        vi.stubGlobal('fetch', vi.fn(async () => ({
            ok: false,
            json: async () => ({
                error: 'no downloads could be retried',
                retried: 0,
                results: [
                    { id: 1, ok: false, reason: 'a completed download cannot be retried' },
                    { id: 2, ok: false, reason: 'this download is already queued as ab12; wait for it to finish' },
                    { id: 3, ok: false, reason: 'a completed download cannot be retried' },
                ],
            }),
        })));

        await c.send('/v1/downloads/retry', [1, 2, 3], 'retried');

        expect(c.error).toContain('completed download cannot be retried');
        expect(c.error).toContain('already queued as ab12');
        // Deduplicated: the same reason repeated is one sentence, not three.
        expect(c.error.match(/completed download cannot be retried/g)).toHaveLength(1);
        expect(reloadCalls).toBe(0);
    });

    test('a refusal surfaces the server reason and does not reload', async () => {
        const c = mountStore();
        vi.stubGlobal('fetch', vi.fn(async () => ({
            ok: false,
            json: async () => ({ error: 'this download is still running; cancel it first' }),
        })));

        await c.send('/v1/downloads/delete', [7], 'deleted');

        expect(c.error).toBe('this download is still running; cancel it first');
        expect(reloadCalls).toBe(0);
    });

    test('a network failure is reported rather than swallowed', async () => {
        const c = mountStore();
        vi.stubGlobal('fetch', vi.fn(async () => { throw new Error('offline'); }));
        vi.spyOn(console, 'error').mockImplementation(() => {});

        await c.send('/v1/downloads/retry', [1], 'retried');

        expect(c.error).toMatch(/request failed/i);
        expect(c.busy).toBe(false);
        expect(reloadCalls).toBe(0);
    });

    test('an empty selection sends nothing', async () => {
        const c = mountStore();
        const fetchMock = vi.fn();
        vi.stubGlobal('fetch', fetchMock);

        await c.send('/v1/downloads/delete', [], 'deleted');
        await c.removeSelected();

        expect(fetchMock).not.toHaveBeenCalled();
        // And nothing is confirmed either: an empty Delete must not raise a dialog
        // that would then delete nothing.
        expect(confirmCalls).toHaveLength(0);
    });
});

describe('jobs panel row cap', () => {
    function cockpitWithJobs(count: number, limit: number) {
        const c: any = downloadCockpit();
        c._dismissedIds = new Set();
        c.rowLimit = limit;
        c.jobs = Array.from({ length: count }, (_, i) => ({
            id: `job-${i}`,
            url: `http://example.com/${i}`,
            status: 'completed',
            source: 'download',
            // Ascending timestamps, so the newest job is the last one created.
            createdAt: new Date(Date.UTC(2026, 0, 1, 0, i)).toISOString(),
        }));
        return c;
    }

    test('only the newest rows are rendered', () => {
        const c = cockpitWithJobs(25, 10);
        expect(c.displayJobs.length).toBe(25);
        expect(c.visibleJobs.length).toBe(10);
        expect(c.hiddenJobCount).toBe(15);
        // Newest first, so the cap hides the oldest — never a running download.
        expect(c.visibleJobs[0].id).toBe('job-24');
    });

    test('a queue under the limit renders whole and hides nothing', () => {
        const c = cockpitWithJobs(3, 10);
        expect(c.visibleJobs.length).toBe(3);
        expect(c.hiddenJobCount).toBe(0);
    });

    test('work in progress is never capped away', () => {
        // A running download the cap pushed off the list would be uncancellable:
        // the panel is the only place its controls exist.
        const c = cockpitWithJobs(25, 10);
        c.jobs[0].status = 'downloading';
        expect(c.visibleJobs.map((j: any) => j.id)).toContain('job-0');
        expect(c.visibleJobs.length).toBe(11);
    });

    test('jobs the history page cannot show are never capped away', () => {
        // /downloads lists downloads. An export, an import or a plugin action that
        // drops off the panel has nowhere else to be — including its result link.
        const c = cockpitWithJobs(25, 10);
        c.jobs[0].source = 'group-export';
        c.jobs[1]._isAction = true;
        const visible = c.visibleJobs.map((j: any) => j.id);
        expect(visible).toContain('job-0');
        expect(visible).toContain('job-1');
        // The ten newest finished downloads still fit, and nothing else is dropped.
        expect(c.visibleJobs.length).toBe(12);
    });

    test('clearing and counting still see every job, not just the visible ones', () => {
        const c = cockpitWithJobs(25, 10);
        // hasFinishedJobs reads displayJobs: "Clear completed" must reach the rows
        // the cap hides, or a panel of 25 finished jobs would clear only 10.
        expect(c.hasFinishedJobs).toBe(true);
        c.jobs[0].status = 'downloading';
        expect(c.activeCount).toBe(1);
    });
});
