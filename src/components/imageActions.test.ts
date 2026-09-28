import { describe, expect, test, vi, beforeEach, afterEach } from 'vitest';
import { imageActions } from './imageActions.js';
import { customThumbnail } from './customThumbnail.js';

/**
 * The image actions sidebar group became a button that opens a popup, on the same
 * rules as the Custom Thumbnail popup beside it.
 *
 * The rules are in `sidebarPopup.js` and are asserted once, through
 * `customThumbnail` below — two components sharing one implementation is the
 * point, and the second copy is the one that drifts. What is specific to this
 * component is the hand-off: Crop opens a native `<dialog>` that lives outside
 * the popup, so the two must not both be open, and the reader must come back
 * somewhere real when the crop dialog closes.
 */

let component: any;
let pendingTicks: Array<() => void>;
let cropModal: { showModal: ReturnType<typeof vi.fn> };

function flushTicks() {
    const queued = pendingTicks;
    pendingTicks = [];
    queued.forEach((fn) => fn());
}

function button() {
    return { tagName: 'BUTTON', nodeType: 1, isConnected: true, matches: () => true, focus: vi.fn() };
}

beforeEach(() => {
    pendingTicks = [];
    cropModal = { showModal: vi.fn() };
    component = imageActions({ resourceId: 7 });
    component.$nextTick = (fn: () => void) => pendingTicks.push(fn);
    component.$root = { querySelector: () => null, contains: () => false };
    vi.stubGlobal('document', {
        activeElement: null,
        body: {},
        documentElement: {},
        querySelectorAll: () => [],
        getElementById: (id: string) => (id === 'crop-modal-7' ? cropModal : null),
    });
});

afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
});

describe('the image actions popup', () => {
    test('is closed until the button is pressed', () => {
        expect(component.isOpen).toBe(false);
    });

    test('open() takes the button from the event, not from focus', () => {
        // A mouse press does not focus a button in every browser, so
        // `focusedElement()` here would report nothing and the reader would be
        // dropped on <body> when the popup closed.
        const opener = button();
        component.open({ currentTarget: opener });

        expect(component.isOpen).toBe(true);
        expect(component._opener).toBe(opener);
    });

    test('open() with no event — a shortcut — falls back to focus', () => {
        const opener = button();
        (globalThis as any).document.activeElement = opener;

        component.open();

        expect(component._opener).toBe(opener);
    });

    test('open() is a no-op while another dialog is painted', () => {
        (globalThis as any).document = {
            activeElement: null,
            body: {},
            documentElement: {},
            querySelectorAll: () => [{ isConnected: true, checkVisibility: () => true }],
        };

        component.open({ currentTarget: button() });

        expect(component.isOpen).toBe(false);
    });

    test('close() returns focus to the opener, a tick later', () => {
        const opener = button();
        component.open({ currentTarget: opener });

        component.close();
        expect(opener.focus).not.toHaveBeenCalled();
        flushTicks();

        expect(component.isOpen).toBe(false);
        expect(opener.focus).toHaveBeenCalled();
    });
});

describe('opening the crop dialog from the popup', () => {
    test('the popup is gone before the crop dialog opens', () => {
        component.open({ currentTarget: button() });
        expect(component.isOpen).toBe(true);

        component.openCrop();
        // Two aria-modal dialogs at once is a defect whichever way it paints:
        // the Alpine one arms its own x-trap, and the native one makes the rest
        // of the page inert. The popup has to be on its way out first.
        expect(component.isOpen).toBe(false);

        // And not merely flagged as closing: x-trap is still armed until Alpine
        // has torn the subtree down, and showModal() over an armed trap pulls
        // focus straight back into the popup behind it.
        expect(cropModal.showModal).not.toHaveBeenCalled();
        flushTicks();
        expect(cropModal.showModal).toHaveBeenCalledTimes(1);
    });

    test('focus is on the opener before showModal, so the crop dialog can hand it back', () => {
        const opener = button();
        component.open({ currentTarget: opener });

        component.openCrop();
        flushTicks();

        // `<dialog>` restores focus on close to whatever was focused when it
        // opened. The Crop… button is inside the popup that has just been
        // removed, so without this the reader would be dropped on <body> for
        // walking away from a crop.
        expect(opener.focus).toHaveBeenCalled();
    });

    test('a hand-off does not restore focus itself', () => {
        const opener = button();
        component.open({ currentTarget: opener });

        component.openCrop();
        flushTicks();

        // Exactly once, by openCrop. Two restores would fight over focus as the
        // crop dialog mounts.
        expect(opener.focus).toHaveBeenCalledTimes(1);
    });

    test('a missing crop dialog is not an error', () => {
        (globalThis as any).document.getElementById = () => null;
        component.open({ currentTarget: button() });

        component.openCrop();
        flushTicks();

        expect(component.isOpen).toBe(false);
    });
});

describe('the shared popup rules, through the other component', () => {
    test('customThumbnail opens, closes and restores focus the same way', () => {
        const thumb: any = customThumbnail({ resourceId: 7 });
        thumb.$nextTick = (fn: () => void) => pendingTicks.push(fn);
        thumb.$root = { querySelector: () => null, contains: () => false };
        const opener = button();

        thumb.open({ currentTarget: opener });
        expect(thumb.isOpen).toBe(true);
        thumb.close();
        flushTicks();

        expect(thumb.isOpen).toBe(false);
        expect(opener.focus).toHaveBeenCalled();
    });
});
