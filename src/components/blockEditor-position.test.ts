/**
 * Guards the JS positionBetween/_generateBetween port against drift from the Go
 * reference (lib/position.go). The branch ported two Go branches into
 * _generateBetween to fix an ordering bug (positionBetween("a","aa") used to
 * return "aan" > "aa"). Same extract-and-eval approach as the renderMarkdown
 * test, since these are methods on the Alpine data factory.
 */
import { readFileSync } from 'fs';
import { resolve, dirname } from 'path';
import { fileURLToPath } from 'url';
import { describe, it, expect } from 'vitest';

const __dirname = dirname(fileURLToPath(import.meta.url));
const src = readFileSync(resolve(__dirname, './blockEditor.js'), 'utf-8');

function extractMethod(name: string): string {
  const re = new RegExp(name + '\\(before, after\\)\\s*\\{[\\s\\S]*?\\n\\s{4}\\},');
  const m = src.match(re);
  if (!m) throw new Error('could not extract ' + name + ' from blockEditor.js');
  return m[0]
    .replace(name + '(before, after)', 'function ' + name + '(before, after)')
    .replace(/,\s*$/, '')
    .replace(/this\._generateBetween/g, '_generateBetween');
}

const positionBetween = new Function(
  `const MIN_CHAR = 'a'.charCodeAt(0);
   const MAX_CHAR = 'z'.charCodeAt(0);
   ${extractMethod('_generateBetween')}
   ${extractMethod('positionBetween')}
   return positionBetween;`
)() as (before: string, after: string) => string;

describe('positionBetween / _generateBetween — JS/Go parity', () => {
  // Values verified against Go lib.PositionBetween.
  it('matches Go for the regressed adjacent pair ("a","aa") -> "aa" (not "aan")', () => {
    expect(positionBetween('a', 'aa')).toBe('aa');
  });
  it('matches Go for ("b","ba") -> "ba"', () => {
    expect(positionBetween('b', 'ba')).toBe('ba');
  });
  it('matches Go for ("a","b") -> "an"', () => {
    expect(positionBetween('a', 'b')).toBe('an');
  });
  it('returns "n" for the empty/empty (first block) case', () => {
    expect(positionBetween('', '')).toBe('n');
  });

  it('keeps before < result, and result <= after (< after when not adjacent)', () => {
    const pairs: [string, string][] = [
      ['a', 'b'], ['a', 'aa'], ['b', 'ba'], ['n', 'na'], ['a', 'c'],
      ['aa', 'ab'], ['m', 'n'], ['', 'n'], ['n', ''], ['a', ''],
    ];
    for (const [before, after] of pairs) {
      const r = positionBetween(before, after);
      expect(r > before).toBe(true);
      // after === '' is an open upper bound ("past z"), so skip that check.
      if (after !== '') expect(r <= after).toBe(true);
    }
  });

  it('stays strictly ordered across repeated end-appends (the default add path)', () => {
    let last = 'n';
    let prev = '';
    for (let i = 0; i < 50; i++) {
      const next = positionBetween(last, '');
      expect(next > last).toBe(true);
      prev = last;
      last = next;
    }
    expect(prev).not.toBe(last);
  });
});
