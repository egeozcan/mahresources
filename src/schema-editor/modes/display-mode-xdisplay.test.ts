// @vitest-environment happy-dom
// Plan 2026-10-24-template-generation-quality, step 1d: a stored value or a
// hand-written x-display must never crash the display panel.
import { afterEach, describe, expect, it } from 'vitest';
import './display-mode';

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
});
