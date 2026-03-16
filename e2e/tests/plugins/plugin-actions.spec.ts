import { test, expect, getWorkerBaseUrl } from '../../fixtures/base.fixture';
import { request as pwRequest } from '@playwright/test';
import { ApiClient } from '../../helpers/api-client';
import path from 'path';

// Helper to ensure plugin is enabled (tolerates already-enabled state)
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

// Helper to create test entities via API
async function createTestEntities(baseURL: string) {
  const ctx = await pwRequest.newContext({ baseURL });
  const client = new ApiClient(ctx, baseURL);
  const runId = Date.now() + Math.floor(Math.random() * 100000);

  const category = await client.createCategory(`Action Test Category ${runId}`);
  const group = await client.createGroup({
    name: `Action Test Group ${runId}`,
    categoryId: category.ID,
  });
  const resource = await client.createResource({
    filePath: path.join(__dirname, '../../test-assets/sample-image-34.png'),
    name: `Action Resource ${runId}`,
    ownerId: group.ID,
  });

  await ctx.dispose();
  return { resourceId: resource.ID, groupId: group.ID };
}

// ── API Tests (no shared state, use hardcoded/throwaway entity IDs) ──

test.describe('Plugin Actions API', () => {
  test.beforeAll(async () => {
    const baseURL = getWorkerBaseUrl();
    await ensurePluginEnabled(baseURL, 'test-actions');
  });

  test('list actions for resource entity type', async ({ apiClient }) => {
    const response = await apiClient.request.get('/v1/plugin/actions?entity=resource');
    expect(response.ok()).toBeTruthy();
    const data = await response.json();
    expect(Array.isArray(data)).toBeTruthy();
    expect(data.length).toBeGreaterThanOrEqual(2);

    const greetAction = data.find((a: any) => a.id === 'sync-greet');
    expect(greetAction).toBeDefined();
    expect(greetAction.label).toBe('Greet Resource');
    expect(greetAction.entity).toBe('resource');
    expect(greetAction.params).toHaveLength(1);
    expect(greetAction.params[0].name).toBe('greeting');
    expect(greetAction.params[0].type).toBe('text');
    expect(greetAction.params[0].required).toBe(true);

    const asyncAction = data.find((a: any) => a.id === 'async-demo');
    expect(asyncAction).toBeDefined();
    expect(asyncAction.label).toBe('Async Demo');
    expect(asyncAction.async).toBe(true);
  });

  test('list actions for group entity type', async ({ apiClient }) => {
    const response = await apiClient.request.get('/v1/plugin/actions?entity=group');
    expect(response.ok()).toBeTruthy();
    const data = await response.json();
    expect(Array.isArray(data)).toBeTruthy();

    const groupAction = data.find((a: any) => a.id === 'group-action');
    expect(groupAction).toBeDefined();
    expect(groupAction.label).toBe('Group Action');
    expect(groupAction.entity).toBe('group');
    expect(groupAction.placement).toContain('detail');
    expect(groupAction.placement).toContain('bulk');
  });

  test('resource actions do not appear in group entity listing', async ({ apiClient }) => {
    const response = await apiClient.request.get('/v1/plugin/actions?entity=group');
    expect(response.ok()).toBeTruthy();
    const data = await response.json();

    const greetAction = data.find((a: any) => a.id === 'sync-greet');
    expect(greetAction).toBeUndefined();
  });

  test('run sync action with valid params', async ({ apiClient }) => {
    const response = await apiClient.request.post('/v1/jobs/action/run', {
      headers: { 'Content-Type': 'application/json' },
      data: JSON.stringify({
        plugin: 'test-actions',
        action: 'sync-greet',
        entity_ids: [99999],
        params: { greeting: 'Hi there' },
      }),
    });
    expect(response.ok()).toBeTruthy();
    const data = await response.json();
    expect(data.success).toBeTruthy();
    expect(data.message).toContain('Hi there');
    expect(data.message).toContain('99999');
  });

  test('run sync action with missing required param returns error', async ({ apiClient }) => {
    const response = await apiClient.request.post('/v1/jobs/action/run', {
      headers: { 'Content-Type': 'application/json' },
      data: JSON.stringify({
        plugin: 'test-actions',
        action: 'sync-greet',
        entity_ids: [99999],
        params: {},
      }),
    });
    expect(response.ok()).toBeFalsy();
    expect(response.status()).toBe(400);
    const data = await response.json();
    expect(data.errors).toBeDefined();
    expect(data.errors.length).toBeGreaterThan(0);
    expect(data.errors[0].field).toBe('greeting');
  });

  test('run async action returns job ID and completes', async ({ apiClient }) => {
    const response = await apiClient.request.post('/v1/jobs/action/run', {
      headers: { 'Content-Type': 'application/json' },
      data: JSON.stringify({
        plugin: 'test-actions',
        action: 'async-demo',
        entity_ids: [99999],
        params: { steps: 3 },
      }),
    });
    expect(response.status()).toBe(202);
    const data = await response.json();
    expect(data.job_id).toBeDefined();
    expect(typeof data.job_id).toBe('string');

    const jobId = data.job_id;
    let job: any;
    for (let i = 0; i < 20; i++) {
      const jobResp = await apiClient.request.get(`/v1/jobs/action/job?id=${jobId}`);
      expect(jobResp.ok()).toBeTruthy();
      job = await jobResp.json();
      if (job.status === 'completed' || job.status === 'failed') break;
      await new Promise(r => setTimeout(r, 200));
    }
    expect(job.status).toBe('completed');
    expect(job.progress).toBe(100);
    expect(job.result).toBeDefined();
    expect(job.result.message).toBe('Done!');
  });

  test('run action on non-existent plugin returns error', async ({ apiClient }) => {
    const response = await apiClient.request.post('/v1/jobs/action/run', {
      headers: { 'Content-Type': 'application/json' },
      data: JSON.stringify({
        plugin: 'non-existent-plugin',
        action: 'some-action',
        entity_ids: [1],
        params: {},
      }),
    });
    expect(response.ok()).toBeFalsy();
    expect(response.status()).toBe(404);
  });

  test('bulk sync action runs on multiple entities', async ({ apiClient }) => {
    const response = await apiClient.request.post('/v1/jobs/action/run', {
      headers: { 'Content-Type': 'application/json' },
      data: JSON.stringify({
        plugin: 'test-actions',
        action: 'sync-greet',
        entity_ids: [1, 2, 3],
        params: { greeting: 'Bulk hello' },
      }),
    });
    expect(response.ok()).toBeTruthy();
    const data = await response.json();
    expect(data.results).toBeDefined();
    expect(data.results).toHaveLength(3);
    for (const result of data.results) {
      expect(result.success).toBeTruthy();
      expect(result.message).toContain('Bulk hello');
    }
  });

  test('bulk async action returns multiple job IDs', async ({ apiClient }) => {
    const response = await apiClient.request.post('/v1/jobs/action/run', {
      headers: { 'Content-Type': 'application/json' },
      data: JSON.stringify({
        plugin: 'test-actions',
        action: 'async-demo',
        entity_ids: [1, 2],
        params: { steps: 1 },
      }),
    });
    expect(response.status()).toBe(202);
    const data = await response.json();
    expect(data.job_ids).toBeDefined();
    expect(data.job_ids).toHaveLength(2);

    for (const jobId of data.job_ids) {
      let job: any;
      for (let i = 0; i < 20; i++) {
        const jobResp = await apiClient.request.get(`/v1/jobs/action/job?id=${jobId}`);
        job = await jobResp.json();
        if (job.status === 'completed' || job.status === 'failed') break;
        await new Promise(r => setTimeout(r, 200));
      }
      expect(job.status).toBe('completed');
    }
  });
});

// ── UI Tests (each describe block creates its own entities) ──

test.describe('Plugin Actions UI - Detail Pages', () => {
  let resourceId: number;
  let groupId: number;

  test.beforeAll(async () => {
    const baseURL = getWorkerBaseUrl();
    await ensurePluginEnabled(baseURL, 'test-actions');

    const entities = await createTestEntities(baseURL);
    resourceId = entities.resourceId;
    groupId = entities.groupId;
  });

  test('detail page shows plugin action buttons for resource', async ({ page }) => {
    await page.goto(`/resource?id=${resourceId}`);
    await page.waitForLoadState('load');

    const greetButton = page.getByRole('button', { name: 'Greet Resource' });
    await expect(greetButton).toBeVisible();

    const asyncButton = page.getByRole('button', { name: 'Async Demo' });
    await expect(asyncButton).toBeVisible();
  });

  test('clicking action button opens modal with params', async ({ page }) => {
    await page.goto(`/resource?id=${resourceId}`);
    await page.waitForLoadState('load');

    await page.getByRole('button', { name: 'Greet Resource' }).click();

    const modal = page.getByRole('dialog');
    await expect(modal).toBeVisible();
    await expect(modal).toContainText('Greet Resource');

    const greetingInput = modal.locator('#plugin-param-greeting');
    await expect(greetingInput).toBeVisible();
    await expect(greetingInput).toHaveValue('Hello');
  });

  test('submitting action modal shows success result', async ({ page }) => {
    await page.goto(`/resource?id=${resourceId}`);
    await page.waitForLoadState('load');

    await page.getByRole('button', { name: 'Greet Resource' }).click();

    const modal = page.getByRole('dialog');
    await expect(modal).toBeVisible();

    const greetingInput = modal.locator('#plugin-param-greeting');
    await greetingInput.clear();
    await greetingInput.fill('Hello World');

    await modal.getByRole('button', { name: 'Run' }).click();

    const resultArea = modal.locator('[role="status"]');
    await expect(resultArea).toBeVisible({ timeout: 5000 });
    await expect(resultArea).toContainText('Hello World');
  });

  test('detail page shows group action for groups', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    await page.waitForLoadState('load');

    const groupButton = page.getByRole('button', { name: 'Group Action' });
    await expect(groupButton).toBeVisible();
  });

  test('resource actions do not appear on group detail page', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    await page.waitForLoadState('load');

    const greetButton = page.getByRole('button', { name: 'Greet Resource' });
    await expect(greetButton).not.toBeVisible();
  });

  test('group actions do not appear on resource detail page', async ({ page }) => {
    await page.goto(`/resource?id=${resourceId}`);
    await page.waitForLoadState('load');

    const groupButton = page.getByRole('button', { name: 'Group Action' });
    await expect(groupButton).not.toBeVisible();
  });

  test('modal validation prevents submit without required field', async ({ page }) => {
    await page.goto(`/resource?id=${resourceId}`);
    await page.waitForLoadState('load');

    await page.getByRole('button', { name: 'Greet Resource' }).click();

    const modal = page.getByRole('dialog');
    await expect(modal).toBeVisible();

    const greetingInput = modal.locator('#plugin-param-greeting');
    await greetingInput.clear();

    await modal.getByRole('button', { name: 'Run' }).click();

    await expect(modal).toBeVisible();

    const resultArea = modal.locator('[role="status"]');
    await expect(resultArea).not.toBeVisible();
  });

  test('async action submission opens jobs panel', async ({ page }) => {
    await page.goto(`/resource?id=${resourceId}`);
    await page.waitForLoadState('load');

    await page.getByRole('button', { name: 'Async Demo' }).click();

    const modal = page.getByRole('dialog');
    await expect(modal).toBeVisible();

    await modal.getByRole('button', { name: 'Run' }).click();

    await expect(modal).not.toBeVisible({ timeout: 5000 });

    const jobsPanel = page.locator('text=Jobs').first();
    await expect(jobsPanel).toBeVisible({ timeout: 5000 });
  });
});

test.describe('Plugin Actions UI - Card Menu', () => {
  test.beforeAll(async () => {
    const baseURL = getWorkerBaseUrl();
    await ensurePluginEnabled(baseURL, 'test-actions');
  });

  test('card kebab menu appears on resources list page', async ({ page }) => {
    await page.goto('/resources');
    await page.waitForLoadState('load');

    const kebabButton = page.locator('button[aria-label="More actions"]').first();
    await expect(kebabButton).toBeVisible();

    await kebabButton.click();

    const menuItem = page.getByRole('menuitem', { name: 'Greet Resource' });
    await expect(menuItem).toBeVisible();
  });

  test('card kebab menu triggers action modal', async ({ page }) => {
    await page.goto('/resources');
    await page.waitForLoadState('load');

    const kebabButton = page.locator('button[aria-label="More actions"]').first();
    await kebabButton.click();

    await page.getByRole('menuitem', { name: 'Greet Resource' }).click();

    const modal = page.getByRole('dialog');
    await expect(modal).toBeVisible();
    await expect(modal).toContainText('Greet Resource');
  });
});

// ── Plugin Disable/Enable Test (isolated, creates its own entities) ──

test.describe('Plugin Actions - Disable/Enable', () => {
  let resourceId: number;

  test.beforeAll(async () => {
    const baseURL = getWorkerBaseUrl();
    await ensurePluginEnabled(baseURL, 'test-actions');

    const entities = await createTestEntities(baseURL);
    resourceId = entities.resourceId;
  });

  test.afterAll(async () => {
    const baseURL = getWorkerBaseUrl();
    await ensurePluginEnabled(baseURL, 'test-actions');
  });

  test('disabling plugin removes actions from detail page', async ({ page, apiClient }) => {
    // Verify action is visible first
    await page.goto(`/resource?id=${resourceId}`);
    await page.waitForLoadState('load');
    await expect(page.getByRole('button', { name: 'Greet Resource' })).toBeVisible();

    // Disable the plugin
    await apiClient.disablePlugin('test-actions');

    // Reload and verify action is gone
    await page.goto(`/resource?id=${resourceId}`);
    await page.waitForLoadState('load');
    await expect(page.getByRole('button', { name: 'Greet Resource' })).not.toBeVisible();

    // Re-enable for other test suites
    await apiClient.enablePlugin('test-actions');
  });
});
