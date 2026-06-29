/**
 * E2E coverage for crop & rotate exposed inside the lightbox.
 *
 * The operations themselves are covered elsewhere (16-resource-crop,
 * rotate API tests); these specs focus on the lightbox integration:
 * in-place refresh (no full reload), keyboard/pointer isolation while the crop
 * overlay is open, and the underlying-gallery refresh when the viewer closes.
 */
import { test, expect } from '../fixtures/base.fixture';
import path from 'path';

// The lightbox root dialog, distinguished from the paste-upload / entity-picker
// dialogs AND from our own crop overlay (which is also role="dialog").
const LIGHTBOX =
  '[role="dialog"][aria-modal="true"]:not([aria-labelledby="paste-upload-title"]):not([aria-labelledby="entity-picker-title"]):not([aria-labelledby="lightbox-crop-title"])';

test.describe.serial('Lightbox crop & rotate', () => {
  let categoryId: number;
  let ownerGroupId: number;
  let runId: number;

  test.beforeAll(async ({ apiClient }) => {
    runId = Date.now();
    const category = await apiClient.createCategory(
      `Lightbox edit Category ${runId}`,
      'Category for lightbox crop/rotate tests',
    );
    categoryId = category.ID;
    const owner = await apiClient.createGroup({
      name: `Lightbox edit Owner ${runId}`,
      description: 'Owner for lightbox crop/rotate tests',
      categoryId,
    });
    ownerGroupId = owner.ID;
  });

  async function seedImage(apiClient: any, name: string, asset: string): Promise<number> {
    const r = await apiClient.createResource({
      filePath: path.join(__dirname, `../test-assets/${asset}`),
      name: `${name} ${runId}`,
      description: 'lightbox crop/rotate resource',
      ownerId: ownerGroupId,
    });
    return r.ID;
  }

  // Open the lightbox on a specific resource within the owner group's list.
  async function openLightbox(page: any, resourceId: number) {
    await page.goto(`/resources?OwnerId=${ownerGroupId}`);
    await page.waitForLoadState('load');
    const link = page.locator(`[data-lightbox-item][data-resource-id="${resourceId}"]`);
    await expect(link).toBeVisible();
    await link.click();
    await expect(page.locator(LIGHTBOX)).toBeVisible();
  }

  const currentItem = (page: any) =>
    page.evaluate(() => {
      const it = (window as any).Alpine.store('lightbox').getCurrentItem();
      return { width: it.width, height: it.height, viewUrl: it.viewUrl };
    });

  test('rotate updates the image in place (new version, swapped dimensions, no reload)', async ({ apiClient, page }) => {
    // 60×40 so a 90° rotation produces an observable dimension swap.
    const resourceId = await seedImage(apiClient, 'Rotate', 'sample-image-39.png');
    await openLightbox(page, resourceId);

    const before = await currentItem(page);
    expect(before.width).not.toBe(before.height); // precondition: non-square asset

    // Sentinel survives only if the page never reloaded.
    await page.evaluate(() => ((window as any).__noReload = true));

    const rotateBtn = page.locator(LIGHTBOX).getByRole('button', { name: 'Rotate 90 degrees clockwise' });
    await expect(rotateBtn).toBeVisible();
    await Promise.all([
      page.waitForResponse((r) => r.url().includes('/v1/resources/rotate') && r.request().method() === 'POST'),
      rotateBtn.click(),
    ]);

    // Dimensions swap once the refreshed metadata lands.
    await expect.poll(async () => (await currentItem(page)).width).toBe(before.height);
    const after = await currentItem(page);
    expect(after.height).toBe(before.width);
    expect(after.viewUrl).not.toBe(before.viewUrl); // new &v= hash busts the cache

    // Still the same SPA session (no full reload) and the lightbox stayed open.
    expect(await page.evaluate(() => (window as any).__noReload)).toBe(true);
    await expect(page.locator(LIGHTBOX)).toBeVisible();
  });

  test('crop overlay edits the image in place and keeps the lightbox open', async ({ apiClient, page }) => {
    const resourceId = await seedImage(apiClient, 'Crop', 'sample-image-2.png');
    await openLightbox(page, resourceId);
    const before = await currentItem(page);

    await page.locator(LIGHTBOX).getByRole('button', { name: 'Crop image' }).click();

    const overlay = page.locator('[data-crop-overlay]');
    await expect(overlay).toBeVisible();
    await expect(overlay).toHaveAttribute('role', 'dialog');

    // Wait until the crop image is decoded so the Crop button can enable.
    await page.waitForFunction(() => {
      const img = document.querySelector('[data-crop-overlay] img') as HTMLImageElement | null;
      return !!img && img.complete && img.naturalWidth > 0;
    });

    const submit = page.locator('[data-testid="lightbox-crop-submit-button"]');
    // No selection yet → the primary action is disabled (state conveyed to AT).
    await expect(submit).toBeDisabled();

    await page.locator('#lightbox-crop-x').fill('5');
    await page.locator('#lightbox-crop-y').fill('5');
    await page.locator('#lightbox-crop-w').fill('20');
    await page.locator('#lightbox-crop-h').fill('15');

    // Entering width & height completes the selection and enables Crop.
    await expect(submit).toBeEnabled();
    await Promise.all([
      page.waitForResponse((r) => r.url().includes('/v1/resources/crop') && r.request().method() === 'POST'),
      submit.click(),
    ]);

    // Overlay closes, lightbox stays open, image refreshes to the cropped version.
    await expect(overlay).toBeHidden();
    await expect(page.locator(LIGHTBOX)).toBeVisible();
    await expect.poll(async () => (await currentItem(page)).viewUrl).not.toBe(before.viewUrl);
    const after = await currentItem(page);
    expect(after.width).toBe(20);
    expect(after.height).toBe(15);
  });

  test('crop overlay isolates viewer keyboard shortcuts', async ({ apiClient, page }) => {
    await seedImage(apiClient, 'Iso A', 'sample-image-3.png');
    await seedImage(apiClient, 'Iso B', 'sample-image-4.png');
    await page.goto(`/resources?OwnerId=${ownerGroupId}`);
    await page.waitForLoadState('load');
    // Open the first list item so a "next" image exists to (not) navigate to.
    await page.locator('[data-lightbox-item]').first().click();
    await expect(page.locator(LIGHTBOX)).toBeVisible();

    const store = () => page.evaluate(() => {
      const s = (window as any).Alpine.store('lightbox');
      return { index: s.currentIndex, cropOpen: s.cropOpen, editPanelOpen: s.editPanelOpen, isOpen: s.isOpen };
    });

    await page.locator(LIGHTBOX).getByRole('button', { name: 'Crop image' }).click();
    await expect(page.locator('[data-crop-overlay]')).toBeVisible();
    const start = await store();
    expect(start.cropOpen).toBe(true);

    // Viewer navigation must not fire while cropping (helper-gated + explicit guards).
    await page.keyboard.press('ArrowRight');
    await page.keyboard.press('PageDown');
    // Panel shortcut must not open the Info panel.
    await page.locator('#lightbox-crop-x').focus();
    await page.keyboard.press('e');
    let mid = await store();
    expect(mid.index).toBe(start.index);
    expect(mid.cropOpen).toBe(true);
    expect(mid.editPanelOpen).toBe(false);

    // Escape closes the crop overlay first, leaving the lightbox open.
    await page.keyboard.press('Escape');
    await expect(page.locator('[data-crop-overlay]')).toBeHidden();
    const end = await store();
    expect(end.cropOpen).toBe(false);
    expect(end.isOpen).toBe(true);
    expect(end.index).toBe(start.index);
  });

  test('closing the crop overlay returns focus to the Crop button', async ({ apiClient, page }) => {
    const resourceId = await seedImage(apiClient, 'Focus', 'sample-image-37.png');
    await openLightbox(page, resourceId);

    const cropBtn = page.locator(LIGHTBOX).getByRole('button', { name: 'Crop image' });
    await cropBtn.click();
    await expect(page.locator('[data-crop-overlay]')).toBeVisible();

    await page.keyboard.press('Escape');
    await expect(page.locator('[data-crop-overlay]')).toBeHidden();

    // The modal-close contract: focus returns to the control that opened it.
    // Settle past any trap activate/deactivate timing, then assert the FINAL
    // resting element directly (not a lenient poll that could catch a transient).
    await page.waitForTimeout(250);
    const resting = await page.evaluate(() => ({
      label: document.activeElement?.getAttribute('aria-label') || '',
      tag: document.activeElement?.tagName || '',
    }));
    expect(resting.label).toBe('Crop image');
  });

  test('wheel over the crop overlay does not zoom the underlying image', async ({ apiClient, page }) => {
    const resourceId = await seedImage(apiClient, 'Wheel', 'sample-image-5.png');
    await openLightbox(page, resourceId);

    await page.locator(LIGHTBOX).getByRole('button', { name: 'Crop image' }).click();
    const overlay = page.locator('[data-crop-overlay]');
    await expect(overlay).toBeVisible();

    const zoomBefore = await page.evaluate(() => (window as any).Alpine.store('lightbox').zoomLevel);
    await overlay.hover();
    await page.mouse.wheel(0, -200);
    await page.mouse.wheel(0, -200);
    await page.waitForTimeout(150);
    const zoomAfter = await page.evaluate(() => (window as any).Alpine.store('lightbox').zoomLevel);
    expect(zoomAfter).toBe(zoomBefore);
  });

  test('closing the lightbox after a crop refreshes the underlying gallery thumbnail', async ({ apiClient, page }) => {
    const resourceId = await seedImage(apiClient, 'Refresh', 'sample-image-13.png');
    await openLightbox(page, resourceId);

    const gridLink = page.locator(`[data-lightbox-item][data-resource-id="${resourceId}"]`);
    const hashBefore = await gridLink.getAttribute('data-resource-hash');

    await page.locator(LIGHTBOX).getByRole('button', { name: 'Crop image' }).click();
    await expect(page.locator('[data-crop-overlay]')).toBeVisible();
    await page.waitForFunction(() => {
      const img = document.querySelector('[data-crop-overlay] img') as HTMLImageElement | null;
      return !!img && img.complete && img.naturalWidth > 0;
    });

    await page.locator('#lightbox-crop-x').fill('0');
    await page.locator('#lightbox-crop-y').fill('0');
    await page.locator('#lightbox-crop-w').fill('25');
    await page.locator('#lightbox-crop-h').fill('25');
    await Promise.all([
      page.waitForResponse((r) => r.url().includes('/v1/resources/crop') && r.request().method() === 'POST'),
      page.locator('[data-testid="lightbox-crop-submit-button"]').click(),
    ]);
    await expect(page.locator('[data-crop-overlay]')).toBeHidden();
    // Ensure the in-place refresh ran (sets needsRefreshOnClose) before closing.
    await page.waitForFunction(
      (h) => {
        const it = (window as any).Alpine.store('lightbox').getCurrentItem();
        return it && it.hash && it.hash !== h;
      },
      hashBefore,
    );

    await page.locator(LIGHTBOX).getByRole('button', { name: 'Close' }).click();
    await expect(page.locator(LIGHTBOX)).toBeHidden();

    // The morphed gallery item now carries the new version's hash.
    await expect.poll(async () => gridLink.getAttribute('data-resource-hash')).not.toBe(hashBefore);
  });
});
