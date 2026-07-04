import { test, expect } from '../fixtures/base.fixture';
import { MRQLPage } from '../pages/MRQLPage';
import path from 'path';

// The default MRQL resource card renders a thumbnail for image resources.
// Clicking the thumbnail must open the lightbox in place; clicking the card
// body must still navigate to the resource detail page.
test.describe('MRQL default resource card lightbox', () => {
  let categoryId: number;
  let ownerGroupId: number;
  const createdResourceIds: number[] = [];
  const testRunId = Date.now();
  const flatQuery = `type = resource AND name ~ "*MRQL Lightbox ${testRunId}*"`;

  const lightboxDialog = (page: import('@playwright/test').Page) =>
    page.locator(
      '[role="dialog"][aria-modal="true"]:not([aria-labelledby="paste-upload-title"]):not([aria-labelledby="entity-picker-title"])'
    );

  test.beforeAll(async ({ apiClient }) => {
    const category = await apiClient.createCategory(
      `MRQL Lightbox Category ${testRunId}`,
      'Category for MRQL lightbox tests'
    );
    categoryId = category.ID;

    const ownerGroup = await apiClient.createGroup({
      name: `MRQL Lightbox Owner ${testRunId}`,
      description: 'Owner for MRQL lightbox test resources',
      categoryId: categoryId,
    });
    ownerGroupId = ownerGroup.ID;

    const testImageFiles = [
      path.join(__dirname, '../test-assets/sample-image-20.png'),
      path.join(__dirname, '../test-assets/sample-image-21.png'),
    ];
    for (let i = 0; i < testImageFiles.length; i++) {
      const resource = await apiClient.createResource({
        filePath: testImageFiles[i],
        name: `MRQL Lightbox ${testRunId} Image ${i + 1}`,
        description: `Test image ${i + 1} for MRQL lightbox`,
        ownerId: ownerGroupId,
      });
      createdResourceIds.push(resource.ID);
    }
  });

  test.afterAll(async ({ apiClient }) => {
    for (const resourceId of createdResourceIds) {
      try {
        await apiClient.deleteResource(resourceId);
      } catch {
        // Ignore errors during cleanup
      }
    }
    if (ownerGroupId) {
      await apiClient.deleteGroup(ownerGroupId);
    }
    if (categoryId) {
      await apiClient.deleteCategory(categoryId);
    }
  });

  test('clicking a result thumbnail opens the lightbox without navigating', async ({ page }) => {
    const mrql = new MRQLPage(page);
    await mrql.navigate();
    await mrql.enterQuery(flatQuery);
    await mrql.executeQuery();

    const thumbnail = mrql.resultsSection.locator('[data-lightbox-item]').first();
    await expect(thumbnail).toBeVisible();
    await thumbnail.click();

    const lightbox = lightboxDialog(page);
    await expect(lightbox).toBeVisible();
    await expect(lightbox.locator('img').first()).toBeVisible();
    await expect(page).toHaveURL(/\/mrql/);

    // Escape closes the lightbox. (Focus restore to the trigger is not
    // asserted: close()'s synchronous focus() races x-trap's async release,
    // which is flaky under suite load; no other lightbox spec asserts it.)
    await page.keyboard.press('Escape');
    await expect(lightbox).toBeHidden();
  });

  test('lightbox navigates between multiple MRQL results', async ({ page }) => {
    const mrql = new MRQLPage(page);
    await mrql.navigate();
    await mrql.enterQuery(flatQuery);
    await mrql.executeQuery();

    const thumbnails = mrql.resultsSection.locator('[data-lightbox-item]');
    await expect(thumbnails).toHaveCount(2);
    await thumbnails.first().click();

    const lightbox = lightboxDialog(page);
    await expect(lightbox).toBeVisible();

    await page.keyboard.press('ArrowRight');
    await expect(lightbox).toBeVisible();
    await expect(lightbox.locator('img').first()).toBeVisible();
  });

  test('clicking the card body still navigates to the resource page', async ({ page }) => {
    const mrql = new MRQLPage(page);
    await mrql.navigate();
    await mrql.enterQuery(flatQuery);
    await mrql.executeQuery();

    const cardBody = mrql.resultsSection.locator('a[href^="/resource?id="]').first();
    await expect(cardBody).toBeVisible();
    await cardBody.click();

    await page.waitForURL(/\/resource\?id=\d+/);
  });

  test('bucketed GROUP BY thumbnails open the lightbox', async ({ page }) => {
    const mrql = new MRQLPage(page);
    await mrql.navigate();
    await mrql.enterQuery(`${flatQuery} GROUP BY contentType`);
    await mrql.executeQuery();

    // Bucketed mode: heading mentions groups, thumbnails live inside bucket grids
    await expect(mrql.resultsSection.locator('h2')).toContainText('groups');

    const thumbnail = mrql.resultsSection.locator('[data-lightbox-item]').first();
    await expect(thumbnail).toBeVisible();
    await thumbnail.click();

    const lightbox = lightboxDialog(page);
    await expect(lightbox).toBeVisible();
    await expect(page).toHaveURL(/\/mrql/);
  });
});
