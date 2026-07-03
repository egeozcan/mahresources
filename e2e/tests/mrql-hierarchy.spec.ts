import { test, expect } from '../fixtures/base.fixture';
import { getWorkerBaseUrl } from '../fixtures/base.fixture';
import { ApiClient } from '../helpers/api-client';
import { MRQLPage } from '../pages/MRQLPage';
import { request as playwrightRequest } from '@playwright/test';

// E2E coverage for MRQL Package 2 (hierarchy traversal): ancestors. / descendants.
// recursive roots resolving through the group ownership tree.
test.describe('MRQL hierarchy traversal', () => {
  const suffix = `pkg2-${Date.now()}`;
  const gpName = `Hier GP ${suffix}`;
  const parentName = `Hier Parent ${suffix}`;
  const childName = `Hier Child ${suffix}`;
  let categoryId: number;
  let gpId: number;
  let parentId: number;
  let childId: number;

  test.beforeAll(async () => {
    const baseUrl = getWorkerBaseUrl();
    const ctx = await playwrightRequest.newContext({ baseURL: baseUrl });
    const api = new ApiClient(ctx, baseUrl);

    const category = await api.createCategory(`Hier Category ${suffix}`);
    categoryId = category.ID;

    // Grandparent(root) → Parent → Child
    const gp = await api.createGroup({ name: gpName, description: 'root', categoryId });
    gpId = gp.ID;
    const parent = await api.createGroup({ name: parentName, description: 'mid', categoryId, ownerId: gpId });
    parentId = parent.ID;
    const child = await api.createGroup({ name: childName, description: 'leaf', categoryId, ownerId: parentId });
    childId = child.ID;

    await ctx.dispose();
  });

  test.afterAll(async () => {
    const baseUrl = getWorkerBaseUrl();
    const ctx = await playwrightRequest.newContext({ baseURL: baseUrl });
    const api = new ApiClient(ctx, baseUrl);
    for (const id of [childId, parentId, gpId]) {
      try { if (id) await api.deleteGroup(id); } catch { /* ignore */ }
    }
    try { if (categoryId) await api.deleteCategory(categoryId); } catch { /* ignore */ }
    await ctx.dispose();
  });

  test('ancestors matches every group below the named root', async ({ page }) => {
    const mrql = new MRQLPage(page);
    await mrql.navigate();

    // Strict descendants of the grandparent = {Parent, Child}. The grandparent
    // itself is not its own ancestor, so it must not appear.
    await mrql.enterQuery(`type = group AND ancestors.name = "${gpName}"`);
    await mrql.executeQuery();

    const results = await mrql.getResults();
    const texts = (await results.allTextContents()).join('\n');
    expect(texts).toContain(parentName);
    expect(texts).toContain(childName);
    expect(texts).not.toContain(gpName);
  });

  test('descendants matches every ancestor of the named leaf', async ({ page }) => {
    const mrql = new MRQLPage(page);
    await mrql.navigate();

    // Strict ancestors of the child = {Grandparent, Parent}. The child itself
    // is not its own descendant, so it must not appear.
    await mrql.enterQuery(`type = group AND descendants.name = "${childName}"`);
    await mrql.executeQuery();

    const results = await mrql.getResults();
    const texts = (await results.allTextContents()).join('\n');
    expect(texts).toContain(gpName);
    expect(texts).toContain(parentName);
    expect(texts).not.toContain(childName);
  });
});
