import { test, expect } from '../fixtures/base.fixture';
import path from 'path';

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

test.describe('Resource Category Custom Template Rendering', () => {
  const testRunId = Date.now() + Math.floor(Math.random() * 100000);
  let groupCategoryId: number;
  let ownerGroupId: number;
  let resourceCategoryId: number;
  let resourceId: number;

  const customHeader = '<div class="rc-test-header" data-testid="rc-custom-header">Custom Header Content</div>';
  const customSidebar = '<div class="rc-test-sidebar" data-testid="rc-custom-sidebar">Custom Sidebar Content</div>';
  const customSummary = '<div class="rc-test-summary" data-testid="rc-custom-summary">Custom Summary Content</div>';
  const customAvatar = '<span class="rc-test-avatar" data-testid="rc-custom-avatar">★</span>';

  test.beforeAll(async ({ apiClient }) => {
    // Create a group category and owner group (required for resource creation)
    const groupCategory = await apiClient.createCategory(
      `RC Template Test Category ${testRunId}`,
      'Group category for RC template tests'
    );
    groupCategoryId = groupCategory.ID;

    const ownerGroup = await apiClient.createGroup({
      name: `RC Template Test Owner ${testRunId}`,
      categoryId: groupCategoryId,
    });
    ownerGroupId = ownerGroup.ID;

    // Create a resource category with all custom templates
    const rc = await apiClient.createResourceCategory(
      `Template RC ${testRunId}`,
      'Resource category with all custom templates',
      {
        CustomHeader: customHeader,
        CustomSidebar: customSidebar,
        CustomSummary: customSummary,
        CustomAvatar: customAvatar,
      }
    );
    resourceCategoryId = rc.ID;

    // Create a resource assigned to the resource category
    // Use sample-image-21.png which isn't used by other tests
    const resource = await apiClient.createResource({
      filePath: path.join(__dirname, '../test-assets/sample-image-21.png'),
      name: `RC Template Test Image ${testRunId}`,
      description: 'Resource for testing custom template rendering',
      ownerId: ownerGroupId,
      resourceCategoryId: resourceCategoryId,
    });
    resourceId = resource.ID;
  });

  test.afterAll(async ({ apiClient }) => {
    if (resourceId) await apiClient.deleteResource(resourceId).catch(() => {});
    if (ownerGroupId) await apiClient.deleteGroup(ownerGroupId).catch(() => {});
    if (groupCategoryId) await apiClient.deleteCategory(groupCategoryId).catch(() => {});
    if (resourceCategoryId) await apiClient.deleteResourceCategory(resourceCategoryId).catch(() => {});
  });

  test('should render CustomHeader on resource detail page', async ({ page }) => {
    await page.goto(`/resource?id=${resourceId}`);
    await page.waitForLoadState('load');

    const header = page.locator('[data-testid="rc-custom-header"]');
    await expect(header).toBeVisible();
    await expect(header).toContainText('Custom Header Content');
  });

  test('should render CustomSidebar on resource detail page', async ({ page }) => {
    await page.goto(`/resource?id=${resourceId}`);
    await page.waitForLoadState('load');

    const sidebar = page.locator('[data-testid="rc-custom-sidebar"]');
    await expect(sidebar).toBeVisible();
    await expect(sidebar).toContainText('Custom Sidebar Content');
  });

  test('should render CustomSummary on resource card in list', async ({ page }) => {
    await page.goto(`/resources?ResourceCategoryId=${resourceCategoryId}`);
    await page.waitForLoadState('load');

    const summary = page.locator('[data-testid="rc-custom-summary"]');
    await expect(summary).toBeVisible();
    await expect(summary).toContainText('Custom Summary Content');
  });

  test('should render CustomAvatar on resource card in list', async ({ page }) => {
    await page.goto(`/resources?ResourceCategoryId=${resourceCategoryId}`);
    await page.waitForLoadState('load');

    const avatar = page.locator('[data-testid="rc-custom-avatar"]');
    await expect(avatar).toBeVisible();
    await expect(avatar).toContainText('★');
  });

  test('should render category name and CustomSidebar in lightbox edit drawer', async ({ page }) => {
    await page.goto(`/resources?ResourceCategoryId=${resourceCategoryId}`);
    await page.waitForLoadState('load');

    // Open lightbox
    const imageLink = page.locator('[data-lightbox-item]').first();
    await expect(imageLink).toBeVisible();
    await imageLink.click();

    const lightbox = page.locator('[role="dialog"][aria-modal="true"]');
    await expect(lightbox).toBeVisible();

    // Open edit panel
    const editButton = lightbox.locator('button:has-text("Edit")');
    await editButton.click();

    const editPanel = lightbox.locator('[data-edit-panel]');
    await expect(editPanel).toBeVisible();

    // Wait for resource details to load
    await page.waitForTimeout(500);

    // Verify category name link is visible
    const categoryLabel = editPanel.locator('label:has-text("Category")');
    await expect(categoryLabel).toBeVisible();

    const categoryLink = editPanel.locator(`a:has-text("Template RC ${testRunId}")`);
    await expect(categoryLink).toBeVisible();
    await expect(categoryLink).toHaveAttribute('href', `/resourceCategory?id=${resourceCategoryId}`);

    // Verify CustomSidebar content is rendered
    await expect(editPanel.locator('text=Custom Sidebar Content')).toBeVisible();
  });

  test('should not show category section in lightbox for resources without a category', async ({ apiClient, page }) => {
    // Create a resource without a category via apiClient
    const noCatResource = await apiClient.createResource({
      filePath: path.join(__dirname, '../test-assets/sample-image-22.png'),
      name: `No Category Resource ${testRunId}`,
    });
    const noCatResourceId = noCatResource.ID;

    try {
      // Navigate to resources sorted by ID desc so our new resource appears first
      // Use large pageSize to ensure visibility when parallel tests create many resources
      await page.goto('/resources?sort=ID&order=desc&pageSize=100');
      await page.waitForLoadState('load');

      // Find the lightbox-item anchor for this specific resource
      // Retry with reload if resource isn't visible (SQLite read-after-write visibility)
      const imageLink = page.locator(`a[data-resource-id="${noCatResourceId}"]`);
      if (!(await imageLink.isVisible())) {
        await page.waitForTimeout(500);
        await page.reload();
        await page.waitForLoadState('load');
      }
      await expect(imageLink).toBeVisible({ timeout: 10000 });
      await imageLink.click();

      const lightbox = page.locator('[role="dialog"][aria-modal="true"]');
      await expect(lightbox).toBeVisible();

      // Open edit panel
      const editButton = lightbox.locator('button:has-text("Edit")');
      await editButton.click();

      const editPanel = lightbox.locator('[data-edit-panel]');
      await expect(editPanel).toBeVisible();

      // Wait for resource details to load
      await page.waitForTimeout(500);

      // Category section should NOT be visible
      const categoryLabel = editPanel.locator('label:has-text("Category")');
      await expect(categoryLabel).not.toBeVisible();
    } finally {
      if (noCatResourceId) {
        await apiClient.deleteResource(noCatResourceId).catch(() => {});
      }
    }
  });
});
