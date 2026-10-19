// Markdown formatting commands for the markdown editor's toolbar and keymap.
// Each takes a CodeMirror EditorView and returns true, so it can sit in a
// keymap directly. They only dispatch changes and selections, so this module
// imports nothing from CodeMirror and stays out of the main bundle.

// Wrap every selection in `before`/`after`. An empty selection inserts the
// pair with the cursor between them.
export function wrapSelection(view, before, after = before) {
  const { state } = view;
  const changes = [];
  const ranges = [];
  let shift = 0;
  for (const range of state.selection.ranges) {
    changes.push({ from: range.from, insert: before }, { from: range.to, insert: after });
    const from = range.from + shift + before.length;
    const to = range.to + shift + before.length;
    ranges.push({ anchor: from, head: to });
    shift += before.length + after.length;
  }
  view.dispatch({
    changes,
    selection: selectionOf(state, ranges),
    scrollIntoView: true,
    userEvent: 'input.format',
  });
  view.focus();
  return true;
}

// Turn the main selection into a link: `[text](url)` with `url` selected so
// the next keystrokes replace it.
export function insertLink(view) {
  const { state } = view;
  const range = state.selection.main;
  const text = state.sliceDoc(range.from, range.to);
  const insert = `[${text}](url)`;
  const urlFrom = range.from + text.length + 3;
  view.dispatch({
    changes: { from: range.from, to: range.to, insert },
    selection: { anchor: urlFrom, head: urlFrom + 3 },
    scrollIntoView: true,
    userEvent: 'input.format',
  });
  view.focus();
  return true;
}

// Prefix every line the selection touches. `prefix` may be a function of the
// line's index within the selection (numbered lists). A line that already
// starts with the prefix loses it instead, so the button toggles.
export function prefixLines(view, prefix) {
  const { state } = view;
  const range = state.selection.main;
  const first = state.doc.lineAt(range.from).number;
  const last = state.doc.lineAt(range.to).number;
  const changes = [];
  const lines = [];
  for (let n = first; n <= last; n++) lines.push(state.doc.line(n));
  const prefixes = lines.map((_, i) => (typeof prefix === 'function' ? prefix(i) : prefix));
  const allPrefixed = lines.every((line, i) => line.text.startsWith(prefixes[i]));
  lines.forEach((line, i) => {
    if (allPrefixed) {
      changes.push({ from: line.from, to: line.from + prefixes[i].length, insert: '' });
    } else {
      changes.push({ from: line.from, insert: prefixes[i] });
    }
  });
  view.dispatch({ changes, scrollIntoView: true, userEvent: 'input.format' });
  view.focus();
  return true;
}

export const bold = (view) => wrapSelection(view, '**');
export const italic = (view) => wrapSelection(view, '*');
export const inlineCode = (view) => wrapSelection(view, '`');
export const heading = (view) => prefixLines(view, '## ');
export const bulletList = (view) => prefixLines(view, '- ');
export const numberedList = (view) => prefixLines(view, (i) => `${i + 1}. `);

// The keymap the editor installs ahead of CodeMirror's defaults. Mod-Shift-k
// is link rather than CodeMirror's delete-line; Mod-k is left alone so it
// still opens global search.
export const markdownKeymap = [
  { key: 'Mod-b', run: bold },
  { key: 'Mod-i', run: italic },
  { key: 'Mod-Shift-k', run: insertLink },
];

function selectionOf(state, ranges) {
  // EditorSelection lives in @codemirror/state; build it through the state's
  // own constructor so this module needs no import.
  const Selection = state.selection.constructor;
  return Selection.create(
    ranges.map((r) => Selection.range(r.anchor, r.head)),
    state.selection.mainIndex,
  );
}
