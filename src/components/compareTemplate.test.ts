import { describe, it, expect } from 'vitest';
import { readFileSync } from 'fs';

const compareTpl = readFileSync(
    new URL('../../templates/compare.tpl', import.meta.url),
    'utf8',
);

const compareTpls = ['compareImage', 'compareText', 'compareInlineText', 'compareBinary', 'comparePdf'].map(
    (name) => readFileSync(new URL(`../../templates/partials/${name}.tpl`, import.meta.url), 'utf8'),
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

    // Every server value that reaches script goes through `json`, which emits a
    // complete quoted literal. `escapejs` does not: it writes a bare `\u1F600`
    // for an astral character, which JavaScript reads as `\u1F60` plus a stray
    // "0", so an emoji in a resource name comes out mangled.
    it('passes server values into script through the json filter', () => {
        const scripted = [
            ...(compareTpl.match(/x-data="[\s\S]*?"/g) || []),
            ...compareTpls.flatMap((tpl) => tpl.match(/x-data="[\s\S]*?"/g) || []),
        ];
        expect(scripted.length).toBeGreaterThan(0);

        for (const block of scripted) {
            for (const value of block.match(/\{\{[^{}]*\}\}/g) || []) {
                expect(value, `${value} must go through |json`).toMatch(/\|json\s*\}\}/);
            }
        }
    });

    it('carries both merge confirmations as prebuilt sentences', () => {
        expect(compareTpl).toContain('confirmAction({ message: {{ mergeConfirm1|json }} })');
        expect(compareTpl).toContain('confirmAction({ message: {{ mergeConfirm2|json }} })');
    });
});
