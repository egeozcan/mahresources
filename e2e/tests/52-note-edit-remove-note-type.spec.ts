/**
 * Tests that removing the Note Type from a note via the edit form actually
 * clears the NoteTypeId in the database.
 *
 * Bug: When editing a note and removing the Note Type via the autocompleter
 * "Remove" button, then saving, the NoteTypeId is NOT cleared. The
 * autocompleter removes the hidden input entirely (since selectedResults
 * is empty, the x-for loop produces no <input> elements), so the form
 * submits without a NoteTypeId field. The backend interprets the missing
 * field as NoteTypeId=0 and sets the pointer to nil, but the note type
 * association persists after save.
 */
import { test, expect } from '../fixtures/base.fixture';

test.describe('Note edit removes note type', () => {
  let categoryId: number;
  let groupId: number;
  let noteTypeId: number;
  let noteId: number;

  test.beforeAll(async ({ apiClient }) => {
    // Create prerequisite entities
    const category = await apiClient.createCategory(
      'NoteTypeRemoveCat',
      'Category for note-type removal test',
    );
    categoryId = category.ID;

    const group = await apiClient.createGroup({
      name: 'NoteTypeRemoveOwner',
      categoryId,
    });
    groupId = group.ID;

    const noteType = await apiClient.createNoteType(
      'RemovableType',
      'A note type that should be removable',
    );
    noteTypeId = noteType.ID;

    // Create a note WITH a note type
    const note = await apiClient.createNote({
      name: 'NoteWithType',
      description: 'Note that has a note type assigned',
      ownerId: groupId,
      noteTypeId,
    });
    noteId = note.ID;
  });

  test('removing note type via edit form clears NoteTypeId', async ({
    page,
    apiClient,
  }) => {
    // Verify the note type is set before editing
    const beforeEdit = await apiClient.getNote(noteId);
    expect(beforeEdit.NoteTypeId).toBe(noteTypeId);

    // Navigate to the edit form
    await page.goto(`/note/edit?id=${noteId}`);
    await page.waitForLoadState('load');

    // Verify the note type is shown as selected in the form
    const removeButton = page.locator('button').filter({ hasText: /Remove.*RemovableType/i });
    await expect(removeButton).toBeVisible();

    // Click the Remove button to deselect the note type
    await removeButton.click();

    // Verify the note type is no longer shown
    await expect(removeButton).not.toBeVisible();

    // Save the form
    await page.locator('button[type="submit"]').click();

    // Wait for redirect to note display page
    await page.waitForURL(/\/note\?id=/, { timeout: 5000 });

    // Verify via API that the NoteTypeId is now cleared (null/0)
    const afterEdit = await apiClient.getNote(noteId);
    expect(afterEdit.NoteTypeId).toBeFalsy();
  });

  test.afterAll(async ({ apiClient }) => {
    if (noteId) await apiClient.deleteNote(noteId);
    if (groupId) await apiClient.deleteGroup(groupId);
    if (noteTypeId) await apiClient.deleteNoteType(noteTypeId);
    if (categoryId) await apiClient.deleteCategory(categoryId);
  });
});
