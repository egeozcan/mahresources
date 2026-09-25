// @vitest-environment happy-dom

import { afterEach, describe, expect, test, vi } from 'vitest';
import {
    createJobListRefresher,
    datetimeInputFromInstant,
    instantFromDatetimeInput,
    jobBulkCommands,
    jobFilterTimes,
    jobList,
    localizeJobTimes,
    keepDetailsOpen,
    stateChangeAnnouncement,
    stateChanges,
} from './jobList.js';

afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
    document.body.innerHTML = '';
});

function card(id: string, state: string, title = id) {
    return `<article data-job-id="${id}"><div data-entity='${JSON.stringify({ id, state, title })}'></div></article>`;
}

function page({ rows, quick, pagination }: { rows: string; quick: string; pagination?: string }) {
    return `<aside><div data-job-quick-filters>${quick}</div></aside>
        <main><section class="list-container">${rows}</section></main>
        <footer class="footer">${pagination ? `<nav aria-label="Pagination">${pagination}</nav>` : ''}</footer>`;
}

// A morph stand-in: replace the children, which is what the real morph converges to.
const replace = (from: Element, to: Element) => { from.innerHTML = to.innerHTML; };

describe('job list live refresh', () => {
    test('coalesces a burst of stream messages into one fetch of the current URL', async () => {
        vi.useFakeTimers();
        document.body.innerHTML = page({ rows: card('a', 'queued'), quick: 'Active (1)' });
        const fetchImpl = vi.fn(async () => ({
            ok: true,
            text: async () => page({ rows: card('a', 'running'), quick: 'Active (1)' }),
        }));
        const refresher = createJobListRefresher({ fetchImpl, currentURL: () => '/jobs?state=queued', morph: replace, debounceMs: 50 });

        refresher.request();
        refresher.request();
        refresher.request();
        await vi.advanceTimersByTimeAsync(60);

        expect(fetchImpl).toHaveBeenCalledTimes(1);
        expect(fetchImpl.mock.calls[0][0]).toBe('/jobs?state=queued');
        expect(document.querySelector('[data-entity]')?.getAttribute('data-entity')).toContain('running');
    });

    test('a steady stream of events still refreshes, at most once per interval', async () => {
        vi.useFakeTimers();
        document.body.innerHTML = page({ rows: '', quick: '' });
        const fetchImpl = vi.fn(async () => ({ ok: true, text: async () => page({ rows: '', quick: '' }) }));
        const refresher = createJobListRefresher({ fetchImpl, morph: replace, debounceMs: 50 });

        // An event every 20ms for a full second: a debounce that restarts on each
        // one would never fire.
        for (let i = 0; i < 50; i++) {
            refresher.request();
            await vi.advanceTimersByTimeAsync(20);
        }
        expect(fetchImpl.mock.calls.length).toBeGreaterThanOrEqual(15);
        expect(fetchImpl.mock.calls.length).toBeLessThanOrEqual(21);
    });

    test('a message during a request produces one trailing refresh rather than being lost', async () => {
        vi.useFakeTimers();
        document.body.innerHTML = page({ rows: '', quick: '' });
        let release!: () => void;
        const first = new Promise<void>(resolve => { release = resolve; });
        let calls = 0;
        const fetchImpl = vi.fn(async () => {
            calls += 1;
            if (calls === 1) await first;
            return { ok: true, text: async () => page({ rows: card(`row-${calls}`, 'queued'), quick: '' }) };
        });
        const refresher = createJobListRefresher({ fetchImpl, morph: replace, debounceMs: 10 });

        refresher.request();
        await vi.advanceTimersByTimeAsync(15);
        refresher.request();
        await vi.advanceTimersByTimeAsync(15);
        expect(fetchImpl).toHaveBeenCalledTimes(1);
        release();
        await vi.advanceTimersByTimeAsync(30);

        expect(fetchImpl).toHaveBeenCalledTimes(2);
        expect(document.querySelector('[data-job-id="row-2"]')).not.toBeNull();
    });

    test('morphs the quick-filter counts and adds or removes the pagination nav', async () => {
        vi.useFakeTimers();
        document.body.innerHTML = page({ rows: card('a', 'queued'), quick: 'Needs attention (0)' });
        let html = page({ rows: card('a', 'failed'), quick: 'Needs attention (1)', pagination: '<a>Next page</a>' });
        const fetchImpl = vi.fn(async () => ({ ok: true, text: async () => html }));
        const refresher = createJobListRefresher({ fetchImpl, morph: replace, debounceMs: 1 });

        refresher.request();
        await vi.advanceTimersByTimeAsync(5);
        expect(document.querySelector('[data-job-quick-filters]')?.textContent).toBe('Needs attention (1)');
        expect(document.querySelector('footer nav[aria-label="Pagination"]')?.textContent).toBe('Next page');

        html = page({ rows: card('a', 'failed'), quick: 'Needs attention (1)' });
        refresher.request();
        await vi.advanceTimersByTimeAsync(5);
        expect(document.querySelector('footer nav[aria-label="Pagination"]')).toBeNull();
    });

    test('a failed refetch tries again rather than leaving the page stale', async () => {
        vi.useFakeTimers();
        document.body.innerHTML = page({ rows: card('a', 'running'), quick: '' });
        let calls = 0;
        const fetchImpl = vi.fn(async () => {
            calls += 1;
            if (calls === 1) return { ok: false, status: 500, text: async () => '' };
            return { ok: true, text: async () => page({ rows: card('a', 'failed'), quick: '' }) };
        });
        const refresher = createJobListRefresher({
            fetchImpl, morph: replace, debounceMs: 1, retryMs: 20, logger: { error: vi.fn() } as any,
        });

        refresher.request();
        await vi.advanceTimersByTimeAsync(5);
        expect(fetchImpl).toHaveBeenCalledTimes(1);
        await vi.advanceTimersByTimeAsync(30);
        expect(fetchImpl).toHaveBeenCalledTimes(2);
        expect(document.querySelector('[data-entity]')?.getAttribute('data-entity')).toContain('failed');
    });

    test('card times are shown in the reader\'s zone, after load and after every refresh', async () => {
        vi.useFakeTimers();
        const instant = '2026-09-01T13:05:09Z';
        const cardWithTime = `<article data-job-id="a"><time data-local-time datetime="${instant}">server text</time>`
            + `<time data-local-time="seconds" datetime="${instant}">server text</time></article>`;
        document.body.innerHTML = page({ rows: cardWithTime, quick: '' });
        localizeJobTimes(document);
        const expected = new Date(instant);
        const pad = (n: number) => String(n).padStart(2, '0');
        const minute = `${expected.getFullYear()}-${pad(expected.getMonth() + 1)}-${pad(expected.getDate())} ${pad(expected.getHours())}:${pad(expected.getMinutes())}`;
        const [short, long] = document.querySelectorAll('time');
        expect(short.textContent).toBe(minute);
        expect(long.textContent).toBe(`${minute}:${pad(expected.getSeconds())}`);

        const fetchImpl = vi.fn(async () => ({ ok: true, text: async () => page({ rows: cardWithTime, quick: '' }) }));
        const refresher = createJobListRefresher({ fetchImpl, morph: replace, debounceMs: 1 });
        refresher.request();
        await vi.advanceTimersByTimeAsync(5);
        expect(document.querySelector('time')?.textContent).toBe(minute);
    });

    test('a refetch redirected to the login page is not taken for the list', async () => {
        vi.useFakeTimers();
        document.body.innerHTML = page({ rows: card('a', 'running'), quick: 'Active (1)', pagination: '<a>Next page</a>' });
        const fetchImpl = vi.fn(async () => ({ ok: true, redirected: true, url: 'http://localhost/login', text: async () => '<form>Sign in</form>' }));
        const onUnavailable = vi.fn();
        const refresher = createJobListRefresher({
            fetchImpl, morph: replace, debounceMs: 1, retryMs: 10, onUnavailable, logger: { error: vi.fn() } as any,
        });

        refresher.request();
        await vi.advanceTimersByTimeAsync(50);
        expect(document.querySelector('[data-job-quick-filters]')?.textContent).toBe('Active (1)');
        expect(document.querySelector('footer nav[aria-label="Pagination"]')).not.toBeNull();
        expect(onUnavailable).toHaveBeenCalledTimes(1);
        // A redirect is not transient: the page stops asking rather than polling the login page.
        expect(fetchImpl).toHaveBeenCalledTimes(1);
    });

    test('a page without a job list is a failed refresh, not an empty one', async () => {
        vi.useFakeTimers();
        document.body.innerHTML = page({ rows: card('a', 'running'), quick: 'Active (1)' });
        let calls = 0;
        const fetchImpl = vi.fn(async () => {
            calls += 1;
            return { ok: true, text: async () => calls === 1 ? '<p>maintenance</p>' : page({ rows: card('a', 'failed'), quick: 'Active (0)' }) };
        });
        const refresher = createJobListRefresher({ fetchImpl, morph: replace, debounceMs: 1, retryMs: 10, logger: { error: vi.fn() } as any });

        refresher.request();
        await vi.advanceTimersByTimeAsync(5);
        expect(document.querySelector('[data-job-quick-filters]')?.textContent).toBe('Active (1)');
        await vi.advanceTimersByTimeAsync(20);
        expect(document.querySelector('[data-job-quick-filters]')?.textContent).toBe('Active (0)');
    });

    test('reports the state changes of rows present before and after, not membership churn', async () => {
        vi.useFakeTimers();
        document.body.innerHTML = page({ rows: card('a', 'running', 'Import') + card('gone', 'queued'), quick: '' });
        const fetchImpl = vi.fn(async () => ({
            ok: true,
            text: async () => page({ rows: card('a', 'failed', 'Import') + card('new', 'queued'), quick: '' }),
        }));
        const onRowChanges = vi.fn();
        const refresher = createJobListRefresher({ fetchImpl, morph: replace, onRowChanges, debounceMs: 1 });

        refresher.request();
        await vi.advanceTimersByTimeAsync(5);

        expect(onRowChanges).toHaveBeenCalledTimes(1);
        expect(onRowChanges.mock.calls[0][0].map((job: { id: string }) => job.id)).toEqual(['a']);
        expect(stateChangeAnnouncement(onRowChanges.mock.calls[0][0])).toBe('Import failed.');
    });

    test('summarizes a large burst of state changes instead of reading each', () => {
        const before = new Map([1, 2, 3, 4].map(n => [`j${n}`, { id: `j${n}`, state: 'queued' }]));
        const after = new Map([1, 2, 3, 4].map(n => [`j${n}`, { id: `j${n}`, state: 'running' }]));
        expect(stateChangeAnnouncement(stateChanges(before, after))).toBe('4 jobs changed state.');
    });

    test('announces a partial success as partially completed', () => {
        const before = new Map([['p', { id: 'p', title: 'Sweep', state: 'running' }]]);
        const after = new Map([['p', { id: 'p', title: 'Sweep', state: 'succeeded', phase: 'partial' }]]);
        expect(stateChangeAnnouncement(stateChanges(before, after))).toBe('Sweep partially completed.');
    });

    test('keeps a details element the reader opened open across the morph', () => {
        const from = document.createElement('details');
        from.open = true;
        const to = document.createElement('details');
        keepDetailsOpen().updating(from, to);
        expect(to.hasAttribute('open')).toBe(true);

        const closed = document.createElement('details');
        const target = document.createElement('details');
        keepDetailsOpen().updating(closed, target);
        expect(target.hasAttribute('open')).toBe(false);
    });
});

describe('job bulk commands', () => {
    function selection(ids: string[], versions: Record<string, number> = {}) {
        return {
            selectedIds: new Set(ids),
            options: Object.fromEntries(ids.map(id => [id, { entity: { id, version: versions[id] ?? 1 } }])),
            announce: vi.fn(),
        };
    }

    const detail = (id: string, commands: object[], version = 1) => ({ job: { id, version, pinned: false, commands } });

    test('offers the bulk commands every selected job advertises, reading each detail once per version', async () => {
        const dismiss = { key: 'dismiss', label: 'Dismiss', bulk: true };
        const retry = { key: 'retry', label: 'Retry', bulk: true };
        const details: Record<string, object> = {
            a: detail('a', [dismiss, retry]),
            b: detail('b', [dismiss]),
        };
        const fetchImpl = vi.fn(async (url: string) => ({
            ok: true,
            json: async () => details[decodeURIComponent(url.split('/').pop()!)],
        }));
        const component = Object.assign(jobBulkCommands({ fetchImpl }), { $selection: selection(['a', 'b']) });

        await component.sync();
        expect(component.commands().map((command: { key: string }) => command.key)).toEqual(['dismiss']);
        await component.sync();
        expect(fetchImpl).toHaveBeenCalledTimes(2);

        component.$selection = selection(['a', 'b'], { a: 2 });
        details.a = detail('a', [dismiss], 2);
        await component.sync();
        expect(fetchImpl).toHaveBeenCalledTimes(3);
    });

    test('posts the command for the selection and keeps each per-job outcome', async () => {
        const results = [
            { jobId: 'a', key: 'dismiss', status: 'succeeded', code: 'applied' },
            { jobId: 'b', key: 'dismiss', status: 'failed', code: 'conflict', message: 'Changed' },
        ];
        const fetchImpl = vi.fn(async () => ({ ok: true, json: async () => ({ results }) }));
        const refresh = vi.fn();
        window.addEventListener('job-list-refresh', refresh);
        // The bar can hide under the reader once the refresh removes every selected
        // card, so the summary is also shown on the page itself.
        const notices: string[] = [];
        const onNotice = (event: Event) => notices.push((event as CustomEvent).detail.message);
        window.addEventListener('job-list-notice', onNotice);
        const component = Object.assign(jobBulkCommands({ fetchImpl }), { $selection: selection(['a', 'b']) });

        await component.run({ key: 'dismiss', label: 'Dismiss', bulk: true });

        expect(fetchImpl.mock.calls[0][0]).toBe('/v1/jobs/commands/dismiss');
        const body = JSON.parse(fetchImpl.mock.calls[0][1].body);
        expect(body.jobIds).toEqual(['a', 'b']);
        expect(body.idempotencyKey).toEqual(expect.any(String));
        expect(component.outcomes).toEqual(results);
        expect(component.$selection.announce).toHaveBeenCalledWith('1 of 2 jobs: dismiss.');
        expect(notices).toEqual(['1 of 2 jobs: dismiss.']);
        expect(refresh).toHaveBeenCalledTimes(1);
        window.removeEventListener('job-list-refresh', refresh);
        window.removeEventListener('job-list-notice', onNotice);
    });

    test('a pin changes no Job version, so the row pin state invalidates the cached offer', async () => {
        const pin = { key: 'pin', label: 'Pin', bulk: true };
        const unpin = { key: 'unpin', label: 'Unpin', bulk: true };
        let pinned = false;
        const fetchImpl = vi.fn(async () => ({
            ok: true, json: async () => ({ job: { id: 'a', version: 1, pinned, commands: [pin, unpin] } }),
        }));
        const component = Object.assign(jobBulkCommands({ fetchImpl }), { $selection: selection(['a']) });
        await component.sync();
        expect(component.commands().map((command: { key: string }) => command.key)).toEqual(['pin']);

        pinned = true;
        component.$selection.options.a.entity.pinned = true;
        expect(component.commands().map((command: { key: string }) => command.key)).toEqual(['unpin']);
        await component.sync();
        expect(fetchImpl).toHaveBeenCalledTimes(2);
    });

    test('a newer selection with nothing to read does not leave the bar loading', async () => {
        let release!: (value: unknown) => void;
        const pending = new Promise(resolve => { release = resolve; });
        const fetchImpl = vi.fn(async (url: string) => {
            if (url.endsWith('/b')) await pending;
            return { ok: true, json: async () => detail(url.split('/').pop()!, []) };
        });
        const component = Object.assign(jobBulkCommands({ fetchImpl }), { $selection: selection(['a']) });
        await component.sync();

        component.$selection = selection(['a', 'b']);
        const slow = component.sync();
        expect(component.loading).toBe(true);
        component.$selection = selection(['a']);
        await component.sync();
        expect(component.loading).toBe(false);
        release(undefined);
        await slow;
        expect(component.loading).toBe(false);
    });

    test('outcomes survive the refresh that follows the command', async () => {
        const results = [{ jobId: 'b', key: 'dismiss', status: 'failed', code: 'conflict', message: 'Changed' }];
        const fetchImpl = vi.fn(async (url: string) => url.includes('/commands/')
            ? { ok: true, json: async () => ({ results }) }
            : { ok: true, json: async () => detail(url.split('/').pop()!, [], 2) });
        const component = Object.assign(jobBulkCommands({ fetchImpl }), { $selection: selection(['a', 'b']) });

        await component.run({ key: 'dismiss', label: 'Dismiss', bulk: true });
        // The refresh removed the dismissed card and moved the other's version.
        component.$selection = selection(['b'], { b: 2 });
        await component.sync();
        expect(component.outcomes).toEqual(results);
    });

    test('a failed request shows its reason, not only announces it', async () => {
        const fetchImpl = vi.fn(async () => ({ ok: false, status: 403, json: async () => ({ error: 'CSRF token mismatch' }) }));
        const component = Object.assign(jobBulkCommands({ fetchImpl }), { $selection: selection(['a']) });
        // The refresh can hide the bar while the request is in flight, so a
        // failure reaches the page notice as a success does.
        const notices: string[] = [];
        const onNotice = (event: Event) => notices.push((event as CustomEvent).detail.message);
        window.addEventListener('job-list-notice', onNotice);
        await component.run({ key: 'dismiss', label: 'Dismiss', bulk: true });
        window.removeEventListener('job-list-notice', onNotice);
        expect(component.error).toBe('CSRF token mismatch');
        expect(notices).toEqual(['CSRF token mismatch']);

        const offline = Object.assign(jobBulkCommands({ fetchImpl: vi.fn(async () => { throw new Error('Failed to fetch'); }) }), { $selection: selection(['a']) });
        await offline.run({ key: 'dismiss', label: 'Dismiss', bulk: true });
        expect(offline.error).toBe('Failed to fetch');
    });

    test('offers nothing, and runs nothing, while a selected job is newer than its cached commands', async () => {
        const retry = { key: 'retry', label: 'Retry', bulk: true };
        let release!: (value: unknown) => void;
        const pending = new Promise(resolve => { release = resolve; });
        let reads = 0;
        const fetchImpl = vi.fn(async (url: string) => {
            if (url.includes('/commands/')) return { ok: true, json: async () => ({ results: [] }) };
            reads += 1;
            if (reads > 1) await pending;
            return { ok: true, json: async () => detail('a', reads > 1 ? [] : [retry], reads) };
        });
        const component = Object.assign(jobBulkCommands({ fetchImpl }), { $selection: selection(['a']) });
        await component.sync();
        expect(component.commands().map((command: { key: string }) => command.key)).toEqual(['retry']);

        // A live refresh moved the row to version 2; its retry may be gone.
        component.$selection = selection(['a'], { a: 2 });
        const reread = component.sync();
        expect(component.commands()).toEqual([]);
        await component.run(retry);
        expect(fetchImpl.mock.calls.some(([url]) => String(url).includes('/commands/'))).toBe(false);

        release(undefined);
        await reread;
        expect(component.commands()).toEqual([]);
    });

    test('a selection that changed while the confirmation was open is not acted on', async () => {
        const fetchImpl = vi.fn();
        const component = Object.assign(jobBulkCommands({ fetchImpl }), { $selection: selection(['a', 'b']) });
        const ask = vi.fn(async () => {
            // A live refresh removed b's card while the dialog was open.
            component.$selection = selection(['a']);
            return true;
        });
        vi.stubGlobal('Alpine', { store: () => ({ ask }) });

        const notices: string[] = [];
        const onNotice = (event: Event) => notices.push((event as CustomEvent).detail.message);
        window.addEventListener('job-list-notice', onNotice);

        await component.run({ key: 'forget', label: 'Forget replay input', bulk: true, destructive: true });

        expect(fetchImpl).not.toHaveBeenCalled();
        expect(component.error).toMatch(/selection changed/i);
        // The refresh may have emptied the selection and hidden the bar: the page says it too.
        expect(notices).toEqual([component.error]);
        window.removeEventListener('job-list-notice', onNotice);
    });

    test('a failed read stops being reported once that job is deselected', async () => {
        const fetchImpl = vi.fn(async (url: string) => url.endsWith('/b')
            ? { ok: false, status: 500, json: async () => ({}) }
            : { ok: true, json: async () => detail('a', []) });
        const component = Object.assign(jobBulkCommands({ fetchImpl }), { $selection: selection(['a']) });
        await component.sync();
        component.$selection = selection(['a', 'b']);
        await component.sync();
        expect(component.error).not.toBe('');

        component.$selection = selection(['a']);
        await component.sync();
        expect(component.error).toBe('');
    });

    test('asks before running a command that needs confirmation', async () => {
        const ask = vi.fn(async () => false);
        vi.stubGlobal('Alpine', { store: () => ({ ask }) });
        const fetchImpl = vi.fn();
        const component = Object.assign(jobBulkCommands({ fetchImpl }), { $selection: selection(['a']) });

        await component.run({ key: 'forget', label: 'Forget replay input', bulk: true, destructive: true });

        expect(ask).toHaveBeenCalledTimes(1);
        expect(fetchImpl).not.toHaveBeenCalled();
    });
});

describe('job list stream', () => {
    class FakeEventSource {
        listeners = new Map<string, Function>();
        constructor(public url: string) {}
        addEventListener(name: string, callback: Function) { this.listeners.set(name, callback); }
        close() {}
    }

    function connected() {
        vi.stubGlobal('EventSource', FakeEventSource);
        const list = jobList();
        list._refresher = { request: vi.fn(), destroy: vi.fn() } as any;
        list.connect();
        const stream = list.eventSource as unknown as FakeEventSource;
        const send = (name: string, data: object) => stream.listeners.get(name)?.({ data: JSON.stringify(data) });
        return { list, stream, send };
    }

    test('reconciles once at catch-up when changes arrived while it was catching up', () => {
        const { list, send } = connected();
        send('message', { jobId: 'a', deliverySequence: 4 });
        expect(list._refresher.request).not.toHaveBeenCalled();
        send('job-caught-up', { cursor: 'v2:4' });
        expect(list._refresher.request).toHaveBeenCalledTimes(1);

        send('message', { jobId: 'a', deliverySequence: 5 });
        expect(list._refresher.request).toHaveBeenCalledTimes(2);
    });

    test('a reconnect that replays what it missed reconciles the page', () => {
        const { list, stream, send } = connected();
        send('job-caught-up', { cursor: 'v2:4' });
        expect(list._refresher.request).not.toHaveBeenCalled();

        stream.listeners.get('error')?.({});
        send('message', { jobId: 'a', deliverySequence: 5 });
        send('job-caught-up', { cursor: 'v2:5' });
        expect(list._refresher.request).toHaveBeenCalledTimes(1);
    });
});

describe('job filter times', () => {
    test('an end bound shows the minute it closes and converts back to that minute\'s last instant', () => {
        const end = instantFromDatetimeInput('2026-09-01T15:00', true);
        expect(new Date(end).getTime() - new Date('2026-09-01T15:00').getTime()).toBe(59_999);
        expect(datetimeInputFromInstant(end, true)).toBe('2026-09-01T15:00');
        expect(instantFromDatetimeInput('2026-09-01T15:00')).toBe(new Date('2026-09-01T15:00').toISOString());
        expect(instantFromDatetimeInput('2026-09-01T15:00:30', true))
            .toBe(new Date(new Date('2026-09-01T15:00:30').getTime() + 999).toISOString().replace(/Z$/, '999999Z'));
    });

    test('an edited end bound reaches the last nanosecond of its unit, as the server reads one', () => {
        const end = instantFromDatetimeInput('2026-09-01T15:00', true);
        expect(end).toMatch(/\.999999999Z$/);
        expect(end.slice(0, 19)).toBe(new Date(new Date('2026-09-01T15:01').getTime() - 1).toISOString().slice(0, 19));
    });

    test('an exact instant off the minute shows to the second', () => {
        const instant = new Date('2026-09-01T15:00:07').toISOString();
        expect(datetimeInputFromInstant(instant)).toBe('2026-09-01T15:00:07');
    });

    test('submits an untouched bound exactly and an edited one in the reader\'s zone', () => {
        document.body.innerHTML = `<form>
            <input type="datetime-local" name="acceptedBefore" data-bound="end" data-instant="2026-09-01T13:00:00Z">
            <input type="datetime-local" name="acceptedAfter" data-bound="start" data-instant="2026-09-01T10:00:00Z">
        </form>`;
        const form = document.querySelector('form')!;
        const times = Object.assign(jobFilterTimes(), { $root: form });
        times.init();
        const [before, after] = form.querySelectorAll<HTMLInputElement>('input[type="datetime-local"]');
        expect(before.value).toBe(datetimeInputFromInstant('2026-09-01T13:00:00Z', true));
        after.value = '2026-09-01T09:15';

        times.submit();
        const submitted = Object.fromEntries(new FormData(form).entries());
        expect(submitted.acceptedBefore).toBe('2026-09-01T13:00:00Z');
        expect(submitted.acceptedAfter).toBe(new Date('2026-09-01T09:15').toISOString());
    });
});
