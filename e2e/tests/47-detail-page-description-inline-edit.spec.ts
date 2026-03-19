/**
 * Tests that double-click inline editing of a note's description works
 * on the note DETAIL page (not just the list page).
 *
 * Bug: The display templates don't pass descriptionEditUrl to the
 * description partial, so the double-click handler evaluates !!'' as
 * false and the inline edit never activates.
 */
import { test, expect } from '../fixtures/base.fixture';

test.describe('Description inline edit works on detail pages', () => {
  let categoryId: number;
  let ownerGroupId: number;
  let noteId: number;

  test.beforeAll(async ({ apiClient }) => {
    const category = await apiClient.createCategory(
      'Detail Desc Edit Category',
      'For detail page description edit test',
    );
    categoryId = category.ID;

    const ownerGroup = await apiClient.createGroup({
      name: 'Detail Desc Edit Owner',
      categoryId,
    });
    ownerGroupId = ownerGroup.ID;

    const note = await apiClient.createNote({
      name: 'Detail Desc Edit Note',
      description: 'Original detail description',
      ownerId: ownerGroupId,
    });
    noteId = note.ID;
  });

  test('double-clicking description on note detail page should open editor', async ({
    page,
  }) => {
    await page.goto(`/note?id=${noteId}`);
    await page.waitForLoadState('load');

    // Find the description area
    const descriptionArea = page.locator('.description').first();
    await expect(descriptionArea).toBeVisible({ timeout: 5000 });

    // Double-click to enter edit mode
    await descriptionArea.dblclick();

    // A textarea should appear for editing
    const textarea = page.locator('textarea[name="description"]');
    await expect(textarea).toBeVisible({ timeout: 3000 });
  });

  test.afterAll(async ({ apiClient }) => {
    if (noteId) await apiClient.deleteNote(noteId);
    if (ownerGroupId) await apiClient.deleteGroup(ownerGroupId);
    if (categoryId) await apiClient.deleteCategory(categoryId);
  });
});
