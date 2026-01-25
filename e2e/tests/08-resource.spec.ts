import { test, expect } from '../fixtures/base.fixture';
import path from 'path';

test.describe.serial('Resource CRUD Operations', () => {
  let categoryId: number;
  let ownerGroupId: number;
  let tagId: number;
  let createdResourceId: number;
  let testRunId: number; // Unique ID for this test run - set in beforeAll for retry support

  test.beforeAll(async ({ apiClient }) => {
    // Reset state variables at start of each test run to prevent stale values
    createdResourceId = 0;

    // Generate unique ID at start of each test run (including retries)
    // Use timestamp + random to avoid collisions with parallel workers
    testRunId = Date.now() + Math.floor(Math.random() * 100000);

    // Clean up any resource with the test image hash to avoid deduplication conflicts
    // sample-image-9.png hash: b2570251f5100085491bb6da331760031d3fc171
    const testImageHash = 'b2570251f5100085491bb6da331760031d3fc171';
    try {
      const resources = await apiClient.getResources();
      for (const resource of resources) {
        if (resource.Hash === testImageHash) {
          await apiClient.deleteResource(resource.ID);
        }
      }
    } catch {
      // Ignore errors - resources might not exist
    }

    // Create prerequisite data with unique names to avoid conflicts
    const category = await apiClient.createCategory(`Resource Test Category ${testRunId}`, 'Category for resource tests');
    categoryId = category.ID;

    const ownerGroup = await apiClient.createGroup({
      name: `Resource Owner Group ${testRunId}`,
      description: 'Owner for resources',
      categoryId: categoryId,
    });
    ownerGroupId = ownerGroup.ID;

    const tag = await apiClient.createTag(`Resource Test Tag ${testRunId}`, 'Tag for resources');
    tagId = tag.ID;
  });

  test('should upload a file resource', async ({ resourcePage, page }) => {
    // Use a unique image not used by other tests to avoid hash deduplication conflicts
    const testFilePath = path.join(__dirname, '../test-assets/sample-image-9.png');

    // Navigate to new resource page
    await resourcePage.gotoNew();

    // Set file input
    const fileInput = page.locator('input[type="file"]');
    await fileInput.setInputFiles(testFilePath);

    // Fill name
    await page.locator('input[name="Name"]').fill(`E2E Test Image ${testRunId}`);

    // Fill description
    await page.locator('textarea[name="Description"]').fill('Image uploaded by E2E test');

    // Select owner - the Owner field uses an autocompleter with elName='ownerId'
    // Find the combobox inside the Owner section
    const ownerSection = page.locator('div.sm\\:grid:has(span:has-text("Owner"))');
    const ownerInput = ownerSection.locator('input[role="combobox"]').first();
    await ownerInput.click();
    await ownerInput.fill(`Resource Owner Group ${testRunId}`);

    // Wait for dropdown option and click it
    const ownerOption = page.locator(`div[role="option"]:has-text("Resource Owner Group ${testRunId}")`).first();
    await ownerOption.waitFor({ state: 'visible', timeout: 10000 });
    await ownerOption.click();

    // Wait for the hidden input to be created after selection
    await page.waitForSelector('input[name="ownerId"]', { state: 'attached', timeout: 5000 });

    // Click save button
    await page.locator('button[type="submit"]:has-text("Save")').click();
    await page.waitForLoadState('load');

    // Wait a moment for any redirects
    await page.waitForTimeout(1000);

    // Check where we ended up
    const url = page.url();
    if (url.includes('/resource?id=')) {
      createdResourceId = parseInt(new URL(url).searchParams.get('id') || '0');
    } else if (url.includes('/resources')) {
      // If redirected to list, find the resource
      const resourceLink = page.locator(`a:has-text("E2E Test Image ${testRunId}")`).first();
      if (await resourceLink.isVisible()) {
        await resourceLink.click();
        await page.waitForLoadState('load');
        createdResourceId = parseInt(new URL(page.url()).searchParams.get('id') || '0');
      }
    }

    if (!createdResourceId || createdResourceId === 0) {
      // Debug: check for errors on the page
      const errorText = await page.locator('.error, [class*="error"], h1:has-text("error")').textContent().catch(() => '');
      throw new Error(`Could not determine resource ID. URL: ${url}, Error: ${errorText}`);
    }

    expect(createdResourceId).toBeGreaterThan(0);
  });

  test('should display the created resource', async ({ resourcePage, page }) => {
    expect(createdResourceId, 'Resource must be created first').toBeGreaterThan(0);
    await resourcePage.gotoDisplay(createdResourceId);
    // Check that we're on the resource display page by verifying the URL contains the resource ID
    await expect(page).toHaveURL(new RegExp(`/resource\\?id=${createdResourceId}`));
    // Verify the resource heading is visible (contains "Resource" prefix)
    await expect(page.locator('h2:has-text("Resource")').first()).toBeVisible();
  });

  test('should update the resource', async ({ resourcePage, page }) => {
    expect(createdResourceId, 'Resource must be created first').toBeGreaterThan(0);
    await resourcePage.update(createdResourceId, {
      name: 'Updated E2E Image',
      description: 'Updated image description',
    });
    await expect(page.locator('text=Updated E2E Image').first()).toBeVisible();
  });

  test('should delete the resource', async ({ resourcePage }) => {
    expect(createdResourceId, 'Resource must be created first').toBeGreaterThan(0);
    await resourcePage.delete(createdResourceId);
    await resourcePage.verifyResourceNotInList('Updated E2E Image');
  });

  test.afterAll(async ({ apiClient }) => {
    // Clean up resource first (in case delete test was skipped due to earlier failures)
    if (createdResourceId) {
      try {
        await apiClient.deleteResource(createdResourceId);
      } catch {
        // Ignore - resource may have been deleted by the delete test
      }
    }
    // Clean up in reverse dependency order
    if (ownerGroupId) {
      await apiClient.deleteGroup(ownerGroupId);
    }
    if (tagId) {
      await apiClient.deleteTag(tagId);
    }
    if (categoryId) {
      await apiClient.deleteCategory(categoryId);
    }
  });
});

test.describe('Resource from URL', () => {
  let categoryId: number;
  let ownerGroupId: number;
  let testRunId: number;

  test.beforeAll(async ({ apiClient }) => {
    testRunId = Date.now() + Math.floor(Math.random() * 100000);
    const category = await apiClient.createCategory(`URL Resource Category ${testRunId}`, 'Category for URL resources');
    categoryId = category.ID;

    const ownerGroup = await apiClient.createGroup({
      name: `URL Resource Owner ${testRunId}`,
      categoryId: categoryId,
    });
    ownerGroupId = ownerGroup.ID;
  });

  // Skip this test in CI as it depends on external service availability
  // Run locally with: npm run test:headed -- --grep "create resource from URL"
  test.skip('should create resource from URL', async ({ resourcePage }) => {
    // Note: This test is skipped by default because it depends on an external URL
    // (via.placeholder.com) which may be unavailable or slow
    await resourcePage.createFromUrl({
      url: 'https://via.placeholder.com/150',
      name: `Remote Image Resource ${testRunId}`,
      ownerGroupName: `URL Resource Owner ${testRunId}`,
    });
  });

  test.afterAll(async ({ apiClient }) => {
    if (ownerGroupId) {
      await apiClient.deleteGroup(ownerGroupId);
    }
    if (categoryId) {
      await apiClient.deleteCategory(categoryId);
    }
  });
});
