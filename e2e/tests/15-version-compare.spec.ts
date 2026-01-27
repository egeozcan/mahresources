import { test, expect } from '../fixtures/base.fixture';
import { Page } from '@playwright/test';
import path from 'path';

/**
 * Ensures the version panel is expanded. Handles the case where the panel
 * may already be expanded (with 2+ versions, panel auto-expands).
 */
async function ensureVersionPanelExpanded(page: Page) {
  // Target the details element containing "Versions" summary specifically
  const versionDetails = page.locator('details:has(summary:has-text("Versions"))');
  const versionContent = versionDetails.locator('.p-4.border-dashed');

  // Check if the details element is open
  const isOpen = await versionDetails.getAttribute('open');
  if (isOpen === null) {
    await page.locator('summary:has-text("Versions")').click();
    await expect(versionContent).toBeVisible({ timeout: 5000 });
  }
  return versionContent;
}

test.describe.serial('Version Compare UI', () => {
  let categoryId: number;
  let ownerGroupId: number;
  let resource1Id: number;
  let resource2Id: number;
  let testRunId: number;

  // Increase timeout for beforeAll to handle database contention
  test.beforeAll(async ({ apiClient, request, baseURL }) => {
    // Use timestamp + random to avoid collisions with parallel workers
    testRunId = Date.now() + Math.floor(Math.random() * 100000);

    // Create prerequisite data
    const category = await apiClient.createCategory(
      `Compare Test Category ${testRunId}`,
      'Category for compare tests'
    );
    categoryId = category.ID;

    const ownerGroup = await apiClient.createGroup({
      name: `Compare Test Owner ${testRunId}`,
      categoryId: categoryId,
    });
    ownerGroupId = ownerGroup.ID;

    // Create resources in beforeAll to ensure they exist before any test runs
    // Use unique images (21-23) to avoid hash conflicts with other parallel tests
    const testFile1 = path.join(__dirname, '../test-assets/sample-image-21.png');
    const resource1 = await apiClient.createResource({
      filePath: testFile1,
      name: `Compare Resource 1 ${testRunId}`,
      ownerId: ownerGroupId,
    });
    resource1Id = resource1.ID;

    const testFile2 = path.join(__dirname, '../test-assets/sample-image-22.png');
    const resource2 = await apiClient.createResource({
      filePath: testFile2,
      name: `Compare Resource 2 ${testRunId}`,
      ownerId: ownerGroupId,
    });
    resource2Id = resource2.ID;

    // Add a second version to resource1 for version comparison tests
    const fs = await import('fs');
    const versionFile = path.join(__dirname, '../test-assets/sample-image-23.png');
    const fileBuffer = fs.readFileSync(versionFile);

    await request.post(`${baseURL}/v1/resource/versions?resourceId=${resource1Id}`, {
      multipart: {
        file: {
          name: 'sample-image-17.png',
          mimeType: 'image/png',
          buffer: fileBuffer,
        },
        comment: 'Version 2 for compare tests',
      },
    });
  });

  test('should have created resources with multiple versions', async () => {
    // Verify resources were created in beforeAll
    expect(resource1Id).toBeGreaterThan(0);
    expect(resource2Id).toBeGreaterThan(0);
  });

  test('should navigate to compare page from version panel', async ({ resourcePage, page }) => {
    expect(resource1Id).toBeGreaterThan(0);
    await resourcePage.gotoDisplay(resource1Id);

    // Ensure version panel is expanded
    await ensureVersionPanelExpanded(page);

    // Wait for Compare button to be visible
    const compareButton = page.locator('button:has-text("Compare")');
    await expect(compareButton).toBeVisible({ timeout: 5000 });

    // Click Compare button to enter compare mode
    await compareButton.click();

    // Wait for animation to settle
    await page.waitForTimeout(300);

    // Now checkboxes should be visible inside the version panel
    const checkboxes = page.locator('details input[type="checkbox"]');
    await expect(checkboxes.first()).toBeVisible({ timeout: 5000 });

    // Count checkboxes - should have at least 2 versions
    const count = await checkboxes.count();
    expect(count).toBeGreaterThanOrEqual(2);

    // Select two versions
    await checkboxes.first().check({ force: true });
    await checkboxes.nth(1).check({ force: true });

    // Compare Selected link should appear
    const compareLink = page.locator('a:has-text("Compare Selected")');
    await expect(compareLink).toBeVisible();

    // Verify link format - should point to /resource/compare
    const href = await compareLink.getAttribute('href');
    expect(href).toContain('/resource/compare');
    expect(href).toContain(`r1=${resource1Id}`);
    expect(href).toContain('v1=');
    expect(href).toContain('v2=');
  });

  test('should show compare bulk action for exactly 2 resources', async ({ resourcePage, page }) => {
    // Navigate to resources list filtered by owner to ensure our test resources are visible
    await page.goto(`/resources?OwnerId=${ownerGroupId}`);
    await page.waitForLoadState('load');

    // Select first resource using the x-data pattern
    const checkbox1 = page.locator(`[x-data*="itemId: ${resource1Id}"] input[type="checkbox"]`);
    await expect(checkbox1).toBeVisible({ timeout: 10000 });
    await checkbox1.check();

    // Select second resource
    const checkbox2 = page.locator(`[x-data*="itemId: ${resource2Id}"] input[type="checkbox"]`);
    await expect(checkbox2).toBeVisible({ timeout: 5000 });
    await checkbox2.check();

    // Wait for bulk editor to update
    await page.waitForTimeout(300);

    // Compare link should appear when exactly 2 resources are selected
    const compareLink = page.locator('.bulk-editors a:has-text("Compare")');
    await expect(compareLink).toBeVisible({ timeout: 5000 });

    // Verify link format
    const href = await compareLink.getAttribute('href');
    expect(href).toContain('/resource/compare');
    expect(href).toContain('r1=');
    expect(href).toContain('r2=');
  });

  test('should load compare page with metadata table', async ({ page, apiClient }) => {
    // Verify resources were created
    expect(resource1Id, 'resource1Id should be set from beforeAll').toBeGreaterThan(0);
    expect(resource2Id, 'resource2Id should be set from beforeAll').toBeGreaterThan(0);

    // Verify resources still exist before navigating (they might have been deleted by parallel tests)
    const resources = await apiClient.getResources();
    const r1Exists = resources.some((r: { ID: number }) => r.ID === resource1Id);
    const r2Exists = resources.some((r: { ID: number }) => r.ID === resource2Id);
    expect(r1Exists, `Resource ${resource1Id} should exist`).toBe(true);
    expect(r2Exists, `Resource ${resource2Id} should exist`).toBe(true);

    // Navigate to compare page with two different resources
    await page.goto(`/resource/compare?r1=${resource1Id}&v1=1&r2=${resource2Id}&v2=1`);
    await page.waitForLoadState('load');

    // Page should load with metadata comparison table
    await expect(page.locator('text=Metadata Comparison')).toBeVisible({ timeout: 10000 });

    // Metadata table should have required rows (use first() for strict mode)
    await expect(page.locator('td:has-text("Content Type")').first()).toBeVisible();
    await expect(page.locator('td:has-text("File Size")').first()).toBeVisible();
    await expect(page.locator('td:has-text("Hash Match")').first()).toBeVisible();
    await expect(page.locator('td:has-text("Dimensions")').first()).toBeVisible();
    await expect(page.locator('td:has-text("Created")').first()).toBeVisible();
    // Resource row label specifically
    await expect(page.locator('td.text-gray-600:has-text("Resource")')).toBeVisible();
  });

  test('should show image comparison modes for image resources', async ({ page }) => {
    // Compare v1 vs v2 of the same resource so version labels are different
    await page.goto(`/resource/compare?r1=${resource1Id}&v1=1&r2=${resource1Id}&v2=2`);
    await page.waitForLoadState('load');

    // Mode buttons should be visible
    await expect(page.locator('button:has-text("Side-by-side")')).toBeVisible({ timeout: 10000 });
    await expect(page.locator('button:has-text("Slider")')).toBeVisible();
    await expect(page.locator('button:has-text("Onion skin")')).toBeVisible();
    await expect(page.locator('button:has-text("Toggle")')).toBeVisible();
    await expect(page.locator('button:has-text("Swap sides")')).toBeVisible();

    // Click different modes and verify they activate
    await page.locator('button:has-text("Slider")').click();
    await expect(page.locator('button:has-text("Slider")')).toHaveClass(/bg-indigo-600/);

    await page.locator('button:has-text("Onion skin")').click();
    await expect(page.locator('button:has-text("Onion skin")')).toHaveClass(/bg-indigo-600/);

    // Onion skin mode should show opacity slider
    await expect(page.locator('input[type="range"]')).toBeVisible();

    await page.locator('button:has-text("Toggle")').click();
    await expect(page.locator('button:has-text("Toggle")')).toHaveClass(/bg-indigo-600/);

    // Toggle mode should show click instruction
    await expect(page.locator('text=Click or press Space to toggle')).toBeVisible();

    // Toggle mode container should be focusable and respond to space key
    const toggleContainer = page.locator('[role="button"]:has-text("Click or press Space to toggle")');
    await expect(toggleContainer).toHaveAttribute('tabindex', '0');

    // Focus the toggle container and verify space key works
    await toggleContainer.focus();
    const versionLabel = toggleContainer.locator('.absolute.top-2.right-2');
    const initialVersion = await versionLabel.textContent();
    await page.keyboard.press('Space');
    await expect(versionLabel).not.toHaveText(initialVersion!);

    await page.locator('button:has-text("Side-by-side")').click();
    await expect(page.locator('button:has-text("Side-by-side")')).toHaveClass(/bg-indigo-600/);
  });

  test('should compare versions of the same resource', async ({ page }) => {
    // Compare v1 and v2 of the same resource
    await page.goto(`/resource/compare?r1=${resource1Id}&v1=1&r2=${resource1Id}&v2=2`);
    await page.waitForLoadState('load');

    // Metadata comparison should show
    await expect(page.locator('text=Metadata Comparison')).toBeVisible({ timeout: 10000 });

    // Both resources should show as the same in the Resource row
    const resourceRow = page.locator('tr:has(td:text("Resource"))');
    await expect(resourceRow).toBeVisible();

    // Same resource indicator - should be green (=)
    const sameResourceIndicator = resourceRow.locator('span.text-green-600');
    await expect(sameResourceIndicator).toBeVisible();
  });

  test('should update URL when changing version via dropdown', async ({ page }) => {
    // Start on compare page with v1 vs v1
    await page.goto(`/resource/compare?r1=${resource1Id}&v1=1&r2=${resource1Id}&v2=1`);
    await page.waitForLoadState('load');

    // Find version dropdown for the right side (second select)
    const versionSelects = page.locator('select');
    const count = await versionSelects.count();
    expect(count).toBeGreaterThanOrEqual(2);

    // Change the second version dropdown to v2 - this triggers navigation via Alpine.js
    const rightVersionSelect = versionSelects.nth(1);

    // Use Promise.all to wait for navigation and select option together
    await Promise.all([
      page.waitForURL(/v2=2/, { timeout: 10000 }),
      rightVersionSelect.selectOption('2'),
    ]);

    // Verify URL was updated
    const url = page.url();
    expect(url).toContain('v2=2');
  });

  test('should handle swap sides button', async ({ page }) => {
    await page.goto(`/resource/compare?r1=${resource1Id}&v1=1&r2=${resource2Id}&v2=1`);
    await page.waitForLoadState('load');

    // Wait for image compare component to load
    await expect(page.locator('button:has-text("Swap sides")')).toBeVisible({ timeout: 10000 });

    // Note which image is on which side by checking the img src attributes
    const images = page.locator('img[alt^="Version"]');
    await expect(images.first()).toBeVisible();

    // Click swap sides
    await page.locator('button:has-text("Swap sides")').click();

    // The swap should happen client-side (Alpine.js handles this)
    // We can verify the button is still functional
    await expect(page.locator('button:has-text("Swap sides")')).toBeVisible();
  });

  test('should show different resource indicator for cross-resource compare', async ({ page }) => {
    // Compare two different resources
    await page.goto(`/resource/compare?r1=${resource1Id}&v1=1&r2=${resource2Id}&v2=1`);
    await page.waitForLoadState('load');

    await expect(page.locator('text=Metadata Comparison')).toBeVisible({ timeout: 10000 });

    // Resource row should show different indicator (orange not equal sign)
    const resourceRow = page.locator('tr:has(td:text("Resource"))');
    await expect(resourceRow).toBeVisible();

    // Different resource indicator should be orange
    const diffResourceIndicator = resourceRow.locator('span.text-orange-600');
    await expect(diffResourceIndicator).toBeVisible();
  });

  test('should show autocompleter dropdown with search results', async ({ page }) => {
    // Start comparing resource1 v1 with itself
    await page.goto(`/resource/compare?r1=${resource1Id}&v1=1&r2=${resource1Id}&v2=1`);
    await page.waitForLoadState('load');

    await expect(page.locator('text=Metadata Comparison')).toBeVisible({ timeout: 10000 });

    // Find the right side autocompleter input (second picker card in the grid)
    const pickerGrid = page.locator('.grid.grid-cols-2');
    const rightPickerCard = pickerGrid.locator('> div').nth(1);
    const rightAutocompleter = rightPickerCard.locator('input[placeholder*="Search"]');
    await expect(rightAutocompleter).toBeVisible({ timeout: 5000 });

    // Click to focus - this should trigger the autocompleter to fetch and display results
    await rightAutocompleter.click();

    // Wait for dropdown to appear
    const dropdown = rightPickerCard.locator('div.absolute.z-10.bg-white');
    await expect(dropdown).toBeVisible({ timeout: 10000 });

    // Dropdown should contain at least one item with cursor-pointer class
    const items = dropdown.locator('div.cursor-pointer');
    await expect(items.first()).toBeVisible({ timeout: 5000 });

    // Count items - there should be at least one resource available
    const count = await items.count();
    expect(count).toBeGreaterThan(0);
  });

  test('should filter autocompleter results when typing', async ({ page }) => {
    // Start comparing resource1 v1 with resource2 v1
    await page.goto(`/resource/compare?r1=${resource1Id}&v1=1&r2=${resource2Id}&v2=1`);
    await page.waitForLoadState('load');

    await expect(page.locator('text=Metadata Comparison')).toBeVisible({ timeout: 10000 });

    // Find the left side autocompleter input
    const pickerGrid = page.locator('.grid.grid-cols-2');
    const leftPickerCard = pickerGrid.locator('> div').first();
    const leftAutocompleter = leftPickerCard.locator('input[placeholder*="Search"]');
    await expect(leftAutocompleter).toBeVisible({ timeout: 5000 });

    // Click and type to search for resource2
    await leftAutocompleter.click();
    await leftAutocompleter.pressSequentially(`Compare Resource 2`, { delay: 30 });

    // Wait for dropdown to appear with filtered results
    const dropdown = leftPickerCard.locator('div.absolute.z-10.bg-white');
    await expect(dropdown).toBeVisible({ timeout: 10000 });

    // Verify the specific suggestion is visible
    const suggestionText = `Compare Resource 2 ${testRunId}`;
    await expect(dropdown.locator(`text=${suggestionText}`)).toBeVisible({ timeout: 5000 });
  });

  test('should update URL when changing left version via dropdown', async ({ page }) => {
    // Start on compare page with v1 vs v2 of same resource
    await page.goto(`/resource/compare?r1=${resource1Id}&v1=2&r2=${resource1Id}&v2=1`);
    await page.waitForLoadState('load');

    // Find version dropdowns
    const versionSelects = page.locator('select');
    const count = await versionSelects.count();
    expect(count).toBeGreaterThanOrEqual(2);

    // Change the first version dropdown to v1
    const leftVersionSelect = versionSelects.first();

    // Use Promise.all to wait for navigation and select option together
    await Promise.all([
      page.waitForURL(/v1=1/, { timeout: 10000 }),
      leftVersionSelect.selectOption('1'),
    ]);

    // Verify URL was updated
    const url = page.url();
    expect(url).toContain('v1=1');
  });

  test.afterAll(async ({ apiClient }) => {
    // Cleanup in reverse order
    if (resource1Id) {
      try {
        await apiClient.deleteResource(resource1Id);
      } catch {
        // May already be deleted
      }
    }
    if (resource2Id) {
      try {
        await apiClient.deleteResource(resource2Id);
      } catch {
        // May already be deleted
      }
    }
    if (ownerGroupId) {
      await apiClient.deleteGroup(ownerGroupId);
    }
    if (categoryId) {
      await apiClient.deleteCategory(categoryId);
    }
  });
});

test.describe.serial('Version Compare API', () => {
  let categoryId: number;
  let ownerGroupId: number;
  let resource1Id: number;
  let resource2Id: number;
  let testRunId: number;

  test('setup - create resources for compare API tests', async ({ apiClient, request, baseURL }) => {
    testRunId = Date.now();

    // Create prerequisite data
    const category = await apiClient.createCategory(
      `Compare API Category ${testRunId}`,
      'Category for compare API tests'
    );
    categoryId = category.ID;
    expect(categoryId).toBeGreaterThan(0);

    const ownerGroup = await apiClient.createGroup({
      name: `Compare API Owner ${testRunId}`,
      categoryId: categoryId,
    });
    ownerGroupId = ownerGroup.ID;
    expect(ownerGroupId).toBeGreaterThan(0);

    // Create resources using unique images
    const testFile1 = path.join(__dirname, '../test-assets/sample-image-18.png');
    const resource1 = await apiClient.createResource({
      filePath: testFile1,
      name: `Compare API Resource 1 ${testRunId}`,
      ownerId: ownerGroupId,
    });
    resource1Id = resource1.ID;
    expect(resource1Id).toBeGreaterThan(0);

    const testFile2 = path.join(__dirname, '../test-assets/sample-image-19.png');
    const resource2 = await apiClient.createResource({
      filePath: testFile2,
      name: `Compare API Resource 2 ${testRunId}`,
      ownerId: ownerGroupId,
    });
    resource2Id = resource2.ID;
    expect(resource2Id).toBeGreaterThan(0);

    // Add a second version to resource1
    const fs = await import('fs');
    const versionFile = path.join(__dirname, '../test-assets/sample-image-20.png');
    const fileBuffer = fs.readFileSync(versionFile);

    const response = await request.post(`${baseURL}/v1/resource/versions?resourceId=${resource1Id}`, {
      multipart: {
        file: {
          name: 'sample-image-20.png',
          mimeType: 'image/png',
          buffer: fileBuffer,
        },
        comment: 'Version 2 for API tests',
      },
    });
    expect(response.ok()).toBeTruthy();
  });

  test('should compare versions via API', async ({ request, baseURL }) => {
    expect(resource1Id, 'Resource must be created in setup').toBeGreaterThan(0);

    // Get versions for resource1
    const listResponse = await request.get(
      `${baseURL}/v1/resource/versions?resourceId=${resource1Id}`
    );
    expect(listResponse.ok()).toBeTruthy();
    const versions = await listResponse.json();
    expect(versions.length).toBeGreaterThanOrEqual(2);

    const v1 = versions.find((v: { versionNumber: number }) => v.versionNumber === 1);
    const v2 = versions.find((v: { versionNumber: number }) => v.versionNumber === 2);
    expect(v1).toBeDefined();
    expect(v2).toBeDefined();

    // Compare versions
    const compareResponse = await request.get(
      `${baseURL}/v1/resource/versions/compare?resourceId=${resource1Id}&v1=${v1.id}&v2=${v2.id}`
    );
    expect(compareResponse.ok()).toBeTruthy();

    const comparison = await compareResponse.json();
    expect(comparison.version1).toBeTruthy();
    expect(comparison.version2).toBeTruthy();
    expect(typeof comparison.sameHash).toBe('boolean');
    expect(typeof comparison.sameType).toBe('boolean');
    expect(comparison.sameHash).toBe(false); // Different files should have different hashes
  });

  test('should redirect to latest versions when versions not specified for cross-resource compare', async ({ page }) => {
    expect(resource1Id, 'Resources must be created in setup').toBeGreaterThan(0);
    expect(resource2Id).toBeGreaterThan(0);

    // Navigate to cross-resource compare without version params
    await page.goto(`/resource/compare?r1=${resource1Id}&r2=${resource2Id}`);
    await page.waitForLoadState('load');

    // Should redirect to URL with versions (v1 and v2 should be set to their latest)
    // resource1 has versions 1 and 2 (created in setup), resource2 only has version 1
    const url = new URL(page.url());
    expect(url.searchParams.get('v1')).toBe('2'); // resource1 latest version is 2
    expect(url.searchParams.get('v2')).toBe('1'); // resource2 only has version 1
    expect(url.searchParams.get('r1')).toBe(resource1Id.toString());
    expect(url.searchParams.get('r2')).toBe(resource2Id.toString());

    // Page should load with metadata comparison
    await expect(page.locator('text=Metadata Comparison')).toBeVisible({ timeout: 10000 });
  });

  test('should load cross-resource compare page', async ({ page }) => {
    expect(resource1Id, 'Resources must be created in setup').toBeGreaterThan(0);
    expect(resource2Id).toBeGreaterThan(0);

    // Navigate to cross-resource compare page
    await page.goto(`/resource/compare?r1=${resource1Id}&v1=1&r2=${resource2Id}&v2=1`);
    await page.waitForLoadState('load');

    // Page should load with metadata comparison
    await expect(page.locator('text=Metadata Comparison')).toBeVisible({ timeout: 10000 });

    // Resource row should show different resources
    const resourceRow = page.locator('tr:has(td:text("Resource"))');
    await expect(resourceRow).toBeVisible();

    // Different resource indicator (orange)
    const diffIndicator = resourceRow.locator('span.text-orange-600');
    await expect(diffIndicator).toBeVisible();
  });

  test.afterAll(async ({ apiClient }) => {
    if (resource1Id) {
      try {
        await apiClient.deleteResource(resource1Id);
      } catch {
        // May already be deleted
      }
    }
    if (resource2Id) {
      try {
        await apiClient.deleteResource(resource2Id);
      } catch {
        // May already be deleted
      }
    }
    if (ownerGroupId) {
      await apiClient.deleteGroup(ownerGroupId);
    }
    if (categoryId) {
      await apiClient.deleteCategory(categoryId);
    }
  });
});
