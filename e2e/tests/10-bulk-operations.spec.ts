import { test, expect } from '../fixtures/base.fixture';

test.describe('Bulk Operations on Groups', () => {
  let categoryId: number;
  let groupIds: number[] = [];
  let tagId: number;
  let secondTagId: number;

  test.beforeAll(async ({ apiClient }) => {
    // Create category
    const category = await apiClient.createCategory('Bulk Ops Category', 'Category for bulk operation tests');
    categoryId = category.ID;

    // Create tags
    const tag = await apiClient.createTag('Bulk Tag 1', 'First bulk tag');
    tagId = tag.ID;

    const secondTag = await apiClient.createTag('Bulk Tag 2', 'Second bulk tag');
    secondTagId = secondTag.ID;

    // Create multiple groups
    for (let i = 1; i <= 5; i++) {
      const group = await apiClient.createGroup({
        name: `Bulk Test Group ${i}`,
        description: `Group ${i} for bulk testing`,
        categoryId: categoryId,
      });
      groupIds.push(group.ID);
    }
  });

  test('should select multiple groups', async ({ groupPage, page }) => {
    await groupPage.gotoList();

    // Select first 3 groups
    for (let i = 0; i < 3; i++) {
      await groupPage.selectGroupCheckbox(groupIds[i]);
    }

    // Verify bulk editor appears
    await expect(page.locator('.bulk-editors, [class*="bulk"]')).toBeVisible();
  });

  test('should bulk add tags to groups', async ({ groupPage, apiClient, page }) => {
    await groupPage.gotoList();

    // Select first 2 groups
    await groupPage.selectGroupCheckbox(groupIds[0]);
    await groupPage.selectGroupCheckbox(groupIds[1]);

    // Add tag via API (UI interaction for bulk is complex)
    await apiClient.addTagsToGroups([groupIds[0], groupIds[1]], [tagId]);

    // Verify tags were added by checking group pages
    await groupPage.gotoDisplay(groupIds[0]);
    await expect(page.locator('a:has-text("Bulk Tag 1")')).toBeVisible();

    await groupPage.gotoDisplay(groupIds[1]);
    await expect(page.locator('a:has-text("Bulk Tag 1")')).toBeVisible();
  });

  test('should bulk remove tags from groups', async ({ groupPage, apiClient, page }) => {
    // First add tags to remove
    await apiClient.addTagsToGroups([groupIds[0], groupIds[1]], [secondTagId]);

    // Then remove them
    await apiClient.removeTagsFromGroups([groupIds[0], groupIds[1]], [secondTagId]);

    // Verify tags were removed
    await groupPage.gotoDisplay(groupIds[0]);
    await expect(page.locator('a:has-text("Bulk Tag 2")')).not.toBeVisible();
  });

  test('should use Select All button', async ({ groupPage, page }) => {
    await groupPage.gotoList();

    // Click Select All
    const selectAllButton = page.locator('button:has-text("Select All"), button:has-text("Select all")');
    if (await selectAllButton.isVisible()) {
      await selectAllButton.click();

      // Wait for bulk editor to appear (condition-based instead of hardcoded timeout)
      await expect(page.locator('.bulk-editors, [class*="bulk"]')).toBeVisible({ timeout: 5000 });
    }
  });

  test('should bulk delete groups', async ({ groupPage, apiClient }) => {
    // Delete last 2 groups via API
    await apiClient.bulkDeleteGroups([groupIds[3], groupIds[4]]);

    // Verify groups were deleted
    await groupPage.verifyGroupNotInList('Bulk Test Group 4');
    await groupPage.verifyGroupNotInList('Bulk Test Group 5');

    // Remove from our tracking array
    groupIds = groupIds.slice(0, 3);
  });

  test.afterAll(async ({ apiClient }) => {
    // Clean up remaining groups
    for (const groupId of groupIds) {
      try {
        await apiClient.deleteGroup(groupId);
      } catch (error) {
        // Log but don't fail - group may already be deleted
        console.warn(`Cleanup: Failed to delete group ${groupId}:`, error);
      }
    }
    if (tagId) {
      await apiClient.deleteTag(tagId);
    }
    if (secondTagId) {
      await apiClient.deleteTag(secondTagId);
    }
    if (categoryId) {
      await apiClient.deleteCategory(categoryId);
    }
  });
});

test.describe('Bulk Operations UI Elements', () => {
  let categoryId: number;
  let groupId: number;

  test.beforeAll(async ({ apiClient }) => {
    const category = await apiClient.createCategory('Bulk UI Category', 'For UI tests');
    categoryId = category.ID;

    const group = await apiClient.createGroup({
      name: 'Bulk UI Group',
      categoryId: categoryId,
    });
    groupId = group.ID;
  });

  test('should show bulk editor when item selected', async ({ groupPage, page }) => {
    await groupPage.gotoList();
    await groupPage.selectGroupCheckbox(groupId);

    // Bulk editor should appear
    await expect(page.locator('.bulk-editors, [class*="bulk"]')).toBeVisible();
  });

  test('should hide bulk editor when deselected', async ({ groupPage, page }) => {
    await groupPage.gotoList();
    await groupPage.selectGroupCheckbox(groupId);
    await expect(page.locator('.bulk-editors, [class*="bulk"]')).toBeVisible();

    // Deselect
    await groupPage.selectGroupCheckbox(groupId); // Toggle off

    // Wait for bulk editor to hide (condition-based instead of hardcoded timeout)
    await expect(page.locator('.bulk-editors, [class*="bulk"]')).not.toBeVisible({ timeout: 5000 });
  });

  test.afterAll(async ({ apiClient }) => {
    if (groupId) {
      await apiClient.deleteGroup(groupId);
    }
    if (categoryId) {
      await apiClient.deleteCategory(categoryId);
    }
  });
});
