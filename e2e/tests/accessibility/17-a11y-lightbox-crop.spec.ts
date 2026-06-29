/**
 * Accessibility tests for the crop overlay exposed inside the lightbox.
 *
 * Covers: axe-core violations in the open overlay, a labelled dialog, focus
 * trapped within the overlay (Tab cannot reach the visually-covered toolbar /
 * navigation / close buttons), and Escape closing the overlay (not the viewer).
 */
import path from 'path';
import { test, expect } from '../../fixtures/a11y.fixture';

const LIGHTBOX =
  '[role="dialog"][aria-modal="true"]:not([aria-labelledby="paste-upload-title"]):not([aria-labelledby="entity-picker-title"]):not([aria-labelledby="lightbox-crop-title"])';

test.describe.serial('Lightbox crop overlay accessibility', () => {
  let ownerGroupId: number;
  let resourceId: number;
  let runId: number;

  test.beforeAll(async ({ apiClient }) => {
    runId = Date.now();
    const category = await apiClient.createCategory(
      `Lightbox crop a11y Category ${runId}`,
      'Category for lightbox crop a11y tests',
    );
    const owner = await apiClient.createGroup({
      name: `Lightbox crop a11y Owner ${runId}`,
      description: 'Owner for lightbox crop a11y tests',
      categoryId: category.ID,
    });
    ownerGroupId = owner.ID;
    const resource = await apiClient.createResource({
      filePath: path.join(__dirname, '../../test-assets/sample-image-9.png'),
      name: `Lightbox crop a11y resource ${runId}`,
      description: 'Resource used to exercise the lightbox crop overlay',
      ownerId: owner.ID,
    });
    resourceId = resource.ID;
  });

  async function openCropOverlay(page: any) {
    await page.goto(`/resources?OwnerId=${ownerGroupId}`);
    await page.waitForLoadState('load');
    await page.locator(`[data-lightbox-item][data-resource-id="${resourceId}"]`).click();
    await expect(page.locator(LIGHTBOX)).toBeVisible();
    // Wait for the viewer's own image to decode so getCurrentItem() is populated
    // and the (x-show-gated) Crop button is reliably rendered before we click it.
    await page.waitForFunction(() => {
      const img = document.querySelector(
        '[role="dialog"][aria-modal="true"] img',
      ) as HTMLImageElement | null;
      return !!img && img.complete && img.naturalWidth > 0;
    });
    const cropButton = page.locator(LIGHTBOX).getByRole('button', { name: 'Crop image' });
    await expect(cropButton).toBeVisible();
    await cropButton.click();
    const overlay = page.locator('[data-crop-overlay]');
    await expect(overlay).toBeVisible();
    // axe needs the image decoded so it can evaluate the alt text. Wait on the
    // decode signal directly — more robust under parallel-suite load than the
    // layout-sensitive "visible" heuristic.
    await page.waitForFunction(() => {
      const img = document.querySelector('[data-crop-overlay] img') as HTMLImageElement | null;
      return !!img && img.complete && img.naturalWidth > 0;
    });
    return overlay;
  }

  test('open crop overlay has no axe violations and is a labelled dialog', async ({ page, checkComponentA11y }) => {
    const overlay = await openCropOverlay(page);
    await expect(overlay).toHaveAttribute('role', 'dialog');
    await expect(overlay).toHaveAttribute('aria-modal', 'true');
    await expect(page.locator('#lightbox-crop-title')).toHaveText('Crop image');
    await checkComponentA11y('[data-crop-overlay]');
  });

  test('focus stays trapped within the crop overlay', async ({ page }) => {
    const overlay = await openCropOverlay(page);

    // x-trap moves focus into the overlay asynchronously on engage; wait for
    // that before tabbing so the first Tab isn't racing the focus handoff.
    await page.waitForFunction(() => {
      const el = document.activeElement;
      const ov = document.querySelector('[data-crop-overlay]');
      return !!el && !!ov && ov.contains(el);
    });

    // Tab through a generous number of stops; focus must never escape the overlay.
    for (let i = 0; i < 15; i++) {
      await page.keyboard.press('Tab');
      const inside = await page.evaluate(() => {
        const el = document.activeElement;
        const ov = document.querySelector('[data-crop-overlay]');
        return !!el && !!ov && ov.contains(el);
      });
      expect(inside).toBe(true);
    }
    await expect(overlay).toBeVisible();
  });

  test('Escape closes the overlay but leaves the lightbox open', async ({ page }) => {
    const overlay = await openCropOverlay(page);
    await page.keyboard.press('Escape');
    await expect(overlay).toBeHidden();
    await expect(page.locator(LIGHTBOX)).toBeVisible();
  });
});
