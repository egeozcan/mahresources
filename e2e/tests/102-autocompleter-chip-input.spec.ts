import { test, expect } from '../fixtures/base.fixture';

/**
 * Tier 2 — chip-input niceties on the SHARED autocompleter component, exercised through a
 * non-standalone form (/group/new) so the shared-component contract is guarded:
 *  - comma always commits the current token
 *  - backspace on an empty input removes the last applied chip
 *  - a "Create X" row appears for the no-match case and creates+applies the tag
 *  - space does NOT commit by default (multi-word tag names stay typeable)
 */
test.describe('Autocompleter chip-input (shared form)', () => {
  const runId = Date.now();
  let categoryId: number;
  const createdTagIds: number[] = [];

  test.beforeAll(async ({ apiClient }) => {
    const cat = await apiClient.createCategory(`ChipInput Cat ${runId}`);
    categoryId = cat.ID;
  });

  test.afterAll(async ({ apiClient }) => {
    for (const id of createdTagIds) {
      try { await apiClient.deleteTag(id); } catch { /* ignore */ }
    }
    if (categoryId) await apiClient.deleteCategory(categoryId);
  });

  test('comma commits the current token as a chip and clears the input', async ({ page, apiClient }) => {
    const tag = await apiClient.createTag(`ChipComma${runId}`);
    createdTagIds.push(tag.ID);

    await page.goto('/group/new');
    await page.waitForLoadState('load');

    const tagInput = page.getByRole('combobox', { name: /tags/i });
    await tagInput.fill(tag.Name);
    // Wait for the result so the exact-match commit path is exercised.
    await expect(page.locator('[role="option"]').filter({ hasText: tag.Name })).toBeVisible({ timeout: 5000 });

    await tagInput.press(',');

    // The chip is committed (hidden input carries the tag id) and the input is cleared.
    await expect(page.locator(`input[name="tags"][value="${tag.ID}"]`)).toHaveCount(1);
    await expect(tagInput).toHaveValue('');
    // The trailing comma is not left in the buffer.
    await expect(tagInput).not.toHaveValue(',');
  });

  test('backspace on an empty input removes the last applied chip', async ({ page, apiClient }) => {
    const a = await apiClient.createTag(`ChipBsA${runId}`);
    const b = await apiClient.createTag(`ChipBsB${runId}`);
    createdTagIds.push(a.ID, b.ID);

    await page.goto('/group/new');
    await page.waitForLoadState('load');

    const tagInput = page.getByRole('combobox', { name: /tags/i });
    for (const t of [a, b]) {
      await tagInput.fill(t.Name);
      await expect(page.locator('[role="option"]').filter({ hasText: t.Name })).toBeVisible({ timeout: 5000 });
      await tagInput.press(',');
      await expect(page.locator(`input[name="tags"][value="${t.ID}"]`)).toHaveCount(1);
    }

    // Empty input → backspace removes the last chip (b), leaving a.
    await expect(tagInput).toHaveValue('');
    await tagInput.press('Backspace');

    await expect(page.locator(`input[name="tags"][value="${b.ID}"]`)).toHaveCount(0);
    await expect(page.locator(`input[name="tags"][value="${a.ID}"]`)).toHaveCount(1);
  });

  test('a "Create X" row appears for an unknown name and creates + applies it', async ({ page }) => {
    const newName = `ChipCreate${runId}`;

    await page.goto('/group/new');
    await page.waitForLoadState('load');

    const tagInput = page.getByRole('combobox', { name: /tags/i });
    await tagInput.fill(newName);

    const createRow = page.locator('[role="option"]').filter({ hasText: `Create "${newName}"` });
    await expect(createRow).toBeVisible({ timeout: 5000 });

    await createRow.click();

    // The tag was created and applied as a chip (a hidden input now carries an id).
    await expect(page.locator('input[name="tags"]:not([value=""])')).toHaveCount(1, { timeout: 5000 });
    await expect(tagInput).toHaveValue('');
  });

  test('space does NOT commit by default (multi-word names stay typeable)', async ({ page }) => {
    await page.goto('/group/new');
    await page.waitForLoadState('load');

    const tagInput = page.getByRole('combobox', { name: /tags/i });
    await tagInput.focus();
    await tagInput.pressSequentially('still life', { delay: 30 });

    // The space did not commit a chip; the whole phrase remains in the buffer.
    await expect(tagInput).toHaveValue('still life');
    await expect(page.locator('input[name="tags"]:not([value=""])')).toHaveCount(0);
  });
});
