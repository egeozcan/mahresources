/**
 * Privacy mode legend helpers (docs/plans/2026-11-08-template-privacy-mode.md, P9 and P10).
 *
 * The server sends text with Placeholders in it plus a legend of
 * { placeholder, value } entries. The legend is browser-only: it is shown on request
 * and cleared with the surface that holds it. Text is split by the exact legend
 * entries, longest first, never by a generic pattern, so a Placeholder-shaped string
 * the legend does not list stays plain text. Everything is returned as plain
 * strings for x-text; nothing here produces markup.
 */

export function cleanLegend(legend) {
  if (!Array.isArray(legend)) return [];
  return legend.filter((e) => e && typeof e.placeholder === 'string' && e.placeholder !== '' && typeof e.value === 'string');
}

const escapeRe = (s) => s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');

// One alternation of the exact entries, longest first, so a Placeholder that is a
// prefix of another never shadows it. Cached per legend array.
const matchers = new WeakMap();
function matcherFor(legend) {
  let m = matchers.get(legend);
  if (m) return m;
  const byPlaceholder = new Map();
  for (const e of legend) if (!byPlaceholder.has(e.placeholder)) byPlaceholder.set(e.placeholder, e.value);
  const keys = [...byPlaceholder.keys()].sort((a, b) => b.length - a.length);
  m = { byPlaceholder, re: keys.length ? new RegExp(keys.map(escapeRe).join('|'), 'g') : null };
  matchers.set(legend, m);
  return m;
}

/**
 * Split `text` into [{ text, placeholder, value }] pieces. A Placeholder piece has
 * `placeholder` set; its displayed `text` is the Placeholder, or with `reveal`
 * "Placeholder = value". Other pieces carry only `text`.
 */
export function legendSegments(text, legend, reveal = false) {
  const str = typeof text === 'string' ? text : String(text ?? '');
  if (!Array.isArray(legend) || legend.length === 0) return [{ text: str, placeholder: '', value: '' }];
  const { byPlaceholder, re } = matcherFor(legend);
  if (!re) return [{ text: str, placeholder: '', value: '' }];
  const out = [];
  let last = 0;
  re.lastIndex = 0;
  let match;
  while ((match = re.exec(str))) {
    if (match.index > last) out.push({ text: str.slice(last, match.index), placeholder: '', value: '' });
    const value = byPlaceholder.get(match[0]);
    out.push({ text: reveal ? `${match[0]} = ${value}` : match[0], placeholder: match[0], value });
    last = match.index + match[0].length;
  }
  if (last < str.length || out.length === 0) out.push({ text: str.slice(last), placeholder: '', value: '' });
  return out;
}

export const PLACEHOLDER_CLASS = 'rounded-sm bg-sky-100 text-sky-900 outline outline-1 outline-sky-700';

/**
 * The Draft fields the server withheld because they carried a Placeholder the run
 * issued (P11). A withheld field is empty and must never be applied. The server marks
 * it with `withheld: true` on its per-field entry; `data.withheld` as a list of names is
 * read too. Returns a Set of field names.
 */
export function withheldFields(data) {
  const names = new Set();
  if (!data || typeof data !== 'object') return names;
  if (Array.isArray(data.withheld)) for (const n of data.withheld) if (typeof n === 'string') names.add(n);
  for (const key of ['fields', 'slotFields', 'draft']) {
    const entries = data[key];
    if (!entries || typeof entries !== 'object') continue;
    if (Array.isArray(entries)) {
      for (const e of entries) if (e && e.withheld === true && typeof e.name === 'string') names.add(e.name);
    } else {
      for (const [name, e] of Object.entries(entries)) if (e && e.withheld === true) names.add(name);
    }
  }
  return names;
}
