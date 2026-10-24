// @vitest-environment happy-dom
// Plan 2026-10-24-template-generation-quality, step 1d: a stored value or a
// hand-written x-display must never crash the display panel.
import { afterEach, describe, expect, it } from 'vitest';
import './display-mode';
import { renderJsonTable } from '../../tableMaker.js';
import { getLabeledEnumEntries } from '../schema-core';

afterEach(() => {
  document.body.replaceChildren();
});

const geoProps = { latitude: { type: 'number' }, longitude: { type: 'number' } };

async function mount(schema: any, value: any) {
  const el = document.createElement('schema-display-mode') as any;
  el.schema = schema;
  el.value = value;
  document.body.append(el);
  await el.updateComplete;
  return el as HTMLElement;
}

function geoSchema(prop: any) {
  return { type: 'object', properties: { loc: prop } };
}

describe('display panel x-display guards', () => {
  it('renders a valid forced geo pair as before', async () => {
    const el = await mount(geoSchema({ type: 'object', 'x-display': 'geo', properties: geoProps }),
      { loc: { latitude: 52.5, longitude: 13.4 } });
    expect(el.textContent).toContain('52.500000, 13.400000');
    expect(el.querySelector('a[href*="openstreetmap.org"]')).not.toBeNull();
  });

  it('uses the valid lat/lng pair when latitude/longitude are not numbers (forced)', async () => {
    const el = await mount(geoSchema({ type: 'object', 'x-display': 'geo', properties: geoProps }),
      { loc: { latitude: 'bad', longitude: 'bad', lat: 1, lng: 2 } });
    expect(el.textContent).toContain('1.000000, 2.000000');
  });

  it('uses the valid lat/lng pair when latitude/longitude are not numbers (detected)', async () => {
    const el = await mount(geoSchema({ type: 'object' }),
      { loc: { latitude: 'bad', longitude: 'bad', lat: 1, lng: 2 } });
    expect(el.textContent).toContain('1.000000, 2.000000');
  });

  it('falls back to the object grid for a partial value under a forced geo renderer', async () => {
    const el = await mount(geoSchema({ type: 'object', 'x-display': 'geo', properties: geoProps }),
      { loc: { latitude: 1 } });
    expect(el.textContent).toContain('Latitude');
    expect(el.querySelector('a[href*="openstreetmap.org"]')).toBeNull();
  });

  it('renders a non-object value under a forced renderer as its type', async () => {
    const el = await mount(geoSchema({ type: 'string', 'x-display': 'geo' }), { loc: 'Berlin' });
    expect(el.textContent).toContain('Berlin');
    const dims = await mount(geoSchema({ type: 'number', 'x-display': 'dimensions' }), { loc: 7 });
    expect(dims.textContent).toContain('7');
  });

  it('ignores a numeric x-display', async () => {
    const el = await mount(geoSchema({ type: 'string', 'x-display': 5 }), { loc: 'plain' });
    expect(el.textContent).toContain('plain');
    // On an object with properties, a non-string x-display does not stop flattening.
    const obj = await mount(geoSchema({ type: 'object', 'x-display': 5, properties: { a: { type: 'string' } } }),
      { loc: { a: 'inner' } });
    expect(obj.textContent).toContain('inner');
  });

  it('honours an x-display reached through $ref, and guards it', async () => {
    const schema = {
      type: 'object',
      $defs: { geo: { type: 'object', 'x-display': 'geo', properties: geoProps } },
      properties: { loc: { $ref: '#/$defs/geo' } },
    };
    const ok = await mount(schema, { loc: { latitude: 3, longitude: 4 } });
    expect(ok.textContent).toContain('3.000000, 4.000000');
    const partial = await mount(schema, { loc: { latitude: 3 } });
    expect(partial.textContent).toContain('Latitude');
    const numeric = await mount({
      type: 'object',
      $defs: { s: { type: 'string', 'x-display': 9 } },
      properties: { loc: { $ref: '#/$defs/s' } },
    }, { loc: 'via ref' });
    expect(numeric.textContent).toContain('via ref');
  });

  it('honours an x-display reached through allOf, and guards it', async () => {
    const prop = { type: 'object', allOf: [{ 'x-display': 'geo' }], properties: geoProps };
    const ok = await mount(geoSchema(prop), { loc: { lat: 5, lng: 6 } });
    expect(ok.textContent).toContain('5.000000, 6.000000');
    const partial = await mount(geoSchema(prop), { loc: { longitude: 'x' } });
    expect(partial.textContent).toContain('Longitude');
    const numeric = await mount(geoSchema({ type: 'string', allOf: [{ 'x-display': true }] }), { loc: 'via allOf' });
    expect(numeric.textContent).toContain('via allOf');
  });

  it('renders valid forced url, daterange and dimensions values', async () => {
    const el = await mount({
      type: 'object',
      properties: {
        link: { type: 'object', 'x-display': 'url', properties: { href: { type: 'string' }, host: { type: 'string' } } },
        span: { type: 'object', 'x-display': 'daterange', properties: { start: { type: 'string' }, end: { type: 'string' } } },
        size: { type: 'object', 'x-display': 'dimensions', properties: { width: { type: 'number' }, height: { type: 'number' } } },
      },
    }, {
      link: { href: 'https://example.com/p', host: 'example.com' },
      span: { start: '2024-03-15', end: '2024-04-01' },
      size: { width: 1920, height: 1080 },
    });
    expect(el.querySelector('a[href="https://example.com/p"]')).not.toBeNull();
    expect(el.textContent).toContain('2024 \u2014');
    expect(el.textContent).toContain('1920 \u00D7 1080');
  });

  it('does not auto-detect another shape when a forced renderer fails', async () => {
    const el = await mount(geoSchema({ type: 'object', 'x-display': 'geo', properties: { width: { type: 'number' }, height: { type: 'number' } } }),
      { loc: { width: 3, height: 4 } });
    expect(el.textContent).not.toContain('\u00D7');
    expect(el.textContent).toContain('Width');
  });

  it('lets a resolved x-display win over an overlapping auto-detected shape', async () => {
    const value = { loc: { latitude: 1, longitude: 2, width: 3, height: 4 } };
    const viaAllOf = await mount(geoSchema({ type: 'object', allOf: [{ 'x-display': 'dimensions' }] }), value);
    expect(viaAllOf.textContent).toContain('3 \u00D7 4');
    expect(viaAllOf.querySelector('a[href*="openstreetmap.org"]')).toBeNull();
    const viaRef = await mount({
      type: 'object',
      $defs: { d: { type: 'object', 'x-display': 'dimensions' } },
      properties: { loc: { $ref: '#/$defs/d' } },
    }, value);
    expect(viaRef.textContent).toContain('3 \u00D7 4');
  });

  it('renders mixed array members under every forced renderer without throwing', async () => {
    for (const name of ['url', 'geo', 'daterange', 'dimensions']) {
      const el = await mount(geoSchema({ type: 'array', 'x-display': name }), { loc: [null, {}, 7, { a: 'inner' }] });
      expect(el.textContent).toContain('inner');
      expect(el.textContent).toContain('7');
    }
    const plain = await mount(geoSchema({ type: 'array' }), { loc: [null, 'text', { a: 'obj' }] });
    expect(plain.textContent).toContain('text');
    expect(plain.textContent).toContain('obj');
  });

  it('renders a non-primitive value under a scalar type without coercing it', async () => {
    const hostile = { toString: null, note: 'kept' };
    const schemas = [
      geoSchema({ type: 'number', 'x-display': 'dimensions' }),
      { type: 'object', $defs: { n: { type: 'number', 'x-display': 'dimensions' } }, properties: { loc: { $ref: '#/$defs/n' } } },
      geoSchema({ type: 'number', allOf: [{ 'x-display': 'dimensions' }] }),
      geoSchema({ type: 'string' }),
      geoSchema({ type: 'boolean' }),
      geoSchema({ type: 'string', enum: ['a', 'b'] }),
      geoSchema({ oneOf: [{ const: 'a', title: 'A' }, { const: 'b', title: 'B' }] }),
    ];
    for (const schema of schemas) {
      const el = await mount(schema, { loc: hostile });
      expect(el.textContent).toContain('kept');
    }
    const arr = await mount(geoSchema({ type: 'string' }), { loc: [hostile, 'x'] });
    expect(arr.textContent).toContain('kept');
    expect(arr.textContent).toContain('x');
  });

  it('keeps the object grid under the raw and none opt-outs', async () => {
    for (const xd of ['raw', 'none']) {
      const el = await mount(geoSchema({ type: 'object', 'x-display': xd, properties: geoProps }),
        { loc: { latitude: 1, longitude: 2 } });
      expect(el.textContent).toContain('Latitude');
      expect(el.querySelector('a[href*="openstreetmap.org"]')).toBeNull();
    }
  });

  it('falls back to the values for an invalid date range', async () => {
    const value = { loc: { start: '2024-03-15', end: 'bad' } };
    for (const schema of [
      geoSchema({ type: 'object', 'x-display': 'daterange', properties: { start: { type: 'string' }, end: { type: 'string' } } }),
      geoSchema({ type: 'object' }),
    ]) {
      const el = await mount(schema, value);
      expect(el.textContent).toContain('bad');
      expect(el.textContent).not.toContain('Invalid Date');
    }
  });

  it('renders nested mixed arrays through the real renderJsonTable', async () => {
    (window as any).renderJsonTable = renderJsonTable;
    try {
      const el = await mount(geoSchema({ type: 'boolean' }),
        { loc: { a: { b: { c: { d: [null, { x: 'deep' }] } } } } });
      expect(el.textContent).toContain('deep');
    } finally {
      delete (window as any).renderJsonTable;
    }
  });

  it('labels a non-primitive labeled-enum const through JSON', async () => {
    const hostile = { toString: null };
    expect(getLabeledEnumEntries({ oneOf: [{ const: hostile }] })[0].label).toBe('{"toString":null}');
    const el = await mount(geoSchema({ oneOf: [{ const: hostile }, { const: 'b', title: 'B' }] }),
      { loc: { toString: null, note: 'kept' } });
    expect(el.textContent).toContain('kept');
  });

  it('shows Yes/No only for real booleans', async () => {
    const yes = await mount(geoSchema({ type: 'boolean' }), { loc: true });
    expect(yes.textContent).toContain('Yes');
    const str = await mount(geoSchema({ type: 'boolean' }), { loc: 'false' });
    expect(str.textContent).toContain('false');
    expect(str.textContent).not.toContain('Yes');
  });

  it('shows an invalid date string as itself', async () => {
    for (const format of ['date', 'date-time']) {
      const el = await mount(geoSchema({ type: 'string', format }), { loc: 'bad' });
      expect(el.textContent).toContain('bad');
      expect(el.textContent).not.toContain('Invalid Date');
    }
  });
});
