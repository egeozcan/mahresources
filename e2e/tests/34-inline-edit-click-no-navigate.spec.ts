/**
 * Tests that clicking the inline-edit input on a group card in the list view
 * does NOT navigate to the group detail page.
 *
 * Bug: <inline-edit> is nested inside an <a> tag in group.tpl. When the user
 * clicks the pencil icon to enter edit mode, then clicks the input field to
 * position the cursor, the click event bubbles through the shadow DOM boundary
 * to the parent <a>, causing unwanted navigation to /group?id=...
 */
import { test, expect } from '../fixtures/base.fixture';

test.describe('Inline-edit inside link should not navigate on click', () => {
  let categoryId: number;
  let groupId: number;

  test.beforeAll(async ({ apiClient }) => {
    const category = await apiClient.createCategory(
      'Inline Nav Test Category',
      'Category for inline-edit navigation test',
    );
    categoryId = category.ID;

    const group = await apiClient.createGroup({
      name: 'Navigate Test Group',
      description: 'Group to test inline-edit click does not navigate',
      categoryId,
    });
    groupId = group.ID;
  });

  test('clicking inline-edit input on groups list should stay on groups page', async ({
    page,
  }) => {
    await page.goto('/groups');
    await page.waitForLoadState('load');

    // Find the inline-edit component on the group card
    const inlineEdit = page.locator('inline-edit').first();
    await expect(inlineEdit).toBeVisible({ timeout: 5000 });

    // Click the edit button (pencil icon) inside shadow DOM to enter edit mode
    const editButton = inlineEdit.locator('button.edit-button');
    await editButton.click();

    // The input should now be visible inside the shadow DOM
    const input = inlineEdit.locator('input');
    await expect(input).toBeVisible({ timeout: 2000 });

    // Click the input field — this is where the bug manifests:
    // the click bubbles to the parent <a> and navigates away
    await input.click();

    // We should still be on the groups list page, NOT navigated to /group?id=...
    await page.waitForTimeout(500); // allow any navigation to settle
    expect(page.url()).toContain('/groups');
    expect(page.url()).not.toContain('/group?id=');

    // The input should still be visible (still in edit mode)
    await expect(input).toBeVisible();
  });

  test.afterAll(async ({ apiClient }) => {
    if (groupId) await apiClient.deleteGroup(groupId);
    if (categoryId) await apiClient.deleteCategory(categoryId);
  });
});
