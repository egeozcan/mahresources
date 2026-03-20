/**
 * Tests that the note detail page sidebar has an "Add Tag" form, matching
 * the group and resource detail pages.
 *
 * Bug: displayNote.tpl includes tagList.tpl WITHOUT the addTagUrl parameter,
 * so the "Add Tag" autocompleter form is not rendered on the note detail page
 * sidebar. Both displayGroup.tpl and displayResource.tpl correctly pass
 * addTagUrl, allowing users to add tags directly from the detail page.
 *
 * The note detail page only shows the "Tags" heading but no form to add tags,
 * forcing users to go to the edit page instead.
 */
import { test, expect } from '../fixtures/base.fixture';

test.describe('Note detail page should have Add Tag form in sidebar', () => {
  let categoryId: number;
  let groupId: number;
  let noteId: number;
  let tagId: number;

  test.beforeAll(async ({ apiClient }) => {
    const category = await apiClient.createCategory(
      'Note Tag Sidebar Category',
      'For note add-tag sidebar test',
    );
    categoryId = category.ID;

    const group = await apiClient.createGroup({
      name: 'Note Tag Sidebar Owner',
      categoryId,
    });
    groupId = group.ID;

    const tag = await apiClient.createTag(
      'SidebarTestTag',
      'Tag for sidebar add test',
    );
    tagId = tag.ID;

    const note = await apiClient.createNote({
      name: 'Note With Missing Add Tag',
      description: 'This note should have an Add Tag form in the sidebar',
      ownerId: groupId,
    });
    noteId = note.ID;
  });

  test('note detail page sidebar should show Add Tag autocompleter', async ({
    page,
  }) => {
    // Navigate to the note detail page
    await page.goto(`/note?id=${noteId}`);
    await page.waitForLoadState('load');

    // Verify the "Tags" heading is visible in the sidebar
    const tagsHeading = page.locator('aside h2, [role="complementary"] h2').filter({ hasText: 'Tags' });
    await expect(tagsHeading).toBeVisible({ timeout: 5000 });

    // The "Add Tag" form should be present in the sidebar, just like on group and resource detail pages.
    // On group detail page: form[action*="addTags"] with an autocompleter combobox is present.
    // On resource detail page: same pattern.
    // On note detail page: this form is MISSING (the bug).
    const addTagForm = page.locator('form[action*="addTags"]');
    await expect(addTagForm).toBeVisible({ timeout: 3000 });
  });

  test('group detail page has Add Tag form for comparison', async ({
    page,
  }) => {
    // Navigate to the group detail page to confirm it HAS the Add Tag form
    await page.goto(`/group?id=${groupId}`);
    await page.waitForLoadState('load');

    // The "Add Tag" form should be present on the group detail page
    const addTagForm = page.locator('form[action*="addTags"]');
    await expect(addTagForm).toBeVisible({ timeout: 3000 });

    // And it should have an autocompleter combobox
    const tagCombobox = addTagForm.locator('[role="combobox"]');
    await expect(tagCombobox).toBeVisible();
  });

  test.afterAll(async ({ apiClient }) => {
    if (noteId) await apiClient.deleteNote(noteId);
    if (tagId) await apiClient.deleteTag(tagId);
    if (groupId) await apiClient.deleteGroup(groupId);
    if (categoryId) await apiClient.deleteCategory(categoryId);
  });
});
