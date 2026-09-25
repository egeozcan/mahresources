import { afterEach, describe, expect, test, vi } from 'vitest';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import {
    advertisedCommands,
    advertisedOutputs,
    classifyJobState,
    commandEndpoint,
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
        expect(center.graphsFor(center.detail).map((series: any) => series.key)).toEqual(['rate']);
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
