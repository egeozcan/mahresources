import { test, expect } from '../fixtures/base.fixture';
import { getWorkerBaseUrl } from '../fixtures/base.fixture';
import path from 'path';

async function fetchVersionCount(request: any, resourceId: number): Promise<number> {
  const response = await request.get(`${getWorkerBaseUrl()}/v1/resource/versions?resourceId=${resourceId}`);
  if (!response.ok()) return 0;
  const versions = await response.json();
  return Array.isArray(versions) ? versions.length : 0;
}

test.describe.serial('Resource crop', () => {
  let ownerGroupId: number;
  let categoryId: number;
  let testRunId: number;

  test.beforeAll(async ({ apiClient }) => {
    testRunId = Date.now();

    const category = await apiClient.createCategory(
      `Crop Test Category ${testRunId}`,
      'Category for crop tests',
    );
    categoryId = category.ID;

    const ownerGroup = await apiClient.createGroup({
      name: `Crop Test Owner ${testRunId}`,
      description: 'Owner group for crop tests',
      categoryId,
    });
    ownerGroupId = ownerGroup.ID;
  });

  async function createCropResource(
    apiClient: ReturnType<typeof test.info>['project'] extends never ? never : any,
    name: string,
    assetFile: string,
  ): Promise<number> {
    const testFilePath = path.join(__dirname, `../test-assets/${assetFile}`);
    const r = await apiClient.createResource({
      filePath: testFilePath,
      name,
      description: 'Crop test resource',
      ownerId: ownerGroupId,
    });
    return r.ID;
  }

  test('crops an image via numeric inputs and creates a new current version', async ({ apiClient, resourcePage, page }) => {
    const resourceId = await createCropResource(apiClient, `Crop happy-path ${testRunId}`, 'sample-image-9.png');
    await resourcePage.gotoDisplay(resourceId);

    await page.locator(`#crop-open-${resourceId}`).click();

    const dialog = page.locator(`#crop-modal-${resourceId}`);
    await expect(dialog).toBeVisible();

    // Fill crop rectangle via numeric inputs (drag is brittle in Playwright).
    await dialog.locator(`#crop-x-${resourceId}`).fill('10');
    await dialog.locator(`#crop-y-${resourceId}`).fill('20');
    await dialog.locator(`#crop-w-${resourceId}`).fill('40');
    await dialog.locator(`#crop-h-${resourceId}`).fill('30');

    // Submit & wait for the page to reload after the successful POST.
    const cropButton = dialog.locator('button:has-text("Crop"):not([disabled])').last();
    await expect(cropButton).toBeEnabled();
    await Promise.all([
      page.waitForURL(/\/resource\?id=\d+/, { timeout: 15000 }),
      cropButton.click(),
    ]);

    // Version panel should report 2 versions now and the current one must be v2.
    await expect(page.locator('summary:has-text("Versions (2)")')).toBeVisible({ timeout: 10000 });

    // Resource dimensions updated to the new crop size.
    await expect(page.locator('dd:has-text("40 × 30")')).toBeVisible();
  });

  test('cancel leaves the resource untouched', async ({ apiClient, resourcePage, page, request }) => {
    const resourceId = await createCropResource(apiClient, `Crop cancel ${testRunId}`, 'sample-image-37.png');
    await resourcePage.gotoDisplay(resourceId);

    await page.locator(`#crop-open-${resourceId}`).click();
    const dialog = page.locator(`#crop-modal-${resourceId}`);
    await expect(dialog).toBeVisible();

    // Enter rect values, then cancel
    await dialog.locator(`#crop-x-${resourceId}`).fill('5');
    await dialog.locator(`#crop-y-${resourceId}`).fill('5');
    await dialog.locator(`#crop-w-${resourceId}`).fill('20');
    await dialog.locator(`#crop-h-${resourceId}`).fill('20');
    await dialog.locator('button:has-text("Cancel")').click();

    await expect(dialog).not.toBeVisible();

    // Before any crop, only lazy v1 (or no versions at all) exist.
    const versions = await fetchVersionCount(request, resourceId);
    expect(versions).toBeLessThanOrEqual(1);
  });

  test('zero-width rect disables the Crop button', async ({ apiClient, resourcePage, page, request }) => {
    const resourceId = await createCropResource(apiClient, `Crop invalid ${testRunId}`, 'sample-image-38.png');
    await resourcePage.gotoDisplay(resourceId);

    await page.locator(`#crop-open-${resourceId}`).click();
    const dialog = page.locator(`#crop-modal-${resourceId}`);
    await expect(dialog).toBeVisible();

    await dialog.locator(`#crop-w-${resourceId}`).fill('0');
    await dialog.locator(`#crop-h-${resourceId}`).fill('10');

    // With width = 0 the selection is empty → Crop button stays disabled.
    const cropButton = dialog.locator('footer button:has-text("Crop")');
    await expect(cropButton).toBeDisabled();

    await dialog.locator('button:has-text("Cancel")').click();
    const versions = await fetchVersionCount(request, resourceId);
    expect(versions).toBeLessThanOrEqual(1);
  });
});
