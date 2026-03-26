/**
 * Accessibility test: Paste upload dialog inputs must have aria-labels
 *
 * Bug: The tag, category, and series search inputs in the paste upload dialog
 * rely on placeholder text only. Placeholders are not a reliable accessible
 * name (WCAG 1.3.1, 4.1.2). Screen reader users cannot identify what these
 * inputs are for once they start typing.
 */
import { test, expect } from '../../fixtures/a11y.fixture';

test.describe('Paste Upload Dialog - Missing aria-labels', () => {
  /**
   * Helper to open paste upload dialog by calling the Alpine store directly.
   */
  async function openPasteDialog(page: import('@playwright/test').Page, groupId: number) {
    await page.goto(`/group?id=${groupId}`);
    await page.waitForLoadState('load');

    // Open the paste upload dialog via the Alpine store.
    // We create a minimal item array matching the extractPasteContent output format.
    await page.evaluate(() => {
      const store = (window as any).Alpine?.store('pasteUpload');
      if (!store) throw new Error('Alpine pasteUpload store not found');

      // Create a fake file blob for the item
      const blob = new Blob(['test'], { type: 'image/png' });
      const file = new File([blob], 'test-paste.png', { type: 'image/png' });
      const previewUrl = URL.createObjectURL(blob);

      const items = [{
        file,
        name: 'test-paste.png',
        previewUrl,
        type: 'image',
        error: null,
        errorResourceId: null,
        _snippet: null,
      }];

      store.open(items, { type: 'group', id: 0, name: 'Test' });
    });

    // Wait for the dialog to be visible
    const dialog = page.locator('[role="dialog"][aria-labelledby="paste-upload-title"]');
    await dialog.waitFor({ state: 'visible', timeout: 5000 });
  }

  test('Tag search input in paste dialog should have an aria-label', async ({ page, a11yTestData }) => {
    await openPasteDialog(page, a11yTestData.groupId);

    // Find the tag search input by its placeholder
    const tagInput = page.locator('[role="dialog"] input[placeholder="Search tags..."]');
    // The input may be inside a template x-if, so wait for it
    const isVisible = await tagInput.isVisible().catch(() => false);
    test.skip(!isVisible, 'Tag input not visible -- paste dialog may not have rendered metadata section');

    const ariaLabel = await tagInput.getAttribute('aria-label');
    expect(
      ariaLabel,
      'Tag search input should have aria-label="Search tags" or similar, ' +
      'but relies on placeholder only. WCAG 1.3.1 / 4.1.2.'
    ).toBeTruthy();
  });

  test('Category search input in paste dialog should have an aria-label', async ({ page, a11yTestData }) => {
    await openPasteDialog(page, a11yTestData.groupId);

    const catInput = page.locator('[role="dialog"] input[placeholder="Search categories..."]');
    const isVisible = await catInput.isVisible().catch(() => false);
    test.skip(!isVisible, 'Category input not visible');

    const ariaLabel = await catInput.getAttribute('aria-label');
    expect(
      ariaLabel,
      'Category search input should have aria-label="Search categories" or similar, ' +
      'but relies on placeholder only. WCAG 1.3.1 / 4.1.2.'
    ).toBeTruthy();
  });

  test('Series search input in paste dialog should have an aria-label', async ({ page, a11yTestData }) => {
    await openPasteDialog(page, a11yTestData.groupId);

    const seriesInput = page.locator('[role="dialog"] input[placeholder*="series"]');
    const isVisible = await seriesInput.isVisible().catch(() => false);
    test.skip(!isVisible, 'Series input not visible');

    const ariaLabel = await seriesInput.getAttribute('aria-label');
    expect(
      ariaLabel,
      'Series search input should have aria-label="Search series" or similar, ' +
      'but relies on placeholder only. WCAG 1.3.1 / 4.1.2.'
    ).toBeTruthy();
  });

  test('Paste upload dialog should pass axe-core checks when open', async ({ page, checkA11y, a11yTestData }) => {
    await openPasteDialog(page, a11yTestData.groupId);

    // Run axe on the dialog, excluding pre-existing color-contrast issues
    // (the label text-stone-500 on white has insufficient contrast but is a separate issue)
    await checkA11y({
      include: ['[role="dialog"][aria-labelledby="paste-upload-title"]'],
      disableRules: ['color-contrast'],
    });
  });
});
