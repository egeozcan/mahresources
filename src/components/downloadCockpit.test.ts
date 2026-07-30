import { describe, expect, test, vi, beforeEach, afterEach } from 'vitest';
import { downloadCockpit } from './downloadCockpit.js';

/**
 * WS9 — jobs cockpit predicates and ordering.
 *
 * Findings 2, 40, 41 and 113 of docs/ui-bug-hunt-2026-07-29.md. These are pure
 * functions of a job payload, so they are cheaper and more precise to pin here than
 * in a browser; ws9-jobs-cockpit.spec.ts asserts that the template actually uses
 * them and that a real paused download behaves.
 */

function job(overrides = {}) {
    return {
        id: 'a1',
        url: 'http://example.com/files/1Gb.dat',
        status: 'downloading',
        progress: 0,
        totalSize: -1,
        progressPercent: -1,
        createdAt: '2026-07-29T10:00:00Z',
        ...overrides,
    };
}

let component;

beforeEach(() => {
    component = downloadCockpit();
    // init() touches the DOM and opens an EventSource; the predicates under test do
    // not need either, so the component is used unmounted.
    component._dismissedIds = new Set();
});

afterEach(() => {
    vi.unstubAllGlobals();
});

describe('finding 2 — a paused job can be cancelled', () => {
    test('canCancel covers paused as well as the active states', () => {
        for (const status of ['pending', 'downloading', 'processing', 'paused']) {
            expect(component.canCancel(job({ status })), status).toBe(true);
        }
        // The control: a terminal job must not offer Cancel, or the button would
        // produce the 409 the fix introduced.
        for (const status of ['completed', 'failed', 'cancelled']) {
            expect(component.canCancel(job({ status })), status).toBe(false);
        }
    });

    test('paused is neither active nor finished', () => {
        const paused = job({ status: 'paused' });
        expect(component.isActive(paused)).toBe(false);
        expect(component.isFinished(paused)).toBe(false);
    });
});

describe('finding 41 — a paused job keeps its progress readout', () => {
    test('showsProgress covers downloading and paused', () => {
        expect(component.showsProgress(job({ status: 'downloading' }))).toBe(true);
        expect(component.showsProgress(job({ status: 'paused' }))).toBe(true);
        expect(component.showsProgress(job({ status: 'completed' }))).toBe(false);
        expect(component.showsProgress(job({ status: 'pending' }))).toBe(false);
    });

    test('the bytes and percentage a paused job reports are still formatted', () => {
        // The numbers are the ones measured live against a paused download: the
        // server keeps them, only the panel had stopped rendering them.
        const paused = job({ status: 'paused', progress: 196608, totalSize: 52428800, progressPercent: 0.375 });
        expect(component.formatProgress(paused)).toBe('192 KB / 50 MB (0.4%)');
    });
});

describe('finding 113 — the progress bar is named and can be indeterminate', () => {
    test('the name includes the job, not just the prefix', () => {
        const running = job({ progress: 5242880, totalSize: 104857600, progressPercent: 5 });
        expect(component.progressLabel(running)).toBe('Download progress: 1Gb.dat, 5 MB / 100 MB (5.0%)');
    });

    test('an unknown total is named and described rather than reported as 0%', () => {
        const unknown = job({ progress: 0, totalSize: -1 });
        // Before the fix this was `'Download progress: ' + formatProgress(job)`,
        // which is the bare prefix when nothing has arrived and the size is unknown.
        expect(component.progressLabel(unknown)).toBe('Download progress: 1Gb.dat, size unknown');
        expect(component.progressValueNow(unknown)).toBeNull();
        expect(component.progressValueText(unknown)).toBe('Waiting for the first bytes');
    });

    test('a known total reports a value and no valuetext', () => {
        const known = job({ progress: 50, totalSize: 100, progressPercent: 50 });
        expect(component.progressValueNow(known)).toBe(50);
        expect(component.progressValueText(known)).toBeNull();
    });

    test('a paused job says so in its name', () => {
        const paused = job({ status: 'paused', progress: 196608, totalSize: 52428800, progressPercent: 0.375 });
        expect(component.progressLabel(paused)).toContain('paused at');
    });
});

describe('finding 40 — newest first, and finished jobs can be dismissed', () => {
    test('displayJobs is ordered newest first', () => {
        component.jobs = [
            job({ id: 'old', createdAt: '2026-07-29T08:00:00Z' }),
            job({ id: 'new', createdAt: '2026-07-29T12:00:00Z' }),
            job({ id: 'middle', createdAt: '2026-07-29T10:00:00Z' }),
        ];
        expect(component.displayJobs.map(j => j.id)).toEqual(['new', 'middle', 'old']);
    });

    test('jobs sharing a timestamp keep a stable order', () => {
        const stamp = '2026-07-29T08:00:00Z';
        component.jobs = [
            job({ id: 'aaa', createdAt: stamp }),
            job({ id: 'bbb', createdAt: stamp }),
        ];
        // SubmitMultiple copies one creator per URL, so a multi-URL submit lands
        // several jobs on the same instant; without a tie-break the order would
        // depend on the sort implementation.
        expect(component.displayJobs.map(j => j.id)).toEqual(['bbb', 'aaa']);
    });

    test('hasFinishedJobs gates the Clear completed control', () => {
        component.jobs = [job({ status: 'downloading' }), job({ id: 'p', status: 'paused' })];
        expect(component.hasFinishedJobs).toBe(false);
        component.jobs.push(job({ id: 'done', status: 'completed' }));
        expect(component.hasFinishedJobs).toBe(true);
    });

    test('clearCompleted drops finished jobs only after the server agrees', async () => {
        component.jobs = [
            job({ id: 'done', status: 'completed' }),
            job({ id: 'live', status: 'downloading' }),
            job({ id: 'held', status: 'paused' }),
        ];
        const fetch = vi.fn().mockResolvedValue(
            new Response('{"cleared":1,"ids":["done"]}', { status: 200 }));
        vi.stubGlobal('fetch', fetch);

        await component.clearCompleted();

        expect(fetch).toHaveBeenCalledWith('/v1/jobs/clearCompleted', { method: 'POST' });
        expect(component.displayJobs.map(j => j.id).sort()).toEqual(['held', 'live']);
    });

    test('a rejected clear leaves the panel alone', async () => {
        component.jobs = [job({ id: 'done', status: 'completed' })];
        vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('nope', { status: 500 })));

        await component.clearCompleted();

        expect(component.displayJobs.map(j => j.id)).toEqual(['done']);
    });

    test('a cleared job is not re-added by the removal event it provokes', async () => {
        component.jobs = [job({ id: 'done', status: 'completed' })];
        vi.stubGlobal('fetch', vi.fn().mockResolvedValue(
            new Response('{"cleared":1,"ids":["done"]}', { status: 200 })));

        await component.clearCompleted();

        // The SSE `removed` handler's job is to retain finished jobs for display, so
        // without the dismissed-id set every cleared row would come straight back a
        // moment after the button was pressed.
        component.retainedCompletedJobs = [job({ id: 'done', status: 'completed' })];
        expect(component.displayJobs.map(j => j.id)).toEqual([]);
    });
});

/**
 * Review remediation finding 2. The clear used to dismiss the ids *it* thought were
 * finished, snapshotted before `await fetch(...)`; the server decides at handling
 * time. A job that crossed into a terminal state inside that window was cleared
 * server-side, was missing from the dismissed set, and the retain-for-display path
 * put it back — a phantom finished row that survived until the next reconnect.
 */
describe('review finding 2 — the clear dismisses what the server actually cleared', () => {
    test('a job that finished while the request was in flight is dismissed too', async () => {
        // The panel still believes this one is downloading: no `updated` event has
        // arrived yet when the button is pressed.
        component.jobs = [
            job({ id: 'done', status: 'completed' }),
            job({ id: 'racer', status: 'downloading' }),
        ];
        vi.stubGlobal('fetch', vi.fn().mockResolvedValue(
            new Response('{"cleared":2,"ids":["done","racer"]}', { status: 200 })));

        await component.clearCompleted();

        // Then the SSE traffic the clear provokes arrives: the terminal `updated`,
        // then `removed`, whose handler retains finished jobs for display.
        component.retainedCompletedJobs = [job({ id: 'racer', status: 'cancelled' })];

        expect(component.displayJobs.map(j => j.id)).toEqual([]);
    });

    test('the positive control: a job the server kept is still shown', async () => {
        component.jobs = [
            job({ id: 'done', status: 'completed' }),
            job({ id: 'held', status: 'paused' }),
        ];
        vi.stubGlobal('fetch', vi.fn().mockResolvedValue(
            new Response('{"cleared":1,"ids":["done"]}', { status: 200 })));

        await component.clearCompleted();

        // If the client dismissed its whole snapshot regardless, or everything it
        // could see, the paused row would vanish while the server still has it.
        expect(component.displayJobs.map(j => j.id)).toEqual(['held']);
    });

    test('an unreadable success body falls back to the rows that looked finished', async () => {
        // A 2xx means the server did clear them, so the panel must not keep showing
        // rows that are gone. The snapshot is the best guess left.
        component.jobs = [job({ id: 'done', status: 'completed' })];
        vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('<html>', { status: 200 })));

        await component.clearCompleted();

        expect(component.displayJobs.map(j => j.id)).toEqual([]);
    });

    test('a dismissed plugin action row is not retained either', () => {
        // The same defect in the other removal handler: `action_removed` never
        // consulted the dismissed set, so clearing a finished action left its row.
        component.jobs = [job({ id: 'act', status: 'completed', _isAction: true })];
        component._dismissedIds.add('act');

        component.handleJobRemoved({ id: 'act' }, { isAction: true });

        expect(component.retainedCompletedJobs).toEqual([]);
        expect(component.displayJobs.map(j => j.id)).toEqual([]);
    });

    test('the control: an undismissed finished row is still retained for display', () => {
        component.jobs = [job({ id: 'act', status: 'completed', _isAction: true })];

        component.handleJobRemoved({ id: 'act' }, { isAction: true });

        expect(component.retainedCompletedJobs.map(j => j.id)).toEqual(['act']);
        expect(component.displayJobs.map(j => j.id)).toEqual(['act']);
    });

    test('an active row the server dropped is not retained', () => {
        // Eviction and the retention sweep both remove jobs; only finished ones are
        // worth keeping on screen.
        component.jobs = [job({ id: 'live', status: 'downloading' })];

        component.handleJobRemoved({ id: 'live' });

        expect(component.retainedCompletedJobs).toEqual([]);
        expect(component.displayJobs).toEqual([]);
    });
});

/**
 * Review remediation finding 4: the Cmd/Ctrl+Shift+D handler calls toggle() with no
 * event, so captureTrigger had nothing to read and _lastTrigger stayed null — and
 * the restore was gated on it being truthy. The panel is x-trap.noreturn and its
 * contents are torn down by the x-if, so focus fell to <body>.
 *
 * Where focus actually lands is a browser question and is asserted by pressing Tab
 * in ws9-jobs-cockpit.spec.ts. What is decidable here is which element the component
 * chooses to return to.
 */
describe('review finding 4 — the keyboard shortcut has somewhere to return focus to', () => {
    afterEach(() => {
        vi.unstubAllGlobals();
    });

    test('with no event, whatever had focus is captured', () => {
        const input = { tagName: 'INPUT' };
        vi.stubGlobal('document', { activeElement: input, body: {}, documentElement: {} });
        component._trigger = { tagName: 'BUTTON' };

        component.toggle();

        expect(component._lastTrigger).toBe(input);
    });

    test('with focus nowhere, the trigger is the floor rather than <body>', () => {
        const body = { tagName: 'BODY' };
        vi.stubGlobal('document', { activeElement: body, body, documentElement: {} });
        const trigger = { tagName: 'BUTTON' };
        component._trigger = trigger;

        component.toggle();

        // Before the fix this was null, and the $watch restore was gated on it.
        expect(component._lastTrigger).toBe(trigger);
    });

    test('the control: a click still returns to the button that was clicked', () => {
        const clicked = { tagName: 'BUTTON' };
        const somethingElse = { tagName: 'A' };
        vi.stubGlobal('document', { activeElement: somethingElse, body: {}, documentElement: {} });
        component._trigger = { tagName: 'BUTTON' };

        component.toggle({ currentTarget: clicked, target: {} });

        // currentTarget, not activeElement: a click is dispatched before focus
        // settles, and the control the reader pressed is the one to come back to.
        expect(component._lastTrigger).toBe(clicked);
    });

    test('closing does not clear the trigger, so a reopen still has a floor', () => {
        const trigger = { tagName: 'BUTTON' };
        component._trigger = trigger;
        vi.stubGlobal('document', { activeElement: {}, body: {}, documentElement: {} });

        component.toggle();
        expect(component.isOpen).toBe(true);
        component.toggle();
        expect(component.isOpen).toBe(false);
        expect(component._trigger).toBe(trigger);
    });
});
