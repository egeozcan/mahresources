// @vitest-environment happy-dom
// Enum <select> options must never coerce a raw non-primitive value: a parsed
// {"toString":null} throws, and ordinary objects all become "[object Object]".
import { afterEach, describe, expect, it } from 'vitest';
import './form-mode';
import '../tree/detail-panel';
import { schemaToTree } from '../schema-tree-model';

afterEach(() => {
  document.body.replaceChildren();
});

async function mount(prop: any, value: any) {
  const el = document.createElement('schema-form-mode') as any;
  el.schema = { type: 'object', properties: { v: prop } };
  el.value = value;
  document.body.append(el);
  await el.updateComplete;
  return el as HTMLElement;
}

function select(el: HTMLElement) {
  const s = el.querySelector('select');
  if (!s) throw new Error('no select rendered');
  return s;
}

async function choose(el: HTMLElement, text: string) {
  const s = select(el);
  const option = [...s.options].find(o => o.textContent?.trim() === text);
  if (!option) throw new Error(`no option ${text}`);
  let emitted: any;
  el.addEventListener('value-change', e => { emitted = (e as CustomEvent).detail.value; }, { once: true });
  s.value = option.value;
  s.dispatchEvent(new Event('change'));
  return emitted?.v;
}

describe('form enum options', () => {
  it('renders a labeled enum whose const is a hostile object', async () => {
    const el = await mount({ oneOf: [{ const: { toString: null }, title: 'Obj' }, { const: 'a', title: 'A' }] }, {});
    expect([...select(el).options].map(o => o.textContent?.trim())).toContain('Obj');
  });

  it('shows a hostile stored value as the current option of a scalar enum', async () => {
    const el = await mount({ enum: ['a', 'b'] }, { v: { toString: null } });
    expect(select(el).textContent).toContain('{"toString":null} (current)');
  });

  it('shows a hostile stored value as the current option of a labeled enum', async () => {
    const el = await mount({ oneOf: [{ const: 'a', title: 'A' }] }, { v: { toString: null } });
    expect(select(el).textContent).toContain('{"toString":null} (current)');
  });

  it('keeps distinct object consts distinct and maps the selection back', async () => {
    const el = await mount({ oneOf: [{ const: { a: 1 }, title: 'One' }, { const: { b: 2 }, title: 'Two' }] }, {});
    const values = [...select(el).options].map(o => o.value).filter(v => v !== '');
    expect(new Set(values).size).toBe(values.length);
    expect(await choose(el, 'Two')).toEqual({ b: 2 });
  });

  it('keeps distinct object enum values distinct and maps the selection back', async () => {
    const el = await mount({ enum: [{ a: 1 }, { b: 2 }] }, {});
    const values = [...select(el).options].map(o => o.value).filter(v => v !== '');
    expect(new Set(values).size).toBe(values.length);
    expect(await choose(el, '{"b":2}')).toEqual({ b: 2 });
  });

  it('uses the choice text as the option value when every text is distinct', async () => {
    const str = await mount({ enum: ['a', 'b'] }, {});
    expect([...select(str).options].map(o => o.value)).toEqual(['', 'a', 'b']);
    expect(await choose(str, 'b')).toBe('b');
    const num = await mount({ type: 'integer', oneOf: [{ const: 1, title: 'One' }, { const: 2, title: 'Two' }] }, {});
    expect([...select(num).options].map(o => o.value)).toEqual(['', '1', '2']);
    expect(await choose(num, 'Two')).toBe(2);
    const nums = await mount({ type: 'integer', enum: [1, 2] }, {});
    expect(await choose(nums, '2')).toBe(2);
  });

  it('falls back to position keys when choice texts collide, and maps back to the typed value', async () => {
    const numStr = await mount({ enum: [1, '1'] }, {});
    expect([...select(numStr).options].map(o => o.value)).toEqual(['', '0', '1']);
    const pick = (key: string) => {
      let emitted: any;
      numStr.addEventListener('value-change', e => { emitted = (e as CustomEvent).detail.value; }, { once: true });
      const s = select(numStr);
      s.value = key;
      s.dispatchEvent(new Event('change'));
      return emitted?.v;
    };
    expect(pick('1')).toBe('1');
    expect(pick('0')).toBe(1);
    const objStr = await mount({ enum: [{ a: 1 }, '{"a":1}'] }, {});
    expect([...select(objStr).options].map(o => o.value)).toEqual(['', '0', '1']);
  });

  it('falls back to position keys for an empty or reserved text', async () => {
    const empty = await mount({ enum: ['', 'a'] }, {});
    expect([...select(empty).options].map(o => o.value)).toEqual(['', '0', '1']);
    const reserved = await mount({ enum: ['current', 'a'] }, {});
    expect([...select(reserved).options].map(o => o.value)).toEqual(['', '0', '1']);
    expect(await choose(reserved, 'current')).toBe('current');
  });
});

describe('form enum current option', () => {
  it('re-selecting the current option keeps the stored value, not its text', async () => {
    const plain = await mount({ enum: ['a', 'b'] }, { v: { x: 1 } });
    expect(await choose(plain, '{"x":1} (current)')).toEqual({ x: 1 });
    const labeled = await mount({ oneOf: [{ const: 'a', title: 'A' }] }, { v: { x: 1 } });
    expect(await choose(labeled, '{"x":1} (current)')).toEqual({ x: 1 });
  });
});

describe('tree editor x-color', () => {
  it('ignores a hostile x-color on a labeled enum entry', async () => {
    const el = document.createElement('schema-detail-panel') as any;
    el.node = schemaToTree({ type: 'string', oneOf: [{ const: 'a', title: 'A', 'x-color': { toString: null } }] }, 'v');
    document.body.append(el);
    await el.updateComplete;
    const editor = el.shadowRoot?.querySelector('schema-enum-editor') ?? el.querySelector('schema-enum-editor');
    expect(editor).not.toBeNull();
    await editor.updateComplete;
    expect(editor.entries[0].color).toBeUndefined();
  });
});
