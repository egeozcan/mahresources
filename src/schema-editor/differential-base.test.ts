// @vitest-environment happy-dom
//
// Differential test: the schema walkers and renderers of this branch against
// the same modules at the branch's base commit (207cd972), on randomly
// generated valid schemas and values. Any difference fails unless it falls
// under ALLOWED_DIFFERENCES, the documented list of intentional changes, and
// each allowance is checked as narrowly as the change itself: per field, with
// the expected replacement. Head is always run with a time bound and must not
// throw, so an exponential expansion or a crash fails even where a difference
// is allowed.
//
// The base copies live in src/schema-editor/__base__/ (nothing imports them
// outside this test). Regenerate them with
//   git show 207cd972:<path>   for schema-core.ts, display-renderers.ts,
//   form-mode-helpers.ts, modes/display-mode.ts and webcomponents/meta-shortcode.ts,
// rewriting relative imports to the copies and the two custom element names
// to base-schema-display-mode and base-meta-shortcode.
//
// Runs 300 cases with a fixed seed. For a wider local run:
//   DIFF_CASES=5000 DIFF_SEED=7 npx vitest run src/schema-editor/differential-base.test.ts
// DIFF_CHAINS=0 turns off the deep duplicate-branch chains; DIFF_REPORT=<file>
// writes the failures and per-reason allowance counts as JSON.

// Date-only values are formatted on their calendar day in this branch and at
// UTC midnight at base; the two agree in UTC (see calendar-dates).
process.env.TZ = 'UTC';

import { describe, expect, it } from 'vitest';
import * as head from './schema-core';
import * as base from './__base__/schema-core';
import { isLeafSchema as headIsLeaf } from './form-mode-helpers';
import { isLeafSchema as baseIsLeaf } from './__base__/form-mode-helpers';
import './modes/display-mode';
import './__base__/modes/display-mode';
import '../webcomponents/meta-shortcode';
import './__base__/webcomponents/meta-shortcode';

const CASES = Number(process.env.DIFF_CASES ?? 300);
const SEED = Number(process.env.DIFF_SEED ?? 1);
const CHAINS = process.env.DIFF_CHAINS !== '0';
const CALL_BUDGET_MS = 250;
const RENDER_BUDGET_MS = 1000;
// A chain case may run to the work budget before failing as too complex,
// which takes about half a second; it must still finish within 2s.
const CHAIN_BUDGET_MS = 2000;

// ─── Allowed differences ─────────────────────────────────────────────────────

const ALLOWED_DIFFERENCES = {
  'labeled-oneof-kept': 'resolveSchema keeps a labeled oneOf whole; base merged it into its last const and title (so a labeled alternative also lent "Why" to the parent). Normalised: when head\'s resolved schema is a labeled enum, it is collapsed with base\'s own resolveSchema before comparing. flattenSchema fields and display/[meta] rendering are compared for every top-level property except one that reaches a labeled oneOf (head still runs for it, time-bounded).',
  'alternative-titles': 'A non-labeled oneOf/anyOf alternative no longer lends its title to the property. Normalised: base runs on the schema with those alternative titles removed, and resolved schemas are compared with them removed on both sides (unresolved nested schemas pass them through unchanged).',
  'non-conforming-values': 'A property whose generated value was deliberately made non-conforming (noise) renders differently on purpose: an object under a boolean renders as an object, non-primitives are never coerced. Rendering of that property only is not compared; every other property and every walker is.',
  'arrays-render-structurally': 'An array value under a non-array resolved type renders as its members; base printed String(array). Allowed per token: where base shows the array joined with commas, head must show the same stored array\'s members joined without separators (adjacent pills).',
  'yes-no-only-for-booleans': 'Under a boolean type only a real boolean renders as Yes/No. Allowed per token: where base shows Yes or No, head must show a non-boolean primitive stored in that field.',
  'invalid-dates-raw': 'An impossible or invalid date renders as its text. Allowed per token: where base shows "Invalid Date", head must show a string stored in that field.',
  'meta-objects-as-json': '[meta] renders an object or array value as JSON. Allowed when base printed "[object Object]" and head printed exactly the JSON of that field\'s value.',
  'object-under-scalar-type': 'An object or array value whose schema resolves to a declared scalar type renders as the object. Allowed in [meta] when head printed exactly the JSON of that field\'s value.',
  'mixed-array-members': 'An array holding both objects and scalars shows the scalars as text. The display of that property only is not compared.',
  'union-scalar-shown': 'A property whose schema resolves to an object but whose stored value is a scalar or array is shown as that value; base flattened the object alternative and hid it. The display of that property only is not compared.',
  'base-exponential-chains': 'Deep duplicate-branch chains (aN = anyOf/oneOf [ref aN+1, ref aN+1], and diamonds) are exponential at base. For a case with one, base is not run; head runs every walker and renderer under the time bound, and may finish with SchemaTooComplexError (rendered as the too-complex fallback). In any other case that error is a failure.',
  'raw-none-in-meta': '[meta] honours x-display raw/none. Generator: never emitted.',
  'x-color-filtering': 'Unsafe x-color values are ignored. Generator: only hex colors are emitted.',
  'index-keyed-options': 'Form and search option values are index keys. Not compared: neither the form nor search is rendered here.',
  'malformed-schema-fallback': 'A schema that fails validation falls back to the raw value. Generator: only valid schemas are emitted.',
  'calendar-dates': 'Date-only values keep their calendar day west of UTC. Compared in UTC, where base agrees.',
} as const;
type AllowReason = keyof typeof ALLOWED_DIFFERENCES;

// ─── Seeded generator ────────────────────────────────────────────────────────

function mulberry32(seed: number) {
  return () => {
    seed |= 0; seed = (seed + 0x6D2B79F5) | 0;
    let t = Math.imul(seed ^ (seed >>> 15), 1 | seed);
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

interface Gen {
  rng: () => number;
  defs: string[];
  ifs: any[];
}

const pick = <T,>(g: Gen, items: readonly T[]): T => items[Math.floor(g.rng() * items.length)];
const chance = (g: Gen, p: number) => g.rng() < p;
const COLORS = ['#16a34a', '#78716c', '#2563eb'];
const FORMATS = ['date', 'date-time', 'email', 'uri'];

function genLeaf(g: Gen): any {
  if (chance(g, 0.06)) return { oneOf: [{ const: 'x', title: 'Ex', 'x-color': pick(g, COLORS) }, { const: 'y', title: 'Why' }] };
  switch (pick(g, ['string', 'number', 'integer', 'boolean', 'enum', 'const', 'nullable'] as const)) {
    case 'string': {
      const s: any = { type: 'string' };
      if (chance(g, 0.3)) s.format = pick(g, FORMATS);
      if (chance(g, 0.2)) s.minLength = 1;
      if (chance(g, 0.2)) s.maxLength = 20;
      if (chance(g, 0.15)) s.pattern = '^[a-z]*$';
      return s;
    }
    case 'number':
    case 'integer': {
      const s: any = { type: chance(g, 0.5) ? 'number' : 'integer' };
      if (chance(g, 0.3)) s.minimum = 0;
      if (chance(g, 0.3)) s.maximum = 100;
      if (chance(g, 0.2)) s.multipleOf = s.type === 'integer' ? 2 : 0.5;
      if (chance(g, 0.15)) { s.minimum = 0; s.exclusiveMinimum = true; } // draft-04
      else if (chance(g, 0.15)) s.exclusiveMaximum = 1000; // draft-06+
      return s;
    }
    case 'boolean': return { type: 'boolean' };
    case 'enum': return chance(g, 0.5) ? { type: 'string', enum: ['a', 'b', 'c'] } : { type: 'integer', enum: [1, 2, 3] };
    case 'const': return { const: pick(g, ['k', 7, true]) };
    case 'nullable': return { type: [pick(g, ['string', 'number'] as const), 'null'] };
  }
}

function genSchema(g: Gen, depth: number): any {
  const roll = g.rng();
  if (depth >= 3 || roll < 0.35) {
    if (g.defs.length && chance(g, 0.25)) return { $ref: `#/$defs/${pick(g, g.defs)}` };
    return genLeaf(g);
  }
  if (roll < 0.6) return genObject(g, depth + 1);
  if (roll < 0.7) return { type: 'array', items: genSchema(g, depth + 1) };
  // Composition branches are sometimes references, so siblings can share or
  // refer to the same definitions (allOf [ref A, ref B] with A referring to B).
  const branch = () => (g.defs.length && chance(g, 0.35) ? { $ref: `#/$defs/${pick(g, g.defs)}` } : genObject(g, depth + 1));
  if (roll < 0.8) return { allOf: [branch(), branch()] };
  const kw = pick(g, ['anyOf', 'oneOf'] as const);
  const alts: any[] = [branch(), genLeaf(g)];
  if (!alts[0].$ref && chance(g, 0.15)) alts[0].title = 'Variant';
  return { [kw]: alts };
}

function genObject(g: Gen, depth: number): any {
  const properties: Record<string, any> = {};
  const count = 1 + Math.floor(g.rng() * 3);
  for (let i = 0; i < count; i++) {
    const prop = genSchema(g, depth);
    if (chance(g, 0.3)) prop.title = `Field ${i}`;
    if (chance(g, 0.15)) prop.description = 'Help text';
    properties[`f${depth}_${i}`] = prop;
  }
  const s: any = { type: 'object', properties };
  if (chance(g, 0.3)) s.required = [Object.keys(properties)[0]];
  if (chance(g, 0.15)) {
    const key = Object.keys(properties)[0];
    s.if = { properties: { [key]: { const: 'a' } } };
    s.then = { properties: { thenOnly: { type: 'string' } } };
    s.else = { properties: { elseOnly: { type: 'number' } } };
    g.ifs.push(s.if);
  }
  return s;
}

/** A deep chain of duplicate branches (or a diamond), ending in a string. */
function genChain($defs: Record<string, any>, g: Gen): string {
  const depth = 22 + Math.floor(g.rng() * 8);
  const kind = pick(g, ['anyOf', 'oneOf', 'diamond'] as const);
  for (let i = 0; i < depth; i++) {
    if (kind === 'diamond') {
      $defs[`c${i}`] = { anyOf: [{ $ref: `#/$defs/cl${i}` }, { $ref: `#/$defs/cr${i}` }] };
      $defs[`cl${i}`] = { $ref: `#/$defs/c${i + 1}` };
      $defs[`cr${i}`] = { $ref: `#/$defs/c${i + 1}` };
    } else {
      $defs[`c${i}`] = { [kind]: [{ $ref: `#/$defs/c${i + 1}` }, { $ref: `#/$defs/c${i + 1}` }] };
    }
  }
  $defs[`c${depth}`] = { type: 'string' };
  return '#/$defs/c0';
}

function genRoot(g: Gen): { schema: any; chain: boolean } {
  // Definitions may only refer to later ones: acyclic, with shared and
  // diamond references (one definition referred to twice by another).
  const n = Math.floor(g.rng() * 7);
  const names = Array.from({ length: n }, (_, i) => `d${i}`);
  const $defs: Record<string, any> = {};
  for (let i = n - 1; i >= 0; i--) {
    g.defs = names.slice(i + 1);
    if (g.defs.length && chance(g, 0.3)) {
      const target = `#/$defs/${pick(g, g.defs)}`;
      $defs[names[i]] = { [pick(g, ['allOf', 'anyOf'] as const)]: [{ $ref: target }, { $ref: target }] };
    } else {
      $defs[names[i]] = genObject(g, 2);
      // Annotation data that looks like a reference to itself: never a cycle.
      if (chance(g, 0.2)) $defs[names[i]].examples = [{ $ref: `#/$defs/${names[i]}` }];
    }
  }
  g.defs = names;
  const root = genObject(g, 0);
  const chain = CHAINS && chance(g, 0.08);
  if (chain) root.properties.chain = { $ref: genChain($defs, g) };
  if (Object.keys($defs).length) root.$defs = $defs;
  return { schema: root, chain };
}

// A value for schema; with probability noise a leaf gets a value of the wrong type.
function genValue(g: Gen, schema: any, root: any, noise: number, depth = 0): any {
  if (depth > 6 || !schema || typeof schema !== 'object') return undefined;
  if (chance(g, noise)) return pick(g, [{ k: 1 }, 'text', 42, true, null, [1, 2], '2024-02-30']);
  if (schema.$ref) return genValue(g, base.resolveRef(schema.$ref, root), root, noise, depth + 1);
  if (schema.allOf) return genValue(g, schema.allOf.reduce((m: any, s: any) => base.mergeSchemas(m, s.$ref ? base.resolveRef(s.$ref, root) ?? {} : s), {}), root, noise, depth + 1);
  if (schema.oneOf || schema.anyOf) {
    const alts = schema.oneOf || schema.anyOf;
    const alt = pick(g, alts);
    return alt.const !== undefined ? alt.const : genValue(g, alt, root, noise, depth + 1);
  }
  if (schema.const !== undefined) return schema.const;
  if (schema.enum) return pick(g, schema.enum);
  const type = Array.isArray(schema.type) ? (chance(g, 0.3) ? 'null' : schema.type[0]) : schema.type;
  switch (type) {
    case 'object': {
      const out: Record<string, any> = {};
      for (const [k, p] of Object.entries(schema.properties ?? {})) {
        if (chance(g, 0.8)) out[k] = genValue(g, p, root, noise, depth + 1);
      }
      return out;
    }
    case 'array': return [genValue(g, schema.items, root, noise, depth + 1), genValue(g, schema.items, root, noise, depth + 1)];
    case 'string':
      switch (schema.format) {
        case 'date': return '2024-03-15';
        case 'date-time': return '2024-03-15T10:00:00Z';
        case 'email': return 'a@example.com';
        case 'uri': return 'https://example.com/p';
        default: return pick(g, ['abc', 'hello', '']);
      }
    case 'number': return pick(g, [0.5, 3, 42.5]);
    case 'integer': return pick(g, [2, 4, 10]);
    case 'boolean': return chance(g, 0.5);
    case 'null': return null;
    default: return 'v';
  }
}

// ─── Normalisation and allowance predicates ──────────────────────────────────

function normalise(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(normalise);
  if (value === null || typeof value !== 'object') return value;
  const out: Record<string, unknown> = {};
  for (const key of Object.keys(value).sort()) {
    const item = (value as Record<string, unknown>)[key];
    if (item === undefined) continue;
    out[key] = normalise(item);
  }
  return out;
}

/** alternative-titles: the schema with titles removed from non-labeled oneOf/anyOf alternatives. */
function withoutAlternativeTitles(schema: any): any {
  if (schema === null || typeof schema !== 'object') return schema;
  const copy = structuredClone(schema);
  head.visitSubschemas(copy, s => {
    for (const kw of ['oneOf', 'anyOf']) {
      if (!Array.isArray(s[kw]) || (kw === 'oneOf' && base.isLabeledEnum(s))) continue;
      for (const alt of s[kw]) if (alt && typeof alt === 'object') delete alt.title;
    }
  });
  return copy;
}

/** labeled-oneof-kept: whether schema, following references, reaches a labeled oneOf. */
function reachesLabeled(schema: any, root: any): boolean {
  let found = false;
  const seen = new Set<string>();
  const walk = (s: any) => head.visitSubschemas(s, sub => {
    if (base.isLabeledEnum(sub)) found = true;
    if (typeof sub.$ref === 'string' && !seen.has(sub.$ref)) {
      seen.add(sub.$ref);
      walk(base.resolveRef(sub.$ref, root));
    }
  });
  walk(schema);
  return found;
}

/** Arrays of primitives inside value, as [base token (comma-joined), head token (pills)] pairs. */
function arrayTokenPairs(value: any, into: Array<[string, string]> = []): Array<[string, string]> {
  if (Array.isArray(value)) {
    if (value.every(v => v === null || typeof v !== 'object')) into.push([value.join(','), value.map(String).join('')]);
    value.forEach(v => arrayTokenPairs(v, into));
  } else if (value !== null && typeof value === 'object') {
    Object.values(value).forEach(v => arrayTokenPairs(v, into));
  }
  return into;
}

/** Non-boolean primitive leaves of value, as rendered tokens. */
function leafTokens(value: any, into = new Set<string>()): Set<string> {
  if (Array.isArray(value)) value.forEach(v => leafTokens(v, into));
  else if (value !== null && typeof value === 'object') Object.values(value).forEach(v => leafTokens(v, into));
  else if (typeof value === 'string' || typeof value === 'number') String(value).split(' ').forEach(t => into.add(t));
  return into;
}

/**
 * Compare rendered texts token by token. Allowed only where base shows Yes/No
 * (yes-no-only-for-booleans) or "Invalid Date" (invalid-dates-raw) and head
 * shows a non-boolean primitive stored in that field. Returns the reasons, or
 * null when anything else differs.
 */
function allowedTextDifference(baseText: string, headText: string, stored: unknown): AllowReason[] | null {
  if (baseText === headText) return [];
  const INVALID = '\u0000';
  const b = baseText.replace(/Invalid Date/g, INVALID).split(' ');
  const h = headText.split(' ');
  if (b.length !== h.length) return null;
  const leaves = leafTokens(stored);
  const arrays = arrayTokenPairs(stored);
  const reasons = new Set<AllowReason>();
  for (let i = 0; i < b.length; i++) {
    if (b[i] === h[i]) continue;
    if (arrays.some(([joined, pills]) => b[i] === joined && h[i] === pills)) { reasons.add('arrays-render-structurally'); continue; }
    if (!leaves.has(h[i])) return null;
    if (b[i] === 'Yes' || b[i] === 'No') reasons.add('yes-no-only-for-booleans');
    else if (b[i] === INVALID) reasons.add('invalid-dates-raw');
    else return null;
  }
  return [...reasons];
}

/** mixed-array-members: some array in value holds both objects and non-objects. */
function hasMixedArray(value: any): boolean {
  if (Array.isArray(value)) {
    const objects = value.filter(v => v !== null && typeof v === 'object' && !Array.isArray(v)).length;
    if (objects > 0 && objects < value.length) return true;
    return value.some(hasMixedArray);
  }
  return value !== null && typeof value === 'object' && Object.values(value).some(hasMixedArray);
}

const SCALAR_TYPES = new Set(['string', 'number', 'integer', 'boolean', 'null']);

/** object-under-scalar-type: an object or array value whose schema resolves to a declared scalar type. */
function isObjectUnderScalar(prop: any, value: any, root: any): boolean {
  if (value === null || typeof value !== 'object') return false;
  const resolved = head.resolveSchema(prop, root) ?? prop;
  const type = Array.isArray(resolved?.type) ? resolved.type.find((t: string) => t !== 'null') : resolved?.type;
  return typeof type === 'string' && SCALAR_TYPES.has(type);
}

/** union-scalar-shown: prop (or a property under it) resolves to an object schema while its value is not an object. */
function hasUnionScalar(prop: any, value: any, root: any, depth = 0): boolean {
  const resolved = head.resolveSchema(prop, root) ?? prop;
  if (depth > 3 || !resolved?.properties || value === undefined || value === null) return false;
  if (typeof value !== 'object' || Array.isArray(value)) return true;
  return Object.entries(resolved.properties).some(([key, sub]) => hasUnionScalar(sub, value[key], root, depth + 1));
}

// ─── Running both implementations ────────────────────────────────────────────

function outcome(fn: () => unknown): { value?: unknown; error?: string } {
  try {
    return { value: normalise(fn()) };
  } catch (error) {
    return { error: String(error) };
  }
}

async function settle(el: Element) {
  for (let i = 0; i < 3; i++) await (el as any).updateComplete;
}

async function rendered(tag: string, setup: (el: any) => void): Promise<{ text: string; took: number }> {
  const started = performance.now();
  const el = document.createElement(tag) as any;
  setup(el);
  document.body.append(el);
  await settle(el);
  const text = (el.textContent ?? '').replace(/\s+/g, ' ').trim();
  el.remove();
  return { text, took: performance.now() - started };
}

interface Difference { check: string; base: unknown; head: unknown }

async function compareCase(seed: number) {
  const g: Gen = { rng: mulberry32(seed), defs: [], ifs: [] };
  const { schema, chain } = genRoot(g);
  const baseSchema = withoutAlternativeTitles(schema);
  // Noise goes into some properties' values; only those properties' rendering
  // is exempt (non-conforming-values).
  const noisy = new Set<string>();
  const value: Record<string, any> = {};
  for (const [key, prop] of Object.entries(schema.properties) as Array<[string, any]>) {
    const noise = chance(g, 0.2) ? 0.3 : 0;
    if (noise) noisy.add(key);
    if (chance(g, 0.85)) value[key] = genValue(g, prop, schema, noise);
  }

  const differences: Difference[] = [];
  const allowed: AllowReason[] = [];
  const props = Object.entries(schema.properties) as Array<[string, any]>;
  const baseProps = baseSchema.properties as Record<string, any>;
  if (chain) allowed.push('base-exponential-chains');

  /** Run head (time-bounded, must not throw) and, unless chain, base; compare. */
  const check = (name: string, runHead: () => unknown, runBase: () => unknown, normaliseHead: (v: unknown) => unknown = v => v) => {
    const started = performance.now();
    const h = outcome(() => normaliseHead(runHead()));
    const took = performance.now() - started;
    if (took > (chain ? CHAIN_BUDGET_MS : CALL_BUDGET_MS)) { differences.push({ check: `${name} time`, base: null, head: `${took.toFixed(0)}ms` }); return; }
    if (h.error && !(chain && h.error.startsWith('SchemaTooComplexError'))) { differences.push({ check: `${name} threw`, base: null, head: h }); return; }
    if (chain) return;
    const b = outcome(runBase);
    if (JSON.stringify(b) !== JSON.stringify(h)) differences.push({ check: name, base: b, head: h });
  };
  const collapseLabeled = (resolved: unknown) => withoutAlternativeTitles(resolved && head.isLabeledEnum(resolved as any) ? base.resolveSchema(resolved as any, baseSchema) : resolved);
  const baseResolved = (s: any) => () => withoutAlternativeTitles(base.resolveSchema(s, baseSchema));

  check('resolveSchema(root)', () => head.resolveSchema(schema, schema), baseResolved(baseSchema), collapseLabeled);
  for (const [key, prop] of props) {
    check(`resolveSchema(${key})`, () => head.resolveSchema(prop, schema), baseResolved(baseProps[key]), collapseLabeled);
    check(`isLeafSchema(${key})`, () => headIsLeaf(prop, schema), () => baseIsLeaf(baseProps[key], baseSchema));
  }
  check('getDefaultValue', () => head.getDefaultValue(schema, schema), () => base.getDefaultValue(baseSchema, baseSchema));
  check('getPreviewDefaultValue', () => head.getPreviewDefaultValue(schema, schema), () => base.getPreviewDefaultValue(baseSchema, baseSchema));
  check('evaluateCondition(schema)', () => head.evaluateCondition(schema, value, schema), () => base.evaluateCondition(baseSchema, value, baseSchema));
  g.ifs.forEach((cond, i) => check(`evaluateCondition(if ${i})`, () => head.evaluateCondition(cond, value, schema), () => base.evaluateCondition(cond, value, baseSchema)));
  for (const [key, prop] of props) {
    check(`scoreSchemaMatch(${key})`, () => head.scoreSchemaMatch(prop, value[key], schema), () => base.scoreSchemaMatch(baseProps[key], value[key], baseSchema));
  }

  // flattenSchema, field by field (matched by path), except the fields of a
  // top-level property that reaches a labeled oneOf.
  const labeledKeys = new Set(props.filter(([, prop]) => reachesLabeled(prop, schema)).map(([key]) => key));
  {
    const started = performance.now();
    const hf = outcome(() => head.flattenSchema(schema));
    const tooComplexInChain = chain && hf.error?.startsWith('SchemaTooComplexError');
    if (performance.now() - started > (chain ? CHAIN_BUDGET_MS : CALL_BUDGET_MS) || (hf.error && !tooComplexInChain)) differences.push({ check: 'flattenSchema time/threw', base: null, head: hf.error ?? 'slow' });
    else if (!chain) {
      const bf = outcome(() => base.flattenSchema(baseSchema));
      if (bf.error) differences.push({ check: 'flattenSchema', base: bf, head: hf });
      const byPath = (fields: any) => new Map(((fields ?? []) as any[]).map(f => [f.path, f]));
      const hFields = byPath(hf.value);
      const bFields = byPath(bf.value);
      for (const path of new Set([...hFields.keys(), ...bFields.keys()])) {
        if (JSON.stringify(hFields.get(path)) === JSON.stringify(bFields.get(path))) continue;
        if (labeledKeys.has(path.split('.')[0])) allowed.push('labeled-oneof-kept');
        else differences.push({ check: `flattenSchema field ${path}`, base: bFields.get(path), head: hFields.get(path) });
      }
    }
  }

  // Rendering, one top-level property at a time.
  for (const [key, prop] of props) {
    const stored = value[key];
    const exempt: AllowReason | undefined =
      noisy.has(key) ? 'non-conforming-values'
      : labeledKeys.has(key) ? 'labeled-oneof-kept'
      : undefined;
    const single = (root: any) => ({ ...root, properties: { [key]: root.properties[key] } });
    const headPanel = await rendered('schema-display-mode', el => { el.schema = single(schema); el.value = value; });
    if (headPanel.took > (chain ? CHAIN_BUDGET_MS : RENDER_BUDGET_MS)) differences.push({ check: `display ${key} time`, base: null, head: headPanel.took });
    // The predicates resolve with head, so they only run where base is compared.
    const panelExempt = chain ? undefined : exempt
      ?? (hasUnionScalar(prop, stored, schema) ? 'union-scalar-shown' : undefined)
      ?? (hasMixedArray(stored) ? 'mixed-array-members' : undefined);
    if (chain || panelExempt) { if (panelExempt) allowed.push(panelExempt); }
    else {
      const basePanel = await rendered('base-schema-display-mode', el => { el.schema = single(baseSchema); el.value = value; });
      const reasons = allowedTextDifference(basePanel.text, headPanel.text, stored);
      if (!reasons) differences.push({ check: `display ${key}`, base: basePanel.text, head: headPanel.text });
      else allowed.push(...reasons);
    }

    const metaFor = (tag: string, root: any) => rendered(tag, el => {
      el.setAttribute('data-path', key);
      el.setAttribute('data-schema', JSON.stringify(root.$defs ? { ...root.properties[key], $defs: root.$defs } : root.properties[key]));
      el.setAttribute('data-value', stored === undefined ? '' : JSON.stringify(stored));
    });
    const headMeta = await metaFor('meta-shortcode', schema);
    if (headMeta.took > (chain ? CHAIN_BUDGET_MS : RENDER_BUDGET_MS)) differences.push({ check: `[meta] ${key} time`, base: null, head: headMeta.took });
    if (chain || exempt) { if (exempt) allowed.push(exempt); continue; }
    const baseMeta = await metaFor('base-meta-shortcode', baseSchema);
    const json = stored === undefined ? undefined : JSON.stringify(stored);
    if (baseMeta.text === headMeta.text) continue;
    if (baseMeta.text.includes('[object Object]') && headMeta.text === json) allowed.push('meta-objects-as-json');
    else if (isObjectUnderScalar(prop, stored, schema) && headMeta.text === json) allowed.push('object-under-scalar-type');
    else {
      const reasons = allowedTextDifference(baseMeta.text, headMeta.text, stored);
      if (!reasons) differences.push({ check: `[meta] ${key}`, base: baseMeta.text, head: headMeta.text });
      else allowed.push(...reasons);
    }
  }
  return { differences, allowed, schema, value };
}

describe('differential: this branch against base 207cd972', () => {
  it(`${CASES} generated cases from seed ${SEED} match base outside the allowed differences`, async () => {
    const failures: string[] = [];
    const allowedCounts: Partial<Record<AllowReason, number>> = {};
    for (let i = 0; i < CASES; i++) {
      const caseSeed = SEED * 100_003 + i;
      const { differences, allowed, schema, value } = await compareCase(caseSeed);
      allowed.forEach(r => { allowedCounts[r] = (allowedCounts[r] ?? 0) + 1; });
      if (differences.length) {
        failures.push(`case seed ${caseSeed}\n  schema ${JSON.stringify(schema)}\n  value ${JSON.stringify(value)}\n` +
          differences.map(d => `  ${d.check}\n    base ${JSON.stringify(d.base)}\n    head ${JSON.stringify(d.head)}`).join('\n'));
        if (failures.length >= 5) break;
      }
    }
    if (process.env.DIFF_REPORT) {
      (await import('fs')).writeFileSync(process.env.DIFF_REPORT, JSON.stringify({ seed: SEED, cases: CASES, allowedCounts, failures }, null, 2));
    }
    expect(failures, failures.join('\n\n')).toEqual([]);
  }, 900_000);

  it('documents every allowed difference', () => {
    for (const [name, why] of Object.entries(ALLOWED_DIFFERENCES)) expect(why.length, name).toBeGreaterThan(20);
  });
});
