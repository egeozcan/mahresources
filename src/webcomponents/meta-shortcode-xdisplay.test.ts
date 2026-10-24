// @vitest-environment happy-dom
// Plan 2026-10-24-template-generation-quality, step 1d: a stored value or a
// hand-written x-display must never crash the [meta] shortcode.
import { afterEach, describe, expect, it, vi } from 'vitest';
import './meta-shortcode';
import type { MetaShortcode } from './meta-shortcode';

afterEach(() => {
  document.body.replaceChildren();
  vi.unstubAllGlobals();
});

async function mount(schema: any, value: any, attrs: Record<string, string> = {}) {
  const el = document.createElement('meta-shortcode') as MetaShortcode;
  for (const [name, attr] of Object.entries(attrs)) el.setAttribute(name, attr);
  el.setAttribute('data-path', 'loc');
  el.setAttribute('data-entity-type', 'group');
  el.setAttribute('data-entity-id', '1');
  el.setAttribute('data-schema', JSON.stringify(schema));
  el.setAttribute('data-value', JSON.stringify(value));
  document.body.append(el);
  await el.updateComplete;
  return el;
}

const geo = { type: 'object', 'x-display': 'geo' };

describe('[meta] x-display guards', () => {
  it('renders a valid forced geo pair as before', async () => {
    const el = await mount(geo, { latitude: 52.5, longitude: 13.4 });
    expect(el.textContent).toContain('52.500000, 13.400000');
  });

  it('uses the valid lat/lng pair when latitude/longitude are not numbers', async () => {
    const forced = await mount(geo, { latitude: 'bad', longitude: 'bad', lat: 1, lng: 2 });
    expect(forced.textContent).toContain('1.000000, 2.000000');
    const detected = await mount({ type: 'object' }, { latitude: 'bad', longitude: 'bad', lat: 1, lng: 2 });
    expect(detected.textContent).toContain('1.000000, 2.000000');
  });

  it('falls back to normal rendering for a partial value under a forced geo renderer', async () => {
    const el = await mount(geo, { latitude: 1 });
    expect(el.textContent).toContain('{"latitude":1}');
  });

  it('renders a non-object value under a forced renderer as its type', async () => {
    const str = await mount({ type: 'string', 'x-display': 'geo' }, 'Berlin');
    expect(str.textContent).toContain('Berlin');
    const num = await mount({ type: 'number', 'x-display': 'daterange' }, 7);
    expect(num.textContent).toContain('7');
  });

  it('ignores a numeric x-display', async () => {
    const el = await mount({ type: 'string', 'x-display': 5 }, 'plain');
    expect(el.textContent).toContain('plain');
  });

  it('guards an x-display found on the resolved schema ($ref)', async () => {
    const defs = { g: { type: 'object', 'x-display': 'geo' }, n: { type: 'string', 'x-display': 9 } };
    const ok = await mount({ $ref: '#/$defs/g', $defs: defs }, { latitude: 3, longitude: 4 });
    expect(ok.textContent).toContain('3.000000, 4.000000');
    const partial = await mount({ $ref: '#/$defs/g', $defs: defs }, { latitude: 3 });
    expect(partial.textContent).toContain('{"latitude":3}');
    const numeric = await mount({ $ref: '#/$defs/n', $defs: defs }, 'via ref');
    expect(numeric.textContent).toContain('via ref');
  });

  it('guards an x-display found on the resolved schema (allOf)', async () => {
    const ok = await mount({ allOf: [{ type: 'object', 'x-display': 'geo' }] }, { lat: 5, lng: 6 });
    expect(ok.textContent).toContain('5.000000, 6.000000');
    const partial = await mount({ allOf: [{ type: 'object', 'x-display': 'geo' }] }, { longitude: 'x' });
    expect(partial.textContent).toContain('{"longitude":"x"}');
    const numeric = await mount({ allOf: [{ type: 'string', 'x-display': true }] }, 'via allOf');
    expect(numeric.textContent).toContain('via allOf');
  });

  it('renders valid forced url, daterange and dimensions values', async () => {
    const url = await mount({ type: 'object', 'x-display': 'url' }, { href: 'https://example.com/p', host: 'example.com' });
    expect(url.querySelector('a[href="https://example.com/p"]')).not.toBeNull();
    const range = await mount({ type: 'object', 'x-display': 'daterange' }, { start: '2024-03-15', end: '2024-04-01' });
    expect(range.textContent).toContain('2024 \u2014');
    const dims = await mount({ type: 'object', 'x-display': 'dimensions' }, { width: 1920, height: 1080 });
    expect(dims.textContent).toContain('1920 \u00D7 1080');
  });

  it('does not auto-detect another shape when a forced renderer fails', async () => {
    const el = await mount(geo, { width: 3, height: 4 });
    expect(el.textContent).not.toContain('\u00D7');
    expect(el.textContent).toContain('{"width":3,"height":4}');
  });

  it('lets a resolved x-display win over an overlapping auto-detected shape', async () => {
    const value = { latitude: 1, longitude: 2, width: 3, height: 4 };
    const viaAllOf = await mount({ allOf: [{ type: 'object', 'x-display': 'dimensions' }] }, value);
    expect(viaAllOf.textContent).toContain('3 \u00D7 4');
    expect(viaAllOf.querySelector('a[href*="openstreetmap.org"]')).toBeNull();
    const viaRef = await mount({ $ref: '#/$defs/d', $defs: { d: { type: 'object', 'x-display': 'dimensions' } } }, value);
    expect(viaRef.textContent).toContain('3 \u00D7 4');
    // A resolved x-display that fails selection skips auto-detection too.
    const failed = await mount({ allOf: [{ type: 'object', 'x-display': 'url' }] }, value);
    expect(failed.textContent).toContain(JSON.stringify(value));
  });

  it('renders a non-primitive value under a scalar type without coercing it', async () => {
    const hostile = { toString: null };
    const schemas = [
      { type: 'number', 'x-display': 'dimensions' },
      { $ref: '#/$defs/n', $defs: { n: { type: 'number', 'x-display': 'dimensions' } } },
      { allOf: [{ type: 'number', 'x-display': 'dimensions' }] },
      { type: 'boolean' },
      { type: 'string', enum: ['a', 'b'] },
      { oneOf: [{ const: 'a', title: 'A' }, { const: 'b', title: 'B' }] },
      { allOf: [{ enum: ['a', 'b'] }] },
    ];
    for (const schema of schemas) {
      const el = await mount(schema, hostile);
      expect(el.textContent).toContain('{"toString":null}');
    }
    const arr = await mount({ type: 'string' }, [hostile]);
    expect(arr.textContent).toContain('[{"toString":null}]');
  });

  it('shows a non-primitive current value next to pills without coercing it', async () => {
    const el = await mount({ enum: ['a', 'b'] }, { toString: null }, { 'data-pills': 'true' });
    expect(el.textContent).toContain('Current: {"toString":null}');
  });

  it('honours the raw and none opt-outs, raw and resolved', async () => {
    const value = { latitude: 1, longitude: 2 };
    const schemas = [
      { type: 'object', 'x-display': 'raw' },
      { type: 'object', 'x-display': 'none' },
      { $ref: '#/$defs/r', $defs: { r: { type: 'object', 'x-display': 'raw' } } },
      { allOf: [{ type: 'object', 'x-display': 'none' }] },
    ];
    for (const schema of schemas) {
      const el = await mount(schema, value);
      expect(el.textContent).toContain(JSON.stringify(value));
      expect(el.querySelector('a[href*="openstreetmap.org"]')).toBeNull();
    }
  });

  it('falls back to the values for an invalid date range', async () => {
    const value = { start: '2024-03-15', end: 'bad' };
    for (const schema of [{ type: 'object', 'x-display': 'daterange' }, { type: 'object' }]) {
      const el = await mount(schema, value);
      expect(el.textContent).toContain(JSON.stringify(value));
      expect(el.textContent).not.toContain('Invalid Date');
    }
  });

  it('labels a non-primitive labeled-enum const without coercing it', async () => {
    const el = await mount({ oneOf: [{ const: { toString: null } }, { const: 'b', title: 'B' }] }, 'other');
    expect(el.textContent).toContain('other');
  });

  it('dispatches a plugin x-display ahead of a raw or resolved enum', async () => {
    for (const schema of [
      { enum: ['a'], 'x-display': 'plugin:p:t' },
      { allOf: [{ enum: ['a'], 'x-display': 'plugin:p:t' }] },
      { oneOf: [{ const: 'a', title: 'A' }], 'x-display': 'plugin:p:t' },
    ]) {
      const fetch = vi.fn().mockResolvedValue({ ok: true, text: async () => '<b>plugin-out</b>' });
      vi.stubGlobal('fetch', fetch);
      const el = await mount(schema, 'a');
      await vi.waitFor(() => expect(el.textContent).toContain('plugin-out'));
      expect(fetch.mock.calls[0][0]).toBe('/v1/plugins/p/display/render');
      vi.unstubAllGlobals();
    }
  });

  it('shows Yes/No only for real booleans', async () => {
    const yes = await mount({ type: 'boolean' }, true);
    expect(yes.textContent).toContain('Yes');
    const str = await mount({ type: 'boolean' }, 'false');
    expect(str.textContent).toContain('false');
    expect(str.textContent).not.toContain('Yes');
  });

  it('shows an invalid date as itself', async () => {
    const el = await mount({ type: 'string', format: 'date' }, 'bad', { 'data-date-time': 'true' });
    expect(el.textContent).toContain('bad');
    expect(el.textContent).not.toContain('Invalid Date');
    const plain = await mount({ type: 'string', format: 'date' }, 'bad');
    expect(plain.textContent).toContain('bad');
  });

  it('keeps labels of a labeled enum reached through allOf or $ref', async () => {
    const labeled = { oneOf: [{ const: 'a', title: 'Alpha' }, { const: 'b', title: 'Beta' }] };
    for (const schema of [{ allOf: [labeled] }, { $ref: '#/$defs/e', $defs: { e: labeled } }]) {
      const el = await mount(schema, 'a');
      expect(el.textContent).toContain('Alpha');
    }
  });

  it('shows an impossible date range as its values', async () => {
    const value = { start: '2024-02-01', end: '2024-02-31' };
    const el = await mount({ type: 'object', 'x-display': 'daterange' }, value);
    expect(el.textContent).toContain(JSON.stringify(value));
  });

  it('ignores an unsafe pill x-color', async () => {
    for (const color of [{ toString: null }, 'red; background: url(x)']) {
      const el = await mount({ oneOf: [{ const: 'a', title: 'A', 'x-color': color }] }, 'a', { 'data-pills': 'true' });
      const button = el.querySelector<HTMLButtonElement>('[role="radio"]')!;
      expect(button.style.getPropertyValue('--meta-pill-background')).toBe('');
    }
  });
});
