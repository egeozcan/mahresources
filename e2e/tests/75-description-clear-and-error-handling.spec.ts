/**
 * Tests for two description inline edit bugs:
 *
 * Bug 1 (MAJOR): When a user clears a description via inline edit, the
 * description container disappears on reload because the template wraps
 * the entire area in `{% if description %}`. The user can never re-add a
 * description without using the full Edit form.
 *
 * Bug 2 (MINOR): The description inline edit click.away handler only catches
 * network errors via `.catch()`, but HTTP errors (non-2xx) silently close
 * the editor with no feedback.
 */
import { test, expect } from '../fixtures/base.fixture';

test.describe('Description inline edit: clear and error handling', () => {
  let categoryId: number;
  let ownerGroupId: number;
  let noteId: number;

  test.beforeAll(async ({ apiClient }) => {
    const category = await apiClient.createCategory(
      'Desc Clear Test Category',
      'Category for description clear test',
    );
    categoryId = category.ID;

    const ownerGroup = await apiClient.createGroup({
      name: 'Desc Clear Test Owner',
      categoryId,
    });
    ownerGroupId = ownerGroup.ID;

    const note = await apiClient.createNote({
      name: 'Desc Clear Test Note',
      description: 'Initial description that will be cleared',
      ownerId: ownerGroupId,
    });
    noteId = note.ID;
  });

  test('should allow re-editing description after clearing it', async ({
    page,
    apiClient,
  }) => {
    // Navigate to note detail page
    await page.goto(`/note?id=${noteId}`);
    await page.waitForLoadState('load');

    // Find the description area and double-click to edit
    const descriptionArea = page.locator('.description').first();
    await expect(descriptionArea).toBeVisible({ timeout: 5000 });
    await descriptionArea.dblclick();

    // A textarea should appear with the current description
    const textarea = page.locator('textarea[name="description"]');
    await expect(textarea).toBeVisible({ timeout: 3000 });

    // Clear the description
    await textarea.fill('');

    // Wait for the POST request to complete when clicking away
    const savePromise = page.waitForResponse(
      (response) =>
        response.url().includes('/v1/note/editDescription') &&
        response.status() === 200,
    );

    // Click away to trigger save
    await page.locator('h1').first().click();
    await savePromise;
    await page.waitForLoadState('load');

    // Verify via API that the description was cleared
    const noteAfterClear = await apiClient.getNote(noteId);
    expect(noteAfterClear.Description).toBe('');

    // CRITICAL: After reload, the description area should still be visible
    // and editable, even though the description is now empty
    const descriptionAreaAfterClear = page.locator('.description').first();
    await expect(descriptionAreaAfterClear).toBeVisible({ timeout: 5000 });

    // Double-click to enter edit mode again on the empty description
    await descriptionAreaAfterClear.dblclick();

    // Textarea should appear (empty this time)
    const textareaAgain = page.locator('textarea[name="description"]');
    await expect(textareaAgain).toBeVisible({ timeout: 3000 });

    // Add a new description
    await textareaAgain.fill('Re-added description after clearing');

    // Wait for save
    const savePromise2 = page.waitForResponse(
      (response) =>
        response.url().includes('/v1/note/editDescription') &&
        response.status() === 200,
    );

    await page.locator('h1').first().click();
    await savePromise2;
    await page.waitForLoadState('load');

    // Verify the new description was saved
    const noteAfterReAdd = await apiClient.getNote(noteId);
    expect(noteAfterReAdd.Description).toBe('Re-added description after clearing');
  });

  test('should show error feedback when description save fails', async ({
    page,
  }) => {
    // Reset note description for this test
    await page.goto(`/note?id=${noteId}`);
    await page.waitForLoadState('load');

    // Find the description area and double-click to edit
    const descriptionArea = page.locator('.description').first();
    await expect(descriptionArea).toBeVisible({ timeout: 5000 });
    await descriptionArea.dblclick();

    const textarea = page.locator('textarea[name="description"]');
    await expect(textarea).toBeVisible({ timeout: 3000 });

    // Intercept the editDescription request to make it fail
    await page.route('**/v1/note/editDescription**', (route) => {
      route.fulfill({
        status: 500,
        contentType: 'text/plain',
        body: 'Internal Server Error',
      });
    });

    // Modify the description
    await textarea.fill('This should fail to save');

    // Click away to trigger the save attempt
    await page.locator('h1').first().click();

    // The description container should show a red background flash
    // indicating the error (similar to how inline-edit name works)
    const descriptionContainer = page.locator('.description').first();
    await expect(descriptionContainer).toBeVisible({ timeout: 3000 });

    // Check that the red error flash class was applied
    // The description div should have a red-ish background to indicate error
    await expect(descriptionContainer).toHaveCSS('background-color', 'rgb(254, 226, 226)', {
      timeout: 3000,
    });
  });

  test.afterAll(async ({ apiClient }) => {
    if (noteId) await apiClient.deleteNote(noteId);
    if (ownerGroupId) await apiClient.deleteGroup(ownerGroupId);
    if (categoryId) await apiClient.deleteCategory(categoryId);
  });
});
