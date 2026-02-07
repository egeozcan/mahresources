import { test, expect } from '../fixtures/base.fixture';

test.describe.serial('Resource Category CRUD Operations', () => {
  let createdId: number;

  test('should create a new resource category', async ({ resourceCategoryPage }) => {
    createdId = await resourceCategoryPage.create(
      'E2E Test Resource Category',
      'Resource category created by E2E tests'
    );
    expect(createdId).toBeGreaterThan(0);
  });

  test('should display the created resource category', async ({ resourceCategoryPage, page }) => {
    expect(createdId, 'Resource category must be created first').toBeGreaterThan(0);
    await resourceCategoryPage.gotoDisplay(createdId);
    await expect(page.locator('h1, .title')).toContainText('E2E Test Resource Category');
  });

  test('should update the resource category', async ({ resourceCategoryPage, page }) => {
    expect(createdId, 'Resource category must be created first').toBeGreaterThan(0);
    await resourceCategoryPage.update(createdId, {
      name: 'Updated E2E Resource Category',
      description: 'Updated description',
    });
    await expect(page.locator('h1, .title')).toContainText('Updated E2E Resource Category');
  });

  test('should list the resource category', async ({ resourceCategoryPage }) => {
    await resourceCategoryPage.verifyInList('Updated E2E Resource Category');
  });

  test('should delete the resource category', async ({ resourceCategoryPage }) => {
    expect(createdId, 'Resource category must be created first').toBeGreaterThan(0);
    await resourceCategoryPage.delete(createdId);
    await resourceCategoryPage.verifyNotInList('Updated E2E Resource Category');
  });
});

test.describe('Resource Category with Custom Fields', () => {
  let categoryWithSchemaId: number;

  test('should create resource category with MetaSchema', async ({ resourceCategoryPage }) => {
    const metaSchema = JSON.stringify({
      type: 'object',
      properties: {
        resolution: { type: 'string' },
        format: { type: 'string' },
      },
    });

    categoryWithSchemaId = await resourceCategoryPage.create(
      'Media Resource Category',
      'Category for media resources',
      {
        customHeader: '<div class="custom-header">Media</div>',
        customSidebar: 'Sidebar content',
        metaSchema: metaSchema,
      }
    );

    expect(categoryWithSchemaId).toBeGreaterThan(0);
  });

  test.afterAll(async ({ apiClient }) => {
    if (categoryWithSchemaId) {
      await apiClient.deleteResourceCategory(categoryWithSchemaId);
    }
  });
});

test.describe('Resource Category Validation', () => {
  test('should require name field', async ({ resourceCategoryPage, page }) => {
    await resourceCategoryPage.gotoNew();
    await resourceCategoryPage.save();
    // Should stay on the new page due to validation
    await expect(page).toHaveURL(/\/resourceCategory\/new/);
  });
});
