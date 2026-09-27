import http from 'node:http';
import { randomBytes } from 'node:crypto';
import type { AddressInfo } from 'node:net';
import { test, expect } from '../../fixtures/base.fixture';

// A download slow enough to be watched: 5 MiB at 64 KiB every 100 ms is about
// eight seconds, long enough for the live stream to deliver several progress
// frames and for the series to record a rate.
const SIZE = 5 * 1024 * 1024;
const CHUNK = 64 * 1024;

async function startSlowServer() {
  const server = http.createServer((request, response) => {
    // Fresh bytes per request, so no run of this test collides on content hash
    // with another and is folded into an existing resource.
    const body = randomBytes(SIZE);
    response.writeHead(200, { 'Content-Type': 'application/octet-stream', 'Content-Length': String(SIZE) });
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
  return { server, port: (server.address() as AddressInfo).port };
}

async function readJob(request: import('@playwright/test').APIRequestContext, id: string) {
  const response = await request.get(`/v1/jobs/${encodeURIComponent(id)}`);
  return response.ok() ? response.json() : null;
}

// Answers at once: /missing/* with a 404, anything else with a few fresh bytes,
// so a download's whole life fits inside one publish tick of the job runtime.
async function startInstantServer() {
  const server = http.createServer((request, response) => {
    if (request.url?.startsWith('/missing/')) {
      response.writeHead(404, { 'Content-Type': 'text/plain' });
      response.end('gone');
      return;
    }
    const body = randomBytes(2048);
    response.writeHead(200, { 'Content-Type': 'application/octet-stream', 'Content-Length': String(body.length) });
    response.end(body);
  });
  await new Promise<void>(resolve => server.listen(0, '127.0.0.1', resolve));
  return { server, base: `http://127.0.0.1:${(server.address() as AddressInfo).port}` };
}

type Announcement = { text: string; inDrawer: boolean };

// Every text a live region takes, and whether that region sits in the drawer.
// Regions replace their text, so reading one at the end would miss all but the
// last message.
async function recordAnnouncements(page: import('@playwright/test').Page) {
  await page.addInitScript(() => {
    const said: { text: string; inDrawer: boolean }[] = [];
    (window as any).__announced = said;
    const last = new WeakMap<Element, string>();
    const isRegion = (element: Element) => element.hasAttribute('aria-live') || ['status', 'alert'].includes(element.getAttribute('role') || '');
    const regionOf = (node: Node | null) => {
      let element = node && node.nodeType === 1 ? node as Element : node?.parentElement || null;
      while (element && !isRegion(element)) element = element.parentElement;
      return element;
    };
    const check = (region: Element) => {
      const text = (region.textContent || '').replace(/\s+/g, ' ').trim();
      if (text === last.get(region)) return;
      last.set(region, text);
      if (text) said.push({ text, inDrawer: !!region.closest('#job-center-panel') });
    };
    const start = () => new MutationObserver(mutations => {
      const regions = new Set<Element>();
      for (const mutation of mutations) {
        const region = regionOf(mutation.target);
        if (region) regions.add(region);
        mutation.addedNodes.forEach(node => {
          if (node.nodeType !== 1) return;
          if (isRegion(node as Element)) regions.add(node as Element);
          (node as Element).querySelectorAll('[aria-live],[role=status],[role=alert]').forEach(element => regions.add(element));
        });
      }
      regions.forEach(check);
    }).observe(document.documentElement, { subtree: true, childList: true, characterData: true });
    if (document.documentElement) start();
    else document.addEventListener('DOMContentLoaded', start);
  });
}

// The state each job was in when the drawer's ledger first heard of it. The
// tests below are about jobs whose first sight is already their outcome; this
// is how they know that is what they exercised.
async function recordFirstSights(page: import('@playwright/test').Page) {
  await page.evaluate(() => {
    const panel = (window as any).Alpine.$data(document.querySelector('[data-testid="job-panel-root"]'));
    const firstSights: Record<string, string> = {};
    (window as any).__firstSights = firstSights;
    const hearJob = panel.hearJob;
    panel.hearJob = function (job: any, options: any) {
      if (job?.id && job.state && !(job.id in firstSights)) firstSights[job.id] = job.state;
      return hearJob.call(this, job, options);
    };
  });
}

function firstSight(page: import('@playwright/test').Page, id: string): Promise<string | undefined> {
  return page.evaluate(jobId => (window as any).__firstSights[jobId], id);
}

function announcements(page: import('@playwright/test').Page): Promise<Announcement[]> {
  return page.evaluate(() => (window as any).__announced.slice());
}

function mentions(said: Announcement[], name: string) {
  return said.filter(entry => entry.text.includes(name));
}

// The panel's own word on whether the stream is live: after its catch-up, a
// lifecycle event is news; before it, replay.
async function waitUntilLive(page: import('@playwright/test').Page) {
  await expect.poll(() => page.evaluate(() => {
    const root = document.querySelector('[data-testid="job-panel-root"]');
    const panel = root && (window as any).Alpine?.$data(root);
    return !!panel?.streamCaughtUp && panel.connectionStatus === 'connected';
  }), { timeout: 15_000 }).toBe(true);
}

async function submitDownload(
  from: import('@playwright/test').APIRequestContext | import('@playwright/test').Page,
  url: string,
  name: string,
): Promise<string> {
  const data = { URL: url, Name: name, FileName: name };
  let status: number;
  let body: any;
  if ('evaluate' in from) {
    // From the page itself, as a script in this tab would submit it.
    ({ status, body } = await from.evaluate(async payload => {
      const response = await fetch('/v1/download/submit', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
        body: JSON.stringify(payload),
      });
      return { status: response.status, body: await response.json() };
    }, data));
  } else {
    const response = await from.post('/v1/download/submit', { data });
    status = response.status();
    body = await response.json();
  }
  expect(status, JSON.stringify(body)).toBe(202);
  const id = body.jobs?.[0]?.canonicalJobId as string;
  expect(id).toEqual(expect.any(String));
  return id;
}

// Settled: terminal, with every event given a delivery sequence, so the stream
// has either delivered it or will replay it.
async function waitUntilPublished(request: import('@playwright/test').APIRequestContext, id: string, state: string) {
  await expect.poll(async () => (await readJob(request, id))?.state, { timeout: 30_000 }).toBe(state);
  await expect.poll(async () => {
    const response = await request.get(`/v1/jobs/${encodeURIComponent(id)}/events`);
    if (!response.ok()) return false;
    const events = (await response.json()).events || [];
    return events.length > 0 && events.every((event: any) => Number(event.deliverySequence) > 0);
  }, { timeout: 30_000 }).toBe(true);
}

test.describe('Jobs drawer', () => {
  test('record-keeping commands wait under More, open by keyboard, and still work after the row updates', async ({ page }) => {
    const id = 'drawer-more-disclosure';
    let pinned = false;
    const pinRequests: string[] = [];
    const job = () => ({
      id, kind: 'remote-download', state: 'failed', version: pinned ? 2 : 1, pinned,
      title: 'More disclosure job', acceptedAt: new Date().toISOString(),
      failure: { message: 'connection refused' },
      commands: [
        { key: 'retry', label: 'Retry', endpoint: `/v1/jobs/${id}/commands/retry`, jobVersion: pinned ? 2 : 1 },
        { key: 'pin', label: 'Pin', endpoint: `/v1/jobs/${id}/commands/pin`, jobVersion: 1 },
        { key: 'unpin', label: 'Unpin', endpoint: `/v1/jobs/${id}/commands/unpin`, jobVersion: 2 },
        { key: 'forget', label: 'Forget replay input', endpoint: `/v1/jobs/${id}/commands/forget`, jobVersion: pinned ? 2 : 1 },
      ],
      outputs: [],
      lineage: { ancestors: [], successors: [], parents: [], children: [] },
    });
    await page.route(/\/v1\/jobs(?:\?.*)?$/, route => {
      const states = new URL(route.request().url()).searchParams.getAll('state');
      return route.fulfill({ json: { jobs: states.includes('failed') ? [job()] : [], nextCursor: null } });
    });
    await page.route(`**/v1/jobs/${id}`, route => route.fulfill({ json: job() }));
    let releaseUnpin = () => {};
    let unpinArrived = () => {};
    const unpinPending = new Promise<void>(resolve => { unpinArrived = resolve; });
    await page.route(`**/v1/jobs/${id}/commands/unpin`, async route => {
      const held = new Promise<void>(resolve => { releaseUnpin = resolve; });
      unpinArrived();
      await held;
      pinned = false;
      return route.fulfill({ json: { result: { job: job(), message: 'Unpin requested.' } } });
    });
    await page.route(`**/v1/jobs/${id}/commands/pin`, route => {
      pinRequests.push(route.request().method());
      pinned = true;
      return route.fulfill({ json: { result: { job: job(), message: 'Pin requested.' } } });
    });

    await page.goto('/dashboard');
    await page.keyboard.press('Control+Shift+D');
    const drawer = page.getByRole('dialog', { name: 'Jobs' });
    const row = drawer.locator(`article[data-job-id="${id}"]`);
    const controls = row.getByRole('group', { name: 'Advertised controls' });
    // The counts the trigger describes are said inside the modal too, zeroes included.
    await expect(drawer).toHaveAccessibleDescription('Showing 0 active or scheduled jobs and 1 needing attention');

    // Acting on the work stays inline; keeping the record waits under More.
    await expect(controls.getByRole('button', { name: 'Retry', exact: true })).toBeVisible();
    const pin = controls.getByRole('button', { name: 'Pin', exact: true });
    await expect(pin).toBeHidden();
    await expect(controls.getByRole('button', { name: 'Forget replay input' })).toBeHidden();

    const more = controls.locator('summary', { hasText: 'More' });
    await more.focus();
    await page.keyboard.press('Enter');
    await expect(pin).toBeVisible();
    await expect(controls.getByRole('button', { name: 'Forget replay input' })).toBeVisible();
    await page.keyboard.press('Tab');
    await expect(pin).toBeFocused();

    await page.keyboard.press('Enter');
    const confirmation = page.getByRole('alertdialog');
    await expect(confirmation).toBeVisible();
    await confirmation.getByRole('button', { name: 'Pin', exact: true }).click();
    await expect.poll(() => pinRequests).toEqual(['POST']);

    // The updated row swaps Pin for Unpin under the same disclosure, which
    // stays usable rather than being rebuilt shut.
    await expect(row.getByText('Pinned by you', { exact: true })).toBeVisible();
    await expect(pin).toHaveCount(0);
    const unpin = controls.getByRole('button', { name: 'Unpin', exact: true });
    await expect(unpin).toBeVisible();
    // Pin left the row with focus on it; the reader lands on what replaced it,
    // not on the drawer's Close button.
    await expect(unpin).toBeFocused();

    // Focus the reader moves while a command is pending is theirs: when Unpin's
    // answer replaces it, focus stays where they put it. The target is not the
    // Close button, which is where the trap parks focus on its own.
    await page.keyboard.press('Enter');
    await unpinPending;
    const allJobs = drawer.getByRole('link', { name: 'All jobs', exact: true });
    await allJobs.focus();
    releaseUnpin();
    await expect(controls.getByRole('button', { name: 'Pin', exact: true })).toBeVisible();
    await expect(unpin).toHaveCount(0);
    // A restore would land a macrotask after the row re-renders; give it that long.
    await page.waitForTimeout(100);
    await expect(allJobs).toBeFocused();
  });

  test('a focus move the reader makes after the row re-rendered the command is still theirs', async ({ page }) => {
    const id = 'drawer-focus-after-rerender';
    let pinned = true;
    const job = () => ({
      id, kind: 'remote-download', state: 'failed', version: pinned ? 2 : 3, pinned,
      title: 'Re-render focus job', acceptedAt: new Date().toISOString(),
      failure: { message: 'connection refused' },
      commands: [
        { key: 'retry', label: 'Retry', endpoint: `/v1/jobs/${id}/commands/retry`, jobVersion: pinned ? 2 : 3 },
        { key: 'pin', label: 'Pin', endpoint: `/v1/jobs/${id}/commands/pin`, jobVersion: 3 },
        { key: 'unpin', label: 'Unpin', endpoint: `/v1/jobs/${id}/commands/unpin`, jobVersion: 2 },
      ],
      outputs: [],
      lineage: { ancestors: [], successors: [], parents: [], children: [] },
    });
    await page.route(/\/v1\/jobs(?:\?.*)?$/, route => {
      const states = new URL(route.request().url()).searchParams.getAll('state');
      return route.fulfill({ json: { jobs: states.includes('failed') ? [job()] : [], nextCursor: null } });
    });
    await page.route(`**/v1/jobs/${id}`, route => route.fulfill({ json: job() }));
    let releaseUnpin = () => {};
    let unpinArrived = () => {};
    const unpinPending = new Promise<void>(resolve => { unpinArrived = resolve; });
    await page.route(`**/v1/jobs/${id}/commands/unpin`, async route => {
      const held = new Promise<void>(resolve => { releaseUnpin = resolve; });
      unpinArrived();
      await held;
      return route.fulfill({ json: { result: { job: job(), message: 'Unpin requested.' } } });
    });

    await page.goto('/dashboard');
    await page.keyboard.press('Control+Shift+D');
    const drawer = page.getByRole('dialog', { name: 'Jobs' });
    const row = drawer.locator(`article[data-job-id="${id}"]`);
    const controls = row.getByRole('group', { name: 'Advertised controls' });
    await controls.locator('summary', { hasText: 'More' }).click();
    const unpin = controls.getByRole('button', { name: 'Unpin', exact: true });
    await unpin.focus();
    await page.keyboard.press('Enter');
    await unpinPending;

    // An update lands while the command is pending and replaces the control.
    pinned = false;
    await page.evaluate(snapshot => {
      const root = document.querySelector('[data-testid="job-panel-root"]');
      (window as any).Alpine.$data(root).applyStreamSnapshot(snapshot);
    }, job());
    await expect(unpin).toHaveCount(0);

    // Then the reader goes somewhere of their own.
    const allJobs = drawer.getByRole('link', { name: 'All jobs', exact: true });
    await allJobs.focus();
    releaseUnpin();
    // A restore would land a macrotask after the command settles; give it that long.
    await page.waitForTimeout(300);
    await expect(allJobs).toBeFocused();
  });

  test('a row\'s Dismiss takes the row away, shows no box, and hands focus to the next row', async ({ page }) => {
    const rows = [
      { id: 'drawer-dismiss-a', title: 'Dismiss me', acceptedAt: '2026-09-26T10:00:02Z' },
      { id: 'drawer-dismiss-b', title: 'Keep me', acceptedAt: '2026-09-26T10:00:01Z' },
    ].map(row => ({
      ...row, kind: 'remote-download', state: 'failed', version: 2, failure: { message: 'connection refused' },
      commands: [{ key: 'dismiss', label: 'Dismiss', endpoint: `/v1/jobs/${row.id}/commands/dismiss`, jobVersion: 2 }],
      outputs: [], lineage: { ancestors: [], successors: [], parents: [], children: [] },
    }));
    const dismissed = new Set<string>();
    await page.route(/\/v1\/jobs(?:\?.*)?$/, route => {
      const states = new URL(route.request().url()).searchParams.getAll('state');
      return route.fulfill({ json: { jobs: states.includes('failed') ? rows.filter(row => !dismissed.has(row.id)) : [], nextCursor: null } });
    });
    for (const row of rows) {
      await page.route(`**/v1/jobs/${row.id}`, route => route.fulfill({ json: row }));
      // As the server answers: a preference is recorded and no job event follows.
      await page.route(`**/v1/jobs/${row.id}/commands/dismiss`, route => {
        dismissed.add(row.id);
        return route.fulfill({ json: { result: { status: 'succeeded', code: 'applied', message: 'dismissed' } } });
      });
    }

    await page.goto('/dashboard');
    await page.keyboard.press('Control+Shift+D');
    const drawer = page.getByRole('dialog', { name: 'Jobs' });
    const first = drawer.locator('article[data-job-id="drawer-dismiss-a"]');
    await first.getByRole('button', { name: 'Dismiss', exact: true }).focus();
    await page.keyboard.press('Enter');
    await page.getByRole('alertdialog').getByRole('button', { name: 'Dismiss', exact: true }).click();

    await expect(first).toHaveCount(0);
    await expect(drawer.locator('[data-job-panel-notice]')).toBeHidden();
    await expect(drawer.locator('[data-job-panel-announcer]')).toHaveText('Dismiss me dismissed.');
    await expect(drawer.getByRole('link', { name: 'Keep me', exact: true })).toBeFocused();
  });

  test('dismissing the only row hands focus to the job the refresh reveals', async ({ page }) => {
    const rows = [
      { id: 'drawer-reveal-a', title: 'Only shown' },
      { id: 'drawer-reveal-b', title: 'Revealed next' },
    ].map(row => ({
      ...row, kind: 'remote-download', state: 'failed', version: 2, acceptedAt: '2026-09-26T10:00:00Z',
      commands: [{ key: 'dismiss', label: 'Dismiss', endpoint: `/v1/jobs/${row.id}/commands/dismiss`, jobVersion: 2 }],
      outputs: [], lineage: { ancestors: [], successors: [], parents: [], children: [] },
    }));
    let dismissed = false;
    await page.route(/\/v1\/jobs(?:\?.*)?$/, route => {
      const states = new URL(route.request().url()).searchParams.getAll('state');
      // Only one job fits until the first is dismissed.
      return route.fulfill({ json: { jobs: states.includes('failed') ? [dismissed ? rows[1] : rows[0]] : [], nextCursor: null } });
    });
    for (const row of rows) await page.route(`**/v1/jobs/${row.id}`, route => route.fulfill({ json: row }));
    await page.route('**/v1/jobs/drawer-reveal-a/commands/dismiss', route => {
      dismissed = true;
      return route.fulfill({ json: { result: { status: 'succeeded', code: 'applied', message: 'dismissed' } } });
    });

    await page.goto('/dashboard');
    await page.keyboard.press('Control+Shift+D');
    const drawer = page.getByRole('dialog', { name: 'Jobs' });
    await drawer.locator('article[data-job-id="drawer-reveal-a"]').getByRole('button', { name: 'Dismiss', exact: true }).focus();
    await page.keyboard.press('Enter');
    await page.getByRole('alertdialog').getByRole('button', { name: 'Dismiss', exact: true }).click();

    await expect(drawer.getByRole('link', { name: 'Revealed next', exact: true })).toBeFocused();
  });

  test('a running download shows its bar, speed, time left and a speed graph, live', async ({ page, request }) => {
    const { server, port } = await startSlowServer();
    try {
      const stamp = Date.now();
      const name = `job-drawer-${stamp}.bin`;
      const group = await (await request.post('/v1/group', { data: { Name: `job-drawer-${stamp}` } })).json();
      const response = await request.post('/v1/download/submit', {
        data: { URL: `http://127.0.0.1:${port}/${name}`, OwnerId: group.ID ?? group.id, Name: name, FileName: name },
      });
      expect(response.status(), await response.text()).toBe(202);
      const id = (await response.json()).jobs?.[0]?.canonicalJobId as string;
      expect(id).toEqual(expect.any(String));

      await page.goto('/dashboard');
      await page.keyboard.press('Control+Shift+D');
      const drawer = page.getByRole('dialog', { name: 'Jobs' });
      await expect(drawer).toBeVisible();
      // A drawer: pinned to the right edge and as tall as the viewport, once its
      // entrance has finished.
      await drawer.evaluate(element => Promise.all(element.getAnimations().map(animation => animation.finished)));
      const box = await drawer.boundingBox();
      const viewport = page.viewportSize()!;
      expect(Math.round(box!.x + box!.width)).toBe(viewport.width);
      expect(Math.round(box!.height)).toBe(viewport.height);

      const row = drawer.locator(`article:has(a[href="/job?id=${id}"])`);
      await expect(row).toBeVisible({ timeout: 10_000 });
      const bar = row.getByRole('progressbar');
      await expect.poll(async () => Number(await bar.getAttribute('aria-valuenow')), { timeout: 10_000 })
        .toBeGreaterThan(0);
      const first = Number(await bar.getAttribute('aria-valuenow'));
      const stats = row.locator('[data-job-panel-stats]');
      await expect(stats).toContainText(/ of 5\.0 MB/);
      await expect(stats).toContainText(/\d+(\.\d)? (KB|MB)\/s/, { timeout: 10_000 });
      await expect(stats).toContainText(/left|almost done/);
      await expect(row.getByRole('img', { name: /^Speed/ })).toBeVisible({ timeout: 10_000 });
      // The bar keeps moving without a reload: that is the live feed working.
      await expect.poll(async () => Number(await bar.getAttribute('aria-valuenow')), { timeout: 10_000 })
        .toBeGreaterThan(first);

      await expect.poll(async () => (await readJob(request, id))?.state, { timeout: 30_000 }).toBe('succeeded');
      const job = await readJob(request, id);
      expect(job.progress.averageRate).toBeGreaterThan(0);
      expect(job.progress.series.points.length).toBeGreaterThanOrEqual(3);
    } finally {
      server.close();
    }
  });
  test('a failed download says why, in the drawer and on /jobs, without its URL', async ({ page, request }) => {
    // A port nothing listens on: net/http reports the refusal as
    // `Get "<url>": dial tcp …`, so the queue's own error names the whole URL and
    // the assertions below have something to find if the redaction ever stops.
    const probe = http.createServer();
    await new Promise<void>(resolve => probe.listen(0, '127.0.0.1', resolve));
    const port = (probe.address() as AddressInfo).port;
    await new Promise<void>(resolve => probe.close(() => resolve()));

    const name = `job-drawer-failed-${Date.now()}.bin`;
    const response = await request.post('/v1/download/submit', {
      data: { URL: `http://127.0.0.1:${port}/secret-path/${name}?signature=do-not-show`, Name: name, FileName: name },
    });
    expect(response.status(), await response.text()).toBe(202);
    const id = (await response.json()).jobs?.[0]?.canonicalJobId as string;
    await expect.poll(async () => (await readJob(request, id))?.state, { timeout: 30_000 }).toBe('failed');

    await page.goto('/dashboard');
    await page.keyboard.press('Control+Shift+D');
    const drawer = page.getByRole('dialog', { name: 'Jobs' });
    const row = drawer.locator(`article:has(a[href="/job?id=${id}"])`);
    const reason = row.locator('[data-job-panel-failure]');
    await expect(reason).toBeVisible({ timeout: 10_000 });
    await expect(reason).toContainText('Reason:');
    await expect(reason).toContainText('connection refused');
    await expect(reason).not.toContainText('signature');
    await expect(reason).not.toContainText('do-not-show');
    await expect(reason).not.toContainText('secret-path');

    await page.goto(`/jobs?search=${encodeURIComponent(name)}`);
    const card = page.locator(`article[data-job-id="${id}"]`);
    await expect(card).toContainText('connection refused');
    await expect(card).not.toContainText('signature');
    await expect(card).not.toContainText('do-not-show');
  });
  test('a download that fails while the drawer is open is announced from inside it, with its reason', async ({ page, request }) => {
    // Held until the drawer has shown the Job running, so the failure is a live
    // transition, which is what gets announced. A fixed delay would race a slow
    // drawer, whose first sight of the Job could then be the failure itself.
    const held: http.ServerResponse[] = [];
    const server = http.createServer((_request, response) => { held.push(response); });
    const release = () => {
      for (const response of held.splice(0)) {
        response.writeHead(403, { 'Content-Type': 'text/plain' });
        response.end('no');
      }
    };
    await new Promise<void>(resolve => server.listen(0, '127.0.0.1', resolve));
    const port = (server.address() as AddressInfo).port;
    try {
      await page.goto('/dashboard');
      await page.keyboard.press('Control+Shift+D');
      const drawer = page.getByRole('dialog', { name: 'Jobs' });
      await expect(drawer.getByText('Live updates connected')).toBeVisible({ timeout: 10_000 });

      const name = `job-drawer-announced-${Date.now()}.bin`;
      const response = await request.post('/v1/download/submit', {
        data: { URL: `http://127.0.0.1:${port}/${name}`, Name: name, FileName: name },
      });
      expect(response.status(), await response.text()).toBe(202);
      const id = (await response.json()).jobs?.[0]?.canonicalJobId as string;
      const row = drawer.locator(`article:has(a[href="/job?id=${id}"])`);
      await expect(row).toBeVisible({ timeout: 10_000 });
      await expect(row).toContainText('Running', { timeout: 10_000 });
      await expect.poll(() => held.length, { timeout: 10_000 }).toBeGreaterThan(0);
      release();

      const announcer = drawer.locator('[data-job-panel-announcer]');
      await expect(announcer).toHaveAttribute('role', 'status');
      await expect(announcer).toContainText(`${name} failed: HTTP 403 Forbidden`, { timeout: 15_000 });
    } finally {
      release();
      server.close();
    }
  });
});

// A download that fails on a 404, or succeeds on a few bytes, is accepted,
// started and finished inside one publish tick, so the drawer's first read of it
// already finds the outcome. That outcome is news to someone who was connected
// when it happened, and history to a page that connected after.
test.describe('Jobs drawer announcements of jobs that finish at once', () => {
  for (const drawerOpen of [false, true]) {
    const where = drawerOpen ? 'from inside the open drawer' : 'from the page while the drawer is closed';
    test(`are said once, with the reason, ${where}`, async ({ page, request }) => {
      const { server, base } = await startInstantServer();
      try {
        const stamp = Date.now();
        // Finished and published before the page connects: history.
        const oldName = `instant-old-${stamp}.bin`;
        const oldId = await submitDownload(request, `${base}/missing/${oldName}`, oldName);
        await waitUntilPublished(request, oldId, 'failed');

        await recordAnnouncements(page);
        await page.goto('/dashboard');
        if (drawerOpen) {
          await page.keyboard.press('Control+Shift+D');
          await expect(page.getByRole('dialog', { name: 'Jobs' })).toBeVisible();
        }
        await waitUntilLive(page);
        await recordFirstSights(page);

        // One from another client, one from this tab; one failure, one success,
        // swapped between the two runs.
        const failedName = `instant-failed-${stamp}.bin`;
        const succeededName = `instant-succeeded-${stamp}.bin`;
        const failedId = await submitDownload(drawerOpen ? page : request, `${base}/missing/${failedName}`, failedName);
        const succeededId = await submitDownload(drawerOpen ? request : page, `${base}/ok/${succeededName}`, succeededName);

        await expect.poll(async () => mentions(await announcements(page), failedName).map(entry => entry.text).join(' | '), { timeout: 20_000 })
          .toContain(`${failedName} failed: HTTP 404 Not Found.`);
        await expect.poll(async () => mentions(await announcements(page), succeededName).map(entry => entry.text).join(' | '), { timeout: 20_000 })
          .toContain(`${succeededName} succeeded.`);

        // A later job's announcement comes from a refresh that re-reads both:
        // anything said twice would have been said by then.
        const laterName = `instant-later-${stamp}.bin`;
        await submitDownload(request, `${base}/missing/${laterName}`, laterName);
        await expect.poll(async () => mentions(await announcements(page), laterName).length, { timeout: 20_000 }).toBe(1);

        expect(await firstSight(page, failedId), 'the drawer must first see the job at its outcome').toBe('failed');
        expect(await firstSight(page, succeededId), 'the drawer must first see the job at its outcome').toBe('succeeded');
        const said = await announcements(page);
        expect(mentions(said, failedName)).toHaveLength(1);
        expect(mentions(said, succeededName)).toHaveLength(1);
        expect(mentions(said, oldName)).toEqual([]);
        for (const entry of [...mentions(said, failedName), ...mentions(said, succeededName)]) {
          expect(entry.inDrawer, entry.text).toBe(drawerOpen);
        }
      } finally {
        server.close();
      }
    });
  }

  test('a job that finished while the stream was down stays silent after the reconnect, and the next one is said', async ({ page, request }) => {
    const { server, base } = await startInstantServer();
    try {
      const stamp = Date.now();
      await recordAnnouncements(page);
      await page.goto('/dashboard');
      await waitUntilLive(page);

      // Chromium's offline emulation leaves a stream that is already open
      // alone, so the drop is made the way the browser reports one: the stream
      // closes and fires error. The panel's own connect() then opens a new
      // stream, and everything published meanwhile arrives as replay before its
      // catch-up. That is the boundary under test; the browser's own automatic
      // reconnect, resuming from Last-Event-ID, is not exercised here: it
      // replays less, but everything it replays also comes before catch-up.
      await page.evaluate(() => {
        const panel = (window as any).Alpine.$data(document.querySelector('[data-testid="job-panel-root"]'));
        panel.eventSource.close();
        panel.eventSource.dispatchEvent(new Event('error'));
      });
      const missedName = `instant-missed-${stamp}.bin`;
      const missedId = await submitDownload(request, `${base}/missing/${missedName}`, missedName);
      await waitUntilPublished(request, missedId, 'failed');
      await page.evaluate(() => {
        const panel = (window as any).Alpine.$data(document.querySelector('[data-testid="job-panel-root"]'));
        if (panel.streamCaughtUp) throw new Error('the dropped stream still reads as live');
        panel.eventSource = null;
        panel.connect();
      });
      await waitUntilLive(page);
      await recordFirstSights(page);
      // The catch-up refresh has read the missed job before the next one is submitted.
      await expect.poll(() => page.evaluate(id => {
        const panel = (window as any).Alpine.$data(document.querySelector('[data-testid="job-panel-root"]'));
        return panel.jobs.some((job: any) => job.id === id);
      }, missedId), { timeout: 15_000 }).toBe(true);

      const afterName = `instant-after-${stamp}.bin`;
      const afterId = await submitDownload(request, `${base}/missing/${afterName}`, afterName);
      await expect.poll(async () => mentions(await announcements(page), afterName).map(entry => entry.text).join(' | '), { timeout: 20_000 })
        .toContain(`${afterName} failed: HTTP 404 Not Found.`);
      expect(await firstSight(page, afterId), 'the drawer must first see the job at its outcome').toBe('failed');

      const said = await announcements(page);
      expect(mentions(said, afterName)).toHaveLength(1);
      expect(mentions(said, missedName)).toEqual([]);
    } finally {
      server.close();
    }
  });
});
