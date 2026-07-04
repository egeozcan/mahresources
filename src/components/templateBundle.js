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

    async loadSources() {
      const entries = Object.entries(CARRIERS);
      await Promise.all(
        entries.map(async ([key, cfg]) => {
          try {
            const resp = await fetch(cfg.list, { headers: { Accept: 'application/json' } });
            if (resp.ok) {
              const data = await resp.json();
              this.sources[key] = Array.isArray(data) ? data : [];
            }
          } catch {
            /* leave empty on failure */
          }
        }),
      );
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
      this.applyBundle(preset);
    },
  };
}
