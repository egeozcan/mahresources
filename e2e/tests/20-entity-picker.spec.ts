import { test, expect } from '../fixtures/base.fixture';

test.describe('Entity Picker - Resource Selection', () => {
  let categoryId: number;
  let ownerGroupId: number;
  let noteId: number;
  let resourceId: number;

  test.beforeAll(async ({ apiClient }) => {
    // Create prerequisite data
    const category = await apiClient.createCategory('Picker Test Category', 'Category for picker tests');
    categoryId = category.ID;

    const ownerGroup = await apiClient.createGroup({
      name: 'Picker Test Owner',
      categoryId: categoryId,
    });
    ownerGroupId = ownerGroup.ID;

    const note = await apiClient.createNote({
      name: 'Picker Test Note',
      description: 'Note for testing entity picker',
      ownerId: ownerGroupId,
    });
    noteId = note.ID;

    // Create a resource to select
    const resource = await apiClient.createResource({
      name: 'Test Resource for Picker',
      groupId: ownerGroupId,
    });
    resourceId = resource.ID;
  });

  test('should open resource picker from gallery block', async ({ page, baseURL, apiClient }) => {
    // Create a gallery block
    await apiClient.createBlock(noteId, 'gallery', 'n', { resourceIds: [] });

    await page.goto(`${baseURL}/note?id=${noteId}`);
    await page.waitForLoadState('load');

    // Enter edit mode
    await page.locator('button:has-text("Edit Blocks")').click();
    await expect(page.locator('button:has-text("Done")')).toBeVisible();

    // Click Select Resources button
    await page.locator('button:has-text("Select Resources")').click();

    // Modal should open
    await expect(page.locator('[role="dialog"]')).toBeVisible();
    await expect(page.locator('h2:has-text("Select Resources")')).toBeVisible();
  });

  test('should search resources in picker', async ({ page, baseURL, apiClient }) => {
    await apiClient.createBlock(noteId, 'gallery', 'o', { resourceIds: [] });

    await page.goto(`${baseURL}/note?id=${noteId}`);
    await page.waitForLoadState('load');

    await page.locator('button:has-text("Edit Blocks")').click();
    await page.locator('button:has-text("Select Resources")').click();

    // Switch to All Resources tab
    await page.locator('button:has-text("All Resources")').click();

    // Search for resource
    const searchInput = page.locator('[role="dialog"] input[placeholder="Search by name..."]');
    await searchInput.fill('Test Resource');

    // Wait for results
    await page.waitForTimeout(300); // Debounce wait

    // Should show matching resource
    await expect(page.locator('[role="option"]')).toBeVisible();
  });

  test('should select and confirm resources', async ({ page, baseURL, apiClient }) => {
    await apiClient.createBlock(noteId, 'gallery', 'p', { resourceIds: [] });

    await page.goto(`${baseURL}/note?id=${noteId}`);
    await page.waitForLoadState('load');

    await page.locator('button:has-text("Edit Blocks")').click();
    await page.locator('button:has-text("Select Resources")').click();
    await page.locator('button:has-text("All Resources")').click();

    // Click on a resource to select it
    const resourceOption = page.locator('[role="option"]').first();
    await resourceOption.click();

    // Selection count should update
    await expect(page.locator('text=1 selected')).toBeVisible();

    // Confirm selection
    await page.locator('button:has-text("Confirm")').click();

    // Modal should close
    await expect(page.locator('[role="dialog"]')).not.toBeVisible();
  });

  test('should cancel selection without adding', async ({ page, baseURL, apiClient }) => {
    await apiClient.createBlock(noteId, 'gallery', 'q', { resourceIds: [] });

    await page.goto(`${baseURL}/note?id=${noteId}`);
    await page.waitForLoadState('load');

    await page.locator('button:has-text("Edit Blocks")').click();
    await page.locator('button:has-text("Select Resources")').click();
    await page.locator('button:has-text("All Resources")').click();

    // Select a resource
    await page.locator('[role="option"]').first().click();
    await expect(page.locator('text=1 selected')).toBeVisible();

    // Cancel
    await page.locator('button:has-text("Cancel")').click();

    // Modal should close
    await expect(page.locator('[role="dialog"]')).not.toBeVisible();
  });

  test('should close picker with escape key', async ({ page, baseURL, apiClient }) => {
    await apiClient.createBlock(noteId, 'gallery', 'r', { resourceIds: [] });

    await page.goto(`${baseURL}/note?id=${noteId}`);
    await page.waitForLoadState('load');

    await page.locator('button:has-text("Edit Blocks")').click();
    await page.locator('button:has-text("Select Resources")').click();

    await expect(page.locator('[role="dialog"]')).toBeVisible();

    // Press Escape
    await page.keyboard.press('Escape');

    // Modal should close
    await expect(page.locator('[role="dialog"]')).not.toBeVisible();
  });

  test.afterAll(async ({ apiClient }) => {
    if (noteId) await apiClient.deleteNote(noteId);
    if (ownerGroupId) await apiClient.deleteGroup(ownerGroupId);
    if (categoryId) await apiClient.deleteCategory(categoryId);
  });
});

test.describe('Entity Picker - Group Selection', () => {
  let categoryId: number;
  let ownerGroupId: number;
  let noteId: number;
  let selectableGroupId: number;

  test.beforeAll(async ({ apiClient }) => {
    const category = await apiClient.createCategory('Group Picker Category', 'For group picker tests');
    categoryId = category.ID;

    const ownerGroup = await apiClient.createGroup({
      name: 'Group Picker Owner',
      categoryId: categoryId,
    });
    ownerGroupId = ownerGroup.ID;

    // Create a group to select
    const selectableGroup = await apiClient.createGroup({
      name: 'Selectable Test Group',
      categoryId: categoryId,
    });
    selectableGroupId = selectableGroup.ID;

    const note = await apiClient.createNote({
      name: 'Group Picker Test Note',
      ownerId: ownerGroupId,
    });
    noteId = note.ID;
  });

  test('should open group picker from references block', async ({ page, baseURL, apiClient }) => {
    await apiClient.createBlock(noteId, 'references', 'n', { groupIds: [] });

    await page.goto(`${baseURL}/note?id=${noteId}`);
    await page.waitForLoadState('load');

    await page.locator('button:has-text("Edit Blocks")').click();
    await page.locator('button:has-text("Select Groups")').click();

    await expect(page.locator('[role="dialog"]')).toBeVisible();
    await expect(page.locator('h2:has-text("Select Groups")')).toBeVisible();
  });

  test('should display groups as cards', async ({ page, baseURL, apiClient }) => {
    await apiClient.createBlock(noteId, 'references', 'o', { groupIds: [] });

    await page.goto(`${baseURL}/note?id=${noteId}`);
    await page.waitForLoadState('load');

    await page.locator('button:has-text("Edit Blocks")').click();
    await page.locator('button:has-text("Select Groups")').click();

    // Group cards should be visible (not thumbnails)
    const groupCard = page.locator('[role="option"]').first();
    await expect(groupCard).toBeVisible();

    // Should contain group name text
    await expect(groupCard.locator('p.font-medium')).toBeVisible();
  });

  test('should select and confirm groups', async ({ page, baseURL, apiClient }) => {
    await apiClient.createBlock(noteId, 'references', 'p', { groupIds: [] });

    await page.goto(`${baseURL}/note?id=${noteId}`);
    await page.waitForLoadState('load');

    await page.locator('button:has-text("Edit Blocks")').click();
    await page.locator('button:has-text("Select Groups")').click();

    // Select a group
    await page.locator('[role="option"]').first().click();
    await expect(page.locator('text=1 selected')).toBeVisible();

    // Confirm
    await page.locator('button:has-text("Confirm")').click();

    // Modal closes and group appears in block
    await expect(page.locator('[role="dialog"]')).not.toBeVisible();
  });

  test('should filter groups by category', async ({ page, baseURL, apiClient }) => {
    await apiClient.createBlock(noteId, 'references', 'q', { groupIds: [] });

    await page.goto(`${baseURL}/note?id=${noteId}`);
    await page.waitForLoadState('load');

    await page.locator('button:has-text("Edit Blocks")').click();
    await page.locator('button:has-text("Select Groups")').click();

    // Category filter should be visible
    const categoryFilter = page.locator('label:has-text("Category")').locator('..').locator('input');
    await expect(categoryFilter).toBeVisible();
  });

  test('should show already added groups as disabled', async ({ page, baseURL, apiClient }) => {
    // Create block with a group already added
    await apiClient.createBlock(noteId, 'references', 'r', { groupIds: [selectableGroupId] });

    await page.goto(`${baseURL}/note?id=${noteId}`);
    await page.waitForLoadState('load');

    await page.locator('button:has-text("Edit Blocks")').click();
    await page.locator('button:has-text("Select Groups")').click();

    // Find the already-added group and check for "Added" badge
    const addedBadge = page.locator('[role="option"]').filter({ hasText: 'Selectable Test Group' }).locator('text=Added');
    await expect(addedBadge).toBeVisible();
  });

  test('should remove group from references block', async ({ page, baseURL, apiClient }) => {
    await apiClient.createBlock(noteId, 'references', 's', { groupIds: [selectableGroupId] });

    await page.goto(`${baseURL}/note?id=${noteId}`);
    await page.waitForLoadState('load');

    await page.locator('button:has-text("Edit Blocks")').click();

    // Find remove button on the group pill
    const removeButton = page.locator('.block-content').filter({ hasText: 'references' }).locator('button[title="Remove"]');
    await removeButton.click();

    // Group should be removed
    await expect(page.locator('.block-content').filter({ hasText: 'references' }).locator('text=Selectable Test Group')).not.toBeVisible();
  });

  test.afterAll(async ({ apiClient }) => {
    if (noteId) await apiClient.deleteNote(noteId);
    if (selectableGroupId) await apiClient.deleteGroup(selectableGroupId);
    if (ownerGroupId) await apiClient.deleteGroup(ownerGroupId);
    if (categoryId) await apiClient.deleteCategory(categoryId);
  });
});
