export function codeEditor({ mode = 'sql', dbType = 'SQLITE', label = '', shortcodes = false } = {}) {
  return {
    view: null,
    langCompartment: null,
    formatError: '',
    _formatErrorTimer: null,

    async init() {
      this.mode = mode;
      this.shortcodes = shortcodes;
      const hiddenInput = this.$refs.hiddenInput;
      const container = this.$refs.editorContainer;
      const initialValue = hiddenInput.value || '';
      const fieldName = hiddenInput.getAttribute('name') || '';
      const ariaLabel = label || fieldName;

      // Lazy-load CodeMirror core modules
      const [
        { EditorView, keymap, lineNumbers, highlightActiveLine, highlightActiveLineGutter, drawSelection },
        { EditorState, Compartment },
        { defaultKeymap, history, historyKeymap, indentWithTab },
        { syntaxHighlighting, defaultHighlightStyle, bracketMatching, indentOnInput },
        { autocompletion, closeBrackets, closeBracketsKeymap },
      ] = await Promise.all([
        import('@codemirror/view'),
        import('@codemirror/state'),
        import('@codemirror/commands'),
        import('@codemirror/language'),
        import('@codemirror/autocomplete'),
      ]);

      // Shortcode editor tooling (lint + autocomplete + hover) — loaded only for
      // template slots that carry shortcodes, so plain SQL/JSON editors stay lean.
      // Built here (after the core import) because it needs EditorState.
      let shortcodeExtensions = [];
      let shortcodeMod = null;
      if (shortcodes) {
        try {
          const [lintMod, completionMod] = await Promise.all([
            import('./shortcodeLint.js'),
            import('./shortcodeCompletion.js'),
          ]);
          shortcodeMod = lintMod;
          // Meta-path autocomplete reads the MetaSchema being edited in the same form.
          const form = container.closest('form');
          const schemaProvider = () => {
            const el = form && form.querySelector('input[name="MetaSchema"]');
            return el ? el.value : '';
          };
          shortcodeExtensions = [
            ...lintMod.shortcodeLintExtensions(),
            // Override completion with a combined source: shortcode completions
            // inside [ ... ] brackets, delegating to the html/css source elsewhere.
            autocompletion({ override: [completionMod.shortcodeOverrideSource(mode, schemaProvider)] }),
            completionMod.shortcodeAutoTrigger(),
            completionMod.shortcodeHoverTooltip(),
          ];
        } catch (e) {
          console.error('shortcode editor tooling failed to load', e);
          shortcodeExtensions = [];
        }
      }

      this.langCompartment = new Compartment();

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
        ...shortcodeExtensions,
        EditorView.updateListener.of((update) => {
          if (update.docChanged) {
            hiddenInput.value = update.state.doc.toString();
            // Notify a live preview pane (if present) that this slot changed.
            if (shortcodes) {
              container.dispatchEvent(
                new CustomEvent('template-slot-changed', {
                  bubbles: true,
                  detail: { name: fieldName, value: hiddenInput.value },
                }),
              );
            }
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

      // Make the scrollable region keyboard-focusable (axe: scrollable-region-focusable)
      this.view.scrollDOM.tabIndex = 0;

      // Expose the view on the container for test automation
      container._cmView = this.view;

      // Soft-warn on submit when a shortcode template holds error diagnostics.
      if (shortcodes && shortcodeMod) {
        shortcodeMod.installSubmitGuard(container.closest('form'), this.view);
      }

      // Load language extension asynchronously
      if (mode === 'sql') {
        this.loadSQL(dbType);
      } else if (mode === 'html') {
        this.loadHTML();
      } else if (mode === 'json') {
        this.loadJSON();
      } else if (mode === 'css') {
        this.loadCSS();
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
        effects: this.langCompartment.reconfigure(html({ autoCloseTags: false })),
      });
    },

    async loadJSON() {
      const { json } = await import('@codemirror/lang-json');
      this.view.dispatch({
        effects: this.langCompartment.reconfigure(json()),
      });
    },

    async loadCSS() {
      const { css } = await import('@codemirror/lang-css');
      this.view.dispatch({
        effects: this.langCompartment.reconfigure(css()),
      });
    },

    async formatContent() {
      if (!this.view) return;
      const content = this.view.state.doc.toString();
      if (!content.trim()) return;

      this.formatError = '';
      let formatted;

      try {
        if (this.mode === 'json') {
          formatted = JSON.stringify(JSON.parse(content), null, 2);
        } else if (this.mode === 'html') {
          const { formatHtml } = await import('../utils/formatHtml.js');
          formatted = formatHtml(content);
        } else {
          return;
        }
      } catch (err) {
        this.formatError = err.message || 'Formatting failed';
        if (this._formatErrorTimer) clearTimeout(this._formatErrorTimer);
        this._formatErrorTimer = setTimeout(() => { this.formatError = ''; }, 4000);
        return;
      }

      if (formatted === content) return;

      this.view.dispatch({
        changes: { from: 0, to: this.view.state.doc.length, insert: formatted },
      });
    },

    destroy() {
      if (this._formatErrorTimer) clearTimeout(this._formatErrorTimer);
      if (this.view) {
        this.view.destroy();
        this.view = null;
      }
    },
  };
}
