import http from 'node:http';
import { randomBytes } from 'node:crypto';
import type { AddressInfo } from 'node:net';
import type { APIRequestContext, Page, Request } from '@playwright/test';
import { test, expect } from '../../fixtures/base.fixture';

// Answers at once: /missing/* with a 404, /slow/* over about six seconds,
// anything else with a few fresh bytes. /held/* streams like /slow/* but stops a
// sixth of the way in until release() is called.
async function startServer() {
  let release!: () => void;
  const released = new Promise<void>(resolve => { release = resolve; });
  const server = http.createServer((request, response) => {
    if (request.url?.startsWith('/missing/')) {
      response.writeHead(404, { 'Content-Type': 'text/plain' });
      response.end('gone');
      return;
    }
    if (request.url?.startsWith('/slow/') || request.url?.startsWith('/held/')) {
      let held = request.url.startsWith('/held/');
      const body = randomBytes(64 * 1024 * 60);
      response.writeHead(200, { 'Content-Type': 'application/octet-stream', 'Content-Length': String(body.length) });
      let sent = 0;
      let waiting = false;
      const timer = setInterval(() => {
        if (waiting) return;
        if (held && sent === 64 * 1024 * 10) {
          waiting = true;
          void released.then(() => { held = false; waiting = false; });
          return;
        }
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
    const body = randomBytes(2048);
    response.writeHead(200, { 'Content-Type': 'application/octet-stream', 'Content-Length': String(body.length) });
    response.end(body);
  });
  await new Promise<void>(resolve => server.listen(0, '127.0.0.1', resolve));
  return { server, base: `http://127.0.0.1:${(server.address() as AddressInfo).port}`, release };
}

async function submitDownload(request: APIRequestContext, url: string, name: string): Promise<string> {
  const response = await request.post('/v1/download/submit', { data: { URL: url, Name: name, FileName: name } });
  expect(response.status(), await response.text()).toBe(202);
  const id = (await response.json()).jobs?.[0]?.canonicalJobId as string;
  expect(id).toEqual(expect.any(String));
  return id;
}

async function waitForState(request: APIRequestContext, id: string, state: string) {
  await expect.poll(async () => {
    const response = await request.get(`/v1/jobs/${encodeURIComponent(id)}`);
    return response.ok() ? (await response.json()).state : null;
  }, { timeout: 30_000 }).toBe(state);
}

async function dismiss(request: APIRequestContext, ids: string[]) {
  if (!ids.length) return;
  await request.post('/v1/jobs/commands/dismiss', { data: { jobIds: ids, idempotencyKey: `cleanup-${Date.now()}-${Math.random()}` } });
}

function panelState(page: Page) {
  return page.evaluate(() => {
    const panel = (window as any).Alpine.$data(document.querySelector('[data-testid="job-panel-root"]'));
    return {
      caughtUp: !!panel.streamCaughtUp,
      connection: panel.connectionStatus,
      settled: !panel._panelRefreshPromise && !panel._panelRefreshTimer && !panel._panelRefreshMaxTimer,
    };
  });
}

// Every Jobs API request the page makes from now on, by kind.
function recordJobRequests(page: Page) {
  const seen: { lists: URL[]; details: string[]; streams: URL[] } = { lists: [], details: [], streams: [] };
  page.on('request', (request: Request) => {
    const url = new URL(request.url());
    if (request.method() !== 'GET') return;
    if (url.pathname === '/v1/jobs') seen.lists.push(url);
    else if (url.pathname === '/v1/jobs/events') seen.streams.push(url);
    else if (/^\/v1\/jobs\/[0-9a-f-]{36}$/.test(url.pathname)) seen.details.push(url.pathname.slice('/v1/jobs/'.length));
  });
  return seen;
}

const drawerOf = (page: Page) => page.getByRole('dialog', { name: 'Jobs' });

test.describe('Jobs drawer live updates and reads', () => {
  test('a page load reads the lists once from the stream\'s head, and details only while the drawer is open', async ({ page, request }) => {
    const { server, base } = await startServer();
    const ids: string[] = [];
    try {
      const stamp = Date.now();
      for (const name of [`budget-a-${stamp}.bin`, `budget-b-${stamp}.bin`]) {
        const id = await submitDownload(request, `${base}/missing/${name}`, name);
        ids.push(id);
        await waitForState(request, id, 'failed');
      }

      const seen = recordJobRequests(page);
      await page.goto('/dashboard');
      await expect.poll(() => panelState(page), { timeout: 15_000 }).toMatchObject({ caughtUp: true, connection: 'connected', settled: true });

      expect(seen.streams).toHaveLength(1);
      expect(seen.streams[0].searchParams.get('start')).toBe('head');
      // One read of the three lists once the stream catches up. A stream that
      // takes longer than the drawer's first-read wait (1.5 s, which a busy
      // machine can exceed) adds one earlier read; never more.
      expect([3, 6]).toContain(seen.lists.length);
      expect(seen.lists.every(url => url.searchParams.get('order') === 'stateEntered')).toBe(true);
      expect(seen.details).toEqual([]);

      await page.keyboard.press('Control+Shift+D');
      const drawer = drawerOf(page);
      for (const id of ids) await expect(drawer.locator(`article[data-job-id="${id}"]`)).toBeVisible();
      await expect.poll(() => ids.every(id => seen.details.includes(id)), { timeout: 10_000 }).toBe(true);

      // A job that fails now is read once; the rows that did not change are not read again.
      const laterName = `budget-later-${stamp}.bin`;
      const later = await submitDownload(request, `${base}/missing/${laterName}`, laterName);
      ids.push(later);
      await expect(drawer.locator(`article[data-job-id="${later}"]`)).toBeVisible({ timeout: 15_000 });
      await expect.poll(() => seen.details.includes(later), { timeout: 10_000 }).toBe(true);
      await page.waitForTimeout(1000);
      for (const id of ids.slice(0, 2)) expect(seen.details.filter(detail => detail === id), id).toHaveLength(1);
    } finally {
      await dismiss(request, ids);
      server.close();
    }
  });

  test('a stream answered with an error is opened again, and the lists it could not update are read meanwhile', async ({ page, request }) => {
    const { server, base } = await startServer();
    const ids: string[] = [];
    try {
      const stamp = Date.now();
      const oldName = `refused-old-${stamp}.bin`;
      ids.push(await submitDownload(request, `${base}/missing/${oldName}`, oldName));
      await waitForState(request, ids[0], 'failed');

      // A proxy answering 502 while the app restarts: the browser closes the
      // stream for good and never retries it by itself.
      const streams = /\/v1\/jobs\/events\?/;
      await page.route(streams, route => route.fulfill({ status: 502, body: 'bad gateway' }));
      await page.goto('/dashboard');
      await page.keyboard.press('Control+Shift+D');
      const drawer = drawerOf(page);
      await expect(drawer.getByRole('status').filter({ hasText: 'Reconnecting' })).toBeVisible({ timeout: 10_000 });
      await expect(drawer.locator(`article[data-job-id="${ids[0]}"]`)).toBeVisible({ timeout: 10_000 });
      await expect(drawer.getByText('No visible jobs yet.')).toBeHidden();

      await page.unroute(streams);
      await expect(drawer.getByRole('status').filter({ hasText: 'Live updates connected' })).toBeVisible({ timeout: 20_000 });

      const newName = `refused-new-${stamp}.bin`;
      ids.push(await submitDownload(request, `${base}/missing/${newName}`, newName));
      await expect(drawer.locator(`[data-job-panel-group="attention"] article[data-job-id="${ids[1]}"]`)).toBeVisible({ timeout: 15_000 });
    } finally {
      await dismiss(request, ids);
      server.close();
    }
  });

  test('an ended session says so and links to sign in, and the drawer recovers once the session is back', async ({ page }) => {
    let signedIn = false;
    const refuse = (route: import('@playwright/test').Route) => signedIn
      ? route.fallback()
      : route.fulfill({ status: 401, json: { error: 'authentication required' } });
    await page.route(/\/v1\/jobs\?/, refuse);
    await page.route(/\/v1\/jobs\/events\?/, refuse);
    await page.goto('/dashboard');
    await page.keyboard.press('Control+Shift+D');
    const drawer = drawerOf(page);

    const signedOut = drawer.locator('[data-job-panel-signed-out]');
    await expect(signedOut).toContainText('Your session has ended', { timeout: 10_000 });
    await expect(signedOut.getByRole('link', { name: 'Sign in again' })).toHaveAttribute('href', '/login?next=%2Fdashboard');
    await expect(drawer.getByRole('status').filter({ hasText: 'Signed out' })).toBeVisible();
    await expect(page.locator('#job-panel-trigger-counts')).toHaveText('Signed out; jobs are not shown');

    signedIn = true;
    await expect(signedOut).toBeHidden({ timeout: 20_000 });
    await expect(drawer.getByRole('status').filter({ hasText: 'Live updates connected' })).toBeVisible({ timeout: 20_000 });
  });

  test('a list read that fails says so with Try again instead of an empty drawer, and recovers', async ({ page }) => {
    let failing = true;
    await page.route(/\/v1\/jobs\?/, route => failing
      ? route.fulfill({ status: 500, json: { error: 'database is locked' } })
      : route.fallback());
    await page.goto('/dashboard');
    await page.keyboard.press('Control+Shift+D');
    const drawer = drawerOf(page);

    const error = drawer.locator('[data-job-panel-error]');
    await expect(error.getByRole('alert')).toContainText('Jobs could not be loaded: database is locked.', { timeout: 10_000 });
    await expect(drawer.getByText('No visible jobs yet.')).toBeHidden();
    await expect(page.locator('[data-job-panel-active-badge]')).toBeHidden();
    await expect(page.locator('#job-panel-trigger-counts')).toHaveText('Jobs could not be loaded');

    failing = false;
    await error.getByRole('button', { name: 'Try again' }).click();
    await expect(error).toBeHidden({ timeout: 10_000 });
    await expect(page.locator('[data-job-panel-active-badge]')).toBeVisible();
  });

  test('a download that finishes after newer ones leads Finished instead of leaving the drawer', async ({ page, request }) => {
    const { server, base } = await startServer();
    const ids: string[] = [];
    try {
      const stamp = Date.now();
      await page.goto('/dashboard');
      await page.keyboard.press('Control+Shift+D');
      const drawer = drawerOf(page);
      const slowName = `watched-slow-${stamp}.bin`;
      const slow = await submitDownload(request, `${base}/slow/${slowName}`, slowName);
      ids.push(slow);
      await expect(drawer.locator(`[data-job-panel-group="active"] article[data-job-id="${slow}"]`)).toContainText('Running', { timeout: 15_000 });

      // More newer jobs finish than Finished shows (ten by default).
      for (let index = 0; index < 11; index++) {
        const name = `watched-quick-${stamp}-${index}.bin`;
        ids.push(await submitDownload(request, `${base}/ok/${name}`, name));
      }
      for (const id of ids.slice(1)) await waitForState(request, id, 'succeeded');
      await waitForState(request, slow, 'succeeded');

      const finished = drawer.locator('[data-job-panel-group="finished"] article[data-job-id]');
      await expect(finished.first()).toHaveAttribute('data-job-id', slow, { timeout: 15_000 });
      await expect(drawer.locator('[data-job-panel-group-more="finished"]')).toContainText('Showing the 10 most recently finished.');
    } finally {
      await dismiss(request, ids);
      server.close();
    }
  });

  test('a capped group says so, links to the rest, and its badge does not show the page as the total', async ({ page }) => {
    const rows = Array.from({ length: 50 }, (_, index) => ({
      id: `capped-${String(index).padStart(2, '0')}`, kind: 'remote-download', state: 'failed', version: 2,
      title: `Capped ${index}`, acceptedAt: '2026-09-27T10:00:00Z', stateEnteredAt: `2026-09-27T10:${String(59 - index).padStart(2, '0')}:00Z`,
      failure: { message: 'HTTP 404 Not Found' }, commands: [], outputs: [],
    }));
    await page.route(/\/v1\/jobs(?:\?.*)?$/, route => {
      const states = new URL(route.request().url()).searchParams.getAll('state');
      return route.fulfill({ json: states.includes('failed') ? { jobs: rows, nextCursor: 'more' } : { jobs: [] } });
    });
    await page.goto('/dashboard');
    await expect(page.locator('[data-job-panel-attention-badge]')).toHaveText('50+');
    await expect(page.locator('#job-panel-trigger-counts')).toHaveText('Showing 0 active or scheduled jobs and 50 needing attention, more on All jobs');

    await page.keyboard.press('Control+Shift+D');
    const drawer = drawerOf(page);
    await expect(drawer.locator('#job-panel-group-attention')).toHaveText(/Needs attention\s*\(50\+\)/);
    const more = drawer.locator('[data-job-panel-group-more="attention"]');
    await expect(more).toContainText('Showing the 50 most recent.');
    await expect(more.getByRole('link', { name: 'See every job that needs attention' }))
      .toHaveAttribute('href', '/jobs?state=blocked&state=failed&state=interrupted&dismissed=false');
  });

  test('a dismissal in one tab takes the row out of the drawer in another', async ({ page, request, context }) => {
    const { server, base } = await startServer();
    const ids: string[] = [];
    try {
      const name = `two-tabs-${Date.now()}.bin`;
      ids.push(await submitDownload(request, `${base}/missing/${name}`, name));
      await waitForState(request, ids[0], 'failed');

      const other = await context.newPage();
      for (const tab of [page, other]) {
        await tab.goto('/dashboard');
        await tab.keyboard.press('Control+Shift+D');
        await expect(drawerOf(tab).locator(`article[data-job-id="${ids[0]}"]`)).toBeVisible({ timeout: 15_000 });
      }

      const row = drawerOf(page).locator(`article[data-job-id="${ids[0]}"]`);
      // A dismissal can be undone, so it asks nothing first.
      await row.getByRole('button', { name: 'Dismiss', exact: true }).click();
      await expect(row).toHaveCount(0);

      await expect(drawerOf(other).locator(`article[data-job-id="${ids[0]}"]`)).toHaveCount(0, { timeout: 5_000 });
      await other.close();
    } finally {
      await dismiss(request, ids);
      server.close();
    }
  });
});

test.describe('Job Center pages when a read fails', () => {
  test('a Job page whose read failed offers Try again and a way back to All jobs', async ({ page, request }) => {
    const { server, base } = await startServer();
    const ids: string[] = [];
    try {
      const name = `detail-retry-${Date.now()}.bin`;
      ids.push(await submitDownload(request, `${base}/missing/${name}`, name));
      await waitForState(request, ids[0], 'failed');

      let failing = true;
      await page.route(`**/v1/jobs/${ids[0]}`, route => failing
        ? route.fulfill({ status: 500, json: { error: 'job request failed' } })
        : route.fallback());
      await page.goto(`/job?id=${ids[0]}`);
      const error = page.locator('[data-job-detail-error]');
      await expect(error.getByRole('alert')).toHaveText('job request failed');
      await expect(error.getByRole('link', { name: 'All jobs' })).toHaveAttribute('href', '/jobs?dismissed=false');

      failing = false;
      await error.getByRole('button', { name: 'Try again' }).click();
      await expect(page.getByRole('heading', { level: 1, name })).toBeVisible();
      await expect(error).toBeHidden();
    } finally {
      await dismiss(request, ids);
      server.close();
    }
  });

  test('the Job Center says when its list could not be refreshed, and catches up when it can', async ({ page, request }) => {
    const { server, base } = await startServer();
    const ids: string[] = [];
    try {
      await page.goto('/jobs?dismissed=false');
      const status = page.getByTestId('job-live-status');
      await expect(status).toHaveText('Live updates connected', { timeout: 15_000 });

      let failing = true;
      await page.route(/\/jobs\?dismissed=false$/, route => failing
        ? route.fulfill({ status: 500, body: 'unavailable' })
        : route.fallback());
      const name = `list-refresh-${Date.now()}.bin`;
      ids.push(await submitDownload(request, `${base}/missing/${name}`, name));
      await expect(status).toHaveText('The list could not be refreshed and may be out of date; trying again', { timeout: 15_000 });

      failing = false;
      await expect(status).toHaveText('Live updates connected', { timeout: 15_000 });
      await expect(page.locator(`[data-job-id="${ids[0]}"]`)).toBeVisible();
    } finally {
      await dismiss(request, ids);
      server.close();
    }
  });
  test('a running card on the Job Center moves with its progress frames, without refetching the list', async ({ page, request }) => {
    const { server, base, release } = await startServer();
    const ids: string[] = [];
    try {
      const name = `job-center-progress-${Date.now()}.bin`;
      await page.goto(`/jobs?search=${encodeURIComponent(name)}&dismissed=false`);
      await expect(page.getByTestId('job-live-status')).toHaveText('Live updates connected', { timeout: 15_000 });
      // Held, so a page slow to show the first frame does not find the transfer finished.
      ids.push(await submitDownload(request, `${base}/held/${name}`, name));
      const card = page.locator(`[data-job-id="${ids[0]}"]`);
      const value = card.locator('[data-job-progress-value]');
      await expect(value).toHaveText(/^\d+%$/, { timeout: 15_000 });

      // From here on the list is only refetched for a lifecycle change, and the
      // download's next one is its success.
      let refetches = 0;
      page.on('request', request => {
        if (new URL(request.url()).pathname === '/jobs') refetches += 1;
      });
      const first = Number((await value.textContent())!.replace('%', ''));
      release();
      await expect.poll(async () => Number((await value.textContent())!.replace('%', '')), { timeout: 10_000 })
        .toBeGreaterThan(first);
      const moved = Number((await value.textContent())!.replace('%', ''));
      if (moved < 100) expect(refetches).toBe(0);
      await expect(card.locator('[data-job-stats]')).toContainText('/s');
      await waitForState(request, ids[0], 'succeeded');
    } finally {
      await dismiss(request, ids);
      server.close();
    }
  });
});
