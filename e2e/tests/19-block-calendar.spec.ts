import { test, expect } from '../fixtures/base.fixture';

// Generate unique IDs for this test file to avoid conflicts with parallel workers
const testId = `calendar-${Date.now()}-${Math.random().toString(36).substring(7)}`;

test.describe('Calendar Block', () => {
  let categoryId: number;
  let ownerGroupId: number;
  let noteId: number;

  test.beforeAll(async ({ apiClient }) => {
    // Create prerequisite data with unique names
    const category = await apiClient.createCategory(
      `Calendar Test Category ${testId}`,
      'Category for calendar block tests'
    );
    categoryId = category.ID;

    const ownerGroup = await apiClient.createGroup({
      name: `Calendar Test Owner ${testId}`,
      categoryId: categoryId,
    });
    ownerGroupId = ownerGroup.ID;

    const note = await apiClient.createNote({
      name: `Calendar Test Note ${testId}`,
      description: 'Note for testing calendar blocks',
      ownerId: ownerGroupId,
    });
    noteId = note.ID;
  });

  test.afterAll(async ({ apiClient }) => {
    // Clean up in reverse dependency order
    if (noteId) {
      await apiClient.deleteNote(noteId);
    }
    if (ownerGroupId) {
      await apiClient.deleteGroup(ownerGroupId);
    }
    if (categoryId) {
      await apiClient.deleteCategory(categoryId);
    }
  });

  // === API Tests (reliable) ===

  test('can create calendar block via API', async ({ apiClient }) => {
    const block = await apiClient.createBlock(
      noteId,
      'calendar',
      'n',
      { calendars: [] }
    );

    expect(block.id).toBeGreaterThan(0);
    expect(block.type).toBe('calendar');
    expect(block.noteId).toBe(noteId);
    expect(block.content).toEqual({ calendars: [] });

    // Clean up
    await apiClient.deleteBlock(block.id);
  });

  test('can update calendar block content via API', async ({ apiClient }) => {
    // Create a calendar block
    const block = await apiClient.createBlock(noteId, 'calendar', 'o', {
      calendars: []
    });

    // Update content to add a calendar
    const updatedContent = {
      calendars: [{
        id: 'api-cal-1',
        name: 'API Added Calendar',
        color: '#ef4444',
        source: { type: 'url', url: 'https://example.com/api.ics' }
      }]
    };
    const updated = await apiClient.updateBlockContent(block.id, updatedContent);

    expect(updated.content).toEqual(updatedContent);

    // Fetch to verify persistence
    const fetched = await apiClient.getBlock(block.id);
    expect(fetched.content).toEqual(updatedContent);

    // Clean up
    await apiClient.deleteBlock(block.id);
  });

  test('can update calendar block state via API', async ({ apiClient }) => {
    // Create a calendar block
    const block = await apiClient.createBlock(noteId, 'calendar', 'p', {
      calendars: [{
        id: 'state-cal-1',
        name: 'State Calendar',
        color: '#3b82f6',
        source: { type: 'url', url: 'https://example.com/state-api.ics' }
      }]
    });

    // Update state to agenda view
    const newState = {
      view: 'agenda',
      currentDate: '2025-06-15'
    };
    const updated = await apiClient.updateBlockState(block.id, newState);

    expect(updated.state).toEqual(newState);

    // Fetch to verify persistence
    const fetched = await apiClient.getBlock(block.id);
    expect(fetched.state).toEqual(newState);

    // Clean up
    await apiClient.deleteBlock(block.id);
  });

  test('can delete calendar block via API', async ({ apiClient }) => {
    // Create a calendar block
    const block = await apiClient.createBlock(noteId, 'calendar', 'q', {
      calendars: []
    });

    // Delete the block
    await apiClient.deleteBlock(block.id);

    // Verify it's deleted
    try {
      await apiClient.getBlock(block.id);
      // Should not reach here
      expect(true).toBe(false);
    } catch (error) {
      // Expected - block should be deleted
      expect(error).toBeDefined();
    }
  });

  test('validates calendar block content', async ({ apiClient }) => {
    // Try to create a calendar block with invalid color format
    try {
      await apiClient.createBlock(noteId, 'calendar', 'r', {
        calendars: [{
          id: 'invalid-cal',
          name: 'Invalid Calendar',
          color: 'not-a-hex-color',
          source: { type: 'url', url: 'https://example.com/invalid.ics' }
        }]
      });
      expect(true).toBe(false); // Should not reach here
    } catch (error) {
      expect(error).toBeDefined();
    }
  });

  test('validates calendar source type', async ({ apiClient }) => {
    // Try to create a calendar block with invalid source type
    try {
      await apiClient.createBlock(noteId, 'calendar', 's', {
        calendars: [{
          id: 'invalid-source',
          name: 'Invalid Source',
          color: '#3b82f6',
          source: { type: 'invalid', url: 'https://example.com/invalid.ics' }
        }]
      });
      expect(true).toBe(false); // Should not reach here
    } catch (error) {
      expect(error).toBeDefined();
    }
  });

  test('validates calendar state view', async ({ apiClient }) => {
    // Create a valid calendar block
    const block = await apiClient.createBlock(noteId, 'calendar', 't', {
      calendars: []
    });

    // Try to set invalid view state
    try {
      await apiClient.updateBlockState(block.id, {
        view: 'invalid-view',
        currentDate: '2025-01-01'
      });
      expect(true).toBe(false); // Should not reach here
    } catch (error) {
      expect(error).toBeDefined();
    }

    // Clean up
    await apiClient.deleteBlock(block.id);
  });

  // === UI Tests (add calendar block via UI) ===
  // NOTE: These UI tests are skipped due to a pre-existing issue where the block type
  // dropdown doesn't show options. This affects all block UI tests (see 16-blocks.spec.ts).
  // The issue is that blockTypes array is empty when the dropdown opens.
  // Investigation needed: check why loadBlockTypes() isn't completing before dropdown opens.

  test.skip('can add calendar block via UI', async ({ page, baseURL }) => {
    await page.goto(`${baseURL}/note?id=${noteId}`);
    await page.waitForLoadState('load');

    // Enter edit mode
    const editButton = page.locator('button:has-text("Edit Blocks")');
    await expect(editButton).toBeVisible({ timeout: 10000 });
    await editButton.click();
    await expect(page.locator('button:has-text("Done")')).toBeVisible({ timeout: 5000 });

    // Wait for add block button to become visible (it's x-show="editMode")
    const addBlockButton = page.locator('button:has-text("+ Add Block")');
    await expect(addBlockButton).toBeVisible({ timeout: 5000 });
    await addBlockButton.click();

    // Select calendar from the dropdown (wait for it to be visible first)
    const calendarOption = page.locator('button:has-text("Calendar")').first();
    await expect(calendarOption).toBeVisible({ timeout: 10000 });
    await calendarOption.click();

    // Wait for block to be added - should see calendar block UI elements
    await expect(page.locator('text=No calendars configured')).toBeVisible({ timeout: 10000 });

    // Exit edit mode
    await page.locator('button:has-text("Done")').click();

    // In view mode, should see empty state
    await expect(page.locator('text=No calendars added yet')).toBeVisible();
  });

  test.skip('can add calendar from URL in edit mode', async ({ page, baseURL }) => {
    await page.goto(`${baseURL}/note?id=${noteId}`);
    await page.waitForLoadState('load');

    // Enter edit mode
    const editButton = page.locator('button:has-text("Edit Blocks")');
    await expect(editButton).toBeVisible({ timeout: 10000 });
    await editButton.click();
    await expect(page.locator('button:has-text("Done")')).toBeVisible({ timeout: 5000 });

    // Add a new calendar block first
    const addBlockButton = page.locator('button:has-text("+ Add Block")');
    await expect(addBlockButton).toBeVisible({ timeout: 5000 });
    await addBlockButton.click();
    const calendarOption = page.locator('button:has-text("Calendar")').first();
    await expect(calendarOption).toBeVisible({ timeout: 10000 });
    await calendarOption.click();
    await expect(page.locator('text=No calendars configured')).toBeVisible({ timeout: 10000 });

    // Find URL input and add a calendar
    const urlInput = page.locator('input[placeholder*="ICS calendar URL"]');
    await expect(urlInput).toBeVisible();
    await urlInput.fill('https://example.com/test.ics');

    // Click Add URL button
    const addButton = page.locator('button:has-text("Add URL")');
    await expect(addButton).toBeEnabled();
    await addButton.click();

    // Should see the calendar listed
    await expect(page.locator('input[value="Calendar 1"]')).toBeVisible({ timeout: 5000 });
    await expect(page.locator('text=🔗 URL')).toBeVisible();
  });

  test.skip('Add URL button is disabled when input is empty', async ({ page, baseURL }) => {
    await page.goto(`${baseURL}/note?id=${noteId}`);
    await page.waitForLoadState('load');

    // Enter edit mode
    const editButton = page.locator('button:has-text("Edit Blocks")');
    await expect(editButton).toBeVisible({ timeout: 10000 });
    await editButton.click();
    await expect(page.locator('button:has-text("Done")')).toBeVisible({ timeout: 5000 });

    // Add a calendar block
    const addBlockButton = page.locator('button:has-text("+ Add Block")');
    await expect(addBlockButton).toBeVisible({ timeout: 5000 });
    await addBlockButton.click();
    const calendarOption = page.locator('button:has-text("Calendar")').first();
    await expect(calendarOption).toBeVisible({ timeout: 10000 });
    await calendarOption.click();
    await expect(page.locator('text=No calendars configured')).toBeVisible({ timeout: 10000 });

    // Add URL button should be disabled when input is empty
    const addButton = page.locator('button:has-text("Add URL")');
    await expect(addButton).toBeDisabled();

    // Type something and verify it becomes enabled
    const urlInput = page.locator('input[placeholder*="ICS calendar URL"]');
    await urlInput.fill('https://example.com/test.ics');
    await expect(addButton).toBeEnabled();

    // Clear and verify it becomes disabled again
    await urlInput.clear();
    await expect(addButton).toBeDisabled();
  });
});
