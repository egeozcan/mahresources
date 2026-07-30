import { describe, expect, test, vi, beforeEach, afterEach } from 'vitest';
import { pluginActionModal } from './pluginActionModal.js';

/**
 * Round-3 audit, the async plugin action's hand-off to the jobs panel.
 *
 * Two things go wrong in the same three lines. The modal closed itself and asked for
 * the panel in the same tick, so the panel read `document.activeElement` and got this
 * modal's own Run button — connected for one more tick, then removed by the x-if. The
 * panel's restore rejects a detached node, fell back to its header trigger, and the
 * reader ended a plugin action somewhere they had never been. And the panel now
 * declines to open underneath an open dialog, which in the same tick is what this
 * modal still is.
 */

let component: any;
let dispatched: CustomEvent[];
let pendingTicks: Array<() => void>;

function flushTicks() {
    const queued = pendingTicks;
    pendingTicks = [];
    queued.forEach(fn => fn());
}

beforeEach(() => {
    dispatched = [];
    pendingTicks = [];
    component = pluginActionModal();
    component.$nextTick = (fn: () => void) => pendingTicks.push(fn);
    // open() queues a focus-the-first-input tick against the mounted root.
    component.$root = { querySelector: () => null };
    component.action = {
        plugin: 'p', action: 'a', entityIds: [1], entityType: 'resource', params: null,
    };
    vi.stubGlobal('window', {
        dispatchEvent: (e: CustomEvent) => { dispatched.push(e); return true; },
    });
});

afterEach(() => {
    vi.unstubAllGlobals();
});

describe('the opener is captured when the modal opens, not when it closes', () => {
    test('open() records whatever control had focus', () => {
        const actionButton = { tagName: 'BUTTON' };
        vi.stubGlobal('document', { activeElement: actionButton, body: {}, documentElement: {} });

        component.open({ plugin: 'p', action: 'a', entityIds: [1], entityType: 'resource' });

        expect(component._opener).toBe(actionButton);
    });

    test('focus nowhere records nothing rather than <body>', () => {
        const body = { tagName: 'BODY' };
        vi.stubGlobal('document', { activeElement: body, body, documentElement: {} });

        component.open({ plugin: 'p', action: 'a', entityIds: [1], entityType: 'resource' });

        // focusedElement() reports null for <body>, so the panel falls through to its
        // own reasoning instead of being handed a target that cannot hold focus.
        expect(component._opener).toBeNull();
    });
});

describe('the panel is asked for after this modal has actually gone', () => {
    test('an async action hands the panel the control that started it', async () => {
        const actionButton = { tagName: 'BUTTON', isConnected: true };
        component._opener = actionButton;
        vi.stubGlobal('fetch', vi.fn().mockResolvedValue(
            new Response('{"job_id":"abc"}', { status: 200 })));

        await component.submit();

        // Nothing yet: dispatching here would be refused by the panel's guard, because
        // Alpine has not torn this modal down.
        expect(component.isOpen).toBe(false);
        expect(dispatched).toHaveLength(0);

        flushTicks();

        expect(dispatched).toHaveLength(1);
        expect(dispatched[0].type).toBe('jobs-panel-open');
        expect((dispatched[0] as any).detail.returnFocusTo).toBe(actionButton);
    });

    test('the control: a synchronous action does not ask for the panel at all', async () => {
        // A plain result reloads the page instead, so the hand-off must not fire.
        vi.stubGlobal('fetch', vi.fn().mockResolvedValue(
            new Response('{"message":"done"}', { status: 200 })));
        vi.stubGlobal('setTimeout', () => 0);

        await component.submit();
        flushTicks();

        expect(dispatched).toHaveLength(0);
        expect(component.result).toEqual({ message: 'done' });
    });

    test('the control: a rejected action keeps the modal open and says why', async () => {
        vi.stubGlobal('fetch', vi.fn().mockResolvedValue(
            new Response('{"error":"nope"}', { status: 400 })));

        await component.submit();
        flushTicks();

        expect(dispatched).toHaveLength(0);
        expect(component.errors._general).toBe('nope');
    });
});

/**
 * Round 4. Cancel stays enabled while a request is in flight, so this modal is
 * reusable between a submit and its own continuation.
 */
describe('round 4 — a stale run does not destroy the modal that replaced it', () => {
    test('action A returning after the reader opened action B leaves B alone', async () => {
        vi.stubGlobal('document', { activeElement: null, body: {}, documentElement: {} });

        // A is submitted and its request hangs.
        let resolveA: (r: Response) => void = () => {};
        vi.stubGlobal('fetch', vi.fn().mockReturnValue(new Promise<Response>(r => { resolveA = r; })));
        const submitA = component.submit();

        // The reader cancels out of A and opens B, filling it in.
        component.close();
        component.open({ plugin: 'p', action: 'b', entityIds: [2], entityType: 'resource' });
        component.formValues = { note: 'half typed' };

        // Only now does A answer.
        resolveA(new Response('{"job_id":"from-A"}', { status: 200 }));
        await submitA;
        flushTicks();

        expect(component.isOpen, 'a stale run closed the modal that replaced it').toBe(true);
        expect(component.action.action, 'a stale run replaced the open action').toBe('b');
        expect(component.formValues).toEqual({ note: 'half typed' });
        expect(dispatched, 'a stale run opened the jobs panel for an abandoned submission').toHaveLength(0);
    });

    test('the control: the run the modal is actually showing still hands over', async () => {
        vi.stubGlobal('document', { activeElement: null, body: {}, documentElement: {} });
        vi.stubGlobal('fetch', vi.fn().mockResolvedValue(
            new Response('{"job_id":"live"}', { status: 200 })));

        component.open({ plugin: 'p', action: 'a', entityIds: [1], entityType: 'resource' });
        await component.submit();
        flushTicks();

        expect(component.isOpen).toBe(false);
        expect(dispatched).toHaveLength(1);
    });
});

describe('round 4 — the opener a caller names wins over focus', () => {
    test('a card action menu hands over its trigger, not the menu item', () => {
        // cardActionMenu closes itself before dispatching, so the menu item the reader
        // activated is still connected but display:none and cannot take focus back.
        const menuTrigger = { tagName: 'BUTTON' };
        const menuItem = { tagName: 'A' };
        vi.stubGlobal('document', { activeElement: menuItem, body: {}, documentElement: {} });

        component.open({
            plugin: 'p', action: 'a', entityIds: [1], entityType: 'resource',
            returnFocusTo: menuTrigger,
        });

        expect(component._opener).toBe(menuTrigger);
    });

    test('the control: with no named opener, focus is still the answer', () => {
        const button = { tagName: 'BUTTON' };
        vi.stubGlobal('document', { activeElement: button, body: {}, documentElement: {} });

        component.open({ plugin: 'p', action: 'a', entityIds: [1], entityType: 'resource' });

        expect(component._opener).toBe(button);
    });
});
