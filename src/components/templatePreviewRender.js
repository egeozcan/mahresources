// Pure pieces of the template slot preview, shared by the per-slot previews on
// the category / resource category / note type edit forms (slotPreview.js) and
// the slot history dialog (slotHistory.js): the slot list, the CSS cascade, the
// request body, the frame document, and the render-error scan.

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

// The shared stylesheet. Production has three sinks for it and every one of
// them renders it as stylesheet content and nothing else: the {% custom_css %}
// tag on detail and list pages
// (server/template_handlers/template_filters/custom_css_tag.go), the per-card
// block the /mrql API and the inline [mrql] shortcode prepend (mrqlCategoryCSS,
// renderResultCSS), and the share page's own head block (shared/displayNote.tpl).
// None of them ever puts the buffer in body flow, so it has no preview of its own.
export const CSS_SLOT = 'CustomCSS';

const PICKER_SLOT = 'CustomEntityPickerResult';
const PICKER_CSS = 'CustomEntityPickerResultCSS';

export function slotsForEntityType(entityType) {
  return SLOTS.filter((s) => !s.only || s.only === entityType);
}

// Carrier slots render against the category/type itself (not a member entity),
// so the preview uses carrier mode: no member entity picked, categoryId required.
const CARRIER_SLOTS = new Set(['CustomListHeader', 'CustomListFooter']);

export function isCarrierSlot(slot) {
  return CARRIER_SLOTS.has(slot.replace(/CSS$/, ''));
}

export function isPickerSlot(slot) {
  return slot === PICKER_SLOT || slot === PICKER_CSS;
}

// cssCascade matches the production cascade (TemplateCSSParts joined by
// JoinCSS): shared styles, then companion styles, each field's template
// comments removed on their own so an unclosed one cannot hide the next field.
// The entity picker loads only the shared styles and its own companion.
// `read` answers a field's current text.
export function cssCascade(entityType, slot, read) {
  const fields = isPickerSlot(slot)
    ? [CSS_SLOT, PICKER_CSS]
    : [
        CSS_SLOT,
        ...slotsForEntityType(entityType)
          .filter((s) => s.name !== CSS_SLOT && s.name !== PICKER_CSS && s.name.endsWith('CSS'))
          .map((s) => s.name),
      ];
  return fields.map((name) => stripTemplateComments(read(name) || '')).filter(Boolean).join('\n');
}

// previewBody is the JSON the preview endpoint takes. The slot names which
// buffer `content` is, so the server lints it as markup or as a stylesheet.
export function previewBody({ slot, content, css, categoryId, entityId }) {
  return isCarrierSlot(slot)
    ? { carrier: true, slot, content, css, categoryId: Number(categoryId) }
    : { entityId: Number(entityId), slot, content, css, categoryId: categoryId ? Number(categoryId) : 0 };
}

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
// attribute, a tooltip an author has to find by hovering, so the preview spells
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

// frameDocument builds the sandboxed frame's srcdoc for one rendered slot.
//
// Self-contained document: the same stylesheets base.tpl ships, the returned
// CustomCSS, and the app JS bundle (so [meta] web components and Alpine widgets
// hydrate; /public/ is served with a CORS header because module scripts are
// CORS-fetched from this frame's opaque origin).
//
// The frame's own body reset is preview chrome with no counterpart on a real
// page, so it is emitted BEFORE the author's CSS rather than after: base.tpl
// links the app stylesheets and only then renders {% block head %}, where the
// custom_css tag writes its <style>, so nothing of the page's own outranks the
// author at equal specificity. That is the whole of what the ordering fixes:
// production's body carries `class="site bg-stone-50"`, and a class beats a bare
// `body` selector, so padding and background are the page's whichever way this
// frame is ordered. The classes are deliberately not copied here: `.site` is a
// four-row page grid written for the full shell, and this frame holds one slot
// fragment.
//
// The rendered slot is wrapped in the same x-data="{ entity: ... }" scope the
// display pages provide, so Alpine expressions like x-text="entity.Name" behave
// as they will on the real page; the picker result gets its own wrapper class
// and no Alpine, as the picker renders it. sandbox="allow-scripts" (no
// allow-same-origin) keeps the frame origin-isolated.
//
// Finding 95: the bundle's user-settings module used to GET
// /v1/account/settings from inside here, which is cross-origin from an opaque
// origin and produced console errors per render. The host page already holds
// those values, so it hands them over in the document instead.
export function frameDocument(html, css, entity, slot) {
  // Escape "<" inside the JSON so entity content containing "</script>" cannot
  // break out of the inline script block.
  const entityJson = JSON.stringify(entity ?? null).replace(/</g, '\\u003c');
  const settingsJson = JSON.stringify(userSettings.snapshot()).replace(/</g, '\\u003c');
  const wrapper = isPickerSlot(slot) ? 'class="entity-picker-result" x-ignore' : 'x-data="{ entity: window.__previewEntity }"';
  return `<!doctype html><html><head>
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
}

// ensurePreviewStore returns the store the shared entity picker publishes and
// every preview reads: the carrier, the chosen entity, and `entityGen`, bumped
// on every entity change so a preview can watch one value. `ready` turns true
// once the picker has settled on an entity, or on none.
export function ensurePreviewStore() {
  const A = window.Alpine;
  if (!A || typeof A.store !== 'function') return null;
  if (!A.store('templatePreview')) {
    A.store('templatePreview', {
      entityType: '',
      categoryId: 0,
      entityId: 0,
      generatePath: '',
      entityGen: 0,
      ready: false,
    });
  }
  return A.store('templatePreview');
}
