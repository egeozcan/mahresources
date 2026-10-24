// @vitest-environment happy-dom
// shortcodes/schema_extensions.go is the Go source of truth for the schema
// extensions the MetaSchema generator is taught and linted against.
// shortcodes/testdata/schema_extensions_contract.json is that source written
// out (a Go test fails when it is stale). This test checks the real selectors
// and components behave as the contract says, so a change to either side fails
// here or there.
import { afterEach, describe, expect, it } from 'vitest';
import contract from '../../shortcodes/testdata/schema_extensions_contract.json';
import { builtinRenderers } from './display-renderers';
import { isLabeledEnum } from './schema-core';
import './modes/display-mode';
import './modes/form-mode';
import '../webcomponents/meta-shortcode';

const WHERE = 'shortcodes/schema_extensions.go (shortcodes/testdata/schema_extensions_contract.json)';

afterEach(() => {
  document.body.replaceChildren();
});

type Kind = 'number' | 'string' | 'date';
const conforming: Record<Kind, any> = { number: 1.5, string: 'x', date: '2024-03-15' };
const wrong: Record<Kind, any> = { number: 'x', string: 5, date: 'not a date' };

function valueOf(keys: string[], types: Record<string, string>): Record<string, any> {
  return Object.fromEntries(keys.map(k => [k, conforming[types[k] as Kind]]));
}

describe('built-in renderers match the Go contract', () => {
  it('registers the same renderers in the same order', () => {
    expect(builtinRenderers.map(r => r.name), `renderer order differs from ${WHERE}`)
      .toEqual(contract.renderers.map(r => r.name));
  });

  for (const spec of contract.renderers) {
    const renderer = builtinRenderers.find(r => r.name === spec.name);
    const types = spec.fieldTypes as Record<string, string>;

    it(`${spec.name}: accepts every declared shape with conforming values`, () => {
      expect(renderer, `${spec.name} is not registered; see ${WHERE}`).toBeDefined();
      for (const shape of spec.shapes) {
        expect(renderer!.select(valueOf(shape, types)), `${spec.name} rejects ${shape}; see ${WHERE}`).not.toBeNull();
      }
    });

    it(`${spec.name}: rejects an incomplete shape and a wrong field type`, () => {
      for (const shape of spec.shapes) {
        for (const key of shape) {
          const missing = valueOf(shape.filter(k => k !== key), types);
          expect(renderer!.select(missing), `${spec.name} accepts ${shape} without ${key}; see ${WHERE}`).toBeNull();
          const badType = { ...valueOf(shape, types), [key]: wrong[types[key] as Kind] };
          expect(renderer!.select(badType), `${spec.name} accepts a ${types[key]} ${key} that is the wrong type; see ${WHERE}`).toBeNull();
        }
      }
    });

    it(`${spec.name}: rejects keys paired across alternatives`, () => {
      for (const a of spec.shapes) {
        for (const b of spec.shapes) {
          if (a === b) continue;
          const mixed = [...new Set([a[0], ...b.slice(1)])];
          const completes = spec.shapes.some(s => s.every(k => mixed.includes(k)));
          if (completes) continue;
          expect(renderer!.select(valueOf(mixed, types)), `${spec.name} accepts the mix ${mixed}; see ${WHERE}`).toBeNull();
        }
      }
    });
  }

  it('does not give a renderer the name of an opt-out value', () => {
    for (const v of contract.optOut) expect(builtinRenderers.some(r => r.name === v)).toBe(false);
  });
});

// A geo-shaped value that a detecting component draws as a map link, and that
// an opted-out one draws as a plain object.
const geoValue = { latitude: 1, longitude: 2 };

async function panel(xDisplay: string | undefined, value: any) {
  const el = document.createElement('schema-display-mode') as any;
  const prop: any = { type: 'object', properties: { latitude: { type: 'number' }, longitude: { type: 'number' } } };
  if (xDisplay !== undefined) prop['x-display'] = xDisplay;
  el.schema = { type: 'object', properties: { loc: prop } };
  el.value = { loc: value };
  document.body.append(el);
  await el.updateComplete;
  return el as HTMLElement;
}

async function meta(xDisplay: string | undefined, value: any) {
  const el = document.createElement('meta-shortcode') as any;
  const schema: any = { type: 'object' };
  if (xDisplay !== undefined) schema['x-display'] = xDisplay;
  el.setAttribute('data-path', 'loc');
  el.setAttribute('data-entity-type', 'group');
  el.setAttribute('data-entity-id', '1');
  el.setAttribute('data-schema', JSON.stringify(schema));
  el.setAttribute('data-value', JSON.stringify(value));
  document.body.append(el);
  await el.updateComplete;
  return el as HTMLElement;
}

const hasMap = (el: HTMLElement) => el.querySelector('a[href*="openstreetmap.org"]') !== null;

describe('x-display opt-outs match the Go contract', () => {
  it('detects the shape when x-display is unset (the control)', async () => {
    expect(hasMap(await panel(undefined, geoValue))).toBe(true);
    expect(hasMap(await meta(undefined, geoValue))).toBe(true);
  });

  for (const value of contract.optOut) {
    it(`${value} skips auto-detection in the display panel and in [meta]`, async () => {
      expect(hasMap(await panel(value, geoValue)), `the panel still detects under ${value}; see ${WHERE}`).toBe(false);
      expect(hasMap(await meta(value, geoValue)), `[meta] still detects under ${value}; see ${WHERE}`).toBe(false);
    });
  }

  it('an unknown value is not an opt-out', async () => {
    // It has no renderer, so it also draws as a plain object: the lint calls it unknown.
    expect(contract.optOut).not.toContain('map');
    expect(hasMap(await panel('map', geoValue))).toBe(false);
  });
});

describe('labeled-enum complex keys match the Go contract', () => {
  const entry = (extra: Record<string, any>) => ({ oneOf: [{ const: 'a', title: 'A', ...extra }] });

  it('keeps title, description and x-color in a labeled-enum entry', () => {
    for (const key of ['title', 'description', 'x-color']) {
      expect(isLabeledEnum(entry({ [key]: 'v' })), `${key} must not be complex; see ${WHERE}`).toBe(true);
    }
  });

  for (const key of contract.complexKeys) {
    it(`${key} turns an entry into a plain subschema`, () => {
      expect(isLabeledEnum(entry({ [key]: key === 'enum' ? ['a'] : 'v' })), `${key} should be complex; see ${WHERE}`).toBe(false);
    });
  }

  it('lists no key the app does not treat as complex', () => {
    for (const probe of ['default', 'examples', 'readOnly', 'format', 'not', 'then', 'else', 'additionalProperties']) {
      if (contract.complexKeys.includes(probe)) continue;
      expect(isLabeledEnum(entry({ [probe]: 'v' })), `${probe} is complex in schema-core.ts but missing from ${WHERE}`).toBe(true);
    }
  });
});

describe('formats with their own UI match the Go contract', () => {
  const probes = ['uuid', 'hostname', 'ipv4', 'time', 'regex', 'json-pointer'];

  async function formInputType(format: string): Promise<string> {
    const el = document.createElement('schema-form-mode') as any;
    el.schema = { type: 'object', properties: { f: { type: 'string', format } } };
    el.value = {};
    document.body.append(el);
    await el.updateComplete;
    const input = [...el.querySelectorAll('input')].find((i: HTMLInputElement) => i.type !== 'hidden') as HTMLInputElement;
    el.remove();
    return input.type;
  }

  async function displayHTML(format: string | undefined): Promise<string> {
    const el = document.createElement('schema-display-mode') as any;
    el.schema = { type: 'object', properties: { f: { type: 'string', ...(format ? { format } : {}) } } };
    el.value = { f: '2024-03-15' };
    document.body.append(el);
    await el.updateComplete;
    const html = el.innerHTML.replace(/<!--[^>]*-->/g, '');
    el.remove();
    return html;
  }

  it('gives each contract format its own form input and display', async () => {
    const plain = await displayHTML(undefined);
    for (const format of contract.formats) {
      expect(await formInputType(format), `${format} has no form input in form-mode.ts; see ${WHERE}`).not.toBe('text');
      expect(await displayHTML(format), `${format} has no display in display-mode.ts; see ${WHERE}`).not.toBe(plain);
    }
  });

  it('gives no other common format its own form input or display', async () => {
    const plain = await displayHTML(undefined);
    for (const format of probes) {
      if (contract.formats.includes(format)) continue;
      expect(await formInputType(format), `${format} has a form input in form-mode.ts but is missing from ${WHERE}`).toBe('text');
      expect(await displayHTML(format), `${format} has a display in display-mode.ts but is missing from ${WHERE}`).toBe(plain);
    }
  });
});
