import { test, expect } from '../fixtures/base.fixture';
import path from 'path';

test.describe.serial('Auto-select compare versions', () => {
  let categoryId: number;
  let ownerGroupId: number;
  let resourceId: number;
  let testRunId: number;

  test.beforeAll(async ({ apiClient, request, baseURL }) => {
    testRunId = Date.now() + Math.floor(Math.random() * 100000);

    const category = await apiClient.createCategory(
      `AutoCompare Category ${testRunId}`,
      'Category for auto-compare tests'
    );
    categoryId = category.ID;

    const ownerGroup = await apiClient.createGroup({
      name: `AutoCompare Owner ${testRunId}`,
      categoryId: categoryId,
    });
    ownerGroupId = ownerGroup.ID;

    // Create resource with v1
    const testFile1 = path.join(__dirname, '../test-assets/sample-image-10.png');
    const resource = await apiClient.createResource({
      filePath: testFile1,
      name: `AutoCompare Resource ${testRunId}`,
      ownerId: ownerGroupId,
    });
    resourceId = resource.ID;

    // Upload v2 so there are exactly 2 versions
    const fs = await import('fs');
    const testFile2 = path.join(__dirname, '../test-assets/sample-image-11.png');
    const fileBuffer = fs.readFileSync(testFile2);
    const response = await request.post(`${baseURL}/v1/resource/versions?resourceId=${resourceId}`, {
      multipart: {
        file: { name: 'sample-image-11.png', mimeType: 'image/png', buffer: fileBuffer },
        comment: 'Second version for auto-compare test',
      },
    });
    expect(response.ok()).toBeTruthy();
  });

  test('clicking Compare with exactly 2 versions immediately shows Compare Selected', async ({ resourcePage, page }) => {
    expect(resourceId).toBeGreaterThan(0);
    await resourcePage.gotoDisplay(resourceId);

    // Verify exactly 2 versions exist
    await expect(page.locator('summary:has-text("Versions (2)")')).toBeVisible({ timeout: 10000 });

    // Version panel should be auto-expanded (2 versions triggers open)
    const versionContent = page.locator('details:has(summary:has-text("Versions")) .detail-panel-body');
    await expect(versionContent).toBeVisible({ timeout: 5000 });

    // Click Compare button
    const compareButton = page.locator('button:has-text("Compare")').first();
    await expect(compareButton).toBeVisible({ timeout: 5000 });
    await compareButton.click();

    // "Compare Selected" link should appear immediately without any checkbox interaction
    const compareSelectedLink = page.locator('a:has-text("Compare Selected")');
    await expect(compareSelectedLink).toBeVisible({ timeout: 3000 });

    // Verify the link has both v1 and v2
    const href = await compareSelectedLink.getAttribute('href');
    expect(href).toContain('/resource/compare');
    expect(href).toContain(`r1=${resourceId}`);
    expect(href).toMatch(/v1=\d+/);
    expect(href).toMatch(/v2=\d+/);

    // The two version numbers should be different
    const v1Match = href?.match(/v1=(\d+)/);
    const v2Match = href?.match(/v2=(\d+)/);
    expect(v1Match?.[1]).not.toEqual(v2Match?.[1]);
  });

  test('clicking Cancel Compare clears auto-selection', async ({ resourcePage, page }) => {
    expect(resourceId).toBeGreaterThan(0);
    await resourcePage.gotoDisplay(resourceId);

    const compareButton = page.locator('button:has-text("Compare")').first();
    await expect(compareButton).toBeVisible({ timeout: 5000 });

    // Enter compare mode (auto-selects both)
    await compareButton.click();
    await expect(page.locator('a:has-text("Compare Selected")')).toBeVisible({ timeout: 3000 });

    // Cancel compare mode
    await page.locator('button:has-text("Cancel Compare")').click();

    // Compare Selected link should disappear
    await expect(page.locator('a:has-text("Compare Selected")')).not.toBeVisible();
  });

  test('resources with more than 2 versions do not auto-select', async ({ resourcePage, page, request, baseURL }) => {
    expect(resourceId).toBeGreaterThan(0);

    // Upload a third version
    const fs = await import('fs');
    const testFile3 = path.join(__dirname, '../test-assets/sample-image-12.png');
    const fileBuffer = fs.readFileSync(testFile3);
    const response = await request.post(`${baseURL}/v1/resource/versions?resourceId=${resourceId}`, {
      multipart: {
        file: { name: 'sample-image-12.png', mimeType: 'image/png', buffer: fileBuffer },
        comment: 'Third version to test no auto-select',
      },
    });
    expect(response.ok()).toBeTruthy();

    await resourcePage.gotoDisplay(resourceId);

    // Verify exactly 3 versions
    await expect(page.locator('summary:has-text("Versions (3)")')).toBeVisible({ timeout: 10000 });

    const compareButton = page.locator('button:has-text("Compare")').first();
    await expect(compareButton).toBeVisible({ timeout: 5000 });
    await compareButton.click();

    // With 3 versions, Compare Selected should NOT appear automatically
    await page.waitForTimeout(500);
    await expect(page.locator('a:has-text("Compare Selected")')).not.toBeVisible();
  });

  test.afterAll(async ({ apiClient }) => {
    if (resourceId) {
      try { await apiClient.deleteResource(resourceId); } catch { /* ignore */ }
    }
    if (ownerGroupId) {
      try { await apiClient.deleteGroup(ownerGroupId); } catch { /* ignore */ }
    }
    if (categoryId) {
      try { await apiClient.deleteCategory(categoryId); } catch { /* ignore */ }
    }
  });
});
