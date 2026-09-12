/**
 * Regression test: single quotes in entity names must not break x-data attributes
 *
 * Bug: Templates embed entity JSON in single-quoted x-data attributes like:
 *   <div x-data='{ "entity": {"Name":"O'Brien"} }'>
 * A single quote in any entity field terminates the HTML attribute, breaking
 * Alpine.js initialization. This can cause JS errors and broken UI.
 *
 * Entity JSON now lives in data-entity and is parsed by x-data. Verify both
 * the escaped attribute payload and Alpine's initialized entity state.
 */
import { test, expect } from '../../fixtures/base.fixture';

test.describe('Single Quotes in Entity Data - x-data Safety', () => {
  let categoryId: number;
  let groupId: number;
  let noteId: number;

  test.beforeAll(async ({ apiClient }) => {
    const category = await apiClient.createCategory("Quote's Category", "Test category");
    categoryId = category.ID;

    const group = await apiClient.createGroup({
      name: "O'Brien's Test Group",
      description: "Description with 'quotes' inside",
      categoryId: categoryId,
    });
    groupId = group.ID;

    const note = await apiClient.createNote({
      name: "Note's with 'apostrophe",
      description: "It's a test",
    });
    noteId = note.ID;
  });

  test('group card with single-quote name should initialize complete entity data', async ({ page }) => {
    await page.goto('/groups');
    await page.waitForLoadState('load');

    // The group card should be visible with the correct name
    const card = page.locator('article.group-card', { hasText: "O'Brien" });
    await expect(card).toBeVisible();

    const entityDiv = card.locator('div[data-entity][x-data]').first();
    const expectedEntity = {
      ID: groupId,
      Name: "O'Brien's Test Group",
      Description: "Description with 'quotes' inside",
    };

    // A stray quote must not truncate the serialized payload.
    const entityJSON = await entityDiv.getAttribute('data-entity');
    expect(entityJSON).not.toBeNull();
    expect(JSON.parse(entityJSON!)).toMatchObject(expectedEntity);

    // Rendering the name alone cannot prove Alpine initialized successfully.
    await expect.poll(() => entityDiv.evaluate((el) => {
      const alpineWindow = window as unknown as {
        Alpine: { $data: (element: Element) => { entity?: unknown } };
      };
      return alpineWindow.Alpine.$data(el).entity;
    })).toMatchObject(expectedEntity);
  });

  test('note detail page with single-quote name should render correctly', async ({ page }) => {
    await page.goto(`/note?id=${noteId}`);
    await page.waitForLoadState('load');

    // Verify the note name renders correctly in the heading
    await expect(page.locator('h1')).toContainText("Note's");

    // The x-data attribute on the entity div should be valid (uses data-paste-context)
    const xDataDiv = page.locator('div[data-paste-context]');
    const xDataAttr = await xDataDiv.getAttribute('x-data');

    expect(xDataAttr).not.toBeNull();
    // Must contain the note entity and not be truncated
    expect(xDataAttr!).toContain('entity');
    expect(xDataAttr!.trim()).toMatch(/\}$/); // ends with }
  });

  test('group detail page with single-quote name should render fully', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    await page.waitForLoadState('load');

    // The group name should render correctly in the heading
    await expect(page.locator('h1')).toContainText("O'Brien");

    // The sidebar should render (proves x-data didn't break Alpine)
    const sidebar = page.locator('.sidebar-group');
    await expect(sidebar.first()).toBeVisible();

    // The merge form should be functional (it depends on x-data working correctly)
    const mergeForm = page.locator('form[action*="groups/merge"]');
    await expect(mergeForm).toBeVisible();
  });

  test.afterAll(async ({ apiClient }) => {
    if (noteId) await apiClient.deleteNote(noteId);
    if (groupId) await apiClient.deleteGroup(groupId);
    if (categoryId) await apiClient.deleteCategory(categoryId);
  });
});
