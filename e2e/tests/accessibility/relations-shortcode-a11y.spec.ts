/**
 * Accessibility of [relations]: nested native lists whose type labels are
 * plain text announced before their list, and whose links are named by the
 * far group. axe (WCAG 2.x A/AA, which includes the list, listitem,
 * link-name and color-contrast rules) runs on every surface a card or slot
 * puts them: the group detail page, the group list page (CustomSummary) and
 * the MRQL results page (CustomMRQLResult).
 */
import { test, expect } from '../../fixtures/a11y.fixture';

test.describe('[relations] shortcode accessibility', () => {
  const stamp = Date.now();
  let categoryId: number;
  const groupIds: number[] = [];
  const typeIds: number[] = [];
  let carrierId: number;
  let carrierName: string;

  test.beforeAll(async ({ apiClient }) => {
    const category = await apiClient.createCategory(`Rel A11y ${stamp}`, 'relations a11y', {
      CustomSidebar: '[relations][relations direction="in"]',
      CustomSummary: '[relations limit="3"]',
      CustomMRQLResult: '<p>[property path="Name"]</p>[relations limit="3"]',
      SectionConfig: JSON.stringify({ relations: { state: 'off' } }),
    });
    categoryId = category.ID;
    const group = async (name: string) => {
      const g = await apiClient.createGroup({ name, categoryId });
      groupIds.push(g.ID);
      return g;
    };
    carrierName = `Rel A11y Carrier ${stamp}`;
    const carrier = await group(carrierName);
    carrierId = carrier.ID;
    const a = await group(`Rel A11y Dep A ${stamp}`);
    const b = await group(`Rel A11y Dep B ${stamp}`);
    const depends = await apiClient.createRelationType({ name: 'Depends on', fromCategoryId: categoryId, toCategoryId: categoryId });
    const blocks = await apiClient.createRelationType({ name: 'Blocks', fromCategoryId: categoryId, toCategoryId: categoryId });
    typeIds.push(depends.ID, blocks.ID);
    await apiClient.createRelation({ fromGroupId: carrier.ID, toGroupId: a.ID, relationTypeId: depends.ID });
    await apiClient.createRelation({ fromGroupId: carrier.ID, toGroupId: b.ID, relationTypeId: blocks.ID });
    await apiClient.createRelation({ fromGroupId: b.ID, toGroupId: carrier.ID, relationTypeId: blocks.ID });
  });

  test.afterAll(async ({ apiClient }) => {
    for (const id of groupIds) await apiClient.deleteGroup(id).catch(() => {});
    for (const id of typeIds) await apiClient.deleteRelationType(id).catch(() => {});
    if (categoryId) await apiClient.deleteCategory(categoryId).catch(() => {});
  });

  test('group detail page with [relations] has no a11y violations', async ({ page, checkA11y }) => {
    await page.goto(`/group?id=${carrierId}`);
    await page.waitForLoadState('load');
    const lists = page.locator('.relations-shortcode');
    await expect(lists).toHaveCount(2);
    await expect(page.locator('.relations-shortcode__label')).toHaveText(['Blocks', 'Depends on', 'Incoming: Blocks']);
    // Each type is a list item holding its label and a native list.
    const typeItem = page.locator('.relations-shortcode__type').first();
    await expect(typeItem).toHaveRole('listitem');
    await expect(typeItem.getByRole('list')).toHaveCount(1);
    await expect(typeItem.getByRole('link', { name: `Rel A11y Dep B ${stamp}` })).toBeVisible();
    // Scanned before any link is focused: focusing an entity link opens its
    // hover card, which has its own scan.
    await checkA11y();
  });

  test('group list cards with [relations] have no a11y violations', async ({ page, checkA11y }) => {
    await page.goto(`/groups?categories=${categoryId}`);
    await page.waitForLoadState('load');
    await expect(page.locator('.relations-shortcode').first()).toBeVisible();
    await checkA11y();
  });

  test('MRQL results with [relations] cards have no a11y violations', async ({ page, checkA11y }) => {
    await page.goto(`/mrql?q=${encodeURIComponent(`type = group AND name = "${carrierName}"`)}`);
    const results = page.locator('[aria-label="Query results"]');
    await expect(results.locator('.relations-shortcode')).toHaveCount(1, { timeout: 20_000 });
    await expect(results.locator('.relations-shortcode__label')).toHaveText(['Blocks', 'Depends on']);
    await checkA11y();
  });
});
