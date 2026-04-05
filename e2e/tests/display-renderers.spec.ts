/**
 * E2E tests for built-in display renderers (URL, GeoLocation, DateRange, Dimensions).
 * Tests that object values matching well-known shapes render with smart formatting
 * instead of raw key-value grids.
 */
import { test, expect } from '../fixtures/base.fixture';

test.describe('Built-in display renderers', () => {
  let categoryId: number;
  let groupId: number;

  const schema = JSON.stringify({
    type: 'object',
    properties: {
      url: {
        type: 'object',
        title: 'Website',
        properties: {
          href: { type: 'string' },
          host: { type: 'string' },
          protocol: { type: 'string' },
        },
      },
      location: {
        type: 'object',
        title: 'Location',
        properties: {
          latitude: { type: 'number' },
          longitude: { type: 'number' },
        },
      },
      period: {
        type: 'object',
        title: 'Period',
        properties: {
          start: { type: 'string' },
          end: { type: 'string' },
        },
      },
      size: {
        type: 'object',
        title: 'Size',
        properties: {
          width: { type: 'number' },
          height: { type: 'number' },
        },
      },
    },
  });

  const meta = JSON.stringify({
    url: {
      href: 'https://www.example.com/page',
      host: 'www.example.com',
      protocol: 'https:',
    },
    location: {
      latitude: 52.520008,
      longitude: 13.404954,
    },
    period: {
      start: '2024-03-15',
      end: '2024-04-01',
    },
    size: {
      width: 1920,
      height: 1080,
    },
  });

  test.beforeAll(async ({ apiClient }) => {
    const cat = await apiClient.createCategory(
      `Renderer Test ${Date.now()}`,
      'Testing built-in display renderers',
      { MetaSchema: schema },
    );
    categoryId = cat.ID;
    const group = await apiClient.createGroup({
      name: `Renderer Group ${Date.now()}`,
      categoryId: cat.ID,
      meta,
    });
    groupId = group.ID;
  });

  test.afterAll(async ({ apiClient }) => {
    if (groupId) await apiClient.deleteGroup(groupId);
    if (categoryId) await apiClient.deleteCategory(categoryId);
  });

  test('renders URL as clickable link with host subtitle', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    await page.waitForLoadState('load');

    const display = page.locator('schema-editor[mode="display"]');
    await expect(display).toBeVisible({ timeout: 5000 });

    const link = display.locator('a[href="https://www.example.com/page"]');
    await expect(link).toBeVisible({ timeout: 3000 });
    await expect(display).toContainText('www.example.com');
  });

  test('renders GeoLocation as coordinates with map link', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    await page.waitForLoadState('load');

    const display = page.locator('schema-editor[mode="display"]');
    await expect(display).toBeVisible({ timeout: 5000 });

    await expect(display).toContainText('52.520008');
    await expect(display).toContainText('13.404954');

    const mapLink = display.locator('a[href*="openstreetmap.org"]');
    await expect(mapLink).toBeVisible();
  });

  test('renders DateRange as formatted dates', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    await page.waitForLoadState('load');

    const display = page.locator('schema-editor[mode="display"]');
    await expect(display).toBeVisible({ timeout: 5000 });

    await expect(display).toContainText('2024');
    await expect(display).toContainText('\u2014');
  });

  test('renders Dimensions as W x H', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    await page.waitForLoadState('load');

    const display = page.locator('schema-editor[mode="display"]');
    await expect(display).toBeVisible({ timeout: 5000 });

    await expect(display).toContainText('1920');
    await expect(display).toContainText('1080');
    await expect(display).toContainText('\u00D7');
  });
});

test.describe('x-display opt-out', () => {
  test('x-display: "raw" shows key-value grid for URL-shaped object', async ({ page, apiClient }) => {
    const schema = JSON.stringify({
      type: 'object',
      properties: {
        link: {
          type: 'object',
          title: 'Link',
          'x-display': 'raw',
          properties: {
            href: { type: 'string' },
            host: { type: 'string' },
          },
        },
      },
    });
    const cat = await apiClient.createCategory(
      `Raw Test ${Date.now()}`,
      'Testing x-display raw opt-out',
      { MetaSchema: schema },
    );
    const group = await apiClient.createGroup({
      name: `Raw Group ${Date.now()}`,
      categoryId: cat.ID,
      meta: JSON.stringify({
        link: { href: 'https://example.com', host: 'example.com' },
      }),
    });

    try {
      await page.goto(`/group?id=${group.ID}`);
      await page.waitForLoadState('load');

      const display = page.locator('schema-editor[mode="display"]');
      await expect(display).toBeVisible({ timeout: 5000 });

      // Should show as key-value grid (not a clickable link)
      await expect(display).toContainText('https://example.com');
      const link = display.locator('a[href="https://example.com"]');
      await expect(link).not.toBeVisible();
    } finally {
      await apiClient.deleteGroup(group.ID);
      await apiClient.deleteCategory(cat.ID);
    }
  });
});
