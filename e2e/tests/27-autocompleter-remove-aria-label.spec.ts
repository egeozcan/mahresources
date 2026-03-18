import { test, expect } from '../fixtures/base.fixture';

/**
 * Bug: In dropDownSelectedResults.tpl, the remove button for selected autocompleter
 * items has a static aria-label using JS template literal syntax:
 *
 *   aria-label="Remove ${result.Name}"
 *
 * This is a plain HTML attribute, NOT an Alpine.js dynamic binding (:aria-label),
 * so the literal string "Remove ${result.Name}" is rendered instead of the
 * actual item name (e.g., "Remove My Tag"). Screen readers announce the broken
 * literal text, which is an accessibility bug.
 *
 * Note: There IS a visually hidden <span x-text="'Remove ' + result.Name">
 * inside the button that works correctly, but the aria-label on the button
 * itself takes precedence over inner text for assistive technology.
 */
test.describe('Autocompleter remove button aria-label', () => {
  let categoryId: number;
  let tagId: number;
  const testRunId = `${Date.now()}`;

  test.beforeAll(async ({ apiClient }) => {
    const category = await apiClient.createCategory(
      `AriaLabelTestCat ${testRunId}`,
      'Category for autocompleter aria-label test'
    );
    categoryId = category.ID;

    const tag = await apiClient.createTag(
      `AriaLabelTestTag ${testRunId}`,
      'Tag for autocompleter aria-label test'
    );
    tagId = tag.ID;
  });

  test('remove button aria-label should contain the actual item name, not a template literal', async ({
    page,
  }) => {
    // Navigate to the group creation form, which has an autocompleter for tags
    await page.goto('/group/new');
    await page.waitForLoadState('load');

    // First select a category (required for the form)
    const categoryInput = page.getByRole('combobox', { name: 'Category' });
    await categoryInput.click();
    await categoryInput.fill(`AriaLabelTestCat ${testRunId}`);
    const categoryOption = page.locator(
      `div[role="option"]:visible:has-text("AriaLabelTestCat ${testRunId}")`
    ).first();
    await categoryOption.waitFor({ timeout: 10000 });
    await categoryOption.click();

    // Now select a tag in the Tags autocompleter
    const tagInput = page.getByRole('combobox', { name: 'Tags' });
    await tagInput.click();
    await tagInput.fill(`AriaLabelTestTag ${testRunId}`);
    const tagOption = page.locator(
      `div[role="option"]:visible:has-text("AriaLabelTestTag ${testRunId}")`
    ).first();
    await tagOption.waitFor({ timeout: 10000 });
    await tagOption.click();

    // Wait for the selected tag to appear in the "selected results" area
    // The selected item text should be visible
    await expect(
      page.locator(`text=AriaLabelTestTag ${testRunId}`).first()
    ).toBeVisible({ timeout: 5000 });

    // Find the remove button for the selected tag.
    // The button is inside a <p> element that contains the tag name text.
    const selectedTagPill = page.locator(
      `p:has(span:has-text("AriaLabelTestTag ${testRunId}")) button`
    ).first();
    await expect(selectedTagPill).toBeVisible({ timeout: 5000 });

    // THE BUG: The aria-label is the literal string "Remove ${result.Name}"
    // instead of the actual tag name like "Remove AriaLabelTestTag <testRunId>".
    //
    // This test asserts the CORRECT behavior (dynamic name in aria-label),
    // so it should FAIL against the current code.
    const ariaLabel = await selectedTagPill.getAttribute('aria-label');
    expect(ariaLabel).not.toContain('${result.Name}');
    expect(ariaLabel).toContain(`AriaLabelTestTag ${testRunId}`);
  });

  test.afterAll(async ({ apiClient }) => {
    if (tagId) {
      await apiClient.deleteTag(tagId);
    }
    if (categoryId) {
      await apiClient.deleteCategory(categoryId);
    }
  });
});
