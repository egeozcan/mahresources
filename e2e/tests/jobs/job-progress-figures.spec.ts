import http from 'node:http';
import { randomBytes } from 'node:crypto';
import type { AddressInfo } from 'node:net';
import type { APIRequestContext, Locator, Page } from '@playwright/test';
import { test, expect } from '../../fixtures/base.fixture';

// How a Job's figures read on every surface: the drawer, the /jobs card and the
// Job page say the same amount the same way, a finished Job says it once, and a
// stopped transfer of unknown size neither loses its unit nor draws a full bar.

const SIZE = 5 * 1024 * 1024;
const CHUNK = 64 * 1024;

// Streams SIZE bytes 64 KiB every 100 ms. With `sized` false it sends no
// Content-Length, so the transfer's total is unknown.
async function startSlowServer(sized: boolean) {
  const server = http.createServer((request, response) => {
    const body = randomBytes(SIZE);
    const headers: Record<string, string> = { 'Content-Type': 'application/octet-stream' };
    if (sized) headers['Content-Length'] = String(SIZE);
    response.writeHead(200, headers);
    let sent = 0;
    const timer = setInterval(() => {
      const next = Math.min(CHUNK, SIZE - sent);
      response.write(body.subarray(sent, sent + next));
      sent += next;
      if (sent >= SIZE) {
        clearInterval(timer);
        response.end();
      }
    }, 100);
    request.on('close', () => clearInterval(timer));
  });
  await new Promise<void>(resolve => server.listen(0, '127.0.0.1', resolve));
  return { server, base: `http://127.0.0.1:${(server.address() as AddressInfo).port}` };
}

async function readJob(request: APIRequestContext, id: string) {
  const response = await request.get(`/v1/jobs/${encodeURIComponent(id)}`);
  return response.ok() ? response.json() : null;
}

async function submitDownload(request: APIRequestContext, base: string, label: string) {
  const stamp = `${Date.now()}-${Math.random().toString(16).slice(2, 8)}`;
  const name = `${label}-${stamp}.bin`;
  const group = await (await request.post('/v1/group', { data: { Name: `${label}-${stamp}` } })).json();
  const response = await request.post('/v1/download/submit', {
    data: { URL: `${base}/${name}`, OwnerId: group.ID ?? group.id, Name: name, FileName: name },
  });
  expect(response.status(), await response.text()).toBe(202);
  const id = (await response.json()).jobs?.[0]?.canonicalJobId as string;
  expect(id).toEqual(expect.any(String));
  return id;
}

function card(page: Page, id: string): Locator {
  return page.locator(`article[data-job-id="${id}"]`);
}

test.describe('Job progress figures', () => {
  test('a download reads in formatted units on the card and the Job page, and a finished one says its size once', async ({ page, request }) => {
    const { server, base } = await startSlowServer(true);
    try {
      const id = await submitDownload(request, base, 'figures-sized');
      await expect.poll(async () => (await readJob(request, id))?.progress?.completed ?? 0, { timeout: 15_000 }).toBeGreaterThan(0);

      await page.goto('/jobs?state=running&dismissed=false');
      const running = card(page, id);
      await expect(running).toBeVisible({ timeout: 10_000 });
      const stats = running.getByTestId('job-stats');
      await expect(stats).toContainText(/ of 5\.0 MB/);
      // Never the raw byte count the executor reported, above or under the bar.
      await expect(running.getByRole('progressbar').locator('..')).not.toContainText(/\d{5,}/);
      await expect(running.getByRole('progressbar')).toHaveAttribute('aria-valuetext', / of 5\.0 MB/);

      await page.goto(`/job?id=${encodeURIComponent(id)}`);
      const detailStats = page.locator('[data-job-stats]');
      await expect(detailStats).toContainText(/ of 5\.0 MB|5\.0 MB/, { timeout: 10_000 });
      await expect(page.locator('section[aria-labelledby="job-progress-heading"]')).not.toContainText(/\d{5,}/);

      await expect.poll(async () => (await readJob(request, id))?.state, { timeout: 30_000 }).toBe('succeeded');
      await page.goto('/jobs?state=succeeded&dismissed=false');
      const finished = card(page, id);
      await expect(finished).toBeVisible({ timeout: 10_000 });
      await expect(finished.getByTestId('job-stats')).toHaveText(/^5\.0 MB · average \d/);
      await expect(finished.getByText('Completed', { exact: true })).toBeVisible();
    } finally {
      server.close();
    }
  });

  test('a cancelled download of unknown size keeps its amount with a unit and draws an empty track', async ({ page, request }) => {
    const { server, base } = await startSlowServer(false);
    try {
      const id = await submitDownload(request, base, 'figures-unsized');
      await expect.poll(async () => (await readJob(request, id))?.progress?.completed ?? 0, { timeout: 15_000 }).toBeGreaterThan(CHUNK);
      const job = await readJob(request, id);
      const cancel = job.commands?.find((command: any) => command.key === 'cancel');
      expect(cancel).toBeTruthy();
      const key = `figures-cancel-${id}`;
      const response = await request.post(cancel.endpoint, {
        headers: { 'Idempotency-Key': key },
        data: { expectedVersion: cancel.jobVersion, idempotencyKey: key },
      });
      expect(response.ok(), await response.text()).toBe(true);
      await expect.poll(async () => (await readJob(request, id))?.state, { timeout: 20_000 }).toBe('cancelled');

      await page.goto('/jobs?state=cancelled&dismissed=false');
      const cancelled = card(page, id);
      await expect(cancelled).toBeVisible({ timeout: 10_000 });
      await expect(cancelled.getByTestId('job-stats')).toHaveText(/^\d+(\.\d+)? (KB|MB)$/);
      const bar = cancelled.getByRole('progressbar');
      await expect(bar).toHaveAttribute('aria-valuetext', /(KB|MB) processed; total unknown/);
      // An empty track: a full bar would read as done.
      await expect(bar.locator('[data-job-progress-fill]')).toHaveCSS('width', '0px');
      await expect(cancelled.getByTestId('job-kind')).toHaveText('Download');
    } finally {
      server.close();
    }
  });

  test('the drawer says why a job is blocked, names Kinds in words, and lists scheduled work by when it starts', async ({ page }) => {
    const now = Date.now();
    const at = (minutes: number) => new Date(now + minutes * 60_000).toISOString();
    const base = { version: 1, acceptedAt: at(-30), commands: [], outputs: [], lineage: { ancestors: [], successors: [], parents: [], children: [] } };
    const blocked = { ...base, id: 'figures-blocked', kind: 'plugin-action', state: 'blocked', title: 'Blocked action', stateEnteredAt: at(-1), blockedReason: 'plugin-unavailable' };
    const running = { ...base, id: 'figures-running', kind: 'remote-download', state: 'running', title: 'Running download', stateEnteredAt: at(-20),
      progress: { completed: 3 * 1024 * 1024, total: 10 * 1024 * 1024, unit: 'bytes', updatedAt: at(0) } };
    // Scheduled most recently, starting last: state entry would list it first.
    const late = { ...base, id: 'figures-late', kind: 'deferred-download', state: 'scheduled', title: 'Starts later', stateEnteredAt: at(-2), scheduledFor: at(120) };
    const soon = { ...base, id: 'figures-soon', kind: 'deferred-download', state: 'scheduled', title: 'Starts soon', stateEnteredAt: at(-10), scheduledFor: at(30) };
    const all = [blocked, running, late, soon];
    await page.route(/\/v1\/jobs(?:\?.*)?$/, route => {
      const states = new URL(route.request().url()).searchParams.getAll('state');
      return route.fulfill({ json: { jobs: all.filter(job => states.includes(job.state)), nextCursor: null } });
    });
    for (const job of all) await page.route(`**/v1/jobs/${job.id}`, route => route.fulfill({ json: job }));

    await page.goto('/dashboard');
    await page.keyboard.press('Control+Shift+D');
    const drawer = page.getByRole('dialog', { name: 'Jobs' });
    await expect(drawer).toBeVisible();

    const blockedRow = drawer.locator('article[data-job-id="figures-blocked"]');
    await expect(blockedRow.locator('[data-job-panel-blocked]')).toHaveText('Reason: Its plugin is disabled or not loaded.');
    await expect(blockedRow.locator('[data-job-panel-kind]')).toHaveText('Plugin action');

    const active = drawer.locator('[data-job-panel-group="active"] article');
    await expect(active).toHaveCount(3);
    await expect.poll(() => active.evaluateAll(rows => rows.map(row => row.getAttribute('data-job-id'))))
      .toEqual(['figures-running', 'figures-soon', 'figures-late']);
    await expect(drawer.locator('article[data-job-id="figures-soon"] [data-job-panel-kind]')).toHaveText('Scheduled download');
    await expect(drawer.locator('article[data-job-id="figures-running"] [data-job-panel-stats]')).toHaveText(/^3\.0 MB of 10\.0 MB/);
  });

  test('bars and state pills stay visible in forced colours', async ({ page }) => {
    const now = new Date().toISOString();
    const running = {
      id: 'figures-forced', kind: 'remote-download', state: 'running', version: 1, title: 'Forced colours download',
      acceptedAt: now, stateEnteredAt: now, commands: [], outputs: [],
      lineage: { ancestors: [], successors: [], parents: [], children: [] },
      progress: { completed: 4 * 1024 * 1024, total: 10 * 1024 * 1024, unit: 'bytes', updatedAt: now },
    };
    await page.route(/\/v1\/jobs(?:\?.*)?$/, route => {
      const states = new URL(route.request().url()).searchParams.getAll('state');
      return route.fulfill({ json: { jobs: states.includes('running') ? [running] : [], nextCursor: null } });
    });
    await page.route(`**/v1/jobs/${running.id}`, route => route.fulfill({ json: running }));
    await page.emulateMedia({ forcedColors: 'active' });

    await page.goto('/dashboard');
    await page.keyboard.press('Control+Shift+D');
    const row = page.getByRole('dialog', { name: 'Jobs' }).locator(`article[data-job-id="${running.id}"]`);
    const track = row.getByRole('progressbar');
    await expect(track).toBeVisible();
    // The track keeps an outline and the fill a colour forced colours do not
    // replace with the page's background.
    const looks = await track.evaluate(element => {
      const fill = element.firstElementChild as HTMLElement;
      const pill = element.closest('article')!.querySelector('[data-job-panel-state]') as HTMLElement;
      return {
        trackBorder: getComputedStyle(element).borderTopWidth,
        fillAdjust: getComputedStyle(fill).forcedColorAdjust,
        fillBackground: getComputedStyle(fill).backgroundColor,
        pageBackground: getComputedStyle(document.body).backgroundColor,
        pillBorder: getComputedStyle(pill).borderTopWidth,
      };
    });
    expect(looks.trackBorder).toBe('1px');
    expect(looks.fillAdjust).toBe('none');
    expect(looks.fillBackground).not.toBe(looks.pageBackground);
    expect(looks.fillBackground).not.toBe('rgba(0, 0, 0, 0)');
    expect(looks.pillBorder).toBe('1px');
  });
});
