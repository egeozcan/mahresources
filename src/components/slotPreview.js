// slotPreview renders one Custom* template slot, the one its cluster edits,
// against the entity the shared "Preview against" picker chose
// (templatePreview.js), into a sandboxed iframe at the end of that cluster.
//
// It renders lazily: the first time it comes into view, and after an edit to
// its own slot or to any CSS field while it is in view. An edit or an entity
// change while it is off screen only marks it stale, and it renders when it is
// next seen. Every invalidation bumps a generation and aborts the request in
// flight, so only the newest render may touch the preview.

import {
  cssCascade,
  ensurePreviewStore,
  frameDocument,
  isCarrierSlot,
  previewBody,
  renderErrorsFrom,
} from './templatePreviewRender.js';
import { REFRESH_ALL_EVENT } from './templatePreview.js';
import { REVISION_WRITE_EVENT } from './templateBundle.js';

const EDIT_DEBOUNCE_MS = 700;

/**
 * @typedef {object} SlotPreviewMagics
 * @property {{ frame?: HTMLIFrameElement }} $refs The x-ref registry.
 * @property {HTMLElement} $root The element carrying this component's x-data.
 * @property {(expr: string, cb: (value: unknown) => void) => void} $watch Alpine's watcher.
 */

/**
 * @param {object} [options]
 * @param {string} [options.entityType] Carrier entity type: group, resource or note.
 * @param {string} [options.slot] The slot this preview renders.
 * @param {string} [options.previewPath] Endpoint that renders one slot.
 * @param {number|string|null} [options.categoryId] The category/type being edited; null on the create form.
 */
export function slotPreview({ entityType = 'group', slot = 'CustomHeader', previewPath = '', categoryId = null } = {}) {
  const component = {
    entityType,
    slot,
    previewPath,
    categoryId: categoryId || null,
    /** @type {number|string|null} */
    entityId: null,
    // False until the shared picker has settled on an entity (or on none).
    entityReady: false,
    // Shown as "Choosing an entity..." while the picker has not settled.
    waiting: false,
    loading: false,
    error: '',
    // What the slot history dialog last loaded into this slot's editor.
    appliedNote: '',
    /** @type {{ severity: string, message: string, start?: number, end?: number }[]} */
    issues: [],
    /** @type {{ label: string, detail: string, count: number }[]} */
    renderErrors: [],
    visible: false,
    stale: true,
    _refreshSeq: 0,
    /** @type {AbortController|null} */
    _abort: null,
    /** @type {ReturnType<typeof setTimeout>|null} */
    _timer: null,
    /** @type {HTMLFormElement|null} */
    _form: null,
    /** @type {IntersectionObserver|null} */
    _observer: null,

    init() {
      this._form = this.$root.closest('form');
      const store = ensurePreviewStore();
      if (store) this._syncEntity(store);
      this.$watch('$store.templatePreview.entityGen', () => {
        const s = ensurePreviewStore();
        if (s) this._syncEntity(s);
        this.invalidate(false);
      });
      this._form?.addEventListener('template-slot-changed', (e) => {
        const changed = /** @type {CustomEvent} */ (e).detail?.name;
        if (changed === this.slot || changed?.endsWith('CSS')) this.invalidate(true);
      });
      this._form?.addEventListener(REFRESH_ALL_EVENT, () => this.invalidate(false));
      document.addEventListener(REVISION_WRITE_EVENT, (e) => {
        const detail = /** @type {CustomEvent} */ (e).detail || {};
        if ((detail.fields || []).includes(this.slot)) this.appliedNote = detail.notice || '';
      });
      if (typeof IntersectionObserver === 'function') {
        this._observer = new IntersectionObserver(
          (entries) => {
            this.visible = entries.some((entry) => entry.isIntersecting);
            if (this.visible && this.stale) this.refresh();
          },
          { rootMargin: '200px 0px' },
        );
        this._observer.observe(this.$root);
      } else {
        this.visible = true;
        this.refresh();
      }
    },

    destroy() {
      this._observer?.disconnect();
      this._abort?.abort();
      if (this._timer) clearTimeout(this._timer);
    },

    _syncEntity(store) {
      this.entityId = store.entityId || null;
      this.entityReady = !!store.ready;
    },

    // invalidate retires the render in flight and either schedules a new one
    // (in view) or leaves the preview stale until it is seen.
    invalidate(debounce) {
      this._refreshSeq++;
      this._abort?.abort();
      this._abort = null;
      this.stale = true;
      if (this._timer) clearTimeout(this._timer);
      this._timer = null;
      if (!this.visible) return;
      // Rechecked when the pause ends: a preview scrolled away meanwhile stays
      // stale and renders when it is next seen.
      if (debounce) this._timer = setTimeout(() => this.visible && this.refresh(), EDIT_DEBOUNCE_MS);
      else this.refresh();
    },

    _readField(name) {
      if (!this._form) return '';
      const input = this._form.querySelector(`input[name="${name}"]`);
      return input ? input.value : '';
    },

    // _refuse answers without a request: it explains, blanks the frame, and
    // retires anything in flight, whose response would otherwise land after it.
    _refuse(message) {
      this.waiting = false;
      this.error = message;
      this.issues = [];
      this.renderErrors = [];
      this.loading = false;
      this._renderFrame('', '', null);
    },

    async refresh() {
      if (!this.previewPath) return;
      const seq = ++this._refreshSeq;
      this._abort?.abort();
      this._abort = null;
      if (this._timer) clearTimeout(this._timer);
      this._timer = null;
      this.stale = false;
      if (isCarrierSlot(this.slot)) {
        // A list-header slot renders against the category itself, which must
        // already exist. On the create form there is no carrier yet.
        if (!this.categoryId) {
          this._refuse('Save this category first, then reopen the form to preview the list header.');
          return;
        }
      } else if (!this.entityReady) {
        // The picker publishes once it settles, and that renders this again.
        this._refuse('');
        this.waiting = true;
        this.stale = true;
        return;
      } else if (!this.entityId) {
        // Nothing to render against, and nothing said about it otherwise
        // (WS6, finding 29).
        this._refuse(
          'Nothing to preview against yet: this category has no ' +
            `${this.entityType}s, and none exist to borrow. Pick one above, or create one first.`,
        );
        return;
      }
      this.waiting = false;
      this.loading = true;
      this.error = '';
      const body = previewBody({
        slot: this.slot,
        content: this._readField(this.slot),
        css: cssCascade(this.entityType, this.slot, (name) => this._readField(name)),
        categoryId: this.categoryId,
        entityId: this.entityId,
      });
      const abort = typeof AbortController === 'function' ? new AbortController() : null;
      this._abort = abort;
      try {
        const resp = await fetch(this.previewPath, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(body),
          signal: abort?.signal,
        });
        if (seq !== this._refreshSeq) return;
        if (!resp.ok) {
          const err = await resp.json().catch(() => ({}));
          if (seq !== this._refreshSeq) return;
          this.error = err.error || `Preview failed (${resp.status})`;
          this.renderErrors = [];
          this.loading = false;
          return;
        }
        const data = await resp.json();
        if (seq !== this._refreshSeq) return;
        // Two lists because a lint issue's offsets index one buffer or the
        // other; the preview shows severity and message, so it reads them as one.
        this.issues = [...(data.issues || []), ...(data.cssIssues || [])];
        // A shortcode failing in a stylesheet buffer renders its marker there too.
        this.renderErrors = renderErrorsFrom((data.html || '') + (data.css || ''));
        this._renderFrame(data.html || '', data.css || '', data.entity);
      } catch (e) {
        if (seq !== this._refreshSeq) return;
        this.error = 'Preview request failed.';
        this.renderErrors = [];
      }
      this.loading = false;
    },

    _renderFrame(html, css, entity) {
      const frame = this.$refs.frame;
      if (!frame) return;
      frame.srcdoc = frameDocument(html, css, entity, this.slot);
    },
  };
  return /** @type {typeof component & SlotPreviewMagics} */ (component);
}
