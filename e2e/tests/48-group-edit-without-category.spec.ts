/**
 * Tests that a group without a category can be edited via the UI.
 *
 * Bug: The Category autocompleter has min=1, blocking form submission
 * for groups that don't have a category. Category is optional in the
 * data model (CategoryId is nullable).
 */
import { test, expect } from '../fixtures/base.fixture';

test.describe('Group without category can be edited', () => {
  let groupId: number;

  test.beforeAll(async ({ request, baseURL }) => {
    // Create a group WITHOUT a category via JSON API (bypasses client's required categoryId)
    const response = await request.post(`${baseURL}/v1/group`, {
      data: { Name: 'Uncategorized Group', Description: 'Group with no category' },
      headers: { 'Content-Type': 'application/json' },
    });
    const group = await response.json();
    groupId = group.ID;
  });

  test('editing an uncategorized group should save successfully', async ({
    page,
  }) => {
    await page.goto(`/group/edit?id=${groupId}`);
    await page.waitForLoadState('load');

    // Change just the name
    const nameInput = page.locator('input[name="name"]');
    await nameInput.clear();
    await nameInput.fill('Uncategorized Group Edited');

    // Click save
    await page.locator('button[type="submit"]').click();

    // Should redirect to the group display page, NOT stay on the form
    await page.waitForURL(/\/group\?id=/, { timeout: 5000 });

    // Verify the name was updated
    await expect(page.locator('h1')).toContainText('Uncategorized Group Edited');
  });

  test.afterAll(async ({ apiClient }) => {
    if (groupId) await apiClient.deleteGroup(groupId);
  });
});
