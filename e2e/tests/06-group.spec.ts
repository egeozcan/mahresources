import { test, expect } from '../fixtures/base.fixture';

test.describe.serial('Group CRUD Operations', () => {
  let categoryId: number;
  let tagId: number;
  let ownerGroupId: number;
  let createdGroupId: number;

  test.beforeAll(async ({ apiClient }) => {
    // Create prerequisite data
    const category = await apiClient.createCategory('Group Test Category', 'Category for group tests');
    categoryId = category.ID;

    const tag = await apiClient.createTag('Group Test Tag', 'Tag for group tests');
    tagId = tag.ID;

    // Create an owner group
    const ownerGroup = await apiClient.createGroup({
      name: 'Owner Group',
      description: 'Owner for other groups',
      categoryId: categoryId,
    });
    ownerGroupId = ownerGroup.ID;
  });

  test('should create a new group via UI', async ({ groupPage }) => {
    createdGroupId = await groupPage.create({
      name: 'E2E Test Group',
      description: 'Group created by E2E test',
      categoryName: 'Group Test Category',
      url: 'https://example.com',
      tags: ['Group Test Tag'],
      ownerGroupName: 'Owner Group',
    });
    expect(createdGroupId).toBeGreaterThan(0);
  });

  test('should display the created group with relationships', async ({ groupPage, page }) => {
    await groupPage.gotoDisplay(createdGroupId);

    // Verify basic info
    await expect(page.locator('h1, .title')).toContainText('E2E Test Group');

    // Verify tag is shown
    await groupPage.verifyHasTag('Group Test Tag');

    // Verify owner is shown
    await groupPage.verifyHasOwner('Owner Group');
  });

  test('should update the group', async ({ groupPage, page }) => {
    await groupPage.update(createdGroupId, {
      name: 'Updated E2E Group',
      description: 'Updated group description',
      url: 'https://updated-example.com',
    });
    await expect(page.locator('h1, .title')).toContainText('Updated E2E Group');
  });

  test('should list the group', async ({ groupPage }) => {
    await groupPage.verifyGroupInList('Updated E2E Group');
  });

  test('should delete the group', async ({ groupPage }) => {
    await groupPage.delete(createdGroupId);
    await groupPage.verifyGroupNotInList('Updated E2E Group');
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

test.describe('Group Validation', () => {
  let categoryId: number;

  test.beforeAll(async ({ apiClient }) => {
    const category = await apiClient.createCategory('Validation Test Category', 'For validation tests');
    categoryId = category.ID;
  });

  test('should require category when creating group', async ({ groupPage, page }) => {
    await groupPage.gotoNew();
    await groupPage.fillName('Group Without Category');
    await groupPage.save();

    // Category is required - form should not submit successfully
    // Either we stay on the new group form or see an error
    const stayedOnForm = page.url().includes('/group/new');
    const hasError = await page.locator('.error, [class*="error"], [class*="Error"]').isVisible();

    // Form should have been blocked from submission
    expect(stayedOnForm || hasError).toBeTruthy();
  });

  test.afterAll(async ({ apiClient }) => {
    if (categoryId) {
      await apiClient.deleteCategory(categoryId);
    }
  });
});

test.describe('Group Hierarchy', () => {
  let categoryId: number;
  let parentGroupId: number;
  let childGroupId: number;
  const testRunId = Date.now();

  test.beforeAll(async ({ apiClient }) => {
    const category = await apiClient.createCategory(`Hierarchy Test Category ${testRunId}`, 'For hierarchy tests');
    categoryId = category.ID;

    const parentGroup = await apiClient.createGroup({
      name: `Parent Group ${testRunId}`,
      description: 'Parent in hierarchy',
      categoryId: categoryId,
    });
    parentGroupId = parentGroup.ID;
  });

  test('should create child group with owner', async ({ groupPage }) => {
    childGroupId = await groupPage.create({
      name: `Child Group ${testRunId}`,
      description: 'Child in hierarchy',
      categoryName: `Hierarchy Test Category ${testRunId}`,
      ownerGroupName: `Parent Group ${testRunId}`,
    });
    expect(childGroupId).toBeGreaterThan(0);
  });

  test('should display parent on child group page', async ({ groupPage, page }) => {
    await groupPage.gotoDisplay(childGroupId);
    // Use .first() to avoid strict mode violations
    await expect(page.locator(`text=Parent Group ${testRunId}`).first()).toBeVisible();
  });

  test.afterAll(async ({ apiClient }) => {
    if (childGroupId) {
      await apiClient.deleteGroup(childGroupId);
    }
    if (parentGroupId) {
      await apiClient.deleteGroup(parentGroupId);
    }
    if (categoryId) {
      await apiClient.deleteCategory(categoryId);
    }
  });
});
