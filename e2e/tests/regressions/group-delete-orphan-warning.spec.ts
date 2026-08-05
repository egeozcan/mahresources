/**
 * BH-014: deleting a parent group silently orphans its children.
 *
 * Fix: bulk-delete form uses confirmGroupDelete which fetches each
 * selected group's child/note/resource counts, aggregates them, and
 * shows "Delete N groups? This will orphan X child groups and M
 * notes/resources (they'll move to top level)."
 */
import { test, expect } from '../../fixtures/base.fixture';
import { dismissConfirm } from '../../helpers/confirm-dialog';

test.describe('BH-014: group delete orphan-warning dialog', () => {
  let categoryId: number;
  const testRunId = `${Date.now()}-${Math.random().toString(36).substring(2, 8)}`;

  test.beforeAll(async ({ apiClient }) => {
    const category = await apiClient.createCategory(
      `BH014 Category ${testRunId}`,
      'Category for BH-014 orphan-warning test'
    );
    categoryId = category.ID;
  });

  test('parent with 2 children + 1 note shows counts in the confirm', async ({ page, groupPage, apiClient }) => {
    const parent = await apiClient.createGroup({
      name: `BH014 Parent ${testRunId}`,
      categoryId,
    });
    await apiClient.createGroup({
      name: `BH014 Child1 ${testRunId}`,
      categoryId,
      ownerId: parent.ID,
    });
    await apiClient.createGroup({
      name: `BH014 Child2 ${testRunId}`,
      categoryId,
      ownerId: parent.ID,
    });
    await apiClient.createNote({
      name: `BH014 Note ${testRunId}`,
      ownerId: parent.ID,
    });

    // Filter the groups list to a single group we know (unique name)
    await page.goto(`/groups?name=${encodeURIComponent(`BH014 Parent ${testRunId}`)}`);
    await page.waitForLoadState('load');

    await groupPage.selectGroupCheckbox(parent.ID);

    // Open the Delete editor (toggle button is injected by bulkSelectionForms)
    await page.getByRole('button', { name: 'Toggle Delete editor' }).click();
    // Click the Delete submit button inside the bulk-delete form. The click only
    // starts confirmGroupDelete's per-group count fetch; the dialog opens once
    // those resolve, so dismissConfirm's wait is what gives the fetch its time.
    await page.locator('form[action*="groups/delete"] button[type="submit"]').click();

    // Cancel — we're only checking the message.
    const msg = await dismissConfirm(page);
    expect(msg).toMatch(/2\s*child group/i);
    expect(msg).toMatch(/1\s*note/i);
    expect(msg).toMatch(/orphan|top level/i);
  });

  test('leaf-only selection shows a simple confirm without orphan language', async ({ page, groupPage, apiClient }) => {
    const leaf = await apiClient.createGroup({
      name: `BH014 Leaf ${testRunId}`,
      categoryId,
    });

    await page.goto(`/groups?name=${encodeURIComponent(`BH014 Leaf ${testRunId}`)}`);
    await page.waitForLoadState('load');

    await groupPage.selectGroupCheckbox(leaf.ID);

    await page.getByRole('button', { name: 'Toggle Delete editor' }).click();
    await page.locator('form[action*="groups/delete"] button[type="submit"]').click();

    const msg = await dismissConfirm(page);
    // Leaf group: no children/items → dialog should NOT mention orphaning
    expect(msg).not.toMatch(/orphan|child group/i);
  });
});
