import { Page, expect } from '@playwright/test';

/**
 * The image actions popup on a resource detail page.
 *
 * Recalculate Dimensions, Rotate and Crop moved out of the sidebar and into a
 * dialog behind one **Image Actions…** button, so reaching Crop is two steps now
 * and every spec that drove the crop dialog directly has to say so. The helper
 * exists so the two steps are written once: a spec that clicks `#crop-open-<id>`
 * itself works until that button stops being in the document until the popup is
 * open, and then it fails as a timeout rather than as anything that looks like
 * the change it is.
 */

export async function openImageActions(page: Page) {
  await page.getByTestId('image-actions-open').click();
  const popup = page.getByRole('dialog', { name: 'Image Actions' });
  await expect(popup).toBeVisible();
  return popup;
}

/**
 * Open the native crop dialog, the way a reader does: image actions, then Crop.
 * Returns the `<dialog>` locator.
 */
export async function openCropDialog(page: Page, resourceId: number) {
  const popup = await openImageActions(page);
  await popup.locator(`#crop-open-${resourceId}`).click();
  const dialog = page.locator(`#crop-modal-${resourceId}`);
  await expect(dialog).toBeVisible();
  return dialog;
}
