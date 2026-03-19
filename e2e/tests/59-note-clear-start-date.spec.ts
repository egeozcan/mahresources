/**
 * Tests that clearing the Start Date from a note via the edit form actually
 * removes the StartDate in the database.
 *
 * Bug: When editing a note and clearing the Start Date field (datetime-local
 * input), then saving, the StartDate is NOT cleared. The form submits with
 * an empty startDate field, but the backend interprets the empty string as
 * "not provided" and re-populates it from the existing record. This makes
 * it impossible to remove a Start Date (or End Date) once set.
 *
 * Root cause: In note_api_handlers.go, the partial-update handler checks:
 *   if queryVars.StartDate == "" && existing.StartDate != nil {
 *       queryVars.StartDate = existing.StartDate.Format(...)
 *   }
 * Unlike NoteTypeId and OwnerId (which were fixed in commit 2760a70 to
 * check formHasField()), StartDate and EndDate still use the old pattern
 * that cannot distinguish "field not submitted" from "field intentionally
 * cleared".
 *
 * Steps to reproduce:
 * 1. Create a note with a Start Date set
 * 2. Go to the note edit page
 * 3. Clear the Start Date field (set it to empty)
 * 4. Click Save
 * 5. Observe: the Start Date is still set to the old value
 *
 * Expected: After clearing Start Date and saving, the note should have
 * no Start Date (null/empty).
 *
 * Actual: The Start Date persists with its old value; it cannot be cleared.
 */
import { test, expect } from '../fixtures/base.fixture';

test.describe('Note edit clears start date', () => {
  let noteId: number;

  test.beforeAll(async ({ apiClient }) => {
    // Create a note WITH a start date
    const note = await apiClient.createNote({
      name: 'NoteWithStartDate',
      description: 'Note that has a start date assigned',
      startDate: '2025-06-15T14:30',
    });
    noteId = note.ID;
  });

  test('clearing start date via edit form removes StartDate', async ({
    page,
    apiClient,
  }) => {
    // Verify the start date is set before editing
    const beforeEdit = await apiClient.getNote(noteId);
    expect(beforeEdit.StartDate).toBeTruthy();

    // Navigate to the edit form
    await page.goto(`/note/edit?id=${noteId}`);
    await page.waitForLoadState('load');

    // Verify the start date field is populated
    const startDateInput = page.locator('input[name="startDate"]');
    await expect(startDateInput).toBeVisible();
    await expect(startDateInput).not.toHaveValue('');

    // Clear the start date field
    await startDateInput.fill('');

    // Verify the field is now empty
    await expect(startDateInput).toHaveValue('');

    // Save the form
    await page.locator('button[type="submit"]').click();

    // Wait for redirect to note display page
    await page.waitForURL(/\/note\?id=/, { timeout: 5000 });

    // Verify via API that the StartDate is now cleared (null)
    const afterEdit = await apiClient.getNote(noteId);
    expect(afterEdit.StartDate).toBeFalsy();
  });

  test.afterAll(async ({ apiClient }) => {
    if (noteId) await apiClient.deleteNote(noteId);
  });
});
