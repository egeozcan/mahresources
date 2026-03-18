/**
 * Tests that double-click inline editing of a note's description on the
 * notes list page actually saves the changes.
 *
 * Bug: The note card template passes descriptionEditUrl="/blabla" (a dummy
 * placeholder) and the click-away handler doesn't submit the form, so all
 * edits are silently discarded.
 */
import { test, expect } from '../fixtures/base.fixture';

test.describe('Note description inline edit on list page', () => {
  let categoryId: number;
  let ownerGroupId: number;
  let noteId: number;

  test.beforeAll(async ({ apiClient }) => {
    const category = await apiClient.createCategory(
      'Desc Edit Test Category',
      'Category for description inline edit test',
    );
    categoryId = category.ID;

    const ownerGroup = await apiClient.createGroup({
      name: 'Desc Edit Test Owner',
      categoryId,
    });
    ownerGroupId = ownerGroup.ID;

    const note = await apiClient.createNote({
      name: 'Desc Edit Test Note',
      description: 'Original description text',
      ownerId: ownerGroupId,
    });
    noteId = note.ID;
  });

  test('double-click editing description should save changes', async ({
    page,
    apiClient,
  }) => {
    // Navigate to notes list
    await page.goto('/notes');
    await page.waitForLoadState('load');

    // Find the description area on the note card and double-click to edit
    const descriptionArea = page.locator('.card-description .description').first();
    await expect(descriptionArea).toBeVisible({ timeout: 5000 });
    await descriptionArea.dblclick();

    // A textarea should appear with the current description
    const textarea = page.locator('.card-description textarea[name="description"]');
    await expect(textarea).toBeVisible({ timeout: 3000 });

    // Clear and type new description
    await textarea.fill('Updated description via inline edit');

    // Click away to trigger save
    await page.locator('h1').first().click();

    // Wait for the save request to complete
    await page.waitForTimeout(1000);

    // Verify via API that the description was actually saved
    const note = await apiClient.getNote(noteId);
    expect(note.Description).toBe('Updated description via inline edit');
  });

  test.afterAll(async ({ apiClient }) => {
    if (noteId) await apiClient.deleteNote(noteId);
    if (ownerGroupId) await apiClient.deleteGroup(ownerGroupId);
    if (categoryId) await apiClient.deleteCategory(categoryId);
  });
});
