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

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>(done => { resolve = done; });
  return { promise, resolve };
}

const plan = (name: string) => ({
  source_instance_id: name,
  mappings: {}, dangling_refs: [], items: [],
  counts: { groups: 1, resources: 0, notes: 0, series: 0 },
  conflicts: { guid_matches: 0, resource_guid_matches: 0, resource_hash_matches: 0 },
});

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

  it('ignores a bookmarked import response after a new upload owns the page', async () => {
    const oldPlan = deferred<Response>();
    const fetchMock = vi.fn(async (url: string, init?: RequestInit) => {
      if (url === '/v1/imports/imp-a/plan') return oldPlan.promise;
      if (url === '/v1/groups/import/parse') return new Response(JSON.stringify({ jobId: 'imp-b' }), { status: 202 });
      if (url === '/v1/imports/imp-b/plan') return new Response(JSON.stringify(plan('archive B')));
      throw new Error(`unexpected request ${init?.method || 'GET'} ${url}`);
    });
    vi.stubGlobal('fetch', fetchMock);
    vi.stubGlobal('EventSource', class { addEventListener() {} close() {} });
    const c = adminImport();
    c.selectedFile = new Blob(['archive B']) as never;

    const restoringA = c.resume('imp-a');
    await c.upload();
    await c.onParseComplete('imp-b');
    oldPlan.resolve(new Response(JSON.stringify(plan('archive A'))));
    await restoringA;

    expect(c.jobId).toBe('imp-b');
    expect(c.plan?.source_instance_id).toBe('archive B');
  });

  it('ignores a reopened import report that arrives after reset', async () => {
    const oldReport = deferred<Response>();
    let reportReadStarted!: () => void;
    const reportStarted = new Promise<void>(done => { reportReadStarted = done; });
    const fetchMock = vi.fn(async (url: string) => {
      if (url === '/v1/jobs/parse-a') return new Response(JSON.stringify({ id: 'parse-a', lineage: { children: [] } }));
      if (url === '/v1/imports/imp-a/result') { reportReadStarted(); return oldReport.promise; }
      if (url === '/v1/groups/import/parse') return new Response(JSON.stringify({ jobId: 'imp-b' }), { status: 202 });
      if (url === '/v1/imports/imp-b/plan') return new Response(JSON.stringify(plan('archive B')));
      throw new Error(`unexpected request GET ${url}`);
    });
    vi.stubGlobal('fetch', fetchMock);
    vi.stubGlobal('EventSource', class { addEventListener() {} close() {} });
    const c = adminImport();
    c.selectedFile = new Blob(['archive B']) as never;

    const restoringA = c.resumeApplied('imp-a', 'parse-a');
    await reportStarted;
    await c.upload();
    await c.onParseComplete('imp-b');
    oldReport.resolve(new Response(JSON.stringify(report)));
    await restoringA;

    expect(c.jobId).toBe('imp-b');
    expect(c.plan?.source_instance_id).toBe('archive B');
    expect(c.applyResult).toBeNull();
    expect(c.applyOutcome).toBe('');
  });

  it('rechecks the plan after a completed parse before resolving it as consumed', async () => {
    let planReads = 0;
    const fetchMock = vi.fn(async (url: string) => {
      if (url === '/v1/imports/imp-transition/plan') {
        planReads++;
        return planReads === 1
          ? new Response('{}', { status: 404 })
          : new Response(JSON.stringify(plan('newly completed parse')));
      }
      if (url === '/v1/jobs/get?id=imp-transition') {
        return new Response(JSON.stringify({ status: 'completed', canonicalJobId: 'parse-transition' }));
      }
      throw new Error(`unexpected request GET ${url}`);
    });
    vi.stubGlobal('fetch', fetchMock);

    const c = adminImport();
    await c.resume('imp-transition');

    expect(planReads).toBe(2);
    expect(c.jobId).toBe('imp-transition');
    expect(c.plan?.source_instance_id).toBe('newly completed parse');
    expect(c.resumeNotice).toBe('');
  });

  it('does not let an older apply acceptance install state after reset', async () => {
    const oldApply = deferred<Response>();
    const fetchMock = vi.fn(async (url: string) => {
      if (url === '/v1/imports/imp-a/apply') return oldApply.promise;
      throw new Error(`unexpected request GET ${url}`);
    });
    vi.stubGlobal('fetch', fetchMock);
    const c = adminImport();
    c.jobId = 'imp-a';
    c.plan = plan('archive A');

    const applyingA = c.apply();
    c.resetImport();
    oldApply.resolve(new Response(JSON.stringify({ jobId: 'apply-a' }), { status: 202 }));
    await applyingA;

    expect(c.applyJobId).toBeNull();
    expect(c.applying).toBe(false);
  });

  it('does not let a destination search repopulate keys after reset', async () => {
    const oldSearch = deferred<Response>();
    const entry = { decision_key: 'category:old', suggestion: 'create' };
    vi.stubGlobal('fetch', vi.fn(async () => oldSearch.promise));
    const c = adminImport();
    c.plan = { ...plan('archive A'), mappings: { categories: [entry] } };

    const searching = c.searchMappingDest(entry, 'old destination');
    c.resetImport();
    oldSearch.resolve(new Response(JSON.stringify([{ id: 7, name: 'Old destination' }])));
    await searching;

    expect(c.mappingSearchResults).toEqual({});
  });

  it('does not let a closed parse stream change or close the replacement stream', () => {
    const sources: Array<{ listeners: Record<string, (event: { data: string }) => void>; closed: boolean }> = [];
    vi.stubGlobal('EventSource', class {
      listeners: Record<string, (event: { data: string }) => void> = {};
      closed = false;
      constructor() { sources.push(this); }
      addEventListener(type: string, listener: (event: { data: string }) => void) { this.listeners[type] = listener; }
      close() { this.closed = true; }
      emit(type: string, job: unknown) { this.listeners[type]?.({ data: JSON.stringify({ job }) }); }
    });
    const c = adminImport();
    c.subscribeProgress('imp-a');
    const sourceA = sources[0];
    c.resetImport();
    c.jobId = 'imp-b';
    c.subscribeProgress('imp-b');
    const sourceB = sources[1];

    sourceA.emit('updated', { id: 'imp-a', status: 'completed' });

    expect(c.job).toBeNull();
    expect(sourceB.closed).toBe(false);
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
