// @vitest-environment happy-dom
//
// Differential test: the schema walkers and renderers of this branch against
// the same modules at the branch's base commit (207cd972), on randomly
// generated valid schemas and values. Any difference fails unless it falls
// under ALLOWED_DIFFERENCES, the documented list of intentional changes. Each
// call is also time-bounded, so an exponential expansion fails as well.
//
// The base copies live in src/schema-editor/__base__/ (excluded from the bundle:
// nothing imports them outside this test). Regenerate them with
//   git show 207cd972:<path>   for schema-core.ts, display-renderers.ts,
//   form-mode-helpers.ts, modes/display-mode.ts and webcomponents/meta-shortcode.ts,
// rewriting relative imports to the copies and the two custom element names
// to base-schema-display-mode and base-meta-shortcode.
//
// Runs 300 cases with a fixed seed. For a wider local run:
//   DIFF_CASES=5000 DIFF_SEED=7 npx vitest run src/schema-editor/differential-base.test.ts

// Date-only values are formatted on their calendar day in this branch and at
// UTC midnight at base; the two agree in UTC, which is what is compared here
// (the calendar-day change itself is pinned in display-dates-tz.test.ts).
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
const CALL_BUDGET_MS = 250;

// ─── Allowed differences ─────────────────────────────────────────────────────
// Each entry names an intentional change of this branch and how the test
// keeps it out of the comparison. "skip" entries drop one comparison for a
// case; "generator" entries are kept out by never generating the input.

const ALLOWED_DIFFERENCES = {
  'labeled-oneof-kept': 'resolveSchema keeps a labeled oneOf whole (base merged it into its last const and title), so resolution, search flattening and rendering of a schema containing one differ. Skipped for those cases.',
  'alternative-titles': 'A oneOf/anyOf alternative no longer lends its title to the property. Skipped for resolution and rendering when an alternative carries a title.',
  'identical-branches-merged-once': 'Identical allOf/anyOf branches are merged once. Normalised: duplicate allOf/anyOf branches are dropped from both outputs before comparing.',
  'non-conforming-values': 'A value that does not match its schema renders differently on purpose: an object under a boolean renders as an object, Yes/No only for real booleans, non-primitives never coerced, impossible or invalid dates shown as their text. Rendering is skipped for cases with a non-conforming value.',
  'raw-none-in-meta': '[meta] honours x-display raw/none. Generator: never emitted.',
  'x-color-filtering': 'Unsafe x-color values are ignored. Generator: only hex colors are emitted.',
  'index-keyed-options': 'Form and search option values are index keys. Not compared: neither the form nor search is rendered here.',
  'malformed-schema-fallback': 'A schema that fails validation falls back to the raw value. Generator: only valid schemas are emitted.',
  'calendar-dates': 'Date-only values keep their calendar day west of UTC. Compared in UTC, where base agrees.',
  'meta-objects-as-json': '[meta] renders an object or array value as JSON; on some base paths it printed "[object Object]". Skipped when the base text contains "[object Object]".',
  'object-under-scalar-type': 'An object or array value whose schema resolves to a scalar type (a union of object and scalar alternatives merges to the scalar) renders as the object: JSON in [meta], where base printed the scalar rendering ("Yes"). Skipped for [meta] when the value is a non-null object or array and the resolved type is not object or array.',
  'mixed-array-members': 'An array holding both objects and scalars shows the scalars as text; base passed them to the object grid (a dash, or a string spelled out as indexed characters). The display comparison is skipped when some array mixes object and non-object members.',
  'yes-no-only-for-booleans': 'Under a boolean type, only a real boolean renders as Yes/No; a value that conforms to a merged const (allOf of boolean and const 7) renders as itself, where base printed Yes/No. Allowed per token: a position where base shows Yes or No and head shows something else.',
  'invalid-dates-raw': 'An impossible or invalid date renders as its text; base printed "Invalid Date". Allowed per token: a position where base shows Invalid Date and head shows something else.',
  'union-scalar-shown': 'A property whose schema resolves to an object (a union of object and scalar alternatives) but whose stored value is a scalar or array is shown as that value; base flattened the object alternative and hid it. The display comparison is skipped for those cases.',
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
  flags: Set<AllowReason>;
  ifs: any[];
}

const pick = <T,>(g: Gen, items: readonly T[]): T => items[Math.floor(g.rng() * items.length)];
const chance = (g: Gen, p: number) => g.rng() < p;
const COLORS = ['#16a34a', '#78716c', '#2563eb'];
const FORMATS = ['date', 'date-time', 'email', 'uri'];

function genLeaf(g: Gen): any {
  // Rare, so that most cases keep their resolution comparisons (see labeled-oneof-kept).
  if (chance(g, 0.02)) {
    g.flags.add('labeled-oneof-kept');
    return { oneOf: [{ const: 'x', title: 'Ex', 'x-color': pick(g, COLORS) }, { const: 'y', title: 'Why' }] };
  }
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
  if (!alts[0].$ref && chance(g, 0.03)) { alts[0].title = 'Variant'; g.flags.add('alternative-titles'); }
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

function genRoot(g: Gen): any {
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
    }
  }
  g.defs = names;
  const root = genObject(g, 0);
  if (n) root.$defs = $defs;
  return root;
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

// ─── Comparison ──────────────────────────────────────────────────────────────

/**
 * Compare rendered texts token by token. Returns the allowed reasons when the
 * only differing tokens are base Yes/No (yes-no-only-for-booleans) or base
 * "Invalid Date" (invalid-dates-raw); null when anything else differs.
 */
function allowedTextDifference(baseText: string, headText: string): AllowReason[] | null {
  const INVALID = '\u0000';
  const b = baseText.replace(/Invalid Date/g, INVALID).split(' ');
  const h = headText.split(' ');
  if (b.length !== h.length) return null;
  const reasons = new Set<AllowReason>();
  for (let i = 0; i < b.length; i++) {
    if (b[i] === h[i]) continue;
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

/** object-under-scalar-type: an object or array value whose schema resolves to a scalar type. */
function isObjectUnderScalar(prop: any, value: any, root: any): boolean {
  if (value === null || typeof value !== 'object') return false;
  const resolved = head.resolveSchema(prop, root) ?? prop;
  const type = Array.isArray(resolved?.type) ? resolved.type.find((t: string) => t !== 'null') : resolved?.type;
  return type !== 'object' && type !== 'array';
}

/** union-scalar-shown: some property resolves to an object schema while its stored value is not an object. */
function hasUnionScalar(schema: any, value: any, root: any, depth = 0): boolean {
  const resolved = head.resolveSchema(schema, root) ?? schema;
  if (depth > 3 || !resolved?.properties || value === null || typeof value !== 'object') return false;
  return Object.entries(resolved.properties).some(([key, prop]) => {
    const v = value[key];
    const rp = head.resolveSchema(prop as any, root) ?? prop;
    if (!rp?.properties || v === undefined || v === null) return false;
    if (typeof v !== 'object' || Array.isArray(v)) return true;
    return hasUnionScalar(prop, v, root, depth + 1);
  });
}

function normalise(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(normalise);
  if (value === null || typeof value !== 'object') return value;
  const out: Record<string, unknown> = {};
  for (const key of Object.keys(value).sort()) {
    let item = (value as Record<string, unknown>)[key];
    if (item === undefined) continue;
    if ((key === 'allOf' || key === 'anyOf') && Array.isArray(item)) {
      // identical-branches-merged-once
      const seen = new Set<string>();
      item = item.filter(branch => {
        const k = JSON.stringify(normalise(branch));
        if (seen.has(k)) return false;
        seen.add(k);
        return true;
      });
    }
    out[key] = normalise(item);
  }
  return out;
}

function timed<T>(label: string, fn: () => T): T {
  const started = performance.now();
  const result = fn();
  const took = performance.now() - started;
  if (took > CALL_BUDGET_MS) throw new Error(`${label} took ${took.toFixed(0)}ms (budget ${CALL_BUDGET_MS}ms)`);
  return result;
}

function outcome<T>(fn: () => T): { value?: unknown; error?: string } {
  try {
    return { value: normalise(fn()) };
  } catch (error) {
    return { error: String(error) };
  }
}

async function settle(el: Element) {
  for (let i = 0; i < 3; i++) await (el as any).updateComplete;
}

async function renderedText(tag: string, setup: (el: any) => void): Promise<string> {
  const el = document.createElement(tag) as any;
  setup(el);
  document.body.append(el);
  await settle(el);
  const text = (el.textContent ?? '').replace(/\s+/g, ' ').trim();
  el.remove();
  return text;
}

interface Difference { check: string; base: unknown; head: unknown }

async function compareCase(seed: number): Promise<{ differences: Difference[]; skipped: AllowReason[]; schema: any; value: any }> {
  const g: Gen = { rng: mulberry32(seed), defs: [], flags: new Set(), ifs: [] };
  const schema = genRoot(g);
  const noisy = chance(g, 0.25);
  const value = genValue(g, schema, schema, noisy ? 0.15 : 0);
  if (noisy) g.flags.add('non-conforming-values');
  const differences: Difference[] = [];
  const skipped: AllowReason[] = [];
  const resolutionSkip = (['labeled-oneof-kept', 'alternative-titles'] as AllowReason[]).find(r => g.flags.has(r));

  const check = (name: string, run: (impl: 'base' | 'head') => unknown) => {
    const b = outcome(() => run('base'));
    const h = outcome(() => timed(`head ${name}`, () => run('head')));
    if (h.error?.includes('budget')) { differences.push({ check: name, base: b, head: h }); return; }
    if (JSON.stringify(b) !== JSON.stringify(h)) differences.push({ check: name, base: b, head: h });
  };

  const props = Object.entries(schema.properties ?? {}) as Array<[string, any]>;
  if (resolutionSkip) {
    skipped.push(resolutionSkip);
  } else {
    check('resolveSchema(root)', impl => (impl === 'base' ? base : head).resolveSchema(schema, schema));
    for (const [key, prop] of props) {
      check(`resolveSchema(${key})`, impl => (impl === 'base' ? base : head).resolveSchema(prop, schema));
    }
    check('flattenSchema', impl => (impl === 'base' ? base : head).flattenSchema(schema));
  }
  check('getDefaultValue', impl => (impl === 'base' ? base : head).getDefaultValue(schema, schema));
  check('getPreviewDefaultValue', impl => (impl === 'base' ? base : head).getPreviewDefaultValue(schema, schema));
  check('evaluateCondition(schema)', impl => (impl === 'base' ? base : head).evaluateCondition(schema, value, schema));
  g.ifs.forEach((cond, i) => check(`evaluateCondition(if ${i})`, impl => (impl === 'base' ? base : head).evaluateCondition(cond, value, schema)));
  for (const [key, prop] of props) {
    check(`isLeafSchema(${key})`, impl => (impl === 'base' ? baseIsLeaf : headIsLeaf)(prop, schema));
  }

  if (resolutionSkip || g.flags.has('non-conforming-values')) {
    if (g.flags.has('non-conforming-values')) skipped.push('non-conforming-values');
  } else {
    const panel = async (tag: string) => {
      const started = performance.now();
      const text = await renderedText(tag, el => { el.schema = schema; el.value = value; });
      return { text, took: performance.now() - started };
    };
    if (hasUnionScalar(schema, value, schema)) {
      skipped.push('union-scalar-shown');
    } else if (hasMixedArray(value)) {
      skipped.push('mixed-array-members');
    } else {
      const b = await panel('base-schema-display-mode');
      const h = await panel('schema-display-mode');
      const allowed = allowedTextDifference(b.text, h.text);
      if (h.took > CALL_BUDGET_MS * 4 || !allowed) differences.push({ check: 'display panel', base: b, head: h });
      else skipped.push(...allowed);
    }
    for (const [key, prop] of props) {
      const propSchema = schema.$defs ? { ...prop, $defs: schema.$defs } : prop;
      const meta = async (tag: string) => {
        const started = performance.now();
        const text = await renderedText(tag, el => {
          el.setAttribute('data-path', key);
          el.setAttribute('data-schema', JSON.stringify(propSchema));
          el.setAttribute('data-value', value?.[key] === undefined ? '' : JSON.stringify(value[key]));
        });
        return { text, took: performance.now() - started };
      };
      const mb = await meta('base-meta-shortcode');
      const mh = await meta('meta-shortcode');
      if (mh.took > CALL_BUDGET_MS * 4) differences.push({ check: `[meta] ${key} time`, base: mb, head: mh });
      else if (mb.text.includes('[object Object]')) skipped.push('meta-objects-as-json');
      else if (isObjectUnderScalar(propSchema, value?.[key], propSchema)) skipped.push('object-under-scalar-type');
      else {
        const allowed = allowedTextDifference(mb.text, mh.text);
        if (!allowed) differences.push({ check: `[meta] ${key}`, base: mb, head: mh });
        else skipped.push(...allowed);
      }
    }
  }
  return { differences, skipped, schema, value };
}

describe('differential: this branch against base 207cd972', () => {
  it(`${CASES} generated cases from seed ${SEED} match base outside the allowed differences`, async () => {
    const failures: string[] = [];
    const skipCounts: Partial<Record<AllowReason, number>> = {};
    for (let i = 0; i < CASES; i++) {
      const caseSeed = SEED * 100_003 + i;
      const { differences, skipped, schema, value } = await compareCase(caseSeed);
      skipped.forEach(r => { skipCounts[r] = (skipCounts[r] ?? 0) + 1; });
      if (differences.length) {
        failures.push(`case seed ${caseSeed}\n  schema ${JSON.stringify(schema)}\n  value ${JSON.stringify(value)}\n` +
          differences.map(d => `  ${d.check}\n    base ${JSON.stringify(d.base)}\n    head ${JSON.stringify(d.head)}`).join('\n'));
        if (failures.length >= 5) break;
      }
    }
    if (process.env.DIFF_REPORT) {
      (await import('fs')).writeFileSync(process.env.DIFF_REPORT, JSON.stringify({ seed: SEED, cases: CASES, skipCounts, failures }, null, 2));
    }
    expect(failures, failures.join('\n\n')).toEqual([]);
  }, 600_000);

  it('documents every allowed difference', () => {
    for (const [name, why] of Object.entries(ALLOWED_DIFFERENCES)) expect(why.length, name).toBeGreaterThan(20);
  });
});
