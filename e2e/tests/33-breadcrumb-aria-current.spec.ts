/**
 * Bug: Breadcrumb last entry (current page) lacks aria-current="page"
 *
 * WAI-ARIA Authoring Practices for breadcrumbs specify that the last link
 * in a breadcrumb trail (representing the current page) MUST have
 * aria-current="page". The template renders every breadcrumb entry as a
 * plain <a> without aria-current, violating WCAG 2.1 Level AA.
 *
 * Affected template: templates/partials/breadcrumb.tpl
 *
 * Reproduction:
 *   1. Create a parent group and a child group (child owned by parent)
 *   2. Navigate to the child group detail page
 *   3. The breadcrumb renders: Home > ParentGroup > ChildGroup
 *   4. The last link (ChildGroup) should have aria-current="page" but does not
 */
import { test, expect } from '../fixtures/base.fixture';

test.describe('Breadcrumb aria-current="page"', () => {
  let categoryId: number;
  let parentGroupId: number;
  let childGroupId: number;
  const testRunId = Date.now() + Math.floor(Math.random() * 100000);

  test.beforeAll(async ({ apiClient }) => {
    // Create a category for the groups
    const category = await apiClient.createCategory(
      `Breadcrumb Test Category ${testRunId}`,
      'For breadcrumb ARIA test'
    );
    categoryId = category.ID;

    // Create a parent group
    const parentGroup = await apiClient.createGroup({
      name: `Parent Group ${testRunId}`,
      categoryId: category.ID,
    });
    parentGroupId = parentGroup.ID;

    // Create a child group owned by the parent
    const childGroup = await apiClient.createGroup({
      name: `Child Group ${testRunId}`,
      categoryId: category.ID,
      ownerId: parentGroup.ID,
    });
    childGroupId = childGroup.ID;
  });

  test('last breadcrumb link should have aria-current="page"', async ({ page }) => {
    // Navigate to the child group page, which will show a breadcrumb:
    // Groups (home) > Parent Group > Child Group
    await page.goto(`/group?id=${childGroupId}`);
    await page.waitForLoadState('load');

    // The breadcrumb nav should exist
    const breadcrumbNav = page.locator('nav[aria-label="Breadcrumb"]');
    await expect(breadcrumbNav).toBeVisible();

    // Get all breadcrumb links (excluding the home link)
    const breadcrumbLinks = breadcrumbNav.locator('ol li a');
    const linkCount = await breadcrumbLinks.count();

    // Should have at least: home link + parent group + child group = 3
    expect(linkCount).toBeGreaterThanOrEqual(3);

    // The last link should represent the current page and have aria-current="page"
    const lastLink = breadcrumbLinks.last();
    await expect(lastLink).toHaveAttribute('aria-current', 'page');
  });

  test.afterAll(async ({ apiClient }) => {
    // Cleanup in reverse dependency order
    try { await apiClient.deleteGroup(childGroupId); } catch { /* ignore */ }
    try { await apiClient.deleteGroup(parentGroupId); } catch { /* ignore */ }
    try { await apiClient.deleteCategory(categoryId); } catch { /* ignore */ }
  });
});
