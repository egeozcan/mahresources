import { afterEach, describe, expect, it, vi } from 'vitest';
import { adminImport } from './adminImport.js';
import reporterOwnerResult from './fixtures/import-r5-owner-result.json';

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
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((done, fail) => { resolve = done; reject = fail; });
  return { promise, resolve, reject };
}

const plan = (name: string) => ({
  source_instance_id: name,
  mappings: {}, dangling_refs: [], items: [],
  counts: { groups: 1, resources: 0, notes: 0, series: 0 },
  conflicts: { guid_matches: 0, resource_guid_matches: 0, resource_hash_matches: 0 },
});

async function startLiveApply(
  c: ReturnType<typeof adminImport>,
  readResult: () => Promise<Response> | Response,
) {
  const listeners: Record<string, (event: { data: string }) => void> = {};
  vi.stubGlobal('EventSource', class {
    addEventListener(type: string, listener: (event: { data: string }) => void) { listeners[type] = listener; }
    close() {}
  });
  const fetchMock = vi.fn(async (url: string) => {
    if (url === '/v1/imports/imp-1/apply') {
      return new Response(JSON.stringify({ jobId: 'queue-handle', canonicalJobId: 'canonical-apply-1' }), { status: 202 });
    }
    if (url === '/v1/imports/imp-1/result') return readResult();
    throw new Error(`unexpected request ${url}`);
  });
  vi.stubGlobal('fetch', fetchMock);
  c.jobId = 'imp-1';
  c.plan = plan('archive A');
  await c.apply();
  return {
    fetchMock,
    emit(job: unknown) {
      listeners.updated?.({ data: JSON.stringify({ job }) });
    },
  };
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('reopening an import an apply took', () => {
  it('preserves the server newest-first order for fractional times and equal-instant ties', async () => {
    const laterAt = '2026-09-28T05:00:00.11Z';
    const earlierAt = '2026-09-28T05:00:00.1Z';
    expect(Date.parse(laterAt) - Date.parse(earlierAt)).toBe(10);
    const fetchMock = serve({
      '/v1/jobs/parse-1': { status: 200, body: { id: 'parse-1', lineage: { children: [
        { id: 'apply-tie-z', kind: 'group-import-apply', acceptedAt: laterAt },
        { id: 'apply-tie-a', kind: 'group-import-apply', acceptedAt: laterAt },
        { id: 'apply-old', kind: 'group-import-apply', acceptedAt: earlierAt },
      ] } } },
      '/v1/jobs/apply-tie-z': { status: 200, body: { id: 'apply-tie-z', state: 'succeeded', lineage: { successors: [] } } },
      '/v1/jobs/apply-tie-a': { status: 200, body: { id: 'apply-tie-a', state: 'failed', failure: { message: 'same-instant lower ID' }, lineage: { successors: [] } } },
      '/v1/jobs/apply-old': { status: 200, body: { id: 'apply-old', state: 'failed', failure: { message: 'older apply failed' }, lineage: { successors: [] } } },
    });
    const c = adminImport();

    const apply = await c.latestApply('parse-1');

    expect(apply?.id).toBe('apply-tie-z');
    expect(apply?.state).toBe('succeeded');
    expect(fetchMock.mock.calls.map(([url]) => url)).toEqual([
      '/v1/jobs/parse-1', '/v1/jobs/apply-tie-z',
    ]);
  });

  it('uses the outcome attached to the report instead of selecting a visible Job', async () => {
    const fetchMock = serve({
      '/v1/imports/imp-1/result': { status: 200, body: { ...report, apply_outcome: 'succeeded' } },
    });
    const c = adminImport();
    await c.resumeApplied('imp-1');
    expect(c.applyOutcome).toBe('succeeded');
    expect(c.applyResult).toMatchObject(report);
    expect(c.error).toBeNull();
    expect(fetchMock.mock.calls.map(([url]) => url)).toEqual(['/v1/imports/imp-1/result']);
  });

  it('shows a report-bound unknown outcome', async () => {
    serve({
      '/v1/imports/imp-1/result': { status: 200, body: { ...report, apply_outcome: 'unknown' } },
    });
    const c = adminImport();
    await c.resumeApplied('imp-1');
    expect(c.applyOutcome).toBe('unknown');
    expect(c.applyResult).toMatchObject(report);
    expect(c.error).toBeNull();
  });

  it('shows the report-bound failed outcome and failure text', async () => {
    serve({
      '/v1/imports/imp-1/result': { status: 200, body: { ...report, apply_outcome: 'failed', apply_failure: 'the import could not be applied' } },
    });
    const c = adminImport();
    await c.resumeApplied('imp-1');
    expect(c.applyOutcome).toBe('failed');
    expect(c.error).toBe('the import could not be applied');
  });

  it('uses the report-bound unknown outcome instead of an older visible failed Apply', async () => {
    // The owner's Apply failed and restored its plan; an administrator's fresh
    // Apply then wrote this report, whose producer is hidden from the owner.
    const fetchMock = serve({ '/v1/imports/imp-1/result': { status: 200, body: reporterOwnerResult } });
    const c = adminImport();

    await c.resumeApplied('imp-1');

    expect(c.applyOutcome).toBe('unknown');
    expect(c.error).toBeNull();
    expect(c.applyResult).toMatchObject({ created_groups: 1, created_group_ids: [2] });
    expect(fetchMock.mock.calls.map(([url]) => url)).toEqual(['/v1/imports/imp-1/result']);
  });

  it('reports an unavailable outcome when there is no result to associate', async () => {
    serve({ '/v1/imports/imp-1/result': { status: 404, body: { error: 'not found' } } });
    const c = adminImport();
    await c.resumeApplied('imp-1');
    expect(c.applyOutcome).toBe('unknown');
    expect(c.applyResult).toBeNull();
    expect(c.resumeNotice).toContain('could not be verified');
  });

  it('reports a report that could not be read instead of calling the import removed', async () => {
    serve({
      '/v1/imports/imp-1/result': { status: 500, body: { error: 'disk unavailable' } },
    });
    const c = adminImport();
    await expect(c.resumeApplied('imp-1')).rejects.toThrow('The import report could not be read: disk unavailable');
    expect(c.resumeNotice).toBe('');
    expect(c.applyResult).toBeNull();
  });

  it('binds a live completion result request to the accepted canonical Apply', async () => {
    const listeners: Record<string, (event: { data: string }) => void> = {};
    vi.stubGlobal('EventSource', class {
      addEventListener(type: string, listener: (event: { data: string }) => void) { listeners[type] = listener; }
      close() {}
    });
    const fetchMock = vi.fn(async (_url: string, _init?: RequestInit) => new Response(JSON.stringify({
      ...report, apply_outcome: 'unknown',
    })));
    vi.stubGlobal('fetch', fetchMock);
    const c = adminImport();
    c.subscribeApplyProgress('queue-handle', 'imp-1', c._importGeneration, 'canonical-apply-1');

    listeners.updated({ data: JSON.stringify({ job: { id: 'queue-handle', status: 'completed' } }) });
    await new Promise(resolve => setTimeout(resolve, 0));

    expect(fetchMock).toHaveBeenCalledWith('/v1/imports/imp-1/result', {
      headers: { 'X-Expected-Import-Apply': 'canonical-apply-1' },
    });
    expect(c.applyOutcome).toBe('unknown');
    expect(c.error).toBeNull();
  });
});

describe('live Apply result reads', () => {
  it.each([
    ['HTTP failure', () => new Response('{"error":"disk unavailable"}', { status: 500 })],
    ['network failure', () => Promise.reject(new Error('offline'))],
    ['undecodable response', () => new Response('{not json', { status: 200 })],
  ])('keeps the accepted Job failure and announces a %s', async (_label, readResult) => {
    const c = adminImport();
    const live = await startLiveApply(c, readResult);

    live.emit({ id: 'queue-handle', status: 'failed', failure: { message: 'honest A failure' } });
    await new Promise(resolve => setTimeout(resolve, 0));

    expect(c.error).toBe('honest A failure');
    expect(c.applyReadNotice).toContain('report could not be read');
    expect(c.applyJobURL).toBe('/job?id=canonical-apply-1');
    expect(c.applyOutcome).toBe('');
    expect(c.applyResult).toBeNull();
  });

  it('keeps a completed Job separate from a failed result read', async () => {
    const c = adminImport();
    const live = await startLiveApply(c, () => new Response('unavailable', { status: 503 }));

    live.emit({ id: 'queue-handle', status: 'completed' });
    await new Promise(resolve => setTimeout(resolve, 0));

    expect(c.error).toBeNull();
    expect(c.applyOutcome).toBe('');
    expect(c.applyReadNotice).toContain('report could not be read');
    expect(c.applyJobURL).toBe('/job?id=canonical-apply-1');
  });

  it('shows the accepted Job failure when its result is truly missing', async () => {
    const c = adminImport();
    const live = await startLiveApply(c, () => new Response('{"error":"not found"}', { status: 404 }));

    live.emit({ id: 'queue-handle', status: 'failed', failure: { message: 'honest A failure' } });
    await new Promise(resolve => setTimeout(resolve, 0));

    expect(c.error).toBe('honest A failure');
    expect(c.applyOutcome).toBe('');
    expect(c.applyResult).toBeNull();
    expect(c.applyReadNotice).toContain('No Apply report is available');
    expect(c.applyJobURL).toBe('/job?id=canonical-apply-1');
  });

  it('does not attach the accepted Job failure to a newer unknown report', async () => {
    const c = adminImport();
    const live = await startLiveApply(c, () => new Response(JSON.stringify({
      created_groups: 1, created_group_ids: [42], apply_outcome: 'unknown',
    }), { status: 200 }));

    live.emit({ id: 'queue-handle', status: 'failed', failure: { message: 'honest A failure' } });
    await new Promise(resolve => setTimeout(resolve, 0));

    expect(live.fetchMock).toHaveBeenCalledWith('/v1/imports/imp-1/result', {
      headers: { 'X-Expected-Import-Apply': 'canonical-apply-1' },
    });
    expect(c.error).toBeNull();
    expect(c.applyOutcome).toBe('unknown');
    expect(c.applyResult).toMatchObject({ created_groups: 1, created_group_ids: [42] });
    expect(c.applyReadNotice).toBe('');
  });

  it('does not publish a delayed result decode failure after reset', async () => {
    const body = deferred<unknown>();
    const bodyStarted = deferred<void>();
    const c = adminImport();
    const live = await startLiveApply(c, () => ({
      ok: true,
      status: 200,
      json() {
        bodyStarted.resolve();
        return body.promise;
      },
    } as Response));

    live.emit({ id: 'queue-handle', status: 'failed', failure: { message: 'honest A failure' } });
    await bodyStarted.promise;
    c.resetImport();
    c.jobId = 'imp-2';
    c.error = 'B error';
    c.applyReadNotice = 'B report notice';
    c.applyJobURL = '/job?id=canonical-apply-2';
    body.reject(new Error('invalid JSON'));
    await new Promise(resolve => setTimeout(resolve, 0));

    expect(c.jobId).toBe('imp-2');
    expect(c.error).toBe('B error');
    expect(c.applyReadNotice).toBe('B report notice');
    expect(c.applyJobURL).toBe('/job?id=canonical-apply-2');
    expect(c.applyOutcome).toBe('');
    expect(c.applyResult).toBeNull();
  });

  it('does not publish a delayed HTTP result-read fault after reset', async () => {
    const response = deferred<Response>();
    const c = adminImport();
    const live = await startLiveApply(c, () => response.promise);

    live.emit({ id: 'queue-handle', status: 'failed', failure: { message: 'honest A failure' } });
    c.resetImport();
    c.jobId = 'imp-2';
    c.error = 'B error';
    c.applyReadNotice = 'B report notice';
    c.applyJobURL = '/job?id=canonical-apply-2';
    response.resolve(new Response('unavailable', { status: 500 }));
    await new Promise(resolve => setTimeout(resolve, 0));

    expect(c.jobId).toBe('imp-2');
    expect(c.error).toBe('B error');
    expect(c.applyReadNotice).toBe('B report notice');
    expect(c.applyJobURL).toBe('/job?id=canonical-apply-2');
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
      if (url === '/v1/imports/imp-a/result') { reportReadStarted(); return oldReport.promise; }
      if (url === '/v1/groups/import/parse') return new Response(JSON.stringify({ jobId: 'imp-b' }), { status: 202 });
      if (url === '/v1/imports/imp-b/plan') return new Response(JSON.stringify(plan('archive B')));
      throw new Error(`unexpected request GET ${url}`);
    });
    vi.stubGlobal('fetch', fetchMock);
    vi.stubGlobal('EventSource', class { addEventListener() {} close() {} });
    const c = adminImport();
    c.selectedFile = new Blob(['archive B']) as never;

    const restoringA = c.resumeApplied('imp-a');
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
