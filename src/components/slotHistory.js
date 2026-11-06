// slotHistory is the History dialog of one template slot on the category /
// resource category / note type edit forms. A cluster's History button opens
// it for that slot. It lists the Template Revisions that changed the slot,
// diffs the chosen one against what is in the editor now (unsaved edits
// included), previews the old version, and applies it to the editor. Applying
// saves nothing: the author reviews the form and presses Save, as with "Load
// into editor" on the History page.
//
// Freshness: every open and close starts a new session, each revision fetch
// and each preview render carries its own sequence number, and a response
// commits only while all of them still match. Apply reads one immutable
// selection, and writes synchronously: the editors were waited on before the
// dialog let anything be chosen.

import { diffLines } from 'diff';
import { sideBySideRows, unifiedRows } from './templateRevisions.js';
import { editorsReady, REVISION_WRITE_EVENT } from './templateBundle.js';
import {
  CSS_SLOT,
  cssCascade,
  ensurePreviewStore,
  frameDocument,
  isCarrierSlot,
  previewBody,
  renderErrorsFrom,
} from './templatePreviewRender.js';
import { containFocus } from '../utils/containFocus.js';
import { restoreFocus } from '../utils/focus.js';

// The window event a History button dispatches: { slot, label, opener }.
export const OPEN_SLOT_HISTORY_EVENT = 'open-slot-history';

// linesToHunks diffs two texts line by line into the {op, lines} hunks the
// History page's row builders take.
export function linesToHunks(before, after) {
  return diffLines(before || '', after || '').map((part) => {
    const lines = part.value.split('\n');
    if (lines.length > 1 && lines[lines.length - 1] === '') lines.pop();
    return { op: part.added ? 'insert' : part.removed ? 'delete' : 'equal', lines };
  });
}

// cssCompanion is the CSS field that travels with a slot, or '' for the shared
// stylesheet, which has none.
function cssCompanion(slot) {
  return slot === CSS_SLOT ? '' : `${slot}CSS`;
}

/**
 * @typedef {object} SlotHistoryMagics
 * @property {HTMLElement} $root The element carrying this component's x-data.
 * @property {(expr: string, cb: (value: unknown) => void) => void} $watch Alpine's watcher.
 */

/**
 * @param {object} [options]
 * @param {string} [options.apiPrefix] e.g. /v1/category
 * @param {number|string} [options.carrierId] The category/type being edited.
 * @param {string} [options.entityType] group, resource or note.
 * @param {string} [options.previewPath] Endpoint that renders one slot.
 */
export function slotHistory({ apiPrefix = '', carrierId = 0, entityType = 'group', previewPath = '' } = {}) {
  const component = {
    apiPrefix,
    carrierId,
    entityType,
    previewPath,

    isOpen: false,
    slot: '',
    label: '',
    // Loading the list (and waiting on the editors) before anything can be chosen.
    loading: false,
    error: '',
    /** @type {{ number: number, createdAt: string, source: string, label?: string, restoredFrom?: number }[]} */
    revisions: [],
    /** @type {number|null} */
    selected: null,
    /** @type {Readonly<{ session: number, slot: string, number: number, editor: Record<string, string> }>|null} */
    detail: null,
    detailLoading: false,
    detailError: '',
    includeCss: true,
    layout: 'unified',
    /** @type {{ field: string, same: boolean, unified: object[], sideBySide: object[] }[]} */
    diffFields: [],
    previewLoading: false,
    previewError: '',
    /** @type {{ label: string, detail: string, count: number }[]} */
    renderErrors: [],
    /** @type {{ severity: string, message: string }[]} */
    issues: [],

    _session: 0,
    _detailSeq: 0,
    _previewSeq: 0,
    /** @type {AbortController|null} */
    _previewAbort: null,
    /** @type {HTMLElement|null} */
    _opener: null,
    /** @type {HTMLFormElement|null} */
    _form: null,
    /** @type {HTMLIFrameElement|null} */
    _frame: null,
    /** @type {HTMLElement[]} */
    _inerted: [],
    /** @type {{ stop: () => void }|null} */
    _containment: null,

    init() {
      this._form = this.$root.closest('form');
      window.addEventListener(OPEN_SLOT_HISTORY_EVENT, (e) => this.open(/** @type {CustomEvent} */ (e).detail || {}));
      ensurePreviewStore();
      this.$watch('$store.templatePreview.entityGen', () => {
        if (this.isOpen && this.detail) this.renderPreview();
      });
    },

    // ---- Open and close ------------------------------------------------------

    async open({ slot, label, opener }) {
      if (!slot) return;
      if (this.isOpen) this.close({ restore: false });
      const session = ++this._session;
      this._opener = opener || null;
      this.slot = slot;
      this.label = label || slot;
      this.revisions = [];
      this.error = '';
      this._clearSelection();
      this.includeCss = true;
      this.loading = true;
      this.isOpen = true;

      const fields = [slot, cssCompanion(slot)].filter(Boolean);
      if (!(await editorsReady(fields))) {
        if (session !== this._session) return;
        this.loading = false;
        this.error = 'The code editors did not finish loading, so nothing can be compared. Reload the page to try again.';
        return;
      }
      if (session !== this._session) return;
      try {
        const resp = await fetch(`${this.apiPrefix}/revisions?id=${encodeURIComponent(this.carrierId)}`, {
          headers: { Accept: 'application/json' },
        });
        if (session !== this._session) return;
        if (!resp.ok) {
          const text = await this._errorText(resp);
          if (session !== this._session) return;
          this.error = text;
        } else {
          const all = (await resp.json()) || [];
          if (session !== this._session) return;
          this.revisions = all.filter((rev) => (rev.changedSlots || []).includes(slot));
        }
      } catch (err) {
        if (session !== this._session) return;
        this.error = err.message || 'Network error';
      }
      this.loading = false;
    },

    // mount runs when the dialog subtree appears: the rest of the page goes
    // inert, Tab stays inside, and focus moves to the heading.
    mount(dialogRoot) {
      this._applyInert(dialogRoot);
      this._containment?.stop();
      this._containment = containFocus(dialogRoot);
      dialogRoot.querySelector('[data-slot-history-heading]')?.focus();
    },

    close({ restore = true } = {}) {
      if (!this.isOpen) return;
      this._session++;
      this._detailSeq++;
      this._previewSeq++;
      this._previewAbort?.abort();
      this._previewAbort = null;
      this.isOpen = false;
      this._frame = null;
      this._containment?.stop();
      this._containment = null;
      this._releaseInert();
      const opener = this._opener;
      this._opener = null;
      // Once Alpine has torn the dialog down: a macrotask runs after its
      // microtask-flushed effects, and focus moved earlier is lost with the subtree.
      if (restore) setTimeout(() => restoreFocus(opener, null), 0);
    },

    // Everything except the overlay layer's dialog becomes inert, as the
    // confirm dialog does (confirmDialog.js _applyInert): the body's children,
    // and the overlay siblings of the one that holds this dialog.
    _applyInert(dialogRoot) {
      this._releaseInert();
      const ancestors = new Set();
      for (let el = dialogRoot; el && el !== document.body; el = el.parentElement) ancestors.add(el);
      const mark = (el) => {
        if (ancestors.has(el) || el.inert) return;
        el.inert = true;
        this._inerted.push(el);
      };
      for (const child of document.body.children) {
        if (ancestors.has(child)) {
          for (const grandchild of child.children) mark(grandchild);
        } else {
          mark(child);
        }
      }
    },

    _releaseInert() {
      for (const el of this._inerted) el.inert = false;
      this._inerted = [];
    },

    // ---- Selection -----------------------------------------------------------

    _clearSelection() {
      this._detailSeq++;
      this._previewSeq++;
      this._previewAbort?.abort();
      this._previewAbort = null;
      this.selected = null;
      this.detail = null;
      this.detailLoading = false;
      this.detailError = '';
      this.diffFields = [];
      this.previewLoading = false;
      this.previewError = '';
      this.renderErrors = [];
      this.issues = [];
      // The frame lives inside x-if="detail", so clearing the selection removes it.
      this._frame = null;
    },

    async select(number) {
      const session = this._session;
      this._clearSelection();
      this.selected = number;
      const seq = this._detailSeq;
      this.detailLoading = true;
      const fresh = () => session === this._session && seq === this._detailSeq;
      try {
        const resp = await fetch(
          `${this.apiPrefix}/revisions/${number}?id=${encodeURIComponent(this.carrierId)}`,
          { headers: { Accept: 'application/json' } },
        );
        if (!fresh()) return;
        if (!resp.ok) {
          const text = await this._errorText(resp);
          if (!fresh()) return;
          this.detailError = `Could not load revision #${number}: ${text}`;
          this.detailLoading = false;
          return;
        }
        const revision = await resp.json();
        if (!fresh()) return;
        const editor = revision.editor || {};
        if (!Object.prototype.hasOwnProperty.call(editor, this.slot)) {
          this.detailError = `Revision #${number} has no ${this.label}.`;
          this.detailLoading = false;
          return;
        }
        this.detail = Object.freeze({ session, slot: this.slot, number, editor: Object.freeze({ ...editor }) });
      } catch (err) {
        if (!fresh()) return;
        this.detailError = `Could not load revision #${number}: ${err.message || 'network error'}`;
        this.detailLoading = false;
        return;
      }
      this.detailLoading = false;
      this.computeDiff();
      // The preview frame mounts with the detail and renders from bindFrame.
    },

    hasCss() {
      const css = cssCompanion(this.slot);
      return !!css && !!this.detail && Object.prototype.hasOwnProperty.call(this.detail.editor, css) && !!this._input(css);
    },

    // The fields Apply would write: the slot, and its CSS when ticked.
    appliedFields() {
      if (!this.detail) return [];
      return this.hasCss() && this.includeCss ? [this.slot, cssCompanion(this.slot)] : [this.slot];
    },

    computeDiff() {
      if (!this.detail) {
        this.diffFields = [];
        return;
      }
      const fields = this.hasCss() ? [this.slot, cssCompanion(this.slot)] : [this.slot];
      this.diffFields = fields.map((field) => {
        const before = this.detail.editor[field] || '';
        const after = this._readField(field);
        const hunks = linesToHunks(before, after);
        return {
          field,
          same: before === after,
          unified: unifiedRows(hunks),
          sideBySide: sideBySideRows(hunks),
        };
      });
    },

    // Same as the editor when every field Apply would write already matches.
    isSame() {
      const applied = new Set(this.appliedFields());
      const rows = this.diffFields.filter((f) => applied.has(f.field));
      return rows.length > 0 && rows.every((f) => f.same);
    },

    onIncludeCssChange() {
      this.renderPreview();
    },

    // ---- Preview -------------------------------------------------------------

    isCssSlot() {
      return this.slot === CSS_SLOT;
    },

    bindFrame(frame) {
      this._frame = frame;
      if (this.detail) this.renderPreview();
    },

    async renderPreview() {
      const seq = ++this._previewSeq;
      this._previewAbort?.abort();
      this._previewAbort = null;
      const detail = this.detail;
      if (!detail || this.isCssSlot() || !this.previewPath) return;
      const session = this._session;
      const fresh = () => session === this._session && seq === this._previewSeq && this.detail === detail;
      const store = ensurePreviewStore() || {};
      const carrier = isCarrierSlot(detail.slot);
      if (!carrier && !store.entityId) {
        this.previewError = 'Nothing to preview against: pick an entity in Preview against, above the templates.';
        return;
      }
      const css = cssCompanion(detail.slot);
      const swapCss = this.includeCss && this.hasCss();
      const read = (name) => (swapCss && name === css ? detail.editor[name] || '' : this._readField(name));
      const body = previewBody({
        slot: detail.slot,
        content: detail.editor[detail.slot] || '',
        css: cssCascade(this.entityType, detail.slot, read),
        categoryId: this.carrierId,
        entityId: store.entityId,
      });
      this.previewLoading = true;
      this.previewError = '';
      const abort = typeof AbortController === 'function' ? new AbortController() : null;
      this._previewAbort = abort;
      try {
        const resp = await fetch(this.previewPath, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(body),
          signal: abort?.signal,
        });
        if (!fresh()) return;
        if (!resp.ok) {
          const err = await resp.json().catch(() => ({}));
          if (!fresh()) return;
          this.previewError = err.error || `Preview failed (${resp.status})`;
          this.previewLoading = false;
          return;
        }
        const data = await resp.json();
        if (!fresh()) return;
        this.issues = [...(data.issues || []), ...(data.cssIssues || [])];
        this.renderErrors = renderErrorsFrom((data.html || '') + (data.css || ''));
        if (this._frame) this._frame.srcdoc = frameDocument(data.html || '', data.css || '', data.entity, detail.slot);
      } catch (err) {
        if (!fresh()) return;
        this.previewError = 'Preview request failed.';
      }
      this.previewLoading = false;
    },

    // ---- Apply ---------------------------------------------------------------

    canApply() {
      return !!this.detail && this.detail.session === this._session && !this.detailLoading && !this.loading;
    },

    apply() {
      const detail = this.detail;
      if (!detail || detail.session !== this._session) return;
      const fields = this.appliedFields();
      const editor = detail.editor;
      const what = `${detail.slot} from revision #${detail.number}`;
      // Closed first, so the page is no longer inert when the editors change.
      // Focus goes back to the History button; the slot's preview, right
      // beside it, announces what was loaded.
      this.close();
      document.dispatchEvent(
        new CustomEvent(REVISION_WRITE_EVENT, {
          detail: {
            editor,
            fields,
            entry: `loaded:${detail.number}:${detail.slot}`,
            notice: `Loaded ${what}. Review the form, then save. Nothing is saved yet.`,
          },
        }),
      );
    },

    // ---- Display helpers -----------------------------------------------------

    when(rev) {
      const date = new Date(rev.createdAt);
      return Number.isNaN(date.getTime()) ? rev.createdAt : date.toLocaleString();
    },

    sourceText(rev) {
      return rev.source === 'restore' && rev.restoredFrom ? `restore (from #${rev.restoredFrom})` : rev.source;
    },

    marker(op) {
      return op === 'insert' ? '+' : op === 'delete' ? '-' : ' ';
    },

    srText(op) {
      return op === 'insert' ? 'Added: ' : op === 'delete' ? 'Removed: ' : '';
    },

    rowClass(op) {
      if (op === 'insert') return 'bg-green-50 text-green-900';
      if (op === 'delete') return 'bg-red-50 text-red-900';
      return 'text-stone-700';
    },

    gapText(row) {
      return `${row.gap} unchanged ${row.gap === 1 ? 'line' : 'lines'}`;
    },

    _input(name) {
      return this._form ? this._form.querySelector(`input[name="${name}"]`) : null;
    },

    _readField(name) {
      const input = this._input(name);
      return input ? input.value : '';
    },

    async _errorText(resp) {
      const body = await resp.json().catch(() => null);
      return (body && (body.error || body.Error)) || `Request failed (${resp.status})`;
    },
  };
  return /** @type {typeof component & SlotHistoryMagics} */ (component);
}
