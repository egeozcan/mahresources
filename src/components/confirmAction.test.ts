import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import { confirmAction } from './confirmAction.js';
import { selectorRegistry } from '../selector/selectorRegistry';

/**
 * WS14 findings 78 and 153.
 *
 * The plan says the bulk toolbars "reuse the generic delete confirm". They do
 * not: all four author their own message, and confirmAction threw every one of
 * them away, because it destructures its argument and destructuring a string
 * gives `undefined` for every named property. So `confirmAction('Are you sure
 * you want to delete the selected notes?')` produced the default, and the same
 * count-less "Are you sure you want to delete?" appeared for one selected row
 * and for a full Select All.
 */

type Mounted = ReturnType<typeof confirmAction> & { $el: HTMLElement };

/** A form-shaped stand-in; only closest() and the submit event matter here. */
function mountOn(component: ReturnType<typeof confirmAction>, form: HTMLFormElement): Mounted {
    const mounted = component as Mounted;
    mounted.$el = form as unknown as HTMLElement;
    mounted.init();
    return mounted;
}

function fakeForm(dataset: Record<string, string> = {}): HTMLFormElement {
    const form = {
        tagName: 'FORM',
        dataset,
        requestSubmit: vi.fn(),
        closest: (sel: string) => (sel === 'form' ? form : null),
    };
    return form as unknown as HTMLFormElement;
}

function submitEvent(target: HTMLFormElement) {
    return {
        target,
        defaultPrevented: false,
        preventDefault() { (this as { defaultPrevented: boolean }).defaultPrevented = true; },
    };
}

let currentSelectionSize = 0;

// Module scope, because installAlpine closes over it and is defined outside the
// describe block.
let confirmSpy: ReturnType<typeof vi.fn>;

/**
 * Install the two stores the component reads: bulkSelection for {count}/{s}, and
 * confirmDialog for the confirmation itself. Stubbed through vi so afterEach's
 * unstubAllGlobals restores them — a direct assignment to globalThis.window would
 * outlive this file.
 */
function installAlpine(size: number) {
    currentSelectionSize = size;
    vi.stubGlobal('Alpine', {
        store: (name: string) => {
            if (name === 'bulkSelection') {
                return { selectedIds: new Set(Array.from({ length: currentSelectionSize }, (_, i) => i + 1)) };
            }
            if (name === 'confirmDialog') {
                return { ask: (message: string) => confirmSpy(message) };
            }
            return undefined;
        },
    });
    vi.stubGlobal('window', globalThis);
}

function setSelectionSize(size: number) {
    currentSelectionSize = size;
}

describe('confirmAction message handling', () => {
    /**
     * The confirmation is asked of the in-app alertdialog store now, not of
     * window.confirm, and it answers with a promise. setSelectionSize re-stubs
     * Alpine, so the confirmDialog store is installed from here too — otherwise a
     * mid-test call to setSelectionSize would silently drop it and askToConfirm
     * would fail closed.
     */
    beforeEach(() => {
        confirmSpy = vi.fn(async () => true);
        installAlpine(0);
        vi.stubGlobal('document', { addEventListener: vi.fn(), removeEventListener: vi.fn() });
    });

    afterEach(() => {
        vi.restoreAllMocks();
        vi.unstubAllGlobals();
    });

    test('a message passed as a bare string is used, not silently dropped', async () => {
        const form = fakeForm();
        const c = mountOn(confirmAction('Delete the selected notes?'), form);
        await c.events['@submit'].call(c, submitEvent(form));
        expect(confirmSpy).toHaveBeenCalledWith('Delete the selected notes?');
    });

    test('a message passed in an options object is still used', async () => {
        const form = fakeForm();
        const c = mountOn(confirmAction({ message: 'Clone this group and all its associations?' }), form);
        await c.events['@submit'].call(c, submitEvent(form));
        expect(confirmSpy).toHaveBeenCalledWith('Clone this group and all its associations?');
    });

    test('no argument keeps the historic default', async () => {
        const form = fakeForm();
        const c = mountOn(confirmAction(), form);
        await c.events['@submit'].call(c, submitEvent(form));
        expect(confirmSpy).toHaveBeenCalledWith('Are you sure you want to delete?');
    });

    test('{count} and {s} report the live bulk selection, pluralised', async () => {
        const form = fakeForm();
        const c = mountOn(confirmAction('Delete {count} resource{s}? This cannot be undone.'), form);

        setSelectionSize(1);
        await c.events['@submit'].call(c, submitEvent(form));
        expect(confirmSpy).toHaveBeenLastCalledWith('Delete 1 resource? This cannot be undone.');

        setSelectionSize(4);
        await c.events['@submit'].call(c, submitEvent(form));
        expect(confirmSpy).toHaveBeenLastCalledWith('Delete 4 resources? This cannot be undone.');
    });

    test('{winner} names the item the merge form is merging into', async () => {
        const form = fakeForm();
        const unregister = selectorRegistry.register(form, 'winner', {
            getRawValues: () => [{ ID: 7, Name: 'design' }],
            replaceRawValues: () => {},
            replaceByKeys: () => {},
            resolveExactLabels: async () => true,
        });
        setSelectionSize(3);

        const c = mountOn(confirmAction('Merge {count} tag{s} into {winner}? The merged tag{s} will be deleted.'), form);
        await c.events['@submit'].call(c, submitEvent(form));
        expect(confirmSpy).toHaveBeenCalledWith('Merge 3 tags into design? The merged tags will be deleted.');
        unregister();
    });

    test('a message with no placeholders is passed through untouched', async () => {
        const form = fakeForm();
        setSelectionSize(9);
        const c = mountOn(confirmAction('All the similar resources will be deleted. Are you sure?'), form);
        await c.events['@submit'].call(c, submitEvent(form));
        expect(confirmSpy).toHaveBeenCalledWith('All the similar resources will be deleted. Are you sure?');
    });

    test('dismissing the confirm prevents the submit and re-submits nothing', async () => {
        confirmSpy.mockResolvedValue(false);
        const form = fakeForm();
        const c = mountOn(confirmAction('Delete {count} tag{s}?'), form);
        const event = submitEvent(form);
        await c.events['@submit'].call(c, event);

        expect(event.defaultPrevented).toBe(true);
        // The assertion that matters: dismissal left the world unchanged. A
        // dismissed confirm that still performed the action is the exact defect
        // this whole mechanism was rebuilt around.
        expect(form.requestSubmit).not.toHaveBeenCalled();
    });

    test('accepting re-submits the form, because the answer arrives too late for this event', async () => {
        const form = fakeForm();
        const c = mountOn(confirmAction('Delete {count} tag{s}?'), form);
        const event = submitEvent(form);
        await c.events['@submit'].call(c, event);

        // This event is always prevented — the answer cannot arrive synchronously,
        // and a submit left to proceed while we wait is one that happened without
        // a confirmation. The confirmed submit is a fresh event instead.
        expect(event.defaultPrevented).toBe(true);
        expect(form.requestSubmit).toHaveBeenCalledTimes(1);
    });

    test('the re-submit passes through WITHOUT preventDefault, or every AJAX bulk op is a no-op', async () => {
        const form = fakeForm();
        const c = mountOn(confirmAction('Delete {count} tag{s}?'), form);

        // bulkSelection.js listens for submit on the toolbar container and reads
        // event.defaultPrevented synchronously to decide whether to fetch. If the
        // second pass prevented its event too, the delegated listener would bail
        // and the confirmed operation would silently do nothing.
        await c.events['@submit'].call(c, submitEvent(form));
        expect(form.requestSubmit).toHaveBeenCalledTimes(1);

        const reSubmit = submitEvent(form);
        (c as unknown as { _confirmed: boolean })._confirmed = true;
        await c.events['@submit'].call(c, reSubmit);
        expect(reSubmit.defaultPrevented).toBe(false);
    });

    test('a message in data-confirm-message is used, and is never parsed as JS', async () => {
        // Pongo2 autoescapes, and the HTML parser decodes the entity before any JS
        // in the attribute is parsed — so a name containing an apostrophe would
        // terminate the string literal in an x-data expression. Read via dataset it
        // is only ever a DOM string.
        const form = fakeForm({ confirmMessage: "Delete user o'brien?" });
        const c = mountOn(confirmAction(), form);
        await c.events['@submit'].call(c, submitEvent(form));
        expect(confirmSpy).toHaveBeenCalledWith("Delete user o'brien?");
    });

    test('an explicit message wins over data-confirm-message', async () => {
        const form = fakeForm({ confirmMessage: 'from the attribute' });
        const c = mountOn(confirmAction('from the argument'), form);
        await c.events['@submit'].call(c, submitEvent(form));
        expect(confirmSpy).toHaveBeenCalledWith('from the argument');
    });
});
