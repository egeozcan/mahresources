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

function fakeForm(): HTMLFormElement {
    const form = {
        tagName: 'FORM',
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

function setSelectionSize(size: number) {
    // The component reads window.Alpine, which is what the app sets. Stub both
    // through vi so afterEach's unstubAllGlobals restores them — a direct
    // assignment to globalThis.window would outlive this file.
    vi.stubGlobal('Alpine', {
        store: (name: string) => (name === 'bulkSelection'
            ? { selectedIds: new Set(Array.from({ length: size }, (_, i) => i + 1)) }
            : undefined),
    });
    vi.stubGlobal('window', globalThis);
}

describe('confirmAction message handling', () => {
    let confirmSpy: ReturnType<typeof vi.fn>;

    beforeEach(() => {
        confirmSpy = vi.fn(() => true);
        vi.stubGlobal('confirm', confirmSpy);
        vi.stubGlobal('document', { addEventListener: vi.fn(), removeEventListener: vi.fn() });
        setSelectionSize(0);
    });

    afterEach(() => {
        vi.restoreAllMocks();
        vi.unstubAllGlobals();
    });

    test('a message passed as a bare string is used, not silently dropped', () => {
        const form = fakeForm();
        const c = mountOn(confirmAction('Delete the selected notes?'), form);
        c.events['@submit'].call(c, submitEvent(form));
        expect(confirmSpy).toHaveBeenCalledWith('Delete the selected notes?');
    });

    test('a message passed in an options object is still used', () => {
        const form = fakeForm();
        const c = mountOn(confirmAction({ message: 'Clone this group and all its associations?' }), form);
        c.events['@submit'].call(c, submitEvent(form));
        expect(confirmSpy).toHaveBeenCalledWith('Clone this group and all its associations?');
    });

    test('no argument keeps the historic default', () => {
        const form = fakeForm();
        const c = mountOn(confirmAction(), form);
        c.events['@submit'].call(c, submitEvent(form));
        expect(confirmSpy).toHaveBeenCalledWith('Are you sure you want to delete?');
    });

    test('{count} and {s} report the live bulk selection, pluralised', () => {
        const form = fakeForm();
        const c = mountOn(confirmAction('Delete {count} resource{s}? This cannot be undone.'), form);

        setSelectionSize(1);
        c.events['@submit'].call(c, submitEvent(form));
        expect(confirmSpy).toHaveBeenLastCalledWith('Delete 1 resource? This cannot be undone.');

        setSelectionSize(4);
        c.events['@submit'].call(c, submitEvent(form));
        expect(confirmSpy).toHaveBeenLastCalledWith('Delete 4 resources? This cannot be undone.');
    });

    test('{winner} names the item the merge form is merging into', () => {
        const form = fakeForm();
        const unregister = selectorRegistry.register(form, 'winner', {
            getRawValues: () => [{ ID: 7, Name: 'design' }],
            replaceRawValues: () => {},
            replaceByKeys: () => {},
            resolveExactLabels: async () => true,
        });
        setSelectionSize(3);

        const c = mountOn(confirmAction('Merge {count} tag{s} into {winner}? The merged tag{s} will be deleted.'), form);
        c.events['@submit'].call(c, submitEvent(form));
        expect(confirmSpy).toHaveBeenCalledWith('Merge 3 tags into design? The merged tags will be deleted.');
        unregister();
    });

    test('a message with no placeholders is passed through untouched', () => {
        const form = fakeForm();
        setSelectionSize(9);
        const c = mountOn(confirmAction('All the similar resources will be deleted. Are you sure?'), form);
        c.events['@submit'].call(c, submitEvent(form));
        expect(confirmSpy).toHaveBeenCalledWith('All the similar resources will be deleted. Are you sure?');
    });

    test('dismissing the confirm prevents the submit', () => {
        confirmSpy.mockReturnValue(false);
        const form = fakeForm();
        const c = mountOn(confirmAction('Delete {count} tag{s}?'), form);
        const event = submitEvent(form);
        c.events['@submit'].call(c, event);
        expect(event.defaultPrevented).toBe(true);
    });

    test('accepting the confirm leaves the submit alone', () => {
        const form = fakeForm();
        const c = mountOn(confirmAction('Delete {count} tag{s}?'), form);
        const event = submitEvent(form);
        c.events['@submit'].call(c, event);
        expect(event.defaultPrevented).toBe(false);
    });
});
