import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { jobPanel, panelCounts, panelCommandConfirmation, panelFinishedLimit } from './jobPanel.js';

afterEach(() => vi.unstubAllGlobals());

describe('Job Center panel', () => {
    test('counts the undismissed rows shown, including older actionable jobs', async () => {
        const olderActive = {
            id: 'older-active', state: 'running', version: 1,
            acceptedAt: '2020-01-01T00:00:00Z', commands: [{ key: 'pause' }],
        };
        const attention = {
            id: 'visible-attention', state: 'blocked', version: 1,
            acceptedAt: '2020-01-02T00:00:00Z', commands: [{ key: 'retry' }],
        };
        const panel = jobPanel();
        const requests: string[] = [];
        panel.requestJSON = vi.fn(async raw => {
            const url = String(raw);
            requests.push(url);
            if (url === '/v1/jobs/summary') {
                return { byState: { running: 20, blocked: 15 } };
            }
            const states = new URL(url, 'http://localhost').searchParams.getAll('state');
            return { jobs: states.includes('running') ? [olderActive] : states.includes('blocked') ? [attention] : [] };
        });

        await panel.refresh();

        expect(panel.jobs.map(job => job.id)).toEqual(['visible-attention', 'older-active']);
        expect(panelCounts(panel.jobs)).toEqual({ active: 1, attention: 1 });
        const listURLs = requests.filter(url => url.startsWith('/v1/jobs?'))
            .map(url => new URL(url, 'http://localhost'));
        expect(listURLs).toHaveLength(3);
        expect(listURLs.every(url => url.searchParams.get('dismissed') === 'false')).toBe(true);
        expect(listURLs.every(url => !url.searchParams.has('acceptedAfter'))).toBe(true);
        expect(requests).not.toContain('/v1/jobs/summary');
    });

    test('finished dismissal replaces blanket clear and explains pin/forget scope', () => {
        expect(panelCommandConfirmation({ key: 'dismiss', label: 'Dismiss' })).toMatch(/this job/i);
        expect(panelCommandConfirmation({ key: 'pin', label: 'Pin' })).toMatch(/artifacts.*own retention/i);
        expect(panelCommandConfirmation({ key: 'forget', label: 'Forget' })).toMatch(/artifacts are not affected/i);
    });

    test('links completed plugin actions to cached typed entities or historical summary destinations', () => {
        const panel = jobPanel();
        const entityJob = { id: 'fal-new', title: 'Create image', kind: 'plugin-action', state: 'succeeded' };
        const summary = {
            key: 'result', type: 'summary', availability: 'available', destinationUrl: '/resource?id=42',
            url: '/v1/jobs/fal-new/outputs?key=result',
        };
        const entity = {
            key: 'entity', type: 'entity', label: 'Resource', availability: 'available',
            url: '/v1/jobs/fal-new/outputs?key=entity',
        };
        panel.jobs = [entityJob];
        panel.details[entityJob.id] = { ...entityJob, outputs: [summary, entity] };

        expect(panel.resultOutput(entityJob)).toBe(entity);
        expect(panel.resultURL(entityJob)).toBe(entity.url);
        expect(panel.resultLinkLabel(entityJob)).toBe('View resource');
        expect(panel.resultAccessibleLabel(entityJob)).toBe('View resource for Create image');

        const historicalJob = { id: 'fal-old', title: 'Old action', kind: 'plugin-action', state: 'succeeded' };
        const historicalSummary = {
            key: 'result', type: 'summary', availability: 'available', destinationUrl: '/group?id=18',
            url: '/v1/jobs/fal-old/outputs?key=result',
        };
        panel.details[historicalJob.id] = { ...historicalJob, outputs: [historicalSummary] };

        expect(panel.resultOutput(historicalJob)).toBe(historicalSummary);
        expect(panel.resultURL(historicalJob)).toBe('/group?id=18');
        expect(panel.resultLinkLabel(historicalJob)).toBe('View result');
        expect(panel.resultAccessibleLabel(historicalJob)).toBe('View result for Old action');
    });

    test('only offers available safe result destinations for succeeded plugin actions', () => {
        const panel = jobPanel();
        const job = { id: 'fal-job', title: 'Create image', kind: 'plugin-action', state: 'succeeded' };
        panel.details[job.id] = { ...job, outputs: [{
            key: 'result', type: 'summary', availability: 'available',
            destinationUrl: 'javascript:alert(1)', url: '/v1/jobs/fal-job/outputs?key=result',
        }] };

        expect(panel.resultOutput(job)).toBeNull();
        expect(panel.resultURL(job)).toBe('');
        expect(panel.resultAccessibleLabel(job)).toBe('');

        panel.details[job.id].outputs[0].destinationUrl = '//example.com/result';
        expect(panel.resultOutput(job)).toBeNull();

        panel.details[job.id].outputs[0].destinationUrl = '/resource?id=42';
        panel.details[job.id].outputs[0].availability = 'expired';
        expect(panel.resultOutput(job)).toBeNull();

        expect(panel.resultOutput({ ...job, state: 'running' })).toBeNull();
        expect(panel.resultOutput({ ...job, kind: 'remote-download' })).toBeNull();
    });

    test('links a completed download to the resource it created', () => {
        const panel = jobPanel();
        const job = { id: 'dl', title: 'Download from example.com', kind: 'remote-download', state: 'succeeded' };
        const resource = {
            key: 'resource', type: 'entity', label: 'Created resource', availability: 'available',
            url: '/v1/jobs/dl/outputs?key=resource',
        };
        panel.jobs = [job];
        panel.details[job.id] = { ...job, outputs: [resource] };

        expect(panel.resultOutput(job)).toBe(resource);
        expect(panel.resultURL(job)).toBe(resource.url);
        expect(panel.resultLinkLabel(job)).toBe('View created resource');
        expect(panel.resultAccessibleLabel(job)).toBe('View created resource for Download from example.com');
        expect(panel.resultOutput({ ...job, state: 'running' })).toBeNull();
    });

    test('keeps the existing Cmd/Ctrl+Shift+D shortcut and toggles the dialog', () => {
        vi.stubGlobal('document', {
            activeElement: null,
            body: {},
            documentElement: {},
            querySelectorAll: () => [],
        });
        const panel = jobPanel();
        const event = { metaKey: true, ctrlKey: false, shiftKey: true, key: 'd', preventDefault: vi.fn() } as any;

        panel.handleShortcut(event);

        expect(event.preventDefault).toHaveBeenCalledOnce();
        expect(panel.isOpen).toBe(true);
    });

    test('connects to the canonical version 2 event stream', () => {
        class FakeEventSource {
            url: string;
            listeners = new Map<string, Function>();
            constructor(url: string) { this.url = url; }
            addEventListener(name: string, callback: Function) { this.listeners.set(name, callback); }
            close() {}
        }
        vi.stubGlobal('EventSource', FakeEventSource);
        const panel = jobPanel();

        panel.connect();

        expect(panel.eventSource?.url).toBe('/v1/jobs/events?version=2');
        expect(panel.eventSource?.listeners.has('job')).toBe(true);
        expect(panel.eventSource?.listeners.has('job-caught-up')).toBe(true);
    });

    test('refreshes resource lists once when a download Job succeeds', () => {
        const dispatchEvent = vi.fn();
        vi.stubGlobal('window', { dispatchEvent });
        vi.stubGlobal('CustomEvent', class {
            type: string;
            detail: unknown;
            constructor(type: string, init: { detail: unknown }) { this.type = type; this.detail = init.detail; }
        });
        const panel = jobPanel();
        panel.streamCaughtUp = true;
        panel.trackResourceCompletion({ id: 'download-1', kind: 'remote-download', state: 'running' });
        panel.trackResourceCompletion({ id: 'download-1', kind: 'remote-download', state: 'succeeded' });
        panel.trackResourceCompletion({ id: 'download-1', kind: 'remote-download', state: 'succeeded' });
        panel.trackResourceCompletion({ id: 'export-1', kind: 'group-export', state: 'succeeded' });
        expect(dispatchEvent).toHaveBeenCalledOnce();
        expect(dispatchEvent.mock.calls[0][0]).toMatchObject({ type: 'download-completed', detail: { jobId: 'download-1' } });
    });

    function dismissAllHarness(pages: Array<{ ids: string[]; nextCursor?: string }>, failIds: string[] = []) {
        const panel = jobPanel();
        panel.jobs = [{
            id: 'shown-1', state: 'succeeded', version: 7,
            commands: [{ key: 'dismiss', label: 'Dismiss', jobVersion: 7, bulk: true }],
        }];
        panel.details['shown-1'] = panel.jobs[0];
        const listURLs: URL[] = [];
        const posts: Array<{ url: string; init: any; body: any }> = [];
        const busyDuringRefresh: boolean[] = [];
        panel.requestJSON = vi.fn(async (raw: string, init: any = {}) => {
            const url = String(raw);
            if (init.method === 'POST') {
                const body = JSON.parse(init.body);
                posts.push({ url, init, body });
                return {
                    results: body.jobIds.map((jobId: string) => failIds.includes(jobId)
                        ? { jobId, key: 'dismiss', status: 'failed', code: 'conflict', message: 'the job changed; try again' }
                        : { jobId, key: 'dismiss', status: 'succeeded', code: 'applied' }),
                };
            }
            const parsed = new URL(url, 'http://localhost');
            if (parsed.searchParams.get('limit') !== '200') busyDuringRefresh.push(panel.busy);
            if (parsed.searchParams.get('limit') === '200') {
                listURLs.push(parsed);
                const page = pages[listURLs.length - 1] || { ids: [] };
                return { jobs: page.ids.map(id => ({ id, state: 'succeeded', version: 1 })), nextCursor: page.nextCursor };
            }
            return { jobs: [] };
        });
        return { panel, listURLs, posts, busyDuringRefresh };
    }

    test('dismisses every finished job, not only the ones the panel shows', async () => {
        const first = Array.from({ length: 200 }, (_, index) => `job-${index}`);
        const second = ['job-200', 'job-201', 'job-202'];
        const { panel, listURLs, posts, busyDuringRefresh } = dismissAllHarness([{ ids: first, nextCursor: 'c1' }, { ids: second }]);

        const outcome = await panel.dismissFinished();

        expect(listURLs).toHaveLength(2);
        for (const url of listURLs) {
            expect(url.pathname).toBe('/v1/jobs');
            expect(url.searchParams.getAll('state')).toEqual(['succeeded', 'cancelled']);
            expect(url.searchParams.get('dismissed')).toBe('false');
        }
        expect(listURLs[0].searchParams.has('cursor')).toBe(false);
        expect(listURLs[1].searchParams.get('cursor')).toBe('c1');
        expect(posts.map(post => post.url)).toEqual(['/v1/jobs/commands/dismiss', '/v1/jobs/commands/dismiss']);
        expect(posts.flatMap(post => post.body.jobIds)).toEqual([...first, ...second]);
        const keys = posts.map(post => post.init.headers['Idempotency-Key']);
        expect(new Set(keys).size).toBe(2);
        expect(outcome).toEqual({ dismissed: 203, total: 203 });
        expect(panel.notice).toBe('203 finished jobs dismissed.');
        // The refresh that replaces the stale rows runs while the run still owns
        // the button, so a second click cannot start over the finished one.
        expect(busyDuringRefresh.length).toBeGreaterThan(0);
        expect(busyDuringRefresh.every(Boolean)).toBe(true);
        expect(panel.busy).toBe(false);
        await expect(panel.dismissFinished()).resolves.toEqual({ dismissed: 0, total: 0 });
    });

    test('reports a partial dismissal as a count, not a list of ids', async () => {
        const { panel } = dismissAllHarness([{ ids: ['a', 'b', 'c'] }], ['b']);

        await panel.dismissFinished();

        expect(panel.notice).toBe('2 of 3 finished jobs dismissed. Not dismissed: the job changed; try again');
        expect('outcomes' in panel).toBe(false);
    });

    test('removes the shown rows it dismissed even when the refresh fails', async () => {
        const { panel } = dismissAllHarness([{ ids: ['shown-1', 'b'] }]);
        const answer = panel.requestJSON;
        panel.requestJSON = vi.fn(async (url: string, init: any = {}) => {
            if (!init.method && new URL(url, 'http://localhost').searchParams.get('limit') !== '200') {
                throw new Error('Request failed (503)');
            }
            return answer(url, init);
        });

        await panel.dismissFinished();

        expect(panel.notice).toBe('2 finished jobs dismissed.');
        expect(panel.jobs.map(job => job.id)).not.toContain('shown-1');
        expect(panel.finishedCount).toBe(0);
        expect(panel.busy).toBe(false);
    });

    test('removes a row a mid-run refresh brought in once its page is dismissed', async () => {
        const { panel } = dismissAllHarness([{ ids: ['shown-1'], nextCursor: 'c1' }, { ids: ['older-1'] }]);
        const answer = panel.requestJSON;
        let lists = 0;
        panel.requestJSON = vi.fn(async (url: string, init: any = {}) => {
            const limit = !init.method && new URL(url, 'http://localhost').searchParams.get('limit');
            if (limit && limit !== '200') throw new Error('Request failed (503)');
            // A stream-driven refresh lands between the pages and shows an older
            // finished row the run has not reached yet.
            if (limit === '200' && ++lists === 2) {
                panel.jobs = [{ id: 'older-1', state: 'succeeded', version: 1, commands: [{ key: 'dismiss', bulk: true }] }];
            }
            return answer(url, init);
        });

        await panel.dismissFinished();

        expect(panel.notice).toBe('2 finished jobs dismissed.');
        expect(panel.jobs).toEqual([]);
        expect(panel.finishedCount).toBe(0);
    });

    test('discards a refresh issued before a page was dismissed', async () => {
        const { panel } = dismissAllHarness([{ ids: ['shown-1'], nextCursor: 'c1' }, { ids: ['b'] }]);
        const answer = panel.requestJSON;
        const shownRow = panel.jobs[0];
        let releaseStale: () => void = () => {};
        const staleHeld = new Promise<void>(resolve => { releaseStale = resolve; });
        let staleRefresh: Promise<unknown> | null = null;
        let lists = 0;
        let refreshes = 0;
        panel.requestJSON = vi.fn(async (url: string, init: any = {}) => {
            const limit = !init.method && new URL(url, 'http://localhost').searchParams.get('limit');
            if (limit && limit !== '200') {
                // The first refresh is a stream-driven one whose answer predates
                // the dismissal; the final refresh fails.
                if (++refreshes <= 3) {
                    await staleHeld;
                    return { jobs: new URL(url, 'http://localhost').searchParams.getAll('state').includes('succeeded') ? [shownRow] : [] };
                }
                throw new Error('Request failed (503)');
            }
            if (init.method === 'POST' && !staleRefresh) staleRefresh = panel.refresh();
            // The stale answer lands while the next page is being read.
            if (limit === '200' && ++lists === 2) releaseStale();
            return answer(url, init);
        });

        await panel.dismissFinished();
        await staleRefresh;

        expect(panel.notice).toBe('2 finished jobs dismissed.');
        expect(panel.jobs).toEqual([]);
    });

    test('keeps an earlier refusal when a later page fails', async () => {
        const { panel } = dismissAllHarness([{ ids: ['a', 'b'], nextCursor: 'c1' }], ['b']);
        const answer = panel.requestJSON;
        let lists = 0;
        panel.requestJSON = vi.fn(async (url: string, init: any = {}) => {
            if (!init.method && new URL(url, 'http://localhost').searchParams.get('limit') === '200' && ++lists === 2) {
                throw new Error('Request failed (500)');
            }
            return answer(url, init);
        });

        await panel.dismissFinished();

        expect(panel.notice).toBe('1 finished job dismissed before an error: Request failed (500). Not dismissed: the job changed; try again');
    });

    test('a command answer arriving after its row was dismissed does not bring it back', async () => {
        const panel = jobPanel();
        panel.jobs = [];
        panel.applyStreamSnapshot({ id: 'gone', state: 'succeeded', version: 9, pinned: false });
        expect(panel.jobs).toEqual([]);
    });

    test('keeps a shown row the server refused to dismiss', async () => {
        const { panel } = dismissAllHarness([{ ids: ['shown-1'] }], ['shown-1']);
        panel.refresh = vi.fn(async () => {});

        await panel.dismissFinished();

        expect(panel.notice).toBe('0 of 1 finished job dismissed. Not dismissed: the job changed; try again');
        expect(panel.jobs.map(job => job.id)).toEqual(['shown-1']);
    });

    test('says how many were dismissed when a later page fails', async () => {
        const { panel } = dismissAllHarness([{ ids: ['a', 'b'], nextCursor: 'c1' }]);
        const answer = panel.requestJSON;
        let lists = 0;
        panel.requestJSON = vi.fn(async (url: string, init: any = {}) => {
            if (!init.method && new URL(url, 'http://localhost').searchParams.get('limit') === '200' && ++lists === 2) {
                throw new Error('Request failed (500)');
            }
            return answer(url, init);
        });

        const outcome = await panel.dismissFinished();

        expect(outcome).toEqual({ dismissed: 2, total: 2 });
        expect(panel.notice).toBe('2 finished jobs dismissed before an error: Request failed (500)');
        expect(panel.busy).toBe(false);
    });

    test('refreshes viewer pin state after pinning from the panel', async () => {
        const panel = jobPanel();
        const job = { id: 'job-1', state: 'succeeded', version: 4, pinned: false, commands: [{ key: 'pin', label: 'Pin', jobVersion: 4 }] };
        panel.jobs = [job];
        vi.stubGlobal('Alpine', { store: () => ({ ask: vi.fn(async () => true) }) });
        panel.requestJSON = vi.fn(async (url: string, init: any = {}) => {
            if (init.method === 'POST') return { result: { message: 'Pinned' } };
            return { id: 'job-1', state: 'succeeded', version: 4, pinned: true, commands: [
                { key: 'pin', label: 'Pin', jobVersion: 4 }, { key: 'unpin', label: 'Unpin', jobVersion: 4 },
            ] };
        });

        await panel.runCommand(job, job.commands[0]);

        expect(panel.requestJSON).toHaveBeenCalledTimes(2);
        expect(panel.requestJSON.mock.calls[1][0]).toBe('/v1/jobs/job-1');
        expect(panel.jobs[0].pinned).toBe(true);
        expect(panel.commandsFor(panel.jobs[0]).map(command => command.key)).toEqual(['unpin']);
    });
});

describe('Job Center drawer live progress', () => {
    test('asks for the series only for open work, and takes the finished limit from the page', async () => {
        const panel = jobPanel();
        panel.finishedLimit = 7;
        const requests: URL[] = [];
        panel.requestJSON = vi.fn(async raw => {
            const url = new URL(String(raw), 'http://localhost');
            if (url.pathname === '/v1/jobs') requests.push(url);
            const states = url.searchParams.getAll('state');
            return states.includes('succeeded') ? { jobs: [], nextCursor: 'more' } : { jobs: [] };
        });
        await panel.refresh();
        const byGroup = Object.fromEntries(requests.map(url => [url.searchParams.getAll('state')[0], url.searchParams]));
        expect(byGroup.scheduled.get('include')).toBe('progressSeries');
        expect(byGroup.blocked.has('include')).toBe(false);
        expect(byGroup.succeeded.has('include')).toBe(false);
        expect(byGroup.succeeded.get('limit')).toBe('7');
        expect(panel.finishedHasMore).toBe(true);

        const meta = { getAttribute: () => '25' };
        expect(panelFinishedLimit({ querySelector: () => meta } as any)).toBe(25);
        expect(panelFinishedLimit({ querySelector: () => null } as any)).toBe(10);
        // The list API refuses a page over 200, which would blank the drawer.
        expect(panelFinishedLimit({ querySelector: () => ({ getAttribute: () => '5000' }) } as any)).toBe(200);
        expect(panelFinishedLimit({ querySelector: () => ({ getAttribute: () => 'zero' }) } as any)).toBe(10);
    });

    test('a progress frame updates its row in place, never refetches, inserts or announces', () => {
        const panel = jobPanel();
        panel._liveRegion = { announce: vi.fn(), destroy: vi.fn() } as any;
        panel.requestJSON = vi.fn();
        const scheduled = vi.spyOn(panel, 'schedulePanelRefresh');
        panel.jobs = [{
            id: 'job-1', title: 'Download', kind: 'remote-download', state: 'running', version: 4,
            acceptedAt: '2026-09-25T10:00:00Z',
            progress: { completed: 100, total: 1000, unit: 'bytes', series: { intervalMs: 1000, unit: 'bytes', points: [{ t: 1000, c: 100 }] } },
        }];

        panel.handleProgressFrame({ data: JSON.stringify({
            jobId: 'job-1', version: 4, state: 'running', intervalMs: 1000,
            progress: { completed: 600, total: 1000, unit: 'bytes', rate: 500, eta: '2099-01-01T00:00:00Z', etaEstimated: true,
                metrics: [{ key: 'segments', label: 'Segments', value: 6, total: 10, unit: 'items', graph: true }] },
            point: { t: 2000, c: 600, r: 500, v: { segments: 6 } },
        }) });
        panel.handleProgressFrame({ data: JSON.stringify({ jobId: 'unlisted', version: 1, progress: { completed: 1 } }) });
        panel.handleProgressFrame({ data: 'not json' });

        expect(panel.jobs).toHaveLength(1);
        const [job] = panel.jobs;
        expect(job.version).toBe(4);
        expect(job.progress.completed).toBe(600);
        expect(job.progress.series.points).toHaveLength(2);
        expect(panel.progressValue(job)).toBe(60);
        expect(panel.statsText(job)).toMatch(/^600 B of 1000 B · 500 B\/s · about /);
        expect(panel.metricText(panel.metricsFor(job)[0])).toBe('6 of 10');
        expect(panel.graphsFor(job).map(series => series.key)).toEqual([':speed', 'segments']);
        expect(panel.progressValueText(job)).toMatch(/^60%, 600 B of 1000 B, 500 B\/s, about /);
        expect(panel.requestJSON).not.toHaveBeenCalled();
        expect(scheduled).not.toHaveBeenCalled();
        expect(panel._liveRegion.announce).not.toHaveBeenCalled();
    });

    test('a finished row shows its average speed and no graph', () => {
        const panel = jobPanel();
        const job = {
            id: 'done', state: 'succeeded', version: 2,
            progress: { completed: 10, total: 10, unit: 'items', averageRate: 2.5,
                series: { unit: 'items', points: [{ t: 0, c: 0 }, { t: 4000, c: 10, r: 2.5 }] } },
        };
        expect(panel.rateText(job)).toBe('average 2.5/s');
        expect(panel.etaText(job)).toBe('');
        expect(panel.graphsFor(job)).toEqual([]);
        expect(panel.showsProgress(job)).toBe(false);
    });

    test('a list refresh answered before the latest frame keeps the newer progress', async () => {
        const panel = jobPanel();
        panel.jobs = [{ id: 'job-1', state: 'running', version: 4, acceptedAt: '2026-09-25T10:00:00Z',
            progress: { completed: 900, updatedAt: '2026-09-25T10:00:09Z' } }];
        panel.requestJSON = vi.fn(async raw => {
            const url = new URL(String(raw), 'http://localhost');
            if (url.pathname !== '/v1/jobs') return { id: 'job-1', commands: [] };
            return { jobs: url.searchParams.getAll('state').includes('running')
                ? [{ id: 'job-1', state: 'running', version: 4, acceptedAt: '2026-09-25T10:00:00Z',
                    progress: { completed: 300, updatedAt: '2026-09-25T10:00:03Z' } }] : [] };
        });
        await panel.refresh();
        expect(panel.jobs[0].progress.completed).toBe(900);
    });

    test('only running work pulses', () => {
        const panel = jobPanel();
        expect(panel.progressIndeterminate({ state: 'running', progress: {} })).toBe(true);
        expect(panel.progressIndeterminate({ state: 'paused', progress: {} })).toBe(false);
    });

    test('groups rows by what a person does next, leaving empty groups out', () => {
        const panel = jobPanel();
        panel.jobs = [
            { id: 'r', state: 'running', acceptedAt: '2026-09-25T10:00:03Z' },
            { id: 'f', state: 'failed', acceptedAt: '2026-09-25T10:00:02Z' },
        ];
        expect(panel.groups.map(group => [group.key, group.jobs.map(job => job.id)])).toEqual([
            ['attention', ['f']], ['active', ['r']],
        ]);
    });
});

describe('Job Center panel accessibility hooks', () => {
    beforeEach(() => {
        vi.stubGlobal('document', {
            addEventListener: vi.fn(),
            removeEventListener: vi.fn(),
            querySelector: vi.fn(() => null),
            body: { appendChild: vi.fn() },
        });
        vi.stubGlobal('window', { location: { href: '/jobs' }, addEventListener: vi.fn(), removeEventListener: vi.fn() });
    });

    afterEach(() => {
        vi.unstubAllGlobals();
    });

    test('does not announce the initial timeline snapshot as new work', () => {
        const panel = jobPanel();
        panel._liveRegion = { announce: vi.fn(), destroy: vi.fn() } as any;
        const job = { id: 'job-1', state: 'succeeded', version: 2, acceptedAt: '2026-09-22T10:00:00Z' };
        vi.stubGlobal('fetch', vi.fn(async url => ({
            ok: true,
            json: async () => String(url) === '/v1/jobs/summary' ? { byState: { succeeded: 1 } }
                : String(url).startsWith('/v1/jobs/') ? { ...job, commands: [] }
                    : { jobs: [job] },
        })));

        return panel.refresh().then(() => {
            expect(panel.jobs).toHaveLength(1);
            expect(panel._liveRegion.announce).not.toHaveBeenCalled();
        });
    });

    test('announces a newly delivered lifecycle outcome from a complete stream snapshot', () => {
        const panel = jobPanel();
        panel._liveRegion = { announce: vi.fn(), destroy: vi.fn() } as any;
        panel.jobs = [{ id: 'job-1', title: 'Index rebuild', kind: 'maintenance', state: 'running', version: 2 }];
        const fetchMock = vi.fn();
        vi.stubGlobal('fetch', fetchMock);
        panel.markStreamCaughtUp({ data: JSON.stringify({ cursor: 'v2:14' }) });
        return panel.handleStreamMessage({
            data: JSON.stringify({
                job: { id: 'job-1', title: 'Index rebuild', kind: 'maintenance', state: 'failed', version: 3 },
                sequence: 3, deliverySequence: 15,
            }),
            lastEventId: 'v2:15',
        }).then(() => {
            expect(panel._liveRegion.announce).toHaveBeenCalledWith(expect.stringMatching(/Index rebuild.*failed/i));
            expect(fetchMock).not.toHaveBeenCalled();
            expect(panel.lastSequence).toBe(15);
        });
    });

    test('announces a canonical event-only state change after its bounded refresh', async () => {
        vi.useFakeTimers();
        const panel = jobPanel();
        panel.streamCaughtUp = true;
        panel.jobs = [{ id: 'export-1', title: 'Export', kind: 'export', state: 'running', version: 1, acceptedAt: '2026-09-23T10:00:00Z' }];
        panel._liveRegion = { announce: vi.fn(), destroy: vi.fn() } as any;
        const listRequests: string[] = [];
        panel.requestJSON = vi.fn(async raw => {
            const url = String(raw);
            if (url === '/v1/jobs/summary') return { byState: { failed: 1 } };
            if (url.startsWith('/v1/jobs?')) {
                listRequests.push(url);
                const query = new URL(url, 'http://localhost').searchParams;
                return { jobs: query.getAll('state').includes('blocked') ? [
                    { id: 'export-1', title: 'Export', kind: 'export', state: 'failed', version: 2, acceptedAt: '2026-09-23T10:00:00Z' },
                ] : [] };
            }
            return { id: 'export-1', title: 'Export', kind: 'export', state: 'failed', version: 2, commands: [] };
        });

        await Promise.all(Array.from({ length: 100 }, (_, index) => panel.handleStreamMessage({
            data: JSON.stringify({
                id: `event-${index + 1}`, jobId: 'export-1', sequence: index + 2, jobVersion: 2,
                type: 'state-change', deliverySequence: index + 1, createdAt: '2026-09-23T10:01:00Z',
            }),
            lastEventId: `v2:${index + 1}`,
        })));

        expect(panel._liveRegion.announce).not.toHaveBeenCalled();
        await vi.advanceTimersByTimeAsync(150);
        await panel._panelRefreshPromise;

        expect(listRequests).toHaveLength(3);
        expect(panel.jobs[0]).toMatchObject({ id: 'export-1', state: 'failed' });
        expect(panel._liveRegion.announce).toHaveBeenCalledOnce();
        expect(panel._liveRegion.announce).toHaveBeenCalledWith('Export failed.');
        panel.destroy();
        vi.useRealTimers();
    });

    test('does not announce a replay event when catch-up refresh shows a state change', async () => {
        vi.useFakeTimers();
        const panel = jobPanel();
        panel.jobs = [{ id: 'export-1', title: 'Export', kind: 'export', state: 'running', version: 1, acceptedAt: '2026-09-23T10:00:00Z' }];
        panel._liveRegion = { announce: vi.fn(), destroy: vi.fn() } as any;
        panel.requestJSON = vi.fn(async raw => {
            const url = String(raw);
            if (url === '/v1/jobs/summary') return { byState: { failed: 1 } };
            if (url.startsWith('/v1/jobs?')) {
                const query = new URL(url, 'http://localhost').searchParams;
                return { jobs: query.getAll('state').includes('failed') ? [
                    { id: 'export-1', title: 'Export', kind: 'export', state: 'failed', version: 2, acceptedAt: '2026-09-23T10:00:00Z' },
                ] : [] };
            }
            return { id: 'export-1', title: 'Export', kind: 'export', state: 'failed', version: 2, commands: [] };
        });

        await panel.handleStreamMessage({
            data: JSON.stringify({ id: 'event-1', jobId: 'export-1', jobVersion: 2, type: 'state-change', deliverySequence: 1 }),
            lastEventId: 'v2:1',
        });
        panel.markStreamCaughtUp({ data: JSON.stringify({ cursor: 'v2:1' }) });
        await vi.advanceTimersByTimeAsync(150);
        await panel._panelRefreshPromise;

        expect(panel.jobs[0]).toMatchObject({ id: 'export-1', state: 'failed' });
        expect(panel._liveRegion.announce).not.toHaveBeenCalled();
        panel.destroy();
        vi.useRealTimers();
    });

    test('does not announce a failure first observed during replay after reconnect', async () => {
        vi.useFakeTimers();
        class FakeEventSource {
            listeners = new Map<string, Function>();
            constructor(public url: string) {}
            addEventListener(name: string, callback: Function) { this.listeners.set(name, callback); }
            close() {}
        }
        vi.stubGlobal('EventSource', FakeEventSource);
        const panel = jobPanel();
        panel.jobs = [{ id: 'export-1', title: 'Export', kind: 'export', state: 'running', version: 1, acceptedAt: '2026-09-23T10:00:00Z' }];
        panel._liveRegion = { announce: vi.fn(), destroy: vi.fn() } as any;
        panel.requestJSON = vi.fn(async raw => {
            const url = String(raw);
            if (url === '/v1/jobs/summary') return { byState: { failed: 1 } };
            if (url.startsWith('/v1/jobs?')) {
                const query = new URL(url, 'http://localhost').searchParams;
                return { jobs: query.getAll('state').includes('failed') ? [
                    { id: 'export-1', title: 'Export', kind: 'export', state: 'failed', version: 2, acceptedAt: '2026-09-23T10:00:00Z' },
                ] : [] };
            }
            return { id: 'export-1', title: 'Export', kind: 'export', state: 'failed', version: 2, commands: [] };
        });
        panel.connect();
        const stream = panel.eventSource as unknown as FakeEventSource;
        panel.streamCaughtUp = true;

        await panel.handleStreamMessage({
            data: JSON.stringify({ id: 'progress-1', jobId: 'export-1', sequence: 2, jobVersion: 1, type: 'progress', deliverySequence: 1 }),
            lastEventId: 'v2:1',
        });
        expect(panel._liveRegion.announce).not.toHaveBeenCalled();

        stream.listeners.get('error')?.({});
        await panel.handleStreamMessage({
            data: JSON.stringify({ id: 'failed-1', jobId: 'export-1', sequence: 3, jobVersion: 2, type: 'state-change', deliverySequence: 2 }),
            lastEventId: 'v2:2',
        });
        stream.listeners.get('job-caught-up')?.({ data: JSON.stringify({ cursor: 'v2:2' }) });
        await vi.advanceTimersByTimeAsync(150);
        await panel._panelRefreshPromise;

        expect(panel.jobs[0]).toMatchObject({ id: 'export-1', state: 'failed' });
        expect(panel._liveRegion.announce).not.toHaveBeenCalled();
        panel.destroy();
        vi.useRealTimers();
    });

    test('announces only events after the catch-up boundary, including after reconnect', () => {
        class FakeEventSource {
            listeners = new Map<string, Function>();
            constructor(public url: string) {}
            addEventListener(name: string, callback: Function) { this.listeners.set(name, callback); }
            close() {}
        }
        vi.stubGlobal('EventSource', FakeEventSource);
        const panel = jobPanel();
        panel.jobs = [{ id: 'job-1', title: 'Index rebuild', kind: 'maintenance', state: 'queued', version: 1 }];
        panel._liveRegion = { announce: vi.fn(), destroy: vi.fn() } as any;
        panel.connect();
        const stream = panel.eventSource as unknown as FakeEventSource;
        const sendJob = (state: string, version: number, sequence: number) => stream.listeners.get('job')?.({
            data: JSON.stringify({ id: 'job-1', title: 'Index rebuild', kind: 'maintenance', state, version, deliverySequence: sequence }),
            lastEventId: `v2:${sequence}`,
        });

        sendJob('running', 2, 10);
        expect(panel._liveRegion.announce).not.toHaveBeenCalled();
        stream.listeners.get('job-caught-up')?.({ data: JSON.stringify({ cursor: 'v2:10' }) });
        sendJob('failed', 3, 11);
        expect(panel._liveRegion.announce).toHaveBeenCalledTimes(1);

        stream.listeners.get('error')?.({});
        sendJob('succeeded', 4, 12);
        expect(panel._liveRegion.announce).toHaveBeenCalledTimes(1);
        stream.listeners.get('job-caught-up')?.({ data: JSON.stringify({ cursor: 'v2:12' }) });
        sendJob('cancelled', 5, 13);
        expect(panel._liveRegion.announce).toHaveBeenCalledTimes(2);
    });

    test('does not announce replay events or apply page details loaded before the catch-up boundary', async () => {
        let resolveDetail: (value: unknown) => void = () => {};
        let markDetailStarted: () => void = () => {};
        const detailRequest = new Promise(resolve => { resolveDetail = resolve; });
        const detailStarted = new Promise<void>(resolve => { markDetailStarted = resolve; });
        const panel = jobPanel();
        const visibleJob = { id: 'job-1', title: 'Index rebuild', state: 'queued', version: 1, acceptedAt: '2026-09-23T10:00:00Z' };
        panel._liveRegion = { announce: vi.fn(), destroy: vi.fn() } as any;
        panel.requestJSON = vi.fn(async raw => {
            const url = String(raw);
            if (url === '/v1/jobs/summary') return { byState: { queued: 1 } };
            if (url.startsWith('/v1/jobs?')) {
                const query = new URL(url, 'http://localhost').searchParams;
                return { jobs: query.getAll('state').includes('queued') ? [visibleJob] : [] };
            }
            markDetailStarted();
            return detailRequest;
        });

        const refresh = panel.refresh();
        await detailStarted;
        const event = panel.handleStreamMessage({
            data: JSON.stringify({ id: 'event-1', jobId: 'job-1', type: 'failed', sequence: 2, deliverySequence: 1 }),
            lastEventId: 'v2:1',
        });
        panel.markStreamCaughtUp({ data: JSON.stringify({ cursor: 'v2:1' }) });
        resolveDetail({ id: 'job-1', title: 'Index rebuild', state: 'failed', version: 2 });
        await Promise.all([event, refresh]);

        expect(panel._liveRegion.announce).not.toHaveBeenCalled();
        expect(panel.commandsFor(panel.jobs[0])).toEqual([]);
        panel.destroy();
    });

    test('ignores unknown replay detail fetches, refreshes bounded pages after catch-up, and caps live rows', async () => {
        vi.useFakeTimers();
        class FakeEventSource {
            listeners = new Map<string, Function>();
            constructor(public url: string) {}
            addEventListener(name: string, callback: Function) { this.listeners.set(name, callback); }
            close() {}
        }
        vi.stubGlobal('EventSource', FakeEventSource);
        const panel = jobPanel();
        const detailRequests: string[] = [];
        const listRequests: string[] = [];
        const pageRows = new Map<string, unknown[]>();
        for (const [state, count] of [['blocked', 5], ['running', 5], ['succeeded', 5]] as const) {
            pageRows.set(state, Array.from({ length: count }, (_, index) => ({
                id: `page-${state}-${index}`,
                kind: 'maintenance',
                state,
                version: 1,
                acceptedAt: `2026-09-23T10:${String(index).padStart(2, '0')}:00Z`,
            })));
        }
        panel.requestJSON = vi.fn(async raw => {
            const url = String(raw);
            if (url.startsWith('/v1/jobs?')) {
                listRequests.push(url);
                const query = new URL(url, 'http://localhost').searchParams;
                return { jobs: query.getAll('state').flatMap(state => pageRows.get(state) || []) };
            }
            detailRequests.push(url);
            const id = decodeURIComponent(url.slice('/v1/jobs/'.length));
            const state = id.startsWith('page-blocked-') ? 'blocked' : id.startsWith('page-running-') ? 'running' : 'succeeded';
            return { id, kind: 'maintenance', state, version: 1, commands: [] };
        });
        panel.connect();
        const stream = panel.eventSource as unknown as FakeEventSource;
        const replay = Array.from({ length: 100 }, (_, index) => panel.handleStreamMessage({
            data: JSON.stringify({
                id: `event-${index + 1}`, jobId: `historic-${index + 1}`, sequence: index + 1,
                jobVersion: 1, type: 'queued', deliverySequence: index + 1,
            }),
            lastEventId: `v2:${index + 1}`,
        }));
        await Promise.all(replay);

        expect(detailRequests).toEqual([]);
        expect(panel.jobs).toEqual([]);

        stream.listeners.get('job-caught-up')?.({ data: JSON.stringify({ cursor: 'v2:100' }) });
        await vi.advanceTimersByTimeAsync(150);
        await panel._panelRefreshPromise;

        expect(listRequests).toHaveLength(3);
        expect(listRequests.map(url => new URL(url, 'http://localhost').searchParams.get('limit'))).toEqual(['50', '50', '10']);
        expect(panel.jobs).toHaveLength(15);
        expect(detailRequests).toHaveLength(15);
        expect(detailRequests.some(path => path.includes('historic-'))).toBe(false);
        expect(panel.counts).toEqual({ active: 5, attention: 5 });

        for (let index = 0; index < 25; index++) {
            await panel.handleStreamMessage({
                data: JSON.stringify({
                    id: `live-${index}`, title: `Live ${index}`, kind: 'maintenance', state: 'running',
                    version: 1, acceptedAt: `2026-09-23T12:${String(index).padStart(2, '0')}:00Z`,
                    deliverySequence: 101 + index,
                }),
                lastEventId: `v2:${101 + index}`,
            });
            expect(panel.activeJobs.length).toBeLessThanOrEqual(50);
            expect(panel.attentionJobs).toHaveLength(5);
            expect(panel.finishedJobs).toHaveLength(5);
        }

        stream.listeners.get('error')?.({});
        await Promise.all(Array.from({ length: 3 }, (_, index) => panel.handleStreamMessage({
            data: JSON.stringify({
                id: `replay-${index}`, jobId: `reconnected-history-${index}`, deliverySequence: 126 + index,
            }),
            lastEventId: `v2:${126 + index}`,
        })));
        expect(detailRequests).toHaveLength(15);
        stream.listeners.get('job-caught-up')?.({ data: JSON.stringify({ cursor: 'v2:128' }) });
        await vi.advanceTimersByTimeAsync(150);
        await panel._panelRefreshPromise;
        expect(listRequests).toHaveLength(6);
        expect(listRequests.slice(3).map(url => new URL(url, 'http://localhost').searchParams.get('limit'))).toEqual(['50', '50', '10']);
        expect(panel.counts).toEqual({ active: 5, attention: 5 });
        expect(panel.jobs).toHaveLength(15);
        panel.destroy();
        vi.useRealTimers();
    });

    test('refreshes the bounded panel for unknown live events instead of fetching every Job detail', async () => {
        vi.useFakeTimers();
        const panel = jobPanel();
        panel.streamCaughtUp = true;
        const detailRequests: string[] = [];
        const listRequests: string[] = [];
        const visibleRows = ['blocked', 'running', 'succeeded'].flatMap(state => Array.from({ length: 5 }, (_, index) => ({
            id: `visible-${state}-${index}`,
            kind: 'maintenance',
            state,
            version: 1,
            acceptedAt: `2026-09-23T10:${String(index).padStart(2, '0')}:00Z`,
        })));
        panel.requestJSON = vi.fn(async raw => {
            const url = String(raw);
            if (url === '/v1/jobs/summary') return { byState: { running: 5 } };
            if (url.startsWith('/v1/jobs?')) {
                listRequests.push(url);
                const query = new URL(url, 'http://localhost').searchParams;
                return { jobs: visibleRows.filter(job => query.getAll('state').includes(job.state)) };
            }
            detailRequests.push(url);
            const id = decodeURIComponent(url.slice('/v1/jobs/'.length));
            return { id, kind: 'maintenance', state: 'running', version: 1, commands: [] };
        });

        await Promise.all(Array.from({ length: 1000 }, (_, index) => panel.handleStreamMessage({
            data: JSON.stringify({
                id: `event-${index + 1}`, jobId: `live-unknown-${index + 1}`,
                type: 'state-change', deliverySequence: index + 1,
            }),
            lastEventId: `v2:${index + 1}`,
        })));

        await panel.handleStreamMessage({
            data: JSON.stringify({
                id: 'event-1001', deliverySequence: 1001,
                job: { id: 'snapshot-only', kind: 'maintenance', state: 'running', version: 1 },
            }),
            lastEventId: 'v2:1001',
        });

        expect(detailRequests).toEqual([]);
        expect(panel.jobs.some(job => job.id === 'snapshot-only')).toBe(false);
        await vi.advanceTimersByTimeAsync(150);
        await panel._panelRefreshPromise;
        expect(listRequests).toHaveLength(3);
        expect(detailRequests).toHaveLength(15);
        expect(panel.jobs).toHaveLength(15);
        panel.destroy();
        vi.useRealTimers();
    });

    test('coalesces event-only bursts for one visible Job into a bounded page refresh', async () => {
        vi.useFakeTimers();
        const panel = jobPanel();
        panel.streamCaughtUp = true;
        const visibleJob = { id: 'visible-job', kind: 'maintenance', state: 'running', version: 1, acceptedAt: '2026-09-23T10:00:00Z' };
        const detailRequests: string[] = [];
        const listRequests: string[] = [];
        panel.jobs = [visibleJob];
        panel.requestJSON = vi.fn(async raw => {
            const url = String(raw);
            if (url === '/v1/jobs/summary') return { byState: { running: 1 } };
            if (url.startsWith('/v1/jobs?')) {
                listRequests.push(url);
                const query = new URL(url, 'http://localhost').searchParams;
                return { jobs: query.getAll('state').includes('running') ? [visibleJob] : [] };
            }
            detailRequests.push(url);
            return { ...visibleJob, commands: [{ key: 'pause', label: 'Pause', jobVersion: 1 }] };
        });

        await Promise.all(Array.from({ length: 1000 }, (_, index) => panel.handleStreamMessage({
            data: JSON.stringify({
                id: `same-job-event-${index + 1}`, jobId: 'visible-job',
                type: 'state-change', deliverySequence: index + 1,
            }),
            lastEventId: `v2:${index + 1}`,
        })));

        expect(detailRequests).toEqual([]);
        await vi.advanceTimersByTimeAsync(150);
        await panel._panelRefreshPromise;

        expect(listRequests).toHaveLength(3);
        expect(detailRequests).toEqual(['/v1/jobs/visible-job']);
        expect(panel.commandsFor(panel.jobs[0])).toEqual([{ key: 'pause', label: 'Pause', jobVersion: 1 }]);
        panel.destroy();
        vi.useRealTimers();
    });

    test('does not apply a page detail response after a newer page refresh excludes that job', async () => {
        let resolveDetail: (value: unknown) => void = () => {};
        let markDetailStarted: () => void = () => {};
        const detailRequest = new Promise(resolve => { resolveDetail = resolve; });
        const detailStarted = new Promise<void>(resolve => { markDetailStarted = resolve; });
        const panel = jobPanel();
        const visibleJob = { id: 'job-1', title: 'Old job', state: 'running', version: 1, acceptedAt: '2026-09-23T10:00:00Z' };
        let pageCall = 0;
        panel.requestJSON = vi.fn(async raw => {
            const url = String(raw);
            if (url === '/v1/jobs/summary') return { byState: { running: 0 } };
            if (url.startsWith('/v1/jobs?')) {
                const round = Math.floor(pageCall / 3);
                pageCall += 1;
                return { jobs: round === 0 ? [visibleJob] : [] };
            }
            if (url === '/v1/jobs/job-1') {
                markDetailStarted();
                return detailRequest;
            }
            return {};
        });

        const firstRefresh = panel.refresh();
        await detailStarted;
        await panel.refresh();
        expect(panel.jobs).toEqual([]);

        resolveDetail({ ...visibleJob, state: 'succeeded', version: 2, commands: [{ key: 'dismiss' }] });
        await firstRefresh;

        expect(panel.jobs).toEqual([]);
        panel.destroy();
    });

    test('keeps visible command details when unrelated live events arrive during the detail request', async () => {
        vi.useFakeTimers();
        let resolveDetail: (value: unknown) => void = () => {};
        let markDetailStarted: () => void = () => {};
        const detailStarted = new Promise<void>(resolve => { markDetailStarted = resolve; });
        const detailRequest = new Promise(resolve => { resolveDetail = resolve; });
        const visibleJob = { id: 'visible-job', kind: 'maintenance', state: 'running', version: 4, acceptedAt: '2026-09-23T10:00:00Z' };
        const commands = [{ key: 'pause', label: 'Pause', endpoint: '/v1/jobs/visible-job/commands/pause', jobVersion: 4 }];
        const panel = jobPanel();
        panel.streamCaughtUp = true;
        panel.requestJSON = vi.fn(async raw => {
            const url = String(raw);
            if (url === '/v1/jobs/summary') return { byState: { running: 1 } };
            if (url.startsWith('/v1/jobs?')) return { jobs: [visibleJob] };
            markDetailStarted();
            return detailRequest;
        });

        const refresh = panel.refresh();
        await detailStarted;
        for (let sequence = 1; sequence <= 8; sequence++) {
            await panel.handleStreamMessage({
                data: JSON.stringify({ id: `unrelated-${sequence}`, jobId: `other-${sequence}`, deliverySequence: sequence }),
                lastEventId: `v2:${sequence}`,
            });
            await vi.advanceTimersByTimeAsync(10);
        }

        resolveDetail({ ...visibleJob, commands });
        await refresh;

        expect(panel.commandsFor(panel.jobs[0])).toEqual(commands);
        panel.destroy();
        vi.useRealTimers();
    });

    test('runs a maximum-wait refresh while stream events continue arriving', async () => {
        vi.useFakeTimers();
        const panel = jobPanel();
        panel.streamCaughtUp = true;
        let pageRequests = 0;
        panel.requestJSON = vi.fn(async raw => {
            const url = String(raw);
            if (url === '/v1/jobs/summary') return { byState: {} };
            if (url.startsWith('/v1/jobs?')) {
                pageRequests += 1;
                return { jobs: [] };
            }
            const id = decodeURIComponent(url.slice('/v1/jobs/'.length));
            return { id, state: 'running', version: 1 };
        });

        for (let sequence = 1; sequence <= 10; sequence++) {
            await panel.handleStreamMessage({
                data: JSON.stringify({ id: `event-${sequence}`, jobId: `unknown-${sequence}`, deliverySequence: sequence }),
                lastEventId: `v2:${sequence}`,
            });
            await vi.advanceTimersByTimeAsync(100);
        }

        expect(pageRequests).toBeGreaterThan(0);
        panel.destroy();
        vi.useRealTimers();
    });

    test('refreshes the active and attention counts after a delivered state event', async () => {
        vi.useFakeTimers();
        const panel = jobPanel();
        panel.streamCaughtUp = true;
        panel.jobs = [{ id: 'job-1', state: 'running', version: 1 }];
        panel.requestJSON = vi.fn(async raw => {
            const url = String(raw);
            if (url.startsWith('/v1/jobs?')) {
                const states = new URL(url, 'http://localhost').searchParams.getAll('state');
                return { jobs: states.includes('failed') ? [{
                    id: 'job-1', state: 'failed', version: 2, commands: [{ key: 'retry' }],
                }] : [] };
            }
            return { id: 'job-1', state: 'failed', version: 2, commands: [{ key: 'retry' }] };
        });

        await panel.handleStreamMessage({
            data: JSON.stringify({ id: 'job-1', state: 'failed', version: 2, deliverySequence: 1 }),
            lastEventId: 'v2:1',
        });
        await vi.advanceTimersByTimeAsync(250);

        expect(panel.counts).toEqual({ active: 0, attention: 1 });
        expect(panel.requestJSON.mock.calls.filter(([url]) => String(url).startsWith('/v1/jobs?')))
            .toHaveLength(3);
        expect(panel.requestJSON.mock.calls.every(([url]) => !String(url).startsWith('/v1/jobs/summary')))
            .toBe(true);
        vi.useRealTimers();
    });

    test('template traps focus, supports a narrow viewport, and names the new actions', () => {
        const template = readFileSync(fileURLToPath(new URL('../../templates/partials/jobPanel.tpl', import.meta.url)), 'utf8');
        const baseTemplate = readFileSync(fileURLToPath(new URL('../../templates/layouts/base.tpl', import.meta.url)), 'utf8');
        expect(template).toContain('role="dialog" aria-modal="true"');
        expect(template).toContain('x-trap.noscroll.noreturn="isOpen"');
        expect(template).toContain('data-testid="job-panel-overlay"');
        expect(template).toContain('aria-hidden="true" @click="close()"');
        // A full-height drawer from the right edge, full width on a narrow screen.
        expect(template).toContain('job-drawer fixed inset-y-0 right-0');
        expect(template).toContain('w-full max-w-md');
        // Live progress: a real progressbar, formatted stats, metrics and graphs
        // with an accessible summary.
        expect(template).toContain('role="progressbar"');
        expect(template).toContain(':aria-valuetext="progressValueText(job)"');
        expect(template).toContain('x-text="statsText(job)"');
        expect(template).toContain('x-for="metric in metricsFor(job)"');
        expect(template).toContain('x-for="series in graphsFor(job)"');
        expect(template).toContain('role="img" :aria-label="graphLabel(series)"');
        expect(template).toContain('x-for="group in groups"');
        expect(baseTemplate).toContain('name="x-jobs-panel-finished-limit"');
        expect(template).toContain('Dismiss finished');
        expect(template).not.toContain('Dismiss outcomes');
        expect(template).not.toContain('outcome in outcomes');
        // The button stays while a long dismissal runs, and stays focusable: a
        // disabled focused button drops focus to the page behind the dialog.
        expect(template).toContain('x-show="finishedCount > 0 || busy"');
        expect(template).toContain(':aria-disabled="busy.toString()"');
        expect(template).not.toContain(':disabled="busy"');
        expect(template).toContain('data-job-panel-all-jobs');
        expect(template).toContain('All jobs');
        expect(template).toContain('x-for="command in commandsFor(job)"');
        expect(template).toContain('x-if="resultOutput(job)"');
        expect(template).toContain(':href="resultURL(job)"');
        expect(template).toContain(':aria-label="resultAccessibleLabel(job)"');
        expect(template).toContain('x-text="resultLinkLabel(job)"');
        expect(template).toContain('x-show="job.pinned"');
        expect(template).toContain('Pinned by you');
        expect(template).toContain('Active and scheduled jobs shown');
        expect(template).toContain('Jobs needing attention shown');
        expect(template).toContain("'Active jobs shown: ' + activeCount");
        expect(template).not.toContain('Clear completed');
        expect(template).toContain('Active and scheduled');
        expect(template).toContain('Jobs needing attention');
        expect(baseTemplate).toContain('{% include "/partials/jobPanel.tpl" %}');
        expect(baseTemplate).not.toContain('downloadCockpit.tpl');
    });
});
