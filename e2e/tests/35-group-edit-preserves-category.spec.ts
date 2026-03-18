/**
 * Tests that editing a group via the web form preserves its category.
 *
 * Bug: The group edit form hides the Category selector ({% if !group.ID %}),
 * so submitting the edit form sends categoryId=0, which clears the category
 * to "Uncategorized".
 */
import { test, expect } from '../fixtures/base.fixture';

test.describe('Group edit preserves category', () => {
  let categoryId: number;
  let groupId: number;

  test.beforeAll(async ({ apiClient }) => {
    const category = await apiClient.createCategory(
      'Preserve Cat Test',
      'Category for edit-preserves-category test',
    );
    categoryId = category.ID;

    const group = await apiClient.createGroup({
      name: 'Cat Preserve Group',
      description: 'Group whose category should survive editing',
      categoryId,
    });
    groupId = group.ID;
  });

  test('category is preserved after editing group name via UI', async ({
    page,
    apiClient,
  }) => {
    // Verify category is set before editing
    const beforeEdit = await apiClient.getGroup(groupId);
    expect(beforeEdit.CategoryId).toBe(categoryId);

    // Navigate to the edit form
    await page.goto(`/group/edit?id=${groupId}`);
    await page.waitForLoadState('load');

    // Change the name and save
    const nameInput = page.locator('input[name="name"]');
    await nameInput.clear();
    await nameInput.fill('Cat Preserve Group Edited');
    await page.locator('button[type="submit"]').click();

    // Wait for redirect to group display page
    await page.waitForURL(/\/group\?id=/);

    // Verify category is still set via API
    const afterEdit = await apiClient.getGroup(groupId);
    expect(afterEdit.CategoryId).toBe(categoryId);
  });

  test.afterAll(async ({ apiClient }) => {
    if (groupId) await apiClient.deleteGroup(groupId);
    if (categoryId) await apiClient.deleteCategory(categoryId);
  });
});
