import http from 'node:http';
import { randomBytes } from 'node:crypto';
import type { AddressInfo } from 'node:net';
import type { APIRequestContext, Page } from '@playwright/test';
import { test, expect } from '../../fixtures/base.fixture';

// The jobs drawer tells the page's resource lists to refresh when a download
// finishes, so the resource it created appears without a reload. The refresh
// morphs the list in place, which drops anything open inside it (an inline tag
// editor, a description being edited), so it must follow a download this page
// saw finish and nothing else.

// Answers at once with a few fresh bytes, so no run collides on content hash
// and a download's whole life fits inside one publish tick of the job runtime.
async function startInstantServer() {
  const server = http.createServer((_request, response) => {
    const body = randomBytes(2048);
    response.writeHead(200, { 'Content-Type': 'application/octet-stream', 'Content-Length': String(body.length) });
    response.end(body);
  });
  await new Promise<void>(resolve => server.listen(0, '127.0.0.1', resolve));
  return { server, base: `http://127.0.0.1:${(server.address() as AddressInfo).port}` };
}

async function submitDownload(request: APIRequestContext, url: string, ownerId: number, name: string) {
  const response = await request.post('/v1/download/submit', {
    data: { URL: url, OwnerId: ownerId, Name: name, FileName: name },
  });
  expect(response.status(), await response.text()).toBe(202);
  const id = (await response.json()).jobs?.[0]?.canonicalJobId as string;
  expect(id).toEqual(expect.any(String));
  return id;
}

// Terminal, with every event given a delivery sequence: a stream connected
// after this replays none of it as live.
async function waitUntilPublished(request: APIRequestContext, id: string) {
  await expect.poll(async () => {
    const response = await request.get(`/v1/jobs/${encodeURIComponent(id)}`);
    return response.ok() ? (await response.json()).state : null;
  }, { timeout: 30_000 }).toBe('succeeded');
  await expect.poll(async () => {
    const response = await request.get(`/v1/jobs/${encodeURIComponent(id)}/events`);
    if (!response.ok()) return false;
    const events = (await response.json()).events || [];
    return events.length > 0 && events.every((event: any) => Number(event.deliverySequence) > 0);
  }, { timeout: 30_000 }).toBe(true);
}

// Records every download-completed the page dispatches, and whether the
// drawer's stream has caught up, before any page script runs.
async function observeDrawer(page: Page) {
  await page.addInitScript(() => {
    const w = window as any;
    w.__downloadCompleted = [];
    w.__streamCaughtUp = false;
    window.addEventListener('download-completed', event => {
      w.__downloadCompleted.push((event as CustomEvent).detail);
    });
    const NativeEventSource = window.EventSource;
    w.EventSource = class extends NativeEventSource {
      constructor(url: string | URL, init?: EventSourceInit) {
        super(url, init);
        this.addEventListener('job-caught-up', () => { w.__streamCaughtUp = true; });
      }
    };
  });
}

async function createGroup(request: APIRequestContext, name: string): Promise<number> {
  const response = await request.post('/v1/group', { data: { Name: name } });
  expect(response.ok(), await response.text()).toBe(true);
  const group = await response.json();
  return group.ID ?? group.id;
}

async function drawerHolds(page: Page, id: string) {
  await page.waitForFunction(jobId => {
    const root = document.querySelector('[data-testid="job-panel-root"]');
    const data = root && (window as any).Alpine?.$data(root);
    return !!data?.jobs?.some((job: any) => job.id === jobId);
  }, id, { timeout: 15_000 });
}

test.describe('Resource list refresh after downloads', () => {
  let server: http.Server;
  let base: string;

  test.beforeAll(async () => {
    ({ server, base } = await startInstantServer());
  });

  test.afterAll(async () => {
    await new Promise<void>(resolve => server.close(() => resolve()));
  });

  test('a download that finished before the page loaded refreshes no list', async ({ page, request }) => {
    const stamp = `${Date.now()}-${Math.random().toString(36).slice(2)}`;
    const groupId = await createGroup(request, `list-refresh-before-${stamp}`);
    const name = `list-refresh-before-${stamp}.bin`;
    const id = await submitDownload(request, `${base}/${name}`, groupId, name);
    await waitUntilPublished(request, id);

    await observeDrawer(page);
    // The drawer's list reads answer only once its stream has caught up, the
    // order a slow server gives them.
    await page.route(url => url.pathname === '/v1/jobs', async route => {
      await expect.poll(() => page.evaluate(() => (window as any).__streamCaughtUp), { timeout: 15_000 }).toBe(true);
      await route.continue();
    });

    await page.goto(`/resources?OwnerId=${groupId}`);
    await expect(page.getByRole('link', { name, exact: true })).toBeVisible();
    await drawerHolds(page, id);

    expect(await page.evaluate(() => (window as any).__downloadCompleted)).toEqual([]);
    await page.unrouteAll({ behavior: 'ignoreErrors' });
  });

  test('a download that finishes while the page is open refreshes the list', async ({ page, request }) => {
    const stamp = `${Date.now()}-${Math.random().toString(36).slice(2)}`;
    const groupId = await createGroup(request, `list-refresh-live-${stamp}`);
    const name = `list-refresh-live-${stamp}.bin`;

    await observeDrawer(page);
    await page.goto(`/resources?OwnerId=${groupId}`);
    await expect.poll(() => page.evaluate(() => (window as any).__streamCaughtUp), { timeout: 15_000 }).toBe(true);
    await expect(page.getByRole('link', { name, exact: true })).toHaveCount(0);

    const id = await submitDownload(request, `${base}/${name}`, groupId, name);

    await expect(page.getByRole('link', { name, exact: true })).toBeVisible({ timeout: 20_000 });
    expect(await page.evaluate(() => (window as any).__downloadCompleted)).toEqual([{ jobId: id }]);
  });
});
