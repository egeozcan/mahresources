import { test, expect } from '../fixtures/base.fixture';
import path from 'path';

test.describe.serial('Resource CRUD Operations', () => {
  let categoryId: number;
  let ownerGroupId: number;
  let tagId: number;
  let createdResourceId: number;

  test.beforeAll(async ({ apiClient }) => {
    // Create prerequisite data
    const category = await apiClient.createCategory('Resource Test Category', 'Category for resource tests');
    categoryId = category.ID;

    const ownerGroup = await apiClient.createGroup({
      name: 'Resource Owner Group',
      description: 'Owner for resources',
      categoryId: categoryId,
    });
    ownerGroupId = ownerGroup.ID;

    const tag = await apiClient.createTag('Resource Test Tag', 'Tag for resources');
    tagId = tag.ID;
  });

  test('should upload a file resource', async ({ resourcePage }) => {
    const testFilePath = path.join(__dirname, '../test-assets/sample-image.png');

    createdResourceId = await resourcePage.createFromFile({
      filePath: testFilePath,
      name: 'E2E Test Image',
      description: 'Image uploaded by E2E test',
      ownerGroupName: 'Resource Owner Group',
      tags: ['Resource Test Tag'],
    });

    expect(createdResourceId).toBeGreaterThan(0);
  });

  test('should display the created resource', async ({ resourcePage, page }) => {
    expect(createdResourceId, 'Resource must be created first').toBeGreaterThan(0);
    await resourcePage.gotoDisplay(createdResourceId);
    await expect(page.locator('h1, .title, text=E2E Test Image')).toBeVisible();
  });

  test('should update the resource', async ({ resourcePage, page }) => {
    expect(createdResourceId, 'Resource must be created first').toBeGreaterThan(0);
    await resourcePage.update(createdResourceId, {
      name: 'Updated E2E Image',
      description: 'Updated image description',
    });
    await expect(page.locator('text=Updated E2E Image')).toBeVisible();
  });

  test('should delete the resource', async ({ resourcePage }) => {
    expect(createdResourceId, 'Resource must be created first').toBeGreaterThan(0);
    await resourcePage.delete(createdResourceId);
    await resourcePage.verifyResourceNotInList('Updated E2E Image');
  });

  test.afterAll(async ({ apiClient }) => {
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

  test.beforeAll(async ({ apiClient }) => {
    const category = await apiClient.createCategory('URL Resource Category', 'Category for URL resources');
    categoryId = category.ID;

    const ownerGroup = await apiClient.createGroup({
      name: 'URL Resource Owner',
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
      name: 'Remote Image Resource',
      ownerGroupName: 'URL Resource Owner',
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
