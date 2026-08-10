import { describe, expect, test, vi, beforeEach, afterEach } from 'vitest';
import { downloadsManager } from './downloadsManager.js';
import { downloadCockpit } from './downloadCockpit.js';

/**
 * The /downloads page component, and the jobs panel's row cap.
 *
 * Both are pure enough to pin here: selection is a set, the cap is a slice, and
 * the action calls are one fetch whose response shape decides what the user is
 * told. The Playwright spec covers that the template is wired to them.
 */

function rowCheckboxes(ids: number[]) {
    return ids.map(id => ({ value: String(id) }));
}

function mountManager(ids: number[] = [1, 2, 3]) {
    // `any` because these components are plain JS objects that Alpine augments at
    // runtime with $el/$refs/$nextTick; the test plays Alpine's part.
    const component: any = downloadsManager();
    // The component root, as init() captures it.
    component._root = { querySelectorAll: () => rowCheckboxes(ids) };
    // $el as a *method* sees it: the element that called the method — for the
    // select-all checkbox, an element with no rows beneath it. Reading rows from
    // $el instead of the captured root is what made select-all select nothing.
    component.$el = { querySelectorAll: () => [] };
    component.$nextTick = (fn: () => void) => fn();
    component._liveRegion = { announce: vi.fn(), destroy: vi.fn() };
    return component;
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
    test('toggling rows tracks the selection and the select-all state', () => {
        const c = mountManager([1, 2, 3]);
        expect(c.selectedCount).toBe(0);
        expect(c.allSelected).toBe(false);

        c.toggle(1, true);
        c.toggle(2, true);
        expect(c.selectedCount).toBe(2);
        expect(c.isSelected(1)).toBe(true);
        expect(c.allSelected).toBe(false);

        c.toggle(3, true);
        expect(c.allSelected).toBe(true);

        c.toggle(2, false);
        expect(c.selectedCount).toBe(2);
        expect(c.allSelected).toBe(false);
    });

    test('select-all with no rows is not "all selected"', () => {
        // Otherwise the header checkbox renders checked on an empty table, which
        // states that nothing is everything.
        const c = mountManager([]);
        expect(c.allSelected).toBe(false);
    });

    test('toggleAll selects exactly the rendered rows', () => {
        const c = mountManager([4, 5]);
        c.toggleAll(true);
        expect([...c.selected].sort()).toEqual([4, 5]);
        c.toggleAll(false);
        expect(c.selectedCount).toBe(0);
    });
});

describe('actions', () => {
    test('a partly refused batch reports both halves', async () => {
        const c = mountManager();
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
        const c = mountManager();
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
        const c = mountManager();
        vi.stubGlobal('fetch', vi.fn(async () => ({
            ok: false,
            json: async () => ({ error: 'this download is still running; cancel it first' }),
        })));

        await c.send('/v1/downloads/delete', [7], 'deleted');

        expect(c.error).toBe('this download is still running; cancel it first');
        expect(reloadCalls).toBe(0);
    });

    test('a network failure is reported rather than swallowed', async () => {
        const c = mountManager();
        vi.stubGlobal('fetch', vi.fn(async () => { throw new Error('offline'); }));
        vi.spyOn(console, 'error').mockImplementation(() => {});

        await c.send('/v1/downloads/retry', [1], 'retried');

        expect(c.error).toMatch(/request failed/i);
        expect(c.busy).toBe(false);
        expect(reloadCalls).toBe(0);
    });

    test('an empty selection sends nothing', async () => {
        const c = mountManager();
        const fetchMock = vi.fn();
        vi.stubGlobal('fetch', fetchMock);

        await c.send('/v1/downloads/delete', [], 'deleted');

        expect(fetchMock).not.toHaveBeenCalled();
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
