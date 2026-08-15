import { test, expect, getWorkerBaseUrl } from '../../fixtures/base.fixture';
import { request as pwRequest } from '@playwright/test';
import { ApiClient } from '../../helpers/api-client';
import path from 'path';

/**
 * A sync plugin action can refuse, and the modal has to say so.
 *
 * `ActionResult.success` was always produced by the server and always ignored
 * by the modal: any 200 carrying neither a job id nor a redirect was assigned to
 * the result, announced in a `role="status"` box reading "Action completed
 * successfully", and followed by a page reload 1.5s later — for a handler that
 * returned `success=false`, for `mah.abort` on the sync path, and for a bulk run
 * in which every entity failed.
 */

async function ensurePluginEnabled(baseURL: string, name: string) {
  const ctx = await pwRequest.newContext({ baseURL });
  const client = new ApiClient(ctx, baseURL);
  try {
    await client.enablePlugin(name);
  } catch {
    // Already enabled — ignore
  }
  await ctx.dispose();
}

test.describe('Plugin action refusals', () => {
  let resourceId: number;

  test.beforeAll(async () => {
    const baseURL = getWorkerBaseUrl();
    await ensurePluginEnabled(baseURL, 'test-actions');

    const ctx = await pwRequest.newContext({ baseURL });
    const client = new ApiClient(ctx, baseURL);
    const runId = Date.now() + Math.floor(Math.random() * 100000);
    const category = await client.createCategory(`Refusal Category ${runId}`);
    const group = await client.createGroup({
      name: `Refusal Group ${runId}`,
      categoryId: category.ID,
    });
    const resource = await client.createResource({
      filePath: path.join(__dirname, '../../test-assets/sample-image-34.png'),
      name: `Refusal Resource ${runId}`,
      ownerId: group.ID,
    });
    resourceId = resource.ID;
    await ctx.dispose();
  });

  test('the server reports success=false for a refusing handler', async ({ apiClient }) => {
    const response = await apiClient.request.post('/v1/jobs/action/run', {
      data: {
        plugin: 'test-actions',
        action: 'always-refuses',
        entity_ids: [resourceId],
        params: {},
      },
    });
    expect(response.ok()).toBeTruthy();
    const body = await response.json();
    expect(body.success).toBe(false);
    expect(body.message).toContain('quota exhausted');
  });

  test('a refusal renders as an alert, not as a success', async ({ page }) => {
    await page.goto(`/resource?id=${resourceId}`);
    await page.waitForLoadState('load');

    await page.getByRole('button', { name: 'Always Refuses' }).click();

    const modal = page.getByRole('dialog');
    await expect(modal).toBeVisible();
    await modal.getByRole('button', { name: 'Run' }).click();

    // role="alert", so a screen reader interrupts with it.
    const alert = modal.locator('[role="alert"]');
    await expect(alert).toBeVisible({ timeout: 5000 });
    await expect(alert).toContainText('quota exhausted');

    // And NOT the success box.
    await expect(modal.locator('[role="status"]')).toHaveCount(0);
  });

  test('a refusal does not reload the page or close the modal', async ({ page }) => {
    await page.goto(`/resource?id=${resourceId}`);
    await page.waitForLoadState('load');

    // A marker that a reload would destroy.
    await page.evaluate(() => {
      (window as any).__refusalMarker = 'still here';
    });

    await page.getByRole('button', { name: 'Always Refuses' }).click();
    const modal = page.getByRole('dialog');
    await modal.getByRole('button', { name: 'Run' }).click();
    await expect(modal.locator('[role="alert"]')).toBeVisible({ timeout: 5000 });

    // The success path closes and reloads after 1.5s; wait past that.
    await page.waitForTimeout(2500);

    const marker = await page.evaluate(() => (window as any).__refusalMarker);
    expect(marker).toBe('still here');
    // The reason has to stay readable.
    await expect(modal.locator('[role="alert"]')).toBeVisible();
  });

  test('a bulk run reports which entities failed', async ({ apiClient }) => {
    const response = await apiClient.request.post('/v1/jobs/action/run', {
      data: {
        plugin: 'test-actions',
        action: 'refuses-even',
        // One even and one odd id, so the run is partly successful whichever
        // ids the fixtures happened to get.
        entity_ids: [resourceId, resourceId + 1],
        params: {},
      },
    });
    expect(response.ok()).toBeTruthy();
    const body = await response.json();
    expect(Array.isArray(body.results)).toBeTruthy();
    expect(body.results).toHaveLength(2);

    const failed = body.results.filter((r: any) => r.success === false);
    const succeeded = body.results.filter((r: any) => r.success === true);
    expect(failed).toHaveLength(1);
    expect(succeeded).toHaveLength(1);
    expect(failed[0].message).toContain('Refused even resource');
  });

  test('required combines with show_when: mandatory only in its own branch', async ({ apiClient }) => {
    // The simple branch does not show publish_at, so it is not required there.
    const simple = await apiClient.request.post('/v1/jobs/action/run', {
      data: {
        plugin: 'test-actions',
        action: 'branch-required',
        entity_ids: [resourceId],
        params: { mode: 'simple' },
      },
    });
    expect(simple.ok()).toBeTruthy();
    expect((await simple.json()).message).toBe('published now');

    // The scheduled branch shows it, so omitting it is a validation error.
    const missing = await apiClient.request.post('/v1/jobs/action/run', {
      data: {
        plugin: 'test-actions',
        action: 'branch-required',
        entity_ids: [resourceId],
        params: { mode: 'scheduled' },
      },
    });
    expect(missing.status()).toBe(400);
    const errors = (await missing.json()).errors;
    expect(errors.some((e: any) => e.field === 'publish_at')).toBeTruthy();

    // Supplied, it goes through.
    const supplied = await apiClient.request.post('/v1/jobs/action/run', {
      data: {
        plugin: 'test-actions',
        action: 'branch-required',
        entity_ids: [resourceId],
        params: { mode: 'scheduled', publish_at: '2030-01-01' },
      },
    });
    expect(supplied.ok()).toBeTruthy();
    expect((await supplied.json()).message).toBe('scheduled for 2030-01-01');
  });

  test('a value for a hidden param is dropped before the handler sees it', async ({ apiClient }) => {
    // A direct API caller can submit what the modal would have stripped. The
    // handler must not receive it.
    const response = await apiClient.request.post('/v1/jobs/action/run', {
      data: {
        plugin: 'test-actions',
        action: 'branch-required',
        entity_ids: [resourceId],
        params: { mode: 'simple', publish_at: 'smuggled' },
      },
    });
    expect(response.ok()).toBeTruthy();
    expect((await response.json()).message).toBe('published now');
  });
});
