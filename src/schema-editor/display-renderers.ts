import { html, nothing, type TemplateResult } from 'lit';

/**
 * A built-in display renderer: a shape detector paired with a renderer.
 * Detectors are checked in order; first match wins.
 */
export interface BuiltinRenderer {
  name: string;
  detect: (val: any) => boolean;
  render: (val: any) => TemplateResult;
}

// ── URL / Location ──────────────────────────────────────────────────────────

function isURLShape(val: any): boolean {
  return (
    val != null &&
    typeof val === 'object' &&
    typeof val.href === 'string' &&
    (typeof val.host === 'string' || typeof val.hostname === 'string')
  );
}

function renderURL(val: any): TemplateResult {
  const href = val.href as string;
  const host = (val.host || val.hostname || '') as string;
  return html`
    <div>
      <a href=${href} target="_blank" rel="noopener noreferrer"
        class="text-indigo-600 hover:text-indigo-800 underline decoration-indigo-300 break-all"
        @click=${(e: Event) => e.stopPropagation()}
      >${href}</a>
      ${host ? html`<div class="text-[10px] font-mono text-stone-400 mt-0.5">${host}</div>` : nothing}
    </div>
  `;
}

// ── GeoLocation ─────────────────────────────────────────────────────────────

function isGeoShape(val: any): boolean {
  if (val == null || typeof val !== 'object') return false;
  const hasLatLon =
    (typeof val.latitude === 'number' && typeof val.longitude === 'number') ||
    (typeof val.lat === 'number' && typeof val.lng === 'number');
  return hasLatLon;
}

function renderGeo(val: any): TemplateResult {
  const lat = (val.latitude ?? val.lat) as number;
  const lng = (val.longitude ?? val.lng) as number;
  const osmUrl = `https://www.openstreetmap.org/?mlat=${lat}&mlon=${lng}#map=15/${lat}/${lng}`;
  return html`
    <div>
      <span class="font-mono text-sm">${lat.toFixed(6)}, ${lng.toFixed(6)}</span>
      <a href=${osmUrl} target="_blank" rel="noopener noreferrer"
        class="text-indigo-600 hover:text-indigo-800 underline decoration-indigo-300 text-xs ml-2"
        @click=${(e: Event) => e.stopPropagation()}
      >View on map</a>
    </div>
  `;
}

// ── Date Range ──────────────────────────────────────────────────────────────

function isDateRangeShape(val: any): boolean {
  if (val == null || typeof val !== 'object') return false;
  if (typeof val.start !== 'string' || typeof val.end !== 'string') return false;
  const s = new Date(val.start);
  const e = new Date(val.end);
  return !isNaN(s.getTime()) && !isNaN(e.getTime());
}

function renderDateRange(val: any): TemplateResult {
  const opts: Intl.DateTimeFormatOptions = { year: 'numeric', month: 'short', day: 'numeric' };
  const s = new Date(val.start).toLocaleDateString(undefined, opts);
  const e = new Date(val.end).toLocaleDateString(undefined, opts);
  return html`<span class="text-sm">${s} \u2014 ${e}</span>`;
}

// ── Dimensions ──────────────────────────────────────────────────────────────

function isDimensionsShape(val: any): boolean {
  return (
    val != null &&
    typeof val === 'object' &&
    typeof val.width === 'number' &&
    typeof val.height === 'number'
  );
}

function renderDimensions(val: any): TemplateResult {
  return html`<span class="font-mono text-sm">${val.width} \u00D7 ${val.height}</span>`;
}

// ── Registry ────────────────────────────────────────────────────────────────

/** Ordered list of built-in renderers. First match wins. */
export const builtinRenderers: BuiltinRenderer[] = [
  { name: 'url', detect: isURLShape, render: renderURL },
  { name: 'geo', detect: isGeoShape, render: renderGeo },
  { name: 'daterange', detect: isDateRangeShape, render: renderDateRange },
  { name: 'dimensions', detect: isDimensionsShape, render: renderDimensions },
];

/** Look up a built-in renderer by name (for forced x-display values). */
export function getBuiltinRenderer(name: string): BuiltinRenderer | undefined {
  return builtinRenderers.find(r => r.name === name);
}

/** Run shape detectors against a value. Returns the first matching renderer, or undefined. */
export function detectShape(val: any): BuiltinRenderer | undefined {
  if (val == null || typeof val !== 'object' || Array.isArray(val)) return undefined;
  return builtinRenderers.find(r => r.detect(val));
}
