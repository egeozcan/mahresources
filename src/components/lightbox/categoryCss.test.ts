// @vitest-environment happy-dom
import { afterEach, describe, expect, it, vi } from 'vitest';

vi.mock('../../index.js', () => ({ abortableFetch: vi.fn() }));
vi.mock('../../userSettings.js', () => ({ whenLoaded: vi.fn().mockResolvedValue(undefined), get: vi.fn(), set: vi.fn() }));

import { editPanelMethods } from './editPanel.js';

const css = '.lb { color: red }';
const store = (category: unknown) => ({ ...editPanelMethods, displayDetails: () => ({ resourceCategory: category }) });

afterEach(() => { document.body.innerHTML = ''; });

describe('categoryCSSToInject', () => {
  // Whatever the page holds may be replaced away without the panel noticing, so the panel
  // carries its own copy even when the page has one.
  it.each([
    '',
    '<style data-mr-custom-css="resource:4"></style>',
    '<div id="hovercard-popover"><style data-mr-custom-css="resource:4"></style></div>',
    '<div class="list-container"><style data-mr-custom-css="resource:4"></style></div>',
  ])('carries the category CSS whatever the page holds: %s', (html) => {
    document.body.innerHTML = html;
    expect(store({ ID: 4, CustomCSS: css }).categoryCSSToInject()).toBe(css);
  });

  it('carries nothing for a category without CSS or no category', () => {
    expect(store({ ID: 4, CustomCSS: '' }).categoryCSSToInject()).toBe('');
    expect(store(undefined).categoryCSSToInject()).toBe('');
  });
});
