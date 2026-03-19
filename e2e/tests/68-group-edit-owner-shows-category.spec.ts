/**
 * Tests that the group edit form shows the Owner with its category name.
 *
 * Bug: On the group edit page, the Owner autocompleter pill shows just the
 * owner's name (e.g. "ParentGroup") instead of "ParentGroup (CategoryName)".
 * This happens because GroupCreateContextProvider uses `group.Owner` from the
 * initial Preload("Owner"), which does NOT preload Owner.Category.
 * In contrast, the note edit page calls GetGroup(ownerId) for the owner,
 * which fully preloads Category, so the note edit form correctly shows
 * "ParentGroup (CategoryName)".
 */
import { test, expect } from '../fixtures/base.fixture';

test.describe('Group edit Owner shows category', () => {
  let categoryId: number;
  let parentGroupId: number;
  let childGroupId: number;

  test.beforeAll(async ({ apiClient }) => {
    const category = await apiClient.createCategory(
      'OwnerCatTest',
      'Category for owner display test',
    );
    categoryId = category.ID;

    const parent = await apiClient.createGroup({
      name: 'ParentGroupForTest',
      description: 'The parent/owner group',
      categoryId,
    });
    parentGroupId = parent.ID;

    const child = await apiClient.createGroup({
      name: 'ChildGroupForTest',
      description: 'The child group with an owner',
      categoryId,
      ownerId: parentGroupId,
    });
    childGroupId = child.ID;
  });

  test('owner pill on group edit form includes category name', async ({
    page,
  }) => {
    // Navigate to the child group's edit form
    await page.goto(`/group/edit?id=${childGroupId}`);
    await page.waitForLoadState('load');

    // The Owner autocompleter should show the parent group with its category.
    // On the note edit form this correctly shows "ParentGroupForTest (OwnerCatTest)".
    // On the group edit form this incorrectly shows just "ParentGroupForTest".
    const ownerPill = page.locator(
      'button[aria-label*="Remove ParentGroupForTest"]',
    );
    await expect(ownerPill).toBeVisible();

    // The pill's sibling text span should include the category name in parentheses.
    // getItemDisplayName returns "Name (Category)" when extraInfo="Category" is set
    // and the Category object is present on the item.
    const pillContainer = ownerPill.locator('..');
    const pillText = await pillContainer.locator('span').first().textContent();

    expect(pillText).toContain('(OwnerCatTest)');
  });

  test.afterAll(async ({ apiClient }) => {
    if (childGroupId) await apiClient.deleteGroup(childGroupId);
    if (parentGroupId) await apiClient.deleteGroup(parentGroupId);
    if (categoryId) await apiClient.deleteCategory(categoryId);
  });
});
