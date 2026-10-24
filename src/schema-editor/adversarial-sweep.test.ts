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
import './modes/search-mode';
import './schema-editor';
import './tree/detail-panel';
import './modes/edit-mode';
import '../webcomponents/meta-shortcode';
import { flattenSchema, intersectFields, resolveSchema, safeColor, getLabeledEnumEntries, withTypedKeywords, sameValue, mergeSchemas, getDefaultValue, getPreviewDefaultValue, evaluateCondition, scoreSchemaMatch } from './schema-core';
import { isLeafSchema, stripStaleKeys } from './form-mode-helpers';
import { schemaToTree } from './schema-tree-model';
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
    expect(el.textContent).toContain('3 \u00D7 4');
    expect(Date.now() - started).toBeLessThan(1000);
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
    // Defaults have no data to follow, so the repeated reference is unresolved.
    expect(getDefaultValue(schema, schema)).toEqual({ name: '', child: undefined });
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

  it('an acyclic diamond of references expands linearly', () => {
    // Each level refers to the next twice: without merging identical allOf
    // branches once, the branch list doubles at every level.
    const $defs: Record<string, any> = {};
    for (let i = 0; i < 30; i++) $defs[`a${i}`] = { allOf: [{ $ref: `#/$defs/a${i + 1}` }, { $ref: `#/$defs/a${i + 1}` }] };
    $defs.a30 = { properties: { leaf: { type: 'string', default: 'L' } } };
    const root = { $defs, type: 'object', properties: { v: { $ref: '#/$defs/a0' } } } as any;
    const started = Date.now();
    expect(resolveSchema({ $ref: '#/$defs/a0' }, root)!.properties.leaf.type).toBe('string');
    expect(getDefaultValue({ $ref: '#/$defs/a0' }, root)).toEqual({ leaf: 'L' });
    isLeafSchema({ $ref: '#/$defs/a0' }, root);
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
