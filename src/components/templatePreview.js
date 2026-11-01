// templatePreview drives the live preview pane on the category / resource
// category / note type edit forms. It renders a selected Custom* template slot
// against a real entity via the server preview endpoint, into a sandboxed
// iframe. The CSRF token is attached automatically by the global fetch wrapper
// (src/csrf.js).

import * as userSettings from '../userSettings.js';
import { stripTemplateComments } from './templateCommentRanges.js';

// `only` restricts a slot to one carrier: the surface it renders on exists for
// that carrier alone (there is no group table view, no note lightbox), so the
// field is not on the other two models and offering it would preview nothing.
const SLOTS = [
  { name: 'CustomHeader', label: 'Header' },
  { name: 'CustomHeaderCSS', label: 'Header CSS' },
  { name: 'CustomDetailFooter', label: 'Detail Footer' },
  { name: 'CustomDetailFooterCSS', label: 'Detail Footer CSS' },
  { name: 'CustomSidebar', label: 'Sidebar' },
  { name: 'CustomSidebarCSS', label: 'Sidebar CSS' },
  { name: 'CustomPreview', label: 'Preview', only: 'resource' },
  { name: 'CustomPreviewCSS', label: 'Preview CSS', only: 'resource' },
  { name: 'CustomLightbox', label: 'Lightbox', only: 'resource' },
  { name: 'CustomLightboxCSS', label: 'Lightbox CSS', only: 'resource' },
  { name: 'CustomSummary', label: 'Summary' },
  { name: 'CustomSummaryCSS', label: 'Summary CSS' },
  { name: 'CustomAvatar', label: 'Avatar' },
  { name: 'CustomAvatarCSS', label: 'Avatar CSS' },
  { name: 'CustomHoverCard', label: 'Hover Card' },
  { name: 'CustomHoverCardCSS', label: 'Hover Card CSS' },
  { name: 'CustomCell', label: 'Table Cell', only: 'resource' },
  { name: 'CustomCellCSS', label: 'Table Cell CSS', only: 'resource' },
  { name: 'CustomOwnEntities', label: 'Own Entities', only: 'group' },
  { name: 'CustomOwnEntitiesCSS', label: 'Own Entities CSS', only: 'group' },
  { name: 'CustomListHeader', label: 'List Header' },
  { name: 'CustomListHeaderCSS', label: 'List Header CSS' },
  { name: 'CustomListFooter', label: 'List Footer' },
  { name: 'CustomListFooterCSS', label: 'List Footer CSS' },
  { name: 'CustomMRQLResult', label: 'MRQL Result' },
  { name: 'CustomMRQLResultCSS', label: 'MRQL Result CSS' },
  { name: 'CustomEntityPickerResult', label: 'Entity Picker Result' },
  { name: 'CustomEntityPickerResultCSS', label: 'Entity Picker Result CSS' },
  { name: 'CustomCSS', label: 'CSS' },
];

// CSS slots hold stylesheets rather than markup. Production has
// three sinks for it and every one of them renders it as stylesheet content and
// nothing else: the {% custom_css %} tag on detail and list pages
// (server/template_handlers/template_filters/custom_css_tag.go), the per-card
// block the /mrql API and the inline [mrql] shortcode prepend (mrqlCategoryCSS,
// renderResultCSS), and the share page's own head block (shared/displayNote.tpl).
// None of them ever puts the buffer in body flow.
const CSS_SLOT = 'CustomCSS';

// Per-browser layout choice: show the pane under the slot being edited rather
// than below the whole Templates fieldset.
const DOCKED_KEY = 'templatePreview:docked';

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

// The server's failure markers, exactly as shortcodes/markers.go,
// mrql_handler.go and conditional_handler.go write them. They are matched in
// the raw text rather than in a parsed document, because a marker can land
// where a parser does not see elements: inside a <style> (a raw-text element),
// a <template> body, or a CSS buffer. Their text and title are HTML-escaped by
// the server, so neither can contain "<" or '"'.
const MARKER_PATTERN =
  /<span class="shortcode-error[^"]*" title="([^"]*)">⚠ ([^<]*)<\/span>|<(?:div|span) class="(?:mrql-results )?mrql-error[^"]*">([^<]*)<\/(?:div|span)>/g;

function decodeEntities(text) {
  return new DOMParser().parseFromString(`<!doctype html><body>${text}`, 'text/html').body.textContent || '';
}

// renderErrorsFrom lists the shortcode failures in a rendered slot. A failing
// shortcode renders a short marker in place with the reason only in its title
// attribute, a tooltip an author has to find by hovering, so the pane spells
// each one out under the frame. MRQL errors carry their whole message as text.
// Repeats collapse with a count, since an [each] loop repeats the same failure
// once per item.
export function renderErrorsFrom(html) {
  // Most renders are clean; skip the scan unless a marker could be present.
  if (!html || !html.includes('-error')) return [];
  /** @type {Map<string, { label: string, detail: string, count: number }>} */
  const seen = new Map();
  for (const [, title, label, mrql] of html.matchAll(MARKER_PATTERN)) {
    const err =
      mrql !== undefined
        ? { label: 'mrql', detail: decodeEntities(mrql) }
        : { label: decodeEntities(label), detail: decodeEntities(title) };
    const key = `${err.label}\u0000${err.detail}`;
    const prev = seen.get(key);
    if (prev) prev.count++;
    else seen.set(key, { ...err, count: 1 });
  }
  return [...seen.values()];
}

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

/**
 * Alpine injects these through the component's scope chain rather than onto the
 * data object, so they are declared as types and never as properties: a real
 * `$refs` on the data object shadows Alpine's magic, and `_renderFrame` would
 * stop finding the iframe.
 *
 * @typedef {object} TemplatePreviewMagics
 * @property {{ frame?: HTMLIFrameElement }} $refs The x-ref registry.
 * @property {HTMLElement} $root The element carrying this component's x-data.
 */

/**
 * @param {object} [options]
 * @param {string} [options.entityType] Carrier entity type: group, resource or note.
 * @param {string} [options.previewPath] Endpoint that renders one slot.
 * @param {number|string|null} [options.categoryId] The category/type being edited; null on the create form.
 * @param {string} [options.generatePath] Endpoint behind the per-slot generate buttons.
 */
export function templatePreview({ entityType = 'group', previewPath = '', categoryId = null, generatePath = '' } = {}) {
  const component = {
    entityType,
    previewPath,
    generatePath,
    categoryId: categoryId || null,
    slots: slotsForEntityType(entityType),
    slot: 'CustomHeader',
    /** @type {number|string|null} */
    entityId: null,
    entityLabel: '',
    query: '',
    /** @type {{ id: number, name: string }[]} */
    suggestions: [],
    open: false,
    loading: false,
    error: '',
    // Set when the preview fell back to an entity outside this category because
    // the category has no members yet. The pane says so, since the author would
    // otherwise reasonably assume the sample carries the category's own meta
    // (WS6, finding 29).
    usingUnscopedSample: false,
    /** @type {{ severity: string, message: string, start?: number, end?: number }[]} */
    issues: [],
    /** @type {{ label: string, detail: string, count: number }[]} */
    renderErrors: [],
    /** @type {ReturnType<typeof setTimeout>|null} */
    _refreshTimer: null,
    /** @type {ReturnType<typeof setTimeout>|null} */
    _searchTimer: null,
    /** @type {HTMLFormElement|null} */
    _form: null,
    _refreshSeq: 0,
    docked: false,
    // Captured in init(): the pane itself, and a marker where the template put
    // it, so undocking can return it there.
    /** @type {HTMLElement|null} */
    _pane: null,
    /** @type {Comment|null} */
    _home: null,

    async init() {
      this._form = this.$root.closest('form');
      this._pane = this.$root;
      // Docking places the pane among the form's slot clusters.
      if (this._form) {
        this._home = document.createComment('template preview home');
        this._pane.before(this._home);
        try {
          this.docked = localStorage.getItem(DOCKED_KEY) === '1';
        } catch (e) {
          /* storage unavailable: stay undocked */
        }
        // After Alpine's synchronous initial walk: it continues from this
        // element's next sibling, and a pane moved now would send it back over
        // clusters it has already initialized.
        if (this.docked) queueMicrotask(() => this._dock());
        // Docked, the pane follows the cluster focus enters. The Custom CSS
        // editor has no cluster and keeps the slot: alone it renders no markup.
        // Registered before the entity lookup below, so focus that moves while
        // it is pending is still followed; the refresh at the end of init()
        // then renders whichever slot is selected.
        this._form.addEventListener('focusin', (e) => {
          if (!this.docked || !(e.target instanceof Element) || this._pane?.contains(e.target)) return;
          const name = e.target.closest('[data-template-cluster]')?.getAttribute('data-template-cluster');
          if (!name || name === this.slot.replace(/CSS$/, '') || !this.slots.some((s) => s.name === name)) return;
          this.slot = name;
          this._dock();
          this.refresh();
        });
      }

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
      // A restore writes entityId directly rather than through _selectEntity,
      // so it is the one way the pane can hold an entity the store has never
      // heard of — and a generate button reading the store would then ground on
      // nothing while the pane showed a remembered entity by name.
      this._publishStore();

      if (!this.entityId) {
        await this._loadDefaultEntity();
      }

      if (this._form) {
        this._form.addEventListener('template-slot-changed', (e) => {
          const changed = e.detail && e.detail.name;
          if (changed === this.slot || changed?.endsWith('CSS')) {
            this._scheduleRefresh();
          }
        });
      }

      // Unconditionally: with no entity at all — a fresh deployment, or a
      // category whose members were deleted — refresh() is what says so and
      // blanks the frame. Guarded on entityId, that case reached neither, and
      // the pane sat at an unexplained 384px void, which is the very thing WS6
      // finding 29 was raised about.
      this.refresh();
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
      return CARRIER_SLOTS.has(this.slot.replace(/CSS$/, ''));
    },

    // isCSSSlot reports whether the selected buffer is a stylesheet. It reaches
    // the frame through a <style> element and nothing else, which is what every
    // production sink does with it (see CSS_SLOT), so the frame body is empty
    // while it is selected and the pane says why rather than leaving an
    // unexplained void.
    isCSSSlot() {
      return this.slot.endsWith('CSS');
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
      if (this.docked) this._dock();
      this.refresh();
    },

    onDockChange() {
      try {
        localStorage.setItem(DOCKED_KEY, this.docked ? '1' : '0');
      } catch (e) {
        /* ignore quota errors */
      }
      if (this.docked) this._dock();
      else if (this._home) this._placeAfter(this._home);
    },

    // _dock moves the pane under the selected slot's cluster, or under the
    // Custom CSS editor row for the shared stylesheet.
    _dock() {
      if (!this._form) return;
      const anchor =
        this.slot === CSS_SLOT
          ? this._form.querySelector(`input[name="${CSS_SLOT}"]`)?.parentElement?.parentElement
          : this._form.querySelector(`[data-template-cluster="${this.slot.replace(/CSS$/, '')}"]`);
      if (anchor) this._placeAfter(anchor);
    },

    // _placeAfter moves the pane only when it is not already there, since a
    // re-inserted iframe reloads its document. Alpine sees a move, not a
    // removal, so the component survives. Reparenting blurs a focused control
    // inside the pane and shifts whatever is below its old spot, so the
    // focused element is refocused and kept at the same viewport position.
    _placeAfter(anchor) {
      const pane = this._pane;
      if (!pane || anchor.nextSibling === pane) return;
      const active = document.activeElement instanceof HTMLElement && document.activeElement !== document.body
        ? document.activeElement
        : null;
      const top = active?.getBoundingClientRect().top;
      anchor.after(pane);
      if (!active || top === undefined) return;
      if (document.activeElement !== active) active.focus({ preventScroll: true });
      const delta = active.getBoundingClientRect().top - top;
      if (delta) window.scrollBy(0, delta);
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
      // Concurrent refreshes (slot switch racing a debounced edit) can resolve
      // out of order; only the newest request may touch the pane state. Claimed
      // here rather than at the fetch, because the two branches below answer
      // without sending one and must retire an in-flight request too: a
      // response for the slot just left would otherwise land afterwards and
      // overwrite the frame they had deliberately blanked. They also clear
      // `loading` for the same reason — a superseded response returns before
      // the line that would have cleared it.
      const seq = ++this._refreshSeq;
      const carrier = this.isCarrierSlot();
      if (carrier) {
        // A list-header slot renders against the category itself, which must
        // already exist. On the create form there is no carrier yet.
        if (!this.categoryId) {
          this.error = 'Save this category first, then reopen the form to preview the list header.';
          this.issues = [];
          this.renderErrors = [];
          this.loading = false;
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
        this.renderErrors = [];
        this.loading = false;
        this._renderFrame('', '', null);
        return;
      }
      this.loading = true;
      this.error = '';
      const content = this._readSlot(this.slot);
      // Match the production cascade (TemplateCSSParts joined by JoinCSS):
      // shared styles, then companion styles, each field's template comments
      // removed on their own so an unclosed one cannot hide the next field.
      const pickerSlot = this.slot === 'CustomEntityPickerResult' || this.slot === 'CustomEntityPickerResultCSS';
      const cssFields = pickerSlot
        ? [CSS_SLOT, 'CustomEntityPickerResultCSS']
        : [CSS_SLOT, ...this.slots.filter((s) => s.name !== CSS_SLOT && s.name !== 'CustomEntityPickerResultCSS' && s.name.endsWith('CSS')).map((s) => s.name)];
      const css = cssFields.map((name) => stripTemplateComments(this._readSlot(name) || '')).filter(Boolean).join('\n');
      try {
        // The slot names which buffer `content` is. With CustomCSS selected it
        // is a stylesheet, which carries no <style> wrapper to say so, and the
        // server has to be told or its issue list stays silent where the editor
        // gutter warns.
        const slot = this.slot;
        const cssSlot = slot.endsWith('CSS');
        const body = carrier
          ? { carrier: true, slot, content, css, categoryId: Number(this.categoryId) }
          : {
              entityId: Number(this.entityId),
              slot,
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
          this.renderErrors = [];
          this.loading = false;
          return;
        }
        const data = await resp.json();
        if (seq !== this._refreshSeq) return;
        // Two lists because a lint issue's offsets index one buffer or the
        // other; the pane shows severity and message, so it reads them as one.
        this.issues = [...(data.issues || []), ...(data.cssIssues || [])];
        // With the CSS slot selected the request sent one buffer as both
        // `content` and `css`, so `data.html` is the stylesheet a second time.
        // The frame renders `css` into a <style> element, which is the only
        // thing production ever does with a CustomCSS buffer; injecting the
        // other copy into the body as well printed the stylesheet's own source
        // as page text — a document no saved template can produce. Keyed on the
        // slot the request was built from, not on the two halves reading alike,
        // which for a markup slot means two documents however similar they are.
        // A shortcode failing in a stylesheet buffer renders its marker there
        // too. On the CSS slot `html` is that same buffer a second time.
        this.renderErrors = renderErrorsFrom(cssSlot ? data.css || '' : (data.html || '') + (data.css || ''));
        this._renderFrame(cssSlot ? '' : data.html || '', data.css || '', data.entity);
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
      // Escape "<" inside the JSON so entity content containing "</script>"
      // cannot break out of the inline script block.
      const entityJson = JSON.stringify(entity ?? null).replace(/</g, '\\u003c');
      // Self-contained document: the same stylesheets base.tpl ships, the
      // returned CustomCSS, and the app JS bundle (so [meta] web components and
      // Alpine widgets hydrate — /public/ is served with a CORS header because
      // module scripts are CORS-fetched from this frame's opaque origin).
      // The frame's own body reset is preview chrome with no counterpart on a
      // real page, so it is emitted BEFORE the author's CSS rather than after:
      // base.tpl links the app stylesheets and only then renders {% block head
      // %}, where the custom_css tag writes its <style>, so nothing of the
      // page's own outranks the author at equal specificity. Emitted after, the
      // chrome silently did, and a `body` rule the author wrote previewed as
      // having no effect at all.
      //
      // That is the whole of what the ordering fixes, and it is not the same as
      // reproducing a real page. Production's body carries `class="site
      // bg-stone-50"`, and a class beats a bare `body` selector: `.site` sets
      // padding (public/index.css) and `bg-stone-50` sets background, so those
      // two properties are the page's whichever way this frame is ordered. The
      // classes are deliberately not copied here — `.site` is a four-row page
      // grid written for the header/nav/main/footer shell, and this frame holds
      // one slot fragment, so wearing it would substitute a different wrong
      // context for the current one. The pane says the frame is a sandbox.
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
      const pickerSlot = this.slot === 'CustomEntityPickerResult' || this.slot === 'CustomEntityPickerResultCSS';
      const wrapper = pickerSlot
        ? 'class="entity-picker-result" x-ignore'
        : 'x-data="{ entity: window.__previewEntity }"';
      frame.srcdoc = `<!doctype html><html><head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<link rel="stylesheet" href="/public/index.css">
<link rel="stylesheet" href="/public/tailwind.css">
<link rel="stylesheet" href="/public/jsonTable.css">
<style>body{margin:0;padding:1rem;background:#fff;color:#1c1917;}</style>
<style>${css}</style>
<script>window.__previewEntity = ${entityJson};
window.__mahUserSettings = ${settingsJson};</script>
</head><body>
<div ${wrapper}>
${html}
</div>
<script type="module" src="/public/dist/main.js"></script>
</body></html>`;
    },
  };
  return /** @type {typeof component & TemplatePreviewMagics} */ (component);
}
