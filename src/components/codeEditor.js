export function codeEditor({ mode = 'sql', dbType = 'SQLITE', label = '', shortcodes = false, generate = false } = {}) {
  return {
    view: null,
    langCompartment: null,
    formatError: '',
    _formatErrorTimer: null,

    // Natural-language generation state (only meaningful when generate is true).
    generate,
    generationPrompt: '',
    generating: false,
    generationError: '',
    generationStatus: '',
    generatedContent: '',
    generatedValid: null,
    generatedIssues: [],
    _generationRequestId: 0,

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

    // generateFromPrompt asks the server to draft this slot (or MetaSchema) from
    // the natural-language prompt, grounded on the carrier + a sample entity read
    // from the shared templatePreview store. Valid drafts auto-apply when the
    // editor is untouched since the request started; invalid drafts wait for an
    // explicit "Use anyway".
    async generateFromPrompt() {
      const prompt = (this.generationPrompt || '').trim();
      this.generationError = '';
      this.generationStatus = '';
      this.generatedContent = '';
      this.generatedValid = null;
      this.generatedIssues = [];

      if (!prompt) {
        this.generationError = 'Describe what you want first.';
        return;
      }

      const store = (window.Alpine && window.Alpine.store('templatePreview')) || {};
      const generatePath = store.generatePath || '';
      if (!generatePath) {
        this.generationError = 'Generation is unavailable on this form.';
        return;
      }

      const hiddenInput = this.$refs.hiddenInput;
      const fieldName = hiddenInput ? hiddenInput.getAttribute('name') || '' : '';
      const target = fieldName === 'MetaSchema' ? 'metaschema' : 'slot';
      const form = this.$refs.editorContainer.closest('form');
      const metaSchema = (form && form.querySelector('input[name="MetaSchema"]')?.value) || '';

      const requestId = ++this._generationRequestId;
      const snapshot = this.view ? this.view.state.doc.toString() : '';
      this.generating = true;
      this.generationStatus = 'Generating…';

      try {
        const resp = await fetch(generatePath, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            target,
            slot: target === 'slot' ? fieldName : '',
            mode: this.mode,
            content: snapshot,
            metaSchema,
            prompt,
            categoryId: store.categoryId || 0,
            entityId: store.entityId || 0,
          }),
        });
        const data = await resp.json().catch(() => null);
        if (requestId !== this._generationRequestId) return;
        if (!resp.ok) {
          this.generationError = (data && (data.error || data.Error)) || `Generation failed (${resp.status})`;
          this.generationStatus = '';
          return;
        }

        this.generatedContent = (data && data.content) || '';
        this.generatedValid = !!(data && data.valid);
        this.generatedIssues = data && Array.isArray(data.issues) ? data.issues : [];

        if (!this.generatedContent) {
          this.generationError = 'The model returned no content.';
          this.generationStatus = '';
          return;
        }

        if (!this.generatedValid) {
          this.generationStatus = 'Generated content needs review.';
          this.generationError =
            this.generatedIssues.map((i) => i.message).filter(Boolean).join('; ') ||
            'Generated content has issues.';
          return;
        }

        // Auto-apply only when the editor is unchanged since the request started.
        if (this.view && this.view.state.doc.toString() === snapshot) {
          this.applyGenerated();
          this.generationStatus = 'Generated content applied.';
        } else {
          this.generationStatus = 'Generated content is ready.';
        }
      } catch (err) {
        if (requestId !== this._generationRequestId) return;
        this.generationError = err.message || 'Network error';
        this.generationStatus = '';
      } finally {
        if (requestId === this._generationRequestId) this.generating = false;
      }
    },

    applyGenerated() {
      if (!this.generatedContent || !this.view) return;
      this.view.dispatch({
        changes: { from: 0, to: this.view.state.doc.length, insert: this.generatedContent },
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
