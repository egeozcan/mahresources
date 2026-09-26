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
