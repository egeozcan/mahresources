import { afterEach, describe, expect, test, vi } from 'vitest';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import {
    advertisedCommands,
    advertisedOutputs,
    buildJobListURL,
    classifyJobState,
    commandEndpoint,
    dateTimeLocalValue,
    dateTimeQueryValue,
    jobCenter,
    outputEndpoint,
    parseJobCenterURL,
    progressText,
    progressValue,
    reduceJobStreamEvent,
    reduceJobSnapshot,
    selectedBulkCommands,
    serializeJobCenterURL,
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

    test('the state text classifies a job without using its Kind or source', () => {
        expect(classifyJobState(unfamiliarJob)).toBe('active');
        expect(classifyJobState({ ...unfamiliarJob, state: 'blocked' })).toBe('attention');
        expect(classifyJobState({ ...unfamiliarJob, state: 'succeeded' })).toBe('finished');
    });
});

describe('Job Center URL state', () => {
    test('round-trips each filter, multi-value dimension, view and keyset cursor', () => {
        const state = {
            view: 'all',
            filters: {
                search: 'staged archive',
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

    test('converts RFC3339 URL timestamps for local controls and datetime-local values for the API', () => {
        const timestamp = '2026-09-23T12:30:00.000Z';
        const local = dateTimeLocalValue(timestamp);

        expect(new Date(local).toISOString()).toBe(timestamp);
        expect(dateTimeQueryValue(local)).toBe(timestamp);
    });
});

describe('canonical event reducer', () => {
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
        expect(progressValue({ progress: { completed: 1, total: 4 } })).toBe(25);
    });
});

describe('Job Center templates', () => {
    const detailTemplate = readFileSync(fileURLToPath(new URL('../../templates/displayJob.tpl', import.meta.url)), 'utf8');
    const listTemplate = readFileSync(fileURLToPath(new URL('../../templates/listJobs.tpl', import.meta.url)), 'utf8');

    test('renders commands and outputs only from the advertised detail arrays', () => {
        expect(detailTemplate).toContain('x-for="command in advertisedCommands(detail)"');
        expect(detailTemplate).toContain('x-for="output in advertisedOutputs(detail)"');
        expect(detailTemplate).toContain(':href="outputEndpoint(output)"');
        expect(detailTemplate).not.toMatch(/detail\.(?:kind|source)\s*===/);
        expect(detailTemplate).not.toMatch(/command\.(?:kind|source)\s*===/);
    });

    test('keeps state text and indeterminate progress readable without color', () => {
        expect(listTemplate).toContain('x-text="stateLabel(job)"');
        expect(listTemplate).toContain('role="progressbar"');
        expect(listTemplate).toContain(':aria-valuetext="progressText(job)"');
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

    test('does not put replayed timeline events in a live announcement region', () => {
        const timeline = detailTemplate.split('data-testid="job-timeline"')[1]?.split('</section>')[0] || '';
        expect(timeline).not.toContain('aria-live');
    });

    test('keeps the list/detail routes behind the explicit cutover context gate', () => {
        const baseTemplate = readFileSync(fileURLToPath(new URL('../../templates/layouts/base.tpl', import.meta.url)), 'utf8');
        expect(baseTemplate).toContain('{% if jobCenterCutoverEnabled %}');
        expect(baseTemplate).toContain('{% include "/partials/jobPanel.tpl" %}');
        expect(baseTemplate).toContain('{% include "/partials/downloadCockpit.tpl" %}');
    });
});
