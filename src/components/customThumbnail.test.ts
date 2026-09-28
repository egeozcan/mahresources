import { describe, expect, test, vi, beforeEach, afterEach } from 'vitest';
import { customThumbnail } from './customThumbnail.js';

/**
 * The Custom Thumbnail sidebar group became a button that opens a popup.
 *
 * Two properties came with it, and only one of them was the visible change.
 *
 * The popup: open() records what had focus and close() gives it back, because
 * `x-trap`'s own restore points at whatever was focused when the trap armed — for
 * a sidebar button that is still on screen, so it happens to work, but the
 * component owns it the way pluginActionModal and massEditModal do rather than
 * relying on that.
 *
 * The paste: `@paste.window` used to be live on the whole page, so an image pasted
 * anywhere on the resource detail page replaced the thumbnail with no visible
 * affordance. It now only applies while the popup is open — and it listens in the
 * CAPTURE phase and stops the event, because the global `setupPasteListener()`
 * shares `window` and would otherwise handle the same paste a second time. Its
 * guard 2 ("a file input is on the page and the clipboard has files") merges the
 * clipboard into whichever input that is — ours — and dispatches `change`, which
 * the component's own `@change` turns into a second upload. Ordering decides who
 * wins today (Alpine binds `@paste.window` inside `Alpine.start()`, which runs
 * before `setupPasteListener()`), which is not a reason to leave the double
 * upload in place.
 */

let component: any;
let pendingTicks: Array<() => void>;
let uploads: Array<{ url: string; method: string }>;

function flushTicks() {
    const queued = pendingTicks;
    pendingTicks = [];
    queued.forEach((fn) => fn());
}

function imageItem(file: any) {
    return { kind: 'file', type: 'image/png', getAsFile: () => file };
}

function pasteEvent(items: any[]) {
    return {
        clipboardData: { items },
        preventDefault: vi.fn(),
        stopImmediatePropagation: vi.fn(),
    };
}

beforeEach(() => {
    pendingTicks = [];
    uploads = [];
    component = customThumbnail({ resourceId: 7 });
    component.$nextTick = (fn: () => void) => pendingTicks.push(fn);
    component.$root = { querySelector: () => null, contains: () => false };
    vi.stubGlobal('fetch', vi.fn(async (url: string, init: any) => {
        uploads.push({ url, method: init.method });
        return { ok: true, status: 204, text: async () => '' };
    }));
    vi.stubGlobal('document', {
        activeElement: null,
        body: {},
        documentElement: {},
        querySelectorAll: () => [],
    });
});

afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
});

describe('the popup', () => {
    test('is closed until the button is pressed', () => {
        expect(component.isOpen).toBe(false);
    });

    test('open() records whatever control had focus', () => {
        const opener = { tagName: 'BUTTON' };
        (globalThis as any).document.activeElement = opener;

        component.open();

        expect(component.isOpen).toBe(true);
        expect(component._opener).toBe(opener);
    });

    test('focus nowhere records nothing rather than <body>', () => {
        const body = { tagName: 'BODY' };
        (globalThis as any).document = { activeElement: body, body, documentElement: {}, querySelectorAll: () => [] };

        component.open();

        expect(component._opener).toBeNull();
    });

    test('close() gives focus back to the opener, a tick later', () => {
        const opener = { tagName: "BUTTON", isConnected: true, matches: () => true, focus: vi.fn() };
        (globalThis as any).document.activeElement = opener;
        component.open();

        component.close();
        // Still armed: x-if has not torn the subtree down yet, so a synchronous
        // restore would pull focus straight back into the dialog.
        expect(opener.focus).not.toHaveBeenCalled();
        flushTicks();

        expect(component.isOpen).toBe(false);
        expect(opener.focus).toHaveBeenCalled();
    });

    test('reopening clears the previous outcome rather than showing it', () => {
        component.statusMessage = 'Custom thumbnail saved.';
        component.errorMessage = 'nope';

        component.open();

        expect(component.statusMessage).toBe('');
        expect(component.errorMessage).toBe('');
    });

    test('declines to open on top of another dialog', () => {
        const other = { id: 'other', nodeType: 1, isConnected: true, checkVisibility: () => true };
        (globalThis as any).document = {
            activeElement: null,
            body: {},
            documentElement: {},
            querySelectorAll: () => [other],
        };

        component.open();

        // Two aria-modal dialogs at once is a defect whichever way it paints:
        // each arms its own x-trap and the reader is held by one while looking at
        // the other. That is what src/utils/modality.js exists to stop.
        expect(component.isOpen).toBe(false);
    });
});

describe('pasting an image', () => {
    test('is ignored while the popup is closed', async () => {
        const file = new File(['x'], 'shot.png', { type: 'image/png' });
        const event = pasteEvent([imageItem(file)]);

        await component.onPaste(event);
        await flush();

        expect(uploads).toEqual([]);
        // Not preventDefault()ed either: with the popup closed this paste is none
        // of our business, and swallowing it would take it away from whatever
        // else on the page handles pastes.
        expect(event.preventDefault).not.toHaveBeenCalled();
    });

    test('uploads the image while the popup is open', async () => {
        component.open();
        const file = new File(['x'], 'shot.png', { type: 'image/png' });
        const event = pasteEvent([imageItem(file)]);

        await component.onPaste(event);
        await flush();

        expect(uploads).toEqual([{ url: '/v1/resource/preview?id=7', method: 'POST' }]);
        expect(event.preventDefault).toHaveBeenCalled();
        expect(component.errorMessage).toBe('');
    });

    test('takes the paste away from the global paste handler', async () => {
        component.open();
        const event = pasteEvent([imageItem(new File(['x'], 'shot.png', { type: 'image/png' }))]);

        await component.onPaste(event);
        await flush();

        // The file input inside the popup makes setupPasteListener's guard 2 fire
        // on the same paste: it merges the clipboard into that input and
        // dispatches change, which uploads the same bytes a second time.
        expect(event.stopImmediatePropagation).toHaveBeenCalled();
    });

    test('leaves a clipboard with no image to the global paste handler', async () => {
        component.open();
        const event = pasteEvent([{ kind: 'string', type: 'text/plain', getAsFile: () => null }]);

        await component.onPaste(event);
        await flush();

        expect(uploads).toEqual([]);
        expect(event.stopImmediatePropagation).not.toHaveBeenCalled();
    });
});

async function flush() {
    await Promise.resolve();
    await Promise.resolve();
}
