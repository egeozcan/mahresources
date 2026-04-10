const VOID_ELEMENTS = new Set([
  'area', 'base', 'br', 'col', 'embed', 'hr', 'img', 'input',
  'link', 'meta', 'source', 'track', 'wbr',
]);

const INDENT = '  ';

/**
 * Lightweight HTML formatter that preserves template directives and non-standard attributes.
 * Adjusts whitespace between tags based on nesting depth.
 * Best-effort: returns original input if it cannot parse.
 */
export function formatHtml(input) {
  if (!input || !input.trim()) return input;

  try {
    // Tokenize into tags and text segments
    // Matches: HTML tags, HTML comments, template directives ({% %}, {{ }})
    const tokenRegex = /(<!--[\s\S]*?-->|<\/[^>]+>|<[^>]+\/\s*>|<[^>]+>|{%[\s\S]*?%}|{{[\s\S]*?}})/g;
    const tokens = [];
    let lastIndex = 0;
    let match;

    while ((match = tokenRegex.exec(input)) !== null) {
      if (match.index > lastIndex) {
        tokens.push({ type: 'text', value: input.slice(lastIndex, match.index) });
      }
      const raw = match[0];
      if (raw.startsWith('<!--')) {
        tokens.push({ type: 'comment', value: raw });
      } else if (raw.startsWith('{%') || raw.startsWith('{{')) {
        tokens.push({ type: 'template', value: raw });
      } else if (raw.startsWith('</')) {
        const tagName = raw.match(/<\/\s*([a-zA-Z][a-zA-Z0-9-]*)/)?.[1]?.toLowerCase() || '';
        tokens.push({ type: 'close', value: raw, tag: tagName });
      } else if (raw.endsWith('/>')) {
        tokens.push({ type: 'selfclose', value: raw });
      } else {
        const tagName = raw.match(/<\s*([a-zA-Z][a-zA-Z0-9-]*)/)?.[1]?.toLowerCase() || '';
        tokens.push({ type: 'open', value: raw, tag: tagName });
      }
      lastIndex = match.index + match[0].length;
    }

    if (lastIndex < input.length) {
      tokens.push({ type: 'text', value: input.slice(lastIndex) });
    }

    // Build formatted output
    const lines = [];
    let depth = 0;

    for (const token of tokens) {
      const trimmed = token.value.trim();
      if (!trimmed) continue;

      if (token.type === 'close') {
        depth = Math.max(0, depth - 1);
        lines.push(INDENT.repeat(depth) + trimmed);
      } else if (token.type === 'open') {
        lines.push(INDENT.repeat(depth) + trimmed);
        if (!VOID_ELEMENTS.has(token.tag)) {
          depth++;
        }
      } else {
        // text, comment, template directive, self-closing tag
        lines.push(INDENT.repeat(depth) + trimmed);
      }
    }

    return lines.join('\n');
  } catch {
    return input;
  }
}
