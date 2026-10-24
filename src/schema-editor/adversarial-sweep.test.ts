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
import '../webcomponents/meta-shortcode';
import { flattenSchema, intersectFields, resolveSchema, safeColor, getLabeledEnumEntries } from './schema-core';
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
