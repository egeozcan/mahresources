/**
 * Tests that the "Related Notes" section on a group detail page uses the
 * correct parameter (groups) instead of ownerId.
 *
 * Bug: The "New" button under "Related Notes" navigates to /note/new?ownerId=ID
 * instead of /note/new?groups=ID, creating an ownership relationship instead
 * of a many-to-many relationship.
 */
import { test, expect } from '../fixtures/base.fixture';

test.describe('Related Notes uses groups param, not ownerId', () => {
  let categoryId: number;
  let groupId: number;
  let noteId: number;

  test.beforeAll(async ({ apiClient }) => {
    const category = await apiClient.createCategory(
      'Related Notes Param Test Category',
      'For related notes param test',
    );
    categoryId = category.ID;

    const group = await apiClient.createGroup({
      name: 'Related Notes Param Test Group',
      categoryId,
    });
    groupId = group.ID;

    // Create a note RELATED to the group (not owned by it)
    const note = await apiClient.createNote({
      name: 'Related Note Test',
      description: 'This note is related to the group, not owned by it',
      groups: [groupId],
    });
    noteId = note.ID;
  });

  test('See All form under Related Notes should use groups param', async ({
    page,
  }) => {
    await page.goto(`/group?id=${groupId}`);
    await page.waitForLoadState('load');

    // The "Related Notes" section should be visible
    const relatedSection = page.locator('text=Related Notes').first();
    await expect(relatedSection).toBeVisible({ timeout: 5000 });

    // The seeAll template uses a <form action="/notes"> with a hidden input
    // for the parameter. Find all forms with action="/notes"
    const notesForms = page.locator('form[action="/notes"]');
    const count = await notesForms.count();

    let hasGroupsParam = false;
    for (let i = 0; i < count; i++) {
      const form = notesForms.nth(i);
      // Check if this form has a hidden input with name="groups"
      const groupsInput = form.locator('input[type="hidden"][name="groups"]');
      if (await groupsInput.count() > 0) {
        hasGroupsParam = true;
        break;
      }
    }

    // At least one notes form should use groups= parameter (for Related Notes)
    expect(hasGroupsParam).toBe(true);
  });

  test.afterAll(async ({ apiClient }) => {
    if (noteId) await apiClient.deleteNote(noteId);
    if (groupId) await apiClient.deleteGroup(groupId);
    if (categoryId) await apiClient.deleteCategory(categoryId);
  });
});
