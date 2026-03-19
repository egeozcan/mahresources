/**
 * Tests that clicking a category badge on a sub-group card within a group
 * detail page navigates to the groups list filtered by that category,
 * not back to the same group page.
 *
 * Bug: The seeAll.tpl template doesn't pass tagBaseUrl when rendering
 * sub-group cards, so withQuery() appends to the current URL (/group?id=X)
 * instead of navigating to /groups?categories=Y.
 */
import { test, expect } from '../fixtures/base.fixture';

test.describe('Sub-group category badge links to groups list', () => {
  let categoryId: number;
  let parentGroupId: number;
  let childGroupId: number;

  test.beforeAll(async ({ apiClient }) => {
    const category = await apiClient.createCategory(
      'BadgeLinkTestCategory',
      'For badge link test',
    );
    categoryId = category.ID;

    const parentGroup = await apiClient.createGroup({
      name: 'Badge Link Parent',
      categoryId,
    });
    parentGroupId = parentGroup.ID;

    const childGroup = await apiClient.createGroup({
      name: 'Badge Link Child',
      categoryId,
      ownerId: parentGroupId,
    });
    childGroupId = childGroup.ID;
  });

  test('category badge on sub-group card should link to /groups filtered list', async ({
    page,
  }) => {
    await page.goto(`/group?id=${parentGroupId}`);
    await page.waitForLoadState('load');

    // Find the category badge link on the child group card
    const categoryBadge = page.locator('a.card-badge--category').filter({
      hasText: 'BadgeLinkTestCategory',
    }).first();
    await expect(categoryBadge).toBeVisible({ timeout: 5000 });

    // Get the href — it should point to /groups?categories=X, not /group?...
    const href = await categoryBadge.getAttribute('href');
    expect(href).toContain('/groups');
    expect(href).not.toMatch(/^\/group\?/);
  });

  test.afterAll(async ({ apiClient }) => {
    if (childGroupId) await apiClient.deleteGroup(childGroupId);
    if (parentGroupId) await apiClient.deleteGroup(parentGroupId);
    if (categoryId) await apiClient.deleteCategory(categoryId);
  });
});
