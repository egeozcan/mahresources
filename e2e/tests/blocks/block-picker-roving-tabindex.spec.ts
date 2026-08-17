import { test, expect } from '../../fixtures/base.fixture';

/**
 * The add-block picker is a listbox with a roving tabindex: exactly one option
 * carries tabindex="0" and the arrow keys move it. The arrow handlers computed
 * the next option from `activePickerIndex` rather than from what is actually
 * focused, and nothing updated that index when focus arrived any other way — so
 * a screen reader moving through the options left it pointing at the old one,
 * and the next ArrowDown jumped somewhere unrelated instead of advancing by one.
 *
 * Focus is moved here with .focus() rather than by pressing Tab on purpose:
 * non-active options carry tabindex="-1", so Tab cannot reach them and the
 * defect is not reproducible through Tab. Assistive technology is the real
 * caller, and .focus() is what it does.
 */
test.describe('Add-block picker roving tabindex', () => {
  let noteId: number;

  test.beforeAll(async ({ apiClient }) => {
    const category = await apiClient.createCategory(
      `Picker Focus Category ${Date.now()}`,
      'Category for picker focus tests',
    );
    const ownerGroup = await apiClient.createGroup({
      name: `Picker Focus Owner ${Date.now()}`,
      categoryId: category.ID,
    });
    const note = await apiClient.createNote({
      name: 'Picker Focus Note',
      description: 'Note for picker focus tests',
      ownerId: ownerGroup.ID,
    });
    noteId = note.ID;
  });

  test('ArrowDown advances from the focused option, not from a stale index', async ({ page, baseURL }) => {
    await page.goto(`${baseURL}/note?id=${noteId}`);
    await page.waitForLoadState('load');

    await page.locator('button:has-text("Edit Blocks")').click();
    await expect(page.locator('button:has-text("Done")')).toBeVisible();

    await page.locator('[data-testid="add-block-trigger"]').click();

    const options = page.locator('[role="option"][data-block-type]');
    await expect(options.first()).toBeVisible();

    const count = await options.count();
    // Three options are needed for the assertion to distinguish "advanced by
    // one" from "jumped back to the top".
    expect(count).toBeGreaterThanOrEqual(3);

    // Focus the third option the way assistive technology would, then step down
    // once. The next option is the fourth if there is one, otherwise the third
    // stays put at the end of the list.
    const startIndex = 2;
    const expectedIndex = Math.min(startIndex + 1, count - 1);

    await options.nth(startIndex).focus();
    await expect(options.nth(startIndex)).toBeFocused();

    await page.keyboard.press('ArrowDown');

    await expect(options.nth(expectedIndex)).toBeFocused();
    // The roving tabindex has to move with focus, or the next Tab into the
    // listbox lands on a different option than the one the reader is on.
    await expect(options.nth(expectedIndex)).toHaveAttribute('tabindex', '0');
    await expect(options.nth(expectedIndex)).toHaveAttribute('aria-selected', 'true');
  });
});
