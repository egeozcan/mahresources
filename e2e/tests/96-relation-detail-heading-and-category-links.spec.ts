/**
 * Tests that the relation detail page:
 * 1. Uses properly matched h3 tags for "From Group" and "To Group" headings
 *    (not h3 opened / h4 closed)
 * 2. Category badge links point to /groups?categories=X, not
 *    /relation?categories=X&id=Y
 */
import { test, expect } from '../fixtures/base.fixture';

test.describe('Relation detail page headings and category badge links', () => {
  let categoryId: number;
  let group1Id: number;
  let group2Id: number;
  let relationTypeId: number;
  let relationId: number;
  const testRunId = Date.now();

  test.beforeAll(async ({ apiClient }) => {
    const category = await apiClient.createCategory(
      `RelDetailCat ${testRunId}`,
      'Category for relation detail tests',
    );
    categoryId = category.ID;

    const group1 = await apiClient.createGroup({
      name: `RelDetailFrom ${testRunId}`,
      description: 'From group',
      categoryId,
    });
    group1Id = group1.ID;

    const group2 = await apiClient.createGroup({
      name: `RelDetailTo ${testRunId}`,
      description: 'To group',
      categoryId,
    });
    group2Id = group2.ID;

    const relationType = await apiClient.createRelationType({
      name: `RelDetailType ${testRunId}`,
      description: 'Relation type for detail test',
      fromCategoryId: categoryId,
      toCategoryId: categoryId,
    });
    relationTypeId = relationType.ID;

    const relation = await apiClient.createRelation({
      name: `RelDetail ${testRunId}`,
      fromGroupId: group1Id,
      toGroupId: group2Id,
      relationTypeId,
    });
    relationId = relation.ID;
  });

  test('h3 headings for From Group and To Group should be properly closed', async ({
    page,
    request,
    baseURL,
  }) => {
    // Check the raw HTML source for mismatched h3/h4 tags.
    // Browsers auto-correct broken HTML, so DOM checks alone won't catch
    // a <h3>...</h4> mismatch. Fetch the raw response and inspect it.
    const response = await request.get(`${baseURL}/relation?id=${relationId}`);
    const html = await response.text();

    // The raw HTML should NOT contain <h3 ... >...</h4>
    expect(html).not.toMatch(/<h3[^>]*>.*?<\/h4>/s);

    // It should contain properly matched <h3>...</h3> for the group titles
    expect(html).toMatch(/<h3 class="sidebar-group-title">From Group<\/h3>/);
    expect(html).toMatch(/<h3 class="sidebar-group-title">To Group<\/h3>/);

    // Also verify via the DOM that the headings render correctly
    await page.goto(`/relation?id=${relationId}`);
    await page.waitForLoadState('load');

    const headings = page.locator('h3.sidebar-group-title');
    await expect(headings).toHaveCount(2);
    await expect(headings.nth(0)).toContainText('From Group');
    await expect(headings.nth(1)).toContainText('To Group');
  });

  test('category badge links should point to /groups, not /relation', async ({
    page,
  }) => {
    await page.goto(`/relation?id=${relationId}`);
    await page.waitForLoadState('load');

    // Find category badge links within the group cards
    const categoryBadges = page.locator('a.card-badge--category');
    const count = await categoryBadges.count();
    expect(count).toBeGreaterThanOrEqual(2);

    for (let i = 0; i < count; i++) {
      const href = await categoryBadges.nth(i).getAttribute('href');
      // Should link to /groups?categories=X
      expect(href).toContain('/groups');
      expect(href).toContain(`categories=${categoryId}`);
      // Should NOT link to /relation?... (the current page)
      expect(href).not.toMatch(/^\/relation\?/);
    }
  });

  test.afterAll(async ({ apiClient }) => {
    if (relationId) {
      try { await apiClient.deleteRelation(relationId); } catch { /* ignore */ }
    }
    if (group1Id) {
      try { await apiClient.deleteGroup(group1Id); } catch { /* ignore */ }
    }
    if (group2Id) {
      try { await apiClient.deleteGroup(group2Id); } catch { /* ignore */ }
    }
    if (relationTypeId) {
      try { await apiClient.deleteRelationType(relationTypeId); } catch { /* ignore */ }
    }
    if (categoryId) {
      try { await apiClient.deleteCategory(categoryId); } catch { /* ignore */ }
    }
  });
});
