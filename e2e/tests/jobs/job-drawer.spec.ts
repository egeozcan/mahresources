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
    await expect(reason).not.toContainText('secret-path');

    await page.goto(`/jobs?search=${encodeURIComponent(name)}`);
    const card = page.locator(`article[data-job-id="${id}"]`);
    await expect(card).toContainText('connection refused');
    await expect(card).not.toContainText('signature');
  });
  test('a download that fails while the drawer is open is announced from inside it, with its reason', async ({ page, request }) => {
    // Held long enough for the drawer to see the Job running, so the failure is
    // a live transition, which is what gets announced.
    const server = http.createServer((_request, response) => {
      setTimeout(() => {
        response.writeHead(403, { 'Content-Type': 'text/plain' });
        response.end('no');
      }, 3000);
    });
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
      await expect(drawer.locator(`article:has(a[href="/job?id=${id}"])`)).toBeVisible({ timeout: 10_000 });

      const announcer = drawer.locator('[data-job-panel-announcer]');
      await expect(announcer).toHaveAttribute('role', 'status');
      await expect(announcer).toContainText(`${name} failed: HTTP 403 Forbidden`, { timeout: 15_000 });
    } finally {
      server.close();
    }
  });
});
