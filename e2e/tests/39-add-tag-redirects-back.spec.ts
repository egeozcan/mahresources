/**
 * Tests that adding a tag on a group detail page redirects back to the same
 * detail page, not the groups list page.
 *
 * Bug: The tag form uses encodeURIComponent(window.location) which produces
 * a full absolute URL (http://...). The server's isSafeRedirect rejects
 * absolute URLs (must start with /), falling back to the list page.
 */
import { test, expect } from '../fixtures/base.fixture';

test.describe('Add tag redirect stays on detail page', () => {
  let categoryId: number;
  let groupId: number;
  let tagId: number;

  test.beforeAll(async ({ apiClient }) => {
    const category = await apiClient.createCategory(
      'Tag Redirect Test Category',
      'For tag redirect test',
    );
    categoryId = category.ID;

    const group = await apiClient.createGroup({
      name: 'Tag Redirect Test Group',
      description: 'Group for testing tag add redirect',
      categoryId,
    });
    groupId = group.ID;

    const tag = await apiClient.createTag(
      'RedirectTestTag',
      'Tag for redirect test',
    );
    tagId = tag.ID;
  });

  test('adding a tag on group detail page should stay on that page', async ({
    page,
  }) => {
    // Navigate to the group detail page
    await page.goto(`/group?id=${groupId}`);
    await page.waitForLoadState('load');

    // Find the "Add Tag" autocompleter combobox and type the tag name
    const tagInput = page.locator('form[action*="addTags"] [role="combobox"]');
    await expect(tagInput).toBeVisible({ timeout: 5000 });
    await tagInput.fill('RedirectTestTag');

    // Wait for and select the dropdown result
    const option = page.locator('[role="option"]').filter({ hasText: 'RedirectTestTag' });
    await expect(option).toBeVisible({ timeout: 5000 });
    await option.click();

    // Click the "Add Tags" submit button
    await page.locator('form[action*="addTags"] button[type="submit"]').click();

    // Wait for the form to submit and page to reload
    await page.waitForLoadState('load');

    // Should still be on the group detail page, NOT redirected to /groups
    expect(page.url()).toContain(`/group?id=${groupId}`);
    expect(page.url()).not.toMatch(/\/groups(\?|$)/);
  });

  test.afterAll(async ({ apiClient }) => {
    if (groupId) await apiClient.deleteGroup(groupId);
    if (tagId) await apiClient.deleteTag(tagId);
    if (categoryId) await apiClient.deleteCategory(categoryId);
  });
});
