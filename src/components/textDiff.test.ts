import { describe, expect, it } from 'vitest';
import { textDiff, wordSegments } from './textDiff.js';

/** Builds a component with inline text and runs the diff, skipping the fetch path. */
function diffOf(left: string, right: string) {
  const component: any = textDiff({ leftText: left, rightText: right });
  component.computeDiff();
  return component;
}

describe('textDiff split alignment', () => {
  it('puts the two halves of one change on the same row', () => {
    const left = ['a', 'OLD ONE', 'OLD TWO', 'b'].join('\n');
    const right = ['a', 'NEW ONE', 'NEW TWO', 'b'].join('\n');
    const d = diffOf(left, right);

    expect(d.splitLeft.length).toBe(d.splitRight.length);

    const leftChanged = d.splitLeft
      .map((row: any, i: number) => (row.changed ? i : -1))
      .filter((i: number) => i >= 0);
    const rightChanged = d.splitRight
      .map((row: any, i: number) => (row.changed ? i : -1))
      .filter((i: number) => i >= 0);

    // The point of a split diff: a changed line and its replacement occupy the
    // same row index, so the eye can compare them across the gutter.
    expect(leftChanged).toEqual(rightChanged);
  });

  it('pads the shorter side so both columns stay the same length', () => {
    const left = ['a', 'gone', 'b'].join('\n');
    const right = ['a', 'one', 'two', 'three', 'b'].join('\n');
    const d = diffOf(left, right);

    expect(d.splitLeft.length).toBe(d.splitRight.length);
    expect(d.splitLeft.filter((r: any) => r.blank).length).toBe(2);
    expect(d.splitRight.filter((r: any) => r.blank).length).toBe(0);
  });

  it('counts one edit as one change however many lines it spans', () => {
    const left = ['a', 'x1', 'x2', 'x3', 'b'].join('\n');
    const right = ['a', 'y1', 'y2', 'y3', 'b'].join('\n');
    const d = diffOf(left, right);

    expect(d.changeCount).toBe(1);
    expect(d.stats).toEqual({ added: 3, removed: 3 });
  });
});

describe('textDiff context folding', () => {
  const long = (n: number, tag: string) =>
    Array.from({ length: n }, (_, i) => `${tag} line ${i}`).join('\n');

  it('collapses a long unchanged run and keeps context either side', () => {
    const body = long(60, 'same');
    const d = diffOf(`head\n${body}\ntail`, `HEAD\n${body}\nTAIL`);

    const foldRows = d.unifiedRows.filter((r: any) => r.fold);
    expect(foldRows.length).toBe(1);
    // 60 unchanged lines minus three lines of context at each end.
    expect(foldRows[0].foldLength).toBe(54);
    expect(d.unifiedRows.length).toBeLessThan(d.unifiedDiff.length);
  });

  it('leaves a short unchanged run alone', () => {
    const d = diffOf(`head\n${long(4, 'same')}\ntail`, `HEAD\n${long(4, 'same')}\nTAIL`);
    expect(d.unifiedRows.filter((r: any) => r.fold).length).toBe(0);
  });

  it('expanding a fold restores its rows', () => {
    const body = long(60, 'same');
    const d = diffOf(`head\n${body}\ntail`, `HEAD\n${body}\nTAIL`);

    const folded = d.unifiedRows.length;
    const id = d.unifiedRows.find((r: any) => r.fold).foldId;
    d.expandFold(id);

    expect(d.unifiedRows.length).toBe(d.unifiedDiff.length);
    expect(d.unifiedRows.length).toBeGreaterThan(folded);
    expect(d.unifiedRows.filter((r: any) => r.fold).length).toBe(0);
  });

  it('folds both views at the same places', () => {
    const body = long(60, 'same');
    const d = diffOf(`head\n${body}\ntail`, `HEAD\n${body}\nTAIL`);

    const foldsOf = (rows: any[]) => rows.filter((r) => r.fold).map((r) => r.foldLength);
    expect(foldsOf(d.splitLeftRows)).toEqual(foldsOf(d.unifiedRows));
    expect(foldsOf(d.splitRightRows)).toEqual(foldsOf(d.unifiedRows));
  });
});

describe('wordSegments', () => {
  it('marks only the part of the line that changed', () => {
    const [left, right] = wordSegments(
      '  "maxConnections": 10,',
      '  "maxConnections": 25,',
    );
    expect(left).not.toBeNull();
    const changedLeft = left!.filter((s) => s.changed).map((s) => s.text).join('');
    const changedRight = right!.filter((s) => s.changed).map((s) => s.text).join('');
    expect(changedLeft).toContain('10');
    expect(changedRight).toContain('25');
    expect(changedLeft).not.toContain('maxConnections');
  });

  it('gives up when the two lines share almost nothing', () => {
    // Marking every token is noisier than marking the whole line.
    expect(wordSegments('alpha beta gamma', 'entirely different words here')).toEqual([null, null]);
  });

  it('round-trips the original text', () => {
    const [left, right] = wordSegments('the quick brown fox', 'the slow brown fox');
    expect(left!.map((s) => s.text).join('')).toBe('the quick brown fox');
    expect(right!.map((s) => s.text).join('')).toBe('the slow brown fox');
  });
});

describe('textDiff patch export', () => {
  // Prefixed lines alone read as a patch and apply as nothing: `git apply` needs
  // the file headers to know what it is patching and the hunk header to know
  // where.
  it('emits a patch with file and hunk headers', () => {
    const component: any = textDiff({
      leftText: 'a\nb\n',
      rightText: 'a\nc\n',
      leftName: 'Older \u2014 v1',
      rightName: 'Current \u2014 v2',
    });
    component.computeDiff();

    const lines = component.asUnifiedText().split('\n');
    // The em dash is folded to a hyphen so the header stays readable: the patch
    // library escapes a non-ASCII name to octal, the way git does.
    expect(lines).toContain('--- Older - v1');
    expect(lines).toContain('+++ Current - v2');
    expect(lines.some((l: string) => /^@@ -\d+,\d+ \+\d+,\d+ @@/.test(l))).toBe(true);
    expect(lines).toContain('-b');
    expect(lines).toContain('+c');
  });

  it('names the sides generically when the caller supplies none', () => {
    const d = diffOf('a\n', 'b\n');
    expect(d.asUnifiedText()).toContain('--- left');
    expect(d.asUnifiedText()).toContain('+++ right');
  });
});

describe('textDiff end-of-file newline', () => {
  // Splitting on '\n' turns both "a" and "a\n" into ["a"], so a pair differing
  // only in the closing newline rendered as an identical removed and added row.
  it('marks the side that has no closing newline', () => {
    const d = diffOf('a\n', 'a');

    const left = d.splitLeft.filter((r: any) => !r.blank);
    const right = d.splitRight.filter((r: any) => !r.blank);
    expect(left[left.length - 1].noNewline).toBeUndefined();
    expect(right[right.length - 1].noNewline).toBe(true);

    const removed = d.unifiedDiff.filter((r: any) => r.type !== 'added');
    const added = d.unifiedDiff.filter((r: any) => r.type !== 'removed');
    expect(removed[removed.length - 1].noNewline).toBeUndefined();
    expect(added[added.length - 1].noNewline).toBe(true);
  });

  it('marks nothing when both sides end cleanly', () => {
    const d = diffOf('a\nb\n', 'a\nc\n');
    expect([...d.splitLeft, ...d.splitRight, ...d.unifiedDiff].some((r: any) => r.noNewline)).toBe(false);
  });
});
