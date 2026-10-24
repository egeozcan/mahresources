import { html, nothing, type TemplateResult } from 'lit';

/**
 * A built-in display renderer. `select` validates a value and returns the
 * normalised fields the renderer draws, or null when the value does not have
 * the shape. Detection and rendering both go through it, so a value is only
 * ever rendered from the fields that made it match.
 * Detectors are checked in order; first match wins.
 */
export interface BuiltinRenderer<T = any> {
  name: string;
  select: (val: any) => T | null;
  detect: (val: any) => boolean;
  render: (fields: T) => TemplateResult;
}

function builtin<T>(name: string, select: (val: any) => T | null, render: (fields: T) => TemplateResult): BuiltinRenderer<T> {
  return { name, select, detect: val => select(val) !== null, render };
}

function asObject(val: any): Record<string, any> | null {
  return val != null && typeof val === 'object' && !Array.isArray(val) ? val : null;
}

// ── URL / Location ──────────────────────────────────────────────────────────

interface URLFields { href: string; host: string }

function selectURL(val: any): URLFields | null {
  const o = asObject(val);
  if (!o || typeof o.href !== 'string') return null;
  if (typeof o.host !== 'string' && typeof o.hostname !== 'string') return null;
  const host = (typeof o.host === 'string' && o.host) || (typeof o.hostname === 'string' && o.hostname) || '';
  return { href: o.href, host };
}

function renderURL({ href, host }: URLFields): TemplateResult {
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

interface GeoFields { lat: number; lng: number }

/** latitude/longitude when both are numbers, else lat/lng when both are. */
function selectGeo(val: any): GeoFields | null {
  const o = asObject(val);
  if (!o) return null;
  if (typeof o.latitude === 'number' && typeof o.longitude === 'number') {
    return { lat: o.latitude, lng: o.longitude };
  }
  if (typeof o.lat === 'number' && typeof o.lng === 'number') {
    return { lat: o.lat, lng: o.lng };
  }
  return null;
}

function renderGeo({ lat, lng }: GeoFields): TemplateResult {
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

interface DateRangeFields { start: Date; end: Date }

function selectDateRange(val: any): DateRangeFields | null {
  const o = asObject(val);
  if (!o || typeof o.start !== 'string' || typeof o.end !== 'string') return null;
  const start = parseDisplayDate(o.start);
  const end = parseDisplayDate(o.end);
  if (!start || !end) return null;
  return { start, end };
}

// YYYY-MM-DD, optionally followed by a time (T or space), optional seconds and
// fraction, and an optional zone (Z or ±HH:MM / ±HHMM).
const ISO_DATE = /^(\d{4})-(\d{2})-(\d{2})(?:[Tt ](\d{2}):(\d{2})(?::(\d{2})(?:\.(\d+))?)?(?:([Zz])|([+-])(\d{2}):?(\d{2}))?)?$/;

const DAYS_IN_MONTH = [31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31];

// Proleptic Gregorian, computed directly: Date.UTC maps years 0-99 to 1900-1999.
function daysInMonth(year: number, month: number): number {
  const leap = year % 4 === 0 && (year % 100 !== 0 || year % 400 === 0);
  return month === 2 && leap ? 29 : DAYS_IN_MONTH[month - 1];
}

/**
 * Parse a stored ISO date or date-time for display, or null when it is not a
 * real one. This is the one date parser every display uses.
 *
 * - Each component is range-checked before a Date is built, so an impossible
 *   value ("2024-02-30", "T25:00", "+24:00") is rejected rather than rolled
 *   over into another date.
 * - A date-only value is a calendar date: it is built in local time from its
 *   parts. new Date("2024-03-15") is UTC midnight, the previous day west of UTC.
 * - A date-time with a zone is that instant; one without a zone is local time,
 *   as ECMAScript reads it.
 * - Anything that is not ISO returns null, and the caller shows the text.
 */
export function parseDisplayDate(text: string): Date | null {
  const m = ISO_DATE.exec(text);
  if (!m) return null;
  const year = Number(m[1]);
  const month = Number(m[2]);
  const day = Number(m[3]);
  if (month < 1 || month > 12 || day < 1 || day > daysInMonth(year, month)) return null;

  const date = new Date(0);
  if (m[4] === undefined) {
    date.setFullYear(year, month - 1, day);
    date.setHours(0, 0, 0, 0);
    return date;
  }

  const hour = Number(m[4]);
  const minute = Number(m[5]);
  const second = m[6] === undefined ? 0 : Number(m[6]);
  // Any number of fraction digits; only the first three reach the Date.
  const ms = m[7] === undefined ? 0 : Number(m[7].slice(0, 3).padEnd(3, '0'));
  if (hour > 23 || minute > 59 || second > 59) return null;

  if (m[8] === undefined && m[9] === undefined) {
    date.setFullYear(year, month - 1, day);
    date.setHours(hour, minute, second, ms);
    return date;
  }
  let offsetMinutes = 0;
  if (m[9] !== undefined) {
    const offsetHours = Number(m[10]);
    const offsetMins = Number(m[11]);
    if (offsetHours > 23 || offsetMins > 59) return null;
    offsetMinutes = (m[9] === '-' ? -1 : 1) * (offsetHours * 60 + offsetMins);
  }
  date.setUTCFullYear(year, month - 1, day);
  date.setUTCHours(hour, minute, second, ms);
  return new Date(date.getTime() - offsetMinutes * 60_000);
}

function renderDateRange({ start, end }: DateRangeFields): TemplateResult {
  const opts: Intl.DateTimeFormatOptions = { year: 'numeric', month: 'short', day: 'numeric' };
  const s = start.toLocaleDateString(undefined, opts);
  const e = end.toLocaleDateString(undefined, opts);
  return html`<span class="text-sm">${s} \u2014 ${e}</span>`;
}

// ── Dimensions ──────────────────────────────────────────────────────────────

interface DimensionsFields { width: number; height: number }

function selectDimensions(val: any): DimensionsFields | null {
  const o = asObject(val);
  if (!o || typeof o.width !== 'number' || typeof o.height !== 'number') return null;
  return { width: o.width, height: o.height };
}

function renderDimensions({ width, height }: DimensionsFields): TemplateResult {
  return html`<span class="font-mono text-sm">${width} \u00D7 ${height}</span>`;
}

// ── Registry ────────────────────────────────────────────────────────────────

/** Ordered list of built-in renderers. First match wins. */
export const builtinRenderers: BuiltinRenderer[] = [
  builtin('url', selectURL, renderURL),
  builtin('geo', selectGeo, renderGeo),
  builtin('daterange', selectDateRange, renderDateRange),
  builtin('dimensions', selectDimensions, renderDimensions),
];

/** Look up a built-in renderer by name (for forced x-display values). */
export function getBuiltinRenderer(name: string): BuiltinRenderer | undefined {
  return builtinRenderers.find(r => r.name === name);
}

/** Run shape detectors against a value. Returns the first matching renderer, or undefined. */
export function detectShape(val: any): BuiltinRenderer | undefined {
  return builtinRenderers.find(r => r.detect(val));
}

/** Render val with the renderer when its shape matches, or null when it does not. */
export function renderBuiltin(renderer: BuiltinRenderer, val: any): TemplateResult | null {
  const fields = renderer.select(val);
  return fields === null ? null : renderer.render(fields);
}

/**
 * The x-display annotation as a string, or '' when it is absent or not a
 * string. A hand-written schema can hold any JSON there.
 */
export function xDisplayOf(schema: any): string {
  const xd = schema?.['x-display'];
  return typeof xd === 'string' ? xd : '';
}
