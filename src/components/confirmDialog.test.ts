import { describe, expect, test, vi, beforeEach, afterEach } from 'vitest';
import { registerConfirmDialogStore, askToConfirm } from './confirmDialog.js';

/**
 * The property that matters is not "the dialog appeared".
 *
 * The 2026-07-29 UI bug hunt found a live case where a *dismissed* confirm still
 * performed the action: the AJAX bulk-submit listener ran before the form's own
 * handler and called preventDefault unconditionally, so the merge happened after
 * the reader clicked Cancel. So every test here asserts the answer that reaches
 * the caller, and the fail-closed cases assert that the answer is `false` — a bug
 * in this dialog must mean nothing happens, never that something is destroyed
 * without being confirmed.
 */

let store;

function registerStore() {
    let registered;
    registerConfirmDialogStore({ store: (_name, value) => { registered = value; } });
    return registered;
}

/**
 * The suite runs without a DOM, so `document` is stubbed the way the other
 * component tests here do it. `body.children` is what the inert walk reads.
 */
function stubDocument(bodyChildren = []) {
    const body = { children: bodyChildren };
    vi.stubGlobal('document', {
        activeElement: body,
        body,
        documentElement: {},
    });
    return body;
}

/** A stand-in for a top-level element the inert walk can reach. */
function el({ children = [], inert = false } = {}) {
    return { children, inert, isConnected: true, parentElement: null };
}

beforeEach(() => {
    store = registerStore();
    stubDocument();
});

afterEach(() => {
    vi.unstubAllGlobals();
});

describe('the answer that reaches the caller', () => {
    test('accept resolves true', async () => {
        const answer = store.ask('Delete 3 resources?');
        store.accept();
        expect(await answer).toBe(true);
    });

    test('cancel resolves false', async () => {
        const answer = store.ask('Delete 3 resources?');
        store.cancel();
        expect(await answer).toBe(false);
    });

    test('a second ask while one is open resolves false rather than replacing it', async () => {
        const first = store.ask('Delete the group?');
        const second = store.ask('Delete everything?');

        expect(await second).toBe(false);
        // The first is still the one on screen, and still answerable.
        expect(store.message).toBe('Delete the group?');
        store.accept();
        expect(await first).toBe(true);
    });

    test('settling twice does not resolve twice', async () => {
        const answer = store.ask('Delete?');
        store.accept();
        store.cancel();
        expect(await answer).toBe(true);
        expect(store.isOpen).toBe(false);
    });
});

describe('askToConfirm fails closed', () => {
    test('resolves false when the store is not registered', async () => {
        vi.stubGlobal('Alpine', { store: () => undefined });
        const error = vi.spyOn(console, 'error').mockImplementation(() => {});

        expect(await askToConfirm('Delete everything?')).toBe(false);
        expect(error).toHaveBeenCalled();

        error.mockRestore();
    });

    test('delegates to the store when it is registered', async () => {
        vi.stubGlobal('Alpine', { store: (name) => (name === 'confirmDialog' ? store : undefined) });

        const answer = askToConfirm('Delete the note?');
        expect(store.message).toBe('Delete the note?');
        store.accept();
        expect(await answer).toBe(true);
    });
});

describe('the page beneath is made genuinely inert', () => {
    test('every body child except the dialog ancestor chain goes inert', () => {
        const header = el();
        const main = el();
        const dialogRoot = el();
        const overlays = el({ children: [dialogRoot] });
        stubDocument([header, main, overlays]);
        dialogRoot.parentElement = overlays;

        store._applyInert(dialogRoot);

        expect(header.inert).toBe(true);
        expect(main.inert).toBe(true);
        // The ancestor itself must stay interactive or the dialog inside it dies.
        expect(overlays.inert).toBe(false);
        expect(dialogRoot.inert).toBe(false);
    });

    test('a sibling overlay goes inert too, so a confirm opened from a modal is truly on top', () => {
        const dialogRoot = el();
        const blockEditorModal = el();
        const overlays = el({ children: [blockEditorModal, dialogRoot] });
        stubDocument([overlays]);
        dialogRoot.parentElement = overlays;

        store._applyInert(dialogRoot);

        expect(blockEditorModal.inert).toBe(true);
        expect(dialogRoot.inert).toBe(false);
    });

    test('releasing restores only what this dialog set', () => {
        const alreadyInert = el({ inert: true });
        const main = el();
        const dialogRoot = el();
        const overlays = el({ children: [dialogRoot] });
        stubDocument([alreadyInert, main, overlays]);
        dialogRoot.parentElement = overlays;

        store._applyInert(dialogRoot);
        store._releaseInert();

        expect(main.inert).toBe(false);
        // Someone else owns this one; clearing it would be clobbering their state.
        expect(alreadyInert.inert).toBe(true);
    });

    test('cancelling releases the inert it applied', () => {
        const main = el();
        const dialogRoot = el();
        const overlays = el({ children: [dialogRoot] });
        stubDocument([main, overlays]);
        dialogRoot.parentElement = overlays;

        store.ask('Delete?');
        store._applyInert(dialogRoot);
        expect(main.inert).toBe(true);

        store.cancel();
        expect(main.inert).toBe(false);
    });
});

describe('focus returns to the control that opened the dialog', () => {
    test('the opener is focused again on settle', async () => {
        const opener = { isConnected: true, focus: vi.fn(), matches: () => true };
        const body = { children: [] };
        vi.stubGlobal('document', {
            activeElement: opener,
            body,
            documentElement: {},
        });

        const answer = store.ask('Delete?');
        store.accept();
        await answer;
        // The restore is deliberately deferred to a macrotask: done synchronously,
        // x-trap is still armed and pulls focus back inside, and the reader then
        // lands on <body> when x-if removes the subtree.
        await new Promise((r) => setTimeout(r, 0));

        expect(opener.focus).toHaveBeenCalled();
    });

    test('an opener detached by a re-render is not focused, and nothing throws', async () => {
        const opener = { isConnected: false, focus: vi.fn(), matches: () => true };
        const body = { children: [] };
        vi.stubGlobal('document', {
            activeElement: opener,
            body,
            documentElement: {},
        });

        const answer = store.ask('Delete?');
        store.accept();

        expect(await answer).toBe(true);
        await new Promise((r) => setTimeout(r, 0));
        expect(opener.focus).not.toHaveBeenCalled();
    });
});
