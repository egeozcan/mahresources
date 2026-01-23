import { test, expect } from '../fixtures/base.fixture';
import path from 'path';

test.describe('Lightbox Functionality', () => {
  let categoryId: number;
  let ownerGroupId: number;
  const createdResourceIds: number[] = [];
  const testRunId = Date.now();

  test.beforeAll(async ({ apiClient }) => {
    // Create prerequisite data
    const category = await apiClient.createCategory(
      `Lightbox Test Category ${testRunId}`,
      'Category for lightbox tests'
    );
    categoryId = category.ID;

    const ownerGroup = await apiClient.createGroup({
      name: `Lightbox Test Owner ${testRunId}`,
      description: 'Owner for lightbox test resources',
      categoryId: categoryId,
    });
    ownerGroupId = ownerGroup.ID;

    // Create multiple image resources for testing - use unique images to avoid duplicate detection
    const testImageFiles = [
      path.join(__dirname, '../test-assets/sample-image.png'),
      path.join(__dirname, '../test-assets/sample-image-2.png'),
      path.join(__dirname, '../test-assets/sample-image-3.png'),
      path.join(__dirname, '../test-assets/sample-image-4.png'),
      path.join(__dirname, '../test-assets/sample-image-5.png'),
    ];
    for (let i = 0; i < testImageFiles.length; i++) {
      const resource = await apiClient.createResource({
        filePath: testImageFiles[i],
        name: `Lightbox Test Image ${i + 1} - ${testRunId}`,
        description: `Test image ${i + 1} for lightbox`,
        ownerId: ownerGroupId,
      });
      createdResourceIds.push(resource.ID);
    }
  });

  test.afterAll(async ({ apiClient }) => {
    // Clean up resources
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

  test('should open lightbox when clicking an image', async ({ page }) => {
    await page.goto('/resources');
    await page.waitForLoadState('load');

    // Find a lightbox-enabled image link and click it
    const imageLink = page.locator('[data-lightbox-item]').first();
    await expect(imageLink).toBeVisible();
    await imageLink.click();

    // Verify lightbox opened
    const lightbox = page.locator('[role="dialog"][aria-modal="true"]');
    await expect(lightbox).toBeVisible();

    // Verify image is displayed
    const lightboxImage = lightbox.locator('img');
    await expect(lightboxImage).toBeVisible();
  });

  test('should hide loading spinner after image loads (cached media fix)', async ({ page }) => {
    await page.goto('/resources');
    await page.waitForLoadState('load');

    // Click to open lightbox
    const imageLink = page.locator('[data-lightbox-item]').first();
    await imageLink.click();

    // Wait for lightbox to open
    const lightbox = page.locator('[role="dialog"][aria-modal="true"]');
    await expect(lightbox).toBeVisible();

    // Wait for image to actually load in the DOM
    const lightboxImage = lightbox.locator('img');
    await expect(lightboxImage).toBeVisible();

    // Wait for image to fully load (naturalWidth > 0 indicates loaded)
    await page.waitForFunction(
      () => {
        const img = document.querySelector('[role="dialog"] img');
        return img && (img as HTMLImageElement).complete && (img as HTMLImageElement).naturalWidth > 0;
      },
      { timeout: 5000 }
    );

    // Give Alpine time to react to state changes
    await page.waitForTimeout(500);

    // Verify loading spinner is hidden after image loads
    const loadingSpinner = lightbox.locator('svg.animate-spin');
    await expect(loadingSpinner).toBeHidden({ timeout: 5000 });
  });

  test('should navigate between images using arrow buttons', async ({ page }) => {
    await page.goto('/resources');
    await page.waitForLoadState('load');

    // Open lightbox on first image
    const imageLink = page.locator('[data-lightbox-item]').first();
    await imageLink.click();

    const lightbox = page.locator('[role="dialog"][aria-modal="true"]');
    await expect(lightbox).toBeVisible();

    // Get initial counter
    const counter = lightbox.locator('div.bg-black\\/50').first();
    await expect(counter).toContainText('1');

    // Click next button
    const nextButton = lightbox.locator('button[aria-label="Next"]');
    await nextButton.click();

    // Wait for navigation
    await page.waitForTimeout(300);

    // Verify counter updated
    await expect(counter).toContainText('2');
  });

  test('should navigate using keyboard arrows', async ({ page }) => {
    await page.goto('/resources');
    await page.waitForLoadState('load');

    // Open lightbox
    const imageLink = page.locator('[data-lightbox-item]').first();
    await imageLink.click();

    const lightbox = page.locator('[role="dialog"][aria-modal="true"]');
    await expect(lightbox).toBeVisible();

    // Navigate with right arrow
    await page.keyboard.press('ArrowRight');
    await page.waitForTimeout(300);

    const counter = lightbox.locator('div.bg-black\\/50').first();
    await expect(counter).toContainText('2');

    // Navigate back with left arrow
    await page.keyboard.press('ArrowLeft');
    await page.waitForTimeout(300);
    await expect(counter).toContainText('1');
  });

  test('should close lightbox with Escape key', async ({ page }) => {
    await page.goto('/resources');
    await page.waitForLoadState('load');

    // Open lightbox
    const imageLink = page.locator('[data-lightbox-item]').first();
    await imageLink.click();

    const lightbox = page.locator('[role="dialog"][aria-modal="true"]');
    await expect(lightbox).toBeVisible();

    // Press Escape
    await page.keyboard.press('Escape');

    // Verify lightbox closed
    await expect(lightbox).toBeHidden();
  });

  test('should close lightbox when clicking backdrop', async ({ page }) => {
    await page.goto('/resources');
    await page.waitForLoadState('load');

    // Open lightbox
    const imageLink = page.locator('[data-lightbox-item]').first();
    await imageLink.click();

    const lightbox = page.locator('[role="dialog"][aria-modal="true"]');
    await expect(lightbox).toBeVisible();

    // Click backdrop (the dark overlay)
    const backdrop = lightbox.locator('div.bg-black\\/90');
    await backdrop.click({ position: { x: 10, y: 10 } });

    // Verify lightbox closed
    await expect(lightbox).toBeHidden();
  });

  test('should show details link that navigates to resource page', async ({ page }) => {
    await page.goto('/resources');
    await page.waitForLoadState('load');

    // Open lightbox
    const imageLink = page.locator('[data-lightbox-item]').first();
    await imageLink.click();

    const lightbox = page.locator('[role="dialog"][aria-modal="true"]');
    await expect(lightbox).toBeVisible();

    // Find and click Details link
    const detailsLink = lightbox.locator('a:has-text("Details")');
    await expect(detailsLink).toBeVisible();

    // Click and verify navigation
    await detailsLink.click();
    await page.waitForLoadState('load');

    // Should be on resource detail page
    expect(page.url()).toContain('/resource?id=');
  });
});

test.describe('Lightbox Pagination Data Attributes', () => {
  test('pagination nav should have data-has-next and data-has-prev attributes', async ({ page }) => {
    // This test relies on resources created by earlier tests (Lightbox Functionality's beforeAll creates 5 resources)
    // With 5+ resources and pageSize=2, we should have pagination
    await page.goto('/resources?pageSize=2');
    await page.waitForLoadState('load');

    // Check pagination nav exists and has correct data attributes on page 1
    const paginationNav = page.locator('nav[aria-label="Pagination"]');
    await expect(paginationNav).toBeVisible({ timeout: 5000 });

    // On page 1 with 5+ items and pageSize=2, should have next but not prev
    await expect(paginationNav).toHaveAttribute('data-has-prev', 'false');
    await expect(paginationNav).toHaveAttribute('data-has-next', 'true');

    // Navigate to page 2
    await page.goto('/resources?pageSize=2&page=2');
    await page.waitForLoadState('load');

    // Refresh the locator for the new page
    const paginationNav2 = page.locator('nav[aria-label="Pagination"]');
    await expect(paginationNav2).toBeVisible({ timeout: 5000 });

    // On page 2 with 5+ items and pageSize=2, should have both prev and next
    await expect(paginationNav2).toHaveAttribute('data-has-prev', 'true');
    // Note: with exactly 5 items and pageSize=2, page 2 has items 3-4, page 3 would have item 5
    // So hasNext should be true on page 2
    await expect(paginationNav2).toHaveAttribute('data-has-next', 'true');
  });

  test('pagination links should have data-pagination-prev and data-pagination-next attributes', async ({ page }) => {
    // This test relies on resources created by earlier tests (Lightbox Functionality's beforeAll creates 5 resources)
    // Navigate to page 2 where both prev and next should exist (with 5+ resources and pageSize=1)
    await page.goto('/resources?pageSize=1&page=2');
    await page.waitForLoadState('load');

    // Check prev link has data attribute
    const prevLink = page.locator('a[data-pagination-prev]');
    await expect(prevLink).toBeVisible();

    // Check next link has data attribute
    const nextLink = page.locator('a[data-pagination-next]');
    await expect(nextLink).toBeVisible();
  });
});

test.describe('Lightbox Loading State', () => {
  test('loading spinner should disappear when navigating between cached images', async ({ page }) => {
    // This test relies on resources created by earlier tests (Lightbox Functionality's beforeAll creates 5 resources)
    await page.goto('/resources');
    await page.waitForLoadState('load');

    // Open lightbox
    const imageLink = page.locator('[data-lightbox-item]').first();
    await imageLink.click();

    const lightbox = page.locator('[role="dialog"][aria-modal="true"]');
    await expect(lightbox).toBeVisible();

    // Wait for first image to load
    await page.waitForTimeout(500);

    // Navigate to next image
    await page.keyboard.press('ArrowRight');

    // Wait a bit for navigation and loading
    await page.waitForTimeout(500);

    // Spinner should be hidden after image loads
    const loadingSpinner = lightbox.locator('svg.animate-spin');
    await expect(loadingSpinner).toBeHidden();

    // Navigate back
    await page.keyboard.press('ArrowLeft');
    await page.waitForTimeout(500);

    // Spinner should still be hidden (cached image)
    await expect(loadingSpinner).toBeHidden();
  });
});
