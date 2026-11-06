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
