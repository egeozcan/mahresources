// @vitest-environment happy-dom
// The schema editors must never coerce a raw non-primitive schema const, enum
// entry, default or stored value: String() and Lit's attribute and .value
// coercion throw on a parsed {"toString":null}, and turn every other object
// into the same "[object Object]".
import { afterEach, describe, expect, it } from 'vitest';
import './tree/node-editors/enum-editor';
import './tree/node-editors/boolean-editor';
import './modes/form-mode';
import './modes/search-mode';

afterEach(() => {
  document.body.replaceChildren();
});

const hostile = { toString: null };

async function mountEl(tag: string, props: Record<string, any>) {
  const el = document.createElement(tag) as any;
  Object.assign(el, props);
  document.body.append(el);
  await el.updateComplete;
  return el;
}

function inputs(el: any): HTMLInputElement[] {
  return [...(el.shadowRoot ?? el).querySelectorAll('input')];
}

describe('tree enum editor', () => {
  it('renders plain object values as distinct JSON', async () => {
    const el = await mountEl('schema-enum-editor', { values: [hostile, { a: 1 }, { b: 2 }] });
    expect(inputs(el).map(i => i.value)).toEqual(['{"toString":null}', '{"a":1}', '{"b":2}']);
    const removes = [...el.shadowRoot.querySelectorAll('button.remove')].map((b: Element) => b.getAttribute('aria-label'));
    expect(removes).toEqual(['Remove value {"toString":null}', 'Remove value {"a":1}', 'Remove value {"b":2}']);
  });

  it('keeps ordinary object values apart instead of "[object Object]"', async () => {
    const plain = await mountEl('schema-enum-editor', { values: [{ a: 1 }, { b: 2 }] });
    expect(inputs(plain).map(i => i.value)).toEqual(['{"a":1}', '{"b":2}']);
    const labeled = await mountEl('schema-enum-editor', { labeled: true, entries: [{ value: { a: 1 }, label: '' }, { value: { b: 2 }, label: '' }] });
    const values = inputs(labeled).filter(i => i.getAttribute('aria-label')?.startsWith('Enum value')).map(i => i.value);
    expect(values).toEqual(['{"a":1}', '{"b":2}']);
  });

  it('renders labeled object values, with and without a color, as distinct JSON', async () => {
    const el = await mountEl('schema-enum-editor', {
      labeled: true,
      entries: [{ value: hostile, label: 'X', color: '#abcdef' }, { value: { a: 1 }, label: '' }],
    });
    const valueInputs = inputs(el).filter(i => i.getAttribute('aria-label')?.startsWith('Enum value'));
    expect(valueInputs.map(i => i.value)).toEqual(['{"toString":null}', '{"a":1}']);
    const labels = [...el.shadowRoot.querySelectorAll('[aria-label]')].map((n: Element) => n.getAttribute('aria-label'));
    expect(labels).toContain('Color for value {"toString":null}');
    expect(labels).toContain('Add color for value {"a":1}');
    expect(labels).not.toContain('Remove value [object Object]');
  });
});

describe('tree boolean editor', () => {
  it('renders a non-boolean default and const without coercing them', async () => {
    const el = await mountEl('schema-boolean-editor', { schema: { type: 'boolean', default: hostile, const: hostile } });
    expect(el.shadowRoot.querySelectorAll('select')).toHaveLength(2);
  });
});

describe('form inputs', () => {
  async function form(prop: any, value: any) {
    return mountEl('schema-form-mode', { schema: { type: 'object', properties: { v: prop } }, value });
  }

  it('renders an object const as JSON', async () => {
    const el = await form({ const: hostile }, {});
    const input = el.querySelector('input[aria-label="Constant value"]') as HTMLInputElement;
    expect(input.value).toBe('{"toString":null}');
  });

  it('renders a stored object under a string or number field without throwing', async () => {
    for (const prop of [{ type: 'string' }, { type: 'number' }, { type: 'integer' }]) {
      const el = await form(prop, { v: hostile });
      expect(el.querySelector('input')).not.toBeNull();
    }
  });
});

describe('search fields', () => {
  it('renders object enum values and labeled consts without coercing them', async () => {
    for (const prop of [
      { enum: [hostile, 'a'] },
      { oneOf: [{ const: hostile, title: 'Obj' }, { const: 'a', title: 'A' }] },
    ]) {
      const el = await mountEl('schema-search-mode', {
        schema: JSON.stringify({ type: 'object', properties: { v: prop } }),
        metaQuery: JSON.stringify([{ name: 'v', value: hostile, operation: 'EQ' }]),
      });
      expect(el.textContent).toContain('V');
    }
  });
});
