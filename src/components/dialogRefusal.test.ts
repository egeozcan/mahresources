// @vitest-environment happy-dom

import { afterEach, describe, expect, test, vi } from 'vitest';
import { refuseOverModal } from '../utils/modality.js';
import { jobPanel } from './jobPanel.js';
import { globalSearch } from './globalSearch.js';

vi.mock('../userSettings.js', () => ({ get: () => undefined, saveNow: async () => false, whenLoaded: async () => {} }));

afterEach(() => {
    document.body.innerHTML = '';
    vi.unstubAllGlobals();
});

// A painted aria-modal dialog the reader is inside. happy-dom lays nothing out,
// so it is told it is painted.
function openDialog(label: string) {
    document.body.insertAdjacentHTML('beforeend', `<div role="dialog" aria-modal="true" aria-label="${label}"><input></div>`);
    const dialog = document.body.lastElementChild as HTMLElement;
    (dialog as any).checkVisibility = () => true;
    return dialog;
}

const settle = () => new Promise(resolve => setTimeout(resolve, 80));

describe('a shortcut refused over another dialog', () => {
    test('is said and shown inside the dialog the reader is in', async () => {
        const dialog = openDialog('Search');
        refuseOverModal(dialog, 'Close this dialog first to open Jobs.');
        const notice = dialog.querySelector('[data-modal-refusal]') as HTMLElement;
        expect(notice.getAttribute('role')).toBe('status');
        await settle();
        expect(notice.textContent).toBe('Close this dialog first to open Jobs.');

        // Asked again, it is emptied first, so the same words are heard again,
        // and one notice serves.
        refuseOverModal(dialog, 'Close this dialog first to open Jobs.');
        expect(notice.textContent).toBe('');
        await settle();
        expect(notice.textContent).toBe('Close this dialog first to open Jobs.');
        expect(dialog.querySelectorAll('[data-modal-refusal]').length).toBe(1);
    });

    test('leaves once it has been read, so a dialog that is hidden and shown again does not show it', async () => {
        vi.useFakeTimers();
        try {
            const dialog = openDialog('Picker');
            refuseOverModal(dialog, 'Close this dialog first to open Jobs.');
            vi.advanceTimersByTime(50);
            expect(dialog.querySelector('[data-modal-refusal]')?.textContent).toBe('Close this dialog first to open Jobs.');
            vi.advanceTimersByTime(9_000);
            // Asked again while shown, it stays for the full time from then.
            refuseOverModal(dialog, 'Close this dialog first to open Jobs.');
            vi.advanceTimersByTime(50 + 9_000);
            expect(dialog.querySelector('[data-modal-refusal]')).not.toBeNull();
            vi.advanceTimersByTime(1_000);
            expect(dialog.querySelector('[data-modal-refusal]')).toBeNull();
        } finally {
            vi.useRealTimers();
        }
    });

    test('the Jobs shortcut inside search says why inside search', async () => {
        const dialog = openDialog('Search');
        const panel = jobPanel() as any;
        panel._liveRegion = { announce: vi.fn(), cancel: vi.fn(), pending: () => false, destroy: vi.fn() };
        panel.toggle();
        expect(panel.isOpen).toBe(false);
        await settle();
        expect(dialog.querySelector('[data-modal-refusal]')?.textContent).toBe('Close this dialog first to open Jobs.');
        expect(panel._liveRegion.announce).not.toHaveBeenCalled();
    });

    test('the search shortcut inside the Jobs drawer says why inside the drawer', async () => {
        const dialog = openDialog('Jobs');
        const search = globalSearch() as any;
        search._liveRegion = { announce: vi.fn(), destroy: vi.fn() };
        search.toggle();
        expect(search.isOpen).toBe(false);
        await settle();
        expect(dialog.querySelector('[data-modal-refusal]')?.textContent).toBe('Close this dialog first to open search.');
        expect(search._liveRegion.announce).not.toHaveBeenCalled();
    });
});
