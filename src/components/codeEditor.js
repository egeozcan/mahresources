import { EditorView, keymap, lineNumbers, highlightActiveLine, highlightActiveLineGutter, drawSelection } from '@codemirror/view';
import { EditorState, Compartment } from '@codemirror/state';
import { defaultKeymap, history, historyKeymap, indentWithTab } from '@codemirror/commands';
import { syntaxHighlighting, defaultHighlightStyle, bracketMatching, indentOnInput } from '@codemirror/language';
import { autocompletion, closeBrackets, closeBracketsKeymap } from '@codemirror/autocomplete';

export function codeEditor({ mode = 'sql', dbType = 'SQLITE', label = '' } = {}) {
  return {
    view: null,
    langCompartment: new Compartment(),

    async init() {
      const hiddenInput = this.$refs.hiddenInput;
      const container = this.$refs.editorContainer;
      const initialValue = hiddenInput.value || '';
      const fieldName = hiddenInput.getAttribute('name') || '';
      const ariaLabel = label || fieldName;

      // Start with an empty language compartment — will be filled async
      const extensions = [
        lineNumbers(),
        highlightActiveLine(),
        highlightActiveLineGutter(),
        drawSelection(),
        indentOnInput(),
        bracketMatching(),
        closeBrackets(),
        history(),
        syntaxHighlighting(defaultHighlightStyle, { fallback: true }),
        autocompletion(),
        keymap.of([
          ...closeBracketsKeymap,
          ...defaultKeymap,
          ...historyKeymap,
          indentWithTab,
        ]),
        this.langCompartment.of([]),
        EditorView.updateListener.of((update) => {
          if (update.docChanged) {
            hiddenInput.value = update.state.doc.toString();
          }
        }),
        EditorView.contentAttributes.of({
          'aria-label': ariaLabel,
          'data-language': mode,
        }),
        EditorView.theme({
          '&': { minHeight: '200px', maxHeight: '60vh' },
          '.cm-scroller': { overflow: 'auto', minHeight: '200px' },
          '.cm-content': { minHeight: '200px' },
        }),
      ];

      this.view = new EditorView({
        state: EditorState.create({ doc: initialValue, extensions }),
        parent: container,
      });

      // Load language extension asynchronously
      if (mode === 'sql') {
        this.loadSQL(dbType);
      } else if (mode === 'html') {
        this.loadHTML();
      }
    },

    async loadSQL(dbType) {
      const { sql, SQLite, PostgreSQL } = await import('@codemirror/lang-sql');
      const dialect = dbType === 'POSTGRES' ? PostgreSQL : SQLite;

      // Fetch schema for autocompletion
      let schema = undefined;
      try {
        const resp = await fetch('/v1/query/schema');
        if (resp.ok) {
          schema = await resp.json();
        }
      } catch (e) {
        // Schema fetch failed — proceed without it
      }

      this.view.dispatch({
        effects: this.langCompartment.reconfigure(sql({ dialect, schema })),
      });
    },

    async loadHTML() {
      const { html } = await import('@codemirror/lang-html');
      this.view.dispatch({
        effects: this.langCompartment.reconfigure(html()),
      });
    },

    destroy() {
      if (this.view) {
        this.view.destroy();
        this.view = null;
      }
    },
  };
}
