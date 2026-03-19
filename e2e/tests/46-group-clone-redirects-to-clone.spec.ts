/**
 * Tests that cloning a group redirects to the newly created clone,
 * not back to the original group.
 *
 * Bug: The clone form appends a redirect param pointing to the original
 * group, overriding the server's redirect to the new clone.
 */
import { test, expect } from '../fixtures/base.fixture';

test.describe('Group clone redirects to the new clone', () => {
  let categoryId: number;
  let groupId: number;

  test.beforeAll(async ({ apiClient }) => {
    const category = await apiClient.createCategory(
      'Clone Redirect Test Category',
      'For clone redirect test',
    );
    categoryId = category.ID;

    const group = await apiClient.createGroup({
      name: 'Original Group To Clone',
      description: 'This group will be cloned',
      categoryId,
    });
    groupId = group.ID;
  });

  test('cloning a group should redirect to the cloned group, not the original', async ({
    page,
  }) => {
    await page.goto(`/group?id=${groupId}`);
    await page.waitForLoadState('load');

    // Accept the confirm dialog that will appear
    page.on('dialog', (dialog) => dialog.accept());

    // Find and click the Clone button
    const cloneButton = page.locator('form[action*="clone"] button[type="submit"]');
    await expect(cloneButton).toBeVisible({ timeout: 5000 });
    await cloneButton.click();

    // Wait for navigation after clone
    await page.waitForLoadState('load');

    // The URL should point to a DIFFERENT group (the clone), not the original
    const url = new URL(page.url());
    const newId = url.searchParams.get('id');
    expect(newId).toBeTruthy();
    expect(parseInt(newId!, 10)).not.toBe(groupId);
  });

  test.afterAll(async ({ apiClient }) => {
    // Clean up both original and any clones
    const groups = await apiClient.getGroups();
    for (const g of groups) {
      if (g.Name === 'Original Group To Clone' || g.Name.includes('Original Group To Clone')) {
        await apiClient.deleteGroup(g.ID);
      }
    }
    if (categoryId) await apiClient.deleteCategory(categoryId);
  });
});
