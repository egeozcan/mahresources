// templateBundle powers the "Copy from…", Export, Import, and "Start from preset"
// tools on the three category-template edit forms (Category, Resource Category,
// Note Type). It is a client-side form-filling aid only: nothing is saved until
// the user submits the form. It fills the CodeMirror slot editors, the Meta JSON
// Schema editor, and (same-carrier only) the Section Config component.
//
// Bundle shape (schemaVersion 1), shared with server/template_presets/*.json and
// the export download:
//   { schemaVersion, carrier, name, description,
//     slots: { header, sidebar, summary, avatar, listHeader, mrqlResult, css },
//     metaSchema, sectionConfig }

// Slot key -> hidden-input field name on the form. listHeader is additive to the
// schemaVersion-1 bundle: older bundles simply omit it and import leaves it empty.
const SLOT_FIELDS = {
  header: 'CustomHeader',
  sidebar: 'CustomSidebar',
  summary: 'CustomSummary',
  avatar: 'CustomAvatar',
  listHeader: 'CustomListHeader',
  mrqlResult: 'CustomMRQLResult',
  css: 'CustomCSS',
};

// Carrier -> display label + list endpoint used to populate the "Copy from" picker.
const CARRIERS = {
  category: { label: 'Category', list: '/v1/categories' },
  resourceCategory: { label: 'Resource Category', list: '/v1/resourceCategories' },
  noteType: { label: 'Note Type', list: '/v1/note/noteTypes' },
};

const BUNDLE_SCHEMA_VERSION = 1;

// constants.MaxResultsPerPage on the server. A full page means there may be more.
const PAGE_SIZE = 50;
// Bound on how far loadSources() will page, so a pathological taxonomy cannot
// turn opening a form into an unbounded fetch loop. Hitting it is reported.
const SOURCE_PAGE_LIMIT = 20;

// fetchAllPages walks ?page=1..N until a short page arrives, and reports whether
// it reached the end or stopped at the page cap.
export async function fetchAllPages(url, fetchImpl = fetch) {
  const items = [];
  for (let page = 1; page <= SOURCE_PAGE_LIMIT; page++) {
    let batch;
    try {
      const sep = url.includes('?') ? '&' : '?';
      const resp = await fetchImpl(`${url}${sep}page=${page}`, { headers: { Accept: 'application/json' } });
      if (!resp.ok) return { items, complete: true };
      const data = await resp.json();
      batch = Array.isArray(data) ? data : [];
    } catch {
      // Network failure mid-walk: keep what arrived rather than losing the lot.
      return { items, complete: true };
    }
    items.push(...batch);
    if (batch.length < PAGE_SIZE) return { items, complete: true };
  }
  return { items, complete: false };
}

export function templateBundle({ carrier } = {}) {
  return {
    carrier,
    carriers: CARRIERS,
    // sources[carrier] = array of full entity objects (from the list endpoint).
    sources: { category: [], resourceCategory: [], noteType: [] },
    presets: [],
    copyChoice: '',
    presetChoice: '',
    message: '',
    messageKind: 'info', // 'info' | 'warn'
    loading: false,
    generationPrompt: '',
    generating: false,

    async init() {
      await Promise.all([this.loadSources(), this.loadPresets()]);
    },

    notify(msg, kind = 'info') {
      this.message = msg;
      this.messageKind = kind;
    },

    // ---- Editor plumbing ----------------------------------------------------

    // setEditor fills a CodeMirror-backed hidden input by name. Dispatching a
    // doc-replace transaction on the editor view updates the hidden input and
    // fires the template-slot-changed event, so the live preview refreshes too.
    setEditor(name, value) {
      const input = document.querySelector(`[name="${name}"]`);
      if (!input) return;
      const wrapper = input.closest('[x-data]');
      const container = wrapper && wrapper.querySelector('[x-ref="editorContainer"]');
      const view = container && container._cmView;
      if (view) {
        view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: value || '' } });
      } else {
        input.value = value || '';
        input.dispatchEvent(new Event('input', { bubbles: true }));
      }
    },

    getEditor(name) {
      const input = document.querySelector(`[name="${name}"]`);
      return input ? input.value : '';
    },

    // setSectionConfig updates the sectionConfigForm Alpine component's reactive
    // config so its checkboxes and the hidden SectionConfig input update. Shapes
    // differ per carrier, so this is only called for same-carrier fills.
    setSectionConfig(jsonStr) {
      if (!jsonStr) return;
      const el = document.querySelector('[x-data^="sectionConfigForm"]');
      if (!el || !window.Alpine) return;
      let parsed;
      try {
        parsed = JSON.parse(jsonStr);
      } catch {
        return;
      }
      if (!parsed || typeof parsed !== 'object') return;
      const data = window.Alpine.$data(el);
      if (data && data.config) {
        // Merge so any keys absent from the source keep their defaults.
        Object.assign(data.config, parsed);
      }
    },

    // ---- Bundle assembly / application -------------------------------------

    // entityToBundle normalizes a full entity object (from a list endpoint) into
    // the bundle shape.
    entityToBundle(obj, sourceCarrier) {
      const slots = {};
      for (const [key, field] of Object.entries(SLOT_FIELDS)) {
        slots[key] = obj[field] || '';
      }
      // Carrier JSON tags disagree: Category/Resource Category serialize this as
      // `sectionConfig` (lowercase json tag), Note Type as `SectionConfig` (no
      // tag, so the Go field name). Accept either so copy-from keeps the section
      // layout for all three carriers.
      let sectionConfig = obj.SectionConfig ?? obj.sectionConfig;
      if (sectionConfig && typeof sectionConfig !== 'string') {
        sectionConfig = JSON.stringify(sectionConfig);
      }
      return {
        schemaVersion: BUNDLE_SCHEMA_VERSION,
        carrier: sourceCarrier,
        name: obj.Name || '',
        description: obj.Description || '',
        slots,
        metaSchema: obj.MetaSchema || '',
        sectionConfig: sectionConfig || '',
      };
    },

    // currentBundle reads the current editor contents into a bundle for export.
    currentBundle() {
      const slots = {};
      for (const [key, field] of Object.entries(SLOT_FIELDS)) {
        slots[key] = this.getEditor(field);
      }
      const nameInput = document.querySelector('[name="Name"], [name="name"]');
      const descInput = document.querySelector('[name="Description"]');
      return {
        schemaVersion: BUNDLE_SCHEMA_VERSION,
        carrier: this.carrier,
        name: nameInput ? nameInput.value : '',
        description: descInput ? descInput.value : '',
        slots,
        metaSchema: this.getEditor('MetaSchema'),
        sectionConfig: this.getEditor('SectionConfig'),
      };
    },

    // Finding 154: applying a preset replaced authored template content with no
    // prompt. It is recoverable — Cmd+Z in the editor restores it and nothing is
    // persisted until Save — but nothing said so, and a preset applied by
    // accident looks exactly like losing the work. Named per the campaign's
    // confirmation-wording decision: say what will be replaced and how much.
    //
    // Returns true when the caller may proceed. Empty slots need no prompt at
    // all, which is the common case on a create form.
    confirmOverwrite(sourceLabel) {
      const filled = Object.values(SLOT_FIELDS)
        .filter((field) => this.getEditor(field).trim() !== '');
      if (this.getEditor('MetaSchema').trim() !== '') filled.push('MetaSchema');
      if (filled.length === 0) return true;
      const what = filled.length === 1 ? '1 template field' : `${filled.length} template fields`;
      return window.confirm(
        `Replace ${what} with ${sourceLabel}? The current content is discarded (undo in the editor still works, and nothing is saved until you press Save).`,
      );
    },

    // applyBundle fills the form from a bundle. Slots and metaSchema always
    // apply; SectionConfig only applies when the source carrier matches (shapes
    // differ per carrier). Name/description are left untouched — this is a
    // template aid, not a rename.
    applyBundle(bundle) {
      if (!bundle || typeof bundle !== 'object') {
        this.notify('Not a valid template bundle.', 'warn');
        return;
      }
      if (typeof bundle.schemaVersion === 'number' && bundle.schemaVersion > BUNDLE_SCHEMA_VERSION) {
        this.notify(`Bundle schema version ${bundle.schemaVersion} is newer than supported (${BUNDLE_SCHEMA_VERSION}).`, 'warn');
        return;
      }
      const slots = bundle.slots || {};
      for (const [key, field] of Object.entries(SLOT_FIELDS)) {
        this.setEditor(field, slots[key] || '');
      }
      this.setEditor('MetaSchema', bundle.metaSchema || '');

      const sameCarrier = !bundle.carrier || bundle.carrier === this.carrier;
      if (sameCarrier) {
        this.setSectionConfig(bundle.sectionConfig || '');
        this.notify('Filled the form from the selected template. Review, then save.', 'info');
      } else {
        this.notify(
          `Filled shared fields from a ${CARRIERS[bundle.carrier] ? CARRIERS[bundle.carrier].label : bundle.carrier} template. Section config was skipped (shapes differ per carrier).`,
          'warn',
        );
      }
    },

    // ---- Copy from an existing category ------------------------------------

    // Finding 28: this issued one un-paged GET per carrier and the list
    // endpoints return at most constants.MaxResultsPerPage (50) rows, ignoring
    // any maxResults parameter. On an instance with 73 categories and 67 note
    // types the picker offered 50 of each and the other 40 could never be used
    // as a copy source, with nothing on screen saying the list was cut off.
    // The endpoints do honour ?page=, so page until a short page arrives.
    async loadSources() {
      const entries = Object.entries(CARRIERS);
      const truncated = [];
      await Promise.all(
        entries.map(async ([key, cfg]) => {
          const { items, complete } = await fetchAllPages(cfg.list);
          this.sources[key] = items;
          if (!complete) truncated.push(cfg.label);
        }),
      );
      if (truncated.length) {
        this.notify(
          `Showing the first ${SOURCE_PAGE_LIMIT * PAGE_SIZE} of the available ${truncated.join(' / ')} templates. Copy from a more specific one if the source you want is missing.`,
          'warn',
        );
      }
    },

    copyFrom() {
      if (!this.copyChoice) return;
      const sep = this.copyChoice.indexOf(':');
      const sourceCarrier = this.copyChoice.slice(0, sep);
      const id = Number(this.copyChoice.slice(sep + 1));
      const list = this.sources[sourceCarrier] || [];
      const obj = list.find((o) => o.ID === id || o.id === id);
      if (!obj) {
        this.notify('Could not find the selected source.', 'warn');
        return;
      }
      // Finding 154: copy clobbers exactly as hard as a preset does.
      if (!this.confirmOverwrite(`the template from "${obj.Name || 'the selected source'}"`)) return;
      this.applyBundle(this.entityToBundle(obj, sourceCarrier));
    },

    // ---- Export ------------------------------------------------------------

    exportBundle() {
      const bundle = this.currentBundle();
      const json = JSON.stringify(bundle, null, 2);
      const blob = new Blob([json], { type: 'application/json' });
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      const base = (bundle.name || 'template').trim().toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '') || 'template';
      a.href = url;
      a.download = `${base}.bundle.json`;
      document.body.appendChild(a);
      a.click();
      document.body.removeChild(a);
      URL.revokeObjectURL(url);
      this.notify('Exported the current template contents.', 'info');
    },

    // ---- Import ------------------------------------------------------------

    importBundle(event) {
      const file = event.target.files && event.target.files[0];
      if (!file) return;
      const reader = new FileReader();
      reader.onload = () => {
        let bundle;
        try {
          bundle = JSON.parse(reader.result);
        } catch {
          this.notify('That file is not valid JSON.', 'warn');
          return;
        }
        // Finding 154: import is the third path into applyBundle.
        if (!this.confirmOverwrite(`the imported bundle "${file.name}"`)) return;
        this.applyBundle(bundle);
      };
      reader.readAsText(file);
      // Reset so re-importing the same file fires change again.
      event.target.value = '';
    },

    // ---- Presets (work item 4) ---------------------------------------------

    async loadPresets() {
      try {
        const resp = await fetch('/v1/templatePresets', { headers: { Accept: 'application/json' } });
        if (resp.ok) {
          const data = await resp.json();
          this.presets = Array.isArray(data) ? data : [];
        }
      } catch {
        /* leave empty */
      }
    },

    // Presets relevant to this carrier come first, then the rest (a preset can
    // still be applied cross-carrier, filling shared fields only).
    get presetOptions() {
      return [...this.presets].sort((a, b) => {
        const am = a.carrier === this.carrier ? 0 : 1;
        const bm = b.carrier === this.carrier ? 0 : 1;
        return am - bm;
      });
    },

    applyPreset() {
      if (!this.presetChoice) return;
      const preset = this.presets.find((p) => p.name === this.presetChoice);
      if (!preset) return;
      // Finding 154: this is the path the report reproduced on.
      if (!this.confirmOverwrite(`the "${preset.name}" preset`)) return;
      this.applyBundle(preset);
    },

    // ---- Whole-template generation -----------------------------------------

    // generateBundle asks the server to design a full template from the prompt
    // and fills every returned slot editor. The generate route matches the
    // carrier prefix (/v1/{carrier}/generateTemplate); the sample entity comes
    // from the shared templatePreview store.
    async generateBundle() {
      const prompt = (this.generationPrompt || '').trim();
      if (!prompt) {
        this.notify('Describe the template you want first.', 'warn');
        return;
      }
      const store = (window.Alpine && window.Alpine.store('templatePreview')) || {};
      const generatePath = `/v1/${this.carrier}/generateTemplate`;
      this.generating = true;
      this.notify('Generating the whole template…', 'info');
      try {
        const resp = await fetch(generatePath, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            target: 'bundle',
            prompt,
            metaSchema: this.getEditor('MetaSchema'),
            categoryId: store.categoryId || 0,
            entityId: store.entityId || 0,
          }),
        });
        const data = await resp.json().catch(() => null);
        if (!resp.ok) {
          this.notify((data && (data.error || data.Error)) || `Generation failed (${resp.status})`, 'warn');
          return;
        }
        const slots = (data && data.slots) || {};
        let filled = 0;
        for (const field of Object.values(SLOT_FIELDS)) {
          if (Object.prototype.hasOwnProperty.call(slots, field)) {
            this.setEditor(field, slots[field] || '');
            filled++;
          }
        }
        if (!filled) {
          this.notify((data && data.explanation) || 'The model returned no slots.', 'warn');
          return;
        }
        if (data && data.valid === false) {
          this.notify(`Filled ${filled} slot(s), but the draft has issues — review before saving.`, 'warn');
        } else {
          this.notify(`Filled ${filled} slot(s) from your description. Review, then save.`, 'info');
        }
      } catch (err) {
        this.notify(err.message || 'Network error', 'warn');
      } finally {
        this.generating = false;
      }
    },
  };
}
