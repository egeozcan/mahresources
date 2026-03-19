/**
 * Tests that inline-editing a tag's name on its detail page does not
 * include the "Tag " prefix in the saved name.
 *
 * Bug: The title template passes {{ pageTitle }} (e.g., "Tag my-tag") as the
 * inline-edit content instead of just the entity name. Editing saves the
 * prefix as part of the name, causing silent data corruption.
 */
import { test, expect } from '../fixtures/base.fixture';

test.describe('Inline-edit name does not include type prefix', () => {
  let tagId: number;

  test.beforeAll(async ({ apiClient }) => {
    const tag = await apiClient.createTag(
      'PrefixTestTag',
      'Tag for prefix test',
    );
    tagId = tag.ID;
  });

  test('editing tag name via inline-edit should not prepend "Tag " prefix', async ({
    page,
    apiClient,
  }) => {
    await page.goto(`/tag?id=${tagId}`);
    await page.waitForLoadState('load');

    // Find the inline-edit component in the title
    const inlineEdit = page.locator('inline-edit').first();
    await expect(inlineEdit).toBeVisible({ timeout: 5000 });

    // Click the edit button (pencil icon)
    const editButton = inlineEdit.locator('button.edit-button');
    await editButton.click();

    // The input should be visible and pre-filled
    const input = inlineEdit.locator('input');
    await expect(input).toBeVisible({ timeout: 2000 });

    // Check the input value — it should be just "PrefixTestTag", not "Tag PrefixTestTag"
    const inputValue = await input.inputValue();
    expect(inputValue).toBe('PrefixTestTag');
    expect(inputValue).not.toContain('Tag ');
  });

  test.afterAll(async ({ apiClient }) => {
    if (tagId) await apiClient.deleteTag(tagId);
  });
});
