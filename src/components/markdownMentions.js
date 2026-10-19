// @-mention completion for the markdown editor: the CodeMirror counterpart
// of mentionTextarea.js. Typing `@` at the start of a line or after
// whitespace, then two or more characters, searches /v1/search and offers
// matches; choosing one inserts `@[type:id:name]`.

const TYPE_LABELS = {
  resource: 'Resource',
  note: 'Note',
  group: 'Group',
  tag: 'Tag',
  category: 'Category',
};

// The mention being typed: `@` then anything up to the cursor except a
// newline, another `@` or a bracket (a finished marker is `@[...]`).
const MENTION_BEFORE_CURSOR = /@[^\n@[\]]{0,60}/;
const MIN_QUERY = 2;
const DEBOUNCE_MS = 150;

// `announce` reports the result count to a live region, as mentionTextarea.js
// did; CodeMirror wires the listbox ARIA but does not announce counts.
export function mentionCompletionSource(allowedTypes, announce = () => {}) {
  return async (context) => {
    const match = context.matchBefore(MENTION_BEFORE_CURSOR);
    if (!match) return null;
    const charBefore = match.from === 0 ? '' : context.state.sliceDoc(match.from - 1, match.from);
    if (charBefore && !/\s/.test(charBefore)) return null;
    const query = match.text.slice(1);
    if (query.length < MIN_QUERY) return null;

    const controller = new AbortController();
    // onDocChange: CodeMirror otherwise lets a query the user has typed past
    // run to completion, and the next query waits for it.
    context.addEventListener('abort', () => controller.abort(), { onDocChange: true });
    await new Promise((resolve) => setTimeout(resolve, DEBOUNCE_MS));
    if (context.aborted) return null;

    let results;
    try {
      let url = `/v1/search?q=${encodeURIComponent(query)}&limit=10`;
      if (allowedTypes) url += `&types=${encodeURIComponent(allowedTypes)}`;
      const response = await fetch(url, { signal: controller.signal });
      if (!response.ok) return null;
      results = (await response.json()).results || [];
    } catch (err) {
      if (err.name !== 'AbortError') console.error('Mention search error:', err);
      return null;
    }
    if (context.aborted) return null;

    if (results.length === 0) {
      announce('No suggestions found.');
      return null;
    }
    announce(`${results.length} suggestion${results.length === 1 ? '' : 's'}. Use arrow keys to navigate.`);

    return {
      from: match.from,
      // The server already matched; CodeMirror's own fuzzy filter would drop
      // results whose name does not contain the query verbatim.
      filter: false,
      options: results.map((r) => ({
        label: r.name,
        detail: TYPE_LABELS[r.type] || r.type,
        type: 'mention',
        apply: `@[${r.type}:${r.id}:${r.name}]`,
      })),
    };
  };
}
