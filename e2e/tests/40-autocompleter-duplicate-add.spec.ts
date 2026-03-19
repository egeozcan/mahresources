/**
 * Tests that the autocompleter handles duplicate tag creation gracefully.
 *
 * Bug 1: pushVal() uses x.name (lowercase) but API returns x.Name (uppercase),
 * so existing tags are never found in results, causing an incorrect "Add?"
 * prompt for tags that already exist.
 *
 * Bug 2: addVal() doesn't check HTTP response status, so 400 error responses
 * are pushed into selectedResults as phantom "undefined" items.
 */
import { test, expect } from '../fixtures/base.fixture';

test.describe('Autocompleter does not create phantom items on duplicate', () => {
  let categoryId: number;
  let ownerGroupId: number;
  let tagId: number;

  test.beforeAll(async ({ apiClient }) => {
    const category = await apiClient.createCategory(
      'Autocomplete Dup Test Category',
      'For autocompleter duplicate test',
    );
    categoryId = category.ID;

    const ownerGroup = await apiClient.createGroup({
      name: 'Autocomplete Dup Test Owner',
      categoryId,
    });
    ownerGroupId = ownerGroup.ID;

    const tag = await apiClient.createTag(
      'ExistingTestTag',
      'A tag that already exists',
    );
    tagId = tag.ID;
  });

  test('typing an existing tag name and pressing Enter should select it, not offer to create', async ({
    page,
  }) => {
    await page.goto('/group/new');
    await page.waitForLoadState('load');

    // Find the Tags combobox specifically (second combobox, after Category)
    const tagInput = page.getByRole('combobox', { name: /tags/i });
    await expect(tagInput).toBeVisible({ timeout: 5000 });
    await tagInput.fill('ExistingTestTag');

    // Wait for results to load in the dropdown
    const option = page.locator('[role="option"]').filter({ hasText: 'ExistingTestTag' });
    await expect(option).toBeVisible({ timeout: 5000 });

    // Press Escape to close dropdown (text stays), then Enter to trigger pushVal
    await tagInput.press('Escape');
    await expect(option).not.toBeVisible();
    await tagInput.press('Enter');

    // The bug: pushVal uses x.name (lowercase) so it doesn't find the existing
    // tag in results and shows an "Add ExistingTestTag?" prompt.
    // With the fix, it should find the existing tag and re-open the dropdown
    // instead of offering to create a duplicate.
    const addButton = page.getByRole('button', { name: /add existingtesttag/i });

    // The "Add" button should NOT appear for a tag that already exists in results
    await expect(addButton).not.toBeVisible({ timeout: 2000 });
  });

  test.afterAll(async ({ apiClient }) => {
    if (tagId) await apiClient.deleteTag(tagId);
    if (ownerGroupId) await apiClient.deleteGroup(ownerGroupId);
    if (categoryId) await apiClient.deleteCategory(categoryId);
  });
});
