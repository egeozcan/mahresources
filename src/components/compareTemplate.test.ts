import { describe, it, expect } from 'vitest';
import { readFileSync } from 'fs';

const compareTpl = readFileSync(
    new URL('../../templates/compare.tpl', import.meta.url),
    'utf8',
);

describe('compare page template', () => {
    // An attribute is HTML-decoded before Alpine parses what is left, so pongo2's
    // autoescape does not make a value safe inside an event expression: `&#39;`
    // becomes `'` and closes the string. A resource version's hash is not a hash
    // by construction — group import writes it straight from the manifest — so
    // the copy control reads the rendered element instead of carrying the value.
    it('never interpolates a server value into an Alpine event expression', () => {
        const handlers = compareTpl.match(/@[a-z]+(\.[a-z-]+)*="[^"]*"/g) || [];
        expect(handlers.length).toBeGreaterThan(0);

        const carrying = handlers.filter((h) => /\{\{/.test(h));
        expect(carrying).toEqual([]);
    });

    it('copies the hash out of the element that renders it', () => {
        expect(compareTpl).toContain('<code x-ref="hash">');
        expect(compareTpl).toContain('copyText($refs.hash.textContent.trim())');
    });

    // The confirmation text is a JS string literal, which is the one place a
    // server value legitimately reaches script — and the only escaping that
    // survives the attribute decode is escapejs.
    it('escapes the names in the merge confirmations', () => {
        const confirms = compareTpl.match(/confirmAction\(\{[\s\S]*?\}\)"/g) || [];
        expect(confirms).toHaveLength(2);
        for (const confirm of confirms) {
            const interpolations = confirm.match(/\{\{[^}]*\}\}/g) || [];
            expect(interpolations.length).toBeGreaterThan(0);
            for (const value of interpolations) {
                expect(value).toMatch(/\|escapejs\s*\}\}/);
            }
        }
    });
});
