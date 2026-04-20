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

  test('aspect preset stays locked when the rect hits an image edge', async ({ apiClient, resourcePage, page }) => {
    const resourceId = await createCropResource(apiClient, `Crop aspect ${testRunId}`, 'sample-image-39.png'); // 60×40
    await resourcePage.gotoDisplay(resourceId);

    await page.locator(`#crop-open-${resourceId}`).click();
    const dialog = page.locator(`#crop-modal-${resourceId}`);
    await expect(dialog).toBeVisible();

    // Lock to 1:1 and request a rect whose naive clamp would crop the width
    // and height independently (producing a non-square rect).
    await dialog.locator(`#crop-aspect-${resourceId}`).selectOption('1:1');
    await dialog.locator(`#crop-x-${resourceId}`).fill('40');
    await dialog.locator(`#crop-y-${resourceId}`).fill('10');
    await dialog.locator(`#crop-w-${resourceId}`).fill('30');
    await dialog.locator(`#crop-h-${resourceId}`).fill('30');

    // The clamp must preserve the 1:1 aspect, so W and H end up equal.
    const widthValue = await dialog.locator(`#crop-w-${resourceId}`).inputValue();
    const heightValue = await dialog.locator(`#crop-h-${resourceId}`).inputValue();
    expect(Number(widthValue)).toBeGreaterThan(0);
    expect(Number(widthValue)).toBe(Number(heightValue));
    // Both must fit inside the 60×40 image.
    expect(40 + Number(widthValue)).toBeLessThanOrEqual(60);
    expect(10 + Number(heightValue)).toBeLessThanOrEqual(40);

    await dialog.locator('button:has-text("Cancel")').click();
  });

  test('locked aspect: editing height also drives width', async ({ apiClient, resourcePage, page }) => {
    const resourceId = await createCropResource(apiClient, `Crop h-driver ${testRunId}`, 'sample-image-2.png');
    await resourcePage.gotoDisplay(resourceId);

    await page.locator(`#crop-open-${resourceId}`).click();
    const dialog = page.locator(`#crop-modal-${resourceId}`);
    await expect(dialog).toBeVisible();

    await dialog.locator(`#crop-aspect-${resourceId}`).selectOption('1:1');
    await dialog.locator(`#crop-x-${resourceId}`).fill('0');
    await dialog.locator(`#crop-y-${resourceId}`).fill('0');
    await dialog.locator(`#crop-w-${resourceId}`).fill('40');
    // Now drive from height — it should stick, and width should follow.
    await dialog.locator(`#crop-h-${resourceId}`).fill('15');

    const widthValue = await dialog.locator(`#crop-w-${resourceId}`).inputValue();
    const heightValue = await dialog.locator(`#crop-h-${resourceId}`).inputValue();
    expect(Number(heightValue)).toBe(15);
    expect(Number(widthValue)).toBe(15);

    await dialog.locator('button:has-text("Cancel")').click();
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
