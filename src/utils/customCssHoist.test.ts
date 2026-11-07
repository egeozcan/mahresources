// @vitest-environment happy-dom
import { afterEach, describe, expect, it } from 'vitest';
import { hoistCustomCSS, startCustomCSSHoist } from './customCssHoist.js';

const block = (key: string, css: string) => `<style data-mr-custom-css="${key}">${css}</style>`;
const headBlocks = () => [...document.head.querySelectorAll('style[data-mr-custom-css]')].map((s) => `${s.getAttribute('data-mr-custom-css')}|${s.textContent}`);

afterEach(() => {
  document.head.innerHTML = '';
  document.body.innerHTML = '';
});

describe('hoistCustomCSS', () => {
  it('moves category blocks into the head, so replacing their container keeps them', () => {
    document.body.innerHTML = `<div class="list-container">${block('group:3', '.a{}')}</div>`;
    hoistCustomCSS(document.body);
    expect(headBlocks()).toEqual(['group:3|.a{}']);
    document.querySelector('.list-container')!.innerHTML = '';
    expect(headBlocks()).toEqual(['group:3|.a{}']);
  });

  it('drops a copy the head already holds, and keeps a block with different rules', () => {
    document.head.innerHTML = block('group:3', '.a{}');
    document.body.innerHTML = block('group:3', '.a{}') + block('group:3', '.b{}');
    hoistCustomCSS(document.body);
    expect(headBlocks()).toEqual(['group:3|.a{}', 'group:3|.b{}']);
    expect(document.body.querySelectorAll('style').length).toBe(0);
  });

  it('leaves other style elements, the lightbox copy included, where they are', () => {
    document.body.innerHTML = '<style data-mr-lightbox-css>.l{}</style><style>.x{}</style>';
    hoistCustomCSS(document.body);
    expect(document.body.querySelectorAll('style').length).toBe(2);
    expect(headBlocks()).toEqual([]);
  });
});

describe('startCustomCSSHoist', () => {
  it('hoists what is on the page and what arrives later', async () => {
    document.body.innerHTML = block('note:1', '.n{}');
    const observer = startCustomCSSHoist(document);
    const region = document.createElement('div');
    region.innerHTML = `<p>x</p>${block('resource:2', '.r{}')}`;
    document.body.appendChild(region);
    await new Promise((r) => setTimeout(r, 0));
    observer.disconnect();
    expect(headBlocks()).toEqual(['note:1|.n{}', 'resource:2|.r{}']);
    region.remove();
    expect(headBlocks()).toEqual(['note:1|.n{}', 'resource:2|.r{}']);
  });
});
