import { describe, it, expect } from 'vitest';
import { templateCommentRanges, stripTemplateComments } from './templateCommentRanges.js';

const cut = stripTemplateComments;

// Same cases as shortcodes/comment_test.go and templatecomment's tests, so the dimming matches what the
// server removes.
describe('templateCommentRanges', () => {
  it.each([
    ['a[comment]hidden[/comment]b', 'ab'],
    ['x\n[comment]\nhidden\n[/comment]\ny', 'x\n\ny'],
    ['a[comment x\n]z[/comment]b', 'a[comment x\n]zb'],
    ['1[comment]a[/comment]2[comment]b[/comment]3', '123'],
    ['a[comment] A [comment] B [/comment] C [/comment]b', 'ab'],
    ['keep[comment]gone [meta path="x"]', 'keep'],
    ['a[/comment]b', 'ab'],
    ['a[comment reason="old sidebar"]x[/comment]b', 'ab'],
    ['a[comment reason="[/comment]"]x[/comment]b', 'ab'],
    ['a[comment reason="x]y"]z[/comment]b', 'ab'],
    ["a[comment don't]z[/comment]b", 'ab'],
    ['a[comment\u00a0x]z[/comment]b', 'a[comment\u00a0x]zb'],
    ['a[comment ' + '"'.repeat(5000) + ']z[/comment]b', 'ab'],
    ['[commentary][comments]', '[commentary][comments]'],
    ['plain [b] text', 'plain [b] text'],
    ['[comm[comment][/comment]ent]VISIBLE', ''],
  ])('%j -> %j', (input, want) => {
    expect(cut(input)).toBe(want);
  });

  it('stays linear on an unclosed opener full of quotes', () => {
    const start = performance.now();
    templateCommentRanges('[comment ' + '"'.repeat(200000));
    templateCommentRanges('[comment '.repeat(50000));
    expect(performance.now() - start).toBeLessThan(500);
  });

  it('bounds repeated removal', () => {
    const n = 20000;
    const start = performance.now();
    stripTemplateComments('[/comm'.repeat(n) + '[/comment]' + 'ent]'.repeat(n));
    expect(performance.now() - start).toBeLessThan(500);
  });

  it('uses UTF-16 offsets', () => {
    expect(templateCommentRanges('é😀[comment]x[/comment]')).toEqual([{ from: 3, to: 23 }]);
  });
});
