import { test, expect } from '../fixtures/base.fixture';

test.describe('Note wide display sidebar parity', () => {
  let noteTypeId: number;
  let noteId: number;

  test.beforeAll(async ({ apiClient }) => {
    // Create a note type
    const noteType = await apiClient.createNoteType(
      'WideDisplayTestType',
      'Type for wide display sidebar test',
    );
    noteTypeId = noteType.ID;

    // Create a note with the note type assigned
    const note = await apiClient.createNote({
      name: 'WideDisplayTestNote',
      description: 'Description for wide display test',
      noteTypeId: noteTypeId,
    });
    noteId = note.ID;
  });

  test('regular detail page shows Note Type in sidebar', async ({ page }) => {
    await page.goto(`/note?id=${noteId}`);

    // The regular detail page should show the Note Type section
    const sidebar = page.locator('aside, [role="complementary"]');
    await expect(sidebar.getByText('Note Type', { exact: true })).toBeVisible();
    await expect(sidebar.getByRole('link', { name: 'WideDisplayTestType' })).toBeVisible();
  });

  test('wide display page should also show Note Type in sidebar', async ({ page }) => {
    await page.goto(`/note/text?id=${noteId}`);

    // BUG: The wide display page is missing the Note Type section
    // in its sidebar, unlike the regular detail page.
    const sidebar = page.locator('aside, [role="complementary"]');
    await expect(sidebar.getByText('Note Type', { exact: true })).toBeVisible();
    await expect(sidebar.getByRole('link', { name: 'WideDisplayTestType' })).toBeVisible();
  });

  test.afterAll(async ({ apiClient }) => {
    if (noteId) {
      await apiClient.deleteNote(noteId);
    }
    if (noteTypeId) {
      await apiClient.deleteNoteType(noteTypeId);
    }
  });
});
