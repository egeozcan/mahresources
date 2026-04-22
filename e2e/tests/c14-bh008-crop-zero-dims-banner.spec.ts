/**
 * BH-008: crop selection overlay was invisible when the underlying image
 * could not be decoded in the browser (naturalWidth/Height === 0 or an
 * onerror event). The Crop button stayed enabled and a confused user would
 * submit a nonsense rect that the server had to reject.
 *
 * Fix surface: an explicit "cannot be decoded" banner appears, the overlay
 * is hidden, and the Crop button is disabled whenever the modal detects a
 * decode failure.
 */
import { test, expect } from '../fixtures/base.fixture';
import path from 'path';

test.describe('BH-008: crop modal surfaces decode failures', () => {
  test('banner appears + Crop disabled when image fails to decode', async ({ page, apiClient }) => {
    const testRunId = Date.now();

    const category = await apiClient.createCategory(
      `BH-008 Category ${testRunId}`,
      'Category for BH-008 crop-zero-dims test',
    );
    const ownerGroup = await apiClient.createGroup({
      name: `BH-008 Owner ${testRunId}`,
      description: 'Owner for BH-008 crop-zero-dims test',
      categoryId: category.ID,
    });

    // Upload a normal PNG so we have a real resource with a working crop
    // modal. We'll then break the <img> ref client-side to force the
    // decode-failed path — this is the realistic scenario (corrupt file,
    // zero-dim SVG, etc.) without needing server-side fixture gymnastics.
    const resource = await apiClient.createResource({
      filePath: path.join(__dirname, '../test-assets/sample-image-9.png'),
      name: `BH-008 Decode-Failed Harness ${testRunId}`,
      description: 'BH-008 zero-dim crop test',
      ownerId: ownerGroup.ID,
    });

    await page.goto(`/resource?id=${resource.ID}`);
    await page.locator(`#crop-open-${resource.ID}`).click();

    const dialog = page.locator(`#crop-modal-${resource.ID}`);
    await expect(dialog).toBeVisible();

    // Force the decode-failed state by dispatching 'error' on the <img>.
    // This mirrors what the browser does for a broken/corrupt file or a
    // zero-dim SVG once BH-039 accepts them server-side.
    await page.evaluate((id) => {
      const modal = document.getElementById(`crop-modal-${id}`);
      const img = modal?.querySelector('img') as HTMLImageElement | null;
      if (img) {
        img.dispatchEvent(new Event('error'));
      }
    }, resource.ID);

    const banner = dialog.locator('[data-testid="crop-decode-failed-banner"]');
    await expect(banner).toBeVisible();
    await expect(banner).toContainText(/could not be decoded|cropping is unavailable/i);

    const cropBtn = dialog.locator('[data-testid="crop-submit-button"]');
    await expect(cropBtn).toBeDisabled();

    // Cleanup best-effort.
    try { await apiClient.deleteResource(resource.ID); } catch { /* ignore */ }
    try { await apiClient.deleteGroup(ownerGroup.ID); } catch { /* ignore */ }
    try { await apiClient.deleteCategory(category.ID); } catch { /* ignore */ }
  });
});
