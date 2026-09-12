import type { BrowsePage, BrowseValue, EntityBrowseSource } from './entityBrowseTypes';

function record(value: unknown): value is Record<string, unknown> {
 return value !== null && typeof value === 'object' && !Array.isArray(value);
}
function validValue(value: unknown): value is BrowseValue {
 return record(value) && Number.isSafeInteger(value.ID) && Number(value.ID) > 0 && typeof value.Name === 'string';
}
function invalid(): never { throw new Error('Invalid entity browser response'); }
async function request(path: string, parameters: URLSearchParams, signal: AbortSignal): Promise<unknown> {
 const response = await fetch(`${path}?${parameters}`, { signal, headers: { Accept: 'application/json' } });
 if (!response.ok) throw new Error(`Entity browser request failed (${response.status})`);
 const body: unknown = await response.json();
 if (signal.aborted) throw new DOMException('Aborted', 'AbortError');
 return body;
}

export function createHttpEntityBrowseSource(): EntityBrowseSource {
 return {
  async search(input, signal) {
   const parameters = new URLSearchParams({ entity: input.entity, filter: input.filter, constraints: input.constraints, page: String(input.page) });
   const body = await request('/v1/entity-picker', parameters, signal);
   if (!record(body) || !Array.isArray(body.items) || body.items.length > 50 ||
       !Number.isSafeInteger(body.page) || Number(body.page) < 1 || typeof body.hasNext !== 'boolean' ||
       !Array.isArray(body.styles) || !Array.isArray(body.warnings) ||
       !body.items.every(item => record(item) && validValue(item.value) && typeof item.html === 'string') ||
       !body.styles.every(style => record(style) && typeof style.key === 'string' && typeof style.css === 'string') ||
       !body.warnings.every(warning => typeof warning === 'string')) invalid();
   return body as unknown as BrowsePage;
  },
  async resolve(input, signal) {
   const ids = [...new Set(input.ids)];
   if (ids.some(id => !Number.isSafeInteger(id) || id < 1)) invalid();
   const items: BrowseValue[] = [];
   for (let offset = 0; offset < ids.length; offset += 50) {
    const parameters = new URLSearchParams({ entity: input.entity, constraints: input.constraints });
    const batch = ids.slice(offset, offset + 50);
    for (const id of batch) parameters.append('id', String(id));
    const body = await request('/v1/entity-picker/resolve', parameters, signal);
    if (!record(body) || !Array.isArray(body.items) || body.items.length > batch.length ||
        !body.items.every(item => validValue(item) && batch.includes(item.ID))) invalid();
    items.push(...body.items as BrowseValue[]);
   }
   if (new Set(items.map(value => value.ID)).size !== items.length) invalid();
   return items;
  },
 };
}
