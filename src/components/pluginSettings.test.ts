import { describe, expect, test, vi, beforeEach, afterEach } from 'vitest';
import { pluginSettings, SAVED_ANNOUNCEMENT_MS } from './pluginSettings.js';

/**
 * Final review of 2026-07-31, finding 4.
 *
 * `saved = true` was followed on the next line by `window.location.reload()`. The
 * property under test is therefore not "the status was set" — it always was, and
 * because reload() is asynchronous the span could even paint for a few hundred ms.
 * The property is that the save path does not *begin a navigation in the same task*
 * as the announcement, because a live-region mutation racing the teardown of its own
 * document is an announcement nobody hears.
 *
 * So every test here asserts against a reload spy, and the timing is asserted by
 * flushing microtasks (Alpine schedules its effects on one) and demanding that the
 * navigation still has not started.
 */

let component: any;
let reload: ReturnType<typeof vi.fn>;
let timers: Array<{ id: number; fn: () => void; delay: number }>;
let cleared: number[];
let nextTimerId: number;

/** The suite runs in plain node, so the form and FormData are stand-ins. */
function fakeSubmitEvent(values: Record<string, string> = { api_key: 'k' }) {
    vi.stubGlobal('FormData', class {
        entries() { return Object.entries(values)[Symbol.iterator](); }
    });
    return { target: { querySelectorAll: () => [] } };
}

function respondWith(body: string, status = 200) {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(
        new Response(body, { status, headers: { 'Content-Type': 'application/json' } })));
}

/** Let Alpine-style microtask work run without letting any timer fire. */
async function flushMicrotasks() {
    await Promise.resolve();
    await new Promise<void>(resolve => queueMicrotask(resolve));
}

beforeEach(() => {
    component = pluginSettings('test-banner');
    reload = vi.fn();
    timers = [];
    cleared = [];
    nextTimerId = 1;
    vi.stubGlobal('window', { location: { reload } });
    vi.stubGlobal('setTimeout', (fn: () => void, delay: number) => {
        const id = nextTimerId++;
        timers.push({ id, fn, delay });
        return id;
    });
    vi.stubGlobal('clearTimeout', (id: number) => { cleared.push(id); });
});

afterEach(() => {
    vi.unstubAllGlobals();
});

describe('the announcement is not raced by its own navigation', () => {
    test('a successful save does not begin a navigation in the same task', async () => {
        respondWith('{"ok":true}');

        await component.saveSettings(fakeSubmitEvent());

        expect(component.saved, 'nothing was announced at all').toBe(true);
        expect(reload, 'the navigation began in the same task as the announcement').not.toHaveBeenCalled();

        // Alpine's x-show/x-text effect runs on a microtask. Draining every microtask
        // there is must still leave the document alive.
        await flushMicrotasks();
        expect(reload, 'the navigation began before the status could be observed').not.toHaveBeenCalled();
    });

    test('the reload is scheduled, with time for the status to stand', async () => {
        respondWith('{"ok":true}');

        await component.saveSettings(fakeSubmitEvent());

        expect(timers).toHaveLength(1);
        expect(timers[0].delay).toBe(SAVED_ANNOUNCEMENT_MS);
        expect(SAVED_ANNOUNCEMENT_MS).toBeGreaterThan(0);
    });

    test('the reload still happens — finding 136 needs the server to re-render', async () => {
        respondWith('{"ok":true}');

        await component.saveSettings(fakeSubmitEvent());
        timers[0].fn();

        expect(reload).toHaveBeenCalledTimes(1);
    });
});

describe('the control: a save that failed announces nothing and goes nowhere', () => {
    test('a validation failure reports the message and schedules no navigation', async () => {
        respondWith('{"errors":[{"message":"api_key is required"}]}', 400);

        await component.saveSettings(fakeSubmitEvent());
        await flushMicrotasks();

        expect(component.error).toBe('api_key is required');
        expect(component.saved).toBe(false);
        expect(timers, 'a failed save scheduled a reload').toHaveLength(0);
        expect(reload).not.toHaveBeenCalled();
    });

    test('a thrown request reports the message and schedules no navigation', async () => {
        vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('offline')));

        await component.saveSettings(fakeSubmitEvent());

        expect(component.error).toBe('offline');
        expect(component.saved).toBe(false);
        expect(timers).toHaveLength(0);
    });
});

describe('the delay does not cut short the save that follows it', () => {
    test('a second save cancels the first save\'s pending navigation', async () => {
        respondWith('{"ok":true}');
        await component.saveSettings(fakeSubmitEvent());
        const pending = timers[0].id;

        // The reader spots a typo and saves again, well inside the window. That request
        // hangs here so the assertion is about the state it runs in.
        vi.stubGlobal('fetch', vi.fn().mockReturnValue(new Promise<Response>(() => {})));
        void component.saveSettings(fakeSubmitEvent({ api_key: 'corrected' }));
        await flushMicrotasks();

        expect(cleared, 'the old reload would have aborted the new request mid-flight')
            .toContain(pending);
        expect(reload).not.toHaveBeenCalled();
    });
});
