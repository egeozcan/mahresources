import { test, expect } from '../fixtures/base.fixture';
import path from 'path';

test.describe('Auto-detect resource category', () => {
  let categoryId: number;
  const testRunId = Date.now() + Math.floor(Math.random() * 100000);

  test.beforeEach(async ({ apiClient }) => {
    const cat = await apiClient.createResourceCategory(
      `Auto PNG ${testRunId}`,
      'Auto-detects PNG uploads',
      {
        AutoDetectRules: JSON.stringify({
          contentTypes: ['image/png'],
          priority: 10,
        }),
      }
    );
    categoryId = cat.ID;
  });

  test.afterEach(async ({ apiClient }) => {
    if (categoryId) {
      await apiClient.deleteResourceCategory(categoryId).catch(() => {});
    }
  });

  test('resource uploaded without category is auto-detected', async ({ apiClient }) => {
    // Upload a PNG without specifying a category (resourceCategoryId omitted)
    const resource = await apiClient.createResource({
      filePath: path.join(__dirname, '../test-assets/sample-image-37.png'),
      name: `auto-detect-test-${testRunId}.png`,
    });

    try {
      // Fetch the resource detail to check its category
      const detail = await apiClient.getResource(resource.ID);
      expect(detail.resourceCategoryId).toBe(categoryId);
    } finally {
      await apiClient.deleteResource(resource.ID);
    }
  });

  test('resource uploaded with explicit category is not overridden', async ({ apiClient }) => {
    const resource = await apiClient.createResource({
      filePath: path.join(__dirname, '../test-assets/sample-image-38.png'),
      name: `explicit-cat-test-${testRunId}.png`,
      resourceCategoryId: 1, // Default category
    });

    try {
      const detail = await apiClient.getResource(resource.ID);
      expect(detail.resourceCategoryId).toBe(1);
    } finally {
      await apiClient.deleteResource(resource.ID);
    }
  });

  test('category autocompleter allows empty selection on resource create form', async ({ page }) => {
    await page.goto('/resource/new');
    await page.waitForLoadState('load');

    // The autocompleter for ResourceCategoryId should exist
    // It renders as a div with x-data containing elName:'ResourceCategoryId'
    // containing an input with role="combobox"
    const resourceCategorySection = page.locator(
      'div.sm\\:grid:has(span:has-text("Resource Category"))'
    );
    await expect(resourceCategorySection).toBeVisible();

    const combobox = resourceCategorySection.locator('input[role="combobox"]');
    await expect(combobox).toBeVisible();

    // No category should be pre-selected for new resources.
    // The autocompleter renders selected items as <p> badges with class bg-amber-100.
    // For a new resource, there should be no selected items.
    const selectedBadges = resourceCategorySection.locator('p.bg-amber-100');
    await expect(selectedBadges).toHaveCount(0);
  });
});
