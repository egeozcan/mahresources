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

test.describe('Group Category Custom Template Rendering', () => {
  const testRunId = Date.now() + Math.floor(Math.random() * 100000);
  let categoryId: number;
  let parentGroupId: number;
  let groupId: number;

  const customHeader = '<div data-testid="gc-custom-header">Group Custom Header</div>';
  const customSidebar = '<div data-testid="gc-custom-sidebar">Group Custom Sidebar</div>';
  const customSummary = '<div data-testid="gc-custom-summary">Group Custom Summary</div>';

  test.beforeAll(async ({ apiClient }) => {
    const category = await apiClient.createCategory(
      `GC Template Test ${testRunId}`,
      'Group category for custom template rendering tests',
      {
        CustomHeader: customHeader,
        CustomSidebar: customSidebar,
        CustomSummary: customSummary,
      }
    );
    categoryId = category.ID;

    // Create a parent group to own the child group (for sub-groups list test)
    const parentGroup = await apiClient.createGroup({
      name: `GC Template Parent ${testRunId}`,
      categoryId: categoryId,
    });
    parentGroupId = parentGroup.ID;

    const group = await apiClient.createGroup({
      name: `GC Template Test Group ${testRunId}`,
      categoryId: categoryId,
      ownerId: parentGroupId,
    });
    groupId = group.ID;
  });

  test.afterAll(async ({ apiClient }) => {
    if (groupId) await apiClient.deleteGroup(groupId).catch(() => {});
    if (parentGroupId) await apiClient.deleteGroup(parentGroupId).catch(() => {});
    if (categoryId) await apiClient.deleteCategory(categoryId).catch(() => {});
  });

  test('should render CustomHeader on group detail page', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    await page.waitForLoadState('load');

    const header = page.locator('[data-testid="gc-custom-header"]');
    await expect(header).toBeVisible();
    await expect(header).toContainText('Group Custom Header');
  });

  test('should render CustomSidebar on group detail page', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    await page.waitForLoadState('load');

    const sidebar = page.locator('[data-testid="gc-custom-sidebar"]');
    await expect(sidebar).toBeVisible();
    await expect(sidebar).toContainText('Group Custom Sidebar');
  });

  test('should render CustomSummary on group card in list', async ({ page }) => {
    await page.goto(`/groups?categories=${categoryId}`);
    await page.waitForLoadState('load');

    const summary = page.locator('[data-testid="gc-custom-summary"]').first();
    await expect(summary).toBeVisible();
    await expect(summary).toContainText('Group Custom Summary');
  });

  test('should render CustomSummary on sub-group card in parent detail page', async ({ page }) => {
    await page.goto(`/group?id=${parentGroupId}`);
    await page.waitForLoadState('load');

    const summary = page.locator('[data-testid="gc-custom-summary"]');
    await expect(summary).toBeVisible();
    await expect(summary).toContainText('Group Custom Summary');
  });
});
