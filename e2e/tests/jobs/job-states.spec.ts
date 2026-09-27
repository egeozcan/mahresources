import http from 'node:http';
import { randomBytes } from 'node:crypto';
import type { AddressInfo } from 'node:net';
import { test, expect } from '../../fixtures/base.fixture';
import { gotoMockedJobPage } from '../../helpers/job-page';

// What each Job surface says a Job is doing: the drawer, the Job page and the /jobs
// card read one state table, so a paused download is paused everywhere, scheduled
// work says when it starts, and work nobody is doing never reads "Working".

const DEAD_URL = 'http://127.0.0.1:9/';

// Sends a few bytes and then holds the response open, so a download stays running
// until the test pauses or cancels it.
async function startHeldServer() {
  const server = http.createServer((request, response) => {
    response.writeHead(200, { 'Content-Type': 'application/octet-stream', 'Content-Length': String(4 * 1024 * 1024) });
    response.write(randomBytes(64 * 1024));
    const timer = setInterval(() => response.write(randomBytes(1024)), 500);
    request.on('close', () => clearInterval(timer));
  });
  await new Promise<void>(resolve => server.listen(0, '127.0.0.1', resolve));
  return { server, base: `http://127.0.0.1:${(server.address() as AddressInfo).port}` };
}

async function readJob(request: import('@playwright/test').APIRequestContext, id: string) {
  const response = await request.get(`/v1/jobs/${encodeURIComponent(id)}`);
  return response.ok() ? response.json() : null;
}

async function submitDownload(request: import('@playwright/test').APIRequestContext, url: string, name: string) {
  const group = await (await request.post('/v1/group', { data: { Name: name } })).json();
  const response = await request.post('/v1/download/submit', {
    data: { URL: url, OwnerId: group.ID ?? group.id, Name: name, FileName: name },
  });
  expect(response.status(), await response.text()).toBe(202);
  const id = (await response.json()).jobs?.[0]?.canonicalJobId as string;
  expect(id).toEqual(expect.any(String));
  return id;
}

async function cancelIfUnfinished(request: import('@playwright/test').APIRequestContext, id: string) {
  const job = await readJob(request, id);
  const cancel = job?.commands?.find((command: { key: string }) => command.key === 'cancel');
  if (!cancel) return;
  await request.post(cancel.endpoint, {
    data: { expectedVersion: cancel.jobVersion, idempotencyKey: `cleanup-${id}-${job.version}` },
  });
}

test.describe('Job states on every surface', () => {
  test('a download paused from the drawer is paused, not blocked, and says what Resume does', async ({ page, request }) => {
    const { server, base } = await startHeldServer();
    const name = `job-states-pause-${Date.now()}.bin`;
    let id = '';
    try {
      id = await submitDownload(request, `${base}/${name}`, name);
      await expect.poll(async () => (await readJob(request, id))?.state, { timeout: 20_000 }).toBe('running');

      await page.goto('/dashboard');
      await page.keyboard.press('Control+Shift+D');
      const drawer = page.getByRole('dialog', { name: 'Jobs' });
      const row = drawer.locator(`article[data-job-id="${id}"]`);
      await expect(row).toBeVisible({ timeout: 10_000 });
      await row.getByRole('group', { name: 'Job actions' }).getByRole('button', { name: 'Pause', exact: true }).click();
      // What Resume will do is said before the pause, where the choice is made.
      const confirmation = page.getByRole('alertdialog');
      await expect(confirmation).toContainText('Resume starts it again from the beginning');
      await expect(confirmation.getByRole('button', { name: 'Go back', exact: true })).toBeVisible();
      await confirmation.getByRole('button', { name: 'Pause', exact: true }).click();

      await expect.poll(async () => (await readJob(request, id))?.state, { timeout: 20_000 }).toBe('paused');
      // Still active work, under its own group, with the paused pill and the reason.
      const active = drawer.locator('[data-job-panel-group="active"]');
      await expect(active.locator(`article[data-job-id="${id}"]`)).toBeVisible();
      await expect(drawer.locator('[data-job-panel-group="attention"]').locator(`article[data-job-id="${id}"]`)).toHaveCount(0);
      await expect(row).toContainText('Paused');
      await expect(row).toContainText('Resume starts the download again from the beginning.');
      await expect(row).not.toContainText('Blocked');
      await expect(row.getByRole('button', { name: 'Resume', exact: true })).toBeVisible();

      // The state filter finds it, and so does the legacy /downloads address.
      await page.goto(`/jobs?state=paused&dismissed=false&search=${encodeURIComponent(name)}`);
      const card = page.locator(`article[data-job-id="${id}"]`);
      await expect(card.getByTestId('job-state')).toHaveText('Paused');
      await expect(card.getByTestId('job-state')).not.toHaveClass(/card-badge--danger/);
      await page.goto('/downloads?Status=paused');
      await expect(page.locator(`article[data-job-id="${id}"]`)).toBeVisible();
    } finally {
      if (id) await cancelIfUnfinished(request, id);
      server.close();
    }
  });

  test('a pause the transfer has not confirmed reads Pausing, not Paused', async ({ page }) => {
    const id = 'states-pause-requested';
    let requested = false;
    const job = () => ({
      id, kind: 'remote-download', state: 'running', version: requested ? 2 : 1, title: 'Held elsewhere',
      acceptedAt: '2026-09-26T10:00:03Z', controlIntent: requested ? 'pause' : '', phase: requested ? 'pausing' : 'downloading',
      progress: { completed: 1024, unit: 'bytes' },
      commands: requested ? [] : [
        { key: 'pause', label: 'Pause', endpoint: `/v1/jobs/${id}/commands/pause`, jobVersion: 1,
          confirmation: 'Pause this download? The bytes received so far are discarded, and Resume starts it again from the beginning.' },
      ],
      outputs: [], lineage: { ancestors: [], successors: [], parents: [], children: [] },
    });
    await page.route(/\/v1\/jobs(?:\?.*)?$/, route => {
      const states = new URL(route.request().url()).searchParams.getAll('state');
      return route.fulfill({ json: { jobs: states.includes('running') ? [job()] : [], nextCursor: null } });
    });
    await page.route(`**/v1/jobs/${id}`, route => route.fulfill({ json: job() }));
    await page.route(`**/v1/jobs/${id}/events**`, route => route.fulfill({ json: { events: [] } }));
    await page.route(`**/v1/jobs/${id}/commands/pause`, route => {
      requested = true;
      return route.fulfill({ json: { result: { job: job(), status: 'succeeded', code: 'requested',
        message: 'Pause requested. The download is held when the server running it next checks, about once a second.' } } });
    });

    await page.goto('/dashboard');
    await page.keyboard.press('Control+Shift+D');
    const drawer = page.getByRole('dialog', { name: 'Jobs' });
    const row = drawer.locator(`article[data-job-id="${id}"]`);
    await row.getByRole('group', { name: 'Job actions' }).getByRole('button', { name: 'Pause', exact: true }).click();
    await page.getByRole('alertdialog').getByRole('button', { name: 'Pause', exact: true }).click();
    await expect(row).toContainText('Pausing');
    await expect(row).not.toContainText('Paused');
    await expect(drawer.locator('[data-job-panel-notice]')).toContainText('Pause requested');

    await gotoMockedJobPage(page, id);
    const detail = page.getByTestId('job-detail');
    await expect(page).toHaveTitle(/^Held elsewhere \(Pausing\) - Job - /);
    await expect(detail).toContainText('Pausing');
    await expect(detail).not.toContainText('Paused');
  });

  test('a failed download that recorded nothing shows no progress on its page', async ({ page, request }) => {
    const name = `job-states-failed-${Date.now()}.bin`;
    const id = await submitDownload(request, `${DEAD_URL}${name}`, name);
    await expect.poll(async () => (await readJob(request, id))?.state, { timeout: 20_000 }).toBe('failed');

    await page.goto(`/job?id=${encodeURIComponent(id)}`);
    const detail = page.getByTestId('job-detail');
    await expect(detail.getByRole('heading', { name: 'Failure' })).toBeVisible();
    await expect(detail.getByRole('heading', { name: 'Progress' })).toHaveCount(0);
    await expect(detail.getByRole('progressbar')).toHaveCount(0);
    await expect(detail).not.toContainText('Working');
  });

  test('scheduled and queued rows never read as working, and scheduled ones say when they start', async ({ page }) => {
    const soon = new Date(Date.now() + 45 * 60 * 1000).toISOString();
    const jobs = [
      { id: 'states-scheduled', kind: 'deferred-download', state: 'scheduled', version: 1, title: 'Scheduled for later',
        acceptedAt: '2026-09-26T10:00:02Z', scheduledFor: soon, progress: {},
        commands: [{ key: 'cancel', label: 'Cancel', endpoint: '/v1/jobs/states-scheduled/commands/cancel', jobVersion: 1 }] },
      { id: 'states-queued', kind: 'remote-download', state: 'queued', version: 1, title: 'Waiting for a slot',
        acceptedAt: '2026-09-26T10:00:01Z', progress: {},
        commands: [{ key: 'cancel', label: 'Cancel', endpoint: '/v1/jobs/states-queued/commands/cancel', jobVersion: 1 }] },
    ];
    await page.route(/\/v1\/jobs(?:\?.*)?$/, route => {
      const states = new URL(route.request().url()).searchParams.getAll('state');
      return route.fulfill({ json: { jobs: jobs.filter(job => states.includes(job.state)), nextCursor: null } });
    });
    for (const job of jobs) await page.route(`**/v1/jobs/${job.id}`, route => route.fulfill({ json: job }));
    await page.route('**/v1/jobs/states-scheduled/events**', route => route.fulfill({ json: { events: [] } }));

    await page.goto('/dashboard');
    await page.keyboard.press('Control+Shift+D');
    const drawer = page.getByRole('dialog', { name: 'Jobs' });
    const scheduled = drawer.locator('article[data-job-id="states-scheduled"]');
    await expect(scheduled.locator('[data-job-panel-scheduled]')).toHaveText(/^Starts .+ \(in 4[45] min\)$/);
    for (const id of ['states-scheduled', 'states-queued']) {
      const row = drawer.locator(`article[data-job-id="${id}"]`);
      await expect(row).toBeVisible();
      await expect(row.getByRole('progressbar')).toHaveCount(0);
      await expect(row).not.toContainText('In progress');
    }

    await gotoMockedJobPage(page, 'states-scheduled');
    const detail = page.getByTestId('job-detail');
    await expect(detail.locator('[data-job-scheduled-for]')).toHaveText(/^Starts .+ \(in 4[45] min\)$/);
    await expect(detail.getByRole('heading', { name: 'Progress' })).toHaveCount(0);
    await expect(detail).not.toContainText('Working');
  });
});
