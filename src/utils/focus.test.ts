import { afterEach, describe, expect, test, vi } from 'vitest';
import { focusedElement } from './focus.js';

/**
 * Review remediation finding 4: a dialog opened by a keyboard shortcut has no
 * trigger event to capture, so the only honest answer to "where should focus go
 * back to?" is wherever it was. `document.activeElement` alone is not that answer —
 * it is `<body>` when focus is nowhere, and restoring to `<body>` succeeds (focusOn
 * borrows a tabindex) while leaving the reader exactly where losing focus would.
 */
describe('focusedElement', () => {
    afterEach(() => {
        vi.unstubAllGlobals();
    });

    test('reports the element that has focus', () => {
        const input = { tagName: 'INPUT' };
        vi.stubGlobal('document', { activeElement: input, body: {}, documentElement: {} });
        expect(focusedElement()).toBe(input);
    });

    test('reports nothing when focus is on <body>, so callers can fall back', () => {
        const body = { tagName: 'BODY' };
        vi.stubGlobal('document', { activeElement: body, body, documentElement: {} });
        expect(focusedElement()).toBeNull();
    });

    test('reports nothing for <html> or for no active element at all', () => {
        const documentElement = { tagName: 'HTML' };
        vi.stubGlobal('document', { activeElement: documentElement, body: {}, documentElement });
        expect(focusedElement()).toBeNull();

        vi.stubGlobal('document', { activeElement: null, body: {}, documentElement: {} });
        expect(focusedElement()).toBeNull();
    });
});
