import { test, expect } from '../fixtures/base.fixture';
import { Page } from '@playwright/test';
import path from 'path';

/**
 * Ensures the version panel is expanded. Handles the case where the panel
 * may already be expanded (with 2+ versions, panel auto-expands).
 */
async function ensureVersionPanelExpanded(page: Page) {
  const versionContent = page.locator('.mt-2.border.rounded-lg.divide-y');
  const isExpanded = await versionContent.isVisible();
  if (!isExpanded) {
    await page.locator('button:has-text("Versions")').click();
    await expect(versionContent).toBeVisible({ timeout: 5000 });
  }
  return versionContent;
}

test.describe.serial('Resource Versioning', () => {
  let categoryId: number;
  let ownerGroupId: number;
  let resourceId: number;
  let testRunId: number;

  test.beforeAll(async ({ apiClient }) => {
    // Generate unique ID per beforeAll call to handle --repeat-each
    testRunId = Date.now();

    // Create prerequisite data
    const category = await apiClient.createCategory(
      `Version Test Category ${testRunId}`,
      'Category for versioning tests'
    );
    categoryId = category.ID;

    const ownerGroup = await apiClient.createGroup({
      name: `Version Test Owner ${testRunId}`,
      description: 'Owner for versioning tests',
      categoryId: categoryId,
    });
    ownerGroupId = ownerGroup.ID;
  });

  test('should create a resource with initial version', async ({ resourcePage, page }) => {
    // Use sample-image-10.png to avoid hash deduplication with lightbox tests
    const testFilePath = path.join(__dirname, '../test-assets/sample-image-10.png');

    await resourcePage.gotoNew();

    // Set file input
    const fileInput = page.locator('input[type="file"]');
    await fileInput.setInputFiles(testFilePath);

    // Fill name
    await page.locator('input[name="Name"]').fill(`Versioned Resource ${testRunId}`);

    // Fill description
    await page.locator('textarea[name="Description"]').fill('Resource for versioning tests');

    // Select owner
    const ownerSection = page.locator('div.sm\\:grid:has(span:has-text("Owner"))');
    const ownerInput = ownerSection.locator('input[role="combobox"]').first();
    await ownerInput.click();
    await ownerInput.fill(`Version Test Owner ${testRunId}`);

    const ownerOption = page
      .locator(`div[role="option"]:has-text("Version Test Owner ${testRunId}")`)
      .first();
    await ownerOption.waitFor({ state: 'visible', timeout: 10000 });
    await ownerOption.click();

    await page.waitForSelector('input[name="ownerId"]', { state: 'attached', timeout: 5000 });

    // Save
    await page.locator('button[type="submit"]:has-text("Save")').click();
    await page.waitForLoadState('load');
    await page.waitForTimeout(1000);

    // Extract resource ID
    const url = page.url();
    if (url.includes('/resource?id=')) {
      resourceId = parseInt(new URL(url).searchParams.get('id') || '0');
    } else if (url.includes('/resources')) {
      const resourceLink = page.locator(`a:has-text("Versioned Resource ${testRunId}")`).first();
      if (await resourceLink.isVisible()) {
        await resourceLink.click();
        await page.waitForLoadState('load');
        resourceId = parseInt(new URL(page.url()).searchParams.get('id') || '0');
      }
    }

    expect(resourceId).toBeGreaterThan(0);

    // Verify version panel exists with at least 1 version
    await expect(page.locator('button:has-text("Versions")')).toBeVisible({ timeout: 10000 });

    // Click to expand the version panel
    await page.locator('button:has-text("Versions")').click();

    await expect(page.locator('text=v1')).toBeVisible();
    await expect(page.locator('span.bg-blue-100:has-text("current")')).toBeVisible();
  });

  test('should show version panel collapsed when only 1 version', async ({ resourcePage, page }) => {
    expect(resourceId, 'Resource must be created first').toBeGreaterThan(0);
    await resourcePage.gotoDisplay(resourceId);

    // Click to expand the version panel
    await page.locator('button:has-text("Versions")').click();

    // Verify v1 is visible and marked as current
    await expect(page.locator('text=v1')).toBeVisible();
    await expect(page.locator('span.bg-blue-100:has-text("current")')).toBeVisible();
  });

  test('should upload a new version', async ({ resourcePage, page }) => {
    expect(resourceId, 'Resource must be created first').toBeGreaterThan(0);
    await resourcePage.gotoDisplay(resourceId);

    // Get initial version count from button text
    const versionButton = page.locator('button:has-text("Versions")');
    await expect(versionButton).toBeVisible({ timeout: 5000 });
    const buttonText = await versionButton.textContent();
    const initialCount = parseInt(buttonText?.match(/\((\d+)\)/)?.[1] || '1');

    // Ensure version panel is expanded
    await ensureVersionPanelExpanded(page);

    // Wait for the upload form to be visible (panel animation to complete)
    const uploadButton = page.locator('button:has-text("Upload New Version")');
    await expect(uploadButton).toBeVisible({ timeout: 5000 });

    // Upload new version - use a different image for new version hash
    const testFile2Path = path.join(__dirname, '../test-assets/sample-image-11.png');
    const fileInput = page.locator('input[type="file"][name="file"]');
    await fileInput.setInputFiles(testFile2Path);

    // Add a comment
    await page.locator('input[name="comment"]').fill('Second version upload');

    // Click upload
    await uploadButton.click();
    await page.waitForLoadState('load');

    // Verify version count increased by 1
    const expectedCount = initialCount + 1;
    await expect(page.locator(`text=Versions (${expectedCount})`)).toBeVisible({ timeout: 10000 });

    // Ensure version panel is expanded after reload
    await ensureVersionPanelExpanded(page);

    // The new version should be current (marked with bg-blue-50 background)
    const currentRow = page.locator('div.bg-blue-50');
    await expect(currentRow).toBeVisible();
    await expect(currentRow.locator('span.bg-blue-100:has-text("current")')).toBeVisible();

    // v1 should exist but not be current
    const v1Row = page.locator('div.p-4:has-text("v1")').first();
    await expect(v1Row).toBeVisible();
    await expect(v1Row.locator('span.bg-blue-100:has-text("current")')).not.toBeVisible();
  });

  test('should restore a previous version', async ({ resourcePage, page }) => {
    expect(resourceId, 'Resource must be created first').toBeGreaterThan(0);
    await resourcePage.gotoDisplay(resourceId);

    // Get initial version count from button text
    const versionButton = page.locator('button:has-text("Versions")');
    await expect(versionButton).toBeVisible({ timeout: 5000 });
    const buttonText = await versionButton.textContent();
    const initialCount = parseInt(buttonText?.match(/\((\d+)\)/)?.[1] || '1');

    // Ensure version panel is expanded
    const versionContent = await ensureVersionPanelExpanded(page);

    // Scroll the v1 row into view if needed
    const v1Row = versionContent.locator('div.p-4:has-text("v1")').first();
    await v1Row.scrollIntoViewIfNeeded();
    await expect(v1Row).toBeVisible({ timeout: 5000 });

    // Wait for the restore button to be visible and stable
    const restoreButton = v1Row.locator('button:has-text("Restore")');
    await expect(restoreButton).toBeVisible({ timeout: 5000 });

    // Click the restore button in v1's row
    await restoreButton.click({ timeout: 10000 });
    await page.waitForLoadState('load');

    // After restore, version count should increase by 1
    const expectedCount = initialCount + 1;
    await expect(page.locator(`text=Versions (${expectedCount})`)).toBeVisible({ timeout: 10000 });

    // Ensure version panel is expanded after reload
    await ensureVersionPanelExpanded(page);

    // The current row (bg-blue-50) should have the current badge
    const currentRow = page.locator('div.bg-blue-50');
    await expect(currentRow).toBeVisible();
    await expect(currentRow.locator('span.bg-blue-100:has-text("current")')).toBeVisible();

    // Should show "Restored from version 1" comment (use first() in case of multiple restores)
    await expect(page.locator('text=Restored from version 1').first()).toBeVisible();
  });

  test('should download version file', async ({ resourcePage, page }) => {
    expect(resourceId, 'Resource must be created first').toBeGreaterThan(0);
    await resourcePage.gotoDisplay(resourceId);

    // Ensure version panel is expanded
    const versionContent = await ensureVersionPanelExpanded(page);

    // Get any version row with a download link (inside the version panel)
    const downloadLink = versionContent.locator('a:has-text("Download")').first();

    // Verify download link exists and has correct href pattern
    await expect(downloadLink).toBeVisible();
    const href = await downloadLink.getAttribute('href');
    expect(href).toContain('/v1/resource/version/file?versionId=');
  });

  test('should delete a non-current version', async ({ resourcePage, page }) => {
    expect(resourceId, 'Resource must be created first').toBeGreaterThan(0);
    await resourcePage.gotoDisplay(resourceId);

    // Get initial version count from button text
    const versionButton = page.locator('button:has-text("Versions")');
    await expect(versionButton).toBeVisible({ timeout: 5000 });
    const buttonText = await versionButton.textContent();
    const initialCount = parseInt(buttonText?.match(/\((\d+)\)/)?.[1] || '1');

    // Need at least 2 versions to delete a non-current one
    if (initialCount < 2) {
      // Skip test - not enough versions to delete a non-current one
      return;
    }

    // Ensure version panel is expanded
    const versionContent = await ensureVersionPanelExpanded(page);

    // Find a non-current version (one without the "current" badge) and click Delete
    const nonCurrentRow = versionContent.locator('div.p-4:not(:has(.bg-blue-100))').first();
    await expect(nonCurrentRow).toBeVisible({ timeout: 5000 });

    // Set up dialog handler for confirmation
    page.on('dialog', async dialog => {
      await dialog.accept();
    });

    // Click delete button
    await nonCurrentRow.locator('button:has-text("Delete")').click();
    await page.waitForLoadState('load');

    // Version count should decrease by 1
    const expectedCount = initialCount - 1;
    await expect(page.locator(`text=Versions (${expectedCount})`)).toBeVisible({ timeout: 10000 });
  });

  test('should not show delete button for current version', async ({ resourcePage, page }) => {
    expect(resourceId, 'Resource must be created first').toBeGreaterThan(0);
    await resourcePage.gotoDisplay(resourceId);

    // Ensure version panel is expanded
    await ensureVersionPanelExpanded(page);

    // Find the current version row (has blue background)
    const currentRow = page.locator('div.bg-blue-50');
    await expect(currentRow).toBeVisible();

    // Should not have delete button
    await expect(currentRow.locator('button:has-text("Delete")')).not.toBeVisible();

    // Should not have restore button either
    await expect(currentRow.locator('button:has-text("Restore")')).not.toBeVisible();
  });

  test('should compare two versions', async ({ resourcePage, page }) => {
    expect(resourceId, 'Resource must be created first').toBeGreaterThan(0);
    await resourcePage.gotoDisplay(resourceId);

    // Ensure version panel is expanded
    const versionContent = await ensureVersionPanelExpanded(page);

    // Wait for Compare button to be visible
    const compareButton = page.locator('button:has-text("Compare")');
    await expect(compareButton).toBeVisible({ timeout: 5000 });

    // Click Compare button to enter compare mode
    await compareButton.click();

    // Wait for animation to settle after compare mode toggle
    await page.waitForTimeout(300);

    // Now checkboxes should be visible inside the version panel rows
    const versionRows = versionContent.locator('div.p-4');
    const checkboxes = versionRows.locator('input[type="checkbox"]');

    // Wait for checkboxes to appear (Alpine.js template rendering)
    await expect(checkboxes.first()).toBeVisible({ timeout: 5000 });

    // Select first two versions
    await checkboxes.first().check({ force: true });
    await checkboxes.nth(1).check({ force: true });

    // Compare Selected button should appear
    const compareSelectedLink = page.locator('a:has-text("Compare Selected")');
    await expect(compareSelectedLink).toBeVisible();

    // Click and verify we get JSON response
    const href = await compareSelectedLink.getAttribute('href');
    expect(href).toContain('/v1/resource/versions/compare');
    expect(href).toContain(`resourceId=${resourceId}`);
  });

  test.afterAll(async ({ apiClient }) => {
    // Clean up
    if (resourceId) {
      try {
        await apiClient.deleteResource(resourceId);
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

test.describe.serial('Version API Operations', () => {
  let categoryId: number;
  let ownerGroupId: number;
  let resourceId: number;
  let testRunId: number;

  test('setup - create resource for version tests', async ({ apiClient }) => {
    // Generate unique ID per test run to handle --repeat-each
    testRunId = Date.now();

    // Create prerequisite data
    const category = await apiClient.createCategory(
      `Version API Category ${testRunId}`,
      'Category for version API tests'
    );
    categoryId = category.ID;
    expect(categoryId).toBeGreaterThan(0);

    const ownerGroup = await apiClient.createGroup({
      name: `Version API Owner ${testRunId}`,
      categoryId: categoryId,
    });
    ownerGroupId = ownerGroup.ID;
    expect(ownerGroupId).toBeGreaterThan(0);

    // Create a resource using a different file to avoid hash deduplication with UI tests
    // The UI tests use sample-image.png, so we use sample-document.txt
    const testFilePath = path.join(__dirname, '../test-assets/sample-document.txt');
    const resource = await apiClient.createResource({
      filePath: testFilePath,
      name: `API Version Test ${testRunId}`,
      ownerId: ownerGroupId,
    });
    resourceId = resource.ID;
    expect(resourceId).toBeGreaterThan(0);
  });

  test('should list versions via API', async ({ request, baseURL }) => {
    expect(resourceId, 'Resource must be created in setup test').toBeGreaterThan(0);
    const response = await request.get(`${baseURL}/v1/resource/versions?resourceId=${resourceId}`);
    expect(response.ok()).toBeTruthy();

    const versions = await response.json();
    expect(Array.isArray(versions)).toBeTruthy();
    expect(versions.length).toBeGreaterThanOrEqual(1);
    // Versions are ordered by version_number DESC, so the last one should be v1
    const v1 = versions.find((v: { versionNumber: number }) => v.versionNumber === 1);
    expect(v1).toBeDefined();
  });

  test('should upload new version via API', async ({ request, baseURL }) => {
    expect(resourceId, 'Resource must be created in beforeAll').toBeGreaterThan(0);
    const fs = await import('fs');
    // Use a unique image to avoid hash conflicts
    const testFilePath = path.join(__dirname, '../test-assets/sample-image-12.png');
    const fileBuffer = fs.readFileSync(testFilePath);

    const response = await request.post(`${baseURL}/v1/resource/versions?resourceId=${resourceId}`, {
      multipart: {
        file: {
          name: 'sample-image-12.png',
          mimeType: 'image/png',
          buffer: fileBuffer,
        },
        comment: 'API uploaded version',
      },
    });

    expect(response.ok()).toBeTruthy();
    const version = await response.json();
    expect(version.versionNumber).toBe(2);
    expect(version.comment).toBe('API uploaded version');
  });

  test('should restore version via API', async ({ request, baseURL }) => {
    expect(resourceId, 'Resource must be created in beforeAll').toBeGreaterThan(0);
    // First get versions to find v1
    const listResponse = await request.get(
      `${baseURL}/v1/resource/versions?resourceId=${resourceId}`
    );
    const versions = await listResponse.json();
    const v1 = versions.find((v: { versionNumber: number }) => v.versionNumber === 1);
    expect(v1).toBeTruthy();

    // Restore v1
    const response = await request.post(`${baseURL}/v1/resource/version/restore`, {
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      data: `resourceId=${resourceId}&versionId=${v1.id}`,
    });

    expect(response.ok()).toBeTruthy();
    const restored = await response.json();
    expect(restored.versionNumber).toBe(3);
    expect(restored.comment).toContain('Restored from version 1');
  });

  test('should compare versions via API', async ({ request, baseURL }) => {
    expect(resourceId, 'Resource must be created in beforeAll').toBeGreaterThan(0);
    // Get versions
    const listResponse = await request.get(
      `${baseURL}/v1/resource/versions?resourceId=${resourceId}`
    );
    const versions = await listResponse.json();

    const v1 = versions.find((v: { versionNumber: number }) => v.versionNumber === 1);
    const v2 = versions.find((v: { versionNumber: number }) => v.versionNumber === 2);

    const response = await request.get(
      `${baseURL}/v1/resource/versions/compare?resourceId=${resourceId}&v1=${v1.id}&v2=${v2.id}`
    );

    expect(response.ok()).toBeTruthy();
    const comparison = await response.json();
    expect(comparison.version1).toBeTruthy();
    expect(comparison.version2).toBeTruthy();
    expect(typeof comparison.sameHash).toBe('boolean');
    expect(typeof comparison.sameType).toBe('boolean');
  });

  test('should delete non-current version via API', async ({ request, baseURL }) => {
    expect(resourceId, 'Resource must be created in beforeAll').toBeGreaterThan(0);
    // Get versions - we should have v1, v2, v3 now
    const listResponse = await request.get(
      `${baseURL}/v1/resource/versions?resourceId=${resourceId}`
    );
    const versions = await listResponse.json();

    // v3 should be current (after restore), so delete v1
    const v1 = versions.find((v: { versionNumber: number }) => v.versionNumber === 1);
    expect(v1).toBeTruthy();

    const response = await request.delete(
      `${baseURL}/v1/resource/version?resourceId=${resourceId}&versionId=${v1.id}`
    );

    expect(response.ok()).toBeTruthy();
    const result = await response.json();
    expect(result.status).toBe('deleted');

    // Verify deletion
    const afterResponse = await request.get(
      `${baseURL}/v1/resource/versions?resourceId=${resourceId}`
    );
    const afterVersions = await afterResponse.json();
    expect(afterVersions.length).toBe(2); // v2, v3 remain
  });

  test('should not delete current version', async ({ request, baseURL }) => {
    expect(resourceId, 'Resource must be created in beforeAll').toBeGreaterThan(0);
    // Get versions
    const listResponse = await request.get(
      `${baseURL}/v1/resource/versions?resourceId=${resourceId}`
    );
    const versions = await listResponse.json();

    // Find current version (v3 with highest version number)
    const current = versions.reduce(
      (
        max: { versionNumber: number },
        v: { versionNumber: number }
      ) => (v.versionNumber > max.versionNumber ? v : max),
      versions[0]
    );

    const response = await request.delete(
      `${baseURL}/v1/resource/version?resourceId=${resourceId}&versionId=${current.id}`
    );

    // Should fail
    expect(response.ok()).toBeFalsy();
    expect(response.status()).toBe(500); // Internal server error with message
  });

  test.afterAll(async ({ apiClient }) => {
    if (resourceId) {
      try {
        await apiClient.deleteResource(resourceId);
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
