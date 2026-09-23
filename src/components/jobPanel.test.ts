import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { jobPanel, panelCounts, panelCommandConfirmation } from './jobPanel.js';

afterEach(() => vi.unstubAllGlobals());

describe('Job Center panel', () => {
    test('keeps active and attention counts separate', () => {
        expect(panelCounts({ byState: { scheduled: 2, queued: 3, running: 4, paused: 1, blocked: 5, failed: 6, interrupted: 2 } }))
            .toEqual({ active: 10, attention: 13 });
    });

    test('finished dismissal replaces blanket clear and explains pin/forget scope', () => {
        expect(panelCommandConfirmation({ key: 'dismiss', label: 'Dismiss' })).toMatch(/this job/i);
        expect(panelCommandConfirmation({ key: 'pin', label: 'Pin' })).toMatch(/artifacts.*own retention/i);
        expect(panelCommandConfirmation({ key: 'forget', label: 'Forget' })).toMatch(/artifacts are not affected/i);
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

    test('opens only advertised panel commands and sends per-job outcomes', async () => {
        const fetchMock = vi.fn(async () => ({
            ok: true,
            json: async () => ({ results: [{ jobId: 'job-1', key: 'dismiss', status: 'succeeded', code: 'applied', message: 'Dismissed' }] }),
        }));
        vi.stubGlobal('fetch', fetchMock);
        const panel = jobPanel();
        panel.jobs = [{
            id: 'job-1', state: 'succeeded', version: 7,
            commands: [{ key: 'dismiss', label: 'Dismiss finished', endpoint: '/v1/jobs/job-1/commands/dismiss', jobVersion: 7, bulk: true }],
        }, { id: 'job-2', state: 'cancelled', version: 3, commands: [] }];

        const outcomes = await panel.dismissFinished();

        expect(fetchMock.mock.calls[0][0]).toBe('/v1/jobs/commands/dismiss');
        expect(outcomes).toMatchObject([
            { jobId: 'job-1', code: 'applied' },
            { jobId: 'job-2', code: 'not-advertised', status: 'failed' },
        ]);
        expect(panel.notice).toMatch(/1 of 2 finished jobs dismissed/i);
        expect(fetchMock.mock.calls.some(([url]) => String(url).includes('dismissed=false'))).toBe(true);
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

    test('announces a newly delivered lifecycle outcome in text', () => {
        const panel = jobPanel();
        panel._liveRegion = { announce: vi.fn(), destroy: vi.fn() } as any;
        panel.jobs = [{ id: 'job-1', title: 'Index rebuild', kind: 'maintenance', state: 'running', version: 2 }];
        vi.stubGlobal('fetch', vi.fn(async () => ({
            ok: true,
            json: async () => ({ id: 'job-1', title: 'Index rebuild', kind: 'maintenance', state: 'failed', version: 3 }),
        })));
        panel.markStreamCaughtUp({ data: JSON.stringify({ cursor: 'v2:14' }) });
        return panel.handleStreamMessage({
            data: JSON.stringify({ jobId: 'job-1', type: 'failed', sequence: 3, deliverySequence: 15 }),
            lastEventId: 'v2:15',
        }).then(() => {
            expect(panel._liveRegion.announce).toHaveBeenCalledWith(expect.stringMatching(/Index rebuild.*failed/i));
            expect(fetch).toHaveBeenCalledWith('/v1/jobs/job-1', expect.anything());
            expect(panel.lastSequence).toBe(15);
        });
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

    test('does not announce a replay snapshot that finishes loading after the catch-up boundary', async () => {
        let resolveDetail: (value: unknown) => void = () => {};
        const detailRequest = new Promise(resolve => { resolveDetail = resolve; });
        const panel = jobPanel();
        panel.jobs = [{ id: 'job-1', title: 'Index rebuild', state: 'queued', version: 1 }];
        panel._liveRegion = { announce: vi.fn(), destroy: vi.fn() } as any;
        panel.requestJSON = vi.fn(() => detailRequest);

        const event = panel.handleStreamMessage({
            data: JSON.stringify({ id: 'event-1', jobId: 'job-1', type: 'failed', sequence: 2, deliverySequence: 1 }),
            lastEventId: 'v2:1',
        });
        panel.markStreamCaughtUp({ data: JSON.stringify({ cursor: 'v2:1' }) });
        resolveDetail({ id: 'job-1', title: 'Index rebuild', state: 'failed', version: 2 });
        await event;

        expect(panel._liveRegion.announce).not.toHaveBeenCalled();
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
        let summaryCalls = 0;
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
            if (url === '/v1/jobs/summary') {
                summaryCalls += 1;
                return { byState: { running: summaryCalls === 1 ? 5 : 6, blocked: 5 } };
            }
            if (url.startsWith('/v1/jobs?')) {
                listRequests.push(url);
                const query = new URL(url, 'http://localhost').searchParams;
                return { jobs: query.getAll('state').flatMap(state => pageRows.get(state) || []) };
            }
            detailRequests.push(url);
            const id = decodeURIComponent(url.slice('/v1/jobs/'.length));
            return { id, kind: 'maintenance', state: 'running', version: 1, commands: [] };
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
        expect(listRequests.every(url => new URL(url, 'http://localhost').searchParams.get('limit') === '5')).toBe(true);
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
            expect(panel.jobs.length).toBeLessThanOrEqual(15);
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
        expect(listRequests.slice(3).every(url => new URL(url, 'http://localhost').searchParams.get('limit') === '5')).toBe(true);
        expect(summaryCalls).toBe(2);
        expect(panel.counts).toEqual({ active: 6, attention: 5 });
        expect(panel.jobs).toHaveLength(15);
        panel.destroy();
        vi.useRealTimers();
    });

    test('refreshes the active and attention counts after a delivered state event', async () => {
        vi.useFakeTimers();
        const panel = jobPanel();
        panel.streamCaughtUp = true;
        panel.jobs = [{ id: 'job-1', state: 'running', version: 1 }];
        panel.summary = { byState: { running: 1, failed: 0 } };
        panel.requestJSON = vi.fn(async () => ({ byState: { running: 0, failed: 1 } }));

        await panel.handleStreamMessage({
            data: JSON.stringify({ id: 'job-1', state: 'failed', version: 2, deliverySequence: 1 }),
            lastEventId: 'v2:1',
        });
        await vi.advanceTimersByTimeAsync(250);

        expect(panel.counts).toEqual({ active: 0, attention: 1 });
        expect(panel.requestJSON).toHaveBeenCalledWith('/v1/jobs/summary');
        vi.useRealTimers();
    });

    test('template traps focus, supports a narrow viewport, and names the new actions', () => {
        const template = readFileSync(fileURLToPath(new URL('../../templates/partials/jobPanel.tpl', import.meta.url)), 'utf8');
        const baseTemplate = readFileSync(fileURLToPath(new URL('../../templates/layouts/base.tpl', import.meta.url)), 'utf8');
        expect(template).toContain('role="dialog" aria-modal="true"');
        expect(template).toContain('x-trap.noscroll.noreturn="isOpen"');
        expect(template).toContain('data-testid="job-panel-overlay"');
        expect(template).toContain('aria-hidden="true" @click="close()"');
        expect(template).toContain('fixed inset-x-0 bottom-0');
        expect(template).toContain('Dismiss finished');
        expect(template).toContain('All jobs');
        expect(template).toContain('x-for="command in commandsFor(job)"');
        expect(template).not.toContain('Clear completed');
        expect(template).toContain('Active and scheduled');
        expect(template).toContain('Needs attention');
        expect(baseTemplate).toContain('{% if jobCenterCutoverEnabled %}');
        expect(baseTemplate).toContain('{% include "/partials/jobPanel.tpl" %}');
        expect(baseTemplate).toContain('{% include "/partials/downloadCockpit.tpl" %}');
    });
});
