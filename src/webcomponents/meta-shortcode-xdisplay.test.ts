// @vitest-environment happy-dom
// Plan 2026-10-24-template-generation-quality, step 1d: a stored value or a
// hand-written x-display must never crash the [meta] shortcode.
import { afterEach, describe, expect, it } from 'vitest';
import './meta-shortcode';
import type { MetaShortcode } from './meta-shortcode';

afterEach(() => {
  document.body.replaceChildren();
});

async function mount(schema: any, value: any) {
  const el = document.createElement('meta-shortcode') as MetaShortcode;
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
});
