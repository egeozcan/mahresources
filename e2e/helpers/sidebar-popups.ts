/**
 * The sidebar popups on a resource detail page.
 *
 * Recalculate Dimensions, Rotate, Crop, Trim and the custom-thumbnail controls
 * all moved out of the sidebar and behind a button, so reaching any of them is
 * two steps where it used to be one. Every spec that drove one of them
 * directly has to say so, and the helper exists so the two steps are written
 * once: a spec that clicks `#crop-open-<id>` itself works until that button
 * stops being in the document until its popup is open, and then it fails as a
 * timeout rather than as anything that looks like the change it is.
 */
import { Page, expect } from '@playwright/test';

export async function openImageActions(page: Page) {
  await page.getByTestId('image-actions-open').click();
  const popup = page.getByRole('dialog', { name: 'Image Actions' });
  await expect(popup).toBeVisible();
  return popup;
}

export async function openVideoActions(page: Page) {
  await page.getByTestId('video-actions-open').click();
  const popup = page.getByRole('dialog', { name: 'Video Actions' });
  await expect(popup).toBeVisible();
  return popup;
}

export async function openCustomThumbnail(page: Page) {
  await page.getByTestId('custom-thumbnail-open').click();
  const popup = page.getByRole('dialog', { name: 'Custom Thumbnail' });
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
