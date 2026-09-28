/**
 * Accessibility tests for the Image Actions popup.
 *
 * Recalculate Dimensions, Rotate and Crop left the sidebar for a dialog behind
 * one button, on the same rules as the Custom Thumbnail popup next to it. The
 * case here that the other modals do not have is the hand-off: Crop opens the
 * native `<dialog>` crop UI, so the two must never be open together, and the
 * reader must land back on the Image Actions button when the crop dialog closes
 * — `<dialog>` restores focus to whatever was focused when it opened, and the
 * Crop… button that opened it is inside the popup that has just been removed.
 */
import path from 'path';
import { test, expect } from '../../fixtures/a11y.fixture';
import { openImageActions, openCropDialog } from '../../helpers/sidebar-popups';

test.describe.serial('Image actions popup accessibility', () => {
  let resourceId: number;
  let runId: number;

  test.beforeAll(async ({ apiClient }) => {
    runId = Date.now();

    const category = await apiClient.createCategory(
      `Image actions a11y Category ${runId}`,
      'Category for image actions a11y tests',
    );
    const owner = await apiClient.createGroup({
      name: `Image actions a11y Owner ${runId}`,
      description: 'Owner for image actions a11y tests',
      categoryId: category.ID,
    });
    const resource = await apiClient.createResource({
      filePath: path.join(__dirname, '../../test-assets/sample-image-9.png'),
      name: `Image actions a11y resource ${runId}`,
      description: 'Resource used to exercise the image actions popup',
      ownerId: owner.ID,
    });
    resourceId = resource.ID;
  });

  test('the closed sidebar shows one button, not the three actions', async ({ page }) => {
    await page.goto(`/resource?id=${resourceId}`);
    await page.waitForLoadState('load');

    await expect(page.getByTestId('image-actions-open')).toBeVisible();
    await expect(page.getByTestId('image-actions-recalculate')).toHaveCount(0);
    await expect(page.getByTestId('image-actions-rotate')).toHaveCount(0);
    await expect(page.getByTestId('image-actions-crop')).toHaveCount(0);
    await expect(page.getByRole('dialog')).toHaveCount(0);
  });

  test('the open popup has no axe violations', async ({ page, checkA11y }) => {
    await page.goto(`/resource?id=${resourceId}`);
    await page.waitForLoadState('load');

    await openImageActions(page);

    await checkA11y();
  });

  test('Escape closes the popup and returns focus to its button', async ({ page }) => {
    await page.goto(`/resource?id=${resourceId}`);
    await page.waitForLoadState('load');

    const opener = page.getByTestId('image-actions-open');
    await opener.click();
    await expect(page.getByRole('dialog', { name: 'Image Actions' })).toBeVisible();

    await page.keyboard.press('Escape');
    await expect(page.getByRole('dialog', { name: 'Image Actions' })).toHaveCount(0);
    await expect(opener).toBeFocused();
  });

  test('focus stays inside the popup', async ({ page }) => {
    await page.goto(`/resource?id=${resourceId}`);
    await page.waitForLoadState('load');

    await openImageActions(page);

    const insideDialog = () =>
      page.evaluate(() => {
        const dlg = document.querySelector('[role="dialog"][aria-modal="true"]');
        return !!dlg && !!document.activeElement && dlg.contains(document.activeElement);
      });

    // Four controls (close, recalculate, rotate, crop); past the last and back
    // past the first in both directions, which is where the trap shows.
    for (let i = 0; i < 6; i += 1) {
      await page.keyboard.press('Tab');
      expect(await insideDialog()).toBe(true);
    }
    for (let i = 0; i < 6; i += 1) {
      await page.keyboard.press('Shift+Tab');
      expect(await insideDialog()).toBe(true);
    }
  });

  test('Crop hands off to the crop dialog rather than stacking two dialogs', async ({ page }) => {
    await page.goto(`/resource?id=${resourceId}`);
    await page.waitForLoadState('load');

    await openCropDialog(page, resourceId);

    // The Image Actions popup is gone, and it is gone before the crop dialog
    // opened — two aria-modal dialogs at once is the defect either way it paints.
    await expect(page.getByRole('dialog', { name: 'Image Actions' })).toHaveCount(0);
    await expect(page.locator(`#crop-modal-${resourceId}`)).toBeVisible();
  });

  test('closing the crop dialog returns focus to the Image Actions button', async ({ page }) => {
    await page.goto(`/resource?id=${resourceId}`);
    await page.waitForLoadState('load');

    const opener = page.getByTestId('image-actions-open');
    const dialog = await openCropDialog(page, resourceId);
    await dialog.locator('button:has-text("Cancel")').click();
    await expect(dialog).not.toBeVisible();

    // <dialog> restores focus on close to whatever was focused when it opened.
    // That is the Crop… button inside the popup that no longer exists, unless
    // the hand-off parked focus on the sidebar button first.
    await expect(opener).toBeFocused();
  });
});
