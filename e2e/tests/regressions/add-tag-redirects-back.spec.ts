/**
 * Tests that adding a tag on a group detail page stays on that page.
 *
 * Historical bug: the tag form used an absolute `redirect=` URL, which the
 * server's isSafeRedirect rejected, falling back to the groups list. The form is
 * gone now — the association is persisted in place as it is chosen — so the
 * failure this guards against is any navigation at all on an add.
 */
import { test, expect } from '../../fixtures/base.fixture';

test.describe('Add tag stays on the detail page', () => {
  let categoryId: number;
  let groupId: number;
  let tagId: number;

  test.beforeAll(async ({ apiClient }) => {
    const category = await apiClient.createCategory(
      'Tag Redirect Test Category',
      'For tag redirect test',
    );
    categoryId = category.ID;

    const group = await apiClient.createGroup({
      name: 'Tag Redirect Test Group',
      description: 'Group for testing tag add redirect',
      categoryId,
    });
    groupId = group.ID;

    const tag = await apiClient.createTag(
      'RedirectTestTag',
      'Tag for redirect test',
    );
    tagId = tag.ID;
  });

  test('adding a tag on group detail page should stay on that page', async ({
    page,
    apiClient,
  }) => {
    // Navigate to the group detail page
    await page.goto(`/group?id=${groupId}`);
    await page.waitForLoadState('load');

    // A page-lifetime marker survives only if the add does not navigate.
    await page.evaluate(() => {
      (window as unknown as { __inPlace?: boolean }).__inPlace = true;
    });

    // Find the "Add Tag" autocompleter combobox and type the tag name
    const tagInput = page.locator('[data-selector-field="editedId"] input[role="combobox"]');
    await expect(tagInput).toBeVisible({ timeout: 5000 });
    await tagInput.fill('RedirectTestTag');

    // Wait for and select the dropdown result. Choosing it persists the
    // association itself — there is no submit button and no redirect.
    const option = page.locator('[role="option"]').filter({ hasText: 'RedirectTestTag' });
    await expect(option).toBeVisible({ timeout: 5000 });
    const addTagPost = page.waitForResponse(
      (r) => r.url().includes('/v1/groups/addTags') && r.request().method() === 'POST',
    );
    await option.click();
    await addTagPost;

    // The add landed...
    await expect
      .poll(async () => {
        const fresh = (await apiClient.getGroup(groupId)) as unknown as {
          Tags?: { ID: number }[];
        };
        return (fresh.Tags ?? []).some((t) => t.ID === tagId);
      })
      .toBe(true);

    // ...and the page never moved.
    expect(page.url()).toContain(`/group?id=${groupId}`);
    expect(page.url()).not.toMatch(/\/groups(\?|$)/);
    expect(
      await page.evaluate(() => (window as unknown as { __inPlace?: boolean }).__inPlace),
    ).toBe(true);
  });

  test.afterAll(async ({ apiClient }) => {
    if (groupId) await apiClient.deleteGroup(groupId);
    if (tagId) await apiClient.deleteTag(tagId);
    if (categoryId) await apiClient.deleteCategory(categoryId);
  });
});
