/**
 * Tests that the relation detail page provides an inline-edit widget in the
 * page title, enabling users to rename the relation directly from the
 * display page.
 *
 * Bug: RelationContextProvider sets mainEntity but not mainEntityType, so
 * title.tpl renders a plain text span instead of the <inline-edit> web
 * component.
 *
 * Fix: Add "mainEntityType": "relation" to the context in
 * RelationContextProvider, matching the pattern used by all other entity
 * context providers.
 */
import { test, expect } from '../fixtures/base.fixture';

test.describe('Relation detail page inline edit', () => {
  let categoryId: number;
  let group1Id: number;
  let group2Id: number;
  let relationTypeId: number;
  let relationId: number;
  const testRunId = Date.now();

  test.beforeAll(async ({ apiClient }) => {
    const category = await apiClient.createCategory(
      `RelInlineEditCat ${testRunId}`,
      'Category for relation inline edit test',
    );
    categoryId = category.ID;

    const group1 = await apiClient.createGroup({
      name: `RelInlineFrom ${testRunId}`,
      categoryId,
    });
    group1Id = group1.ID;

    const group2 = await apiClient.createGroup({
      name: `RelInlineTo ${testRunId}`,
      categoryId,
    });
    group2Id = group2.ID;

    const relationType = await apiClient.createRelationType({
      name: `RelInlineRT ${testRunId}`,
      description: 'Type for inline edit test',
      fromCategoryId: categoryId,
      toCategoryId: categoryId,
    });
    relationTypeId = relationType.ID;
  });

  test('relation detail page should have inline-edit in title', async ({
    relationPage,
    page,
  }) => {
    // Create a relation via the UI
    relationId = await relationPage.create({
      name: `RelInlineTest ${testRunId}`,
      description: 'Relation for inline edit test',
      relationTypeName: `RelInlineRT ${testRunId}`,
      fromGroupName: `RelInlineFrom ${testRunId}`,
      toGroupName: `RelInlineTo ${testRunId}`,
    });

    // Navigate to the relation detail page
    await relationPage.gotoDisplay(relationId);

    // The title section should contain an inline-edit web component
    const titleSection = page.locator('.title');
    await expect(titleSection).toBeVisible();

    const inlineEdit = titleSection.locator('inline-edit');
    await expect(inlineEdit).toBeVisible({ timeout: 5000 });

    // Verify the inline-edit has the correct post URL
    const postUrl = await inlineEdit.getAttribute('post');
    expect(postUrl).toContain('/v1/relation/editName');
    expect(postUrl).toContain(`id=${relationId}`);
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
