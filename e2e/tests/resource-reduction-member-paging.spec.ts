/**
 * Resource Reduction review: "Show more members" on a Cluster the page cut at its member
 * budget (design docs/plans/2026-11-24-reduction-plan-tables-design.md D6).
 *
 * A page shares a budget of copies among the Clusters over 12 copies, 2000 by default;
 * a Cluster past it renders only its kept copy and a "Show more members" button. Reaching
 * that with real data would take two thousand uploads, so this spec runs its own server
 * with REDUCTION_PAGE_MEMBER_BUDGET=20 and two Identical Clusters of 15 copies: the first
 * fits, the second is cut. (A Cluster stores at most 200 copies, so a cut Cluster's
 * remaining copies always come back in one page.)
 */
import path from 'path';
import { test as base, expect } from '@playwright/test';
import type { APIRequestContext, Locator, Page } from '@playwright/test';
import { ApiClient } from '../helpers/api-client';
import { ServerInfo, startServer, stopServer } from '../fixtures/server-manager';

const test = base.extend<{ apiClient: ApiClient }, { budgetServer: ServerInfo }>({
  budgetServer: [async ({}, use) => {
    const server = await startServer(3, { env: { REDUCTION_PAGE_MEMBER_BUDGET: '20' } });
    await use(server);
    await stopServer(server.proc);
  }, { scope: 'worker', auto: true }],

  baseURL: async ({ budgetServer }, use) => {
    await use(`http://127.0.0.1:${budgetServer.port}`);
  },

  apiClient: async ({ request, baseURL }, use) => {
    await use(new ApiClient(request, baseURL!));
  },
});

const COPIES = 15;

async function makeIdenticalCluster(request: APIRequestContext, baseURL: string, apiClient: ApiClient, label: string, image: string): Promise<number[]> {
  const filePath = path.join(__dirname, `../test-assets/${image}`);
  const first = await apiClient.createResource({ filePath, name: `${label} 00` });
  const stored = await request.get(`${baseURL}/v1/resource/view?id=${first.ID}`);
  expect(stored.ok()).toBeTruthy();
  const bytes = Buffer.from(await stored.body());
  const ids = [first.ID];
  for (let i = 1; i < COPIES; i++) {
    const copy = await apiClient.createResource({ filePath, name: `${label} ${String(i).padStart(2, '0')}` });
    const upload = await request.post(`${baseURL}/v1/resource/versions?resourceId=${copy.ID}`, {
      multipart: { file: { name: 'copy.png', mimeType: 'image/png', buffer: bytes }, comment: 'made identical to the first' },
    });
    expect(upload.ok()).toBeTruthy();
    ids.push(copy.ID);
  }
  return ids;
}

async function seedCutReduction(request: APIRequestContext, baseURL: string, apiClient: ApiClient): Promise<number> {
  const label = `RR paging ${Date.now()}`;
  const ids = [
    ...await makeIdenticalCluster(request, baseURL, apiClient, `${label} A`, 'sample-image-10.png'),
    ...await makeIdenticalCluster(request, baseURL, apiClient, `${label} B`, 'sample-image-11.png'),
  ];
  const created = await request.post(`${baseURL}/v1/reduction`, { data: { name: label, resourceIds: ids } });
  expect(created.ok()).toBeTruthy();
  const id = (await created.json()).id as number;
  const list = await (await request.get(`${baseURL}/v1/reductions`)).json();
  const row = list.reductions.find((r: { id: number }) => r.id === id);
  const compute = await request.post(`${baseURL}/v1/reduction/compute`, { data: { id, version: row.version } });
  expect(compute.ok()).toBeTruthy();
  await expect.poll(async () => {
    const l = await (await request.get(`${baseURL}/v1/reductions`)).json();
    return l.reductions.find((r: { id: number }) => r.id === id)?.status;
  }, { timeout: 60_000 }).toBe('ready');
  return id;
}

// The Cluster the budget cut, addressed by its id so the locator survives the button going away.
async function cutCluster(page: Page): Promise<Locator> {
  const withButton = page.getByTestId('reduction-cluster').filter({ has: page.getByTestId('cluster-more-members') });
  await expect(withButton).toHaveCount(1);
  const clusterId = await withButton.getAttribute('data-cluster-id');
  return page.locator(`[data-testid="reduction-cluster"][data-cluster-id="${clusterId}"]`);
}

test.describe('Resource Reduction Show more members', () => {
  test('a Cluster cut at the page budget shows its kept copy, then every copy, and focus lands on the first one added', async ({ page, request, baseURL, apiClient }) => {
    test.setTimeout(120_000);
    const id = await seedCutReduction(request, baseURL!, apiClient);

    await page.goto(`/reduction?id=${id}`);
    const cut = await cutCluster(page);
    await expect(cut.getByRole('heading', { level: 3 })).toContainText(`${COPIES} copies`);
    await expect(cut.getByTestId('reduction-member')).toHaveCount(1);
    await expect(cut.getByTestId('cluster-shown-count')).toHaveText(`Showing 1 of ${COPIES} copies.`);

    await cut.getByTestId('cluster-more-members').click();

    await expect(cut.getByTestId('reduction-member')).toHaveCount(COPIES);
    await expect(cut.getByTestId('cluster-more-members')).toHaveCount(0);
    await expect(cut.getByTestId('reduction-member').nth(1)).toBeFocused();
    await expect(page.locator('[aria-live="polite"]').filter({ hasText: `Showed ${COPIES - 1} more copies` })).toHaveCount(1);
    const added = cut.getByTestId('reduction-member').nth(1);
    await expect(added.getByTestId('member-role')).toHaveText('Remove');
    await expect(added.getByTestId('member-promote')).toBeVisible();
  });

  test('focus the reviewer moved elsewhere while the copies loaded is left where it is', async ({ page, request, baseURL, apiClient }) => {
    test.setTimeout(120_000);
    const id = await seedCutReduction(request, baseURL!, apiClient);

    await page.goto(`/reduction?id=${id}`);
    const cut = await cutCluster(page);
    let release: () => void = () => {};
    const held = new Promise<void>((resolve) => { release = resolve; });
    await page.route('**/v1/reduction/members**', async (route) => {
      await held;
      await route.continue();
    });

    await cut.getByTestId('cluster-more-members').click();
    // Tab away while the request is out.
    const elsewhere = cut.getByTestId('cluster-skip');
    await elsewhere.focus();
    await expect(elsewhere).toBeFocused();
    release();

    await expect(cut.getByTestId('reduction-member')).toHaveCount(COPIES);
    await expect(elsewhere).toBeFocused();
  });
});
