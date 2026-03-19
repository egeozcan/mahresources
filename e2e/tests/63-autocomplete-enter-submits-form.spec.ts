import { test, expect } from '../fixtures/base.fixture';

/**
 * Bug: Pressing Enter in an empty autocomplete field does NOT submit the
 * parent form, even though the code intends it to.
 *
 * In dropdown.js, the `@keydown.enter.prevent` handler checks if the input
 * is empty and the dropdown is closed, then calls:
 *
 *     form.dispatchEvent(new Event('submit'));
 *
 * However, `dispatchEvent(new Event('submit'))` only fires event listeners
 * attached to the form element — it does NOT trigger the browser's native
 * form submission behavior. The fix is to use `form.requestSubmit()` which
 * properly triggers validation and submission like clicking a submit button.
 *
 * Expected behavior: When the user fills in the required title, focuses an
 * empty autocomplete field (e.g., Owner), and presses Enter, the form should
 * submit — the same way pressing Enter in the title field submits the form.
 *
 * Actual behavior: Nothing happens. The form stays on the creation page.
 */
test.describe('Autocomplete Enter key should submit the form', () => {
  test('pressing Enter in empty autocomplete field submits the parent form', async ({
    page,
  }) => {
    // Navigate to the note creation form
    await page.goto('/note/new');

    // Fill in the required title field
    await page.getByRole('textbox', { name: 'Title' }).fill('Enter Submit Test Note');

    // Focus the Owner autocomplete field (which is empty)
    const ownerInput = page.getByRole('combobox', { name: 'Owner' });
    await ownerInput.click();

    // Wait for any initial dropdown fetch to settle, then ensure dropdown is closed
    await page.waitForTimeout(500);

    // Clear the field to make sure it's empty and dropdown closes
    await ownerInput.fill('');
    await page.waitForTimeout(300);

    // Press Enter — this should submit the form because:
    //   1. The autocomplete input is empty
    //   2. The dropdown is not active
    //   3. The code tries to dispatch a submit event on the form
    await ownerInput.press('Enter');

    // Wait for navigation (form submission should redirect to the note display page)
    await page.waitForURL(/\/note\?id=\d+/, { timeout: 5000 });

    // Verify we landed on the note detail page
    expect(page.url()).toMatch(/\/note\?id=\d+/);

    // Verify the note was created with the correct title
    await expect(page.getByRole('heading', { level: 1 })).toContainText(
      'Enter Submit Test Note'
    );
  });
});
