import { test, expect } from '../fixtures/base.fixture';

test.describe('BH-025: adminExport survives reload', () => {
  test('progress panel re-appears after page reload during running export', async ({ page, apiClient }) => {
    // Create a category + group to export
    const suffix = Date.now();
    const category = await apiClient.createCategory(`bh025-cat-${suffix}`);
    const group = await apiClient.createGroup({ name: `bh025-grp-${suffix}`, categoryId: category.ID });

    await page.goto('/admin/export');

    // Type the group name to search and add it
    await page.locator('[aria-label="Search groups to add"]').fill(group.Name);
    await page.waitForTimeout(400); // debounce
    await page.locator('ul li button').filter({ hasText: group.Name }).first().click();

    // Start the export
    await page.locator('[data-testid="export-submit-button"]').click();

    // Progress panel should appear immediately
    await expect(page.locator('[data-testid="export-progress-panel"]')).toBeVisible({ timeout: 5000 });

    // Reload the page mid-export
    await page.reload();

    // Wait for Alpine to finish initializing (x-cloak is removed by Alpine after init)
    await page.waitForFunction(() => document.querySelectorAll('[x-cloak]').length === 0, { timeout: 8000 });

    // BH-025 symptom: panel was not rehydrated after reload — it should now reappear
    await expect(page.locator('[data-testid="export-progress-panel"]')).toBeVisible({ timeout: 8000 });
  });
});
