import path from 'path';
import { test, expect } from '../fixtures/a11y.fixture';
import type { APIRequestContext } from '@playwright/test';

/**
 * Resource Reduction review: the member cap, the apply report's counts and the
 * "being upgraded" refusal (design docs/plans/2026-11-24-reduction-plan-tables-design.md
 * D6, D8, D10, D15).
 *
 * A Cluster stores at most 200 copies: 203 identical files make a Cluster of 200
 * with the other 3 left out, and the card says so. (Paging a Cluster the page cut at
 * its member budget, "Show more members", is in resource-reduction-member-paging.spec.ts.)
 * The apply report speaks from the response's own counts and names a refused
 * Cluster by the winnerName the response carries instead of fetching each Resource.
 */
const MEMBER_CAP = 200;

type ApiClient = {
  createResource: (data: { filePath: string; name: string }) => Promise<{ ID: number; Name: string }>;
};

async function makeBigIdenticalCluster(
  request: APIRequestContext,
  baseURL: string,
  apiClient: ApiClient,
  label: string,
  count: number,
): Promise<number[]> {
  const filePath = path.join(__dirname, '../test-assets/sample-image-10.png');
  const first = await apiClient.createResource({ filePath, name: `${label} 000` });
  const stored = await request.get(`${baseURL}/v1/resource/view?id=${first.ID}`);
  expect(stored.ok()).toBeTruthy();
  const bytes = Buffer.from(await stored.body());

  const ids: number[] = [first.ID];
  const make = async (i: number) => {
    const copy = await apiClient.createResource({ filePath, name: `${label} ${String(i).padStart(3, '0')}` });
    const upload = await request.post(`${baseURL}/v1/resource/versions?resourceId=${copy.ID}`, {
      multipart: {
        file: { name: 'copy.png', mimeType: 'image/png', buffer: bytes },
        comment: 'made identical to the first',
      },
    });
    expect(upload.ok()).toBeTruthy();
    return copy.ID;
  };
  for (let start = 1; start < count; start += 8) {
    const batch = Array.from({ length: Math.min(8, count - start) }, (_, k) => start + k);
    ids.push(...await Promise.all(batch.map(make)));
  }
  return ids;
}

async function createAndCompute(request: APIRequestContext, baseURL: string, name: string, resourceIds: number[]): Promise<number> {
  const created = await request.post(`${baseURL}/v1/reduction`, { data: { name, resourceIds } });
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

async function makePair(request: APIRequestContext, baseURL: string, apiClient: ApiClient, label: string) {
  const ids = await makeBigIdenticalCluster(request, baseURL, apiClient, label, 2);
  return ids;
}

test.describe('Resource Reduction member cap and apply report', () => {
  test('203 identical files make a Cluster of 200 copies and the card says 3 were left out', async ({ page, apiClient, request, baseURL, checkA11y }) => {
    test.setTimeout(240_000);
    const label = `RR cap ${Date.now()}`;
    const ids = await makeBigIdenticalCluster(request, baseURL!, apiClient, label, MEMBER_CAP + 3);
    const id = await createAndCompute(request, baseURL!, label, ids);

    await page.goto(`/reduction?id=${id}`);
    const card = page.getByTestId('reduction-cluster').first();
    await expect(card.getByRole('heading', { level: 3 })).toContainText(`${MEMBER_CAP} copies`);
    await expect(card.getByTestId('reduction-member')).toHaveCount(MEMBER_CAP);
    await expect(card.getByTestId('cluster-truncated')).toHaveText(
      `Showing ${MEMBER_CAP} copies; 3 more were left out. Apply this Cluster, then compute again for the rest.`,
    );
    // Within the page's budget, so every stored copy is on the card and there is nothing to page.
    await expect(card.getByTestId('cluster-more-members')).toHaveCount(0);
    await checkA11y();
  });

  test('the apply report uses the response counts, names refused Clusters by winnerName and points to the tabs when the lists are capped', async ({ page, apiClient, request, baseURL }) => {
    const label = `RR report ${Date.now()}`;
    const ids = await makePair(request, baseURL!, apiClient, label);
    const id = await createAndCompute(request, baseURL!, label, ids);

    const resourceFetches: string[] = [];
    page.on('request', (req) => {
      if (/\/v1\/resource\?id=/.test(req.url())) resourceFetches.push(req.url());
    });
    await page.route('**/v1/reduction/apply', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          applied: [{ clusterId: 'a1', winnerId: 11, winnerName: 'kept-a.png', loserIds: [12] }],
          stale: [{ clusterId: 's1', winnerId: 21, winnerName: 'refused-winner.png', reason: 'left the selection' }],
          changed: 0,
          errors: 0,
          appliedCount: 1200,
          staleCount: 40,
          destroyed: 2400,
          more: true,
        }),
      });
    });

    await page.goto(`/reduction?id=${id}`);
    await page.getByTestId('reduction-apply').click();
    await page.getByRole('alertdialog').getByRole('button', { name: /^Apply \d+ Clusters?$/ }).click();

    const result = page.getByTestId('reduction-apply-result');
    await expect(result).toContainText('Applied 1200 Clusters, removed 2400 copies.');
    await expect(result).toContainText('40 Clusters could not be applied');
    await expect(result).toContainText('Cluster keeping refused-winner.png');
    await expect(result).toContainText('left the selection');
    await expect(result.getByTestId('reduction-apply-more')).toContainText('Applied and Stale tabs');
    expect(resourceFetches, 'the report must not fetch each winner').toEqual([]);
  });

  test('a refresh the server refuses as "being upgraded" says so instead of a bare status', async ({ page, apiClient, request, baseURL }) => {
    const label = `RR upgrading ${Date.now()}`;
    const ids = await makePair(request, baseURL!, apiClient, label);
    const id = await createAndCompute(request, baseURL!, label, ids);

    await page.goto(`/reduction?id=${id}`);
    await page.route(/\/reduction\?id=\d+/, async (route) => {
      if (route.request().resourceType() === 'fetch') {
        await route.fulfill({ status: 503, contentType: 'text/html', body: '<p>being upgraded</p>' });
        return;
      }
      await route.continue();
    });
    await page.getByTestId('cluster-skip').first().click();
    await expect(page.locator('[data-reduction-error]:visible').first()).toContainText('This Reduction is being upgraded, try again shortly');
  });
});
