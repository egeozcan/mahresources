import { test, expect } from '../../fixtures/base.fixture';

const DEAD_URL = 'http://127.0.0.1:9/';

type Job = {
  id: string;
  state: string;
  title?: string;
  version: number;
  pinned?: boolean;
  commands?: Array<{ key: string; endpoint: string; jobVersion: number }>;
  lineage?: { ancestors?: Array<{ id: string }> };
};

async function createGroup(request: import('@playwright/test').APIRequestContext, name: string) {
  const response = await request.post('/v1/group', { data: { Name: name } });
  expect(response.ok(), await response.text()).toBe(true);
  const group = await response.json();
  return (group.ID ?? group.id) as number;
}

async function submitFailingDownload(
  request: import('@playwright/test').APIRequestContext,
  groupId: number,
  name: string,
) {
  const response = await request.post('/v1/download/submit', {
    data: { URL: `${DEAD_URL}${name}`, OwnerId: groupId, Name: name },
  });
  expect(response.status(), await response.text()).toBe(202);
  const body = await response.json();
  const accepted = body.jobs?.[0];
  expect(accepted?.id).toEqual(expect.any(String));
  expect(accepted?.canonicalJobId).toEqual(expect.any(String));
  return { legacyId: accepted.id as string, canonicalId: accepted.canonicalJobId as string };
}

async function readJob(request: import('@playwright/test').APIRequestContext, id: string): Promise<Job | null> {
  const response = await request.get(`/v1/jobs/${encodeURIComponent(id)}`);
  if (!response.ok()) return null;
  return response.json();
}

async function waitForJobState(
  request: import('@playwright/test').APIRequestContext,
  id: string,
  state: string,
) {
  await expect.poll(async () => (await readJob(request, id))?.state ?? null, {
    timeout: 20_000,
    message: `job ${id} did not reach ${state}`,
  }).toBe(state);
}

test.describe('Job Center', () => {
  test('requests a filtered summary with the current list filters and Any dismissal scope', async ({ page }) => {
    let summaryRequestURL = '';
    await page.route('**/v1/jobs/summary**', async route => {
      summaryRequestURL = route.request().url();
      await route.fulfill({ json: { byState: {} } });
    });
    await page.route(/\/v1\/jobs(?:\?.*)?$/, route => route.fulfill({ json: { jobs: [], nextCursor: null } }));

    await page.goto('/jobs?view=all&search=archive&command=retry&kind=remote-download&state=failed&origin=api&ownerId=12&actorId=13&acceptedAfter=2026-09-01T00%3A00%3A00Z&acceptedBefore=2026-09-20T00%3A00%3A00Z&relationship=retry-of&pinned=true');

    await expect(page.getByTestId('job-center')).toBeVisible();
    await expect.poll(() => summaryRequestURL).not.toBe('');
    const summary = new URL(summaryRequestURL).searchParams;
    expect(summary.get('search')).toBe('archive');
    expect(summary.get('command')).toBe('retry');
    expect(summary.get('kind')).toBe('remote-download');
    expect(summary.get('state')).toBe('failed');
    expect(summary.get('origin')).toBe('api');
    expect(summary.get('ownerId')).toBe('12');
    expect(summary.get('actorId')).toBe('13');
    expect(summary.get('acceptedAfter')).toBe('2026-09-01T00:00:00.000Z');
    expect(summary.get('acceptedBefore')).toBe('2026-09-20T00:00:00.000Z');
    expect(summary.get('relationship')).toBe('retry-of');
    expect(summary.get('pinned')).toBe('true');
    expect(summary.has('dismissed')).toBe(false);
    expect(summary.has('cursor')).toBe(false);
    expect(summary.has('limit')).toBe(false);
  });

  test('Dismissed Any shows previously dismissed jobs in All jobs', async ({ page }) => {
    const dismissedJob = {
      id: 'dismissed-download', kind: 'remote-download', state: 'failed', version: 1,
      title: 'Dismissed download', dismissed: true, acceptedAt: new Date().toISOString(),
    };
    const listURLs: URL[] = [];
    await page.route('**/v1/jobs/summary**', route => {
      const hidden = new URL(route.request().url()).searchParams.get('dismissed') === 'false';
      return route.fulfill({ json: { byState: { failed: hidden ? 0 : 1 } } });
    });
    await page.route(/\/v1\/jobs(?:\?.*)?$/, route => {
      const url = new URL(route.request().url());
      listURLs.push(url);
      return route.fulfill({ json: { jobs: url.searchParams.get('dismissed') === 'false' ? [] : [dismissedJob] } });
    });

    await page.goto('/jobs?view=all&dismissed=false');
    await expect(page.locator('[data-job-id="dismissed-download"]')).toHaveCount(0);
    await page.getByText('Filter jobs', { exact: true }).click();
    await page.getByRole('searchbox', { name: 'Search' }).fill('download');
    await page.getByRole('combobox', { name: 'Dismissed' }).selectOption('');
    await page.getByRole('button', { name: 'Apply filters' }).click();

    await expect(page.locator('[data-job-id="dismissed-download"]')).toBeVisible();
    expect(listURLs.some(url => !url.searchParams.has('dismissed'))).toBe(true);
    await page.getByRole('button', { name: 'Overview' }).click();
    await expect(page.locator('[data-job-id="dismissed-download"]')).toHaveCount(0);
    expect(new URL(page.url()).searchParams.has('search')).toBe(false);
  });

  test('findings 41 and 113: paused progress stays visible and unknown totals keep a named indeterminate bar', async ({ page }) => {
    const acceptedAt = new Date().toISOString();
    const jobs = [
      { id: 'progress-paused', kind: 'remote-download', state: 'paused', version: 2,
        title: 'Paused transfer', acceptedAt, progress: { completed: 20, total: 50, unit: 'MB' } },
      { id: 'progress-unknown', kind: 'remote-download', state: 'running', version: 2,
        title: 'Unknown size transfer', acceptedAt, progress: { completed: 7, total: null, unit: 'bytes' } },
      // A transfer that never learned its size and then finished keeps that last
      // row: it must read as done, not pulse "In progress" forever.
      { id: 'progress-finished-unknown', kind: 'remote-download', state: 'succeeded', version: 3,
        title: 'Finished unknown size transfer', acceptedAt, progress: {} },
    ];
    await page.route(/\/v1\/jobs(?:\?.*)?$/, route => route.fulfill({ json: { jobs, nextCursor: null } }));
    await page.route('**/v1/jobs/summary', route => route.fulfill({ json: { byState: { paused: 1, running: 1 } } }));

    await page.goto('/jobs?view=all');
    const paused = page.locator('[data-job-id="progress-paused"]');
    await expect(paused).toContainText('Paused');
    await expect(paused.getByRole('progressbar', { name: /Paused transfer progress/ })).toHaveAttribute('aria-valuenow', '40');

    const unknown = page.locator('[data-job-id="progress-unknown"]');
    const bar = unknown.getByRole('progressbar', { name: /Unknown size transfer progress/ });
    await expect(bar).toBeVisible();
    await expect(bar).not.toHaveAttribute('aria-valuenow', /.+/);
    await expect(bar).toHaveAttribute('aria-valuetext', /7 bytes processed; total unknown/);
    await expect(unknown.locator('.animate-pulse')).toHaveCount(1);
    await expect(unknown).toContainText('In progress');

    const finished = page.locator('[data-job-id="progress-finished-unknown"]');
    const finishedBar = finished.getByRole('progressbar', { name: /Finished unknown size transfer progress/ });
    await expect(finishedBar).toHaveAttribute('aria-valuenow', '100');
    await expect(finishedBar).toHaveAttribute('aria-valuetext', 'Completed');
    await expect(finished.locator('.animate-pulse')).toHaveCount(0);
    await expect(finished).not.toContainText('In progress');
  });

  test('a background download from the create form reaches the panel and /jobs with a link to its resource', async ({ page, request, baseURL }) => {
    const stamp = Date.now();
    // A fresh owner per run: a hash collision under another owner still succeeds
    // (it returns the existing Resource), so a shared server or a retry cannot turn
    // this into a failed download.
    const groupId = await createGroup(request, `job-center-form-download-${stamp}`);
    const source = `${baseURL}/public/favicon/ms-icon-150x150.png`;
    await page.goto(`/resource/new?OwnerId=${groupId}&URL=${encodeURIComponent(source)}`);
    await page.getByLabel('Download in background').check();
    await page.locator('form[x-data="resourceUpload()"] button[type="submit"]').click();
    await expect(page).toHaveURL(new RegExp(`/group\\?id=${groupId}$`));

    // The form's background path must accept a durable Job; before, it went to the
    // queue alone and the download never appeared in the Jobs panel or /jobs.
    await expect.poll(async () => {
      const response = await request.get('/v1/jobs?kind=remote-download&state=succeeded&limit=50');
      if (!response.ok()) return false;
      const body = await response.json();
      return (body.jobs as Job[]).some(candidate => candidate.title === `Download from ${new URL(source).host}`);
    }, { timeout: 20_000 }).toBe(true);

    const trigger = page.getByRole('button', { name: 'Open Jobs panel' });
    await trigger.click();
    const panel = page.getByRole('dialog', { name: 'Jobs' });
    const panelLink = panel.getByRole('link', { name: /^View created resource for Download from / }).first();
    await expect(panelLink).toBeVisible();
    await panelLink.click();
    await expect(page).toHaveURL(/\/resource\?id=\d+$/);
    const resourceURL = page.url();

    await page.goto('/jobs');
    const listLink = page.getByRole('link', { name: /^View created resource for Download from / }).first();
    await expect(listLink).toBeVisible();
    await listLink.click();
    await expect(page).toHaveURL(/\/resource\?id=\d+$/);
    expect(page.url()).toBe(resourceURL);
  });

  test('the legacy Downloads page redirects to the canonical list with compatible filters', async ({ page }) => {
    await page.route(/\/v1\/jobs(?:\?.*)?$/, route => route.fulfill({ json: { jobs: [], nextCursor: 'next-download-page' } }));
    const response = await page.goto('/downloads?URL=legacy-search&Status=failed&CreatedAfter=2026-09-01');

    expect(response?.status()).toBe(200);
    await expect(page).toHaveURL(/\/jobs\?/);
    const url = new URL(page.url());
    expect(url.searchParams.get('kind')).toBe('remote-download');
    expect(url.searchParams.get('state')).toBe('failed');
    expect(url.searchParams.get('search')).toBe('legacy-search');
    expect(url.searchParams.get('acceptedAfter')).toBe('2026-09-01T00:00:00Z');
    await expect(page.getByTestId('job-center')).toBeVisible();
    await expect(page.getByRole('button', { name: 'Load more jobs' })).toBeVisible();
  });

  test('lists a failed job, opens its detail, and follows the advertised Retry successor', async ({ page, request }) => {
    const stamp = Date.now();
    const name = `job-center-retry-${stamp}.bin`;
    const groupId = await createGroup(request, `job-center-retry-${stamp}`);
    const { canonicalId } = await submitFailingDownload(request, groupId, name);
    await waitForJobState(request, canonicalId, 'failed');

    await page.goto('/jobs?view=all');
    const row = page.locator(`[data-job-id="${canonicalId}"]`);
    await expect(row).toBeVisible();
    await expect(row).toContainText('Failed');
    const original = await readJob(request, canonicalId);
    expect(original?.title).toBeTruthy();
    await expect(row).toContainText(original!.title!);
    await row.getByRole('link', { name: new RegExp(`^Open job ${original!.title}$`) }).first().click();

    await expect(page).toHaveURL(new RegExp(`/job\\?id=${canonicalId}$`));
    const detail = page.getByTestId('job-detail');
    await expect(detail.getByRole('heading', { name: original!.title!, exact: true })).toBeVisible();
    await expect(detail.getByRole('heading', { name: 'Job context' })).toBeVisible();
    await expect(detail.getByRole('heading', { name: 'Outputs' })).toBeVisible();
    await expect(detail.getByRole('heading', { name: 'Timeline' })).toBeVisible();

    await detail.getByRole('button', { name: 'Retry', exact: true }).click();
    const detailJobId = detail.locator('header span.font-mono.text-xs');
    await expect(detailJobId).not.toHaveText(canonicalId);
    const successorId = (await detailJobId.textContent())?.trim();
    expect(successorId).toEqual(expect.any(String));
    expect(successorId).not.toBe(canonicalId);
    await expect(page).toHaveURL(new RegExp(`/job\\?id=${successorId}$`));

    const successor = await readJob(request, successorId!);
    expect(successor?.lineage?.ancestors?.some(job => job.id === canonicalId)).toBe(true);
    await expect(page.getByTestId('job-detail').getByRole('heading', { name: original!.title!, exact: true })).toBeVisible();
  });

  test('shows the viewer pin in the list, detail, and panel and lets them unpin', async ({ page, request }) => {
    const stamp = Date.now();
    const groupId = await createGroup(request, `job-center-pin-${stamp}`);
    const { canonicalId } = await submitFailingDownload(request, groupId, `job-center-pin-${stamp}.bin`);
    await waitForJobState(request, canonicalId, 'failed');

    const original = await readJob(request, canonicalId);
    expect(original?.pinned).toBe(false);
    const pin = original?.commands?.find(command => command.key === 'pin');
    expect(pin).toBeTruthy();
    const idempotencyKey = `job-center-pin-${stamp}`;
    const pinResponse = await request.post(pin!.endpoint, {
      headers: { 'Idempotency-Key': idempotencyKey },
      data: { expectedVersion: pin!.jobVersion, idempotencyKey },
    });
    expect(pinResponse.ok(), await pinResponse.text()).toBe(true);
    await expect.poll(async () => (await readJob(request, canonicalId))?.pinned).toBe(true);

    await page.goto('/jobs?view=all');
    const row = page.locator(`[data-job-id="${canonicalId}"]`);
    await expect(row.getByText('Pinned by you', { exact: true })).toBeVisible();

    await page.goto(`/job?id=${canonicalId}`);
    const detail = page.getByTestId('job-detail');
    await expect(detail.getByText('Pinned by you', { exact: true })).toBeVisible();
    await expect(detail.getByRole('button', { name: 'Unpin', exact: true })).toBeVisible();
    await expect(detail.getByRole('button', { name: 'Pin', exact: true })).toHaveCount(0);
    await page.reload();
    await expect(detail.getByText('Pinned by you', { exact: true })).toBeVisible();

    await page.goto('/dashboard');
    await page.getByRole('button', { name: 'Open Jobs panel' }).click();
    const panel = page.getByRole('dialog', { name: 'Jobs' });
    const card = panel.locator('article').filter({ has: page.locator(`a[href="/job?id=${canonicalId}"]`) });
    await expect(card.getByText('Pinned by you', { exact: true })).toBeVisible();

    await page.goto(`/job?id=${canonicalId}`);
    await detail.getByRole('button', { name: 'Unpin', exact: true }).click();
    await expect(detail.getByText('Pinned by you', { exact: true })).toBeHidden();
    await expect(detail.getByRole('button', { name: 'Pin', exact: true })).toBeVisible();
    await expect.poll(async () => (await readJob(request, canonicalId))?.pinned).toBe(false);
  });

  test('the shared panel lists recent work and links to detail and All jobs', async ({ page, request }) => {
    const stamp = Date.now();
    const name = `job-center-panel-${stamp}.bin`;
    const groupId = await createGroup(request, `job-center-panel-${stamp}`);
    const { canonicalId } = await submitFailingDownload(request, groupId, name);
    await waitForJobState(request, canonicalId, 'failed');

    await page.goto('/dashboard');
    const trigger = page.getByRole('button', { name: 'Open Jobs panel' });
    await trigger.click();
    const panel = page.getByRole('dialog', { name: 'Jobs' });
    await expect(panel).toBeVisible();
    await expect(panel.locator(`a[href="/job?id=${canonicalId}"]`).first()).toBeVisible();
    await expect(panel).toContainText('Failed');
    await panel.getByRole('link', { name: 'All jobs', exact: true }).click();
    await expect(page).toHaveURL('/jobs');
    await expect(page.getByTestId('job-center')).toBeVisible();
  });

  test('the panel shows older actionable jobs and labels counts as shown rows', async ({ page }) => {
    const olderJob = {
      id: 'panel-older-active-job',
      kind: 'maintenance',
      state: 'running',
      version: 1,
      title: 'Older active job',
      acceptedAt: '2020-01-01T00:00:00Z',
      commands: [{ key: 'pause', label: 'Pause', endpoint: '/v1/jobs/panel-older-active-job/commands/pause', jobVersion: 1 }],
    };
    const listURLs: URL[] = [];
    let summaryRequests = 0;
    await page.route('**/v1/jobs/summary**', async route => {
      summaryRequests += 1;
      await route.fulfill({ json: { byState: { running: 20, failed: 10 } } });
    });
    await page.route(/\/v1\/jobs(?:\?.*)?$/, route => {
      const url = new URL(route.request().url());
      if (url.pathname !== '/v1/jobs') return route.continue();
      listURLs.push(url);
      return route.fulfill({ json: { jobs: url.searchParams.getAll('state').includes('running') ? [olderJob] : [] } });
    });

    await page.goto('/dashboard');
    await page.getByRole('button', { name: 'Open Jobs panel' }).click();

    const panel = page.getByRole('dialog', { name: 'Jobs' });
    await expect(panel.getByRole('link', { name: 'Older active job', exact: true })).toBeVisible();
    await expect(panel.getByText('Active and scheduled jobs shown')).toBeVisible();
    await expect(panel.getByText('Active and scheduled jobs shown').locator('..').getByText('1', { exact: true })).toBeVisible();
    expect(listURLs.length).toBeGreaterThanOrEqual(3);
    expect(listURLs.length % 3).toBe(0);
    expect(listURLs.every(url => url.searchParams.get('dismissed') === 'false')).toBe(true);
    expect(listURLs.every(url => !url.searchParams.has('acceptedAfter'))).toBe(true);
    expect(summaryRequests).toBe(0);
  });

  test('legacy download and job-handle APIs still resolve canonical work', async ({ request }) => {
    const stamp = Date.now();
    const name = `job-center-compat-${stamp}.bin`;
    const groupId = await createGroup(request, `job-center-compat-${stamp}`);
    const { legacyId, canonicalId } = await submitFailingDownload(request, groupId, name);
    await waitForJobState(request, canonicalId, 'failed');

    await expect.poll(async () => {
      const response = await request.get('/v1/downloads');
      if (!response.ok()) return null;
      const body = await response.json();
      return (body.downloads as Array<{ jobId: string; canonicalJobId?: string }>)
        .find(entry => entry.jobId === legacyId)?.canonicalJobId ?? null;
    }, { timeout: 15_000 }).toBe(canonicalId);

    const legacyResponse = await request.get(`/v1/jobs/get?id=${encodeURIComponent(legacyId)}`);
    expect(legacyResponse.ok(), await legacyResponse.text()).toBe(true);
    expect(legacyResponse.headers()['deprecation']).toBeTruthy();
    expect(legacyResponse.headers()['link']).toContain('/v1/jobs');
    const legacyBody = await legacyResponse.json();
    expect(legacyBody.id).toBe(legacyId);
    expect(legacyBody.canonicalJobId).toBe(canonicalId);
  });
});
