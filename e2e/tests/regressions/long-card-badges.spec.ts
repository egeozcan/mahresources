import { test, expect } from '../../fixtures/base.fixture';

test('a long category badge stays inside its card instead of covering sidebar controls', async ({ page, apiClient }) => {
  const prefix = `Long badge ${Date.now()}`;
  const name = prefix + ' ' + 'unbreakable-category-name'.repeat(20);
  const category = await apiClient.createCategory(name);
  const group = await apiClient.createGroup({ name: prefix, categoryId: category.ID });
  try {
    await page.goto(`/groups?Name=${encodeURIComponent(prefix)}`);
    const badge = page.locator('.card-badge--category').filter({ hasText: name });
    await expect(badge).toBeVisible();
    const main = await page.getByRole('main').boundingBox();
    expect(main).not.toBeNull();
    const geometry = await badge.evaluate(element => ({
      right: element.getBoundingClientRect().right,
      scroll: element.scrollWidth,
      client: element.clientWidth,
    }));
    expect(geometry.right, 'badge must not paint over the filter sidebar').toBeLessThanOrEqual(main!.x + main!.width);
    expect(geometry.scroll, 'text must wrap, not escape a width-constrained badge').toBeLessThanOrEqual(geometry.client);
  } finally {
    await apiClient.deleteGroup(group.ID);
    await apiClient.deleteCategory(category.ID);
  }
});
