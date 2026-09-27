import http from 'node:http';
import { randomBytes } from 'node:crypto';
import type { AddressInfo } from 'node:net';
import type { APIRequestContext } from '@playwright/test';
import { test, expect } from '../../fixtures/base.fixture';
import { gotoMockedJobPage } from '../../helpers/job-page';

// The Job page: which Job it is, when it ran, how its relatives relate to it,
// every event it recorded, and its files under names that open.

// /slow/* sends its bytes over about three seconds; anything else fails at once.
async function startServer() {
  const server = http.createServer((request, response) => {
    if (request.url?.startsWith('/slow/')) {
      const body = randomBytes(64 * 1024 * 30);
      response.writeHead(200, { 'Content-Type': 'application/octet-stream', 'Content-Length': String(body.length) });
      let sent = 0;
      const timer = setInterval(() => {
        response.write(body.subarray(sent, sent + 64 * 1024));
        sent += 64 * 1024;
        if (sent >= body.length) {
          clearInterval(timer);
          response.end();
        }
      }, 100);
      request.on('close', () => clearInterval(timer));
      return;
    }
    response.writeHead(500, { 'Content-Type': 'text/plain' });
    response.end('failing on purpose');
  });
  await new Promise<void>(resolve => server.listen(0, '127.0.0.1', resolve));
  return { server, base: `http://127.0.0.1:${(server.address() as AddressInfo).port}` };
}

async function submitDownload(request: APIRequestContext, url: string, name: string): Promise<string> {
  const response = await request.post('/v1/download/submit', { data: { URL: url, Name: name, FileName: name } });
  expect(response.status(), await response.text()).toBe(202);
  const id = (await response.json()).jobs?.[0]?.canonicalJobId as string;
  expect(id).toEqual(expect.any(String));
  return id;
}

async function readJob(request: APIRequestContext, id: string) {
  const response = await request.get(`/v1/jobs/${encodeURIComponent(id)}`);
  return response.ok() ? response.json() : null;
}

async function waitForState(request: APIRequestContext, id: string, state: string) {
  await expect.poll(async () => (await readJob(request, id))?.state ?? null, { timeout: 30_000 }).toBe(state);
}

async function dismiss(request: APIRequestContext, ids: string[]) {
  if (!ids.length) return;
  await request.post('/v1/jobs/commands/dismiss', { data: { jobIds: ids, idempotencyKey: `cleanup-${Date.now()}-${Math.random()}` } });
}

test.describe('The Job page', () => {
  test('is titled by its Job and says when it ran and how long its history is kept', async ({ page, request }) => {
    const { server, base } = await startServer();
    const name = `job-detail-times-${Date.now()}.bin`;
    const id = await submitDownload(request, `${base}/failing/${name}`, name);
    try {
      await waitForState(request, id, 'failed');
      await page.goto(`/job?id=${encodeURIComponent(id)}`);

      await expect(page).toHaveTitle(new RegExp(`^${name.replace(/\./g, '\\.')} \\(Failed\\) - Job - `));
      await expect(page.getByRole('heading', { level: 1 })).toHaveCount(1);
      await expect(page.getByRole('heading', { level: 1 })).toHaveText(name);

      const times = page.locator('[data-job-times]');
      await expect(times.getByRole('heading', { name: 'Times' })).toBeVisible();
      await expect(times).toContainText(/Times are in your time zone/);
      for (const key of ['accepted', 'started', 'finished', 'expires']) {
        const row = times.locator(`[data-job-time="${key}"]`);
        await expect(row.locator('time')).toHaveText(/^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$/);
        await expect(row.locator('time')).toHaveAttribute('datetime', /\d{4}-\d{2}-\d{2}T/);
      }
      await expect(times.locator('[data-job-time="expires"]')).toContainText('History kept until');
      await expect(times.locator('[data-job-time="expires"]')).toContainText(/\(in \d+ d/);
      // The timeline writes its times the same way.
      await expect(page.getByTestId('job-timeline').locator('time').first()).toHaveText(/^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$/);
    } finally {
      await dismiss(request, [id]);
      server.close();
    }
  });

  test('answers an id that names no Job with a 404 and a way back to the Job Center', async ({ page }) => {
    for (const id of ['01a0ffff-0000-7000-8000-000000000000', 'not-a-uuid', 'a%2Fb']) {
      const response = await page.goto(`/job?id=${id}`);
      expect(response?.status()).toBe(404);
      await expect(page.getByRole('alert')).toContainText("That job doesn't exist, or it has been deleted.");
      await page.getByRole('link', { name: 'Back to Job Center' }).click();
      await expect(page).toHaveURL(/\/jobs\?dismissed=false$/);
    }
  });

  test('says how a retry relates to the Job it retried, with its state and when it was accepted', async ({ page, request }) => {
    const { server, base } = await startServer();
    const name = `job-detail-lineage-${Date.now()}.bin`;
    const id = await submitDownload(request, `${base}/failing/${name}`, name);
    const ids = [id];
    try {
      await waitForState(request, id, 'failed');
      await page.goto(`/job?id=${encodeURIComponent(id)}`);
      await page.getByRole('group', { name: 'Job actions' }).getByRole('button', { name: 'Retry', exact: true }).click();
      await expect(page).not.toHaveURL(new RegExp(`id=${id}$`));
      const successorId = new URL(page.url()).searchParams.get('id')!;
      ids.push(successorId);
      await waitForState(request, successorId, 'failed');
      await page.reload();

      // The retry's own page says it is one, and which Job it retried.
      await expect(page).toHaveTitle(new RegExp(`^${name.replace(/\./g, '\\.')} \\(Failed\\) - Job - `));
      const earlier = page.locator('[data-job-lineage="ancestors"]');
      await expect(earlier.getByRole('heading', { name: 'Earlier runs' })).toBeVisible();
      const entry = earlier.getByRole('listitem');
      await expect(entry).toContainText(`Retry of ${name}`);
      await expect(entry).toContainText(/Failed · accepted \d{4}-\d{2}-\d{2} \d{2}:\d{2}/);
      await expect(entry.getByRole('link', { name })).toHaveAttribute('href', `/job?id=${id}`);

      await entry.getByRole('link', { name }).click();
      const later = page.locator('[data-job-lineage="successors"]');
      await expect(later.getByRole('heading', { name: 'Later runs' })).toBeVisible();
      await expect(later.getByRole('listitem')).toContainText(`Retried as ${name}`);
    } finally {
      await dismiss(request, ids);
      server.close();
    }
  });

  test('adds events to its timeline as they happen', async ({ page, request }) => {
    const { server, base } = await startServer();
    const name = `job-detail-live-${Date.now()}.bin`;
    const id = await submitDownload(request, `${base}/slow/${name}`, name);
    try {
      await page.goto(`/job?id=${encodeURIComponent(id)}`);
      const timeline = page.getByTestId('job-timeline');
      await expect(timeline).toContainText('accepted');
      await expect(page.getByTestId('job-detail').locator('header')).toContainText('Succeeded', { timeout: 30_000 });
      await expect(timeline).toContainText('succeeded');
      // Nothing that ended says what it was doing while it ran.
      const job = await readJob(request, id);
      expect(job?.phase ?? '').toBe('');
    } finally {
      await dismiss(request, [id]);
      server.close();
    }
  });

  test('reads past the first page of events, and past its bound when asked', async ({ page }) => {
    const id = 'detail-many-events';
    const total = 1214;
    const event = (sequence: number) => ({
      id: `${id}-${sequence}`, jobId: id, sequence, jobVersion: 3, type: sequence === total ? 'blocked' : 'progress-note',
      createdAt: '2026-09-26T10:00:00Z',
    });
    await page.route(`**/v1/jobs/${id}/events?*`, route => {
      const url = new URL(route.request().url());
      const after = Number(url.searchParams.get('afterSequence') || 0);
      const limit = Number(url.searchParams.get('limit') || 200);
      const last = Math.min(after + limit, total);
      const events = Array.from({ length: last - after }, (_, i) => event(after + i + 1));
      return route.fulfill({ json: { events, nextSequence: last < total ? last : undefined } });
    });
    await page.route(`**/v1/jobs/${id}`, route => route.fulfill({ json: {
      id, kind: 'remote-download', title: 'Many events', state: 'blocked', version: 3, acceptedAt: '2026-09-26T10:00:00Z',
      commands: [], outputs: [], lineage: { ancestors: [], successors: [], parents: [], children: [] },
    } }));

    await gotoMockedJobPage(page, id);
    const timeline = page.getByTestId('job-timeline');
    await expect(timeline.getByRole('listitem')).toHaveCount(1000);
    const more = page.locator('[data-job-timeline-more]');
    await expect(more).toContainText('This job has more events than are shown.');
    await more.getByRole('button', { name: 'Show later events' }).click();
    await expect(timeline.getByRole('listitem')).toHaveCount(total);
    await expect(timeline.getByRole('listitem').last()).toContainText('blocked');
    await expect(more).toBeHidden();
  });

  test('offers a finished export for download under a name that opens, here and on its card', async ({ page, request, apiClient }) => {
    const category = await apiClient.createCategory(`job-detail-export-${Date.now()}`);
    const group = await apiClient.createGroup({ name: `job-detail-export-${Date.now()}`, categoryId: category.ID });
    const submitted = await request.post('/v1/groups/export', { data: { rootGroupIds: [group.ID] } });
    expect(submitted.ok(), await submitted.text()).toBe(true);
    const { canonicalJobId } = await submitted.json();
    try {
      await waitForState(request, canonicalJobId, 'succeeded');
      await page.goto(`/job?id=${encodeURIComponent(canonicalJobId)}`);
      const outputs = page.locator('section[aria-labelledby="job-outputs-heading"]');
      await expect(outputs).toContainText('1 output');
      const link = outputs.getByRole('link', { name: 'Download exported archive' });
      const [download] = await Promise.all([page.waitForEvent('download'), link.click()]);
      expect(download.suggestedFilename()).toMatch(/^exported-archive-\d{8}-\d{6}\.tar(\.gz)?$/);

      // The /jobs card offers it too; the drawer draws its row from the same rule (resultOutput).
      await page.goto('/jobs?kind=group-export&dismissed=false');
      const card = page.locator(`[data-job-id="${canonicalJobId}"]`);
      await expect(card.getByTestId('job-result-link')).toHaveText('Download exported archive');
      await expect(card.getByTestId('job-result-link')).toHaveAttribute('href', `/v1/jobs/${canonicalJobId}/outputs?key=artifact`);
    } finally {
      await dismiss(request, [canonicalJobId]);
    }
  });
});

test.describe('A Job Center address with a filter it cannot use', () => {
  test('keeps the filter form, says what is wrong, and offers to clear the filters', async ({ page }) => {
    const response = await page.goto('/jobs?state=bogus&kind=remote-download&dismissed=false');
    expect(response?.status()).toBe(400);
    await expect(page).toHaveTitle(/^Job Center - /);
    await expect(page.getByRole('alert')).toHaveText('This filter cannot be used: unknown state "bogus".');
    await expect(page.getByRole('form', { name: 'Filter jobs' })).toBeAttached();
    await page.getByRole('link', { name: 'clear all filters' }).click();
    await expect(page).toHaveURL(/\/jobs\?dismissed=false$/);
    await expect(page.getByTestId('job-live-status')).toBeVisible();
  });
});
