// @vitest-environment happy-dom
import { afterEach, describe, expect, it, vi } from 'vitest';
import './meta-shortcode';
import type { MetaShortcode } from './meta-shortcode';

afterEach(() => {
  document.body.replaceChildren();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

async function mount(attrs: Record<string, string> = {}) {
  const el = document.createElement('meta-shortcode') as MetaShortcode;
  for (const [name, value] of Object.entries({
    'data-pills': 'true', 'data-editable': 'true', 'data-path': 'task.priority',
    'data-entity-type': 'group', 'data-entity-id': '12', 'data-value': '2',
    'data-schema': JSON.stringify({ title: 'Priority', oneOf: [
      { const: 1, title: 'Low' }, { const: 2, title: 'Medium' }, { const: 3, title: 'High' },
    ] }), ...attrs,
  })) el.setAttribute(name, value);
  document.body.append(el);
  await el.updateComplete;
  return el;
}

function buttons(el: MetaShortcode) { return [...el.querySelectorAll<HTMLButtonElement>('[role="radio"]')]; }
function selected(el: MetaShortcode) { return el.querySelector('[aria-checked="true"]')?.textContent?.trim(); }

describe('pills shortcode', () => {
  it('uses oneOf colors for the selected value and clears them when selection changes', async () => {
    const el = await mount({
      'data-value': '0',
      'data-schema': JSON.stringify({ type: ['integer', 'null'], oneOf: [
        { const: 0, title: 'inactive', 'x-color': '#d5d5d5' },
        { const: 1, title: 'active', 'x-color': '#008e00' },
        { const: null, title: 'Unset' },
      ] }),
    });
    expect(selected(el)).toBe('inactive');
    expect(buttons(el)[0].style.getPropertyValue('--meta-pill-background')).toContain('#d5d5d5');
    expect(buttons(el)[1].style.getPropertyValue('--meta-pill-background')).toBe('');
    el.setAttribute('data-value', '1');
    await el.updateComplete;
    expect(selected(el)).toBe('active');
    expect(buttons(el)[0].style.getPropertyValue('--meta-pill-background')).toBe('');
    expect(buttons(el)[1].style.getPropertyValue('--meta-pill-background')).toContain('#008e00');
    expect(buttons(el)[1].style.getPropertyValue('--meta-pill-text')).toContain('#008e00');
    el.setAttribute('data-value', 'null');
    await el.updateComplete;
    expect(selected(el)).toBe('Unset');
    expect(buttons(el).every(button => button.style.getPropertyValue('--meta-pill-background') === '')).toBe(true);
  });

  it('renders labeled enums and saves numeric values, syncing a sibling', async () => {
    const fetch = vi.fn().mockResolvedValue({ ok: true, json: async () => ({ meta: { task: { priority: 3 } } }) });
    vi.stubGlobal('fetch', fetch);
    const el = await mount();
    const sibling = await mount({ 'data-editable': 'false' });
    expect(selected(el)).toBe('Medium');
    expect(el.querySelector('[role="radiogroup"]')?.getAttribute('aria-label')).toBe('Priority');
    buttons(el)[2].click();
    await vi.waitFor(() => expect(selected(el)).toBe('High'));
    await sibling.updateComplete;
    expect(selected(sibling)).toBe('High');
    expect(fetch.mock.calls[0][0]).toBe('/v1/group/editMeta?id=12');
    expect(fetch.mock.calls[0][1].body.get('path')).toBe('task.priority');
    expect(fetch.mock.calls[0][1].body.get('value')).toBe('3');
    buttons(sibling)[0].click();
    expect(fetch).toHaveBeenCalledTimes(1);
  });

  it('uses a plain enum and refreshes options after morph attribute changes', async () => {
    const el = await mount({ 'data-value': '"Low"', 'data-schema': '{"enum":["Low","High"]}' });
    expect(selected(el)).toBe('Low');
    el.setAttribute('data-options', '["Low","Medium","High"]');
    el.setAttribute('data-value', '"Medium"');
    el.refreshFromMorph();
    await el.updateComplete;
    expect(buttons(el)).toHaveLength(3);
    expect(selected(el)).toBe('Medium');
  });

  it.each([false, null, 0, '0'])('preserves the type of manual option %j without a schema', async value => {
    const fetch = vi.fn().mockResolvedValue({ ok: true, json: async () => ({ meta: { task: { priority: value } } }) });
    vi.stubGlobal('fetch', fetch);
    const el = await mount({ 'data-schema': '', 'data-options': JSON.stringify([{ value, label: 'Choose' }]), 'data-value': '' });
    expect(selected(el)).toBeUndefined();
    expect(fetch).not.toHaveBeenCalled();
    buttons(el)[0].click();
    await vi.waitFor(() => expect(selected(el)).toBe('Choose'));
    expect(fetch.mock.calls[0][1].body.get('value')).toBe(JSON.stringify(value));
  });

  it('keeps arbitrary labels as text and manual choices override the schema', async () => {
    const label = '<img src=x onerror=alert(1)>';
    const el = await mount({ 'data-options': JSON.stringify([{ value: 2, label }]) });
    expect(buttons(el)).toHaveLength(1);
    expect(selected(el)).toBe(label);
    expect(el.querySelector('img')).toBeNull();
  });

  it('keeps the saved selection after failure and retries, preventing concurrent writes', async () => {
    let respond!: (result: unknown) => void;
    const fetch = vi.fn().mockImplementationOnce(() => new Promise(resolve => { respond = resolve; }))
      .mockResolvedValue({ ok: true, json: async () => ({ meta: { task: { priority: 3 } } }) });
    vi.stubGlobal('fetch', fetch);
    vi.spyOn(console, 'error').mockImplementation(() => {});
    const el = await mount();
    buttons(el)[2].click();
    buttons(el)[0].click();
    expect(fetch).toHaveBeenCalledTimes(1);
    respond({ ok: false, status: 400 });
    await vi.waitFor(() => expect(el.textContent).toContain('Could not save'));
    expect(selected(el)).toBe('Medium');
    buttons(el)[2].click();
    await vi.waitFor(() => expect(selected(el)).toBe('High'));
    expect(el.textContent).not.toContain('Could not save');
  });

  it('supports arrow key selection and focuses the new pill', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: true, json: async () => ({ meta: { task: { priority: 3 } } }) }));
    const el = await mount();
    expect(buttons(el).map(b => b.tabIndex)).toEqual([-1, 0, -1]);
    buttons(el)[1].dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowRight', bubbles: true }));
    await vi.waitFor(() => expect(selected(el)).toBe('High'));
    expect(document.activeElement).toBe(buttons(el)[2]);
    expect(buttons(el).map(b => b.tabIndex)).toEqual([-1, -1, 0]);
  });

  it.each(['bad json', '{}', '[]', '[{"value":{},"label":"Bad"}]'])('reports unusable options: %s', async options => {
    const el = await mount({ 'data-options': options });
    expect(buttons(el)).toHaveLength(0);
    expect(el.textContent).toContain('No options configured');
    expect(el.textContent).toContain('Current: 2');
  });
});
