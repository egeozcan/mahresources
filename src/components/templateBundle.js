// templateBundle powers the "Copy from…", Export, Import, and "Start from preset"
// tools on the three category-template edit forms (Category, Resource Category,
// Note Type). It is a client-side form-filling aid only: nothing is saved until
// the user submits the form. It fills the CodeMirror slot editors, the Meta JSON
// Schema editor, and (same-carrier only) the Section Config component.
//
// Bundle shape (schemaVersion 1), shared with server/template_presets/*.json and
// the export download:
//   { schemaVersion, carrier, name, description,
//     slots: { <every key of SLOT_FIELDS below> },
//     metaSchema, sectionConfig }
//
// Slot key -> hidden-input field name on the form.
import { askToConfirm } from './confirmDialog.js';

// One map for all three carriers, not three. Carrier-specific slots (preview,
// lightbox and cell are resource-only; ownEntities is group-only) are listed
// here too because both accessors tolerate a field with no editor on the current
// form: getEditor returns '' and setEditor returns early. That is what makes a
// cross-carrier copy degrade instead of erroring -- a resource bundle applied to
// a category form drops the three slots a category has no surface for.
// Additive, so schemaVersion stays 1: an older bundle omits the new keys and
// reads as empty, and an older reader ignores keys it does not know.
const SLOT_FIELDS = {
  header: 'CustomHeader',
  detailFooter: 'CustomDetailFooter',
  sidebar: 'CustomSidebar',
  preview: 'CustomPreview',
  lightbox: 'CustomLightbox',
  summary: 'CustomSummary',
  avatar: 'CustomAvatar',
  hoverCard: 'CustomHoverCard',
  cell: 'CustomCell',
  ownEntities: 'CustomOwnEntities',
  listHeader: 'CustomListHeader',
  listFooter: 'CustomListFooter',
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

// fetchAllPages walks ?page=1..N until a short page arrives.
//
// `complete` means "the list really ends here", and it is the only thing that
// decides whether the caller warns the user that sources are missing. A page that
// 500s or a fetch that throws is not the end of the list — it is the walk giving
// up part way — so both report complete:false. Reporting them as complete
// recreated finding 28 exactly: the picker offered a truncated list of copy
// sources with nothing on screen saying so, which is the defect the paging was
// added to fix.
//
// The items collected so far are always kept: a partial list the user is told
// about beats an empty one.
export async function fetchAllPages(url, fetchImpl = fetch) {
  const items = [];
  for (let page = 1; page <= SOURCE_PAGE_LIMIT; page++) {
    let batch;
    try {
      const sep = url.includes('?') ? '&' : '?';
      const resp = await fetchImpl(`${url}${sep}page=${page}`, { headers: { Accept: 'application/json' } });
      if (!resp.ok) return { items, complete: false, reason: 'error' };
      const data = await resp.json();
      batch = Array.isArray(data) ? data : [];
    } catch {
      // Network failure mid-walk: keep what arrived rather than losing the lot.
      return { items, complete: false, reason: 'error' };
    }
    items.push(...batch);
    if (batch.length < PAGE_SIZE) return { items, complete: true, reason: null };
  }
  return { items, complete: false, reason: 'cap' };
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
    //
    // `bundle` is what is about to be applied, and it is needed because
    // applyBundle does not write the same set of fields every time: the slots and
    // MetaSchema always go, but SectionConfig goes only for a same-carrier bundle
    // that carries one (setSectionConfig no-ops on an empty string). Counting the
    // slots alone meant a form whose *only* authored content was the section
    // layout scored zero fields at risk, returned true without prompting, and had
    // that layout replaced silently — finding 154 surviving for exactly one field.
    async confirmOverwrite(sourceLabel, bundle = null) {
      const filled = Object.values(SLOT_FIELDS)
        .filter((field) => this.getEditor(field).trim() !== '');
      if (this.getEditor('MetaSchema').trim() !== '') filled.push('MetaSchema');
      if (this.willReplaceSectionConfig(bundle) && this.getEditor('SectionConfig').trim() !== '') {
        filled.push('SectionConfig');
      }
      if (filled.length === 0) return true;
      const what = filled.length === 1 ? '1 template field' : `${filled.length} template fields`;
      return askToConfirm(
        `Replace ${what} with ${sourceLabel}? The current content is discarded (undo in the editor still works, and nothing is saved until you press Save).`,
      );
    },

    // willReplaceSectionConfig mirrors the branch in applyBundle exactly: the
    // section layout is overwritten only by a same-carrier bundle that actually
    // carries one. Kept as its own predicate so the confirmation and the write
    // cannot drift apart.
    willReplaceSectionConfig(bundle) {
      if (!bundle || typeof bundle !== 'object') return false;
      if (!bundle.sectionConfig) return false;
      return !bundle.carrier || bundle.carrier === this.carrier;
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
      const cappedAt = [];
      const failed = [];
      await Promise.all(
        entries.map(async ([key, cfg]) => {
          const { items, complete, reason } = await fetchAllPages(cfg.list);
          this.sources[key] = items;
          if (complete) return;
          (reason === 'cap' ? cappedAt : failed).push(cfg.label);
        }),
      );
      // Two different incomplete lists, and they need different words: one hit a
      // bound this code chose, the other lost a request. Both have to say
      // something, because an incomplete picker with nothing on screen is
      // finding 28.
      const messages = [];
      if (cappedAt.length) {
        messages.push(`Showing the first ${SOURCE_PAGE_LIMIT * PAGE_SIZE} of the available ${cappedAt.join(' / ')} templates.`);
      }
      if (failed.length) {
        messages.push(`Could not load the whole ${failed.join(' / ')} list — a request failed, so some copy sources are missing. Reload to try again.`);
      }
      if (messages.length) {
        messages.push('Copy from a more specific one if the source you want is missing.');
        this.notify(messages.join(' '), 'warn');
      }
    },

    async copyFrom() {
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
      const bundle = this.entityToBundle(obj, sourceCarrier);
      if (!await this.confirmOverwrite(`the template from "${obj.Name || 'the selected source'}"`, bundle)) return;
      this.applyBundle(bundle);
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
      reader.onload = async () => {
        let bundle;
        try {
          bundle = JSON.parse(reader.result);
        } catch {
          this.notify('That file is not valid JSON.', 'warn');
          return;
        }
        // Finding 154: import is the third path into applyBundle.
        if (!await this.confirmOverwrite(`the imported bundle "${file.name}"`, bundle)) return;
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

    async applyPreset() {
      if (!this.presetChoice) return;
      const preset = this.presets.find((p) => p.name === this.presetChoice);
      if (!preset) return;
      // Finding 154: this is the path the report reproduced on.
      if (!await this.confirmOverwrite(`the "${preset.name}" preset`, preset)) return;
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
