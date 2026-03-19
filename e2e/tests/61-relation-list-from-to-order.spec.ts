import { test, expect } from '../fixtures/base.fixture';

/**
 * Bug: On the relations list page, the "from" and "to" groups are displayed
 * in the wrong order. The left card shows the ToGroup and the right card
 * shows the FromGroup, when it should be the opposite.
 *
 * Root cause: In listRelations.tpl, the left partial (relation.tpl) renders
 * entity.ToGroup, and the right partial (relation_reverse.tpl) renders
 * entity.FromGroup. These partials were designed for the group detail page
 * (where they show the "other" group), but on the relations list page they
 * result in swapped from/to display.
 */
test.describe('Relation list page from/to group order', () => {
  let categoryId: number;
  let fromGroupId: number;
  let toGroupId: number;
  let relationTypeId: number;
  let relationId: number;
  const testRunId = Date.now();

  const fromGroupName = `SourceGroup ${testRunId}`;
  const toGroupName = `TargetGroup ${testRunId}`;

  test.beforeAll(async ({ apiClient }) => {
    const category = await apiClient.createCategory(
      `RelListCat ${testRunId}`,
      'Category for relation list order test'
    );
    categoryId = category.ID;

    const fromGroup = await apiClient.createGroup({
      name: fromGroupName,
      description: 'The FROM group',
      categoryId,
    });
    fromGroupId = fromGroup.ID;

    const toGroup = await apiClient.createGroup({
      name: toGroupName,
      description: 'The TO group',
      categoryId,
    });
    toGroupId = toGroup.ID;

    const relationType = await apiClient.createRelationType({
      name: `PointsTo ${testRunId}`,
      fromCategoryId: categoryId,
      toCategoryId: categoryId,
    });
    relationTypeId = relationType.ID;

    const relation = await apiClient.createRelation({
      name: `TestRel ${testRunId}`,
      fromGroupId,
      toGroupId,
      relationTypeId,
    });
    relationId = relation.ID;
  });

  test('from group should appear on the left and to group on the right', async ({
    page,
  }) => {
    // Navigate to the relations list page
    await page.goto('/relations');
    await page.waitForLoadState('load');

    // Find the relation card
    const relationCard = page.locator('article.relation-card', {
      hasText: `TestRel ${testRunId}`,
    });
    await expect(relationCard).toBeVisible();

    // The relation card has two group sub-articles inside .relation-groups
    // The first article (left side) should show the FROM group (SourceGroup)
    // The second article (right side) should show the TO group (TargetGroup)
    const groupArticles = relationCard.locator('.relation-groups article');
    await expect(groupArticles).toHaveCount(2);

    const leftGroup = groupArticles.nth(0);
    const rightGroup = groupArticles.nth(1);

    // The FROM group (SourceGroup) should be on the LEFT
    await expect(leftGroup).toContainText(fromGroupName);
    // The TO group (TargetGroup) should be on the RIGHT
    await expect(rightGroup).toContainText(toGroupName);
  });

  test.afterAll(async ({ apiClient }) => {
    try { await apiClient.deleteRelation(relationId); } catch { /* ignore */ }
    try { await apiClient.deleteGroup(fromGroupId); } catch { /* ignore */ }
    try { await apiClient.deleteGroup(toGroupId); } catch { /* ignore */ }
    try { await apiClient.deleteRelationType(relationTypeId); } catch { /* ignore */ }
    try { await apiClient.deleteCategory(categoryId); } catch { /* ignore */ }
  });
});
