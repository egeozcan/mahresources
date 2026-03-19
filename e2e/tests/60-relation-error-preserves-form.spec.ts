import { test, expect } from '../fixtures/base.fixture';

/**
 * Bug: When creating a relation fails (e.g. category mismatch), the server
 * redirects back to /relation/new?...&Error=category+mismatch. The template
 * renderer sees the `errorMessage` context variable (set from the Error query
 * param) and renders error.tpl instead of createRelation.tpl. This causes the
 * entire form to disappear — the user sees the error message but cannot fix
 * their input and retry.
 *
 * Expected: The form should remain visible with the error message displayed,
 * and the previously-selected values (Type, From Group, To Group) should be
 * pre-populated so the user can correct and resubmit.
 */
test.describe('Relation creation error preserves form', () => {
  let categoryAId: number;
  let categoryBId: number;
  let groupAId: number;
  let groupBId: number;
  let relationTypeId: number;

  test.beforeAll(async ({ apiClient }) => {
    const runId = Date.now();

    // Create two DIFFERENT categories
    const catA = await apiClient.createCategory(
      `CatA-${runId}`,
      'Category A for mismatch test'
    );
    categoryAId = catA.ID;

    const catB = await apiClient.createCategory(
      `CatB-${runId}`,
      'Category B for mismatch test'
    );
    categoryBId = catB.ID;

    // Create a relation type that expects FromCategory=CatA, ToCategory=CatB
    const relType = await apiClient.createRelationType({
      name: `TypeAB-${runId}`,
      description: 'Expects A->B',
      fromCategoryId: categoryAId,
      toCategoryId: categoryBId,
    });
    relationTypeId = relType.ID;

    // Create Group A in CatA
    const gA = await apiClient.createGroup({
      name: `GroupA-${runId}`,
      description: 'In category A',
      categoryId: categoryAId,
    });
    groupAId = gA.ID;

    // Create Group B also in CatA (wrong for "To" side — should be CatB)
    const gB = await apiClient.createGroup({
      name: `GroupB-${runId}`,
      description: 'In category A (wrong for To side)',
      categoryId: categoryAId,
    });
    groupBId = gB.ID;
  });

  test('form should remain visible after relation creation error', async ({
    page,
  }) => {
    // Navigate directly to the error-redirect URL that the server produces
    // when relation creation fails with a category mismatch
    await page.goto(
      `/relation/new?FromGroupId=${groupAId}&ToGroupId=${groupBId}&GroupRelationTypeId=${relationTypeId}&Error=category+mismatch`
    );
    await page.waitForLoadState('load');

    // The error message should be visible
    await expect(page.locator('text=category mismatch')).toBeVisible();

    // BUG: The form fields should still be visible so the user can fix and retry.
    // Currently, the renderer switches to error.tpl and the form disappears entirely.
    const formElement = page.locator('form');
    await expect(formElement).toBeVisible();

    // The save/submit button should still be present
    await expect(
      page.locator('button[type="submit"]:has-text("Save")')
    ).toBeVisible();
  });

  test.afterAll(async ({ apiClient }) => {
    try { await apiClient.deleteGroup(groupAId); } catch { /* ignore */ }
    try { await apiClient.deleteGroup(groupBId); } catch { /* ignore */ }
    try { await apiClient.deleteRelationType(relationTypeId); } catch { /* ignore */ }
    try { await apiClient.deleteCategory(categoryAId); } catch { /* ignore */ }
    try { await apiClient.deleteCategory(categoryBId); } catch { /* ignore */ }
  });
});
