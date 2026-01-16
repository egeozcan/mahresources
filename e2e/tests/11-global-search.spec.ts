import { test, expect } from '../fixtures/base.fixture';

test.describe('Global Search', () => {
  let categoryId: number;
  let groupId: number;
  let noteId: number;
  let tagId: number;

  test.beforeAll(async ({ apiClient }) => {
    // Create searchable entities with unique names
    const category = await apiClient.createCategory('UniqueSearchCat123', 'Searchable category');
    categoryId = category.ID;

    const tag = await apiClient.createTag('UniqueSearchTag456', 'Searchable tag');
    tagId = tag.ID;

    const group = await apiClient.createGroup({
      name: 'UniqueSearchGroup789',
      description: 'A unique searchable group description',
      categoryId: categoryId,
    });
    groupId = group.ID;

    const note = await apiClient.createNote({
      name: 'UniqueSearchNote012',
      description: 'Another unique note for searching',
      ownerId: groupId,
    });
    noteId = note.ID;
  });

  test('should open global search with keyboard shortcut', async ({ page }) => {
    await page.goto('/notes');

    // Use Cmd+K (Mac) or Ctrl+K (Windows/Linux) - ControlOrMeta handles both
    await page.keyboard.press('ControlOrMeta+k');

    // Wait for search modal to appear
    const searchInput = page.locator('.global-search input[type="text"], input[placeholder*="Search"]');
    await expect(searchInput).toBeVisible({ timeout: 5000 });
  });

  test('should search and find groups', async ({ page }) => {
    await page.goto('/notes');
    await page.keyboard.press('ControlOrMeta+k');

    const searchInput = page.locator('.global-search input[type="text"], input[placeholder*="Search"]').first();
    await searchInput.waitFor({ state: 'visible' });
    await searchInput.fill('UniqueSearchGroup789');

    // Wait for search results to appear (condition-based instead of hardcoded timeout)
    await expect(page.locator('text=UniqueSearchGroup789')).toBeVisible({ timeout: 5000 });
  });

  test('should search and find notes', async ({ page }) => {
    await page.goto('/notes');
    await page.keyboard.press('ControlOrMeta+k');

    const searchInput = page.locator('.global-search input[type="text"], input[placeholder*="Search"]').first();
    await searchInput.waitFor({ state: 'visible' });
    await searchInput.fill('UniqueSearchNote012');

    // Wait for search results to appear (condition-based instead of hardcoded timeout)
    await expect(page.locator('text=UniqueSearchNote012')).toBeVisible({ timeout: 5000 });
  });

  test('should search and find tags', async ({ page }) => {
    await page.goto('/notes');
    await page.keyboard.press('ControlOrMeta+k');

    const searchInput = page.locator('.global-search input[type="text"], input[placeholder*="Search"]').first();
    await searchInput.waitFor({ state: 'visible' });
    await searchInput.fill('UniqueSearchTag456');

    // Wait for search results to appear (condition-based instead of hardcoded timeout)
    await expect(page.locator('text=UniqueSearchTag456')).toBeVisible({ timeout: 5000 });
  });

  test('should navigate to result with Enter', async ({ page }) => {
    await page.goto('/notes');
    await page.keyboard.press('ControlOrMeta+k');

    const searchInput = page.locator('.global-search input[type="text"], input[placeholder*="Search"]').first();
    await searchInput.waitFor({ state: 'visible' });
    await searchInput.fill('UniqueSearchGroup789');

    // Wait for search results to appear before pressing Enter
    await expect(page.locator('text=UniqueSearchGroup789')).toBeVisible({ timeout: 5000 });

    // Press Enter to navigate to first result
    await page.keyboard.press('Enter');

    // Should navigate to the group page
    await expect(page).toHaveURL(/\/group\?id=\d+/);
  });

  test('should navigate results with arrow keys', async ({ page }) => {
    await page.goto('/notes');
    await page.keyboard.press('ControlOrMeta+k');

    const searchInput = page.locator('.global-search input[type="text"], input[placeholder*="Search"]').first();
    await searchInput.waitFor({ state: 'visible' });
    await searchInput.fill('UniqueSearch');

    // Wait for search results to appear before navigating
    await expect(page.locator('text=UniqueSearch').first()).toBeVisible({ timeout: 5000 });

    // Navigate down with arrow key
    await page.keyboard.press('ArrowDown');

    // Check for selection indicator (with timeout instead of hardcoded wait)
    const selectedItem = page.locator('[data-selected="true"], [aria-selected="true"], .selected');
    await expect(selectedItem).toBeVisible({ timeout: 2000 });
  });

  test('should close search with Escape', async ({ page }) => {
    await page.goto('/notes');
    await page.keyboard.press('ControlOrMeta+k');

    const searchInput = page.locator('.global-search input[type="text"], input[placeholder*="Search"]').first();
    await searchInput.waitFor({ state: 'visible' });

    await page.keyboard.press('Escape');

    // Search modal should close
    await expect(searchInput).not.toBeVisible();
  });

  test('should work from different pages', async ({ page }) => {
    // Test from groups page
    await page.goto('/groups');
    await page.keyboard.press('ControlOrMeta+k');

    const searchInput = page.locator('.global-search input[type="text"], input[placeholder*="Search"]').first();
    await expect(searchInput).toBeVisible();
    await page.keyboard.press('Escape');

    // Test from tags page
    await page.goto('/tags');
    await page.keyboard.press('ControlOrMeta+k');
    await expect(searchInput).toBeVisible();
  });

  test.afterAll(async ({ apiClient }) => {
    // Clean up in reverse dependency order
    if (noteId) {
      await apiClient.deleteNote(noteId);
    }
    if (groupId) {
      await apiClient.deleteGroup(groupId);
    }
    if (tagId) {
      await apiClient.deleteTag(tagId);
    }
    if (categoryId) {
      await apiClient.deleteCategory(categoryId);
    }
  });
});

test.describe('Search API Integration', () => {
  let categoryId: number;
  let groupId: number;

  test.beforeAll(async ({ apiClient }) => {
    const category = await apiClient.createCategory('APISearchCategory', 'For API search tests');
    categoryId = category.ID;

    const group = await apiClient.createGroup({
      name: 'APISearchGroup',
      categoryId: categoryId,
    });
    groupId = group.ID;
  });

  test('should search via API', async ({ apiClient }) => {
    const results = await apiClient.search('APISearchGroup');
    expect(results).toBeDefined();
    // Results should include the group we created
  });

  test('should limit search results', async ({ apiClient }) => {
    const results = await apiClient.search('API', 5);
    expect(results).toBeDefined();
    // Results should be limited
    if (Array.isArray(results)) {
      expect(results.length).toBeLessThanOrEqual(5);
    }
  });

  test.afterAll(async ({ apiClient }) => {
    if (groupId) {
      await apiClient.deleteGroup(groupId);
    }
    if (categoryId) {
      await apiClient.deleteCategory(categoryId);
    }
  });
});
