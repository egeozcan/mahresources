/**
 * Tests that the Text view preserves multi-sort criteria when filters
 * are applied, instead of silently dropping all but the first sort.
 *
 * Bug: The Text view used selectInput.tpl (single-value dropdown) for
 * sorting, which could only hold one sort criterion. Switching from
 * List view (multi-sort) to Text view and applying filters silently
 * dropped additional sort criteria.
 */
import { test, expect } from '../fixtures/base.fixture';

test.describe('Text view preserves multi-sort criteria', () => {
  let categoryId: number;

  test.beforeAll(async ({ apiClient }) => {
    const category = await apiClient.createCategory(
      'Multi Sort Test Category',
      'For multi-sort text view test',
    );
    categoryId = category.ID;

    // Create a few groups so there's data to sort
    for (let i = 0; i < 3; i++) {
      await apiClient.createGroup({
        name: `Sort Test Group ${i}`,
        categoryId,
      });
    }
  });

  test('navigating to text view with multi-sort should preserve all sort params after form submit', async ({
    page,
  }) => {
    // Navigate to text view with two sort criteria in URL
    await page.goto('/groups/text?SortBy=name+desc&SortBy=created_at+asc');
    await page.waitForLoadState('load');

    // Click "Apply Filters" button
    await page.locator('button[type="submit"]').first().click();
    await page.waitForLoadState('load');

    // Check the URL — both SortBy params should be preserved
    const url = new URL(page.url());
    const sortParams = url.searchParams.getAll('SortBy');

    expect(sortParams.length).toBeGreaterThanOrEqual(2);
    expect(sortParams).toContain('name desc');
    expect(sortParams).toContain('created_at asc');
  });

  test.afterAll(async ({ apiClient }) => {
    const groups = await apiClient.getGroups();
    for (const g of groups) {
      if (g.Name.startsWith('Sort Test Group')) {
        await apiClient.deleteGroup(g.ID);
      }
    }
    if (categoryId) await apiClient.deleteCategory(categoryId);
  });
});
