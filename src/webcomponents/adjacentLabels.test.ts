// @vitest-environment happy-dom

// A deferred body is rendered by its own request, so an [adjacent] inside it
// can repeat a landmark name the page already holds. The swap renames it.
import { afterEach, beforeAll, describe, expect, it } from 'vitest';

let uniqueAdjacentLabels: (container: Element) => void;

beforeAll(async () => {
  ({ uniqueAdjacentLabels } = await import('./deferredShortcodes.js'));
});

afterEach(() => {
  document.body.innerHTML = '';
});

const nav = (label: string) => `<nav class="shortcode-adjacent" aria-label="${label}"><a href="/note?id=1">x</a></nav>`;
const labels = () => [...document.querySelectorAll('nav.shortcode-adjacent')].map((n) => n.getAttribute('aria-label'));

describe('uniqueAdjacentLabels', () => {
  it('numbers a deferred nav whose name the page already holds', () => {
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
});
