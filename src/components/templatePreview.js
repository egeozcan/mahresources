// templatePreview drives the live preview pane on the category / resource
// category / note type edit forms. It renders a selected Custom* template slot
// against a real entity via the server preview endpoint, into a sandboxed
// iframe. The CSRF token is attached automatically by the global fetch wrapper
// (src/csrf.js).

import * as userSettings from '../userSettings.js';

// `only` restricts a slot to one carrier: the surface it renders on exists for
// that carrier alone (there is no group table view, no note lightbox), so the
// field is not on the other two models and offering it would preview nothing.
const SLOTS = [
  { name: 'CustomHeader', label: 'Header' },
  { name: 'CustomDetailFooter', label: 'Detail Footer' },
  { name: 'CustomSidebar', label: 'Sidebar' },
  { name: 'CustomPreview', label: 'Preview', only: 'resource' },
  { name: 'CustomLightbox', label: 'Lightbox', only: 'resource' },
  { name: 'CustomSummary', label: 'Summary' },
  { name: 'CustomAvatar', label: 'Avatar' },
  { name: 'CustomHoverCard', label: 'Hover Card' },
  { name: 'CustomCell', label: 'Table Cell', only: 'resource' },
  { name: 'CustomOwnEntities', label: 'Own Entities', only: 'group' },
  { name: 'CustomListHeader', label: 'List Header' },
  { name: 'CustomListFooter', label: 'List Footer' },
  { name: 'CustomMRQLResult', label: 'MRQL Result' },
  { name: 'CustomCSS', label: 'CSS' },
];

export function slotsForEntityType(entityType) {
  return SLOTS.filter((s) => !s.only || s.only === entityType);
}

// Carrier slots render against the category/type itself (not a member entity),
// so the preview uses carrier mode: no member entity picked, categoryId required.
const CARRIER_SLOTS = new Set(['CustomListHeader', 'CustomListFooter']);

const LIST_ENDPOINTS = {
  group: '/v1/groups',
  resource: '/v1/resources',
  note: '/v1/notes',
};

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

// Query parameter that restricts each list endpoint to one category/type.
const CATEGORY_PARAMS = {
  group: 'categoryId',
  resource: 'resourceCategoryId',
  note: 'noteTypeId',
};

export function templatePreview({ entityType = 'group', previewPath = '', categoryId = null, generatePath = '' } = {}) {
  return {
    entityType,
    previewPath,
    generatePath,
    categoryId: categoryId || null,
    slots: slotsForEntityType(entityType),
    slot: 'CustomHeader',
    entityId: null,
    entityLabel: '',
    query: '',
    suggestions: [],
    open: false,
    loading: false,
    error: '',
    // Set when the preview fell back to an entity outside this category because
    // the category has no members yet. The pane says so, since the author would
    // otherwise reasonably assume the sample carries the category's own meta
    // (WS6, finding 29).
    usingUnscopedSample: false,
    issues: [],
    _refreshTimer: null,
    _searchTimer: null,
    _form: null,
    _refreshSeq: 0,

    async init() {
      this._form = this.$root.closest('form');

      // Publish carrier + selection to a shared store so the per-slot generate
      // buttons (on separate codeEditor islands) can ground on the same sample
      // entity without threading context through every editor include.
      this._publishStore();

      // Restore last-used entity for this entity type.
      try {
        const saved = JSON.parse(localStorage.getItem(this._storageKey()) || 'null');
        if (saved && saved.id) {
          this.entityId = saved.id;
          this.entityLabel = saved.label || `#${saved.id}`;
          this.query = this.entityLabel;
        }
      } catch (e) {
        /* ignore malformed storage */
      }

      if (!this.entityId) {
        await this._loadDefaultEntity();
      }

      if (this._form) {
        this._form.addEventListener('template-slot-changed', (e) => {
          const changed = e.detail && e.detail.name;
          if (changed === this.slot || changed === 'CustomCSS') {
            this._scheduleRefresh();
          }
        });
      }

      if (this.entityId) this.refresh();
    },

    _storageKey() {
      // Keyed per category so a remembered entity from another category (or
      // from the create form) is never restored into a scoped editor.
      return `templatePreview:${this.entityType}:${this.categoryId || 'any'}`;
    },

    // _publishStore mirrors the current carrier + selected entity into the global
    // Alpine store the generate buttons read on click.
    _publishStore() {
      const A = window.Alpine;
      if (!A || typeof A.store !== 'function') return;
      let s = A.store('templatePreview');
      if (!s) {
        A.store('templatePreview', {});
        s = A.store('templatePreview');
      }
      s.entityType = this.entityType;
      s.categoryId = this.categoryId ? Number(this.categoryId) : 0;
      s.entityId = this.entityId ? Number(this.entityId) : 0;
      s.generatePath = this.generatePath;
    },

    // _scopeParam returns the query-string fragment restricting list requests
    // to the category being edited, or '' on the create form (a new category
    // cannot have entities yet, so the pick falls back to all entities there).
    _scopeParam() {
      if (!this.categoryId) return '';
      return `&${CATEGORY_PARAMS[this.entityType]}=${encodeURIComponent(this.categoryId)}`;
    },

    slotLabel() {
      const s = this.slots.find((x) => x.name === this.slot);
      return s ? s.label : this.slot;
    },

    // isCarrierSlot reports whether the selected slot renders against the
    // category/type itself (carrier mode) rather than a member entity.
    isCarrierSlot() {
      return CARRIER_SLOTS.has(this.slot);
    },

    hasErrors() {
      return this.issues.some((i) => i.severity === 'error');
    },

    async _loadDefaultEntity() {
      try {
        const resp = await fetch(`${LIST_ENDPOINTS[this.entityType]}?maxResults=1${this._scopeParam()}`);
        if (!resp.ok) return;
        const first = normalizeList(await resp.json())[0];
        if (first) {
          this._selectEntity(first);
          return;
        }
      } catch (e) {
        /* fall through to the unscoped sample below */
      }

      // WS6 finding 29: on the edit form of a category that has no members yet,
      // the scoped lookup can only ever return nothing, so the preview pane
      // stayed an unexplained 384px void — at exactly the moment an author most
      // needs it. Fall back to any entity of the right type so the template can
      // still be seen rendering, and say that is what happened. The scoping is
      // deliberate and is NOT dropped in general: a category-scoped template
      // should normally render against an entity that carries the category.
      if (!this._scopeParam()) return;
      try {
        const resp = await fetch(`${LIST_ENDPOINTS[this.entityType]}?maxResults=1`);
        if (!resp.ok) return;
        const first = normalizeList(await resp.json())[0];
        if (first) {
          this.usingUnscopedSample = true;
          this._selectEntity(first);
        }
      } catch (e) {
        /* nothing to preview against at all */
      }
    },

    onSearchInput() {
      if (this._searchTimer) clearTimeout(this._searchTimer);
      this._searchTimer = setTimeout(() => this._search(), 250);
    },

    async _search() {
      const q = this.query.trim();
      if (!q) {
        this.suggestions = [];
        this.open = false;
        return;
      }
      try {
        const resp = await fetch(
          `${LIST_ENDPOINTS[this.entityType]}?name=${encodeURIComponent(q)}&maxResults=8${this._scopeParam()}`,
        );
        if (!resp.ok) return;
        this.suggestions = normalizeList(await resp.json())
          .slice(0, 8)
          .map((it) => ({ id: entityId(it), name: entityName(it) }));
        this.open = this.suggestions.length > 0;
      } catch (e) {
        /* ignore search failure */
      }
    },

    _selectEntity(it) {
      const id = entityId(it);
      const name = entityName(it);
      this.entityId = id;
      this.entityLabel = name;
      this.query = name;
      this.suggestions = [];
      this.open = false;
      try {
        localStorage.setItem(this._storageKey(), JSON.stringify({ id, label: name }));
      } catch (e) {
        /* ignore quota errors */
      }
      this._publishStore();
    },

    pick(it) {
      this._selectEntity(it);
      this.refresh();
    },

    onSlotChange() {
      this.refresh();
    },

    _scheduleRefresh() {
      if (this._refreshTimer) clearTimeout(this._refreshTimer);
      this._refreshTimer = setTimeout(() => this.refresh(), 700);
    },

    _readSlot(name) {
      if (!this._form) return '';
      const input = this._form.querySelector(`input[name="${name}"]`);
      return input ? input.value : '';
    },

    async refresh() {
      if (!this.previewPath) return;
      const carrier = this.isCarrierSlot();
      if (carrier) {
        // A list-header slot renders against the category itself, which must
        // already exist. On the create form there is no carrier yet.
        if (!this.categoryId) {
          this.error = 'Save this category first, then reopen the form to preview the list header.';
          this.issues = [];
          this._renderFrame('', '', null);
          return;
        }
      } else if (!this.entityId) {
        // Nothing to render against, and nothing said about it. Match the
        // carrier branch above: explain, and blank the frame so the pane is not
        // a silent void (WS6, finding 29).
        this.error =
          'Nothing to preview against yet — this category has no ' +
          `${this.entityType}s, and none exist to borrow. Pick one above, or create one first.`;
        this.issues = [];
        this._renderFrame('', '', null);
        return;
      }
      // Concurrent refreshes (slot switch racing a debounced edit) can resolve
      // out of order; only the newest request may touch the pane state.
      const seq = ++this._refreshSeq;
      this.loading = true;
      this.error = '';
      const content = this._readSlot(this.slot);
      const css = this._readSlot('CustomCSS');
      try {
        const body = carrier
          ? { carrier: true, content, css, categoryId: Number(this.categoryId) }
          : {
              entityId: Number(this.entityId),
              content,
              css,
              categoryId: this.categoryId ? Number(this.categoryId) : 0,
            };
        const resp = await fetch(this.previewPath, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(body),
        });
        if (seq !== this._refreshSeq) return;
        if (!resp.ok) {
          const err = await resp.json().catch(() => ({}));
          if (seq !== this._refreshSeq) return;
          this.error = err.error || `Preview failed (${resp.status})`;
          this.loading = false;
          return;
        }
        const data = await resp.json();
        if (seq !== this._refreshSeq) return;
        this.issues = data.issues || [];
        this._renderFrame(data.html || '', data.css || '', data.entity);
      } catch (e) {
        if (seq !== this._refreshSeq) return;
        this.error = 'Preview request failed.';
      }
      this.loading = false;
    },

    _renderFrame(html, css, entity) {
      const frame = this.$refs.frame;
      if (!frame) return;
      // Escape "<" inside the JSON so entity content containing "</script>"
      // cannot break out of the inline script block.
      const entityJson = JSON.stringify(entity ?? null).replace(/</g, '\\u003c');
      // Self-contained document: the same stylesheets base.tpl ships, the
      // returned CustomCSS, and the app JS bundle (so [meta] web components and
      // Alpine widgets hydrate — /public/ is served with a CORS header because
      // module scripts are CORS-fetched from this frame's opaque origin).
      // The rendered slot is wrapped in the same x-data="{ entity: ... }"
      // scope the display pages provide, so Alpine expressions like
      // x-text="entity.Name" behave as they will on the real page.
      // sandbox="allow-scripts" (no allow-same-origin) keeps it origin-isolated;
      // API-backed widgets degrade gracefully.
      //
      // Finding 95: the bundle's user-settings module used to GET
      // /v1/account/settings from inside here, which is cross-origin from an
      // opaque origin and produced six console errors per render (and grew:
      // 6 -> 18 -> 30 across three Refreshes). The host page already holds those
      // values, so it hands them over in the document instead. See
      // src/userSettings.js — a seeded page serves reads from the snapshot and
      // never touches the network.
      const settingsJson = JSON.stringify(userSettings.snapshot()).replace(/</g, '\\u003c');
      frame.srcdoc = `<!doctype html><html><head>
<meta charset="utf-8">
<link rel="stylesheet" href="/public/index.css">
<link rel="stylesheet" href="/public/tailwind.css">
<link rel="stylesheet" href="/public/jsonTable.css">
<style>${css}</style>
<style>body{margin:0;padding:1rem;background:#fff;color:#1c1917;}</style>
<script>window.__previewEntity = ${entityJson};
window.__mahUserSettings = ${settingsJson};</script>
</head><body>
<div x-data="{ entity: window.__previewEntity }">
${html}
</div>
<script type="module" src="/public/dist/main.js"></script>
</body></html>`;
    },
  };
}
