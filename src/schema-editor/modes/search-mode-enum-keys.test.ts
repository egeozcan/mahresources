// @vitest-environment happy-dom
// Search-mode enum controls: the DOM value is the choice's text while every
// choice has a distinct text, else its position; a selection maps back to the
// typed choice either way, and the hidden-input encoding is unchanged.
import { afterEach, describe, expect, it } from 'vitest';
import './search-mode';
import { choiceKeys, choiceIndexOf } from '../schema-core';

afterEach(() => {
  document.body.replaceChildren();
});

async function search(prop: any) {
  const el = document.createElement('schema-search-mode') as any;
  el.schema = JSON.stringify({ type: 'object', properties: { v: prop } });
  el.metaQuery = '[]';
  document.body.append(el);
  await el.updateComplete;
  return el as HTMLElement;
}

async function settle(el: HTMLElement) {
  await (el as any).updateComplete;
  await new Promise(resolve => setTimeout(resolve, 0));
  await (el as any).updateComplete;
}

function hidden(el: HTMLElement) {
  return [...el.querySelectorAll<HTMLInputElement>('input[type="hidden"]')].map(i => i.value);
}

describe('choiceKeys', () => {
  it('uses text for distinct primitives and objects', () => {
    expect(choiceKeys(['US', 'UK'])).toEqual(['US', 'UK']);
    expect(choiceKeys([1, 2])).toEqual(['1', '2']);
    expect(choiceKeys([{ a: 1 }, 'x'])).toEqual(['{"a":1}', 'x']);
  });
  it('uses positions when texts collide, are empty, or are reserved', () => {
    expect(choiceKeys([1, '1'])).toEqual(['0', '1']);
    expect(choiceKeys([{ a: 1 }, '{"a":1}'])).toEqual(['0', '1']);
    expect(choiceKeys(['', 'a'])).toEqual(['0', '1']);
    expect(choiceKeys(['current', 'a'], ['current'])).toEqual(['0', '1']);
  });
  it('maps a key back to its index', () => {
    expect(choiceIndexOf(['US', 'UK'], 'UK')).toBe(1);
    expect(choiceIndexOf(['0', '1'], '1')).toBe(1);
    expect(choiceIndexOf(['US', 'UK'], 'FR')).toBe(-1);
  });
});

describe('search mode enum keys', () => {
  it('checkboxes carry the choice text and check the typed choice', async () => {
    const el = await search({ enum: ['US', 'UK'] });
    const boxes = [...el.querySelectorAll<HTMLInputElement>('input[type="checkbox"]')];
    expect(boxes.map(b => b.value)).toEqual(['US', 'UK']);
    boxes[1].checked = true;
    boxes[1].dispatchEvent(new Event('change'));
    await settle(el);
    expect(hidden(el)).toEqual(['v:EQ:"UK"']);
  });

  it('checkboxes of colliding choices carry positions', async () => {
    const el = await search({ enum: [1, '1'] });
    const boxes = [...el.querySelectorAll<HTMLInputElement>('input[type="checkbox"]')];
    expect(boxes.map(b => b.value)).toEqual(['0', '1']);
  });

  it('select options carry text and map the selection back to the typed value', async () => {
    const el = await search({ enum: [10, 20, 30, 40, 50, 60, 70] });
    const s = el.querySelector<HTMLSelectElement>('select[multiple]')!;
    expect([...s.options].map(o => o.value).slice(0, 3)).toEqual(['10', '20', '30']);
    s.options[1].selected = true;
    s.dispatchEvent(new Event('change'));
    await settle(el);
    expect(hidden(el)).toEqual(['v:EQ:20']);
  });

  it('select options of colliding choices carry positions and map back', async () => {
    const el = await search({ enum: [1, '1', 'c', 'd', 'e', 'f', 'g'] });
    const s = el.querySelector<HTMLSelectElement>('select[multiple]')!;
    expect([...s.options].map(o => o.value).slice(0, 3)).toEqual(['0', '1', '2']);
    s.options[1].selected = true;
    s.dispatchEvent(new Event('change'));
    await settle(el);
    expect(hidden(el)).toEqual(['v:EQ:"1"']);
  });
});
