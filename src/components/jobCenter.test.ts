import { afterEach, describe, expect, test, vi } from 'vitest';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import {
    advertisedCommands,
    advertisedOutputs,
    buildJobListURL,
    buildJobSummaryURL,
    classifyJobState,
    commandEndpoint,
    dateTimeLocalValue,
    dateTimeQueryValue,
    JOB_COMMAND_FILTER_ENABLED,
    jobCenter,
    jobCommands,
    outputEndpoint,
    outputLinkAccessibleLabel,
    outputLinkLabel,
    outputJSONLinkURL,
    outputLinkURL,
    parseJobCenterURL,
    progressAccessibleText,
    progressText,
    progressValue,
    reduceJobStreamEvent,
    reduceJobSnapshot,
    selectedBulkCommands,
    serializeJobCenterURL,
    warningEvents,
} from './jobCenter.js';

const unfamiliarJob = {
    id: 'job-unknown-kind',
    kind: 'future-plugin-kind',
    state: 'queued',
    version: 4,
    acceptedAt: '2026-09-22T10:00:00Z',
    commands: [
        { key: 'inspect', label: 'Inspect payload', endpoint: '/v1/jobs/job-unknown-kind/commands/inspect', jobVersion: 4, bulk: true },
        { key: 'archive', label: 'Archive result', endpoint: '/v1/jobs/job-unknown-kind/commands/archive', jobVersion: 4, bulk: false },
    ],
    outputs: [
        { key: 'summary', type: 'summary', label: 'Summary report', url: '/v1/jobs/job-unknown-kind/outputs?key=summary', availability: 'available' },
    ],
};

afterEach(() => vi.unstubAllGlobals());

describe('Job Center API declarations', () => {
    test('individual pin controls follow viewer pin state and refresh after changing it', async () => {
        const pin = { key: 'pin', label: 'Pin job', jobVersion: 4, bulk: true };
        const unpin = { key: 'unpin', label: 'Unpin job', jobVersion: 4, bulk: false };
        const inspect = { key: 'inspect', label: 'Inspect', jobVersion: 4 };
        expect(jobCommands({ pinned: false, commands: [pin, unpin, inspect] }).map(command => command.key))
            .toEqual(['pin', 'inspect']);
        expect(jobCommands({ pinned: true, commands: [pin, unpin, inspect] }).map(command => command.key))
            .toEqual(['unpin', 'inspect']);

        const center = jobCenter();
        center.detail = { id: 'job-1', state: 'succeeded', version: 4, pinned: false, commands: [pin, unpin] } as any;
        center.jobs = [center.detail];
        vi.stubGlobal('Alpine', { store: () => ({ ask: vi.fn(async () => true) }) });
        center.fetchJSON = vi.fn(async (url: string, init: any = {}) => {
            if (init?.method === 'POST') return { result: { message: 'Pinned' } } as any;
            return { job: { id: 'job-1', state: 'succeeded', version: 4, pinned: true, commands: [pin, unpin] } } as any;
        });

        await center.runCommand(center.detail, pin);

        expect(center.fetchJSON).toHaveBeenCalledTimes(2);
        expect(center.fetchJSON.mock.calls[1][0]).toBe('/v1/jobs/job-1');
        expect(center.detail?.pinned).toBe(true);
        expect(center.commandsFor(center.detail).map(command => command.key)).toEqual(['unpin']);
    });

    test('uses advertised commands and outputs for an unfamiliar Kind', () => {
        expect(advertisedCommands(unfamiliarJob).map(command => command.key)).toEqual(['inspect', 'archive']);
        expect(advertisedOutputs(unfamiliarJob).map(output => output.key)).toEqual(['summary']);
        expect(commandEndpoint(unfamiliarJob, advertisedCommands(unfamiliarJob)[0]))
            .toBe('/v1/jobs/job-unknown-kind/commands/inspect');
        expect(outputEndpoint(advertisedOutputs(unfamiliarJob)[0]))
            .toBe('/v1/jobs/job-unknown-kind/outputs?key=summary');
    });

    test('runs the server-advertised control for a future Kind without inferring from source or state', async () => {
        const fetchMock = vi.fn(async () => ({ ok: true, json: async () => ({ message: 'Inspection started' }) }));
        vi.stubGlobal('fetch', fetchMock);
        const center = jobCenter();
        center._liveRegion = { announce: vi.fn() } as any;

        await center.runCommand(unfamiliarJob, advertisedCommands(unfamiliarJob)[0]);

        expect(fetchMock.mock.calls[0][0]).toBe('/v1/jobs/job-unknown-kind/commands/inspect');
        expect(JSON.parse(fetchMock.mock.calls[0][1].body)).toMatchObject({ expectedVersion: 4 });
    });

    test('posts the intersected bulk command and preserves each server outcome', async () => {
        const results = [
            { jobId: 'job-unknown-kind', key: 'inspect', status: 'succeeded', code: 'applied' },
            { jobId: 'job-second', key: 'inspect', status: 'failed', code: 'stale', message: 'Changed' },
        ];
        const fetchMock = vi.fn(async () => ({ ok: true, json: async () => ({ results }) }));
        vi.stubGlobal('fetch', fetchMock);
        const center = jobCenter();
        center.jobs = [unfamiliarJob, {
            ...unfamiliarJob,
            id: 'job-second',
            commands: [{ key: 'inspect', label: 'Inspect', endpoint: '/v1/jobs/job-second/commands/inspect', bulk: true }],
        }];
        center.selectedIds = new Set(['job-unknown-kind', 'job-second']);
        center.refreshCurrentView = vi.fn();

        const [command] = center.bulkCommands();
        await center.runBulkCommand(command);

        expect(fetchMock.mock.calls[0][0]).toBe('/v1/jobs/commands/inspect');
        expect(JSON.parse(fetchMock.mock.calls[0][1].body).jobIds).toEqual(['job-unknown-kind', 'job-second']);
        expect(center.bulkOutcomes).toEqual(results);
    });

    test('keeps expanded and selected row state across the list refresh after a bulk command', async () => {
        const command = { key: 'inspect', label: 'Inspect', bulk: true };
        const current = {
            ...unfamiliarJob,
            id: 'visible-job',
            state: 'running',
            version: 4,
            uiExpanded: true,
            uiSelected: true,
        };
        const refreshed = { ...current, state: 'paused', version: 5, uiExpanded: undefined, uiSelected: undefined };
        const center = jobCenter();
        center.jobs = [current];
        center.selectedIds = new Set([current.id]);
        center.fetchJSON = vi.fn(async input => {
            const url = new URL(String(input), 'http://localhost');
            if (url.pathname === '/v1/jobs/commands/inspect') {
                return { results: [{ jobId: current.id, key: 'inspect', status: 'succeeded', code: 'applied' }] };
            }
            if (url.pathname === '/v1/jobs/summary') return { byState: { paused: 1 } };
            return { jobs: url.searchParams.getAll('state').includes('paused') ? [refreshed] : [] };
        });

        await center.runBulkCommand(command);

        expect(center.jobs).toHaveLength(1);
        expect(center.jobs[0]).toMatchObject({ id: current.id, state: 'paused', uiExpanded: true, uiSelected: true });
        expect(center.selectedIds.has(current.id)).toBe(true);
    });

    test('recomputes bulk pin controls from refreshed rows when selected detail cache is stale', async () => {
        const job = { id: 'bulk-pin-job', state: 'succeeded', version: 4, pinned: false };
        const commands = [
            { key: 'pin', label: 'Pin', jobVersion: 4, bulk: true },
            { key: 'unpin', label: 'Unpin', jobVersion: 4, bulk: true },
            { key: 'inspect', label: 'Inspect', jobVersion: 4, bulk: true },
        ];
        const center = jobCenter();
        center.view = 'all';
        center.jobs = [job];
        center.details[job.id] = { ...job, commands };
        center.selectedIds = new Set([job.id]);
        vi.stubGlobal('Alpine', { store: () => ({ ask: vi.fn(async () => true) }) });
        center.fetchJSON = vi.fn(async (url: string) => {
            if (url.startsWith('/v1/jobs/commands/pin')) {
                return { results: [{ jobId: job.id, key: 'pin', status: 'succeeded', code: 'applied' }] } as any;
            }
            if (url.startsWith('/v1/jobs/summary')) return { byState: { succeeded: 1 } } as any;
            return { jobs: [{ ...job, pinned: true }] } as any;
        });

        await center.runBulkCommand(center.bulkCommands().find(command => command.key === 'pin')!);

        expect(center.details[job.id].pinned).toBe(false);
        expect(center.selectedJobs()[0]).toMatchObject({ id: job.id, pinned: true });
        expect(center.bulkCommands().map(command => command.key)).toEqual(['unpin', 'inspect']);
    });

    test('bulk actions are the intersection of selected advertised bulk commands', () => {
        const second = {
            ...unfamiliarJob,
            id: 'job-second',
            commands: [{ key: 'inspect', label: 'Inspect', endpoint: '/v1/jobs/job-second/commands/inspect', bulk: true }],
        };

        expect(selectedBulkCommands([unfamiliarJob, second], ['job-unknown-kind', 'job-second']).map(command => command.key))
            .toEqual(['inspect']);
        expect(selectedBulkCommands([unfamiliarJob, second], ['job-unknown-kind']).map(command => command.key))
            .toEqual(['inspect']);
        expect(selectedBulkCommands([unfamiliarJob], ['job-unknown-kind', 'job-not-loaded'])).toEqual([]);
    });

    test('bulk pin actions match the selected jobs while preserving common commands', () => {
        const pinCommands = [
            { key: 'pin', label: 'Pin', bulk: true },
            { key: 'unpin', label: 'Unpin', bulk: true },
            { key: 'inspect', label: 'Inspect', bulk: true },
        ];
        const first = { id: 'job-first', pinned: false, commands: [...pinCommands, { key: 'first-only', bulk: true }] };
        const second = { id: 'job-second', pinned: false, commands: pinCommands };
        const pinnedFirst = { ...first, pinned: true };
        const pinnedSecond = { ...second, pinned: true };

        expect(selectedBulkCommands([first, second], [first.id, second.id]).map(command => command.key))
            .toEqual(['pin', 'inspect']);
        expect(selectedBulkCommands([pinnedFirst, pinnedSecond], [first.id, second.id]).map(command => command.key))
            .toEqual(['unpin', 'inspect']);
        expect(selectedBulkCommands([first, pinnedSecond], [first.id, second.id]).map(command => command.key))
            .toEqual(['pin', 'unpin', 'inspect']);
    });

    test('the state text classifies a job without using its Kind or source', () => {
        expect(classifyJobState(unfamiliarJob)).toBe('active');
        expect(classifyJobState({ ...unfamiliarJob, state: 'blocked' })).toBe('attention');
        expect(classifyJobState({ ...unfamiliarJob, state: 'succeeded' })).toBe('finished');
    });
});

describe('Job Center URL state', () => {
    test('opens filtered legacy and shared links in the paginated all view', () => {
        const filtered = parseJobCenterURL('?kind=remote-download&state=failed');
        expect(filtered.view).toBe('all');
        expect(filtered.filters.kinds).toEqual(['remote-download']);
        expect(filtered.filters.states).toEqual(['failed']);
        expect(parseJobCenterURL('?pinned=false').view).toBe('all');
        expect(parseJobCenterURL('').view).toBe('home');
    });

    test('round-trips each filter, multi-value dimension, view and keyset cursor', () => {
        const state = {
            view: 'all',
            filters: {
                search: 'staged archive',
                command: 'retry',
                kinds: ['remote-download', 'plugin-command-run'],
                states: ['failed', 'blocked'],
                origins: ['user', 'schedule'],
                ownerId: '12',
                actorId: '13',
                acceptedAfter: '2026-09-01T00:00:00.000Z',
                acceptedBefore: '2026-09-20T00:00:00.000Z',
                relationship: 'retry-of',
                pinned: true,
                dismissed: false,
            },
            cursor: 'list-v1-cursor-token',
        };

        expect(parseJobCenterURL(serializeJobCenterURL(state))).toEqual(state);
    });

    test('builds canonical list requests with repeated filter values and cursor fields', () => {
        const url = new URL(buildJobListURL({
            filters: { kinds: ['a', 'b'], states: ['running', 'paused'], search: 'report', acceptedAfter: '2026-09-23T12:30' },
            cursor: 'list-v1-next-page',
            limit: 50,
        }), 'http://localhost');

        expect(url.pathname).toBe('/v1/jobs');
        expect(url.searchParams.getAll('kind')).toEqual(['a', 'b']);
        expect(url.searchParams.getAll('state')).toEqual(['running', 'paused']);
        expect(url.searchParams.get('search')).toBe('report');
        expect(url.searchParams.get('cursor')).toBe('list-v1-next-page');
        expect(url.searchParams.get('acceptedAfter')).toBe(new Date('2026-09-23T12:30').toISOString());
        expect(url.searchParams.has('command')).toBe(false);
    });

    test('builds summary requests from the same filters as the visible list without list pagination', () => {
        const filters = {
            search: 'staged archive',
            command: 'retry',
            kinds: ['remote-download', 'plugin-command-run'],
            states: ['failed', 'blocked'],
            origins: ['user', 'schedule'],
            ownerId: '12',
            actorId: '13',
            acceptedAfter: '2026-09-01T00:00:00.000Z',
            acceptedBefore: '2026-09-20T00:00:00.000Z',
            relationship: 'retry-of',
            pinned: true,
            dismissed: false,
        };
        const summary = new URL(buildJobSummaryURL({ filters }), 'http://localhost');

        expect(summary.pathname).toBe('/v1/jobs/summary');
        expect(summary.searchParams.get('search')).toBe('staged archive');
        expect(summary.searchParams.get('command')).toBe('retry');
        expect(summary.searchParams.getAll('kind')).toEqual(['remote-download', 'plugin-command-run']);
        expect(summary.searchParams.getAll('state')).toEqual(['failed', 'blocked']);
        expect(summary.searchParams.getAll('origin')).toEqual(['user', 'schedule']);
        expect(summary.searchParams.get('ownerId')).toBe('12');
        expect(summary.searchParams.get('actorId')).toBe('13');
        expect(summary.searchParams.get('acceptedAfter')).toBe('2026-09-01T00:00:00.000Z');
        expect(summary.searchParams.get('acceptedBefore')).toBe('2026-09-20T00:00:00.000Z');
        expect(summary.searchParams.get('relationship')).toBe('retry-of');
        expect(summary.searchParams.get('pinned')).toBe('true');
        expect(summary.searchParams.get('dismissed')).toBe('false');
        expect(summary.searchParams.has('cursor')).toBe(false);
        expect(summary.searchParams.has('limit')).toBe(false);
        const list = new URL(buildJobListURL({ filters, cursor: 'list-v1-page-two', limit: 50 }), 'http://localhost');
        expect(list.searchParams.get('cursor')).toBe('list-v1-page-two');
        expect(list.searchParams.get('limit')).toBe('50');
        expect(new URL(buildJobSummaryURL({ filters: { search: 'download' } }), 'http://localhost')
            .searchParams.get('dismissed')).toBe('false');
    });

    test('loads the next opaque keyset cursor and retains newest-first order', async () => {
        const pages = [
            { jobs: [
                { id: 'older', acceptedAt: '2026-09-20T10:00:00Z' },
                { id: 'newer', acceptedAt: '2026-09-22T10:00:00Z' },
            ], nextCursor: 'list-v1.next-page' },
            { jobs: [{ id: 'oldest', acceptedAt: '2026-09-18T10:00:00Z' }] },
        ];
        const fetchMock = vi.fn(async () => ({ ok: true, json: async () => pages.shift() }));
        vi.stubGlobal('fetch', fetchMock);
        const replaceState = vi.fn();
        vi.stubGlobal('location', { pathname: '/jobs', search: '' });
        vi.stubGlobal('history', { replaceState });
        const center = jobCenter();
        center.view = 'all';

        await center.loadAll();
        await center.loadMore();

        expect(new URL(fetchMock.mock.calls[0][0], 'http://localhost').searchParams.get('cursor')).toBeNull();
        expect(new URL(fetchMock.mock.calls[1][0], 'http://localhost').searchParams.get('cursor')).toBe('list-v1.next-page');
        expect(center.jobs.map(job => job.id)).toEqual(['newer', 'older', 'oldest']);
        expect(center.nextCursor).toBeNull();
        expect(replaceState.mock.calls.at(-1)[2]).toContain('cursor=list-v1.next-page');
    });

    test('does not refresh the current keyset page for an off-screen streamed Job', async () => {
        const visible = { ...unfamiliarJob, id: 'visible-job' };
        const center = jobCenter();
        center.view = 'all';
        center.filters = { ...center.filters, states: ['failed'] };
        center.jobs = [visible];
        center.nextCursor = 'opaque-page-two-cursor';
        center.fetchJSON = vi.fn(async () => ({ id: 'off-screen-job', state: 'failed', version: 3 }));
        center.refreshCurrentView = vi.fn();

        await center.handleStreamMessage({
            data: JSON.stringify({
                id: 'event-21', jobId: 'off-screen-job', sequence: 21, jobVersion: 3,
                type: 'failed', deliverySequence: 21, createdAt: '2026-09-23T12:00:00Z',
            }),
            lastEventId: 'v2:21',
        });
        await Promise.resolve();

        expect(center.jobs).toEqual([visible]);
        expect(center.nextCursor).toBe('opaque-page-two-cursor');
        expect(center.fetchJSON).not.toHaveBeenCalled();
        expect(center.refreshCurrentView).not.toHaveBeenCalled();
    });

    test('refreshes filtered All membership and its loaded window after streamed state changes', async () => {
        vi.useFakeTimers();
        const leaving = { ...unfamiliarJob, id: 'running-leaves', state: 'running', version: 1, acceptedAt: '2026-09-23T10:03:00Z' };
        const staying = { ...unfamiliarJob, id: 'running-stays', state: 'running', version: 1, acceptedAt: '2026-09-23T10:02:00Z', uiExpanded: true, uiSelected: true };
        const initialTail = { ...unfamiliarJob, id: 'old-page-two', state: 'running', version: 1, acceptedAt: '2026-09-23T10:01:00Z' };
        const newlyMatching = { ...unfamiliarJob, id: 'running-enters', state: 'running', version: 2, acceptedAt: '2026-09-23T10:04:00Z' };
        const refreshedTail = { ...unfamiliarJob, id: 'new-page-two', state: 'running', version: 1, acceptedAt: '2026-09-23T10:00:00Z' };
        const center = jobCenter();
        center.view = 'all';
        center.filters = { ...center.filters, states: ['running'] };
        center.jobs = [leaving, staying];
        center.selectedIds = new Set([staying.id]);
        let serverChanged = false;
        center.fetchJSON = vi.fn(async raw => {
            const url = new URL(String(raw), 'http://localhost');
            if (url.pathname === '/v1/jobs/summary') return { byState: { running: 2, succeeded: 0 } };
            const cursor = url.searchParams.get('cursor');
            if (!serverChanged) {
                return cursor === 'initial-page-two'
                    ? { jobs: [initialTail] }
                    : { jobs: [leaving, staying], nextCursor: 'initial-page-two' };
            }
            return cursor === 'refreshed-page-two'
                ? { jobs: [refreshedTail] }
                : { jobs: [newlyMatching, staying], nextCursor: 'refreshed-page-two' };
        });

        await center.loadAll();
        await center.loadMore();
        expect(center.jobs.map(job => job.id)).toEqual([leaving.id, staying.id, initialTail.id]);

        serverChanged = true;
        center.streamCaughtUp = true;
        await center.handleStreamMessage({
            data: JSON.stringify({ id: leaving.id, state: 'succeeded', version: 2, deliverySequence: 1 }),
            lastEventId: 'v2:1',
        });
        await vi.advanceTimersByTimeAsync(150);
        await center._streamRefreshPromise;

        expect(center.jobs.map(job => job.id)).toEqual([newlyMatching.id, staying.id, refreshedTail.id]);
        expect(center.jobs.find(job => job.id === staying.id)).toMatchObject({ uiExpanded: true, uiSelected: true });
        expect(center.selectedIds.has(staying.id)).toBe(true);
        expect(center.nextCursor).toBeNull();
        const refreshedRequests = center.fetchJSON.mock.calls
            .map(([url]) => new URL(String(url), 'http://localhost'))
            .filter(url => url.pathname === '/v1/jobs' && url.searchParams.getAll('state').includes('running'));
        expect(refreshedRequests.slice(-2).map(url => url.searchParams.get('cursor')))
            .toEqual([null, 'refreshed-page-two']);
        center.destroy();
        vi.useRealTimers();
    });

    test('converts RFC3339 URL timestamps for local controls and datetime-local values for the API', () => {
        const timestamp = '2026-09-23T12:30:00.000Z';
        const local = dateTimeLocalValue(timestamp);

        expect(new Date(local).toISOString()).toBe(timestamp);
        expect(dateTimeQueryValue(local)).toBe(timestamp);
    });
});

describe('canonical event reducer', () => {
    test('keeps incoming snapshots inside the currently loaded keyset window', () => {
        const visible = { ...unfamiliarJob, id: 'visible-job', uiExpanded: true };
        const result = reduceJobStreamEvent([visible], {
            deliverySequence: 8,
            job: { ...unfamiliarJob, id: 'new-unrelated-job', acceptedAt: '2026-09-23T12:00:00Z' },
        }, 7);

        expect(result.jobs).toEqual([visible]);
        expect(result.changed).toBe(false);
        expect(result.lastSequence).toBe(8);
    });

    test('dedupes canonical v2 DTOs by delivery ID and requests the current visible snapshot', () => {
        const existing = { ...unfamiliarJob, uiExpanded: true, uiSelected: true };
        const event = {
            id: 'event-9', jobId: existing.id, sequence: 14, jobVersion: 5,
            type: 'failed', deliverySequence: 9, createdAt: '2026-09-22T10:01:00Z',
        };
        const first = reduceJobStreamEvent([existing], event, 8);
        expect(first).toMatchObject({ lastSequence: 9, changed: true, needsSnapshot: true, jobId: existing.id });
        expect(first.jobs[0]).toBe(existing);

        const duplicate = reduceJobStreamEvent([existing], { ...event, lastEventId: 'v2:9' }, first.lastSequence);
        expect(duplicate.lastSequence).toBe(9);
        expect(duplicate.changed).toBe(false);
    });

    test('snapshot updates preserve row focus state and move rows by lifecycle class', () => {
        const existing = { ...unfamiliarJob, uiExpanded: true, uiSelected: true };
        const changed = reduceJobSnapshot([existing], {
            ...unfamiliarJob, state: 'failed', version: 5, title: 'Needs review',
        }, false, 9);
        expect(changed.lastSequence).toBe(9);
        expect(changed.jobs[0]).toMatchObject({
            title: 'Needs review',
            state: 'failed',
            uiExpanded: true,
            uiSelected: true,
        });
        expect(changed.previousClass).toBe('active');
        expect(changed.nextClass).toBe('attention');
        expect(changed.announcement).toMatch(/failed/i);
    });

    test('does not announce a snapshot replay as new work', () => {
        const result = reduceJobSnapshot([unfamiliarJob], { ...unfamiliarJob, state: 'succeeded', version: 5 }, true, 11);
        expect(result.jobs).toHaveLength(1);
        expect(result.announcement).toBe('');
    });

    test('uses readable text for indeterminate progress and omits a false percentage', () => {
        const job = { ...unfamiliarJob, progress: { phase: 'Scanning', completed: 0, total: null } };
        expect(progressText(job)).toBe('Scanning');
        expect(progressValue(job)).toBeNull();
        expect(progressAccessibleText(job)).toBe('Scanning; 0 processed; total unknown');
        expect(progressValue({ progress: { completed: 1, total: 4 } })).toBe(25);
    });

    test('shows stale percent progress as complete only for successful plugin-action jobs', () => {
        const staleSuccess = {
            kind: 'plugin-action',
            state: 'succeeded',
            progress: { completed: 70, total: 100, unit: 'percent', message: 'Fetching result...' },
        };
        expect(progressText(staleSuccess)).toBe('Completed');
        expect(progressValue(staleSuccess)).toBe(100);
        expect(progressAccessibleText(staleSuccess)).toBe('Completed');

        for (const state of ['running', 'failed']) {
            const job = { ...staleSuccess, state };
            expect(progressText(job)).toBe('Fetching result...');
            expect(progressValue(job)).toBe(70);
        }

        const otherKind = { ...staleSuccess, kind: 'deferred-download' };
        expect(progressText(otherKind)).toBe('Fetching result...');
        expect(progressValue(otherKind)).toBe(70);
    });
});

describe('Job Center event stream catch-up boundary', () => {
    test('announces only after catch-up completes, and resets that boundary on reconnect', () => {
        class FakeEventSource {
            listeners = new Map<string, Function>();
            constructor(public url: string) {}
            addEventListener(name: string, callback: Function) { this.listeners.set(name, callback); }
            close() {}
        }
        vi.stubGlobal('EventSource', FakeEventSource);
        const center = jobCenter();
        center.view = 'all';
        center.jobs = [{ id: 'live-job', title: 'Index rebuild', kind: 'maintenance', state: 'queued', version: 1 }];
        center._liveRegion = { announce: vi.fn(), destroy: vi.fn() } as any;
        center.scheduleStreamRefresh = vi.fn();
        center.connect();
        const stream = center.eventSource as unknown as FakeEventSource;
        const sendJob = (state: string, version: number, sequence: number) => stream.listeners.get('job')?.({
            data: JSON.stringify({ id: 'live-job', title: 'Index rebuild', kind: 'maintenance', state, version, deliverySequence: sequence }),
            lastEventId: `v2:${sequence}`,
        });

        sendJob('running', 2, 10);
        expect(center._liveRegion.announce).not.toHaveBeenCalled();
        stream.listeners.get('job-caught-up')?.({ data: JSON.stringify({ cursor: 'v2:10' }) });
        sendJob('failed', 3, 11);
        expect(center._liveRegion.announce).toHaveBeenCalledTimes(1);
        expect(center._liveRegion.announce).toHaveBeenLastCalledWith('Index rebuild failed.');

        stream.listeners.get('error')?.({});
        sendJob('succeeded', 4, 12);
        expect(center._liveRegion.announce).toHaveBeenCalledTimes(1);
        stream.listeners.get('job-caught-up')?.({ data: JSON.stringify({ cursor: 'v2:12' }) });
        sendJob('cancelled', 5, 13);
        expect(center._liveRegion.announce).toHaveBeenCalledTimes(2);
        expect(center._liveRegion.announce).toHaveBeenLastCalledWith('Index rebuild cancelled.');
    });

    test('does not announce a replay snapshot that finishes loading after the catch-up boundary', async () => {
        let resolveDetail: (value: unknown) => void = () => {};
        const detailRequest = new Promise(resolve => { resolveDetail = resolve; });
        const center = jobCenter();
        center.jobs = [{ id: 'live-job', title: 'Index rebuild', state: 'queued', version: 1 }];
        center._liveRegion = { announce: vi.fn(), destroy: vi.fn() } as any;
        center.scheduleStreamRefresh = vi.fn();
        center.fetchJSON = vi.fn(() => detailRequest);

        center.handleStreamMessage({
            data: JSON.stringify({ id: 'event-1', jobId: 'live-job', type: 'failed', sequence: 2, deliverySequence: 1 }),
            lastEventId: 'v2:1',
        });
        center.markStreamCaughtUp({ data: JSON.stringify({ cursor: 'v2:1' }) });
        resolveDetail({ id: 'live-job', title: 'Index rebuild', state: 'failed', version: 2 });
        await Promise.resolve();
        await Promise.resolve();

        expect(center._liveRegion.announce).not.toHaveBeenCalled();
    });
});

describe('Job Center live summary', () => {
    test('refreshes summaries with submitted filters and ignores results from an older filter generation', async () => {
        let resolveFirstSummary: (value: unknown) => void = () => {};
        const firstSummary = new Promise(resolve => { resolveFirstSummary = resolve; });
        vi.stubGlobal('FormData', class {
            private values: Record<string, string>;
            constructor(form: { search: string }) { this.values = { search: form.search }; }
            get(key: string) { return this.values[key] || null; }
        });
        const center = jobCenter();
        center.fetchJSON = vi.fn(async raw => {
            const url = new URL(String(raw), 'http://localhost');
            if (url.pathname === '/v1/jobs/summary') {
                if (url.searchParams.get('search') === 'older filter') return firstSummary;
                return { byState: { queued: 1 } };
            }
            if (url.pathname === '/v1/jobs') {
                return { jobs: [{ id: url.searchParams.get('search'), state: 'queued' }] };
            }
            return {};
        });
        const submit = (search: string) => {
            return center.submitFilters({ search } as HTMLFormElement);
        };

        let olderSubmissionSettled = false;
        const olderSubmission = submit('older filter').then(() => { olderSubmissionSettled = true; });
        const newerSubmission = submit('newer filter');
        await newerSubmission;

        expect(center.jobs.map(job => job.id)).toEqual(['newer filter']);
        expect(center.summary).toEqual({ byState: { queued: 1 } });
        expect(olderSubmissionSettled).toBe(false);
        expect(center.fetchJSON.mock.calls.map(([raw]) => String(raw))).toEqual([
            expect.stringContaining('/v1/jobs/summary?search=older+filter&dismissed=false'),
            expect.stringContaining('/v1/jobs?search=older+filter&dismissed=false&limit=50'),
            expect.stringContaining('/v1/jobs/summary?search=newer+filter&dismissed=false'),
            expect.stringContaining('/v1/jobs?search=newer+filter&dismissed=false&limit=50'),
        ]);

        resolveFirstSummary({ byState: { failed: 1 } });
        await olderSubmission;
        expect(center.summary).toEqual({ byState: { queued: 1 } });
    });

    test('clears foreground loading when a stream refresh invalidates the submitted list request', async () => {
        vi.useFakeTimers();
        let resolveList: (value: unknown) => void = () => {};
        const pendingList = new Promise(resolve => { resolveList = resolve; });
        vi.stubGlobal('FormData', class {
            private values: Record<string, string>;
            constructor(form: { search: string }) { this.values = { search: form.search }; }
            get(key: string) { return this.values[key] || null; }
        });
        const center = jobCenter();
        center.fetchJSON = vi.fn(raw => {
            const url = new URL(String(raw), 'http://localhost');
            if (url.pathname === '/v1/jobs/summary') return Promise.resolve({ byState: {} });
            return pendingList;
        });

        const submission = center.submitFilters({ search: 'current filter' } as HTMLFormElement);
        expect(center.loading).toBe(true);
        center.scheduleStreamRefresh();
        resolveList({ jobs: [{ id: 'current filter', state: 'queued' }] });
        await submission;

        expect(center.loading).toBe(false);
        center.destroy();
        vi.useRealTimers();
    });

    test('uses current filters for initial and live summary refreshes', async () => {
        vi.useFakeTimers();
        const center = jobCenter();
        center.view = 'all';
        center.filters = {
            ...center.filters,
            search: 'Index rebuild',
            command: 'retry',
            kinds: ['remote-download'],
            origins: ['api'],
            ownerId: '12',
            actorId: '13',
            acceptedAfter: '2026-09-01T00:00:00.000Z',
            acceptedBefore: '2026-09-20T00:00:00.000Z',
            relationship: 'retry-of',
            pinned: true,
            dismissed: null,
        };
        center.streamCaughtUp = true;
        center.jobs = [{ id: 'live-job', title: 'Index rebuild', state: 'running', version: 1 }];
        center.summary = { byState: { running: 1, failed: 0 } };
        const summaryURLs: URL[] = [];
        let currentState = 'running';
        center.fetchJSON = vi.fn(async raw => {
            const url = new URL(String(raw), 'http://localhost');
            if (url.pathname === '/v1/jobs/summary') {
                summaryURLs.push(url);
                return { byState: { running: 0, failed: 1 } };
            }
            if (url.pathname === '/v1/jobs') return { jobs: [{ id: 'live-job', title: 'Index rebuild', state: currentState, version: 2 }] };
            return {};
        });

        await center.load();
        currentState = 'failed';
        const initialSummaryURL = summaryURLs[0];
        expect(initialSummaryURL).toBeDefined();
        expect(initialSummaryURL.searchParams.get('search')).toBe('Index rebuild');
        expect(initialSummaryURL.searchParams.get('dismissed')).toBe('false');

        center.handleStreamMessage({
            data: JSON.stringify({ id: 'live-job', title: 'Index rebuild', state: 'failed', version: 2, deliverySequence: 1 }),
            lastEventId: 'v2:1',
        });
        await vi.advanceTimersByTimeAsync(250);

        expect(center.sections.attention.map(job => job.id)).toEqual(['live-job']);
        expect(center.summary.byState).toEqual({ running: 0, failed: 1 });
        expect(summaryURLs).toHaveLength(2);
        expect(summaryURLs[1].search).toBe(initialSummaryURL.search);
        vi.useRealTimers();
    });

    test('coalesces off-screen home events into one ordered refresh', async () => {
        vi.useFakeTimers();
        const center = jobCenter();
        center.view = 'home';
        center.fetchJSON = vi.fn(async url => {
            if (new URL(String(url), 'http://localhost').pathname === '/v1/jobs/summary') return { byState: {} };
            if (String(url).startsWith('/v1/jobs/')) return { id: 'off-screen', state: 'queued', version: 1 };
            return { jobs: [] };
        });
        const sendEvent = (sequence: number, id: string) => center.handleStreamMessage({
            data: JSON.stringify({
                id: `event-${sequence}`, jobId: id, sequence, jobVersion: 1,
                type: 'queued', deliverySequence: sequence, createdAt: '2026-09-23T12:00:00Z',
            }),
            lastEventId: `v2:${sequence}`,
        });

        sendEvent(1, 'first-new-job');
        sendEvent(2, 'second-new-job');
        await Promise.resolve();
        expect(center.fetchJSON.mock.calls.filter(([url]) => new URL(String(url), 'http://localhost').pathname === '/v1/jobs/summary')).toHaveLength(0);

        await vi.advanceTimersByTimeAsync(250);
        expect(center.fetchJSON.mock.calls.filter(([url]) => new URL(String(url), 'http://localhost').pathname === '/v1/jobs/summary')).toHaveLength(1);
        expect(center.fetchJSON.mock.calls.filter(([url]) => String(url).startsWith('/v1/jobs?'))).toHaveLength(3);
        vi.useRealTimers();
    });

    test('discards an in-flight summary that predates a later stream event', async () => {
        vi.useFakeTimers();
        let resolveFirstSummary: (value: unknown) => void = () => {};
        const firstSummary = new Promise(resolve => { resolveFirstSummary = resolve; });
        let summaryCalls = 0;
        let activeRequests = 0;
        let maximumConcurrentRequests = 0;
        const center = jobCenter();
        center.view = 'all';
        center.streamCaughtUp = true;
        center.jobs = [{ id: 'live-job', state: 'running', version: 1 }];
        center.summary = { byState: { running: 1 } };
        center.fetchJSON = vi.fn(async url => {
            if (new URL(String(url), 'http://localhost').pathname !== '/v1/jobs/summary') return {};
            summaryCalls += 1;
            activeRequests += 1;
            maximumConcurrentRequests = Math.max(maximumConcurrentRequests, activeRequests);
            if (summaryCalls === 1) {
                return firstSummary.finally(() => { activeRequests -= 1; });
            }
            activeRequests -= 1;
            return { byState: { succeeded: 1 } };
        });
        const sendState = (state: string, version: number, sequence: number) => center.handleStreamMessage({
            data: JSON.stringify({ id: 'live-job', state, version, deliverySequence: sequence }),
            lastEventId: `v2:${sequence}`,
        });

        sendState('failed', 2, 1);
        await vi.advanceTimersByTimeAsync(150);
        sendState('succeeded', 3, 2);
        expect(summaryCalls).toBe(1);
        resolveFirstSummary({ byState: { failed: 1 } });
        await Promise.resolve();
        await Promise.resolve();
        expect(center.summary).toEqual({ byState: { running: 1 } });

        await vi.advanceTimersByTimeAsync(150);
        await Promise.resolve();
        expect(summaryCalls).toBe(2);
        expect(maximumConcurrentRequests).toBe(1);
        expect(center.summary).toEqual({ byState: { succeeded: 1 } });
        vi.useRealTimers();
    });
});

describe('Job Center templates', () => {
    const detailTemplate = readFileSync(fileURLToPath(new URL('../../templates/displayJob.tpl', import.meta.url)), 'utf8');
    const listTemplate = readFileSync(fileURLToPath(new URL('../../templates/listJobs.tpl', import.meta.url)), 'utf8');

    test('renders commands and outputs only from the advertised detail arrays', () => {
        expect(detailTemplate).toContain('x-for="command in commandsFor(detail)"');
        expect(detailTemplate).toContain('role="group" aria-label="Advertised job commands"');
        expect(detailTemplate).toContain('x-for="output in advertisedOutputs(detail)"');
        expect(detailTemplate).toContain(':href="outputLinkURL(output, advertisedOutputs(detail))"');
        expect(detailTemplate).not.toMatch(/detail\.(?:kind|source)\s*===/);
        expect(detailTemplate).not.toMatch(/command\.(?:kind|source)\s*===/);
    });

    test('opens historical entity redirects directly and labels typed entity outputs clearly', () => {
        const historical = {
            key: 'result', type: 'summary', destinationUrl: '/resource?id=1',
            url: '/v1/jobs/old-job/outputs?key=result', availability: 'available',
        };
        const entity = {
            key: 'entity', type: 'entity', label: 'Resource',
            url: '/v1/jobs/new-job/outputs?key=entity', availability: 'available',
        };
        const summaryURL = '/v1/jobs/old-job/outputs?key=result';
        expect(outputLinkURL(historical, [historical])).toBe('/resource?id=1');
        expect(outputJSONLinkURL(historical, [historical])).toBe(summaryURL);
        expect(outputLinkLabel(historical, [historical])).toBe('View result');
        expect(outputLinkAccessibleLabel(historical, [historical])).toBe('View result');
        expect(outputLinkURL(entity)).toBe('/v1/jobs/new-job/outputs?key=entity');
        expect(outputLinkLabel(entity)).toBe('View resource');
        expect(outputLinkAccessibleLabel(entity)).toBe('View resource');

        const both = [historical, entity];
        expect(outputLinkURL(historical, both)).toBe(summaryURL);
        expect(outputJSONLinkURL(historical, both)).toBe('');
        expect(outputLinkLabel(historical, both)).toBe('View JSON result');
        expect(outputLinkAccessibleLabel(historical, both)).toBe('View JSON result');
        expect(outputLinkURL(historical, [historical, { ...entity, key: 'another', label: 'Group' }])).toBe('/resource?id=1');
        expect(outputLinkURL(historical, [historical, { ...entity, availability: 'removed' }])).toBe('/resource?id=1');
        expect(detailTemplate).toContain(':href="outputLinkURL(output, advertisedOutputs(detail))"');
        expect(detailTemplate).toContain('outputLinkURL(output, advertisedOutputs(detail))');
        expect(detailTemplate).toContain('outputJSONLinkURL(output, advertisedOutputs(detail))');
        expect(detailTemplate).toContain('outputLinkAccessibleLabel(output, advertisedOutputs(detail))');
        expect(detailTemplate).toContain('x-text="outputLinkLabel(output, advertisedOutputs(detail))"');
    });

    test('shows a visible, viewer-specific pin marker in list and detail views', () => {
        expect(listTemplate).toContain('x-show="job.pinned"');
        expect(listTemplate).toContain('Pinned by you');
        expect(detailTemplate).toContain('x-show="detail.pinned"');
        expect(detailTemplate).toContain('Pinned by you');
    });

    test('shows safe ownership, origin, warnings, output expiry, and log links from detail DTOs', () => {
        expect(detailTemplate).toContain('detail.ownerUserId');
        expect(detailTemplate).toContain('detail.actorUserId');
        expect(detailTemplate).toContain('detail.origin');
        expect(detailTemplate).toContain('detail.summary');
        expect(detailTemplate).toContain('warningEvents()');
        expect(detailTemplate).toContain('output.expiresAt');
        expect(detailTemplate).toContain('x-text="outputLinkLabel(output, advertisedOutputs(detail))"');
        expect(outputLinkLabel({ type: 'log' })).toBe('Open log');
        expect(outputLinkAccessibleLabel({ type: 'log', label: 'stderr' })).toBe('Open log stderr');
        expect(detailTemplate).not.toMatch(/diagnosticRef|resultPath|rawPath/);
    });

    test('shows only warning timeline events in the warning summary', () => {
        const warning = { id: 'warning-1', type: 'warning', detail: { message: 'Output was shortened' } };
        const truncated = { id: 'truncated-1', type: 'events-truncated', detail: { beforeSequence: 4 } };
        expect(warningEvents([
            { id: 'state-1', type: 'state-change' },
            warning,
            truncated,
        ])).toEqual([warning, truncated]);
    });

    test('sends the advertised-command filter to the canonical list API', () => {
        expect(JOB_COMMAND_FILTER_ENABLED).toBe(true);
        expect(jobCenter().commandFilterEnabled).toBe(true);
        expect(listTemplate).toContain('name="command"');
        expect(listTemplate).toContain(':disabled="!commandFilterEnabled"');
        expect(buildJobListURL({ filters: { command: 'retry' } })).toContain('command=retry');
    });

    test('findings 41 and 113: paused and indeterminate progress stay visible and named', () => {
        expect(listTemplate).toContain('x-text="stateLabel(job)"');
        expect(listTemplate).toContain('<template x-if="job.progress">');
        expect(listTemplate).toContain('role="progressbar"');
        expect(listTemplate).toContain(':aria-valuetext="progressAccessibleText(job)"');
        expect(listTemplate).toContain("(job.title || job.kind || 'Job') + ' progress: '");
        expect(listTemplate).toContain(':aria-valuenow="progressValue(job)"');
    });

    test('provides the required default sections and newest-first paginated All jobs view', () => {
        const center = jobCenter();
        center.sections = { attention: [], active: [], finished: [], other: [] };
        expect(center.displaySections.map(section => section.title)).toEqual([
            'Needs attention', 'Active and scheduled', 'Recent finished',
        ]);
        center.view = 'all';
        expect(center.displaySections.map(section => section.title)).toEqual(['All jobs']);
        expect(listTemplate).toContain('x-for="section in displaySections"');
        expect(listTemplate).toContain('Load more jobs');
    });

    test('keeps job row DOM keyed by identity so focused controls survive membership refreshes', () => {
        expect(listTemplate).toContain('<template x-for="job in section.jobs" :key="job.id">');
    });

    test('does not put replayed timeline events in a live announcement region', () => {
        const timeline = detailTemplate.split('data-testid="job-timeline"')[1]?.split('</section>')[0] || '';
        expect(timeline).not.toContain('aria-live');
    });

    test('renders the shared Job panel after cutover', () => {
        const baseTemplate = readFileSync(fileURLToPath(new URL('../../templates/layouts/base.tpl', import.meta.url)), 'utf8');
        expect(baseTemplate).toContain('{% include "/partials/jobPanel.tpl" %}');
        expect(baseTemplate).not.toContain('downloadCockpit.tpl');
    });
});
