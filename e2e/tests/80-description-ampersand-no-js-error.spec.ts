import { test, expect } from '../fixtures/base.fixture';

test.describe('Description with special characters', () => {
  let categoryId: number;
  let groupId: number;
  let noteId: number;

  test.beforeAll(async ({ apiClient }) => {
    const category = await apiClient.createCategory(`AmpDesc Cat ${Date.now()}`, '');
    categoryId = category.ID;
    const group = await apiClient.createGroup({
      name: `AmpDesc Group ${Date.now()}`,
      categoryId,
    });
    groupId = group.ID;
    const note = await apiClient.createNote({
      name: `Ampersand Note ${Date.now()}`,
      description: 'Tom & Jerry are friends. Also: <b>bold</b> & "quoted"',
      ownerId: groupId,
    });
    noteId = note.ID;
  });

  test.afterAll(async ({ apiClient }) => {
    if (noteId) await apiClient.deleteNote(noteId).catch(() => {});
    if (groupId) await apiClient.deleteGroup(groupId).catch(() => {});
    if (categoryId) await apiClient.deleteCategory(categoryId).catch(() => {});
  });

  test('notes list should have no JS errors when description contains &', async ({ page }) => {
    const errors: string[] = [];
    page.on('pageerror', (error) => errors.push(error.message));

    await page.goto('/notes');
    await page.waitForLoadState('load');
    await page.waitForTimeout(500);

    const jsErrors = errors.filter(e => !e.includes('ResizeObserver'));
    expect(jsErrors).toHaveLength(0);
  });

  test('note detail should have no JS errors when description contains &', async ({ page }) => {
    const errors: string[] = [];
    page.on('pageerror', (error) => errors.push(error.message));

    await page.goto(`/note?id=${noteId}`);
    await page.waitForLoadState('load');
    await page.waitForTimeout(500);

    const jsErrors = errors.filter(e => !e.includes('ResizeObserver'));
    expect(jsErrors).toHaveLength(0);
  });
});
