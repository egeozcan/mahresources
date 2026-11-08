// @vitest-environment happy-dom
// The hoist takes category <style> blocks out of the live list, but a refreshed list arrives
// with them still in place. Morph matches children by position, so those extra siblings would
// make it replace unchanged cards (losing their inputs and focus) on every refresh. The morph
// seam hoists the incoming blocks first, so both sides have the same shape.
import { afterEach, beforeAll, describe, expect, it } from 'vitest';
import Alpine from 'alpinejs';
import morph from '@alpinejs/morph';
import { morphAndReinitChangedComponents } from './shortcodeElementMorph.js';
import { hoistCustomCSS } from './customCssHoist.js';

beforeAll(() => {
  Alpine.plugin(morph);
  (window as any).Alpine = Alpine;
});

afterEach(() => {
  document.head.innerHTML = '';
  document.body.innerHTML = '';
});

const listHTML = (value: string) =>
  `<style data-mr-custom-css="resource:4">.c{color:red}</style><article id="a"><input name="note" value="${value}"></article>` +
  `<style data-mr-custom-css="resource:5">.d{color:blue}</style><article id="b"><input name="note2" value="${value}"></article>`;

describe('morphing a refreshed list after the hoist', () => {
  it('keeps unchanged cards, their unsaved input and focus, and the stylesheets', () => {
    document.body.innerHTML = `<div class="list-container">${listHTML('server')}</div>`;
    hoistCustomCSS(document.body);
    const live = document.querySelector('.list-container') as HTMLElement;
    const card = document.getElementById('a');
    const input = card!.querySelector('input') as HTMLInputElement;
    input.value = 'unsaved';
    input.focus();

    const incoming = document.createElement('div');
    incoming.className = 'list-container';
    incoming.innerHTML = listHTML('server');
    morphAndReinitChangedComponents(live, incoming);

    expect(document.getElementById('a')).toBe(card);
    expect(input.value).toBe('unsaved');
    expect(document.activeElement).toBe(input);
    expect(live.querySelectorAll('style').length).toBe(0);
    expect(document.head.querySelectorAll('style[data-mr-custom-css]').length).toBe(2);
  });
});
