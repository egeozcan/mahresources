/**
 * Accessibility tests for the Custom Thumbnail popup.
 *
 * The sidebar group is one button now; the upload and regenerate controls live
 * in a dialog behind it, so this is a new modal surface on the resource detail
 * page: axe-core over the open dialog, Escape to close, the focus trap, and the
 * focus return to the button that opened it (owned by the component, not by
 * `x-trap` — its own restore points at whatever held focus when it armed).
 */
import path from 'path';
import { test, expect } from '../../fixtures/a11y.fixture';

test.describe.serial('Custom thumbnail popup accessibility', () => {
  let resourceId: number;
  let runId: number;

  test.beforeAll(async ({ apiClient }) => {
    runId = Date.now();

    const category = await apiClient.createCategory(
      `Custom thumb a11y Category ${runId}`,
      'Category for custom thumbnail a11y tests',
    );
    const owner = await apiClient.createGroup({
      name: `Custom thumb a11y Owner ${runId}`,
      description: 'Owner for custom thumbnail a11y tests',
      categoryId: category.ID,
    });
    const resource = await apiClient.createResource({
      filePath: path.join(__dirname, '../../test-assets/sample-image-9.png'),
      name: `Custom thumb a11y resource ${runId}`,
      description: 'Resource used to exercise the custom thumbnail popup',
      ownerId: owner.ID,
    });
    resourceId = resource.ID;
  });

  test('the closed sidebar shows one button, not the controls', async ({ page }) => {
    await page.goto(`/resource?id=${resourceId}`);
    await page.waitForLoadState('load');

    await expect(page.getByTestId('custom-thumbnail-open')).toBeVisible();
    await expect(page.getByRole('button', { name: 'Upload Image' })).toHaveCount(0);
    await expect(page.getByRole('button', { name: 'Regenerate from Source' })).toHaveCount(0);
    await expect(page.getByRole('dialog')).toHaveCount(0);
  });

  test('the open popup has no axe violations', async ({ page, checkA11y }) => {
    await page.goto(`/resource?id=${resourceId}`);
    await page.waitForLoadState('load');

    await page.getByTestId('custom-thumbnail-open').click();
    const dialog = page.getByRole('dialog', { name: 'Custom Thumbnail' });
    await expect(dialog).toBeVisible();

    await checkA11y();
  });

  test('Escape closes the popup and returns focus to its button', async ({ page }) => {
    await page.goto(`/resource?id=${resourceId}`);
    await page.waitForLoadState('load');

    const opener = page.getByTestId('custom-thumbnail-open');
    await opener.click();
    await expect(page.getByRole('dialog', { name: 'Custom Thumbnail' })).toBeVisible();

    await page.keyboard.press('Escape');
    await expect(page.getByRole('dialog', { name: 'Custom Thumbnail' })).toHaveCount(0);
    // A closed dialog that leaves the reader on <body> is the defect the
    // component's own _opener exists to prevent.
    await expect(opener).toBeFocused();
  });

  test('focus stays inside the popup', async ({ page }) => {
    await page.goto(`/resource?id=${resourceId}`);
    await page.waitForLoadState('load');

    await page.getByTestId('custom-thumbnail-open').click();
    const dialog = page.getByRole('dialog', { name: 'Custom Thumbnail' });
    await expect(dialog).toBeVisible();

    const insideDialog = () =>
      page.evaluate(() => {
        const dlg = document.querySelector('[role="dialog"][aria-modal="true"]');
        return !!dlg && !!document.activeElement && dlg.contains(document.activeElement);
      });

    // Forward past the last control, then back past the first: the trap is what
    // makes either end come back inside rather than onto the page behind.
    for (let i = 0; i < 6; i += 1) {
      await page.keyboard.press('Tab');
      expect(await insideDialog()).toBe(true);
    }
    for (let i = 0; i < 6; i += 1) {
      await page.keyboard.press('Shift+Tab');
      expect(await insideDialog()).toBe(true);
    }
  });
});
