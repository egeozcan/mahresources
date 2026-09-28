import { afterEach, describe, expect, it, vi } from 'vitest';
import { adminImport } from './adminImport.js';

type Routes = Record<string, { status: number; body: unknown }>;

function serve(routes: Routes) {
  const fetchMock = vi.fn(async (url: string) => {
    const route = routes[url];
    if (!route) return new Response('{"error":"not found"}', { status: 404 });
    return new Response(JSON.stringify(route.body), {
      status: route.status,
      headers: { 'Content-Type': 'application/json' },
    });
  });
  vi.stubGlobal('fetch', fetchMock);
  return fetchMock;
}

const report = { created_groups: 1, created_group_ids: [7] };
const parseDetail = (children: unknown[]) => ({ status: 200, body: { id: 'parse-1', lineage: { children } } });

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('reopening an import an apply took', () => {
  it('shows the report as a success only when the apply this viewer can see succeeded', async () => {
    serve({
      '/v1/imports/imp-1/result': { status: 200, body: report },
      '/v1/jobs/parse-1': parseDetail([{ id: 'apply-1', kind: 'group-import-apply', state: 'succeeded', acceptedAt: '2026-09-28T00:00:00Z' }]),
      '/v1/jobs/apply-1': { status: 200, body: { id: 'apply-1', state: 'succeeded', lineage: { successors: [] } } },
    });
    const c = adminImport();
    await c.resumeApplied('imp-1', 'parse-1');
    expect(c.applyOutcome).toBe('succeeded');
    expect(c.applyResult).toEqual(report);
    expect(c.error).toBeNull();
  });

  it('says the outcome is not available when the viewer cannot see the apply', async () => {
    serve({
      '/v1/imports/imp-1/result': { status: 200, body: report },
      '/v1/jobs/parse-1': parseDetail([]),
    });
    const c = adminImport();
    await c.resumeApplied('imp-1', 'parse-1');
    expect(c.applyOutcome).toBe('unknown');
    expect(c.applyResult).toEqual(report);
    expect(c.error).toBeNull();
  });

  it('says the outcome is not available when another account retried the apply', async () => {
    serve({
      '/v1/imports/imp-1/result': { status: 200, body: report },
      '/v1/jobs/parse-1': parseDetail([{ id: 'apply-1', kind: 'group-import-apply', state: 'failed', acceptedAt: '2026-09-28T00:00:00Z' }]),
      '/v1/jobs/apply-1': { status: 200, body: { id: 'apply-1', state: 'failed', lineage: { successors: [], retriedElsewhere: true } } },
    });
    const c = adminImport();
    await c.resumeApplied('imp-1', 'parse-1');
    expect(c.applyOutcome).toBe('unknown');
    expect(c.error).toBeNull();
  });

  it('follows a Retry to the newest apply and reports its failure', async () => {
    serve({
      '/v1/imports/imp-1/result': { status: 200, body: report },
      '/v1/jobs/parse-1': parseDetail([{ id: 'apply-1', kind: 'group-import-apply', state: 'succeeded', acceptedAt: '2026-09-28T00:00:00Z' }]),
      '/v1/jobs/apply-1': { status: 200, body: { id: 'apply-1', state: 'succeeded', lineage: { successors: [{ id: 'apply-2', kind: 'group-import-apply', acceptedAt: '2026-09-28T00:05:00Z' }] } } },
      '/v1/jobs/apply-2': { status: 200, body: { id: 'apply-2', state: 'failed', failure: { message: 'the import could not be applied' }, lineage: { successors: [] } } },
    });
    const c = adminImport();
    await c.resumeApplied('imp-1', 'parse-1');
    expect(c.applyOutcome).toBe('failed');
    expect(c.error).toBe('the import could not be applied');
  });

  it('reports a report that could not be read instead of calling the import removed', async () => {
    serve({
      '/v1/imports/imp-1/result': { status: 500, body: { error: 'disk unavailable' } },
      '/v1/jobs/parse-1': parseDetail([]),
    });
    const c = adminImport();
    await expect(c.resumeApplied('imp-1', 'parse-1')).rejects.toThrow('The import report could not be read: disk unavailable');
    expect(c.resumeNotice).toBe('');
    expect(c.applyResult).toBeNull();
  });
});
