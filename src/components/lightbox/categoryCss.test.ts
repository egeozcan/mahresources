// @vitest-environment happy-dom
import { afterEach, describe, expect, it, vi } from 'vitest';

vi.mock('../../index.js', () => ({ abortableFetch: vi.fn() }));
vi.mock('../../userSettings.js', () => ({ whenLoaded: vi.fn().mockResolvedValue(undefined), get: vi.fn(), set: vi.fn() }));

import { editPanelMethods } from './editPanel.js';

const css = '.lb { color: red }';
const store = () => ({
  ...editPanelMethods,
  displayDetails: () => ({ resourceCategory: { ID: 4, CustomCSS: css } }),
});
const style = (parentHTML: string) => {
  document.body.innerHTML = parentHTML;
};

afterEach(() => { document.body.innerHTML = ''; });

describe('categoryCSSToInject', () => {
  it('adds nothing when the page itself carries the stylesheet', () => {
    style('<style data-mr-custom-css="resource:4"></style>');
    expect(store().categoryCSSToInject()).toBe('');
  });

  it('injects when the only copy is inside the hover card, which is cleared on close', () => {
    style('<div id="hovercard-popover"><style data-mr-custom-css="resource:4"></style></div>');
    expect(store().categoryCSSToInject()).toBe(css);
  });

  it.each(['autorefresh-shortcode', 'lazy-shortcode', 'details-shortcode', 'div data-shortcode-region="t"', 'div data-text-block-rendered'])(
    'injects when the only copy is inside <%s>, whose content is replaced',
    (tag) => {
      style(`<${tag}><style data-mr-custom-css="resource:4"></style></${tag.split(' ')[0]}>`);
      expect(store().categoryCSSToInject()).toBe(css);
    },
  );

  it('a lasting copy elsewhere still counts beside a temporary one', () => {
    style('<div id="hovercard-popover"><style data-mr-custom-css="resource:4"></style></div><style data-mr-custom-css="resource:4"></style>');
    expect(store().categoryCSSToInject()).toBe('');
  });
});
