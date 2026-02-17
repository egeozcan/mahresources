/**
 * Accessibility tests for interactive components
 *
 * Tests various component states for WCAG 2.1 Level AA compliance.
 * Components are tested in their different states (closed, open, with data, etc.)
 */
import { test, expect } from '../../fixtures/a11y.fixture';

test.describe('Component Accessibility - Global Search', () => {
  test('Global Search button should be accessible', async ({ page, checkA11y }) => {
    await page.goto('/notes');
    await page.waitForLoadState('load');

    // Check the search trigger button is accessible
    const searchButton = page.locator('[aria-label="Open search dialog"]');
    await expect(searchButton).toBeVisible();

    // Full page check includes the search button
    await checkA11y();
  });

  test('Global Search dialog (open) should be accessible', async ({ page, checkA11y }) => {
    await page.goto('/notes');
    await page.waitForLoadState('load');

    // Open search with keyboard shortcut
    await page.keyboard.press('Meta+k');

    // Wait for dialog to open and input to be visible
    await page.waitForSelector('input[role="combobox"]', { timeout: 5000 });

    // Check the open search dialog
    await checkA11y();
  });

  test('Global Search with results should be accessible', async ({ page, checkA11y, a11yTestData }) => {
    await page.goto('/notes');
    await page.waitForLoadState('load');

    // Open search
    await page.keyboard.press('Meta+k');
    // Use more specific selector for global search (aria-label="Search")
    const searchInput = page.locator('.global-search input[aria-label="Search"]');
    await searchInput.waitFor({ state: 'visible', timeout: 5000 });

    // Type to search - use the test note name
    await searchInput.fill('A11y Test');

    // Wait for results to load
    await page.waitForSelector('[role="option"]', { timeout: 5000 });

    // Check accessibility with results
    await checkA11y();
  });

  test('Global Search with no results should be accessible', async ({ page, checkA11y }) => {
    await page.goto('/notes');
    await page.waitForLoadState('load');

    // Open search
    await page.keyboard.press('Meta+k');
    // Use more specific selector for global search (aria-label="Search")
    const searchInput = page.locator('.global-search input[aria-label="Search"]');
    await searchInput.waitFor({ state: 'visible', timeout: 5000 });

    // Type something that won't match
    await searchInput.fill('xyznonexistent123');

    // Wait for "no results" state
    await page.waitForSelector('text=No results found', { timeout: 5000 });

    await checkA11y();
  });
});

test.describe('Component Accessibility - Autocompleter/Dropdown', () => {
  test('Tag autocompleter on create note form should be accessible', async ({ page, checkA11y }) => {
    await page.goto('/note/new');
    await page.waitForLoadState('load');

    // The form with autocompleter should be accessible
    await checkA11y();
  });

  test('Group autocompleter on create note form should be accessible', async ({ page, checkA11y }) => {
    await page.goto('/note/new');
    await page.waitForLoadState('load');

    // Click on a tag/group autocompleter to expand it
    const autocompleterInput = page.locator('input[placeholder*="tag"], input[placeholder*="group"]').first();
    if (await autocompleterInput.isVisible()) {
      await autocompleterInput.click();
      // Give dropdown time to open
      await page.waitForTimeout(300);
    }

    await checkA11y();
  });
});

test.describe('Component Accessibility - Bulk Selection', () => {
  test('Bulk selection toolbar (no items selected) should be accessible', async ({ page, checkA11y }) => {
    await page.goto('/notes');
    await page.waitForLoadState('load');

    // Bulk selection is present on list pages
    await checkA11y();
  });

  test('Group list with bulk selection should be accessible', async ({ page, checkA11y }) => {
    await page.goto('/groups');
    await page.waitForLoadState('load');

    await checkA11y();
  });

  test('Resource list with bulk selection should be accessible', async ({ page, checkA11y }) => {
    await page.goto('/resources');
    await page.waitForLoadState('load');

    await checkA11y();
  });
});

test.describe('Component Accessibility - Web Components', () => {
  test('Expandable text on note detail should be accessible', async ({ page, checkA11y, a11yTestData }) => {
    await page.goto(`/note?id=${a11yTestData.noteId}`);
    await page.waitForLoadState('load');

    // Check if expandable-text exists on the page
    const expandableText = page.locator('expandable-text');
    const hasExpandable = await expandableText.count() > 0;

    if (hasExpandable) {
      await expect(expandableText.first()).toBeVisible();
    }

    await checkA11y();
  });

  test('Inline edit on note detail should be accessible', async ({ page, checkA11y, a11yTestData }) => {
    await page.goto(`/note?id=${a11yTestData.noteId}`);
    await page.waitForLoadState('load');

    // Check if inline-edit exists
    const inlineEdit = page.locator('inline-edit');
    const hasInlineEdit = await inlineEdit.count() > 0;

    if (hasInlineEdit) {
      await expect(inlineEdit.first()).toBeVisible();
    }

    await checkA11y();
  });

  test('Inline edit on group detail should be accessible', async ({ page, checkA11y, a11yTestData }) => {
    await page.goto(`/group?id=${a11yTestData.groupId}`);
    await page.waitForLoadState('load');

    await checkA11y();
  });
});

test.describe('Component Accessibility - Confirm Action', () => {
  test('Delete button with confirm action should be accessible', async ({ page, checkA11y, a11yTestData }) => {
    // Visit a detail page that has delete functionality
    await page.goto(`/note?id=${a11yTestData.noteId}`);
    await page.waitForLoadState('load');

    // Check if there's a delete/confirm action button
    const confirmActionButton = page.locator('[x-data*="confirmAction"], button:has-text("Delete")');
    const hasConfirmAction = await confirmActionButton.count() > 0;

    if (hasConfirmAction) {
      // The button itself should be accessible
      await checkA11y();
    } else {
      // Page still needs to be accessible
      await checkA11y();
    }
  });
});

test.describe('Component Accessibility - Forms', () => {
  test('Note creation form should have proper labels', async ({ page, checkA11y }) => {
    await page.goto('/note/new');
    await page.waitForLoadState('load');

    // Verify form elements have labels
    const formInputs = page.locator('input:not([type="hidden"]), textarea, select');
    const inputCount = await formInputs.count();

    // Should have at least name field
    expect(inputCount).toBeGreaterThan(0);

    await checkA11y();
  });

  test('Group creation form should have proper labels', async ({ page, checkA11y }) => {
    await page.goto('/group/new');
    await page.waitForLoadState('load');

    await checkA11y();
  });

  test('Resource creation form should have proper labels', async ({ page, checkA11y }) => {
    await page.goto('/resource/new');
    await page.waitForLoadState('load');

    await checkA11y();
  });

  test('Category creation form should have proper labels', async ({ page, checkA11y }) => {
    await page.goto('/category/new');
    await page.waitForLoadState('load');

    await checkA11y();
  });

  test('Query creation form should have proper labels', async ({ page, checkA11y }) => {
    await page.goto('/query/new');
    await page.waitForLoadState('load');

    await checkA11y();
  });

  test('Tag creation form should have proper labels', async ({ page, checkA11y }) => {
    await page.goto('/tag/new');
    await page.waitForLoadState('load');

    await checkA11y();
  });
});

test.describe('Component Accessibility - Navigation', () => {
  test('Main navigation should be accessible', async ({ page, checkA11y }) => {
    await page.goto('/notes');
    await page.waitForLoadState('load');

    // Check the nav element exists
    const nav = page.locator('nav');
    const hasNav = await nav.count() > 0;

    if (hasNav) {
      await expect(nav.first()).toBeVisible();
    }

    await checkA11y();
  });

  test('Page with sidebar/navigation should be accessible', async ({ page, checkA11y, a11yTestData }) => {
    // Group detail often has navigation elements
    await page.goto(`/group?id=${a11yTestData.groupId}`);
    await page.waitForLoadState('load');

    await checkA11y();
  });
});

test.describe('Component Accessibility - Tables and Lists', () => {
  test('Notes list table should be accessible', async ({ page, checkA11y }) => {
    await page.goto('/notes');
    await page.waitForLoadState('load');

    await checkA11y();
  });

  test('Groups list table should be accessible', async ({ page, checkA11y }) => {
    await page.goto('/groups');
    await page.waitForLoadState('load');

    await checkA11y();
  });

  test('Resources list (detailed view) should be accessible', async ({ page, checkA11y }) => {
    await page.goto('/resources/details');
    await page.waitForLoadState('load');

    await checkA11y();
  });

  test('Tags list should be accessible', async ({ page, checkA11y }) => {
    await page.goto('/tags');
    await page.waitForLoadState('load');

    await checkA11y();
  });
});

test.describe('Component Accessibility - Entity Picker', () => {
  const testRunId = Date.now() + Math.floor(Math.random() * 100000);
  let pickerCategoryId: number;
  let pickerGroupId: number;
  let pickerResourceId: number;

  test.beforeAll(async ({ apiClient }) => {
    // Create shared infrastructure (category, group, resource) once
    const category = await apiClient.createCategory(
      `Picker A11y Category ${testRunId}`,
      'For picker a11y tests'
    );
    pickerCategoryId = category.ID;

    const group = await apiClient.createGroup({
      name: `Picker A11y Group ${testRunId}`,
      categoryId: category.ID,
    });
    pickerGroupId = group.ID;

    const path = await import('path');
    const resource = await apiClient.createResource({
      filePath: path.join(__dirname, '../../test-assets/sample-image.png'),
      name: `Picker A11y Test Resource ${testRunId}`,
      ownerId: group.ID,
    });
    pickerResourceId = resource.ID;
  });

  /**
   * Create a fresh note with blocks for a single test.
   * Each test gets its own note to avoid shared-state flakiness
   * where a note could disappear between serial tests under concurrent load.
   */
  async function createPickerNote(apiClient: { createNote: Function; createBlock: Function }): Promise<number> {
    const note = await apiClient.createNote({
      name: `Picker A11y Note ${Date.now()}`,
      description: 'Note for picker accessibility tests',
      ownerId: pickerGroupId,
    });
    await apiClient.createBlock(note.ID, 'gallery', `gallery-${Date.now()}`, { resourceIds: [] });
    await apiClient.createBlock(note.ID, 'references', `refs-${Date.now()}`, { groupIds: [] });
    return note.ID;
  }

  test('Entity picker modal should be accessible', async ({ page, checkA11y, apiClient }) => {
    const noteId = await createPickerNote(apiClient);
    try {
      await page.goto(`/note?id=${noteId}`);
      await page.waitForLoadState('load');

      const editBlocksBtn = page.locator('button:has-text("Edit Blocks")');
      await editBlocksBtn.waitFor({ state: 'visible', timeout: 30000 });
      await editBlocksBtn.click();
      await page.locator('button:has-text("Select Resources")').click();

      const dialog = page.locator('[aria-labelledby="entity-picker-title"]');
      await dialog.waitFor({ state: 'visible', timeout: 5000 });

      await checkA11y({ include: ['[aria-labelledby="entity-picker-title"]'] });
    } finally {
      await apiClient.deleteNote(noteId).catch(() => {});
    }
  });

  test('Entity picker with search results should be accessible', async ({ page, checkA11y, apiClient }) => {
    const noteId = await createPickerNote(apiClient);
    try {
      await page.goto(`/note?id=${noteId}`);
      await page.waitForLoadState('load');

      const editBlocksBtn = page.locator('button:has-text("Edit Blocks")');
      await editBlocksBtn.waitFor({ state: 'visible', timeout: 30000 });
      await editBlocksBtn.click();
      await page.locator('button:has-text("Select Resources")').click();

      await page.locator('button:has-text("All Resources")').click();
      await page.waitForSelector('[role="option"]', { timeout: 5000 });

      await checkA11y({ include: ['[aria-labelledby="entity-picker-title"]'] });
    } finally {
      await apiClient.deleteNote(noteId).catch(() => {});
    }
  });

  test('Group picker modal should be accessible', async ({ page, checkA11y, apiClient }) => {
    const noteId = await createPickerNote(apiClient);
    try {
      await page.goto(`/note?id=${noteId}`);
      await page.waitForLoadState('load');

      const editBlocksBtn = page.locator('button:has-text("Edit Blocks")');
      await editBlocksBtn.waitFor({ state: 'visible', timeout: 30000 });
      await editBlocksBtn.click();
      await page.locator('button:has-text("Select Groups")').click();

      const dialog = page.locator('[aria-labelledby="entity-picker-title"]');
      await dialog.waitFor({ state: 'visible', timeout: 5000 });

      await checkA11y({ include: ['[aria-labelledby="entity-picker-title"]'] });
    } finally {
      await apiClient.deleteNote(noteId).catch(() => {});
    }
  });

  test.afterAll(async ({ apiClient }) => {
    try {
      if (pickerResourceId) await apiClient.deleteResource(pickerResourceId);
    } catch { /* ignore cleanup errors */ }
    try {
      if (pickerGroupId) await apiClient.deleteGroup(pickerGroupId);
    } catch { /* ignore cleanup errors */ }
    try {
      if (pickerCategoryId) await apiClient.deleteCategory(pickerCategoryId);
    } catch { /* ignore cleanup errors */ }
  });
});
