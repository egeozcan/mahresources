import { test, expect } from '../fixtures/base.fixture';
import path from 'path';

/**
 * Keyboard accessibility for the inline tag editor on the resources list page.
 *
 * Covers three fixes:
 * 1. ArrowDown/ArrowUp reopen a closed dropdown.
 * 2. Enter with text in the combobox selects the highlighted tag (no form
 *    submission / navigation), and the inline editor form never navigates.
 * 3. Focus returns to the combobox after removing the last selected tag.
 */
test.describe('Inline tag editor keyboard accessibility', () => {
  let categoryId: number;
  let ownerGroupId: number;
  let resourceId: number;
  let tag1Id: number;
  let tag2Id: number;

  test.beforeAll(async ({ apiClient }) => {
    const category = await apiClient.createCategory(
      'InlineTagKB Category',
      'For inline tag keyboard tests',
    );
    categoryId = category.ID;

    const ownerGroup = await apiClient.createGroup({
      name: 'InlineTagKB Owner',
      categoryId,
    });
    ownerGroupId = ownerGroup.ID;

    const tag1 = await apiClient.createTag(
      'kbtag_alpha',
      'keyboard test tag 1',
    );
    tag1Id = tag1.ID;

    const tag2 = await apiClient.createTag(
      'kbtag_beta',
      'keyboard test tag 2',
    );
    tag2Id = tag2.ID;

    const fs = await import('fs');
    const os = await import('os');
    const tmpFile = path.join(os.tmpdir(), 'inline-tag-kb-test.txt');
    fs.writeFileSync(tmpFile, 'keyboard tag test content');

    const resource = await apiClient.createResource({
      filePath: tmpFile,
      name: 'InlineTagKB Resource',
      ownerId: ownerGroupId,
    });
    resourceId = resource.ID;

    fs.unlinkSync(tmpFile);
  });

  /** Click the first Edit Tags button and return the tag editor combobox. */
  async function openTagEditor(page: import('@playwright/test').Page) {
    const editBtn = page.locator('button.edit-in-list').first();
    await editBtn.click();

    // The tag editor form appears inside .card-tags; the combobox is an input
    const combobox = page.locator('.card-tags input[role="combobox"]').first();
    await expect(combobox).toBeFocused({ timeout: 3000 });
    return combobox;
  }

  test('Enter selects the first matching tag without navigating away', async ({
    page,
  }) => {
    await page.goto('/resources');

    const combobox = await openTagEditor(page);

    // Type to filter — should show kbtag_alpha
    await combobox.fill('kbtag_a');
    await expect(
      page.getByRole('option', { name: 'kbtag_alpha' }),
    ).toBeVisible({ timeout: 3000 });

    // Press Enter directly — should select, NOT navigate
    await combobox.press('Enter');

    // Still on the resources page
    expect(page.url()).toContain('/resources');
    expect(page.url()).not.toContain('editedId');

    // Tag pill should appear
    await expect(
      page.getByRole('button', { name: 'Remove kbtag_alpha' }),
    ).toBeVisible({ timeout: 3000 });
  });

  test('ArrowDown reopens a closed dropdown', async ({ page }) => {
    await page.goto('/resources');

    const combobox = await openTagEditor(page);

    // Dropdown should be open
    const listbox = page.locator('.card-tags [role="listbox"]');
    await expect(listbox).toBeVisible({ timeout: 3000 });

    // Close with Escape
    await combobox.press('Escape');
    await expect(listbox).not.toBeVisible();

    // Reopen with ArrowDown
    await combobox.press('ArrowDown');
    await expect(listbox).toBeVisible({ timeout: 1000 });
  });

  test('Enter with empty input selects the first tag without navigating', async ({
    page,
  }) => {
    await page.goto('/resources');

    const combobox = await openTagEditor(page);

    // Wait for results to load and first option to be highlighted
    const firstOption = page.locator('.card-tags [role="option"]').first();
    await expect(firstOption).toBeVisible({ timeout: 3000 });

    // Get the name of the first option so we can verify it was selected
    const firstName = await firstOption.textContent();

    // Press Enter with empty input — should select the highlighted tag, NOT navigate
    await combobox.press('Enter');

    // Still on the resources page (no form submission navigation)
    expect(page.url()).toContain('/resources');
    expect(page.url()).not.toContain('editedId');

    // The first tag should appear as a pill
    await expect(
      page.getByRole('button', { name: `Remove ${firstName}` }),
    ).toBeVisible({ timeout: 3000 });
  });

  test('focus returns to combobox after removing a tag via keyboard', async ({
    page,
    apiClient,
  }) => {
    // Create a dedicated resource so we start with a clean tag state
    const fs = await import('fs');
    const os = await import('os');
    const tmpFile = path.join(os.tmpdir(), 'focus-test-resource.txt');
    fs.writeFileSync(tmpFile, 'focus test');
    const res = await apiClient.createResource({
      filePath: tmpFile,
      name: 'FocusTestResource',
      ownerId: ownerGroupId,
    });
    fs.unlinkSync(tmpFile);

    try {
      await page.goto('/resources');

      // Find the Edit Tags button for our specific resource
      const article = page.locator('article', {
        has: page.getByRole('link', { name: 'FocusTestResource' }),
      });
      await article.locator('button.edit-in-list').click();

      const combobox = article.locator('input[role="combobox"]');
      await expect(combobox).toBeFocused({ timeout: 3000 });

      // Type and select a tag
      await combobox.fill('kbtag_b');
      await expect(
        page.getByRole('option', { name: 'kbtag_beta' }),
      ).toBeVisible({ timeout: 3000 });
      await combobox.press('Enter');

      // Verify tag pill appeared
      const removeBtn = page.getByRole('button', {
        name: 'Remove kbtag_beta',
      });
      await expect(removeBtn).toBeVisible({ timeout: 3000 });

      // Focus the Remove button and press Enter via keyboard
      await removeBtn.focus();
      await expect(removeBtn).toBeFocused();
      await removeBtn.press('Enter');

      // Tag pill should be gone
      await expect(removeBtn).not.toBeVisible();

      // Focus should be back on the combobox
      await expect(combobox).toBeFocused();
    } finally {
      await apiClient.deleteResource(res.ID);
    }
  });

  test.afterAll(async ({ apiClient }) => {
    if (resourceId) await apiClient.deleteResource(resourceId);
    if (tag1Id) await apiClient.deleteTag(tag1Id);
    if (tag2Id) await apiClient.deleteTag(tag2Id);
    if (ownerGroupId) await apiClient.deleteGroup(ownerGroupId);
    if (categoryId) await apiClient.deleteCategory(categoryId);
  });
});
