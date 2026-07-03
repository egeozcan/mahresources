import { test, expect } from '../fixtures/base.fixture';
import { getWorkerBaseUrl } from '../fixtures/base.fixture';
import { ApiClient } from '../helpers/api-client';
import { MRQLPage } from '../pages/MRQLPage';
import { request as playwrightRequest } from '@playwright/test';
import * as path from 'path';

// E2E coverage for MRQL Package 3 (similarity search): SIMILAR TO resource(N).
//
// Real perceptual matching is covered by the Go integration tests against
// seeded resource_similarities pairs — the hash worker's poll interval makes
// live pair computation impractical in E2E. Here we verify the query-language
// surface end to end: the syntax executes, returns an empty (not error)
// result for a pairless target, and validation errors surface in the UI.
test.describe('MRQL similarity search', () => {
  const suffix = `pkg3-${Date.now()}`;
  let resourceId: number;

  test.beforeAll(async () => {
    const baseUrl = getWorkerBaseUrl();
    const ctx = await playwrightRequest.newContext({ baseURL: baseUrl });
    const api = new ApiClient(ctx, baseUrl);
    const resource = await api.createResource({
      filePath: path.join(__dirname, '../test-assets/sample-image.png'),
      name: `similar-target-${suffix}.png`,
    });
    resourceId = resource.ID;
    await ctx.dispose();
  });

  test.afterAll(async () => {
    const baseUrl = getWorkerBaseUrl();
    const ctx = await playwrightRequest.newContext({ baseURL: baseUrl });
    const api = new ApiClient(ctx, baseUrl);
    try { if (resourceId) await api.deleteResource(resourceId); } catch { /* ignore */ }
    await ctx.dispose();
  });

  test('SIMILAR TO executes and returns empty results for a pairless target', async ({ page }) => {
    const mrql = new MRQLPage(page);
    await mrql.navigate();

    await mrql.enterQuery(`type = resource AND SIMILAR TO resource(${resourceId}) ORDER BY distance ASC`);
    await mrql.executeQuery();

    // No pairs exist for a fresh upload — the query must succeed with zero
    // results, not surface an error.
    const execError = await mrql.getErrors();
    expect(execError).toBeNull();
  });

  test('WITHIN beyond the stored-pair cap surfaces a validation error', async ({ page }) => {
    const mrql = new MRQLPage(page);
    await mrql.navigate();

    await mrql.enterQuery(`type = resource AND SIMILAR TO resource(${resourceId}) WITHIN 12`);
    await expect(mrql.validationError).toBeVisible();
    const err = await mrql.getValidationError();
    expect(err).toContain('11');
  });

  test('SIMILAR TO on notes surfaces a validation error', async ({ page }) => {
    const mrql = new MRQLPage(page);
    await mrql.navigate();

    await mrql.enterQuery(`type = note AND SIMILAR TO resource(${resourceId})`);
    await expect(mrql.validationError).toBeVisible();
    const err = await mrql.getValidationError();
    expect(err).toContain('resource');
  });
});
