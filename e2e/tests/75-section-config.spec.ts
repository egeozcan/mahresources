import { test, expect } from '../fixtures/base.fixture';
import * as fs from 'fs';
import * as path from 'path';

test.describe.serial('Group Section Config - Hidden sections', () => {
  let categoryId: number;
  let groupId: number;

  test.beforeAll(async ({ apiClient }) => {
    const category = await apiClient.createCategory(
      `SC Hidden Test ${Date.now()}`,
      'Category with hidden sections',
      {
        SectionConfig: JSON.stringify({
          tags: false,
          clone: false,
          merge: false,
          relations: { state: 'off' },
        }),
      }
    );
    categoryId = category.ID;

    const group = await apiClient.createGroup({
      name: `SC Hidden Group ${Date.now()}`,
      description: 'Group with hidden sections',
      categoryId,
    });
    groupId = group.ID;
  });

  test.afterAll(async ({ apiClient }) => {
    await apiClient.deleteGroup(groupId).catch(() => {});
    await apiClient.deleteCategory(categoryId).catch(() => {});
  });

  test('should not show Tags section', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    await page.waitForLoadState('load');

    // Tags section in sidebar should not be present (no addTags form)
    const tagForm = page.locator('form[action*="addTags"]');
    await expect(tagForm).toHaveCount(0);
  });

  test('should not show Clone form', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    await page.waitForLoadState('load');

    await expect(page.locator('text=Clone group?')).toHaveCount(0);
  });

  test('should not show Merge form', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    await page.waitForLoadState('load');

    await expect(page.locator('text=Merge others with this group?')).toHaveCount(0);
  });

  test('should not show Relations section', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    await page.waitForLoadState('load');

    // The detail-collapsible <details> with "Relations" summary should not exist
    const relationsDetails = page.locator('details.detail-collapsible:has(summary:text-is("Relations"))');
    await expect(relationsDetails).toHaveCount(0);
  });
});

test.describe.serial('Group Section Config - Collapsed state', () => {
  let categoryId: number;
  let groupId: number;

  test.beforeAll(async ({ apiClient }) => {
    const category = await apiClient.createCategory(
      `SC Collapsed Test ${Date.now()}`,
      'Category with collapsed section',
      {
        SectionConfig: JSON.stringify({
          ownEntities: { state: 'collapsed' },
        }),
      }
    );
    categoryId = category.ID;

    const group = await apiClient.createGroup({
      name: `SC Collapsed Group ${Date.now()}`,
      description: 'Group with collapsed Own Entities',
      categoryId,
    });
    groupId = group.ID;
  });

  test.afterAll(async ({ apiClient }) => {
    await apiClient.deleteGroup(groupId).catch(() => {});
    await apiClient.deleteCategory(categoryId).catch(() => {});
  });

  test('should render Own Entities as collapsed (no open attribute)', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    await page.waitForLoadState('load');

    const ownEntitiesDetails = page.locator('details:has(summary:text-is("Own Entities"))');
    await expect(ownEntitiesDetails).toHaveCount(1);
    // The details element should exist but NOT have the open attribute
    await expect(ownEntitiesDetails).not.toHaveAttribute('open', /.*/);
  });
});

test.describe.serial('Group Section Config - Open state', () => {
  let categoryId: number;
  let groupId: number;

  test.beforeAll(async ({ apiClient }) => {
    const category = await apiClient.createCategory(
      `SC Open Test ${Date.now()}`,
      'Category with open relations',
      {
        SectionConfig: JSON.stringify({
          relations: { state: 'open' },
        }),
      }
    );
    categoryId = category.ID;

    const group = await apiClient.createGroup({
      name: `SC Open Group ${Date.now()}`,
      description: 'Group with open Relations',
      categoryId,
    });
    groupId = group.ID;
  });

  test.afterAll(async ({ apiClient }) => {
    await apiClient.deleteGroup(groupId).catch(() => {});
    await apiClient.deleteCategory(categoryId).catch(() => {});
  });

  test('should render Relations as open', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    await page.waitForLoadState('load');

    const relationsDetails = page.locator('details.detail-collapsible:has(summary:text-is("Relations"))');
    await expect(relationsDetails).toHaveCount(1);
    await expect(relationsDetails).toHaveAttribute('open', /.*/);
  });
});

test.describe.serial('Group Section Config - Default behavior', () => {
  let categoryId: number;
  let groupId: number;

  test.beforeAll(async ({ apiClient }) => {
    const category = await apiClient.createCategory(
      `SC Default Test ${Date.now()}`,
      'Category with no SectionConfig'
    );
    categoryId = category.ID;

    const group = await apiClient.createGroup({
      name: `SC Default Group ${Date.now()}`,
      description: 'Group with all defaults',
      categoryId,
    });
    groupId = group.ID;
  });

  test.afterAll(async ({ apiClient }) => {
    await apiClient.deleteGroup(groupId).catch(() => {});
    await apiClient.deleteCategory(categoryId).catch(() => {});
  });

  test('should show all key sections by default', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    await page.waitForLoadState('load');

    // Own Entities and Related Entities summaries should be visible
    await expect(page.locator('summary:text-is("Own Entities")')).toHaveCount(1);
    await expect(page.locator('summary:text-is("Related Entities")')).toHaveCount(1);
    await expect(page.locator('details.detail-collapsible:has(summary:text-is("Relations"))')).toHaveCount(1);

    // Clone and Merge forms should be visible
    await expect(page.locator('text=Clone group?')).toHaveCount(1);
    await expect(page.locator('text=Merge others with this group?')).toHaveCount(1);

    // Tags section (addTags form) should be visible
    const tagForm = page.locator('form[action*="addTags"]');
    await expect(tagForm).toHaveCount(1);
  });
});

test.describe.serial('Resource Section Config', () => {
  let categoryId: number;
  let resCategoryId: number;
  let groupId: number;
  let resourceId: number;
  let tmpFile: string;

  test.beforeAll(async ({ apiClient }) => {
    tmpFile = path.join(process.cwd(), `sc-test-${Date.now()}.txt`);
    fs.writeFileSync(tmpFile, 'section config test content');

    const category = await apiClient.createCategory(
      `SC Res Category ${Date.now()}`,
      'For resource section config test'
    );
    categoryId = category.ID;

    const resCategory = await apiClient.createResourceCategory(
      `SC ResCategory ${Date.now()}`,
      'Resource category with hidden sections',
      {
        SectionConfig: JSON.stringify({
          tags: false,
          metadataGrid: false,
          technicalDetails: { state: 'off' },
        }),
      } as any
    );
    resCategoryId = resCategory.ID;

    const group = await apiClient.createGroup({
      name: `SC Res Owner ${Date.now()}`,
      description: 'Owner for resource section config test',
      categoryId,
    });
    groupId = group.ID;

    const resource = await apiClient.createResource({
      filePath: tmpFile,
      name: `SC Test Resource ${Date.now()}`,
      ownerId: groupId,
      resourceCategoryId: resCategoryId,
    });
    resourceId = resource.ID;
  });

  test.afterAll(async ({ apiClient }) => {
    await apiClient.deleteResource(resourceId).catch(() => {});
    await apiClient.deleteGroup(groupId).catch(() => {});
    await apiClient.deleteResourceCategory(resCategoryId).catch(() => {});
    await apiClient.deleteCategory(categoryId).catch(() => {});
    try { fs.unlinkSync(tmpFile); } catch { /* ignore */ }
  });

  test('should not show metadata panel when grid and details are off', async ({ page }) => {
    await page.goto(`/resource?Id=${resourceId}`);
    await page.waitForLoadState('load');

    const metadataPanel = page.locator('[aria-label="Resource metadata"]');
    await expect(metadataPanel).toHaveCount(0);
  });

  test('should not show Tags in sidebar', async ({ page }) => {
    await page.goto(`/resource?Id=${resourceId}`);
    await page.waitForLoadState('load');

    const tagForm = page.locator('form[action*="addTags"]');
    await expect(tagForm).toHaveCount(0);
  });
});

test.describe.serial('Section Config Edit Form', () => {
  let categoryId: number;

  test.beforeAll(async ({ apiClient }) => {
    const category = await apiClient.createCategory(
      `SC Edit Form Test ${Date.now()}`,
      'Category for edit form test'
    );
    categoryId = category.ID;
  });

  test.afterAll(async ({ apiClient }) => {
    await apiClient.deleteCategory(categoryId).catch(() => {});
  });

  test('should persist Tags checkbox state after save', async ({ page }) => {
    // Navigate to edit page
    await page.goto(`/category/edit?id=${categoryId}`);
    await page.waitForLoadState('load');

    // Wait for Alpine.js to initialize the sectionConfigForm component
    const fieldset = page.locator('fieldset:has(legend:text-is("Section Visibility"))');
    await expect(fieldset).toBeVisible();

    // The hidden input is populated by Alpine - wait for it to have a value
    const hiddenInput = fieldset.locator('input[name="SectionConfig"]');
    await expect(hiddenInput).toHaveAttribute('value', /.+/);

    // Find the Tags checkbox and verify it starts checked (default)
    const tagsCheckbox = fieldset.locator('label:has-text("Tags") input[type="checkbox"]').first();
    await expect(tagsCheckbox).toBeChecked();

    // Uncheck it
    await tagsCheckbox.uncheck();
    await expect(tagsCheckbox).not.toBeChecked();

    // Save the form - click the submit button
    await page.locator('button[type="submit"], input[type="submit"]').first().click();
    await page.waitForLoadState('load');

    // Navigate back to edit page
    await page.goto(`/category/edit?id=${categoryId}`);
    await page.waitForLoadState('load');

    // Wait for Alpine.js to initialize again
    const fieldsetAfter = page.locator('fieldset:has(legend:text-is("Section Visibility"))');
    await expect(fieldsetAfter).toBeVisible();
    const hiddenInputAfter = fieldsetAfter.locator('input[name="SectionConfig"]');
    await expect(hiddenInputAfter).toHaveAttribute('value', /.+/);

    // Verify Tags checkbox is still unchecked (value persisted)
    const tagsCheckboxAfter = fieldsetAfter.locator('label:has-text("Tags") input[type="checkbox"]').first();
    await expect(tagsCheckboxAfter).not.toBeChecked();
  });
});
