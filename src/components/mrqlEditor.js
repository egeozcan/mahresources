import * as userSettings from '../userSettings.js';

export function mrqlEditor() {
  return {
    view: null,
    langCompartment: null,

    // UI state
    showDocs: false,
    showSaveDialog: false,
    executing: false,
    validationError: '',
    error: '',
    generationPrompt: '',
    generating: false,
    generationError: '',
    generationStatus: '',
    generatedQuery: '',
    generatedExplanation: '',
    generatedValid: null,
    generatedErrors: [],

    // Results
    result: null,

    // Package 4: parameter placeholders ($name) derived from the validate
    // response, plus their current values. paramValues is preserved while the
    // placeholder set is unchanged.
    params: [],
    paramValues: {},

    // Package 4: EXPLAIN panel state.
    explainResult: null,
    showExplain: false,
    explaining: false,

    // Package 4: export in-flight flag.
    exporting: false,

    // BH-013: banner state, driven by the response's default_limit_applied flag.
    defaultLimitApplied: false,
    appliedLimit: 0,

    // Save dialog
    saveName: '',
    saveDescription: '',
    saveError: '',

    // BH-012: tracks the saved query currently loaded into the editor so Save
    // can branch between PUT (update) and POST (create). Cleared when the
    // editor content diverges from the loaded query (handled via watcher).
    loadedSavedQueryId: null,
    loadedSavedQueryName: '',

    // Saved queries (initialized from server-rendered JSON, or fetched)
    savedQueries: [],

    // Query history (server-backed user setting "mrqlHistory")
    history: [],
    // Gate persistence until the server history has hydrated, so queries run before the
    // load resolves are merged with — not overwritten onto — the saved history.
    _historyLoaded: false,

    // Validation debounce timer
    _validateTimer: null,
    _generationRequestId: 0,
    _generationEditorSnapshot: '',

    get totalCount() {
      if (!this.result) return 0;
      if (this.result.mode === 'aggregated') return this.result.rows?.length || 0;
      if (this.result.mode === 'bucketed') {
        return (this.result.groups || []).reduce((sum, g) => sum + (g.items?.length || 0), 0);
      }
      return (this.result.resources?.length || 0)
        + (this.result.notes?.length || 0)
        + (this.result.groups?.length || 0);
    },

    // BH-012: surface the Update affordance only when a saved query is loaded
    // and the editor has a non-empty, valid query.
    get canUpdate() {
      if (!this.loadedSavedQueryId) return false;
      if (this.validationError) return false;
      return this.getQuery().trim().length > 0;
    },

    async init() {
      // Load MRQL history from the server-backed user-settings store (non-blocking so the
      // editor renders immediately; history fills in reactively when it resolves).
      userSettings.whenLoaded().then(() => {
        const stored = userSettings.get('mrqlHistory');
        if (Array.isArray(stored) && stored.length) {
          // Merge any queries run before load with the stored history (current first).
          this.history = [...this.history, ...stored]
            .filter((q, i, a) => a.indexOf(q) === i)
            .slice(0, 20);
        }
        this._historyLoaded = true;
        // Persist the merge if the user already ran a query before load resolved.
        if (this.history.length) userSettings.set('mrqlHistory', this.history);
      });

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
          {
            key: 'Mod-Enter',
            run() {
              self.execute();
              return true;
            },
          },
          {
            key: 'Ctrl-Enter',
            run() {
              self.execute();
              return true;
            },
          },
          {
            key: 'Mod-Shift-Enter',
            run() {
              self.explain();
              return true;
            },
          },
          {
            key: 'Ctrl-Shift-Enter',
            run() {
              self.explain();
              return true;
            },
          },
          ...closeBracketsKeymap,
          ...defaultKeymap,
          ...historyKeymap,
          indentWithTab,
        ]),
        this.langCompartment.of(mrqlLang),
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

      // Expose the view on the container for test automation
      container._cmView = this.view;

      // Load query from URL if present, and auto-execute
      const params = new URLSearchParams(window.location.search);
      const urlQuery = params.get('q');
      const savedId = params.get('saved');
      if (urlQuery) {
        this.setQuery(urlQuery);
        this.execute({ pushState: false });
      } else if (savedId) {
        // Package 5c: ?saved=<id> loads a saved query (found via global search).
        // loadSavedQuery handles parameterized queries (focuses the first empty
        // param input instead of auto-running).
        this.loadSavedQueryById(savedId);
      }

      // Handle back/forward navigation
      this._popstateHandler = () => {
        const q = new URLSearchParams(window.location.search).get('q');
        if (q) {
          this.setQuery(q);
          this.execute({ pushState: false });
        } else {
          this.setQuery('');
          this.result = null;
          this.error = '';
        }
      };
      window.addEventListener('popstate', this._popstateHandler);
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
        this.syncParams(data.params || []);
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

    // syncParams reconciles the placeholder list from a validate response with
    // the current param inputs, preserving already-entered values while the
    // placeholder set is unchanged.
    syncParams(names) {
      names = Array.isArray(names) ? names : [];
      const unchanged = names.length === this.params.length
        && names.every((n, i) => n === this.params[i]);
      if (unchanged) return;
      const next = {};
      for (const n of names) {
        next[n] = Object.prototype.hasOwnProperty.call(this.paramValues, n)
          ? this.paramValues[n] : '';
      }
      this.paramValues = next;
      this.params = names;
    },

    // paramsPayload builds the params object to send with execute/explain/export.
    // Empty inputs are omitted so the server reports them as missing (400) rather
    // than binding an empty string.
    paramsPayload() {
      const out = {};
      for (const n of this.params) {
        const v = this.paramValues[n];
        if (v !== undefined && v !== null && v !== '') out[n] = v;
      }
      return out;
    },

    // focusFirstEmptyParam moves focus to the first param input without a value.
    focusFirstEmptyParam() {
      this.$nextTick(() => {
        for (const n of this.params) {
          if (!this.paramValues[n]) {
            const el = document.getElementById('mrql-param-' + n);
            if (el) { el.focus(); return; }
          }
        }
      });
    },

    async generateFromPrompt() {
      const prompt = this.generationPrompt.trim();
      this.generationError = '';
      this.generationStatus = '';
      this.generatedQuery = '';
      this.generatedExplanation = '';
      this.generatedValid = null;
      this.generatedErrors = [];

      if (!prompt) {
        this.generationError = 'Describe what results you want first.';
        this.$nextTick(() => this.$refs.generationPrompt?.focus());
        return;
      }

      const requestId = ++this._generationRequestId;
      const editorSnapshot = this.getQuery();
      this._generationEditorSnapshot = editorSnapshot;
      this.generating = true;
      this.generationStatus = 'Generating...';

      try {
        const resp = await fetch('/v1/mrql/generate', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ prompt }),
        });
        const data = await resp.json().catch(() => null);
        if (requestId !== this._generationRequestId) return;
        if (!resp.ok) {
          this.generationError = data?.error || data?.Error || `Generation failed (${resp.status})`;
          this.generationStatus = '';
          return;
        }

        this.generatedQuery = data?.query || '';
        this.generatedExplanation = data?.explanation || '';
        this.generatedValid = !!data?.valid;
        this.generatedErrors = Array.isArray(data?.errors) ? data.errors : [];

        if (!this.generatedValid) {
          this.generationStatus = 'Generated query needs review.';
          this.generationError = this.generatedErrors.map((e) => e.message || JSON.stringify(e)).join('; ') || 'Generated query is invalid.';
          return;
        }

        if (this.getQuery() !== editorSnapshot) {
          this.generationStatus = 'Generated query is ready.';
          return;
        }

        this.applyGeneratedQuery();
        this.generationStatus = 'Generated query is ready.';
      } catch (err) {
        if (requestId !== this._generationRequestId) return;
        this.generationError = err.message || 'Network error';
        this.generationStatus = '';
      } finally {
        if (requestId === this._generationRequestId) this.generating = false;
      }
    },

    applyGeneratedQuery() {
      if (!this.generatedQuery) return;
      this.setQuery(this.generatedQuery);
      this.clearLoadedSaved();
      this.result = null;
      this.error = '';
      this.defaultLimitApplied = false;
      this.appliedLimit = 0;
      this.scheduleValidation();
    },

    async execute({ pushState = true } = {}) {
      const query = this.getQuery().trim();
      if (!query) return;

      this.executing = true;
      this.error = '';
      this.result = null;
      // BH-013: clear the banner state at the start of each request.
      this.defaultLimitApplied = false;
      this.appliedLimit = 0;

      try {
        const resp = await fetch('/v1/mrql?render=1', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ query, params: this.paramsPayload() }),
        });

        if (!resp.ok) {
          const errData = await resp.json().catch(() => null);
          this.error = errData?.error || errData?.Error || `Request failed (${resp.status})`;
          return;
        }

        this.result = await resp.json();
        // BH-013: capture the default-limit signal from the response payload.
        this.defaultLimitApplied = !!(this.result && this.result.default_limit_applied);
        this.appliedLimit = (this.result && this.result.applied_limit) || 0;
        this.addToHistory(query);

        // Update URL so back/forward works (skip if already the same query)
        if (pushState) {
          const currentQ = new URLSearchParams(window.location.search).get('q');
          if (currentQ !== query) {
            const url = new URL(window.location);
            url.searchParams.set('q', query);
            window.history.pushState({ q: query }, '', url);
          }
        }
      } catch (err) {
        this.error = err.message || 'Network error';
      } finally {
        this.executing = false;
      }
    },

    // explain calls /v1/mrql/explain and shows the SQL panel. Bound to the
    // Explain button and Mod-Shift-Enter (Run stays Mod-Enter).
    async explain() {
      const query = this.getQuery().trim();
      if (!query) return;

      this.explaining = true;
      this.error = '';
      try {
        const resp = await fetch('/v1/mrql/explain', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ query, params: this.paramsPayload() }),
        });
        if (!resp.ok) {
          const errData = await resp.json().catch(() => null);
          this.error = errData?.error || errData?.Error || `Explain failed (${resp.status})`;
          return;
        }
        this.explainResult = await resp.json();
        this.showExplain = true;
      } catch (err) {
        this.error = err.message || 'Network error';
      } finally {
        this.explaining = false;
      }
    },

    // exportResults re-submits the current query + params to /v1/mrql/export and
    // triggers a file download via a blob URL.
    async exportResults(format) {
      const query = this.getQuery().trim();
      if (!query) return;

      this.exporting = true;
      this.error = '';
      try {
        const resp = await fetch('/v1/mrql/export?format=' + encodeURIComponent(format), {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ query, params: this.paramsPayload() }),
        });
        if (!resp.ok) {
          const errData = await resp.json().catch(() => null);
          this.error = errData?.error || errData?.Error || `Export failed (${resp.status})`;
          return;
        }
        const blob = await resp.blob();
        let filename = 'mrql-export.' + format;
        const cd = resp.headers.get('Content-Disposition') || '';
        const m = /filename="?([^";]+)"?/.exec(cd);
        if (m) filename = m[1];
        const url = URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = url;
        a.download = filename;
        document.body.appendChild(a);
        a.click();
        a.remove();
        URL.revokeObjectURL(url);
      } catch (err) {
        this.error = err.message || 'Network error';
      } finally {
        this.exporting = false;
      }
    },

    // copyText copies a statement's SQL to the clipboard (best-effort).
    async copyText(text) {
      try {
        await navigator.clipboard.writeText(text);
      } catch (_) { /* clipboard unavailable — ignore */ }
    },

    addToHistory(query) {
      // Remove duplicate if exists, prepend
      this.history = [query, ...this.history.filter((h) => h !== query)].slice(0, 20);
      // Persist only after the server history has loaded, so an early query cannot
      // overwrite saved history (it is merged in on load instead).
      if (this._historyLoaded) userSettings.set('mrqlHistory', this.history);
    },

    loadFromHistory(query) {
      this.setQuery(query);
      this.execute();
    },

    async fetchSavedQueries() {
      try {
        const resp = await fetch('/v1/mrql/saved?all=1');
        if (resp.ok) {
          this.savedQueries = await resp.json();
          if (!Array.isArray(this.savedQueries)) this.savedQueries = [];
        }
      } catch (_) { /* ignore */ }
    },

    // loadSavedQueryById fetches a single saved query by id and loads it,
    // used for the ?saved=<id> deep link from global search (package 5c).
    async loadSavedQueryById(id) {
      try {
        const resp = await fetch('/v1/mrql/saved?id=' + encodeURIComponent(id));
        if (!resp.ok) return;
        const q = await resp.json();
        if (q && (q.query || q.Query)) {
          this.loadSavedQuery(q);
        }
      } catch (_) { /* ignore */ }
    },

    loadSavedQuery(q) {
      this.setQuery(q.query);
      // BH-012: remember which saved query we loaded so Save can branch to PUT.
      this.loadedSavedQueryId = q.id ?? q.ID ?? null;
      this.loadedSavedQueryName = q.name ?? q.Name ?? '';
      // Package 4: a parameterized saved query is not auto-run (that would 400
      // on the unbound params); instead surface the inputs and focus the first.
      const names = q.params ?? q.Params ?? [];
      this.syncParams(names);
      if (names.length > 0) {
        this.focusFirstEmptyParam();
      } else {
        this.execute();
      }
    },

    // BH-012: reset loaded-saved-query tracking so the next Save acts as a
    // fresh create (POST) rather than an update of the previously loaded row.
    clearLoadedSaved() {
      this.loadedSavedQueryId = null;
      this.loadedSavedQueryName = '';
    },

    // BH-012: PUT branch — reuses the loaded saved-query id. Does NOT open
    // the dialog; the name is preserved.
    async updateQuery() {
      if (!this.loadedSavedQueryId) return;
      const query = this.getQuery().trim();
      if (!query) return;
      if (this.validationError) {
        this.saveError = 'Fix syntax errors before updating';
        return;
      }
      this.saveError = '';

      try {
        const resp = await fetch('/v1/mrql/saved?id=' + encodeURIComponent(this.loadedSavedQueryId), {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            name: this.loadedSavedQueryName,
            query: query,
            description: '',
          }),
        });
        if (!resp.ok) {
          const errData = await resp.json().catch(() => null);
          this.saveError = errData?.error || errData?.Error || `Update failed (${resp.status})`;
          return;
        }
        await this.fetchSavedQueries();
      } catch (err) {
        this.saveError = err.message || 'Network error';
      }
    },

    async saveQuery() {
      const query = this.getQuery().trim();
      if (!query || !this.saveName.trim()) return;

      if (this.validationError) {
        this.saveError = 'Fix syntax errors before saving';
        return;
      }

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

        // BH-012: save-as-new means the current editor contents are now a
        // brand-new saved query — clear the loaded-id so the next Update
        // button reference targets nothing (UI hides the button).
        this.clearLoadedSaved();

        this.showSaveDialog = false;
        this.saveName = '';
        this.saveDescription = '';
        this.saveError = '';
        await this.fetchSavedQueries();
      } catch (err) {
        this.saveError = err.message || 'Network error';
      }
    },

    async deleteSavedQuery(id, name) {
      if (!window.confirm('Delete saved query "' + (name || id) + '"?')) return;
      try {
        const resp = await fetch('/v1/mrql/saved/delete?id=' + id, {
          method: 'POST',
        });

        if (resp.ok) {
          // BH-012: if the deleted row is the one currently loaded into the
          // editor, clear the loaded-id so Update no longer offers to PUT
          // against a non-existent row.
          if (this.loadedSavedQueryId && Number(this.loadedSavedQueryId) === Number(id)) {
            this.clearLoadedSaved();
          }
          await this.fetchSavedQueries();
        }
      } catch (_) { /* ignore */ }
    },

    destroy() {
      if (this._validateTimer) clearTimeout(this._validateTimer);
      if (this._popstateHandler) {
        window.removeEventListener('popstate', this._popstateHandler);
      }
      if (this.view) {
        this.view.destroy();
        this.view = null;
      }
    },
  };
}
