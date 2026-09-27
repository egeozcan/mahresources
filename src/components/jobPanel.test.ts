import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import { readdirSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { jobPanel, panelBadgeText, panelCounts, panelCommandConfirmation, panelCommandSplit, panelCountsText, panelFinishedLimit, panelFocusSuccessorKeys, panelGroupJobsURL, panelGroups, panelLifecycleEvents, panelStateTone } from './jobPanel.js';
import { preferenceCommandJobIDs } from '../utils/jobPreferenceChannel.js';

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

    test('connects at the head of the canonical stream, and resumes from its cursor in its owner scope', () => {
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

        expect(panel.eventSource?.url).toBe('/v1/jobs/events?version=2&start=head');
        expect(panel.eventSource?.listeners.has('job')).toBe(true);
        expect(panel.eventSource?.listeners.has('job-caught-up')).toBe(true);

        panel.eventSource = null;
        panel.lastSequence = 42;
        panel.ownerScope = 'me';
        panel.connect();
        const resumed = new URL(String(panel.eventSource?.url), 'http://localhost').searchParams;
        expect(resumed.get('cursor')).toBe('v2:42');
        expect(resumed.has('start')).toBe(false);
        expect(resumed.get('owner')).toBe('me');
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
        panel._liveRegion = { announce: vi.fn(), destroy: vi.fn() } as any;
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
        // A full success needs no box: the rows leaving say it. It is spoken.
        expect(panel.notice).toBe('');
        expect(panel._liveRegion.announce).toHaveBeenLastCalledWith('203 finished jobs dismissed.');
        // The refresh that replaces the stale rows runs while the run still owns
        // the button, so a second click cannot start over the finished one.
        expect(busyDuringRefresh.length).toBeGreaterThan(0);
        expect(busyDuringRefresh.every(Boolean)).toBe(true);
        expect(panel.busy).toBe(false);
        await expect(panel.dismissFinished()).resolves.toEqual({ dismissed: 0, total: 0 });
    });

    test('a row command whose outcome names a page on this site opens it', async () => {
        const assign = vi.fn();
        vi.stubGlobal('location', { origin: 'http://localhost', assign });
        const panel = jobPanel();
        panel._liveRegion = { announce: vi.fn(), destroy: vi.fn() } as any;
        const row = { id: 'cmd-1', title: 'encode', kind: 'plugin-command', state: 'failed', version: 3, acceptedAt: '2026-09-26T10:00:02Z',
            commands: [{ key: 'inspect', label: 'Inspect command history', jobVersion: 3 }] };
        panel.jobs = [row];
        panel.requestJSON = vi.fn(async () => ({ result: {
            status: 'succeeded', code: 'applied', message: 'Opening the command history.',
            detail: { runId: 'abc', location: '/admin/plugin-command-runs?id=abc' },
        } })) as any;

        await panel.runCommand(row, row.commands[0]);

        expect(assign).toHaveBeenCalledWith('/admin/plugin-command-runs?id=abc');
    });

    function rowCommandPanel(answer: (url: string, init: any) => any) {
        const panel = jobPanel();
        panel._liveRegion = { announce: vi.fn(), destroy: vi.fn() } as any;
        panel.jobs = [
            { id: 'row-1', title: 'first.bin', kind: 'remote-download', state: 'failed', version: 4, acceptedAt: '2026-09-26T10:00:02Z', commands: [{ key: 'dismiss', label: 'Dismiss', jobVersion: 4 }] },
            { id: 'row-2', title: 'second.bin', kind: 'remote-download', state: 'failed', version: 2, acceptedAt: '2026-09-26T10:00:01Z', commands: [{ key: 'dismiss', label: 'Dismiss', jobVersion: 2 }] },
        ];
        panel.requestJSON = vi.fn(async (url: string, init: any = {}) => answer(String(url), init));
        // Dismiss asks first; the reader accepts.
        vi.stubGlobal('Alpine', { store: () => ({ ask: async () => true }) });
        return panel;
    }

    test('a row\'s Dismiss removes the row at once and shows no box', async () => {
        // The server records a preference and emits no job event, so nothing
        // else would refresh the row away.
        let second: any;
        const heldLists: Array<() => void> = [];
        const panel = rowCommandPanel((url, init) => init.method === 'POST'
            ? { result: { status: 'succeeded', code: 'applied', message: 'dismissed' } }
            // The server's lists leave the dismissed job out, but answer slowly.
            : new Promise(resolve => heldLists.push(() => resolve({
                jobs: new URL(url, 'http://localhost').searchParams.getAll('state').includes('failed') ? [second] : [],
            }))));
        second = panel.jobs[1];

        await panel.runCommand(panel.jobs[0], panel.jobs[0].commands[0]);

        // Gone before the refresh answers, not because of it.
        expect(heldLists.length).toBeGreaterThan(0);
        expect(panel.jobs.map(job => job.id)).toEqual(['row-2']);
        heldLists.forEach(release => release());
        await vi.waitFor(() => expect(panel.requestJSON).toHaveBeenCalledTimes(4));
        expect(panel.jobs.map(job => job.id)).toEqual(['row-2']);
        expect(panel.notice).toBe('');
        expect(panel._liveRegion.announce).toHaveBeenLastCalledWith('first.bin dismissed.');
    });

    test('a refresh that left before the Dismiss answered does not bring the row back', async () => {
        const pendingLists: Array<(value: any) => void> = [];
        const panel = rowCommandPanel((url, init) => init.method === 'POST'
            ? { result: { message: 'dismissed' } }
            : new Promise(resolve => { pendingLists.push(resolve); }));
        const row = panel.jobs[0];
        const stale = panel.refresh();
        const staleReads = pendingLists.length;
        await panel.runCommand(row, row.commands[0]);
        // The first lists were read before the dismissal and still hold the row;
        // the refresh the Dismiss starts reads them without it.
        pendingLists.forEach((answer, index) => answer({ jobs: index < staleReads ? [row] : [] }));
        await stale;
        await vi.waitFor(() => expect(panel.requestJSON).toHaveBeenCalledTimes(staleReads * 2 + 1));

        expect(panel.jobs.map(job => job.id)).not.toContain('row-1');
    });

    test('a Dismiss refreshes, so a job beyond the shown limit takes the freed place', async () => {
        const older = { id: 'row-3', title: 'third.bin', kind: 'remote-download', state: 'failed', version: 1, acceptedAt: '2026-09-26T10:00:00Z', commands: [{ key: 'dismiss', label: 'Dismiss', jobVersion: 1 }] };
        let dismissed = false;
        const panel = rowCommandPanel((url, init) => {
            if (init.method === 'POST') { dismissed = true; return { result: { message: 'dismissed' } }; }
            const states = new URL(url, 'http://localhost').searchParams.getAll('state');
            const rows = [...panel.jobs.filter(job => job.id !== 'row-1' || !dismissed), older];
            return { jobs: states.includes('failed') ? rows.filter((row, index, all) => all.findIndex(other => other.id === row.id) === index) : [] };
        });

        await panel.runCommand(panel.jobs[0], panel.jobs[0].commands[0]);
        await vi.waitFor(() => expect(panel.jobs.map(job => job.id)).toEqual(['row-2', 'row-3']));
    });

    test('a command whose result the row cannot show keeps the server\'s words in the box', async () => {
        const panel = rowCommandPanel(() => ({ result: { status: 'succeeded', code: 'applied', message: 'pinned 2 of 3 visible related jobs' } }));

        await panel.runCommand(panel.jobs[0], { key: 'pin-lineage', label: 'Pin visible lineage', jobVersion: 4 });

        expect(panel.notice).toBe('pinned 2 of 3 visible related jobs');
    });

    for (const key of ['cancel', 'pause', 'resume']) {
        test(`a ${key} request keeps its acknowledgement in the box, since the row may not change yet`, async () => {
            const panel = rowCommandPanel(() => ({ result: { status: 'succeeded', code: 'requested', message: `${key} requested` } }));

            await panel.runCommand(panel.jobs[0], { key, label: key, jobVersion: 4 });

            expect(panel.notice).toBe(`${key} requested`);
        });
    }

    test('a row command that fails still says why in the box', async () => {
        const panel = rowCommandPanel(() => { throw new Error('Request failed (500)'); });

        await panel.runCommand(panel.jobs[0], panel.jobs[0].commands[0]);

        expect(panel.jobs.map(job => job.id)).toEqual(['row-1', 'row-2']);
        expect(panel.notice).toBe('Request failed (500)');
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

        // A full success needs no box: the rows leaving say it. It is spoken.
        expect(panel.notice).toBe('');
        expect(panel._liveRegion.announce).toHaveBeenLastCalledWith('2 finished jobs dismissed.');
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

        // A full success needs no box: the rows leaving say it. It is spoken.
        expect(panel.notice).toBe('');
        expect(panel._liveRegion.announce).toHaveBeenLastCalledWith('2 finished jobs dismissed.');
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

        // A full success needs no box: the rows leaving say it. It is spoken.
        expect(panel.notice).toBe('');
        expect(panel._liveRegion.announce).toHaveBeenLastCalledWith('2 finished jobs dismissed.');
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

    // A refresh another job's event triggered can read this job's new state
    // before this job's own event arrives; that event then finds nothing to
    // announce. The refresh has to say it, or the change is never heard.
    // Rows the reader has already been told about on the current live stream,
    // as a refresh after catch-up leaves them.
    function showHeard(panel: any, rows: any[]) {
        panel.streamCaughtUp = true;
        panel.jobs = rows;
        rows.forEach(row => panel.hearJob(row));
    }

    function refreshingPanel(listed: any[]) {
        const panel = jobPanel();
        panel._liveRegion = { announce: vi.fn(), destroy: vi.fn() } as any;
        panel.requestJSON = vi.fn(async raw => {
            const url = String(raw);
            if (url.startsWith('/v1/jobs?')) {
                const states = new URL(url, 'http://localhost').searchParams.getAll('state');
                return { jobs: listed.filter(job => states.includes(job.state)) };
            }
            return { ...listed.find(job => url.endsWith(job.id)), commands: [] };
        });
        return panel;
    }

    test('a stream reset stops the drawer instead of reloading the page it sits on', async () => {
        const reload = vi.fn();
        vi.stubGlobal('location', { reload });
        const panel = jobPanel();
        panel._liveRegion = { announce: vi.fn(), destroy: vi.fn() } as any;
        const close = vi.fn();
        panel.eventSource = { close } as any;
        panel.jobs = [{ id: 'dl-1', title: 'old.bin', kind: 'remote-download', state: 'failed', version: 10 }];
        panel.details = { 'dl-1': panel.jobs[0] };
        panel.requestJSON = vi.fn(async () => ({ jobs: [] }));

        panel.markStreamCaughtUp({ data: JSON.stringify({ cursor: 'v2:875', reset: true }) });

        expect(reload).not.toHaveBeenCalled();
        expect(close).toHaveBeenCalledTimes(1);
        expect(panel.streamStopped).toBe(true);
        expect(panel.jobs).toEqual([]);
        expect(panel.details).toEqual({});
        expect(panel._liveRegion.announce).toHaveBeenCalledWith(expect.stringContaining('Job updates stopped'));

        // Nothing reads or repairs the list from here on, and a stream cannot
        // be reopened under it.
        panel.schedulePanelRefresh();
        await panel.refresh();
        panel.connect();
        expect(panel.requestJSON).not.toHaveBeenCalled();
        expect(panel.eventSource).toBeNull();

        panel.reloadPage();
        expect(reload).toHaveBeenCalledTimes(1);
        vi.unstubAllGlobals();
    });

    test('a command confirmed after the drawer stopped is never sent', async () => {
        const panel = jobPanel();
        panel._liveRegion = { announce: vi.fn(), destroy: vi.fn() } as any;
        let answerConfirmation: (value: boolean) => void = () => {};
        vi.stubGlobal('Alpine', { store: () => ({ ask: () => new Promise(resolve => { answerConfirmation = resolve; }) }) });
        panel.requestJSON = vi.fn(async () => ({ result: {} }));
        const job = { id: 'dl-1', title: 'old.bin', kind: 'remote-download', state: 'failed', version: 10 };

        const running = panel.runCommandUnfocused(job, { key: 'forget', label: 'Forget replay input', endpoint: '/v1/jobs/dl-1/commands/forget', jobVersion: 10 });
        panel.markStreamCaughtUp({ data: JSON.stringify({ cursor: 'v2:875', reset: true }) });
        answerConfirmation(true);
        await running;

        expect(panel.requestJSON).not.toHaveBeenCalled();
        vi.unstubAllGlobals();
    });

    test('a stopped drawer says only that it stopped, and nothing about Jobs after', async () => {
        vi.useFakeTimers();
        const panel = jobPanel();
        panel._liveRegion = { announce: vi.fn(), cancel: vi.fn(), destroy: vi.fn() } as any;
        // A count of outcomes still on its way, and one scheduled to be said.
        panel._countNews = { jobId: null, count: 2, text: '2 jobs finished or need attention; see the Jobs panel.' };
        panel.countUnsaidOutcome();

        panel.markStreamCaughtUp({ data: JSON.stringify({ cursor: 'v2:875', reset: true }) });
        panel.announceNews([{ jobId: 'dl-1', state: 'failed', version: 3, text: 'old.bin failed.' }]);
        panel.countUnsaidOutcome();
        await vi.advanceTimersByTimeAsync(5000);

        const said = panel._liveRegion.announce.mock.calls.map((call: any[]) => call[0]);
        expect(said).toEqual(["Job updates stopped because this server's database was restored or replaced. Reload the page to see current jobs."]);
        panel.destroy();
        vi.useRealTimers();
    });

    // An answer to a request the drawer sent before it stopped lands on a
    // stopped drawer: it must change nothing and say nothing.
    function lateAnswers(panel: any) {
        const pending: Array<(value: unknown) => void> = [];
        const fetchMock = vi.fn(() => new Promise(resolve => {
            pending.push(body => resolve({ ok: true, status: 200, json: async () => body }));
        }));
        vi.stubGlobal('fetch', fetchMock);
        panel._liveRegion = { announce: vi.fn(), destroy: vi.fn() } as any;
        return { fetchMock, answer: (body: unknown) => pending.shift()!(body) };
    }

    test('a command answered after the drawer stopped changes and says nothing', async () => {
        const panel = jobPanel();
        const { answer } = lateAnswers(panel);
        const job = { id: 'dl-1', title: 'old.bin', kind: 'remote-download', state: 'running', version: 10 };
        panel.jobs = [job];

        const running = panel.runCommandUnfocused(job, { key: 'cancel', label: 'Cancel', endpoint: '/v1/jobs/dl-1/commands/cancel', jobVersion: 10 });
        await Promise.resolve();
        panel.markStreamCaughtUp({ data: JSON.stringify({ cursor: 'v2:875', reset: true }) });
        answer({ result: { job: { ...job, state: 'cancelled', version: 11 }, message: 'Cancelled' } });
        await running;

        expect(panel.notice).toBe('');
        expect(panel.jobs).toEqual([]);
        expect(panel._liveRegion.announce).toHaveBeenCalledTimes(1);
        vi.unstubAllGlobals();
    });

    test('a pin refresh answered after the drawer stopped keeps no detail', async () => {
        const panel = jobPanel();
        const { answer } = lateAnswers(panel);
        const reading = panel.refreshJobPreference('old-job').catch(() => null);
        await Promise.resolve();
        panel.markStreamCaughtUp({ data: JSON.stringify({ cursor: 'v2:875', reset: true }) });
        answer({ id: 'old-job', title: 'old.bin', kind: 'remote-download', state: 'failed', version: 10, pinned: true });
        await reading;

        expect(panel.details).toEqual({});
        vi.unstubAllGlobals();
    });

    test('a dismissal answered after the drawer stopped changes and says nothing', async () => {
        const panel = jobPanel();
        const { fetchMock, answer } = lateAnswers(panel);
        panel.jobs = [{ id: 'dl-2', title: 'done.bin', kind: 'remote-download', state: 'succeeded', version: 4, commands: [{ key: 'dismiss', label: 'Dismiss', bulk: true, jobVersion: 4 }] }];

        const dismissing = panel.dismissFinished();
        await Promise.resolve();
        answer({ jobs: [{ id: 'dl-2' }], nextCursor: '' });
        await vi.waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2));
        panel.markStreamCaughtUp({ data: JSON.stringify({ cursor: 'v2:875', reset: true }) });
        answer({ results: [{ jobId: 'dl-2', status: 'succeeded', code: 'applied' }] });
        await dismissing;

        expect(panel.notice).toBe('');
        expect(fetchMock).toHaveBeenCalledTimes(2);
        expect(panel._liveRegion.announce).toHaveBeenCalledTimes(1);
        vi.unstubAllGlobals();
    });

    test('the stopped drawer says why and offers a reload in place of its list', () => {
        const template = readFileSync(fileURLToPath(new URL('../../templates/partials/jobPanel.tpl', import.meta.url)), 'utf8');
        const at = template.indexOf('<div x-show="streamStopped"');
        expect(at).toBeGreaterThan(-1);
        const stopped = template.slice(at, template.indexOf('</div>', at));
        expect(stopped).toContain('data-job-panel-stopped');
        expect(stopped).toContain("Job updates stopped because this server's database was restored or replaced.");
        expect(stopped).toContain('@click="reloadPage()"');
        expect(template).toContain('jobs.length === 0 && !error && !streamStopped');
    });

    test('announces a transition a refresh reads before the job\'s own event arrives', async () => {
        const panel = refreshingPanel([{ id: 'dl-1', title: 'clip.mp4', kind: 'remote-download', state: 'failed', version: 3, acceptedAt: '2026-09-26T10:00:00Z' }]);
        panel.lastSequence = 10;
        showHeard(panel, [{ id: 'dl-1', title: 'clip.mp4', kind: 'remote-download', state: 'running', version: 2, acceptedAt: '2026-09-26T10:00:00Z' }]);

        await panel.refresh();

        expect(panel._liveRegion.announce).toHaveBeenCalledTimes(1);
        expect(panel._liveRegion.announce).toHaveBeenCalledWith(expect.stringMatching(/clip\.mp4.*failed/i));

        // The job's own event, arriving late, finds the row already failed.
        await panel.handleStreamMessage({
            data: JSON.stringify({ job: { id: 'dl-1', title: 'clip.mp4', kind: 'remote-download', state: 'failed', version: 3 }, sequence: 3, deliverySequence: 11 }),
            lastEventId: 'v2:11',
        });
        expect(panel._liveRegion.announce).toHaveBeenCalledTimes(1);
    });

    test('a change one refresh announces is not announced again by the next', async () => {
        const failed = { id: 'dl-4', title: 'clip.mp4', kind: 'remote-download', state: 'failed', version: 3, acceptedAt: '2026-09-26T10:00:00Z' };
        const panel = refreshingPanel([failed]);
        showHeard(panel, [{ ...failed, state: 'running', version: 2 }]);

        await panel.refresh();
        await panel.refresh();

        expect(panel._liveRegion.announce).toHaveBeenCalledTimes(1);
    });

    test('a job a refresh dropped between two group reads still has its late event announced, once', async () => {
        const running = { id: 'dl-12', title: 'between.bin', kind: 'remote-download', state: 'running', version: 2, acceptedAt: '2026-09-26T10:00:00Z', commands: [{ key: 'cancel' }] };
        const failed = { ...running, state: 'failed', version: 3, commands: [{ key: 'retry' }] };
        const panel = refreshingPanel([]);
        panel.lastSequence = 10;
        showHeard(panel, [running]);
        panel.requestJSON = vi.fn(async () => ({ jobs: [] }));

        await panel.refresh();
        expect(panel.jobs).toEqual([]);

        await panel.handleStreamMessage({ data: JSON.stringify({ job: failed, sequence: 3, deliverySequence: 11 }), lastEventId: 'v2:11' });
        expect(panel._liveRegion.announce).toHaveBeenCalledTimes(1);
        expect(panel._liveRegion.announce).toHaveBeenCalledWith('between.bin failed.');

        panel.requestJSON = vi.fn(async raw => {
            const states = new URL(String(raw), 'http://localhost').searchParams.getAll('state');
            return { jobs: states.includes('failed') ? [failed] : [] };
        });
        await panel.refresh();
        expect(panel.jobs.map(job => job.state)).toEqual(['failed']);
        expect(panel._liveRegion.announce).toHaveBeenCalledTimes(1);
    });

    test('an event-only update after a reconnect does not turn the catch-up refresh into news', async () => {
        const failed = { id: 'dl-13', title: 'history.bin', kind: 'remote-download', state: 'failed', version: 3, acceptedAt: '2026-09-26T10:00:00Z' };
        const panel = refreshingPanel([failed]);
        panel.lastSequence = 10;
        showHeard(panel, [{ ...failed, state: 'running', version: 2 }]);
        // Disconnected while it failed; caught up again.
        panel._streamGeneration += 2;
        await panel.handleStreamMessage({
            data: JSON.stringify({ id: 'e-1', jobId: 'dl-13', jobVersion: 4, type: 'warning', deliverySequence: 11 }),
            lastEventId: 'v2:11',
        });

        await panel.refresh();

        expect(panel.jobs[0].state).toBe('failed');
        expect(panel._liveRegion.announce).not.toHaveBeenCalled();
    });

    test('a list answered before a newer stream snapshot does not roll the row back or repeat its announcement', async () => {
        // Rows that carry their commands skip the detail fetch, which would
        // otherwise paper over a rolled-back row.
        const failed = { id: 'dl-5', title: 'clip.mp4', kind: 'remote-download', state: 'failed', version: 3, acceptedAt: '2026-09-26T10:00:00Z', commands: [{ key: 'retry' }] };
        const running = { ...failed, state: 'running', version: 2, commands: [{ key: 'cancel' }] };
        const panel = refreshingPanel([failed]);
        panel.lastSequence = 10;
        showHeard(panel, [running]);
        const list = panel.requestJSON;
        let snapshotApplied = false;
        panel.requestJSON = vi.fn(async raw => {
            const url = String(raw);
            if (!snapshotApplied && url.startsWith('/v1/jobs?')) {
                // The list was read while running; the snapshot lands before it answers.
                snapshotApplied = true;
                await panel.handleStreamMessage({
                    data: JSON.stringify({ job: failed, sequence: 3, deliverySequence: 11 }),
                    lastEventId: 'v2:11',
                });
            }
            if (url.startsWith('/v1/jobs?')) {
                const states = new URL(url, 'http://localhost').searchParams.getAll('state');
                return { jobs: states.includes('running') ? [running] : [] };
            }
            return list(raw);
        });

        await panel.refresh();
        expect(panel.jobs.find(job => job.id === 'dl-5')?.state).toBe('failed');

        panel.requestJSON = list;
        await panel.refresh();

        expect(panel._liveRegion.announce).toHaveBeenCalledTimes(1);
        expect(panel._liveRegion.announce).toHaveBeenCalledWith(expect.stringMatching(/clip\.mp4 failed/));
    });

    test('a row the stream moved between two group reads is kept, not dropped until the next refresh', async () => {
        const running = { id: 'dl-6', title: 'move.bin', kind: 'remote-download', state: 'running', version: 2, acceptedAt: '2026-09-26T10:00:00Z', commands: [{ key: 'cancel' }] };
        const failed = { ...running, state: 'failed', version: 3, commands: [{ key: 'retry' }] };
        const panel = refreshingPanel([]);
        panel.lastSequence = 10;
        showHeard(panel, [running]);
        let snapshotApplied = false;
        panel.requestJSON = vi.fn(async raw => {
            const states = new URL(String(raw), 'http://localhost').searchParams.getAll('state');
            // Needs attention is read before the failure, Active after it:
            // neither list holds the job, and the snapshot lands in between.
            if (states.includes('running') && !snapshotApplied) {
                snapshotApplied = true;
                await panel.handleStreamMessage({ data: JSON.stringify({ job: failed, sequence: 3, deliverySequence: 11 }), lastEventId: 'v2:11' });
            }
            return { jobs: [] };
        });

        await panel.refresh();

        expect(panel.jobs.map(job => `${job.id}:${job.state}`)).toEqual(['dl-6:failed']);
        expect(panel._liveRegion.announce).toHaveBeenCalledTimes(1);

        // With no stream change during the next read, a job the lists no longer
        // hold is gone.
        await panel.refresh();
        expect(panel.jobs).toEqual([]);
    });

    test('a row a command answered during the reads is not kept once the lists drop it', async () => {
        const done = { id: 'dl-7', title: 'gone.bin', kind: 'remote-download', state: 'succeeded', version: 4, acceptedAt: '2026-09-26T10:00:00Z', commands: [{ key: 'dismiss' }] };
        const panel = refreshingPanel([]);
        showHeard(panel, [done]);
        panel.requestJSON = vi.fn(async () => {
            // Dismiss answered while the lists were read after it took effect.
            panel.applyStreamSnapshot({ ...done, version: 5 });
            return { jobs: [] };
        });

        await panel.refresh();

        expect(panel.jobs).toEqual([]);
    });

    test('a state change only the detail fetch reads is announced, and the late event does not repeat it', async () => {
        const running = { id: 'dl-8', title: 'detail.bin', kind: 'remote-download', state: 'running', version: 2, acceptedAt: '2026-09-26T10:00:00Z' };
        const failed = { ...running, state: 'failed', version: 3, failure: { message: 'HTTP 403 Forbidden' } };
        const panel = refreshingPanel([]);
        panel.lastSequence = 10;
        panel.isOpen = true;
        showHeard(panel, [running]);
        panel.requestJSON = vi.fn(async raw => {
            const url = String(raw);
            if (url.startsWith('/v1/jobs?')) {
                const states = new URL(url, 'http://localhost').searchParams.getAll('state');
                return { jobs: states.includes('running') ? [running] : [] };
            }
            // The job failed between the list read and its detail read.
            return { ...failed, commands: [{ key: 'retry' }] };
        });

        await panel.refresh();

        expect(panel.jobs[0].state).toBe('failed');
        expect(panel._liveRegion.announce).toHaveBeenCalledTimes(1);
        expect(panel._liveRegion.announce).toHaveBeenCalledWith('detail.bin failed: HTTP 403 Forbidden.');

        await panel.handleStreamMessage({ data: JSON.stringify({ job: failed, sequence: 3, deliverySequence: 11 }), lastEventId: 'v2:11' });
        expect(panel._liveRegion.announce).toHaveBeenCalledTimes(1);
    });

    test('on a first load, a detail newer than its list row is history, not news', async () => {
        const running = { id: 'dl-9', title: 'first.bin', kind: 'remote-download', state: 'running', version: 2, acceptedAt: '2026-09-26T10:00:00Z' };
        const panel = refreshingPanel([]);
        panel.isOpen = true;
        // The drawer's first load runs while the stream is still catching up.
        panel.streamCaughtUp = false;
        panel.requestJSON = vi.fn(async raw => {
            const url = String(raw);
            if (url.startsWith('/v1/jobs?')) {
                const states = new URL(url, 'http://localhost').searchParams.getAll('state');
                return { jobs: states.includes('running') ? [running] : [] };
            }
            return { ...running, state: 'failed', version: 3, commands: [] };
        });

        await panel.refresh();
        expect(panel.jobs[0].state).toBe('failed');

        // Caught up: the refresh that follows reads the same history.
        panel.streamCaughtUp = true;
        panel._streamGeneration += 1;
        await panel.refresh();

        expect(panel._liveRegion.announce).not.toHaveBeenCalled();
    });

    test('a refresh says what its lists found at once, however long a detail read takes', async () => {
        const running = { id: 'dl-10', title: 'late.bin', kind: 'remote-download', state: 'running', version: 2, acceptedAt: '2026-09-26T10:00:00Z' };
        const failed = { ...running, state: 'failed', version: 3 };
        const other = { id: 'dl-11', title: 'slow.bin', kind: 'remote-download', state: 'running', version: 1, acceptedAt: '2026-09-26T09:00:00Z' };
        const panel = refreshingPanel([]);
        panel.lastSequence = 10;
        panel.isOpen = true;
        showHeard(panel, [running, other]);
        let releaseDetail = () => {};
        const detailHeld = new Promise<void>(resolve => { releaseDetail = resolve; });
        panel.requestJSON = vi.fn(async raw => {
            const url = String(raw);
            if (url.startsWith('/v1/jobs?')) {
                const states = new URL(url, 'http://localhost').searchParams.getAll('state');
                return { jobs: [failed, other].filter(job => states.includes(job.state)) };
            }
            // One row's detail stalls; nothing waits on it.
            if (url.endsWith('dl-11')) await detailHeld;
            return { ...(url.endsWith('dl-11') ? other : failed), commands: [] };
        });

        const refreshing = panel.refresh();
        await vi.waitFor(() => expect(panel._liveRegion.announce).toHaveBeenCalledWith('late.bin failed.'));
        releaseDetail();
        await refreshing;

        expect(panel._liveRegion.announce.mock.calls.map(call => call[0])).toEqual(['late.bin failed.']);
    });

    test('a list read begun before catch-up is recorded as history, even when it answers after', async () => {
        const running = { id: 'dl-14', title: 'boundary.bin', kind: 'remote-download', state: 'running', version: 2, acceptedAt: '2026-09-26T10:00:00Z', commands: [{ key: 'cancel' }] };
        const failed = { ...running, state: 'failed', version: 3, commands: [{ key: 'retry' }] };
        const panel = refreshingPanel([]);
        panel.streamCaughtUp = false;
        panel.requestJSON = vi.fn(async raw => {
            // The stream catches up while the first load's list is in flight.
            panel.streamCaughtUp = true;
            panel._streamGeneration += 1;
            const states = new URL(String(raw), 'http://localhost').searchParams.getAll('state');
            return { jobs: states.includes('running') ? [running] : [] };
        });
        await panel.refresh();

        // The next refresh reads a failure that happened before catch-up.
        panel.requestJSON = vi.fn(async raw => {
            const states = new URL(String(raw), 'http://localhost').searchParams.getAll('state');
            return { jobs: states.includes('failed') ? [failed] : [] };
        });
        await panel.refresh();

        expect(panel.jobs[0].state).toBe('failed');
        expect(panel._liveRegion.announce).not.toHaveBeenCalled();
    });

    test('a detail read that answers after a disconnect records what it found without saying it', async () => {
        const running = { id: 'dl-15', title: 'held.bin', kind: 'remote-download', state: 'running', version: 2, acceptedAt: '2026-09-26T10:00:00Z' };
        const failed = { ...running, state: 'failed', version: 3 };
        const panel = refreshingPanel([]);
        panel.isOpen = true;
        showHeard(panel, [running]);
        let releaseDetail = () => {};
        const detailHeld = new Promise<void>(resolve => { releaseDetail = resolve; });
        panel.requestJSON = vi.fn(async raw => {
            const url = String(raw);
            if (url.startsWith('/v1/jobs?')) {
                const states = new URL(url, 'http://localhost').searchParams.getAll('state');
                return { jobs: [running].filter(job => states.includes(job.state)) };
            }
            // The job failed after the lists were read; its detail says so.
            await detailHeld;
            return { ...failed, commands: [] };
        });

        const refreshing = panel.refresh();
        await vi.waitFor(() => expect(panel._detailReads.has('dl-15')).toBe(true));
        // The stream drops while the detail read is in flight.
        panel.streamCaughtUp = false;
        panel._streamGeneration += 1;
        releaseDetail();
        await refreshing;
        await panel._detailLoad;

        expect(panel.jobs[0].state).toBe('failed');
        expect(panel._liveRegion.announce).not.toHaveBeenCalled();
    });

    test('news is said in the order it is learned, and a detail read that answers late says nothing stale', async () => {
        const base = { id: 'dl-17', title: 'again.bin', kind: 'remote-download', acceptedAt: '2026-09-26T10:00:00Z' };
        const panel = refreshingPanel([]);
        panel.lastSequence = 10;
        panel.isOpen = true;
        showHeard(panel, [{ ...base, state: 'queued', version: 1 }]);
        let releaseDetail = () => {};
        const detailHeld = new Promise<void>(resolve => { releaseDetail = resolve; });
        panel.requestJSON = vi.fn(async raw => {
            const url = String(raw);
            if (url.startsWith('/v1/jobs?')) {
                const states = new URL(url, 'http://localhost').searchParams.getAll('state');
                return { jobs: [{ ...base, state: 'running', version: 2 }].filter(job => states.includes(job.state)) };
            }
            await detailHeld;
            return { ...base, state: 'running', version: 2, commands: [] };
        });

        const refreshing = panel.refresh();
        await vi.waitFor(() => expect(panel._detailReads.has('dl-17')).toBe(true));
        await panel.handleStreamMessage({ data: JSON.stringify({ job: { ...base, state: 'paused', version: 3 }, sequence: 3, deliverySequence: 11 }), lastEventId: 'v2:11' });
        await panel.handleStreamMessage({ data: JSON.stringify({ job: { ...base, state: 'running', version: 4 }, sequence: 4, deliverySequence: 12 }), lastEventId: 'v2:12' });
        releaseDetail();
        await refreshing;

        const spoken = panel._liveRegion.announce.mock.calls.map(call => call[0]);
        expect(spoken).toEqual(['again.bin running.', 'again.bin paused.', 'again.bin running.']);
        expect(panel.jobs[0].version).toBe(4);
    });

    test('after a reconnect, a live lifecycle event makes the refresh that reads it news', async () => {
        const failed = { id: 'dl-19', title: 'live.bin', kind: 'remote-download', state: 'failed', version: 3, acceptedAt: '2026-09-26T10:00:00Z' };
        const panel = refreshingPanel([failed]);
        panel.lastSequence = 10;
        showHeard(panel, [{ ...failed, state: 'running', version: 2 }]);
        panel._streamGeneration += 2; // disconnected and caught up again, nothing missed
        await panel.handleStreamMessage({
            data: JSON.stringify({ id: 'e-2', jobId: 'dl-19', jobVersion: 3, type: 'failed', deliverySequence: 11 }),
            lastEventId: 'v2:11',
        });
        expect(panel._liveRegion.announce).not.toHaveBeenCalled();

        await panel.refresh();

        expect(panel._liveRegion.announce).toHaveBeenCalledTimes(1);
        expect(panel._liveRegion.announce).toHaveBeenCalledWith('live.bin failed.');
    });

    for (const delivery of ['snapshot', 'event-only'] as const) {
        test(`a change a read withheld at the reconnect boundary is said when its live ${delivery} arrives`, async () => {
            const failed = { id: 'dl-20', title: 'boundary.bin', kind: 'remote-download', state: 'failed', version: 3, acceptedAt: '2026-09-26T10:00:00Z', commands: [{ key: 'retry' }] };
            const panel = refreshingPanel([failed]);
            panel.lastSequence = 10;
            showHeard(panel, [{ ...failed, state: 'running', version: 2, commands: [{ key: 'cancel' }] }]);
            panel._streamGeneration += 1; // caught up; the job fails just after
            await panel.refresh();
            expect(panel.jobs[0].state).toBe('failed');
            expect(panel._liveRegion.announce).not.toHaveBeenCalled();

            const message = delivery === 'snapshot'
                ? { job: failed, sequence: 3, deliverySequence: 11 }
                : { id: 'e-3', jobId: 'dl-20', jobVersion: 3, type: 'failed', deliverySequence: 11 };
            await panel.handleStreamMessage({ data: JSON.stringify(message), lastEventId: 'v2:11' });

            expect(panel._liveRegion.announce).toHaveBeenCalledTimes(1);
            expect(panel._liveRegion.announce).toHaveBeenCalledWith('boundary.bin failed.');
            // Said once: a second delivery of the same event says nothing.
            await panel.handleStreamMessage({ data: JSON.stringify(message), lastEventId: 'v2:11' });
            expect(panel._liveRegion.announce).toHaveBeenCalledTimes(1);
        });
    }

    test('a read begun before catch-up does not use up a live event\'s proof', async () => {
        const failed = { id: 'dl-21', title: 'proof.bin', kind: 'remote-download', state: 'failed', version: 3, acceptedAt: '2026-09-26T10:00:00Z', commands: [{ key: 'retry' }] };
        const panel = refreshingPanel([failed]);
        panel.lastSequence = 10;
        showHeard(panel, [{ ...failed, state: 'running', version: 2, commands: [{ key: 'cancel' }] }]);
        const staleGeneration = panel._streamGeneration;
        panel._streamGeneration += 2;
        await panel.handleStreamMessage({ data: JSON.stringify({ id: 'e-4', jobId: 'dl-21', jobVersion: 3, type: 'failed', deliverySequence: 11 }), lastEventId: 'v2:11' });
        // A list read that began before catch-up answers now: it may not speak.
        const spoken: any[] = [];
        panel.hearFromRead(failed, staleGeneration, spoken);
        expect(spoken).toEqual([]);

        await panel.refresh();

        expect(panel._liveRegion.announce).toHaveBeenCalledTimes(1);
        expect(panel._liveRegion.announce).toHaveBeenCalledWith('proof.bin failed.');
    });

    test('a command answer carrying a change a live event proved says it', async () => {
        const failed = { id: 'dl-22', title: 'conflict.bin', kind: 'remote-download', state: 'failed', version: 2, acceptedAt: '2026-09-26T10:00:00Z' };
        const panel = refreshingPanel([failed]);
        panel.lastSequence = 10;
        showHeard(panel, [{ ...failed, state: 'running', version: 1 }]);
        await panel.handleStreamMessage({ data: JSON.stringify({ id: 'e-5', jobId: 'dl-22', jobVersion: 2, type: 'failed', deliverySequence: 11 }), lastEventId: 'v2:11' });

        // A 409 on the reader's command hands back the failed job; the command
        // says the proved change together with its own notice, in one message,
        // since a second message would cancel the first.
        panel.requestJSON = vi.fn(async (raw, init) => {
            if (init?.method === 'POST') {
                const error: any = new Error('conflict');
                error.status = 409;
                error.payload = { job: failed };
                throw error;
            }
            return { jobs: [failed].filter(job => new URL(String(raw), 'http://localhost').searchParams.getAll('state').includes(job.state)) };
        });
        await panel.runCommand({ ...failed, state: 'running', version: 1 }, { key: 'cancel', label: 'Cancel', endpoint: '/v1/jobs/dl-22/commands/cancel', jobVersion: 1 });
        await panel.refresh();

        expect(panel._liveRegion.announce).toHaveBeenCalledTimes(1);
        expect(panel._liveRegion.announce).toHaveBeenCalledWith('conflict.bin failed. This job changed. The latest details are shown.');
    });

    test('a live event\'s proof does not outlive a disconnect', async () => {
        const queued = { id: 'dl-23', title: 'replayed.bin', kind: 'remote-download', state: 'queued', version: 3, acceptedAt: '2026-09-26T10:00:00Z' };
        const panel = refreshingPanel([queued]);
        panel.lastSequence = 10;
        showHeard(panel, [{ ...queued, state: 'running', version: 1 }]);
        await panel.handleStreamMessage({ data: JSON.stringify({ id: 'e-6', jobId: 'dl-23', jobVersion: 2, type: 'paused', deliverySequence: 11 }), lastEventId: 'v2:11' });
        panel.dropStream();
        panel.streamCaughtUp = true;
        panel._streamGeneration += 1;

        await panel.refresh();

        expect(panel._liveRegion.announce).not.toHaveBeenCalled();
    });

    test('a detail read that answers after a live event-only update says nothing stale', async () => {
        const base = { id: 'dl-24', title: 'superseded.bin', kind: 'remote-download', acceptedAt: '2026-09-26T10:00:00Z' };
        const panel = refreshingPanel([]);
        panel.lastSequence = 10;
        panel.isOpen = true;
        showHeard(panel, [{ ...base, state: 'running', version: 2 }]);
        let releaseDetail = () => {};
        const detailHeld = new Promise<void>(resolve => { releaseDetail = resolve; });
        const succeeded = { ...base, state: 'succeeded', version: 3 };
        panel.requestJSON = vi.fn(async raw => {
            const url = String(raw);
            if (url.startsWith('/v1/jobs?')) {
                const states = new URL(url, 'http://localhost').searchParams.getAll('state');
                return { jobs: [succeeded].filter(job => states.includes(job.state)) };
            }
            await detailHeld;
            return { ...succeeded, commands: [] };
        });

        const refreshing = panel.refresh();
        await vi.waitFor(() => expect(panel._detailReads.has('dl-24')).toBe(true));
        await panel.handleStreamMessage({ data: JSON.stringify({ id: 'e-7', jobId: 'dl-24', jobVersion: 4, type: 'failed', deliverySequence: 11 }), lastEventId: 'v2:11' });
        releaseDetail();
        await refreshing;

        expect(panel._liveRegion.announce.mock.calls.map(call => call[0])).toEqual(['superseded.bin succeeded.']);
    });

    test('a plugin action cancelled through its not-started event is news after a reconnect', async () => {
        const cancelled = { id: 'dl-26', title: 'Transcribe', kind: 'plugin-action', state: 'cancelled', version: 3, acceptedAt: '2026-09-26T10:00:00Z', commands: [] as any[] };
        const panel = refreshingPanel([cancelled]);
        panel.lastSequence = 10;
        showHeard(panel, [{ ...cancelled, state: 'queued', version: 2 }]);
        panel._streamGeneration += 1;
        await panel.refresh();
        expect(panel._liveRegion.announce).not.toHaveBeenCalled();

        await panel.handleStreamMessage({ data: JSON.stringify({ id: 'e-8', jobId: 'dl-26', jobVersion: 3, type: 'not-started', deliverySequence: 11 }), lastEventId: 'v2:11' });

        expect(panel._liveRegion.announce).toHaveBeenCalledWith('Transcribe cancelled.');
    });

    test('two withheld outcomes released back to back are said together', async () => {
        const rows = [
            { id: 'dl-27', title: 'one.bin', kind: 'remote-download', state: 'failed', version: 3, acceptedAt: '2026-09-26T10:00:02Z', commands: [] as any[] },
            { id: 'dl-28', title: 'two.bin', kind: 'remote-download', state: 'failed', version: 5, acceptedAt: '2026-09-26T10:00:01Z', commands: [] as any[] },
        ];
        const panel = refreshingPanel(rows);
        panel.lastSequence = 10;
        showHeard(panel, rows.map(row => ({ ...row, state: 'running', version: row.version - 1 })));
        panel._streamGeneration += 1;
        await panel.refresh();

        await panel.handleStreamMessage({ data: JSON.stringify({ id: 'e-9', jobId: 'dl-27', jobVersion: 3, type: 'failed', deliverySequence: 11 }), lastEventId: 'v2:11' });
        await panel.handleStreamMessage({ data: JSON.stringify({ id: 'e-10', jobId: 'dl-28', jobVersion: 5, type: 'failed', deliverySequence: 12 }), lastEventId: 'v2:12' });

        const calls = panel._liveRegion.announce.mock.calls.map((call: any[]) => call[0]);
        expect(calls.at(-1)).toBe('one.bin failed. two.bin failed.');
    });

    test('a command notice leaves out proved news a newer change has superseded', async () => {
        const base = { id: 'dl-29', title: 'stale.bin', kind: 'remote-download', acceptedAt: '2026-09-26T10:00:00Z' };
        const panel = refreshingPanel([]);
        panel.lastSequence = 10;
        showHeard(panel, [{ ...base, state: 'running', version: 1, pinned: false }]);
        // A live event proves v2 is news; the Pin answer carries it.
        await panel.handleStreamMessage({ data: JSON.stringify({ id: 'e-11', jobId: 'dl-29', jobVersion: 2, type: 'paused', deliverySequence: 11 }), lastEventId: 'v2:11' });
        panel.requestJSON = vi.fn(async (raw, init) => {
            if (init?.method === 'POST') return { result: { job: { ...base, state: 'paused', version: 2, pinned: true } } };
            // While the preference is read, the job resumes, live.
            await panel.handleStreamMessage({ data: JSON.stringify({ job: { ...base, state: 'running', version: 3 }, sequence: 3, deliverySequence: 12 }), lastEventId: 'v2:12' });
            return { ...base, state: 'running', version: 3, pinned: true };
        });

        vi.stubGlobal('Alpine', { store: () => ({ ask: async () => true }) });
        await panel.runCommand({ ...base, state: 'running', version: 1 }, { key: 'pin', label: 'Pin', endpoint: '/v1/jobs/dl-29/commands/pin', jobVersion: 1 });

        const last = panel._liveRegion.announce.mock.calls.at(-1)[0];
        expect(last).not.toContain('paused');
        expect(last).toContain('stale.bin running.');
        expect(last).toContain('stale.bin pinned.');
    });

    test('a command notice is kept when another job\'s news follows within the window', async () => {
        const mine = { id: 'dl-30', title: 'mine.bin', kind: 'remote-download', state: 'failed', version: 2, acceptedAt: '2026-09-26T10:00:01Z', pinned: false };
        const other = { id: 'dl-31', title: 'other.bin', kind: 'remote-download', state: 'running', version: 1, acceptedAt: '2026-09-26T10:00:00Z' };
        const panel = refreshingPanel([]);
        panel.lastSequence = 10;
        showHeard(panel, [mine, other]);
        panel.requestJSON = vi.fn(async (_raw, init) => init?.method === 'POST'
            ? { result: { job: { ...mine, version: 3 }, message: 'Dismissed.' } }
            : { ...mine, version: 3 });
        vi.stubGlobal('Alpine', { store: () => ({ ask: async () => true }) });

        await panel.runCommand(mine, { key: 'dismiss', label: 'Dismiss', endpoint: '/v1/jobs/dl-30/commands/dismiss', jobVersion: 2 });
        await panel.handleStreamMessage({ data: JSON.stringify({ job: { ...other, state: 'failed', version: 2 }, sequence: 2, deliverySequence: 11 }), lastEventId: 'v2:11' });

        const last = panel._liveRegion.announce.mock.calls.at(-1)[0];
        expect(last).toContain('other.bin failed.');
        expect(last).toContain('mine.bin dismissed.');
    });

    test('a withheld transition survives a later same-state version and is said by its late live event', async () => {
        const base = { id: 'dl-32', title: 'bumped.bin', kind: 'remote-download', acceptedAt: '2026-09-26T10:00:00Z', commands: [] as any[] };
        const panel = refreshingPanel([{ ...base, state: 'running', version: 3 }]);
        panel.lastSequence = 10;
        showHeard(panel, [{ ...base, state: 'queued', version: 2 }]);
        panel._streamGeneration += 1;
        await panel.refresh(); // withholds "running" at v3
        // A Cancel request moves the still-running job to v4; a read records it.
        panel.requestJSON = vi.fn(async raw => {
            const states = new URL(String(raw), 'http://localhost').searchParams.getAll('state');
            return { jobs: states.includes('running') ? [{ ...base, state: 'running', version: 4 }] : [] };
        });
        await panel.refresh();
        expect(panel._liveRegion.announce).not.toHaveBeenCalled();

        await panel.handleStreamMessage({ data: JSON.stringify({ id: 'e-12', jobId: 'dl-32', jobVersion: 3, type: 'started', deliverySequence: 11 }), lastEventId: 'v2:11' });

        expect(panel._liveRegion.announce).toHaveBeenCalledWith('bumped.bin running.');
    });

    test('a live proof inside a withheld change\'s range says it, even when the read saw a later version', async () => {
        const base = { id: 'dl-33', title: 'ranged.bin', kind: 'remote-download', acceptedAt: '2026-09-26T10:00:00Z' };
        const panel = refreshingPanel([]);
        panel.lastSequence = 10;
        showHeard(panel, [{ ...base, state: 'running', version: 2 }]);
        const staleGeneration = panel._streamGeneration;
        panel._streamGeneration += 2;
        await panel.handleStreamMessage({ data: JSON.stringify({ id: 'e-13', jobId: 'dl-33', jobVersion: 3, type: 'failed', deliverySequence: 11 }), lastEventId: 'v2:11' });
        // A read begun before catch-up answers with v4, a same-state control version.
        panel.hearFromRead({ ...base, state: 'failed', version: 4 }, staleGeneration, []);
        const spoken: any[] = [];
        panel.hearFromRead({ ...base, state: 'failed', version: 4 }, panel._streamGeneration, spoken);

        expect(spoken.map(entry => entry.text)).toEqual(['ranged.bin failed.']);
    });

    test('news survives a same-state version and is still said', async () => {
        const base = { id: 'dl-34', title: 'bump.bin', kind: 'remote-download', acceptedAt: '2026-09-26T10:00:00Z' };
        const panel = refreshingPanel([]);
        showHeard(panel, [{ ...base, state: 'running', version: 2 }]);
        const spoken: any[] = [];
        panel.hearFromRead({ ...base, state: 'failed', version: 3 }, panel._streamGeneration, spoken);
        // A command answer records a same-state version before the news is said.
        panel.hearJob({ ...base, state: 'failed', version: 4 });

        panel.announceNews(spoken);

        expect(panel._liveRegion.announce).toHaveBeenCalledWith('bump.bin failed.');
    });

    test('pending news does not outlive a disconnect', () => {
        const base = { id: 'dl-35', title: 'dropped.bin', kind: 'remote-download', acceptedAt: '2026-09-26T10:00:00Z' };
        const panel = refreshingPanel([]);
        showHeard(panel, [{ ...base, state: 'running', version: 2 }]);
        panel.announceNews([{ ...panel.newsEntry('dl-35', 'dropped.bin failed.'), state: 'running', version: 2 }]);
        panel.dropStream();

        panel.announceNotice('A dialog is open. Close it before opening Jobs.');

        expect(panel._liveRegion.announce.mock.calls.at(-1)[0]).toBe('A dialog is open. Close it before opening Jobs.');
    });

    for (const reconnected of [false, true]) {
        test(`a 409 answer that arrives before the job's live event is heard as a read${reconnected ? ', after a reconnect' : ''}`, async () => {
            const failed = { id: 'dl-36', title: 'answered.bin', kind: 'remote-download', state: 'failed', version: 3, acceptedAt: '2026-09-26T10:00:00Z', failure: { message: 'HTTP 500' } };
            const panel = refreshingPanel([]);
            panel.lastSequence = 10;
            showHeard(panel, [{ ...failed, state: 'running', version: 2, failure: undefined }]);
            if (reconnected) panel._streamGeneration += 2;
            panel.requestJSON = vi.fn(async () => {
                const error: any = new Error('conflict');
                error.status = 409;
                error.payload = { job: failed };
                throw error;
            });
            vi.stubGlobal('Alpine', { store: () => ({ ask: async () => true }) });
            await panel.runCommand({ ...failed, state: 'running', version: 2 }, { key: 'cancel', label: 'Cancel', endpoint: '/v1/jobs/dl-36/commands/cancel', jobVersion: 2 });
            await panel.handleStreamMessage({ data: JSON.stringify({ id: 'e-14', jobId: 'dl-36', jobVersion: 3, type: 'failed', deliverySequence: 11 }), lastEventId: 'v2:11' });

            const said = panel._liveRegion.announce.mock.calls.map((call: any[]) => call[0]).join(' | ');
            expect(said).toContain('answered.bin failed: HTTP 500.');
            expect(said).toContain('This job changed. The latest details are shown.');
            expect(said.split('answered.bin failed').length - 1).toBeLessThanOrEqual(2);
        });
    }

    test('a failure a Pin\'s preference read reveals is said, and its late event does not repeat it', async () => {
        const base = { id: 'dl-37', title: 'pinned.bin', kind: 'remote-download', acceptedAt: '2026-09-26T10:00:00Z' };
        const panel = refreshingPanel([]);
        panel.lastSequence = 10;
        showHeard(panel, [{ ...base, state: 'running', version: 2, pinned: false }]);
        panel.requestJSON = vi.fn(async (_raw, init) => init?.method === 'POST'
            ? { result: { job: { ...base, state: 'running', version: 2, pinned: true } } }
            // The job failed while Pin was pending; its event has not arrived.
            : { ...base, state: 'failed', version: 3, pinned: true, failure: { message: 'disk full' } });
        vi.stubGlobal('Alpine', { store: () => ({ ask: async () => true }) });

        await panel.runCommand({ ...base, state: 'running', version: 2 }, { key: 'pin', label: 'Pin', endpoint: '/v1/jobs/dl-37/commands/pin', jobVersion: 2 });
        await panel.handleStreamMessage({ data: JSON.stringify({ id: 'e-15', jobId: 'dl-37', jobVersion: 3, type: 'failed', deliverySequence: 11 }), lastEventId: 'v2:11' });

        const said = panel._liveRegion.announce.mock.calls.map((call: any[]) => call[0]);
        expect(said.join(' | ')).toContain('pinned.bin failed: disk full.');
        expect(said.at(-1)).toContain('pinned.bin pinned.');
    });

    test('several transitions one refresh reads are said together, not overwritten', async () => {
        const rows = [
            { id: 'a', title: 'first.bin', kind: 'remote-download', state: 'failed', version: 3, acceptedAt: '2026-09-26T10:00:02Z' },
            { id: 'b', title: 'second.bin', kind: 'remote-download', state: 'succeeded', version: 3, acceptedAt: '2026-09-26T10:00:01Z' },
        ];
        const panel = refreshingPanel(rows);
        showHeard(panel, rows.map(row => ({ ...row, state: 'running', version: 2 })));

        await panel.refresh();

        expect(panel._liveRegion.announce).toHaveBeenCalledTimes(1);
        const spoken = panel._liveRegion.announce.mock.calls[0][0];
        expect(spoken).toMatch(/first\.bin failed/);
        expect(spoken).toMatch(/second\.bin succeeded/);
    });

    test('a refresh after a reconnect does not announce what changed while disconnected', async () => {
        const panel = refreshingPanel([{ id: 'dl-2', title: 'old.zip', kind: 'remote-download', state: 'failed', version: 3, acceptedAt: '2026-09-26T10:00:00Z' }]);
        showHeard(panel, [{ id: 'dl-2', title: 'old.zip', kind: 'remote-download', state: 'running', version: 2, acceptedAt: '2026-09-26T10:00:00Z' }]);
        // Disconnected, then caught up again: the rows were read under the old stream.
        panel._streamGeneration += 1;

        await panel.refresh();

        expect(panel.jobs[0].state).toBe('failed');
        expect(panel._liveRegion.announce).not.toHaveBeenCalled();
    });

    test('the first refresh never announces the rows it loads', async () => {
        const panel = refreshingPanel([{ id: 'dl-3', title: 'x.bin', kind: 'remote-download', state: 'failed', version: 3, acceptedAt: '2026-09-26T10:00:00Z' }]);
        panel.streamCaughtUp = true;
        await panel.refresh();
        expect(panel._liveRegion.announce).not.toHaveBeenCalled();
    });

    // A job's whole life can fit inside one publish tick: its accepted, started
    // and failed events arrive together, with no snapshot, so the first read of
    // it already finds the outcome. That read is its first sight.
    async function deliverLive(panel: any, jobId: string, events: [string, number][], firstSequence: number) {
        let sequence = firstSequence;
        for (const [type, jobVersion] of events) {
            await panel.handleStreamMessage({
                data: JSON.stringify({ id: `e-${jobId}-${jobVersion}`, jobId, jobVersion, type, deliverySequence: sequence }),
                lastEventId: `v2:${sequence}`,
            });
            sequence += 1;
        }
    }
    const fastLife: [string, number][] = [['accepted', 1], ['started', 2], ['failed', 3]];

    test('a job accepted and failed within one tick is announced once, with its reason', async () => {
        const failed = { id: 'dl-40', title: 'fast.bin', kind: 'remote-download', state: 'failed', version: 3, acceptedAt: '2026-09-26T10:00:00Z', failure: { code: 'http-status', message: 'HTTP 404 Not Found' } };
        const panel = refreshingPanel([failed]);
        panel.lastSequence = 10;
        showHeard(panel, []);

        await deliverLive(panel, 'dl-40', fastLife, 11);
        expect(panel._liveRegion.announce).not.toHaveBeenCalled();
        await panel.refresh();

        expect(panel._liveRegion.announce).toHaveBeenCalledTimes(1);
        expect(panel._liveRegion.announce).toHaveBeenCalledWith('fast.bin failed: HTTP 404 Not Found.');
        await panel.refresh();
        await deliverLive(panel, 'dl-40', fastLife, 11);
        expect(panel._liveRegion.announce).toHaveBeenCalledTimes(1);
    });

    test('a job a read first finds finished is announced when its live events follow the read', async () => {
        const succeeded = { id: 'dl-41', title: 'instant.png', kind: 'remote-download', state: 'succeeded', version: 3, acceptedAt: '2026-09-26T10:00:00Z' };
        const panel = refreshingPanel([succeeded]);
        panel.lastSequence = 10;
        showHeard(panel, []);

        // Another job's event scheduled this refresh; this job's own events
        // are published on the next tick.
        await panel.refresh();
        expect(panel._liveRegion.announce).not.toHaveBeenCalled();
        await deliverLive(panel, 'dl-41', [['accepted', 1], ['started', 2], ['succeeded', 3]], 11);

        expect(panel._liveRegion.announce).toHaveBeenCalledTimes(1);
        expect(panel._liveRegion.announce).toHaveBeenCalledWith('instant.png succeeded.');
        await panel.refresh();
        expect(panel._liveRegion.announce).toHaveBeenCalledTimes(1);
    });

    test('the arrival of work is not news; its outcome is', async () => {
        const base = { id: 'dl-42', title: 'slow.bin', kind: 'remote-download', acceptedAt: '2026-09-26T10:00:00Z' };
        const panel = refreshingPanel([{ ...base, state: 'running', version: 2 }]);
        panel.lastSequence = 10;
        showHeard(panel, []);

        await deliverLive(panel, 'dl-42', [['accepted', 1], ['started', 2]], 11);
        await panel.refresh();
        expect(panel._liveRegion.announce).not.toHaveBeenCalled();

        panel.requestJSON = vi.fn(async raw => {
            const states = new URL(String(raw), 'http://localhost').searchParams.getAll('state');
            return { jobs: states.includes('succeeded') ? [{ ...base, state: 'succeeded', version: 3 }] : [] };
        });
        await deliverLive(panel, 'dl-42', [['succeeded', 3]], 13);
        await panel.refresh();

        expect(panel._liveRegion.announce).toHaveBeenCalledTimes(1);
        expect(panel._liveRegion.announce).toHaveBeenCalledWith('slow.bin succeeded.');
    });

    test('a job that finished before the stream caught up is history, and one that finishes after is news', async () => {
        class FakeEventSource {
            listeners = new Map<string, Function>();
            constructor(public url: string) {}
            addEventListener(name: string, callback: Function) { this.listeners.set(name, callback); }
            close() {}
        }
        vi.stubGlobal('EventSource', FakeEventSource);
        const old = { id: 'dl-43', title: 'old.bin', kind: 'remote-download', state: 'failed', version: 3, acceptedAt: '2026-09-26T09:00:00Z', failure: { message: 'HTTP 500' } };
        const fresh = { id: 'dl-44', title: 'fresh.bin', kind: 'remote-download', state: 'failed', version: 3, acceptedAt: '2026-09-26T10:00:00Z', failure: { message: 'HTTP 403 Forbidden' } };
        let listed = [old];
        const panel = refreshingPanel([]);
        panel.requestJSON = vi.fn(async raw => {
            const url = String(raw);
            if (url.startsWith('/v1/jobs?')) {
                const states = new URL(url, 'http://localhost').searchParams.getAll('state');
                return { jobs: listed.filter(job => states.includes(job.state)) };
            }
            return { ...listed.find(job => url.endsWith(job.id)), commands: [] };
        });
        panel.connect();
        const stream = panel.eventSource as unknown as FakeEventSource;
        const send = (message: object, sequence: number) => stream.listeners.get('job')?.({ data: JSON.stringify({ ...message, deliverySequence: sequence }), lastEventId: `v2:${sequence}` });

        // The page loads: the first read and the replay both see a job that
        // finished before it connected.
        await panel.refresh();
        await send({ id: 'e-1', jobId: 'dl-43', jobVersion: 1, type: 'accepted' }, 1);
        await send({ id: 'e-2', jobId: 'dl-43', jobVersion: 3, type: 'failed' }, 2);
        stream.listeners.get('job-caught-up')?.({ data: JSON.stringify({ cursor: 'v2:2' }) });
        await panel.refresh();
        expect(panel._liveRegion.announce).not.toHaveBeenCalled();

        // Disconnected while a second job ran and failed; the replay brings it.
        stream.listeners.get('error')?.({});
        listed = [fresh, old];
        await send({ id: 'e-3', jobId: 'dl-44', jobVersion: 1, type: 'accepted' }, 3);
        await send({ id: 'e-4', jobId: 'dl-44', jobVersion: 3, type: 'failed' }, 4);
        stream.listeners.get('job-caught-up')?.({ data: JSON.stringify({ cursor: 'v2:4' }) });
        await panel.refresh();
        expect(panel._liveRegion.announce).not.toHaveBeenCalled();

        // A third job accepted and failed after catch-up is news.
        const late = { ...fresh, id: 'dl-45', title: 'late.bin', acceptedAt: '2026-09-26T11:00:00Z' };
        listed = [late, fresh, old];
        await send({ id: 'e-5', jobId: 'dl-45', jobVersion: 1, type: 'accepted' }, 5);
        await send({ id: 'e-6', jobId: 'dl-45', jobVersion: 3, type: 'failed' }, 6);
        await panel.refresh();

        expect(panel._liveRegion.announce).toHaveBeenCalledTimes(1);
        expect(panel._liveRegion.announce).toHaveBeenCalledWith('late.bin failed: HTTP 403 Forbidden.');
        panel.destroy();
    });

    // More live lifecycle events than the ledger remembers can arrive between a
    // job's own events and the read that first sees it, from other jobs.
    const otherJobsBurst = async (panel: any, count: number, firstSequence: number) => {
        for (let index = 0; index < count; index++) {
            await deliverLive(panel, `other-${firstSequence + index}`, [['queued', 1]], firstSequence + index);
        }
    };

    test('a first-seen outcome is still said after a burst of other jobs\' events larger than the ledger', async () => {
        const failed = { id: 'dl-49', title: 'buried.bin', kind: 'remote-download', state: 'failed', version: 3, acceptedAt: '2026-09-26T10:00:00Z', failure: { message: 'HTTP 404 Not Found' } };
        const panel = refreshingPanel([failed]);
        panel.lastSequence = 10;
        showHeard(panel, []);

        await deliverLive(panel, 'dl-49', fastLife, 11);
        await otherJobsBurst(panel, 1500, 20);
        await panel.refresh();

        expect(panel._liveRegion.announce).toHaveBeenCalledTimes(1);
        expect(panel._liveRegion.announce).toHaveBeenCalledWith('buried.bin failed: HTTP 404 Not Found.');
    });

    test('a burst that arrives while the refresh reading a first-seen outcome is in flight does not lose it', async () => {
        const failed = { id: 'dl-50', title: 'inflight.bin', kind: 'remote-download', state: 'failed', version: 3, acceptedAt: '2026-09-26T10:00:00Z' };
        const panel = refreshingPanel([failed]);
        const list = panel.requestJSON;
        let releaseLists = () => {};
        const listsHeld = new Promise<void>(resolve => { releaseLists = resolve; });
        panel.requestJSON = vi.fn(async (raw: string) => {
            if (String(raw).startsWith('/v1/jobs?')) await listsHeld;
            return list(raw);
        });
        panel.lastSequence = 10;
        showHeard(panel, []);

        await deliverLive(panel, 'dl-50', fastLife, 11);
        const refreshing = panel.refresh();
        await otherJobsBurst(panel, 1500, 20);
        releaseLists();
        await refreshing;

        expect(panel._liveRegion.announce).toHaveBeenCalledWith('inflight.bin failed.');
    });

    test('a refresh whose reads failed keeps the proofs for the next one, however many arrived', async () => {
        const failed = { id: 'dl-51', title: 'retried.bin', kind: 'remote-download', state: 'failed', version: 3, acceptedAt: '2026-09-26T10:00:00Z' };
        const panel = refreshingPanel([failed]);
        const list = panel.requestJSON;
        panel.lastSequence = 10;
        showHeard(panel, []);

        await deliverLive(panel, 'dl-51', fastLife, 11);
        await otherJobsBurst(panel, 1500, 20);
        panel.requestJSON = vi.fn(async () => { throw new Error('Request failed (503)'); });
        await panel.refresh();
        expect(panel.error).toBe('Request failed (503)');
        await otherJobsBurst(panel, 1, 2000);
        panel.requestJSON = list;
        await panel.refresh();

        expect(panel._liveRegion.announce).toHaveBeenCalledTimes(1);
        expect(panel._liveRegion.announce).toHaveBeenCalledWith('retried.bin failed.');
    });

    test('a read that leaves a job out keeps its proof, and the read that first lists it says the outcome', async () => {
        const failed = { id: 'dl-52', title: 'paged-out.bin', kind: 'remote-download', state: 'failed', version: 3, acceptedAt: '2026-09-26T09:00:00Z' };
        const panel = refreshingPanel([]);
        panel.lastSequence = 10;
        showHeard(panel, []);

        await deliverLive(panel, 'dl-52', fastLife, 11);
        // Newer failures fill the Needs attention page, so this read succeeds
        // without the job.
        await panel.refresh();
        expect(panel._liveRegion.announce).not.toHaveBeenCalled();
        // More events than the store holds follow, from other jobs.
        await otherJobsBurst(panel, 1500, 20);
        expect(panel._liveVersions.has('dl-52')).toBe(true);

        // The newer failures are dismissed, and the job moves up into the page.
        panel.requestJSON = refreshingPanel([failed]).requestJSON;
        await panel.refresh();

        expect(panel._liveRegion.announce).toHaveBeenCalledTimes(1);
        expect(panel._liveRegion.announce).toHaveBeenCalledWith('paged-out.bin failed.');
    });

    test('a full proof store drops a proof that is not an outcome before an unread outcome', async () => {
        const panel = refreshingPanel([]);
        panel.lastSequence = 10;
        showHeard(panel, []);

        await deliverLive(panel, 'dl-54', [['failed', 3]], 11);
        await otherJobsBurst(panel, 1500, 20);

        expect(panel._liveVersions.size).toBe(1000);
        expect(panel._liveVersions.has('dl-54')).toBe(true);
    });

    test('outcomes the proof store cannot keep while reads keep failing are said once, as a count', async () => {
        vi.useFakeTimers();
        const panel = refreshingPanel([]);
        panel.requestJSON = vi.fn(async () => { throw new Error('Request failed (503)'); });
        panel.lastSequence = 10;
        showHeard(panel, []);

        for (let index = 0; index < 1500; index++) {
            await deliverLive(panel, `lost-${index}`, [['failed', 3]], 20 + index);
            if (index % 500 === 499) await panel.refresh();
            expect(panel._liveVersions.size).toBeLessThanOrEqual(1000);
        }
        expect(panel.error).toBe('Request failed (503)');
        await vi.advanceTimersByTimeAsync(1000);

        const said = panel._liveRegion.announce.mock.calls.map((call: any[]) => call[0]);
        expect(said).toEqual(['500 jobs finished or need attention; see the Jobs panel.']);
        panel.destroy();
        vi.useRealTimers();
    });

    test('an outcome of a job heard before a reconnect is counted when the full store drops its proof', async () => {
        vi.useFakeTimers();
        const running = { id: 'dl-56', title: 'reconnected.bin', kind: 'remote-download', state: 'running', version: 2, acceptedAt: '2026-09-26T10:00:00Z' };
        const panel = refreshingPanel([{ ...running, state: 'failed', version: 3 }]);
        panel.lastSequence = 10;
        showHeard(panel, [running]);
        panel.dropStream();
        panel.markStreamCaughtUp({ data: JSON.stringify({ cursor: 'v2:10' }) });

        await deliverLive(panel, 'dl-56', [['failed', 3]], 11);
        for (let index = 0; index < 1000; index++) {
            await deliverLive(panel, `unread-${index}`, [['failed', 3]], 20 + index);
        }
        expect(panel._liveVersions.has('dl-56')).toBe(false);
        await panel.refresh();
        await vi.advanceTimersByTimeAsync(1000);

        const said = panel._liveRegion.announce.mock.calls.map((call: any[]) => call[0]);
        expect(said).toEqual(['1 job finished or needs attention; see the Jobs panel.']);
        panel.destroy();
        vi.useRealTimers();
    });

    test('an outcome delivered live just before a disconnect is counted, and the read after the reconnect adds nothing', async () => {
        vi.useFakeTimers();
        const failed = { id: 'dl-53', title: 'dropped.bin', kind: 'remote-download', state: 'failed', version: 3, acceptedAt: '2026-09-26T10:00:00Z' };
        const panel = refreshingPanel([failed]);
        panel.lastSequence = 10;
        showHeard(panel, []);

        await deliverLive(panel, 'dl-53', fastLife, 11);
        panel.dropStream();
        // A refresh while the stream is reconnecting.
        await panel.refresh();
        panel.markStreamCaughtUp({ data: JSON.stringify({ cursor: 'v2:13' }) });
        await panel.refresh();
        await vi.advanceTimersByTimeAsync(1000);

        const said = panel._liveRegion.announce.mock.calls.map((call: any[]) => call[0]);
        expect(said).toEqual(['1 job finished or needs attention; see the Jobs panel.']);
        panel.destroy();
        vi.useRealTimers();
    });

    // A refresh begun while the stream reconnects answers after its catch-up.
    function straddlingPanel(listed: any[]) {
        const panel = refreshingPanel(listed);
        const list = panel.requestJSON;
        let releaseLists = () => {};
        const listsHeld = new Promise<void>(resolve => { releaseLists = resolve; });
        panel.requestJSON = vi.fn(async (raw: string) => {
            if (String(raw).startsWith('/v1/jobs?')) await listsHeld;
            return list(raw);
        });
        return { panel, list, releaseLists };
    }

    test('an outcome made while disconnected stays silent when a read straddles the catch-up', async () => {
        vi.useFakeTimers();
        // Rows that carry their commands: no detail read follows the lists.
        const failed = { id: 'dl-57', title: 'straddled.bin', kind: 'remote-download', state: 'failed', version: 3, acceptedAt: '2026-09-26T10:00:00Z', commands: [{ key: 'retry' }] };
        const { panel, list, releaseLists } = straddlingPanel([failed]);
        panel.lastSequence = 10;
        showHeard(panel, []);

        await deliverLive(panel, 'dl-57', [['accepted', 1]], 11);
        panel.dropStream();
        // The job fails while disconnected; the refresh begun now answers after
        // the catch-up, and the next one reads it again.
        const straddling = panel.refresh();
        panel.markStreamCaughtUp({ data: JSON.stringify({ cursor: 'v2:13' }) });
        releaseLists();
        await straddling;
        panel.requestJSON = list;
        await panel.refresh();
        await vi.advanceTimersByTimeAsync(1000);

        expect(panel._liveRegion.announce).not.toHaveBeenCalled();
        panel.destroy();
        vi.useRealTimers();
    });

    test('a detail read in a refresh that straddles the catch-up does not swallow a live outcome', async () => {
        vi.useFakeTimers();
        const failed = { id: 'dl-58', title: 'swallowed.bin', kind: 'remote-download', state: 'failed', version: 3, acceptedAt: '2026-09-26T10:00:00Z' };
        const { panel, list, releaseLists } = straddlingPanel([failed]);
        panel.lastSequence = 10;
        showHeard(panel, []);

        panel.dropStream();
        const straddling = panel.refresh();
        panel.markStreamCaughtUp({ data: JSON.stringify({ cursor: 'v2:13' }) });
        // Published just after the catch-up, so delivered live: news.
        await deliverLive(panel, 'dl-58', fastLife, 14);
        releaseLists();
        await straddling;
        panel.requestJSON = list;
        await panel.refresh();
        await vi.advanceTimersByTimeAsync(1000);

        const said = panel._liveRegion.announce.mock.calls.map((call: any[]) => call[0]);
        expect(said).toEqual(['swallowed.bin failed.']);
        panel.destroy();
        vi.useRealTimers();
    });

    test('a live outcome a refresh reads is said when its lists land, whatever happens to its detail reads', async () => {
        vi.useFakeTimers();
        const failed = { id: 'dl-59', title: 'held-out.bin', kind: 'remote-download', state: 'failed', version: 3, acceptedAt: '2026-09-26T10:00:00Z' };
        const panel = refreshingPanel([failed]);
        const list = panel.requestJSON;
        let releaseDetail = () => {};
        const detailHeld = new Promise<void>(resolve => { releaseDetail = resolve; });
        let detailAsked = () => {};
        const detailStarted = new Promise<void>(resolve => { detailAsked = resolve; });
        panel.requestJSON = vi.fn(async (raw: string) => {
            if (!String(raw).startsWith('/v1/jobs?')) {
                detailAsked();
                await detailHeld;
            }
            return list(raw);
        });
        panel.lastSequence = 10;
        panel.isOpen = true;
        showHeard(panel, []);

        await deliverLive(panel, 'dl-59', fastLife, 11);
        const refreshing = panel.refresh();
        await detailStarted;
        // The lists have been read and said; the stream drops before the detail answers.
        panel.dropStream();
        releaseDetail();
        await refreshing;
        panel.markStreamCaughtUp({ data: JSON.stringify({ cursor: 'v2:13' }) });
        panel.requestJSON = list;
        await panel.refresh();
        await vi.advanceTimersByTimeAsync(1000);

        const said = panel._liveRegion.announce.mock.calls.map((call: any[]) => call[0]);
        expect(said).toEqual(['held-out.bin failed.']);
        expect(panel._unsaidOutcomes).toBe(0);
        panel.destroy();
        vi.useRealTimers();
    });

    test('a refresh that says a held outcome releases its proof', async () => {
        const failed = { id: 'dl-60', title: 'released.bin', kind: 'remote-download', state: 'failed', version: 3, acceptedAt: '2026-09-26T10:00:00Z' };
        const panel = refreshingPanel([failed]);
        panel.lastSequence = 10;
        showHeard(panel, []);

        await deliverLive(panel, 'dl-60', fastLife, 11);
        await panel.refresh();

        expect(panel._liveRegion.announce).toHaveBeenCalledWith('released.bin failed.');
        expect(panel._liveVersions.has('dl-60')).toBe(false);
        panel.dropStream();
        expect(panel._unsaidOutcomes).toBe(0);
    });

    test('overlapping refreshes say a live outcome once, and a drop after it is said counts nothing', async () => {
        vi.useFakeTimers();
        const base = { id: 'dl-61', title: 'overlap.bin', kind: 'remote-download', acceptedAt: '2026-09-26T10:00:00Z' };
        const panel = refreshingPanel([]);
        panel.lastSequence = 10;
        panel.isOpen = true;
        showHeard(panel, [{ ...base, state: 'queued', version: 1 }]);
        let listed: any[] = [{ ...base, state: 'running', version: 2 }];
        const gates: Array<() => void> = [];
        panel.requestJSON = vi.fn(async (raw: string) => {
            const url = String(raw);
            if (url.startsWith('/v1/jobs?')) {
                const states = new URL(url, 'http://localhost').searchParams.getAll('state');
                return { jobs: listed.filter(job => states.includes(job.state)) };
            }
            const answer = { ...listed[0], commands: [] };
            await new Promise<void>(resolve => gates.push(resolve));
            return answer;
        });

        const first = panel.refresh();
        await vi.waitFor(() => expect(gates).toHaveLength(1));
        await deliverLive(panel, 'dl-61', [['failed', 3]], 11);
        listed = [{ ...base, state: 'failed', version: 3 }];
        const second = panel.refresh();
        await vi.waitFor(() => expect(panel.jobs[0]?.state).toBe('failed'));
        panel.dropStream();
        gates.forEach(release => release());
        await first;
        await second;
        gates.forEach(release => release());
        await vi.advanceTimersByTimeAsync(1000);

        const said = panel._liveRegion.announce.mock.calls.map((call: any[]) => call[0]);
        expect(said).toEqual(['overlap.bin running.', 'overlap.bin failed.']);
        panel.destroy();
        vi.useRealTimers();
    });

    test('a count still on its way to the region survives a drop and rides the next message', async () => {
        vi.useFakeTimers();
        const panel = refreshingPanel([]);
        panel.lastSequence = 10;
        showHeard(panel, []);

        panel.countUnsaidOutcome();
        await vi.advanceTimersByTimeAsync(1000);
        expect(panel._liveRegion.announce).toHaveBeenLastCalledWith('1 job finished or needs attention; see the Jobs panel.');
        // Before the region has spoken it, the stream drops and a notice follows.
        await vi.advanceTimersByTimeAsync(20);
        panel.dropStream();
        panel.announceNotice('A dialog is open. Close it before opening Jobs.');

        expect(panel._liveRegion.announce).toHaveBeenLastCalledWith(
            '1 job finished or needs attention; see the Jobs panel. A dialog is open. Close it before opening Jobs.');
        panel.destroy();
        vi.useRealTimers();
    });

    test('a count is carried until its message has landed, not only for the coalescing window', async () => {
        vi.useFakeTimers();
        const panel = refreshingPanel([]);
        panel.lastSequence = 10;
        showHeard(panel, []);

        panel.countUnsaidOutcome();
        await vi.advanceTimersByTimeAsync(1000);
        // The clock reaches the end of the window before the region's timer runs.
        vi.setSystemTime(Date.now() + 50);
        panel.announceNotice('A dialog is open. Close it before opening Jobs.');

        expect(panel._liveRegion.announce).toHaveBeenLastCalledWith(
            '1 job finished or needs attention; see the Jobs panel. A dialog is open. Close it before opening Jobs.');
        // Once that message has landed, the count is not said again.
        await vi.advanceTimersByTimeAsync(60);
        panel.announceNotice('A dialog is open. Close it before opening Jobs.');
        expect(panel._liveRegion.announce).toHaveBeenLastCalledWith('A dialog is open. Close it before opening Jobs.');
        panel.destroy();
        vi.useRealTimers();
    });

    test('a job accepted live that failed while disconnected is history after the reconnect', async () => {
        const failed = { id: 'dl-55', title: 'meanwhile.bin', kind: 'remote-download', state: 'failed', version: 3, acceptedAt: '2026-09-26T10:00:00Z' };
        const panel = refreshingPanel([failed]);
        panel.lastSequence = 10;
        showHeard(panel, []);

        await deliverLive(panel, 'dl-55', [['accepted', 1]], 11);
        panel.dropStream();
        panel.markStreamCaughtUp({ data: JSON.stringify({ cursor: 'v2:13' }) });
        await panel.refresh();

        expect(panel._liveRegion.announce).not.toHaveBeenCalled();
        expect(panel._liveVersions.has('dl-55')).toBe(false);
    });

    test('a read that began before catch-up withholds a first-seen outcome for its live event', async () => {
        const failed = { id: 'dl-46', title: 'straddle.bin', kind: 'remote-download', state: 'failed', version: 3, acceptedAt: '2026-09-26T10:00:00Z' };
        const panel = refreshingPanel([]);
        panel.lastSequence = 10;
        showHeard(panel, []);
        const staleGeneration = panel._streamGeneration;
        panel._streamGeneration += 1;
        const spoken: any[] = [];
        panel.hearFromRead(failed, staleGeneration, spoken);
        expect(spoken).toEqual([]);

        await deliverLive(panel, 'dl-46', [['failed', 3]], 11);

        expect(panel._liveRegion.announce).toHaveBeenCalledTimes(1);
        expect(panel._liveRegion.announce).toHaveBeenCalledWith('straddle.bin failed.');
    });

    test('a live snapshot of a job first seen at its outcome is said, and one of new work is not', async () => {
        const panel = refreshingPanel([]);
        panel.lastSequence = 10;
        showHeard(panel, []);
        await panel.handleStreamMessage({
            data: JSON.stringify({ job: { id: 'dl-47', title: 'queued.bin', kind: 'remote-download', state: 'queued', version: 1 }, deliverySequence: 11 }),
            lastEventId: 'v2:11',
        });
        await panel.handleStreamMessage({
            data: JSON.stringify({ job: { id: 'dl-48', title: 'done.bin', kind: 'remote-download', state: 'succeeded', version: 3 }, deliverySequence: 12 }),
            lastEventId: 'v2:12',
        });

        expect(panel._liveRegion.announce).toHaveBeenCalledTimes(1);
        expect(panel._liveRegion.announce).toHaveBeenCalledWith('done.bin succeeded.');
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

    test('a detail read that straddles the catch-up records its job as history', async () => {
        let resolveDetail: (value: unknown) => void = () => {};
        let markDetailStarted: () => void = () => {};
        const detailRequest = new Promise(resolve => { resolveDetail = resolve; });
        const detailStarted = new Promise<void>(resolve => { markDetailStarted = resolve; });
        const panel = jobPanel();
        panel.isOpen = true;
        const visibleJob = { id: 'job-1', title: 'Index rebuild', state: 'queued', version: 1, acceptedAt: '2026-09-23T10:00:00Z' };
        panel._liveRegion = { announce: vi.fn(), destroy: vi.fn() } as any;
        panel.requestJSON = vi.fn(async raw => {
            const url = String(raw);
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
        expect(listRequests.every(url => new URL(url, 'http://localhost').searchParams.get('order') === 'stateEntered')).toBe(true);
        expect(panel.jobs).toHaveLength(15);
        // Closed, the drawer shows no commands, so it reads none.
        expect(detailRequests).toEqual([]);
        panel.isOpen = true;
        await panel.loadStaleDetails();
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
        expect(detailRequests).toEqual([]);
        expect(panel.jobs).toHaveLength(15);
        panel.isOpen = true;
        await panel.loadStaleDetails();
        expect(detailRequests).toHaveLength(15);
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
        panel.isOpen = true;
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
        panel.isOpen = true;
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
        panel.isOpen = true;
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

        await vi.waitFor(() => expect(panel.commandsFor(panel.jobs[0])).toEqual(commands));
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

    test('announces from inside the open drawer, which is aria-modal, and from the page otherwise', () => {
        vi.useFakeTimers();
        const announcer = { textContent: 'stale', isConnected: true };
        vi.stubGlobal('document', {
            querySelector: vi.fn((selector: string) =>
                selector === '#job-center-panel [data-job-panel-announcer]' ? announcer : null),
        });
        const panel = jobPanel();
        const page: string[] = [];
        panel._liveRegion = { announce: vi.fn((message: string) => page.push(message)), destroy: vi.fn() } as any;

        panel.isOpen = true;
        panel.announce('video.mp4 failed: HTTP 403 Forbidden.');
        expect(announcer.textContent).toBe('');
        vi.advanceTimersByTime(50);
        expect(announcer.textContent).toBe('video.mp4 failed: HTTP 403 Forbidden.');
        expect(page).toEqual([]);

        panel.isOpen = false;
        panel.announce('clip.mp4 failed.');
        expect(page).toEqual(['clip.mp4 failed.']);
        vi.useRealTimers();
    });

    test('a drawer announcement lands where the drawer is when it lands, and the newest wins', () => {
        vi.useFakeTimers();
        let announcer = { textContent: '', isConnected: true };
        vi.stubGlobal('document', {
            querySelector: vi.fn((selector: string) =>
                selector === '#job-center-panel [data-job-panel-announcer]' ? announcer : null),
        });
        const panel = jobPanel();
        const page: string[] = [];
        panel._liveRegion = { announce: vi.fn((message: string) => page.push(message)), destroy: vi.fn() } as any;

        // Queued inside the drawer; the drawer closes (x-if removes its region)
        // and it lands on the page instead.
        panel.isOpen = true;
        panel.announce('A failed.');
        panel.isOpen = false;
        announcer.isConnected = false;
        vi.advanceTimersByTime(50);
        expect(page).toEqual(['A failed.']);

        // The same, but a newer failure is said on the page before A lands:
        // only the newer one is said.
        page.length = 0;
        announcer = { textContent: '', isConnected: true };
        panel.isOpen = true;
        panel.announce('B failed.');
        panel.isOpen = false;
        announcer.isConnected = false;
        vi.advanceTimersByTime(20);
        panel.announce('C failed.');
        vi.advanceTimersByTime(100);
        expect(page).toEqual(['C failed.']);

        // Closed and reopened before it lands: said inside the new dialog.
        page.length = 0;
        announcer = { textContent: '', isConnected: true };
        panel.isOpen = true;
        panel.announce('D failed.');
        panel.isOpen = false;
        announcer.isConnected = false;
        panel.isOpen = true;
        announcer = { textContent: '', isConnected: true };
        vi.advanceTimersByTime(50);
        expect(announcer.textContent).toBe('D failed.');
        expect(page).toEqual([]);
        vi.useRealTimers();
    });

    test('the newest announcement wins across the page region and the drawer region', () => {
        vi.useFakeTimers();
        let announcer = { textContent: '', isConnected: true };
        vi.stubGlobal('document', {
            querySelector: vi.fn((selector: string) =>
                selector === '#job-center-panel [data-job-panel-announcer]' ? announcer : null),
        });
        // A page region with createLiveRegion's own delay and cancel.
        const spoken: string[] = [];
        const page = {
            pending: null as string | null, timer: undefined as any,
            announce(message: string) {
                clearTimeout(this.timer);
                this.pending = message;
                this.timer = setTimeout(() => { spoken.push(message); this.pending = null; }, 50);
            },
            cancel() { clearTimeout(this.timer); const message = this.pending; this.pending = null; return message; },
            destroy() {},
        };
        const panel = jobPanel();
        panel._liveRegion = page as any;

        // A said on the page; the drawer opens and B is said inside it; the
        // drawer closes before B lands. Only B is said, and on the page.
        panel.announce('A failed.');
        vi.advanceTimersByTime(5);
        panel.isOpen = true;
        panel.announce('B failed.');
        panel.isOpen = false;
        announcer.isConnected = false;
        vi.advanceTimersByTime(200);
        expect(spoken).toEqual(['B failed.']);

        // A said on the page just before the drawer opens: said inside it.
        spoken.length = 0;
        announcer = { textContent: '', isConnected: true };
        panel.announce('C failed.');
        panel.isOpen = true;
        panel.adoptPendingAnnouncement();
        vi.advanceTimersByTime(200);
        expect(announcer.textContent).toBe('C failed.');
        expect(spoken).toEqual([]);

        // Nothing pending: opening says nothing.
        announcer.textContent = '';
        panel.adoptPendingAnnouncement();
        vi.advanceTimersByTime(200);
        expect(announcer.textContent).toBe('');
        vi.useRealTimers();
    });

    test('the drawer carries its own polite status region, inside the dialog', () => {
        const template = readFileSync(fileURLToPath(new URL('../../templates/partials/jobPanel.tpl', import.meta.url)), 'utf8');
        const dialogStart = template.indexOf('role="dialog" aria-modal="true"');
        const announcer = template.indexOf('data-job-panel-announcer');
        const dialogEnd = template.indexOf('</section>', template.lastIndexOf('</footer>'));
        expect(dialogStart).toBeGreaterThan(-1);
        expect(announcer).toBeGreaterThan(dialogStart);
        expect(announcer).toBeLessThan(dialogEnd);
        expect(template).toContain('role="status" aria-live="polite" aria-atomic="true" data-job-panel-announcer');
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
        expect(template).toContain('x-if="resultOutput(job)"');
        expect(template).toContain(':href="resultURL(job)"');
        expect(template).toContain(':aria-label="resultAccessibleLabel(job)"');
        expect(template).toContain('x-text="resultLinkLabel(job)"');
        expect(template).toContain('x-show="job.pinned"');
        expect(template).toContain('Pinned by you');
        // The two count cards above the list repeated the trigger's badges and
        // the section headings; the counts live on the trigger alone.
        expect(template).not.toContain('Active and scheduled jobs shown');
        expect(template).toContain('x-for="command in primaryCommandsFor(job)"');
        expect(template).toContain('x-for="command in moreCommandsFor(job)"');
        // The trigger's aria-label is its name, so the badges reach assistive
        // technology through its description, not their own labels.
        expect(template).toContain('aria-describedby="job-panel-trigger-counts"');
        expect(template).toContain('<span id="job-panel-trigger-counts" class="sr-only" x-text="countsText"></span>');
        expect(template).not.toContain('Clear completed');
        expect(baseTemplate).toContain('{% include "/partials/jobPanel.tpl" %}');
        expect(baseTemplate).not.toContain('downloadCockpit.tpl');
    });
});

describe('Job Center panel rows', () => {
    test('keeps the commands that act on the work inline and moves record keeping under More', () => {
        const commands = [
            { key: 'retry' }, { key: 'dismiss' }, { key: 'pin' }, { key: 'pin-lineage' },
            { key: 'forget' }, { key: 'unpin' }, { key: 'cancel' },
        ];
        const { primary, more } = panelCommandSplit(commands);
        expect(primary.map(command => command.key)).toEqual(['retry', 'dismiss', 'cancel']);
        expect(more.map(command => command.key)).toEqual(['pin', 'pin-lineage', 'forget', 'unpin']);
        expect(panelCommandSplit(undefined)).toEqual({ primary: [], more: [] });
    });

    test('names a tone for each state, which colours the status icon and pill', () => {
        expect(panelStateTone({ state: 'running' })).toBe('working');
        expect(panelStateTone({ state: 'queued' })).toBe('waiting');
        expect(panelStateTone({ state: 'scheduled' })).toBe('waiting');
        expect(panelStateTone({ state: 'paused' })).toBe('paused');
        expect(panelStateTone({ state: 'succeeded' })).toBe('done');
        expect(panelStateTone({ state: 'succeeded', phase: 'partial' })).toBe('warning');
        expect(panelStateTone({ state: 'blocked' })).toBe('warning');
        expect(panelStateTone({ state: 'failed' })).toBe('failed');
        expect(panelStateTone({ state: 'interrupted' })).toBe('failed');
        expect(panelStateTone({ state: 'cancelled' })).toBe('neutral');
        expect(panelStateTone({})).toBe('neutral');
    });

    test('the panel exposes the split per job, after the pin filter', () => {
        const panel = jobPanel();
        const job = { id: 'j', pinned: true, commands: [{ key: 'retry' }, { key: 'pin' }, { key: 'unpin' }] };
        expect(panel.primaryCommandsFor(job).map(command => command.key)).toEqual(['retry']);
        expect(panel.moreCommandsFor(job).map(command => command.key)).toEqual(['unpin']);
    });
});

describe('Job Center trigger counts', () => {
    test('describes both badges in words, including zero, as counts of what is shown', () => {
        expect(panelCountsText({ active: 0, attention: 0 })).toBe('Showing 0 active or scheduled jobs and 0 needing attention');
        expect(panelCountsText({ active: 1, attention: 9 })).toBe('Showing 1 active or scheduled job and 9 needing attention');
        expect(panelCountsText(undefined)).toBe('Showing 0 active or scheduled jobs and 0 needing attention');
    });
});

describe('Job Center drawer connection and list reads', () => {
    class ClosingEventSource {
        static made: ClosingEventSource[] = [];
        listeners = new Map<string, Function>();
        readyState = 1;
        closed = false;
        constructor(public url: string) { ClosingEventSource.made.push(this); }
        addEventListener(name: string, callback: Function) { this.listeners.set(name, callback); }
        close() { this.closed = true; this.readyState = 2; }
        // The browser answering a non-200 response: the source closes for good.
        refuse() { this.readyState = 2; this.listeners.get('error')?.({}); }
        // A dropped connection the browser reconnects itself.
        drop() { this.readyState = 0; this.listeners.get('error')?.({}); }
    }

    beforeEach(() => {
        ClosingEventSource.made = [];
        vi.stubGlobal('EventSource', ClosingEventSource);
    });

    function listingPanel(rows: any[]) {
        const panel = jobPanel();
        panel._liveRegion = { announce: vi.fn(), destroy: vi.fn(), cancel: vi.fn() } as any;
        const listURLs: URL[] = [];
        panel.requestJSON = vi.fn(async (raw: string) => {
            const url = new URL(String(raw), 'http://localhost');
            if (url.pathname === '/v1/jobs') {
                listURLs.push(url);
                return { jobs: rows.filter(job => url.searchParams.getAll('state').includes(job.state)) };
            }
            const id = decodeURIComponent(url.pathname.slice('/v1/jobs/'.length));
            return { ...rows.find(job => job.id === id), commands: [] };
        });
        return { panel, listURLs };
    }

    test('a stream the browser gave up on is opened again, resuming from its cursor, and the lists are read meanwhile', async () => {
        vi.useFakeTimers();
        const { panel, listURLs } = listingPanel([{ id: 'a', state: 'running', version: 1, acceptedAt: '2026-09-27T10:00:00Z' }]);
        panel.connect();
        const first = ClosingEventSource.made[0];
        first.listeners.get('job-caught-up')?.({ data: JSON.stringify({ cursor: 'v2:40' }) });
        await vi.advanceTimersByTimeAsync(200);
        const readsBefore = listURLs.length;

        first.refuse();
        expect(first.closed).toBe(true);
        expect(panel.eventSource).toBe(null);
        expect(panel.connectionText).toBe('Reconnecting');
        await vi.advanceTimersByTimeAsync(0);
        expect(listURLs.length).toBe(readsBefore + 3);

        await vi.advanceTimersByTimeAsync(1000);
        const second = ClosingEventSource.made[1];
        expect(second).toBeDefined();
        expect(new URL(second.url, 'http://localhost').searchParams.get('cursor')).toBe('v2:40');

        // Refused again, it waits twice as long before the next attempt.
        second.refuse();
        await vi.advanceTimersByTimeAsync(1500);
        expect(ClosingEventSource.made).toHaveLength(2);
        await vi.advanceTimersByTimeAsync(600);
        expect(ClosingEventSource.made).toHaveLength(3);

        // A catch-up resets the delay.
        ClosingEventSource.made[2].listeners.get('job-caught-up')?.({ data: JSON.stringify({ cursor: 'v2:41' }) });
        expect(panel._streamRetryDelay).toBe(0);
        panel.destroy();
        vi.useRealTimers();
    });

    test('a stream that caught up at v2:0 is reopened from v2:0, not from the head', async () => {
        vi.useFakeTimers();
        const { panel } = listingPanel([]);
        panel.connect();
        ClosingEventSource.made[0].listeners.get('job-caught-up')?.({ data: JSON.stringify({ cursor: 'v2:0' }) });
        ClosingEventSource.made[0].refuse();
        await vi.advanceTimersByTimeAsync(1000);
        const reopened = new URL(ClosingEventSource.made[1].url, 'http://localhost').searchParams;
        expect(reopened.get('cursor')).toBe('v2:0');
        expect(reopened.has('start')).toBe(false);
        panel.destroy();
        vi.useRealTimers();
    });

    test('a destroyed drawer reads, retries and reopens nothing', async () => {
        vi.useFakeTimers();
        const { panel } = listingPanel([{ id: 'a', state: 'failed', version: 1, acceptedAt: '2026-09-27T10:00:00Z' }]);
        panel.isOpen = true;
        await panel.refresh();
        let failDetail = () => {};
        panel.requestJSON = vi.fn(() => new Promise((_resolve, reject) => { failDetail = () => reject(new Error('gone')); }));
        panel.details = {};
        const pending = panel.loadStaleDetails();
        panel.connect();
        panel.destroy();
        failDetail();
        await pending;
        ClosingEventSource.made.at(-1)?.refuse();
        await vi.advanceTimersByTimeAsync(120000);
        expect(panel._listRetryTimer).toBe(null);
        expect(panel.requestJSON).toHaveBeenCalledTimes(1);
        expect(ClosingEventSource.made).toHaveLength(1);
        vi.useRealTimers();
    });

    test('a detail read answered after the row\'s pin changed is read again rather than undoing the pin', async () => {
        const rows = [{ id: 'p', state: 'failed', version: 2, acceptedAt: '2026-09-27T10:00:00Z', pinned: false }];
        const { panel } = listingPanel(rows);
        panel.isOpen = true;
        let releaseFirst = () => {};
        const firstHeld = new Promise<void>(resolve => { releaseFirst = resolve; });
        const list = panel.requestJSON;
        const answers: boolean[] = [];
        panel.requestJSON = vi.fn(async (raw: string) => {
            if (String(raw).startsWith('/v1/jobs?')) return list(raw);
            const pinned = rows[0].pinned;
            if (answers.push(pinned) === 1) await firstHeld;
            return { ...rows[0], pinned, commands: [{ key: pinned ? 'unpin' : 'pin', jobVersion: 2 }] };
        });

        await panel.refresh();
        await vi.waitFor(() => expect(answers).toEqual([false]));
        // Pinned in another tab: no version moves, and a read lists it pinned.
        rows[0] = { ...rows[0], pinned: true };
        await panel.refresh();
        releaseFirst();
        await vi.waitFor(() => expect(answers).toEqual([false, true]));
        await vi.waitFor(() => expect(panel.commandsFor(panel.jobs[0])).toEqual([{ key: 'unpin', jobVersion: 2 }]));
        expect(panel.jobs[0].pinned).toBe(true);
    });

    test('overlapping detail passes share four readers', async () => {
        const rows = Array.from({ length: 12 }, (_, index) => ({ id: `d-${index}`, state: 'failed', version: 1, acceptedAt: '2026-09-27T10:00:00Z' }));
        const { panel } = listingPanel(rows);
        const list = panel.requestJSON;
        let inFlight = 0;
        let most = 0;
        const gates: Array<() => void> = [];
        panel.requestJSON = vi.fn(async (raw: string) => {
            if (String(raw).startsWith('/v1/jobs?')) return list(raw);
            inFlight += 1;
            most = Math.max(most, inFlight);
            await new Promise<void>(resolve => gates.push(resolve));
            inFlight -= 1;
            return list(raw);
        });
        await panel.refresh();
        panel.isOpen = true;
        const first = panel.loadStaleDetails();
        const second = panel.loadStaleDetails();
        await vi.waitFor(async () => {
            while (gates.length) gates.shift()!();
            await Promise.resolve();
            expect(Object.keys(panel.details)).toHaveLength(12);
        });
        await Promise.all([first, second]);
        expect(most).toBe(4);
        expect(panel.requestJSON.mock.calls.filter(([raw]: any[]) => !String(raw).startsWith('/v1/jobs?'))).toHaveLength(12);
    });

    test('a pin read answered after another page unpinned the job does not pin it again', async () => {
        const rows = [{ id: 'q', state: 'failed', version: 2, acceptedAt: '2026-09-27T10:00:00Z', pinned: false }];
        const { panel } = listingPanel(rows);
        await panel.refresh();
        let releaseRead = () => {};
        const readHeld = new Promise<void>(resolve => { releaseRead = resolve; });
        const list = panel.requestJSON;
        panel.requestJSON = vi.fn(async (raw: string) => {
            if (String(raw).startsWith('/v1/jobs?')) return list(raw);
            await readHeld;
            // Read before the other page's unpin.
            return { ...rows[0], pinned: true, commands: [{ key: 'unpin', jobVersion: 2 }] };
        });
        // This tab pinned; its follow-up read is on its way when another page unpins.
        const reading = panel.refreshJobPreference('q');
        panel.hearPreferenceBroadcast({ command: 'unpin', jobIds: ['q'] });
        releaseRead();
        await reading;
        await vi.waitFor(() => expect(list).toBeDefined());
        expect(panel.details.q?.pinned).not.toBe(true);
        expect(panel.jobs[0].pinned).toBe(false);
    });

    test('Forget in this tab reads the row\'s detail again, so Retry and Forget leave with the replay input', async () => {
        const rows = [{ id: 'f', state: 'failed', version: 4, acceptedAt: '2026-09-27T10:00:00Z' }];
        let forgotten = false;
        const panel = jobPanel();
        panel._liveRegion = { announce: vi.fn(), destroy: vi.fn(), cancel: vi.fn() } as any;
        panel.isOpen = true;
        vi.stubGlobal('fetch', vi.fn(async (raw: string, init: any = {}) => {
            const url = new URL(String(raw), 'http://localhost');
            if (init.method === 'POST') {
                forgotten = true;
                return { ok: true, json: async () => ({ result: { status: 'succeeded', job: { ...rows[0] } } }) };
            }
            if (url.pathname === '/v1/jobs') {
                return { ok: true, json: async () => ({ jobs: url.searchParams.getAll('state').includes('failed') ? rows : [] }) };
            }
            // Forget moves no version; only the offered commands change.
            const commands = forgotten ? [{ key: 'dismiss', jobVersion: 4 }] : [{ key: 'retry', jobVersion: 4 }, { key: 'forget', jobVersion: 4 }];
            return { ok: true, json: async () => ({ ...rows[0], commands }) };
        }));
        await panel.refresh();
        await vi.waitFor(() => expect(panel.commandsFor(panel.jobs[0]).map((command: any) => command.key)).toEqual(['retry', 'forget']));

        await panel.requestJSON('/v1/jobs/f/commands/forget', { method: 'POST', body: '{}' });
        await vi.waitFor(() => expect(panel.commandsFor(panel.jobs[0]).map((command: any) => command.key)).toEqual(['dismiss']));
    });

    test('a list read begun before this tab pinned a job does not unpin its row', async () => {
        const row = { id: 'r', state: 'failed', version: 5, acceptedAt: '2026-09-27T10:00:00Z', pinned: false };
        let pinned = false;
        const panel = jobPanel();
        panel._liveRegion = { announce: vi.fn(), destroy: vi.fn(), cancel: vi.fn() } as any;
        let releaseOld = () => {};
        const oldHeld = new Promise<void>(resolve => { releaseOld = resolve; });
        let lists = 0;
        vi.stubGlobal('fetch', vi.fn(async (raw: string, init: any = {}) => {
            const url = new URL(String(raw), 'http://localhost');
            if (init.method === 'POST') {
                pinned = true;
                return { ok: true, json: async () => ({ result: { status: 'succeeded' } }) };
            }
            if (url.pathname === '/v1/jobs') {
                lists += 1;
                const answer = { ...row, pinned };
                if (lists <= 3) await oldHeld;
                return { ok: true, json: async () => ({ jobs: url.searchParams.getAll('state').includes('failed') ? [answer] : [] }) };
            }
            return { ok: true, json: async () => ({ ...row, pinned, commands: [] }) };
        }));
        panel.jobs = [row];
        panel.startScheduledPanelRefresh();
        await vi.waitFor(() => expect(lists).toBe(3));
        await panel.requestJSON('/v1/jobs/r/commands/pin', { method: 'POST', body: '{}' });
        releaseOld();
        await vi.waitFor(() => expect(lists).toBe(6));
        await vi.waitFor(() => expect(panel.jobs[0]?.pinned).toBe(true));
        await new Promise(resolve => setTimeout(resolve, 10));
        expect(panel.jobs[0].pinned).toBe(true);
    });

    test('Dismiss finished dismisses what the drawer lists, in its owner scope', async () => {
        const panel = jobPanel();
        panel._liveRegion = { announce: vi.fn(), destroy: vi.fn(), cancel: vi.fn() } as any;
        panel.ownerScope = 'me';
        panel.jobs = [{ id: 'mine', state: 'succeeded', version: 1, commands: [{ key: 'dismiss', jobVersion: 1 }] }];
        panel.details.mine = panel.jobs[0];
        const lists: URL[] = [];
        panel.requestJSON = vi.fn(async (raw: string, init: any = {}) => {
            if (init.method === 'POST') return { results: [{ jobId: 'mine', status: 'succeeded', code: 'applied' }] };
            const url = new URL(String(raw), 'http://localhost');
            lists.push(url);
            return { jobs: url.searchParams.get('limit') === '200' ? [{ id: 'mine', state: 'succeeded', version: 1 }] : [] };
        });
        await panel.dismissFinished();
        const walk = lists.filter(url => url.searchParams.get('limit') === '200');
        expect(walk.length).toBeGreaterThan(0);
        expect(walk.every(url => url.searchParams.get('owner') === 'me')).toBe(true);
    });

    test('a stream the browser reconnects itself is left to the browser', async () => {
        vi.useFakeTimers();
        const { panel } = listingPanel([]);
        panel.connect();
        ClosingEventSource.made[0].drop();
        await vi.advanceTimersByTimeAsync(60000);
        expect(ClosingEventSource.made).toHaveLength(1);
        expect(panel.eventSource).toBe(ClosingEventSource.made[0]);
        panel.destroy();
        vi.useRealTimers();
    });

    test('a 401 says the session ended and offers a sign-in back to this page, and a later read recovers', async () => {
        vi.useFakeTimers();
        vi.stubGlobal('location', { pathname: '/notes', search: '?tag=3' });
        const panel = jobPanel();
        let signedIn = false;
        panel.requestJSON = vi.fn(async () => {
            if (!signedIn) {
                const error: any = new Error('authentication required');
                error.status = 401;
                throw error;
            }
            return { jobs: [] };
        });

        await panel.refresh();
        expect(panel.signedOut).toBe(true);
        expect(panel.error).toBe('');
        expect(panel.connectionText).toBe('Signed out');
        expect(panel.countsText).toBe('Signed out; jobs are not shown');
        expect(panel.signInURL).toBe('/login?next=%2Fnotes%3Ftag%3D3');

        signedIn = true;
        await vi.advanceTimersByTimeAsync(2000);
        await panel._panelRefreshPromise;
        expect(panel.signedOut).toBe(false);
        expect(panel.loaded).toBe(true);
        panel.destroy();
        vi.useRealTimers();
    });

    test('a failed read keeps its rows, says why, and is read again after a growing delay', async () => {
        vi.useFakeTimers();
        const rows = [{ id: 'a', state: 'failed', version: 1, acceptedAt: '2026-09-27T10:00:00Z' }];
        const { panel } = listingPanel(rows);
        await panel.refresh();
        expect(panel.jobs).toHaveLength(1);

        const good = panel.requestJSON;
        let attempts = 0;
        panel.requestJSON = vi.fn(async () => {
            attempts += 1;
            throw new Error('database is locked');
        });
        await panel.refresh();
        expect(panel.error).toBe('database is locked');
        expect(panel.jobs).toHaveLength(1);
        expect(panel.countsText).toBe('Showing 0 active or scheduled jobs and 1 needing attention');

        const afterFirst = attempts;
        await vi.advanceTimersByTimeAsync(2000);
        expect(attempts).toBeGreaterThan(afterFirst);
        const afterSecond = attempts;
        // The next attempt waits twice as long.
        await vi.advanceTimersByTimeAsync(3000);
        expect(attempts).toBe(afterSecond);
        panel.requestJSON = good;
        await vi.advanceTimersByTimeAsync(1100);
        await panel._panelRefreshPromise;
        expect(panel.error).toBe('');
        panel.destroy();
        vi.useRealTimers();
    });

    test('a first read that fails leaves no counts, not zeroes, and opening the drawer tries again', async () => {
        const { panel } = listingPanel([{ id: 'a', state: 'failed', version: 1, acceptedAt: '2026-09-27T10:00:00Z' }]);
        const good = panel.requestJSON;
        panel.requestJSON = vi.fn(async () => { throw new Error('job request failed'); });
        await panel.refresh();
        expect(panel.loaded).toBe(false);
        expect(panel.countsText).toBe('Jobs could not be loaded');

        panel.requestJSON = good;
        panel.retryNow();
        await panel._panelRefreshPromise;
        expect(panel.loaded).toBe(true);
        expect(panel.attentionCount).toBe(1);
        expect(panel.countsText).toBe('Showing 0 active or scheduled jobs and 1 needing attention');
        panel.destroy();
    });

    test('a read in flight when the stream drops still applies its rows', async () => {
        const rows = [{ id: 'a', state: 'failed', version: 1, acceptedAt: '2026-09-27T10:00:00Z' }];
        const { panel } = listingPanel(rows);
        const list = panel.requestJSON;
        let release = () => {};
        const held = new Promise<void>(resolve => { release = resolve; });
        panel.requestJSON = vi.fn(async (raw: string) => { await held; return list(raw); });

        const reading = panel.refresh();
        panel.dropStream();
        release();
        await reading;

        expect(panel.jobs.map(job => job.id)).toEqual(['a']);
        expect(panel.loaded).toBe(true);
        expect(panel._liveRegion.announce).not.toHaveBeenCalled();
    });

    test('a slow first read is applied when a newer one is still on its way, and the newer one replaces it', async () => {
        const { panel } = listingPanel([]);
        const answers: Array<(rows: any[]) => void> = [];
        panel.requestJSON = vi.fn(async (raw: string) => {
            const states = new URL(String(raw), 'http://localhost').searchParams.getAll('state');
            const rows: any[] = await new Promise(resolve => answers.push(resolve));
            return { jobs: rows.filter(job => states.includes(job.state)) };
        });
        const old = [{ id: 'old', state: 'failed', version: 1, acceptedAt: '2026-09-27T10:00:00Z' }];
        const fresh = [{ id: 'fresh', state: 'failed', version: 1, acceptedAt: '2026-09-27T10:01:00Z' }];

        const first = panel.startScheduledPanelRefresh();
        await vi.waitFor(() => expect(answers).toHaveLength(3));
        panel._panelRefreshPromise = null;
        panel.startScheduledPanelRefresh();
        await vi.waitFor(() => expect(answers).toHaveLength(6));
        answers.slice(0, 3).forEach(answer => answer(old));
        await vi.waitFor(() => expect(panel.jobs.map(job => job.id)).toEqual(['old']));
        answers.slice(3).forEach(answer => answer(fresh));
        await vi.waitFor(() => expect(panel.jobs.map(job => job.id)).toEqual(['fresh']));
        await first;

        // An answer to a read older than one already applied is dropped.
        expect(panel._appliedGeneration).toBe(2);
    });

    test('an older read that succeeds after a newer one failed keeps the failure and its retry', async () => {
        vi.useFakeTimers();
        const { panel } = listingPanel([]);
        let releaseOld = () => {};
        const oldHeld = new Promise<void>(resolve => { releaseOld = resolve; });
        let calls = 0;
        panel.requestJSON = vi.fn(async () => {
            calls += 1;
            if (calls <= 3) { await oldHeld; return { jobs: [] }; }
            throw new Error('database is locked');
        });
        const older = panel.startScheduledPanelRefresh();
        panel._panelRefreshPromise = null;
        await panel.refresh();
        expect(panel.error).toBe('database is locked');
        releaseOld();
        await older;
        await vi.advanceTimersByTimeAsync(0);

        expect(panel.loaded).toBe(false);
        expect(panel.error).toBe('database is locked');
        expect(panel._listRetryTimer).not.toBe(null);
        panel.destroy();
        vi.useRealTimers();
    });

    test('a row that moves on while its detail is read has its detail read again', async () => {
        const rows = [{ id: 'm', state: 'running', version: 1, acceptedAt: '2026-09-27T10:00:00Z' }];
        const { panel } = listingPanel(rows);
        panel.isOpen = true;
        let releaseFirst = () => {};
        const firstHeld = new Promise<void>(resolve => { releaseFirst = resolve; });
        const detailVersions: number[] = [];
        const list = panel.requestJSON;
        panel.requestJSON = vi.fn(async (raw: string) => {
            if (String(raw).startsWith('/v1/jobs?')) return list(raw);
            const version = rows[0].version;
            if (detailVersions.push(version) === 1) await firstHeld;
            return { ...rows[0], version, commands: [{ key: 'cancel', jobVersion: version }] };
        });

        await panel.refresh();
        await vi.waitFor(() => expect(detailVersions).toEqual([1]));
        // The job moves on, a read lists it at version 2 while its version-1
        // detail is still in flight.
        rows[0] = { ...rows[0], state: 'failed', version: 2 };
        await panel.refresh();
        releaseFirst();
        await vi.waitFor(() => expect(panel.details.m?.version).toBe(2));
        expect(detailVersions).toEqual([1, 2]);
        expect(panel.commandsFor(panel.jobs[0])).toEqual([{ key: 'cancel', jobVersion: 2 }]);
    });

    test('a detail that keeps answering older than its row is asked again after a delay, not in a loop', async () => {
        vi.useFakeTimers();
        const rows = [{ id: 'lag', state: 'failed', version: 3, acceptedAt: '2026-09-27T10:00:00Z' }];
        const { panel } = listingPanel(rows);
        panel.isOpen = true;
        const list = panel.requestJSON;
        let details = 0;
        panel.requestJSON = vi.fn(async (raw: string) => {
            if (String(raw).startsWith('/v1/jobs?')) return list(raw);
            details += 1;
            return { ...rows[0], version: 1, commands: [] };
        });
        await panel.refresh();
        await vi.advanceTimersByTimeAsync(100);
        expect(details).toBe(1);
        await vi.advanceTimersByTimeAsync(2000);
        expect(details).toBe(2);
        panel.destroy();
        vi.useRealTimers();
    });

    test('a group holding only its first page says so, and its badge never shows the page as the total', async () => {
        const failures = Array.from({ length: 50 }, (_, index) => ({
            id: `f-${String(index).padStart(2, '0')}`, state: 'failed', version: 1, acceptedAt: '2026-09-27T10:00:00Z',
        }));
        const panel = jobPanel();
        panel.requestJSON = vi.fn(async (raw: string) => {
            const states = new URL(String(raw), 'http://localhost').searchParams.getAll('state');
            return states.includes('failed') ? { jobs: failures, nextCursor: 'more' } : { jobs: [] };
        });
        await panel.refresh();

        expect(panel.attentionBadge).toBe('50+');
        expect(panel.activeBadge).toBe('0');
        expect(panel.countsText).toBe('Showing 0 active or scheduled jobs and 50 needing attention, more on All jobs');
        const [attention] = panel.groups;
        expect(attention).toMatchObject({ key: 'attention', more: true, countText: '50+', moreText: 'Showing the 50 most recent.' });
        expect(new URL(attention.moreURL, 'http://localhost').searchParams.getAll('state')).toEqual(['blocked', 'failed', 'interrupted']);
        expect(new URL(attention.moreURL, 'http://localhost').searchParams.get('dismissed')).toBe('false');
    });

    test('groups are read and ordered by when each job entered its state, and the badge text marks a capped group', () => {
        expect(panelBadgeText(3, false)).toBe('3');
        expect(panelBadgeText(50, true)).toBe('50+');
        expect(panelGroupJobsURL(panelGroups(10)[2], 'me')).toBe('/jobs?state=succeeded&state=cancelled&dismissed=false&owner=me');
        const panel = jobPanel();
        // Accepted long ago, finished last: it leads Finished.
        panel.jobs = panel.bounded([
            { id: 'quick', state: 'succeeded', version: 3, acceptedAt: '2026-09-27T10:05:00Z', stateEnteredAt: '2026-09-27T10:05:01Z' },
            { id: 'long', state: 'succeeded', version: 3, acceptedAt: '2026-09-27T10:00:00Z', stateEnteredAt: '2026-09-27T10:30:00.1Z' },
            { id: 'tie', state: 'succeeded', version: 3, acceptedAt: '2026-09-27T10:00:00Z', stateEnteredAt: '2026-09-27T10:30:00.10Z' },
        ]);
        expect(panel.finishedJobs.map(job => job.id)).toEqual(['tie', 'long', 'quick']);
    });

    test('a closed drawer reads no detail; an open one reads each row once, and again only when its version or pin moves', async () => {
        const rows = [
            { id: 'a', state: 'running', version: 1, acceptedAt: '2026-09-27T10:00:00Z', pinned: false },
            { id: 'b', state: 'failed', version: 2, acceptedAt: '2026-09-27T10:01:00Z', pinned: false },
        ];
        const { panel } = listingPanel(rows);
        const details = () => panel.requestJSON.mock.calls.map(([raw]: any[]) => String(raw)).filter((url: string) => !url.startsWith('/v1/jobs?'));

        await panel.refresh();
        expect(details()).toEqual([]);

        panel.isOpen = true;
        await panel.loadStaleDetails();
        expect(details().sort()).toEqual(['/v1/jobs/a', '/v1/jobs/b']);

        await panel.refresh();
        expect(details()).toHaveLength(2);

        rows[0].version = 2;
        rows[1].pinned = true;
        await panel.refresh();
        expect(details().sort()).toEqual(['/v1/jobs/a', '/v1/jobs/a', '/v1/jobs/b', '/v1/jobs/b']);
    });

    test('a record-keeping command tells the other tabs, which read their lists again', async () => {
        expect(preferenceCommandJobIDs('/v1/jobs/j-1/commands/dismiss', { method: 'POST' })).toEqual(['j-1']);
        expect(preferenceCommandJobIDs('/v1/jobs/j-1/commands/pin', { method: 'POST' })).toEqual(['j-1']);
        expect(preferenceCommandJobIDs('/v1/jobs/commands/dismiss', { method: 'POST', body: JSON.stringify({ jobIds: ['x', 'y'] }) })).toEqual(['x', 'y']);
        expect(preferenceCommandJobIDs('/v1/jobs/j-1/commands/cancel', { method: 'POST' })).toBe(null);
        expect(preferenceCommandJobIDs('/v1/jobs/j-1', {})).toBe(null);

        const posted: any[] = [];
        const panel = jobPanel();
        panel._broadcast = { postMessage: (message: any) => posted.push(message), close: vi.fn() } as any;
        // The bulk answer says which Jobs it applied to; a refused one is not reported.
        vi.stubGlobal('fetch', vi.fn(async () => ({ ok: true, json: async () => ({ results: [
            { jobId: 'x', status: 'succeeded', code: 'applied' },
            { jobId: 'y', status: 'failed', code: 'conflict' },
        ] }) })));
        await panel.requestJSON('/v1/jobs/commands/dismiss', { method: 'POST', body: JSON.stringify({ jobIds: ['x', 'y'] }) });
        await panel.requestJSON('/v1/jobs/x/commands/retry', { method: 'POST', body: '{}' });
        expect(posted).toEqual([{ command: 'dismiss', jobIds: ['x'] }]);

        // The receiving tab takes the dismissed row away at once, and fences
        // every read begun before, so none of them puts it back.
        const { panel: other } = listingPanel([]);
        other.jobs = [{ id: 'x', state: 'succeeded', version: 1 }, { id: 'z', state: 'succeeded', version: 1 }];
        other.details.x = { id: 'x', version: 1 };
        let releaseStale = () => {};
        const staleHeld = new Promise<void>(resolve => { releaseStale = resolve; });
        const current = other.requestJSON;
        let reads = 0;
        other.requestJSON = vi.fn(async (raw: string) => {
            reads += 1;
            if (reads <= 3) {
                await staleHeld;
                return { jobs: [{ id: 'x', state: 'succeeded', version: 1 }] };
            }
            return current(raw);
        });
        const stale = other.startScheduledPanelRefresh();
        other.hearPreferenceBroadcast({ command: 'dismiss', jobIds: ['x'] });
        expect(other.details.x).toBeUndefined();
        expect(other.jobs.map((job: any) => job.id)).toEqual(['z']);
        releaseStale();
        await stale;
        await vi.waitFor(() => expect(reads).toBe(6));
        expect(other.jobs.some((job: any) => job.id === 'x')).toBe(false);
    });

    test('a scope change reads the lists and reopens the stream for that scope, and older reads never apply', async () => {
        const { panel, listURLs } = listingPanel([]);
        panel.connect();
        panel.lastSequence = 9;
        const before = panel.requestJSON;
        let release = () => {};
        const held = new Promise<void>(resolve => { release = resolve; });
        panel.requestJSON = vi.fn(async (raw: string) => {
            if (!String(raw).includes('owner=me')) {
                await held;
                return { jobs: [{ id: 'someone-else', state: 'failed', version: 1, acceptedAt: '2026-09-27T10:00:00Z' }] };
            }
            return before(raw);
        });
        const stale = panel.refresh();

        panel.setOwnerScope('me');
        release();
        await stale;
        await panel._panelRefreshPromise;
        await vi.waitFor(() => expect(listURLs.length).toBe(3));

        expect(listURLs.every(url => url.searchParams.get('owner') === 'me')).toBe(true);
        expect(panel.jobs).toEqual([]);
        expect(ClosingEventSource.made).toHaveLength(2);
        expect(ClosingEventSource.made[0].closed).toBe(true);
        const reopened = new URL(ClosingEventSource.made[1].url, 'http://localhost').searchParams;
        expect(reopened.get('owner')).toBe('me');
        expect(reopened.get('cursor')).toBe('v2:9');
    });

    test('a page reads its lists once its stream catches up, or after a wait when it does not', async () => {
        vi.useFakeTimers();
        const element = () => ({ setAttribute: vi.fn(), style: {}, remove: vi.fn(), textContent: '' });
        vi.stubGlobal('document', {
            addEventListener: vi.fn(), removeEventListener: vi.fn(), querySelector: vi.fn(() => null),
            createElement: element, body: { appendChild: vi.fn() },
        });
        vi.stubGlobal('window', { addEventListener: vi.fn(), removeEventListener: vi.fn() });
        const slow = listingPanel([]);
        slow.panel.$el = { dataset: {}, querySelector: () => null };
        slow.panel.init();
        await vi.advanceTimersByTimeAsync(1000);
        expect(slow.listURLs).toHaveLength(0);
        await vi.advanceTimersByTimeAsync(600);
        expect(slow.listURLs).toHaveLength(3);
        slow.panel.destroy();

        const quick = listingPanel([]);
        quick.panel.$el = { dataset: {}, querySelector: () => null };
        quick.panel.init();
        ClosingEventSource.made.at(-1)!.listeners.get('job-caught-up')?.({ data: JSON.stringify({ cursor: 'v2:5' }) });
        await vi.advanceTimersByTimeAsync(2000);
        expect(quick.listURLs).toHaveLength(3);
        quick.panel.destroy();
        vi.useRealTimers();
    });
});

describe('Job Center row focus', () => {
    test('a replaced command hands focus to its counterpart before itself', () => {
        expect(panelFocusSuccessorKeys('pin')).toEqual(['unpin', 'pin']);
        expect(panelFocusSuccessorKeys('unpin')).toEqual(['pin', 'unpin']);
        expect(panelFocusSuccessorKeys('pause')).toEqual(['resume', 'pause']);
        expect(panelFocusSuccessorKeys('retry')).toEqual(['retry']);
        expect(panelFocusSuccessorKeys(undefined)).toEqual([]);
    });

    test('the dialog carries the counts as its description', () => {
        const template = readFileSync(fileURLToPath(new URL('../../templates/partials/jobPanel.tpl', import.meta.url)), 'utf8');
        expect(template).toContain('aria-describedby="job-center-panel-counts"');
        expect(template).toContain('<p id="job-center-panel-counts" class="sr-only" x-text="countsText"></p>');
        expect(template.match(/:data-command-key="command.key"/g)).toHaveLength(2);
    });
});

describe('Job Center lifecycle event types', () => {
    const repo = (path: string) => fileURLToPath(new URL(`../../${path}`, import.meta.url));
    // Event types that record no transition; every other Event constant must be
    // one the panel treats as a live lifecycle event.
    const NOT_TRANSITIONS = new Set(['warning', 'events-truncated', 'output-published', 'output-expired', 'output-removed', 'control-requested']);

    test('every lifecycle Event constant in jobs/types.go is known to the panel', () => {
        const types = readFileSync(repo('jobs/types.go'), 'utf8');
        const constants = [...types.matchAll(/^\s*Event[A-Z]\w*\s*=\s*"([^"]+)"/gm)].map(match => match[1]);
        expect(constants.length).toBeGreaterThan(10);
        for (const type of constants) {
            if (!NOT_TRANSITIONS.has(type)) expect(panelLifecycleEvents.has(type), type).toBe(true);
        }
    });

    // The non-test Go sources of one package directory and its subpackages, each
    // package's files joined so its own constants can be resolved.
    const goPackages = (dir: string): string[] => {
        const own = readdirSync(dir, { withFileTypes: true });
        const files = own.filter(entry => entry.isFile() && entry.name.endsWith('.go') && !entry.name.endsWith('_test.go'));
        const nested = own.filter(entry => entry.isDirectory()).flatMap(entry => goPackages(join(dir, entry.name)));
        return [files.map(file => readFileSync(join(dir, file.name), 'utf8')).join('\n'), ...nested];
    };

    // Every type an EventInput is built with, whether a transition, a finish or
    // an appended event: a string literal, or a constant of the package that
    // builds it, read to its value, whatever the constant is called. Only a
    // qualified jobs.EventX from another package is left to the test above; an
    // operand of any other shape fails, so a new way of naming an event cannot
    // slip past this unread.
    const eventInputTypes = (): Set<string> => {
        const types = new Set<string>();
        for (const dir of ['application_context', 'jobs', 'download_queue', 'plugin_system', 'plugin_commands']) {
            for (const source of goPackages(repo(dir))) {
                const constants = new Map([...source.matchAll(/^\s*(?:const\s+)?(\w+)\s*(?:string\s*)?=\s*"([^"]*)"/gm)].map(match => [match[1], match[2]]));
                for (const [, raw] of source.matchAll(/EventInput\{\s*Type:\s*([^,}]+)/g)) {
                    const operand = raw.trim();
                    const literal = operand.match(/^"([^"]+)"$/);
                    if (literal) types.add(literal[1]);
                    else if (constants.has(operand)) types.add(constants.get(operand)!);
                    else if (/^jobs\.Event[A-Z]\w*$/.test(operand)) continue;
                    else throw new Error(`${dir}: EventInput type ${operand} is neither a literal nor a constant of its package`);
                }
            }
        }
        return types;
    };

    test('every event type a transition or finish carries is known to the panel', () => {
        const types = eventInputTypes();
        expect([...types]).toContain('not-started');
        for (const type of types) {
            if (!NOT_TRANSITIONS.has(type)) expect(panelLifecycleEvents.has(type), type).toBe(true);
        }
    });
});
