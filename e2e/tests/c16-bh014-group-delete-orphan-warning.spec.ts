/**
 * BH-014: deleting a parent group silently orphans its children.
 *
 * Fix: bulk-delete form uses confirmGroupDelete which fetches each
 * selected group's child/note/resource counts, aggregates them, and
 * shows "Delete N groups? This will orphan X child groups and M
 * notes/resources (they'll move to top level)."
 */
import { test, expect } from '../fixtures/base.fixture';

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

    // Observe the confirm() invocation
    const confirmMessages: string[] = [];
    page.on('dialog', async (dialog) => {
      confirmMessages.push(dialog.message());
      await dialog.dismiss(); // Cancel — we're only checking the message
    });

    // Filter the groups list to a single group we know (unique name)
    await page.goto(`/groups?name=${encodeURIComponent(`BH014 Parent ${testRunId}`)}`);
    await page.waitForLoadState('load');

    await groupPage.selectGroupCheckbox(parent.ID);

    // Open the Delete editor (toggle button is injected by bulkSelectionForms)
    await page.getByRole('button', { name: 'Toggle Delete editor' }).click();
    // Click the Delete submit button inside the bulk-delete form
    await page.locator('form[action*="groups/delete"] button[type="submit"]').click();

    // Give the async count-fetch a moment to resolve and fire confirm()
    await expect.poll(() => confirmMessages.length, { timeout: 5000 }).toBeGreaterThan(0);
    const msg = confirmMessages[0];
    expect(msg).toMatch(/2\s*child group/i);
    expect(msg).toMatch(/1\s*note/i);
    expect(msg).toMatch(/orphan|top level/i);
  });

  test('leaf-only selection shows a simple confirm without orphan language', async ({ page, groupPage, apiClient }) => {
    const leaf = await apiClient.createGroup({
      name: `BH014 Leaf ${testRunId}`,
      categoryId,
    });

    const confirmMessages: string[] = [];
    page.on('dialog', async (dialog) => {
      confirmMessages.push(dialog.message());
      await dialog.dismiss();
    });

    await page.goto(`/groups?name=${encodeURIComponent(`BH014 Leaf ${testRunId}`)}`);
    await page.waitForLoadState('load');

    await groupPage.selectGroupCheckbox(leaf.ID);

    await page.getByRole('button', { name: 'Toggle Delete editor' }).click();
    await page.locator('form[action*="groups/delete"] button[type="submit"]').click();

    await expect.poll(() => confirmMessages.length, { timeout: 5000 }).toBeGreaterThan(0);
    // Leaf group: no children/items → dialog should NOT mention orphaning
    expect(confirmMessages[0]).not.toMatch(/orphan|child group/i);
  });
});
