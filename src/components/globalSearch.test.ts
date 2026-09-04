import { describe, expect, test, vi, beforeEach, afterEach } from 'vitest';
import { globalSearch } from './globalSearch.js';

/**
 * Two aria-modal dialogs must never be open at once.
 *
 * The jobs cockpit already refuses to open while another dialog is up, and the
 * comment above its guard names ten dialogs in this app — `globalSearch.tpl` among
 * them. The guard was installed on one side only, so the collision it describes was
 * still reachable from the other: Cmd+K with the jobs panel open mounted a second
 * `aria-modal` dialog with its own `x-trap`, and the two traps fought over the
 * reader.
 *
 * Both dialogs are `fixed inset-0 z-[60]` inside `.header`'s z-40 stacking context,
 * so `z-[60]` orders them against each other and not against the page, and the later
 * DOM sibling wins. That makes "which one paints on top" an accident of template
 * order — which is why the rule has to be that *neither* opens over the other,
 * rather than that this one wins.
 */

let component;

/** A stand-in for a painted (or unpainted) aria-modal dialog. */
function modal({ rendered = true } = {}) {
    return { isConnected: true, checkVisibility: () => rendered };
}

/**
 * The suite runs without a DOM, so `document` is stubbed the way the other
 * component tests here do it. `querySelectorAll` is the only part the guard reads.
 */
function stubDocument(modals = []) {
    const body = {};
    vi.stubGlobal('document', {
        activeElement: body,
        body,
        documentElement: {},
        querySelectorAll: () => modals,
    });
}

beforeEach(() => {
    component = globalSearch();
    component.announce = () => {};
});

afterEach(() => {
    vi.unstubAllGlobals();
});

describe('globalSearch refuses to open under another dialog', () => {
    test('does not open while a rendered aria-modal dialog is up', () => {
        stubDocument([modal()]);

        component.toggle();

        expect(component.isOpen).toBe(false);
    });

    test('opens normally when no dialog is up', () => {
        stubDocument([]);

        component.toggle();

        expect(component.isOpen).toBe(true);
    });

    test('opens when the only aria-modal in the document is not painted', () => {
        // x-show leaves several of this app's overlays in the document with
        // display:none, so querying for [aria-modal] alone would refuse forever.
        stubDocument([modal({ rendered: false })]);

        component.toggle();

        expect(component.isOpen).toBe(true);
    });

    test('closing is never blocked, or an open dialog could trap the reader', () => {
        stubDocument([]);
        component.toggle();
        expect(component.isOpen).toBe(true);

        // On a real page this dialog's own markup is in the document by now. A guard
        // that ran on the way out would find it and refuse to close.
        stubDocument([modal()]);
        component.toggle();

        expect(component.isOpen).toBe(false);
    });
});

describe('globalSearch result identity', () => {
    test('prefers the server-provided taxonomy display type', () => {
        expect(component.getResultLabel({ type: 'note', displayType: 'PM Task' })).toBe('PM Task');
        expect(component.getResultLabel({ type: 'note' })).toBe('Note');
    });

    test('announces the taxonomy display type for keyboard selection', () => {
        const announce = vi.fn();
        component.announce = announce;
        component.results = [{ id: 1, type: 'note', displayType: 'PM Task', name: 'Ship', url: '/note?id=1' }];
        component.selectedIndex = 0;
        component.announceSelectedResult();
        expect(announce).toHaveBeenCalledWith('Ship, PM Task, 1 of 1');
    });
});
