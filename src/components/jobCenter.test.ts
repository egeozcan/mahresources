import { afterEach, describe, expect, test, vi } from 'vitest';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import {
    advertisedCommands,
    advertisedOutputs,
    classifyJobState,
    commandEndpoint,
    commandLocation,
    failureOutput,
    failureText,
    jobCenter,
    jobCommands,
    outputEndpoint,
    outputLinkAccessibleLabel,
    outputLinkLabel,
    outputJSONLinkURL,
    outputLinkURL,
    progressAccessibleText,
    progressIndeterminate,
    progressText,
    progressValue,
    reduceJobStreamEvent,
    reduceJobSnapshot,
    resultOutput,
    resultURL,
    selectedBulkCommands,
    phaseText,
    stateLabel,
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

describe('stateLabel', () => {
    test('names a succeeded job its Kind left unfinished', () => {
        expect(stateLabel({ state: 'succeeded', phase: 'partial' })).toBe('Partially completed');
        expect(stateLabel({ state: 'succeeded' })).toBe('Succeeded');
        // The phase only means "unfinished" once the job has succeeded.
        expect(stateLabel({ state: 'running', phase: 'partial' })).toBe('Running');
    });

    test('a partial success reads as partial in its progress, and its phase is not repeated', () => {
        expect(progressText({ state: 'succeeded', phase: 'partial' })).toBe('Partially completed');
        expect(progressText({ state: 'succeeded' })).toBe('Completed');
        const finishedShare = { state: 'succeeded', phase: 'partial', progress: { completed: 3, total: 3, message: 'Did 3 of 9 shares.' } };
        expect(progressText(finishedShare)).toBe('Partially completed: Did 3 of 9 shares.');
        expect(progressValue(finishedShare)).toBe(100);
        expect(progressAccessibleText(finishedShare)).toBe('Partially completed: Did 3 of 9 shares.');
        expect(phaseText({ state: 'succeeded', phase: 'partial' })).toBe('');
        expect(phaseText({ state: 'running', phase: 'partial' })).toBe('partial');
        expect(phaseText({ state: 'running', phase: 'downloading' })).toBe('downloading');
    });
});

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

describe('command locations', () => {
    const inspect = { key: 'inspect', label: 'Inspect command history', jobVersion: 3 };

    function inspectingCenter(detail: any) {
        const assign = vi.fn();
        vi.stubGlobal('location', { origin: 'http://localhost', assign });
        const center = jobCenter();
        center._liveRegion = { announce: vi.fn() } as any;
        center.fetchJSON = vi.fn(async () => ({
            result: { status: 'succeeded', code: 'applied', message: 'Opening the command history.', detail },
        })) as any;
        return { center, assign };
    }

    test('a command whose outcome names a page on this site opens it', async () => {
        const { center, assign } = inspectingCenter({ runId: 'abc', location: '/admin/plugin-command-runs?id=abc' });

        await center.runCommand({ id: 'job-command', version: 3 } as any, inspect);

        expect(assign).toHaveBeenCalledWith('/admin/plugin-command-runs?id=abc');
        expect(center._liveRegion.announce).toHaveBeenCalledWith('Opening the command history.');
    });

    test('a location that leaves this site, or is not a path, is never followed', async () => {
        for (const location of ['https://elsewhere.example/x', '//elsewhere.example/x', 'javascript:alert(1)', 'admin', '/x#fragment', 42]) {
            const { center, assign } = inspectingCenter({ location });
            await center.runCommand({ id: 'job-command', version: 3 } as any, inspect);
            expect(assign).not.toHaveBeenCalled();
        }
        expect(commandLocation({ detail: 'not an object' })).toBe('');
        expect(commandLocation(null)).toBe('');
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

    test('shows stale percent progress as complete for any successful job', () => {
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
        expect(progressText(otherKind)).toBe('Completed');
        expect(progressValue(otherKind)).toBe(100);
    });

    test('a finished download whose size was never known is complete, not in progress', () => {
        // A transfer with no Content-Length (or an HLS stream) publishes no total,
        // and the last progress row it wrote is the one the finished Job keeps.
        const download = { kind: 'remote-download', state: 'succeeded', progress: {} };
        expect(progressText(download)).toBe('Completed');
        expect(progressValue(download)).toBe(100);
        expect(progressIndeterminate(download)).toBe(false);

        const running = { ...download, state: 'running' };
        expect(progressValue(running)).toBeNull();
        expect(progressIndeterminate(running)).toBe(true);

        // A job that stopped without finishing keeps its honest "unknown", but it
        // is not still working, so nothing may animate as though it were.
        for (const state of ['failed', 'cancelled', 'interrupted', 'blocked']) {
            const stopped = { ...download, state };
            expect(progressValue(stopped)).toBeNull();
            expect(progressIndeterminate(stopped)).toBe(false);
        }
        expect(progressValue({ ...download, state: 'succeeded', progress: { completed: 136, total: 136, unit: 'bytes' } })).toBe(100);
        expect(progressText({ ...download, state: 'succeeded', progress: { completed: 136, total: 136, unit: 'bytes' } })).toBe('136 / 136 bytes');
    });

    test('a succeeded job links to the entity it created, whatever its kind', () => {
        const entity = {
            key: 'resource', type: 'entity', label: 'Created resource', availability: 'available',
            url: '/v1/jobs/dl-1/outputs?key=resource',
        };
        const download = { id: 'dl-1', kind: 'remote-download', state: 'succeeded', outputs: [entity] };
        expect(resultOutput(download)).toBe(entity);
        expect(resultURL(download)).toBe(entity.url);
        expect(outputLinkLabel(entity, download.outputs)).toBe('View created resource');

        expect(resultOutput({ ...download, state: 'running' })).toBeNull();
        expect(resultOutput({ ...download, outputs: [{ ...entity, availability: 'removed' }] })).toBeNull();
        expect(resultOutput({ ...download, outputs: [{ ...entity, url: 'https://example.com/x' }] })).toBeNull();
        expect(resultOutput({ ...download, outputs: [] })).toBeNull();
        expect(resultURL({ ...download, outputs: [] })).toBe('');

        // A summary destination is plugin-action vocabulary; no other kind records one.
        const summary = { key: 'result', type: 'summary', availability: 'available', destinationUrl: '/resource?id=1', url: '/v1/jobs/dl-1/outputs?key=result' };
        expect(resultOutput({ ...download, outputs: [summary] })).toBeNull();
        expect(resultOutput({ ...download, kind: 'plugin-action', outputs: [summary] })).toBe(summary);
    });
});

describe('failure output', () => {
    test('a failed job links to the entity its failure is about, and nothing else does', () => {
        const existing = {
            key: 'existing-resource', type: 'entity', label: 'Existing resource', availability: 'available',
            url: '/v1/jobs/dl-2/outputs?key=existing-resource',
        };
        const duplicate = {
            id: 'dl-2', kind: 'remote-download', state: 'failed', outputs: [existing],
            failure: { code: 'resource-exists', class: 'conflict', message: 'a resource with identical content already exists (#1)' },
        };
        expect(failureOutput(duplicate)).toBe(existing);
        expect(outputLinkLabel(existing, duplicate.outputs)).toBe('View existing resource');
        // It is not what the job made, so it is never offered as the job's result.
        expect(resultOutput(duplicate)).toBeNull();

        expect(failureOutput({ ...duplicate, state: 'succeeded' })).toBeNull();
        expect(failureOutput({ ...duplicate, outputs: [{ ...existing, availability: 'removed' }] })).toBeNull();
        expect(failureOutput({ ...duplicate, outputs: [{ ...existing, url: 'https://example.com/x' }] })).toBeNull();
        expect(failureOutput({ ...duplicate, outputs: [] })).toBeNull();
        // Only the output the failure names: an entity a failed job published for
        // some other reason is not what its failure is about.
        const made = { ...existing, key: 'resource', label: 'Created resource', url: '/v1/jobs/dl-2/outputs?key=resource' };
        expect(failureOutput({ ...duplicate, outputs: [made] })).toBeNull();
        expect(failureOutput({ ...duplicate, outputs: [made, existing] })).toBe(existing);
    });

    test('a link left by an earlier attempt belongs to that failure only', () => {
        // An output cannot be withdrawn, so a reconciled replay of the same Job keeps
        // it. Only the failure that published it shows it, and it is never a result.
        const existing = {
            key: 'existing-resource', type: 'entity', label: 'Existing resource', availability: 'available',
            url: '/v1/jobs/dl-3/outputs?key=existing-resource',
        };
        const made = { ...existing, key: 'resource', label: 'Created resource', url: '/v1/jobs/dl-3/outputs?key=resource' };
        const replayed = { id: 'dl-3', kind: 'remote-download', state: 'failed', outputs: [existing] };
        expect(failureOutput({ ...replayed, failure: { code: 'download-failed', class: 'internal', message: 'HTTP 404 Not Found' } })).toBeNull();
        expect(failureOutput({ ...replayed, failure: { code: 'resource-exists', class: 'conflict', message: 'already exists (#1)' } })).toBe(existing);

        const succeeded = { ...replayed, state: 'succeeded', outputs: [existing, made] };
        expect(resultOutput(succeeded)).toBe(made);
        expect(resultOutput({ ...succeeded, outputs: [existing] })).toBeNull();
    });

    test('the detail page renders it inside the failure section', () => {
        const detailTemplate = readFileSync(fileURLToPath(new URL('../../templates/displayJob.tpl', import.meta.url)), 'utf8');
        const failure = detailTemplate.split('id="job-failure-heading"')[1]?.split('</section>')[0] || '';
        expect(failure).toContain('x-if="failureOutput(detail)"');
        expect(failure).toContain(':href="outputLinkURL(failureOutput(detail), advertisedOutputs(detail))"');
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
        center.jobs = [{ id: 'live-job', title: 'Index rebuild', kind: 'maintenance', state: 'queued', version: 1 }];
        center._liveRegion = { announce: vi.fn(), destroy: vi.fn() } as any;
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

    test('a stream that reset its cursor reloads the page rather than repairing it', () => {
        const reload = vi.fn();
        vi.stubGlobal('location', { reload });
        const center = jobCenter();
        const close = vi.fn();
        center.eventSource = { close } as any;
        center.lastSequence = 5000;
        center.load = vi.fn();

        center.markStreamCaughtUp({ data: JSON.stringify({ cursor: 'v2:875', reset: true }) });

        expect(close).toHaveBeenCalledTimes(1);
        expect(reload).toHaveBeenCalledTimes(1);
        expect(center.lastSequence).toBe(5000);
        expect(center.load).not.toHaveBeenCalled();
        vi.unstubAllGlobals();
    });

    test('does not announce a replay snapshot that finishes loading after the catch-up boundary', async () => {
        let resolveDetail: (value: unknown) => void = () => {};
        const detailRequest = new Promise(resolve => { resolveDetail = resolve; });
        const center = jobCenter();
        center.jobs = [{ id: 'live-job', title: 'Index rebuild', state: 'queued', version: 1 }];
        center._liveRegion = { announce: vi.fn(), destroy: vi.fn() } as any;
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

describe('Job detail stream connection', () => {
    class ClosingEventSource {
        static made: ClosingEventSource[] = [];
        listeners = new Map<string, Function>();
        readyState = 1;
        constructor(public url: string) { ClosingEventSource.made.push(this); }
        addEventListener(name: string, callback: Function) { this.listeners.set(name, callback); }
        close() { this.readyState = 2; }
    }

    test('starts at the head and reads its Job again once caught up', async () => {
        ClosingEventSource.made = [];
        vi.stubGlobal('EventSource', ClosingEventSource);
        const center = jobCenter({ detailId: 'job-3' });
        center._liveRegion = { announce: vi.fn(), destroy: vi.fn() } as any;
        // The page has read its Job.
        center.jobs = [{ id: 'job-3', title: 'Export', state: 'running', version: 2 }];
        center.detail = center.jobs[0];
        center.loading = false;
        center.fetchJSON = vi.fn(async () => ({ id: 'job-3', title: 'Export', state: 'succeeded', version: 3 }));
        center.connect();
        expect(ClosingEventSource.made[0].url).toBe('/v1/jobs/events?version=2&start=head');

        ClosingEventSource.made[0].listeners.get('job-caught-up')?.({ data: JSON.stringify({ cursor: 'v2:30' }) });
        await vi.waitFor(() => expect(center.detail.state).toBe('succeeded'));
        expect(center.fetchJSON).toHaveBeenCalledTimes(1);
        expect(center._liveRegion.announce).not.toHaveBeenCalled();

        // A later catch-up, after a reconnect that replayed nothing, reads nothing.
        ClosingEventSource.made[0].listeners.get('job-caught-up')?.({ data: JSON.stringify({ cursor: 'v2:30' }) });
        expect(center.fetchJSON).toHaveBeenCalledTimes(1);
    });

    test('a catch-up while the page is still reading its Job reads it again once that read finishes', async () => {
        ClosingEventSource.made = [];
        vi.stubGlobal('EventSource', ClosingEventSource);
        const center = jobCenter({ detailId: 'job-4' });
        center._liveRegion = { announce: vi.fn(), destroy: vi.fn() } as any;
        let version = 1;
        let releaseFirst = () => {};
        const firstHeld = new Promise<void>(resolve => { releaseFirst = resolve; });
        const reads: number[] = [];
        center.fetchJSON = vi.fn(async (url: string) => {
            if (url.includes('/events')) return { events: [] };
            const answer = { id: 'job-4', title: 'Export', state: version === 1 ? 'running' : 'succeeded', version };
            reads.push(version);
            if (reads.length === 1) await firstHeld;
            return answer;
        });
        center.connect();
        const loading = center.load();
        await vi.waitFor(() => expect(reads).toEqual([1]));
        // The Job moves on, and the stream catches up at a head past it,
        // while the page's first read is still on its way.
        version = 2;
        ClosingEventSource.made[0].listeners.get('job-caught-up')?.({ data: JSON.stringify({ cursor: 'v2:40' }) });
        releaseFirst();
        await loading;
        await vi.waitFor(() => expect(center.detail.state).toBe('succeeded'));
        expect(reads).toEqual([1, 2]);
    });

    test('a reconciling read answered after a newer live update replaces nothing', async () => {
        ClosingEventSource.made = [];
        vi.stubGlobal('EventSource', ClosingEventSource);
        const center = jobCenter({ detailId: 'job-6' });
        center._liveRegion = { announce: vi.fn(), destroy: vi.fn() } as any;
        center.jobs = [{ id: 'job-6', title: 'Export', state: 'running', version: 2, commands: [{ key: 'cancel', jobVersion: 2 }] }];
        center.detail = center.jobs[0];
        center.loading = false;
        let releaseRead = () => {};
        const readHeld = new Promise<void>(resolve => { releaseRead = resolve; });
        center.fetchJSON = vi.fn(async () => {
            await readHeld;
            return { id: 'job-6', title: 'Export', state: 'running', version: 2, commands: [{ key: 'cancel', jobVersion: 2 }] };
        });
        center.connect();
        ClosingEventSource.made[0].listeners.get('job-caught-up')?.({ data: JSON.stringify({ cursor: 'v2:30' }) });
        // A live update to v3 lands while the reconciling read is on its way.
        center.applyStreamSnapshot({ id: 'job-6', title: 'Export', state: 'succeeded', version: 3, commands: [] });
        releaseRead();
        await vi.waitFor(() => expect(center.fetchJSON).toHaveBeenCalled());
        await Promise.resolve();
        await Promise.resolve();
        expect(center.detail.version).toBe(3);
        expect(center.detail.commands).toEqual([]);
        expect(center.jobs[0].version).toBe(3);
    });

    test('a stream message about the Job while its timeline is read is applied', async () => {
        const center = jobCenter({ detailId: 'job-7' });
        center._liveRegion = { announce: vi.fn(), destroy: vi.fn() } as any;
        center.streamCaughtUp = true;
        let releaseTimeline = () => {};
        const timelineHeld = new Promise<void>(resolve => { releaseTimeline = resolve; });
        let timelineAsked = () => {};
        const timelineStarted = new Promise<void>(resolve => { timelineAsked = resolve; });
        let failed = false;
        center.fetchJSON = vi.fn(async (url: string) => {
            if (url.includes('/events')) {
                timelineAsked();
                await timelineHeld;
                return { events: [] };
            }
            return failed
                ? { id: 'job-7', title: 'Export', state: 'failed', version: 3 }
                : { id: 'job-7', title: 'Export', state: 'running', version: 2 };
        });
        const loading = center.load();
        await timelineStarted;
        // The Job fails while its timeline is read; the stream says so without a snapshot.
        failed = true;
        center.handleStreamMessage({
            data: JSON.stringify({ id: 'e-7', jobId: 'job-7', jobVersion: 3, type: 'failed', deliverySequence: 5 }),
            lastEventId: 'v2:5',
        });
        releaseTimeline();
        await loading;
        await vi.waitFor(() => expect(center.detail.state).toBe('failed'));
    });

    test('a pin read begun before another page unpinned the Job is read again, not applied', async () => {
        const center = jobCenter({ detailId: 'job-8' });
        center._liveRegion = { announce: vi.fn(), destroy: vi.fn() } as any;
        center.jobs = [{ id: 'job-8', state: 'failed', version: 2, pinned: true }];
        center.detail = center.jobs[0];
        center.loading = false;
        let pinned = true;
        let releaseFirst = () => {};
        const firstHeld = new Promise<void>(resolve => { releaseFirst = resolve; });
        let reads = 0;
        center.fetchJSON = vi.fn(async () => {
            reads += 1;
            const answer = { id: 'job-8', state: 'failed', version: 2, pinned };
            if (reads === 1) await firstHeld;
            return answer;
        });
        const reading = center.refreshJobPreference('job-8');
        // Another page unpins it while this page's read is on its way.
        pinned = false;
        center.hearPreferenceChange({ command: 'unpin', jobIds: ['job-8'] });
        releaseFirst();
        await reading;
        await vi.waitFor(() => expect(center.detail.pinned).toBe(false));
    });

    test('a Job read an event started, answered after a pin changed elsewhere, applies the change but not the old pin', async () => {
        const center = jobCenter({ detailId: 'job-11' });
        center._liveRegion = { announce: vi.fn(), destroy: vi.fn() } as any;
        center.streamCaughtUp = true;
        center.jobs = [{ id: 'job-11', title: 'Export', state: 'running', version: 2, pinned: true }];
        center.detail = center.jobs[0];
        center.loading = false;
        let releaseFirst = () => {};
        const firstHeld = new Promise<void>(resolve => { releaseFirst = resolve; });
        let reads = 0;
        center.fetchJSON = vi.fn(async () => {
            reads += 1;
            if (reads === 1) {
                await firstHeld;
                return { id: 'job-11', title: 'Export', state: 'failed', version: 3, pinned: true };
            }
            return { id: 'job-11', title: 'Export', state: 'failed', version: 3, pinned: false };
        });
        center.handleStreamMessage({
            data: JSON.stringify({ id: 'e-11', jobId: 'job-11', jobVersion: 3, type: 'failed', deliverySequence: 21 }),
            lastEventId: 'v2:21',
        });
        center.detail = { ...center.detail, pinned: false };
        center.hearPreferenceChange({ command: 'unpin', jobIds: ['job-11'] });
        releaseFirst();
        await vi.waitFor(() => expect(center.detail.state).toBe('failed'));
        await vi.waitFor(() => expect(reads).toBeGreaterThanOrEqual(2));
        await Promise.resolve();
        expect(center.detail.pinned).toBe(false);
    });

    test('a reconciling read that fails is tried again', async () => {
        vi.useFakeTimers();
        const center = jobCenter({ detailId: 'job-9' });
        center.jobs = [{ id: 'job-9', state: 'running', version: 2 }];
        center.detail = center.jobs[0];
        let attempts = 0;
        center.fetchJSON = vi.fn(async () => {
            attempts += 1;
            if (attempts === 1) throw new Error('Request failed (503)');
            return { id: 'job-9', state: 'succeeded', version: 3 };
        });
        center.reconcileDetail();
        await vi.advanceTimersByTimeAsync(0);
        expect(center.detail.state).toBe('running');
        await vi.advanceTimersByTimeAsync(2000);
        expect(attempts).toBe(2);
        expect(center.detail.state).toBe('succeeded');
        center.destroy();
        vi.useRealTimers();
    });

    test('after a failed first read, the read that succeeds is reconciled with what the stream reported meanwhile', async () => {
        ClosingEventSource.made = [];
        vi.stubGlobal('EventSource', ClosingEventSource);
        const center = jobCenter({ detailId: 'job-10' });
        center._liveRegion = { announce: vi.fn(), destroy: vi.fn() } as any;
        let state = 'running';
        let version = 2;
        let failing = true;
        center.fetchJSON = vi.fn(async (url: string) => {
            if (url.includes('/events')) return { events: [] };
            if (failing) throw new Error('job request failed');
            return { id: 'job-10', state, version };
        });
        center.connect();
        await center.load();
        expect(center.error).toBe('job request failed');
        // Caught up while the page has no Job to reconcile.
        ClosingEventSource.made[0].listeners.get('job-caught-up')?.({ data: JSON.stringify({ cursor: 'v2:12' }) });
        failing = false;
        const retrying = center.load();
        // The Job changes while Try again is reading it; the stream names it.
        state = 'failed';
        version = 3;
        ClosingEventSource.made[0].listeners.get('job')?.({
            data: JSON.stringify({ id: 'e-10', jobId: 'job-10', jobVersion: 3, type: 'failed', deliverySequence: 13 }),
            lastEventId: 'v2:13',
        });
        await retrying;
        await vi.waitFor(() => expect(center.detail.state).toBe('failed'));
    });

    test('a stream that caught up at v2:0 is reopened from v2:0', () => {
        vi.useFakeTimers();
        ClosingEventSource.made = [];
        vi.stubGlobal('EventSource', ClosingEventSource);
        const center = jobCenter();
        center.loading = false;
        center.connect();
        const first = ClosingEventSource.made[0];
        first.listeners.get('job-caught-up')?.({ data: JSON.stringify({ cursor: 'v2:0' }) });
        first.readyState = 2;
        first.listeners.get('error')?.({});
        vi.advanceTimersByTime(1000);
        expect(new URL(ClosingEventSource.made[1].url, 'http://localhost').searchParams.get('cursor')).toBe('v2:0');
        center.destroy();
        vi.useRealTimers();
    });

    test('a stream the browser gave up on is opened again from its cursor, after a growing delay', () => {
        vi.useFakeTimers();
        ClosingEventSource.made = [];
        vi.stubGlobal('EventSource', ClosingEventSource);
        const center = jobCenter();
        center.lastSequence = 12;
        center.connect();
        const first = ClosingEventSource.made[0];
        first.readyState = 2;
        first.listeners.get('error')?.({});
        expect(center.eventSource).toBe(null);
        expect(center.connectionStatus).toBe('reconnecting');
        vi.advanceTimersByTime(1000);
        expect(ClosingEventSource.made).toHaveLength(2);
        expect(new URL(ClosingEventSource.made[1].url, 'http://localhost').searchParams.get('cursor')).toBe('v2:12');
        ClosingEventSource.made[1].readyState = 2;
        ClosingEventSource.made[1].listeners.get('error')?.({});
        vi.advanceTimersByTime(1500);
        expect(ClosingEventSource.made).toHaveLength(2);
        vi.advanceTimersByTime(500);
        expect(ClosingEventSource.made).toHaveLength(3);
        center.destroy();
        vi.useRealTimers();
    });
});

describe('Job detail live progress', () => {
    test('a frame for this Job updates its figures and graph without an announcement', () => {
        class FakeEventSource {
            listeners = new Map<string, Function>();
            constructor(public url: string) {}
            addEventListener(name: string, callback: Function) { this.listeners.set(name, callback); }
            close() {}
        }
        vi.stubGlobal('EventSource', FakeEventSource);
        const center = jobCenter();
        center._liveRegion = { announce: vi.fn(), destroy: vi.fn() } as any;
        center.detail = { id: 'job-9', state: 'running', version: 2,
            progress: { completed: 1, total: 4, unit: 'items', series: { unit: 'items', points: [{ t: 0, c: 1 }] } } };
        center.jobs = [center.detail];
        center.connect();
        const stream = center.eventSource as unknown as FakeEventSource;
        stream.listeners.get('job-progress')?.({ data: JSON.stringify({
            jobId: 'job-9', version: 2, progress: { completed: 3, total: 4, unit: 'items', rate: 2 },
            point: { t: 1000, c: 3, r: 2 },
        }) });
        stream.listeners.get('job-progress')?.({ data: JSON.stringify({ jobId: 'another', version: 1, progress: { completed: 99 } }) });

        expect(center.detail.progress.completed).toBe(3);
        expect(center.jobs[0].progress.completed).toBe(3);
        expect(center.statsText(center.detail)).toBe('3 of 4 items · 2/s');
        expect(center.graphsFor(center.detail).map((series: any) => series.key)).toEqual([':speed']);
        expect(center.sparkline(center.graphsFor(center.detail)[0])).toMatch(/^M/);
        expect(center._liveRegion.announce).not.toHaveBeenCalled();
    });
});

describe('Job Center templates', () => {
    const detailTemplate = readFileSync(fileURLToPath(new URL('../../templates/displayJob.tpl', import.meta.url)), 'utf8');

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

    test('shows a visible, viewer-specific pin marker in the detail view', () => {
        expect(detailTemplate).toContain('x-show="detail.pinned"');
        expect(detailTemplate).toContain('Pinned by you');
    });

    test('shows safe ownership, origin, warnings, output expiry, and log links from detail DTOs', () => {
        expect(detailTemplate).toContain("accountText(detail, 'owner')");
        expect(detailTemplate).toContain("accountText(detail, 'actor')");
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

describe('failure reason', () => {
    test('a failed job says why in its own words, falling back to the code', () => {
        expect(failureText({ state: 'failed', failure: { code: 'download-failed', class: 'internal', message: 'HTTP 403: 403 Forbidden' } }))
            .toBe('HTTP 403: 403 Forbidden');
        expect(failureText({ state: 'failed', failure: { code: 'runtime-unfinished', class: 'internal' } })).toBe('runtime-unfinished');
        expect(failureText({ state: 'failed', failure: { code: 'runtime-unfinished', class: 'internal', message: '   ' } })).toBe('runtime-unfinished');
        expect(failureText({ state: 'succeeded' })).toBe('');
        expect(failureText(null)).toBe('');
    });

    test('a live transition to failed announces the reason with it', () => {
        const previous = { id: 'dl', title: 'video.mp4', kind: 'download', state: 'running', version: 1 };
        const result = reduceJobStreamEvent([previous], {
            job: { ...previous, state: 'failed', version: 2, failure: { code: 'download-failed', class: 'internal', message: 'HTTP 404: 404 Not Found' } },
            deliverySequence: 5,
        }, 4);
        expect(result.announcement).toBe('video.mp4 failed: HTTP 404: 404 Not Found.');
    });
});
