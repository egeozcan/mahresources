import { askToConfirm } from './confirmDialog.js';
import { changedSince, draftSegments, draftWarnings } from './templateDraftReview.js';

// The CodeMirror view behind a field's editor container, or a promise of it: the editor
// mounts from a lazy import, and a prompt can be submitted before that lands. Settles null
// for an editor that could not be built.
function editorFor(container) {
  return Promise.resolve(container?._cmView || container?._cmReady || null);
}

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
    // The pending draft, one { name, text, css } per field it would write. It is
    // shown as text in the review panel and reaches no editor before Apply.
    generatedFields: [],
    _draftSnapshot: {},
    _generationRequestId: 0,

    // A method, not a getter: codeEditor spreads this object into its own, and a
    // spread copies a getter's value once instead of the getter.
    hasDraft() {
      return this.generatedFields.length > 0;
    },

    // Pair controls resolve the HTML editor by name; standalone controls use
    // their own CodeMirror component. Both share the same generation workflow.
    generationContext() {
      const form = fieldName ? this.$el.closest('form') : this.$refs.editorContainer.closest('form');
      const input = fieldName ? form?.querySelector(`input[name="${fieldName}"]`) : this.$refs.hiddenInput;
      const container = fieldName
        ? input?.closest('[x-data]')?.querySelector('[x-ref="editorContainer"]')
        : this.$refs.editorContainer;
      const view = fieldName ? container?._cmView : this.view;
      return { form, name: fieldName || input?.getAttribute('name') || '', view, container };
    },

    // generateFromPrompt asks the server to draft this slot (or MetaSchema) from
    // the natural-language prompt, grounded on the carrier + a sample entity read
    // from the shared templatePreview store. Every draft, valid or not, is held
    // for review as text and reaches the editors only on Apply: an applied draft
    // refreshes the live preview, which runs its scripts against the entity.
    async generateFromPrompt() {
      // Every submission replaces whatever came before it, including one about to be
      // refused below: a request still waiting for its editors, or still in flight, must
      // not land a draft after the reader has asked for something else.
      const requestId = ++this._generationRequestId;
      this.generating = false;
      const prompt = (this.generationPrompt || '').trim();
      this.generationError = '';
      this.generationStatus = '';
      this.generatedContent = '';
      this.generatedSlots = null;
      this.generatedValid = null;
      this.generatedIssues = [];
      this.generatedFields = [];
      this._draftSnapshot = {};

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

      const { form, name: fieldName, container } = this.generationContext();
      let target = fieldName === 'MetaSchema' ? 'metaschema' : 'slot';
      const base = fieldName.endsWith('CSS') ? fieldName.slice(0, -3) : fieldName;
      const pair = [base, `${base}CSS`];
      if (base !== 'Custom' && pair.every((name) => form?.querySelector(`input[name="${name}"]`))) target = 'cluster';

      // Every editor the draft will be written into has to exist before the request goes
      // out: the snapshot Apply checks for edits made meanwhile is read from them. A pair
      // needs its CSS editor too, or the draft would be dropped at apply. A prompt
      // submitted while they are still mounting waits for them rather than being refused,
      // and says so. The control stays usable while it waits, so a load that stalls reads
      // as a wait rather than as a generation in progress, and submitting again replaces
      // the waiting request.
      const editors = target === 'cluster'
        ? pair.map((name) => form.querySelector(`input[name="${name}"]`)?.closest('[x-data]')?.querySelector('[x-ref="editorContainer"]'))
        : [container];
      let views = editors.map((editor) => editor?._cmView || null);
      if (views.some((editorView) => !editorView)) {
        this.generationStatus = 'Waiting for the editor to load…';
        views = await Promise.all(editors.map(editorFor));
        if (requestId !== this._generationRequestId) return;
        if (views.some((editorView) => !editorView)) {
          this.generationError = 'The editor could not be loaded. Reload the page to try again.';
          this.generationStatus = '';
          return;
        }
      }
      this.generating = true;
      this.generationStatus = 'Generating…';
      const view = target === 'cluster' ? views[pair.indexOf(fieldName)] : views[0];
      const pairSnapshot = Object.fromEntries(pair.map((name) => [name, form?.querySelector(`input[name="${name}"]`)?.value || '']));
      this._generatedForm = form;
      const metaSchema = (form && form.querySelector('input[name="MetaSchema"]')?.value) || '';
      const snapshot = view.state.doc.toString();

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

        // Issues are read first: a truncated reply can arrive with half a pair or
        // nothing at all, and its diagnostic is what the reader needs to see.
        this.generatedIssues = data && Array.isArray(data.issues) ? data.issues : [];
        const issueText = this.generatedIssues.map((i) => i.message).filter(Boolean).join('; ');
        const completePair = target === 'cluster' && data?.slots && pair.every((name) => typeof data.slots[name] === 'string');
        if (target === 'cluster' && !completePair && !issueText) {
          this.generationError = 'The model must return both the template and its CSS.';
          this.generationStatus = '';
          return;
        }
        if (completePair) {
          this.generatedSlots = Object.fromEntries(pair.map((name) => [name, data.slots[name]]));
        }
        this.generatedContent = this.generatedSlots ? this.generatedSlots[fieldName] : (target !== 'cluster' && data && data.content) || '';
        this.generatedValid = !!(data && data.valid);

        if (!this.generatedContent && !this.generatedSlots) {
          this.generationError = issueText || 'The model returned no content.';
          this.generationStatus = '';
          return;
        }

        const fields = this.generatedSlots ? pair : [fieldName];
        this._draftSnapshot = this.generatedSlots ? pairSnapshot : { [fieldName]: snapshot };
        this.generatedFields = fields.map((name) => ({
          name,
          text: this.generatedSlots ? this.generatedSlots[name] : this.generatedContent,
          css: name.endsWith('CSS') || this.mode === 'css',
        }));
        if (!this.generatedValid) {
          this.generationStatus = 'Generated draft needs review. Read it below; Apply writes it anyway.';
          this.generationError = issueText || 'Generated content has issues.';
          return;
        }
        this.generationStatus = 'Generated draft is ready. Read it below, then Apply.';
      } catch (err) {
        if (requestId !== this._generationRequestId) return;
        this.generationError = err.message || 'Network error';
        this.generationStatus = '';
      } finally {
        if (requestId === this._generationRequestId) this.generating = false;
      }
    },

    // The editor view a draft field would be written into.
    _viewFor(name) {
      if (!this.generatedSlots) return this.generationContext().view;
      const input = this._generatedForm?.querySelector(`input[name="${name}"]`);
      return input?.closest('[x-data]')?.querySelector('[x-ref="editorContainer"]')?._cmView || null;
    },

    // applyGenerated writes the pending draft. A field edited since the request
    // started (typing, a preset, another generation) is named and the user asked
    // first; cancelling keeps their text.
    async applyGenerated() {
      if (!this.hasDraft()) return;
      const draft = this.generatedFields;
      const edits = draft.map(({ name, text }) => ({ name, text, view: this._viewFor(name) }));
      if (edits.some(({ view }) => !view)) return;
      const views = Object.fromEntries(edits.map(({ name, view }) => [name, view]));
      const changed = changedSince(this._draftSnapshot, edits.map(({ name }) => name), (name) => views[name].state.doc.toString());
      if (changed.length) {
        const confirmed = await askToConfirm(
          `${changed.join(' and ')} changed since you asked for this draft. Replace ${changed.length === 1 ? 'it' : 'them'} with the draft?`,
          { title: 'Replace your edits?', confirmLabel: 'Replace', cancelLabel: 'Keep my edits' },
        );
        if (!confirmed || this.generatedFields !== draft) return;
      }
      for (const { view, text } of edits) view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: text } });
      this.discardGenerated();
      this.generationStatus = 'Generated draft applied.';
    },

    discardGenerated() {
      this.generatedFields = [];
      this._draftSnapshot = {};
      this.generatedContent = '';
      this.generatedSlots = null;
      this.generatedValid = null;
      this.generatedIssues = [];
      this.generationError = '';
      this.generationStatus = '';
    },

    draftSegments,
    draftWarnings,
  };
}
