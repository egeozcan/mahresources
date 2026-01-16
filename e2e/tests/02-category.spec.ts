import { test, expect } from '../fixtures/base.fixture';

test.describe.serial('Category CRUD Operations', () => {
  let createdCategoryId: number;

  test('should create a new category', async ({ categoryPage }) => {
    createdCategoryId = await categoryPage.create(
      'E2E Test Category',
      'Category created by E2E tests'
    );
    expect(createdCategoryId).toBeGreaterThan(0);
  });

  test('should display the created category', async ({ categoryPage, page }) => {
    expect(createdCategoryId, 'Category must be created first').toBeGreaterThan(0);
    await categoryPage.gotoDisplay(createdCategoryId);
    await expect(page.locator('h1, .title')).toContainText('E2E Test Category');
  });

  test('should update the category', async ({ categoryPage, page }) => {
    expect(createdCategoryId, 'Category must be created first').toBeGreaterThan(0);
    await categoryPage.update(createdCategoryId, {
      name: 'Updated E2E Category',
      description: 'Updated category description',
    });
    await expect(page.locator('h1, .title')).toContainText('Updated E2E Category');
  });

  test('should list the category', async ({ categoryPage }) => {
    await categoryPage.verifyCategoryInList('Updated E2E Category');
  });

  test('should delete the category', async ({ categoryPage }) => {
    expect(createdCategoryId, 'Category must be created first').toBeGreaterThan(0);
    await categoryPage.delete(createdCategoryId);
    await categoryPage.verifyCategoryNotInList('Updated E2E Category');
  });
});

test.describe('Category with Custom Fields', () => {
  let categoryWithSchemaId: number;

  test('should create category with MetaSchema', async ({ categoryPage, page }) => {
    const metaSchema = JSON.stringify({
      type: 'object',
      properties: {
        birthDate: { type: 'string', format: 'date' },
        occupation: { type: 'string' },
      },
    });

    categoryWithSchemaId = await categoryPage.create(
      'Person Category',
      'Category for person groups',
      {
        customHeader: '<div class="custom-header">Person</div>',
        customSidebar: 'Sidebar content',
        metaSchema: metaSchema,
      }
    );

    expect(categoryWithSchemaId).toBeGreaterThan(0);
  });

  test.afterAll(async ({ apiClient }) => {
    if (categoryWithSchemaId) {
      await apiClient.deleteCategory(categoryWithSchemaId);
    }
  });
});

test.describe('Category Validation', () => {
  test('should require name field', async ({ categoryPage, page }) => {
    await categoryPage.gotoNew();
    await categoryPage.save();
    // HTML5 required validation prevents submission
    await expect(page).toHaveURL(/\/category\/new/);
  });
});
