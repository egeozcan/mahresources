// @vitest-environment happy-dom
import { afterEach, describe, expect, it, vi } from 'vitest';
import './meta-shortcode';
import type { MetaShortcode } from './meta-shortcode';

afterEach(() => {
  document.body.replaceChildren();
  vi.unstubAllGlobals();
});

async function mount(value: unknown, attrs: Record<string, string> = {}) {
  const el = document.createElement('meta-shortcode') as MetaShortcode;
  for (const [name, val] of Object.entries({
    'data-date-time': 'true', 'data-editable': 'true', 'data-path': 'deadline',
    'data-entity-type': 'note', 'data-entity-id': '12', 'data-value': JSON.stringify(value),
    'data-schema': JSON.stringify({ type: 'string', format: 'date', default: '2024-02-29' }), ...attrs,
  })) el.setAttribute(name, val);
  document.body.append(el);
  await el.updateComplete;
  return el;
}

async function edit(el: MetaShortcode) {
  el.querySelector<HTMLButtonElement>('button')!.click();
  await el.updateComplete;
  return el.querySelector<HTMLInputElement>('input')!;
}

async function save(el: MetaShortcode) {
  [...el.querySelectorAll('button')].find(b => b.textContent?.trim() === 'Save')!.click();
  await vi.waitFor(() => expect(el.querySelector('input')).toBeNull());
}

describe('datetime shortcode editor', () => {
  it('shows invalid text safely, initializes the default and leaves storage alone on cancel', async () => {
    const fetch = vi.fn();
    vi.stubGlobal('fetch', fetch);
    const el = await mount('<img src=x onerror=alert(1)>');
    expect(el.textContent).toContain('<img src=x onerror=alert(1)>');
    expect(el.querySelector('img')).toBeNull();
    expect((await edit(el)).value).toBe('2024-02-29');
    [...el.querySelectorAll('button')].find(b => b.textContent?.trim() === 'Cancel')!.click();
    await el.updateComplete;
    expect(el.textContent).toContain('<img src=x onerror=alert(1)>');
    expect(fetch).not.toHaveBeenCalled();
  });

  it('saves an unchanged schema default only on explicit Save', async () => {
    const fetch = vi.fn().mockResolvedValue({ ok: true, json: async () => ({ meta: { deadline: '2024-02-29' } }) });
    vi.stubGlobal('fetch', fetch);
    const el = await mount('bad');
    await edit(el);
    await save(el);
    expect(fetch.mock.calls[0][0]).toBe('/v1/note/editMeta?id=12');
    expect(fetch.mock.calls[0][1].body.get('value')).toBe('"2024-02-29"');
    expect(el.textContent).toContain('2024-02-29');
  });

  it('preserves the source offset, saves custom output, and refreshes sibling displays', async () => {
    const value = '2026-09-05T14:30:00+02:00';
    const saved = '2026-09-06T16:45:00+02:00';
    const fetch = vi.fn().mockResolvedValue({ ok: true, json: async () => ({ meta: { deadline: saved } }) });
    vi.stubGlobal('fetch', fetch);
    const el = await mount(value, { 'data-layout': 'January 2, 2006 at 15:04' });
    const sibling = await mount(value, { 'data-editable': 'false' });
    const input = await edit(el);
    expect(input.type).toBe('datetime-local');
    input.value = '2026-09-06T16:45';
    input.dispatchEvent(new Event('input'));
    await el.updateComplete;
    await save(el);
    await sibling.updateComplete;
    expect(fetch.mock.calls[0][1].body.get('value')).toBe(JSON.stringify(saved));
    expect(el.textContent).toContain('September 6, 2026 at 16:45');
    expect(sibling.textContent).toContain(saved);
    expect(sibling.querySelector('button')).toBeNull();
  });

  it('keeps edits available when saving fails', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: false, status: 500 }));
    const error = vi.spyOn(console, 'error').mockImplementation(() => {});
    const el = await mount('2026-09-05');
    await edit(el);
    [...el.querySelectorAll('button')].find(b => b.textContent?.trim() === 'Save')!.click();
    await vi.waitFor(() => expect(error).toHaveBeenCalled());
    expect(el.querySelector('input')).not.toBeNull();
    error.mockRestore();
  });
});
