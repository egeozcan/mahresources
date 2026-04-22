/**
 * BH-021: blockEditor's renderMarkdown only recognized **bold**, *italic*,
 * and [link](url). Users expect the common GFM tokens too:
 *   _italic_  → <em>
 *   `code`    → <code>
 *   ~~strike~~ → <s>
 *
 * The blockEditor module is an Alpine data factory — methods live on the
 * returned object, not on the module's exported bindings. Extract the
 * renderMarkdown function by reading the source and evaluating the method
 * body in isolation. Keeps the test framework-free.
 */
import { readFileSync } from 'fs';
import { resolve, dirname } from 'path';
import { fileURLToPath } from 'url';
import { describe, it, expect } from 'vitest';

const __dirname = dirname(fileURLToPath(import.meta.url));
const src = readFileSync(resolve(__dirname, './blockEditor.js'), 'utf-8');

// Match the method from `renderMarkdown(text) {` up to the matching `},` at
// the same indentation level (4 spaces inside the returned object literal).
const match = src.match(/renderMarkdown\(text\)\s*\{[\s\S]*?\n\s{4}\},/);
if (!match) {
  throw new Error('could not extract renderMarkdown from blockEditor.js');
}
const methodSrc = match[0];
// Convert `renderMarkdown(text) { … },` into a standalone function expression.
// Strip the trailing `,` after the closing brace so eval returns a function.
const fnBody = methodSrc.replace('renderMarkdown(text)', 'function renderMarkdown(text)').replace(/,\s*$/, '');
// Wrap in a `return` so `new Function` hands back the renderMarkdown function.
const renderMarkdown = new Function(`${fnBody}\nreturn renderMarkdown;`)() as (t: string) => string;

describe('BH-021: renderMarkdown extended tokens', () => {
  it('renders `_italic_` as <em>', () => {
    expect(renderMarkdown('hello _world_')).toMatch(/hello <em>world<\/em>/);
  });

  it('renders inline `code` as <code>', () => {
    expect(renderMarkdown('call `foo()` please')).toMatch(/call <code>foo\(\)<\/code> please/);
  });

  it('renders `~~strike~~` as <s>', () => {
    expect(renderMarkdown('~~gone~~')).toMatch(/<s>gone<\/s>/);
  });

  it('preserves existing **bold** behavior', () => {
    expect(renderMarkdown('**hi**')).toMatch(/<strong>hi<\/strong>/);
  });

  it('preserves existing *italic* behavior', () => {
    expect(renderMarkdown('*hi*')).toMatch(/<em>hi<\/em>/);
  });

  it('renders [text](url) as anchor', () => {
    expect(renderMarkdown('[mahr](https://example.com)')).toMatch(
      /<a href="https:\/\/example\.com"/
    );
  });

  it('escapes HTML in user text', () => {
    expect(renderMarkdown('<script>alert(1)</script>')).not.toMatch(/<script>/);
  });

  // ─── Word-boundary tests for _italic_ ─────────────────────────────────────

  it('does NOT mangle snake_case_identifiers inside prose', () => {
    // `some_snake_case_name` has underscores *inside* a word — must not become <em>
    const out = renderMarkdown('the variable some_snake_case_name is used');
    expect(out).not.toMatch(/<em>/);
    expect(out).toContain('some_snake_case_name');
  });

  it('only italicizes true _underscore italics_ surrounded by non-word chars', () => {
    expect(renderMarkdown('this is _emphasis_ here')).toMatch(
      /this is <em>emphasis<\/em> here/
    );
  });

  it('protects inline-code content from further inline passes', () => {
    // Backtick code must be processed first; inside <code>, stray *s should not
    // become <strong> / <em>. We emit HTML-escaped asterisks inside code.
    const out = renderMarkdown('`hello *world*`');
    expect(out).toMatch(/<code>hello \*world\*<\/code>/);
    expect(out).not.toMatch(/<strong>|<em>/);
  });

  it('handles a mixed line with all tokens', () => {
    const out = renderMarkdown('**bold** and _italic_ and `code` and ~~strike~~');
    expect(out).toContain('<strong>bold</strong>');
    expect(out).toContain('<em>italic</em>');
    expect(out).toContain('<code>code</code>');
    expect(out).toContain('<s>strike</s>');
  });

  it('returns "" for empty input', () => {
    expect(renderMarkdown('')).toBe('');
  });

  it('preserves newlines as <br>', () => {
    expect(renderMarkdown('a\nb')).toBe('a<br>b');
  });
});
