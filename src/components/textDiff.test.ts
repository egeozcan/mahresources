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
  it('emits a prefixed line per diff row', () => {
    const d = diffOf('a\nb\n', 'a\nc\n');
    expect(d.asUnifiedText().split('\n')).toEqual([' a', '-b', '+c']);
  });
});
