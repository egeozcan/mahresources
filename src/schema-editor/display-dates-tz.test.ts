// @vitest-environment happy-dom
// Date-only values are calendar dates, not instants. West of UTC,
// new Date("2024-03-15") is the evening of March 14, so formatting it locally
// shifted every date back a day. Node applies a TZ change at runtime.
import { afterAll, describe, expect, it } from 'vitest';
import './modes/display-mode';
import '../webcomponents/meta-shortcode';

const originalTZ = process.env.TZ;
process.env.TZ = 'America/Los_Angeles';
afterAll(() => {
  if (originalTZ === undefined) delete process.env.TZ;
  else process.env.TZ = originalTZ;
});

const opts: Intl.DateTimeFormatOptions = { year: 'numeric', month: 'short', day: 'numeric' };
const local = (y: number, m: number, d: number) => new Date(y, m - 1, d).toLocaleDateString(undefined, opts);

async function panel(prop: any, value: any) {
  const el = document.createElement('schema-display-mode') as any;
  el.schema = { type: 'object', properties: { loc: prop } };
  el.value = { loc: value };
  document.body.append(el);
  await el.updateComplete;
  return el as HTMLElement;
}

async function meta(schema: any, value: any) {
  const el = document.createElement('meta-shortcode') as any;
  el.setAttribute('data-path', 'loc');
  el.setAttribute('data-schema', JSON.stringify(schema));
  el.setAttribute('data-value', JSON.stringify(value));
  document.body.append(el);
  await el.updateComplete;
  return el as HTMLElement;
}

describe('calendar dates west of UTC', () => {
  it('runs in a timezone where UTC midnight is the previous local day', () => {
    expect(new Date('2024-03-15').getDate()).toBe(14);
  });

  it('formats a format: date value on its own calendar day', async () => {
    const el = await panel({ type: 'string', format: 'date' }, '2024-03-15');
    expect(el.textContent).toContain(local(2024, 3, 15));
  });

  it('formats date-range endpoints on their own calendar days in both consumers', async () => {
    const range = { start: '2024-03-15', end: '2024-04-01' };
    const p = await panel({ type: 'object', 'x-display': 'daterange' }, range);
    const m = await meta({ type: 'object', 'x-display': 'daterange' }, range);
    for (const el of [p, m]) {
      expect(el.textContent).toContain(local(2024, 3, 15));
      expect(el.textContent).toContain(local(2024, 4, 1));
    }
  });
});
