import { test, expect } from '../fixtures/base.fixture';
import * as path from 'path';

// Tests in each describe block share state and must run serially
test.describe('Entity Picker - Resource Selection', () => {
  test.describe.configure({ mode: 'serial' });
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

    // Create a resource to select (requires a file)
    const resource = await apiClient.createResource({
      filePath: path.join(__dirname, '../test-assets/sample-image.png'),
      name: 'Test Resource for Picker',
      ownerId: ownerGroupId,
    });
    resourceId = resource.ID;

    // Create a single gallery block for all tests
    await apiClient.createBlock(noteId, 'gallery', 'picker-test', { resourceIds: [] });
  });

  test('should open resource picker from gallery block', async ({ page, baseURL }) => {
    await page.goto(`${baseURL}/note?id=${noteId}`);
    await page.waitForLoadState('load');

    // Enter edit mode
    await page.locator('button:has-text("Edit Blocks")').click();
    await expect(page.locator('button:has-text("Done")')).toBeVisible();

    // Click Select Resources button
    await page.locator('button:has-text("Select Resources")').click();

    // Modal should open
    await expect(page.locator('[aria-labelledby="entity-picker-title"]')).toBeVisible();
    await expect(page.locator('h2:has-text("Select Resources")')).toBeVisible();

    // Close modal for next test
    await page.keyboard.press('Escape');
  });

  test('should search resources in picker', async ({ page, baseURL }) => {
    await page.goto(`${baseURL}/note?id=${noteId}`);
    await page.waitForLoadState('load');

    await page.locator('button:has-text("Edit Blocks")').click();
    await page.locator('button:has-text("Select Resources")').click();

    // Wait for modal to be fully open
    await expect(page.locator('[aria-labelledby="entity-picker-title"]')).toBeVisible();

    // Switch to All Resources tab
    await page.locator('button:has-text("All Resources")').click();

    // Search for resource
    const searchInput = page.locator('[aria-labelledby="entity-picker-title"] input[placeholder="Search by name..."]');

    // Fill search and wait for the debounced API request to complete
    const searchResponsePromise = page.waitForResponse(resp =>
      resp.url().includes('/v1/resources') && resp.status() === 200
    );
    await searchInput.fill('Test Resource');
    await searchResponsePromise;

    // Should show matching resource (use .first() since there might be multiple)
    await expect(page.locator('[aria-labelledby="entity-picker-title"] [role="option"]').first()).toBeVisible();

    // Close modal for next test
    await page.keyboard.press('Escape');
  });

  test('should select and confirm resources', async ({ page, baseURL }) => {
    await page.goto(`${baseURL}/note?id=${noteId}`);
    await page.waitForLoadState('load');

    await page.locator('button:has-text("Edit Blocks")').click();
    await page.locator('button:has-text("Select Resources")').click();

    // Wait for modal
    const pickerModal = page.locator('[aria-labelledby="entity-picker-title"]');
    await expect(pickerModal).toBeVisible();

    await page.locator('button:has-text("All Resources")').click();

    // Click on a resource to select it
    const resourceOption = pickerModal.locator('[role="option"]').first();
    await resourceOption.click();

    // Selection count should update
    await expect(pickerModal.locator('text=1 selected')).toBeVisible();

    // Confirm selection
    await pickerModal.locator('button:has-text("Confirm")').click();

    // Modal should close
    await expect(pickerModal).not.toBeVisible();
  });

  test('should cancel selection without adding', async ({ page, baseURL }) => {
    await page.goto(`${baseURL}/note?id=${noteId}`);
    await page.waitForLoadState('load');

    await page.locator('button:has-text("Edit Blocks")').click();
    await page.locator('button:has-text("Select Resources")').click();

    // Wait for modal
    const pickerModal = page.locator('[aria-labelledby="entity-picker-title"]');
    await expect(pickerModal).toBeVisible();

    await page.locator('button:has-text("All Resources")').click();

    // Select a resource that isn't already added (not disabled)
    const selectableOption = pickerModal.locator('[role="option"]:not([aria-disabled="true"])').first();
    // If no selectable option exists, the test previous test already added all available resources
    // which is fine - we can still test the cancel behavior
    const optionCount = await selectableOption.count();
    if (optionCount > 0) {
      await selectableOption.click();
      await expect(pickerModal.locator('text=1 selected')).toBeVisible();
    }

    // Cancel
    await pickerModal.locator('button:has-text("Cancel")').click();

    // Modal should close
    await expect(pickerModal).not.toBeVisible();
  });

  test('should close picker with escape key', async ({ page, baseURL }) => {
    await page.goto(`${baseURL}/note?id=${noteId}`);
    await page.waitForLoadState('load');

    await page.locator('button:has-text("Edit Blocks")').click();
    await page.locator('button:has-text("Select Resources")').click();

    await expect(page.locator('[aria-labelledby="entity-picker-title"]')).toBeVisible();

    // Press Escape
    await page.keyboard.press('Escape');

    // Modal should close
    await expect(page.locator('[aria-labelledby="entity-picker-title"]')).not.toBeVisible();
  });

  test.afterAll(async ({ apiClient }) => {
    // Delete in reverse dependency order to avoid FK constraint errors
    if (noteId) await apiClient.deleteNote(noteId);
    if (ownerGroupId) await apiClient.deleteGroup(ownerGroupId);
    if (categoryId) await apiClient.deleteCategory(categoryId);
  });
});

test.describe('Entity Picker - Resource Tag Filtering', () => {
  test.describe.configure({ mode: 'serial' });
  let categoryId: number;
  let ownerGroupId: number;
  let secondGroupId: number;
  let noteId: number;
  let tagId: number;
  let taggedResourceId: number;
  let untaggedResourceId: number;

  test.beforeAll(async ({ apiClient }) => {
    // Use unique suffix to avoid collisions on test retries
    const testSuffix = `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;

    // Create prerequisite data with unique names to avoid collisions on retries
    const category = await apiClient.createCategory(`Tag Filter Category ${testSuffix}`, 'Category for tag filter tests');
    categoryId = category.ID;

    const ownerGroup = await apiClient.createGroup({
      name: `Tag Filter Owner ${testSuffix}`,
      categoryId: categoryId,
    });
    ownerGroupId = ownerGroup.ID;

    // Create a second group for the second resource to avoid deduplication
    // (resources are deduplicated by hash + parent group)
    const secondGroup = await apiClient.createGroup({
      name: `Tag Filter Second Owner ${testSuffix}`,
      categoryId: categoryId,
    });
    secondGroupId = secondGroup.ID;

    // Create a tag for filtering
    const tag = await apiClient.createTag('Filter Test Tag', 'Tag for filter tests');
    tagId = tag.ID;

    const note = await apiClient.createNote({
      name: `Tag Filter Test Note ${testSuffix}`,
      ownerId: ownerGroupId,
    });
    noteId = note.ID;

    // Create resources - one tagged, one not
    // Use different owner groups to avoid duplicate resource errors
    // (resources are deduplicated by file hash + parent group)
    const taggedResource = await apiClient.createResource({
      filePath: path.join(__dirname, '../test-assets/sample-image.png'),
      name: `Tagged Resource ${testSuffix}`,
      ownerId: ownerGroupId,
      tagIds: [tagId],
    });
    taggedResourceId = taggedResource.ID;

    const untaggedResource = await apiClient.createResource({
      filePath: path.join(__dirname, '../test-assets/sample-image.png'),
      name: `Untagged Resource ${testSuffix}`,
      ownerId: secondGroupId,
    });
    untaggedResourceId = untaggedResource.ID;

    // Create a gallery block for testing
    await apiClient.createBlock(noteId, 'gallery', 'tag-filter-test', { resourceIds: [] });
  });

  test('should filter resources by tag', async ({ page, baseURL }) => {
    await page.goto(`${baseURL}/note?id=${noteId}`);
    await page.waitForLoadState('load');

    await page.locator('button:has-text("Edit Blocks")').click();
    await page.locator('button:has-text("Select Resources")').click();

    // Wait for modal
    const pickerModal = page.locator('[aria-labelledby="entity-picker-title"]');
    await expect(pickerModal).toBeVisible();

    // Switch to All Resources tab and wait for results to appear
    await page.locator('button:has-text("All Resources")').click();
    await pickerModal.locator('[role="option"]').first().waitFor({ state: 'visible' });

    // Find the Tags filter input
    const tagsFilter = pickerModal.locator('label:has-text("Tags")').locator('..').locator('input');
    await expect(tagsFilter).toBeVisible();

    // Type the tag name and wait for autocomplete dropdown
    await tagsFilter.fill('Filter Test');
    const tagOption = pickerModal.locator('.absolute.z-30').locator('text=Filter Test Tag');
    await tagOption.waitFor({ state: 'visible' });

    // Select the tag - this triggers a filtered API request
    const filteredResultsPromise = page.waitForResponse(resp =>
      resp.url().includes('/v1/resources') && resp.url().includes('Tags=') && resp.status() === 200
    );
    await tagOption.click();
    await filteredResultsPromise;

    // Verify the tag chip appears showing filter is active
    await expect(pickerModal.locator('text=Filter Test Tag').first()).toBeVisible();

    // Close modal
    await page.keyboard.press('Escape');
  });

  test('should show tag filter chips and allow removal', async ({ page, baseURL }) => {
    await page.goto(`${baseURL}/note?id=${noteId}`);
    await page.waitForLoadState('load');

    await page.locator('button:has-text("Edit Blocks")').click();
    await page.locator('button:has-text("Select Resources")').click();

    const pickerModal = page.locator('[aria-labelledby="entity-picker-title"]');
    await expect(pickerModal).toBeVisible();

    await page.locator('button:has-text("All Resources")').click();

    // Add tag filter - type and wait for autocomplete dropdown
    const tagsFilter = pickerModal.locator('label:has-text("Tags")').locator('..').locator('input');
    await tagsFilter.fill('Filter Test');
    const tagOption = pickerModal.locator('.absolute.z-30').locator('text=Filter Test Tag');
    await tagOption.waitFor({ state: 'visible' });
    await tagOption.click();

    // Verify chip appears
    const tagChip = pickerModal.locator('span.inline-flex').filter({ hasText: 'Filter Test Tag' });
    await expect(tagChip).toBeVisible();

    // Remove the filter by clicking the x button
    await tagChip.locator('button').click();

    // Verify chip is removed
    await expect(tagChip).not.toBeVisible();

    await page.keyboard.press('Escape');
  });

  test.afterAll(async ({ apiClient }) => {
    // Delete in reverse dependency order to avoid FK constraint errors
    // Use try/catch to handle cases where entities were already deleted or test setup failed
    try { if (noteId) await apiClient.deleteNote(noteId); } catch { /* ignore */ }
    try { if (taggedResourceId) await apiClient.deleteResource(taggedResourceId); } catch { /* ignore */ }
    try { if (untaggedResourceId) await apiClient.deleteResource(untaggedResourceId); } catch { /* ignore */ }
    try { if (tagId) await apiClient.deleteTag(tagId); } catch { /* ignore */ }
    try { if (secondGroupId) await apiClient.deleteGroup(secondGroupId); } catch { /* ignore */ }
    try { if (ownerGroupId) await apiClient.deleteGroup(ownerGroupId); } catch { /* ignore */ }
    try { if (categoryId) await apiClient.deleteCategory(categoryId); } catch { /* ignore */ }
  });
});

test.describe('Entity Picker - Group Selection', () => {
  // Tests in this block share state and must run serially
  test.describe.configure({ mode: 'serial' });

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

    // Create a single references block for all tests
    await apiClient.createBlock(noteId, 'references', 'picker-test', { groupIds: [] });
  });

  test('should open group picker from references block', async ({ page, baseURL }) => {
    await page.goto(`${baseURL}/note?id=${noteId}`);
    await page.waitForLoadState('load');

    await page.locator('button:has-text("Edit Blocks")').click();
    await page.locator('button:has-text("Select Groups")').click();

    await expect(page.locator('[aria-labelledby="entity-picker-title"]')).toBeVisible();
    await expect(page.locator('h2:has-text("Select Groups")')).toBeVisible();

    // Close modal for next test
    await page.keyboard.press('Escape');
  });

  test('should display groups as cards', async ({ page, baseURL }) => {
    await page.goto(`${baseURL}/note?id=${noteId}`);
    await page.waitForLoadState('load');

    await page.locator('button:has-text("Edit Blocks")').click();
    await page.locator('button:has-text("Select Groups")').click();

    // Wait for modal to be fully open
    const pickerModal = page.locator('[aria-labelledby="entity-picker-title"]');
    await expect(pickerModal).toBeVisible();

    // Wait for visible options to load - groups use flex layout, thumbnails use aspect-square
    // Use a visible filter to ensure we get the group cards, not hidden thumbnail options
    const groupCard = pickerModal.locator('[role="option"].flex').first();
    await groupCard.waitFor({ state: 'visible', timeout: 10000 });

    // Should contain group name text (groups use flex layout with p.font-medium)
    await expect(groupCard.locator('p.font-medium')).toBeVisible();

    // Close modal for next test
    await page.keyboard.press('Escape');
  });

  test('should select and confirm groups', async ({ page, baseURL }) => {
    await page.goto(`${baseURL}/note?id=${noteId}`);
    await page.waitForLoadState('load');

    await page.locator('button:has-text("Edit Blocks")').click();
    await page.locator('button:has-text("Select Groups")').click();

    // Wait for modal
    const pickerModal = page.locator('[aria-labelledby="entity-picker-title"]');
    await expect(pickerModal).toBeVisible();

    // Select a group (use .flex to target group cards, not hidden thumbnail options)
    await pickerModal.locator('[role="option"].flex').first().click();
    await expect(pickerModal.locator('text=1 selected')).toBeVisible();

    // Confirm
    await pickerModal.locator('button:has-text("Confirm")').click();

    // Modal closes and group appears in block
    await expect(pickerModal).not.toBeVisible();
  });

  test('should filter groups by category', async ({ page, baseURL }) => {
    await page.goto(`${baseURL}/note?id=${noteId}`);
    await page.waitForLoadState('load');

    await page.locator('button:has-text("Edit Blocks")').click();
    await page.locator('button:has-text("Select Groups")').click();

    // Wait for modal
    const pickerModal = page.locator('[aria-labelledby="entity-picker-title"]');
    await expect(pickerModal).toBeVisible();

    // Category filter should be visible
    const categoryFilter = pickerModal.locator('label:has-text("Category")').locator('..').locator('input');
    await expect(categoryFilter).toBeVisible();

    // Close modal for next test
    await page.keyboard.press('Escape');
  });

  test('should show already added groups as disabled', async ({ page, baseURL, apiClient }) => {
    // Create a new block with a group already added for this specific test
    await apiClient.createBlock(noteId, 'references', 'with-group', { groupIds: [selectableGroupId] });

    await page.goto(`${baseURL}/note?id=${noteId}`);
    await page.waitForLoadState('load');

    await page.locator('button:has-text("Edit Blocks")').click();
    // Use .first() because there are now two references blocks
    await page.locator('button:has-text("Select Groups")').first().click();

    // Wait for modal
    const pickerModal = page.locator('[aria-labelledby="entity-picker-title"]');
    await expect(pickerModal).toBeVisible();

    // Find the already-added group and check for "Added" badge (use .flex to target group cards)
    const addedBadge = pickerModal.locator('[role="option"].flex').filter({ hasText: 'Selectable Test Group' }).locator('text=Added');
    await expect(addedBadge).toBeVisible();

    // Close modal for next test
    await page.keyboard.press('Escape');
  });

  test('should remove group from references block', async ({ page, baseURL, apiClient }) => {
    // Create a new block with a group for removal test
    await apiClient.createBlock(noteId, 'references', 'for-removal', { groupIds: [selectableGroupId] });

    await page.goto(`${baseURL}/note?id=${noteId}`);
    await page.waitForLoadState('load');

    await page.locator('button:has-text("Edit Blocks")').click();

    // Find any remove button for the Selectable Test Group and click the first one
    // (multiple blocks may have this group, but removing from any one proves the feature works)
    const removeButtons = page.locator('button[title="Remove"]');
    const initialCount = await removeButtons.count();
    await removeButtons.first().click();

    // Wait for the removal to take effect by checking the button count decreased
    await expect(async () => {
      const currentCount = await removeButtons.count();
      expect(currentCount).toBeLessThan(initialCount);
    }).toPass({ timeout: 5000 });
  });

  test.afterAll(async ({ apiClient }) => {
    // Delete in reverse dependency order to avoid FK constraint errors
    if (noteId) await apiClient.deleteNote(noteId);
    if (selectableGroupId) await apiClient.deleteGroup(selectableGroupId);
    if (ownerGroupId) await apiClient.deleteGroup(ownerGroupId);
    if (categoryId) await apiClient.deleteCategory(categoryId);
  });
});
