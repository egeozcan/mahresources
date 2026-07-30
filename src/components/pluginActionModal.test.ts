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
