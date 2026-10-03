// @vitest-environment happy-dom

import { afterEach, describe, expect, test, vi } from 'vitest';
import {
    createJobListRefresher,
    datetimeInputFromInstant,
    instantFromDatetimeInput,
    jobBulkCommands,
    bulkCommandReport,
    jobFilterTimes,
    jobList,
    jobSummary,
    localizeJobTimes,
    keepDetailsOpen,
    stateChangeAnnouncement,
    stateChanges,
} from './jobList.js';
import { followJobAnnouncements } from '../utils/jobAnnouncements.js';

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

    test('is idle only with nothing asked for, due, out or to retry', async () => {
        vi.useFakeTimers();
        document.body.innerHTML = page({ rows: '', quick: '' });
        let answer!: (response: unknown) => void;
        const fetchImpl = vi.fn(() => new Promise(resolve => { answer = resolve; }));
        const refresher = createJobListRefresher({ fetchImpl, morph: replace, debounceMs: 50, retryMs: 500, logger: { error() {} } as any });
        expect(refresher.idle()).toBe(true);
        refresher.request();
        expect(refresher.idle()).toBe(false);
        await vi.advanceTimersByTimeAsync(60);
        // Out, and a second event during it asks for a trailing refresh.
        refresher.request();
        answer({ ok: false, status: 500 });
        await vi.advanceTimersByTimeAsync(0);
        expect(refresher.idle()).toBe(false);
        await vi.advanceTimersByTimeAsync(500);
        answer({ ok: true, text: async () => page({ rows: '', quick: '' }) });
        await vi.advanceTimersByTimeAsync(0);
        expect(refresher.idle()).toBe(true);
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
        expect(onRowChanges.mock.calls[0][0].map((change: any) => [change.previous.state, change.next.id, change.next.state]))
            .toEqual([['running', 'a', 'failed']]);
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

    test('says a failure with its reason, as the drawer does', () => {
        const before = new Map([['f', { id: 'f', title: 'sunrise.png', state: 'running' }]]);
        const after = new Map([['f', { id: 'f', title: 'sunrise.png', state: 'failed', failure: { code: 'http-404', message: 'HTTP 404 Not Found' } }]]);
        expect(stateChangeAnnouncement(stateChanges(before, after))).toBe('sunrise.png failed: HTTP 404 Not Found.');
    });

    test('hands its changes to the drawer and says the ones the drawer hands back', () => {
        const heard: any[] = [];
        const stop = followJobAnnouncements({
            hear: (changes: any[]) => {
                heard.push(...changes);
                return changes.filter(change => change.next.ownerUserId !== 7);
            },
        });
        const list = jobList();
        const said: string[] = [];
        list._liveRegion = { announce: (text: string) => said.push(text), destroy() {} } as any;
        const mine = { previous: { id: 'mine', state: 'running', version: 2 }, next: { id: 'mine', title: 'mine.bin', state: 'succeeded', version: 3, ownerUserId: 7 } };
        const theirs = {
            previous: { id: 'theirs', state: 'running', version: 2 },
            next: { id: 'theirs', title: 'theirs.bin', state: 'failed', version: 3, ownerUserId: 8, failure: { message: 'HTTP 403 Forbidden' } },
        };
        list.announceChanges([mine, theirs]);
        expect(heard).toEqual([mine, theirs]);
        expect(said).toEqual(['theirs.bin failed: HTTP 403 Forbidden.']);
        stop();
        // With no drawer on the page, the list says everything itself.
        list.announceChanges([mine]);
        expect(said).toEqual(['theirs.bin failed: HTTP 403 Forbidden.', 'mine.bin succeeded.']);
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
            options: Object.fromEntries(ids.map(id => [id, { entity: { id, title: `${id}.png`, version: versions[id] ?? 1 } }])),
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
        // Each outcome names its Job as the list does, with a link, since the
        // refresh may already have taken its card away.
        expect(component.outcomes).toEqual([
            { jobId: 'a', title: 'a.png', url: '/job?id=a', text: 'Done' },
            { jobId: 'b', title: 'b.png', url: '/job?id=b', text: 'Changed' },
        ]);
        const said = 'Dismissed 1 of 2 selected jobs. Not done for b.png: Changed.';
        expect(component.$selection.announce).toHaveBeenCalledWith(said);
        expect(notices).toEqual([said]);
        expect(refresh).toHaveBeenCalledTimes(1);
        window.removeEventListener('job-list-refresh', refresh);
        window.removeEventListener('job-list-notice', onNotice);
    });

    test('a bulk result reads as what was done to how many of the selected jobs', () => {
        const titles = { a: 'a.png', b: 'b.png', c: 'c.png', d: 'd.png', e: 'e.png' };
        const applied = (jobId: string) => ({ jobId, status: 'succeeded', code: 'applied' });
        const refused = (jobId: string, message = '') => ({ jobId, status: 'failed', code: 'not-advertised', message });
        const say = (key: string, label: string, ids: string[], results: object[]) =>
            bulkCommandReport({ key, label }, ids, results, titles).message;

        expect(say('pin', 'Pin', ['a'], [applied('a')])).toBe('Pinned 1 of 1 selected job.');
        expect(say('unpin', 'Unpin', ['a', 'b'], [applied('a'), applied('b')])).toBe('Unpinned 2 of 2 selected jobs.');
        expect(say('undismiss', 'Undismiss', ['a'], [applied('a')])).toBe('Returned 1 of 1 selected job to the list.');
        expect(say('retry', 'Retry', ['a', 'b'], [applied('a'), refused('b', 'the job does not offer that command')]))
            .toBe('Retried 1 of 2 selected jobs. Not done for b.png: the job does not offer that command.');
        expect(say('cancel', 'Cancel', ['a', 'b'], [applied('a'), { jobId: 'b', status: 'succeeded', code: 'requested' }]))
            .toBe('Cancelled 1 of 2 selected jobs. Cancel requested for 1 of 2 selected jobs.');
        expect(say('pin-lineage', 'Pin visible lineage', ['a'], [applied('a')])).toBe('Pin visible lineage done for 1 of 1 selected job.');
        expect(say('dismiss', 'Dismiss', ['a', 'b', 'c', 'd', 'e'], ['a', 'b', 'c', 'd', 'e'].map(id => refused(id))))
            .toBe('Dismiss was not done for a.png, b.png, c.png and 2 more; each says why in the list of outcomes.');
        expect(bulkCommandReport({ key: 'dismiss', label: 'Dismiss' }, ['a'], [refused('a')], titles).outcomes)
            .toEqual([{ jobId: 'a', title: 'a.png', url: '/job?id=a', text: 'No longer offered' }]);
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

    test('a dismissal changes no Job version, so the row\'s dismissed state picks Dismiss or Undismiss', async () => {
        const dismiss = { key: 'dismiss', label: 'Dismiss', bulk: true };
        const undismiss = { key: 'undismiss', label: 'Undismiss', bulk: true };
        const fetchImpl = vi.fn(async (url: string) => ({
            ok: true, json: async () => ({ job: { id: decodeURIComponent(url.split('/').pop()!), version: 1, commands: [dismiss, undismiss] } }),
        }));
        const component = Object.assign(jobBulkCommands({ fetchImpl }), { $selection: selection(['a', 'b']) });
        await component.sync();
        expect(component.commands().map((command: { key: string }) => command.key)).toEqual(['dismiss']);

        component.$selection.options.a.entity.dismissed = true;
        // A mixed selection may take either; each is idempotent.
        expect(component.commands().map((command: { key: string }) => command.key)).toEqual(['dismiss', 'undismiss']);
        component.$selection.options.b.entity.dismissed = true;
        expect(component.commands().map((command: { key: string }) => command.key)).toEqual(['undismiss']);
    });

    test('a pin, a dismissal and an undismissal run without asking, and a destructive confirmation is styled as one', async () => {
        const ask = vi.fn(async () => false);
        vi.stubGlobal('Alpine', { store: () => ({ ask }) });
        const fetchImpl = vi.fn(async () => ({ ok: true, json: async () => ({ results: [] }) }));
        const component = Object.assign(jobBulkCommands({ fetchImpl }), { $selection: selection(['a']) });

        for (const key of ['pin', 'dismiss', 'undismiss']) await component.run({ key, label: key, bulk: true });
        expect(ask).not.toHaveBeenCalled();
        expect(fetchImpl).toHaveBeenCalledTimes(3);

        await component.run({ key: 'cancel', label: 'Cancel', bulk: true, destructive: true, confirmation: 'Stop these?' });
        expect(ask).toHaveBeenCalledWith('Stop these? This applies to 1 selected job.', expect.objectContaining({ title: 'Cancel', confirmLabel: 'Cancel', cancelLabel: undefined, destructive: true, fallbackFocus: expect.any(Function) }));
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
        expect(component.outcomes).toEqual([{ jobId: 'b', title: 'b.png', url: '/job?id=b', text: 'Changed' }]);
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

        await component.run({ key: 'forget', label: 'Forget saved input', bulk: true, destructive: true });

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

    test('a bulk Cancel asks in the Kind\'s own words when every selected job shares them, and plainly when they differ', async () => {
        const ask = vi.fn(async () => false);
        vi.stubGlobal('Alpine', { store: () => ({ ask }) });
        const cancel = (confirmation: string) => ({ key: 'cancel', label: 'Cancel', bulk: true, destructive: true, confirmation });
        const component = Object.assign(jobBulkCommands({ fetchImpl: vi.fn() }), { $selection: selection(['a', 'b']) });
        component.details = {
            a: { id: 'a', version: 1, commands: [cancel('Stop this download?')] },
            b: { id: 'b', version: 1, commands: [cancel('Stop this download?')] },
        };
        await component.run(cancel('Stop this download?'));
        expect(ask.mock.calls[0][0]).toBe('Stop this download? This applies to 2 selected jobs.');

        component.details.b = { id: 'b', version: 1, commands: [cancel('Stop this export?')] };
        await component.run(cancel('Stop this download?'));
        expect(ask.mock.calls[1][0]).toBe('Cancel the selected jobs? This applies to 2 selected jobs.');
        expect(ask.mock.calls[1][1]).toMatchObject({ destructive: true, confirmLabel: 'Cancel' });
    });

    test('asks before running a command that needs confirmation', async () => {
        const ask = vi.fn(async () => false);
        vi.stubGlobal('Alpine', { store: () => ({ ask }) });
        const fetchImpl = vi.fn();
        const component = Object.assign(jobBulkCommands({ fetchImpl }), { $selection: selection(['a']) });

        await component.run({ key: 'forget', label: 'Forget saved input', bulk: true, destructive: true });

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

    test('a stream that reset its cursor reloads the page rather than repairing it', () => {
        const reload = vi.fn();
        const { list, stream, send } = connected();
        vi.stubGlobal('location', { reload });
        const close = vi.fn();
        (stream as any).close = close;
        send('job-caught-up', { cursor: 'v2:875', reset: true });
        expect(close).toHaveBeenCalledTimes(1);
        expect(reload).toHaveBeenCalledTimes(1);
        expect(list._refresher.request).not.toHaveBeenCalled();
        vi.unstubAllGlobals();
    });

    test('the first catch-up reconciles the page it was rendered with, and a quiet one after that does not', () => {
        const { list, stream, send } = connected();
        expect(stream.url).toBe('/v1/jobs/events?version=2&start=head');
        send('job-caught-up', { cursor: 'v2:4' });
        expect(list._refresher.request).toHaveBeenCalledTimes(1);

        stream.listeners.get('error')?.({});
        send('job-caught-up', { cursor: 'v2:4' });
        expect(list._refresher.request).toHaveBeenCalledTimes(1);
    });

    test('a reconnect that replays what it missed reconciles the page', () => {
        const { list, stream, send } = connected();
        send('job-caught-up', { cursor: 'v2:4' });
        expect(list._refresher.request).toHaveBeenCalledTimes(1);

        stream.listeners.get('error')?.({});
        send('message', { jobId: 'a', deliverySequence: 5 });
        send('job-caught-up', { cursor: 'v2:5' });
        expect(list._refresher.request).toHaveBeenCalledTimes(2);
    });

    test('a stream the browser gave up on is opened again from its cursor, and the page reconciles', () => {
        vi.useFakeTimers();
        const { list, stream, send } = connected();
        stream.listeners.get('job')?.({ data: JSON.stringify({ jobId: 'a' }), lastEventId: 'v2:7' });
        send('job-caught-up', { cursor: 'v2:7' });
        const requests = (list._refresher.request as any).mock.calls.length;

        (stream as any).readyState = 2;
        stream.listeners.get('error')?.({});
        expect(list.eventSource).toBe(null);
        expect(list.connectionText).toBe('Reconnecting to live updates');
        vi.advanceTimersByTime(1000);
        const reopened = list.eventSource as unknown as FakeEventSource;
        expect(new URL(reopened.url, 'http://localhost').searchParams.get('cursor')).toBe('v2:7');
        reopened.listeners.get('job-caught-up')?.({ data: JSON.stringify({ cursor: 'v2:7' }) });
        expect((list._refresher.request as any).mock.calls.length).toBe(requests + 1);
        vi.useRealTimers();
    });

    test('a dismissal, pin or forget made in another tab or panel refreshes the list', () => {
        const channels: any[] = [];
        vi.stubGlobal('BroadcastChannel', class {
            onmessage: any = null;
            constructor(public name: string) { channels.push(this); }
            postMessage() {}
            close() {}
        });
        vi.stubGlobal('EventSource', FakeEventSource);
        const list = jobList();
        (list as any).$root = document.body;
        list.init();
        list._refresher = { request: vi.fn(), destroy: vi.fn() } as any;
        channels.find(channel => channel.name === 'mahresources-job-preferences').onmessage({ data: { jobIds: ['a'] } });
        expect(list._refresher.request).toHaveBeenCalledTimes(1);
        list.destroy();
    });

    test('a stream that caught up at v2:0 is reopened from v2:0', () => {
        vi.useFakeTimers();
        const { list, stream, send } = connected();
        send('job-caught-up', { cursor: 'v2:0' });
        (stream as any).readyState = 2;
        stream.listeners.get('error')?.({});
        vi.advanceTimersByTime(1000);
        const reopened = list.eventSource as unknown as FakeEventSource;
        expect(new URL(reopened.url, 'http://localhost').searchParams.get('cursor')).toBe('v2:0');
        vi.useRealTimers();
    });

    test('says when the list could not be refreshed, until a refresh succeeds', async () => {
        document.body.innerHTML = '<div class="list-container"></div>';
        const failed = vi.fn();
        const recovered = vi.fn();
        let ok = false;
        const refresher = createJobListRefresher({
            fetchImpl: vi.fn(async () => ok
                ? { ok: true, redirected: false, text: async () => '<div class="list-container"></div>' }
                : { ok: false, redirected: false, status: 500 }) as any,
            currentURL: () => 'http://localhost/jobs?dismissed=false',
            morph: () => {},
            onFailed: failed,
            onRecovered: recovered,
            logger: { error: () => {} } as any,
            debounceMs: 0,
            retryMs: 10,
        });
        refresher.request();
        await vi.waitFor(() => expect(failed).toHaveBeenCalled());
        ok = true;
        await vi.waitFor(() => expect(recovered).toHaveBeenCalled());
        refresher.destroy();

        const list = jobList();
        list.connectionStatus = 'connected';
        list.refreshFailed = true;
        expect(list.connectionText).toBe('The list could not be refreshed and may be out of date; trying again');
    });
});

describe('job summary panel', () => {
    const summary = {
        total: 120, succeeded: 90, failed: 10, terminal: 100, successRate: 0.9,
        queue: { median: 1_500_000_000, p95: 12_000_000_000 }, run: { median: 65_000_000_000, p95: 600_000_000_000 },
        failures: [{ class: 'dependency', count: 7 }, { class: 'timeout', count: 3 }],
    };

    test('reads the list\'s own filter over the chosen window and says the figures in words', async () => {
        const fetchImpl = vi.fn(async () => ({ ok: true, json: async () => summary }));
        const panel = Object.assign(jobSummary({ fetchImpl }), { query: 'state=failed&dismissed=false' });
        panel.window = '7d';
        await panel.load();
        expect(fetchImpl.mock.calls[0][0]).toBe('/v1/jobs/summary?state=failed&dismissed=false&window=7d');
        expect(panel.figures()).toEqual([
            { label: 'Jobs', value: '120' },
            { label: 'Succeeded', value: '90 of 100 finished (90%)' },
            { label: 'Failed', value: '10' },
            { label: 'Time queued', value: 'median 2 s, 95% within 12 s' },
            { label: 'Time running', value: 'median 1 min, 95% within 10 min' },
            { label: 'Failures by class', value: 'dependency 7, timeout 3' },
        ]);
    });

    test('says why a summary could not be read', async () => {
        const panel = Object.assign(jobSummary({ fetchImpl: vi.fn(async () => ({ ok: false, status: 400, json: async () => ({ error: 'window may not exceed 90 days' }) })) }), { query: '' });
        await panel.load();
        expect(panel.error).toBe('window may not exceed 90 days');
        expect(panel.summary).toBe(null);
    });

    test('applies only the answer for the window chosen last', async () => {
        const answers: Array<(value: unknown) => void> = [];
        const fetchImpl = vi.fn(() => new Promise(resolve => answers.push(resolve)));
        const panel = Object.assign(jobSummary({ fetchImpl: fetchImpl as any }), { query: '' });
        panel.window = '30d';
        const slow = panel.load();
        panel.window = '7d';
        const fast = panel.load();
        answers[1]({ ok: true, json: async () => ({ ...summary, total: 7 }) });
        await fast;
        answers[0]({ ok: true, json: async () => ({ ...summary, total: 30 }) });
        await slow;
        expect(panel.summary.total).toBe(7);
        expect(panel.loading).toBe(false);
    });

    test('queues one export for a second press while the first is on its way', async () => {
        let answer!: (value: unknown) => void;
        const fetchImpl = vi.fn(() => new Promise(resolve => { answer = resolve; }));
        const panel = Object.assign(jobSummary({ fetchImpl: fetchImpl as any }), { exportQuery: '' });
        panel.exportFrom = '2025-01-01';
        panel.exportTo = '2025-12-31';
        const first = panel.exportSummary();
        await panel.exportSummary();
        answer({ ok: true, json: async () => ({ job: { id: 'export-1', title: 'Job summary' } }) });
        await first;
        expect(fetchImpl).toHaveBeenCalledTimes(1);
    });

    test('queues an export of the list\'s filter for the chosen days, and links the Job it made', async () => {
        const fetchImpl = vi.fn(async () => ({ ok: true, json: async () => ({ job: { id: 'export-1', title: 'Job summary, 2025-01-01 to 2026-01-01' } }) }));
        const panel = Object.assign(jobSummary({ fetchImpl }), { query: 'kind=remote-download&owner=me', exportQuery: 'kind=remote-download&ownerId=9' });
        panel.exportFrom = '2025-01-01';
        panel.exportTo = '2025-12-31';
        panel.exportFormat = 'json';
        await panel.exportSummary();
        const [url, init] = fetchImpl.mock.calls[0];
        expect(url).toBe('/v1/jobs/summary/export?kind=remote-download&ownerId=9');
        expect(init.method).toBe('POST');
        expect(JSON.parse(init.body)).toEqual({
            from: new Date('2025-01-01T00:00').toISOString(), to: localDayEndForTest('2025-12-31'), format: 'json',
        });
        expect(panel.exported).toEqual({ url: '/job?id=export-1', title: 'Job summary, 2025-01-01 to 2026-01-01' });
    });

    test('shows an export\'s refusal as the server words it', async () => {
        const panel = Object.assign(jobSummary({
            fetchImpl: vi.fn(async () => ({ ok: false, status: 400, json: async () => ({ error: 'summary export range must exceed 90 days' }) })),
        }), { query: '' });
        panel.exportFrom = '2026-01-01';
        panel.exportTo = '2026-01-02';
        await panel.exportSummary();
        expect(panel.exportError).toBe('summary export range must exceed 90 days');
        expect(panel.exported).toBe(null);
    });

    test.each(['2026-03-29', '2026-10-25'])('ends the selected local day before midnight across DST: %s', async day => {
        const fetchImpl = vi.fn(async () => ({ ok: true, json: async () => ({ job: { id: 'export-1' } }) }));
        const panel = Object.assign(jobSummary({ fetchImpl }), { exportQuery: '' });
        panel.exportFrom = day;
        panel.exportTo = day;
        await panel.exportSummary();
        const { to } = JSON.parse(fetchImpl.mock.calls[0][1].body);
        expect(to).toBe(localDayEndForTest(day));
        expect(to).toMatch(/\.999999999Z$/);
        if (Intl.DateTimeFormat().resolvedOptions().timeZone === 'Europe/Berlin') {
            const start = new Date(`${day}T00:00`).getTime();
            const next = new Date(`${day}T00:00`);
            next.setDate(next.getDate() + 1);
            const expectedHours = day === '2026-03-29' ? 23 : 25;
            expect(next.getTime() - start).toBe(expectedHours * 60 * 60 * 1000);
        }
    });
});

function localDayEndForTest(day: string) {
    const nextMidnight = new Date(`${day}T00:00`);
    nextMidnight.setDate(nextMidnight.getDate() + 1);
    nextMidnight.setTime(nextMidnight.getTime() - 1);
    return nextMidnight.toISOString().replace(/\.(\d{3})Z$/, '.$1999999Z');
}

describe('job list live progress', () => {
    // A running card as the server renders it (templates/partials/job.tpl).
    function runningCard(id: string, {
        version = 3,
        updatedAt = '2026-09-28T10:00:00Z',
        completed = 91756,
        total = 2097152,
        rate = 65536,
        eta = '2026-09-28T10:00:31Z',
        etaEstimated = true,
    } = {}) {
        const percent = Math.round(completed / total * 100);
        const snapshot = JSON.stringify({ completed, total, unit: 'bytes', rate, eta, etaEstimated });
        return `<article data-job-id="${id}"><div data-entity='${JSON.stringify({ id, state: 'running', title: `${id}.bin`, version })}'>
            <div data-job-progress data-progress-updated-at="${updatedAt}" data-progress-snapshot='${snapshot}'>
                <span data-job-progress-text>${completed} / ${total} bytes</span><span data-job-progress-value>${percent}%</span>
                <div role="progressbar" data-job-progress-bar aria-valuenow="${percent}" aria-valuetext="${completed} / ${total} bytes" aria-label="${id}.bin progress: ${completed} / ${total} bytes">
                    <div class="h-2 rounded bg-amber-800" data-job-progress-fill style="width:${percent}%"></div>
                </div>
                <p data-job-stats>63.8 KB/s · about 31 s left</p>
            </div>
        </div></article>`;
    }

    function frame(id: string, completed: number, extra: object = {}) {
        return {
            jobId: id, version: 3, state: 'running',
            progress: {
                completed, total: 2097152, unit: 'bytes', rate: 65536, updatedAt: '2026-09-28T10:00:10Z',
                eta: '2026-09-28T10:00:40Z', etaEstimated: true, ...extra,
            },
        };
    }

    function listOn(html: string) {
        document.body.innerHTML = `<main><section class="list-container">${html}</section></main>`;
        const list = jobList();
        (list as any).$root = document.body;
        return list;
    }

    function refreshThroughRefresher(list: ReturnType<typeof jobList>) {
        let rows = '';
        const fetchImpl = vi.fn(async () => ({
            ok: true,
            text: async () => page({ rows, quick: '' }),
        }));
        const refresher = createJobListRefresher({
            fetchImpl,
            morph: replace,
            onRefreshed: () => list.reconcileProgressCards(),
            debounceMs: 1,
        });
        (list as any)._refresher = refresher;
        return async (nextRows: string) => {
            rows = nextRows;
            refresher.request();
            await vi.advanceTimersByTimeAsync(2);
        };
    }

    const progressOf = (id: string) => {
        const card = document.querySelector(`[data-job-id="${id}"]`)!;
        return {
            text: card.querySelector('[data-job-progress-text]')!.textContent,
            value: card.querySelector('[data-job-progress-value]')!.textContent,
            now: card.querySelector('[data-job-progress-bar]')!.getAttribute('aria-valuenow'),
            width: (card.querySelector('[data-job-progress-fill]') as HTMLElement).style.width,
            stats: card.querySelector('[data-job-stats]')!.textContent,
        };
    };

    test('a progress frame moves a running card\'s bar, amount, speed and time left', () => {
        vi.useFakeTimers({ now: new Date('2026-09-28T10:00:10Z') });
        const list = listOn(runningCard('a') + runningCard('b'));
        list.handleProgressFrame({ data: JSON.stringify(frame('a', 1048576)) });
        const moved = progressOf('a');
        // The line above the bar and the stats line follow the rules every Job
        // surface shares (progressText, jobStatsText): the amount is in the
        // stats line, formatted, with the speed and the time left.
        expect({ ...moved, stats: undefined }).toEqual({ text: 'Working', value: '50%', now: '50', width: '50%', stats: undefined });
        expect(moved.stats).toContain('1.0 MB of 2.0 MB');
        expect(moved.stats).toContain('64.0 KB/s');
        expect(moved.stats).toContain('about 30 s left');
        // A card no frame named is left as the server drew it.
        expect(progressOf('b').value).toBe('4%');
        list.destroy();
    });

    test('time left counts down between frames, and a stalled speed goes away', () => {
        vi.useFakeTimers({ now: new Date('2026-09-28T10:00:10Z') });
        const list = listOn(runningCard('a'));
        list.handleProgressFrame({ data: JSON.stringify(frame('a', 1048576)) });
        vi.advanceTimersByTime(5000);
        expect(progressOf('a').stats).toContain('64.0 KB/s');
        expect(progressOf('a').stats).toContain('about 25 s left');
        vi.advanceTimersByTime(6000);
        expect(progressOf('a').stats).not.toContain('/s');
        expect(progressOf('a').stats).not.toContain('left');
        list.destroy();
    });

    test('a frame for a card that is not on the page, or older than the card, changes nothing', () => {
        vi.useFakeTimers({ now: new Date('2026-09-28T10:00:10Z') });
        const list = listOn(runningCard('a', { version: 5 }));
        list.handleProgressFrame({ data: JSON.stringify(frame('z', 1048576)) });
        list.handleProgressFrame({ data: JSON.stringify(frame('a', 1048576)) });
        expect(progressOf('a').value).toBe('4%');
        list.destroy();
    });

    test('a frame older than the progress a refresh drew moves nothing back', () => {
        vi.useFakeTimers({ now: new Date('2026-09-28T10:00:21Z') });
        const list = listOn(runningCard('a', { updatedAt: '2026-09-28T10:00:20Z' }));
        list.handleProgressFrame({ data: JSON.stringify(frame('a', 1048576)) });
        expect(progressOf('a').value).toBe('4%');
        list.handleProgressFrame({ data: JSON.stringify(frame('a', 1572864, { updatedAt: '2026-09-28T10:00:21Z' })) });
        expect(progressOf('a').value).toBe('75%');
        // Nor does one older than a frame already drawn.
        list.handleProgressFrame({ data: JSON.stringify(frame('a', 1048576, { updatedAt: '2026-09-28T10:00:20.500Z' })) });
        expect(progressOf('a').value).toBe('75%');
        list.destroy();
    });

    test('a refresh that drew older progress than the page holds is brought forward again', () => {
        vi.useFakeTimers({ now: new Date('2026-09-28T10:00:10Z') });
        const list = listOn(runningCard('a'));
        list.handleProgressFrame({ data: JSON.stringify(frame('a', 1048576)) });
        // The refresh read the Job before that frame was reported.
        document.querySelector('section')!.innerHTML = runningCard('a', { updatedAt: '2026-09-28T10:00:05Z' });
        list.reapplyProgress();
        expect(progressOf('a').value).toBe('50%');
        // One that read it after draws what it read, and the page lets it.
        document.querySelector('section')!.innerHTML = runningCard('a', { updatedAt: '2026-09-28T10:00:11Z' });
        list.reapplyProgress();
        expect(progressOf('a').value).toBe('4%');
        list.destroy();
    });

    test('an equal-timestamp refresh keeps a frame clocking until its rate and estimate go stale', () => {
        vi.useFakeTimers({ now: new Date('2026-09-28T10:00:10Z') });
        const list = listOn(runningCard('a'));
        list.handleProgressFrame({ data: JSON.stringify(frame('a', 1048576)) });
        document.querySelector('section')!.innerHTML = runningCard('a', {
            updatedAt: '2026-09-28T10:00:10Z', completed: 1048576, rate: 65536, eta: '2026-09-28T10:00:40Z',
        });
        list.reapplyProgress();
        expect(list._progress.has('a')).toBe(true);
        expect(list._progressClock).not.toBe(null);
        vi.advanceTimersByTime(5000);
        expect(progressOf('a').stats).toContain('about 25 s left');
        vi.advanceTimersByTime(6000);
        expect(progressOf('a').stats).not.toContain('/s');
        expect(progressOf('a').stats).not.toContain('left');
        list.destroy();
    });

    test('a newer server-rendered progress snapshot replaces an older held frame', () => {
        vi.useFakeTimers({ now: new Date('2026-09-28T10:00:15Z') });
        const list = listOn(runningCard('a'));
        list.handleProgressFrame({ data: JSON.stringify(frame('a', 1048576)) });
        document.querySelector('section')!.innerHTML = runningCard('a', {
            updatedAt: '2026-09-28T10:00:12Z', completed: 1572864, rate: 1024, eta: '2026-09-28T10:00:52Z',
        });
        list.reapplyProgress();
        expect(list._progress.get('a')?.progress).toMatchObject({ unit: 'bytes', rate: 1024, eta: '2026-09-28T10:00:52Z' });
        expect(progressOf('a').value).toBe('75%');
        vi.advanceTimersByTime(5000);
        expect(progressOf('a').stats).toContain('about 32 s left');
        list.destroy();
    });

    test('a newer server-rendered Job version wins when executor timestamps disagree', () => {
        vi.useFakeTimers({ now: new Date('2026-09-28T10:00:15Z') });
        const list = listOn(runningCard('a'));
        list.handleProgressFrame({ data: JSON.stringify(frame('a', 1048576, { updatedAt: '2026-09-28T10:00:10Z' })) });
        document.querySelector('section')!.innerHTML = runningCard('a', {
            version: 4, updatedAt: '2026-09-28T10:00:09Z', completed: 786432, rate: 2048, eta: '2026-09-28T10:00:55Z',
        });
        list.reapplyProgress();
        expect(list._progress.get('a')).toMatchObject({
            version: 4, progress: { unit: 'bytes', rate: 2048, eta: '2026-09-28T10:00:55Z' },
        });
        expect(progressOf('a').value).toBe('38%');
        list.destroy();
    });

    test('a held newer server snapshot restores its bar after a later stale refresh', () => {
        vi.useFakeTimers({ now: new Date('2026-09-28T10:00:15Z') });
        const list = listOn(runningCard('a'));
        list.handleProgressFrame({ data: JSON.stringify(frame('a', 1048576, { updatedAt: '2026-09-28T10:00:10Z' })) });
        document.querySelector('section')!.innerHTML = runningCard('a', {
            updatedAt: '2026-09-28T10:00:12Z', completed: 1572864, rate: 1024, eta: '2026-09-28T10:00:52Z',
        });
        list.reapplyProgress();
        expect(progressOf('a').value).toBe('75%');
        document.querySelector('section')!.innerHTML = runningCard('a', {
            updatedAt: '2026-09-28T10:00:11Z', completed: 524288, rate: 512, eta: '2026-09-28T10:00:45Z',
        });
        list.reapplyProgress();
        expect(progressOf('a').value).toBe('75%');
        expect(progressOf('a').text).toBe('Working');
        expect(progressOf('a').stats).toContain('1.5 MB of 2.0 MB');
        list.destroy();
    });

    test('initial server-rendered progress starts a clock before any SSE frame arrives', () => {
        vi.useFakeTimers({ now: new Date('2026-09-28T10:00:10Z') });
        const list = listOn(runningCard('a', { updatedAt: '2026-09-28T10:00:10Z', eta: '2026-09-28T10:00:40Z' }));
        list.init();
        expect(list._progress.has('a')).toBe(true);
        expect(list._progressClock).not.toBe(null);
        vi.advanceTimersByTime(5000);
        expect(progressOf('a').stats).toContain('about 25 s left');
        vi.advanceTimersByTime(6000);
        expect(progressOf('a').stats).not.toContain('/s');
        expect(progressOf('a').stats).not.toContain('left');
        list.destroy();
    });

    test('the refresher seeds a newly inserted running card after its only frame arrived before insertion', async () => {
        vi.useFakeTimers({ now: new Date('2026-09-28T10:00:10Z') });
        const list = listOn('');
        list.initializeProgressClock();
        const refresh = refreshThroughRefresher(list);
        list.handleProgressFrame({ data: JSON.stringify(frame('new', 1048576)) });
        expect(list._progress.has('new')).toBe(false);

        await refresh(runningCard('new'));
        expect(list._progress.has('new')).toBe(true);
        expect(list._progressClock).not.toBe(null);
        vi.advanceTimersByTime(11000);
        expect(progressOf('new').stats).not.toContain('/s');
        expect(progressOf('new').stats).not.toContain('left');
        list.destroy();
    });

    test('the refresher seeds a queued card when it becomes running without another frame', async () => {
        vi.useFakeTimers({ now: new Date('2026-09-28T10:00:10Z') });
        const queued = `<article data-job-id="queued"><div data-entity='${JSON.stringify({ id: 'queued', state: 'queued', title: 'queued.bin', version: 3 })}'></div></article>`;
        const list = listOn(queued);
        list.initializeProgressClock();
        const refresh = refreshThroughRefresher(list);

        await refresh(runningCard('queued'));
        expect(list._progress.has('queued')).toBe(true);
        expect(list._progressClock).not.toBe(null);
        vi.advanceTimersByTime(11000);
        expect(progressOf('queued').stats).not.toContain('/s');
        list.destroy();
    });

    test('the refresher rehydrates a running card that returns after removal', async () => {
        vi.useFakeTimers({ now: new Date('2026-09-28T10:00:10Z') });
        const list = listOn(runningCard('returning'));
        list.initializeProgressClock();
        const refresh = refreshThroughRefresher(list);

        await refresh('');
        expect(list._progress.has('returning')).toBe(false);
        expect(list._progressClock).toBe(null);
        list.handleProgressFrame({ data: JSON.stringify(frame('returning', 1048576)) });
        expect(list._progress.has('returning')).toBe(false);

        await refresh(runningCard('returning'));
        expect(list._progress.has('returning')).toBe(true);
        expect(list._progressClock).not.toBe(null);
        vi.advanceTimersByTime(11000);
        expect(progressOf('returning').stats).not.toContain('/s');
        list.destroy();
    });

    test('the refresher and frame path compare full timestamps and prefer a higher frame version', async () => {
        vi.useFakeTimers({ now: new Date('2026-09-28T10:00:10Z') });
        const list = listOn(runningCard('precise', {
            updatedAt: '2026-09-28T10:00:00.0001Z', total: 1000, completed: 100,
        }));
        list.initializeProgressClock();
        const refresh = refreshThroughRefresher(list);

        await refresh(runningCard('precise', {
            updatedAt: '2026-09-28T10:00:00.0009Z', total: 1000, completed: 900,
        }));
        expect(progressOf('precise').value).toBe('90%');
        list.handleProgressFrame({ data: JSON.stringify(frame('precise', 50, {
            total: 1000, updatedAt: '2026-09-28T10:00:00.000001Z',
        })) });
        expect(progressOf('precise').value).toBe('90%');

        list.handleProgressFrame({ data: JSON.stringify({
            ...frame('precise', 950, { total: 1000, updatedAt: '2026-09-28T09:59:59.999999Z' }), version: 4,
        }) });
        expect(progressOf('precise').value).toBe('95%');
        list.handleProgressFrame({ data: JSON.stringify(frame('precise', 10, {
            total: 1000, updatedAt: '2026-09-28T10:00:00.000999Z',
        })) });
        expect(progressOf('precise').value).toBe('95%');
        list.destroy();
    });

    test('the stream delivers progress frames to the page', () => {
        vi.useFakeTimers({ now: new Date('2026-09-28T10:00:10Z') });
        class FakeEventSource {
            listeners = new Map<string, Function>();
            constructor(public url: string) {}
            addEventListener(name: string, callback: Function) { this.listeners.set(name, callback); }
            close() {}
        }
        vi.stubGlobal('EventSource', FakeEventSource);
        const list = listOn(runningCard('a'));
        list._refresher = { request: vi.fn(), destroy: vi.fn() } as any;
        list.connect();
        const stream = list.eventSource as unknown as FakeEventSource;
        stream.listeners.get('job-progress')?.({ data: JSON.stringify(frame('a', 1048576)) });
        expect(progressOf('a').value).toBe('50%');
        // Progress is not a lifecycle change: it refetches nothing.
        expect(list._refresher.request).not.toHaveBeenCalled();
        list.destroy();
    });
});

// happy-dom's PageTransitionEvent does not carry persisted, so the page show a
// browser dispatches is built by hand.
function pageShow(persisted: boolean) {
    const event = new Event('pageshow');
    Object.defineProperty(event, 'persisted', { value: persisted });
    return event;
}

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
    test('a page brought back from the back-forward cache shows the filters it was rendered with', () => {
        document.body.innerHTML = `<form>
            <input type="checkbox" name="kind" value="plugin-action">
            <input type="checkbox" name="state" value="succeeded" checked>
            <select name="pinned"><option value="">Any</option><option value="false">Not pinned</option></select>
            <input type="datetime-local" name="acceptedAfter" data-bound="start" data-instant="2026-09-01T10:00:00Z">
        </form>`;
        const form = document.querySelector('form')!;
        const times = Object.assign(jobFilterTimes(), { $root: form });
        times.init();
        const shown = form.querySelector<HTMLInputElement>('input[type="datetime-local"]')!.value;
        // The reader changes the form and applies it, and the browser keeps this
        // page as it was left: the new choices, and the instants submit() swapped in.
        form.querySelector<HTMLInputElement>('input[name="kind"]')!.checked = true;
        form.querySelector<HTMLSelectElement>('select')!.value = 'false';
        form.querySelector<HTMLInputElement>('input[type="datetime-local"]')!.value = '2026-09-03T08:00';
        times.submit();

        window.dispatchEvent(pageShow(true));

        const entries = [...new FormData(form).entries()];
        expect(entries).toEqual([
            ['state', 'succeeded'],
            ['pinned', ''],
            ['acceptedAfter', shown],
        ]);
        // A second apply sends the time the reader then chooses, not the old one.
        form.querySelector<HTMLInputElement>('input[type="datetime-local"]')!.value = '2026-09-04T08:00';
        times.submit();
        expect(new FormData(form).getAll('acceptedAfter')).toEqual([new Date('2026-09-04T08:00').toISOString()]);
        times.destroy();
    });

    test('an ordinary page show leaves the form alone', () => {
        document.body.innerHTML = `<form><input type="checkbox" name="kind" value="plugin-action"></form>`;
        const form = document.querySelector('form')!;
        const times = Object.assign(jobFilterTimes(), { $root: form });
        times.init();
        form.querySelector<HTMLInputElement>('input[name="kind"]')!.checked = true;
        window.dispatchEvent(pageShow(false));
        expect(form.querySelector<HTMLInputElement>('input[name="kind"]')!.checked).toBe(true);
        times.destroy();
    });
});
