// @vitest-environment happy-dom
// Adversarial inputs through every schema editor and renderer entry point.
// Four classes recur: (1) enum identity taken from display text, (2) sorting
// or comparing raw values, (3) raw values reaching .value, text or attributes,
// (4) dates that roll over instead of failing, plus (5) resolution dropping
// sibling keywords and (6) x-color strictness. The targeted tests pin each
// reported repro; the matrix at the end feeds the hostile values through every
// mountable entry point and asserts nothing throws or prints "[object Object]".
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import './modes/display-mode';
import './modes/form-mode';
import './__base__/modes/form-mode';
import './modes/search-mode';
import './schema-editor';
import './tree/detail-panel';
import './modes/edit-mode';
import '../webcomponents/meta-shortcode';
import { flattenSchema, intersectFields, resolveSchema, safeColor, getLabeledEnumEntries, withTypedKeywords, sameValue, mergeSchemas, getDefaultValue, getPreviewDefaultValue, evaluateCondition, scoreSchemaMatch, visitSubschemas, withSchemaBudget, schemaWorkDone, SchemaTooComplexError, SCHEMA_WORK_BUDGET, mergeAllOf, countSchemaWork, valueText, inputText, valueKey } from './schema-core';
import * as baseCore from './__base__/schema-core';
import { isLeafSchema, stripStaleKeys } from './form-mode-helpers';
import { guardSchemaAction, schemaActionError, clearSchemaActionError } from './schema-fallback';
import { schemaToTree, treeToSchema } from './schema-tree-model';
import { renderJsonTable } from '../tableMaker.js';

// A nested element (a tree node editor inside the detail panel) that throws
// while updating rejects a promise nobody awaits. Collect those per test, so
// the failure lands on the test that caused it.
const unhandled: unknown[] = [];
const onUnhandled = (reason: unknown) => { unhandled.push(reason); };
beforeEach(() => {
  unhandled.length = 0;
  process.on('unhandledRejection', onUnhandled);
});

afterEach(async () => {
  await new Promise(resolve => setTimeout(resolve, 0));
  process.off('unhandledRejection', onUnhandled);
  document.body.replaceChildren();
  vi.unstubAllGlobals();
  expect(unhandled.map(String)).toEqual([]);
});

const H = { toString: null };
const OBJ = { a: 1 };
const STR = '{"a":1}';

async function settle(root: Element) {
  for (let round = 0; round < 4; round++) {
    const pending: Promise<unknown>[] = [];
    const walk = (node: Element | ShadowRoot) => {
      for (const el of Array.from(node.querySelectorAll('*'))) {
        const u = (el as any).updateComplete;
        if (u) pending.push(u);
        if ((el as any).shadowRoot) walk((el as any).shadowRoot);
      }
    };
    if ((root as any).updateComplete) pending.push((root as any).updateComplete);
    walk(root);
    await Promise.all(pending);
  }
}

async function mountEl(tag: string, props: Record<string, any> = {}, attrs: Record<string, string> = {}) {
  const el = document.createElement(tag) as any;
  for (const [k, v] of Object.entries(attrs)) el.setAttribute(k, v);
  Object.assign(el, props);
  document.body.append(el);
  await settle(el);
  return el;
}

/** Every text node, attribute value and form-control value under el, shadow roots included. */
function snapshot(el: Element): string {
  const parts: string[] = [el.textContent ?? ''];
  const walk = (node: Element | ShadowRoot) => {
    for (const child of Array.from(node.querySelectorAll('*'))) {
      for (const a of Array.from(child.attributes)) parts.push(a.value);
      if ('value' in child && typeof (child as any).value === 'string') parts.push((child as any).value);
      const sr = (child as any).shadowRoot as ShadowRoot | null;
      if (sr) { parts.push(sr.textContent ?? ''); walk(sr); }
    }
  };
  walk(el);
  // An element that renders into its own shadow root keeps its content there.
  const own = (el as any).shadowRoot as ShadowRoot | null;
  if (own) { parts.push(own.textContent ?? ''); walk(own); }
  return parts.join('\n');
}

function expectClean(el: Element) {
  const text = snapshot(el);
  expect(text).not.toContain('[object Object]');
  expect(text).not.toContain('Invalid Date');
}

// ─── 1. Enum identity is the index, never the text ──────────────────────────

describe('enum identity', () => {
  async function form(prop: any, value: any) {
    return mountEl('schema-form-mode', { schema: { type: 'object', properties: { v: prop } }, value });
  }
  async function pick(el: HTMLElement, optionIndex: number) {
    const s = el.querySelector('select')!;
    let emitted: any;
    el.addEventListener('value-change', e => { emitted = (e as CustomEvent).detail.value; }, { once: true });
    s.value = s.options[optionIndex].value;
    s.dispatchEvent(new Event('change'));
    return emitted?.v;
  }

  it('form: a string that looks like an object stays a string', async () => {
    const plain = await form({ enum: [OBJ, STR] }, {});
    expect(await pick(plain, 1)).toEqual(OBJ);
    const plain2 = await form({ enum: [OBJ, STR] }, {});
    expect(await pick(plain2, 2)).toBe(STR);
    const labeled = await form({ oneOf: [{ const: OBJ, title: 'Obj' }, { const: STR, title: 'Str' }] }, {});
    expect(await pick(labeled, 2)).toBe(STR);
  });

  it('form: a stored object equal to a choice selects that choice', async () => {
    const el = await form({ enum: [OBJ, { b: 2 }] }, { v: { b: 2 } });
    const s = el.querySelector('select')!;
    expect(s.options[s.selectedIndex].textContent?.trim()).toBe('{"b":2}');
    expect(s.textContent).not.toContain('(current)');
  });

  async function search(prop: any) {
    return mountEl('schema-search-mode', {
      schema: JSON.stringify({ type: 'object', properties: { v: prop } }),
      metaQuery: '[]',
    });
  }
  function hidden(el: HTMLElement) {
    return [...el.querySelectorAll<HTMLInputElement>('input[type="hidden"]')].map(i => i.value);
  }

  it('search checkboxes: checking one of a colliding pair checks only it', async () => {
    const el = await search({ enum: [OBJ, STR] });
    const boxes = [...el.querySelectorAll<HTMLInputElement>('input[type="checkbox"]')];
    boxes[1].checked = true;
    boxes[1].dispatchEvent(new Event('change'));
    await settle(el);
    const after = [...el.querySelectorAll<HTMLInputElement>('input[type="checkbox"]')];
    expect(after.map(b => b.checked)).toEqual([false, true]);
    expect(hidden(el)).toEqual(['v:EQ:"{\\"a\\":1}"']);
  });

  it('search select: selecting one of a colliding pair selects only it', async () => {
    const el = await search({ enum: [OBJ, STR, 'c', 'd', 'e', 'f', 'g'] });
    const s = el.querySelector<HTMLSelectElement>('select[multiple]')!;
    s.options[1].selected = true;
    s.dispatchEvent(new Event('change'));
    await settle(el);
    expect([...el.querySelector<HTMLSelectElement>('select[multiple]')!.options].map(o => o.selected).slice(0, 2)).toEqual([false, true]);
    expect(hidden(el)).toEqual(['v:EQ:"{\\"a\\":1}"']);
  });

  it('search: enum [1, "1"] submits whichever was chosen', async () => {
    for (const [index, expected] of [[0, 'v:EQ:1'], [1, 'v:EQ:"1"']] as const) {
      const el = await search({ enum: [1, '1'] });
      const box = el.querySelectorAll<HTMLInputElement>('input[type="checkbox"]')[index];
      box.checked = true;
      box.dispatchEvent(new Event('change'));
      await settle(el);
      expect(hidden(el)).toEqual([expected]);
      document.body.replaceChildren();
    }
  });

  it('search: a stored value restores the matching choice, loosely typed like before', async () => {
    const el = await mountEl('schema-search-mode', {
      schema: JSON.stringify({ type: 'object', properties: { v: { enum: [1, 2] } } }),
      metaQuery: JSON.stringify([{ name: 'v', value: '2', operation: 'EQ' }]),
    });
    expect([...el.querySelectorAll<HTMLInputElement>('input[type="checkbox"]')].map(b => b.checked)).toEqual([false, true]);
    expect(hidden(el)).toEqual(['v:EQ:2']);
  });
});

// ─── 2. No comparison or sort on raw values ─────────────────────────────────

describe('multi-schema intersection', () => {
  const lists = (...props: any[]) => props.map(p => flattenSchema({ type: 'object', properties: { v: p } }));

  it('compares labeled consts without coercing them', () => {
    const fields = intersectFields(lists(
      { oneOf: [{ const: H, title: 'H' }, { const: 'a', title: 'A' }] },
      { oneOf: [{ const: 'a', title: 'A' }, { const: H, title: 'H' }] },
    ));
    expect(fields[0].enum).toHaveLength(2);
  });

  it('keeps colliding values distinct and equal sets equal', () => {
    // Same declared type, so only the enum comparison decides.
    expect(intersectFields(lists({ type: 'string', enum: [OBJ, 'x'] }, { type: 'string', enum: ['x', STR] }))[0].enum).toBeNull();
    expect(intersectFields(lists({ type: 'string', enum: [H, 'x'] }, { type: 'string', enum: ['x', H] }))[0].enum).toHaveLength(2);
    expect(intersectFields(lists({ type: 'string', enum: ['b', 'a'] }, { type: 'string', enum: ['a', 'b'] }))[0].enum).toEqual(['b', 'a']);
  });
});

// ─── 3. Raw values never reach .value, text or attributes ──────────────────

describe('tree node editors', () => {
  const schemas: Array<[string, any]> = [
    ['string', { type: 'string', default: H, const: H, minLength: H, maxLength: H, pattern: H, format: H, title: H, description: H }],
    ['number', { type: 'number', default: H, const: H, minimum: H, maximum: H, exclusiveMinimum: H, exclusiveMaximum: H, multipleOf: H }],
    ['integer', { type: 'integer', default: OBJ, const: OBJ, minimum: OBJ }],
    ['boolean', { type: 'boolean', default: H, const: H }],
    ['object', { type: 'object', minProperties: H, maxProperties: H, properties: {} }],
    ['array', { type: 'array', minItems: H, maxItems: H, items: { type: 'string' } }],
    ['enum', { type: 'string', enum: [H, OBJ, STR] }],
    ['labeled', { type: 'string', oneOf: [{ const: H, title: 'H', 'x-color': H }, { const: OBJ, title: '' }] }],
    ['ref', { $ref: H }],
  ];
  for (const [name, schema] of schemas) {
    it(`renders a hostile ${name} schema`, async () => {
      const el = await mountEl('schema-detail-panel', { node: schemaToTree(schema, 'v'), defsNames: ['x'] });
      expectClean(el);
    });
  }
});

// ─── 4. One strict date parser ──────────────────────────────────────────────

describe('strict dates', () => {
  async function panel(prop: any, value: any) {
    return mountEl('schema-display-mode', { schema: { type: 'object', properties: { loc: prop } }, value: { loc: value } });
  }

  it('shows impossible date-times as their text', async () => {
    for (const bad of ['2024-02-30T00:00:00Z', '2024-03-15T25:00:00Z', '2024-03-15T10:60:00Z', '2024-03-15T10:00:00+24:00', '2024-13-01T00:00:00Z']) {
      const el = await panel({ type: 'string', format: 'date-time' }, bad);
      expect(el.textContent).toContain(bad);
      const range = await panel({ type: 'object', 'x-display': 'daterange' }, { start: '2024-02-01T00:00:00Z', end: bad });
      expect(range.textContent).toContain(bad);
    }
  });

  it('still formats valid date-times', async () => {
    const el = await panel({ type: 'string', format: 'date-time' }, '2024-03-15T10:00:00Z');
    expect(el.textContent).not.toContain('2024-03-15T10:00:00Z');
    expect(el.textContent).toContain('2024');
    const range = await panel({ type: 'object', 'x-display': 'daterange' }, { start: '2024-03-15T10:00:00Z', end: '2024-04-01T00:00:00+02:00' });
    expect(range.textContent).toContain('—');
  });

  it('[meta] date-time shows an impossible date-time as its text', async () => {
    const el = await mountEl('meta-shortcode', {}, {
      'data-path': 'loc', 'data-date-time': 'true',
      'data-schema': JSON.stringify({ type: 'string', format: 'date-time' }),
      'data-value': JSON.stringify('2024-02-30T00:00:00Z'),
    });
    expect(el.textContent).toContain('2024-02-30T00:00:00Z');
  });
});

// ─── 5. Resolution keeps sibling keywords ───────────────────────────────────

describe('resolution keeps siblings', () => {
  const labeled = [{ const: 'a', title: 'Alpha' }, { const: 'b', title: 'Beta' }];

  it('keeps the labeled oneOf and every sibling annotation', () => {
    const r = resolveSchema({ title: 'T', description: 'D', oneOf: labeled, anyOf: [{ 'x-display': 'plugin:p:t' }] }, {})!;
    expect(r.oneOf).toEqual(labeled);
    expect(r['x-display']).toBe('plugin:p:t');
    expect(r.title).toBe('T');
    expect(r.description).toBe('D');
    const viaRef = resolveSchema({ $ref: '#/$defs/e', 'x-display': 'plugin:p:t' }, { $defs: { e: { oneOf: labeled } } } as any)!;
    expect(viaRef.oneOf).toEqual(labeled);
    expect(viaRef['x-display']).toBe('plugin:p:t');
  });

  it('dispatches a sibling plugin x-display in both consumers', async () => {
    const prop = { oneOf: labeled, anyOf: [{ 'x-display': 'plugin:p:t' }] };
    for (const mount of [
      () => mountEl('meta-shortcode', {}, { 'data-path': 'loc', 'data-schema': JSON.stringify(prop), 'data-value': '"a"' }),
      () => mountEl('schema-display-mode', { schema: { type: 'object', properties: { loc: prop } }, value: { loc: 'a' } }),
    ]) {
      const fetch = vi.fn().mockResolvedValue({ ok: true, text: async () => '<b>plugin-out</b>' });
      vi.stubGlobal('fetch', fetch);
      const el = await mount();
      await vi.waitFor(() => expect(el.textContent).toContain('plugin-out'));
      vi.unstubAllGlobals();
    }
  });
});

// ─── 6. x-color ─────────────────────────────────────────────────────────────

describe('x-color allowlist', () => {
  it('accepts word characters and rejects breakout characters', () => {
    expect(safeColor('var(--status_color)')).toBe('var(--status_color)');
    expect(getLabeledEnumEntries({ oneOf: [{ const: 'a', 'x-color': 'var(--status_color)' }] })[0].color).toBe('var(--status_color)');
    for (const bad of ['red;x', 'a{b}', 'a<b', 'a>b', 'a"b', "a'b", 'a\\b', 'a:b']) {
      expect(safeColor(bad)).toBeUndefined();
    }
  });
});

describe('keyword types across drafts', () => {
  // Every keyword withTypedKeywords checks, at a type each draft allows.
  const common = {
    type: ['number', 'null'], title: 'T', description: 'D', pattern: '^a', format: 'date',
    maximum: 9, multipleOf: 2, minLength: 1, maxLength: 5, minItems: 0, maxItems: 3,
    minProperties: 0, maxProperties: 4, 'x-display': 'raw',
  };

  it('draft-04: a boolean exclusive bound becomes the exclusive numeric bound', () => {
    const s = { $schema: 'http://json-schema.org/draft-04/schema#', ...common, minimum: 0, exclusiveMinimum: true, exclusiveMaximum: true };
    const { minimum, maximum, exclusiveMinimum, exclusiveMaximum, ...rest } = s;
    expect(withTypedKeywords(s)).toEqual({ ...rest, exclusiveMinimum: 0, exclusiveMaximum: 9 });
    expect(withTypedKeywords({ minimum: 0, exclusiveMinimum: false })).toEqual({ minimum: 0 });
    expect(withTypedKeywords({ exclusiveMinimum: true })).toEqual({});
  });

  it('draft-07 and 2020-12: numeric exclusive bounds and every other keyword survive', () => {
    for (const $schema of ['http://json-schema.org/draft-07/schema#', 'https://json-schema.org/draft/2020-12/schema']) {
      const s = { $schema, ...common, minimum: -1, exclusiveMinimum: 0, exclusiveMaximum: 10 };
      expect(withTypedKeywords(s)).toEqual(s);
    }
  });

  it('form: a draft-04 exclusive minimum still rejects the bound', async () => {
    const el = await mountEl('schema-form-mode', {
      schema: { type: 'object', properties: { v: { type: 'number', minimum: 0, exclusiveMinimum: true } } }, value: {},
    });
    expect(el.textContent).toContain('>0');
    const input = el.querySelector('input[type="number"]') as HTMLInputElement;
    input.value = '0';
    input.dispatchEvent(new Event('blur'));
    await settle(el);
    expect(el.textContent).toContain('Must be greater than 0');
  });
});

describe('deep equality ignores key order', () => {
  it('canonicalises object keys, keeps array order and primitive types', () => {
    expect(sameValue({ b: 2, a: { d: 1, c: [1, 2] } }, { a: { c: [1, 2], d: 1 }, b: 2 })).toBe(true);
    expect(sameValue([1, 2], [2, 1])).toBe(false);
    expect(sameValue('1', 1)).toBe(false);
    expect(sameValue({ a: '1' }, { a: 1 })).toBe(false);
  });

  it('form: a stored object with other key order selects its choice', async () => {
    const el = await mountEl('schema-form-mode', {
      schema: { type: 'object', properties: { v: { enum: [{ b: 2, a: 1 }, 'x'] } } }, value: { v: { a: 1, b: 2 } },
    });
    expect(el.querySelector('select')!.textContent).not.toContain('(current)');
  });

  it('multi-category: equivalent enums with other key order keep their enum', () => {
    const lists = [{ b: 2, a: 1 }, { a: 1, b: 2 }].map(o => flattenSchema({ type: 'object', properties: { v: { type: 'string', enum: [o] } } }));
    expect(intersectFields(lists)[0].enum).toHaveLength(1);
  });
});

describe('merging never lets an invalid annotation erase a valid one', () => {
  const root = { $defs: { d: { type: 'object', 'x-display': 'dimensions', title: 'T', description: 'D' } } } as any;

  it('$ref siblings', () => {
    const r = resolveSchema({ $ref: '#/$defs/d', 'x-display': 7, title: {}, description: [] }, root)!;
    expect(r['x-display']).toBe('dimensions');
    expect(r.title).toBe('T');
    expect(r.description).toBe('D');
  });

  it('allOf branches and x-color', () => {
    const r = resolveSchema({ allOf: [{ 'x-display': 'dimensions', title: 'T' }, { 'x-display': 7, title: 5 }] }, {})!;
    expect(r['x-display']).toBe('dimensions');
    expect(r.title).toBe('T');
    expect(mergeSchemas({ 'x-color': '#abc' }, { 'x-color': { toString: null } })['x-color']).toBe('#abc');
  });

  it('[meta] still renders the inherited renderer', async () => {
    const el = await mountEl('meta-shortcode', {}, {
      // Geo would win auto-detection, so only the inherited forced renderer gives 3 × 4.
      'data-path': 'v', 'data-value': JSON.stringify({ latitude: 1, longitude: 2, width: 3, height: 4 }),
      'data-schema': JSON.stringify({ $ref: '#/$defs/d', 'x-display': 7, $defs: { d: { type: 'object', 'x-display': 'dimensions' } } }),
    });
    expect(el.textContent).toContain('3 × 4');
  });
});

describe('search: stored values are re-matched on every rebuild', () => {
  function hidden(el: HTMLElement) {
    return [...el.querySelectorAll<HTMLInputElement>('input[type="hidden"]')].map(i => i.value);
  }
  const schemaOf = (prop: any) => JSON.stringify({ type: 'object', properties: { v: prop } });

  it('keeps the exact encoding of numbers, booleans and strings across a rebuild', async () => {
    for (const [prop, stored, expected] of [
      [{ enum: [1, 2] }, 2, 'v:EQ:2'],
      [{ enum: [true, false] }, true, 'v:EQ:true'],
      [{ enum: ['a', 'b'] }, 'a', 'v:EQ:"a"'],
    ] as const) {
      const el = await mountEl('schema-search-mode', { schema: schemaOf(prop), metaQuery: JSON.stringify([{ name: 'v', value: stored, operation: 'EQ' }]) });
      expect(hidden(el)).toEqual([expected]);
      el.schema = schemaOf({ ...prop, title: 'Rebuilt' });
      await settle(el);
      expect(hidden(el)).toEqual([expected]);
      document.body.replaceChildren();
    }
  });

  it('a number typed into a plain field matches the choice when the field becomes an enum', async () => {
    const el = await mountEl('schema-search-mode', { schema: schemaOf({ type: 'number' }), metaQuery: '[]' });
    const input = el.querySelector('input[type="number"]') as HTMLInputElement;
    input.value = '2';
    input.dispatchEvent(new Event('input'));
    await settle(el);
    expect(hidden(el)).toEqual(['v:EQ:2']);
    el.schema = schemaOf({ type: 'number', enum: [1, 2] });
    await settle(el);
    expect(hidden(el)).toEqual(['v:EQ:2']);
    expect([...el.querySelectorAll<HTMLInputElement>('input[type="checkbox"]')].map(b => b.checked)).toEqual([false, true]);
  });
});

describe('inherited property names as schema and data keys', () => {
  // JSON.parse makes "__proto__" an own key; a plain object literal would not.
  const KEYS = ['__proto__', 'constructor', 'toString'];
  const props = JSON.parse('{"__proto__":{"type":"string","title":"Proto"},"constructor":{"type":"string","title":"Ctor"},"toString":{"type":"string","title":"ToStr"}}');
  const schema = { type: 'object', properties: props };
  const value = JSON.parse('{"__proto__":"p-val","constructor":"c-val","toString":"t-val"}');

  it('merging a branch with those keys neither throws nor loses them', () => {
    expect(() => mergeSchemas({}, JSON.parse('{"__proto__":"annotation"}'))).not.toThrow();
    const r = resolveSchema({ allOf: [{ type: 'object', properties: props }, JSON.parse('{"properties":{"__proto__":{"description":"D"}}}')] }, {})!;
    expect(Object.keys(r.properties)).toEqual(KEYS);
    expect(r.properties.__proto__).toEqual({ type: 'string', title: 'Proto', description: 'D' });
    expect(flattenSchema(schema).map(f => f.path)).toEqual(KEYS);
  });

  it('every entry point shows the fields and their values, and nothing inherited', async () => {
    const panel = await mountEl('schema-display-mode', { schema, value });
    const form = await mountEl('schema-form-mode', { schema, value });
    const search = await mountEl('schema-search-mode', { schema: JSON.stringify(schema), metaQuery: '[]' });
    for (const el of [panel, form, search]) {
      expectClean(el);
      expect(snapshot(el)).not.toContain('[native code]');
    }
    for (const v of ['p-val', 'c-val', 't-val']) expect(snapshot(panel)).toContain(v);
    expect([...form.querySelectorAll('input:not([type="hidden"])')].map(i => i.value)).toEqual(['p-val', 'c-val', 't-val']);
    expect(search.querySelectorAll('input[type="text"]')).toHaveLength(3);
    // With no stored data, an inherited name is absent, not Object's own member.
    const empty = await mountEl('schema-display-mode', { schema, value: {} });
    expect(snapshot(empty)).not.toContain('[native code]');
    const emptyForm = await mountEl('schema-form-mode', { schema, value: {} });
    expect([...emptyForm.querySelectorAll('input:not([type="hidden"])')].map(i => i.value)).toEqual(['', '', '']);
    const meta = await mountEl('meta-shortcode', {}, { 'data-path': 'constructor', 'data-schema': '{"type":"string"}', 'data-value': '' , 'data-hide-empty': 'true' });
    expect(snapshot(meta)).not.toContain('[native code]');
  });

  it('a property only one branch declares passes through unchanged, inherited names included', () => {
    // Filtering protects an existing valid value; a branch that adds a new
    // property (even one named like an Object member) hands it on as written.
    const added = JSON.parse('{"properties":{"constructor":{"type":"string","x-display":7},"toString":{"title":{}}}}');
    const r = mergeSchemas({ properties: {} }, added);
    expect(r.properties.constructor).toEqual({ type: 'string', 'x-display': 7 });
    expect(r.properties.toString).toEqual({ title: {} });
  });

  it('a $ref through an inherited name resolves to nothing', () => {
    expect(resolveSchema({ $ref: '#/__proto__' }, {})).toBeNull();
    expect(resolveSchema({ $ref: '#/constructor' }, {})).toBeNull();
  });
});

describe('form submission enforces the same bounds blur shows', () => {
  async function submitWith(prop: any, v: number) {
    const form = document.createElement('form');
    document.body.append(form);
    const el = document.createElement('schema-form-mode') as any;
    el.schema = { type: 'object', properties: { v: prop } };
    el.value = { v };
    form.append(el);
    await settle(el);
    const ev = new Event('submit', { cancelable: true });
    form.dispatchEvent(ev);
    await settle(el);
    return { blocked: ev.defaultPrevented, text: el.textContent as string };
  }

  it('draft-04 boolean exclusive bounds block submit', async () => {
    const low = await submitWith({ type: 'number', minimum: 1, exclusiveMinimum: true }, 0.5);
    expect(low.blocked).toBe(true);
    expect(low.text).toContain('Must be greater than 1');
    expect((await submitWith({ type: 'number', minimum: 1, exclusiveMinimum: true }, 1)).blocked).toBe(true);
    expect((await submitWith({ type: 'number', maximum: 5, exclusiveMaximum: true }, 5)).blocked).toBe(true);
    expect((await submitWith({ type: 'number', minimum: 1, exclusiveMinimum: true }, 2)).blocked).toBe(false);
  });

  it('draft-07 numeric exclusive bounds block submit', async () => {
    expect((await submitWith({ type: 'number', exclusiveMinimum: 1 }, 1)).blocked).toBe(true);
    expect((await submitWith({ type: 'number', exclusiveMaximum: 5 }, 5)).blocked).toBe(true);
    expect((await submitWith({ type: 'number', multipleOf: 2 }, 3)).blocked).toBe(true);
    expect((await submitWith({ type: 'number', exclusiveMinimum: 1, exclusiveMaximum: 5 }, 3)).blocked).toBe(false);
  });
});

describe('nested property merges filter invalid values too', () => {
  it('an allOf branch cannot erase an inherited property annotation', () => {
    const r = resolveSchema({ allOf: [
      { type: 'object', properties: { v: { type: 'object', 'x-display': 'dimensions', title: 'V' } } },
      { properties: { v: { 'x-display': 7, title: {}, description: 'kept' } } },
    ] }, {})!;
    expect(r.properties.v).toEqual({ type: 'object', 'x-display': 'dimensions', title: 'V', description: 'kept' });
  });
});

describe('date parser edge cases', () => {
  async function panel(format: string, value: string) {
    return mountEl('schema-display-mode', { schema: { type: 'object', properties: { v: { type: 'string', format } } }, value: { v: value } });
  }

  it('accepts any number of fraction digits', async () => {
    for (const v of ['2024-03-15T10:00:00.1234567890Z', '2024-03-15T10:00:00.5Z', '2024-03-15T10:00:00.123456789012Z']) {
      expect((await panel('date-time', v)).textContent).not.toContain(v);
    }
  });

  it('checks leap years on the proleptic calendar, low years included', async () => {
    for (const v of ['0000-02-29', '0400-02-29', '2000-02-29', '2024-02-29']) {
      expect((await panel('date', v)).textContent).not.toContain(v);
    }
    for (const v of ['0100-02-29', '1900-02-29', '2023-02-29', '0001-02-29']) {
      expect((await panel('date', v)).textContent).toContain(v);
    }
  });
});

// ─── Class A: reference cycles (valid JSON Schema) ───────────────────────────

describe('reference cycles', () => {
  const cycles: Array<[string, any]> = [
    ['self', { $defs: { r: { $ref: '#/$defs/r' } } }],
    ['two-node', { $defs: { a: { $ref: '#/$defs/b' }, b: { $ref: '#/$defs/a' } } }],
    ['allOf', { $defs: { r: { title: 'R', allOf: [{ $ref: '#/$defs/r' }] } } }],
  ];
  const tree = { $defs: { node: { type: 'object', properties: { name: { type: 'string' }, child: { $ref: '#/$defs/node' } } } } };

  for (const [name, defs] of cycles) {
    it(`a ${name} cycle resolves without overflowing the stack`, () => {
      const first = Object.keys(defs.$defs)[0];
      const root = { ...defs, type: 'object', properties: { v: { $ref: `#/$defs/${first}` } } };
      expect(() => resolveSchema({ $ref: `#/$defs/${first}` }, root)).not.toThrow();
      expect(() => flattenSchema(root)).not.toThrow();
      expect(() => getDefaultValue(root, root)).not.toThrow();
      expect(() => getPreviewDefaultValue(root, root)).not.toThrow();
      expect(() => evaluateCondition({ $ref: `#/$defs/${first}` }, {}, root)).not.toThrow();
    });
  }

  it('a branching cycle finishes quickly instead of growing exponentially', () => {
    const root = { $defs: { r: { anyOf: [{ $ref: '#/$defs/r' }, { $ref: '#/$defs/r' }, { $ref: '#/$defs/r' }], oneOf: [{ $ref: '#/$defs/r' }, { $ref: '#/$defs/r' }] } } };
    const started = Date.now();
    resolveSchema({ $ref: '#/$defs/r' }, root);
    evaluateCondition({ $ref: '#/$defs/r' }, { a: 1 }, root);
    getDefaultValue({ $ref: '#/$defs/r' }, root);
    expect(Date.now() - started).toBeLessThan(2000);
  });

  it('every entry point renders a cycle and a recursive tree schema', async () => {
    for (const root of [
      ...cycles.map(([, defs]) => ({ ...defs, type: 'object', properties: { v: { $ref: `#/$defs/${Object.keys(defs.$defs)[0]}` } } })),
      { ...tree, type: 'object', properties: { v: { $ref: '#/$defs/node' } } },
      { ...tree, $ref: '#/$defs/node' },
    ]) {
      const value = { v: { name: 'n', child: { name: 'c' } } };
      for (const el of [
        await mountEl('schema-display-mode', { schema: root, value }),
        await mountEl('schema-form-mode', { schema: root, value }),
        await mountEl('schema-search-mode', { schema: JSON.stringify(root), metaQuery: '[]' }),
        await mountEl('schema-edit-mode', { schema: root }),
        await mountEl('meta-shortcode', {}, { 'data-path': 'v', 'data-schema': JSON.stringify({ ...root.properties?.v, $defs: root.$defs }), 'data-value': JSON.stringify(value.v) }),
      ]) {
        expectClean(el);
        // A cycle is valid JSON Schema: it renders, it does not fall back.
        expect(snapshot(el), `${el.tagName} ${JSON.stringify(root)}`).not.toContain('Schema could not be used');
      }
      document.body.replaceChildren();
    }
  });
});

// ─── Class A: submission regressions ─────────────────────────────────────────

describe('submission checks', () => {
  async function mountInForm(prop: any, v: unknown) {
    const form = document.createElement('form');
    document.body.append(form);
    const el = document.createElement('schema-form-mode') as any;
    el.schema = { type: 'object', properties: { v: prop } };
    el.value = { v };
    form.append(el);
    await settle(el);
    const submit = async () => {
      const ev = new Event('submit', { cancelable: true });
      form.dispatchEvent(ev);
      await settle(el);
      return ev.defaultPrevented;
    };
    return { el, submit };
  }

  it('multipleOf accepts a value whose quotient overflows, as base did', async () => {
    const { submit } = await mountInForm({ type: 'number', multipleOf: 0.5 }, 1e308);
    expect(await submit()).toBe(false);
  });

  it('multipleOf tolerates floating-point remainders', async () => {
    for (const [m, v, blocked] of [[0.1, 0.3, false], [0.01, 1.23, false], [0.1, 0.35, true], [2, 3, true], [2, 4, false]] as const) {
      const { submit } = await mountInForm({ type: 'number', multipleOf: m }, v);
      expect(await submit(), `${v} multipleOf ${m}`).toBe(blocked);
      document.body.replaceChildren();
    }
  });

  it('an error from a replaced field does not block the new one', async () => {
    const { el, submit } = await mountInForm({ type: 'number', minimum: 10 }, 5);
    expect(await submit()).toBe(true);
    el.schema = { type: 'object', properties: { v: { type: 'boolean' } } };
    el.value = { v: true };
    await settle(el);
    expect(await submit()).toBe(false);
  });
});

// ─── Class B: malformed schemas the server would reject ──────────────────────

describe('malformed schemas fall back at each entry point', () => {
  const NOTE = 'Schema could not be used';
  const shapes: Array<[string, any, any]> = [
    ['null property schema', { type: 'object', properties: { v: null, w: { type: 'string' } } }, { v: 1, w: 'x' }],
    ['allOf: [null]', { type: 'object', allOf: [null], properties: { v: { type: 'string' } } }, { v: 'x' }],
    ['nested allOf: [null]', { type: 'object', properties: { v: { allOf: [null] } } }, { v: 'x' }],
    ['required: 5', { type: 'object', required: 5, properties: { v: { type: 'string' } } }, { v: 'x' }],
    ['hostile if minimum', { type: 'object', properties: { v: { type: 'number' } }, if: { properties: { v: { minimum: H } } }, then: { title: 'T' } }, { v: 3 }],
    ['oneOf: [null]', { type: 'object', oneOf: [null], properties: { v: { type: 'string' } } }, { v: 'x' }],
  ];

  // Where the malformed part reaches an entry point's processing, that entry
  // point falls back; where normalisation (a null property schema is skipped)
  // or the entry point's own handling makes it irrelevant, it renders.
  const F = 'fallback', R = 'rendered';
  const EXPECTED: Record<string, Record<string, string>> = {
    'null property schema': { display: F, form: F, search: R, tree: R, meta: R },
    'allOf: [null]': { display: F, form: F, search: F, tree: F, meta: F },
    'nested allOf: [null]': { display: F, form: F, search: F, tree: F, meta: R },
    'required: 5': { display: R, form: F, search: R, tree: F, meta: R },
    'hostile if minimum': { display: F, form: F, search: R, tree: R, meta: R },
    'oneOf: [null]': { display: F, form: F, search: F, tree: F, meta: F },
  };

  for (const [name, schema, value] of shapes) {
    it(`${name}: every entry point renders or falls back`, async () => {
      const log = vi.spyOn(console, 'error').mockImplementation(() => {});
      const results: Record<string, string> = {};
      const entries: Array<[string, () => Promise<HTMLElement>]> = [
        ['display', () => mountEl('schema-display-mode', { schema, value })],
        ['form', () => mountEl('schema-form-mode', { schema, value })],
        ['search', () => mountEl('schema-search-mode', { schema: JSON.stringify(schema), metaQuery: '[]' })],
        ['tree', () => mountEl('schema-edit-mode', { schema })],
        ['meta', () => mountEl('meta-shortcode', {}, { 'data-path': 'v', 'data-schema': JSON.stringify(schema), 'data-value': JSON.stringify(value) })],
      ];
      for (const [entry, mount] of entries) {
        const el = await mount();
        expectClean(el);
        results[entry] = snapshot(el).includes(NOTE) ? 'fallback' : 'rendered';
        document.body.replaceChildren();
      }
      // A fallback is always logged; a normal render never is.
      const fellBack = Object.values(results).filter(r => r === 'fallback').length;
      expect(log.mock.calls.length >= fellBack).toBe(true);
      log.mockRestore();
      expect(results).toEqual(EXPECTED[name]);
    });
  }
});

// ─── Class A: cycle detection is by path, not by depth ───────────────────────

describe('branching reference cycles', () => {
  const branching: Array<[string, any]> = [
    ['two-branch self allOf', { r: { allOf: [{ $ref: '#/$defs/r' }, { $ref: '#/$defs/r' }] } }],
    ['mutual a<->b through allOf', { r: { allOf: [{ $ref: '#/$defs/b' }] }, b: { allOf: [{ $ref: '#/$defs/r' }, { $ref: '#/$defs/r' }] } }],
    ['self through oneOf and anyOf', { r: { oneOf: [{ $ref: '#/$defs/r' }, { $ref: '#/$defs/r' }, { type: 'string' }], anyOf: [{ $ref: '#/$defs/r' }, { $ref: '#/$defs/r' }] } }],
  ];

  for (const [name, defs] of branching) {
    it(`${name}: every walker and entry point finishes within 2s`, async () => {
      const root = { $defs: defs, type: 'object', properties: { v: { $ref: '#/$defs/r' } } } as any;
      const ref = { $ref: '#/$defs/r' };
      const started = Date.now();
      resolveSchema(ref, root);
      flattenSchema(root);
      getDefaultValue(root, root);
      getPreviewDefaultValue(root, root);
      evaluateCondition(ref, {}, root);
      isLeafSchema(ref, root);
      scoreSchemaMatch(ref, {}, root);
      stripStaleKeys({ v: {} }, root, root);
      for (const el of [
        await mountEl('schema-display-mode', { schema: root, value: { v: {} } }),
        await mountEl('schema-form-mode', { schema: root, value: { v: {} } }),
        await mountEl('schema-search-mode', { schema: JSON.stringify(root), metaQuery: '[]' }),
        await mountEl('schema-edit-mode', { schema: root }),
        await mountEl('meta-shortcode', {}, { 'data-path': 'v', 'data-schema': JSON.stringify({ ...ref, $defs: defs }), 'data-value': '{}' }),
      ]) expect(snapshot(el)).not.toContain('Schema could not be used');
      expect(Date.now() - started).toBeLessThan(2000);
    });
  }
});

describe('round 10 repros', () => {
  it('an anyOf chain of doubled references resolves quickly in [meta]', async () => {
    const $defs: Record<string, any> = {};
    for (let i = 0; i < 17; i++) $defs[`a${i}`] = { anyOf: [{ $ref: `#/$defs/a${i + 1}` }, { $ref: `#/$defs/a${i + 1}` }] };
    $defs.a17 = { type: 'object' };
    const started = Date.now();
    const el = await mountEl('meta-shortcode', {}, {
      'data-path': 'v', 'data-schema': JSON.stringify({ $defs, $ref: '#/$defs/a0' }), 'data-value': JSON.stringify({ width: 3, height: 4 }),
    });
    // Finishes quickly either way: rendered, or the visible too-complex fallback.
    expect(Date.now() - started).toBeLessThan(1000);
    expect(el.textContent!.includes('3 \u00D7 4') || el.textContent!.includes('Schema too complex to display')).toBe(true);
  });

  it('a reference reached through a sibling allOf branch is not a cycle', async () => {
    const root = {
      $defs: {
        A: { type: 'object', properties: { x: { $ref: '#/$defs/B' } } },
        B: { type: 'object', properties: { y: { type: 'string' } } },
      },
      allOf: [{ $ref: '#/$defs/A' }, { $ref: '#/$defs/B' }],
    };
    expect(getDefaultValue(root, root)).toEqual({ x: { y: '' }, y: '' });
    const el = await mountEl('schema-form-mode', { schema: root, value: {} });
    expect(el.textContent).not.toContain('Unresolvable reference');
  });

  it('a stored null is data: a nullable recursive schema follows it', async () => {
    const root = { $defs: { N: { type: ['object', 'null'], properties: { child: { $ref: '#/$defs/N' } } } }, $ref: '#/$defs/N' };
    const el = await mountEl('schema-form-mode', { schema: root, value: { child: null } });
    expect(el.textContent).not.toContain('Unresolvable reference');
  });
});

describe('duplicate-branch chains stay linear in every walker', () => {
  // aN = anyOf/oneOf [ref aN+1, ref aN+1] down to a string, and a diamond of
  // distinct references to the same next level. A walker that iterates the
  // branches independently doubles its work at every level.
  function chain(kw: 'anyOf' | 'oneOf', depth: number) {
    const $defs: Record<string, any> = {};
    for (let i = 0; i < depth; i++) $defs[`a${i}`] = { [kw]: [{ $ref: `#/$defs/a${i + 1}` }, { $ref: `#/$defs/a${i + 1}` }] };
    $defs[`a${depth}`] = { type: 'string' };
    return { $defs, type: 'object', properties: { v: { $ref: '#/$defs/a0' } } } as any;
  }
  function diamond(depth: number) {
    const $defs: Record<string, any> = {};
    for (let i = 0; i < depth; i++) {
      $defs[`a${i}`] = { anyOf: [{ $ref: `#/$defs/b${i}` }, { $ref: `#/$defs/c${i}` }] };
      $defs[`b${i}`] = { $ref: `#/$defs/a${i + 1}` };
      $defs[`c${i}`] = { $ref: `#/$defs/a${i + 1}` };
    }
    $defs[`a${depth}`] = { type: 'string' };
    return { $defs, type: 'object', properties: { v: { $ref: '#/$defs/a0' } } } as any;
  }
  const roots: Array<[string, any]> = [['anyOf chain', chain('anyOf', 30)], ['oneOf chain', chain('oneOf', 30)], ['diamond', diamond(22)]];
  const within2s = (label: string, run: () => unknown) => {
    const started = Date.now();
    try {
      run();
    } catch (error) {
      // Too complex is an acceptable, visible outcome; anything else is not.
      if (!(error instanceof SchemaTooComplexError)) throw error;
    }
    expect(Date.now() - started, label).toBeLessThan(2000);
  };

  for (const [name, root] of roots) {
    const ref = { $ref: '#/$defs/a0' };
    it(`${name}: scoring, conditions, defaults, isLeafSchema and flatten`, () => {
      within2s('scoreSchemaMatch match', () => scoreSchemaMatch(ref, 'x', root));
      within2s('scoreSchemaMatch miss', () => scoreSchemaMatch(ref, 5, root));
      within2s('evaluateCondition miss', () => evaluateCondition(ref, 5, root));
      within2s('evaluateCondition match', () => evaluateCondition(ref, 'x', root));
      within2s('getDefaultValue', () => getDefaultValue(root, root));
      within2s('getPreviewDefaultValue', () => getPreviewDefaultValue(root, root));
      within2s('isLeafSchema', () => isLeafSchema(ref, root));
      within2s('flattenSchema', () => flattenSchema(root));
    });

    it(`${name}: the display panel and [meta] render within 2s`, async () => {
      // Each render finishes within 2s: rendered, or the too-complex fallback.
      const timed = async (label: string, mount: () => Promise<unknown>) => {
        const started = Date.now();
        await mount();
        expect(Date.now() - started, label).toBeLessThan(2000);
      };
      for (const value of ['x', 5]) {
        await timed('display', () => mountEl('schema-display-mode', { schema: { ...root, if: { properties: { v: ref } }, then: {} }, value: { v: value } }));
        await timed('[meta]', () => mountEl('meta-shortcode', {}, { 'data-path': 'v', 'data-schema': JSON.stringify({ ...ref, $defs: root.$defs }), 'data-value': JSON.stringify(value) }));
        await timed('form', () => mountEl('schema-form-mode', { schema: { type: 'object', properties: { v: { anyOf: [ref, { type: 'number' }] } }, $defs: root.$defs }, value: { v: value } }));
        document.body.replaceChildren();
      }
    });
  }

  it('evaluateCondition still counts duplicate oneOf matches', () => {
    expect(evaluateCondition({ oneOf: [{ type: 'string' }, { type: 'string' }] }, 'x', {})).toBe(false);
    expect(evaluateCondition({ anyOf: [{ type: 'string' }, { type: 'string' }] }, 'x', {})).toBe(true);
  });
});

describe('annotation data never creates a cycle', () => {
  it('renaming a definition rewrites references, not $ref keys inside data', async () => {
    const schema = {
      type: 'object',
      properties: { x: { $ref: '#/$defs/A' } },
      $defs: { A: { type: 'string', examples: [{ $ref: '#/$defs/A' }], default: { $ref: '#/$defs/A' }, allOf: [{ $ref: '#/$defs/A' }] } },
    };
    const el = await mountEl('schema-edit-mode', { schema });
    const find = (node: any): any => node.isDef && node.name === 'A' ? node : [...(node.children ?? []), ...(node.variants ?? [])].map(find).find(Boolean);
    el._selectedId = find(el._root).id;
    el._handleNodeChange(new CustomEvent('node-change', { detail: { field: 'name', value: 'B' } }));
    const out = treeToSchema(el._root);
    expect(out.properties.x.$ref).toBe('#/$defs/B');
    expect(out.$defs.B.allOf[0].$ref).toBe('#/$defs/B');
    expect(out.$defs.B.examples[0].$ref).toBe('#/$defs/A');
    expect(out.$defs.B.default.$ref).toBe('#/$defs/A');
  });

  it('a $ref inside examples or an unknown keyword is data, not a reference', async () => {
    const root = {
      $defs: {
        A: { type: 'object', properties: { x: { $ref: '#/$defs/B' } } },
        B: { type: 'object', properties: { y: { type: 'string' } }, examples: [{ $ref: '#/$defs/B' }], 'x-meta': { $ref: '#/$defs/B' } },
      },
      allOf: [{ $ref: '#/$defs/A' }, { $ref: '#/$defs/B' }],
    };
    expect(getDefaultValue(root, root)).toEqual({ x: { y: '' }, y: '' });
    const el = await mountEl('schema-form-mode', { schema: root, value: {} });
    expect(el.textContent).not.toContain('Unresolvable reference');
  });
});

describe('round 12: one work budget, base merge semantics', () => {
  const TOO_COMPLEX = 'Schema too complex to display';

  it('a later allOf override wins, as at base', async () => {
    const $defs = { A: { title: 'Alpha', default: 'A' }, B: { title: 'Beta', default: 'B' } };
    const A = { $ref: '#/$defs/A' }, B = { $ref: '#/$defs/B' };
    const v = { allOf: [{ allOf: [A, B, A, B] }, { allOf: [A] }] };
    const root = { $defs, type: 'object', properties: { v } } as any;
    expect(resolveSchema(v, root)!.title).toBe('Alpha');
    expect(getDefaultValue(v, root)).toBe('A');
    const el = await mountEl('schema-display-mode', { schema: root, value: { v: 'z' } });
    expect(el.textContent).toContain('Alpha');
  });

  it('a diamond of anyOf references in the form finishes within 2s', async () => {
    const $defs: Record<string, any> = { a0: { type: 'string' }, b0: { type: 'string' } };
    for (let i = 1; i <= 20; i++) {
      $defs[`a${i}`] = { anyOf: [{ $ref: `#/$defs/a${i - 1}` }, { $ref: `#/$defs/b${i - 1}` }] };
      $defs[`b${i}`] = { anyOf: [{ $ref: `#/$defs/a${i - 1}` }, { $ref: `#/$defs/b${i - 1}` }] };
    }
    const started = Date.now();
    const el = await mountEl('schema-form-mode', { schema: { $defs, type: 'object', properties: { v: { $ref: '#/$defs/a20' } } }, value: { v: 'x' } });
    expect(Date.now() - started).toBeLessThan(2000);
    const text = snapshot(el);
    expect(text.includes(TOO_COMPLEX) || el.querySelector('select, input:not([type="hidden"])') !== null).toBe(true);
  });

  it('an annotated allOf chain as the root if evaluates once per subtree and picks the branch', async () => {
    const $defs: Record<string, any> = { r0: { properties: { v: { const: 'a' } } } };
    for (let i = 1; i <= 21; i++) $defs[`r${i}`] = { title: `R${i}`, allOf: [{ $ref: `#/$defs/r${i - 1}` }, { $ref: `#/$defs/r${i - 1}` }] };
    const schema = {
      $defs, type: 'object', properties: { v: { type: 'string' } }, if: { $ref: '#/$defs/r21' },
      then: { properties: { thenOnly: { type: 'string', title: 'Then field' } } },
      else: { properties: { elseOnly: { type: 'string', title: 'Else field' } } },
    };
    const value = { v: 'a', thenOnly: 'T1', elseOnly: 'E1' };
    const started = Date.now();
    const panel = await mountEl('schema-display-mode', { schema, value });
    const form = await mountEl('schema-form-mode', { schema, value });
    expect(Date.now() - started).toBeLessThan(2000);
    for (const el of [panel, form]) expect(snapshot(el)).not.toContain(TOO_COMPLEX);
    expect(panel.textContent).toContain('Then field');
    expect(panel.textContent).not.toContain('Else field');
  });

  it('normal large schemas stay well under the budget', () => {
    const properties: Record<string, any> = {};
    for (let i = 0; i < 10002; i++) properties[`p${i}`] = { type: 'string', default: 'd' };
    const wide = { type: 'object', properties };
    let gate: any = { properties: { gate: { const: true } } };
    for (let i = 0; i < 34; i++) gate = { allOf: [gate] };
    withSchemaBudget(() => {
      getDefaultValue(wide, wide);
      getPreviewDefaultValue(wide, wide);
      flattenSchema(wide);
      resolveSchema(wide, wide);
      evaluateCondition(gate, { gate: false }, {});
    });
    expect(schemaWorkDone()).toBeLessThan(SCHEMA_WORK_BUDGET / 2);
  });

  it('one budget spans a whole render, not each walker call', async () => {
    const $defs: Record<string, any> = { a0: { type: 'string' }, b0: { type: 'string' } };
    for (let i = 1; i <= 12; i++) {
      $defs[`a${i}`] = { anyOf: [{ $ref: `#/$defs/a${i - 1}` }, { $ref: `#/$defs/b${i - 1}` }] };
      $defs[`b${i}`] = { anyOf: [{ $ref: `#/$defs/a${i - 1}` }, { $ref: `#/$defs/b${i - 1}` }] };
    }
    const render = async (tag: string, count: number) => {
      const properties: Record<string, any> = {};
      const value: Record<string, string> = {};
      for (let i = 0; i < count; i++) { properties[`p${i}`] = { $ref: '#/$defs/a12' }; value[`p${i}`] = 'x'; }
      const el = await mountEl(tag, { schema: { $defs, type: 'object', properties }, value });
      const text = snapshot(el);
      document.body.replaceChildren();
      return text;
    };
    for (const tag of ['schema-form-mode', 'schema-display-mode']) {
      // One such field renders well under the budget; sixty of them pass it
      // together, though each walker call stays as cheap as for one field.
      expect(await render(tag, 1), tag).not.toContain('Schema too complex');
      expect(await render(tag, 60), tag).toContain('Schema too complex to display');
    }
  });

  it('an optional recursive object offers Initialize, as at base', async () => {
    const root = { $defs: { N: { type: 'object', default: {}, properties: { child: { $ref: '#/$defs/N' } } } }, $ref: '#/$defs/N' };
    const el = await mountEl('schema-form-mode', { schema: root, value: {} });
    expect(el.textContent).not.toContain('Unresolvable reference');
    expect(el.textContent).toContain('Initialize');
  });
});

describe('round 13: linear reference analysis, no state after a fallback, scoring by value', () => {
  const TOO_COMPLEX = 'Schema too complex to display';
  const CONDITION_BRANCHES = {
    then: { properties: { thenOnly: { type: 'string', title: 'Then field' } } },
    else: { properties: { elseOnly: { type: 'string', title: 'Else field' } } },
  };

  const chain = (length: number, last: any) => {
    const $defs: Record<string, any> = {};
    for (let i = 0; i < length; i++) $defs[`a${i}`] = { $ref: `#/$defs/a${i + 1}` };
    $defs[`a${length}`] = last;
    return $defs;
  };

  it('an 8,000-reference chain as the root if renders or falls back within 1s', async () => {
    const $defs = chain(8000, { properties: { v: { const: 'a' } } });
    const schema = { $defs, type: 'object', properties: { v: { type: 'string' } }, if: { $ref: '#/$defs/a0' }, ...CONDITION_BRANCHES };
    const started = Date.now();
    const panel = await mountEl('schema-display-mode', { schema, value: { v: 'a', thenOnly: 'T', elseOnly: 'E' } });
    expect(Date.now() - started).toBeLessThan(1000);
    const text = snapshot(panel);
    expect(text.includes('Then field') || text.includes('showing the raw value')).toBe(true);
  });

  it('an 8,000-reference ring is recursive throughout and still renders within 1s', async () => {
    const $defs: Record<string, any> = {};
    for (let i = 0; i < 8000; i++) $defs[`a${i}`] = { properties: { next: { $ref: `#/$defs/a${(i + 1) % 8000}` } } };
    const schema = { $defs, type: 'object', properties: { v: { $ref: '#/$defs/a0' } } };
    const started = Date.now();
    const panel = await mountEl('schema-display-mode', { schema, value: { v: { next: { next: 'x' } } } });
    expect(Date.now() - started).toBeLessThan(1000);
    expect(snapshot(panel)).toContain('x');
  });

  it('visiting a 50,000-level schema does not overflow the stack, and keeps keyword order', () => {
    let deep: any = { $ref: '#/$defs/end' };
    for (let i = 0; i < 50000; i++) deep = { not: deep };
    let count = 0;
    expect(() => visitSubschemas(deep, () => { count++; })).not.toThrow();
    expect(count).toBe(50001);
    const order: string[] = [];
    visitSubschemas({ title: 'root', not: { title: 'not' }, allOf: [{ title: 'all0' }, { title: 'all1' }], properties: { p: { title: 'p', items: { title: 'p.items' } } } },
      s => order.push(s.title));
    expect(order).toEqual(['root', 'not', 'all0', 'all1', 'p', 'p.items']);
  });

  it('the reference analysis charges the work budget for every reference and edge', () => {
    // The condition is the chain's last link, so evaluating it is cheap and
    // the difference is the analysis of the root's whole reference graph:
    // each extra link is two subschemas visited, one reference and one edge.
    const work = (length: number) => {
      const root = { $defs: chain(length, { properties: { v: { const: 'a' } } }) };
      withSchemaBudget(() => evaluateCondition({ $ref: `#/$defs/a${length}` }, { v: 'a' }, root));
      return schemaWorkDone();
    };
    expect(work(5010) - work(10)).toBeGreaterThanOrEqual(3 * 5000);
  });

  it('recursion is decided per strongly connected component', () => {
    // b and c form a cycle, d refers to itself, and a (reached from x and
    // from its own sibling) is on no cycle.
    const root: any = {
      $defs: {
        a: { type: 'object', properties: { q: { type: 'string', default: 'A' } } },
        x: { type: 'object', allOf: [{ $ref: '#/$defs/a' }, { properties: { p: { $ref: '#/$defs/a' } } }] },
        b: { type: 'object', properties: { y: { $ref: '#/$defs/c' } } },
        c: { type: 'object', properties: { z: { $ref: '#/$defs/b' } } },
        d: { type: 'object', properties: { self: { $ref: '#/$defs/d' } } },
        w: { type: 'object', properties: { b: { $ref: '#/$defs/b' } } },
      },
    };
    // a is on the path when its sibling p refers to it again, and resolves
    // there, because it is not recursive.
    expect(getDefaultValue({ $ref: '#/$defs/x' }, root)).toEqual({ q: 'A', p: { q: 'A' } });
    // A two-reference cycle and a self-reference stop at their first repeat.
    expect(getDefaultValue({ $ref: '#/$defs/b' }, root)).toEqual({ y: {} });
    expect(getDefaultValue({ $ref: '#/$defs/w' }, root)).toEqual({ b: { y: {} } });
    expect(getDefaultValue({ $ref: '#/$defs/d' }, root)).toEqual({});
    // In a three-reference cycle every member is recursive, including the
    // one the analysis started from.
    const ring: any = { $defs: {
      a: { type: 'object', properties: { x: { $ref: '#/$defs/b' } } },
      b: { type: 'object', properties: { y: { $ref: '#/$defs/c' } } },
      c: { type: 'object', properties: { z: { $ref: '#/$defs/a' } } },
    } };
    expect(getDefaultValue({ $ref: '#/$defs/a' }, ring)).toEqual({ x: { y: {} } });
    expect(getDefaultValue({ $ref: '#/$defs/b' }, ring)).toEqual({ y: { z: {} } });
    expect(getDefaultValue({ $ref: '#/$defs/c' }, ring)).toEqual({ z: { x: {} } });
  });

  it('a budget fallback leaves no walk state behind for the next render', async () => {
    let doubling: any = { title: 'leaf', properties: { v: { const: 'a' } } };
    for (let i = 0; i < 20; i++) doubling = { title: `L${i}`, allOf: [doubling, doubling] };
    const heavy = { type: 'object', properties: { v: { type: 'string' } }, if: { allOf: [doubling] }, ...CONDITION_BRANCHES };
    const fallen = await mountEl('schema-display-mode', { schema: heavy, value: { v: 'a' } });
    expect(snapshot(fallen)).toContain(TOO_COMPLEX);
    document.body.replaceChildren();

    const schema = { $defs: { c: { properties: { v: { const: 'a' } } } }, type: 'object', properties: { v: { type: 'string' } }, if: { $ref: '#/$defs/c' }, ...CONDITION_BRANCHES };
    const value = { v: 'a', thenOnly: 'T', elseOnly: 'E' };
    const el: any = await mountEl('schema-display-mode', { schema, value });
    expect(el.textContent).toContain('Then field');
    value.v = 'b';
    el.requestUpdate();
    await settle(el);
    expect(el.textContent).toContain('Else field');
    expect(el.textContent).not.toContain('Then field');
  });

  it('a budget error thrown by the depth guard itself unwinds the depth', () => {
    // Every branch is one guarded walker call, so the budget runs out in the
    // guard's own count. A top-level walker call outside any render has its
    // own budget, and no withSchemaBudget around it to restore the walk state.
    const wide = { allOf: new Array(SCHEMA_WORK_BUDGET + 1).fill({}) };
    expect(() => evaluateCondition(wide, {}, {})).toThrow(SchemaTooComplexError);
    // A leaked depth would keep the previous walk's memo, so mutated data
    // would keep its old verdict.
    const data = { v: 'a' };
    const root: any = { $defs: { c: { properties: { v: { const: 'a' } } } } };
    expect(evaluateCondition({ $ref: '#/$defs/c' }, data, root)).toBe(true);
    data.v = 'b';
    expect(evaluateCondition({ $ref: '#/$defs/c' }, data, root)).toBe(false);
  });

  it('scoring compares const and enum by value, so equal object branches score alike', () => {
    const second = { k: 1 };
    // The second branch's own const object: dropping that branch as a
    // repeat of the first must not change its score.
    expect(scoreSchemaMatch({ anyOf: [{ const: { k: 1 } }, { const: second }] } as any, second, {})).toBe(100);
    const data = { k: 1 };
    expect(scoreSchemaMatch({ anyOf: [{ const: { k: 1 } }, { const: { k: 1 } }] } as any, data, {})).toBe(100);
    expect(scoreSchemaMatch({ const: { k: 1 } } as any, data, {})).toBe(100);
    expect(scoreSchemaMatch({ properties: { p: { const: [1] } } } as any, { p: [1] }, {}))
      .toBe(scoreSchemaMatch({ properties: { p: { const: 1 } } } as any, { p: 1 }, {}));
    expect(scoreSchemaMatch({ properties: { p: { enum: [[1]] } } } as any, { p: [1] }, {}))
      .toBe(scoreSchemaMatch({ properties: { p: { enum: [1] } } } as any, { p: 1 }, {}));
  });
});

describe('round 14: cycle defaults, budgeted equality, deep schemas, linear merging', () => {
  it('a repeated reference still contributes its own default, as at base', () => {
    const node = { type: 'object', properties: { name: { default: 'branch' }, child: { $ref: '#/$defs/node', default: { name: 'leaf' } } } };
    const root: any = { $defs: { node }, $ref: '#/$defs/node' };
    const expected = { name: 'branch', child: { name: 'leaf' } };
    expect(getDefaultValue(root, root)).toEqual(expected);
    expect(getPreviewDefaultValue(root, root)).toEqual(expected);
    expect(baseCore.getDefaultValue(root, root)).toEqual(expected);
    expect(baseCore.getPreviewDefaultValue(root, root)).toEqual(expected);
    // Without a default of its own the repeat still stops: the nested
    // {$ref} step expands once (the root step is a different schema), and
    // its own repeat is cut.
    const bare: any = { $defs: { node: { type: 'object', properties: { child: { $ref: '#/$defs/node' } } } }, $ref: '#/$defs/node' };
    expect(getDefaultValue(bare, bare)).toEqual({ child: {} });
  });

  it('values of different JSON types are unequal without canonicalising either', () => {
    const big: Record<string, number> = {};
    for (let i = 0; i < 20000; i++) big[`k${i}`] = i;
    withSchemaBudget(() => {
      expect(sameValue(big, 1)).toBe(false);
      expect(sameValue([1], { 0: 1 })).toBe(false);
      expect(sameValue('1', 1)).toBe(false);
      expect(sameValue(null, {})).toBe(false);
    });
    expect(schemaWorkDone()).toBeLessThan(10);
  });

  it('canonical keys are charged per node and computed once per value within a render', () => {
    const make = () => { const o: Record<string, number> = {}; for (let i = 0; i < 20000; i++) o[`k${i}`] = i; return o; };
    const a = make(), b = make();
    withSchemaBudget(() => expect(sameValue(a, b)).toBe(true));
    expect(schemaWorkDone()).toBeGreaterThanOrEqual(40000);
    withSchemaBudget(() => { for (let i = 0; i < 100; i++) sameValue(a, b); });
    expect(schemaWorkDone()).toBeLessThan(3 * 40000);
    // The cache lives for one render only: a value mutated between renders
    // compares by its new content.
    b.k0 = -1;
    withSchemaBudget(() => expect(sameValue(a, b)).toBe(false));
    expect(sameValue(a, b)).toBe(false);
    b.k0 = 0;
    expect(sameValue(a, b)).toBe(true);
  });

  it('an enum of numbers scored against a 20,000-key value canonicalises it once', () => {
    // pi's repro. Rendering 20,000 keys is slow in the test DOM whatever the
    // schema says, so this times the scoring, which was the cost: every enum
    // member canonicalised the whole value.
    const big: Record<string, number> = {};
    for (let i = 0; i < 20000; i++) big[`k${i}`] = i;
    const v = { oneOf: [
      { title: 'Numbers', properties: { p: { enum: Array.from({ length: 1000 }, (_, i) => i) } } },
      { title: 'Object', properties: { p: { const: big } } },
    ] };
    const started = Date.now();
    const score = withSchemaBudget(() => scoreSchemaMatch(v as any, { p: { ...big } }, { type: 'object', properties: { v } }));
    expect(Date.now() - started).toBeLessThan(500);
    expect(score).toBeGreaterThan(50);
    // The value and the const, 20,001 nodes each, and nothing per number.
    expect(schemaWorkDone()).toBeLessThan(2 * 20001 + 100);
  });

  it('a 10,000-level schema falls back or renders in every entry point instead of throwing', async () => {
    const depth = 10000;
    const deepText = '{"not":'.repeat(depth) + '{"type":"string"}' + '}'.repeat(depth);
    const rootText = `{"type":"object","properties":{"v":${deepText}}}`;
    const mounts: Array<[string, () => Promise<HTMLElement>]> = [
      ['display', () => mountEl('schema-display-mode', { schema: JSON.parse(rootText), value: { v: 'x' } })],
      ['form', () => mountEl('schema-form-mode', { schema: JSON.parse(rootText), value: { v: 'x' } })],
      ['form-string', () => mountEl('schema-form-mode', { schema: rootText, value: { v: 'x' } })],
      ['search', () => mountEl('schema-search-mode', { schema: rootText, metaQuery: '[]' })],
      ['tree', () => mountEl('schema-edit-mode', { schema: JSON.parse(rootText) })],
      ['tree-root', () => mountEl('schema-edit-mode', { schema: JSON.parse(deepText) })],
      ['meta', () => mountEl('meta-shortcode', {}, { 'data-path': 'v', 'data-schema': deepText, 'data-value': '"x"' })],
    ];
    for (const [name, mount] of mounts) {
      const el = await mount();
      expect(el.isConnected, name).toBe(true);
      // The tree editor serialises the whole schema, which overflows here.
      if (name.startsWith('tree')) expect(snapshot(el), name).toContain('showing the raw value');
      document.body.replaceChildren();
    }
  });

  it('a schema JSON.stringify cannot serialise falls back in the tree editor', async () => {
    // Node's JSON.stringify survives depths that overflow it in a browser
    // (pi's 8,000 nested not), so the overflow is simulated.
    // The echo check serialises the incoming schema once a tree exists.
    const el: any = await mountEl('schema-edit-mode', { schema: { type: 'object' } });
    const schema: any = { type: 'object', properties: { v: { type: 'string' } } };
    Object.defineProperty(schema, 'toJSON', { enumerable: false, value: () => { throw new RangeError('Maximum call stack size exceeded'); } });
    el.schema = schema;
    await settle(el);
    expect(snapshot(el)).toContain('Schema could not be used; showing the raw value.');
    expect(snapshot(el)).toContain('too deeply nested to show');
  });

  it('a value the form cannot copy falls back, and the next value renders', async () => {
    const schema = { type: 'object', properties: { v: { type: 'string', title: 'Vee' } } };
    // structuredClone refuses a function where JSON.stringify skips it.
    const el: any = await mountEl('schema-form-mode', { schema, value: { v: 'x', f: () => 1 } });
    expect(snapshot(el)).toContain('showing the raw value');
    el.value = { v: 'y' };
    await settle(el);
    expect(snapshot(el)).not.toContain('showing the raw value');
    expect(el.querySelector('input:not([type="hidden"])')?.value).toBe('y');
  });

  it('a fallback whose raw value cannot be serialised still renders', async () => {
    let deep: any = { type: 'string' };
    for (let i = 0; i < 10000; i++) deep = { not: deep };
    const el = await mountEl('schema-edit-mode', { schema: deep });
    expect(snapshot(el)).toMatch(/showing the raw value|Root|object|string/i);
  });

  it('5,000 allOf branches of one property each merge in linear time, without touching the input', async () => {
    const allOf = Array.from({ length: 5000 }, (_, i) => ({ properties: { [`p${i}`]: { type: 'string', title: `P${i}` } } }));
    const schema: any = { type: 'object', properties: { v: { type: 'object', allOf } } };
    const before = JSON.stringify(schema);
    const started = Date.now();
    const resolved = withSchemaBudget(() => resolveSchema(schema.properties.v, schema))!;
    expect(Date.now() - started).toBeLessThan(300);
    expect(Object.keys(resolved.properties)).toHaveLength(5000);
    expect(withSchemaBudget(() => resolveSchema(schema.properties.v, schema))).toEqual(resolved);
    expect(JSON.stringify(schema)).toBe(before);
    const value = { v: { p0: 'zero', p4999: 'last' } };
    const panelStarted = Date.now();
    const panel = await mountEl('schema-display-mode', { schema, value });
    expect(Date.now() - panelStarted).toBeLessThan(1000);
    expect(panel.textContent).toContain('P4999');
    expect(JSON.stringify(schema)).toBe(before);
  });

  it('merging in place never writes into a property object the caller owns', () => {
    const shared = { type: 'object', properties: { a: { type: 'string' } } };
    const schema: any = { allOf: [shared, { properties: { b: { type: 'string' } } }, { properties: { c: { type: 'string' } } }] };
    const resolved = resolveSchema(schema, schema)!;
    expect(Object.keys(resolved.properties).sort()).toEqual(['a', 'b', 'c']);
    expect(Object.keys(shared.properties)).toEqual(['a']);
    const merged = mergeAllOf(schema, schema).merged;
    expect(Object.keys(merged.properties).sort()).toEqual(['a', 'b', 'c']);
    expect(Object.keys(shared.properties)).toEqual(['a']);
    // The schema's own properties, beside its allOf, are the caller's too.
    const own: any = { properties: { a: { type: 'string' } }, allOf: [{ properties: { b: { type: 'string' } } }, { properties: { c: { type: 'string' } } }] };
    expect(Object.keys(resolveSchema(own, own)!.properties).sort()).toEqual(['a', 'b', 'c']);
    expect(Object.keys(mergeAllOf(own, own).merged.properties).sort()).toEqual(['a', 'b', 'c']);
    expect(scoreSchemaMatch(own, { a: 'x', b: 'y', c: 'z' }, own)).toBeGreaterThan(0);
    expect(Object.keys(own.properties)).toEqual(['a']);
  });
});

describe('round 15: patterns the browser cannot compile, exact multipleOf, per-operation recursion', () => {
  async function mountInForm(prop: any, v: unknown) {
    const form = document.createElement('form');
    document.body.append(form);
    const el = document.createElement('schema-form-mode') as any;
    el.schema = { type: 'object', properties: { v: prop } };
    el.value = { v };
    form.append(el);
    await settle(el);
    const input = el.querySelector('input:not([type="hidden"])') as HTMLInputElement;
    const submit = async () => {
      const ev = new Event('submit', { cancelable: true });
      form.dispatchEvent(ev);
      await settle(el);
      return ev.defaultPrevented;
    };
    return { el, input, submit };
  }

  it('a Go (?i) prefix becomes the i flag, on blur and on submit', async () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {});
    warn.mockClear();
    const { el, input, submit } = await mountInForm({ type: 'string', pattern: '(?i)^[a-z]+$' }, 'Book');
    input.dispatchEvent(new Event('blur'));
    await settle(el);
    expect(el.textContent).not.toContain('Must match');
    expect(await submit()).toBe(false);
    document.body.replaceChildren();
    const bad = await mountInForm({ type: 'string', pattern: '(?i)^[a-z]+$' }, 'Book 2');
    expect(await bad.submit()).toBe(true);
    expect(warn).not.toHaveBeenCalled();
  });

  it('a pattern JS cannot compile is no constraint, with a warning, never an exception', async () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {});
    warn.mockClear();
    const pattern = '(?P<word>[a-z]+)';
    const { el, input, submit } = await mountInForm({ type: 'string', pattern }, 'Book');
    input.dispatchEvent(new Event('blur'));
    await settle(el);
    expect(el.textContent).not.toContain('Must match');
    expect(await submit()).toBe(false);
    expect(warn).toHaveBeenCalled();
    expect(evaluateCondition({ pattern }, 'Book', {})).toBe(true);
    expect(evaluateCondition({ properties: { v: { pattern } } }, { v: 'Book' }, {})).toBe(true);
  });

  it('conditions apply a (?i) pattern case-insensitively', () => {
    expect(evaluateCondition({ pattern: '(?i)^book$' }, 'BOOK', {})).toBe(true);
    expect(evaluateCondition({ pattern: '(?i)^book$' }, 'BOOKS', {})).toBe(false);
    expect(evaluateCondition({ properties: { v: { pattern: '(?i)^b' } } }, { v: 'Book' }, {})).toBe(true);
    expect(evaluateCondition({ properties: { v: { pattern: '(?i)^b' } } }, { v: 'Cook' }, {})).toBe(false);
  });

  it('multipleOf is exact at the divisor\'s decimal places', async () => {
    for (const [m, v, blocked] of [
      [0.01, 10000000.005, true], [0.1, 0.35, true], [0.1, 0.3, false], [0.01, 1.23, false],
      [1e-7, 3e-7, false], [1.5e-7, 4.5e-7, false], [1.5e-7, 4e-7, true], [0.5, 1e308, false],
      [0.25, 0.75, false], [3, 9, false], [3, 10, true], [3, 3000000001, true],
      // Past exact integer range: 2^62 leaves 1 when divided by 3.
      [3, 2 ** 62, true], [4, 2 ** 62, false],
    ] as const) {
      const { submit } = await mountInForm({ type: 'number', multipleOf: m }, v);
      expect(await submit(), `${v} multipleOf ${m}`).toBe(blocked);
      document.body.replaceChildren();
    }
  });

  it('recursion is classified afresh after the root is edited in place', () => {
    // a repeats on its own path through the sibling p, so the first walk
    // classifies the references (a: not recursive) before the edit.
    const root: any = {
      $defs: { a: { type: 'object', properties: { name: { default: 'x' } } } },
      allOf: [{ $ref: '#/$defs/a' }, { properties: { p: { $ref: '#/$defs/a' } } }],
    };
    expect(withSchemaBudget(() => getDefaultValue(root, root))).toEqual({ name: 'x', p: { name: 'x' } });
    root.$defs.a.properties.child = { $ref: '#/$defs/a' };
    const fresh = structuredClone(root);
    const expected = getDefaultValue(fresh, fresh);
    expect(JSON.stringify(expected).length).toBeLessThan(60);
    expect(withSchemaBudget(() => getDefaultValue(root, root))).toEqual(expected);
    expect(getDefaultValue(root, root)).toEqual(expected);
  });
});

describe('round 16: unicode-mode patterns, restoring a schema after a fallback', () => {
  async function submitWith(pattern: string, v: string) {
    const form = document.createElement('form');
    document.body.append(form);
    const el = document.createElement('schema-form-mode') as any;
    el.schema = { type: 'object', properties: { v: { type: 'string', pattern } } };
    el.value = { v };
    form.append(el);
    await settle(el);
    const ev = new Event('submit', { cancelable: true });
    form.dispatchEvent(ev);
    await settle(el);
    document.body.replaceChildren();
    return ev.defaultPrevented;
  }

  it('a (?i) pattern with a Unicode property keeps its meaning', async () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {});
    warn.mockClear();
    const pattern = '(?i)^[\\p{L}\\s-]+$';
    expect(await submitWith(pattern, 'Alice')).toBe(false);
    expect(await submitWith(pattern, 'Zoë Saldaña')).toBe(false);
    expect(await submitWith(pattern, 'Alice 2')).toBe(true);
    expect(evaluateCondition({ pattern }, 'Alice', {})).toBe(true);
    expect(warn).not.toHaveBeenCalled();
  });

  it('a Unicode property pattern without (?i) is case-sensitive', async () => {
    expect(await submitWith('^\\p{Lu}', 'Alice')).toBe(false);
    expect(await submitWith('^\\p{Lu}', 'alice')).toBe(true);
    expect(evaluateCondition({ pattern: '^\\p{Lu}' }, 'alice', {})).toBe(false);
  });

  it('a pattern only valid without unicode mode behaves as at base', async () => {
    // \- outside a class is an identity escape only without the u flag.
    expect(await submitWith('^a\\-b$', 'a-b')).toBe(false);
    expect(await submitWith('^a\\-b$', 'ab')).toBe(true);
    expect(evaluateCondition({ pattern: '^a\\-b$' }, 'ab', {})).toBe(baseCore.evaluateCondition({ pattern: '^a\\-b$' }, 'ab', {}));
    expect(evaluateCondition({ pattern: '^a\\-b$' }, 'a-b', {})).toBe(true);
  });

  it('a pattern whose meaning depends on unicode mode but fails in it is no constraint', async () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {});
    warn.mockClear();
    // \- makes the u compile fail; without u, \p{L} would mean "p{L}".
    for (const pattern of ['^\\p{L}\\-$', '^\\u{41}\\-$']) {
      expect(await submitWith(pattern, 'zz'), pattern).toBe(false);
      expect(evaluateCondition({ pattern }, 'zz', {}), pattern).toBe(true);
    }
    expect(warn).toHaveBeenCalled();
  });

  it('restoring the last emitted schema after a fallback rebuilds the tree', async () => {
    const schema = { type: 'object', properties: { name: { type: 'string', title: 'Name field' } } };
    const el: any = await mountEl('schema-edit-mode', { schema });
    let emitted = '';
    el.addEventListener('schema-change', (e: CustomEvent) => { emitted = e.detail.schema; });
    el._emitSchemaChange();
    expect(emitted).not.toBe('');
    el.schema = { $defs: { bad: null } };
    await settle(el);
    expect(snapshot(el)).toContain('showing the raw value');
    // The parent echoes back exactly what the editor last emitted.
    el.schema = JSON.parse(emitted);
    await settle(el);
    expect(snapshot(el)).not.toContain('showing the raw value');
    expect(snapshot(el)).toContain('name');
  });
});

describe('round 17: user actions inside the fallback boundary, typed search values', () => {
  const doubling = () => {
    const $defs: Record<string, any> = { a30: { type: 'string', default: 'L' } };
    for (let i = 29; i >= 0; i--) $defs[`a${i}`] = { allOf: [{ $ref: `#/$defs/a${i + 1}` }, { $ref: `#/$defs/a${i + 1}` }] };
    return $defs;
  };

  function collectErrors() {
    const errors: unknown[] = [];
    const onError = (e: ErrorEvent) => { errors.push(e.error ?? e.message); e.preventDefault(); };
    window.addEventListener('error', onError);
    return { errors, stop: () => window.removeEventListener('error', onError) };
  }

  it('Add Item on an over-budget item schema falls back instead of throwing', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => {});
    const schema = { $defs: doubling(), type: 'object', properties: { items: { type: 'array', items: { $ref: '#/$defs/a0' } } } };
    const el: any = await mountEl('schema-form-mode', { schema, value: { items: [] } });
    const add = [...el.querySelectorAll('button')].find((b: HTMLButtonElement) => b.textContent?.includes('Add Item')) as HTMLButtonElement;
    expect(add).toBeTruthy();
    const { errors, stop } = collectErrors();
    let thrown: unknown = null;
    try { add.click(); } catch (error) { thrown = error; }
    await settle(el);
    stop();
    expect(thrown).toBeNull();
    expect(errors).toEqual([]);
    expect(snapshot(el)).toContain('Schema too complex to display');
    // A new value leaves the fallback.
    el.value = { items: [] };
    await settle(el);
    expect(snapshot(el)).not.toContain('Schema too complex to display');
  });

  it('the [meta] edit action on a missing value falls back instead of throwing', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => {});
    const el: any = await mountEl('meta-shortcode', {}, {
      'data-path': 'v', 'data-editable': 'true', 'data-entity-type': 'group', 'data-entity-id': '1',
      'data-schema': JSON.stringify({ $defs: doubling(), $ref: '#/$defs/a0' }),
    });
    const edit = el.querySelector('button[aria-label^="Edit"]') as HTMLButtonElement;
    expect(edit).toBeTruthy();
    const { errors, stop } = collectErrors();
    let thrown: unknown = null;
    try { edit.click(); } catch (error) { thrown = error; }
    await settle(el);
    stop();
    expect(thrown).toBeNull();
    expect(errors).toEqual([]);
    expect(snapshot(el)).toContain('Schema too complex to display');
  });

  it('an action runs under one budget, with its host as this, and catches rejections', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => {});
    const host = { requestUpdate: vi.fn(), name: 'host' };
    // One budget spans the whole action: work counted before a walker call
    // still counts against it.
    const heavy = guardSchemaAction(host, 'test', () => { countSchemaWork(SCHEMA_WORK_BUDGET); return getDefaultValue({ type: 'string' }); });
    expect(heavy()).toBeUndefined();
    expect(schemaActionError(host)).toBeInstanceOf(SchemaTooComplexError);
    expect(host.requestUpdate).toHaveBeenCalled();
    clearSchemaActionError(host);
    const named = guardSchemaAction(host, 'test', function (this: { name: string }, suffix: string) { return this.name + suffix; });
    expect(named('!')).toBe('host!');
    expect(schemaActionError(host)).toBeUndefined();
    const rejected = guardSchemaAction(host, 'test', async () => { throw new Error('later'); });
    await rejected();
    expect(String(schemaActionError(host))).toContain('later');
  });

  it('every event binding in the five entry points goes through the action boundary', async () => {
    const fs = await import('node:fs');
    const path = await import('node:path');
    const files = [
      'src/schema-editor/modes/display-mode.ts',
      'src/schema-editor/modes/form-mode.ts',
      'src/schema-editor/modes/search-mode.ts',
      'src/schema-editor/modes/edit-mode.ts',
      'src/webcomponents/meta-shortcode.ts',
    ];
    // Handlers that only stop propagation reach no schema code.
    const inert = new Set(['(e: Event) => e.stopPropagation()']);
    const unguarded: string[] = [];
    let bindings = 0;
    for (const file of files) {
      const source = fs.readFileSync(path.resolve(process.cwd(), file), 'utf8');
      for (const match of source.matchAll(/@[\w-]+=\$\{/g)) {
        bindings++;
        const rest = source.slice(match.index! + match[0].length);
        if (rest.startsWith('this._act(')) continue;
        const expression = rest.slice(0, rest.indexOf('}'));
        if (inert.has(expression)) continue;
        unguarded.push(`${file}: ${match[0]}${rest.slice(0, 60).replace(/\s+/g, ' ')}`);
      }
      // Listeners added by hand are guarded where they are defined.
      for (const match of source.matchAll(/addEventListener\([^,]+,\s*this\.(\w+)/g)) {
        bindings++;
        if (!new RegExp(`${match[1]}[^=\\n]*=\\s*this\\._act\\(`).test(source)) unguarded.push(`${file}: listener ${match[1]}`);
      }
    }
    expect(bindings).toBeGreaterThan(40);
    expect(unguarded).toEqual([]);
  });

  it('a typed number filter stays a number when the schema rebuilds into a mixed enum', async () => {
    const el: any = await mountEl('schema-search-mode', {
      schema: JSON.stringify({ type: 'object', properties: { rating: { type: 'number' } } }),
      metaQuery: '[]',
    });
    const input = el.querySelector('input[type="number"]') as HTMLInputElement;
    input.value = '1';
    input.dispatchEvent(new Event('input'));
    await settle(el);
    const hidden = () => [...el.querySelectorAll('input[type="hidden"]')].map((i: any) => i.value);
    expect(hidden()).toEqual(['rating:EQ:1']);
    el.schema = JSON.stringify({ type: 'object', properties: { rating: { type: ['number', 'string'], enum: [1, '1', 2] } } });
    await settle(el);
    expect(hidden()).toEqual(['rating:EQ:1']);
  });

  it('a string filter rebuilt into the same mixed enum keeps its string choice', async () => {
    const el: any = await mountEl('schema-search-mode', {
      schema: JSON.stringify({ type: 'object', properties: { rating: { type: 'string' } } }),
      metaQuery: '[]',
    });
    const input = el.querySelector('input[type="text"]') as HTMLInputElement;
    input.value = '1';
    input.dispatchEvent(new Event('input'));
    await settle(el);
    el.schema = JSON.stringify({ type: 'object', properties: { rating: { type: ['number', 'string'], enum: [1, '1', 2] } } });
    await settle(el);
    expect([...el.querySelectorAll('input[type="hidden"]')].map((i: any) => i.value)).toEqual(['rating:EQ:"1"']);
  });
});

describe('round 18: selector-safe property names, defaults pruned only on real recursion', () => {
  const NAMES = ['source["id"]', "it's", 'a\\b', 'x y', '[0]', 'q"'];

  async function exercise(el: HTMLElement) {
    // Every control an entry point renders, through its action: buttons are
    // clicked, inputs and selects get input, change and blur.
    for (let round = 0; round < 2; round++) {
      for (const button of [...el.querySelectorAll<HTMLButtonElement>('button')]) {
        if (!button.isConnected || button.disabled) continue;
        button.click();
        await settle(el);
      }
      for (const control of [...el.querySelectorAll<HTMLInputElement | HTMLSelectElement>('input:not([type="hidden"]), select, textarea')]) {
        if (!control.isConnected) continue;
        if (control instanceof HTMLInputElement && control.type !== 'checkbox' && control.type !== 'radio') control.value = control.type === 'number' ? '1' : 'v';
        for (const type of ['input', 'change', 'blur']) control.dispatchEvent(new Event(type));
        await settle(el);
      }
    }
  }

  it('property names with quotes, brackets, backslashes and spaces survive every action', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => {});
    vi.stubGlobal('confirm', () => true);
    vi.stubGlobal('prompt', () => null);
    // [meta]'s Save posts the value; answer it without a server.
    vi.stubGlobal('fetch', async () => ({ ok: true, status: 200, json: async () => ({}) }));
    const properties: Record<string, any> = {};
    const value: Record<string, any> = {};
    for (const name of NAMES) {
      properties[name] = { type: 'number', title: `T ${name}` };
      value[name] = 1;
    }
    const schema = { type: 'object', properties };
    const errors: unknown[] = [];
    const onError = (e: ErrorEvent) => { errors.push(e.error ?? e.message); e.preventDefault(); };
    window.addEventListener('error', onError);
    try {
      for (const [tag, props] of [
        ['schema-search-mode', { schema: JSON.stringify(schema), metaQuery: '[]' }],
        ['schema-form-mode', { schema, value }],
        ['schema-display-mode', { schema, value }],
        ['schema-edit-mode', { schema }],
      ] as const) {
        const el = await mountEl(tag, props as any);
        await exercise(el);
        expect(snapshot(el), tag).not.toContain('showing the raw value');
        document.body.replaceChildren();
      }
      for (const name of NAMES) {
        const el = await mountEl('meta-shortcode', {}, {
          'data-path': name, 'data-editable': 'true', 'data-entity-type': 'group', 'data-entity-id': '1',
          'data-schema': JSON.stringify(properties[name]), 'data-value': '1',
        });
        await exercise(el);
        expect(snapshot(el), name).not.toContain('showing the raw value');
        document.body.replaceChildren();
      }
    } finally {
      window.removeEventListener('error', onError);
    }
    expect(errors.map(String)).toEqual([]);
  });

  it('the search operator toggle focuses its own field whatever the name', async () => {
    const name = 'source["id"]';
    const el: any = await mountEl('schema-search-mode', {
      schema: JSON.stringify({ type: 'object', properties: { [name]: { type: 'number' } } }), metaQuery: '[]',
    });
    const toggle = el.querySelector('button[data-operator-toggle]') as HTMLButtonElement;
    expect(toggle).toBeTruthy();
    toggle.click();
    await settle(el);
    await new Promise(resolve => setTimeout(resolve, 0));
    const select = el.querySelector('select') as HTMLSelectElement;
    expect(select).toBeTruthy();
    expect(document.activeElement).toBe(select);
    select.value = select.options[1].value;
    select.dispatchEvent(new Event('change'));
    await settle(el);
    await new Promise(resolve => setTimeout(resolve, 0));
    expect(document.activeElement).toBe(el.querySelector('button[data-operator-toggle]'));
  });

  it('a deferred step that throws inside an action reaches the fallback', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => {});
    const el: any = await mountEl('schema-search-mode', {
      schema: JSON.stringify({ type: 'object', properties: { n: { type: 'number' } } }), metaQuery: '[]',
    });
    // Break the deferred focus step: its rejection must reach the boundary.
    const focus = HTMLSelectElement.prototype.focus;
    HTMLSelectElement.prototype.focus = () => { throw new SyntaxError('broken focus'); };
    try {
      (el.querySelector('button[data-operator-toggle]') as HTMLButtonElement).click();
      await settle(el);
      await new Promise(resolve => setTimeout(resolve, 0));
      await settle(el);
    } finally {
      HTMLSelectElement.prototype.focus = focus;
    }
    expect(schemaActionError(el)).toBeInstanceOf(SyntaxError);
    expect(snapshot(el)).toContain('showing the raw value');

    // Choosing an operator focuses the toggle again, also deferred.
    const other: any = await mountEl('schema-search-mode', {
      schema: JSON.stringify({ type: 'object', properties: { n: { type: 'number' } } }), metaQuery: '[]',
    });
    (other.querySelector('button[data-operator-toggle]') as HTMLButtonElement).click();
    await settle(other);
    const buttonFocus = HTMLButtonElement.prototype.focus;
    HTMLButtonElement.prototype.focus = () => { throw new SyntaxError('broken toggle focus'); };
    try {
      const select = other.querySelector('select') as HTMLSelectElement;
      select.value = select.options[1].value;
      select.dispatchEvent(new Event('change'));
      await settle(other);
      await new Promise(resolve => setTimeout(resolve, 0));
      await settle(other);
    } finally {
      HTMLButtonElement.prototype.focus = buttonFocus;
    }
    expect(String(schemaActionError(other))).toContain('broken toggle focus');
  });

  it('a reference repeated off its recursive path still gets its full default, as at base', () => {
    const $defs = {
      Node: { type: 'object', properties: { name: { type: 'string', default: 'Untitled' }, children: { type: 'array', items: { $ref: '#/$defs/Node' } } } },
    };
    const root: any = { $defs, type: 'array', items: { allOf: [{ $ref: '#/$defs/Node' }], properties: { selected: { $ref: '#/$defs/Node' } } } };
    const expected = { name: 'Untitled', children: [], selected: { name: 'Untitled', children: [] } };
    expect(getDefaultValue(root.items, root)).toEqual(expected);
    expect(getPreviewDefaultValue(root.items, root)).toEqual(expected);
    expect(baseCore.getDefaultValue(root.items, root)).toEqual(expected);
    // The same through an allOf branch of the repeated property.
    const viaAllOf: any = { $defs, allOf: [{ $ref: '#/$defs/Node' }], properties: { selected: { allOf: [{ $ref: '#/$defs/Node' }] } } };
    expect(getDefaultValue(viaAllOf, viaAllOf)).toEqual(expected);
    expect(getPreviewDefaultValue(viaAllOf, viaAllOf)).toEqual(expected);
    expect(baseCore.getDefaultValue(viaAllOf, viaAllOf)).toEqual(expected);
  });

  it('Add Item builds the same default through the form', async () => {
    const $defs = {
      Node: { type: 'object', properties: { name: { type: 'string', default: 'Untitled' }, children: { type: 'array', items: { $ref: '#/$defs/Node' } } } },
    };
    const schema: any = { $defs, type: 'object', properties: { list: { type: 'array', items: { allOf: [{ $ref: '#/$defs/Node' }], properties: { selected: { $ref: '#/$defs/Node' } } } } } };
    const el: any = await mountEl('schema-form-mode', { schema, value: { list: [] } });
    let emitted: any;
    el.addEventListener('value-change', (e: CustomEvent) => { emitted = e.detail.value; });
    ([...el.querySelectorAll('button')].find((b: HTMLButtonElement) => b.textContent?.includes('Add Item')) as HTMLButtonElement).click();
    await settle(el);
    expect(emitted?.list?.[0]).toEqual({ name: 'Untitled', children: [], selected: { name: 'Untitled', children: [] } });
  });

  it('a property cycle through objects is still cut at its first repeat', () => {
    const root: any = { $defs: { N: { type: 'object', properties: { name: { default: 'n' }, self: { $ref: '#/$defs/N' } } } }, $ref: '#/$defs/N' };
    // The root step ({$defs, $ref}) differs from the nested {$ref} step, so
    // the nested one expands once and its own repeat is cut.
    expect(getDefaultValue(root, root)).toEqual({ name: 'n', self: { name: 'n' } });
    expect(getPreviewDefaultValue(root, root)).toEqual({ name: 'n', self: { name: 'n' } });
    // Through an allOf branch too.
    const viaAllOf: any = { $defs: { N: { allOf: [{ properties: { name: { default: 'n' }, next: { $ref: '#/$defs/N' } } }] } }, $ref: '#/$defs/N' };
    expect(getDefaultValue(viaAllOf, viaAllOf)).toEqual({ name: 'n', next: { name: 'n' } });
  });
});

describe('round 19: typed patternDescription, defaults pruned on the live expansion stack', () => {
  it('a non-string patternDescription falls back to the standard message', async () => {
    expect(withTypedKeywords({ pattern: '^a$', patternDescription: H } as any)).not.toHaveProperty('patternDescription');
    expect(withTypedKeywords({ pattern: '^a$', patternDescription: 'Letters only' } as any)).toHaveProperty('patternDescription', 'Letters only');
    for (const description of [H, { a: 1 }, 7, ['x']]) {
      const el: any = await mountEl('schema-form-mode', {
        schema: { type: 'object', properties: { v: { type: 'string', pattern: '^[A-Z]+$', patternDescription: description } } },
        value: { v: 'bad' },
      });
      const input = el.querySelector('input:not([type="hidden"])') as HTMLInputElement;
      input.dispatchEvent(new Event('blur'));
      await settle(el);
      expect(el.textContent).toContain('Must match the expected format (^[A-Z]+$)');
      expect(el.textContent).not.toContain('[object Object]');
      expect(schemaActionError(el)).toBeUndefined();
      document.body.replaceChildren();
    }
  });

  it('a string patternDescription is the message', async () => {
    const el: any = await mountEl('schema-form-mode', {
      schema: { type: 'object', properties: { v: { type: 'string', pattern: '^[A-Z]+$', patternDescription: 'Capitals only' } } },
      value: { v: 'bad' },
    });
    (el.querySelector('input:not([type="hidden"])') as HTMLInputElement).dispatchEvent(new Event('blur'));
    await settle(el);
    expect(el.textContent).toContain('Capitals only');
  });

  const NODE = {
    type: 'object',
    properties: { name: { type: 'string', default: 'Untitled' }, kind: { type: 'string', default: 'leaf' } },
    if: { properties: { kind: { const: 'branch' } } },
    then: { properties: { child: { $ref: '#/$defs/Node' } } },
  };

  it('an inactive then never makes a reference look recursive (pi\'s repro)', () => {
    const root: any = { $defs: { Node: NODE }, allOf: [{ $ref: '#/$defs/Node' }, { properties: { selected: { $ref: '#/$defs/Node' } } }] };
    const expected = { name: 'Untitled', kind: 'leaf', selected: { name: 'Untitled', kind: 'leaf' } };
    expect(getDefaultValue(root, root)).toEqual(expected);
    expect(getPreviewDefaultValue(root, root)).toEqual(expected);
    expect(baseCore.getDefaultValue(root, root)).toEqual(expected);
  });

  it('an active then that recurs is cut at its first repeat', () => {
    const branch = { ...NODE, properties: { ...NODE.properties, kind: { type: 'string', default: 'branch' } } };
    const root: any = { $defs: { Node: branch }, $ref: '#/$defs/Node' };
    // The root step ({$defs, $ref}) differs from the nested {$ref} step, so
    // the nested one expands once and its own repeat is cut.
    expect(getDefaultValue(root, root)).toEqual({ name: 'Untitled', kind: 'branch', child: { name: 'Untitled', kind: 'branch' } });
  });

  it('a mutual a -> b -> a cycle is cut at the first repeat', () => {
    const root: any = {
      $defs: {
        a: { type: 'object', properties: { name: { default: 'a' }, b: { $ref: '#/$defs/b' } } },
        b: { type: 'object', properties: { name: { default: 'b' }, a: { $ref: '#/$defs/a' } } },
      },
      $ref: '#/$defs/a',
    };
    // The root step differs from the {$ref a} inside b, so a expands twice and
    // the repeat of the {$ref b} step is cut.
    const expected = { name: 'a', b: { name: 'b', a: { name: 'a' } } };
    expect(getDefaultValue(root, root)).toEqual(expected);
    expect(getPreviewDefaultValue(root, root)).toEqual(expected);
  });

  it('an allOf-only cycle reached through an allOf ends at its first repeat', () => {
    const root: any = {
      $defs: {
        A: { allOf: [{ $ref: '#/$defs/B' }], properties: { x: { default: 1 } } },
        B: { allOf: [{ $ref: '#/$defs/A' }], properties: { y: { default: 2 } } },
      },
      allOf: [{ $ref: '#/$defs/A' }],
    };
    expect(withSchemaBudget(() => getDefaultValue(root, root))).toEqual({ x: 1, y: 2 });
    expect(withSchemaBudget(() => getPreviewDefaultValue(root, root))).toEqual({ x: 1, y: 2 });
  });

  it('an alias cycle stops at once, without walking to the depth limit', () => {
    const root: any = { $defs: { A: { $ref: '#/$defs/B', default: 'a' }, B: { $ref: '#/$defs/A' } }, $ref: '#/$defs/A' };
    expect(withSchemaBudget(() => getDefaultValue(root, root))).toBe('a');
    expect(schemaWorkDone()).toBeLessThan(200);
    const self: any = { $defs: { S: { $ref: '#/$defs/S' } }, type: 'object', properties: { v: { $ref: '#/$defs/S' } } };
    withSchemaBudget(() => getDefaultValue(self, self));
    expect(schemaWorkDone()).toBeLessThan(200);
  });

  it('a reference reached from a sibling of another expansion is expanded, as at base', () => {
    // d1 = allOf [d2, d2], written beside d2 in an allOf: d2 repeats under d1,
    // but expanding d2 never produced d1.
    const $defs = {
      d2: { type: 'object', properties: { x: { type: 'string', default: 'x' } } },
      d1: { allOf: [{ $ref: '#/$defs/d2' }, { $ref: '#/$defs/d2' }] },
    };
    const root: any = { $defs, allOf: [{ $ref: '#/$defs/d2' }, { properties: { f: { $ref: '#/$defs/d1' } } }] };
    const expected = { x: 'x', f: { x: 'x' } };
    expect(getDefaultValue(root, root)).toEqual(expected);
    expect(getPreviewDefaultValue(root, root)).toEqual(expected);
    expect(baseCore.getDefaultValue(root, root)).toEqual(expected);
  });

  it('sibling expansions of one reference never share the stack', () => {
    const root: any = {
      $defs: { P: { type: 'object', properties: { v: { default: 'p' } } } },
      type: 'object',
      properties: { first: { $ref: '#/$defs/P' }, second: { $ref: '#/$defs/P' }, nested: { type: 'object', properties: { third: { $ref: '#/$defs/P' } } } },
    };
    expect(getDefaultValue(root, root)).toEqual({ first: { v: 'p' }, second: { v: 'p' }, nested: { third: { v: 'p' } } });
  });
});

describe('round 20: defaults pruned on the identity of the referencing node', () => {
  const branchNode = (child: any) => ({
    type: 'object',
    properties: { name: { type: 'string', default: 'Untitled' }, kind: { type: 'string', default: 'branch' } },
    if: { properties: { kind: { const: 'branch' } } },
    then: { properties: { child } },
  });

  it('a recursion a sibling override ends one level down is built, as at base (pi\'s repro)', () => {
    const $defs = { Node: branchNode({ $ref: '#/$defs/Node', properties: { kind: { default: 'leaf' } } }) };
    const root: any = { $defs, type: 'object', properties: { nodes: { type: 'array', items: { $ref: '#/$defs/Node' } } } };
    const item = root.properties.nodes.items;
    const expected = { name: 'Untitled', kind: 'branch', child: { name: 'Untitled', kind: 'leaf' } };
    expect(getDefaultValue(item, root)).toEqual(expected);
    expect(getPreviewDefaultValue(item, root)).toEqual(expected);
    expect(baseCore.getDefaultValue(item, root)).toEqual(expected);
  });

  it('Add Item builds that default through the form, without falling back', async () => {
    const $defs = { Node: branchNode({ $ref: '#/$defs/Node', properties: { kind: { default: 'leaf' } } }) };
    const schema: any = { $defs, type: 'object', properties: { nodes: { type: 'array', items: { $ref: '#/$defs/Node' } } } };
    const el: any = await mountEl('schema-form-mode', { schema, value: { nodes: [] } });
    let emitted: any;
    el.addEventListener('value-change', (e: CustomEvent) => { emitted = e.detail.value; });
    ([...el.querySelectorAll('button')].find((b: HTMLButtonElement) => b.textContent?.includes('Add Item')) as HTMLButtonElement).click();
    await settle(el);
    expect(emitted?.nodes?.[0]).toEqual({ name: 'Untitled', kind: 'branch', child: { name: 'Untitled', kind: 'leaf' } });
    expect(snapshot(el)).not.toContain('showing the raw value');
  });

  it('a reference a merge carries into a schema with other siblings is a new step, as at base', () => {
    // p merges Z's {$ref Y} with a sibling holding inner: {$ref Z}; inside
    // inner, Z's own {$ref Y} is the same node but a different schema.
    const $defs = {
      Y: { type: 'object', properties: { v: { default: 'y' } } },
      Z: { type: 'object', properties: { p: { $ref: '#/$defs/Y' } } },
    };
    const root: any = { $defs, allOf: [{ $ref: '#/$defs/Z' }, { properties: { p: { type: 'object', properties: { inner: { $ref: '#/$defs/Z' } } } } }] };
    const expected = baseCore.getDefaultValue(root, root);
    expect(expected).toEqual({ p: { v: 'y', inner: { p: { v: 'y' } } } });
    expect(getDefaultValue(root, root)).toEqual(expected);
    expect(getPreviewDefaultValue(root, root)).toEqual(baseCore.getPreviewDefaultValue(root, root));
  });

  it('a recursion through the same node whose override does not end it is cut at that node\'s first repeat', () => {
    // The override changes the name, not kind, so then stays active.
    const $defs = { Node: branchNode({ $ref: '#/$defs/Node', properties: { name: { default: 'kid' } } }) };
    const root: any = { $defs, type: 'object', properties: { nodes: { type: 'array', items: { $ref: '#/$defs/Node' } } } };
    const item = root.properties.nodes.items;
    const expected = { name: 'Untitled', kind: 'branch', child: { name: 'kid', kind: 'branch' } };
    expect(withSchemaBudget(() => getDefaultValue(item, root))).toEqual(expected);
    expect(schemaWorkDone()).toBeLessThan(500);
    expect(withSchemaBudget(() => getPreviewDefaultValue(item, root))).toEqual({ name: 'Untitled', kind: 'branch', child: { name: 'kid', kind: 'branch' } });
  });
});

describe('round 21: deferred work inside the boundary, settled initialisation, no leaked steps', () => {
  function collectUncaught() {
    const errors: unknown[] = [];
    const onUncaught = (error: unknown) => { errors.push(error); };
    const onWindowError = (e: ErrorEvent) => { errors.push(e.error ?? e.message); e.preventDefault(); };
    process.on('uncaughtException', onUncaught);
    window.addEventListener('error', onWindowError);
    return { errors, stop: () => { process.off('uncaughtException', onUncaught); window.removeEventListener('error', onWindowError); } };
  }

  it('a deferred default that cannot be serialised falls back instead of throwing', async () => {
    const logged = vi.spyOn(console, 'error').mockImplementation(() => {});
    logged.mockClear();
    const unserialisable = { toJSON() { throw new RangeError('Maximum call stack size exceeded'); } };
    for (const keyword of ['oneOf', 'anyOf']) {
      const { errors, stop } = collectUncaught();
      let el: any;
      try {
        el = await mountEl('schema-form-mode', {
          schema: { type: 'object', properties: { v: { [keyword]: [{ type: 'object', default: unserialisable }] } } },
          value: {},
        });
        await new Promise(resolve => setTimeout(resolve, 0));
        await settle(el);
      } finally {
        stop();
      }
      expect(errors.map(String), keyword).toEqual([]);
      // The failed assignment was caught at the boundary; a later deferred
      // initialisation may then succeed and leave the fallback.
      expect(logged.mock.calls.some(call => String(call[0]).includes('schema action failed')), keyword).toBe(true);
      logged.mockClear();
      document.body.replaceChildren();
    }
  });

  it('every deferred callback in the five entry points goes through the action boundary', async () => {
    const fs = await import('node:fs');
    const path = await import('node:path');
    const unguarded: string[] = [];
    for (const file of [
      'src/schema-editor/modes/display-mode.ts',
      'src/schema-editor/modes/form-mode.ts',
      'src/schema-editor/modes/search-mode.ts',
      'src/schema-editor/modes/edit-mode.ts',
      'src/webcomponents/meta-shortcode.ts',
    ]) {
      const source = fs.readFileSync(path.resolve(process.cwd(), file), 'utf8');
      for (const match of source.matchAll(/\b(queueMicrotask|setTimeout|requestAnimationFrame)\(|\.then\(/g)) {
        const rest = source.slice(match.index! + match[0].length);
        const line = source.slice(source.lastIndexOf('\n', match.index!) + 1, source.indexOf('\n', match.index!));
        // The one helper that defers, and callbacks already guarded.
        if (rest.startsWith('this._act(') || /\bguarded\b/.test(rest.slice(0, 20))) continue;
        // A handler returning its deferred promise is guarded by its own _act.
        if (/^\s*return this\.updateComplete\.then\(/.test(line)) continue;
        unguarded.push(`${file}: ${line.trim()}`);
      }
    }
    expect(unguarded).toEqual([]);
  });

  it('a recursive default that resolves to nothing does not initialise forever', async () => {
    const schema = { $defs: { N: { $ref: '#/$defs/N' } }, type: 'object', properties: { v: { oneOf: [{ $ref: '#/$defs/N' }] } } };
    for (const keyword of ['oneOf', 'anyOf']) {
      const s = JSON.parse(JSON.stringify(schema).replace('"oneOf"', `"${keyword}"`));
      const el: any = document.createElement('schema-form-mode');
      let changes = 0;
      el.addEventListener('value-change', () => { changes++; });
      el.schema = s;
      el.value = {};
      document.body.append(el);
      const settled = await Promise.race([
        (async () => { for (let i = 0; i < 5; i++) { await el.updateComplete; await new Promise(r => setTimeout(r, 0)); } return true; })(),
        new Promise(resolve => setTimeout(() => resolve(false), 1000)),
      ]);
      expect(settled, keyword).toBe(true);
      expect(el.isUpdatePending, keyword).toBe(false);
      // An undefined default is not assigned at all.
      expect(changes, keyword).toBe(0);
      document.body.replaceChildren();
    }
  });

  it('a defined default is put in place as at base, again once the field is emptied', async () => {
    const schema = { type: 'object', properties: { v: { oneOf: [{ type: 'object', properties: { a: { type: 'string', default: 'x' } } }] } } };
    const run = async (tag: string) => {
      const el: any = document.createElement(tag);
      const values: unknown[] = [];
      el.addEventListener('value-change', (e: CustomEvent) => { values.push(JSON.parse(JSON.stringify(e.detail.value))); });
      el.schema = schema;
      el.value = {};
      document.body.append(el);
      for (let i = 0; i < 4; i++) { await settle(el); await new Promise(r => setTimeout(r, 0)); }
      el.value = {};
      for (let i = 0; i < 4; i++) { await settle(el); await new Promise(r => setTimeout(r, 0)); }
      el.remove();
      return values;
    };
    const head = await run('schema-form-mode');
    expect(head.filter((v: any) => v?.v?.a === 'x').length).toBe(2);
    expect(head).toEqual(await run('base-schema-form-mode'));
  });

  it('a budget error while registering expansion steps leaves none behind', () => {
    const $defs = { A: { properties: { a: { default: 1 } } }, B: { properties: { b: { default: 2 } } } };
    const root: any = { $defs, allOf: [{ $ref: '#/$defs/A' }, { $ref: '#/$defs/B' }] };
    const fresh = getDefaultValue(root, root);
    expect(fresh).toEqual({ a: 1, b: 2 });
    // Run out of budget at every point of a build, then build again in the
    // same operation: a step left registered would cut A or B short.
    let probed = 0;
    for (let spent = SCHEMA_WORK_BUDGET - 400; spent <= SCHEMA_WORK_BUDGET; spent++) {
      const again = withSchemaBudget(() => {
        try {
          countSchemaWork(spent);
          getDefaultValue(root, root);
        } catch (error) {
          if (!(error instanceof SchemaTooComplexError)) throw error;
          probed++;
        }
        // A fresh budget for the second build, in the same operation.
        countSchemaWork(-schemaWorkDone());
        return getDefaultValue(root, root);
      });
      expect(again, `spent ${spent}`).toEqual(fresh);
    }
    expect(probed).toBeGreaterThan(0);
  });
});

describe('round 22: variant initialisation composes, keys on full paths, never touches the schema', () => {
  // Mount tag with schema and value, let every deferred initialisation run,
  // and return the settled value and the emitted ones.
  async function settledForm(tag: string, schema: any, value: any) {
    const el: any = document.createElement(tag);
    const emitted: unknown[] = [];
    el.addEventListener('value-change', (e: CustomEvent) => { emitted.push(JSON.parse(JSON.stringify(e.detail.value ?? null))); });
    el.schema = schema;
    el.value = value;
    document.body.append(el);
    for (let round = 0; round < 8; round++) {
      await settle(el);
      await new Promise(resolve => setTimeout(resolve, 0));
    }
    const final = JSON.parse(JSON.stringify(el.value ?? null));
    el.remove();
    return { el, final, emitted };
  }

  const S = (keyword: string) => ({ [keyword]: [{ type: 'string', default: 'hello' }, { type: 'number', default: 2 }] });

  it('sibling variant defaults both land, as at base', async () => {
    for (const keyword of ['oneOf', 'anyOf']) {
      const schema = { type: 'object', properties: { a: S(keyword), b: S(keyword) } };
      const head = await settledForm('schema-form-mode', schema, {});
      const base = await settledForm('base-schema-form-mode', schema, {});
      expect(head.final, keyword).toEqual({ a: 'hello', b: 'hello' });
      expect(head.final, keyword).toEqual(base.final);
      expect(head.emitted.at(-1), keyword).toEqual({ a: 'hello', b: 'hello' });
    }
  });

  it('nested variants under different parents each initialise', async () => {
    for (const keyword of ['oneOf', 'anyOf']) {
      const O = { [keyword]: [{ type: 'object', properties: { choice: S(keyword) } }, { type: 'null' }] };
      const schema = { type: 'object', properties: { a: O, b: O } };
      const head = await settledForm('schema-form-mode', schema, { a: {}, b: {} });
      const base = await settledForm('base-schema-form-mode', schema, { a: {}, b: {} });
      expect(head.final, keyword).toEqual({ a: { choice: 'hello' }, b: { choice: 'hello' } });
      expect(head.final, keyword).toEqual(base.final);
    }
  });

  it('mounting and editing never change the schema\'s own default objects', async () => {
    for (const keyword of ['oneOf', 'anyOf']) {
      const variant = {
        type: 'object',
        default: { kind: 'a', other: 'keep' },
        properties: { kind: { type: 'string' } },
        if: { properties: { kind: { const: 'b' } } },
        then: { properties: { other: { type: 'string' } } },
      };
      const schema: any = { type: 'object', properties: { v: { [keyword]: [variant, { type: 'null' }] } } };
      const before = JSON.stringify(schema);
      const el: any = await mountEl('schema-form-mode', { schema, value: {} });
      for (let round = 0; round < 4; round++) { await settle(el); await new Promise(r => setTimeout(r, 0)); }
      expect(JSON.stringify(schema), keyword).toBe(before);
      const input = el.querySelector('input:not([type="hidden"])') as HTMLInputElement | null;
      if (input) {
        input.value = 'b';
        input.dispatchEvent(new Event('input'));
        input.dispatchEvent(new Event('blur'));
        await settle(el);
      }
      expect(JSON.stringify(schema), keyword).toBe(before);
      document.body.replaceChildren();
    }
  });

  it('schema defaults handed to editable data are copies everywhere', async () => {
    const shared = { tags: ['x'] };
    const schema: any = {
      type: 'object',
      properties: {
        obj: { type: 'object', default: shared, properties: { tags: { type: 'array', items: { type: 'string' } } } },
        list: { type: 'array', items: { type: 'object', default: { n: 1 }, properties: { n: { type: 'number' } } } },
        // Conditional cleanup removes other from an added item: from a copy.
        cleaned: { type: 'array', items: {
          type: 'object', default: { kind: 'a', other: 'keep' }, properties: { kind: { type: 'string' } },
          if: { properties: { kind: { const: 'b' } } }, then: { properties: { other: { type: 'string' } } },
        } },
      },
    };
    const before = JSON.stringify(schema);
    const el: any = await mountEl('schema-form-mode', { schema, value: {} });
    for (const button of [...el.querySelectorAll('button')] as HTMLButtonElement[]) {
      if (/Add|Initialize/.test(button.textContent ?? '')) { button.click(); await settle(el); }
    }
    for (const input of [...el.querySelectorAll('input:not([type="hidden"])')] as HTMLInputElement[]) {
      input.value = input.type === 'number' ? '5' : 'y';
      input.dispatchEvent(new Event('input'));
      input.dispatchEvent(new Event('blur'));
      await settle(el);
    }
    expect(JSON.stringify(schema)).toBe(before);
  });

  it('no emitted value shares an object with the schema', async () => {
    // A listener may edit what it receives; it must never be editing the
    // schema's own default or const.
    const variantDefault = { kind: 'a' };
    const itemDefault = { n: 1 };
    const constValue = { fixed: true };
    const schema: any = {
      type: 'object',
      properties: {
        v: { oneOf: [{ type: 'object', default: variantDefault, properties: { kind: { type: 'string' } } }] },
        w: { anyOf: [{ type: 'object', default: variantDefault, properties: { kind: { type: 'string' } } }] },
        c: { const: constValue },
        list: { type: 'array', items: { type: 'object', default: itemDefault, properties: { n: { type: 'number' } } } },
      },
    };
    const el: any = document.createElement('schema-form-mode');
    const shared: string[] = [];
    const schemaObjects = new Set<object>([variantDefault, itemDefault, constValue]);
    const visit = (value: unknown, path: string) => {
      if (value === null || typeof value !== 'object') return;
      if (schemaObjects.has(value as object)) shared.push(path);
      for (const [key, child] of Object.entries(value as object)) visit(child, `${path}.${key}`);
    };
    el.addEventListener('value-change', (e: CustomEvent) => visit(e.detail.value, 'value'));
    el.schema = schema;
    el.value = { list: [] };
    document.body.append(el);
    for (let i = 0; i < 4; i++) { await settle(el); await new Promise(r => setTimeout(r, 0)); }
    ([...el.querySelectorAll('button')].find((b: HTMLButtonElement) => b.textContent?.includes('Add Item')) as HTMLButtonElement).click();
    for (let i = 0; i < 4; i++) { await settle(el); await new Promise(r => setTimeout(r, 0)); }
    expect(shared).toEqual([]);
  });

  it('an object const is assigned once, as a copy, and settles', async () => {
    const schema: any = { type: 'object', properties: { c: { const: { kind: 'fixed', tags: ['t'] } } } };
    const before = JSON.stringify(schema);
    const head = await settledForm('schema-form-mode', schema, {});
    expect(head.final).toEqual({ c: { kind: 'fixed', tags: ['t'] } });
    expect(head.emitted.length).toBeLessThan(3);
    expect(head.final).toEqual((await settledForm('base-schema-form-mode', JSON.parse(before), {})).final);
    expect(JSON.stringify(schema)).toBe(before);
  });

  it('generated variant defaults settle to the same value as base', async () => {
    let seed = 220022;
    const random = () => {
      seed |= 0; seed = (seed + 0x6d2b79f5) | 0;
      let t = Math.imul(seed ^ (seed >>> 15), 1 | seed);
      t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
      return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
    };
    const pick = <T,>(items: T[]): T => items[Math.floor(random() * items.length)];
    const scalar = () => pick<any>([
      { type: 'string', default: 'hello' }, { type: 'number', default: 2 }, { type: 'boolean', default: true },
      { type: 'string' }, { type: 'integer', enum: [1, 2, 3] }, { const: 'k' }, { type: 'null' },
    ]);
    const variantSchema = (depth: number): any => {
      const keyword = pick(['oneOf', 'anyOf']);
      const branches = Array.from({ length: 1 + Math.floor(random() * 3) }, () =>
        depth > 0 && random() < 0.4 ? objectSchema(depth - 1) : scalar());
      const out: any = { [keyword]: branches };
      if (random() < 0.2) out.default = pick<any>(['d', 3, { x: 1 }]);
      return out;
    };
    const objectSchema = (depth: number): any => {
      const properties: Record<string, any> = {};
      const count = 1 + Math.floor(random() * 3);
      for (let i = 0; i < count; i++) properties[`p${i}`] = depth > 0 && random() < 0.6 ? variantSchema(depth - 1) : scalar();
      const out: any = { type: 'object', properties };
      if (random() < 0.25) out.default = { p0: 'given', extra: { keep: true } };
      return out;
    };
    const failures: string[] = [];
    for (let i = 0; i < 40; i++) {
      const schema = objectSchema(3);
      const value = pick<any>([{}, { p0: undefined }, { p1: {} }]);
      const before = JSON.stringify(schema);
      const head = await settledForm('schema-form-mode', schema, JSON.parse(JSON.stringify(value ?? {})));
      const base = await settledForm('base-schema-form-mode', JSON.parse(before), JSON.parse(JSON.stringify(value ?? {})));
      if (JSON.stringify(head.final) !== JSON.stringify(base.final)) {
        failures.push(`case ${i}\n  schema ${before}\n  head ${JSON.stringify(head.final)}\n  base ${JSON.stringify(base.final)}`);
      }
      if (JSON.stringify(schema) !== before) failures.push(`case ${i}: head changed the schema\n  ${before}\n  ${JSON.stringify(schema)}`);
    }
    expect(failures).toEqual([]);
  });
});

describe('round 23: serialisers that never throw, renames on copies', () => {
  // Node's JSON.stringify survives depths that overflow it in a browser, so
  // the overflow is simulated with a throwing toJSON.
  const tooDeep = () => {
    const value: any = { a: 1 };
    Object.defineProperty(value, 'toJSON', { enumerable: false, value: () => { throw new RangeError('Maximum call stack size exceeded'); } });
    return value;
  };

  it('valueText and inputText return a placeholder instead of throwing', () => {
    expect(() => valueText(tooDeep())).not.toThrow();
    expect(valueText(tooDeep())).toBe('(value too deep to show)');
    expect(inputText(tooDeep())).toBe('(value too deep to show)');
    expect(valueText({ a: 1 })).toBe('{"a":1}');
    expect(valueText(3)).toBe('3');
  });

  it('value equality survives a value deeper than the stack', () => {
    let a: any = 'leaf';
    let b: any = 'leaf';
    for (let i = 0; i < 50000; i++) { a = { n: a }; b = { n: b }; }
    expect(() => sameValue(a, b)).not.toThrow();
    expect(sameValue(a, a)).toBe(true);
    expect(() => valueKey(a)).not.toThrow();
  });

  it('a tree editor child shows the placeholder for a default it cannot serialise', async () => {
    const schema: any = { type: 'string', default: tooDeep() };
    const el: any = await mountEl('schema-edit-mode', { schema });
    await new Promise(resolve => setTimeout(resolve, 0));
    await settle(el);
    expect(snapshot(el)).toContain('(value too deep to show)');
  });

  it('the conditional editor shows the placeholder for a branch it cannot serialise', async () => {
    const el: any = await mountEl('schema-conditional-editor', { schema: { if: tooDeep(), then: { type: 'string' } } });
    expect(snapshot(el)).toContain('(value too deep to show)');
  });

  it('renaming a definition never changes the caller\'s schema, and the result stays consistent', async () => {
    const schema: any = {
      $defs: { Old: { type: 'string' } },
      type: 'object',
      properties: { v: { type: 'string' } },
      if: { $ref: '#/$defs/Old' },
      then: { properties: { w: { $ref: '#/$defs/Old' } } },
    };
    const original = JSON.stringify(schema);
    const el: any = await mountEl('schema-edit-mode', { schema });
    let emitted: any;
    el.addEventListener('schema-change', (e: CustomEvent) => { emitted = JSON.parse(e.detail.schema); });
    const defs = el._root.children.find((c: any) => c.name === '$defs');
    el._selectedId = defs.children.find((c: any) => c.name === 'Old').id;
    el._handleNodeChange(new CustomEvent('node-change', { detail: { field: 'name', value: 'New' } }));
    await settle(el);
    expect(JSON.stringify(schema)).toBe(original);
    expect(Object.keys(emitted.$defs)).toEqual(['New']);
    expect(emitted.if.$ref).toBe('#/$defs/New');
    expect(emitted.then.properties.w.$ref).toBe('#/$defs/New');
  });
});

describe('schema-bearing keywords by draft', () => {
  const ref = () => ({ $ref: '#/$defs/D' });
  const positions: Array<[string, string, () => any]> = [
    ['draft-04', 'properties', () => ({ properties: { p: ref() } })],
    ['draft-04', 'patternProperties', () => ({ patternProperties: { '^x': ref() } })],
    ['draft-04', 'additionalProperties', () => ({ additionalProperties: ref() })],
    ['draft-04', 'items (schema)', () => ({ items: ref() })],
    ['draft-04', 'items (array)', () => ({ items: [ref()] })],
    ['draft-04', 'additionalItems', () => ({ additionalItems: ref() })],
    ['draft-04', 'dependencies (schema)', () => ({ dependencies: { x: ref(), y: ['a', 'b'] } })],
    ['draft-04', 'allOf', () => ({ allOf: [ref()] })],
    ['draft-04', 'anyOf', () => ({ anyOf: [ref()] })],
    ['draft-04', 'oneOf', () => ({ oneOf: [ref()] })],
    ['draft-04', 'not', () => ({ not: ref() })],
    ['draft-04', 'definitions', () => ({ definitions: { X: ref() } })],
    ['draft-06', 'contains', () => ({ contains: ref() })],
    ['draft-06', 'propertyNames', () => ({ propertyNames: ref() })],
    ['draft-07', 'if', () => ({ if: ref() })],
    ['draft-07', 'then', () => ({ then: ref() })],
    ['draft-07', 'else', () => ({ else: ref() })],
    ['2019-09', 'dependentSchemas', () => ({ dependentSchemas: { x: ref() } })],
    ['2019-09', 'unevaluatedItems', () => ({ unevaluatedItems: ref() })],
    ['2019-09', 'unevaluatedProperties', () => ({ unevaluatedProperties: ref() })],
    ['2019-09', 'contentSchema', () => ({ contentSchema: ref() })],
    ['2019-09', '$defs', () => ({ $defs: { X: ref() } })],
    ['2020-12', 'prefixItems', () => ({ prefixItems: [ref()] })],
  ];
  for (const [draft, keyword, build] of positions) {
    it(`${draft}: ${keyword} is visited`, () => {
      const refs: string[] = [];
      visitSubschemas(build(), s => { if (typeof s.$ref === 'string') refs.push(s.$ref); });
      expect(refs).toEqual(['#/$defs/D']);
    });
  }

  it('annotation and data keywords are not visited', () => {
    const refs: string[] = [];
    visitSubschemas({ examples: [ref()], default: ref(), const: ref(), enum: [ref()], 'x-meta': ref(), dependencies: { y: ['$ref'] } }, s => {
      if (typeof s.$ref === 'string') refs.push(s.$ref);
    });
    expect(refs).toEqual([]);
  });

  it('renaming a definition updates a draft-07 dependencies reference', async () => {
    const schema = { type: 'object', properties: { x: { type: 'string' } }, dependencies: { x: { $ref: '#/$defs/D' }, y: ['x'] }, $defs: { D: { type: 'object' } } };
    const el = await mountEl('schema-edit-mode', { schema });
    const find = (node: any): any => node.isDef && node.name === 'D' ? node : [...(node.children ?? []), ...(node.variants ?? [])].map(find).find(Boolean);
    el._selectedId = find(el._root).id;
    el._handleNodeChange(new CustomEvent('node-change', { detail: { field: 'name', value: 'E' } }));
    const out = treeToSchema(el._root);
    expect(out.dependencies.x.$ref).toBe('#/$defs/E');
    expect(out.dependencies.y).toEqual(['x']);
  });
});

describe('large acyclic schemas behave as at base', () => {
  it('34 nested allOf layers still evaluate the gate', () => {
    let gate: any = { properties: { gate: { const: true } } };
    for (let i = 0; i < 34; i++) gate = { allOf: [gate] };
    expect(evaluateCondition(gate, { gate: false }, {})).toBe(false);
    expect(evaluateCondition(gate, { gate: true }, {})).toBe(true);
  });

  it('34 nested allOf layers in if keep else-only data in the form', async () => {
    let gate: any = { properties: { gate: { const: true } } };
    for (let i = 0; i < 34; i++) gate = { allOf: [gate] };
    const schema = {
      type: 'object', properties: { gate: { type: 'boolean' } }, if: gate,
      then: { properties: { thenOnly: { type: 'string' } } },
      else: { properties: { elseOnly: { type: 'string' } } },
    };
    const el = await mountEl('schema-form-mode', { schema, value: { gate: false, elseOnly: 'keep' } });
    expect([...el.querySelectorAll<HTMLInputElement>('input[type="text"]')].map(i => i.value)).toContain('keep');
  });

  it('10,002 properties all get defaults', () => {
    const properties: Record<string, any> = {};
    for (let i = 0; i < 10002; i++) properties[`p${i}`] = { type: 'string', default: 'd' };
    const schema = { type: 'object', properties };
    const defined = (o: object) => Object.values(o).filter(v => v !== undefined);
    expect(defined(getDefaultValue(schema, schema))).toHaveLength(10002);
    expect(defined(getPreviewDefaultValue(schema, schema))).toHaveLength(10002);
  });

  it('a 65-deep chain of nested objects still renders its input', async () => {
    let schema: any = { type: 'string' };
    let value: any = 'deep';
    for (let i = 0; i < 65; i++) { schema = { type: 'object', properties: { n: schema } }; value = { n: value }; }
    const el = await mountEl('schema-form-mode', { schema, value });
    expect([...el.querySelectorAll<HTMLInputElement>('input[type="text"]')].map(i => i.value)).toEqual(['deep']);
    expect(el.textContent).not.toContain('Unresolvable reference');
  });

  it('a recursive schema follows stored data as deep as it goes', async () => {
    const defs = { node: { type: 'object', properties: { name: { type: 'string' }, child: { $ref: '#/$defs/node' } } } };
    const schema = { $defs: defs, $ref: '#/$defs/node' };
    const value = { name: 'n0', child: { name: 'n1', child: { name: 'n2' } } };
    const el = await mountEl('schema-form-mode', { schema, value });
    const names = [...el.querySelectorAll<HTMLInputElement>('input[type="text"]')].map(i => i.value);
    // Each stored level renders, and only those: the form does not expand an
    // absent optional object.
    expect(names).toEqual(['n0', 'n1', 'n2']);
    // Defaults have no data to follow: the nested {$ref} step expands once
    // and its repeat is left unresolved.
    expect(getDefaultValue(schema, schema)).toEqual({ name: '', child: { name: '' } });
    // The condition walker also descends into the data: the nested name fails.
    const cond = { $defs: { node: { properties: { name: { const: 'x' }, child: { $ref: '#/$defs/node' } } } }, $ref: '#/$defs/node' };
    expect(evaluateCondition(cond, { name: 'x', child: { name: 'y' } }, cond)).toBe(false);
    expect(evaluateCondition(cond, { name: 'x', child: { name: 'x' } }, cond)).toBe(true);
    // Reaching the nested level through allOf meets "node" again while it is
    // still on the outer path; descending into the stored value starts fresh.
    const viaAllOf = { $defs: { node: { properties: { name: { const: 'x' }, child: { allOf: [{ $ref: '#/$defs/node' }] } } } }, $ref: '#/$defs/node' };
    expect(evaluateCondition(viaAllOf, { name: 'x', child: { name: 'y' } }, viaAllOf)).toBe(false);
  });

  it('a recursive schema through array items follows the stored items', async () => {
    const defs = { node: { type: 'object', properties: { name: { type: 'string' }, kids: { type: 'array', items: { $ref: '#/$defs/node' } } } } };
    const schema = { $defs: defs, $ref: '#/$defs/node' };
    const value = { name: 'root', kids: [{ name: 'k1', kids: [{ name: 'k2' }] }] };
    const el = await mountEl('schema-form-mode', { schema, value });
    expect([...el.querySelectorAll<HTMLInputElement>('input[type="text"]')].map(i => i.value)).toEqual(['root', 'k1', 'k2']);
  });

  it('an acyclic diamond of references finishes quickly: correct, or too complex', () => {
    // Each level refers to the next twice, so base's merge semantics double
    // the branch list at every level; the work budget stops it visibly.
    const $defs: Record<string, any> = {};
    for (let i = 0; i < 30; i++) $defs[`a${i}`] = { allOf: [{ $ref: `#/$defs/a${i + 1}` }, { $ref: `#/$defs/a${i + 1}` }] };
    $defs.a30 = { properties: { leaf: { type: 'string', default: 'L' } } };
    const root = { $defs, type: 'object', properties: { v: { $ref: '#/$defs/a0' } } } as any;
    const correctOrTooComplex = (run: () => void) => {
      try { run(); } catch (error) { if (!(error instanceof SchemaTooComplexError)) throw error; }
    };
    const started = Date.now();
    correctOrTooComplex(() => expect(resolveSchema({ $ref: '#/$defs/a0' }, root)!.properties.leaf.type).toBe('string'));
    correctOrTooComplex(() => expect(getDefaultValue({ $ref: '#/$defs/a0' }, root)).toEqual({ leaf: 'L' }));
    correctOrTooComplex(() => isLeafSchema({ $ref: '#/$defs/a0' }, root));
    expect(Date.now() - started).toBeLessThan(2000);
  });
});

describe('pills with hand-written options', () => {
  it('ignores non-scalar option values and labels, and unsafe colors', async () => {
    const el = await mountEl('meta-shortcode', {}, {
      'data-path': 'v', 'data-pills': 'true', 'data-value': JSON.stringify(H),
      'data-options': JSON.stringify([{ value: H, label: 'x' }, { value: 'a', label: H }, H, OBJ, STR, { value: 'b', label: 'B', color: H }]),
    });
    expectClean(el);
    // An invalid entry rejects the whole manual list (pillOptions), by design.
    expect(el.querySelectorAll('[role="radio"]')).toHaveLength(0);
    expect(el.textContent).toContain('No options configured');
  });

  it('keeps scalar look-alikes apart and selects by typed value', async () => {
    const el = await mountEl('meta-shortcode', {}, {
      'data-path': 'v', 'data-pills': 'true', 'data-value': '1',
      'data-options': JSON.stringify(['1', 1, { value: STR, label: STR, color: 'var(--status_color)' }]),
    });
    expectClean(el);
    const radios = [...el.querySelectorAll('[role="radio"]')];
    expect(radios.map(r => r.getAttribute('aria-checked'))).toEqual(['false', 'true', 'false']);
  });
});

// ─── Matrix: every hostile value through every entry point ──────────────────

const props: any[] = [
  { type: 'string', default: H, const: undefined },
  { type: 'number', default: H, minimum: H },
  { type: 'string', minLength: H, pattern: H, format: H },
  { enum: [H, OBJ, STR] },
  { oneOf: [{ const: H, title: 'H' }, { const: OBJ }, { const: STR }] },
  { type: 'object', properties: { x: { type: 'string' } }, default: H },
  { type: 'array', items: { type: 'string' }, default: H },
  { oneOf: [{ const: 'a', title: 'A', 'x-color': H }], anyOf: [{ 'x-display': 'geo' }], title: 'T' },
  { type: 'string', format: 'date-time' },
  { type: 'string', format: 'date' },
  { type: 'object', 'x-display': 'daterange' },
  { allOf: [{ enum: [H, 'a'] }], 'x-display': 'dimensions' },
  { type: 'string', title: H, description: H },
  { type: 'number', if: { minimum: H, maximum: H, const: H, enum: [H] }, then: { title: 'T' }, else: { description: 'D' } },
  { type: 'object', properties: { x: { type: 'number', title: H } }, if: { properties: { x: { minimum: H, pattern: H } } }, then: { properties: { y: { type: 'string', default: H } } } },
  { $ref: H },
  { oneOf: [{ const: 'a', title: H }, { const: 'b', title: 'B' }] },
  { oneOf: [{ type: 'string', title: H }, { type: 'object', properties: { k: { type: 'string' } } }] },
  { anyOf: [{ type: 'string', title: H, description: H }, { type: 'number', minimum: H }] },
  { type: 'string', format: 'date', default: '2024-02-30' },
].map(p => JSON.parse(JSON.stringify(p)));
const values: any[] = [H, OBJ, STR, '2024-02-30T00:00:00Z', [H, null, OBJ], 'plain', 'a', 5, { start: '2024-02-01', end: '2024-02-30' }];

describe('matrix', () => {
  for (const [pi, prop] of props.entries()) {
    it(`property schema #${pi} renders every hostile value cleanly`, async () => {
      for (const value of values) {
        const schema = { type: 'object', properties: { v: prop } };
        const elements = [
          await mountEl('schema-display-mode', { schema, value: { v: value } }),
          await mountEl('schema-form-mode', { schema, value: { v: value } }),
          await mountEl('schema-search-mode', { schema: JSON.stringify(schema), metaQuery: JSON.stringify([{ name: 'v', value, operation: 'EQ' }]) }),
          await mountEl('meta-shortcode', {}, { 'data-path': 'v', 'data-schema': JSON.stringify(prop), 'data-value': JSON.stringify(value) }),
          await mountEl('meta-shortcode', {}, { 'data-path': 'v', 'data-pills': 'true', 'data-schema': JSON.stringify(prop), 'data-value': JSON.stringify(value) }),
          await mountEl('meta-shortcode', {}, { 'data-path': 'v', 'data-date-time': 'true', 'data-schema': JSON.stringify(prop), 'data-value': JSON.stringify(value) }),
          await mountEl('schema-detail-panel', { node: schemaToTree(prop, 'v') }),
        ];
        // [meta editable] opens the form inside <schema-editor>.
        const editable = await mountEl('meta-shortcode', {}, { 'data-path': 'v', 'data-editable': 'true', 'data-entity-type': 'group', 'data-entity-id': '1', 'data-schema': JSON.stringify(prop), 'data-value': JSON.stringify(value) });
        editable.querySelector<HTMLButtonElement>('button[aria-label^="Edit"]')?.click();
        await settle(editable);
        elements.push(editable);
        const table = document.createElement('div');
        table.append(renderJsonTable({ v: value, nested: { a: { b: { c: [value, null, OBJ] } } } }));
        document.body.append(table);
        elements.push(table);
        for (const el of elements) expectClean(el);
        document.body.replaceChildren();
      }
    });
  }
});
