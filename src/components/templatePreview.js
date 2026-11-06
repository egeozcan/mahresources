// templatePreviewContext drives the "Preview against" picker at the top of the
// Templates fieldset on the category / resource category / note type edit
// forms. It chooses the one entity every slot preview (slotPreview.js) renders
// against, and publishes it to the `templatePreview` store, which the previews
// and the per-slot generate buttons read. The search field itself is the shared
// single-entity selector (src/selector/); this component scopes its searches,
// seeds it with the remembered or default entity, and listens for picks.

import { ensurePreviewStore } from './templatePreviewRender.js';

const LIST_ENDPOINTS = {
  group: '/v1/groups',
  resource: '/v1/resources',
  note: '/v1/notes',
};

// Query parameter that restricts each list endpoint to one category/type.
const CATEGORY_PARAMS = {
  group: 'categoryId',
  resource: 'resourceCategoryId',
  note: 'noteTypeId',
};

// Every slot preview listens for this on the form to re-render at once.
export const REFRESH_ALL_EVENT = 'template-preview-refresh-all';

function normalizeList(data) {
  if (Array.isArray(data)) return data;
  if (data && Array.isArray(data.items)) return data.items;
  if (data && Array.isArray(data.results)) return data.results;
  return [];
}

function entityId(it) {
  return it.ID ?? it.id;
}

function entityName(it) {
  return it.Name ?? it.name ?? `#${entityId(it)}`;
}

/**
 * Alpine injects $root through the component's scope chain rather than onto
 * the data object, so it is declared as a type and never as a property.
 *
 * @typedef {object} TemplatePreviewContextMagics
 * @property {HTMLElement} $root The element carrying this component's x-data.
 * @property {(callback: () => void) => void} $nextTick Runs the callback after Alpine's next render.
 */

/**
 * @param {object} [options]
 * @param {string} [options.entityType] Carrier entity type: group, resource or note.
 * @param {number|string|null} [options.categoryId] The category/type being edited; null on the create form.
 * @param {string} [options.generatePath] Endpoint behind the per-slot generate buttons.
 */
export function templatePreviewContext({ entityType = 'group', categoryId = null, generatePath = '' } = {}) {
  const component = {
    entityType,
    generatePath,
    categoryId: categoryId || null,
    /** @type {number|string|null} */
    entityId: null,
    entityLabel: '',
    // Set when the preview fell back to an entity outside this category because
    // the category has no members yet. Said once here, since the author would
    // otherwise reasonably assume the sample carries the category's own meta
    // (WS6, finding 29).
    usingUnscopedSample: false,
    ready: false,
    /** @type {HTMLElement|null} */
    _fieldEl: null,
    // Bumped by every deliberate pick, so a default lookup that answers after
    // one is discarded instead of replacing it.
    _pickSeq: 0,

    async init() {
      // Captured here: $root is the calling element inside a method.
      this._fieldEl = this.$root?.querySelector('[data-tp-entity-field] [data-selector-profile]');
      // Restore last-used entity for this entity type.
      try {
        const saved = JSON.parse(localStorage.getItem(this._storageKey()) || 'null');
        if (saved && saved.id) {
          this.entityId = saved.id;
          this.entityLabel = saved.label || `#${saved.id}`;
          // The selector initializes after this component, so seed it once it has.
          this.$nextTick?.(() => this._showInSelector());
        }
      } catch (e) {
        /* ignore malformed storage */
      }

      // The carrier and the generate endpoint first, unsettled: the generate
      // buttons need them whether or not an entity is ever found.
      this._publishStore();
      const pickSeq = this._pickSeq;
      if (!this.entityId) await this._loadDefaultEntity(pickSeq);
      // A pick made meanwhile has already published.
      if (pickSeq !== this._pickSeq) return;
      this.ready = true;
      this._publishStore();
    },

    _storageKey() {
      // Keyed per category so a remembered entity from another category (or
      // from the create form) is never restored into a scoped editor.
      return `templatePreview:${this.entityType}:${this.categoryId || 'any'}`;
    },

    // _publishStore mirrors the carrier and the chosen entity into the store,
    // bumping entityGen so every preview re-renders against it.
    _publishStore() {
      const s = ensurePreviewStore();
      if (!s) return;
      s.entityType = this.entityType;
      s.categoryId = this.categoryId ? Number(this.categoryId) : 0;
      s.entityId = this.entityId ? Number(this.entityId) : 0;
      s.generatePath = this.generatePath;
      s.ready = this.ready;
      s.entityGen = (s.entityGen || 0) + 1;
    },

    // _scopeParam returns the query-string fragment restricting list requests
    // to the category being edited, or '' on the create form (a new category
    // cannot have entities yet, so the pick falls back to all entities there).
    _scopeParam() {
      if (!this.categoryId) return '';
      return `&${CATEGORY_PARAMS[this.entityType]}=${encodeURIComponent(this.categoryId)}`;
    },

    // The same scope as request parameters for the shared selector's searches
    // and its Browse dialog, which call this per request.
    scopeParameters() {
      if (!this.categoryId) return {};
      return { [CATEGORY_PARAMS[this.entityType]]: this.categoryId };
    },

    // _showInSelector mirrors the chosen entity into the shared field without
    // announcing it as a pick: the selector did not choose it.
    _showInSelector() {
      const field = this._fieldEl && globalThis.Alpine?.$data?.(this._fieldEl);
      if (!field?._replaceSelection) return;
      field._replaceSelection(this.entityId ? [{ ID: this.entityId, Name: this.entityLabel }] : [], { silent: true });
    },

    async _loadDefaultEntity(pickSeq) {
      const stale = () => pickSeq !== this._pickSeq;
      try {
        const resp = await fetch(`${LIST_ENDPOINTS[this.entityType]}?maxResults=1${this._scopeParam()}`);
        if (stale()) return;
        if (resp.ok) {
          const first = normalizeList(await resp.json())[0];
          if (stale()) return;
          if (first) {
            this._selectEntity(first);
            return;
          }
        }
      } catch (e) {
        /* fall through to the unscoped sample below */
      }

      // WS6 finding 29: on the edit form of a category that has no members yet,
      // the scoped lookup can only ever return nothing. Fall back to any entity
      // of the right type so the template can still be seen rendering, and say
      // that is what happened.
      if (!this._scopeParam() || stale()) return;
      try {
        const resp = await fetch(`${LIST_ENDPOINTS[this.entityType]}?maxResults=1`);
        if (!resp.ok || stale()) return;
        const first = normalizeList(await resp.json())[0];
        if (first && !stale()) {
          this.usingUnscopedSample = true;
          this._selectEntity(first);
        }
      } catch (e) {
        /* nothing to preview against at all */
      }
    },

    _selectEntity(it) {
      const id = entityId(it);
      const name = entityName(it);
      this.entityId = id;
      this.entityLabel = name;
      this._showInSelector();
      try {
        localStorage.setItem(this._storageKey(), JSON.stringify({ id, label: name }));
      } catch (e) {
        /* ignore quota errors */
      }
      // Only a settled picker publishes: before then the previews say they are
      // still choosing, and init() publishes once with whatever it found.
      if (this.ready) this._publishStore();
    },

    pick(it) {
      this._pickSeq++;
      // A pick is a settled choice even while the default lookup is pending.
      this.usingUnscopedSample = false;
      this.ready = true;
      this._selectEntity(it);
    },

    // onEntityChange receives the shared selector's change record for a pick
    // or a removal made in the field. Removing the chip leaves nothing to
    // preview against until another entity is picked.
    onEntityChange(change) {
      const picked = change.current[0]?.raw;
      if (picked) {
        this.pick(picked);
        return;
      }
      this._pickSeq++;
      this.usingUnscopedSample = false;
      this.ready = true;
      this.entityId = null;
      this.entityLabel = '';
      try {
        localStorage.removeItem(this._storageKey());
      } catch (e) {
        /* ignore storage errors */
      }
      this._publishStore();
    },

    refreshAll() {
      this.$root.closest('form')?.dispatchEvent(new CustomEvent(REFRESH_ALL_EVENT));
    },
  };
  return /** @type {typeof component & TemplatePreviewContextMagics} */ (component);
}
