/**
 * BH-010: Schema-editor Preview Form seeds numeric fields with 0 instead
 * of leaving them empty. This makes the onBlur validator fire "Must be at
 * least 1900" even though the user typed nothing, and makes range-validated
 * fields look pre-populated when they aren't.
 *
 * Fix: getPreviewValue now calls getPreviewDefaultValue, which returns
 * `undefined` for number/integer/string with no explicit `default`.
 * _renderNumberInput also renders data===0 as empty when the schema has
 * no explicit default (defensive belt-and-braces).
 */
import { test, expect } from '../fixtures/base.fixture';

test.describe('BH-010: preview form numeric fields render empty', () => {
  test('integer field with min/max constraint renders empty in Preview tab', async ({
    page,
    apiClient,
  }) => {
    const nt = await apiClient.createNoteType(
      `BH010-${Date.now()}`,
      undefined,
      {
        MetaSchema: JSON.stringify({
          type: 'object',
          properties: {
            year: { type: 'integer', minimum: 1900, maximum: 2100 },
          },
        }),
      }
    );

    await page.goto(`/noteType/edit?id=${nt.ID}`);

    // Open the Visual Editor modal
    await page.getByRole('button', { name: /visual editor/i }).click();

    // Switch to Preview Form tab
    await page.getByRole('tab', { name: /preview form/i }).click();

    // Wait for the form-mode element to render inside the preview panel
    await page.waitForSelector('#panel-preview schema-form-mode', { timeout: 5000 });
    const yearInput = page.locator('#panel-preview #field-year');
    await expect(yearInput).toBeVisible();

    // BH-010: input must render empty, NOT "0"
    await expect(yearInput).toHaveValue('');

    // Blurring the empty field should NOT surface a range error
    await yearInput.focus();
    await yearInput.blur();
    const errorSpan = page.locator('#panel-preview #field-year-error');
    const errorText = (await errorSpan.textContent().catch(() => ''))?.trim() ?? '';
    // Either the error element isn't there at all, or it doesn't contain a
    // bogus range message for a field the user never touched.
    expect(errorText).not.toMatch(/at least|at most|1900|2100/i);
  });
});
