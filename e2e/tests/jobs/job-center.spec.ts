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
    // FileName becomes the Job's title, which is what a test searches the list by.
    data: { URL: `${DEAD_URL}${name}`, OwnerId: groupId, Name: name, FileName: name },
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
  test('renders the list on the server, readable without JavaScript', async ({ browser, request, baseURL }) => {
    const stamp = Date.now();
    const name = `job-center-nojs-${stamp}.bin`;
    const groupId = await createGroup(request, `job-center-nojs-${stamp}`);
    const { canonicalId } = await submitFailingDownload(request, groupId, name);
    await waitForJobState(request, canonicalId, 'failed');

    const context = await browser.newContext({ baseURL, javaScriptEnabled: false });
    try {
      const page = await context.newPage();
      await page.goto(`/jobs?search=${encodeURIComponent(name)}`);
      const row = page.locator(`[data-job-id="${canonicalId}"]`);
      await expect(row).toBeVisible();
      await expect(row.getByRole('link', { name, exact: true })).toHaveAttribute('href', `/job?id=${canonicalId}`);
      await expect(row.getByTestId('job-state')).toHaveText('Failed');
      // The filters are an ordinary GET form in the standard sidebar.
      await expect(page.getByRole('form', { name: 'Filter jobs' })).toBeVisible();
      await expect(page.getByRole('searchbox', { name: 'Search' })).toHaveValue(name);
    } finally {
      await context.close();
    }
  });

  test('the sidebar form round-trips the canonical filter parameters', async ({ page }) => {
    await page.goto('/jobs?search=archive&command=retry&kind=remote-download&state=failed&state=blocked&origin=api&origin=plugin&ownerId=12&actorId=13&acceptedAfter=2026-09-01T14%3A30&acceptedBefore=2026-09-20&relationship=retry-of&pinned=true&dismissed=any');

    const form = page.getByRole('form', { name: 'Filter jobs' });
    await expect(form.getByRole('searchbox', { name: 'Search' })).toHaveValue('archive');
    await expect(form.getByRole('combobox', { name: 'Available command' })).toHaveValue('retry');
    await expect(form.getByRole('checkbox', { name: 'remote-download' })).toBeChecked();
    await expect(form.getByRole('checkbox', { name: 'failed' })).toBeChecked();
    await expect(form.getByRole('checkbox', { name: 'blocked' })).toBeChecked();
    await expect(form.getByRole('checkbox', { name: 'queued' })).not.toBeChecked();
    await expect(form.getByRole('searchbox', { name: 'Origin' })).toHaveValue('api, plugin');
    await expect(form.getByLabel('Accepted after')).toHaveValue('2026-09-01T14:30');
    await expect(form.getByLabel('Accepted before')).toHaveValue('2026-09-20T23:59');
    await expect(form.getByRole('combobox', { name: 'Relationship' })).toHaveValue('retry-of');
    await expect(form.getByRole('combobox', { name: 'Pinned' })).toHaveValue('true');
    await expect(form.getByRole('combobox', { name: 'Dismissed' })).toHaveValue('any');

    await form.getByRole('checkbox', { name: 'blocked' }).uncheck();
    await form.getByRole('button', { name: 'Apply Filters' }).click();
    await expect(page).toHaveURL(/\/jobs\?/);
    const url = new URL(page.url());
    expect(url.searchParams.getAll('state')).toEqual(['failed']);
    expect(url.searchParams.get('kind')).toBe('remote-download');
    expect(url.searchParams.get('command')).toBe('retry');
    expect(url.searchParams.get('dismissed')).toBe('any');
    expect(url.searchParams.get('origin')).toBe('api, plugin');
    // With JavaScript the bounds travel as instants: an untouched one exactly as
    // it arrived, so neither the time of day nor the end of the range moves.
    const [after, before] = await page.evaluate(() => [
      new Date('2026-09-01T14:30').getTime(),
      new Date('2026-09-21T00:00').getTime() - 1,
    ]);
    expect(new Date(url.searchParams.get('acceptedAfter')!).getTime()).toBe(after);
    expect(new Date(url.searchParams.get('acceptedBefore')!).getTime()).toBe(before);
    await expect(form.getByLabel('Accepted after')).toHaveValue('2026-09-01T14:30');
    await expect(form.getByLabel('Accepted before')).toHaveValue('2026-09-20T23:59');
    await expect(page.getByRole('alert')).toHaveCount(0);
  });

  test('Dismissed Any shows a job the viewer dismissed, which the default list hides', async ({ page, request }) => {
    const stamp = Date.now();
    const name = `job-center-dismissed-${stamp}.bin`;
    const groupId = await createGroup(request, `job-center-dismissed-${stamp}`);
    const { canonicalId } = await submitFailingDownload(request, groupId, name);
    await waitForJobState(request, canonicalId, 'failed');
    const job = await readJob(request, canonicalId);
    const dismiss = job?.commands?.find(command => command.key === 'dismiss');
    expect(dismiss).toBeTruthy();
    const key = `job-center-dismiss-${stamp}`;
    const response = await request.post(dismiss!.endpoint, {
      headers: { 'Idempotency-Key': key },
      data: { expectedVersion: dismiss!.jobVersion, idempotencyKey: key },
    });
    expect(response.ok(), await response.text()).toBe(true);

    await page.goto(`/jobs?search=${encodeURIComponent(name)}`);
    await expect(page.locator(`[data-job-id="${canonicalId}"]`)).toHaveCount(0);
    await expect(page.getByText('No jobs match these filters.')).toBeVisible();

    await page.getByRole('combobox', { name: 'Dismissed' }).selectOption('any');
    await page.getByRole('button', { name: 'Apply Filters' }).click();
    await expect(page.locator(`[data-job-id="${canonicalId}"]`)).toBeVisible();
    expect(new URL(page.url()).searchParams.get('dismissed')).toBe('any');
  });

  test('quick filters count the rows they open and toggle their State filter', async ({ page, request }) => {
    const stamp = Date.now();
    const name = `job-center-quick-${stamp}`;
    const groupId = await createGroup(request, name);
    const first = await submitFailingDownload(request, groupId, `${name}-a.bin`);
    const second = await submitFailingDownload(request, groupId, `${name}-b.bin`);
    await waitForJobState(request, first.canonicalId, 'failed');
    await waitForJobState(request, second.canonicalId, 'failed');

    await page.goto(`/jobs?search=${encodeURIComponent(name)}`);
    const attention = page.locator('[data-job-quick-filter="attention"]');
    await expect(attention).toContainText('Needs attention (2)');
    await expect(page.locator('[data-job-quick-filter="active"]')).toContainText('Active (0)');
    await expect(page.locator('[data-job-quick-filter="finished"]')).toContainText('Finished (2)');

    await attention.click();
    const url = new URL(page.url());
    expect(url.searchParams.get('search')).toBe(name);
    expect(url.searchParams.getAll('state').sort()).toEqual(['blocked', 'failed', 'interrupted']);
    await expect(page.locator('[data-job-quick-filter="attention"]')).toHaveAttribute('aria-current', 'true');
    await expect(page.locator('[data-job-id]')).toHaveCount(2);

    await page.locator('[data-job-quick-filter="attention"]').click();
    expect(new URL(page.url()).searchParams.getAll('state')).toEqual([]);
    await expect(page.locator('[data-job-quick-filter="attention"]')).not.toHaveAttribute('aria-current', /.+/);
  });

  test('pages with Previous and Next through the standard pagination bar', async ({ page, request }) => {
    test.slow();
    const stamp = Date.now();
    const name = `job-center-pages-${stamp}`;
    const groupId = await createGroup(request, name);
    const ids: string[] = [];
    for (let i = 0; i < 51; i++) {
      ids.push((await submitFailingDownload(request, groupId, `${name}-${String(i).padStart(2, '0')}.bin`)).canonicalId);
    }

    await page.goto(`/jobs?search=${encodeURIComponent(name)}`);
    await expect(page.locator('[data-job-id]')).toHaveCount(50);
    const nav = page.getByRole('navigation', { name: 'Pagination' });
    await expect(nav.getByRole('link', { name: 'Previous page' })).toHaveCount(0);
    await nav.getByRole('link', { name: 'Next page' }).click();

    await expect(page.locator('[data-job-id]')).toHaveCount(1);
    // Newest first, so the one row left is the first submitted.
    await expect(page.locator(`[data-job-id="${ids[0]}"]`)).toBeVisible();
    expect(new URL(page.url()).searchParams.get('search')).toBe(name);
    await expect(nav.getByRole('link', { name: 'Next page' })).toHaveCount(0);

    await nav.getByRole('link', { name: 'Previous page' }).click();
    await expect(page.locator('[data-job-id]')).toHaveCount(50);
    await expect(page.locator(`[data-job-id="${ids[50]}"]`)).toBeVisible();
    await expect(nav.getByRole('link', { name: 'Previous page' })).toHaveCount(0);
  });

  test('a job accepted while the list is open appears without a reload', async ({ page, request }) => {
    const stamp = Date.now();
    const name = `job-center-live-${stamp}`;
    const groupId = await createGroup(request, name);

    await page.goto(`/jobs?search=${encodeURIComponent(name)}`);
    await expect(page.getByTestId('job-live-status')).toHaveText('Live updates connected');
    await expect(page.locator('[data-job-id]')).toHaveCount(0);
    const attention = page.locator('[data-job-quick-filter="attention"]');
    await expect(attention).toContainText('Needs attention (0)');

    const { canonicalId } = await submitFailingDownload(request, groupId, `${name}.bin`);
    const row = page.locator(`[data-job-id="${canonicalId}"]`);
    await expect(row).toBeVisible({ timeout: 20_000 });
    await expect(row.getByTestId('job-state')).toHaveText('Failed', { timeout: 20_000 });
    await expect(attention).toContainText('Needs attention (1)');
  });

  test('bulk commands offer what every selected job advertises and report per-job outcomes', async ({ page, request }) => {
    const stamp = Date.now();
    const name = `job-center-bulk-${stamp}`;
    const groupId = await createGroup(request, name);
    const first = await submitFailingDownload(request, groupId, `${name}-a.bin`);
    const second = await submitFailingDownload(request, groupId, `${name}-b.bin`);
    await waitForJobState(request, first.canonicalId, 'failed');
    await waitForJobState(request, second.canonicalId, 'failed');

    await page.goto(`/jobs?search=${encodeURIComponent(name)}`);
    await page.locator(`[data-job-id="${first.canonicalId}"]`).getByRole('checkbox').check();
    await page.locator(`[data-job-id="${second.canonicalId}"]`).getByRole('checkbox').check();
    await expect(page.getByTestId('bulk-selected-count')).toHaveText('2 jobs selected');

    const commands = page.getByRole('group', { name: 'Commands for the selected jobs' });
    const dismiss = commands.getByRole('button', { name: 'Dismiss', exact: true });
    await expect(dismiss).toBeVisible();
    await dismiss.click();
    const outcomes = page.getByRole('list', { name: 'Bulk command outcomes' });
    await expect(outcomes.getByRole('listitem')).toHaveCount(2);

    // The default list hides what the viewer dismissed; the live refresh removes both,
    // and with them the bar, so the summary stays visible on the page itself.
    await expect(page.locator('[data-job-id]')).toHaveCount(0, { timeout: 10_000 });
    await expect(page.getByTestId('job-list-notice')).toHaveText('2 of 2 jobs: dismiss.');
    await expect.poll(async () => {
      const response = await request.get(`/v1/jobs?search=${encodeURIComponent(name)}&dismissed=true`);
      return ((await response.json()).jobs as Job[]).length;
    }).toBe(2);
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
    const response = await page.goto('/downloads?URL=legacy-search&Status=failed&CreatedAfter=2026-09-01');

    expect(response?.status()).toBe(200);
    await expect(page).toHaveURL(/\/jobs\?/);
    const url = new URL(page.url());
    expect(url.searchParams.get('kind')).toBe('remote-download');
    expect(url.searchParams.get('state')).toBe('failed');
    expect(url.searchParams.get('search')).toBe('legacy-search');
    expect(url.searchParams.get('acceptedAfter')).toBe('2026-09-01T00:00:00Z');
    await expect(page.getByTestId('job-center')).toBeVisible();
    const form = page.getByRole('form', { name: 'Filter jobs' });
    await expect(form.getByRole('checkbox', { name: 'failed' })).toBeChecked();
    await expect(form.getByRole('searchbox', { name: 'Search' })).toHaveValue('legacy-search');
  });

  test('lists a failed job, opens its detail, and follows the advertised Retry successor', async ({ page, request }) => {
    const stamp = Date.now();
    const name = `job-center-retry-${stamp}.bin`;
    const groupId = await createGroup(request, `job-center-retry-${stamp}`);
    const { canonicalId } = await submitFailingDownload(request, groupId, name);
    await waitForJobState(request, canonicalId, 'failed');

    await page.goto(`/jobs?search=${encodeURIComponent(name)}`);
    const row = page.locator(`[data-job-id="${canonicalId}"]`);
    await expect(row).toBeVisible();
    await expect(row).toContainText('Failed');
    const original = await readJob(request, canonicalId);
    expect(original?.title).toBeTruthy();
    await row.getByRole('link', { name: original!.title!, exact: true }).click();

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

    await page.goto(`/jobs?search=${encodeURIComponent(`job-center-pin-${stamp}`)}`);
    const row = page.locator(`[data-job-id="${canonicalId}"]`);
    await expect(row.getByText('Pinned by you', { exact: true })).toBeVisible();
    await expect(page.locator('[data-job-quick-filter="pinned"]')).toContainText('Pinned by me (1)');

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
