import { test, expect } from '../../fixtures/base.fixture';

const DEAD_URL = 'http://127.0.0.1:9/';

type Job = {
  id: string;
  state: string;
  title?: string;
  version: number;
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
  test('findings 41 and 113: paused progress stays visible and unknown totals keep a named indeterminate bar', async ({ page }) => {
    const acceptedAt = new Date().toISOString();
    const jobs = [
      { id: 'progress-paused', kind: 'remote-download', state: 'paused', version: 2,
        title: 'Paused transfer', acceptedAt, progress: { completed: 20, total: 50, unit: 'MB' } },
      { id: 'progress-unknown', kind: 'remote-download', state: 'running', version: 2,
        title: 'Unknown size transfer', acceptedAt, progress: { completed: 7, total: null, unit: 'bytes' } },
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
