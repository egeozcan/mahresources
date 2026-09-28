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

describe('a second import on the same page', () => {
  it('starts from nothing the first import left behind', async () => {
    serve({ '/v1/groups/import/parse': { status: 202, body: { jobId: 'imp-2' } } });
    vi.stubGlobal('EventSource', class { addEventListener() {} close() {} });
    const c = adminImport();
    c.applyResult = report;
    c.applyOutcome = 'succeeded';
    c.applyJobId = 'apply-1';
    c.error = 'an old failure';
    c.resumeNotice = 'an old notice';
    c.decisions.excluded_items.push('g0001');
    c.decisions.mapping_actions['category:A'] = { include: true, action: 'create' };
    c.selectedFile = new Blob(['archive']) as never;

    await c.upload();

    expect(c.jobId).toBe('imp-2');
    expect(c.applyResult).toBeNull();
    expect(c.applyOutcome).toBe('');
    expect(c.applyJobId).toBeNull();
    expect(c.error).toBeNull();
    expect(c.resumeNotice).toBe('');
    expect(c.decisions.excluded_items).toEqual([]);
    expect(c.decisions.mapping_actions).toEqual({});
  });
});

describe('finding the apply a reopened import describes', () => {
  it('follows a Retry chain of any length to its newest apply', async () => {
    const routes: Routes = {
      '/v1/jobs/parse-1': parseDetail([{ id: 'apply-0', kind: 'group-import-apply', state: 'failed', acceptedAt: '2026-09-28T00:00:00Z' }]),
    };
    for (let i = 0; i < 15; i++) {
      routes[`/v1/jobs/apply-${i}`] = { status: 200, body: { id: `apply-${i}`, state: 'failed', lineage: { successors: [
        { id: `apply-${i + 1}`, kind: 'group-import-apply', acceptedAt: `2026-09-28T00:${String(i + 1).padStart(2, '0')}:00Z` },
      ] } } };
    }
    routes['/v1/jobs/apply-15'] = { status: 200, body: { id: 'apply-15', state: 'succeeded', lineage: { successors: [] } } };
    serve(routes);
    const apply = await adminImport().latestApply('parse-1');
    expect(apply?.id).toBe('apply-15');
    expect(apply?.state).toBe('succeeded');
  });

  it('reports a Job read that failed instead of calling the apply invisible', async () => {
    serve({
      '/v1/jobs/parse-1': parseDetail([{ id: 'apply-1', kind: 'group-import-apply', state: 'succeeded', acceptedAt: '2026-09-28T00:00:00Z' }]),
      '/v1/jobs/apply-1': { status: 500, body: { error: 'database is locked' } },
    });
    await expect(adminImport().latestApply('parse-1')).rejects.toThrow('database is locked');
  });
});
