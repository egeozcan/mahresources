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
    shortJobId,
    timelineEventLabel,
} from './jobCenter.js';
import { formatLocalTime } from '../utils/localTime.js';

const detailTemplate = readFileSync(fileURLToPath(new URL('../../templates/displayJob.tpl', import.meta.url)), 'utf8');

const SECOND = 1_000_000_000;

describe('the Job page title', () => {
    test('names the Job, its state and the end of its id, as the server rendered it', () => {
        expect(jobDocumentTitle({ id: 'job-1', title: 'Download from example.test', state: 'failed' }, 'mahresources'))
            .toBe('Download from example.test (Failed) - Job job1 - mahresources');
        expect(jobDocumentTitle({ id: 'job-2', kind: 'group-export', state: 'running', controlIntent: 'pause' }, 'mahresources'))
            .toBe('group-export (Pausing) - Job job2 - mahresources');
        expect(jobDocumentTitle({ id: 'job-1', state: 'succeeded', phase: 'partial' }, ''))
            .toBe('job-1 (Partially completed) - Job job1');
    });

    test('tells two attempts of one download apart by the end of their ids, as jobs.ShortID does', () => {
        // The same cases as jobs/short_id_test.go.
        expect(shortJobId('01a0ddb2-7830-7abc-8def-0123456789AB')).toBe('456789ab');
        expect(shortJobId('job-1')).toBe('job1');
        expect(shortJobId('a/b')).toBe('ab');
        expect(shortJobId('')).toBe('');
        expect(shortJobId('dışa-ID-12345678')).toBe('12345678');
        const failed = { title: 'Download', state: 'failed' };
        expect(jobDocumentTitle({ ...failed, id: '01a0ddb2-7830-7abc-8def-000000000001' }, 'm'))
            .not.toBe(jobDocumentTitle({ ...failed, id: '01a0ddb2-7830-7abc-8def-000000000002' }, 'm'));
    });

    test('keeps the title current from the component and has the one h1 in the layout', () => {
        expect(detailTemplate).toContain('x-effect="syncDocumentTitle()"');
        expect(detailTemplate).toContain('data-site-title="{{ title }}"');
        expect(detailTemplate).not.toContain('<h1');
    });

    test('names the Job in the page heading once it reads the Job the server could not', () => {
        const heading = { textContent: 'Job' };
        const title = { textContent: 'Job' };
        vi.stubGlobal('document', {
            title: 'Job - mahresources',
            getElementById: (id: string) => (id === 'page-title' ? { querySelector: () => heading } : null),
        });
        try {
            const center = jobCenter({ detailId: 'job-9' }) as any;
            center._siteTitle = 'mahresources';
            center.detail = { id: 'job-9', title: 'Recovered download', state: 'running' };
            center.syncDocumentTitle();
            expect((globalThis as any).document.title).toBe('Recovered download (Running) - Job job9 - mahresources');
            expect(heading.textContent).toBe('Recovered download');
            expect(title.textContent).toBe('Job');
        } finally {
            vi.unstubAllGlobals();
        }
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
        expect(groups[0].entries[0]).toMatchObject({
            id: 'job-2', short: 'job2', relation: 'Retry of', name: 'Download', state: 'Failed',
            accepted: formatLocalTime(retried.acceptedAt, { seconds: true }),
        });
        expect(groups[1].entries[0]).toMatchObject({ id: 'job-4', relation: 'Retried as', state: 'Succeeded' });
    });

    test('lists a Job related twice once per relation', () => {
        const twice = [{ ...retried, relation: 'retry-of' }, { ...retried, relation: 'repeat-of', state: 'failed' }];
        const [group] = lineageGroups({ ...job, lineage: { ancestors: twice } });
        expect(group.entries.map(entry => entry.relation)).toEqual(['Retry of', 'Repeat of']);
        expect(new Set(group.entries.map(entry => entry.key)).size).toBe(2);
        expect(detailTemplate).toContain(':key="related.key"');
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
    function deferred<T>() {
        let resolve!: (value: T) => void;
        const promise = new Promise<T>(done => { resolve = done; });
        return { promise, resolve };
    }
    const flushMicrotasks = async () => { for (let i = 0; i < 12; i++) await Promise.resolve(); };

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

    test('reads the timeline again once the stream has caught up, for an event published between the two', async () => {
        const made: any[] = [];
        class Source {
            listeners = new Map<string, Function>();
            readyState = 1;
            constructor(public url: string) { made.push(this); }
            addEventListener(name: string, callback: Function) { this.listeners.set(name, callback); }
            close() { this.readyState = 2; }
        }
        vi.stubGlobal('EventSource', Source);
        try {
            let published = 2;
            const center = page('job-t5', url => {
                if (!url.includes('/events')) return { id: 'job-t5', state: published > 2 ? 'succeeded' : 'running', version: published };
                return { events: events(after(url) + 1, published) };
            });
            center.streamCaughtUp = false;
            center.connect();
            await center.load();
            expect(center.timeline.map(event => event.sequence)).toEqual([1, 2]);
            // The Job succeeds after the timeline was read, before the stream
            // reached its head: the stream will never deliver that event.
            published = 3;
            made[0].listeners.get('job-caught-up')?.({ data: JSON.stringify({ cursor: 'v2:50' }) });
            await vi.waitFor(() => expect(center.timeline.map(event => event.sequence)).toEqual([1, 2, 3]));
        } finally {
            vi.unstubAllGlobals();
        }
    });

    test('coalesces wake-ups during a read and continues from the event it just received', async () => {
        const firstPage = deferred<{ events: any[] }>();
        let reads = 0;
        let activeReads = 0;
        let maxActiveReads = 0;
        const cursors: number[] = [];
        const center = page('job-t7', async url => {
            if (!url.includes('/events')) return { id: 'job-t7', state: 'running', version: 1 };
            cursors.push(after(url));
            reads += 1;
            activeReads += 1;
            maxActiveReads = Math.max(maxActiveReads, activeReads);
            const result = reads === 1 ? await firstPage.promise : { events: events(2, 3) };
            activeReads -= 1;
            return result;
        });

        const loading = center.load();
        await flushMicrotasks();
        expect(reads).toBe(1);
        center.followTimeline();
        center.followTimeline();
        expect(reads).toBe(1);

        firstPage.resolve({ events: events(1, 1) });
        await loading;
        expect(center.timeline.map((event: any) => event.sequence)).toEqual([1, 2, 3]);
        expect(cursors).toEqual([0, 1]);
        expect(reads).toBe(2);
        expect(maxActiveReads).toBe(1);
    });

    test('reloads from the beginning and ignores a response from the replaced read', async () => {
        const firstPage = deferred<{ events: any[]; nextSequence?: number }>();
        let reads = 0;
        const cursors: number[] = [];
        const center = page('job-t8', url => {
            if (!url.includes('/events')) return { id: 'job-t8', state: 'running', version: 1 };
            cursors.push(after(url));
            reads += 1;
            return reads === 1 ? firstPage.promise : { events: events(1, 3) };
        });

        const firstLoad = center.load();
        await flushMicrotasks();
        expect(reads).toBe(1);
        const replacementLoad = center.load();
        await flushMicrotasks();
        expect(reads).toBe(1);

        firstPage.resolve({ events: events(1, 1), nextSequence: 1 });
        await Promise.all([firstLoad, replacementLoad]);
        expect(cursors).toEqual([0, 0]);
        expect(center.timeline.map((event: any) => event.sequence)).toEqual([1, 2, 3]);
        expect(center.timelineMore).toBe(false);
    });

    test('drops a pending event response and queued wake-up when the page is destroyed', async () => {
        const firstPage = deferred<{ events: any[]; nextSequence?: number }>();
        let reads = 0;
        const center = page('job-t9', url => {
            if (!url.includes('/events')) return { id: 'job-t9', state: 'running', version: 1 };
            reads += 1;
            return firstPage.promise;
        });

        const loading = center.load();
        await flushMicrotasks();
        expect(reads).toBe(1);
        center.followTimeline();
        center.destroy();
        firstPage.resolve({ events: events(1, 1), nextSequence: 1 });
        await loading;
        await flushMicrotasks();

        expect(center.timeline).toEqual([]);
        expect(reads).toBe(1);
    });

    test('a timeline read that failed says so and reads again when asked', async () => {
        let failing = true;
        const center = page('job-t6', url => {
            if (!url.includes('/events')) return { id: 'job-t6', state: 'succeeded', version: 3 };
            if (failing) throw new Error('Request failed (502)');
            return { events: events(after(url) + 1, 3) };
        });
        await center.load();
        expect(center.timelineError).toBe('Request failed (502)');
        failing = false;
        await center.retryTimeline();
        expect(center.timelineError).toBe('');
        expect(center.timeline.map(event => event.sequence)).toEqual([1, 2, 3]);
        expect(detailTemplate).toContain('@click="retryTimeline()"');

        // A read the stream started that fails says so too, rather than
        // leaving the timeline short without a word.
        failing = true;
        center.handleStreamMessage({ data: JSON.stringify({ id: 'e-4', jobId: 'job-t6', jobVersion: 4, sequence: 4, type: 'pinned', deliverySequence: 60 }), lastEventId: 'v2:60' });
        await vi.waitFor(() => expect(center.timelineError).toBe('Request failed (502)'));
    });

    test('names every event type in words, one added later included', () => {
        expect(timelineEventLabel({ type: 'output-published' })).toBe('Output published');
        expect(timelineEventLabel({ type: 'retried' })).toBe('Retried');
        expect(timelineEventLabel({ type: 'a_future_type' })).toBe('A future type');
        expect(timelineEventLabel({ type: '' })).toBe('Event');
        expect(timelineEventLabel(null)).toBe('Event');
        expect(detailTemplate).toContain('x-text="timelineEventLabel(event)"');
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
