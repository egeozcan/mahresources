import { test, expect } from '../fixtures/base.fixture';
import { getWorkerBaseUrl } from '../fixtures/base.fixture';
import { ApiClient } from '../helpers/api-client';
import { MRQLPage } from '../pages/MRQLPage';
import { request as playwrightRequest } from '@playwright/test';
import * as path from 'path';

// E2E coverage for MRQL Package 1 (counting and aggregation):
// HAVING on aggregated GROUP BY and date-bucket grouping (created.month).
test.describe('MRQL counting and aggregation', () => {
  let categoryId: number;
  let groupId: number;
  const resourceIds: number[] = [];

  test.beforeAll(async () => {
    const baseUrl = getWorkerBaseUrl();
    const ctx = await playwrightRequest.newContext({ baseURL: baseUrl });
    const api = new ApiClient(ctx, baseUrl);
    const suffix = `pkg1-${Date.now()}`;

    const category = await api.createCategory(`Pkg1 Test Category ${suffix}`);
    categoryId = category.ID;

    const group = await api.createGroup({
      name: `Pkg1 Test Group ${suffix}`,
      description: 'Group for counting/aggregation E2E tests',
      categoryId: categoryId,
    });
    groupId = group.ID;

    // Two PNG images (same contentType) and one text file, so
    // HAVING COUNT() >= 2 keeps the image/png bucket.
    const imgPath = path.join(__dirname, '../test-assets/sample-image.png');
    const imgPath2 = path.join(__dirname, '../test-assets/sample-image-2.png');
    const txtPath = path.join(__dirname, '../test-assets/sample-document.txt');

    for (const [filePath, name] of [
      [imgPath, `Pkg1 Image 1 ${suffix}`],
      [imgPath2, `Pkg1 Image 2 ${suffix}`],
      [txtPath, `Pkg1 Doc ${suffix}`],
    ] as const) {
      const r = await api.createResource({ filePath, name, ownerId: groupId });
      resourceIds.push(r.ID);
    }

    await ctx.dispose();
  });

  test.afterAll(async () => {
    const baseUrl = getWorkerBaseUrl();
    const ctx = await playwrightRequest.newContext({ baseURL: baseUrl });
    const api = new ApiClient(ctx, baseUrl);

    for (const id of resourceIds) {
      try { await api.deleteResource(id); } catch { /* ignore */ }
    }
    try { if (groupId) await api.deleteGroup(groupId); } catch { /* ignore */ }
    try { if (categoryId) await api.deleteCategory(categoryId); } catch { /* ignore */ }

    await ctx.dispose();
  });

  test('HAVING filters aggregated buckets in the rendered table', async ({ page }) => {
    const mrql = new MRQLPage(page);
    await mrql.navigate();

    await mrql.enterQuery('type = resource GROUP BY contentType COUNT() HAVING COUNT() >= 2 ORDER BY count DESC');
    await mrql.executeQuery();

    const heading = mrql.resultsSection.locator('h2');
    await expect(heading).toContainText('rows');

    const table = mrql.resultsSection.locator('table');
    await expect(table).toBeVisible();

    // Headers include the group key and count
    const headers = table.locator('thead th');
    const headerCount = await headers.count();
    const headerTexts: string[] = [];
    for (let i = 0; i < headerCount; i++) {
      const text = await headers.nth(i).textContent();
      if (text) headerTexts.push(text.trim().toLowerCase());
    }
    expect(headerTexts).toContain('contenttype');
    expect(headerTexts).toContain('count');
    const countCol = headerTexts.indexOf('count');

    // We seeded two image/png resources, so at least one bucket survives,
    // and every surviving bucket must have count >= 2 (HAVING applied).
    const dataRows = table.locator('tbody tr');
    const rowCount = await dataRows.count();
    expect(rowCount).toBeGreaterThan(0);
    for (let i = 0; i < rowCount; i++) {
      const countText = await dataRows.nth(i).locator('td').nth(countCol).textContent();
      expect(Number(countText?.trim())).toBeGreaterThanOrEqual(2);
    }
  });

  test('created.month aggregation renders month-labeled rows', async ({ page }) => {
    const mrql = new MRQLPage(page);
    await mrql.navigate();

    await mrql.enterQuery('type = resource GROUP BY created.month COUNT() ORDER BY created.month ASC');
    await mrql.executeQuery();

    const table = mrql.resultsSection.locator('table');
    await expect(table).toBeVisible();

    const headers = table.locator('thead th');
    const headerCount = await headers.count();
    const headerTexts: string[] = [];
    for (let i = 0; i < headerCount; i++) {
      const text = await headers.nth(i).textContent();
      if (text) headerTexts.push(text.trim().toLowerCase());
    }
    expect(headerTexts).toContain('created.month');
    const monthCol = headerTexts.indexOf('created.month');

    // Seeded resources were created just now, so a current-month bucket exists.
    const dataRows = table.locator('tbody tr');
    const rowCount = await dataRows.count();
    expect(rowCount).toBeGreaterThan(0);
    for (let i = 0; i < rowCount; i++) {
      const label = await dataRows.nth(i).locator('td').nth(monthCol).textContent();
      expect(label?.trim()).toMatch(/^\d{4}-\d{2}$/);
    }
  });

  test('relation count query validates and runs', async ({ page }) => {
    const mrql = new MRQLPage(page);
    await mrql.navigate();

    await mrql.enterQuery('type = resource AND tags.count = 0 ORDER BY tags.count ASC LIMIT 5');
    await mrql.executeQuery();

    // Query must execute without an error alert; our seeded resources are
    // untagged so results are expected.
    const count = await mrql.getResultCount();
    expect(count).toBeGreaterThan(0);
  });
});
