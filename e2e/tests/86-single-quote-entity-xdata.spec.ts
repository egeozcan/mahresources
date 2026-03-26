/**
 * Regression test: single quotes in entity names must not break x-data attributes
 *
 * Bug: Templates embed entity JSON in single-quoted x-data attributes like:
 *   <div x-data='{ "entity": {"Name":"O'Brien"} }'>
 * A single quote in any entity field terminates the HTML attribute, breaking
 * Alpine.js initialization. This can cause JS errors and broken UI.
 *
 * Fix: Modify the |json filter to HTML-entity-encode single quotes (&#39;)
 * when the output will be used in HTML attributes.
 */
import { test, expect } from '../fixtures/base.fixture';

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

  test('group card with single-quote name should have valid x-data attribute', async ({ page }) => {
    await page.goto('/groups');
    await page.waitForLoadState('load');

    // The group card should be visible with the correct name
    const card = page.locator('article.group-card', { hasText: "O'Brien" });
    await expect(card).toBeVisible();

    // The x-data attribute on the inner div should NOT be truncated by a stray single quote.
    // If the attribute is broken, it would be cut off at the first unescaped single quote.
    const xDataDiv = card.locator('div[x-data]').first();
    const xDataAttr = await xDataDiv.getAttribute('x-data');

    // The attribute must contain the entity name (possibly with &#39; decoded to ')
    // and must end with a closing brace, proving it wasn't truncated.
    expect(xDataAttr).not.toBeNull();
    expect(xDataAttr!).toContain('Brien');
    expect(xDataAttr!.trim()).toMatch(/\}$/); // ends with }
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
