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
