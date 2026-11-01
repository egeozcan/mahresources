import { ViewPlugin, Decoration, EditorView } from '@codemirror/view';

// This module is a lazy chunk, so it imports nothing the main bundle also
// imports: a chunk that does imports main.js by a URL other than the page's
// (main.js?v=...), and the browser then runs main a second time, which throws
// on its first customElements.define. The editor passes the scanner in.

const commentMark = Decoration.mark({ class: 'cm-template-comment' });

function build(view, templateCommentRanges) {
  const ranges = templateCommentRanges(view.state.doc.toString());
  return Decoration.set(ranges.filter((r) => r.to > r.from).map((r) => commentMark.range(r.from, r.to)));
}

// templateCommentExtensions dims every template comment in a shortcode editor,
// so what the server will remove reads as removed. templateCommentRanges is the
// scanner from templateCommentRanges.js.
export function templateCommentExtensions(templateCommentRanges) {
  return [
    ViewPlugin.fromClass(
      class {
        constructor(view) {
          this.decorations = build(view, templateCommentRanges);
        }
        update(update) {
          if (update.docChanged) this.decorations = build(update.view, templateCommentRanges);
        }
      },
      { decorations: (v) => v.decorations },
    ),
    EditorView.baseTheme({
      // An opaque colour, not opacity: fading the syntax colours took them
      // under 4.5:1. stone-500 on white is 4.8:1.
      '.cm-template-comment, .cm-template-comment *': { color: '#78716c !important', fontStyle: 'italic' },
    }),
  ];
}
