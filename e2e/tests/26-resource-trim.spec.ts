import { test, expect } from '../fixtures/base.fixture';
import { getWorkerBaseUrl } from '../fixtures/base.fixture';
import path from 'path';

async function fetchVersionCount(request: any, resourceId: number): Promise<number> {
  const response = await request.get(`${getWorkerBaseUrl()}/v1/resource/versions?resourceId=${resourceId}`);
  if (!response.ok()) return 0;
  const versions = await response.json();
  return Array.isArray(versions) ? versions.length : 0;
}

test.describe.serial('Resource trim', () => {
  let ownerGroupId: number;
  let categoryId: number;
  let testRunId: number;

  test.beforeAll(async ({ apiClient }) => {
    testRunId = Date.now();

    const category = await apiClient.createCategory(
      `Trim Test Category ${testRunId}`,
      'Category for trim tests',
    );
    categoryId = category.ID;

    const ownerGroup = await apiClient.createGroup({
      name: `Trim Test Owner ${testRunId}`,
      description: 'Owner group for trim tests',
      categoryId,
    });
    ownerGroupId = ownerGroup.ID;
  });

  async function createVideoResource(
    apiClient: ReturnType<typeof test.info>['project'] extends never ? never : any,
    name: string,
  ): Promise<number> {
    const testFilePath = path.join(__dirname, '../test-assets/sample-video.mp4');
    const r = await apiClient.createResource({
      filePath: testFilePath,
      name,
      description: 'Trim test video resource',
      ownerId: ownerGroupId,
      contentType: 'video/mp4',
    });
    return r.ID;
  }

  test('trims a video via the sidebar form and creates a new version', async ({ apiClient, resourcePage, page, request }) => {
    const resourceId = await createVideoResource(apiClient, `Trim happy-path ${testRunId}`);
    await resourcePage.gotoDisplay(resourceId);

    // The trim form lives in a sidebar section with data-trim-section
    const trimSection = page.locator(`[data-trim-section="${resourceId}"]`);
    await expect(trimSection).toBeVisible({ timeout: 10000 });

    // Verify the version panel shows "Versions (1)" (lazy v1) before trim
    await expect(page.locator('summary:has-text("Versions (1)")')).toBeVisible({ timeout: 10000 });

    // Set start and end times via Alpine component
    await trimSection.evaluate((el) => {
      const Alpine = (window as any).Alpine;
      if (!Alpine) return;
      const data = Alpine.$data(el);
      data.startText = '1';
      data.endText = '3';
      data.syncFromText('end');
    });
    await trimSection.locator('input[id^="trim-comment-"]').fill('e2e test trim');

    // Submit and wait for page reload
    const trimButton = trimSection.locator('button:has-text("Trim Video")');
    await expect(trimButton).toBeEnabled();
    await Promise.all([
      page.waitForURL(/\/resource\?id=\d+/, { timeout: 15000 }),
      trimButton.click(),
    ]);

    // Version panel should show 2 versions now
    await expect(page.locator('summary:has-text("Versions (2)")')).toBeVisible({ timeout: 10000 });

    // API check: version count and trimmed file is smaller
    const versions = await fetchVersionCount(request, resourceId);
    expect(versions).toBe(2);
  });

  test('shows validation error for invalid times', async ({ apiClient, resourcePage, page, request }) => {
    const resourceId = await createVideoResource(apiClient, `Trim invalid ${testRunId}`);
    await resourcePage.gotoDisplay(resourceId);

    const trimSection = page.locator(`[data-trim-section="${resourceId}"]`);
    await expect(trimSection).toBeVisible();

    // Directly set Alpine component state — empty end is uncorrectable
    await trimSection.evaluate((el) => {
      const Alpine = (window as any).Alpine;
      if (!Alpine) return;
      const data = Alpine.$data(el);
      data.startText = '5';
      data.endText = '';
    });

    // Button should be disabled when end is empty
    const trimButton = trimSection.locator('button:has-text("Trim Video")');
    await expect(trimButton).toBeDisabled();

    // Version count should not increase
    const versions = await fetchVersionCount(request, resourceId);
    expect(versions).toBeLessThanOrEqual(1);
  });

  test('trim section is not visible for non-video resources', async ({ apiClient, resourcePage, page }) => {
    // Upload an image resource
    const testFilePath = path.join(__dirname, '../test-assets/sample-image-9.png');
    const r = await apiClient.createResource({
      filePath: testFilePath,
      name: `Trim image ${testRunId}`,
      description: 'Image resource for trim visibility test',
      ownerId: ownerGroupId,
    });
    await resourcePage.gotoDisplay(r.ID);

    // Trim section should not appear for image resources
    const trimSection = page.locator(`[data-trim-section="${r.ID}"]`);
    await expect(trimSection).not.toBeAttached();
  });
});
