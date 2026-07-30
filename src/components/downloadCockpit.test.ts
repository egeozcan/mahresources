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
 * Round-3 audit of the jobs cockpit. The matrix in docs/todo.md crossed every job
 * state with every control; these are the cells whose failure is decidable here
 * rather than in Go — the panel's own reading of what the server told it.
 */
describe('round 3 — the panel and the server agree about which rows exist', () => {
    test('a job named by both the init listing and its own added event is one row', () => {
        // The SSE handler subscribes and *then* lists the queue, which is the right
        // order — a job created between the two would otherwise be missed entirely.
        // The cost is that a job created in that window arrives twice. Pushing both
        // left a second row that no `updated` ever reached, because updates resolve a
        // job by the first id that matches: it sat at "Pending" with live controls on
        // it for the rest of the session.
        component.jobs = [job({ id: 'racer', status: 'pending' })];

        const isNew = component.upsertJob(job({ id: 'racer', status: 'downloading', progress: 4096 }));

        expect(isNew).toBe(false);
        expect(component.jobs).toHaveLength(1);
        expect(component.jobs[0].status).toBe('downloading');
        expect(component.jobs[0].progress).toBe(4096);
    });

    test('the control: a job the panel has not seen is added as a new row', () => {
        component.jobs = [job({ id: 'other' })];

        const isNew = component.upsertJob(job({ id: 'fresh' }));

        expect(isNew).toBe(true);
        expect(component.jobs.map(j => j.id)).toEqual(['other', 'fresh']);
    });

    test('a paused row the retention sweep removed is not kept on screen', () => {
        // Paused is neither active nor finished, and the retain-for-display path
        // tested for "not active" while its comment said "finished". A paused
        // download the 24-hour sweep removed stayed as a row whose Resume and Cancel
        // addressed a job the server no longer had.
        component.jobs = [job({ id: 'held', status: 'paused' })];

        component.handleJobRemoved({ id: 'held' });

        expect(component.retainedCompletedJobs).toEqual([]);
        expect(component.displayJobs).toEqual([]);
    });
});

describe('round 3 — a refused control says so', () => {
    /**
     * All four controls were `fetch(...).catch(console.error)`, and fetch does not
     * reject on 4xx. Every refusal these endpoints produce is reachable from the
     * panel, because a row's state is a snapshot from the last event that arrived:
     * Cancel on a download that has just completed answers 409, anything on a row the
     * sweep removed answers 404. Nothing moved and nothing was said.
     */
    test('a 409 is announced with the server\'s own words', async () => {
        const announced: string[] = [];
        component.announce = (m: string) => announced.push(m);
        vi.stubGlobal('fetch', vi.fn().mockResolvedValue(
            new Response('{"error":"job a1 cannot be cancelled (status: completed)"}', { status: 409 })));

        const ok = await component.cancelJob('a1');

        expect(ok).toBe(false);
        expect(announced).toEqual(['job a1 cannot be cancelled (status: completed)']);
    });

    test('a refusal with no readable body still says something', async () => {
        const announced: string[] = [];
        component.announce = (m: string) => announced.push(m);
        vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('<html>', { status: 404 })));

        await component.resumeJob('a1');

        expect(announced).toEqual(['That job could not be resumed.']);
    });

    test('the control: an accepted control announces nothing, because the row will say it', async () => {
        const announced: string[] = [];
        component.announce = (m: string) => announced.push(m);
        const fetchMock = vi.fn().mockResolvedValue(new Response('{"status":"paused"}', { status: 200 }));
        vi.stubGlobal('fetch', fetchMock);

        const ok = await component.pauseJob('a1');

        expect(ok).toBe(true);
        expect(announced).toEqual([]);
        expect(fetchMock).toHaveBeenCalledWith('/v1/jobs/pause', expect.objectContaining({
            method: 'POST',
            body: 'id=a1',
        }));
    });
});

describe('round 3 — the panel declines to open underneath a modal', () => {
    /**
     * `.header` is a stacking context at z-index 40, so the panel's z-[60] orders it
     * against header siblings only. The app's true modals live in `.overlays` at
     * z-index 41 in the root stacking context — above the entire header layer — so
     * the panel opens *behind* one, moves focus inside it and traps it there with
     * x-trap. Whether it renders behind or in front, two aria-modal dialogs open at
     * once is the defect, so the panel declines.
     *
     * Which element wins the hit test is a browser question and is asserted in
     * ws9-jobs-cockpit.spec.ts. What is decidable here is the decision.
     */
    function stubOverlays(modals: any[]) {
        vi.stubGlobal('document', {
            querySelectorAll: (sel: string) =>
                sel === '[aria-modal="true"]' ? modals : [],
            activeElement: null,
            body: {},
            documentElement: {},
        });
    }

    const shown = { isConnected: true, checkVisibility: () => true };
    const hidden = { isConnected: true, checkVisibility: () => false };

    test('the keyboard shortcut does not open the panel while a dialog is up', () => {
        stubOverlays([hidden, shown]);
        const announced: string[] = [];
        component.announce = (m: string) => announced.push(m);

        component.toggle();

        expect(component.isOpen).toBe(false);
        expect(announced[0]).toContain('A dialog is open');
    });

    test('an incoming plugin action job does not open it either', () => {
        stubOverlays([shown]);
        component.announce = () => {};

        component.openFromEvent();

        expect(component.isOpen).toBe(false);
    });

    test('the control: overlays that exist but are not rendered do not block it', () => {
        // Three of the four overlays use x-show, so they stay in the document with
        // display:none. Querying for them is not enough on its own — a guard that
        // counted those would leave the panel permanently unopenable.
        stubOverlays([hidden, hidden]);
        component._trigger = { tagName: 'BUTTON' };

        component.toggle();

        expect(component.isOpen).toBe(true);
        expect(component.blockingModal()).toBeNull();
    });

    test('an already-open panel still closes while a dialog is up', () => {
        // The guard is on opening. Refusing to *close* would be a trap of its own.
        stubOverlays([shown]);
        component.isOpen = true;

        component.toggle();

        expect(component.isOpen).toBe(false);
    });
});

/**
 * Round 4. Every finding below is one an independent review found in the round-3 fix
 * itself, which is the point: the guard, the dedupe and the retain-for-display each
 * shipped covering less than they claimed.
 */
describe('round 4 — the guard covers every dialog, not one layer of them', () => {
    const shown = { isConnected: true, checkVisibility: () => true };

    test('a dialog outside .overlays blocks the panel too', () => {
        // The first version queried `.overlays [aria-modal="true"]`. The global search
        // dialog is a *header sibling* of this panel — same z-index, ordered by DOM
        // position — and six more live in mrql.tpl, json.tpl, menu.tpl,
        // blockEditor.tpl, schemaEditorModal.tpl and globalSearch.tpl. A guard that
        // covers four of ten dialogs recreates the defect for the other six.
        const headerDialog = { ...shown, id: 'global-search' };
        vi.stubGlobal('document', {
            querySelectorAll: (sel: string) =>
                sel === '[aria-modal="true"]' ? [headerDialog] : [],
            activeElement: null, body: {}, documentElement: {},
        });
        component.announce = () => {};

        component.toggle();

        expect(component.isOpen).toBe(false);
        expect(component.blockingModal()).toBe(headerDialog);
    });

    test('the panel never counts itself as the thing blocking it', () => {
        // The sweep is document-wide now, and this panel is an aria-modal dialog. A
        // path that consults the guard while the panel is open would otherwise find
        // itself and refuse.
        const ownPanel = { ...shown };
        component._root = { contains: (el: any) => el === ownPanel };
        vi.stubGlobal('document', {
            querySelectorAll: (sel: string) =>
                sel === '[aria-modal="true"]' ? [ownPanel] : [],
            activeElement: null, body: {}, documentElement: {},
        });

        expect(component.blockingModal()).toBeNull();
    });
});

describe('round 4 — a removed job is judged by what the server said, not by the local row', () => {
    test('a removal that reports completion is retained even if the update was missed', () => {
        // notifySubscribers drops an event rather than blocking on a slow subscriber,
        // so the terminal `updated` can simply not arrive. The local row then still
        // says "downloading", is not finished, is not retained — and the completion,
        // its warnings and an export's download link disappear at the moment they
        // become useful.
        component.jobs = [job({ id: 'exp', status: 'downloading', source: 'group-export' })];

        component.handleJobRemoved({ id: 'exp', status: 'completed', resultPath: '_exports/exp.tar' });

        expect(component.retainedCompletedJobs.map(j => j.id)).toEqual(['exp']);
        expect(component.retainedCompletedJobs[0].status).toBe('completed');
        expect(component.retainedCompletedJobs[0].resultPath).toBe('_exports/exp.tar');
    });

    test('the control: a removal that reports an active job is still dropped', () => {
        component.jobs = [job({ id: 'live', status: 'downloading' })];

        component.handleJobRemoved({ id: 'live', status: 'downloading' });

        expect(component.retainedCompletedJobs).toEqual([]);
    });
});

describe('round 3 — a plugin action returns focus to the control that started it', () => {
    test('openFromEvent prefers the opener the request names', () => {
        // pluginActionModal closes itself and then asks for the panel, so what has
        // focus at that moment is its own Run button — which its x-if is about to
        // remove. The panel's restore rejects a detached node and falls back to the
        // header trigger, so the reader ended a plugin action somewhere they had
        // never been.
        const runButton = { tagName: 'BUTTON', isConnected: true, checkVisibility: () => true };
        const actionButton = { tagName: 'BUTTON', isConnected: true, checkVisibility: () => true };
        vi.stubGlobal('document', {
            querySelectorAll: () => [],
            activeElement: runButton,
            body: {},
            documentElement: {},
        });
        component._trigger = { tagName: 'BUTTON' };

        component.openFromEvent({ returnFocusTo: actionButton });

        expect(component._lastTrigger).toBe(actionButton);
    });

    test('an opener that is connected but no longer painted is rejected too', () => {
        // Round 4: a card action menu closes via x-show before dispatching, so its
        // menu item stays in the document at display:none. `isConnected` is true and
        // focusing it silently fails, which lands the reader on <body> — the state all
        // of this exists to avoid. The check is "is it painted", not "is it attached".
        const hidden = { tagName: 'BUTTON', isConnected: true, checkVisibility: () => false };
        const live = { tagName: 'INPUT' };
        vi.stubGlobal('document', {
            querySelectorAll: () => [],
            activeElement: live,
            body: {},
            documentElement: {},
        });
        component._trigger = { tagName: 'BUTTON' };

        component.openFromEvent({ returnFocusTo: hidden });

        expect(component._lastTrigger).toBe(live);
    });

    test('an opener that is already gone falls back to where focus is', () => {
        const detached = { tagName: 'BUTTON', isConnected: false };
        const live = { tagName: 'INPUT' };
        vi.stubGlobal('document', {
            querySelectorAll: () => [],
            activeElement: live,
            body: {},
            documentElement: {},
        });
        component._trigger = { tagName: 'BUTTON' };

        component.openFromEvent({ returnFocusTo: detached });

        expect(component._lastTrigger).toBe(live);
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
        vi.stubGlobal('document', { querySelectorAll: () => [], activeElement: input, body: {}, documentElement: {} });
        component._trigger = { tagName: 'BUTTON' };

        component.toggle();

        expect(component._lastTrigger).toBe(input);
    });

    test('with focus nowhere, the trigger is the floor rather than <body>', () => {
        const body = { tagName: 'BODY' };
        vi.stubGlobal('document', { querySelectorAll: () => [], activeElement: body, body, documentElement: {} });
        const trigger = { tagName: 'BUTTON' };
        component._trigger = trigger;

        component.toggle();

        // Before the fix this was null, and the $watch restore was gated on it.
        expect(component._lastTrigger).toBe(trigger);
    });

    test('the control: a click still returns to the button that was clicked', () => {
        const clicked = { tagName: 'BUTTON' };
        const somethingElse = { tagName: 'A' };
        vi.stubGlobal('document', { querySelectorAll: () => [], activeElement: somethingElse, body: {}, documentElement: {} });
        component._trigger = { tagName: 'BUTTON' };

        component.toggle({ currentTarget: clicked, target: {} });

        // currentTarget, not activeElement: a click is dispatched before focus
        // settles, and the control the reader pressed is the one to come back to.
        expect(component._lastTrigger).toBe(clicked);
    });

    test('a reopen does not return to a trigger captured on an earlier open', () => {
        // The floor is the trigger element, not `this._lastTrigger`. A control captured
        // on a previous open may be long gone from the page — the panel's own rows are
        // rebuilt constantly — and restoring to a detached node lands on <body>.
        const stale = { tagName: 'A' };
        const trigger = { tagName: 'BUTTON' };
        component._trigger = trigger;

        vi.stubGlobal('document', { querySelectorAll: () => [], activeElement: stale, body: {}, documentElement: {} });
        component.toggle();
        component.toggle();
        expect(component.isOpen).toBe(false);

        // Second open, with focus nowhere: the stale capture must not be reused.
        const body = { tagName: 'BODY' };
        vi.stubGlobal('document', { querySelectorAll: () => [], activeElement: body, body, documentElement: {} });
        component.toggle();

        expect(component._lastTrigger).toBe(trigger);
    });

    test('an event-less open from elsewhere still captures where the reader was', () => {
        // `jobs-panel-open` and an incoming plugin action job both set isOpen directly.
        // openFromEvent is what those paths call so the reader is returned to the
        // control they were on, rather than to the panel's own trigger.
        const input = { tagName: 'INPUT' };
        vi.stubGlobal('document', { querySelectorAll: () => [], activeElement: input, body: {}, documentElement: {} });
        component._trigger = { tagName: 'BUTTON' };

        component.openFromEvent();

        expect(component.isOpen).toBe(true);
        expect(component._lastTrigger).toBe(input);
    });
});
