import { describe, expect, it } from 'vitest';
import { changedSince, changeSummary, diffRows, draftSegments, draftWarnings, lineDiff, notAppliedSummary } from './templateDraftReview.js';

const flagged = (text: string, css = false) => draftSegments(text, css).filter(s => s.flagged).map(s => s.text);

describe('template draft review aids', () => {
  it('marks the constructs that run code in markup', () => {
    const html = `<script>x</script><img src=x onerror="go()"><div x-init="location='https://example.invalid'" @click="a" :class="b"><a href="javascript:alert(1)">`;
    expect(flagged(html)).toEqual(['<script', ' onerror=', ' x-init=', ' @click=', ' :class=', 'javascript:']);
    expect(draftWarnings(html)).toHaveLength(4);
    expect(draftSegments(html).map(s => s.text).join('')).toBe(html);
  });

  it('marks a stylesheet breakout in CSS and nothing in plain rules', () => {
    expect(flagged('.a{color:red}</style><div x-init="go()">', true)).toEqual(['</style']);
    expect(draftWarnings('.a{color:red}', true)).toEqual([]);
    expect(draftSegments('.a{color:red}', true)).toEqual([{ text: '.a{color:red}', flagged: false }]);
  });

  it('lists only the snapshotted fields that changed', () => {
    const now: Record<string, string> = { A: 'a', B: 'edited' };
    expect(changedSince({ A: 'a', B: 'b' }, ['A', 'B', 'C'], name => now[name] ?? '')).toEqual(['B']);
  });
});

describe('before/after summary of a draft field', () => {
  it('names a field that was empty as new, with its line count', () => {
    expect(changeSummary('', '<h1>a</h1>\n<p>b</p>')).toMatchObject({ status: 'new', added: 2, removed: 0, text: 'New: 2 lines' });
    expect(changeSummary('  \n', 'x').status).toBe('new');
  });

  it('names a field the draft empties as cleared', () => {
    expect(changeSummary('a\nb\nc', '')).toMatchObject({ status: 'cleared', added: 0, removed: 3, text: 'Cleared: removes 3 lines' });
  });

  it('says when nothing changes', () => {
    expect(changeSummary('a\nb', 'a\nb')).toMatchObject({ status: 'unchanged', text: 'No change' });
    expect(changeSummary('', '')).toMatchObject({ status: 'unchanged', text: 'No change' });
  });

  it('counts the lines a replacement adds and removes', () => {
    const s = changeSummary('a\nb\nc\nd', 'a\nB\nc\nd\ne');
    expect(s).toMatchObject({ status: 'changed', added: 2, removed: 1, beforeLines: 4, afterLines: 5 });
    expect(s.text).toBe('Changed: +2 / -1 lines (4 to 5)');
  });

  it('calls a change that moves no line whitespace only', () => {
    expect(changeSummary('a\n', 'a')).toMatchObject({ status: 'changed', added: 0, removed: 0, text: 'Changed: whitespace only' });
  });

  it('uses the singular for one line', () => {
    expect(changeSummary('', 'x').text).toBe('New: 1 line');
  });
});

describe('line diff of a draft field', () => {
  const kinds = (rows: { kind: string; text: string }[]) => rows.map(r => `${r.kind[0]}${r.text}`);

  it('keeps unchanged lines and marks removals before additions', () => {
    expect(kinds(lineDiff('a\nb\nc', 'a\nx\nc'))).toEqual(['sa', 'db', 'ax', 'sc']);
  });

  it('finds the longest common run, not just the shared ends', () => {
    expect(kinds(lineDiff('a\nb\nc\nd', 'b\nc\nd\na'))).toEqual(['da', 'sb', 'sc', 'sd', 'aa']);
  });

  it('falls back to remove-all then add-all past the size cap', () => {
    const before = Array.from({ length: 1200 }, (_, i) => `b${i}`).join('\n');
    const after = Array.from({ length: 1200 }, (_, i) => `a${i}`).join('\n');
    const rows = lineDiff(before, after);
    expect(rows).toHaveLength(2400);
    expect(rows.slice(0, 1200).every(r => r.kind === 'del')).toBe(true);
    expect(rows.slice(1200).every(r => r.kind === 'add')).toBe(true);
  });

  it('folds long unchanged runs into a gap row, keeping context around changes', () => {
    const before = ['1', '2', '3', '4', '5', '6', '7', '8', '9', '10'].join('\n');
    const after = ['1', '2', '3', '4', '5', '6', '7', '8', '9', 'ten'].join('\n');
    const rows = diffRows(before, after, 2);
    expect(rows.map(r => r.kind)).toEqual(['gap', 'same', 'same', 'del', 'add']);
    expect(rows[0].text).toBe('7 unchanged lines');
  });

  it('numbers every row with a stable key', () => {
    const rows = diffRows('a', 'b');
    expect(new Set(rows.map(r => r.key)).size).toBe(rows.length);
  });
});

describe('a field Apply will not write', () => {
  it('is summarised as not applied, whatever the draft holds', () => {
    expect(notAppliedSummary('a\nb')).toMatchObject({ status: 'blocked', added: 0, removed: 0, text: 'Not applied' });
  });
});
