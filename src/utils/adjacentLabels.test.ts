// @vitest-environment happy-dom

// A fragment inserted after the page loaded (a deferred body, a hover card, a
// re-rendered text block) is rendered by its own request, so an [adjacent]
// inside it can repeat a landmark name the page already holds.
import { afterEach, describe, expect, it } from 'vitest';
import { startAdjacentLabels, uniqueAdjacentLabels } from './adjacentLabels.js';

afterEach(() => {
  document.body.innerHTML = '';
});

const nav = (label: string) => `<nav class="shortcode-adjacent" aria-label="${label}"><a href="/note?id=1">x</a></nav>`;
const labels = () => [...document.querySelectorAll('nav.shortcode-adjacent')].map((n) => n.getAttribute('aria-label'));

describe('uniqueAdjacentLabels', () => {
  it('numbers an inserted nav whose name the page already holds', () => {
    document.body.innerHTML = nav('Previous and next') + nav('Previous and next (2)') + '<div id="lazy"></div>';
    const lazy = document.getElementById('lazy')!;
    lazy.innerHTML = nav('Previous and next') + nav('Previous and next (2)') + nav('Chapters');
    uniqueAdjacentLabels(lazy);
    expect(labels()).toEqual([
      'Previous and next',
      'Previous and next (2)',
      'Previous and next (3)',
      'Previous and next (4)',
      'Chapters',
    ]);
  });

  it('leaves names that are already unique alone, including on a re-render', () => {
    document.body.innerHTML = nav('Previous and next') + '<div id="region">' + nav('Chapters') + '</div>';
    const region = document.getElementById('region')!;
    region.innerHTML = nav('Chapters');
    uniqueAdjacentLabels(region);
    expect(labels()).toEqual(['Previous and next', 'Chapters']);
  });

  it('treats names that differ only in whitespace as one', () => {
    document.body.innerHTML = nav(' Meetings ') + '<div id="card"></div>';
    const card = document.getElementById('card')!;
    card.innerHTML = nav('Meetings');
    uniqueAdjacentLabels(card);
    expect(labels()).toEqual([' Meetings ', 'Meetings (2)']);
  });

  it('handles the nav itself being the inserted node', () => {
    document.body.innerHTML = nav('Chapters');
    const tpl = document.createElement('template');
    tpl.innerHTML = nav('Chapters');
    const added = tpl.content.firstElementChild!;
    document.body.appendChild(added);
    uniqueAdjacentLabels(added);
    expect(labels()).toEqual(['Chapters', 'Chapters (2)']);
  });
});

describe('collisions with other navigation landmarks', () => {
  it('numbers an [adjacent] named like the page menu, on load and on insertion', async () => {
    document.body.innerHTML = '<nav aria-label="Main"></nav>' + nav('Main') + nav('Main (2)') + '<div id="slot"></div>';
    const observer = startAdjacentLabels();
    try {
      expect(labels()).toEqual(['Main (2)', 'Main (3)']);
      document.getElementById('slot')!.innerHTML = '<nav aria-label="Pages"></nav>' + nav('Pages');
      await new Promise((resolve) => setTimeout(resolve, 0));
      expect(labels()).toEqual(['Main (2)', 'Main (3)', 'Pages (2)']);
    } finally {
      observer.disconnect();
    }
  });
});

describe('startAdjacentLabels', () => {
  it('renumbers a nav that any insertion path adds, such as a hover card', async () => {
    document.body.innerHTML = nav('Previous and next') + '<div id="popover"></div>';
    const observer = startAdjacentLabels();
    try {
      document.getElementById('popover')!.innerHTML = nav('Previous and next');
      await new Promise((resolve) => setTimeout(resolve, 0));
      expect(labels()).toEqual(['Previous and next', 'Previous and next (2)']);
    } finally {
      observer.disconnect();
    }
  });
});
