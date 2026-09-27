import { describe, expect, test, vi } from 'vitest';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import {
    jobCenter,
    jobDocumentTitle,
    jobDurationRows,
    jobTimeRows,
    lineageGroups,
    outputCountText,
    relativeTimeText,
} from './jobCenter.js';
import { formatLocalTime } from '../utils/localTime.js';

const detailTemplate = readFileSync(fileURLToPath(new URL('../../templates/displayJob.tpl', import.meta.url)), 'utf8');

const SECOND = 1_000_000_000;

describe('the Job page title', () => {
    test('names the Job and its state, as the server rendered it', () => {
        expect(jobDocumentTitle({ title: 'Download from example.test', state: 'failed' }, 'mahresources'))
            .toBe('Download from example.test (Failed) - Job - mahresources');
        expect(jobDocumentTitle({ kind: 'group-export', state: 'running', controlIntent: 'pause' }, 'mahresources'))
            .toBe('group-export (Pausing) - Job - mahresources');
        expect(jobDocumentTitle({ id: 'job-1', state: 'succeeded', phase: 'partial' }, ''))
            .toBe('job-1 (Partially completed) - Job');
    });

    test('keeps the title current from the component and has the one h1 in the layout', () => {
        expect(detailTemplate).toContain('x-effect="syncDocumentTitle()"');
        expect(detailTemplate).toContain('data-site-title="{{ title }}"');
        expect(detailTemplate).not.toContain('<h1');
    });
});

describe('the Job page times', () => {
    const now = Date.parse('2026-09-26T12:30:00Z');

    test('says when a Job was accepted, started and finished, and how long its history is kept', () => {
        const job = {
            state: 'succeeded',
            acceptedAt: '2026-09-26T12:00:00Z',
            startedAt: '2026-09-26T12:01:00Z',
            finishedAt: '2026-09-26T12:27:00Z',
            expiresAt: '2026-10-26T12:30:00Z',
        };
        const rows = jobTimeRows(job, now);
        expect(rows.map(row => row.label)).toEqual(['Accepted', 'Started', 'Finished', 'History kept until']);
        expect(rows[0]).toEqual({
            key: 'accepted', label: 'Accepted', at: job.acceptedAt,
            text: formatLocalTime(job.acceptedAt, { seconds: true }), relative: '30 min ago',
        });
        expect(rows[2].relative).toBe('3 min ago');
        expect(rows[3].relative).toBe('in 30 d');
    });

    test('lists only the instants a Job has reached, and a scheduled start', () => {
        const rows = jobTimeRows({ state: 'scheduled', acceptedAt: '2026-09-26T12:29:30Z', scheduledFor: '2026-09-26T14:30:00Z' }, now);
        expect(rows.map(row => [row.label, row.relative])).toEqual([
            ['Accepted', 'less than a minute ago'],
            ['Scheduled for', 'in 2 h'],
        ]);
    });

    test('counts the time in the state a Job is in now, as well as what it banked', () => {
        const running = {
            state: 'running',
            stateEnteredAt: '2026-09-26T12:20:00Z',
            queueDuration: 90 * SECOND,
            runningDuration: 5 * 60 * SECOND,
            pausedDuration: 0,
            blockedDuration: 0,
        };
        expect(jobDurationRows(running, now)).toEqual([
            { key: 'queue', label: 'Time queued', text: '2 min' },
            { key: 'running', label: 'Time running', text: '15 min' },
        ]);
        const finished = { ...running, state: 'succeeded' };
        expect(jobDurationRows(finished, now).map(row => row.text)).toEqual(['2 min', '5 min']);
    });

    test('writes a relative time either way round', () => {
        expect(relativeTimeText('2026-09-26T12:29:59Z', now)).toBe('less than a minute ago');
        expect(relativeTimeText('2026-09-26T12:00:00Z', now)).toBe('30 min ago');
        expect(relativeTimeText('2026-09-26T15:30:00Z', now)).toBe('in 3 h');
        expect(relativeTimeText('', now)).toBe('');
    });

    test('names the reader\'s zone and writes every time in the page one way', () => {
        expect(detailTemplate).toContain('timeZoneText');
        expect(detailTemplate).not.toContain('toLocaleString');
    });
});

describe('the Job page outputs', () => {
    test('counts outputs in words that fit the number', () => {
        expect(outputCountText(0)).toBe('');
        expect(outputCountText(1)).toBe('1 output');
        expect(outputCountText(3)).toBe('3 outputs');
        expect(detailTemplate).not.toContain('available records');
    });
});

describe('the Job page lineage', () => {
    const job = { id: 'job-3', state: 'failed' };
    const retried = { id: 'job-2', title: 'Download', state: 'failed', acceptedAt: '2026-09-26T12:00:00Z', relation: 'retry-of' };
    const retry = { id: 'job-4', title: 'Download', state: 'succeeded', acceptedAt: '2026-09-26T12:10:00Z', relation: 'retry-of' };

    test('says how each related Job is related, its state and when it was accepted', () => {
        const groups = lineageGroups({ ...job, lineage: { ancestors: [retried], successors: [retry], parents: [], children: [] } });
        expect(groups.map(group => group.heading)).toEqual(['Earlier runs', 'Later runs']);
        expect(groups[0].entries[0]).toMatchObject({ id: 'job-2', relation: 'Retry of', name: 'Download', state: 'Failed', accepted: formatLocalTime(retried.acceptedAt) });
        expect(groups[1].entries[0]).toMatchObject({ id: 'job-4', relation: 'Retried as', state: 'Succeeded' });
    });

    test('tells a Continue and a Repeat from a Retry', () => {
        const partial = { id: 'job-1', state: 'succeeded', phase: 'partial', title: 'Share', relation: 'retry-of' };
        const repeated = { id: 'job-0', state: 'succeeded', title: 'Share', relation: 'repeat-of' };
        const groups = lineageGroups({ id: 'job-5', state: 'succeeded', lineage: { ancestors: [partial, repeated] } });
        expect(groups[0].entries.map(entry => entry.relation)).toEqual(['Continuation of', 'Repeat of']);
        const successors = lineageGroups({ id: 'job-1', state: 'succeeded', phase: 'partial', lineage: { successors: [{ ...partial, id: 'job-5' }] } });
        expect(successors[0].entries[0].relation).toBe('Continued as');
        const stages = lineageGroups({ id: 'job-6', state: 'succeeded', lineage: { parents: [{ id: 'p', state: 'succeeded', relation: 'parent-child' }], children: [{ id: 'c', state: 'queued', relation: 'parent-child' }] } });
        expect(stages.map(group => [group.heading, group.entries[0].relation])).toEqual([['Part of', 'Stage of'], ['Stages', 'Stage']]);
    });
});

describe('the Job page timeline', () => {
    function page(detailId: string, answer: (url: string) => any) {
        const center = jobCenter({ detailId });
        center._liveRegion = { announce: vi.fn(), destroy: vi.fn() } as any;
        center.streamCaughtUp = true;
        center.fetchJSON = vi.fn(async (url: string) => answer(url));
        return center;
    }
    const events = (from: number, to: number) => Array.from({ length: to - from + 1 }, (_, i) => ({
        id: `e-${from + i}`, sequence: from + i, type: 'progress', createdAt: '2026-09-26T12:00:00Z',
    }));
    const after = (url: string) => Number(new URL(url, 'http://x').searchParams.get('afterSequence') || 0);

    test('reads past the first page, so the event that explains the current state is shown', async () => {
        const center = page('job-t1', url => {
            if (!url.includes('/events')) return { id: 'job-t1', state: 'blocked', version: 40 };
            const from = after(url);
            return from === 0 ? { events: events(1, 200), nextSequence: 200 } : { events: events(from + 1, 214) };
        });
        await center.load();
        expect(center.timeline.map(event => event.sequence)).toEqual(events(1, 214).map(event => event.sequence));
        expect(center.timelineMore).toBe(false);
    });

    test('stops reading at a bound, says there is more, and reads on when asked', async () => {
        const total = 2500;
        const center = page('job-t2', url => {
            if (!url.includes('/events')) return { id: 'job-t2', state: 'running', version: 9 };
            const from = after(url);
            const to = Math.min(from + 200, total);
            return { events: events(from + 1, to), nextSequence: to < total ? to : undefined };
        });
        await center.load();
        expect(center.timeline.length).toBe(1000);
        expect(center.timelineMore).toBe(true);
        await center.loadLaterEvents();
        expect(center.timeline.length).toBe(2000);
        await center.loadLaterEvents();
        expect(center.timeline.length).toBe(total);
        expect(center.timelineMore).toBe(false);
        expect(detailTemplate).toContain('@click="loadLaterEvents()"');
    });

    test('adds the events the stream reports for this Job, once each, in order', async () => {
        let published = 2;
        const center = page('job-t3', url => {
            if (!url.includes('/events')) return { id: 'job-t3', state: 'running', version: 2 };
            const from = after(url);
            return { events: events(from + 1, published) };
        });
        await center.load();
        expect(center.timeline.map(event => event.sequence)).toEqual([1, 2]);
        published = 4;
        center.handleStreamMessage({ data: JSON.stringify({ id: 'e-3', jobId: 'job-t3', jobVersion: 3, sequence: 3, type: 'output-published', deliverySequence: 30 }), lastEventId: 'v2:30' });
        center.handleStreamMessage({ data: JSON.stringify({ id: 'e-4', jobId: 'job-t3', jobVersion: 4, sequence: 4, type: 'succeeded', deliverySequence: 31 }), lastEventId: 'v2:31' });
        await vi.waitFor(() => expect(center.timeline.map(event => event.sequence)).toEqual([1, 2, 3, 4]));
        // Another Job's event reads nothing for this page's timeline.
        const reads = (center.fetchJSON as any).mock.calls.filter(([url]: [string]) => url.includes('/events')).length;
        center.handleStreamMessage({ data: JSON.stringify({ id: 'x-1', jobId: 'other', jobVersion: 1, sequence: 1, type: 'accepted', deliverySequence: 32 }), lastEventId: 'v2:32' });
        await Promise.resolve();
        expect((center.fetchJSON as any).mock.calls.filter(([url]: [string]) => url.includes('/events')).length).toBe(reads);
    });

    test('drops the phase a finished Job no longer has', async () => {
        let finished = false;
        const center = page('job-t4', url => {
            if (url.includes('/events')) return { events: [] };
            return finished
                ? { id: 'job-t4', state: 'succeeded', version: 5 }
                : { id: 'job-t4', state: 'running', phase: 'downloading segments', version: 4 };
        });
        await center.load();
        expect(center.phaseText(center.detail)).toBe('downloading segments');
        finished = true;
        center.handleStreamMessage({ data: JSON.stringify({ id: 'e-9', jobId: 'job-t4', jobVersion: 5, sequence: 9, type: 'succeeded', deliverySequence: 40 }), lastEventId: 'v2:40' });
        await vi.waitFor(() => expect(center.detail.state).toBe('succeeded'));
        expect(center.phaseText(center.detail)).toBe('');
    });
});
