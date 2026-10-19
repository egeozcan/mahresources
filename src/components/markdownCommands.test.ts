/**
 * Toolbar/keymap commands for the markdown editor, run against a real
 * CodeMirror state through a minimal view stand-in.
 */
import { describe, it, expect } from 'vitest';
import { EditorState, EditorSelection } from '@codemirror/state';
import { bold, italic, insertLink, heading, bulletList, numberedList } from './markdownCommands.js';

function viewOf(doc: string, ranges: Array<[number, number]>) {
  const view: any = {
    state: EditorState.create({
      doc,
      selection: EditorSelection.create(ranges.map(([a, h]) => EditorSelection.range(a, h))),
      extensions: EditorState.allowMultipleSelections.of(true),
    }),
    dispatch(spec: any) { this.state = this.state.update(spec).state; },
    focus() {},
  };
  return view;
}

const selected = (view: any) =>
  view.state.selection.ranges.map((r: any) => view.state.sliceDoc(r.from, r.to));

describe('markdown commands', () => {
  it('bold wraps the selection and keeps it selected', () => {
    const view = viewOf('a word here', [[2, 6]]);
    bold(view);
    expect(view.state.doc.toString()).toBe('a **word** here');
    expect(selected(view)).toEqual(['word']);
  });

  it('wraps every selection when there are several', () => {
    const view = viewOf('one two', [[0, 3], [4, 7]]);
    italic(view);
    expect(view.state.doc.toString()).toBe('*one* *two*');
    expect(selected(view)).toEqual(['one', 'two']);
  });

  it('an empty selection inserts the pair with the cursor inside', () => {
    const view = viewOf('ab', [[1, 1]]);
    bold(view);
    expect(view.state.doc.toString()).toBe('a****b');
    expect(view.state.selection.main.head).toBe(3);
  });

  it('link wraps the text and selects the url placeholder', () => {
    const view = viewOf('see site now', [[4, 8]]);
    insertLink(view);
    expect(view.state.doc.toString()).toBe('see [site](url) now');
    expect(selected(view)).toEqual(['url']);
  });

  it('heading and lists prefix every selected line, and toggle off', () => {
    const view = viewOf('a\nb\nc', [[0, 3]]);
    bulletList(view);
    expect(view.state.doc.toString()).toBe('- a\n- b\nc');
    bulletList(view);
    expect(view.state.doc.toString()).toBe('a\nb\nc');

    numberedList(view);
    expect(view.state.doc.toString()).toBe('1. a\n2. b\nc');

    const h = viewOf('Title', [[0, 0]]);
    heading(h);
    expect(h.state.doc.toString()).toBe('## Title');
  });
});
