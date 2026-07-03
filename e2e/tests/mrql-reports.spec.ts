import { test, expect } from '../fixtures/base.fixture';
import { getWorkerBaseUrl } from '../fixtures/base.fixture';
import { ApiClient } from '../helpers/api-client';
import { MRQLPage } from '../pages/MRQLPage';
import { expectComponentNoViolations } from '../helpers/accessibility/axe-helper';
import { request as playwrightRequest } from '@playwright/test';
import * as path from 'path';

// E2E coverage for MRQL Package 4 (Saved Queries as Reports):
// parameter inputs, the EXPLAIN panel, and CSV/JSON export.
test.describe('MRQL reports: params, explain, export', () => {
  let resourceId: number;
  let groupId: number;
  let categoryId: number;
  const needle = `pkg4needle${Date.now()}`;

  test.beforeAll(async () => {
    const baseUrl = getWorkerBaseUrl();
    const ctx = await playwrightRequest.newContext({ baseURL: baseUrl });
    const api = new ApiClient(ctx, baseUrl);
    const suffix = `pkg4-${Date.now()}`;

    const category = await api.createCategory(`Pkg4 Category ${suffix}`);
    categoryId = category.ID;

    const group = await api.createGroup({ name: `Pkg4 Group ${suffix}`, categoryId });
    groupId = group.ID;

    const txtPath = path.join(__dirname, '../test-assets/sample-document.txt');
    const r = await api.createResource({
      filePath: txtPath,
      name: `Report Resource ${needle}`,
      ownerId: groupId,
    });
    resourceId = r.ID;

    await ctx.dispose();
  });

  test.afterAll(async () => {
    const baseUrl = getWorkerBaseUrl();
    const ctx = await playwrightRequest.newContext({ baseURL: baseUrl });
    const api = new ApiClient(ctx, baseUrl);
    try { if (resourceId) await api.deleteResource(resourceId); } catch { /* ignore */ }
    try { if (groupId) await api.deleteGroup(groupId); } catch { /* ignore */ }
    try { if (categoryId) await api.deleteCategory(categoryId); } catch { /* ignore */ }
    await ctx.dispose();
  });

  test('parameter inputs appear and bind on run', async ({ page }) => {
    const mrql = new MRQLPage(page);
    await mrql.navigate();

    await mrql.enterQuery(`type = "resource" AND name ~ $needle`);
    // Validation is debounced; the params fieldset appears once it resolves.
    await expect(mrql.paramsFieldset).toBeVisible({ timeout: 5000 });
    await expect(mrql.paramsFieldset.locator('input[data-param="needle"]')).toBeVisible();

    await mrql.fillParam('needle', needle);
    await mrql.executeQuery();

    await expect(mrql.resultsSection).toContainText('Report Resource');
  });

  test('running with an empty param surfaces a missing-param error', async ({ page }) => {
    const mrql = new MRQLPage(page);
    await mrql.navigate();

    await mrql.enterQuery(`type = "resource" AND name ~ $needle`);
    await expect(mrql.paramsFieldset).toBeVisible({ timeout: 5000 });

    // Leave the param empty and run.
    await mrql.executeQuery();
    await expect(mrql.resultsSection.locator('[role="alert"]')).toContainText('missing parameter $needle');
  });

  test('explain panel shows the SQL', async ({ page }) => {
    const mrql = new MRQLPage(page);
    await mrql.navigate();

    await mrql.enterQuery(`type = "resource" AND name ~ "${needle}"`);
    await mrql.clickExplain();

    await expect(mrql.explainPanel).toContainText('resources');
    // The interpolated SQL should reflect the search term.
    await expect(mrql.explainPanel.locator('pre')).toContainText(needle);
  });

  test('CSV export downloads a file with the expected header row', async ({ page }) => {
    const mrql = new MRQLPage(page);
    await mrql.navigate();

    await mrql.enterQuery(`type = "resource" AND name ~ "${needle}"`);
    await mrql.executeQuery();

    const downloadPromise = page.waitForEvent('download');
    await mrql.exportCsvButton.click();
    const download = await downloadPromise;

    expect(download.suggestedFilename()).toMatch(/\.csv$/);
    const stream = await download.createReadStream();
    const chunks: Buffer[] = [];
    for await (const chunk of stream) chunks.push(Buffer.from(chunk));
    const text = Buffer.concat(chunks).toString('utf-8');
    const firstLine = text.split('\n')[0];
    expect(firstLine).toContain('id,name,description,content_type');
  });

  test('parameter controls are accessible', async ({ page }) => {
    const mrql = new MRQLPage(page);
    await mrql.navigate();

    await mrql.enterQuery(`type = "resource" AND name ~ $needle`);
    await expect(mrql.paramsFieldset).toBeVisible({ timeout: 5000 });

    // The param input must have an associated accessible name (its <label>).
    await expect(mrql.paramsFieldset.locator('input[data-param="needle"]')).toHaveAccessibleName(/needle/);

    // No axe violations in the params fieldset.
    await expectComponentNoViolations(page, '[data-testid="mrql-params"]');
  });
});
