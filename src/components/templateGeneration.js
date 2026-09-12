// Shared generation state for a template/CSS pair or a standalone schema/CSS editor.
export function templateGeneration({ fieldName = '', mode = 'html' } = {}) {
  return {
    mode,
    // One prompt and draft state shared by all fields this control generates.
    generationPrompt: '',
    generating: false,
    generationError: '',
    generationStatus: '',
    generatedContent: '',
    generatedSlots: null,
    _generatedForm: null,
    generatedValid: null,
    generatedIssues: [],
    _generationRequestId: 0,

    // Pair controls resolve the HTML editor by name; standalone controls use
    // their own CodeMirror component. Both share the same generation workflow.
    generationContext() {
      const form = fieldName ? this.$el.closest('form') : this.$refs.editorContainer.closest('form');
      const input = fieldName ? form?.querySelector(`input[name="${fieldName}"]`) : this.$refs.hiddenInput;
      const view = fieldName
        ? input?.closest('[x-data]')?.querySelector('[x-ref="editorContainer"]')?._cmView
        : this.view;
      return { form, name: fieldName || input?.getAttribute('name') || '', view };
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
      this.generatedSlots = null;
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

      const { form, name: fieldName, view } = this.generationContext();
      if (!view) {
        this.generationError = 'The editor is still loading. Try again in a moment.';
        return;
      }
      let target = fieldName === 'MetaSchema' ? 'metaschema' : 'slot';
      const base = fieldName.endsWith('CSS') ? fieldName.slice(0, -3) : fieldName;
      const pair = [base, `${base}CSS`];
      if (base !== 'Custom' && pair.every((name) => form?.querySelector(`input[name="${name}"]`))) target = 'cluster';
      const pairSnapshot = Object.fromEntries(pair.map((name) => [name, form?.querySelector(`input[name="${name}"]`)?.value || '']));
      this._generatedForm = form;
      const metaSchema = (form && form.querySelector('input[name="MetaSchema"]')?.value) || '';

      const requestId = ++this._generationRequestId;
      const snapshot = view.state.doc.toString();
      this.generating = true;
      this.generationStatus = 'Generating…';

      try {
        const resp = await fetch(generatePath, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            target,
            slot: target === 'metaschema' ? '' : fieldName,
            mode: this.mode,
            content: target === 'cluster' ? JSON.stringify(pairSnapshot) : snapshot,
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

        if (target === 'cluster' && (!data?.slots || !pair.every((name) => typeof data.slots[name] === 'string'))) {
          this.generationError = 'The model must return both the template and its CSS.';
          this.generationStatus = '';
          return;
        }
        if (target === 'cluster') {
          this.generatedSlots = Object.fromEntries(pair.map((name) => [name, data.slots[name]]));
        }
        this.generatedContent = this.generatedSlots ? this.generatedSlots[fieldName] : (data && data.content) || '';
        this.generatedValid = !!(data && data.valid);
        this.generatedIssues = data && Array.isArray(data.issues) ? data.issues : [];

        if (!this.generatedContent && !this.generatedSlots) {
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
        if (view.state.doc.toString() === snapshot && (target !== 'cluster' || pair.every((name) => form.querySelector(`input[name="${name}"]`)?.value === pairSnapshot[name]))) {
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
      if (this.generatedSlots) {
        const edits = Object.entries(this.generatedSlots).map(([name, content]) => {
          const input = this._generatedForm?.querySelector(`input[name="${name}"]`);
          const view = input?.closest('[x-data]')?.querySelector('[x-ref="editorContainer"]')?._cmView;
          return { view, content };
        });
        if (edits.some(({ view }) => !view)) return;
        for (const { view, content } of edits) view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: content } });
        return;
      }
      const { view } = this.generationContext();
      if (!this.generatedContent || !view) return;
      view.dispatch({
        changes: { from: 0, to: view.state.doc.length, insert: this.generatedContent },
      });
    },

  };
}
