import path from 'path';
import { test, expect } from '../../fixtures/base.fixture';
import type { APIRequestContext, Page } from '@playwright/test';

/**
 * Resource Reduction review with a Resource named by a ~600 character URL with no
 * spaces (what an Instagram import produces). The name must never push a card, the
 * header or the page wider than its container, the justification must stay compact,
 * and a thumbnail that fails to load must fall back to a type placeholder.
 */
const LONG_NAME = 'https://scontent-fra3-1.cdninstagram.com/v/t51.82787-15/616485245_18088116590034356_8888896898880302030_n.jpg'
  + '?stp=dst-jpg_e35_tt6&_nc_cat=110&_nc_map=urlgen_bucketless&ig_cache_key=MjM1MzYwOTczOTE1MTQwODcxNg%3D%3D.3-ccb7-5&ccb=7-5'
  + '&_nc_sid=58cdad&efg=eyJ2ZW5jb2RlX3RhZyI6IkZFRUQueHBpZHMuOTM3LnNkci5yZWd1bGFyX3Bob3RvLkMzIn0%3D&_nc_ohc=DkR6r3Qfe3QQ7kNvwEq7ni'
  + '&_nc_oc=AdpW556nDSIyOMAo4HJcssJ-IqXyJwZc2WKyHj65hhw-GIZZCZnBOrwCiAJsk2jebrYpLLqyvIhQhyuGdcYhEMBT&_nc_zt=23'
  + '&_nc_ht=scontent-fra3-1.cdninstagram.com&_nc_gid=m5kV1iGtsSL47NKcwPoh8A&_nc_ss=7b7a8&_nc_ct=RQosPBUIFQIlBAAAAA'
  + '&oh=00_AQRArOT5TYOiVXXUpvSCqYu532daFv7f7NNpQDjHCqOEHA&oe=6B0AD2D';

async function makeLongNamedPair(
  request: APIRequestContext,
  baseURL: string,
  apiClient: { createResource: (data: { filePath: string; name: string }) => Promise<{ ID: number; Name: string }> },
  suffix: string,
) {
  const filePath = path.join(__dirname, '../../test-assets/sample-image-10.png');
  const keeper = await apiClient.createResource({ filePath, name: `${LONG_NAME}&k=${suffix}` });
  const twin = await apiClient.createResource({ filePath, name: `${LONG_NAME}&t=${suffix}` });
  const stored = await request.get(`${baseURL}/v1/resource/view?id=${keeper.ID}`);
  expect(stored.ok()).toBeTruthy();
  const upload = await request.post(`${baseURL}/v1/resource/versions?resourceId=${twin.ID}`, {
    multipart: {
      file: { name: 'twin.png', mimeType: 'image/png', buffer: Buffer.from(await stored.body()) },
      comment: 'made identical to the keeper',
    },
  });
  expect(upload.ok()).toBeTruthy();
  const created = await request.post(`${baseURL}/v1/reduction`, {
    data: { name: `long names ${suffix}`, resourceIds: [keeper.ID, twin.ID] },
  });
  expect(created.ok()).toBeTruthy();
  const id = (await created.json()).id as number;
  const list = await (await request.get(`${baseURL}/v1/reductions`)).json();
  const row = list.reductions.find((r: { id: number }) => r.id === id);
  const compute = await request.post(`${baseURL}/v1/reduction/compute`, { data: { id, version: row.version } });
  expect(compute.ok()).toBeTruthy();
  await expect.poll(async () => {
    const l = await (await request.get(`${baseURL}/v1/reductions`)).json();
    return l.reductions.find((r: { id: number }) => r.id === id)?.status;
  }, { timeout: 30_000 }).toBe('ready');
  return { id, keeper, twin };
}

async function expectContained(page: Page) {
  const cards = page.getByTestId('reduction-member');
  await expect(cards).toHaveCount(2);
  for (let i = 0; i < 2; i++) {
    const card = cards.nth(i);
    const overflow = await card.evaluate(el => el.scrollWidth - el.clientWidth);
    expect(overflow, `card ${i} scrollWidth - clientWidth`).toBeLessThanOrEqual(0);
    const box = (await card.boundingBox())!;
    const name = (await card.locator('.reduction-member-name').boundingBox())!;
    expect(name.x).toBeGreaterThanOrEqual(box.x - 0.5);
    expect(name.x + name.width).toBeLessThanOrEqual(box.x + box.width + 0.5);
  }
  const doc = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
  expect(doc, 'page horizontal overflow').toBeLessThanOrEqual(0);
}

test.describe('Reduction review with very long Resource names', () => {
  for (const width of [2000, 1440, 390, 320]) {
    test(`names stay inside their containers at ${width}px`, async ({ page, apiClient, request, baseURL }) => {
      const { id } = await makeLongNamedPair(request, baseURL!, apiClient, `${Date.now()}-${width}`);
      await page.setViewportSize({ width, height: 900 });
      await page.goto(`/reduction?id=${id}`);
      await expect(page.getByTestId('reduction-cluster')).toBeVisible();
      await expectContained(page);

      const head = await page.locator('.reduction-cluster-head').first().boundingBox();
      expect(head!.height, 'cluster header height').toBeLessThan(width <= 390 ? 260 : 160);

      // The full name stays available on hover and to assistive tech.
      await expect(page.getByTestId('cluster-decided-by')).toHaveAttribute('title', /cdninstagram.*oe=6B0AD2D/);
      await expect(page.locator('.reduction-member-name a').first()).toHaveAttribute('title', /cdninstagram.*oe=6B0AD2D/);
    });
  }

  test('a thumbnail that fails to load shows a type placeholder', async ({ page, apiClient, request, baseURL }) => {
    const { id, keeper } = await makeLongNamedPair(request, baseURL!, apiClient, `${Date.now()}-ph`);
    await page.route(new RegExp(`/v1/resource/preview\\?id=${keeper.ID}&`), route => route.abort());
    await page.goto(`/reduction?id=${id}`);
    const card = page.locator(`[data-testid="reduction-member"][data-resource-id="${keeper.ID}"]`);
    const placeholder = card.getByTestId('member-thumb-placeholder');
    await expect(placeholder).toBeVisible();
    await expect(placeholder).toContainText('image/png');
    await expect(card.locator('img')).toBeHidden();
    // The other card keeps its thumbnail and shows no placeholder.
    const other = page.locator(`[data-testid="reduction-member"]:not([data-resource-id="${keeper.ID}"])`);
    await expect(other.locator('img')).toBeVisible();
    await expect(other.getByTestId('member-thumb-placeholder')).toBeHidden();
  });
});
