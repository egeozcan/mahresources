/**
 * Tests that inline description editing on a list page saves via AJAX
 * and stays on the list page, instead of navigating to the detail page.
 *
 * Bug: The description template uses $refs.form.submit() which does a full
 * page POST, causing navigation to the entity detail page. Should use
 * fetch() to save in-place.
 */
import { test, expect } from '../fixtures/base.fixture';

test.describe('Description inline edit stays on list page', () => {
  let categoryId: number;
  let ownerGroupId: number;
  let noteId: number;

  test.beforeAll(async ({ apiClient }) => {
    const category = await apiClient.createCategory(
      'Desc Stay List Category',
      'For description stay-on-list test',
    );
    categoryId = category.ID;

    const ownerGroup = await apiClient.createGroup({
      name: 'Desc Stay List Owner',
      categoryId,
    });
    ownerGroupId = ownerGroup.ID;

    const note = await apiClient.createNote({
      name: 'Desc Stay List Note',
      description: 'Original list description',
      ownerId: ownerGroupId,
    });
    noteId = note.ID;
  });

  test('editing description on notes list should stay on /notes', async ({
    page,
    apiClient,
  }) => {
    await page.goto('/notes');
    await page.waitForLoadState('load');

    // Find the description area and double-click to edit
    const descriptionArea = page.locator('.card-description .description').first();
    await expect(descriptionArea).toBeVisible({ timeout: 5000 });
    await descriptionArea.dblclick();

    // Textarea should appear
    const textarea = page.locator('.card-description textarea[name="description"]');
    await expect(textarea).toBeVisible({ timeout: 3000 });

    // Modify the text
    await textarea.fill('Updated from list page');

    // Click away to trigger save
    await page.locator('h1').first().click();

    // Wait for any navigation or AJAX to complete
    await page.waitForTimeout(1500);

    // Should still be on the notes list page
    expect(page.url()).toContain('/notes');
    expect(page.url()).not.toContain('/note?id=');

    // Verify the description was actually saved via API
    const note = await apiClient.getNote(noteId);
    expect(note.Description).toBe('Updated from list page');
  });

  test.afterAll(async ({ apiClient }) => {
    if (noteId) await apiClient.deleteNote(noteId);
    if (ownerGroupId) await apiClient.deleteGroup(ownerGroupId);
    if (categoryId) await apiClient.deleteCategory(categoryId);
  });
});
