/**
 * Tests that the "Edit Tags" button on the dashboard works, not just on
 * the resources list page.
 *
 * Bug: The click handler for .edit-in-list buttons is registered with the
 * selector ".list-container .tags", but the dashboard uses ".dashboard-grid"
 * instead of ".list-container", so the handler never attaches.
 */
import { test, expect } from '../fixtures/base.fixture';
import path from 'path';

test.describe('Dashboard Edit Tags button works', () => {
  let categoryId: number;
  let ownerGroupId: number;
  let resourceId: number;
  let tagId: number;

  test.beforeAll(async ({ apiClient }) => {
    const category = await apiClient.createCategory(
      'Dashboard Tag Test Category',
      'For dashboard edit-tags test',
    );
    categoryId = category.ID;

    const ownerGroup = await apiClient.createGroup({
      name: 'Dashboard Tag Test Owner',
      categoryId,
    });
    ownerGroupId = ownerGroup.ID;

    const tag = await apiClient.createTag(
      'DashboardTestTag',
      'Tag for dashboard test',
    );
    tagId = tag.ID;

    // Create a small temp file for the resource
    const fs = await import('fs');
    const os = await import('os');
    const tmpFile = path.join(os.tmpdir(), 'dashboard-tag-test.txt');
    fs.writeFileSync(tmpFile, 'test content for dashboard tag test');

    const resource = await apiClient.createResource({
      filePath: tmpFile,
      name: 'Dashboard Test Resource',
      ownerId: ownerGroupId,
    });
    resourceId = resource.ID;

    fs.unlinkSync(tmpFile);
  });

  test('clicking Edit Tags on dashboard should show the tag editor', async ({
    page,
  }) => {
    await page.goto('/dashboard');
    await page.waitForLoadState('load');

    // Find the "Edit Tags" button on a resource card
    const editTagsButton = page.locator('button.edit-in-list').first();
    await expect(editTagsButton).toBeVisible({ timeout: 5000 });

    // Click the button
    await editTagsButton.click();

    // After clicking, the tag editor (an autocompleter form) should appear
    // inside the .tags container, replacing the button
    const tagEditor = page.locator('.tags form.active, .card-tags form.active').first();
    await expect(tagEditor).toBeVisible({ timeout: 3000 });
  });

  test.afterAll(async ({ apiClient }) => {
    if (resourceId) await apiClient.deleteResource(resourceId);
    if (tagId) await apiClient.deleteTag(tagId);
    if (ownerGroupId) await apiClient.deleteGroup(ownerGroupId);
    if (categoryId) await apiClient.deleteCategory(categoryId);
  });
});
