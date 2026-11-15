// Reading aids for a generated template draft held for review. A draft is shown
// as text and never rendered before Apply; these mark the constructs in it that
// run code once applied, so a reviewer sees them. They are not a security
// boundary: Alpine attributes, event handlers and stylesheet breakouts have too
// many spellings for a pattern list to be complete, and nothing here claims to
// be. The boundary is that nothing reaches an editor, or the preview, before
// the user presses Apply.

const HTML_PATTERNS = [
  { label: '<script>', re: /<script\b/gi },
  { label: 'event handler (on…=)', re: /\son[a-z]+\s*=/gi },
  { label: 'Alpine directive (x-…, @…, :…)', re: /\s(?:x-[\w:.-]+|@[\w:.-]+|:[a-z][\w:.-]*)\s*=/gi },
  { label: 'javascript: URL', re: /javascript\s*:/gi },
];

const CSS_PATTERNS = [
  { label: '</style> breakout', re: /<\/style/gi },
  { label: 'javascript: URL', re: /javascript\s*:/gi },
];

function patternsFor(css) {
  return css ? CSS_PATTERNS : HTML_PATTERNS;
}

// draftSegments splits text into [{ text, flagged }] runs for x-text rendering,
// with flagged runs covering each matched construct.
export function draftSegments(text, css = false) {
  const value = String(text || '');
  const ranges = [];
  for (const { re } of patternsFor(css)) {
    re.lastIndex = 0;
    let match;
    while ((match = re.exec(value)) !== null) {
      ranges.push([match.index, match.index + match[0].length]);
      if (match[0].length === 0) re.lastIndex++;
    }
  }
  ranges.sort((a, b) => a[0] - b[0]);
  const segments = [];
  let at = 0;
  for (const [start, end] of ranges) {
    if (end <= at) continue;
    const from = Math.max(start, at);
    if (from > at) segments.push({ text: value.slice(at, from), flagged: false });
    segments.push({ text: value.slice(from, end), flagged: true });
    at = end;
  }
  if (at < value.length || segments.length === 0) segments.push({ text: value.slice(at), flagged: false });
  return segments;
}

// draftWarnings names the kinds of code-running construct the text contains,
// for a sentence a screen reader announces (highlighting alone is not read out).
export function draftWarnings(text, css = false) {
  const value = String(text || '');
  return patternsFor(css)
    .filter(({ re }) => {
      re.lastIndex = 0;
      return re.test(value);
    })
    .map(({ label }) => label);
}

// changedSince lists the fields whose current value differs from the snapshot
// taken when the request started.
export function changedSince(snapshot, fields, read) {
  return fields.filter((name) => Object.prototype.hasOwnProperty.call(snapshot, name) && read(name) !== snapshot[name]);
}

// ---- Before and after ----------------------------------------------------
// A draft is compared with the field as it stood when Generate was pressed, the
// text the model was given. Line based: templates are short, and a reader
// scanning a review wants "which lines change", not a character diff.

// Past this many cells the quadratic table costs more than the review is worth,
// and the diff falls back to "every old line goes, every new line comes".
const MAX_DIFF_CELLS = 1_000_000;

// One trailing newline is not a line of its own.
function splitLines(text) {
  const value = String(text ?? '');
  if (value === '') return [];
  const lines = value.split('\n');
  if (lines.length > 1 && lines[lines.length - 1] === '') lines.pop();
  return lines;
}

// lineDiff returns [{ kind: 'same' | 'del' | 'add', text }] turning before into
// after, with each change's removals ahead of its additions.
export function lineDiff(before, after) {
  const a = splitLines(before);
  const b = splitLines(after);
  let start = 0;
  while (start < a.length && start < b.length && a[start] === b[start]) start++;
  let endA = a.length;
  let endB = b.length;
  while (endA > start && endB > start && a[endA - 1] === b[endB - 1]) { endA--; endB--; }

  const rows = a.slice(0, start).map((text) => ({ kind: 'same', text }));
  const midA = a.slice(start, endA);
  const midB = b.slice(start, endB);
  const n = midA.length;
  const m = midB.length;
  if (n * m > MAX_DIFF_CELLS) {
    for (const text of midA) rows.push({ kind: 'del', text });
    for (const text of midB) rows.push({ kind: 'add', text });
  } else if (n || m) {
    // table[i * (m + 1) + j] = length of the longest common run of midA[i..] and midB[j..].
    // min(n, m) <= 1000 under the cap, so 16 bits hold every entry.
    const width = m + 1;
    const table = new Uint16Array((n + 1) * width);
    for (let i = n - 1; i >= 0; i--) {
      for (let j = m - 1; j >= 0; j--) {
        table[i * width + j] = midA[i] === midB[j]
          ? table[(i + 1) * width + j + 1] + 1
          : Math.max(table[(i + 1) * width + j], table[i * width + j + 1]);
      }
    }
    let i = 0;
    let j = 0;
    while (i < n || j < m) {
      if (i < n && j < m && midA[i] === midB[j]) {
        rows.push({ kind: 'same', text: midA[i] }); i++; j++;
      } else if (j >= m || (i < n && table[(i + 1) * width + j] >= table[i * width + j + 1])) {
        rows.push({ kind: 'del', text: midA[i] }); i++;
      } else {
        rows.push({ kind: 'add', text: midB[j] }); j++;
      }
    }
  }
  for (const text of a.slice(endA)) rows.push({ kind: 'same', text });
  return rows;
}

const lines = (count) => `${count} line${count === 1 ? '' : 's'}`;

// changeSummary says in one phrase what Apply does to a field: { status: 'new' |
// 'cleared' | 'unchanged' | 'changed', added, removed, beforeLines, afterLines, text }.
export function changeSummary(before, after) {
  const was = String(before ?? '');
  const next = String(after ?? '');
  const beforeLines = splitLines(was).length;
  const afterLines = splitLines(next).length;
  const base = { beforeLines, afterLines };
  if (was === next) return { ...base, status: 'unchanged', added: 0, removed: 0, text: 'No change' };
  if (was.trim() === '' && next.trim() !== '') return { ...base, status: 'new', added: afterLines, removed: 0, text: `New: ${lines(afterLines)}` };
  if (next.trim() === '' && was.trim() !== '') return { ...base, status: 'cleared', added: 0, removed: beforeLines, text: `Cleared: removes ${lines(beforeLines)}` };
  const rows = lineDiff(was, next);
  const added = rows.filter((row) => row.kind === 'add').length;
  const removed = rows.filter((row) => row.kind === 'del').length;
  const text = added || removed
    ? `Changed: +${added} / -${removed} ${added + removed === 1 ? 'line' : 'lines'} (${beforeLines} to ${afterLines})`
    : 'Changed: whitespace only';
  return { ...base, status: 'changed', added, removed, text };
}

// The summary of a field the draft returns but Apply will not write: half a pair,
// or a field Privacy mode withheld (and its companion).
export function notAppliedSummary(before) {
  const beforeLines = splitLines(before).length;
  return { status: 'blocked', added: 0, removed: 0, beforeLines, afterLines: beforeLines, text: 'Not applied' };
}

// diffRows is lineDiff for display: unchanged runs longer than the context
// around a change fold into one { kind: 'gap', text: 'N unchanged lines' } row.
// Every row carries a key unique within the result, for x-for.
export function diffRows(before, after, context = 3) {
  const rows = lineDiff(before, after);
  const out = [];
  let at = 0;
  while (at < rows.length) {
    if (rows[at].kind !== 'same') { out.push(rows[at]); at++; continue; }
    let end = at;
    while (end < rows.length && rows[end].kind === 'same') end++;
    const keepHead = at === 0 ? 0 : context;
    const keepTail = end === rows.length ? 0 : context;
    const run = end - at;
    if (run > keepHead + keepTail + 1) {
      out.push(...rows.slice(at, at + keepHead));
      out.push({ kind: 'gap', text: `${lines(run - keepHead - keepTail).replace('line', 'unchanged line')}` });
      out.push(...rows.slice(end - keepTail, end));
    } else {
      out.push(...rows.slice(at, end));
    }
    at = end;
  }
  return out.map((row, index) => ({ ...row, key: index }));
}
