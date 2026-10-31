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
import { announceApplied, REVISION_APPLIED_EVENT } from './revisionApplied.js';
import { draftSegments, draftWarnings } from './templateDraftReview.js';
import { afterDialogTeardown, returnFocusToGenerate, reviewDisclosure } from './generationDisclosure.js';

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
  headerCSS: 'CustomHeaderCSS',
  detailFooter: 'CustomDetailFooter',
  detailFooterCSS: 'CustomDetailFooterCSS',
  sidebar: 'CustomSidebar',
  sidebarCSS: 'CustomSidebarCSS',
  preview: 'CustomPreview',
  previewCSS: 'CustomPreviewCSS',
  lightbox: 'CustomLightbox',
  lightboxCSS: 'CustomLightboxCSS',
  summary: 'CustomSummary',
  summaryCSS: 'CustomSummaryCSS',
  avatar: 'CustomAvatar',
  avatarCSS: 'CustomAvatarCSS',
  hoverCard: 'CustomHoverCard',
  hoverCardCSS: 'CustomHoverCardCSS',
  cell: 'CustomCell',
  cellCSS: 'CustomCellCSS',
  ownEntities: 'CustomOwnEntities',
  ownEntitiesCSS: 'CustomOwnEntitiesCSS',
  listHeader: 'CustomListHeader',
  listHeaderCSS: 'CustomListHeaderCSS',
  listFooter: 'CustomListFooter',
  listFooterCSS: 'CustomListFooterCSS',
  mrqlResult: 'CustomMRQLResult',
  mrqlResultCSS: 'CustomMRQLResultCSS',
  entityPickerResult: 'CustomEntityPickerResult',
  entityPickerResultCSS: 'CustomEntityPickerResultCSS',
  css: 'CustomCSS',
};

// The sections a whole-template request can name: every HTML slot (each is
// generated with its CSS companion) and CustomCSS, which only gains new rules.
const SECTION_FIELDS = [...Object.values(SLOT_FIELDS).filter((field) => !field.endsWith('CSS')), 'CustomCSS'];

// sectionLabel is a field name as words: CustomMRQLResult -> "MRQL Result".
export function sectionLabel(field) {
  if (field === 'CustomCSS') return 'Shared CSS (CustomCSS)';
  return field
    .replace(/^Custom/, '')
    .replace(/([a-z])([A-Z])/g, '$1 $2')
    .replace(/([A-Z]+)([A-Z][a-z])/g, '$1 $2');
}

// Carrier -> display label + list endpoint used to populate the "Copy from" picker.
const CARRIERS = {
  category: { label: 'Category', list: '/v1/categories', revisions: '/v1/category/revisions' },
  resourceCategory: { label: 'Resource Category', list: '/v1/resourceCategories', revisions: '/v1/resourceCategory/revisions' },
  noteType: { label: 'Note Type', list: '/v1/note/noteTypes', revisions: '/v1/noteType/revisions' },
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
    // The sections this form has an editor for, and the ones ticked. Nothing
    // ticked lets the generator choose.
    sections: [],
    selectedSections: [],
    // A generated draft waiting for review: { slots, fields, issues, slotIssues,
    // explanation, valid, snapshot }. Nothing reaches an editor until
    // applyPendingDraft; snapshot is every editor as the request sent it.
    pendingDraft: null,
    // The send was refused with 409: what would be sent changed after review.
    groundingChanged: false,
    _generationRequestId: 0,
    // What this form applied before the save, posted as revisionApplied so the
    // Template Revision it records says where the content came from.
    appliedEntries: [],
    // The "Loaded revision #N" notice; empty when nothing was loaded.
    revisionNotice: '',
    revisionNoticeKind: 'info',
    draftSegments,
    draftWarnings,

    async init() {
      this.sections = this.sectionOptions();
      document.addEventListener(REVISION_APPLIED_EVENT, (event) => this.recordApplied(event.detail.entry));
      this.loadRevisionFromUrl();
      await Promise.all([this.loadSources(), this.loadPresets()]);
    },

    // recordApplied remembers one revisionApplied entry for the save.
    recordApplied(entry) {
      if (!this.appliedEntries.includes(entry)) this.appliedEntries = [...this.appliedEntries, entry];
    },

    // sectionOptions lists the sections the form can take: the carrier's slot
    // editors are server-rendered, so an editor's input says the carrier has it.
    sectionOptions() {
      return SECTION_FIELDS
        .filter((field) => document.querySelector(`[name="${field}"]`))
        .map((field) => ({ field, label: sectionLabel(field) }));
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
    //
    // `fields` is for a caller that knows exactly which fields it writes (Apply on
    // a generated draft): the question then names those and no others. `edited`
    // marks the ones changed since the request began, and `options` goes to the
    // dialog.
    async confirmOverwrite(sourceLabel, bundle = null, { fields = null, edited = [], appended = [], options = undefined } = {}) {
      if (fields) {
        if (fields.length === 0) return true;
        const named = fields.map((field) => (appended.includes(field) ? `${field} (new rules are added after your current CSS)` : field));
        const marked = edited.length ? ` ${edited.join(', ')} ${edited.length === 1 ? 'was' : 'were'} edited since you asked for this draft.` : '';
        return askToConfirm(
          `Replace ${fields.length === 1 ? '1 template field' : `${fields.length} template fields`} with ${sourceLabel}? This changes ${named.join(', ')}.${marked} Undo in the editor still works, and nothing is saved until you press Save.`,
          options,
        );
      }
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
        return false;
      }
      if (typeof bundle.schemaVersion === 'number' && bundle.schemaVersion > BUNDLE_SCHEMA_VERSION) {
        this.notify(`Bundle schema version ${bundle.schemaVersion} is newer than supported (${BUNDLE_SCHEMA_VERSION}).`, 'warn');
        return false;
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
      return true;
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
      if (this.applyBundle(bundle)) this.recordApplied(`copied:${sourceCarrier}:${id}`);
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

    // ---- Load a Template Revision into the form ----------------------------

    // loadRevisionFromUrl fills the form from ?loadRevision=<n>[&slot=<slot>
    // &includeCss=0], which the History page links to. Nothing is saved: the
    // user reviews the form and presses Save, and revisionApplied says so.
    loadRevisionFromUrl() {
      try {
        const params = new URLSearchParams(window.location.search);
        const number = Number(params.get('loadRevision'));
        const idInput = document.querySelector('form input[name="ID"]');
        if (!Number.isInteger(number) || number < 1 || !idInput) {
          this.releaseServerLock();
          return;
        }
        const slot = params.get('slot') || '';
        const includeCss = params.get('includeCss') !== '0';
        // A reload should show the saved state, not load the revision over edits.
        params.delete('loadRevision');
        params.delete('slot');
        params.delete('includeCss');
        const query = params.toString();
        window.history.replaceState(null, '', window.location.pathname + (query ? `?${query}` : '') + window.location.hash);
        this.loadRevision(idInput.value, number, slot, includeCss, idInput.form);
      } catch (err) {
        // Nothing else will release the lock the server rendered.
        this.releaseServerLock();
        throw err;
      }
    },

    // The server renders the form inert (data-revision-locked) when the page was
    // asked to load a revision, so it cannot be edited before this script runs.
    releaseServerLock() {
      const form = document.querySelector('form[data-revision-locked]');
      if (form) {
        form.inert = false;
        form.removeAttribute('aria-busy');
        form.removeAttribute('data-revision-locked');
      }
      document.getElementById('revision-preload-status')?.remove();
    },

    // setFormBusy keeps the form from being edited or submitted while a revision
    // is on its way: the response would overwrite whatever was typed meanwhile.
    // inert blocks pointer, keyboard and focus; the live region is outside the
    // form because inert content is not announced.
    setFormBusy(form, text) {
      if (!form) return;
      form.inert = !!text;
      if (text) form.setAttribute('aria-busy', 'true');
      else this.releaseServerLock();
      if (!text) form.removeAttribute('aria-busy');
      if (!this._busyRegion) {
        this._busyRegion = document.createElement('div');
        this._busyRegion.className = 'sr-only';
        this._busyRegion.setAttribute('role', 'status');
        document.body.appendChild(this._busyRegion);
      }
      setTimeout(() => { this._busyRegion.textContent = text; }, 0);
    },

    async loadRevision(carrierId, number, slot, includeCss, form) {
      const what = slot ? `${slot} from revision #${number}` : `revision #${number}`;
      this.revisionNotice = `Loading ${what}. The form is locked until it arrives.`;
      this.revisionNoticeKind = 'info';
      this.setFormBusy(form, `Loading ${what}. The form is locked until it arrives.`);
      try {
        const resp = await fetch(`${CARRIERS[this.carrier].revisions}/${number}?id=${encodeURIComponent(carrierId)}`, { headers: { Accept: 'application/json' } });
        if (!resp.ok) {
          const body = await resp.json().catch(() => null);
          this.announceRevision(`Could not load ${what}: ${(body && (body.error || body.Error)) || resp.status}`, 'warn');
          return;
        }
        const revision = await resp.json();
        const editor = revision.editor || {};
        const fields = slot ? this.slotFields(editor, slot, includeCss) : Object.keys(editor);
        if (!fields.length) {
          this.announceRevision(`Could not load ${what}: this form has no such slot.`, 'warn');
          return;
        }
        await this.whenEditorsReady();
        for (const field of fields) this.setRevisionField(field, editor[field]);
        this.recordApplied(slot ? `loaded:${number}:${slot}` : `loaded:${number}`);
        this.announceRevision(`Loaded ${what}. Review the form, then save. Nothing is saved yet.`, 'info');
      } catch (err) {
        this.announceRevision(`Could not load ${what}: ${err.message || 'network error'}`, 'warn');
      } finally {
        this.setFormBusy(form, '');
      }
    },

    // slotFields is the editor keys one slot covers: its field and, when asked,
    // its CSS companion. [] when the revision has no such slot.
    slotFields(editor, slot, includeCss) {
      // A CSS companion is not a slot of its own: it travels with its primary.
      if (slot.endsWith('CSS') && slot !== 'CustomCSS') return [];
      if (!Object.prototype.hasOwnProperty.call(editor, slot)) return [];
      const fields = [slot];
      const css = `${slot}CSS`;
      if (includeCss && Object.prototype.hasOwnProperty.call(editor, css)) fields.push(css);
      return fields;
    },

    // setRevisionField writes one tracked field from a revision's editor payload.
    // SectionConfig replaces the whole control state: the payload is already the
    // server's resolved config, and merging it would keep stale keys (a merge of
    // {"tags":false} over current defaults is not what the resolver produces).
    setRevisionField(field, value) {
      if (field === 'SectionConfig') {
        this.replaceSectionConfig(value);
        return;
      }
      const input = document.querySelector(`[name="${field}"][type="checkbox"]`);
      if (input) {
        input.checked = value === true;
        input.dispatchEvent(new Event('change', { bubbles: true }));
        return;
      }
      const target = document.querySelector(`[name="${field}"]`);
      if (target && target.tagName === 'TEXTAREA') {
        // A plain textarea has no CodeMirror view; setEditor would look for one in
        // whichever component happens to wrap it.
        target.value = value || '';
        target.dispatchEvent(new Event('input', { bubbles: true }));
        return;
      }
      this.setEditor(field, value);
    },

    replaceSectionConfig(jsonStr) {
      const el = document.querySelector('[x-data^="sectionConfigForm"]');
      if (!el || !window.Alpine) return;
      let parsed;
      try {
        parsed = JSON.parse(jsonStr);
      } catch {
        this.announceRevision('The revision\'s section layout could not be read, so it was left as it is.', 'warn');
        return;
      }
      if (!parsed || typeof parsed !== 'object') return;
      const data = window.Alpine.$data(el);
      if (data) data.config = parsed;
    },

    // whenEditorsReady waits for every CodeMirror editor on the form to exist;
    // they mount after the page's load event, and a write before then is lost.
    async whenEditorsReady() {
      const deadline = Date.now() + 15000;
      for (;;) {
        const containers = [...document.querySelectorAll('[x-ref="editorContainer"]')];
        if (containers.every((container) => container._cmReady)) {
          await Promise.all(containers.map((container) => container._cmReady));
          return;
        }
        if (Date.now() > deadline) return;
        await new Promise((resolve) => setTimeout(resolve, 50));
      }
    },

    // The notice takes focus: the person followed a link to get here, and this
    // says what happened to the form.
    announceRevision(text, kind) {
      this.revisionNotice = text;
      this.revisionNoticeKind = kind;
      setTimeout(() => {
        const notice = this.$refs && this.$refs.revisionNotice;
        if (notice) notice.focus();
      }, 0);
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
      if (this.applyBundle(preset)) this.recordApplied(`preset:${preset.name}`);
    },

    // ---- Whole-template generation -----------------------------------------

    // generateBundle asks the server to design a full template from the prompt
    // and holds the returned slots as a pending draft, listed per slot with its
    // issues. Nothing is written to an editor until the user presses Apply. The
    // generate route matches the carrier prefix (/v1/{carrier}/generateTemplate);
    // the sample entity comes from the shared templatePreview store.
    async generateBundle() {
      const prompt = (this.generationPrompt || '').trim();
      if (!prompt) {
        this.notify('Describe the template you want first.', 'warn');
        return;
      }
      const store = (window.Alpine && window.Alpine.store('templatePreview')) || {};
      const generatePath = `/v1/${this.carrier}/generateTemplate`;
      const requestId = ++this._generationRequestId;
      this.generating = true;
      this.pendingDraft = null;
      this.groundingChanged = false;
      this.notify('Preparing what will be sent…', 'info');
      const snapshot = Object.fromEntries(Object.values(SLOT_FIELDS).map((field) => [field, this.getEditor(field)]));
      const body = {
        target: 'bundle',
        content: JSON.stringify(snapshot),
        prompt,
        metaSchema: this.getEditor('MetaSchema'),
        categoryId: store.categoryId || 0,
        entityId: store.entityId || 0,
        // The unsaved identity and layout; content already holds every slot.
        name: this.getEditor('name'),
        description: this.getEditor('Description'),
        sectionConfig: this.getEditor('SectionConfig'),
      };
      // Ticked sections are forced; none ticked lets the generator choose.
      if (this.selectedSections.length) body.slots = [...this.selectedSections];
      const post = (extra) => fetch(generatePath, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ ...body, ...extra }),
      });
      // Missing refs (a test fixture, a partial render) only cost the focus moves.
      const { bundleGenerate, bundleStatus, bundlePrompt } = this.$refs || {};
      try {
        // A dry run assembles exactly what would be sent and sends nothing; the
        // reader approves it before the real request goes out with its hash.
        const dryResp = await post({ dryRun: true });
        const disclosure = await dryResp.json().catch(() => null);
        if (requestId !== this._generationRequestId) return;
        if (!dryResp.ok || !disclosure?.groundingHash) {
          this.notify((disclosure && (disclosure.error || disclosure.Error)) || `Generation failed (${dryResp.status})`, 'warn');
          return;
        }
        // Generate is enabled while the dialog is open, so Cancel can return to it.
        this.generating = false;
        this.notify('', 'info');
        const approved = await reviewDisclosure(disclosure, [bundleGenerate, bundleStatus, bundlePrompt]);
        if (requestId !== this._generationRequestId || !approved) return;

        this.generating = true;
        this.notify('Generating the whole template… this can take up to a minute.', 'info');
        afterDialogTeardown().then(() => {
          if (requestId === this._generationRequestId && this.generating) bundleStatus?.focus();
        });
        const resp = await post({ groundingHash: disclosure.groundingHash });
        const data = await resp.json().catch(() => null);
        if (requestId !== this._generationRequestId) return;
        if (resp.status === 409) {
          // Never reopen the dialog by itself: another modal may be open by now.
          this.groundingChanged = true;
          this.notify('The data to be sent changed.', 'warn');
          return;
        }
        if (!resp.ok) {
          this.notify((data && (data.error || data.Error)) || `Generation failed (${resp.status})`, 'warn');
          return;
        }
        const slots = (data && data.slots) || {};
        const issues = data && Array.isArray(data.issues) ? data.issues : [];
        // A pair is applied whole or not at all. Half a pair from a reply that
        // reported issues (one cut off at the output limit) is listed for
        // review but blocked from Apply; from a clean reply it is a broken
        // contract and nothing is offered.
        const blocked = [];
        for (const field of Object.values(SLOT_FIELDS).filter((name) => !name.endsWith('CSS'))) {
          const htmlPresent = Object.prototype.hasOwnProperty.call(slots, field);
          const cssPresent = Object.prototype.hasOwnProperty.call(slots, `${field}CSS`);
          const malformed = (htmlPresent && typeof slots[field] !== 'string') || (cssPresent && typeof slots[`${field}CSS`] !== 'string');
          if (htmlPresent === cssPresent && !malformed) continue;
          if (!malformed && issues.length) {
            blocked.push(htmlPresent ? field : `${field}CSS`);
            continue;
          }
          this.notify(`The model must return ${field} and ${field}CSS together. No changes applied.`, 'warn');
          return;
        }
        const returned = Object.values(SLOT_FIELDS).filter((field) => Object.prototype.hasOwnProperty.call(slots, field));
        if (!returned.length) {
          // A truncated reply has nothing to apply; its issue says why.
          const issueText = issues.map((i) => i.message).filter(Boolean).join(' ');
          this.notify(issueText || (data && data.explanation) || 'The model returned no slots.', 'warn');
          return;
        }
        const slotIssues = (data && data.slotIssues) || {};
        // Issues about the reply as a whole (truncation) belong to no slot.
        const slotIssueKeys = new Set(Object.values(slotIssues).flat().map((i) => `${i.severity}:${i.message}`));
        // Sections whose pair could not be generated are named in their own
        // warning; their per-section issue would repeat it.
        const failedSlots = data && Array.isArray(data.failedSlots) ? data.failedSlots.filter((name) => typeof name === 'string') : [];
        const isFailedSlotIssue = (i) => failedSlots.some((name) => String(i.message).startsWith(`${name} could not be generated`));
        this.pendingDraft = {
          slots: Object.fromEntries(returned.map((field) => [field, slots[field]])),
          fields: returned,
          valid: !(data && data.valid === false),
          explanation: (data && data.explanation) || '',
          issues: issues.filter((i) => !slotIssueKeys.has(`${i.severity}:${i.message}`) && !isFailedSlotIssue(i)),
          slotIssues,
          snapshot,
          blocked,
          failedSlots,
        };
        // Display only: Apply depends on the pending draft, never on this text.
        const repairedNote = data?.repair?.applied ? ' (auto-repaired)' : '';
        if (failedSlots.length) {
          this.notify(`Generated ${returned.length} slot(s)${repairedNote}, but ${failedSlots.join(', ')} could not be generated and ${failedSlots.length === 1 ? 'was' : 'were'} left as ${failedSlots.length === 1 ? 'it is' : 'they are'}. Review the draft below, then Apply.`, 'warn');
        } else if (this.pendingDraft.valid) {
          this.notify(`Generated ${returned.length} slot(s)${repairedNote}. Review the draft below, then Apply.`, 'info');
        } else {
          this.notify(`Generated ${returned.length} slot(s)${repairedNote}, but the draft has issues. Review it below before applying.`, 'warn');
        }
      } catch (err) {
        if (requestId !== this._generationRequestId) return;
        this.notify(err.message || 'Network error', 'warn');
      } finally {
        if (requestId === this._generationRequestId) {
          this.generating = false;
          returnFocusToGenerate(this, bundleStatus, bundleGenerate, [bundlePrompt]);
        }
      }
    },

    // applyPendingDraft writes the reviewed draft into the editors. A returned
    // slot edited since the request started (typing, a preset, another
    // generation) is named and the user asked first; cancelling keeps every
    // editor as it is.
    async applyPendingDraft() {
      const draft = this.pendingDraft;
      if (!draft) return;
      if (!this.canApplyDraft()) return;
      const fields = draft.fields.filter((field) => !draft.blocked.includes(field));
      // One question, before any editor is written: the fields the draft would
      // change that hold something or were edited since the request began, the
      // latter marked. A field the user cleared meanwhile is named too.
      const replaced = [];
      const edited = [];
      const appended = [];
      for (const field of fields) {
        const current = this.getEditor(field);
        const next = draft.slots[field] || '';
        if (current.trim() === next.trim()) continue;
        const editedSince = Object.prototype.hasOwnProperty.call(draft.snapshot, field) && current !== draft.snapshot[field];
        if (current.trim() === '' && !editedSince) continue;
        replaced.push(field);
        if (editedSince) edited.push(field);
        else if (field === 'CustomCSS' && next.startsWith(current)) appended.push(field);
      }
      if (replaced.length) {
        // Apply is removed with the panel once the draft is written, so the
        // dialog's own focus return needs somewhere stable to land.
        const confirmed = await this.confirmOverwrite('the generated draft', null, {
          fields: replaced,
          edited,
          appended,
          options: { title: 'Replace your content?', confirmLabel: 'Replace', cancelLabel: 'Keep my content', fallbackFocus: () => this._focusChain().find((el) => el?.isConnected && !el.disabled && el.getClientRects().length > 0) },
        });
        if (!confirmed || this.pendingDraft !== draft) return;
      }
      for (const field of fields) this.setEditor(field, draft.slots[field] || '');
      announceApplied('generated');
      this.pendingDraft = null;
      this.notify(`Filled ${fields.length} slot(s) from your description. Review, then save.`, 'info');
      this._focusAfterPanelCloses();
    },

    discardPendingDraft() {
      this.pendingDraft = null;
      this.notify('Discarded the generated draft.', 'info');
      this._focusAfterPanelCloses();
    },

    // Whether Apply has anything to write: half pairs are listed, not applied.
    canApplyDraft() {
      const draft = this.pendingDraft;
      return !!draft && draft.fields.some((field) => !draft.blocked.includes(field));
    },

    // Apply and Discard live in the panel they remove; once Alpine has torn it
    // down, focus would be on <body>, so it goes back to Generate.
    _focusAfterPanelCloses() {
      const [button, ...fallbacks] = this._focusChain();
      returnFocusToGenerate(this, null, button, fallbacks);
    },

    // Where focus goes when Generate cannot take it: its status region, then
    // the prompt.
    _focusChain() {
      const refs = this.$refs || {};
      return [refs.bundleGenerate, refs.bundleStatus, refs.bundlePrompt];
    },
  };
}
