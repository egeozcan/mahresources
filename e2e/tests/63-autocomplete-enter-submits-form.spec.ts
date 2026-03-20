import { test, expect } from '../fixtures/base.fixture';

/**
 * Pressing Enter in an empty autocomplete field should submit the parent form.
 *
 * The autocomplete's @keydown.enter handler uses form.requestSubmit() when
 * the input value is empty, allowing the form to submit naturally.
 */
test.describe('Autocomplete Enter key should submit the form', () => {
  test('pressing Enter in empty autocomplete field submits the parent form', async ({
    page,
  }) => {
    await page.goto('/note/new');

    // Fill in the required title field
    await page.getByRole('textbox', { name: 'Title' }).fill('Enter Submit Test Note');

    // Focus the Owner autocomplete field (which is empty)
    const ownerInput = page.getByRole('combobox', { name: 'Owner' });
    await ownerInput.click();

    // Wait for any dropdown fetch to settle
    await page.waitForTimeout(500);

    // Clear the field to ensure it's empty
    await ownerInput.fill('');
    await page.waitForTimeout(300);

    // Press Enter — should submit the form since the input is empty
    await ownerInput.press('Enter');

    // Wait for navigation (form submission redirects to the note display page)
    await page.waitForURL(/\/note\?id=\d+/, { timeout: 5000 });

    // Verify we landed on the note detail page with the correct title
    expect(page.url()).toMatch(/\/note\?id=\d+/);
    await expect(page.getByRole('heading', { level: 1 })).toContainText(
      'Enter Submit Test Note'
    );
  });
});
