// @vitest-environment happy-dom

import { describe, expect, test, vi } from 'vitest';
// The bundle's user-settings module loads from the server on import once a
// document exists; answer it before the import runs.
vi.hoisted(() => {
    globalThis.fetch = (async () => ({ ok: false, status: 401, json: async () => ({}) })) as unknown as typeof fetch;
});

import { linesToHunks } from './slotHistory.js';
import { unifiedRows } from './templateRevisions.js';

describe('linesToHunks', () => {
    test('splits a changed line into a delete and an insert, keeping the rest equal', () => {
        expect(linesToHunks('<p>a</p>\n<p>one</p>\n<p>z</p>\n', '<p>a</p>\n<p>two</p>\n<p>z</p>\n')).toEqual([
            { op: 'equal', lines: ['<p>a</p>'] },
            { op: 'delete', lines: ['<p>one</p>'] },
            { op: 'insert', lines: ['<p>two</p>'] },
            { op: 'equal', lines: ['<p>z</p>'] },
        ]);
    });

    test('texts without a final newline diff the same way', () => {
        expect(linesToHunks('<p>one</p>', '<p>two</p>')).toEqual([
            { op: 'delete', lines: ['<p>one</p>'] },
            { op: 'insert', lines: ['<p>two</p>'] },
        ]);
    });

    test('an empty side is all inserts, and feeds the History page row builder', () => {
        const hunks = linesToHunks('', 'a\nb');
        expect(hunks).toEqual([{ op: 'insert', lines: ['a', 'b'] }]);
        expect(unifiedRows(hunks).map((row) => row.op)).toEqual(['insert', 'insert']);
    });
});

describe('slot history freshness', () => {
    test("a closed dialog's late error body does not land in the next dialog", async () => {
        const { slotHistory } = await import('./slotHistory.js');
        let releaseBody: () => void = () => {};
        const bodyHeld = new Promise<void>((resolve) => {
            releaseBody = resolve;
        });
        let calls = 0;
        vi.stubGlobal(
            'fetch',
            vi.fn(async () => {
                calls++;
                if (calls === 1) {
                    return {
                        ok: false,
                        status: 500,
                        json: async () => {
                            await bodyHeld;
                            return { error: 'STALE HEADER ERROR' };
                        },
                    };
                }
                return new Promise(() => {}); // the second list request stays pending
            }),
        );
        const component = slotHistory({ apiPrefix: '/v1/category', carrierId: 4 });
        component._form = document.createElement('form');

        const first = component.open({ slot: 'CustomHeader', label: 'Custom Header' });
        await vi.waitFor(() => expect(calls).toBe(1));
        component.close({ restore: false });
        void component.open({ slot: 'CustomSidebar', label: 'Custom Sidebar' });
        await vi.waitFor(() => expect(calls).toBe(2));
        releaseBody();
        await first;

        expect(component.slot).toBe('CustomSidebar');
        expect(component.error).toBe('');
        expect(component.loading).toBe(true);
        vi.unstubAllGlobals();
    });
});

describe('slot history diff follows the editor', () => {
    test('an editor change while the dialog is open recomputes the diff', async () => {
        const { slotHistory } = await import('./slotHistory.js');
        const form = document.createElement('form');
        form.innerHTML = '<input name="CustomHeader" value="<p>before</p>"><input name="CustomHeaderCSS" value="">';
        const component = slotHistory({ apiPrefix: '/v1/category', carrierId: 4 });
        component._form = form;
        component.$root = form as unknown as HTMLElement;
        component.$watch = () => {};
        component.init();
        component.isOpen = true;
        component.slot = 'CustomHeader';
        component.detail = Object.freeze({ session: component._session, slot: 'CustomHeader', number: 1, editor: { CustomHeader: '<p>old</p>', CustomHeaderCSS: '' } });
        component.computeDiff();
        expect(component.diffFields[0].unified.some((r: { text?: string }) => r.text === '<p>before</p>')).toBe(true);

        // A formatter finishing late rewrites the editor under the open dialog.
        (form.querySelector('input[name="CustomHeader"]') as HTMLInputElement).value = '<p>formatted</p>';
        form.dispatchEvent(new CustomEvent('template-slot-changed', { bubbles: true, detail: { name: 'CustomHeader' } }));

        const texts = component.diffFields[0].unified.map((r: { text?: string }) => r.text);
        expect(texts).toContain('<p>formatted</p>');
        expect(texts).not.toContain('<p>before</p>');
    });
});
