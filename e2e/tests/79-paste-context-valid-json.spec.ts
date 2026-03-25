import { test, expect } from '../fixtures/base.fixture';

test.describe('Paste context attribute has valid JSON', () => {
  let categoryId: number;
  let noteId: number;

  test.beforeAll(async ({ apiClient }) => {
    const category = await apiClient.createCategory(`PasteCtx Cat ${Date.now()}`, '');
    categoryId = category.ID;
    // Create a note WITHOUT an owner
    const note = await apiClient.createNote({ name: `No Owner Note ${Date.now()}` });
    noteId = note.ID;
  });

  test.afterAll(async ({ apiClient }) => {
    if (noteId) await apiClient.deleteNote(noteId).catch(() => {});
    if (categoryId) await apiClient.deleteCategory(categoryId).catch(() => {});
  });

  test('note detail page should have valid JSON in data-paste-context', async ({ page }) => {
    await page.goto(`/note?id=${noteId}`);
    await page.waitForLoadState('load');

    const pasteContext = await page.locator('[data-paste-context]').first().getAttribute('data-paste-context');
    expect(pasteContext).toBeTruthy();

    // Should be parseable as valid JSON
    expect(() => JSON.parse(pasteContext!)).not.toThrow();
  });
});
