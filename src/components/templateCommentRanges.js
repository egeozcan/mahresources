// The editor's copy of the server's template comment rules
// (templatecomment/templatecomment.go): comments nest, an unclosed [comment]
// runs to the end, a stray [/comment] is a region of its own, and the opener
// ends at its first "]" and fits on one line. It has no imports, so plain-text fallbacks can use it
// without loading CodeMirror. Offsets are UTF-16 indices, as CodeMirror uses.

const OPEN = '[comment';
const CLOSE = '[/comment]';

// The space that may follow the name: Go's \s without the line breaks (and,
// unlike JavaScript's \s, without NBSP and other Unicode spaces).
const isSpace = (c) => c === ' ' || c === '\t' || c === '\f';

// Forward-only: an opener that meets a line break before its "]" fails, and so
// does every later one on that line, so the scan resumes at the break, which
// keeps a text of unfinished openers linear.
function openers(text) {
  const spans = [];
  for (let pos = 0; ; ) {
    const start = text.indexOf(OPEN, pos);
    if (start < 0) return spans;
    const after = start + OPEN.length;
    if (text[after] === ']') {
      spans.push({ from: start, to: after + 1, closing: false });
      pos = after + 1;
    } else if (after < text.length && isSpace(text[after])) {
      let i = after;
      while (i < text.length && text[i] !== ']' && text[i] !== '\n' && text[i] !== '\r') i++;
      if (text[i] !== ']') {
        pos = i;
        continue;
      }
      spans.push({ from: start, to: i + 1, closing: false });
      pos = i + 1;
    } else {
      pos = after;
    }
  }
}

function closers(text) {
  const spans = [];
  for (let pos = 0; ; ) {
    const start = text.indexOf(CLOSE, pos);
    if (start < 0) return spans;
    spans.push({ from: start, to: start + CLOSE.length, closing: true });
    pos = start + CLOSE.length;
  }
}

export function templateCommentRanges(text) {
  if (!text || !text.includes('comment')) return [];
  const tags = [...openers(text), ...closers(text)].sort((a, b) => a.from - b.from);

  const ranges = [];
  let depth = 0;
  let openFrom = 0;
  let lastEnd = 0;
  for (const tag of tags) {
    if (tag.from < lastEnd) continue;
    lastEnd = tag.to;
    if (!tag.closing) {
      if (depth === 0) openFrom = tag.from;
      depth++;
    } else if (depth === 0) {
      ranges.push({ from: tag.from, to: tag.to });
    } else if (--depth === 0) {
      ranges.push({ from: openFrom, to: tag.to });
    }
  }
  if (depth > 0) ranges.push({ from: openFrom, to: text.length });
  return ranges;
}

// stripTemplateComments is what the server shows for text: every comment
// removed, repeating until none is left, since removing one can join the text
// around it into a new tag. The pass cap matches the server's maxPasses.
const MAX_PASSES = 16;

export function stripTemplateComments(text) {
  for (let pass = 0; pass < MAX_PASSES; pass++) {
    const ranges = templateCommentRanges(text);
    if (ranges.length === 0) break;
    let out = '';
    let last = 0;
    for (const r of ranges) {
      out += text.slice(last, r.from);
      last = r.to;
    }
    text = out + text.slice(last);
  }
  return text;
}
