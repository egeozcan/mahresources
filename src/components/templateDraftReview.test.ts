import { describe, expect, it } from 'vitest';
import { changedSince, draftSegments, draftWarnings } from './templateDraftReview.js';

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
