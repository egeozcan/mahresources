import { test, expect } from '../../fixtures/base.fixture';

test('groups list "Export selected" pre-fills the export page', async ({ page, apiClient }) => {
  const timestamp = Date.now();

  // categoryId is required by createGroup
  const category = await apiClient.createCategory(`BulkExportCat_${timestamp}`);

  const a = await apiClient.createGroup({ name: `BulkA_${timestamp}`, categoryId: category.ID });
  const b = await apiClient.createGroup({ name: `BulkB_${timestamp}`, categoryId: category.ID });

  await page.goto('/groups');
  await page.waitForLoadState('load');

  // Select both groups using the same pattern as GroupPage.selectGroupCheckbox
  await page.locator(`[x-data*="itemId: ${a.ID}"] input[type="checkbox"]`).check();
  await page.locator(`[x-data*="itemId: ${b.ID}"] input[type="checkbox"]`).check();

  await page.getByTestId('bulk-export-selected').click();

  await expect(page).toHaveURL(new RegExp(`/admin/export\\?groups=(${a.ID},${b.ID}|${b.ID},${a.ID})`));

  // The chip picker populates asynchronously via fetch
  await expect(page.getByText(`BulkA_${timestamp}`)).toBeVisible({ timeout: 10000 });
  await expect(page.getByText(`BulkB_${timestamp}`)).toBeVisible({ timeout: 10000 });
});
