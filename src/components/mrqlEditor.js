export function mrqlEditor() {
  return {
    view: null,
    langCompartment: null,
    lintCompartment: null,

    // UI state
    showDocs: false,
    showSaveDialog: false,
    executing: false,
    validationError: '',
    error: '',

    // Results
    result: null,

    // Save dialog
    saveName: '',
    saveDescription: '',
    saveError: '',

    // Saved queries (initialized from server-rendered JSON, or fetched)
    savedQueries: [],

    // Query history (localStorage)
    history: [],

    // Validation debounce timer
    _validateTimer: null,

    get totalCount() {
      if (!this.result) return 0;
      return (this.result.resources?.length || 0)
        + (this.result.notes?.length || 0)
        + (this.result.groups?.length || 0);
    },

    async init() {
      // Load history from localStorage
      try {
        const stored = localStorage.getItem('mrql_history');
        if (stored) this.history = JSON.parse(stored);
      } catch (_) { /* ignore */ }

      // Fetch saved queries
      this.fetchSavedQueries();

      const container = this.$refs.editorContainer;

      // Lazy-load CodeMirror core modules
      const [
        { EditorView, keymap, lineNumbers, highlightActiveLine, highlightActiveLineGutter, drawSelection },
        { EditorState, Compartment },
        { defaultKeymap, history, historyKeymap, indentWithTab },
        { syntaxHighlighting, defaultHighlightStyle, bracketMatching, indentOnInput, StreamLanguage },
        { autocompletion, closeBrackets, closeBracketsKeymap },
      ] = await Promise.all([
        import('@codemirror/view'),
        import('@codemirror/state'),
        import('@codemirror/commands'),
        import('@codemirror/language'),
        import('@codemirror/autocomplete'),
      ]);

      this.langCompartment = new Compartment();
      this.lintCompartment = new Compartment();

      // MRQL keywords and operators for syntax highlighting
      const mrqlKeywords = new Set([
        'AND', 'OR', 'NOT', 'IN', 'IS', 'EMPTY', 'NULL',
        'ORDER', 'BY', 'ASC', 'DESC', 'LIMIT', 'OFFSET',
        'TEXT', 'TYPE',
      ]);

      const mrqlLang = StreamLanguage.define({
        token(stream) {
          // Skip whitespace
          if (stream.eatSpace()) return null;

          // Quoted strings
          if (stream.match('"')) {
            while (!stream.eol()) {
              if (stream.next() === '"') break;
            }
            return 'string';
          }

          // Numbers (with optional unit suffix like MB, KB, GB, d, h, m)
          if (stream.match(/^-?\d+(\.\d+)?\s*(MB|KB|GB|TB|ms|[smhdwy])?/i)) {
            return 'number';
          }

          // Operators
          if (stream.match(/^[!~]=?|^[><=]+/)) {
            return 'operator';
          }

          // Parentheses / brackets
          if (stream.match(/^[()]/)) {
            return 'bracket';
          }

          // Comma
          if (stream.match(',')) {
            return 'punctuation';
          }

          // Words — check if keyword
          if (stream.match(/^[a-zA-Z_][a-zA-Z0-9_.]*/, true)) {
            const word = stream.current();
            if (mrqlKeywords.has(word.toUpperCase())) {
              return 'keyword';
            }
            // "type" is a special keyword
            if (word.toLowerCase() === 'type') {
              return 'keyword';
            }
            return 'variableName';
          }

          // Fallback: advance one character
          stream.next();
          return null;
        },
      });

      // Autocompletion source that calls the server
      const mrqlCompletion = async (context) => {
        const pos = context.pos;
        const doc = context.state.doc.toString();

        // Find the word being typed
        const word = context.matchBefore(/[a-zA-Z_.]*/);
        if (!word && !context.explicit) return null;

        try {
          const resp = await fetch('/v1/mrql/complete', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ query: doc, cursor: pos }),
          });
          if (!resp.ok) return null;
          const data = await resp.json();
          if (!data.suggestions || data.suggestions.length === 0) return null;

          return {
            from: word ? word.from : pos,
            options: data.suggestions.map((s) => ({
              label: s.label || s.value,
              apply: s.value,
              type: s.type === 'keyword' ? 'keyword'
                : s.type === 'field' ? 'property'
                : s.type === 'operator' ? 'operator'
                : s.type === 'entity_type' ? 'type'
                : 'text',
            })),
          };
        } catch (_) {
          return null;
        }
      };

      const self = this;

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
        autocompletion({
          override: [mrqlCompletion],
          activateOnTyping: true,
        }),
        keymap.of([
          ...closeBracketsKeymap,
          ...defaultKeymap,
          ...historyKeymap,
          indentWithTab,
          {
            key: 'Mod-Enter',
            run() {
              self.execute();
              return true;
            },
          },
        ]),
        this.langCompartment.of(mrqlLang),
        this.lintCompartment.of([]),
        EditorView.updateListener.of((update) => {
          if (update.docChanged) {
            self.scheduleValidation();
          }
        }),
        EditorView.contentAttributes.of({
          'aria-label': 'MRQL query',
          'data-language': 'mrql',
        }),
        EditorView.theme({
          '&': { minHeight: '120px', maxHeight: '40vh' },
          '.cm-scroller': { overflow: 'auto', minHeight: '120px' },
          '.cm-content': { minHeight: '120px' },
        }),
      ];

      this.view = new EditorView({
        state: EditorState.create({ doc: '', extensions }),
        parent: container,
      });
    },

    getQuery() {
      return this.view ? this.view.state.doc.toString() : '';
    },

    setQuery(text) {
      if (!this.view) return;
      this.view.dispatch({
        changes: { from: 0, to: this.view.state.doc.length, insert: text },
      });
    },

    scheduleValidation() {
      if (this._validateTimer) clearTimeout(this._validateTimer);
      this._validateTimer = setTimeout(() => this.validate(), 500);
    },

    async validate() {
      const query = this.getQuery().trim();
      if (!query) {
        this.validationError = '';
        return;
      }

      try {
        const resp = await fetch('/v1/mrql/validate', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ query }),
        });
        if (!resp.ok) {
          this.validationError = 'Validation request failed';
          return;
        }
        const data = await resp.json();
        if (data.valid) {
          this.validationError = '';
        } else if (data.errors && data.errors.length > 0) {
          this.validationError = data.errors.map((e) => e.message || JSON.stringify(e)).join('; ');
        } else {
          this.validationError = 'Invalid query';
        }
      } catch (_) {
        // Network error — silently ignore
      }
    },

    async execute() {
      const query = this.getQuery().trim();
      if (!query) return;

      this.executing = true;
      this.error = '';
      this.result = null;

      try {
        const resp = await fetch('/v1/mrql', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ query }),
        });

        if (!resp.ok) {
          const errData = await resp.json().catch(() => null);
          this.error = errData?.error || errData?.Error || `Request failed (${resp.status})`;
          return;
        }

        this.result = await resp.json();
        this.addToHistory(query);
      } catch (err) {
        this.error = err.message || 'Network error';
      } finally {
        this.executing = false;
      }
    },

    addToHistory(query) {
      // Remove duplicate if exists, prepend
      this.history = [query, ...this.history.filter((h) => h !== query)].slice(0, 20);
      try {
        localStorage.setItem('mrql_history', JSON.stringify(this.history));
      } catch (_) { /* quota exceeded — ignore */ }
    },

    loadFromHistory(query) {
      this.setQuery(query);
    },

    async fetchSavedQueries() {
      try {
        const resp = await fetch('/v1/mrql/saved');
        if (resp.ok) {
          this.savedQueries = await resp.json();
          if (!Array.isArray(this.savedQueries)) this.savedQueries = [];
        }
      } catch (_) { /* ignore */ }
    },

    loadSavedQuery(q) {
      this.setQuery(q.query);
    },

    async saveQuery() {
      const query = this.getQuery().trim();
      if (!query || !this.saveName.trim()) return;

      this.saveError = '';

      try {
        const resp = await fetch('/v1/mrql/saved', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            name: this.saveName.trim(),
            query: query,
            description: this.saveDescription.trim(),
          }),
        });

        if (!resp.ok) {
          const errData = await resp.json().catch(() => null);
          this.saveError = errData?.error || errData?.Error || `Save failed (${resp.status})`;
          return;
        }

        this.showSaveDialog = false;
        this.saveName = '';
        this.saveDescription = '';
        this.saveError = '';
        await this.fetchSavedQueries();
      } catch (err) {
        this.saveError = err.message || 'Network error';
      }
    },

    async deleteSavedQuery(id) {
      try {
        const resp = await fetch('/v1/mrql/saved/delete?id=' + id, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ id }),
        });

        if (resp.ok) {
          await this.fetchSavedQueries();
        }
      } catch (_) { /* ignore */ }
    },

    destroy() {
      if (this._validateTimer) clearTimeout(this._validateTimer);
      if (this.view) {
        this.view.destroy();
        this.view = null;
      }
    },
  };
}
