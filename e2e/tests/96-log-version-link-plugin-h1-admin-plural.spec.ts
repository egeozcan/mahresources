import { test, expect } from '../fixtures/base.fixture';
import path from 'path';

/**
 * Bug 1: Log entry links for resource_version go to 404.
 * When EntityType is "resource_version", the listLogs and displayLog templates
 * generate links to /resource_version?id=X, which doesn't exist.
 * It should not produce a broken link.
 *
 * Bug 2: Duplicate h1 on Manage Plugins page.
 * The base layout already renders an h1 from pageTitle, but managePlugins.tpl
 * also has its own <h1>, resulting in two h1 elements.
 *
 * Bug 3: Admin overview hardcoded plural nouns.
 * "1 resources" and "1 groups" should be "1 resource" and "1 group".
 */

test.describe('Bug 1: resource_version log links should not 404', () => {
  let categoryId: number;
  let ownerGroupId: number;
  let resourceId: number;

  test.beforeAll(async ({ apiClient }) => {
    const testRunId = Date.now();
    const category = await apiClient.createCategory(
      `LogLink Test Cat ${testRunId}`,
      'Category for log link test'
    );
    categoryId = category.ID;

    const ownerGroup = await apiClient.createGroup({
      name: `LogLink Test Owner ${testRunId}`,
      categoryId: categoryId,
    });
    ownerGroupId = ownerGroup.ID;

    // Create a resource
    const testFilePath = path.join(__dirname, '../test-assets/sample-image-34.png');
    const resource = await apiClient.createResource({
      filePath: testFilePath,
      name: `LogLink Test Resource ${testRunId}`,
      ownerId: ownerGroupId,
    });
    resourceId = resource.ID;
  });

  test('resource_version log entry link should point to a valid page (not 404)', async ({
    page,
    request,
    baseURL,
  }) => {
    // Upload a new version to create a resource_version log entry
    const fs = await import('fs');
    const testFilePath = path.join(__dirname, '../test-assets/sample-image-35.png');
    const fileBuffer = fs.readFileSync(testFilePath);

    const uploadResponse = await request.post(
      `${baseURL}/v1/resource/versions?resourceId=${resourceId}`,
      {
        multipart: {
          file: {
            name: 'sample-image-35.png',
            mimeType: 'image/png',
            buffer: fileBuffer,
          },
          comment: 'Version for log link test',
        },
      }
    );
    expect(uploadResponse.ok()).toBeTruthy();

    // Navigate to the logs page filtered by resource_version entity type
    await page.goto(`${baseURL}/logs?EntityType=resource_version`);
    await page.waitForLoadState('load');

    // Find a resource_version log entry link (non-deleted create action)
    const versionLink = page.locator(
      'td a.text-amber-700[href*="resource_version"]'
    );

    // The link should NOT point to /resource_version?id=X (which 404s)
    // Instead it should either not be a link, or point to /resource?id=X
    const linkCount = await versionLink.count();
    if (linkCount > 0) {
      // If there IS a link with resource_version in the href, that's the bug
      // Follow it and verify it doesn't 404
      const href = await versionLink.first().getAttribute('href');
      const resp = await page.goto(`${baseURL}${href}`);
      expect(resp?.status(), `Link ${href} should not return 404`).not.toBe(404);
    }

    // Alternative check: navigate to logs and verify no link contains "resource_version" in href
    await page.goto(`${baseURL}/logs?EntityType=resource_version`);
    await page.waitForLoadState('load');
    const brokenLinks = page.locator('td a[href^="/resource_version"]');
    await expect(
      brokenLinks,
      'No links should point to /resource_version (which has no route)'
    ).toHaveCount(0);
  });

  test.afterAll(async ({ apiClient }) => {
    if (resourceId) {
      try {
        await apiClient.deleteResource(resourceId);
      } catch {
        /* cleanup best-effort */
      }
    }
    if (ownerGroupId) {
      try {
        await apiClient.deleteGroup(ownerGroupId);
      } catch {
        /* cleanup */
      }
    }
    if (categoryId) {
      try {
        await apiClient.deleteCategory(categoryId);
      } catch {
        /* cleanup */
      }
    }
  });
});

test.describe('Bug 2: Manage Plugins page should have exactly one h1', () => {
  test('plugins manage page has exactly one h1 element', async ({ page, baseURL }) => {
    await page.goto(`${baseURL}/plugins/manage`);
    await page.waitForLoadState('load');

    const h1Elements = page.locator('h1');
    await expect(
      h1Elements,
      'There should be exactly one h1 element on the page'
    ).toHaveCount(1);
  });
});

test.describe('Bug 3: Admin overview should use correct singular/plural nouns', () => {
  let categoryId: number;
  let groupId: number;
  let tagId: number;

  test.beforeAll(async ({ apiClient }) => {
    const testRunId = Date.now();

    // Create a category with exactly one group
    const category = await apiClient.createCategory(
      `Singular Test Cat ${testRunId}`,
      'Category for singular/plural test'
    );
    categoryId = category.ID;

    const group = await apiClient.createGroup({
      name: `Singular Test Group ${testRunId}`,
      categoryId: categoryId,
    });
    groupId = group.ID;
  });

  test('admin overview should say "1 group" not "1 groups" for single-count categories', async ({
    page,
    baseURL,
  }) => {
    await page.goto(`${baseURL}/admin/overview`);
    await page.waitForLoadState('networkidle');

    // Wait for the detailed statistics section and Alpine.js hydration
    const detailedSection = page.locator('section[aria-label="Detailed statistics"]');
    await expect(
      detailedSection.locator('h3:has-text("Top Categories")')
    ).toBeVisible({ timeout: 30000 });

    // Wait for Alpine.js to fully render the category count spans
    // Poll until we see our category's count text (either correct or buggy form)
    await page.waitForFunction(
      () => {
        const section = document.querySelector('section[aria-label="Detailed statistics"]');
        if (!section) return false;
        const spans = section.querySelectorAll('span');
        return Array.from(spans).some(s => /\d+ groups?$/.test(s.textContent?.trim() || ''));
      },
      { timeout: 15000 }
    );

    // Now verify correct singular form
    const correctText = detailedSection.locator('span:text("1 group")');
    await expect(
      correctText,
      '"1 group" (singular) should appear for a category with one group'
    ).toHaveCount(1);

    // Verify the buggy plural form does NOT appear
    const buggyText = detailedSection.locator('span:text("1 groups")');
    await expect(
      buggyText,
      '"1 groups" should not appear - it should be "1 group"'
    ).toHaveCount(0);
  });

  test.afterAll(async ({ apiClient }) => {
    if (groupId) {
      try {
        await apiClient.deleteGroup(groupId);
      } catch {
        /* cleanup */
      }
    }
    if (categoryId) {
      try {
        await apiClient.deleteCategory(categoryId);
      } catch {
        /* cleanup */
      }
    }
  });
});
