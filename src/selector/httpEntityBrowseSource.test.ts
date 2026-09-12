import { afterEach, describe, expect, it, vi } from 'vitest';
import { createHttpEntityBrowseSource } from './httpEntityBrowseSource';

afterEach(() => vi.unstubAllGlobals());
const page = { items: [{ value: { ID: 1, Name: 'First' }, html: '<b>First</b>' }], page: 1, hasNext: false, styles: [], warnings: [] };
const input = { entity: 'group' as const, filter: 'Categories=2&Tags=3&Tags=4', constraints: 'Categories=1', page: 1 };
describe('entity browse transport', () => {
 it('encodes independent filters once and passes the abort signal', async () => {
  const fetch = vi.fn(async () => new Response(JSON.stringify(page)));
  vi.stubGlobal('fetch', fetch);
  const signal = new AbortController().signal;
  expect(await createHttpEntityBrowseSource().search(input, signal)).toEqual(page);
  const params = new URL(String(fetch.mock.calls[0][0]), 'http://localhost').searchParams;
  expect(params.get('filter')).toBe(input.filter);
  expect(params.get('constraints')).toBe(input.constraints);
  expect(fetch.mock.calls[0][1].signal).toBe(signal);
 });
 it.each([{ ...page, items: {} }, { ...page, page: 0 }, { ...page, hasNext: 'yes' }, { ...page, items: [{ value: { ID: 1 }, html: '' }] }, { ...page, styles: [{}] }])('refuses malformed envelopes', async (body) => {
  vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify(body))));
  await expect(createHttpEntityBrowseSource().search(input, new AbortController().signal)).rejects.toThrow();
 });
 it('reports non-JSON and HTTP errors', async () => {
  const fetch = vi.fn().mockResolvedValueOnce(new Response('broken')).mockResolvedValueOnce(new Response('denied', { status: 403 }));
  vi.stubGlobal('fetch', fetch);
  const source = createHttpEntityBrowseSource(), signal = new AbortController().signal;
  await expect(source.search(input, signal)).rejects.toThrow();
  await expect(source.search(input, signal)).rejects.toThrow('403');
 });
 it('resolves more than one page in bounded batches without partial results', async () => {
  const fetch = vi.fn(async (url) => {
   const ids = new URL(url, 'http://localhost').searchParams.getAll('id').map(Number);
   expect(ids.length).toBeLessThanOrEqual(50);
   return new Response(JSON.stringify({ items: ids.map(ID => ({ ID, Name: String(ID) })) }));
  });
  vi.stubGlobal('fetch', fetch);
  const ids = Array.from({ length: 101 }, (_, i) => i + 1);
  expect((await createHttpEntityBrowseSource().resolve({ entity: 'group', constraints: '', ids }, new AbortController().signal)).map(v => v.ID)).toEqual(ids);
  expect(fetch).toHaveBeenCalledTimes(3);
 });
});
